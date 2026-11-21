//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
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
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/agent"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/conversations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/costcontrol"
	httpapi "github.com/karadjev/AI-Business-Assistant/apps/api/internal/http"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/db"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/telegram"
)

type recoveryFixture struct {
	catalog.Tenant
	token, csrf string
}

func recoveryConfig() config.Agent {
	c, _ := config.LoadAgent(func(string) string { return "" })
	c.Provider = "openai_compatible"
	c.LiveEnabled = true
	c.Model = "synthetic-recovery"
	c.Live = config.Live{ExternalAuthorized: true, BaseURL: "https://provider.example/v1", AllowedBases: []string{"https://provider.example/v1"}, InputRate: 1000000, OutputRate: 1000000, PlatformMicroRUB: 1000000, Parallel: 16, DialogInput: 20000, DialogOutput: 5000, DialogRequests: 30}
	return c
}
func recoveryTelegramConfig() config.Telegram {
	return config.Telegram{Mode: "mock", Enabled: true, Key: bytes.Repeat([]byte{83}, 32), Retention: 7 * 24 * time.Hour}
}

// Actual Telegram wire adapter, with every host except the in-process emulator
// forbidden. This is test-only injection; no configured public endpoint is used.
func redirectTelegramTo(t *testing.T, endpoint string) func() {
	t.Helper()
	u, e := url.Parse(endpoint)
	if e != nil || u.Hostname() != "127.0.0.1" {
		t.Fatal("non-loopback fixture")
	}
	prior := http.DefaultTransport
	wire := &http.Transport{Proxy: nil}
	http.DefaultTransport = auditRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.telegram.org" {
			return nil, errors.New("unexpected HTTP destination")
		}
		copy := r.Clone(r.Context())
		v := *r.URL
		v.Scheme = u.Scheme
		v.Host = u.Host
		copy.URL = &v
		copy.Host = u.Host
		return wire.RoundTrip(copy)
	})
	return func() { http.DefaultTransport = prior; wire.CloseIdleConnections() }
}
func TestRecoveryCrashHelper(t *testing.T) {
	mode := os.Getenv("ABA_RECOVERY_CRASH")
	if mode == "" {
		t.Skip("subprocess helper only")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	d, e := db.Open(ctx, testDSN(t, "ABA_TEST_DATABASE_URL", "aba_app"), 2, time.Second)
	if e != nil {
		t.Fatal(e)
	}
	defer d.Close()
	if mode == "recover_once" {
		if _, e := (costcontrol.Service{Pool: d.Pool}).RecoverStale(ctx); e != nil {
			t.Fatal(e)
		}
		return
	}
	if mode == "telegram" || mode == "resume_telegram" {
		restore := redirectTelegramTo(t, os.Getenv("ABA_RECOVERY_WIRE"))
		defer restore()
		service := telegram.Service{Pool: d.Pool, Config: recoveryTelegramConfig(), Transport: telegram.NewHTTPTransport()}
		if mode == "telegram" {
			service.Transport = crashAfterReceipt{Inner: telegram.NewHTTPTransport()}
		}
		claimed, err := service.SendOne(ctx)
		if mode == "resume_telegram" {
			if err != nil || claimed {
				t.Fatal("restart attempted unknown resend", err)
			}
			return
		}
		e = err
		if e != nil {
			t.Fatal(e)
		}
		t.Fatal("crash hook not reached")
	}
	identity := auth.New(d.Pool, config.Auth{SessionTTL: time.Hour, IdleTTL: time.Hour})
	session, e := identity.Authenticate(ctx, os.Getenv("ABA_RECOVERY_SESSION"))
	if e != nil {
		t.Fatal(e)
	}
	tenant := catalog.Tenant{Session: session, OrganizationID: os.Getenv("ABA_RECOVERY_ORG")}
	ledger := costcontrol.Service{Pool: d.Pool, Config: recoveryConfig()}
	if _, e = ledger.Reserve(ctx, tenant, os.Getenv("ABA_RECOVERY_CONVERSATION"), 10, 10, 1); e != nil {
		t.Fatal(e)
	}
	if mode == "after_response" {
		u, e := url.Parse(os.Getenv("ABA_RECOVERY_WIRE"))
		if e != nil || u.Hostname() != "127.0.0.1" {
			t.Fatal("invalid fixture")
		}
		provider := agent.Compatible{Endpoint: u.String(), Client: &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: time.Second}}
		if _, e = provider.Generate(ctx, agent.Request{InputTokens: 1000, OutputTokens: 100}); e != nil {
			t.Fatal(e)
		}
	}
	// Real process termination: no deferred settle/connection cleanup runs.
	os.Exit(87)
}

