//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
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
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/orders"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/db"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/telegram"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/whatsapp"
)

type preliveShop struct {
	owner, staff                                                   catalog.Tenant
	cookie, csrf, telegramID, whatsappID, secret, product, variant string
	bot                                                            int64
	phone                                                          string
	price                                                          int64
}

// Three organizations with both channels, their own owner/staff/catalog/stock.
// HTTP requests are actual provider wire requests redirected only to loopback.
func testPreliveMatrix(t *testing.T, d *db.Database, admin *pgx.Conn, ctx context.Context) {
	t.Helper()
	cfg, _ := config.LoadAgent(func(string) string { return "" })
	chat := &conversations.Service{Pool: d.Pool, Config: cfg, Provider: agent.Mock{}}
	tc := config.Telegram{Enabled: true, Mode: "mock", Key: bytes.Repeat([]byte{71}, 32), Retention: 7 * 24 * time.Hour, AllowedUsers: map[int64]bool{101001: true, 101002: true}}
	wk := bytes.Repeat([]byte{72}, 32)
	var tgSends, waSends atomic.Int64
	wire := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != "POST" {
			t.Error("unexpected wire method")
			w.WriteHeader(400)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/getMe") {
			id, _ := strconv.ParseInt(strings.Split(strings.TrimPrefix(r.URL.Path, "/bot"), ":")[0], 10, 64)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{"id": id, "is_bot": true, "first_name": "Synthetic", "username": fmt.Sprintf("synthetic_%d_bot", id)}})
			return
		}
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Error("bad wire body")
			w.WriteHeader(400)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/sendMessage") {
			n := tgSends.Add(1)
			if body["chat_id"] != float64(101001) && body["chat_id"] != float64(101002) {
				t.Error("TG recipient")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{"message_id": n}})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/messages") {
			n := waSends.Add(1)
			if body["messaging_product"] != "whatsapp" || body["to"] != "101001" && body["to"] != "101002" {
				t.Error("WA recipient")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"messages": []any{map[string]string{"id": fmt.Sprintf("synthetic-wire-%d", n)}}})
			return
		}
		t.Error("unexpected wire operation")
		w.WriteHeader(400)
	}))
	defer wire.Close()
	restore := redirectTelegramTo(t, wire.URL)
	defer restore()
	local, _ := url.Parse(wire.URL)
	direct := &http.Transport{Proxy: nil}
	defer direct.CloseIdleConnections()
	waTransport := whatsapp.HTTPMock{Transport: auditRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "whatsapp.mock.invalid" {
			return nil, fmt.Errorf("unexpected destination blocked")
		}
		c := r.Clone(r.Context())
		u := *r.URL
		u.Scheme = local.Scheme
		u.Host = local.Host
		c.URL = &u
		return direct.RoundTrip(c)
	})}
	tg := telegram.Service{Pool: d.Pool, Config: tc, Chat: chat, Transport: telegram.NewHTTPTransport()}
	wa := whatsapp.Service{Pool: d.Pool, Key: wk, Chat: chat, Transport: waTransport}
	identity := auth.New(d.Pool, config.Auth{Origin: "http://127.0.0.1:3000", SessionTTL: time.Hour, IdleTTL: time.Hour})
	var logs bytes.Buffer
	handler := httpapi.NewHandlerWithChannels(d, slog.New(slog.NewJSONHandler(&logs, nil)), 5*time.Second, 65536, identity, cfg, agent.Mock{}, tc, config.WhatsApp{Key: wk})
	server := httptest.NewServer(handler)
	defer server.Close()
	client := &http.Client{Transport: direct, Timeout: 6 * time.Second}
	user := func(name string) (auth.Session, string, string) {
		uid, token, csrf := auth.UUID(), auth.Token(), auth.Token()
		if _, e := admin.Exec(ctx, `INSERT INTO platform.users(id,email,password_hash,display_name,email_verified_at) VALUES($1,$2,'synthetic',$3,now())`, uid, strings.ToLower(name)+"@prelive.example.test", name); e != nil {
			t.Fatal(e)
		}
		if _, e := admin.Exec(ctx, `INSERT INTO platform.sessions(id,user_id,token_hash,csrf_hash,expires_at,idle_expires_at) VALUES($1,$2,$3,$4,now()+interval '1 hour',now()+interval '1 hour')`, auth.UUID(), uid, auth.Hash(token), auth.Hash(csrf)); e != nil {
			t.Fatal(e)
		}
		s, e := identity.Authenticate(ctx, token)
		if e != nil {
			t.Fatal(e)
		}
		return s, token, csrf
	}
	shops := []preliveShop{}
	creds := whatsapp.Credentials{AccessToken: "mock:prelive-access", AppSecret: "mock:prelive-app-secret", VerifyToken: "mock:prelive-verify-token", AllowedUsers: []string{"101001", "101002"}}
	cat := catalog.Service{Pool: d.Pool}
	orderService := orders.Service{Pool: d.Pool, StockFreshness: cfg.StockFreshness}
	for i, name := range []string{"shop-a-8v", "shop-b-8v", "shop-c-8v"} {
		session, cookie, csrf := user(name)
		org, e := (organizations.Service{Pool: d.Pool}).CreateTest(ctx, session, name)
		if e != nil {
			t.Fatal(e)
		}
		member, _, _ := user(name + "-staff")
		if _, e = admin.Exec(ctx, `INSERT INTO platform.organization_members(organization_id,user_id,role,can_manage_orders) VALUES($1,$2,'member',true)`, org.ID, member.User.ID); e != nil {
			t.Fatal(e)
		}
		shop := preliveShop{owner: catalog.Tenant{Session: session, OrganizationID: org.ID}, staff: catalog.Tenant{Session: member, OrganizationID: org.ID}, cookie: cookie, csrf: csrf, price: int64(i+1)*110000 + 99, bot: 91001 + int64(i), phone: fmt.Sprint(41001 + i), secret: auth.Token()}
		raw, e := cat.CreateProduct(ctx, shop.owner, catalog.Input{Name: name + " synthetic phone", SKU: "PHONE8V", PriceAmount: shop.price})
		if e != nil {
			t.Fatal(e)
		}
		var product struct {
			ID       string `json:"id"`
			Variants []struct {
				ID string `json:"id"`
			} `json:"variants"`
		}
		if json.Unmarshal(raw, &product) != nil || len(product.Variants) != 1 {
			t.Fatal("product")
		}
		shop.product, shop.variant = product.ID, product.Variants[0].ID
		if _, e = cat.UpdateProduct(ctx, shop.owner, shop.product, catalog.Input{ExpectedVersion: 1}, "publish"); e != nil {
			t.Fatal(e)
		}
		if _, e = cat.Adjust(ctx, shop.owner, catalog.Input{VariantID: shop.variant, ExpectedVersion: 1, QuantityOnHand: 1, Reason: "Synthetic pre-live stock"}); e != nil {
			t.Fatal(e)
		}
		if _, e = tg.Connect(ctx, shop.owner, fmt.Sprint(shop.bot)+":"+strings.Repeat("v", 24)); e != nil {
			t.Fatal("TG connect local HTTP", e)
		}
		if e = admin.QueryRow(ctx, `SELECT id FROM platform.telegram_integrations WHERE organization_id=$1`, org.ID).Scan(&shop.telegramID); e != nil {
			t.Fatal(e)
		}
		if _, e = admin.Exec(ctx, `UPDATE platform.telegram_integrations SET webhook_hash=$2 WHERE id=$1`, shop.telegramID, auth.Hash(shop.secret)); e != nil {
			t.Fatal(e)
		}
		if _, e = wa.Connect(ctx, shop.owner, whatsapp.ConnectInput{AppID: "100", BusinessID: "200", PhoneID: shop.phone, GraphVersion: "v0.0", Credentials: creds}); e != nil {
			t.Fatal(e)
		}
		if e = admin.QueryRow(ctx, `SELECT id FROM platform.channel_integrations WHERE organization_id=$1`, org.ID).Scan(&shop.whatsappID); e != nil {
			t.Fatal(e)
		}
		if _, e = wa.Control(ctx, shop.owner, true, true); e != nil {
			t.Fatal(e)
		} // Mock only, live remains impossible.
		shops = append(shops, shop)
	}
	request := func(method, path string, body []byte, headers map[string]string) (int, []byte) {
		t.Helper()
		r, e := http.NewRequestWithContext(ctx, method, server.URL+path, bytes.NewReader(body))
		if e != nil {
			t.Fatal(e)
		}
		for k, v := range headers {
			r.Header.Set(k, v)
		}
		res, e := client.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		b, e := io.ReadAll(io.LimitReader(res.Body, 131072))
		if e != nil {
			t.Fatal(e)
		}
		return res.StatusCode, b
	}
	var sequence int64 = 1
	inbound := func(shop preliveShop, ch string, customer int64, text, action string) []byte {
		t.Helper()
		sequence++
		if ch == "telegram" {
			m := telegram.Message{ID: sequence, From: telegram.User{ID: customer}, Chat: telegram.Chat{ID: customer, Type: "private"}, Text: text}
			u := telegram.Update{ID: sequence, Message: &m}
			if action != "" {
				u.Message = nil
				u.Callback = &telegram.Callback{ID: fmt.Sprint(sequence), From: m.From, Message: m, Data: action}
			}
			raw, _ := json.Marshal(u)
			return raw
		}
		msg := map[string]any{"id": fmt.Sprintf("message-%d", sequence), "from": fmt.Sprint(customer), "timestamp": fmt.Sprint(time.Now().Unix()), "type": "text", "text": map[string]string{"body": text}}
		if action != "" {
			msg["type"] = "interactive"
			msg["interactive"] = map[string]any{"type": "button_reply", "button_reply": map[string]string{"id": action}}
		}
		value := map[string]any{"metadata": map[string]string{"phone_number_id": shop.phone}, "messages": []any{msg}}
		entry := map[string]any{"id": "200", "changes": []any{map[string]any{"field": "messages", "value": value}}}
		raw, _ := json.Marshal(map[string]any{"object": "whatsapp_business_account", "entry": []any{entry}})
		return raw
	}
	receive := func(shop preliveShop, ch string, raw []byte, want int) {
		t.Helper()
		id := shop.telegramID
		headers := map[string]string{"Content-Type": "application/json"}
		if ch == "telegram" {
			headers["X-Telegram-Bot-Api-Secret-Token"] = shop.secret
		} else {
			id = shop.whatsappID
			headers["X-Hub-Signature-256"] = whatsapp.Signature(creds.AppSecret, raw)
		}
		code, _ := request("POST", "/api/v1/webhooks/"+ch+"/"+id, raw, headers)
		if code != want {
			t.Fatalf("%s webhook got %d want %d", ch, code, want)
		}
	}
	drain := func(ch string) {
		t.Helper()
		for i := 0; i < 30; i++ {
			var found bool
			var e error
			if ch == "telegram" {
				found, e = tg.ProcessOne(ctx)
			} else {
				found, e = wa.ProcessOne(ctx)
			}
			if e != nil {
				t.Fatal("processor", ch, e)
			}
			if !found {
				break
			}
			if i == 29 {
				t.Fatal("inbox did not drain")
			}
		}
		for i := 0; i < 50; i++ {
			var found bool
			var e error
			if ch == "telegram" {
				found, e = tg.SendOne(ctx)
			} else {
				found, e = wa.SendOne(ctx)
			}
			if e != nil {
				t.Fatal("sender", ch, e)
			}
			if !found {
				return
			}
		}
		t.Fatal("outbox did not drain")
	}
	conversation := func(shop preliveShop, ch string, customer int64) string {
		t.Helper()
		var id string
		query := `SELECT conversation_id FROM platform.telegram_principals WHERE organization_id=$1 AND user_id=$2`
		arg := any(customer)
		if ch == "whatsapp" {
			query = `SELECT conversation_id FROM platform.channel_principals WHERE organization_id=$1 AND external_user_id=$2`
			arg = fmt.Sprint(customer)
		}
		if e := admin.QueryRow(ctx, query, shop.owner.OrganizationID, arg).Scan(&id); e != nil {
			t.Fatal(e)
		}
		return id
	}
	makeOrder := func(shop preliveShop, ch string, customer int64) string {
		t.Helper()
		conv := conversation(shop, ch, customer)
		receive(shop, ch, inbound(shop, ch, customer, "", "offer:"+shop.variant), 200)
		drain(ch)
		var quote string
		if e := admin.QueryRow(ctx, `SELECT id FROM platform.order_quotes WHERE conversation_id=$1 ORDER BY created_at DESC,id LIMIT 1`, conv).Scan(&quote); e != nil {
			t.Fatal(e)
		}
		raw := inbound(shop, ch, customer, "", "confirm:"+quote)
		receive(shop, ch, raw, 200)
		receive(shop, ch, raw, 200)
		drain(ch)
		receive(shop, ch, inbound(shop, ch, customer, "", "confirm:"+quote), 200)
		drain(ch)
		var id string
		var price int64
		var quantity int
		if e := admin.QueryRow(ctx, `SELECT o.id,i.unit_price_amount,i.quantity FROM platform.orders o JOIN platform.order_items i ON i.order_id=o.id WHERE o.conversation_id=$1 AND o.quote_id=$2`, conv, quote).Scan(&id, &price, &quantity); e != nil {
			t.Fatal(e)
		}
		if price != shop.price || quantity != 1 {
			t.Fatal("incorrect order snapshot")
		}
		var n int
		if e := admin.QueryRow(ctx, `SELECT count(*) FROM platform.orders WHERE quote_id=$1`, quote).Scan(&n); e != nil || n != 1 {
			t.Fatal("duplicate order", e, n)
		}
		return id
	}
	for _, shop := range shops {
		for _, ch := range []string{"telegram", "whatsapp"} {
			t.Run(shop.owner.OrganizationID[:8]+"_"+ch+"_catalog_consent_reserve", func(t *testing.T) {
				raw := inbound(shop, ch, 101001, "Есть PHONE8V?", "")
				receive(shop, ch, raw, 200)
				receive(shop, ch, raw, 200)
				drain(ch)
				conv := conversation(shop, ch, 101001)
				detail, e := chat.Get(ctx, shop.owner, conv)
				if e != nil {
					t.Fatal(e)
				}
				var facts struct {
					Messages []struct {
						Data struct {
							Cards []struct {
								Variant string `json:"variant_id"`
								Price   int64  `json:"price_amount"`
							} `json:"cards"`
						} `json:"data"`
					} `json:"messages"`
				}
				if json.Unmarshal(detail, &facts) != nil {
					t.Fatal("invalid detail")
				}
				found := false
				for _, message := range facts.Messages {
					for _, card := range message.Data.Cards {
						found = true
						if card.Variant != shop.variant || card.Price != shop.price {
							t.Fatal("wrong tenant catalog facts")
						}
					}
				}
				if !found {
					t.Fatal("no authoritative catalog facts")
				}

				code, body := request("GET", "/api/v1/organizations/"+shop.owner.OrganizationID+"/conversations?channel="+ch, nil, map[string]string{"Cookie": "aba_session=" + shop.cookie})
				if code != 200 || !bytes.Contains(body, []byte(conv)) {
					t.Fatal("dashboard API channel filter", code)
				}
				var count int
				if e = admin.QueryRow(ctx, `SELECT count(*) FROM platform.orders WHERE conversation_id=$1`, conv).Scan(&count); e != nil || count != 0 {
					t.Fatal("order before explicit consent")
				}
				// Missing SKU is not asserted available; shared tool returns no catalog cards.
				receive(shop, ch, inbound(shop, ch, 101002, "Есть ABSENT8V-NOT-IN-CATALOG?", ""), 200)
				drain(ch)
				absent := conversation(shop, ch, 101002)
				var cards int
				if e = admin.QueryRow(ctx, `SELECT coalesce(jsonb_array_length(data->'cards'),0) FROM platform.messages WHERE conversation_id=$1 AND role='assistant' ORDER BY created_at DESC,id LIMIT 1`, absent).Scan(&cards); e != nil || cards != 0 {
					t.Fatal("invented missing product", e, cards)
				}
				receive(shop, ch, inbound(shop, ch, 101002, "Есть PHONE8V?", ""), 200)
				drain(ch)
				first := makeOrder(shop, ch, 101001)
				second := makeOrder(shop, ch, 101002)
				var reserved int
				if e = admin.QueryRow(ctx, `SELECT quantity_reserved FROM platform.inventory WHERE variant_id=$1`, shop.variant).Scan(&reserved); e != nil || reserved != 0 {
					t.Fatal("reserve before staff")
				}
				transition := func(id, action string) error {
					_, e := orderService.Change(ctx, shop.staff, id, action, orders.Transition{ExpectedVersion: 1, IdempotencyKey: auth.UUID(), Reason: "Synthetic pre-live decision"})
					return e
				}
				// Reject one request; it must never touch stock.
				if e = transition(first, "reject"); e != nil {
					t.Fatal(e)
				}
				if e = admin.QueryRow(ctx, `SELECT quantity_reserved FROM platform.inventory WHERE variant_id=$1`, shop.variant).Scan(&reserved); e != nil || reserved != 0 {
					t.Fatal("reject reserved")
				}
				third := makeOrder(shop, ch, 101001)
				// Two real pending requests contend for exactly one last unit.
				var wg sync.WaitGroup
				results := make(chan error, 2)
				for _, id := range []string{second, third} {
					wg.Add(1)
					go func(id string) { defer wg.Done(); results <- transition(id, "confirm") }(id)
				}
				wg.Wait()
				close(results)
				success := 0
				for e := range results {
					if e == nil {
						success++
					}
				}
				if success != 1 {
					t.Fatal("overselling or both denied", success)
				}
				if e = admin.QueryRow(ctx, `SELECT quantity_reserved FROM platform.inventory WHERE variant_id=$1`, shop.variant).Scan(&reserved); e != nil || reserved != 1 {
					t.Fatal("incorrect final reserve")
				}
				// Cancel the winner through the public business service; repeated actions must not release twice.
				var winner string
				if e = admin.QueryRow(ctx, `SELECT id FROM platform.orders WHERE organization_id=$1 AND source=$2 AND status='confirmed'`, shop.owner.OrganizationID, ch).Scan(&winner); e != nil {
					t.Fatal(e)
				}
				command := orders.Transition{ExpectedVersion: 2, IdempotencyKey: auth.UUID(), Reason: "Synthetic cleanup"}
				if _, e = orderService.Change(ctx, shop.staff, winner, "cancel", command); e != nil {
					t.Fatal(e)
				}
				if _, e = orderService.Change(ctx, shop.staff, winner, "cancel", command); e != nil {
					t.Fatal(e)
				}
				if e = admin.QueryRow(ctx, `SELECT quantity_reserved FROM platform.inventory WHERE variant_id=$1`, shop.variant).Scan(&reserved); e != nil || reserved != 0 {
					t.Fatal("double release")
				}
				// The other channel-created request remains pending: change authoritative
				// fixture facts between buyer consent and staff decision, then check rollback.
				var pending string
				if e = admin.QueryRow(ctx, `SELECT id FROM platform.orders WHERE organization_id=$1 AND source=$2 AND status='pending_confirmation' LIMIT 1`, shop.owner.OrganizationID, ch).Scan(&pending); e != nil {
					t.Fatal(e)
				}
				if _, e = admin.Exec(ctx, `UPDATE platform.product_variants SET price_amount=price_amount+1 WHERE id=$1`, shop.variant); e != nil {
					t.Fatal(e)
				}
				if e = transition(pending, "confirm"); e == nil || e.Error() != "PRICE_CHANGED" {
					t.Fatal("price change not rejected", e)
				}
				if _, e = admin.Exec(ctx, `UPDATE platform.product_variants SET price_amount=$2 WHERE id=$1`, shop.variant, shop.price); e != nil {
					t.Fatal(e)
				}
				if _, e = admin.Exec(ctx, `UPDATE platform.inventory SET quantity_on_hand=0 WHERE variant_id=$1`, shop.variant); e != nil {
					t.Fatal(e)
				}
				if e = transition(pending, "confirm"); e == nil || e.Error() != "INSUFFICIENT_STOCK" {
					t.Fatal("sold out not rejected", e)
				}
				if e = admin.QueryRow(ctx, `SELECT quantity_reserved FROM platform.inventory WHERE variant_id=$1`, shop.variant).Scan(&reserved); e != nil || reserved != 0 {
					t.Fatal("conflict changed stock", e, reserved)
				}
				if _, e = admin.Exec(ctx, `UPDATE platform.inventory SET quantity_on_hand=1 WHERE variant_id=$1`, shop.variant); e != nil {
					t.Fatal(e)
				}
			})
		}
	}
	// Cross-tenant API, service, principal and SQL checks for all A->B->C pairs.
	for i, shop := range shops {
		other := shops[(i+1)%3]
		for _, ch := range []string{"telegram", "whatsapp"} {
			conv := conversation(other, ch, 101001)
			code, _ := request("GET", "/api/v1/organizations/"+other.owner.OrganizationID+"/conversations/"+conv, nil, map[string]string{"Cookie": "aba_session=" + shop.cookie})
			if code != 404 {
				t.Fatal("foreign API conversation", code)
			}
			if _, e := chat.Send(ctx, shop.owner, conv, conversations.SendInput{Text: "Cross tenant attempt", Role: "staff", IdempotencyKey: auth.UUID()}); e == nil {
				t.Fatal("foreign staff reply")
			}
			if _, e := cat.Customer().CheckInventory(ctx, shop.owner, other.variant); e == nil {
				t.Fatal("foreign inventory")
			}
			if _, e := orderService.PrepareQuote(ctx, shop.owner, conversation(shop, ch, 101001), []orders.CartItem{{VariantID: other.variant, Quantity: 1}}); e == nil {
				t.Fatal("foreign quote")
			}
			tx, e := tenancy.Begin(ctx, d.Pool, shop.owner, true, true)
			if e != nil {
				t.Fatal(e)
			}
			var n int
			if e = tx.QueryRow(ctx, `SELECT count(*) FROM platform.conversations WHERE id=$1`, conv).Scan(&n); e != nil || n != 0 {
				t.Fatal("RLS leak", e, n)
			}
			_ = tx.Rollback(ctx)
		}
		if conversation(shop, "telegram", 101001) == conversation(shop, "whatsapp", 101001) {
			t.Fatal("cross-channel customer merged")
		}
	}
	t.Run("whatsapp_retry_exhaustion_and_stale_fencing", func(t *testing.T) {
		shop := shops[2]
		conv := conversation(shop, "whatsapp", 101001)
		if _, e := chat.Control(ctx, shop.owner, conv, "human", 0); e != nil {
			t.Fatal(e)
		}
		if _, e := chat.Send(ctx, shop.staff, conv, conversations.SendInput{Text: "Synthetic retry exhaustion", Role: "staff", IdempotencyKey: auth.UUID()}); e != nil {
			t.Fatal(e)
		}
		var id, stale string
		if e := admin.QueryRow(ctx, `SELECT id FROM platform.channel_outbox WHERE conversation_id=$1 AND status='pending' ORDER BY created_at DESC,id LIMIT 1`, conv).Scan(&id); e != nil {
			t.Fatal(e)
		}
		before := waSends.Load()
		for attempt := 1; attempt <= 5; attempt++ {
			cap := auth.UUID()
			var raw []byte
			if e := d.Pool.QueryRow(ctx, `SELECT platform.channel_claim_send($1,$2)`, wa.Proof(), cap).Scan(&raw); e != nil || !bytes.Contains(raw, []byte(id)) {
				t.Fatal("claim", attempt, e, string(raw))
			}
			// Results from a preceding lease must never complete the current one.
			if stale != "" {
				if _, e := d.Pool.Exec(ctx, `SELECT platform.channel_finish_send($1,'sent',0,'stale')`, stale); e != nil {
					t.Fatal(e)
				}
			}
			var state string
			if e := admin.QueryRow(ctx, `SELECT status FROM platform.channel_outbox WHERE id=$1`, id).Scan(&state); e != nil || state != "sending" {
				t.Fatal("stale fence", e, state)
			}
			if _, e := d.Pool.Exec(ctx, `SELECT platform.channel_finish_send($1,'pending',60,'')`, cap); e != nil {
				t.Fatal(e)
			}
			stale = cap
			if e := admin.QueryRow(ctx, `SELECT status FROM platform.channel_outbox WHERE id=$1`, id).Scan(&state); e != nil {
				t.Fatal(e)
			}
			if attempt < 5 && state != "pending" || attempt == 5 && state != "failed" {
				t.Fatal("retry limit", attempt, state)
			}
			// A retry is not claimable during backoff; only the isolated test clock is advanced.
			var next []byte
			if e := d.Pool.QueryRow(ctx, `SELECT platform.channel_claim_send($1,$2)`, wa.Proof(), auth.UUID()).Scan(&next); e != nil || len(next) > 0 && string(next) != "null" {
				t.Fatal("retry bypassed backoff", e)
			}
			if _, e := admin.Exec(ctx, `UPDATE platform.channel_outbox SET available_at=now()-interval '1 second' WHERE id=$1`, id); e != nil {
				t.Fatal(e)
			}
		}
		if did, e := wa.SendOne(ctx); e != nil || did || waSends.Load() != before {
			t.Fatal("dead-letter resent", e, did)
		}
		var events int
		if e := admin.QueryRow(ctx, `SELECT count(*) FROM platform.channel_events WHERE outbox_id=$1 AND kind='sending'`, id).Scan(&events); e != nil || events != 5 {
			t.Fatal("missing attempt audit", e, events)
		}
	})
	for _, ch := range []string{"telegram", "whatsapp"} {
		t.Run(ch+"_negative_webhook_worker_and_kill", func(t *testing.T) {
			shop := shops[0]
			raw := inbound(shop, ch, 101001, "Есть PHONE8V?", "")
			id, header, secret := shop.telegramID, "X-Telegram-Bot-Api-Secret-Token", shop.secret
			if ch == "whatsapp" {
				id, header, secret = shop.whatsappID, "X-Hub-Signature-256", whatsapp.Signature(creds.AppSecret, raw)
			}
			path := "/api/v1/webhooks/" + ch + "/" + id
			for _, bad := range []string{"", strings.Repeat("z", len(secret))} {
				code, _ := request("POST", path, raw, map[string]string{"Content-Type": "application/json", header: bad})
				if code != 403 {
					t.Fatal("missing/invalid secret", code)
				}
			}
			code, _ := request("POST", "/api/v1/webhooks/"+ch+"/"+auth.UUID(), raw, map[string]string{"Content-Type": "application/json", header: secret})
			if code != 403 {
				t.Fatal("unknown integration", code)
			}
			if ch == "whatsapp" {
				code, _ = request("POST", path, append(append([]byte{}, raw...), byte(' ')), map[string]string{"Content-Type": "application/json", header: secret})
				if code != 403 {
					t.Fatal("changed body signature accepted", code)
				}
				wrongPhone := []byte(strings.ReplaceAll(string(raw), shop.phone, "41999"))
				receive(shop, ch, wrongPhone, 403)
			}
			receive(shop, ch, inbound(shop, ch, 999001, "Есть PHONE8V?", ""), 403)
			tooLarge := inbound(shop, ch, 101001, strings.Repeat("x", 70000), "")
			receive(shop, ch, tooLarge, 413)
			// A supplied organization_id is ignored: routing is bound to the verified integration.
			var document map[string]any
			_ = json.Unmarshal(inbound(shop, ch, 101001, "Есть PHONE8V?", ""), &document)
			document["organization_id"] = shops[1].owner.OrganizationID
			spoof, _ := json.Marshal(document)
			table := "telegram_inbox"
			if ch == "whatsapp" {
				table = "channel_inbox"
			}
			var before, after int
			if e := admin.QueryRow(ctx, `SELECT count(*) FROM platform.`+table+` WHERE organization_id=$1`, shops[1].owner.OrganizationID).Scan(&before); e != nil {
				t.Fatal(e)
			}
			receive(shop, ch, spoof, 200)
			if e := admin.QueryRow(ctx, `SELECT count(*) FROM platform.`+table+` WHERE organization_id=$1`, shops[1].owner.OrganizationID).Scan(&after); e != nil || before != after {
				t.Fatal("organization body spoof", e)
			}
			if ch == "whatsapp" {
				var denied []byte
				if e := d.Pool.QueryRow(ctx, `SELECT platform.channel_claim($1,$2,$3,$4)`, bytes.Repeat([]byte{99}, 32), auth.UUID(), auth.UUID(), auth.UUID()).Scan(&denied); e != nil || len(denied) > 0 && string(denied) != "null" {
					t.Fatal("worker proof bypass", e)
				}
			}
			drain(ch)
			if ch == "telegram" {
				if _, e := tg.Disconnect(ctx, shop.owner); e != nil {
					t.Fatal(e)
				}
			} else {
				if _, e := wa.Control(ctx, shop.owner, false, false); e != nil {
					t.Fatal(e)
				}
			}
			receive(shop, ch, inbound(shop, ch, 101001, "Канал отключён", ""), 403)
			var did bool
			var e error
			if ch == "telegram" {
				did, e = tg.SendOne(ctx)
			} else {
				did, e = wa.SendOne(ctx)
			}
			if e != nil || did {
				t.Fatal("disabled channel sending", e, did)
			}
		})
	}

	if tgSends.Load() == 0 || waSends.Load() == 0 {
		t.Fatal("wire not exercised")
	}
	for _, shop := range shops {
		if strings.Contains(logs.String(), shop.secret) || strings.Contains(logs.String(), shop.cookie) {
			t.Fatal("webhook/session secret logged")
		}
	}
	if strings.Contains(logs.String(), creds.AppSecret) || strings.Contains(logs.String(), creds.AccessToken) {
		t.Fatal("secret in logs")
	}
	t.Logf("PASS: 3 shops x 2 channels actual HTTP webhook/inbox/agent/catalog/outbox, verified tenant prices, missing product, explicit quote consent, duplicate replay, staff reject, concurrent last-unit reserve/cancel, API/SQL isolation; wire sends TG=%d WA=%d. This is not a Next SSR browser E2E.", tgSends.Load(), waSends.Load())
}
