package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/channel"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
)

// FinancialAdmission uses the same pinned connection as the worker integration
// lock. No transaction is kept open during HTTP. Simulation is a DB-constrained
// test realm and can only be used by an explicitly offline transport.
type FinancialAdmission struct {
	Conn       *pgxpool.Conn
	Proof      []byte
	Capability string
	Simulation bool
}

var _ SendAdmission = (*FinancialAdmission)(nil)

func (a *FinancialAdmission) OfflineOnly() bool { return a == nil || a.Simulation }
func (a *FinancialAdmission) Reserve(ctx context.Context, c Config, m channel.Message, t *Template) (string, error) {
	if a == nil || a.Conn == nil || len(a.Proof) != 32 || !organizations.ValidID(a.Capability) || !organizations.ValidID(m.CorrelationID) || m.OrganizationID != c.OrganizationID || m.IntegrationID != c.IntegrationID {
		return "", channel.ErrRejected
	}
	name, language := "", ""
	params := []string{}
	if t != nil {
		name = t.Name
		language = t.Language
		params = t.Parameters
	}
	raw, _ := json.Marshal(params)
	actions := m.Actions
	if actions == nil {
		actions = []channel.Action{}
	}
	act, _ := json.Marshal(actions)
	var id *string
	err := a.Conn.QueryRow(ctx, `SELECT platform.wa_reserve($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, a.Proof, a.Capability, m.CorrelationID, m.OrganizationID, m.IntegrationID, c.PhoneNumberID, c.BusinessAccountID, c.GraphVersion, m.ExternalUserID, m.Text, act, name, language, raw, a.Simulation).Scan(&id)
	if err != nil || id == nil {
		return "", &CloudError{Code: "FINANCIAL_ADMISSION_DENIED"}
	}
	return *id, nil
}
func (a *FinancialAdmission) Finish(ctx context.Context, id string, r channel.SendResult) error {
	if a == nil || a.Conn == nil || !organizations.ValidID(id) {
		return channel.ErrRejected
	}
	_, e := a.Conn.Exec(ctx, `SELECT platform.wa_finish($1,$2,$3,$4,$5)`, a.Proof, id, string(r.State), r.ProviderID, r.NoChargeConfirmed)
	if e != nil {
		return &CloudError{Code: "FINANCIAL_SETTLEMENT_UNKNOWN"}
	}
	return nil
}
func (s Service) RecoverFinancial(ctx context.Context) error {
	if len(s.Key) != 32 {
		return nil
	}
	_, e := s.Pool.Exec(ctx, `SELECT platform.wa_recover($1),platform.wa_expire_provider($1)`, s.Proof())
	return e
}
func (s Service) FinancialState(ctx context.Context, t catalog.Tenant) (json.RawMessage, error) {
	tx, e := s.begin(ctx, t)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	var owner bool
	if e = tx.QueryRow(ctx, `SELECT platform.has_membership($1,true)`, t.OrganizationID).Scan(&owner); e != nil || !owner {
		return nil, auth.Failure(403, "FORBIDDEN", "Финансовые данные доступны owner")
	}
	var raw []byte
	e = tx.QueryRow(ctx, `SELECT jsonb_build_object('mode','mock','live_enabled',false,'currency','RUB','unit','microRUB','budget',coalesce((SELECT jsonb_build_object('limit',limit_amount,'reserved',reserved,'spent',spent,'available',limit_amount-reserved-spent,'synthetic',synthetic) FROM platform.wa_budgets WHERE organization_id=$1),'{"limit":0,"reserved":0,"spent":0,"available":0,"synthetic":false}'::jsonb),'reasons',jsonb_build_array('LIVE_DISABLED','NO_VERIFIED_LIVE_PRICING'),'blocked_operations',(SELECT count(*) FROM platform.wa_financial_events WHERE organization_id=$1 AND reservation_id IS NULL AND kind IN ('STALE_OR_DISABLED','LIVE_DISABLED','DUPLICATE_OR_UNKNOWN','TEMPLATE_BLOCKED','WINDOW_CLOSED','PRICING_UNVERIFIED','BUDGET_EXHAUSTED')),'reservations',coalesce((SELECT jsonb_agg(jsonb_build_object('id',id,'status',status,'amount',amount,'charged',charged,'version',version,'synthetic',synthetic)) FROM (SELECT * FROM platform.wa_reservations WHERE organization_id=$1 ORDER BY created_at DESC LIMIT 50)x),'[]'::jsonb))`, t.OrganizationID).Scan(&raw)
	if e != nil {
		return nil, tenancy.Error(e)
	}
	return raw, nil
}
func (s Service) Templates(ctx context.Context, t catalog.Tenant) (json.RawMessage, error) {
	tx, e := s.begin(ctx, t)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	var owner bool
	if e = tx.QueryRow(ctx, `SELECT platform.has_membership($1,true)`, t.OrganizationID).Scan(&owner); e != nil || !owner {
		return nil, auth.Failure(403, "FORBIDDEN", "Реестр доступен owner")
	}
	var raw []byte
	e = tx.QueryRow(ctx, `SELECT coalesce(jsonb_agg(jsonb_build_object('id',id,'name',name,'language',language,'category',category,'status',status,'verified_at',verified_at,'verified_until',verified_until,'source',source,'policy_version',policy_version,'synthetic',synthetic,'provenance_status',platform.wa_resource_status(organization_id,'template',id),'unavailable_reason',CASE WHEN platform.wa_resource_status(organization_id,'template',id) IN ('REVOKED','EXPIRED','INTEGRITY_MISMATCH','UNVERIFIED') THEN platform.wa_resource_status(organization_id,'template',id) WHEN synthetic THEN 'SYNTHETIC_NOT_META_APPROVAL' WHEN verified_until<=now() THEN 'VERIFICATION_EXPIRED' WHEN status<>'APPROVED' THEN 'NOT_APPROVED' ELSE 'LIVE_DISABLED' END)),'[]'::jsonb) FROM (SELECT * FROM platform.wa_templates WHERE organization_id=$1 ORDER BY name,language LIMIT 100)x`, t.OrganizationID).Scan(&raw)
	if e != nil {
		return nil, tenancy.Error(e)
	}
	return raw, nil
}
func (s Service) Reconcile(ctx context.Context, t catalog.Tenant, id string, v, charge int64, evidence string) error {
	if !organizations.ValidID(id) || v < 1 || charge < 0 || len(evidence) < 10 || len(evidence) > 1000 {
		return auth.Failure(422, "VALIDATION_ERROR", "Нужны версия, сумма и ссылка на подтверждение сверки")
	}
	tx, e := s.begin(ctx, t)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	_, e = tx.Exec(ctx, `SELECT platform.wa_reconcile($1,$2,$3,$4,$5)`, t.OrganizationID, id, v, charge, evidence)
	if e != nil {
		var state interface{ SQLState() string }
		if errors.As(e, &state) {
			if state.SQLState() == "P0002" {
				return auth.Failure(404, "NOT_FOUND", "Ресурс не найден")
			}
			if state.SQLState() == "40001" {
				return auth.Failure(409, "VERSION_CONFLICT", "Финансовая операция изменилась")
			}
		}
		return tenancy.Error(e)
	}
	return tx.Commit(ctx)
}

// Verifier is a backend trust boundary, not a request schema. No live verifier is
// installed. Mock attestations are irreversibly persisted as SYNTHETIC_MOCK.
type TemplateRecord struct {
	ID, Name, Language, Category, Status string
	Components                           json.RawMessage
	ParameterSchema                      []string
	VerifiedUntil                        time.Time
}
type TemplateVerifier interface {
	Verify(context.Context, Config, string, string) (TemplateRecord, error)
	OfflineOnly() bool
}
type MockTemplateVerifier struct{ Record TemplateRecord }

func (MockTemplateVerifier) OfflineOnly() bool { return true }
func (v MockTemplateVerifier) Verify(ctx context.Context, c Config, name, lang string) (TemplateRecord, error) {
	if ctx.Err() != nil || v.Record.Name != name || v.Record.Language != lang {
		return TemplateRecord{}, channel.ErrRejected
	}
	return v.Record, nil
}
func (s Service) VerifyTemplate(ctx context.Context, c Config, name, lang string, v TemplateVerifier) error {
	if len(s.Key) != 32 || v == nil || !v.OfflineOnly() {
		return errors.New("live template verification is disabled")
	}
	r, e := v.Verify(ctx, c, name, lang)
	if e != nil {
		return channel.ErrRejected
	}
	if !organizations.ValidID(r.ID) || r.Name != name || r.Language != lang || !templateName.MatchString(r.Name) || !templateLanguage.MatchString(r.Language) {
		return channel.ErrRejected
	}
	payload, _ := json.Marshal(map[string]any{"waba_id": c.BusinessAccountID, "name": r.Name, "language": r.Language, "category": r.Category, "status": r.Status, "components": r.Components, "parameter_schema": r.ParameterSchema})
	var expected int64
	// Backend proof, not an owner-controlled approval field, selects the current revision.
	err := s.Pool.QueryRow(ctx, `SELECT platform.wa_provider_version($1,$2,$3)`, s.Proof(), c.IntegrationID, r.ID).Scan(&expected)
	if err != nil {
		return tenancy.Error(err)
	}
	return s.ProvisionTemplate(ctx, c, expected, MockTemplateSource{Document: ProviderDocument{ID: r.ID, PolicyVersion: "synthetic-v1", ExpiresAt: r.VerifiedUntil, Payload: payload}})
}
func (s Service) QueueTemplate(ctx context.Context, t catalog.Tenant, conv, id, key string, params []string) (string, error) {
	if !organizations.ValidID(conv) || !organizations.ValidID(id) || !organizations.ValidID(key) {
		return "", channel.ErrRejected
	}
	tx, e := tenancy.Begin(ctx, s.Pool, t, true, true)
	if e != nil {
		return "", e
	}
	defer tx.Rollback(ctx)
	raw, _ := json.Marshal(params)
	var result string
	e = tx.QueryRow(ctx, `SELECT platform.wa_enqueue_template($1,$2,$3,$4,$5)`, t.OrganizationID, conv, id, raw, key).Scan(&result)
	if e != nil {
		return "", tenancy.Error(e)
	}
	if e = tx.Commit(ctx); e != nil {
		return "", e
	}
	return result, nil
}
