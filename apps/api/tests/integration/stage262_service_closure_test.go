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
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/db"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/migrate"
)

func testStage262ServiceClosure(t *testing.T, d *db.Database, admin *pgx.Conn, ctx context.Context) {
	svc := agentruntime.Service{Pool: d.Pool, Key: bytes.Repeat([]byte{110}, 32)}
	identity := auth.New(d.Pool, config.Auth{SessionTTL: time.Hour, IdleTTL: time.Hour})
	owner := seedStage23Owner(t, d, admin, ctx, identity, 87)
	if err := svc.UpdateProfile(ctx, owner.tenant, "hybrid", "mixed_nonfood", 1); err != nil {
		t.Fatal(err)
	}
	f := stage26Configure(t, d, ctx, owner, 968)
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := admin.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	org := owner.tenant.OrganizationID
	exec(`UPDATE platform.product_variants SET attributes='{"oem":"CLOSURE-PART"}' WHERE organization_id=$1 AND id=$2`, org, f.product.VariantID)
	exec(`INSERT INTO platform.catalog_fitment_evidence(id,organization_id,variant_id,make,model,engine,year_from,year_to,compatible,source_ref,verified_at,valid_until)
  VALUES(gen_random_uuid(),$1,$2,'synthetic','closure','engine',2020,2030,true,'synthetic:closure',now()-interval '1 hour',now()+interval '1 day')`, org, f.product.VariantID)
	exec(`UPDATE platform.service_offerings SET part_rule='{"state":"REQUIRED","oem":"CLOSURE-PART","quantity":2}',revision=revision+1 WHERE organization_id=$1 AND id=$2`, org, f.selection.OfferingID)
	f.selection.OfferingRevision++
	exec("UPDATE platform.inventory SET quantity_on_hand=1,manual_updated_at=now() WHERE organization_id=$1 AND variant_id=$2", org, f.product.VariantID)
	process := func(n int) {
		t.Helper()
		for i := 0; i < n; i++ {
			worked, err := svc.ProcessOne(ctx)
			if !worked || err != nil {
				t.Fatalf("step %d: %t %v", i, worked, err)
			}
		}
	}
	submit := func() string {
		t.Helper()
		id, err := svc.SubmitService(ctx, owner.tenant, agentruntime.IndustryDelegationRequest[agentruntime.ServiceRequirements]{
			ConversationID: owner.conversation, IdempotencyKey: auth.UUID(),
			Requirements: agentruntime.ServiceRequirements{Selection: &f.selection, Vehicle: catalog.FitmentVehicle{Make: "synthetic", Model: "closure", Engine: "engine", Year: 2025}}})
		if err != nil {
			t.Fatal(err)
		}
		process(6)
		return id
	}
	short := submit()
	if _, err := svc.PrepareDelegatedBooking(ctx, owner.tenant, short); err == nil {
		t.Fatal("short stock admitted booking")
	}
	exec("UPDATE platform.inventory SET quantity_on_hand=2,manual_updated_at=now() WHERE organization_id=$1 AND variant_id=$2", org, f.product.VariantID)
	parent := submit()
	raw, err := svc.PrepareDelegatedBooking(ctx, owner.tenant, parent)
	var request struct {
		ID      string
		Version int64
	}
	if err != nil || json.Unmarshal(raw, &request) != nil || request.ID == "" {
		t.Fatalf("prepare: %v", err)
	}
	exec("UPDATE platform.product_variants SET price_amount=price_amount+1 WHERE organization_id=$1 AND id=$2", org, f.product.VariantID)
	if _, err = f.requestSvc.Change(ctx, owner.tenant, request.ID, "confirm", stage26Command(request.Version)); err == nil {
		t.Fatal("changed price confirmed")
	}
	if s := stage26Snapshot(t, admin, ctx, f, request.ID); s.reserved != 0 || s.allocations != 0 {
		t.Fatal("rejected confirmation left holds")
	}
	exec("UPDATE platform.product_variants SET price_amount=price_amount-1 WHERE organization_id=$1 AND id=$2", org, f.product.VariantID)
	if _, err = f.requestSvc.Change(ctx, owner.tenant, request.ID, "confirm", stage26Command(request.Version)); err != nil {
		t.Fatal(err)
	}
	// Re-delivery can produce a different transport event ID for the same committed order version.
	exec(`INSERT INTO platform.order_events(id,organization_id,order_id,actor_id,kind,reason,idempotency_key,request_hash,result)
	SELECT gen_random_uuid(),organization_id,order_id,actor_id,kind,reason,gen_random_uuid(),request_hash,result
	FROM platform.order_events WHERE organization_id=$1 AND order_id=$2 AND kind='confirmed'`, org, request.ID)
	var jobs int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM platform.agent_tasks WHERE organization_id=$1 AND parent_id=$2 AND action='delegation.service_receipt'`, org, parent).Scan(&jobs); err != nil || jobs != 1 {
		t.Fatalf("event replay produced %d receipts: %v", jobs, err)
	}
	var receiptID string
	var leaked []byte
	if err := admin.QueryRow(ctx, `SELECT id FROM platform.agent_tasks WHERE organization_id=$1 AND parent_id=$2 AND action='delegation.service_receipt'`, org, parent).Scan(&receiptID); err != nil {
		t.Fatal(err)
	}
	if err := d.Pool.QueryRow(ctx, "SELECT platform.service_receipt_context($1,$2)", receiptID, auth.UUID()).Scan(&leaked); err != nil || len(leaked) != 0 {
		t.Fatalf("receipt leaked without a matching lease: %v", err)
	}
	receipt := func(status, booking, reservation string, attempt int) {
		t.Helper()
		var got, a, b string
		var count int
		err := admin.QueryRow(ctx, `SELECT t.status,coalesce(e.result->0->'receipt'->>'booking_status',''),
  coalesce(e.result->0->'receipt'->>'reservation_status',''),e.attempt
  FROM platform.agent_tasks t JOIN platform.agent_executions e ON e.task_id=t.id AND e.organization_id=t.organization_id
  WHERE t.organization_id=$1 AND t.parent_id=$2 AND t.action='delegation.service_receipt'
  ORDER BY t.delegation_step DESC,e.attempt DESC LIMIT 1`, org, parent).Scan(&got, &a, &b, &count)
		if err != nil || got != status || a != booking || b != reservation || count != attempt {
			t.Fatalf("receipt status=%s booking=%s reservation=%s attempts=%d: %v", got, a, b, count, err)
		}
	}
	// Force the receipt read to fail transiently on the real PostgreSQL path.
	lock, err := admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback(ctx)
	if _, err = lock.Exec(ctx, "LOCK TABLE platform.orders IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatal(err)
	}
	process(1)
	if err = lock.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	receipt("queued", "", "", 1)
	exec(`UPDATE platform.channel_inbox q SET available_at=clock_timestamp() FROM platform.agent_tasks t WHERE t.id=q.id AND t.parent_id=$1 AND t.action='delegation.service_receipt'`, parent)
	process(1)
	receipt("completed", "confirmed", "active", 2)
	stage26ForceDue(t, admin, ctx, org, request.ID)
	if v, err := f.requestSvc.ExpireOne(ctx); !v || err != nil {
		t.Fatalf("expire: %t %v", v, err)
	}
	process(1)
	receipt("completed", "cancelled", "expired", 1)
	if s := stage26Snapshot(t, admin, ctx, f, request.ID); s.reserved != 0 || s.allocations != 0 {
		t.Fatal("expired booking left holds")
	}
}

func TestServiceClosureFocused(t *testing.T) {
	if os.Getenv("ABA_TEST_ONLY_SERVICE_CLOSURE") != "1" {
		t.Skip("requires the focused disposable runner")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
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
	testStage262ServiceClosure(t, d, admin, ctx)
}
