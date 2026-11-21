package telegram

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
)

type DeliveryResolution struct {
	ExpectedVersion int64  `json:"expected_version"`
	Decision        string `json:"decision"`
	IdempotencyKey  string `json:"idempotency_key"`
	Confirm         bool   `json:"confirm_resolution"`
}

func (s Service) ResolveDelivery(ctx context.Context, t catalog.Tenant, id string, in DeliveryResolution) (json.RawMessage, error) {
	if !organizations.ValidID(id) || !organizations.ValidID(in.IdempotencyKey) || in.ExpectedVersion < 1 || !in.Confirm || (in.Decision != "delivered" && in.Decision != "abandon") {
		return nil, auth.Failure(422, "INVALID_RESOLUTION", "Подтвердите решение и текущую версию доставки; повторная отправка не разрешена")
	}
	tx, e := s.begin(ctx, t, false)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	var b []byte
	e = tx.QueryRow(ctx, `SELECT platform.telegram_resolve_delivery($1,$2,$3,$4,$5)`, t.OrganizationID, id, in.ExpectedVersion, in.Decision, in.IdempotencyKey).Scan(&b)
	if e != nil {
		return nil, recoveryError(e)
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, tenancy.Error(e)
	}
	return b, nil
}
func recoveryError(e error) error {
	var state interface{ SQLState() string }
	if errors.As(e, &state) {
		switch state.SQLState() {
		case "40001":
			return auth.Failure(409, "DELIVERY_CONFLICT", "Доставка изменилась или этот ключ использован для другого решения")
		case "P0002":
			return auth.Failure(404, "NOT_FOUND", "Доставка не найдена")
		}
	}
	return tenancy.Error(e)
}
func (s Service) Deliveries(ctx context.Context, t catalog.Tenant) (json.RawMessage, error) {
	tx, e := s.begin(ctx, t, false)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	var b []byte
	e = tx.QueryRow(ctx, `SELECT jsonb_build_object('items',coalesce(jsonb_agg(x),'[]'::jsonb)) FROM (SELECT q.id,q.status,q.version,q.attempts,q.last_error,q.created_at,(SELECT coalesce(jsonb_agg(to_jsonb(e) ORDER BY e.id),'[]'::jsonb) FROM platform.telegram_delivery_events e WHERE e.outbox_id=q.id) AS events FROM platform.telegram_outbox q WHERE q.organization_id=$1 AND q.status IN ('uncertain','failed') ORDER BY (q.status='uncertain') DESC,q.created_at,q.id LIMIT 50) x`, t.OrganizationID).Scan(&b)
	if e != nil {
		return nil, tenancy.Error(e)
	}
	return b, nil
}
