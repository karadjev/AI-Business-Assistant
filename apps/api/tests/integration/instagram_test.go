//go:build integration

package integration

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/agent"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/channel"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/conversations"
	httpapi "github.com/karadjev/AI-Business-Assistant/apps/api/internal/http"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/instagram"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/db"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/whatsapp"
)

// Invoked only by TestDatabaseAndMigrations after its isolated aba_test marker,
// roles, migration up/down/repeat/concurrent gates. No working DSN is accepted.
func testInstagramDurable(t *testing.T, d *db.Database, admin *pgx.Conn, ctx context.Context) {
	cfg := config.Instagram{Enabled: true, Key: bytes.Repeat([]byte{73}, 32), AppSecret: "mock:instagram-stage2-secret", VerifyToken: "mock:instagram-stage2-verify"}
	ac, _ := config.LoadAgent(func(string) string { return "" })
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	chat := &conversations.Service{Pool: d.Pool, Config: ac, Provider: agent.Mock{}, Log: log}
	svc := instagram.Service{Pool: d.Pool, Config: cfg, Chat: chat, Log: log}
	identity := auth.New(d.Pool, config.Auth{Origin: "http://127.0.0.1:3000", SessionTTL: time.Hour, IdleTTL: time.Hour})
	handler := httpapi.NewHandlerWithChannels(d, log, 10*time.Second, 65536, identity, ac, agent.Mock{}, config.Telegram{}, config.WhatsApp{}, cfg)
	type shop struct {
		tenant                                   catalog.Tenant
		token, csrf, id, account, callback, conv string
	}
	shops := make([]shop, 3)
	for i := range shops {
		uid, token, csrf := auth.UUID(), auth.Token(), auth.Token()
		if _, err := admin.Exec(ctx, `INSERT INTO platform.users(id,email,password_hash,display_name,email_verified_at) VALUES($1,$2,'synthetic','Instagram fixture',now())`, uid, fmt.Sprintf("instagram-stage2-%d@example.test", i)); err != nil {
			t.Fatal(err)
		}
		if _, err := admin.Exec(ctx, `INSERT INTO platform.sessions(id,user_id,token_hash,csrf_hash,expires_at,idle_expires_at) VALUES($1,$2,$3,$4,now()+interval '1 hour',now()+interval '1 hour')`, auth.UUID(), uid, auth.Hash(token), auth.Hash(csrf)); err != nil {
			t.Fatal(err)
		}
		session, err := identity.Authenticate(ctx, token)
		if err != nil {
			t.Fatal(err)
		}
		org, err := (organizations.Service{Pool: d.Pool}).CreateTest(ctx, session, fmt.Sprintf("Instagram Stage2 %d", i))
		if err != nil {
			t.Fatal(err)
		}
		shops[i] = shop{tenant: catalog.Tenant{Session: session, OrganizationID: org.ID}, token: token, csrf: csrf}
	}
	request := func(index int, method, path string, want int) []byte {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(`{}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", identity.Config.Origin)
		if index >= 0 {
			r.AddCookie(&http.Cookie{Name: "aba_session", Value: shops[index].token})
			r.Header.Set("X-CSRF-Token", shops[index].csrf)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s endpoint status %d want %d: %s", method, w.Code, want, w.Body.String())
		}
		return w.Body.Bytes()
	}
	root := func(i int) string {
		return "/api/v1/organizations/" + shops[i].tenant.OrganizationID + "/integrations/instagram"
	}
	t.Run("endpoints-auth-state", func(t *testing.T) {
		request(-1, "GET", root(0), 401)
		request(1, "GET", root(0), 404)
		r := httptest.NewRequest("POST", root(0)+"/oauth/start", nil)
		r.Header.Set("Origin", identity.Config.Origin)
		r.AddCookie(&http.Cookie{Name: "aba_session", Value: shops[0].token})
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal("missing CSRF accepted")
		}
		for i := range shops {
			var started struct {
				ID       string `json:"integration_id"`
				Callback string `json:"callback_url"`
			}
			if err := json.Unmarshal(request(i, "POST", root(i)+"/oauth/start", 200), &started); err != nil || started.ID == "" {
				t.Fatal("start missing durable identity")
			}
			shops[i].id, shops[i].callback = started.ID, started.Callback
		}
		foreign := strings.Replace(shops[0].callback, shops[0].tenant.OrganizationID, shops[1].tenant.OrganizationID, 1)
		request(1, "GET", foreign, 403)
		for i := range shops {
			request(i, "GET", shops[i].callback, 200)
			request(i, "GET", shops[i].callback, 403)
			var status struct {
				Connection struct {
					Account string `json:"account_id"`
					Status  string `json:"status"`
				}
			}
			raw := request(i, "GET", root(i), 200)
			if json.Unmarshal(raw, &status) != nil || status.Connection.Status != "connecting" {
				t.Fatal("unobserved webhook shown connected")
			}
			shops[i].account = status.Connection.Account
			if bytes.Contains(raw, []byte("mock:account:")) || bytes.Contains(raw, cfg.Key) {
				t.Fatal("secret in status")
			}
		}
		// A store owner cannot operate another store, even with their own valid session.
		request(1, "POST", root(0)+"/disconnect", 404)
		if _, err := admin.Exec(ctx, `INSERT INTO platform.organization_members(organization_id,user_id,role,can_manage_orders) VALUES($1,$2,'member',true)`, shops[0].tenant.OrganizationID, shops[1].tenant.Session.User.ID); err != nil {
			t.Fatal(err)
		}
		request(1, "GET", root(0), 404)

	})
	if shops[0].id == "" {
		return
	}
	t.Run("encrypted-storage-and-constraints", func(t *testing.T) {
		var cipher []byte
		if err := admin.QueryRow(ctx, `SELECT secret_cipher FROM platform.channel_integrations WHERE id=$1`, shops[0].id).Scan(&cipher); err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(cipher, []byte("mock:account:")) {
			t.Fatal("plaintext credential")
		}
		box := instagram.CredentialBox{Key: cfg.Key}
		a := instagram.Binding{OrganizationID: shops[0].tenant.OrganizationID, IntegrationID: shops[0].id, AccountID: shops[0].account}
		if _, err := box.Open(a, cipher); err != nil {
			t.Fatal("stored credential cannot be decrypted")
		}
		for _, i := range []int{1, 2} {
			foreign := a
			foreign.OrganizationID = shops[i].tenant.OrganizationID
			if _, err := box.Open(foreign, cipher); err == nil {
				t.Fatal("foreign credential decrypted")
			}
		}
		if _, err := d.Pool.Exec(ctx, `SELECT secret_cipher FROM platform.channel_integrations`); err == nil {
			t.Fatal("runtime can read ciphertext")
		}
		if _, err := d.Pool.Exec(ctx, `SELECT state_hash FROM platform.instagram_oauth_states`); err == nil {
			t.Fatal("runtime can read state store")
		}
		var denied []byte
		if err := d.Pool.QueryRow(ctx, `SELECT platform.channel_credentials($1,$2)`, shops[0].id, svc.Proof()).Scan(&denied); err != nil || len(denied) != 0 {
			t.Fatal("provider credentials namespace leaked", err)
		}
		// Cannot substitute another account or tenant even through owner-authorized SQL.
		tx, err := tenancy.Begin(ctx, d.Pool, shops[1].tenant, true, true)
		if err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(ctx, `SELECT platform.instagram_prepare($1,$2,$3,$4,$5)`, shops[0].tenant.OrganizationID, shops[0].id, shops[0].account, cipher, auth.Hash(string(svc.Proof())))
		tx.Rollback(ctx)
		if err == nil {
			t.Fatal("foreign prepare")
		}
		_, err = admin.Exec(ctx, `UPDATE platform.instagram_connections SET account_id=$1 WHERE integration_id=$2`, shops[0].account, shops[1].id)
		if err == nil {
			t.Fatal("duplicate account accepted")
		}
		var protected int
		if err = admin.QueryRow(ctx, `SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='platform' AND c.relname IN ('instagram_connections','instagram_oauth_states') AND c.relrowsecurity AND c.relforcerowsecurity`).Scan(&protected); err != nil || protected != 2 {
			t.Fatal("FORCE RLS missing")
		}
	})
	// Seed tenant-distinct real products before any webhook. This makes the first
	// worker turn exercise common AI -> catalog reads rather than an empty search.
	catalogFixtures := make([]instagramCatalogFixture, len(shops))
	for i := range shops {
		catalogFixtures[i] = seedInstagramCatalog(t, d, ctx, shops[i].tenant, i)
	}
	payload := func(i int, mid string, media bool) []byte {
		message := map[string]any{"mid": mid, "text": fmt.Sprintf("Есть SYNTH-IG-%d?", i)}
		if media {
			message["attachments"] = []any{map[string]string{"type": "image"}}
		}
		raw, _ := json.Marshal(map[string]any{"object": "instagram", "entry": []any{map[string]any{"id": shops[i].account, "messaging": []any{map[string]any{"sender": map[string]string{"id": "9700"}, "recipient": map[string]string{"id": shops[i].account}, "timestamp": time.Now().UnixMilli(), "message": message}}}}})
		return raw
	}
	sign := func(raw []byte) string {
		h := hmac.New(sha256.New, []byte(cfg.AppSecret))
		h.Write(raw)
		return "sha256=" + hex.EncodeToString(h.Sum(nil))
	}
	t.Run("webhook-durable-duplicate-and-worker", func(t *testing.T) {
		verify := httptest.NewRequest("GET", "/api/v1/webhooks/instagram/"+shops[0].id+"?"+url.Values{"hub.mode": {"subscribe"}, "hub.verify_token": {cfg.VerifyToken}, "hub.challenge": {"123456"}}.Encode(), nil)
		verified := httptest.NewRecorder()
		handler.ServeHTTP(verified, verify)
		if verified.Code != 200 || verified.Body.String() != "123456" || verified.Result().Header.Get("Content-Type") != "text/plain; charset=utf-8" {
			t.Fatal("registered webhook verification")
		}
		unsigned := httptest.NewRequest("POST", "/api/v1/webhooks/instagram/"+shops[0].id, bytes.NewReader(payload(0, "ig-unsigned", false)))
		rejected := httptest.NewRecorder()
		handler.ServeHTTP(rejected, unsigned)
		if rejected.Code != 403 {
			t.Fatal("unsigned registered webhook accepted")
		}

		for i := range shops {
			raw := payload(i, "ig-stage2-first", false)
			signed := httptest.NewRequest("POST", "/api/v1/webhooks/instagram/"+shops[i].id, bytes.NewReader(raw))
			signed.Header.Set("X-Hub-Signature-256", sign(raw))
			signed.Header.Set("Content-Type", "application/json")
			accepted := httptest.NewRecorder()
			handler.ServeHTTP(accepted, signed)
			if accepted.Code != 200 {
				t.Fatalf("registered signed ingress: %d", accepted.Code)
			}
			var pending, messages int
			if err := admin.QueryRow(ctx, `SELECT (SELECT count(*) FROM platform.channel_inbox WHERE integration_id=$1 AND status='pending'),(SELECT count(*) FROM platform.messages WHERE organization_id=$2)`, shops[i].id, shops[i].tenant.OrganizationID).Scan(&pending, &messages); err != nil || pending != 1 || messages != 0 {
				t.Fatal("HTTP acknowledgement must persist queue without running AI", pending, messages, err)
			}
			if n, err := svc.Ingress(ctx, shops[i].id, sign(raw), raw); err != nil || n != 0 {
				t.Fatal("duplicate", n, err)
			}
		}
		for range shops {
			if worked, err := svc.ProcessOne(ctx); err != nil || !worked {
				t.Fatal("worker", worked, err)
			}
		}
		for i := range shops {
			if err := admin.QueryRow(ctx, `SELECT conversation_id FROM platform.channel_principals WHERE integration_id=$1`, shops[i].id).Scan(&shops[i].conv); err != nil {
				t.Fatal(err)
			}
			var count int
			if err := admin.QueryRow(ctx, `SELECT count(*) FROM platform.messages WHERE conversation_id=$1 AND role='customer'`, shops[i].conv).Scan(&count); err != nil || count != 1 {
				t.Fatal("duplicate customer processing", count, err)
			}
			if _, err := chat.Get(ctx, shops[i].tenant, shops[i].conv); err != nil {
				t.Fatal("own conversation", err)
			}
		}
		for i := range shops {
			assertInstagramCatalogResponse(t, admin, ctx, shops[i].tenant.OrganizationID, shops[i].conv, catalogFixtures[i], catalogFixtures)
		}
		// Seed actual attachment rows in both A and B before cross-tenant read tests.
		for _, i := range []int{0, 1} {
			media := payload(i, "ig-stage2-attachment-proof", true)
			if _, err := svc.Ingress(ctx, shops[i].id, sign(media), media); err != nil {
				t.Fatal(err)
			}
		}
		for range 2 {
			if worked, err := svc.ProcessOne(ctx); err != nil || !worked {
				t.Fatal("attachment worker", err)
			}
		}
		for _, i := range []int{0, 1} {
			var count int
			if err := admin.QueryRow(ctx, `SELECT count(*) FROM platform.messages WHERE conversation_id=$1 AND data @> '{"attachments":[{"type":"image"}]}'::jsonb`, shops[i].conv).Scan(&count); err != nil || count != 1 {
				t.Fatal("attachment fixture missing", err)
			}
		}
		// Telegram and WhatsApp configuration coexist in the same store. Preserve WA semantics.
		wa := whatsapp.Service{Pool: d.Pool, Key: bytes.Repeat([]byte{74}, 32), Chat: chat, Transport: whatsapp.Mock{}}
		if _, err := wa.Connect(ctx, shops[0].tenant, whatsapp.ConnectInput{AppID: "100", BusinessID: "200", PhoneID: "9700300", GraphVersion: "v0.0", Credentials: whatsapp.Credentials{AccessToken: "mock:synthetic", AppSecret: "mock:synthetic-app-secret", VerifyToken: "mock:synthetic-verify-token", AllowedUsers: []string{"9700"}}}); err != nil {
			t.Fatal("WA coexistence", err)
		}
		if _, err := wa.Status(ctx, shops[0].tenant); err != nil {
			t.Fatal("WA status with IG", err)
		}
		if _, err := wa.Control(ctx, shops[0].tenant, false, false); err != nil {
			t.Fatal("WA control with IG", err)
		}
		var active bool
		if err := admin.QueryRow(ctx, `SELECT active FROM platform.channel_integrations WHERE id=$1`, shops[0].id).Scan(&active); err != nil || !active {
			t.Fatal("WA control changed Instagram")
		}
	})
	t.Run("connected-outbound-safety-gate", func(t *testing.T) {
		if sent, err := svc.SendOne(ctx); err != nil || sent {
			t.Fatalf("Instagram worker sent despite configured safety block: %t, %v", sent, err)
		}
		for _, shop := range shops {
			var active, enabled bool
			var status string
			if err := admin.QueryRow(ctx, `SELECT i.active,i.sending_enabled,c.status
 FROM platform.channel_integrations i JOIN platform.instagram_connections c
 ON c.organization_id=i.organization_id AND c.integration_id=i.id
 WHERE i.organization_id=$1 AND i.id=$2`, shop.tenant.OrganizationID, shop.id).Scan(&active, &enabled, &status); err != nil || !active || enabled || status != "connected" {
				t.Fatalf("connected mock still requires sending off: %t, %t, %s, %v", active, enabled, status, err)
			}
			var cancelled, unsafe int
			if err := admin.QueryRow(ctx, `SELECT count(*) FILTER (WHERE q.status='cancelled'),
 count(*) FILTER (WHERE q.status<>'cancelled' OR q.attempts<>0 OR q.capability IS NOT NULL OR q.provider_id IS NOT NULL
  OR p.id IS NULL OR NOT EXISTS(SELECT 1 FROM platform.channel_events e WHERE e.organization_id=q.organization_id
    AND e.integration_id=q.integration_id AND e.outbox_id=q.id AND e.kind='cancelled'))
 FROM platform.channel_outbox q LEFT JOIN platform.channel_principals p ON p.organization_id=q.organization_id AND p.integration_id=q.integration_id
 AND p.conversation_id=q.conversation_id AND p.external_user_id=q.external_user_id AND p.channel='instagram'
 WHERE q.organization_id=$1 AND q.integration_id=$2`, shop.tenant.OrganizationID, shop.id).Scan(&cancelled, &unsafe); err != nil || cancelled == 0 || unsafe != 0 {
				t.Fatalf("unchecked Instagram outbox: cancelled=%d unsafe=%d err=%v", cancelled, unsafe, err)
			}
		}
	})
	t.Run("three-tenant-rls-and-authority", func(t *testing.T) {
		for i := range shops {
			for j := range shops {
				if i == j {
					continue
				}
				if _, err := chat.Get(ctx, shops[i].tenant, shops[j].conv); err == nil {
					t.Fatal("foreign conversation")
				}
				tx, err := tenancy.Begin(ctx, d.Pool, shops[i].tenant, true, true)
				if err != nil {
					t.Fatal(err)
				}
				for _, query := range []string{`SELECT count(*) FROM platform.messages WHERE conversation_id=$1`, `SELECT count(*) FROM platform.messages WHERE conversation_id=$1 AND data ? 'attachments'`, `SELECT count(*) FROM platform.channel_inbox WHERE conversation_id=$1`, `SELECT count(*) FROM platform.channel_outbox WHERE conversation_id=$1`} {
					var count int
					if err = tx.QueryRow(ctx, query, shops[j].conv).Scan(&count); err != nil || count != 0 {
						t.Fatal("foreign rows visible", err)
					}
				}
				tx.Rollback(ctx)
			}
		}
		var empty int
		if err := d.Pool.QueryRow(ctx, `SELECT count(*) FROM platform.instagram_connections`).Scan(&empty); err != nil || empty != 0 {
			t.Fatal("empty context exposed connections")
		}
		if _, err := svc.Status(ctx, catalog.Tenant{OrganizationID: shops[0].tenant.OrganizationID}); err == nil {
			t.Fatal("missing session accepted")
		}
		if _, err := svc.Status(ctx, catalog.Tenant{Session: shops[0].tenant.Session}); err == nil {
			t.Fatal("empty tenant accepted")
		}
		raw := payload(0, "ig-stage2-forged", false)
		if _, err := svc.Ingress(ctx, shops[1].id, sign(raw), raw); err == nil {
			t.Fatal("foreign external account")
		}
		if _, err := svc.Ingress(ctx, auth.UUID(), sign(raw), raw); err == nil {
			t.Fatal("unknown integration")
		}
		batch, err := instagram.Normalize(instagram.Binding{OrganizationID: shops[0].tenant.OrganizationID, IntegrationID: shops[0].id, AccountID: shops[0].account}, raw, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		m := batch.Messages[0]
		m.OrganizationID = shops[1].tenant.OrganizationID
		forged, _ := json.Marshal([]channel.Message{m})
		var inserted int
		if err = d.Pool.QueryRow(ctx, `SELECT platform.channel_ingress($1,$2,$3)`, shops[0].id, svc.Proof(), forged).Scan(&inserted); err == nil {
			t.Fatal("forged queue tenant accepted")
		}
	})
	t.Run("attachments-leases-retry-dlq", func(t *testing.T) {
		raw := payload(0, "ig-stage2-image", true)
		if _, err := svc.Ingress(ctx, shops[0].id, sign(raw), raw); err != nil {
			t.Fatal(err)
		}
		var poisonedID string
		if err := admin.QueryRow(ctx, `SELECT id FROM platform.channel_inbox WHERE integration_id=$1 AND external_id='message:ig-stage2-image'`, shops[0].id).Scan(&poisonedID); err != nil {
			t.Fatal(err)
		}
		// Even storage corruption cannot replace the SQL-authoritative tenant/conversation.
		if _, err := admin.Exec(ctx, `UPDATE platform.channel_inbox SET payload=payload||jsonb_build_object('organization_id',$2::text,'conversation_id',$3::text,'channel_type','whatsapp') WHERE id=$1`, poisonedID, shops[1].tenant.OrganizationID, shops[1].conv); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.ProcessOne(ctx); err != nil {
			t.Fatal(err)
		}
		var kind string
		if err := admin.QueryRow(ctx, `SELECT data->'attachments'->0->>'type' FROM platform.messages WHERE conversation_id=$1 AND data->>'external_message_id'='ig-stage2-image'`, shops[0].conv).Scan(&kind); err != nil || kind != "image" {
			t.Fatal("attachment metadata", err)
		}
		raw = payload(0, "ig-stage2-retry", false)
		if _, err := svc.Ingress(ctx, shops[0].id, sign(raw), raw); err != nil {
			t.Fatal(err)
		}
		var cap, id string
		for attempt := 1; attempt <= 5; attempt++ {
			cap = auth.UUID()
			var claimed []byte
			if err := d.Pool.QueryRow(ctx, `SELECT platform.channel_claim($1,$2,$3,$4)`, svc.Proof(), cap, auth.UUID(), auth.UUID()).Scan(&claimed); err != nil || len(claimed) == 0 {
				t.Fatal("retry claim", err)
			}
			var m channel.Message
			if json.Unmarshal(claimed, &m) != nil {
				t.Fatal("claim decode")
			}
			id = m.CorrelationID
			if attempt == 1 {
				leased := catalog.Tenant{OrganizationID: shops[0].tenant.OrganizationID, ChannelType: "instagram", ChannelCapability: cap}
				assertInstagramCatalogLease(t, d, admin, ctx, leased, catalogFixtures)
				t.Run("specialist_catalog_scope", func(t *testing.T) {
					testStage22SpecialistCatalog(t, ctx, d, leased, m.ConversationID, m.ExternalUserID, catalogFixtures)
				})
				foreign := catalog.Tenant{OrganizationID: shops[1].tenant.OrganizationID, ChannelType: "instagram", ChannelCapability: cap}
				if tx, err := tenancy.Begin(ctx, d.Pool, foreign, true, true); err == nil {
					tx.Rollback(ctx)
					t.Fatal("cross-tenant lease")
				}
				wrongChannel := catalog.Tenant{OrganizationID: shops[0].tenant.OrganizationID, ChannelType: "whatsapp", ChannelCapability: cap}
				if tx, err := tenancy.Begin(ctx, d.Pool, wrongChannel, true, true); err == nil {
					tx.Rollback(ctx)
					t.Fatal("cross-channel lease")
				}
			}
			if _, err := d.Pool.Exec(ctx, `SELECT platform.channel_finish_classified($1,false,true)`, cap); err != nil {
				t.Fatal(err)
			}
			var status string
			var scheduled bool
			if err := admin.QueryRow(ctx, `SELECT status,available_at>now()+make_interval(secs=>greatest(1,power(2,attempts)::integer-1)) FROM platform.channel_inbox WHERE id=$1`, id).Scan(&status, &scheduled); err != nil {
				t.Fatal(err)
			}
			if attempt < 5 {
				if status != "pending" || !scheduled {
					t.Fatal("backoff missing")
				}
				if _, err := admin.Exec(ctx, `UPDATE platform.channel_inbox SET available_at=now()-interval '1 second' WHERE id=$1`, id); err != nil {
					t.Fatal(err)
				}
			} else if status != "failed" {
				t.Fatal("retry limit/DLQ missing")
			}
		}
		if worked, err := svc.ProcessOne(ctx); err != nil || worked {
			t.Fatal("dead letter reprocessed")
		}
	})
	t.Run("write-isolation-dlq-and-pooled-connections", func(t *testing.T) {
		tenants := make([]catalog.Tenant, len(shops))
		convs := make([]string, len(shops))
		for i := range shops {
			tenants[i] = shops[i].tenant
			convs[i] = shops[i].conv
		}
		testInstagramWritesAndPool(t, d, admin, ctx, tenants, convs)
	})
	t.Run("worker-interruption-after-message-commit-before-ack", func(t *testing.T) {
		if _, err := chat.Control(ctx, shops[0].tenant, shops[0].conv, "human", 0); err != nil {
			t.Fatal(err)
		}
		var ordersBefore int
		if err := admin.QueryRow(ctx, `SELECT count(*) FROM platform.orders WHERE organization_id=$1`, shops[0].tenant.OrganizationID).Scan(&ordersBefore); err != nil {
			t.Fatal(err)
		}
		raw := payload(0, "ig-stage2-interrupted", false)
		if _, err := svc.Ingress(ctx, shops[0].id, sign(raw), raw); err != nil {
			t.Fatal(err)
		}
		cap := auth.UUID()
		var claimed []byte
		if err := d.Pool.QueryRow(ctx, `SELECT platform.channel_claim($1,$2,$3,$4)`, svc.Proof(), cap, auth.UUID(), auth.UUID()).Scan(&claimed); err != nil {
			t.Fatal(err)
		}
		var m channel.Message
		if json.Unmarshal(claimed, &m) != nil || m.ConversationID != shops[0].conv {
			t.Fatal("interruption claim binding")
		}
		workerTenant := catalog.Tenant{OrganizationID: m.OrganizationID, ChannelType: "instagram", ChannelCapability: cap}
		if err := (channel.Engine{Chat: chat}).Receive(ctx, workerTenant, m); err != nil {
			t.Fatal(err)
		}
		// Simulate a crash after shared service commit but before inbox acknowledgement.
		if _, err := admin.Exec(ctx, `UPDATE platform.channel_inbox SET lease_until=now()-interval '1 second' WHERE id=$1`, m.CorrelationID); err != nil {
			t.Fatal(err)
		}
		if worked, err := svc.ProcessOne(ctx); err != nil || !worked {
			t.Fatal("interrupted worker recovery", err)
		}
		// A late finalizer from the interrupted worker cannot regress the new result.
		if _, err := d.Pool.Exec(ctx, `SELECT platform.channel_finish_classified($1,false,true)`, cap); err != nil {
			t.Fatal(err)
		}
		var state string
		var attempts, count, ordersAfter int
		if err := admin.QueryRow(ctx, `SELECT status,attempts FROM platform.channel_inbox WHERE id=$1`, m.CorrelationID).Scan(&state, &attempts); err != nil || state != "done" || attempts != 2 {
			t.Fatal("recovery fencing", err)
		}
		if err := admin.QueryRow(ctx, `SELECT count(*) FROM platform.messages WHERE conversation_id=$1 AND role='customer' AND client_key=$2`, m.ConversationID, m.CorrelationID).Scan(&count); err != nil || count != 1 {
			t.Fatal("duplicate after interrupted worker", err)
		}
		if err := admin.QueryRow(ctx, `SELECT count(*) FROM platform.orders WHERE organization_id=$1`, shops[0].tenant.OrganizationID).Scan(&ordersAfter); err != nil || ordersAfter != ordersBefore {
			t.Fatal("order side effect replay", err)
		}
	})
	t.Run("connection-error-states-are-real-backend-state", func(t *testing.T) {
		assertStatus := func(want string) {
			t.Helper()
			var result struct{ Connection struct{ Status string } }
			if json.Unmarshal(request(0, "GET", root(0)+"/webhook-status", 200), &result) != nil || result.Connection.Status != want {
				t.Fatal("wrong connection status", result.Connection.Status, want)
			}
		}
		assertStatus("connected")
		if _, err := admin.Exec(ctx, `UPDATE platform.instagram_connections SET permissions='{}' WHERE integration_id=$1`, shops[0].id); err != nil {
			t.Fatal(err)
		}
		assertStatus("permission_error")
		if _, err := admin.Exec(ctx, `UPDATE platform.instagram_connections SET permissions=ARRAY['instagram_business_basic','instagram_business_manage_messages'],webhook_subscribed=false WHERE integration_id=$1`, shops[0].id); err != nil {
			t.Fatal(err)
		}
		assertStatus("webhook_error")
		if _, err := admin.Exec(ctx, `UPDATE platform.instagram_connections SET webhook_subscribed=true WHERE integration_id=$1`, shops[0].id); err != nil {
			t.Fatal(err)
		}
		assertStatus("connected")
		cancelled, err := svc.Start(ctx, shops[2].tenant)
		if err != nil {
			t.Fatal(err)
		}
		cancelURL, _ := url.Parse(cancelled["callback_url"].(string))
		q := cancelURL.Query()
		q.Set("error", "access_denied")
		if err = svc.Callback(ctx, shops[2].tenant, q); err == nil {
			t.Fatal("denied consent accepted")
		}
		var result struct{ Connection struct{ Status string } }
		raw, err := svc.Status(ctx, shops[2].tenant)
		if err != nil || json.Unmarshal(raw, &result) != nil || result.Connection.Status != "reauthorization_required" {
			t.Fatal("authorization failure not durable", err)
		}
	})
	t.Run("expired-token-fences-durable-jobs", func(t *testing.T) {
		raw := payload(1, "ig-stage2-expiry", false)
		if _, err := svc.Ingress(ctx, shops[1].id, sign(raw), raw); err != nil {
			t.Fatal(err)
		}
		if _, err := admin.Exec(ctx, `UPDATE platform.instagram_connections SET token_expires_at=now()-interval '1 second' WHERE integration_id=$1`, shops[1].id); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Ingress(ctx, shops[1].id, sign(raw), raw); err == nil {
			t.Fatal("expired token ingress")
		}
		if worked, err := svc.ProcessOne(ctx); err != nil || worked {
			t.Fatal("expired job executed", err)
		}
		var status string
		if err := admin.QueryRow(ctx, `SELECT status FROM platform.channel_inbox WHERE integration_id=$1 AND external_id='message:ig-stage2-expiry'`, shops[1].id).Scan(&status); err != nil || status != "cancelled" {
			t.Fatal("expired job remained pending", err)
		}
		response := request(1, "GET", root(1), 200)
		if !bytes.Contains(response, []byte("reauthorization_required")) {
			t.Fatal("expiry status missing")
		}
	})
	t.Run("state-concurrency-and-revocation", func(t *testing.T) {
		expiring, err := svc.Start(ctx, shops[2].tenant)
		if err != nil {
			t.Fatal(err)
		}
		expiredURL, _ := url.Parse(expiring["callback_url"].(string))
		if _, err = admin.Exec(ctx, `UPDATE platform.instagram_oauth_states SET created_at=now()-interval '11 minutes',expires_at=now()-interval '1 minute' WHERE integration_id=$1 AND consumed_at IS NULL`, shops[2].id); err != nil {
			t.Fatal(err)
		}
		if err = svc.Callback(ctx, shops[2].tenant, expiredURL.Query()); err == nil {
			t.Fatal("expired durable state accepted")
		}
		started, err := svc.Start(ctx, shops[2].tenant)
		if err != nil {
			t.Fatal(err)
		}
		callback, _ := url.Parse(started["callback_url"].(string))
		var wg sync.WaitGroup
		results := make(chan error, 2)
		for range 2 {
			wg.Add(1)
			go func() { defer wg.Done(); results <- svc.Callback(ctx, shops[2].tenant, callback.Query()) }()
		}
		wg.Wait()
		close(results)
		success := 0
		for err := range results {
			if err == nil {
				success++
			}
		}
		if success != 1 {
			t.Fatal("state not consumed exactly once")
		}
		// A previously persisted event cannot prove a new connection generation.
		restarted, err := svc.Start(ctx, shops[0].tenant)
		if err != nil {
			t.Fatal(err)
		}
		restartedURL, _ := url.Parse(restarted["callback_url"].(string))
		if err = svc.Callback(ctx, shops[0].tenant, restartedURL.Query()); err != nil {
			t.Fatal(err)
		}
		duplicate := payload(0, "ig-stage2-first", false)
		if n, err := svc.Ingress(ctx, shops[0].id, sign(duplicate), duplicate); err != nil || n != 0 {
			t.Fatal("duplicate after reconnect", err)
		}
		var state struct{ Connection struct{ Status string } }
		response, err := svc.Status(ctx, shops[0].tenant)
		if err != nil || json.Unmarshal(response, &state) != nil || state.Connection.Status != "connecting" {
			t.Fatal("replay falsely verified subscription", err)
		}
		request(0, "POST", root(0)+"/disconnect", 200)
		raw := payload(0, "ig-stage2-after-revoke", false)
		if _, err := svc.Ingress(ctx, shops[0].id, sign(raw), raw); err == nil {
			t.Fatal("revoked ingress")
		}
		var size int
		if err := admin.QueryRow(ctx, `SELECT octet_length(secret_cipher) FROM platform.channel_integrations WHERE id=$1`, shops[0].id).Scan(&size); err != nil || size != 0 {
			t.Fatal("revoked token retained")
		}
	})
}
