//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/channel"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/db"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/migrate"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/whatsapp"
)

func testWhatsAppFinancial(t *testing.T, d *db.Database, admin *pgx.Conn, ctx context.Context) {
	t.Helper()
	wa := whatsapp.Service{Pool: d.Pool, Key: bytes.Repeat([]byte{95}, 32), Transport: whatsapp.Mock{}}
	execSQL := func(q string, args ...any) {
		t.Helper()
		if _, e := admin.Exec(ctx, q, args...); e != nil {
			t.Fatal(e)
		}
	}
	uid := auth.UUID()
	token := auth.Token()
	csrf := auth.Token()
	execSQL(`INSERT INTO platform.users(id,email,password_hash,display_name,email_verified_at) VALUES($1,$2,'synthetic','finance',now())`, uid, uid+"@financial.example.test")
	execSQL(`INSERT INTO platform.sessions(id,user_id,token_hash,csrf_hash,expires_at,idle_expires_at) VALUES($1,$2,$3,$4,now()+interval '1 hour',now()+interval '1 hour')`, auth.UUID(), uid, auth.Hash(token), auth.Hash(csrf))
	identity := auth.New(d.Pool, config.Auth{Origin: "http://127.0.0.1:3000", SessionTTL: time.Hour, IdleTTL: time.Hour})
	session, e := identity.Authenticate(ctx, token)
	if e != nil {
		t.Fatal(e)
	}
	org, e := (organizations.Service{Pool: d.Pool}).CreateTest(ctx, session, "Synthetic WhatsApp Financial")
	if e != nil {
		t.Fatal(e)
	}
	tenant := catalog.Tenant{OrganizationID: org.ID, Session: session}
	memberID, memberToken := auth.UUID(), auth.Token()
	execSQL(`INSERT INTO platform.users(id,email,password_hash,display_name,email_verified_at) VALUES($1,$2,'synthetic','finance staff',now())`, memberID, memberID+"@financial.example.test")
	execSQL(`INSERT INTO platform.sessions(id,user_id,token_hash,csrf_hash,expires_at,idle_expires_at) VALUES($1,$2,$3,$4,now()+interval '1 hour',now()+interval '1 hour')`, auth.UUID(), memberID, auth.Hash(memberToken), auth.Hash(auth.Token()))
	execSQL(`INSERT INTO platform.organization_members(organization_id,user_id,role,can_manage_orders) VALUES($1,$2,'member',true)`, org.ID, memberID)
	memberSession, err := identity.Authenticate(ctx, memberToken)
	if err != nil {
		t.Fatal(err)
	}
	memberTenant := catalog.Tenant{OrganizationID: org.ID, Session: memberSession}

	credentials := whatsapp.Credentials{AccessToken: "mock:synthetic-financial", AppSecret: "mock:synthetic-app-secret", VerifyToken: "mock:synthetic-verify-token", AllowedUsers: []string{"700"}}
	if _, e = wa.Connect(ctx, tenant, whatsapp.ConnectInput{AppID: "991", BusinessID: "992", PhoneID: "993", GraphVersion: "v99.0", Credentials: credentials}); e != nil {
		t.Fatal(e)
	}
	var integration string
	if e = admin.QueryRow(ctx, `SELECT id FROM platform.channel_integrations WHERE organization_id=$1`, org.ID).Scan(&integration); e != nil {
		t.Fatal(e)
	}
	if _, e = wa.Control(ctx, tenant, true, true); e != nil {
		t.Fatal(e)
	}
	conv, customer := auth.UUID(), auth.UUID()
	execSQL(`INSERT INTO platform.conversations(id,organization_id,customer_id,created_by,channel,control_mode,expires_at) VALUES($1,$2,$3,$4,'whatsapp','human',now()+interval '1 day')`, conv, org.ID, customer, uid)
	execSQL(`INSERT INTO platform.channel_principals(id,organization_id,integration_id,external_user_id,conversation_id,window_until,recipient_region) VALUES($1,$2,$3,'700',$4,now()+interval '1 hour','ZZ')`, customer, org.ID, integration, conv)
	migrationDSN := testDSN(t, "ABA_TEST_MIGRATION_DATABASE_URL", "aba_migrator")
	if err := migrate.Run(ctx, migrationDSN, "down"); err != nil {
		t.Fatalf("password-reset migration rollback: %v", err)
	}
	if err := migrate.Run(ctx, migrationDSN, "down"); err != nil {
		t.Fatalf("additive vehicle-service migration rollback: %v", err)
	}
	if err := migrate.Run(ctx, migrationDSN, "down"); err == nil {
		t.Fatal("rollback discarded verified recipient metadata")
	}
	if err := migrate.Run(ctx, migrationDSN, "up"); err != nil {
		t.Fatalf("restore latest schema after rollback refusal: %v", err)
	}
	c := whatsapp.Config{OrganizationID: org.ID, IntegrationID: integration, AppID: "991", BusinessAccountID: "992", PhoneNumberID: "993", GraphVersion: "v99.0", AccessToken: credentials.AccessToken, AppSecret: credentials.AppSecret, VerifyToken: credentials.VerifyToken, AllowedUsers: map[string]bool{"700": true}, Window: 24 * time.Hour}
	execSQL(`INSERT INTO platform.wa_budgets(organization_id) VALUES($1)`, org.ID)
	pricing := auth.UUID()
	execSQL(`INSERT INTO platform.wa_pricing(id,organization_id,integration_id,policy_version,region,category,message_type,billing_basis,currency,max_charge,source,verified_at,verified_until,effective_at,verified,synthetic) VALUES($1,$2,$3,'SYNTHETIC-v1','ZZ','SERVICE','text','verified_maximum','RUB',10,'SYNTHETIC_FIXTURE',now(),now()+interval '1 day',now(),true,true)`, pricing, org.ID, integration)
	reset := func() {
		execSQL(`UPDATE platform.wa_platform_budget SET limit_amount=100000,synthetic=true WHERE singleton`)
		execSQL(`UPDATE platform.wa_budgets SET limit_amount=100000,enabled=true,kill=false,synthetic=true WHERE organization_id=$1`, org.ID)
		execSQL(`UPDATE platform.channel_integrations SET active=true,sending_enabled=true WHERE id=$1`, integration)
		execSQL(`UPDATE platform.channel_principals SET window_until=now()+interval '1 hour',recipient_region='ZZ' WHERE id=$1`, customer)
		execSQL(`UPDATE platform.wa_pricing SET verified=true,currency='RUB',verified_until=now()+interval '1 day',verified_at=now(),max_charge=10,verified_free=false,region='ZZ',billing_basis='verified_maximum' WHERE id=$1`, pricing)
		attestSynthetic(t, admin, ctx, "pricing", pricing)
	}
	job := func() (channel.Message, string) {
		id, cap := auth.UUID(), auth.UUID()
		execSQL(`INSERT INTO platform.channel_outbox(id,organization_id,integration_id,conversation_id,external_user_id,payload,role,status,attempts,capability,lease_until) VALUES($1,$2,$3,$4,'700','{"text":"synthetic outbound"}','staff','sending',1,$5,now()+interval '1 minute')`, id, org.ID, integration, conv, cap)
		return channel.Message{OrganizationID: org.ID, IntegrationID: integration, ConversationID: conv, ExternalUserID: "700", ExternalChatID: "700", Channel: channel.WhatsApp, Direction: "outbound", Kind: channel.Text, Text: "synthetic outbound", CorrelationID: id}, cap
	}
	reserve := func(m channel.Message, cap string, sim bool, template *whatsapp.Template) (string, error) {
		conn, e := d.Pool.Acquire(ctx)
		if e != nil {
			return "", e
		}
		defer conn.Release()
		return (&whatsapp.FinancialAdmission{Conn: conn, Proof: wa.Proof(), Capability: cap, Simulation: sim}).Reserve(ctx, c, m, template)
	}
	finish := func(id string, state channel.Delivery) {
		conn, e := d.Pool.Acquire(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer conn.Release()
		if e = (&whatsapp.FinancialAdmission{Conn: conn, Proof: wa.Proof(), Simulation: true}).Finish(ctx, id, channel.SendResult{State: state, NoChargeConfirmed: state == channel.Failed || state == channel.Cancelled}); e != nil {
			t.Fatal(e)
		}
	}
	for _, name := range []string{"budget_zero", "budget_insufficient", "pricing_unknown", "pricing_expired", "currency", "region", "disabled", "kill", "synthetic_in_live", "window", "unknown_billing", "free_unverified"} {
		t.Run("WA_financial_"+name, func(t *testing.T) {
			reset()
			m, cap := job()
			sim := true
			switch name {
			case "budget_zero":
				execSQL(`UPDATE platform.wa_budgets SET limit_amount=reserved+spent WHERE organization_id=$1`, org.ID)
			case "budget_insufficient":
				execSQL(`UPDATE platform.wa_budgets SET limit_amount=reserved+spent+9 WHERE organization_id=$1`, org.ID)
			case "pricing_unknown":
				execSQL(`UPDATE platform.wa_pricing SET max_charge=NULL WHERE id=$1`, pricing)
			case "pricing_expired":
				execSQL(`UPDATE platform.wa_pricing SET verified_at=now()-interval '2 days',verified_until=now()-interval '1 day' WHERE id=$1`, pricing)
			case "currency":
				execSQL(`UPDATE platform.wa_pricing SET currency='USD' WHERE id=$1`, pricing)
			case "region":
				execSQL(`UPDATE platform.channel_principals SET recipient_region=NULL WHERE id=$1`, customer)
			case "disabled":
				execSQL(`UPDATE platform.channel_integrations SET active=false WHERE id=$1`, integration)
			case "kill":
				execSQL(`UPDATE platform.wa_budgets SET kill=true WHERE organization_id=$1`, org.ID)
			case "synthetic_in_live":
				sim = false
			case "window":
				execSQL(`UPDATE platform.channel_principals SET window_until=now()-interval '1 day' WHERE id=$1`, customer)
			case "free_unverified":
				execSQL(`UPDATE platform.wa_pricing SET max_charge=0,verified_free=false WHERE id=$1`, pricing)
			case "unknown_billing":
				execSQL(`UPDATE platform.wa_pricing SET billing_basis='unknown' WHERE id=$1`, pricing)
			}
			if id, e := reserve(m, cap, sim, nil); e == nil || id != "" {
				t.Fatal("financial bypass", name)
			}
			var reason string
			if err := admin.QueryRow(ctx, `SELECT kind FROM platform.wa_financial_events WHERE organization_id=$1 ORDER BY id DESC LIMIT 1`, org.ID).Scan(&reason); err != nil {
				t.Fatal(err)
			}
			want := "PRICING_UNVERIFIED"
			switch name {
			case "budget_zero", "budget_insufficient":
				want = "BUDGET_EXHAUSTED"
			case "disabled", "window":
				want = "STALE_OR_DISABLED"
			case "kill", "synthetic_in_live":
				want = "LIVE_DISABLED"
			}
			if reason != want {
				t.Fatalf("wrong admission gate: got %s want %s", reason, want)
			}

			execSQL(`UPDATE platform.channel_outbox SET status='failed',capability=NULL WHERE id=$1`, m.CorrelationID)
		})
	}
	t.Run("WA_verified_free_zero_budget", func(t *testing.T) {
		reset()
		m, cap := job()
		execSQL(`UPDATE platform.wa_budgets SET limit_amount=0 WHERE organization_id=$1`, org.ID)
		execSQL(`UPDATE platform.wa_platform_budget SET limit_amount=0 WHERE singleton`)
		execSQL(`UPDATE platform.wa_pricing SET max_charge=0,verified_free=true WHERE id=$1`, pricing)
		attestSynthetic(t, admin, ctx, "pricing", pricing)
		conn, e := d.Pool.Acquire(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer conn.Release()
		a := &whatsapp.FinancialAdmission{Conn: conn, Proof: wa.Proof(), Capability: cap, Simulation: true}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"messages":[{"id":"wamid.synthetic.free"}]}`))
		}))
		defer server.Close()
		tr, e := whatsapp.NewOfflineCloudTransport(whatsapp.CloudOptions{Enabled: true, ExternalAuthorized: true, AllowVerifiedFree: true, SupportedVersions: []string{"v99.0"}, Admission: a}, server.URL)
		if e != nil {
			t.Fatal(e)
		}
		if res, e := tr.Send(ctx, c, m); e != nil || res.State != channel.Sent {
			t.Fatal("verified free rule rejected", e)
		}
		execSQL(`UPDATE platform.channel_outbox SET status='sent',capability=NULL WHERE id=$1`, m.CorrelationID)
	})
	reset()
	m, cap := job()
	id, e := reserve(m, cap, true, nil)
	if e != nil {
		t.Fatal("sufficient synthetic budget", e)
	}
	if _, e = reserve(m, cap, true, nil); e == nil {
		t.Fatal("duplicate reserve")
	}
	finish(id, channel.Sent)
	finish(id, channel.Sent)
	finish(id, channel.Failed)
	var charged, reserved int64
	if e = admin.QueryRow(ctx, `SELECT charged FROM platform.wa_reservations WHERE id=$1`, id).Scan(&charged); e != nil || charged != 10 {
		t.Fatal("double settlement", e, charged)
	}
	execSQL(`UPDATE platform.channel_outbox SET status='sent',capability=NULL WHERE id=$1`, m.CorrelationID)
	t.Run("WA_rejected_unknown_cost_is_held", func(t *testing.T) {
		reset()
		m, cap := job()
		id, e := reserve(m, cap, true, nil)
		if e != nil {
			t.Fatal(e)
		}
		conn, e := d.Pool.Acquire(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer conn.Release()
		if e = (&whatsapp.FinancialAdmission{Conn: conn, Proof: wa.Proof(), Simulation: true}).Finish(ctx, id, channel.SendResult{State: channel.Failed}); e != nil {
			t.Fatal(e)
		}
		var status string
		_ = admin.QueryRow(ctx, `SELECT status FROM platform.wa_reservations WHERE id=$1`, id).Scan(&status)
		if status != "unknown" {
			t.Fatal("unknown cost treated as free")
		}
		execSQL(`UPDATE platform.channel_outbox SET status='failed',capability=NULL WHERE id=$1`, m.CorrelationID)
	})
	t.Run("WA_credential_phone_binding", func(t *testing.T) {
		reset()
		m, cap := job()
		conn, e := d.Pool.Acquire(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer conn.Release()
		wrong := c
		wrong.PhoneNumberID = "999"
		if _, e := (&whatsapp.FinancialAdmission{Conn: conn, Proof: wa.Proof(), Capability: cap, Simulation: true}).Reserve(ctx, wrong, m, nil); e == nil {
			t.Fatal("foreign phone credentials were admitted")
		}
		execSQL(`UPDATE platform.channel_outbox SET status='failed',capability=NULL WHERE id=$1`, m.CorrelationID)
	})
	t.Run("WA_cancel_before_send_releases_once", func(t *testing.T) {
		reset()
		m, cap := job()
		id, e := reserve(m, cap, true, nil)
		if e != nil {
			t.Fatal(e)
		}
		finish(id, channel.Cancelled)
		finish(id, channel.Cancelled)
		var status string
		_ = admin.QueryRow(ctx, `SELECT status FROM platform.wa_reservations WHERE id=$1`, id).Scan(&status)
		if status != "released" {
			t.Fatal("cancel reserve was not released")
		}
		execSQL(`UPDATE platform.channel_outbox SET status='cancelled',capability=NULL WHERE id=$1`, m.CorrelationID)
	})
	t.Run("WA_concurrent_duplicate_workers", func(t *testing.T) {
		reset()
		m, cap := job()
		var wg sync.WaitGroup
		ch := make(chan string, 2)
		for k := 0; k < 2; k++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				id, e := reserve(m, cap, true, nil)
				if e == nil {
					ch <- id
				}
			}()
		}
		wg.Wait()
		close(ch)
		ids := []string{}
		for id := range ch {
			ids = append(ids, id)
		}
		if len(ids) != 1 {
			t.Fatal("duplicate worker admission", len(ids))
		}
		finish(ids[0], channel.Failed)
		execSQL(`UPDATE platform.channel_outbox SET status='failed',capability=NULL WHERE id=$1`, m.CorrelationID)
	})
	t.Run("WA_concurrent_budget", func(t *testing.T) {
		reset()
		execSQL(`UPDATE platform.wa_budgets SET limit_amount=reserved+spent+10 WHERE organization_id=$1`, org.ID)
		m1, c1 := job()
		m2, c2 := job()
		ch := make(chan string, 2)
		var wg sync.WaitGroup
		for _, pair := range []struct {
			m channel.Message
			c string
		}{{m1, c1}, {m2, c2}} {
			wg.Add(1)
			go func(p struct {
				m channel.Message
				c string
			}) {
				defer wg.Done()
				id, e := reserve(p.m, p.c, true, nil)
				if e == nil {
					ch <- id
				}
			}(pair)
		}
		wg.Wait()
		close(ch)
		ids := []string{}
		for id := range ch {
			ids = append(ids, id)
		}
		if len(ids) != 1 {
			t.Fatal("budget race", len(ids))
		}
		finish(ids[0], channel.Failed)
		execSQL(`UPDATE platform.channel_outbox SET status='failed',capability=NULL WHERE id IN ($1,$2)`, m1.CorrelationID, m2.CorrelationID)
	})
	t.Run("WA_unknown_manual_reconcile", func(t *testing.T) {
		reset()
		var heldBefore int64
		if err := admin.QueryRow(ctx, `SELECT reserved FROM platform.wa_budgets WHERE organization_id=$1`, org.ID).Scan(&heldBefore); err != nil {
			t.Fatal(err)
		}
		m, cap := job()
		id, e := reserve(m, cap, true, nil)
		if e != nil {
			t.Fatal(e)
		}
		finish(id, channel.Unknown)
		finish(id, channel.Failed)
		if e = admin.QueryRow(ctx, `SELECT reserved FROM platform.wa_budgets WHERE organization_id=$1`, org.ID).Scan(&reserved); e != nil || reserved != heldBefore+10 {
			t.Fatal("unknown refund", reserved, e)
		}
		if _, e = reserve(m, cap, true, nil); e == nil {
			t.Fatal("unknown resend")
		}
		var version int64
		_ = admin.QueryRow(ctx, `SELECT version FROM platform.wa_reservations WHERE id=$1`, id).Scan(&version)
		if e := wa.Reconcile(ctx, memberTenant, id, version, 0, "synthetic-staff-cannot-reconcile"); e == nil {
			t.Fatal("staff bypassed financial owner restriction")
		}
		for k := 0; k < 2; k++ {
			if e = wa.Reconcile(ctx, tenant, id, version, 0, "synthetic-confirmed-no-charge"); e == nil {
				t.Fatal("legacy owner financial bypass")
			}
		}
		execSQL(`UPDATE platform.channel_outbox SET status='delivery_unknown',capability=NULL WHERE id=$1`, m.CorrelationID)
	})
	for _, mode := range []string{"timeout", "reset"} {
		t.Run("WA_financial_HTTP_"+mode, func(t *testing.T) {
			reset()
			m, cap := job()
			conn, e := d.Pool.Acquire(ctx)
			if e != nil {
				t.Fatal(e)
			}
			defer conn.Release()
			a := &whatsapp.FinancialAdmission{Conn: conn, Proof: wa.Proof(), Capability: cap, Simulation: true}
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if mode == "timeout" {
					time.Sleep(250 * time.Millisecond)
					_, _ = w.Write([]byte(`{"messages":[{"id":"wamid.late"}]}`))
					return
				}
				h := w.(http.Hijacker)
				c, _, _ := h.Hijack()
				c.Close()
			}))
			defer server.Close()
			tr, e := whatsapp.NewOfflineCloudTransport(whatsapp.CloudOptions{Enabled: true, ExternalAuthorized: true, OrganizationBudget: 100000, PlatformBudget: 100000, SupportedVersions: []string{"v99.0"}, Admission: a, Timeout: 100 * time.Millisecond}, server.URL)
			if e != nil {
				t.Fatal(e)
			}
			result, e := tr.Send(ctx, c, m)
			if e == nil || result.State != channel.Unknown || calls.Load() != 1 {
				t.Fatal("uncertain HTTP was retried/released", e, result)
			}
			var state string
			if e = admin.QueryRow(ctx, `SELECT status FROM platform.wa_reservations WHERE outbox_id=$1`, m.CorrelationID).Scan(&state); e != nil || state != "unknown" {
				t.Fatal("financial unknown lost", state, e)
			}
			execSQL(`UPDATE platform.channel_outbox SET status='delivery_unknown',capability=NULL WHERE id=$1`, m.CorrelationID)
		})
	}
	// Actual process exits with a durable reservation before/after local provider receipt.
	for _, phase := range []string{"before_send", "after_send"} {
		t.Run("WA_crash_"+phase, func(t *testing.T) {
			reset()
			m, cap := job()
			var receipts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receipts.Add(1)
				if r.Method != "POST" {
					t.Error("crash test did not use actual messages POST")
				}
				_, _ = w.Write([]byte(`{"messages":[{"id":"wamid.synthetic.crash"}]}`))
			}))
			defer server.Close()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWAAdmissionCrashHelper$")
			cmd.Env = append(os.Environ(), "ABA_WA_CRASH=1", "ABA_WA_JOB="+m.CorrelationID, "ABA_WA_CAP="+cap, "ABA_WA_ORG="+org.ID, "ABA_WA_INTEGRATION="+integration, "ABA_WA_PROOF="+hex.EncodeToString(wa.Proof()), "ABA_WA_PHASE="+phase, "ABA_WA_LOCAL_URL="+server.URL)
			err := cmd.Run()
			var exit *exec.ExitError
			if !strings.Contains(fmt.Sprint(err), "exit status 77") || !errorsAsExit(err, &exit) {
				t.Fatal("child did not crash as expected", err)
			}
			if (phase == "before_send" && receipts.Load() != 0) || (phase == "after_send" && receipts.Load() != 1) {
				t.Fatal("incorrect crash/send boundary", receipts.Load())
			}
			execSQL(`UPDATE platform.channel_outbox SET lease_until=now()-interval '1 minute',status='delivery_unknown',capability=NULL WHERE id=$1`, m.CorrelationID)
			var recoverWG sync.WaitGroup
			recoverErrors := make(chan error, 2)
			for n := 0; n < 2; n++ {
				recoverWG.Add(1)
				go func() { defer recoverWG.Done(); recoverErrors <- wa.RecoverFinancial(ctx) }()
			}
			recoverWG.Wait()
			close(recoverErrors)
			for e := range recoverErrors {
				if e != nil {
					t.Fatal(e)
				}
			}
			var recoveryEvents int
			if e := admin.QueryRow(ctx, `SELECT count(*) FROM platform.wa_financial_events e JOIN platform.wa_reservations r ON r.id=e.reservation_id WHERE r.outbox_id=$1 AND e.kind='RECOVERED_UNKNOWN'`, m.CorrelationID).Scan(&recoveryEvents); e != nil || recoveryEvents != 1 {
				t.Fatal("duplicate recovery audit/money change", recoveryEvents, e)
			}
			var status string
			_ = admin.QueryRow(ctx, `SELECT status FROM platform.wa_reservations WHERE outbox_id=$1`, m.CorrelationID).Scan(&status)
			if status != "unknown" {
				t.Fatal("crash money recycled", status)
			}
		})
	}
	// Verifier mocks create only synthetic approvals, never trusted Meta attestations.
	reset()
	templateID := auth.UUID()
	rec := whatsapp.TemplateRecord{ID: templateID, Name: "synthetic_order", Language: "ru", Category: "UTILITY", Status: "APPROVED", Components: json.RawMessage(`[{"type":"BODY","text":"Заказ {{1}}"}]`), ParameterSchema: []string{"text"}, VerifiedUntil: time.Now().Add(time.Hour)}
	if e = wa.VerifyTemplate(ctx, c, rec.Name, rec.Language, whatsapp.MockTemplateVerifier{Record: rec}); e != nil {
		t.Fatal(e)
	}
	if e = wa.VerifyTemplate(ctx, c, rec.Name, rec.Language, whatsapp.MockTemplateVerifier{Record: rec}); e != nil {
		t.Fatal("duplicate verify", e)
	}
	longRecord := rec
	longRecord.ID = auth.UUID()
	longRecord.Name = strings.Repeat("t", 512)
	if e := wa.VerifyTemplate(ctx, c, longRecord.Name, longRecord.Language, whatsapp.MockTemplateVerifier{Record: longRecord}); e != nil {
		t.Fatal("bounded long template name", e)
	}
	invalid := rec
	invalid.ID = auth.UUID()
	invalid.Name = "invalid_components"
	invalid.Components = json.RawMessage(`[{"type":"HEADER"}]`)
	if e := wa.VerifyTemplate(ctx, c, invalid.Name, invalid.Language, whatsapp.MockTemplateVerifier{Record: invalid}); e == nil {
		t.Fatal("invalid verifier components persisted")
	}
	invalid = rec
	invalid.ID = auth.UUID()
	invalid.Name = "invalid_placeholders"
	invalid.Components = json.RawMessage(`[{"type":"BODY","text":"{{2}}"}]`)
	if e := wa.VerifyTemplate(ctx, c, invalid.Name, invalid.Language, whatsapp.MockTemplateVerifier{Record: invalid}); e == nil {
		t.Fatal("invalid parameter schema persisted")
	}

	templatePrice := auth.UUID()
	execSQL(`INSERT INTO platform.wa_pricing SELECT $1,organization_id,integration_id,'SYNTHETIC-template','ZZ','UTILITY','template',billing_basis,currency,10,false,source,verified_at,verified_until,effective_at,verified,synthetic FROM platform.wa_pricing WHERE id=$2`, templatePrice, pricing)
	attestSynthetic(t, admin, ctx, "pricing", templatePrice)
	makeTemplateJob := func() (channel.Message, string) {
		m, cap := job()
		m.Text = ""
		execSQL(`UPDATE platform.channel_outbox SET payload=jsonb_build_object('template_id',$2::uuid,'parameters',jsonb_build_array('synthetic'),'text','') WHERE id=$1`, m.CorrelationID, templateID)
		return m, cap
	}
	for _, name := range []string{"PENDING", "REJECTED", "PAUSED", "DISABLED", "UNKNOWN", "DRAFT", "expired", "language", "params", "waba", "cross_tenant", "components", "not_found", "live_mock"} {
		t.Run("WA_template_"+name, func(t *testing.T) {
			execSQL(`UPDATE platform.wa_templates SET status='APPROVED',verified_until=now()+interval '1 hour',waba_id='992',language='ru',components='[{"type":"BODY","text":"Заказ {{1}}"}]' WHERE id=$1`, templateID)
			attestSynthetic(t, admin, ctx, "template", templateID)
			m, cap := makeTemplateJob()
			v := whatsapp.Template{Name: rec.Name, Language: "ru", Parameters: []string{"synthetic"}}
			sim := true
			switch name {
			case "expired":
				execSQL(`UPDATE platform.wa_templates SET verified_until=now()-interval '1 minute' WHERE id=$1`, templateID)
			case "language":
				v.Language = "en"
			case "params":
				v.Parameters = []string{}
			case "waba":
				execSQL(`UPDATE platform.wa_templates SET waba_id='999' WHERE id=$1`, templateID)
			case "cross_tenant":
				m.OrganizationID = auth.UUID()
			case "components":
				execSQL(`UPDATE platform.wa_templates SET components='[{"type":"HEADER"}]' WHERE id=$1`, templateID)
			case "not_found":
				execSQL(`UPDATE platform.channel_outbox SET payload=jsonb_set(payload,'{template_id}',to_jsonb($2::text)) WHERE id=$1`, m.CorrelationID, auth.UUID())
			case "live_mock":
				sim = false
			default:
				execSQL(`UPDATE platform.wa_templates SET status=$2 WHERE id=$1`, templateID, name)
			}
			if _, e = reserve(m, cap, sim, &v); e == nil {
				t.Fatal("template bypass", name)
			}
			execSQL(`UPDATE platform.channel_outbox SET status='failed',capability=NULL WHERE id=$1`, m.CorrelationID)
		})
	}
	execSQL(`UPDATE platform.wa_templates SET status='APPROVED',verified_until=now()+interval '1 hour',waba_id='992',language='ru',components='[{"type":"BODY","text":"Заказ {{1}}"}]' WHERE id=$1`, templateID)
	attestSynthetic(t, admin, ctx, "template", templateID)
	for _, mode := range []string{"no_pricing", "no_budget", "parameter_type"} {
		t.Run("WA_template_financial_"+mode, func(t *testing.T) {
			reset()
			m, cap := makeTemplateJob()
			switch mode {
			case "no_pricing":
				execSQL(`UPDATE platform.wa_pricing SET max_charge=NULL WHERE id=$1`, templatePrice)
			case "no_budget":
				execSQL(`UPDATE platform.wa_budgets SET limit_amount=reserved+spent WHERE organization_id=$1`, org.ID)
			case "parameter_type":
				execSQL(`UPDATE platform.channel_outbox SET payload=jsonb_set(payload,'{parameters}',jsonb_build_array(1)) WHERE id=$1`, m.CorrelationID)
			}
			if _, e := reserve(m, cap, true, &whatsapp.Template{Name: rec.Name, Language: "ru", Parameters: []string{"synthetic"}}); e == nil {
				t.Fatal("template without valid finance/schema accepted")
			}
			execSQL(`UPDATE platform.wa_pricing SET max_charge=10 WHERE id=$1`, templatePrice)
			attestSynthetic(t, admin, ctx, "pricing", templatePrice)
			execSQL(`UPDATE platform.channel_outbox SET status='failed',capability=NULL WHERE id=$1`, m.CorrelationID)
		})
	}
	// Real serializer and DB admission, local HTTP only, outside free-text window.
	t.Run("WA_approved_template_HTTP_admission", func(t *testing.T) {
		reset()
		execSQL(`UPDATE platform.channel_principals SET window_until=now()-interval '1 day' WHERE id=$1`, customer)
		m, cap := makeTemplateJob()
		conn, e := d.Pool.Acquire(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer conn.Release()
		a := &whatsapp.FinancialAdmission{Conn: conn, Proof: wa.Proof(), Capability: cap, Simulation: true}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var b map[string]any
			_ = json.NewDecoder(r.Body).Decode(&b)
			if b["type"] != "template" {
				t.Error("wrong serializer")
			}
			_, _ = w.Write([]byte(`{"messages":[{"id":"wamid.synthetic.template"}]}`))
		}))
		defer server.Close()
		tr, e := whatsapp.NewOfflineCloudTransport(whatsapp.CloudOptions{Enabled: true, ExternalAuthorized: true, OrganizationBudget: 100000, PlatformBudget: 100000, SupportedVersions: []string{"v99.0"}, Admission: a}, server.URL)
		if e != nil {
			t.Fatal(e)
		}
		result, e := tr.SendTemplate(ctx, c, m, whatsapp.Template{Name: rec.Name, Language: "ru", Parameters: []string{"synthetic"}})
		if e != nil || result.State != channel.Sent {
			t.Fatal("template admission/HTTP", e, result)
		}
		// Production constructor rejects the same synthetic financial permit before HTTP.
		if _, e = whatsapp.NewCloudTransport(whatsapp.CloudOptions{Enabled: true, ExternalAuthorized: true, OrganizationBudget: 1, PlatformBudget: 1, SupportedVersions: []string{"v99.0"}, Admission: a}).SendTemplate(ctx, c, m, whatsapp.Template{Name: rec.Name, Language: "ru", Parameters: []string{"synthetic"}}); e == nil {
			t.Fatal("synthetic permit accepted in real transport")
		}
		execSQL(`UPDATE platform.channel_outbox SET status='sent',capability=NULL WHERE id=$1`, m.CorrelationID)
	})
	t.Run("WA_template_queue_positive_outside_window", func(t *testing.T) {
		reset()
		execSQL(`UPDATE platform.channel_principals SET window_until=now()-interval '1 day' WHERE id=$1`, customer)
		execSQL(`UPDATE platform.channel_outbox SET status='cancelled',capability=NULL WHERE integration_id=$1 AND status IN ('sending','pending','delivery_unknown')`, integration)
		key := auth.UUID()
		if _, e := wa.QueueTemplate(ctx, tenant, conv, templateID, key, []string{"synthetic"}); e != nil {
			t.Fatal(e)
		}
		if _, e := wa.SendOne(ctx); e != nil {
			t.Fatal("template worker send", e)
		}
		var status string
		if e := admin.QueryRow(ctx, `SELECT status FROM platform.channel_outbox WHERE id=$1`, key).Scan(&status); e != nil || status != "sent" {
			t.Fatal("template outbox not sent", status, e)
		}
		if _, e := wa.QueueTemplate(ctx, tenant, conv, templateID, key, []string{"synthetic"}); e != nil {
			t.Fatal("queue replay", e)
		}
		var n int
		_ = admin.QueryRow(ctx, `SELECT count(*) FROM platform.wa_reservations WHERE outbox_id=$1 AND status='charged'`, key).Scan(&n)
		if n != 1 {
			t.Fatal("template charged more than once", n)
		}
	})
	// Revocation/worker delay after enqueue is rechecked at dispatch; same outbox queue.
	t.Run("WA_template_queue_revoked_before_dispatch", func(t *testing.T) {
		reset()
		key := auth.UUID()
		id, e := wa.QueueTemplate(ctx, tenant, conv, templateID, key, []string{"synthetic"})
		if e != nil || id != key {
			t.Fatal("queue", e)
		}
		if _, e = wa.QueueTemplate(ctx, tenant, conv, templateID, key, []string{"synthetic"}); e != nil {
			t.Fatal("idempotent queue", e)
		}
		revoked := rec
		revoked.Status = "PAUSED"
		if err := wa.VerifyTemplate(ctx, c, rec.Name, rec.Language, whatsapp.MockTemplateVerifier{Record: revoked}); err != nil {
			t.Fatal(err)
		}
		// Old unknown messages intentionally fence the same principal. Close only synthetic
		// older jobs to let the newly enqueued test task be claimed, never change money.
		execSQL(`UPDATE platform.channel_outbox SET status='cancelled',capability=NULL WHERE integration_id=$1 AND id<>$2 AND status IN ('sending','pending','delivery_unknown')`, integration, key)
		if _, e = wa.SendOne(ctx); e != nil {
			t.Fatal(e)
		}
		var status string
		_ = admin.QueryRow(ctx, `SELECT status FROM platform.channel_outbox WHERE id=$1`, key).Scan(&status)
		if status != "failed" {
			t.Fatal("revoked queue sent", status)
		}
	})
	// Owner-only finance methods and forged tenant context are checked independently of UI.
	if _, e := wa.FinancialState(ctx, memberTenant); e == nil {
		t.Fatal("staff finance read")
	}
	if _, e := wa.Templates(ctx, memberTenant); e == nil {
		t.Fatal("staff template registry read")
	}
	if _, e = wa.FinancialState(ctx, tenant); e != nil {
		t.Fatal(e)
	}
	if _, e = wa.Templates(ctx, tenant); e != nil {
		t.Fatal(e)
	}
	alien := tenant
	alien.OrganizationID = auth.UUID()
	if _, e = wa.FinancialState(ctx, alien); e == nil {
		t.Fatal("foreign financial read")
	}
	testTrusted9C(t, d, admin, ctx, wa, c, tenant, memberTenant, pricing, job, reserve, finish, reset)
	var balanced bool
	if e = admin.QueryRow(ctx, `SELECT b.reserved=coalesce((SELECT sum(amount) FROM platform.wa_reservations WHERE organization_id=b.organization_id AND status IN ('reserved','unknown')),0) AND b.spent=coalesce((SELECT sum(charged) FROM platform.wa_reservations WHERE organization_id=b.organization_id),0) FROM platform.wa_budgets b WHERE b.organization_id=$1`, org.ID).Scan(&balanced); e != nil || !balanced {
		t.Fatal("ledger conservation", e)
	}
	t.Log("PASS: durable WhatsApp financial reserve/settle/unknown/recovery, pricing/window/region, templates/trusted mock registry/queue, same local HTTP serializer and synthetic-to-live denial")
}

func errorsAsExit(e error, target **exec.ExitError) bool {
	v, ok := e.(*exec.ExitError)
	if ok {
		*target = v
	}
	return ok
}

type waCrashAdmission struct {
	base  *whatsapp.FinancialAdmission
	phase string
}

func (a waCrashAdmission) OfflineOnly() bool { return true }
func (a waCrashAdmission) Reserve(ctx context.Context, c whatsapp.Config, m channel.Message, v *whatsapp.Template) (string, error) {
	id, e := a.base.Reserve(ctx, c, m, v)
	if e == nil && a.phase == "before_send" {
		os.Exit(77)
	}
	return id, e
}
func (a waCrashAdmission) Finish(context.Context, string, channel.SendResult) error {
	os.Exit(77)
	return nil
}
func TestWAAdmissionCrashHelper(t *testing.T) {
	if os.Getenv("ABA_WA_CRASH") != "1" {
		t.Skip("subprocess helper only")
	}
	ctx := context.Background()
	d, e := db.Open(ctx, testDSN(t, "ABA_TEST_DATABASE_URL", "aba_app"), 1, time.Second)
	if e != nil {
		t.Fatal(e)
	}
	conn, e := d.Pool.Acquire(ctx)
	if e != nil {
		t.Fatal(e)
	}
	proof, _ := hex.DecodeString(os.Getenv("ABA_WA_PROOF"))
	a := &whatsapp.FinancialAdmission{Conn: conn, Proof: proof, Capability: os.Getenv("ABA_WA_CAP"), Simulation: true}
	c := whatsapp.Config{OrganizationID: os.Getenv("ABA_WA_ORG"), IntegrationID: os.Getenv("ABA_WA_INTEGRATION"), AppID: "991", BusinessAccountID: "992", PhoneNumberID: "993", GraphVersion: "v99.0", AccessToken: "mock:synthetic", AppSecret: "mock:synthetic-app-secret", VerifyToken: "mock:synthetic-verify-token", AllowedUsers: map[string]bool{"700": true}, Window: 24 * time.Hour}
	m := channel.Message{OrganizationID: c.OrganizationID, IntegrationID: c.IntegrationID, CorrelationID: os.Getenv("ABA_WA_JOB"), ExternalUserID: "700", ExternalChatID: "700", Channel: channel.WhatsApp, Kind: channel.Text, Direction: "outbound", Text: "synthetic outbound"}
	u, e := url.Parse(os.Getenv("ABA_WA_LOCAL_URL"))
	if e != nil || u.Hostname() != "127.0.0.1" {
		t.Fatal("local mock only")
	}
	tr, e := whatsapp.NewOfflineCloudTransport(whatsapp.CloudOptions{Enabled: true, ExternalAuthorized: true, OrganizationBudget: 100000, PlatformBudget: 100000, SupportedVersions: []string{"v99.0"}, Admission: waCrashAdmission{a, os.Getenv("ABA_WA_PHASE")}}, u.String())
	if e != nil {
		t.Fatal(e)
	}
	if _, e = tr.Send(ctx, c, m); e != nil {
		t.Fatal(e)
	}
	t.Fatal("crash boundary was not reached")
}
