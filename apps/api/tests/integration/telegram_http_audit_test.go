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
	httpapi "github.com/karadjev/AI-Business-Assistant/apps/api/internal/http"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/db"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/telegram"
)

type auditRoundTripper func(*http.Request) (*http.Response, error)

func (f auditRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Runs the REAL adapter + inbox/processor/agent/orders/outbox against a loopback
// HTTP Telegram emulator. No function-return fake substitutes for HTTP semantics.
func testTelegramHTTPAudit(t *testing.T, d *db.Database, admin *pgx.Conn, ctx context.Context, tenant catalog.Tenant) {
	t.Helper()
	var sent, next atomic.Int64
	wire := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Error("unexpected method")
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/getMe"):
			_, _ = io.WriteString(w, `{"ok":true,"result":{"id":99001,"is_bot":true,"first_name":"Synthetic","username":"synthetic_bot"}}`)
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			sent.Add(1)
			var payload map[string]any
			if e := json.NewDecoder(r.Body).Decode(&payload); e != nil {
				t.Error(e)
			}
			if payload["chat_id"] != float64(99002) || payload["parse_mode"] != nil {
				t.Error("wrong HTTP destination/format")
			}
			code := next.Swap(0)
			if code == 429 {
				w.WriteHeader(429)
				_, _ = io.WriteString(w, `{"ok":false,"error_code":429,"parameters":{"retry_after":1}}`)
				return
			}
			if code == 500 {
				w.WriteHeader(500)
				_, _ = io.WriteString(w, `{"ok":false,"error_code":500}`)
				return
			}
			_, _ = io.WriteString(w, `{"ok":true,"result":{"message_id":123}}`)
		default:
			t.Error("unexpected Telegram operation")
			w.WriteHeader(400)
		}
	}))
	defer wire.Close()
	local, _ := url.Parse(wire.URL)
	direct := &http.Transport{Proxy: nil}
	defer direct.CloseIdleConnections()
	previous := http.DefaultTransport
	http.DefaultTransport = auditRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.telegram.org" {
			return nil, errors.New("unexpected external destination blocked by offline fixture")
		}
		clone := r.Clone(r.Context())
		u := *r.URL
		u.Scheme = local.Scheme
		u.Host = local.Host
		clone.URL = &u
		clone.Host = ""
		return direct.RoundTrip(clone)
	})
	defer func() { http.DefaultTransport = previous }()
	cfg, _ := config.LoadAgent(func(string) string { return "" })
	cs := conversations.Service{Pool: d.Pool, Config: cfg, Provider: agent.Mock{}}
	tc := config.Telegram{Mode: "mock", Enabled: true, Key: bytes.Repeat([]byte{45}, 32), Retention: 7 * 24 * time.Hour, AllowedUsers: map[int64]bool{99002: true}}
	tg := telegram.Service{Pool: d.Pool, Config: tc, Transport: telegram.NewHTTPTransport(), Chat: &cs}
	if _, e := tg.Connect(ctx, tenant, "99001:"+strings.Repeat("a", 24)); e != nil {
		t.Fatal("local HTTP getMe", e)
	}
	cat := catalog.Service{Pool: d.Pool}
	raw, e := cat.CreateProduct(ctx, tenant, catalog.Input{Name: "Synthetic HTTP8A", SKU: "HTTP8A", PriceAmount: 10000})
	if e != nil {
		t.Fatal(e)
	}
	var p struct {
		ID       string `json:"id"`
		Variants []struct {
			ID string `json:"id"`
		} `json:"variants"`
	}
	_ = json.Unmarshal(raw, &p)
	variant := p.Variants[0].ID
	if _, e = cat.UpdateProduct(ctx, tenant, p.ID, catalog.Input{ExpectedVersion: 1}, "publish"); e != nil {
		t.Fatal(e)
	}
	if _, e = cat.Adjust(ctx, tenant, catalog.Input{VariantID: variant, ExpectedVersion: 1, QuantityOnHand: 2, Reason: "synthetic HTTP contract"}); e != nil {
		t.Fatal(e)
	}
	var integration string
	if e = admin.QueryRow(ctx, `SELECT id FROM platform.telegram_integrations WHERE organization_id=$1`, tenant.OrganizationID).Scan(&integration); e != nil {
		t.Fatal(e)
	}
	secret := auth.Token()
	if _, e = admin.Exec(ctx, `UPDATE platform.telegram_integrations SET webhook_hash=$2 WHERE id=$1`, integration, auth.Hash(secret)); e != nil {
		t.Fatal(e)
	}
	handler := httpapi.NewHandlerWithAgent(d, slog.New(slog.NewJSONHandler(io.Discard, nil)), 5*time.Second, 65536, auth.New(d.Pool, config.Auth{}), cfg, agent.Mock{}, tc)
	hook := func(u telegram.Update, header string) int {
		b, _ := json.Marshal(u)
		r := httptest.NewRequest("POST", "/api/v1/webhooks/telegram/"+integration, bytes.NewReader(b))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Telegram-Bot-Api-Secret-Token", header)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w.Code
	}
	update := telegram.Update{ID: 1, Message: &telegram.Message{From: telegram.User{ID: 99002}, Chat: telegram.Chat{ID: 99002, Type: "private"}, Text: "Есть HTTP8A?"}}
	if code := hook(update, auth.Token()); code != 403 {
		t.Fatal("wrong secret", code)
	}
	denied := update
	copyMessage := *update.Message
	copyMessage.From.ID = 99003
	copyMessage.Chat.ID = 99003
	denied.Message = &copyMessage
	if code := hook(denied, secret); code != 403 {
		t.Fatal("allowlist", code)
	}
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if code := hook(update, secret); code != 200 {
				t.Error("concurrent durable webhook", code)
			}
		}()
	}
	wg.Wait()
	if _, e = tg.ProcessOne(ctx); e != nil {
		t.Fatal(e)
	}
	next.Store(429)
	if _, e = tg.SendOne(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e = admin.Exec(ctx, `UPDATE platform.telegram_outbox SET available_at=now() WHERE organization_id=$1`, tenant.OrganizationID); e != nil {
		t.Fatal(e)
	}
	if _, e = tg.SendOne(ctx); e != nil || sent.Load() != 2 {
		t.Fatal("HTTP429/backoff", e, sent.Load())
	}
	callback := func(id int64, data string) telegram.Update {
		return telegram.Update{ID: id, Callback: &telegram.Callback{ID: "synthetic", From: telegram.User{ID: 99002}, Message: telegram.Message{Chat: telegram.Chat{ID: 99002, Type: "private"}}, Data: data}}
	}
	push := func(u telegram.Update) {
		if code := hook(u, secret); code != 200 {
			t.Fatal("callback webhook", code)
		}
		if _, e = tg.ProcessOne(ctx); e != nil {
			t.Fatal(e)
		}
		if _, e = tg.SendOne(ctx); e != nil {
			t.Fatal(e)
		}
	}
	push(callback(2, "offer:"+variant))
	var conv, quote string
	if e = admin.QueryRow(ctx, `SELECT conversation_id FROM platform.telegram_principals WHERE organization_id=$1`, tenant.OrganizationID).Scan(&conv); e != nil {
		t.Fatal(e)
	}
	if e = admin.QueryRow(ctx, `SELECT id FROM platform.order_quotes WHERE conversation_id=$1`, conv).Scan(&quote); e != nil {
		t.Fatal(e)
	}
	push(callback(3, "confirm:"+quote))
	push(callback(3, "confirm:"+quote))
	var orders, reserved int
	if e = admin.QueryRow(ctx, `SELECT count(*) FROM platform.orders WHERE conversation_id=$1`, conv).Scan(&orders); e != nil || orders != 1 {
		t.Fatal("explicit HTTP flow order dedup", e, orders)
	}
	if e = admin.QueryRow(ctx, `SELECT quantity_reserved FROM platform.inventory WHERE variant_id=$1`, variant).Scan(&reserved); e != nil || reserved != 0 {
		t.Fatal("premature reserve", e)
	}
	if _, e = cs.Control(ctx, tenant, conv, "human", 0); e != nil {
		t.Fatal(e)
	}
	if _, e = cs.Send(ctx, tenant, conv, conversations.SendInput{Text: "Synthetic staff HTTP", Role: "staff", IdempotencyKey: auth.UUID()}); e != nil {
		t.Fatal(e)
	}
	next.Store(500)
	if _, e = tg.SendOne(ctx); e != nil {
		t.Fatal(e)
	}
	before := sent.Load()
	restarted := tg
	if claimed, e := restarted.SendOne(ctx); e != nil || claimed || sent.Load() != before {
		t.Fatal("uncertain HTTP delivery replayed after restart", e)
	}
	if _, e = tg.Disconnect(ctx, tenant); e != nil {
		t.Fatal(e)
	}
	update.ID = 4
	if code := hook(update, secret); code != 403 {
		t.Fatal("disabled integration still accepted webhook", code)
	}
	t.Log("VERIFIED: real Telegram HTTP adapter against loopback emulator: getMe, webhook secret/allowlist/concurrent dedup, catalog,429,retry,quote/button/order,no premature reserve,staff,500 uncertain/restart,disconnect")
}
