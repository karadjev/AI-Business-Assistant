//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/agent"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/agentruntime"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/conversations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/orders"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/db"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/servicebooking"
)

type stage26Fixture struct {
	owner      stage23Owner
	product    instagramCatalogFixture
	selection  servicebooking.Selection
	resources  []string
	requestSvc orders.Service
	bookingSvc servicebooking.Service
}
type stage26State struct {
	status            string
	version           int64
	reserved          int
	onHand            int
	bookingStatus     string
	allocations       int
	bookings          int
	reserveMoves      int
	releaseMoves      int
	headers           int
	reservationStatus string
}

func stage26Configure(t *testing.T, d *db.Database, ctx context.Context, owner stage23Owner, index int) stage26Fixture {
	t.Helper()
	f := stage26Fixture{owner: owner, product: seedInstagramCatalog(t, d, ctx, owner.tenant, index), requestSvc: orders.Service{Pool: d.Pool}, bookingSvc: servicebooking.Service{Pool: d.Pool}}
	f.selection, f.resources = stage26ConfigureBooking(t, ctx, owner.tenant, f.bookingSvc, index)
	return f
}

func stage26ConfigureBooking(t *testing.T, ctx context.Context, tenant catalog.Tenant, booking servicebooking.Service, index int) (servicebooking.Selection, []string) {
	t.Helper()
	var resources []string
	for _, kind := range []string{"room", "equipment"} {
		r, err := booking.SaveResource(ctx, tenant, servicebooking.Resource{Name: fmt.Sprintf("Synthetic atomic %s %d", kind, index), Kind: kind, Capacity: 1, Active: true})
		if err != nil {
			t.Fatal(err)
		}
		resources = append(resources, r.ID)
	}
	o, err := booking.SaveOffering(ctx, tenant, servicebooking.Offering{Name: fmt.Sprintf("Synthetic atomic service %d", index), Status: "active", PriceModel: "on_request", PriceState: "unknown", Currency: "RUB", MinMinutes: 30, MaxMinutes: 120, Timezone: "UTC", ResourceIDs: resources})
	if err != nil {
		t.Fatal(err)
	}
	base := time.Now().UTC().Truncate(24 * time.Hour).Add(72 * time.Hour)
	for _, id := range resources {
		for _, layer := range []string{"business", "offering", "shift"} {
			if _, err = booking.SaveWindow(ctx, tenant, servicebooking.Window{OfferingID: o.ID, ResourceID: id, Layer: layer, StartsAt: base, EndsAt: base.Add(24 * time.Hour), Timezone: "UTC"}); err != nil {
				t.Fatal(err)
			}
		}
	}
	list, err := booking.Search(ctx, tenant, o.Name, 10)
	if err != nil || len(list) != 1 {
		t.Fatalf("configure offering %v %v", list, err)
	}
	return servicebooking.Selection{OfferingID: o.ID, OfferingRevision: list[0].Revision, StartsAt: base.Add(3 * time.Hour), EndsAt: base.Add(4 * time.Hour), Timezone: "UTC", Units: 1}, resources
}
func (f stage26Fixture) intake(quantity int, selection *servicebooking.Selection) orders.Intake {
	details := orders.IntakeDetails{Items: []orders.CartItem{{VariantID: f.product.VariantID, Quantity: quantity}}, Selection: selection}
	if selection == nil {
		details.OfferingID = f.selection.OfferingID
	}
	return orders.Intake{ConversationID: f.owner.conversation, IdempotencyKey: auth.UUID(), Kind: "hybrid_order", Details: details}
}
func stage26Create(t *testing.T, ctx context.Context, f stage26Fixture, in orders.Intake) string {
	t.Helper()
	raw, err := f.requestSvc.PrepareIntake(ctx, f.owner.tenant, in)
	if err != nil {
		t.Fatal("prepare hybrid", err)
	}
	var v struct {
		ID      string `json:"id"`
		Version int64  `json:"version"`
	}
	if err = json.Unmarshal(raw, &v); err != nil || v.ID == "" || v.Version != 1 {
		t.Fatalf("hybrid request shape %s %v", raw, err)
	}
	return v.ID
}
func stage26Snapshot(t *testing.T, admin *pgx.Conn, ctx context.Context, f stage26Fixture, id string) stage26State {
	t.Helper()
	var s stage26State
	err := admin.QueryRow(ctx, `SELECT r.status,r.version,
 (SELECT quantity_reserved FROM platform.inventory WHERE organization_id=r.organization_id AND variant_id=$3),
 (SELECT count(*) FROM platform.service_booking_allocations a JOIN platform.service_bookings b ON b.organization_id=a.organization_id AND b.id=a.booking_id WHERE b.organization_id=r.organization_id AND b.request_id=r.id AND a.active),
 (SELECT count(*) FROM platform.service_bookings b WHERE b.organization_id=r.organization_id AND b.request_id=r.id),
 (SELECT count(*) FROM platform.inventory_movements m WHERE m.organization_id=r.organization_id AND m.order_id=r.id AND m.kind='reserve'),
 (SELECT count(*) FROM platform.inventory_movements m WHERE m.organization_id=r.organization_id AND m.order_id=r.id AND m.kind='release'),
 (SELECT count(*) FROM platform.hybrid_reservations h WHERE h.organization_id=r.organization_id AND h.order_id=r.id),
 coalesce((SELECT status FROM platform.hybrid_reservations h WHERE h.organization_id=r.organization_id AND h.order_id=r.id),''),
 (SELECT quantity_on_hand FROM platform.inventory WHERE organization_id=r.organization_id AND variant_id=$3),
 coalesce((SELECT status FROM platform.service_bookings b WHERE b.organization_id=r.organization_id AND b.request_id=r.id),'')
 FROM platform.orders r WHERE r.organization_id=$1 AND r.id=$2`, f.owner.tenant.OrganizationID, id, f.product.VariantID).Scan(&s.status, &s.version, &s.reserved, &s.allocations, &s.bookings, &s.reserveMoves, &s.releaseMoves, &s.headers, &s.reservationStatus, &s.onHand, &s.bookingStatus)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func stage26RequirePending(t *testing.T, s stage26State) {
	t.Helper()
	if s.status != "pending_confirmation" || s.version != 1 || s.allocations != 0 || s.bookings != 0 || s.reserveMoves != 0 || s.releaseMoves != 0 || s.headers != 0 || s.reservationStatus != "" || s.bookingStatus != "" {
		t.Fatalf("failed command leaked effects: %+v", s)
	}
}
func stage26RequireConflict(t *testing.T, err error) {
	t.Helper()
	var failure *auth.Error
	if !errors.As(err, &failure) || failure.Status != 409 {
		t.Fatalf("expected domain conflict, got %v", err)
	}
}
func stage26Command(version int64) orders.Transition {
	return orders.Transition{ExpectedVersion: version, IdempotencyKey: auth.UUID(), Reason: "Synthetic Stage26"}
}

// Main registers this helper in the isolated migration suite. It does not start
// services or select a DSN and intentionally retains all tenant fixtures.
func testStage26Reservation(t *testing.T, d *db.Database, admin *pgx.Conn, ctx context.Context) {
	t.Helper()
	identity := auth.New(d.Pool, config.Auth{SessionTTL: time.Hour, IdleTTL: time.Hour})
	a := seedStage23Owner(t, d, admin, ctx, identity, 80)
	b := seedStage23Owner(t, d, admin, ctx, identity, 81)
	profiles := agentruntime.Service{Pool: d.Pool}
	for _, owner := range []stage23Owner{a, b} {
		if err := profiles.UpdateProfile(ctx, owner.tenant, "hybrid", "mixed_nonfood", 1); err != nil {
			t.Fatal(err)
		}
	}
	t.Run("property-viewing-binds-live-listing-and-allocates-slot", func(t *testing.T) {
		agency := seedStage23Owner(t, d, admin, ctx, identity, 83)
		if err := profiles.UpdateProfile(ctx, agency.tenant, "agency", "real_estate", 1); err != nil {
			t.Fatal(err)
		}
		booking := servicebooking.Service{Pool: d.Pool}
		selection, resources := stage26ConfigureBooking(t, ctx, agency.tenant, booking, 616)
		another, _ := stage26ConfigureBooking(t, ctx, agency.tenant, booking, 617)
		listing := auth.UUID()
		if _, err := admin.Exec(ctx, `INSERT INTO platform.property_listings(id,organization_id,title,locality,rooms,area_m2,status,verified_until,price_amount,currency_code,property_type,viewing_offering_id)
		VALUES($1,$2,'Synthetic viewing','Synthetic locality',2,48.5,'available',now()+interval '1 hour',9500000,'RUB','apartment',$3)`, listing, agency.tenant.OrganizationID, selection.OfferingID); err != nil {
			t.Fatal(err)
		}
		f := stage26Fixture{owner: agency, requestSvc: orders.Service{Pool: d.Pool}}
		in := orders.Intake{ConversationID: agency.conversation, Kind: "property_viewing_request", IdempotencyKey: auth.UUID(), Details: orders.IntakeDetails{ListingID: listing, Selection: &another}}
		_, err := f.requestSvc.PrepareIntake(ctx, agency.tenant, in)
		stage26RequireConflict(t, err)
		in.Details.Selection = &selection
		id := stage26Create(t, ctx, f, in)
		in.IdempotencyKey = auth.UUID()
		otherID := stage26Create(t, ctx, f, in)
		if _, err = admin.Exec(ctx, `UPDATE platform.property_listings SET verified_until=now()-interval '1 minute' WHERE organization_id=$1 AND id=$2`, agency.tenant.OrganizationID, listing); err != nil {
			t.Fatal(err)
		}
		_, err = f.requestSvc.Change(ctx, agency.tenant, id, "confirm", stage26Command(1))
		stage26RequireConflict(t, err)
		if _, err = admin.Exec(ctx, `UPDATE platform.property_listings SET verified_until=now()+interval '1 hour' WHERE organization_id=$1 AND id=$2`, agency.tenant.OrganizationID, listing); err != nil {
			t.Fatal(err)
		}
		if _, err = f.requestSvc.Change(ctx, agency.tenant, id, "confirm", stage26Command(1)); err != nil {
			t.Fatal(err)
		}
		_, err = f.requestSvc.Change(ctx, agency.tenant, otherID, "confirm", stage26Command(1))
		stage26RequireConflict(t, err)
		var count int
		if err = admin.QueryRow(ctx, `SELECT count(*) FROM platform.service_booking_allocations a JOIN platform.service_bookings b
		ON b.organization_id=a.organization_id AND b.id=a.booking_id WHERE b.organization_id=$1 AND b.request_id=$2 AND a.active`, agency.tenant.OrganizationID, id).Scan(&count); err != nil || count != len(resources) {
			t.Fatalf("property viewing allocations: %d %v", count, err)
		}
		if _, err = f.requestSvc.Change(ctx, agency.tenant, id, "cancel", stage26Command(2)); err != nil {
			t.Fatal(err)
		}
		if _, err = f.requestSvc.Change(ctx, agency.tenant, otherID, "confirm", stage26Command(1)); err != nil {
			t.Fatal("cancelled viewing still occupies slot", err)
		}
	})
	t.Run("atomic-success-snapshots-replay-cancellation", func(t *testing.T) {
		f := stage26Configure(t, d, ctx, a, 600)
		in := f.intake(2, &f.selection)
		id := stage26Create(t, ctx, f, in)
		before := stage26Snapshot(t, admin, ctx, f, id)
		stage26RequirePending(t, before)
		if before.reserved != 0 {
			t.Fatal("prepare reserved stock")
		}
		duplicate := stage26Create(t, ctx, f, in)
		if duplicate != id {
			t.Fatal("prepare replay created a second order")
		}
		confirm := stage26Command(1)
		if _, err := f.requestSvc.Change(ctx, a.tenant, id, "confirm", confirm); err != nil {
			t.Fatal(err)
		}
		confirmed := stage26Snapshot(t, admin, ctx, f, id)
		if confirmed.status != "confirmed" || confirmed.version != 2 || confirmed.reserved != 2 || confirmed.allocations != len(f.resources) || confirmed.bookings != 1 || confirmed.reserveMoves != 1 || confirmed.headers != 1 || confirmed.reservationStatus != "active" || confirmed.onHand != 7 || confirmed.bookingStatus != "confirmed" {
			t.Fatalf("atomic confirmation: %+v", confirmed)
		}
		var total *int64
		if err := admin.QueryRow(ctx, `SELECT total_amount FROM platform.orders WHERE organization_id=$1 AND id=$2`, a.tenant.OrganizationID, id).Scan(&total); err != nil || total != nil {
			t.Fatal("unknown service total became a fixed price", total, err)
		}
		var quantity int
		var price int64
		if err := admin.QueryRow(ctx, `SELECT quantity,unit_price_amount FROM platform.order_items WHERE organization_id=$1 AND order_id=$2 AND variant_id=$3`, a.tenant.OrganizationID, id, f.product.VariantID).Scan(&quantity, &price); err != nil || quantity != 2 || price != f.product.Price {
			t.Fatalf("missing canonical item snapshot q=%d price=%d %v", quantity, price, err)
		}
		if _, err := f.requestSvc.Change(ctx, a.tenant, id, "confirm", confirm); err != nil {
			t.Fatal("confirm replay", err)
		}
		if after := stage26Snapshot(t, admin, ctx, f, id); after != confirmed {
			t.Fatalf("confirm replay changed reservation %+v -> %+v", confirmed, after)
		}
		changed := in
		changed.Details.Items = []orders.CartItem{{VariantID: f.product.VariantID, Quantity: 3}}
		_, err := f.requestSvc.PrepareIntake(ctx, a.tenant, changed)
		stage26RequireConflict(t, err)
		cancel := stage26Command(2)
		if _, err = f.requestSvc.Change(ctx, a.tenant, id, "cancel", cancel); err != nil {
			t.Fatal(err)
		}
		cancelled := stage26Snapshot(t, admin, ctx, f, id)
		if cancelled.status != "cancelled" || cancelled.version != 3 || cancelled.reserved != 0 || cancelled.allocations != 0 || cancelled.releaseMoves != 1 || cancelled.reservationStatus != "released" || cancelled.onHand != 7 || cancelled.bookingStatus != "cancelled" {
			t.Fatalf("atomic cancellation %+v", cancelled)
		}
		if _, err = f.requestSvc.Change(ctx, a.tenant, id, "cancel", cancel); err != nil {
			t.Fatal("cancel replay", err)
		}
		if after := stage26Snapshot(t, admin, ctx, f, id); after != cancelled {
			t.Fatal("cancel replay released twice", after)
		}
		if _, err = f.bookingSvc.Availability(ctx, a.tenant, f.selection); err != nil {
			t.Fatal("cancellation did not release slot", err)
		}
	})
	t.Run("stock-lost-before-confirm-slot-rolls-back", func(t *testing.T) {
		f := stage26Configure(t, d, ctx, a, 601)
		id := stage26Create(t, ctx, f, f.intake(2, &f.selection))
		var version int64
		if err := admin.QueryRow(ctx, `SELECT version FROM platform.inventory WHERE organization_id=$1 AND variant_id=$2`, a.tenant.OrganizationID, f.product.VariantID).Scan(&version); err != nil {
			t.Fatal(err)
		}
		if _, err := (catalog.Service{Pool: d.Pool}).Adjust(ctx, a.tenant, catalog.Input{VariantID: f.product.VariantID, QuantityOnHand: 1, ExpectedVersion: version, Reason: "Synthetic stock changed before confirmation"}); err != nil {
			t.Fatal(err)
		}
		_, err := f.requestSvc.Change(ctx, a.tenant, id, "confirm", stage26Command(1))
		stage26RequireConflict(t, err)
		state := stage26Snapshot(t, admin, ctx, f, id)
		stage26RequirePending(t, state)
		if state.reserved != 0 {
			t.Fatal("stock failure leaked reserve")
		}
		if _, err = f.bookingSvc.Availability(ctx, a.tenant, f.selection); err != nil {
			t.Fatal("stock failure leaked slot", err)
		}
	})
	t.Run("slot-lost-before-confirm-stock-rolls-back", func(t *testing.T) {
		f := stage26Configure(t, d, ctx, a, 602)
		id := stage26Create(t, ctx, f, f.intake(2, &f.selection))
		raw, err := f.requestSvc.PrepareIntake(ctx, a.tenant, orders.Intake{ConversationID: a.conversation, IdempotencyKey: auth.UUID(), Kind: "service_booking", Details: orders.IntakeDetails{Selection: &f.selection}})
		if err != nil {
			t.Fatal(err)
		}
		var blocker struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(raw, &blocker) != nil || blocker.ID == "" {
			t.Fatal("blocker response")
		}
		if _, err = f.requestSvc.Change(ctx, a.tenant, blocker.ID, "confirm", stage26Command(1)); err != nil {
			t.Fatal(err)
		}
		_, err = f.requestSvc.Change(ctx, a.tenant, id, "confirm", stage26Command(1))
		stage26RequireConflict(t, err)
		state := stage26Snapshot(t, admin, ctx, f, id)
		stage26RequirePending(t, state)
		if state.reserved != 0 {
			t.Fatal("slot failure leaked stock")
		}
	})
	t.Run("late-audit-failure-rolls-back-both-and-can-retry", func(t *testing.T) {
		f := stage26Configure(t, d, ctx, a, 603)
		id := stage26Create(t, ctx, f, f.intake(2, &f.selection))
		cmd := stage26Command(1)
		// Scoped to this synthetic order only, in the suite's disposable database.
		sql := fmt.Sprintf(`CREATE FUNCTION platform.stage26_fail_confirm_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.order_id='%s'::uuid AND NEW.kind='confirmed' THEN RAISE EXCEPTION 'synthetic late audit failure'; END IF; RETURN NEW; END $$;
 CREATE TRIGGER stage26_fail_confirm_audit BEFORE INSERT ON platform.order_events FOR EACH ROW EXECUTE FUNCTION platform.stage26_fail_confirm_audit()`, id)
		if _, err := admin.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
		remove := func() {
			t.Helper()
			if _, err := admin.Exec(ctx, `DROP TRIGGER IF EXISTS stage26_fail_confirm_audit ON platform.order_events; DROP FUNCTION IF EXISTS platform.stage26_fail_confirm_audit()`); err != nil {
				t.Fatal(err)
			}
		}
		defer remove()
		if _, err := f.requestSvc.Change(ctx, a.tenant, id, "confirm", cmd); err == nil {
			t.Fatal("injected audit failure was ignored")
		}
		state := stage26Snapshot(t, admin, ctx, f, id)
		stage26RequirePending(t, state)
		if state.reserved != 0 {
			t.Fatal("late audit failure leaked stock")
		}
		if _, err := f.bookingSvc.Availability(ctx, a.tenant, f.selection); err != nil {
			t.Fatal("late audit failure leaked slot", err)
		}
		remove()
		if _, err := f.requestSvc.Change(ctx, a.tenant, id, "confirm", cmd); err != nil {
			t.Fatal("rolled-back command cannot retry", err)
		}
		after := stage26Snapshot(t, admin, ctx, f, id)
		if after.reserved != 2 || after.allocations != len(f.resources) || after.reserveMoves != 1 {
			t.Fatal("retry did not reserve both exactly once", after)
		}
	})
	t.Run("race-same-slot-no-partial-stock", func(t *testing.T) {
		f := stage26Configure(t, d, ctx, a, 604)
		stage26Race(t, admin, ctx, f, f.selection, f.selection, 1)
	})
	t.Run("race-last-stock-disjoint-slots-no-partial-slot", func(t *testing.T) {
		f := stage26Configure(t, d, ctx, a, 605)
		other := f.selection
		other.StartsAt = other.StartsAt.Add(2 * time.Hour)
		other.EndsAt = other.EndsAt.Add(2 * time.Hour)
		stage26Race(t, admin, ctx, f, f.selection, other, 7)
	})
	t.Run("race-last-stock-and-same-slot", func(t *testing.T) {
		f := stage26Configure(t, d, ctx, a, 614)
		stage26Race(t, admin, ctx, f, f.selection, f.selection, 7)
	})
	t.Run("schedule-change-invalidates-hybrid-approval", func(t *testing.T) {
		f := stage26Configure(t, d, ctx, a, 615)
		id := stage26Create(t, ctx, f, f.intake(1, &f.selection))
		_, err := f.bookingSvc.SaveWindow(ctx, a.tenant, servicebooking.Window{OfferingID: f.selection.OfferingID, ResourceID: f.resources[0], Layer: "exception", StartsAt: f.selection.StartsAt, EndsAt: f.selection.EndsAt, Timezone: f.selection.Timezone})
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.requestSvc.Change(ctx, a.tenant, id, "confirm", stage26Command(1))
		stage26RequireConflict(t, err)
		state := stage26Snapshot(t, admin, ctx, f, id)
		stage26RequirePending(t, state)
		if state.reserved != 0 {
			t.Fatal("stale schedule acquired stock")
		}
	})
	t.Run("tenant-and-forged-scope-denied-without-effects", func(t *testing.T) {
		f := stage26Configure(t, d, ctx, a, 606)
		id := stage26Create(t, ctx, f, f.intake(1, &f.selection))
		for _, tenant := range []catalog.Tenant{b.tenant, {OrganizationID: a.tenant.OrganizationID, Session: b.tenant.Session}} {
			if _, err := f.requestSvc.Change(ctx, tenant, id, "confirm", stage26Command(1)); err == nil {
				t.Fatal("foreign confirmation succeeded")
			}
			if _, err := f.requestSvc.Change(ctx, tenant, id, "cancel", stage26Command(1)); err == nil {
				t.Fatal("foreign cancellation succeeded")
			}
		}
		own := stage26Snapshot(t, admin, ctx, f, id)
		stage26RequirePending(t, own)
		if own.reserved != 0 {
			t.Fatal("foreign command leaked reserve")
		}
		foreignInput := f.intake(1, &f.selection)
		foreignInput.ConversationID = b.conversation
		if _, err := f.requestSvc.PrepareIntake(ctx, b.tenant, foreignInput); err == nil {
			t.Fatal("cross-tenant references accepted")
		}
	})
	t.Run("legacy-hybrid-intake-without-selection-remains-nonbinding", func(t *testing.T) {
		f := stage26Configure(t, d, ctx, a, 607)
		id := stage26Create(t, ctx, f, f.intake(1, nil))
		if _, err := f.requestSvc.Change(ctx, a.tenant, id, "confirm", stage26Command(1)); err != nil {
			t.Fatal(err)
		}
		state := stage26Snapshot(t, admin, ctx, f, id)
		if state.status != "confirmed" || state.reserved != 0 || state.allocations != 0 || state.bookings != 0 || state.reserveMoves != 0 || state.headers != 0 {
			t.Fatal("intake-only hybrid acquired binding reserves", state)
		}
	})
	t.Run("durable-expiry-releases-once-and-audits", func(t *testing.T) {
		f := stage26Configure(t, d, ctx, a, 608)
		id := stage26Create(t, ctx, f, f.intake(2, &f.selection))
		if _, err := f.requestSvc.Change(ctx, a.tenant, id, "confirm", stage26Command(1)); err != nil {
			t.Fatal(err)
		}
		var expiry time.Time
		var state string
		if err := admin.QueryRow(ctx, `SELECT status,expires_at FROM platform.hybrid_reservations WHERE organization_id=$1 AND order_id=$2`, a.tenant.OrganizationID, id).Scan(&state, &expiry); err != nil || state != "active" || !expiry.Equal(f.selection.EndsAt.Add(24*time.Hour)) {
			t.Fatalf("expiry not bound to slot grace: status=%s expires=%s %v", state, expiry, err)
		}
		if did, err := f.requestSvc.ExpireOne(ctx); err != nil || did {
			t.Fatalf("future hold expired: %t %v", did, err)
		}
		stage26ForceDue(t, admin, ctx, a.tenant.OrganizationID, id)
		// Two durable expiry consumers race the same due order; only one may act.
		var wg sync.WaitGroup
		start := make(chan struct{})
		worked := make([]bool, 2)
		errs := make([]error, 2)
		for i := range worked {
			wg.Add(1)
			go func(i int) { defer wg.Done(); <-start; worked[i], errs[i] = f.requestSvc.ExpireOne(ctx) }(i)
		}
		close(start)
		wg.Wait()
		count := 0
		for i, err := range errs {
			if err != nil {
				t.Fatal(err)
			}
			if worked[i] {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("expiry claims=%d, want one", count)
		}
		expired := stage26Snapshot(t, admin, ctx, f, id)
		if expired.status != "cancelled" || expired.version != 3 || expired.reserved != 0 || expired.allocations != 0 || expired.releaseMoves != 1 || expired.reservationStatus != "expired" || expired.onHand != 7 || expired.bookingStatus != "cancelled" {
			t.Fatal("expiry not atomic", expired)
		}
		var audit int
		if err := admin.QueryRow(ctx, `SELECT count(*) FROM platform.order_events WHERE organization_id=$1 AND order_id=$2 AND kind='cancelled' AND reason='hybrid_reservation_expired' AND actor_id IS NULL`, a.tenant.OrganizationID, id).Scan(&audit); err != nil || audit != 1 {
			t.Fatalf("expiry audit count=%d %v", audit, err)
		}
		if did, err := f.requestSvc.ExpireOne(ctx); err != nil || did {
			t.Fatalf("expiry replay %t %v", did, err)
		}
		if after := stage26Snapshot(t, admin, ctx, f, id); after != expired {
			t.Fatal("expiry replay changed data", after)
		}
		if _, err := f.bookingSvc.Availability(ctx, a.tenant, f.selection); err != nil {
			t.Fatal("expiry did not release slot", err)
		}
	})
	t.Run("cancel-versus-expire-releases-both-once", func(t *testing.T) {
		f := stage26Configure(t, d, ctx, a, 609)
		id := stage26Create(t, ctx, f, f.intake(2, &f.selection))
		if _, err := f.requestSvc.Change(ctx, a.tenant, id, "confirm", stage26Command(1)); err != nil {
			t.Fatal(err)
		}
		stage26ForceDue(t, admin, ctx, a.tenant.OrganizationID, id)
		var wg sync.WaitGroup
		start := make(chan struct{})
		var cancelErr, expireErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_, cancelErr = f.requestSvc.Change(ctx, a.tenant, id, "cancel", stage26Command(2))
		}()
		go func() { defer wg.Done(); <-start; _, expireErr = f.requestSvc.ExpireOne(ctx) }()
		close(start)
		wg.Wait()
		if expireErr != nil {
			t.Fatal(expireErr)
		}
		if cancelErr != nil {
			stage26RequireConflict(t, cancelErr)
		}
		after := stage26Snapshot(t, admin, ctx, f, id)
		if after.status != "cancelled" || after.version != 3 || after.reserved != 0 || after.allocations != 0 || after.releaseMoves != 1 || (after.reservationStatus != "expired" && after.reservationStatus != "released") {
			t.Fatal("double or partial release", after)
		}
		var audits int
		if err := admin.QueryRow(ctx, `SELECT count(*) FROM platform.order_events WHERE organization_id=$1 AND order_id=$2 AND kind='cancelled'`, a.tenant.OrganizationID, id).Scan(&audits); err != nil || audits != 1 {
			t.Fatal("double cancel audit", audits, err)
		}
	})

	t.Run("price-change-requires-new-approval-no-reserve", func(t *testing.T) {
		f := stage26Configure(t, d, ctx, a, 610)
		id := stage26Create(t, ctx, f, f.intake(1, &f.selection))
		if _, err := admin.Exec(ctx, `UPDATE platform.product_variants SET price_amount=price_amount+1,version=version+1 WHERE organization_id=$1 AND id=$2`, a.tenant.OrganizationID, f.product.VariantID); err != nil {
			t.Fatal(err)
		}
		_, err := f.requestSvc.Change(ctx, a.tenant, id, "confirm", stage26Command(1))
		stage26RequireConflict(t, err)
		state := stage26Snapshot(t, admin, ctx, f, id)
		stage26RequirePending(t, state)
		if state.reserved != 0 {
			t.Fatal("price drift acquired stock")
		}
	})
	t.Run("foreign-stock-and-slot-references-rejected-independently", func(t *testing.T) {
		own := stage26Configure(t, d, ctx, a, 611)
		foreign := stage26Configure(t, d, ctx, b, 612)
		for _, in := range []orders.Intake{own.intake(1, &foreign.selection), {ConversationID: a.conversation, IdempotencyKey: auth.UUID(), Kind: "hybrid_order", Details: orders.IntakeDetails{Selection: &own.selection, Items: []orders.CartItem{{VariantID: foreign.product.VariantID, Quantity: 1}}}}} {
			if _, err := own.requestSvc.PrepareIntake(ctx, a.tenant, in); err == nil {
				t.Fatal("foreign stock/slot reference accepted")
			}
			var count int
			if err := admin.QueryRow(ctx, `SELECT count(*) FROM platform.orders WHERE organization_id=$1 AND idempotency_key=$2`, a.tenant.OrganizationID, in.IdempotencyKey).Scan(&count); err != nil || count != 0 {
				t.Fatal("denied cross-tenant intake persisted", count, err)
			}
		}
	})
	t.Run("expiry-audit-failure-rolls-back-and-is-recoverable", func(t *testing.T) {
		f := stage26Configure(t, d, ctx, a, 613)
		id := stage26Create(t, ctx, f, f.intake(2, &f.selection))
		if _, err := f.requestSvc.Change(ctx, a.tenant, id, "confirm", stage26Command(1)); err != nil {
			t.Fatal(err)
		}
		stage26ForceDue(t, admin, ctx, a.tenant.OrganizationID, id)
		sql := fmt.Sprintf(`CREATE FUNCTION platform.stage26_fail_expiry_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.order_id='%s'::uuid AND NEW.reason='hybrid_reservation_expired' THEN RAISE EXCEPTION 'synthetic expiry audit failure'; END IF; RETURN NEW; END $$;
 CREATE TRIGGER stage26_fail_expiry_audit BEFORE INSERT ON platform.order_events FOR EACH ROW EXECUTE FUNCTION platform.stage26_fail_expiry_audit()`, id)
		if _, err := admin.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
		remove := func() {
			t.Helper()
			if _, err := admin.Exec(ctx, `DROP TRIGGER IF EXISTS stage26_fail_expiry_audit ON platform.order_events; DROP FUNCTION IF EXISTS platform.stage26_fail_expiry_audit()`); err != nil {
				t.Fatal(err)
			}
		}
		defer remove()
		if _, err := f.requestSvc.ExpireOne(ctx); err == nil {
			t.Fatal("expiry audit error ignored")
		}
		before := stage26Snapshot(t, admin, ctx, f, id)
		if before.status != "confirmed" || before.version != 2 || before.reserved != 2 || before.allocations != len(f.resources) || before.releaseMoves != 0 || before.reservationStatus != "active" {
			t.Fatal("failed expiry leaked partial release", before)
		}
		remove()
		if did, err := (orders.Service{Pool: d.Pool}).ExpireOne(ctx); err != nil || !did {
			t.Fatalf("durable expiry retry %t %v", did, err)
		}
		after := stage26Snapshot(t, admin, ctx, f, id)
		if after.status != "cancelled" || after.version != 3 || after.reserved != 0 || after.allocations != 0 || after.releaseMoves != 1 || after.reservationStatus != "expired" {
			t.Fatal("retry failed atomic expiry", after)
		}
	})

	t.Run("active-reservation-prevents-profile-switch-until-release", func(t *testing.T) {
		// A separate organization avoids relying on other subtests' remaining holds.
		c := seedStage23Owner(t, d, admin, ctx, identity, 82)
		if err := profiles.UpdateProfile(ctx, c.tenant, "hybrid", "mixed_nonfood", 1); err != nil {
			t.Fatal(err)
		}
		f := stage26Configure(t, d, ctx, c, 614)
		id := stage26Create(t, ctx, f, f.intake(1, &f.selection))
		if _, err := f.requestSvc.Change(ctx, c.tenant, id, "confirm", stage26Command(1)); err != nil {
			t.Fatal(err)
		}
		before, err := profiles.Team(ctx, c.tenant)
		if err != nil {
			t.Fatal(err)
		}
		reservationBefore := stage26Snapshot(t, admin, ctx, f, id)
		for _, target := range []struct{ model, industry string }{{"trade", "electronics"}, {"service", "vehicle_service"}, {"agency", "real_estate"}} {
			err = profiles.UpdateProfile(ctx, c.tenant, target.model, target.industry, before.Profile.Revision)
			var failure *auth.Error
			if !errors.As(err, &failure) || failure.Status != 422 {
				t.Fatalf("active hold allowed %s switch or wrong error: %v", target.model, err)
			}
			after, readErr := profiles.Team(ctx, c.tenant)
			if readErr != nil || after.Profile.Model != "hybrid" || after.Profile.Industry != "mixed_nonfood" || after.Profile.Revision != before.Profile.Revision {
				t.Fatalf("rejected profile change persisted: %+v %v", after.Profile, readErr)
			}
			if reservationAfter := stage26Snapshot(t, admin, ctx, f, id); reservationAfter != reservationBefore {
				t.Fatal("profile rejection changed active reservation", reservationAfter)
			}
		}
		// The rejection must leave cancellation available under the original profile.
		if _, err = f.requestSvc.Change(ctx, c.tenant, id, "cancel", stage26Command(2)); err != nil {
			t.Fatal("profile rejection broke cancellation", err)
		}
		released := stage26Snapshot(t, admin, ctx, f, id)
		if released.reserved != 0 || released.allocations != 0 || released.reservationStatus != "released" {
			t.Fatal("cancellation did not release both", released)
		}
		if err = profiles.UpdateProfile(ctx, c.tenant, "trade", "electronics", before.Profile.Revision); err != nil {
			t.Fatal("released hold still blocks profile switch", err)
		}
		after, err := profiles.Team(ctx, c.tenant)
		if err != nil || after.Profile.Model != "trade" || after.Profile.Industry != "electronics" || after.Profile.Revision != before.Profile.Revision+1 {
			t.Fatalf("post-release profile %+v %v", after.Profile, err)
		}
	})

}
func stage26Race(t *testing.T, admin *pgx.Conn, ctx context.Context, f stage26Fixture, first, second servicebooking.Selection, quantity int) {
	t.Helper()
	chat := conversations.Service{Pool: f.requestSvc.Pool, Provider: agent.Mock{}}
	raw, err := chat.Create(ctx, f.owner.tenant)
	var contender struct {
		ID string `json:"id"`
	}
	if err != nil || json.Unmarshal(raw, &contender) != nil || contender.ID == "" || contender.ID == f.owner.conversation {
		t.Fatalf("second customer conversation: %v", err)
	}
	secondIntake := f.intake(quantity, &second)
	secondIntake.ConversationID = contender.ID
	ids := []string{stage26Create(t, ctx, f, f.intake(quantity, &first)), stage26Create(t, ctx, f, secondIntake)}
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, 2)
	for i := range ids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, errs[i] = f.requestSvc.Change(ctx, f.owner.tenant, ids[i], "confirm", stage26Command(1))
		}(i)
	}
	close(start)
	wg.Wait()
	winner := -1
	for i, err := range errs {
		if err == nil {
			if winner >= 0 {
				t.Fatal("both contenders reserved constrained resources")
			}
			winner = i
		} else {
			stage26RequireConflict(t, err)
		}
	}
	if winner < 0 {
		t.Fatalf("no race winner: %v", errs)
	}
	won := stage26Snapshot(t, admin, ctx, f, ids[winner])
	lost := stage26Snapshot(t, admin, ctx, f, ids[1-winner])
	if won.reserved != quantity || won.allocations != len(f.resources) || won.reserveMoves != 1 || won.status != "confirmed" {
		t.Fatal("incomplete winner", won)
	}
	stage26RequirePending(t, lost)
	if _, err := f.requestSvc.Change(ctx, f.owner.tenant, ids[winner], "cancel", stage26Command(2)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.requestSvc.Change(ctx, f.owner.tenant, ids[1-winner], "confirm", stage26Command(1)); err != nil {
		t.Fatal("loser cannot reserve released stock/slot", err)
	}
}

func stage26ForceDue(t *testing.T, admin *pgx.Conn, ctx context.Context, org, id string) {
	t.Helper()
	tag, err := admin.Exec(ctx, `UPDATE platform.hybrid_reservations SET expires_at=clock_timestamp()-interval '1 second' WHERE organization_id=$1 AND order_id=$2 AND status='active'`, org, id)
	if err != nil || tag.RowsAffected() != 1 {
		t.Fatalf("synthetic due fixture %v", err)
	}
}
