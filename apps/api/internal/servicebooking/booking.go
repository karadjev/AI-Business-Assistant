package servicebooking

import (
	"bytes"
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	bf "github.com/karadjev/AI-Business-Assistant/apps/api/internal/businessfoundation"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
)

// ValidateSelectionTx rechecks current ownership/revision, explicit schedule
// layers, buffers and capacity. It creates no hold or binding reservation.
func ValidateSelectionTx(ctx context.Context, tx pgx.Tx, org string, selection Selection) error {
	_, err := validateSelection(ctx, tx, org, selection)
	return err
}
func validateSelection(ctx context.Context, tx pgx.Tx, org string, selection Selection) (Offering, error) {
	if err := guard(ctx, tx, org, false); err != nil {
		return Offering{}, err
	}
	return inspectSelection(ctx, tx, org, selection, true)
}

func inspectSelection(ctx context.Context, tx pgx.Tx, org string, selection Selection, lock bool) (Offering, error) {
	if err := selection.validate(); err != nil {
		return Offering{}, err
	}
	o, err := readOffering(ctx, tx, org, selection.OfferingID, lock)
	if err != nil {
		return o, err
	}
	if o.Revision != selection.OfferingRevision || o.Timezone != selection.Timezone {
		return o, conflict()
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return o, dbError(err)
	}
	if !selection.StartsAt.After(now) {
		return o, unavailable()
	}
	ref := func(id string) bf.Reference { return bf.Reference{OrganizationID: org, ID: id} }
	resources := []bf.Resource{}
	schedules := []bf.Availability{}
	allocations := []bf.Allocation{}
	// loadOffering sorted IDs. All multi-resource writers lock in this order.
	for _, id := range o.ResourceIDs {
		var r bf.Resource
		r.Reference = ref(id)
		var active bool
		query := `SELECT kind,capacity,active FROM platform.service_resources WHERE organization_id=$1 AND id=$2`
		if lock {
			query += " FOR UPDATE"
		}
		err = tx.QueryRow(ctx, query, org, id).Scan(&r.Kind, &r.Capacity, &active)
		if err != nil {
			return o, dbError(err)
		}
		if !active {
			return o, unavailable()
		}
		resources = append(resources, r)
	}
	bufferedStart := selection.StartsAt.Add(-time.Duration(o.BufferBefore) * time.Minute)
	bufferedEnd := selection.EndsAt.Add(time.Duration(o.BufferAfter) * time.Minute)
	for _, id := range o.ResourceIDs {
		a := bf.Availability{Resource: ref(id), Timezone: o.Timezone, Known: true}
		rows, e := tx.Query(ctx, `SELECT layer,starts_at,ends_at,timezone FROM platform.service_availability_windows WHERE organization_id=$1 AND offering_id=$2 AND resource_id=$3 AND starts_at<$5 AND ends_at>$4 ORDER BY starts_at,id LIMIT 513`, org, o.ID, id, bufferedStart, bufferedEnd)
		if e != nil {
			return o, dbError(e)
		}
		count := 0
		for rows.Next() {
			var layer, zone string
			var i bf.Interval
			if e = rows.Scan(&layer, &i.Start, &i.End, &zone); e != nil {
				rows.Close()
				return o, dbError(e)
			}
			count++
			if zone != o.Timezone {
				rows.Close()
				return o, unavailable()
			}
			switch layer {
			case "business":
				a.Business = append(a.Business, i)
			case "offering":
				a.Offering = append(a.Offering, i)
			case "shift":
				a.Shift = append(a.Shift, i)
			case "exception":
				a.Exceptions = append(a.Exceptions, i)
			}
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return o, dbError(e)
		}
		if count > 512 {
			return o, unavailable()
		}
		schedules = append(schedules, a)
		rows, e = tx.Query(ctx, `SELECT starts_at,ends_at,units FROM platform.service_booking_allocations WHERE organization_id=$1 AND resource_id=$2 AND active AND starts_at<$4 AND ends_at>$3 ORDER BY starts_at,booking_id LIMIT 1025`, org, id, bufferedStart, bufferedEnd)
		if e != nil {
			return o, dbError(e)
		}
		for rows.Next() {
			allocation := bf.Allocation{Resource: ref(id)}
			if e = rows.Scan(&allocation.Interval.Start, &allocation.Interval.End, &allocation.Units); e != nil {
				rows.Close()
				return o, dbError(e)
			}
			allocations = append(allocations, allocation)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return o, dbError(e)
		}
		if len(allocations) > 1024 {
			return o, unavailable()
		}
	}
	proposal := bf.BookingRequest{Order: ref(o.ID), Offering: ref(o.ID), Slot: bf.Interval{Start: selection.StartsAt, End: selection.EndsAt}, Units: selection.Units}
	if err = bf.ValidateBooking(org, proposal, o.domain(org), resources, schedules, allocations); err != nil {
		return o, unavailable()
	}
	return o, nil
}
func (s Service) Availability(ctx context.Context, t catalog.Tenant, selection Selection) (Availability, error) {
	tx, err := begin(ctx, s.Pool, t)
	if err != nil {
		return Availability{}, err
	}
	defer tx.Rollback(ctx)
	if err = ValidateSelectionTx(ctx, tx, t.OrganizationID, selection); err != nil {
		return Availability{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Availability{}, dbError(err)
	}
	return Availability{Available: true, State: "available_nonbinding", Selection: selection}, nil
}

// DelegatedAvailability reads one lease-bound offering from a consistent snapshot.
func (s Service) DelegatedAvailability(ctx context.Context, t catalog.Tenant, selection Selection, checkSlot bool) (Offering, error) {
	if t.AgentTaskID == "" || t.AgentLease == "" || !organizations.ValidID(t.OrganizationID) || !organizations.ValidID(selection.OfferingID) {
		return Offering{}, tenancy.Failure(403, "FORBIDDEN", "Требуется доступ задачи к услуге")
	}
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return Offering{}, dbError(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT set_config('app.agent_task',$1,true),set_config('app.agent_lease',$2,true),set_config('app.organization_id',$3,true)", t.AgentTaskID, t.AgentLease, t.OrganizationID); err != nil {
		return Offering{}, dbError(err)
	}
	var allowed bool
	if err = tx.QueryRow(ctx, "SELECT coalesce(platform.agent_delegation_offering($1)=$2::uuid,false)", t.OrganizationID, selection.OfferingID).Scan(&allowed); err != nil {
		return Offering{}, dbError(err)
	}
	if !allowed {
		return Offering{}, tenancy.Failure(403, "FORBIDDEN", "Услуга вне области задачи")
	}
	if checkSlot {
		return inspectSelection(ctx, tx, t.OrganizationID, selection, false)
	}
	return readOffering(ctx, tx, t.OrganizationID, selection.OfferingID, false)
}

// ConfirmTx must follow the parent order row lock and status/consent transition
// in the SAME transaction. Failure must abort that transaction. No network calls.
func ConfirmTx(ctx context.Context, tx pgx.Tx, org, requestID string, selection Selection) error {
	if err := guard(ctx, tx, org, false); err != nil {
		return err
	}
	if !organizations.ValidID(requestID) {
		return invalid()
	}
	if err := selection.validate(); err != nil {
		return err
	}
	var status, kind string
	err := tx.QueryRow(ctx, `SELECT status,request_kind FROM platform.orders WHERE organization_id=$1 AND id=$2 FOR UPDATE`, org, requestID).Scan(&status, &kind)
	if err != nil {
		return dbError(err)
	}
	if status != "confirmed" || (kind != "service_booking" && kind != "hybrid_order" && kind != "property_viewing_request") {
		return conflict()
	}
	hash := selection.hash()
	var stored []byte
	var bookingStatus string
	err = tx.QueryRow(ctx, `SELECT selection_hash,status FROM platform.service_bookings WHERE organization_id=$1 AND request_id=$2`, org, requestID).Scan(&stored, &bookingStatus)
	if err == nil {
		if bytes.Equal(stored, hash) && bookingStatus == "confirmed" {
			return nil
		}
		return conflict()
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return dbError(err)
	}
	o, err := validateSelection(ctx, tx, org, selection)
	if err != nil {
		return err
	}
	var bookingID string
	err = tx.QueryRow(ctx, `INSERT INTO platform.service_bookings(organization_id,request_id,offering_id,offering_revision,starts_at,ends_at,timezone,units,selection_hash) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id::text`, org, requestID, o.ID, o.Revision, selection.StartsAt, selection.EndsAt, selection.Timezone, selection.Units, hash).Scan(&bookingID)
	if err != nil {
		return dbError(err)
	}
	for _, id := range o.ResourceIDs {
		_, err = tx.Exec(ctx, `INSERT INTO platform.service_booking_allocations(organization_id,booking_id,resource_id,starts_at,ends_at,units) VALUES($1,$2,$3,$4,$5,$6)`, org, bookingID, id, selection.StartsAt.Add(-time.Duration(o.BufferBefore)*time.Minute), selection.EndsAt.Add(time.Duration(o.BufferAfter)*time.Minute), selection.Units)
		if err != nil {
			return dbError(err)
		}
	}
	return nil
}

// CancelTx releases allocations atomically with the parent's cancelled status.
// Missing bookings (cancel before confirmation) and repeated cancellation are safe.
func CancelTx(ctx context.Context, tx pgx.Tx, org, requestID string) error {
	return finishBookingTx(ctx, tx, org, requestID, "cancelled")
}

func CompleteHybridTx(ctx context.Context, tx pgx.Tx, org, requestID string) error {
	return finishBookingTx(ctx, tx, org, requestID, "completed")
}

func finishBookingTx(ctx context.Context, tx pgx.Tx, org, requestID, target string) error {
	if err := guard(ctx, tx, org, false); err != nil {
		return err
	}
	if !organizations.ValidID(requestID) {
		return invalid()
	}
	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM platform.orders WHERE organization_id=$1 AND id=$2 FOR UPDATE`, org, requestID).Scan(&status); err != nil {
		return dbError(err)
	}
	if status != target {
		return conflict()
	}
	var id string
	err := tx.QueryRow(ctx, `SELECT id::text FROM platform.service_bookings WHERE organization_id=$1 AND request_id=$2 FOR UPDATE`, org, requestID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return dbError(err)
	}
	// All cancellation paths lock resources in the same order as confirmation.
	rows, err := tx.Query(ctx, `SELECT r.id FROM platform.service_resources r JOIN platform.service_booking_allocations a ON a.organization_id=r.organization_id AND a.resource_id=r.id WHERE a.organization_id=$1 AND a.booking_id=$2 ORDER BY r.id FOR UPDATE OF r`, org, id)
	if err != nil {
		return dbError(err)
	}
	for rows.Next() {
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return dbError(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE platform.service_bookings SET status=$3 WHERE organization_id=$1 AND id=$2 AND status='confirmed'`, org, id, target); err != nil {
		return dbError(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE platform.service_booking_allocations SET active=false WHERE organization_id=$1 AND booking_id=$2 AND active`, org, id); err != nil {
		return dbError(err)
	}
	return nil
}
