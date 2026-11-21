package catalog

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
)

type StoreProfile struct {
	Name             string `json:"name"`
	BusinessCategory string `json:"business_category"`
	Description      string `json:"description"`
	Address          string `json:"address"`
	OpeningHours     string `json:"opening_hours"`
	Contacts         string `json:"contacts"`
	DeliveryTerms    string `json:"delivery_terms"`
	PaymentMethods   string `json:"payment_methods"`
	ReturnTerms      string `json:"return_terms"`
	AIInstructions   string `json:"ai_instructions"`
	Version          int64  `json:"expected_version"`
}

func (p StoreProfile) valid() bool {
	return textValid(p.Name, 1, 120) && textValid(p.BusinessCategory, 1, 100) && textValid(p.Description, 1, 2000) && textValid(p.Address, 1, 500) && textValid(p.OpeningHours, 1, 500) && textValid(p.Contacts, 1, 500) && textValid(p.DeliveryTerms, 1, 2000) && textValid(p.PaymentMethods, 1, 1000) && textValid(p.ReturnTerms, 1, 2000) && textValid(p.AIInstructions, 0, 2000)
}
func audit(ctx context.Context, tx pgx.Tx, t Tenant, job, kind string) error {
	var id any
	if job != "" {
		id = job
	}
	_, e := tx.Exec(ctx, `INSERT INTO platform.catalog_events(organization_id,import_id,actor_id,kind) VALUES($1,$2,platform.current_user_id(),$3)`, t.OrganizationID, id, kind)
	return dbErrorOrNil(e)
}
func dbErrorOrNil(e error) error {
	if e != nil {
		return dbError(e)
	}
	return nil
}
func (s Service) SaveProfile(ctx context.Context, t Tenant, p StoreProfile) (json.RawMessage, error) {
	if !p.valid() || p.Version < 0 {
		return nil, invalid("Заполните профиль; проверьте длину полей и версию")
	}
	tx, e := s.begin(ctx, t, true)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	var v int64
	e = tx.QueryRow(ctx, `SELECT version FROM platform.store_profiles WHERE organization_id=$1 FOR UPDATE`, t.OrganizationID).Scan(&v)
	if e != nil && e != pgx.ErrNoRows {
		return nil, dbError(e)
	}
	if v != p.Version {
		return nil, conflict()
	}
	_, e = tx.Exec(ctx, `UPDATE platform.organizations SET name=$2,version=version+1,updated_at=now() WHERE id=$1`, t.OrganizationID, strings.TrimSpace(p.Name))
	if e != nil {
		return nil, dbError(e)
	}
	raw, e := object(ctx, tx, `INSERT INTO platform.store_profiles(organization_id,business_category,description,address,opening_hours,contacts,delivery_terms,payment_methods,return_terms,ai_instructions) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(organization_id) DO UPDATE SET business_category=excluded.business_category,description=excluded.description,address=excluded.address,opening_hours=excluded.opening_hours,contacts=excluded.contacts,delivery_terms=excluded.delivery_terms,payment_methods=excluded.payment_methods,return_terms=excluded.return_terms,ai_instructions=excluded.ai_instructions,version=store_profiles.version+1,updated_at=now() RETURNING to_jsonb(store_profiles.*)`, t.OrganizationID, p.BusinessCategory, p.Description, p.Address, p.OpeningHours, p.Contacts, p.DeliveryTerms, p.PaymentMethods, p.ReturnTerms, p.AIInstructions)
	if e == nil {
		e = audit(ctx, tx, t, "", "PROFILE_SAVED")
	}
	return commit(ctx, tx, raw, e)
}
func onboarding(ctx context.Context, tx pgx.Tx, t Tenant) (json.RawMessage, error) {
	return object(ctx, tx, `SELECT jsonb_build_object('profile',coalesce((SELECT to_jsonb(p) FROM platform.store_profiles p WHERE organization_id=$1),'{}'::jsonb),'name',(SELECT name FROM platform.organizations WHERE id=$1),'status',coalesce((SELECT status FROM platform.store_profiles WHERE organization_id=$1),'NOT_STARTED'),'catalog_count',(SELECT count(*) FROM platform.products WHERE organization_id=$1 AND status<>'archived'),'import_errors',(SELECT count(*) FROM platform.catalog_imports WHERE organization_id=$1 AND status='PREVIEW' AND ((preview->>'errors')::integer>0 OR (preview->>'conflicts')::integer>0)),'telegram_configured',EXISTS(SELECT 1 FROM platform.telegram_integrations WHERE organization_id=$1 AND active),'whatsapp_configured',EXISTS(SELECT 1 FROM platform.channel_integrations WHERE channel='whatsapp' AND organization_id=$1 AND active),'imports',coalesce((SELECT jsonb_agg(jsonb_build_object('id',id,'status',status)) FROM (SELECT id,status FROM platform.catalog_imports WHERE organization_id=$1 ORDER BY created_at DESC LIMIT 100)x),'[]'::jsonb),'live_enabled',false,'blockers',jsonb_build_array('LIVE_DISABLED','LLM_BUDGET_ZERO','WHATSAPP_BUDGET_ZERO'))`, t.OrganizationID)
}
func (s Service) Onboarding(ctx context.Context, t Tenant) (json.RawMessage, error) {
	tx, e := s.begin(ctx, t, true)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	return onboarding(ctx, tx, t)
}
func (s Service) OnboardingStep(ctx context.Context, t Tenant, status string, version int64, confirm bool) (json.RawMessage, error) {
	if !confirm || version < 1 {
		return nil, invalid("Нужны версия и подтверждение")
	}
	tx, e := s.begin(ctx, t, true)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	var current string
	var v int64
	e = tx.QueryRow(ctx, `SELECT status,version FROM platform.store_profiles WHERE organization_id=$1 FOR UPDATE`, t.OrganizationID).Scan(&current, &v)
	if e != nil {
		return nil, dbError(e)
	}
	if v != version {
		return nil, conflict()
	}
	var count, errors int
	var channels bool
	e = tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM platform.products WHERE organization_id=$1 AND status<>'archived'),EXISTS(SELECT 1 FROM platform.telegram_integrations WHERE organization_id=$1 AND active) OR EXISTS(SELECT 1 FROM platform.channel_integrations i WHERE organization_id=$1 AND active AND (i.channel='whatsapp' OR EXISTS(SELECT 1 FROM platform.instagram_connections c WHERE c.integration_id=i.id AND c.organization_id=i.organization_id AND c.status='connected' AND c.webhook_seen_at IS NOT NULL AND c.token_expires_at>clock_timestamp() AND c.revoked_at IS NULL))),(SELECT count(*) FROM platform.catalog_imports WHERE organization_id=$1 AND status='PREVIEW' AND ((preview->>'errors')::integer>0 OR (preview->>'conflicts')::integer>0))`, t.OrganizationID).Scan(&count, &channels, &errors)
	if e != nil {
		return nil, dbError(e)
	}
	if count == 0 || errors > 0 || !(status == "CATALOG_READY" || status == "CHANNELS_PENDING" || status == "READY_FOR_REVIEW" || status == "ACTIVE") || (status == "ACTIVE" && (current != "READY_FOR_REVIEW" || !channels)) || (status == "READY_FOR_REVIEW" && !channels) {
		return nil, auth.Failure(409, "ONBOARDING_BLOCKED", "Нужны каталог, отсутствие ошибок и настроенный mock-канал; ACTIVE требует предварительной проверки")
	}
	_, e = tx.Exec(ctx, `UPDATE platform.store_profiles SET status=$2,version=version+1,updated_at=now() WHERE organization_id=$1`, t.OrganizationID, status)
	if e == nil {
		e = audit(ctx, tx, t, "", "ONBOARDING_"+status)
	}
	if e != nil {
		return nil, dbErrorOrNil(e)
	}
	raw, e := onboarding(ctx, tx, t)
	return commit(ctx, tx, raw, e)
}
