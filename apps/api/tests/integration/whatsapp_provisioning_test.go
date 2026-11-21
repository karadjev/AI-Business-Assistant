//go:build integration

package integration

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/channel"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/db"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/whatsapp"
)

// Privileged fixture attestation is deliberately outside API/worker. Used only to
// adapt older 9B synthetic fixtures; 9C tests use the actual provisioning interface.
func attestSynthetic(t *testing.T, admin *pgx.Conn, ctx context.Context, kind, id string) {
	t.Helper()
	table, column := "wa_pricing", "pricing_id"
	if kind == "template" {
		table, column = "wa_templates", "template_id"
	}
	q := fmt.Sprintf(`INSERT INTO platform.wa_provider_records(id,organization_id,integration_id,%s,source_type,source_identity,verification_method,verified_at,expires_at,policy_version,integrity_digest,payload_digest,approval_status) SELECT gen_random_uuid(),p.organization_id,p.integration_id,p.id,'SYNTHETIC_TEST','synthetic:legacy-fixture','OFFLINE_DIGEST',p.verified_at,p.verified_until,p.policy_version,platform.wa_resource_digest($1,p.id),sha256(convert_to('synthetic-legacy','UTF8')),'VERIFIED' FROM platform.%s p JOIN platform.organizations o ON o.id=p.organization_id WHERE p.id=$2 AND p.synthetic AND o.is_test ON CONFLICT(organization_id,%s) DO UPDATE SET source_type='SYNTHETIC_TEST',verification_method='OFFLINE_DIGEST',approval_status='VERIFIED',integrity_digest=excluded.integrity_digest,verified_at=excluded.verified_at,expires_at=excluded.expires_at,revoked_at=NULL`, column, table, column)
	if _, e := admin.Exec(ctx, q, kind, id); e != nil {
		t.Fatal(e)
	}
}

type documentPricing struct{ D whatsapp.ProviderDocument }

func (documentPricing) OfflineOnly() bool { return true }
func (s documentPricing) Pricing(context.Context, whatsapp.Config) (whatsapp.ProviderDocument, error) {
	return s.D, nil
}

