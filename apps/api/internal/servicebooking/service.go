package servicebooking

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
)

type Service struct{ Pool *pgxpool.Pool }

func dbError(err error) error {
	var p *pgconn.PgError
	if errors.As(err, &p) && p.Code == "23P01" {
		return unavailable()
	}
	return tenancy.Error(err)
}
func begin(ctx context.Context, pool *pgxpool.Pool, t catalog.Tenant) (pgx.Tx, error) {
	// Channel scopes cannot exercise staff booking admission/configuration.
	if pool == nil || t.ChannelCapability != "" || t.ChannelType != "" || t.AgentLease != "" || t.AgentTaskID != "" {
		return nil, tenancy.Failure(403, "FORBIDDEN", "Требуется доступ сотрудника")
	}
	tx, err := tenancy.Begin(ctx, pool, t, true, true)
	if err != nil {
		return nil, err
	}
	if err = guard(ctx, tx, t.OrganizationID, false); err != nil {
		_ = tx.Rollback(ctx)
		return nil, err
	}
	return tx, nil
}
func guard(ctx context.Context, tx pgx.Tx, org string, owner bool) error {
	if tx == nil || !organizations.ValidID(org) {
		return invalid()
	}
	var allowed bool
	err := tx.QueryRow(ctx, `SELECT platform.service_booking_scope($1::uuid) AND (NOT $2::boolean OR platform.has_membership($1::uuid,true))`, org, owner).Scan(&allowed)
	if err != nil {
		return dbError(err)
	}
	if !allowed {
		return tenancy.Failure(403, "FORBIDDEN", "Недостаточно прав для услуги")
	}
	return nil
}

const offeringColumns = `id::text,name,status,revision,price_model,price_state,amount_minor,currency,min_minutes,max_minutes,buffer_before,buffer_after,timezone,part_rule`

func scanOffering(row pgx.Row) (Offering, error) {
	var o Offering
	err := row.Scan(&o.ID, &o.Name, &o.Status, &o.Revision, &o.PriceModel, &o.PriceState, &o.AmountMinor, &o.Currency, &o.MinMinutes, &o.MaxMinutes, &o.BufferBefore, &o.BufferAfter, &o.Timezone, &o.PartRule)
	if err == nil && !validPartRule(o.PartRule) {
		o.PartRule = &PartRule{State: "UNKNOWN"}
	}
	return o, err
}
func loadOffering(ctx context.Context, tx pgx.Tx, org, id string) (Offering, error) {
	return readOffering(ctx, tx, org, id, true)
}

func readOffering(ctx context.Context, tx pgx.Tx, org, id string, lock bool) (Offering, error) {
	if !organizations.ValidID(id) {
		return Offering{}, invalid()
	}
	query := `SELECT ` + offeringColumns + ` FROM platform.service_offerings WHERE organization_id=$1 AND id=$2`
	if lock {
		query += " FOR SHARE"
	}
	o, err := scanOffering(tx.QueryRow(ctx, query, org, id))
	if err != nil {
		return o, dbError(err)
	}
	rows, err := tx.Query(ctx, `SELECT resource_id::text FROM platform.service_offering_resources WHERE organization_id=$1 AND offering_id=$2 ORDER BY resource_id LIMIT 17`, org, id)
	if err != nil {
		return o, dbError(err)
	}
	defer rows.Close()
	o.ResourceIDs = []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return o, dbError(err)
		}
		o.ResourceIDs = append(o.ResourceIDs, id)
	}
	if err = rows.Err(); err != nil {
		return o, dbError(err)
	}
	if err = o.validate(org); err != nil {
		return o, err
	}
	return o, nil
}
func (s Service) Search(ctx context.Context, t catalog.Tenant, query string, limit int) ([]Offering, error) {
	if len(query) > 200 || strings.ContainsRune(query, 0) || limit < 1 || limit > 30 {
		return nil, invalid()
	}
	tx, err := begin(ctx, s.Pool, t)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT `+offeringColumns+` FROM platform.service_offerings WHERE organization_id=$1 AND status='active' AND position(lower($2) in lower(name))>0 ORDER BY name,id LIMIT $3`, t.OrganizationID, query, limit)
	if err != nil {
		return nil, dbError(err)
	}
	result := []Offering{}
	for rows.Next() {
		o, e := scanOffering(rows)
		if e != nil {
			rows.Close()
			return nil, dbError(e)
		}
		o.ResourceIDs = []string{}
		result = append(result, o)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, dbError(err)
	}
	// Search returns public descriptors, never resource names or private schedules.
	if err = tx.Commit(ctx); err != nil {
		return nil, dbError(err)
	}
	return result, nil
}
func (s Service) Resources(ctx context.Context, t catalog.Tenant) ([]Resource, error) {
	tx, err := begin(ctx, s.Pool, t)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err = guard(ctx, tx, t.OrganizationID, true); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT id::text,name,kind,capacity,active,revision FROM platform.service_resources WHERE organization_id=$1 ORDER BY id LIMIT 100`, t.OrganizationID)
	if err != nil {
		return nil, dbError(err)
	}
	defer rows.Close()
	out := []Resource{}
	for rows.Next() {
		var r Resource
		if err = rows.Scan(&r.ID, &r.Name, &r.Kind, &r.Capacity, &r.Active, &r.Revision); err != nil {
			return nil, dbError(err)
		}
		out = append(out, r)
	}
	if err = rows.Err(); err != nil {
		return nil, dbError(err)
	}
	rows.Close()
	if err = tx.Commit(ctx); err != nil {
		return nil, dbError(err)
	}
	return out, nil
}

// ValidateOfferingTx checks active service ownership for non-binding repair or
// hybrid intake. It never promises a slot, a zero price, or resource capacity.
func ValidateOfferingTx(ctx context.Context, tx pgx.Tx, org, id string) error {
	if err := guard(ctx, tx, org, false); err != nil {
		return err
	}
	o, err := loadOffering(ctx, tx, org, id)
	if err != nil {
		return err
	}
	if o.Status != "active" {
		return unavailable()
	}
	return nil
}

// Keep generated IDs backend-owned; callers supply expected revisions for updates.
func newID(id string) string {
	if id == "" {
		return auth.UUID()
	}
	return id
}
