//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/agentruntime"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	bf "github.com/karadjev/AI-Business-Assistant/apps/api/internal/businessfoundation"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/orders"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/db"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/migrate"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/servicebooking"
)

func testStage262Industry(t *testing.T, d *db.Database, admin *pgx.Conn, ctx context.Context) {
	svc := agentruntime.Service{Pool: d.Pool, Key: bytes.Repeat([]byte{108}, 32)}
	identity := auth.New(d.Pool, config.Auth{SessionTTL: time.Hour, IdleTTL: time.Hour})
	owner := seedStage23Owner(t, d, admin, ctx, identity, 75)
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := admin.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("UPDATE platform.organizations SET industry_profile='building_materials' WHERE id=$1", owner.tenant.OrganizationID)
	exec(`UPDATE platform.product_variants SET attributes='{"selling_unit":"package","package_quantity":"8"}' WHERE organization_id=$1 AND id=$2`, owner.tenant.OrganizationID, owner.product.VariantID)
	exec("UPDATE platform.inventory SET quantity_on_hand=10,manual_updated_at=now() WHERE organization_id=$1 AND variant_id=$2", owner.tenant.OrganizationID, owner.product.VariantID)
	length := func(n string) bf.ConstructionMeasure { return bf.ConstructionMeasure{Value: n, Unit: "m"} }
	input := bf.ConstructionInput{Operation: bf.ConstructionTiles, Length: length("9"), Width: length("5"), ItemLength: length("1"), ItemWidth: length("1"), WastePercent: "0"}
	submit := func(calc bf.ConstructionInput) string {
		t.Helper()
		id, err := svc.SubmitConstruction(ctx, owner.tenant, agentruntime.IndustryDelegationRequest[agentruntime.ConstructionRequirements]{
			ConversationID: owner.conversation, IdempotencyKey: auth.UUID(), Requirements: agentruntime.ConstructionRequirements{SKU: owner.product.SKU, Calculation: &calc}})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	step := func(n int) {
		t.Helper()
		for i := 0; i < n; i++ {
			v, err := svc.ProcessOne(ctx)
			if !v || err != nil {
				t.Fatalf("step %d: %t %v", i, v, err)
			}
		}
	}
	type result struct {
		State    string
		Proposal *agentruntime.WorkflowDraft
	}
	load := func(id string) result {
		t.Helper()
		var status string
		var raw []byte
		err := admin.QueryRow(ctx, `SELECT t.status,e.result FROM platform.agent_tasks t JOIN platform.agent_executions e ON e.task_id=t.id WHERE t.id=$1 ORDER BY e.attempt DESC LIMIT 1`, id).Scan(&status, &raw)
		var rows []result
		if err != nil || status != "completed" || json.Unmarshal(raw, &rows) != nil || len(rows) != 1 {
			t.Fatalf("status %s: %v", status, err)
		}
		return rows[0]
	}
	id := submit(input)
	step(3)
	// Change the persisted price after catalog retrieval and deterministic calculation.
	exec("UPDATE platform.product_variants SET price_amount=20000 WHERE organization_id=$1 AND id=$2", owner.tenant.OrganizationID, owner.product.VariantID)
	step(2)
	out := load(id)
	if out.State != "proposal_ready" || out.Proposal == nil || out.Proposal.EstimatedTotal == nil || out.Proposal.EstimatedTotal.Amount != 120000 {
		t.Fatalf("proposal did not use current price: %s", out.State)
	}
	exec("UPDATE platform.inventory SET quantity_on_hand=2,manual_updated_at=now() WHERE organization_id=$1 AND variant_id=$2", owner.tenant.OrganizationID, owner.product.VariantID)
	id = submit(input)
	step(5)
	if out = load(id); out.State != "stock_unverified_or_insufficient" || out.Proposal != nil {
		t.Fatal("short stock produced a proposal")
	}
	input.ItemWidth = length("-1")
	id = submit(input)
	step(5)
	if out = load(id); out.State != "calculation_invalid" || out.Proposal != nil {
		t.Fatal("invalid calculation produced a proposal")
	}
}

func testStage262ServiceOnly(t *testing.T, d *db.Database, admin *pgx.Conn, ctx context.Context) {
	svc := agentruntime.Service{Pool: d.Pool, Key: bytes.Repeat([]byte{109}, 32)}
	identity := auth.New(d.Pool, config.Auth{SessionTTL: time.Hour, IdleTTL: time.Hour})
	owner := seedStage23Owner(t, d, admin, ctx, identity, 76)
	if _, err := admin.Exec(ctx, "UPDATE platform.organizations SET business_model='service',industry_profile='vehicle_service' WHERE id=$1", owner.tenant.OrganizationID); err != nil {
		t.Fatal(err)
	}
	booking := servicebooking.Service{Pool: d.Pool}
	selection, resources := stage26ConfigureBooking(t, ctx, owner.tenant, booking, 262)
	in := agentruntime.IndustryDelegationRequest[agentruntime.ServiceRequirements]{
		ConversationID: owner.conversation, Requirements: agentruntime.ServiceRequirements{Selection: &selection, NoPart: true,
			Vehicle: catalog.FitmentVehicle{Make: "synthetic", Model: "service", Year: 2025}},
	}
	run := func(want string) string {
		t.Helper()
		in.IdempotencyKey = auth.UUID()
		id, err := svc.SubmitService(ctx, owner.tenant, in)
		if err != nil {
			t.Fatal(err)
		}
		var status, state string
		for i := 0; i < 20; i++ {
			worked, err := svc.ProcessOne(ctx)
			if err != nil {
				t.Fatalf("step %d: %t %v", i, worked, err)
			}
			if err := admin.QueryRow(ctx, `SELECT t.status, coalesce((SELECT e.result->0->>'state'
			 FROM platform.agent_executions e WHERE e.task_id=t.id ORDER BY e.attempt DESC LIMIT 1),'')
			 FROM platform.agent_tasks t WHERE t.id=$1`, id).Scan(&status, &state); err != nil {
				t.Fatal(err)
			}
			if status == "completed" || status == "failed" || !worked {
				break
			}
		}
		if status != "completed" || state != want {
			t.Fatalf("task %s status %q, result %q want %q", id, status, state, want)
		}
		return id
	}
	id := run("approval_required")
	raw, err := svc.PrepareDelegatedBooking(ctx, owner.tenant, id)
	var request struct {
		ID      string
		Version int64
		Kind    string `json:"request_kind"`
		Status  string
	}
	if err != nil || json.Unmarshal(raw, &request) != nil || request.Kind != "service_booking" || request.Status != "pending_confirmation" {
		t.Fatalf("service intake failed: %v", err)
	}
	service := orders.Service{Pool: d.Pool}
	confirmed, err := service.Change(ctx, owner.tenant, request.ID, "confirm", stage26Command(request.Version))
	if err != nil || json.Unmarshal(confirmed, &request) != nil || request.Status != "confirmed" {
		t.Fatalf("service confirmation: %v", err)
	}
	if _, err = service.Change(ctx, owner.tenant, request.ID, "cancel", stage26Command(request.Version)); err != nil {
		t.Fatal(err)
	}
	in.Requirements.Vehicle.Year = 0
	run("clarification_required")
	in.Requirements.Vehicle.Year = 2025
	selection.StartsAt = selection.StartsAt.Add(2 * time.Hour)
	selection.EndsAt = selection.EndsAt.Add(2 * time.Hour)
	if _, err = admin.Exec(ctx, "UPDATE platform.service_resources SET active=false WHERE organization_id=$1 AND id=$2", owner.tenant.OrganizationID, resources[0]); err != nil {
		t.Fatal(err)
	}
	run("slot_unavailable")
}

func TestStage262Focused(t *testing.T) {
	if os.Getenv("ABA_TEST_ONLY_INDUSTRIES") != "1" {
		t.Skip("requires the focused disposable runner")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	migrationDSN := testDSN(t, "ABA_TEST_MIGRATION_DATABASE_URL", "aba_migrator")
	admin, err := pgx.Connect(ctx, migrationDSN)
	if err != nil {
		t.Fatal("cannot open isolated fixture")
	}
	defer admin.Close(context.Background())
	var safe bool
	err = admin.QueryRow(ctx, `SELECT marker='aba-synthetic-isolated' AND
 NOT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema='platform' AND table_name<>'test_safety')
 FROM platform.test_safety WHERE singleton`).Scan(&safe)
	if err != nil || !safe {
		t.Fatal("fresh isolated schema required")
	}
	if err = migrate.Run(ctx, migrationDSN, "up"); err != nil {
		t.Fatal(err)
	}
	d, err := db.Open(ctx, testDSN(t, "ABA_TEST_DATABASE_URL", "aba_app"), 2, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	t.Run("construction", func(t *testing.T) { testStage262Industry(t, d, admin, ctx) })
	t.Run("service_only", func(t *testing.T) { testStage262ServiceOnly(t, d, admin, ctx) })
}
