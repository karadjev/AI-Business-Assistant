//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/agentruntime"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/orders"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/db"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/servicebooking"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
)

// Registered by main's isolated migration suite. Fixtures intentionally remain
// for its tenant-RLS inventory and rollback-refuses-populated-history checks.
func testStage25Booking(t *testing.T, d *db.Database, admin *pgx.Conn, ctx context.Context) {
	t.Helper()
	identity := auth.New(d.Pool, config.Auth{SessionTTL: time.Hour, IdleTTL: time.Hour})
	a := seedStage23Owner(t, d, admin, ctx, identity, 60)
	b := seedStage23Owner(t, d, admin, ctx, identity, 61)
	svc := servicebooking.Service{Pool: d.Pool}
	requestSvc := orders.Service{Pool: d.Pool}
	if _, err := svc.Search(ctx, a.tenant, "", 10); err == nil {
		t.Fatal("trade business admitted to services")
	}
	profile := agentruntime.Service{Pool: d.Pool}
	if err := profile.UpdateProfile(ctx, a.tenant, "service", "vehicle_service", 1); err != nil {
		t.Fatal(err)
	}
	if err := profile.UpdateProfile(ctx, b.tenant, "service", "personal_services", 1); err != nil {
		t.Fatal(err)
	}
	base := time.Now().UTC().Truncate(24 * time.Hour).Add(48 * time.Hour)
	expectStatus := func(err error, status int) {
		t.Helper()
		var e *auth.Error
		if !errors.As(err, &e) || e.Status != status {
			t.Fatalf("expected %d, got %v", status, err)
		}
	}
	configure := func(capacity, before, after int, secondResource bool) (servicebooking.Offering, []servicebooking.Resource) {
		t.Helper()
		r, err := svc.SaveResource(ctx, a.tenant, servicebooking.Resource{Name: "Synthetic bay", Kind: "capacity", Capacity: capacity, Active: true})
		if err != nil {
			t.Fatal(err)
		}
		resources := []servicebooking.Resource{r}
		ids := []string{r.ID}
		if secondResource {
			r2, err := svc.SaveResource(ctx, a.tenant, servicebooking.Resource{Name: "Synthetic equipment", Kind: "equipment", Capacity: capacity, Active: true})
			if err != nil {
				t.Fatal(err)
			}
			resources = append(resources, r2)
			ids = append(ids, r2.ID)
		}
		o, err := svc.SaveOffering(ctx, a.tenant, servicebooking.Offering{Name: "Synthetic inspection " + r.ID, Status: "active", PriceModel: "on_request", PriceState: "unknown", Currency: "RUB", MinMinutes: 30, MaxMinutes: 120, BufferBefore: before, BufferAfter: after, Timezone: "UTC", ResourceIDs: ids})
		if err != nil {
			t.Fatal(err)
		}
		for _, res := range resources {
			for _, layer := range []string{"business", "offering", "shift"} {
				if _, err = svc.SaveWindow(ctx, a.tenant, servicebooking.Window{OfferingID: o.ID, ResourceID: res.ID, Layer: layer, StartsAt: base, EndsAt: base.Add(24 * time.Hour), Timezone: "UTC"}); err != nil {
					t.Fatal(err)
				}
			}
		}
		found, err := svc.Search(ctx, a.tenant, o.Name, 10)
		if err != nil || len(found) != 1 {
			t.Fatalf("offering search %v %v", found, err)
		}
		o.Revision = found[0].Revision
		if found[0].AmountMinor != nil || found[0].PriceState != "unknown" {
			t.Fatal("unknown service price became zero")
		}
		return o, resources
	}
	selection := func(o servicebooking.Offering, start time.Time) servicebooking.Selection {
		return servicebooking.Selection{OfferingID: o.ID, OfferingRevision: o.Revision, StartsAt: start, EndsAt: start.Add(time.Hour), Timezone: "UTC", Units: 1}
	}
	create := func(sel servicebooking.Selection) (string, orders.Intake) {
		t.Helper()
		in := orders.Intake{ConversationID: a.conversation, IdempotencyKey: auth.UUID(), Kind: "service_booking", Details: orders.IntakeDetails{Selection: &sel}}
		raw, err := requestSvc.PrepareIntake(ctx, a.tenant, in)
		if err != nil {
			t.Fatal(err)
		}
		var r struct {
			ID      string `json:"id"`
			Version int64  `json:"version"`
		}
		if json.Unmarshal(raw, &r) != nil || r.ID == "" || r.Version != 1 {
			t.Fatalf("request %s", raw)
		}
		again, err := requestSvc.PrepareIntake(ctx, a.tenant, in)
		var repeated struct {
			ID string `json:"id"`
		}
		decodeErr := json.Unmarshal(again, &repeated)
		if err != nil || decodeErr != nil || repeated.ID != r.ID {
			t.Fatalf("intake duplicate %v", err)
		}
		return r.ID, in
	}
	t.Run("concurrent-confirm-cancel-idempotency", func(t *testing.T) {
		o, _ := configure(1, 0, 0, false)
		sel := selection(o, base.Add(3*time.Hour))
		available, err := svc.Availability(ctx, a.tenant, sel)
		if err != nil || !available.Available || available.State != "available_nonbinding" {
			t.Fatal(available, err)
		}
		first, _ := create(sel)
		second, _ := create(sel)
		ids := []string{first, second}
		keys := []string{auth.UUID(), auth.UUID()}
		var wg sync.WaitGroup
		errs := make([]error, 2)
		start := make(chan struct{})
		for i := range ids {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				_, errs[i] = requestSvc.Change(ctx, a.tenant, ids[i], "confirm", orders.Transition{ExpectedVersion: 1, IdempotencyKey: keys[i]})
			}(i)
		}
		close(start)
		wg.Wait()
		winner := -1
		for i, err := range errs {
			if err == nil {
				if winner != -1 {
					t.Fatal("double booked capacity one")
				}
				winner = i
			} else {
				expectStatus(err, 409)
			}
		}
		if winner < 0 {
			t.Fatalf("no confirmation: %v", errs)
		}
		repeat := orders.Transition{ExpectedVersion: 1, IdempotencyKey: keys[winner]}
		if _, err = requestSvc.Change(ctx, a.tenant, ids[winner], "confirm", repeat); err != nil {
			t.Fatal("confirm retry", err)
		}
		if _, err = svc.Availability(ctx, a.tenant, sel); err == nil {
			t.Fatal("booked slot offered again")
		}
		cancel := orders.Transition{ExpectedVersion: 2, IdempotencyKey: auth.UUID(), Reason: "synthetic cancellation"}
		if _, err = requestSvc.Change(ctx, a.tenant, ids[winner], "cancel", cancel); err != nil {
			t.Fatal(err)
		}
		if _, err = requestSvc.Change(ctx, a.tenant, ids[winner], "cancel", cancel); err != nil {
			t.Fatal("cancel retry", err)
		}
		loser := 1 - winner
		if _, err = requestSvc.Change(ctx, a.tenant, ids[loser], "confirm", orders.Transition{ExpectedVersion: 1, IdempotencyKey: auth.UUID()}); err != nil {
			t.Fatal("resource not released", err)
		}
		var total, active int
		if err = admin.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE active) FROM platform.service_booking_allocations WHERE organization_id=$1 AND resource_id=(SELECT resource_id FROM platform.service_offering_resources WHERE organization_id=$1 AND offering_id=$2)`, a.tenant.OrganizationID, o.ID).Scan(&total, &active); err != nil || total != 2 || active != 1 {
			t.Fatalf("allocations total=%d active=%d %v", total, active, err)
		}
	})
	t.Run("capacity-two-multiresource-and-buffers", func(t *testing.T) {
		o, _ := configure(2, 15, 15, true)
		sel := selection(o, base.Add(6*time.Hour))
		one, _ := create(sel)
		two, _ := create(sel)
		three, _ := create(sel)
		for _, id := range []string{one, two} {
			if _, err := requestSvc.Change(ctx, a.tenant, id, "confirm", orders.Transition{ExpectedVersion: 1, IdempotencyKey: auth.UUID()}); err != nil {
				t.Fatal(err)
			}
		}
		_, err := requestSvc.Change(ctx, a.tenant, three, "confirm", orders.Transition{ExpectedVersion: 1, IdempotencyKey: auth.UUID()})
		expectStatus(err, 409)
		adjacent := selection(o, sel.EndsAt)
		if _, err = svc.Availability(ctx, a.tenant, adjacent); err == nil {
			t.Fatal("buffer overlap accepted")
		}
		separated := selection(o, sel.EndsAt.Add(30*time.Minute))
		if _, err = svc.Availability(ctx, a.tenant, separated); err != nil {
			t.Fatal("half-open buffered adjacency denied", err)
		}
		var cnt int
		if err = admin.QueryRow(ctx, `SELECT count(*) FROM platform.service_booking_allocations WHERE organization_id=$1 AND booking_id IN (SELECT id FROM platform.service_bookings WHERE organization_id=$1 AND offering_id=$2)`, a.tenant.OrganizationID, o.ID).Scan(&cnt); err != nil || cnt != 4 {
			t.Fatal("multi-resource allocation incomplete", cnt, err)
		}
	})
	t.Run("revision-schedule-and-tenant-guards", func(t *testing.T) {
		o, resources := configure(1, 0, 0, false)
		sel := selection(o, base.Add(10*time.Hour))
		pending, _ := create(sel)
		exception, err := svc.SaveWindow(ctx, a.tenant, servicebooking.Window{OfferingID: o.ID, ResourceID: resources[0].ID, Layer: "exception", StartsAt: sel.StartsAt, EndsAt: sel.EndsAt, Timezone: "UTC"})
		if err != nil {
			t.Fatal(err)
		}
		_, err = requestSvc.Change(ctx, a.tenant, pending, "confirm", orders.Transition{ExpectedVersion: 1, IdempotencyKey: auth.UUID()})
		expectStatus(err, 409)
		if err = svc.DeleteWindow(ctx, a.tenant, exception.ID); err != nil {
			t.Fatal(err)
		}
		if _, err = svc.Availability(ctx, a.tenant, sel); err == nil {
			t.Fatal("stale offering revision accepted")
		}
		foreign := a.tenant
		foreign.OrganizationID = b.tenant.OrganizationID
		if _, err = svc.Search(ctx, foreign, "", 10); err == nil {
			t.Fatal("cross-tenant discovery")
		}
		if _, err = svc.Availability(ctx, b.tenant, sel); err == nil {
			t.Fatal("cross-tenant offering")
		}
		missing, createErr := svc.SaveOffering(ctx, a.tenant, servicebooking.Offering{Name: "Synthetic unknown schedule", Status: "active", PriceModel: "on_request", PriceState: "unknown", Currency: "RUB", MinMinutes: 30, MaxMinutes: 60, Timezone: "UTC", ResourceIDs: []string{resources[0].ID}})
		if createErr != nil || missing.ID == "" {
			t.Fatal("unknown-schedule offering creation failed")
		}
		if _, err = svc.Availability(ctx, a.tenant, selection(missing, base.Add(12*time.Hour))); err == nil {
			t.Fatal("unknown schedule became always available")
		}
	})
	t.Run("delegated-staff-no-configuration", func(t *testing.T) {
		if _, err := admin.Exec(ctx, `INSERT INTO platform.organization_members(organization_id,user_id,role,can_manage_orders) VALUES($1,$2,'member',true)`, a.tenant.OrganizationID, b.tenant.Session.User.ID); err != nil {
			t.Fatal(err)
		}
		staff := catalog.Tenant{OrganizationID: a.tenant.OrganizationID, Session: b.tenant.Session}
		if _, err := svc.Resources(ctx, staff); err == nil {
			t.Fatal("private resources exposed to delegated order staff")
		}
		if _, err := svc.SaveResource(ctx, staff, servicebooking.Resource{Name: "forbidden", Kind: "room", Capacity: 1, Active: true}); err == nil {
			t.Fatal("staff changed configuration")
		}
		o, _ := configure(1, 0, 0, false)
		sel := selection(o, base.Add(15*time.Hour))
		if _, err := svc.Availability(ctx, staff, sel); err != nil {
			t.Fatal("staff cannot query public availability", err)
		}
		id, _ := create(sel)
		if _, err := requestSvc.Change(ctx, staff, id, "confirm", orders.Transition{ExpectedVersion: 1, IdempotencyKey: auth.UUID()}); err != nil {
			t.Fatal("staff cannot confirm", err)
		}
		tx, err := tenancy.Begin(ctx, d.Pool, staff, true, true)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, `UPDATE platform.service_resources SET name='forbidden' WHERE organization_id=$1`, a.tenant.OrganizationID); err == nil {
			t.Fatal("staff SQL configuration update bypass")
		}
	})
}
