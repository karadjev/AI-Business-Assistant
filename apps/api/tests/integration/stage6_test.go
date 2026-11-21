//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/agent"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/conversations"
	httpapi "github.com/karadjev/AI-Business-Assistant/apps/api/internal/http"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/db"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/telegram"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type telegramRetry struct{ calls int }

func (p *telegramRetry) GetMe(ctx context.Context, s string) (telegram.Bot, error) {
	return (telegram.Mock{}).GetMe(ctx, s)
}
func (p *telegramRetry) Send(context.Context, string, int64, json.RawMessage) (telegram.SendResult, error) {
	p.calls++
	if p.calls == 1 {
		return telegram.SendResult{RetryAfter: 1}, io.ErrUnexpectedEOF
	}
	return telegram.SendResult{MessageID: 99}, nil
}
func testStage6(t *testing.T, d *db.Database, admin *pgx.Conn, ctx context.Context) {
	cfg, _ := config.LoadAgent(func(string) string { return "" })
	tc := config.Telegram{Mode: "mock", Key: bytes.Repeat([]byte{11}, 32), Retention: 7 * 24 * time.Hour, Enabled: true}
	chat := &conversations.Service{Pool: d.Pool, Config: cfg, Provider: agent.Mock{}}
	tg := telegram.Service{Pool: d.Pool, Config: tc, Transport: telegram.Mock{}, Chat: chat}
	identity := auth.New(d.Pool, config.Auth{Origin: "http://127.0.0.1:3000", SessionTTL: time.Hour, IdleTTL: time.Hour})
	var logs bytes.Buffer
	h := httpapi.NewHandlerWithAgent(d, slog.New(slog.NewJSONHandler(&logs, nil)), 5*time.Second, 65536, identity, cfg, agent.Mock{}, tc)
	type fixture struct {
		tenant      catalog.Tenant
		token, csrf string
	}
	makeOwner := func(name string) fixture {
		uid := auth.UUID()
		token, csrf := auth.Token(), auth.Token()
		_, e := admin.Exec(ctx, `INSERT INTO platform.users(id,email,password_hash,display_name,email_verified_at) VALUES($1,$2,'synthetic',$3,now())`, uid, name+"@stage6.example.test", name)
		if e != nil {
			t.Fatal(e)
		}
		_, e = admin.Exec(ctx, `INSERT INTO platform.sessions(id,user_id,token_hash,csrf_hash,expires_at,idle_expires_at) VALUES($1,$2,$3,$4,now()+interval '1 hour',now()+interval '1 hour')`, auth.UUID(), uid, auth.Hash(token), auth.Hash(csrf))
		if e != nil {
			t.Fatal(e)
		}
		session, e := identity.Authenticate(ctx, token)
		if e != nil {
			t.Fatal(e)
		}
		o, e := (organizations.Service{Pool: d.Pool}).CreateTest(ctx, session, name)
		if e != nil {
			t.Fatal(e)
		}
		return fixture{catalog.Tenant{Session: session, OrganizationID: o.ID}, token, csrf}
	}
	a, b := makeOwner("telegram-a"), makeOwner("telegram-b")
	if _, e := tg.Connect(ctx, a.tenant, "invalid"); e == nil {
		t.Fatal("invalid token accepted")
	}
	if _, e := tg.Connect(ctx, a.tenant, "mock:60001"); e != nil {
		t.Fatal("connect", e)
	}
	if _, e := tg.Connect(ctx, b.tenant, "mock:60001"); e == nil {
		t.Fatal("bot stolen")
	}
	if _, e := tg.Connect(ctx, b.tenant, "mock:60002"); e != nil {
		t.Fatal(e)
	}
	var ia, ib string
	_ = admin.QueryRow(ctx, `SELECT id FROM platform.telegram_integrations WHERE organization_id=$1`, a.tenant.OrganizationID).Scan(&ia)
	_ = admin.QueryRow(ctx, `SELECT id FROM platform.telegram_integrations WHERE organization_id=$1`, b.tenant.OrganizationID).Scan(&ib)
	var cipher []byte
	if e := admin.QueryRow(ctx, `SELECT token_cipher FROM platform.telegram_integrations WHERE id=$1`, ia).Scan(&cipher); e != nil || bytes.Contains(cipher, []byte("mock:60001")) {
		t.Fatal("token not encrypted")
	}
	if _, e := d.Pool.Exec(ctx, `SELECT token_cipher FROM platform.telegram_integrations`); e == nil {
		t.Fatal("runtime can select ciphertext")
	}
	cs := catalog.Service{Pool: d.Pool}
	raw, e := cs.CreateProduct(ctx, a.tenant, catalog.Input{Name: "Synthetic TEST6", SKU: "TEST6", PriceAmount: 150099})
	if e != nil {
		t.Fatal(e)
	}
	var product struct {
		ID       string `json:"id"`
		Variants []struct {
			ID string `json:"id"`
		} `json:"variants"`
	}
	_ = json.Unmarshal(raw, &product)
	v := product.Variants[0].ID
	if _, e = cs.UpdateProduct(ctx, a.tenant, product.ID, catalog.Input{ExpectedVersion: 1}, "publish"); e != nil {
		t.Fatal(e)
	}
	if _, e = cs.Adjust(ctx, a.tenant, catalog.Input{VariantID: v, ExpectedVersion: 1, QuantityOnHand: 2, Reason: "synthetic"}); e != nil {
		t.Fatal(e)
	}
	update := func(id int64, text string) telegram.Update {
		return telegram.Update{ID: id, Message: &telegram.Message{ID: id, From: telegram.User{ID: 70001}, Chat: telegram.Chat{ID: 70001, Type: "private"}, Text: text}}
	}
	secret := auth.Token()
	if _, e = admin.Exec(ctx, `UPDATE platform.telegram_integrations SET webhook_hash=$2 WHERE id=$1`, ia, auth.Hash(secret)); e != nil {
		t.Fatal(e)
	}
	webhook := func(u telegram.Update, sec string, want int) {
		body, _ := json.Marshal(u)
		r := httptest.NewRequest("POST", "/api/v1/webhooks/telegram/"+ia, bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Telegram-Bot-Api-Secret-Token", sec)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("webhook want%d got%d %s", want, w.Code, w.Body.String())
		}
	}
	webhook(update(1, "Есть TEST6?"), auth.Token(), 403)

	malformed := func(body string, want int) {
		r := httptest.NewRequest("POST", "/api/v1/webhooks/telegram/"+ia, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Telegram-Bot-Api-Secret-Token", secret)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("invalid webhook want%d got%d %s", want, w.Code, w.Body.String())
		}
	}
	malformed("{", 400)
	malformed(`{"message":{"message_id":2,"from":{"id":70001},"chat":{"id":70001,"type":"private"},"text":"synthetic"}}`, 422)
	malformed(`{"update_id":99,"message":{"message_id":2,"from":{"id":70001},"chat":{"id":70001,"type":"group"},"text":"synthetic"}}`, 422)
	malformed(`{"update_id":99,"message":{"text":"`+strings.Repeat("a", 17000)+`"}}`, 413)
	webhook(update(1, "Есть TEST6?"), secret, 200)
	webhook(update(1, "Есть TEST6?"), secret, 200)
	var n int
	_ = admin.QueryRow(ctx, `SELECT count(*) FROM platform.telegram_inbox WHERE integration_id=$1`, ia).Scan(&n)
	if n != 1 {
		t.Fatal("duplicate inbox")
	}
	// Two workers may race; only one can claim this update.
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := tg.ProcessOne(ctx); results <- e }()
	}
	wg.Wait()
	close(results)
	for e := range results {
		if e != nil {
			t.Fatal("worker", e)
		}
	}
	var conv string
	_ = admin.QueryRow(ctx, `SELECT conversation_id FROM platform.telegram_principals WHERE integration_id=$1`, ia).Scan(&conv)
	if conv == "" {
		t.Fatal("principal missing")
	}
	var content string
	if e = admin.QueryRow(ctx, `SELECT payload->>'text' FROM platform.telegram_outbox WHERE conversation_id=$1 ORDER BY created_at LIMIT 1`, conv).Scan(&content); e != nil || !strings.Contains(content, "MOCK") || !strings.Contains(content, "1500.99") {
		t.Fatal("catalog answer", e, content)
	}
	_ = admin.QueryRow(ctx, `SELECT count(*) FROM platform.orders WHERE conversation_id=$1`, conv).Scan(&n)
	if n != 0 {
		t.Fatal("order without consent")
	}
	callback := func(id int64, data string) telegram.Update {
		return telegram.Update{ID: id, Callback: &telegram.Callback{ID: "synthetic", From: telegram.User{ID: 70001}, Message: telegram.Message{ID: id, Chat: telegram.Chat{ID: 70001, Type: "private"}}, Data: data}}
	}
	inject := func(u telegram.Update) {
		if _, e := tg.Inject(ctx, a.tenant, u); e != nil {
			t.Fatal(e)
		}
		if _, e := tg.ProcessOne(ctx); e != nil {
			t.Fatal("process callback", e)
		}
	}
	inject(callback(2, "offer:"+v))
	var quote string
	if e = admin.QueryRow(ctx, `SELECT id FROM platform.order_quotes WHERE conversation_id=$1 ORDER BY created_at DESC LIMIT 1`, conv).Scan(&quote); e != nil {
		t.Fatal("quote", e)
	}
	inject(callback(3, "confirm:"+quote))
	inject(callback(4, "confirm:"+quote))
	_ = admin.QueryRow(ctx, `SELECT count(*) FROM platform.orders WHERE conversation_id=$1 AND source='telegram' AND status='pending_confirmation'`, conv).Scan(&n)
	if n != 1 {
		t.Fatal("explicit callback did not create exactly one order", n)
	}
	var reserved int
	_ = admin.QueryRow(ctx, `SELECT quantity_reserved FROM platform.inventory WHERE variant_id=$1`, v).Scan(&reserved)
	if reserved != 0 {
		t.Fatal("premature reserve")
	}
	if _, e = chat.Get(ctx, b.tenant, conv); e == nil {
		t.Fatal("foreign conversation disclosed")
	}
	// Handoff stops auto answers; staff response is durably queued by DB trigger.
	detail, e := chat.Get(ctx, a.tenant, conv)
	if e != nil {
		t.Fatal(e)
	}
	var state struct {
		Version int64 `json:"version"`
	}
	_ = json.Unmarshal(detail, &state)
	if _, e = chat.Control(ctx, a.tenant, conv, "human", state.Version); e != nil {
		t.Fatal(e)
	}
	var before int
	_ = admin.QueryRow(ctx, `SELECT count(*) FROM platform.messages WHERE conversation_id=$1 AND role='assistant'`, conv).Scan(&before)
	inject(update(5, "Есть TEST6?"))
	_ = admin.QueryRow(ctx, `SELECT count(*) FROM platform.messages WHERE conversation_id=$1 AND role='assistant'`, conv).Scan(&n)
	if n != before {
		t.Fatal("handoff answered")
	}
	if _, e = chat.Send(ctx, a.tenant, conv, conversations.SendInput{Text: "Синтетический ответ сотрудника", Role: "staff", IdempotencyKey: auth.UUID()}); e != nil {
		t.Fatal(e)
	}
	// Handoff now cancels old assistant sends. Queue a second legitimate staff
	// response so retry and lost-receipt recovery remain independent scenarios.
	if _, e = chat.Send(ctx, a.tenant, conv, conversations.SendInput{Text: "Синтетический ответ для проверки lost receipt", Role: "staff", IdempotencyKey: auth.UUID()}); e != nil {
		t.Fatal(e)
	}
	retry := &telegramRetry{}
	tg.Transport = retry
	if _, e = tg.SendOne(ctx); e != nil {
		t.Fatal(e)
	}
	var pending int
	_ = admin.QueryRow(ctx, `SELECT count(*) FROM platform.telegram_outbox WHERE integration_id=$1 AND status='pending' AND attempts=1`, ia).Scan(&pending)
	if pending != 1 {
		t.Fatal("429 retry not scheduled")
	}
	_, _ = admin.Exec(ctx, `UPDATE platform.telegram_outbox SET available_at=now() WHERE integration_id=$1`, ia)
	if _, e = tg.SendOne(ctx); e != nil {
		t.Fatal(e)
	}
	if retry.calls != 2 {
		t.Fatal("outbox not retried")
	}
	// Raw runtime SQL cannot claim ciphertext without server proof; lost send
	// receipt becomes uncertain and blocks subsequent sends of this chat.
	var sendLease []byte
	if e = d.Pool.QueryRow(ctx, `SELECT platform.telegram_claim_send($1,$2,$3,$4)`, auth.UUID(), []byte("invalid-worker-proof"), nil, "mock").Scan(&sendLease); e != nil || len(sendLease) != 0 && string(sendLease) != "null" {
		t.Fatal("runtime claimed cipher without worker secret", e)
	}
	sendCap := auth.UUID()
	if e = d.Pool.QueryRow(ctx, `SELECT platform.telegram_claim_send($1,$2,$3,$4)`, sendCap, tg.WorkerProof(), nil, "mock").Scan(&sendLease); e != nil || len(sendLease) == 0 {
		t.Fatal("send lease claim", e)
	}
	var outbound struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(sendLease, &outbound)
	if _, e = admin.Exec(ctx, `UPDATE platform.telegram_outbox SET lease_until=now()-interval '1 second' WHERE capability=$1`, sendCap); e != nil {
		t.Fatal(e)
	}
	if e = d.Pool.QueryRow(ctx, `SELECT platform.telegram_claim_send($1,$2,$3,$4)`, auth.UUID(), tg.WorkerProof(), nil, "mock").Scan(&sendLease); e != nil || len(sendLease) != 0 && string(sendLease) != "null" {
		t.Fatal("uncertain send did not stop ordering", e)
	}
	var deliveryState string
	if e = admin.QueryRow(ctx, `SELECT status FROM platform.telegram_outbox WHERE id=$1`, outbound.ID).Scan(&deliveryState); e != nil || deliveryState != "uncertain" {
		t.Fatal("lost receipt not uncertain", e)
	}
	if claimed, e := tg.SendOne(ctx); e != nil || claimed || retry.calls != 2 {
		t.Fatal("uncertain automatically resent", e)
	}

	// Ephemeral capability is scoped to exact chat, and does not grant stock UPDATE.
	if _, e = tg.Inject(ctx, a.tenant, update(6, "TEST6")); e != nil {
		t.Fatal(e)
	}
	var denied []byte
	if e = d.Pool.QueryRow(ctx, `SELECT platform.telegram_claim($1,$2,$3,$4,7,$5,$6,$7)`, auth.UUID(), auth.UUID(), auth.UUID(), auth.UUID(), []byte("invalid-worker-proof"), nil, "mock").Scan(&denied); e != nil || len(denied) != 0 && string(denied) != "null" {
		t.Fatal("runtime SQL bypassed server worker gate", e)
	}
	cap := auth.UUID()
	var claimed []byte
	if e = d.Pool.QueryRow(ctx, `SELECT platform.telegram_claim($1,$2,$3,$4,7,$5,$6,$7)`, cap, auth.UUID(), auth.UUID(), auth.UUID(), tg.WorkerProof(), nil, "mock").Scan(&claimed); e != nil {
		t.Fatal(e)
	}
	tenant := catalog.Tenant{OrganizationID: a.tenant.OrganizationID, ChannelCapability: cap}
	tx, e := tenancy.Begin(ctx, d.Pool, tenant, true, true)
	if e != nil {
		t.Fatal("cap scope", e)
	}
	tag, e := tx.Exec(ctx, `UPDATE platform.inventory SET quantity_reserved=1 WHERE variant_id=$1`, v)
	if e != nil || tag.RowsAffected() != 0 {
		t.Fatal("channel can reserve", e)
	}
	var foreign int
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM platform.conversations WHERE organization_id=$1`, b.tenant.OrganizationID).Scan(&foreign); e != nil || foreign != 0 {
		t.Fatal("channel foreign RLS")
	}
	_ = tx.Rollback(ctx)
	// transaction-local capability must not survive pool reuse.
	for i := 0; i < 4; i++ {
		var valid bool
		if e = d.Pool.QueryRow(ctx, `SELECT platform.telegram_scope($1,NULL)`, a.tenant.OrganizationID).Scan(&valid); e != nil || valid {
			t.Fatal("cap leak", e)
		}
	}
	_, _ = admin.Exec(ctx, `UPDATE platform.telegram_inbox SET lease_until=now()-interval '1 second' WHERE capability=$1`, cap)
	if _, e = tg.ProcessOne(ctx); e != nil {
		t.Fatal("restart lease recovery", e)
	}
	for _, table := range []string{"telegram_integrations", "telegram_principals", "telegram_inbox", "telegram_outbox", "telegram_events"} {
		var forced bool
		if e = admin.QueryRow(ctx, `SELECT relforcerowsecurity FROM pg_class WHERE oid=$1::regclass`, "platform."+table).Scan(&forced); e != nil || !forced {
			t.Fatal("not forced", table)
		}
		tx, e = d.Pool.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		_, e = tx.Exec(ctx, `SELECT set_config('app.session_hash',$1,true),set_config('app.organization_id',$2,true)`, hex.EncodeToString(b.tenant.Session.Hash), a.tenant.OrganizationID)
		if e != nil {
			t.Fatal(e)
		}
		if e = tx.QueryRow(ctx, `SELECT count(*) FROM platform.`+table).Scan(&n); e != nil || n != 0 {
			t.Fatal("foreign SQL", table, e)
		}
		_ = tx.Rollback(ctx)
	}

	// Same external sender, different bot/tenant: independent customer conversation.
	if _, e = tg.Inject(ctx, b.tenant, update(1, "Есть TEST6?")); e != nil {
		t.Fatal(e)
	}
	if _, e = tg.ProcessOne(ctx); e != nil {
		t.Fatal(e)
	}
	var convB string
	if e = admin.QueryRow(ctx, `SELECT conversation_id FROM platform.telegram_principals WHERE integration_id=$1 AND user_id=70001`, ib).Scan(&convB); e != nil || convB == conv {
		t.Fatal("principal crossed tenants", e)
	}
	if _, e = tg.Inject(ctx, b.tenant, callback(2, "confirm:"+quote)); e != nil {
		t.Fatal(e)
	}
	if _, e = tg.ProcessOne(ctx); e != nil {
		t.Fatal(e)
	}
	if e = admin.QueryRow(ctx, `SELECT count(*) FROM platform.orders WHERE conversation_id=$1`, convB).Scan(&n); e != nil || n != 0 {
		t.Fatal("foreign consent created order", e)
	}
	if _, e = chat.Send(ctx, a.tenant, conv, conversations.SendInput{Text: "staff impersonation", Role: "customer", IdempotencyKey: auth.UUID()}); e == nil {
		t.Fatal("staff impersonated Telegram customer")
	}
	// A committed customer message survives crash before inbox completion without duplication.
	if _, e = tg.Inject(ctx, a.tenant, update(7, "Crash recovery synthetic")); e != nil {
		t.Fatal(e)
	}
	cap = auth.UUID()
	if e = d.Pool.QueryRow(ctx, `SELECT platform.telegram_claim($1,$2,$3,$4,7,$5,$6,$7)`, cap, auth.UUID(), auth.UUID(), auth.UUID(), tg.WorkerProof(), nil, "mock").Scan(&claimed); e != nil {
		t.Fatal(e)
	}
	var leased telegram.Job
	_ = json.Unmarshal(claimed, &leased)
	tenant.ChannelCapability = cap
	if _, e = chat.Send(ctx, tenant, conv, conversations.SendInput{Text: leased.Payload.Message.Text, Role: "customer", IdempotencyKey: leased.ID}); e != nil {
		t.Fatal(e)
	}
	_, _ = admin.Exec(ctx, `UPDATE platform.telegram_inbox SET lease_until=now()-interval '1 second' WHERE capability=$1`, cap)
	if _, e = tg.ProcessOne(ctx); e != nil {
		t.Fatal(e)
	}
	if e = admin.QueryRow(ctx, `SELECT count(*) FROM platform.messages WHERE conversation_id=$1 AND client_key=$2`, conv, leased.ID).Scan(&n); e != nil || n != 1 {
		t.Fatal("restart duplicated message", e)
	}
	// Lease prevents claiming a subsequent message of the same chat.
	for _, id := range []int64{8, 9} {
		if _, e = tg.Inject(ctx, a.tenant, update(id, "Sequential synthetic")); e != nil {
			t.Fatal(e)
		}
	}
	cap = auth.UUID()
	if e = d.Pool.QueryRow(ctx, `SELECT platform.telegram_claim($1,$2,$3,$4,7,$5,$6,$7)`, cap, auth.UUID(), auth.UUID(), auth.UUID(), tg.WorkerProof(), nil, "mock").Scan(&claimed); e != nil {
		t.Fatal(e)
	}
	var second []byte
	if e = d.Pool.QueryRow(ctx, `SELECT platform.telegram_claim($1,$2,$3,$4,7,$5,$6,$7)`, auth.UUID(), auth.UUID(), auth.UUID(), auth.UUID(), tg.WorkerProof(), nil, "mock").Scan(&second); e != nil || len(second) != 0 && string(second) != "null" {
		t.Fatal("same chat processed concurrently", e)
	}
	_, _ = d.Pool.Exec(ctx, `SELECT platform.telegram_finish($1,NULL)`, cap)
	if _, e = tg.ProcessOne(ctx); e != nil {
		t.Fatal(e)
	}
	// Default member denied; explicit staff reads only, configuration remains owner-only.
	_, _ = admin.Exec(ctx, `UPDATE platform.organization_members SET role='member',can_manage_orders=false WHERE organization_id=$1 AND user_id=$2`, a.tenant.OrganizationID, a.tenant.Session.User.ID)
	if _, e = tg.Status(ctx, a.tenant); e == nil {
		t.Fatal("member without staff read")
	}
	_, _ = admin.Exec(ctx, `UPDATE platform.organization_members SET can_manage_orders=true WHERE organization_id=$1 AND user_id=$2`, a.tenant.OrganizationID, a.tenant.Session.User.ID)
	if _, e = tg.Status(ctx, a.tenant); e != nil {
		t.Fatal("staff read", e)
	}
	if _, e = tg.Check(ctx, a.tenant); e == nil {
		t.Fatal("member checked bot")
	}
	if _, e = tg.Disconnect(ctx, a.tenant); e == nil {
		t.Fatal("member disconnected")
	}
	_, _ = admin.Exec(ctx, `UPDATE platform.organization_members SET role='owner',can_manage_orders=false WHERE organization_id=$1 AND user_id=$2`, a.tenant.OrganizationID, a.tenant.Session.User.ID)
	// Background synthetic retention erases all message/inbox/outbox payloads.
	_, _ = admin.Exec(ctx, `UPDATE platform.conversations SET expires_at=now()-interval '1 second' WHERE id=$1`, convB)
	if _, e = d.Pool.Exec(ctx, `SELECT platform.telegram_purge_expired($1)`, tg.WorkerProof()); e != nil {
		t.Fatal(e)
	}
	if e = admin.QueryRow(ctx, `SELECT count(*) FROM platform.messages WHERE conversation_id=$1 AND content<>''`, convB).Scan(&n); e != nil || n != 0 {
		t.Fatal("retention did not erase", e)
	}
	if _, e = tg.Disconnect(ctx, a.tenant); e != nil {
		t.Fatal(e)
	}
	webhook(update(9, "TEST6"), secret, 403)
	if strings.Contains(logs.String(), secret) || strings.Contains(logs.String(), "mock:60001") || strings.Contains(logs.String(), "Есть TEST6?") {
		t.Fatal("sensitive data in HTTP logs")
	}
	t.Log("PASS: mock Telegram encrypted connect/webhook/dedup/principal/worker/catalog/inline consent/no reserve/handoff/staff/outbox retry/lease recovery/FORCE RLS/pool isolation/disconnect")
	_ = http.MethodGet
}
