package whatsapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/channel"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
)

// Separate contracts: pricing need not come from a template or Graph API.
// No network implementation or official verification capability is installed.
type ProviderDocument struct {
	Kind, ID, SourceType, SourceIdentity, VerificationMethod, PolicyVersion string
	ExpiresAt                                                               time.Time
	Payload                                                                 json.RawMessage
	Digest                                                                  [32]byte
}
type PricingPolicySource interface {
	Pricing(context.Context, Config) (ProviderDocument, error)
	OfflineOnly() bool
}
type TemplateStatusSource interface {
	TemplateStatus(context.Context, Config) (ProviderDocument, error)
	OfflineOnly() bool
}
type MockPricingSource struct{ Document ProviderDocument }
type MockTemplateSource struct{ Document ProviderDocument }

func (MockPricingSource) OfflineOnly() bool  { return true }
func (MockTemplateSource) OfflineOnly() bool { return true }
func mockDocument(ctx context.Context, d ProviderDocument, kind string) (ProviderDocument, error) {
	if ctx.Err() != nil {
		return d, ctx.Err()
	}
	d.Kind = kind
	d.SourceType = "SYNTHETIC_TEST"
	d.SourceIdentity = "synthetic:offline-fixture"
	d.VerificationMethod = "OFFLINE_DIGEST"
	d.Digest = sha256.Sum256(d.Payload)
	return d, nil
}
func (s MockPricingSource) Pricing(ctx context.Context, c Config) (ProviderDocument, error) {
	return mockDocument(ctx, s.Document, "pricing")
}
func (s MockTemplateSource) TemplateStatus(ctx context.Context, c Config) (ProviderDocument, error) {
	return mockDocument(ctx, s.Document, "template")
}
func (s Service) ProvisionPricing(ctx context.Context, c Config, expected int64, source PricingPolicySource) error {
	if source == nil || !source.OfflineOnly() {
		return channel.ErrRejected
	}
	d, e := source.Pricing(ctx, c)
	if e != nil {
		return channel.ErrRejected
	}
	if d.Kind != "pricing" {
		return channel.ErrRejected
	}
	return s.provision(ctx, c, expected, d)
}
func (s Service) ProvisionTemplate(ctx context.Context, c Config, expected int64, source TemplateStatusSource) error {
	if source == nil || !source.OfflineOnly() {
		return channel.ErrRejected
	}
	d, e := source.TemplateStatus(ctx, c)
	if e != nil {
		return channel.ErrRejected
	}
	if d.Kind != "template" {
		return channel.ErrRejected
	}
	return s.provision(ctx, c, expected, d)
}
func (s Service) provision(ctx context.Context, c Config, expected int64, d ProviderDocument) error {
	if len(s.Key) != 32 || !organizations.ValidID(d.ID) || !organizations.ValidID(c.OrganizationID) || !organizations.ValidID(c.IntegrationID) || expected < 0 || len(d.Payload) > 16384 || !json.Valid(d.Payload) {
		return channel.ErrRejected
	}
	var raw []byte
	e := s.Pool.QueryRow(ctx, `SELECT platform.wa_provision($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, s.Proof(), c.OrganizationID, c.IntegrationID, d.Kind, d.ID, expected, d.SourceType, d.SourceIdentity, d.VerificationMethod, d.PolicyVersion, d.ExpiresAt, string(d.Payload), d.Digest[:]).Scan(&raw)
	if e != nil {
		return tenancy.Error(e)
	}
	return resultError(raw)
}
func resultError(raw []byte) error {
	var r struct {
		Code string `json:"code"`
	}
	if json.Unmarshal(raw, &r) != nil {
		return auth.Unavailable()
	}
	switch r.Code {
	case "OK":
		return nil
	case "FORBIDDEN":
		return auth.Failure(403, r.Code, "Нужны назначенные полномочия оператора платформы")
	case "NOT_FOUND":
		return auth.Failure(404, r.Code, "Ресурс не найден")
	case "VERSION_CONFLICT":
		return auth.Failure(409, r.Code, "Версия или доказательства конфликтуют")
	}
	return auth.Failure(422, "INVALID_EVIDENCE", "Доказательство отсутствует, устарело или не прошло проверку")
}

// Operators use the same authenticated session and transaction-local tenant context,
// but authorization is an explicit expiring assignment, not store membership.
func (s Service) operatorTx(ctx context.Context, t catalog.Tenant) (pgx.Tx, error) {
	if !organizations.ValidID(t.OrganizationID) {
		return nil, auth.Failure(404, "NOT_FOUND", "Организация не найдена")
	}
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return nil, tenancy.Error(e)
	}
	if _, e = tx.Exec(ctx, `SELECT set_config('app.session_hash',$1,true),set_config('app.organization_id',$2,true)`, hex.EncodeToString(t.Session.Hash), t.OrganizationID); e != nil {
		_ = tx.Rollback(ctx)
		return nil, tenancy.Error(e)
	}
	return tx, nil
}
func (s Service) OperatorState(ctx context.Context, t catalog.Tenant) (json.RawMessage, error) {
	tx, e := s.operatorTx(ctx, t)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	var raw []byte
	e = tx.QueryRow(ctx, `SELECT platform.wa_operator_state($1)`, t.OrganizationID).Scan(&raw)
	if e != nil {
		return nil, tenancy.Error(e)
	}
	if e = resultError(raw); e != nil {
		return nil, e
	}
	return raw, nil
}

type ReconciliationInput struct {
	EventID           string    `json:"event_id"`
	Version           int64     `json:"expected_version"`
	Decision          string    `json:"decision"`
	Charge            int64     `json:"charge_microrub"`
	EvidenceType      string    `json:"evidence_type"`
	EvidenceReference string    `json:"evidence_reference"`
	EvidenceAt        time.Time `json:"evidence_at"`
	ProviderID        *string   `json:"provider_message_id"`
	Confirm           bool      `json:"confirm_reconciliation"`
}

func (s Service) ReconcileEvidence(ctx context.Context, t catalog.Tenant, id string, in ReconciliationInput) (json.RawMessage, error) {
	if !organizations.ValidID(id) || !organizations.ValidID(in.EventID) || !in.Confirm {
		return nil, auth.Failure(422, "VALIDATION_ERROR", "Нужны id операции и явное подтверждение")
	}
	tx, e := s.operatorTx(ctx, t)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	var raw []byte
	e = tx.QueryRow(ctx, `SELECT platform.wa_reconcile_evidence($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, t.OrganizationID, id, in.EventID, in.Version, in.Decision, in.Charge, in.EvidenceType, in.EvidenceReference, in.EvidenceAt, in.ProviderID).Scan(&raw)
	if e != nil {
		return nil, tenancy.Error(e)
	}
	// Commit rejected decisions as well: authorization/conflict evidence survives API errors.
	if e = tx.Commit(ctx); e != nil {
		return nil, tenancy.Error(e)
	}
	if e = resultError(raw); e != nil {
		return nil, e
	}
	return raw, nil
}
func (s Service) RevokeProvider(ctx context.Context, t catalog.Tenant, id string, version int64) error {
	if !organizations.ValidID(id) {
		return channel.ErrRejected
	}
	tx, e := s.operatorTx(ctx, t)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var raw []byte
	e = tx.QueryRow(ctx, `SELECT platform.wa_revoke_provider($1,$2,$3)`, t.OrganizationID, id, version).Scan(&raw)
	if e != nil {
		return tenancy.Error(e)
	}
	if e = tx.Commit(ctx); e != nil {
		return tenancy.Error(e)
	}
	return resultError(raw)
}
func (s Service) ProviderState(ctx context.Context, t catalog.Tenant) (json.RawMessage, error) {
	tx, e := s.begin(ctx, t)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	var raw []byte
	e = tx.QueryRow(ctx, `SELECT jsonb_build_object('live_enabled',false,'pricing_ready',false,'reason','NO_OFFICIAL_PROVIDER_INSTALLED','records',coalesce((SELECT jsonb_agg(jsonb_build_object('id',id,'source_type',source_type,'verified_at',verified_at,'expires_at',expires_at,'revoked_at',revoked_at,'approval_status',approval_status,'version',version,'status',platform.wa_resource_status(organization_id,CASE WHEN pricing_id IS NULL THEN 'template' ELSE 'pricing' END,coalesce(pricing_id,template_id)))) FROM platform.wa_provider_records WHERE organization_id=$1),'[]'::jsonb))`, t.OrganizationID).Scan(&raw)

	if e != nil {
		return nil, tenancy.Error(e)
	}
	return raw, nil
}