func testTrusted9C(t *testing.T, d *db.Database, admin *pgx.Conn, ctx context.Context, wa whatsapp.Service, c whatsapp.Config, owner, staff catalog.Tenant, pricing string, job func() (channel.Message, string), reserve func(channel.Message, string, bool, *whatsapp.Template) (string, error), finish func(string, channel.Delivery), reset func()) {
	execSQL := func(q string, args ...any) {
		t.Helper()
		if _, e := admin.Exec(ctx, q, args...); e != nil {
			t.Fatal(e)
		}
	}
	var operator catalog.Tenant
	token, uid := auth.Token(), auth.UUID()
	execSQL(`INSERT INTO platform.users(id,email,password_hash,display_name,email_verified_at) VALUES($1,$2,'synthetic','Platform operator',now())`, uid, uid+"@operator.example.test")
	execSQL(`INSERT INTO platform.sessions(id,user_id,token_hash,csrf_hash,expires_at,idle_expires_at) VALUES($1,$2,$3,$4,now()+interval '1 hour',now()+interval '1 hour')`, auth.UUID(), uid, auth.Hash(token), auth.Hash(auth.Token()))
	identity := auth.New(d.Pool, config.Auth{SessionTTL: time.Hour, IdleTTL: time.Hour})
	session, e := identity.Authenticate(ctx, token)
	if e != nil {
		t.Fatal(e)
	}
	operator = catalog.Tenant{Session: session, OrganizationID: c.OrganizationID}
	execSQL(`INSERT INTO platform.wa_operator_grants(organization_id,user_id,expires_at,assigned_by) VALUES($1,$2,now()+interval '1 hour','control:synthetic-test')`, c.OrganizationID, uid)
	// A second independently authenticated platform operator exercises concurrent decisions.
	token2, uid2 := auth.Token(), auth.UUID()
	execSQL(`INSERT INTO platform.users(id,email,password_hash,display_name,email_verified_at) VALUES($1,$2,'synthetic','Second platform operator',now())`, uid2, uid2+"@operator.example.test")
	execSQL(`INSERT INTO platform.sessions(id,user_id,token_hash,csrf_hash,expires_at,idle_expires_at) VALUES($1,$2,$3,$4,now()+interval '1 hour',now()+interval '1 hour')`, auth.UUID(), uid2, auth.Hash(token2), auth.Hash(auth.Token()))
	session2, e := identity.Authenticate(ctx, token2)
	if e != nil {
		t.Fatal(e)
	}
	operator2 := catalog.Tenant{Session: session2, OrganizationID: c.OrganizationID}
	execSQL(`INSERT INTO platform.wa_operator_grants(organization_id,user_id,expires_at,assigned_by) VALUES($1,$2,now()+interval '1 hour','control:synthetic-test')`, c.OrganizationID, uid2)
	doc := func() whatsapp.ProviderDocument {
		raw, _ := json.Marshal(map[string]any{"region": "ZZ", "category": "SERVICE", "message_type": "text", "billing_basis": "verified_maximum", "currency": "RUB", "max_charge": 10, "effective_at": time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)})
		return whatsapp.ProviderDocument{ID: pricing, PolicyVersion: "SYNTHETIC-9C", ExpiresAt: time.Now().Add(time.Hour).UTC().Truncate(time.Microsecond), Payload: raw}
	}
	version := func(id string) int64 {
		var v int64
		if e := admin.QueryRow(ctx, `SELECT version FROM platform.wa_provider_records WHERE pricing_id=$1 OR template_id=$1`, id).Scan(&v); e != nil {
			t.Fatal(e)
		}
		return v
	}
	valid := func(id string) bool {
		var ok bool
		if e := admin.QueryRow(ctx, `SELECT platform.wa_provenance_valid('pricing',$1,true)`, id).Scan(&ok); e != nil {
			t.Fatal(e)
		}
		return ok
	}
	t.Run("9C_pricing_synthetic_verified_duplicate_and_tamper", func(t *testing.T) {
		reset()
		x := doc()
		v := version(pricing)
		if e := wa.ProvisionPricing(ctx, c, v, whatsapp.MockPricingSource{Document: x}); e != nil {
			t.Fatal(e)
		}
		if !valid(pricing) {
			t.Fatal("verified synthetic not usable offline")
		}
		v = version(pricing)
		if e := wa.ProvisionPricing(ctx, c, v-1, whatsapp.MockPricingSource{Document: x}); e != nil || version(pricing) != v {
			t.Fatal("duplicate import", e)
		}
		execSQL(`UPDATE platform.wa_pricing SET max_charge=11 WHERE id=$1`, pricing)
		if valid(pricing) {
			t.Fatal("ordinary write manufactured trust")
		}
		m, cap := job()
		if _, e := reserve(m, cap, true, nil); e == nil {
			t.Fatal("tampered price admitted")
		}
	})
	for _, name := range []string{"MANUAL_UNVERIFIED", "VERIFIED_OPERATOR", "OFFICIAL_PROVIDER", "invalid_digest", "expired", "wrong_tenant", "wrong_integration"} {
		t.Run("9C_provider_"+name, func(t *testing.T) {
			reset()
			x := doc()
			x.Kind = "pricing"
			x.SourceType = "SYNTHETIC_TEST"
			x.SourceIdentity = "synthetic:fixture"
			x.VerificationMethod = "OFFLINE_DIGEST"
			x.Digest = sha256.Sum256(x.Payload)
			cfg := c
			switch name {
			case "MANUAL_UNVERIFIED", "VERIFIED_OPERATOR", "OFFICIAL_PROVIDER":
				x.SourceType = name
				x.VerificationMethod = "OPERATOR_REVIEW"
			case "invalid_digest":
				x.Digest = [32]byte{}
			case "expired":
				x.ExpiresAt = time.Now().Add(-time.Hour)
			case "wrong_tenant":
				cfg.OrganizationID = auth.UUID()
			case "wrong_integration":
				cfg.IntegrationID = auth.UUID()
			}
			e := wa.ProvisionPricing(ctx, cfg, version(pricing), documentPricing{D: x})
			if name == "MANUAL_UNVERIFIED" || name == "VERIFIED_OPERATOR" {
				if e != nil {
					t.Fatal(e)
				}
				if valid(pricing) {
					t.Fatal("operator label treated as provider verification")
				}
			} else if e == nil {
				t.Fatal("invalid origin accepted")
			}
		})
	}
	t.Run("9C_revocation_expiration_and_conflicting_versions", func(t *testing.T) {
		reset()
		x := doc()
		if e := wa.ProvisionPricing(ctx, c, version(pricing), whatsapp.MockPricingSource{Document: x}); e != nil {
			t.Fatal(e)
		}
		var rid string
		_ = admin.QueryRow(ctx, `SELECT id FROM platform.wa_provider_records WHERE pricing_id=$1`, pricing).Scan(&rid)
		if e := wa.RevokeProvider(ctx, owner, rid, version(pricing)); e == nil {
			t.Fatal("owner revoked provider record")
		}
		if e := wa.RevokeProvider(ctx, operator, rid, version(pricing)); e != nil {
			t.Fatal(e)
		}
		if valid(pricing) {
			t.Fatal("revoked admitted")
		}
		if e := wa.ProvisionPricing(ctx, c, version(pricing), whatsapp.MockPricingSource{Document: x}); e != nil {
			t.Fatal(e)
		}
		execSQL(`UPDATE platform.wa_provider_records SET verified_at=now()-interval '2 days',expires_at=now()-interval '1 day' WHERE pricing_id=$1`, pricing)
		if valid(pricing) {
			t.Fatal("expired admitted")
		}
		reset()
		x = doc()
		if e := wa.ProvisionPricing(ctx, c, version(pricing), whatsapp.MockPricingSource{Document: x}); e != nil {
			t.Fatal(e)
		}
		x.ID = auth.UUID()
		x.PolicyVersion = "SYNTHETIC-CONFLICT"
		if e := wa.ProvisionPricing(ctx, c, 0, whatsapp.MockPricingSource{Document: x}); e != nil {
			t.Fatal(e)
		}
		m, cap := job()
		if _, e := reserve(m, cap, true, nil); e == nil {
			t.Fatal("ambiguous pricing admitted")
		}
		execSQL(`UPDATE platform.wa_provider_records SET revoked_at=now() WHERE pricing_id=$1`, x.ID)
	})
	t.Run("9C_pricing_currency_region_category", func(t *testing.T) {
		for _, field := range []string{"currency", "region", "category"} {
			reset()
			x := doc()
			var data map[string]any
			_ = json.Unmarshal(x.Payload, &data)
			data[field] = "UNKNOWN"
			x.Payload, _ = json.Marshal(data)
			if e := wa.ProvisionPricing(ctx, c, version(pricing), whatsapp.MockPricingSource{Document: x}); e == nil {
				m, cap := job()
				if _, e = reserve(m, cap, true, nil); e == nil {
					t.Fatal("invalid pricing admitted", field)
				}
			}
		}
	})
	t.Run("9C_template_origin_revocation_expiry_concurrency", func(t *testing.T) {
		id := auth.UUID()
		raw, _ := json.Marshal(map[string]any{"waba_id": c.BusinessAccountID, "name": "synthetic_9c", "language": "ru", "category": "UTILITY", "status": "APPROVED", "components": []any{map[string]any{"type": "BODY", "text": "Synthetic {{1}}"}}, "parameter_schema": []string{"text"}})
		x := whatsapp.ProviderDocument{ID: id, PolicyVersion: "SYNTHETIC-TEMPLATE-9C", ExpiresAt: time.Now().Add(time.Hour).UTC().Truncate(time.Microsecond), Payload: raw}
		if e := wa.ProvisionTemplate(ctx, c, 0, whatsapp.MockTemplateSource{Document: x}); e != nil {
			t.Fatal(e)
		}
		check := func() bool {
			var ok bool
			_ = admin.QueryRow(ctx, `SELECT platform.wa_template_check($1,$2,$3,'synthetic_9c','ru','["x"]',true)`, c.OrganizationID, c.IntegrationID, id).Scan(&ok)
			return ok
		}
		if !check() {
			t.Fatal("verified synthetic template blocked")
		}
		var recID string
		_ = admin.QueryRow(ctx, `SELECT id FROM platform.wa_provider_records WHERE template_id=$1`, id).Scan(&recID)
		if e := wa.RevokeProvider(ctx, operator, recID, version(id)); e != nil || check() {
			t.Fatal("template revoke", e)
		}
		x.Payload = json.RawMessage(strings.Replace(string(raw), c.BusinessAccountID, "999999999", 1))
		if e := wa.ProvisionTemplate(ctx, c, version(id), whatsapp.MockTemplateSource{Document: x}); e == nil {
			t.Fatal("wrong WABA accepted")
		}
		x.Payload = raw
		if e := wa.ProvisionTemplate(ctx, c, version(id), whatsapp.MockTemplateSource{Document: x}); e != nil {
			t.Fatal(e)
		}
		v := version(id)
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		for _, state := range []string{"PAUSED", "REJECTED"} {
			wg.Add(1)
			go func(state string) {
				defer wg.Done()
				z := x
				z.Payload = json.RawMessage(strings.Replace(string(raw), "APPROVED", state, 1))
				errs <- wa.ProvisionTemplate(ctx, c, v, whatsapp.MockTemplateSource{Document: z})
			}(state)
		}
		wg.Wait()
		close(errs)
		success := 0
		for e := range errs {
			if e == nil {
				success++
			}
		}
		if success != 1 || check() {
			t.Fatal("concurrent template updates", success)
		}
		execSQL(`UPDATE platform.wa_provider_records SET verified_at=now()-interval '2 days',expires_at=now()-interval '1 day' WHERE template_id=$1`, id)
		if check() {
			t.Fatal("expired template")
		}
		if e := wa.RecoverFinancial(ctx); e != nil {
			t.Fatal(e)
		}
	})
	unknown := func() (channel.Message, string, int64) {
		reset()
		m, cap := job()
		id, e := reserve(m, cap, true, nil)
		if e != nil {
			t.Fatal(e)
		}
		finish(id, channel.Unknown)
		execSQL(`UPDATE platform.channel_outbox SET status='delivery_unknown',capability=NULL WHERE id=$1`, m.CorrelationID)
		var v int64
		_ = admin.QueryRow(ctx, `SELECT version FROM platform.wa_reservations WHERE id=$1`, id).Scan(&v)
		return m, id, v
	}
	evidence := func(v int64, decision string) whatsapp.ReconciliationInput {
		return whatsapp.ReconciliationInput{EventID: auth.UUID(), Version: v, Decision: decision, Charge: 0, EvidenceType: "SYNTHETIC_TEST", EvidenceReference: "synthetic:CASE_9C_0001", EvidenceAt: time.Now().UTC().Truncate(time.Microsecond), Confirm: true}
	}
	for _, mode := range []string{"INSUFFICIENT", "CONFIRMED_SENT", "CONFIRMED_NOT_SENT", "missing_evidence", "incorrect_release", "conflict_provider", "unauthorized_staff", "owner_escalation", "wrong_tenant"} {
		t.Run("9C_reconcile_"+mode, func(t *testing.T) {
			m, id, v := unknown()
			in := evidence(v, "CONFIRMED_NOT_SENT")
			actor := operator
			switch mode {
			case "INSUFFICIENT", "CONFIRMED_SENT", "CONFIRMED_NOT_SENT":
				in.Decision = mode
				if mode == "CONFIRMED_NOT_SENT" {
					execSQL(`UPDATE platform.channel_outbox SET provider_id='' WHERE id=$1`, m.CorrelationID)
				}
				if mode == "CONFIRMED_SENT" {
					in.Charge = 10
				}
			case "missing_evidence":
				in.EvidenceReference = ""
			case "incorrect_release":
				in.Decision = "CONFIRMED_SENT"
				in.Charge = 0
			case "conflict_provider":
				execSQL(`UPDATE platform.wa_reservations SET provider_id='wamid.synthetic' WHERE id=$1`, id)
			case "unauthorized_staff":
				actor = staff
			case "owner_escalation":
				actor = owner
			case "wrong_tenant":
				actor.OrganizationID = auth.UUID()
			}
			raw, e := wa.ReconcileEvidence(ctx, actor, id, in)
			if mode == "incorrect_release" || mode == "missing_evidence" || mode == "unauthorized_staff" || mode == "owner_escalation" || mode == "wrong_tenant" {
				if e == nil {
					t.Fatal("unauthorized/invalid evidence accepted")
				}
			} else if e != nil {
				t.Fatal(e)
			}
			var state string
			var charge int64
			_ = admin.QueryRow(ctx, `SELECT status,charged FROM platform.wa_reservations WHERE id=$1`, id).Scan(&state, &charge)
			if mode == "CONFIRMED_SENT" || mode == "CONFIRMED_NOT_SENT" {
				if state == "unknown" {
					t.Fatal("no settlement")
				}
				if _, e := wa.ReconcileEvidence(ctx, actor, id, in); e != nil {
					t.Fatal("replay", e)
				}
				in.EventID = auth.UUID()
				if _, e := wa.ReconcileEvidence(ctx, actor, id, in); e == nil {
					t.Fatal("duplicate settlement")
				}
			} else if state != "unknown" || charge != 0 {
				t.Fatal("incorrect refund", state, string(raw))
			}
			var qstate string
			_ = admin.QueryRow(ctx, `SELECT status FROM platform.channel_outbox WHERE id=$1`, m.CorrelationID).Scan(&qstate)
			if qstate != "delivery_unknown" {
				t.Fatal("financial decision claimed delivery")
			}
		})
	}
	t.Run("9C_concurrent_operators_rollback_delayed_provider_event", func(t *testing.T) {
		m, id, v := unknown()
		in := evidence(v, "CONFIRMED_NOT_SENT")
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		for k := 0; k < 2; k++ {
			actor := operator
			if k == 1 {
				actor = operator2
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				z := in
				z.EventID = auth.UUID()
				_, e := wa.ReconcileEvidence(ctx, actor, id, z)
				errs <- e
			}()
		}
		wg.Wait()
		close(errs)
		n := 0
		for e := range errs {
			if e == nil {
				n++
			}
		}
		if n != 1 {
			t.Fatal("concurrent settlement", n)
		}
		// Late provider receipt never changes the financial ledger and never queues resend.
		var spent int64
		_ = admin.QueryRow(ctx, `SELECT spent FROM platform.wa_budgets WHERE organization_id=$1`, c.OrganizationID).Scan(&spent)
		execSQL(`UPDATE platform.channel_outbox SET provider_id='wamid.delayed',status='read' WHERE id=$1`, m.CorrelationID)
		if e := wa.RecoverFinancial(ctx); e != nil {
			t.Fatal(e)
		}
		var after int64
		_ = admin.QueryRow(ctx, `SELECT spent FROM platform.wa_budgets WHERE organization_id=$1`, c.OrganizationID).Scan(&after)
		if spent != after {
			t.Fatal("late event charged again")
		}
		var conflicts int
		_ = admin.QueryRow(ctx, `SELECT count(*) FROM platform.wa_financial_events WHERE reservation_id=$1 AND kind='LATE_PROVIDER_EVIDENCE_CONFLICT'`, id).Scan(&conflicts)
		if conflicts != 1 {
			t.Fatal("late contradiction not audited")
		}
		_, id, v = unknown()
		tx, e := d.Pool.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback(ctx)
		_, e = tx.Exec(ctx, `SELECT set_config('app.session_hash',encode($1::bytea,'hex'),true),set_config('app.organization_id',$2,true)`, operator.Session.Hash, c.OrganizationID)
		if e != nil {
			t.Fatal(e)
		}
		var raw []byte
		e = tx.QueryRow(ctx, `SELECT platform.wa_reconcile_evidence($1,$2,$3,$4,'CONFIRMED_NOT_SENT',0,'SYNTHETIC_TEST','synthetic:ROLLBACK_001',now(),NULL)`, c.OrganizationID, id, auth.UUID(), v).Scan(&raw)
		if e != nil {
			t.Fatal(e)
		}
		_ = tx.Rollback(ctx)
		var state string
		_ = admin.QueryRow(ctx, `SELECT status FROM platform.wa_reservations WHERE id=$1`, id).Scan(&state)
		if state != "unknown" {
			t.Fatal("rollback lost reserve")
		}
	})
	t.Run("9C_authorized_operator_wrong_tenant_and_conflicting_replay", func(t *testing.T) {
		_, id, v := unknown()
		in := evidence(v, "INSUFFICIENT")
		if _, e := wa.ReconcileEvidence(ctx, operator, id, in); e != nil {
			t.Fatal(e)
		}
		in.Decision = "CONFIRMED_SENT"
		in.Charge = 10
		if _, e := wa.ReconcileEvidence(ctx, operator, id, in); e == nil {
			t.Fatal("conflicting idempotency payload accepted")
		}
		var other string
		if e := admin.QueryRow(ctx, `SELECT id FROM platform.organizations WHERE id<>$1 LIMIT 1`, c.OrganizationID).Scan(&other); e != nil {
			t.Fatal(e)
		}
		execSQL(`INSERT INTO platform.wa_operator_grants(organization_id,user_id,expires_at,assigned_by) VALUES($1,$2,now()+interval '1 hour','control:synthetic-test')`, other, uid2)
		alien := operator2
		alien.OrganizationID = other
		in.EventID = auth.UUID()
		if _, e := wa.ReconcileEvidence(ctx, alien, id, in); e == nil {
			t.Fatal("operator crossed tenant assignment")
		}
	})
	t.Run("9C_operator_expires_while_waiting_for_worker_lock", func(t *testing.T) {
		_, id, v := unknown()
		in := evidence(v, "CONFIRMED_NOT_SENT")
		lock, e := d.Pool.Acquire(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer lock.Release()
		if _, e = lock.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended($1,8210))`, c.IntegrationID); e != nil {
			t.Fatal(e)
		}
		execSQL(`UPDATE platform.wa_operator_grants SET expires_at=clock_timestamp()+interval '100 milliseconds' WHERE organization_id=$1 AND user_id=$2`, c.OrganizationID, uid)
		done := make(chan error, 1)
		go func() { _, e := wa.ReconcileEvidence(ctx, operator, id, in); done <- e }()
		time.Sleep(150 * time.Millisecond)
		if _, e = lock.Exec(ctx, `SELECT pg_advisory_unlock(hashtextextended($1,8210))`, c.IntegrationID); e != nil {
			t.Fatal(e)
		}
		if e = <-done; e == nil {
			t.Fatal("expired waiting operator settled money")
		}
		execSQL(`UPDATE platform.wa_operator_grants SET expires_at=now()+interval '1 hour' WHERE organization_id=$1 AND user_id=$2`, c.OrganizationID, uid)
	})
	t.Run("9C_operator_grant_expiration_and_audit_integrity", func(t *testing.T) {
		if _, e := wa.OperatorState(ctx, operator); e != nil {
			t.Fatal(e)
		}
		execSQL(`UPDATE platform.wa_operator_grants SET expires_at=now()-interval '1 second' WHERE organization_id=$1 AND user_id=$2`, c.OrganizationID, uid)
		if _, e := wa.OperatorState(ctx, operator); e == nil {
			t.Fatal("expired operator allowed")
		}
		var n int
		_ = admin.QueryRow(ctx, `SELECT count(*) FROM platform.wa_financial_events WHERE organization_id=$1 AND kind='UNAUTHORIZED_ATTEMPT'`, c.OrganizationID).Scan(&n)
		if n < 2 {
			t.Fatal("denial audit rolled back")
		}
	})
	t.Log("PASS 9C: provenance/tamper/revocation/expiration/conflict/idempotency/operator isolation/evidence/concurrent settlement/rollback/audit; no official provider installed")
}
