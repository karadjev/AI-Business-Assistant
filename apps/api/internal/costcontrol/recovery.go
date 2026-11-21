package costcontrol

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
)

// RecoverStale never calls a provider, refunds money, or raises any budget limit.
// A fixed server-side age fence prevents recovery of an ordinary active cycle.
func (s Service) RecoverStale(ctx context.Context) (int, error) {
	var n int
	e := s.Pool.QueryRow(ctx, `SELECT platform.llm_recover_stale()`).Scan(&n)
	if e != nil {
		return 0, auth.Unavailable()
	}
	return n, nil
}
func (s Service) Recover(ctx context.Context, t catalog.Tenant, id string) error {
	if !organizations.ValidID(id) {
		return auth.Failure(404, "NOT_FOUND", "Резерв не найден")
	}
	tx, e := tenancy.Begin(ctx, s.Pool, t, true, true)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	_, e = tx.Exec(ctx, `SELECT platform.llm_recover_one($1,$2)`, t.OrganizationID, id)
	if e != nil {
		var state interface{ SQLState() string }
		if errors.As(e, &state) {
			switch state.SQLState() {
			case "40001":
				return auth.Failure(409, "RESERVATION_ACTIVE", "Резерв ещё может быть активен")
			case "P0002":
				return auth.Failure(404, "NOT_FOUND", "Резерв не найден")
			}
		}
		return tenancy.Error(e)
	}
	return tx.Commit(ctx)
}
func (s Service) Reservations(ctx context.Context, t catalog.Tenant) (json.RawMessage, error) {
	tx, e := tenancy.Begin(ctx, s.Pool, t, true, true)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	var b []byte
	e = tx.QueryRow(ctx, `SELECT jsonb_build_object('items',coalesce(jsonb_agg(x),'[]'::jsonb)) FROM (SELECT r.id,r.status,r.amount,r.charged,r.created_at,r.completed_at,(SELECT coalesce(jsonb_agg(to_jsonb(e) ORDER BY e.id),'[]'::jsonb) FROM platform.llm_budget_events e WHERE e.reservation_id=r.id) AS events FROM platform.llm_reservations r WHERE r.organization_id=$1 ORDER BY r.created_at DESC,r.id LIMIT 50) x`, t.OrganizationID).Scan(&b)
	if e != nil {
		return nil, tenancy.Error(e)
	}
	return b, nil
}