type crashAfterReceipt struct{ Inner telegram.Transport }

func (c crashAfterReceipt) GetMe(ctx context.Context, token string) (telegram.Bot, error) {
	return c.Inner.GetMe(ctx, token)
}
func (c crashAfterReceipt) Send(ctx context.Context, token string, id int64, p json.RawMessage) (telegram.SendResult, error) {
	result, e := c.Inner.Send(ctx, token, id, p)
	if e == nil {
		os.Exit(86)
	}
	return result, e
}
func runCrash(t *testing.T, mode, wire string, f recoveryFixture, conv string, code int) {
	t.Helper()
	command := exec.CommandContext(context.Background(), os.Args[0], "-test.run=^TestRecoveryCrashHelper$", "-test.timeout=15s")
	command.Env = append(os.Environ(), "ABA_RECOVERY_CRASH="+mode, "ABA_RECOVERY_WIRE="+wire, "ABA_RECOVERY_SESSION="+f.token, "ABA_RECOVERY_ORG="+f.OrganizationID, "ABA_RECOVERY_CONVERSATION="+conv)
	output, e := command.CombinedOutput()
	if code == 0 && e == nil {
		return
	}
	var exit *exec.ExitError
	if !errors.As(e, &exit) || exit.ExitCode() != code {
		t.Fatalf("synthetic process crash not reached: %v; %s", e, output)
	}
}
func testReliability(t *testing.T, d *db.Database, admin *pgx.Conn, ctx context.Context) {
	t.Helper()
	var schema bool
	if e := admin.QueryRow(ctx, `SELECT to_regprocedure('platform.telegram_resolve_delivery(uuid,uuid,bigint,text,uuid)') IS NOT NULL AND to_regprocedure('platform.llm_recover_stale()') IS NOT NULL`).Scan(&schema); e != nil || !schema {
		t.Fatal("repro A09/A10: durable recovery operations absent", e)
	}
	identity := auth.New(d.Pool, config.Auth{Origin: "http://web:3000", SessionTTL: time.Hour, IdleTTL: time.Hour})
	owner := func() recoveryFixture {
		uid, token, csrf := auth.UUID(), auth.Token(), auth.Token()
		if _, e := admin.Exec(ctx, `INSERT INTO platform.users(id,email,password_hash,display_name,email_verified_at) VALUES($1,$2,'synthetic','Recovery',now())`, uid, uid+"@recovery.example.test"); e != nil {
			t.Fatal(e)
		}
		if _, e := admin.Exec(ctx, `INSERT INTO platform.sessions(id,user_id,token_hash,csrf_hash,expires_at,idle_expires_at) VALUES($1,$2,$3,$4,now()+interval '1 hour',now()+interval '1 hour')`, auth.UUID(), uid, auth.Hash(token), auth.Hash(csrf)); e != nil {
			t.Fatal(e)
		}
		session, e := identity.Authenticate(ctx, token)
		if e != nil {
			t.Fatal(e)
		}
		org, e := (organizations.Service{Pool: d.Pool}).CreateTest(ctx, session, "Synthetic recovery")
		if e != nil {
			t.Fatal(e)
		}
		return recoveryFixture{catalog.Tenant{Session: session, OrganizationID: org.ID}, token, csrf}
	}
	a, b := owner(), owner()
	cfg, _ := config.LoadAgent(func(string) string { return "" })
	chat := conversations.Service{Pool: d.Pool, Config: cfg, Provider: agent.Mock{}}
	var sends atomic.Int64
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var block atomic.Bool
	wire := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Error("wire method")
		}
		if strings.HasSuffix(r.URL.Path, "/getMe") {
			io.WriteString(w, `{"ok":true,"result":{"id":88301,"is_bot":true,"first_name":"Synthetic","username":"recovery_bot"}}`)
			return
		}
		if !strings.HasSuffix(r.URL.Path, "/sendMessage") {
			t.Error("wire route")
			w.WriteHeader(400)
			return
		}
		var body map[string]any
		if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
			t.Error(e)
		}
		if body["chat_id"] != float64(88302) {
			t.Error("wire principal")
		}
		sends.Add(1)
		if block.Load() {
			entered <- struct{}{}
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		io.WriteString(w, `{"ok":true,"result":{"message_id":123}}`)
	}))
	defer wire.Close()
	restore := redirectTelegramTo(t, wire.URL)
	defer restore()
	tg := telegram.Service{Pool: d.Pool, Config: recoveryTelegramConfig(), Transport: telegram.NewHTTPTransport(), Chat: &chat}
	// Connect through the actual adapter to the local emulator using a fake token.
	if _, e := tg.Connect(ctx, a.Tenant, "88301:"+strings.Repeat("a", 30)); e != nil {
		t.Fatal(e)
	}
	update := telegram.Update{ID: 1, Message: &telegram.Message{From: telegram.User{ID: 88302}, Chat: telegram.Chat{ID: 88302, Type: "private"}, Text: "synthetic"}}
	if _, e := tg.Inject(ctx, a.Tenant, update); e != nil {
		t.Fatal(e)
	}
	if _, e := tg.ProcessOne(ctx); e != nil {
		t.Fatal(e)
	}
	var conv string
	if e := admin.QueryRow(ctx, `SELECT conversation_id FROM platform.telegram_principals WHERE organization_id=$1`, a.OrganizationID).Scan(&conv); e != nil {
		t.Fatal(e)
	}
	handler := httpapi.NewHandlerWithAgent(d, slog.New(slog.NewJSONHandler(io.Discard, nil)), 5*time.Second, 65536, identity, cfg, agent.Mock{}, recoveryTelegramConfig())
	api := func(f recoveryFixture, method, path string, input any, csrf bool) int {
		raw, _ := json.Marshal(input)
		r := httptest.NewRequest(method, "/api/v1/organizations/"+f.OrganizationID+path, bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", "http://web:3000")
		r.AddCookie(&http.Cookie{Name: "aba_session", Value: f.token})
		r.AddCookie(&http.Cookie{Name: "aba_csrf", Value: f.csrf})
		if csrf {
			r.Header.Set("X-CSRF-Token", f.csrf)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w.Code
	}
	version := func(id string) int64 {
		var n int64
		if e := admin.QueryRow(ctx, `SELECT version FROM platform.telegram_outbox WHERE id=$1`, id).Scan(&n); e != nil {
			t.Fatal(e)
		}
		return n
	}
	var outbox, fencedCap string
	if e := admin.QueryRow(ctx, `SELECT id FROM platform.telegram_outbox WHERE organization_id=$1`, a.OrganizationID).Scan(&outbox); e != nil {
		t.Fatal(e)
	}
	t.Run("process_dies_after_telegram_receipt_no_resend", func(t *testing.T) {
		runCrash(t, "telegram", wire.URL, a, "", 86)
		var state string
		admin.QueryRow(ctx, `SELECT status FROM platform.telegram_outbox WHERE id=$1`, outbox).Scan(&state)
		if e := admin.QueryRow(ctx, `SELECT capability::text FROM platform.telegram_outbox WHERE id=$1`, outbox).Scan(&fencedCap); e != nil {
			t.Fatal(e)
		}
		if state != "sending" || sends.Load() != 1 {
			t.Fatal("crash window not reached", state, sends.Load())
		}
		if _, e := admin.Exec(ctx, `UPDATE platform.telegram_outbox SET lease_until=now()-interval '1 second' WHERE id=$1`, outbox); e != nil {
			t.Fatal(e)
		}
		runCrash(t, "resume_telegram", wire.URL, a, "", 0)
		if sends.Load() != 1 {
			t.Fatal("new worker process replayed unknown send")
		}
		if e := admin.QueryRow(ctx, `SELECT status FROM platform.telegram_outbox WHERE id=$1`, outbox).Scan(&state); e != nil || state != "uncertain" {
			t.Fatal("missing DELIVERY_UNKNOWN", e, state)
		}
	})
	t.Run("manual_delivery_resolution_auth_csrf_idempotency_fencing", func(t *testing.T) {
		// A backlog of permanent failures must not hide actionable UNKNOWN rows.
		if _, e := admin.Exec(ctx, `INSERT INTO platform.telegram_outbox(id,organization_id,integration_id,conversation_id,chat_id,message_key,payload,status,attempts,created_at) SELECT gen_random_uuid(),organization_id,integration_id,conversation_id,chat_id,'synthetic-failure-'||n,'{}'::jsonb,'failed',5,now()-interval '1 hour' FROM platform.telegram_outbox CROSS JOIN generate_series(1,51) n WHERE id=$1`, outbox); e != nil {
			t.Fatal(e)
		}
		incident, e := tg.Deliveries(ctx, a.Tenant)
		if e != nil {
			t.Fatal(e)
		}
		var page struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
		}
		if e = json.Unmarshal(incident, &page); e != nil || len(page.Items) == 0 || page.Items[0].ID != outbox {
			t.Fatal("failed backlog hides actionable UNKNOWN", e)
		}

		in := telegram.DeliveryResolution{ExpectedVersion: version(outbox), Decision: "abandon", IdempotencyKey: auth.UUID(), Confirm: true}
		if _, e := tg.ResolveDelivery(ctx, b.Tenant, outbox, in); e == nil {
			t.Fatal("foreign resolution")
		}
		if code := api(a, "POST", "/integrations/telegram/deliveries/"+outbox+"/resolve", in, false); code != 403 {
			t.Fatal("CSRF", code)
		}
		if _, e := tg.ResolveDelivery(ctx, a.Tenant, outbox, telegram.DeliveryResolution{ExpectedVersion: in.ExpectedVersion, Decision: "resend", IdempotencyKey: auth.UUID(), Confirm: true}); e == nil {
			t.Fatal("unsafe resend accepted")
		}
		var resolving sync.WaitGroup
		for i := 0; i < 2; i++ {
			resolving.Add(1)
			go func() {
				defer resolving.Done()
				if code := api(a, "POST", "/integrations/telegram/deliveries/"+outbox+"/resolve", in, true); code != 200 {
					t.Error("concurrent resolution retry", code)
				}
			}()
		}
		resolving.Wait()
		in.Decision = "delivered"
		if code := api(a, "POST", "/integrations/telegram/deliveries/"+outbox+"/resolve", in, true); code != 409 {
			t.Fatal("changed decision must conflict", code)
		}
		anon := a
		anon.token = ""
		if code := api(anon, "GET", "/integrations/telegram/deliveries", nil, true); code != 401 {
			t.Fatal("anonymous recovery data", code)
		}
		if _, e := tg.ResolveDelivery(ctx, a.Tenant, outbox, in); e == nil {
			t.Fatal("key changed meaning")
		}
		var n int
		if e := admin.QueryRow(ctx, `SELECT count(*) FROM platform.telegram_delivery_events WHERE outbox_id=$1 AND request_key IS NOT NULL`, outbox).Scan(&n); e != nil || n != 1 {
			t.Fatal("duplicate operator audit", e, n)
		}
		resolvedVersion := version(outbox)
		if _, e := d.Pool.Exec(ctx, `SELECT platform.telegram_finish_send($1,'pending',0,0)`, fencedCap); e != nil || version(outbox) != resolvedVersion {
			t.Fatal("stale worker overwrote operator", e)
		}
		if sends.Load() != 1 {
			t.Fatal("resolution performed external send")
		}
		if code := api(a, "GET", "/integrations/telegram/deliveries", nil, true); code != 200 {
			t.Fatal("delivery read", code)
		}
	})
	t.Run("two_workers_one_actual_send", func(t *testing.T) {
		if _, e := chat.Control(ctx, a.Tenant, conv, "human", 0); e != nil {
			t.Fatal(e)
		}
		if _, e := chat.Send(ctx, a.Tenant, conv, conversations.SendInput{Role: "staff", Text: "Concurrent synthetic staff", IdempotencyKey: auth.UUID()}); e != nil {
			t.Fatal(e)
		}
		block.Store(true)
		done := make(chan error, 1)
		go func() { _, e := tg.SendOne(ctx); done <- e }()
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("no actual HTTP attempt")
		}
		if claimed, e := tg.SendOne(ctx); e != nil || claimed {
			t.Error("second worker claimed same send", e)
		}
		close(release)
		if e := <-done; e != nil {
			t.Fatal(e)
		}
		block.Store(false)
		if sends.Load() != 2 {
			t.Fatal("duplicate concurrent delivery", sends.Load())
		}
		var started, finished int
		if e := admin.QueryRow(ctx, `SELECT count(*) FILTER(WHERE new_status='sending'),count(*) FILTER(WHERE new_status='sent') FROM platform.telegram_delivery_events WHERE organization_id=$1 AND outbox_id<>$2`, a.OrganizationID, outbox).Scan(&started, &finished); e != nil || started != 1 || finished != 1 {
			t.Fatal("attempt audit incomplete", e, started, finished)
		}
	})
	t.Run("member_cannot_resolve_and_disconnect_cancels_queue", func(t *testing.T) {
		if _, e := admin.Exec(ctx, `INSERT INTO platform.organization_members(organization_id,user_id,role,can_manage_orders) VALUES($1,$2,'member',false)`, a.OrganizationID, b.Session.User.ID); e != nil {
			t.Fatal(e)
		}
		member := b.Tenant
		member.OrganizationID = a.OrganizationID
		if _, e := tg.Deliveries(ctx, member); e == nil {
			t.Fatal("member without staff read")
		}
		if _, e := chat.Send(ctx, a.Tenant, conv, conversations.SendInput{Role: "staff", Text: "Cancelled synthetic", IdempotencyKey: auth.UUID()}); e != nil {
			t.Fatal(e)
		}
		if _, e := tg.Disconnect(ctx, a.Tenant); e != nil {
			t.Fatal(e)
		}
		if claimed, e := tg.SendOne(ctx); e != nil || claimed {
			t.Fatal("disconnected queue sent", e)
		}
	})
	// A tiny simulated ledger is approved ONLY by migration role in isolated aba_test.
	ledger := costcontrol.Service{Pool: d.Pool, Config: recoveryConfig()}
	if _, e := admin.Exec(ctx, `UPDATE platform.llm_platform_budget SET limit_microrub=1000000,kill_switch=false WHERE singleton`); e != nil {
		t.Fatal(e)
	}
	defer admin.Exec(ctx, `UPDATE platform.llm_platform_budget SET limit_microrub=0,kill_switch=true WHERE singleton`)
	if _, e := ledger.Update(ctx, a.Tenant, costcontrol.Change{Settings: costcontrol.Settings{AutoEnabled: true, LiveEnabled: true, RequestLimit: 10000, DialogLimit: 100000, DayLimit: 100000, MonthLimit: 100000}, ExpectedVersion: 1}); e != nil {
		t.Fatal(e)
	}
	newChat := func() string {
		raw, e := chat.Create(ctx, a.Tenant)
		if e != nil {
			t.Fatal(e)
		}
		var c struct {
			ID string `json:"id"`
		}
		if e = json.Unmarshal(raw, &c); e != nil {
			t.Fatal(e)
		}
		return c.ID
	}
	t.Run("budget_crashes_and_concurrent_recovery_are_conservative_once", func(t *testing.T) {
		var responses atomic.Int64
		provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			responses.Add(1)
			io.WriteString(w, `{"choices":[{"message":{}}],"usage":{"prompt_tokens":10,"completion_tokens":1}}`)
		}))
		defer provider.Close()
		reservations := []string{}
		for _, mode := range []string{"after_reserve", "after_response"} {
			c := newChat()
			runCrash(t, mode, provider.URL, a, c, 87)
			var id string
			if e := admin.QueryRow(ctx, `SELECT id FROM platform.llm_reservations WHERE conversation_id=$1`, c).Scan(&id); e != nil {
				t.Fatal(e)
			}
			reservations = append(reservations, id)
			if e := ledger.Recover(ctx, a.Tenant, id); e == nil {
				t.Fatal("active budget recovered prematurely")
			}
		}
		if responses.Load() != 1 {
			t.Fatal("provider crash stages not distinguished", responses.Load())
		}
		var beforeSpent, beforeReserved int64
		var beforeCalls int
		if e := admin.QueryRow(ctx, `SELECT spent_microrub,reserved_microrub,active_calls FROM platform.llm_platform_budget WHERE singleton`).Scan(&beforeSpent, &beforeReserved, &beforeCalls); e != nil {
			t.Fatal(e)
		}
		if _, e := admin.Exec(ctx, `UPDATE platform.llm_reservations SET created_at=now()-interval '6 minutes' WHERE id=ANY($1::uuid[])`, reservations); e != nil {
			t.Fatal(e)
		}
		var wg sync.WaitGroup
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				runCrash(t, "recover_once", "", a, "", 0)
			}()
		}
		wg.Wait()
		for _, id := range reservations {
			if e := ledger.Settle(ctx, a.Tenant, id, 0, 0, true); e != nil {
				t.Fatal(e)
			}
			if e := ledger.Recover(ctx, a.Tenant, id); e != nil {
				t.Fatal(e)
			}
			var status string
			var charged int64
			var n int
			if e := admin.QueryRow(ctx, `SELECT r.status,r.charged,(SELECT count(*) FROM platform.llm_budget_events e WHERE e.reservation_id=r.id) FROM platform.llm_reservations r WHERE r.id=$1`, id).Scan(&status, &charged, &n); e != nil || status != "uncertain" || charged != 20 || n != 2 {
				t.Fatal("double charge/release or missing UNKNOWN audit", e, status, charged, n)
			}
			if e := ledger.Recover(ctx, b.Tenant, id); e == nil {
				t.Fatal("foreign budget recovery")
			}
		}
		var spent, reserved int64
		var calls int
		if e := admin.QueryRow(ctx, `SELECT spent_microrub,reserved_microrub,active_calls FROM platform.llm_platform_budget WHERE singleton`).Scan(&spent, &reserved, &calls); e != nil || spent-beforeSpent != 40 || beforeReserved-reserved != 40 || beforeCalls-calls != 2 {
			t.Fatal("ledger recovery not conserved", e, spent, reserved, calls)
		}
		if _, e := d.Pool.Exec(ctx, `SELECT platform.llm_settle_core($1,$2,0,0,true)`, a.OrganizationID, reservations[0]); e == nil {
			t.Fatal("private settlement core exposed")
		}
	})
	t.Run("known_usage_settlement_idempotent_and_owner_recovery_only", func(t *testing.T) {
		c := newChat()
		id, e := ledger.Reserve(ctx, a.Tenant, c, 10, 10, 1)
		if e != nil {
			t.Fatal(e)
		}
		var wg sync.WaitGroup
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if e := ledger.Settle(ctx, a.Tenant, id, 10, 1, true); e != nil {
					t.Error(e)
				}
			}()
		}
		wg.Wait()
		var charged int64
		var events int
		if e := admin.QueryRow(ctx, `SELECT charged,(SELECT count(*) FROM platform.llm_budget_events WHERE reservation_id=$1) FROM platform.llm_reservations WHERE id=$1`, id).Scan(&charged, &events); e != nil || charged != 11 || events != 2 {
			t.Fatal("known settlement doubled", e, charged, events)
		}
		member := b.Tenant
		member.OrganizationID = a.OrganizationID
		if _, e := admin.Exec(ctx, `UPDATE platform.organization_members SET can_manage_orders=true WHERE organization_id=$1 AND user_id=$2`, a.OrganizationID, b.Session.User.ID); e != nil {
			t.Fatal(e)
		}
		if e := ledger.Recover(ctx, member, id); e == nil {
			t.Fatal("staff performed owner financial action")
		}
		if code := api(a, "GET", "/llm-reservations", nil, true); code != 200 {
			t.Fatal("ledger API", code)
		}
		if code := api(a, "POST", "/llm-reservations/"+id+"/recover", map[string]bool{"confirm_conservative_charge": true}, true); code != 200 {
			t.Fatal("owner recovery API", code)
		}
	})

	t.Run("timeout_is_unknown_not_refund", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
			case <-time.After(100 * time.Millisecond):
			}
		}))
		defer server.Close()
		c := newChat()
		p := agent.Compatible{Endpoint: server.URL, Client: &http.Client{Timeout: 20 * time.Millisecond, Transport: &http.Transport{Proxy: nil}}, RequireBudget: true}
		metered := agent.Metered{Provider: p, Ledger: ledger, Tenant: a.Tenant, ConversationID: c}
		if _, e := metered.Generate(ctx, agent.Request{InputTokens: 1000, OutputTokens: 100}); e == nil {
			t.Fatal("timeout reported success")
		}
		var status string
		var charged, amount int64
		if e := admin.QueryRow(ctx, `SELECT status,charged,amount FROM platform.llm_reservations WHERE conversation_id=$1`, c).Scan(&status, &charged, &amount); e != nil || status != "uncertain" || charged != amount || charged != 1100 {
			t.Fatal("timeout refunded unknown cost", e, status, charged, amount)
		}
	})
	t.Log("PASS: actual process exits after Telegram receipt/reserve/provider response; no unknown resend; two workers; manual auth/CSRF/version/idempotency; conservative budget recovery and audit")
}
