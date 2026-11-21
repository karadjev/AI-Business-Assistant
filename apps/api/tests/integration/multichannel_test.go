//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/agent"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
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

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/conversations"
	httpapi "github.com/karadjev/AI-Business-Assistant/apps/api/internal/http"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/db"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/telegram"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/whatsapp"
)

func testMultichannel(t *testing.T, d *db.Database, admin *pgx.Conn, ctx context.Context) {
	cfg, _ := config.LoadAgent(func(string) string { return "" })
	chat := &conversations.Service{Pool: d.Pool, Config: cfg, Provider: agent.Mock{}}
	key := bytes.Repeat([]byte{61}, 32)
	wa := whatsapp.Service{Pool: d.Pool, Chat: chat, Key: key, Transport: whatsapp.Mock{}}
	identity := auth.New(d.Pool, config.Auth{Origin: "http://127.0.0.1:3000", SessionTTL: time.Hour, IdleTTL: time.Hour})
	sessionTokens := map[string][2]string{}
	owner := func(name string) catalog.Tenant {
		uid := auth.UUID()
		token, csrf := auth.Token(), auth.Token()
		_, e := admin.Exec(ctx, `INSERT INTO platform.users(id,email,password_hash,display_name,email_verified_at) VALUES($1,$2,'synthetic',$3,now())`, uid, strings.ToLower(name)+"@multichannel.example.test", name)
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
		sessionTokens[session.User.ID] = [2]string{token, csrf}
		return catalog.Tenant{Session: session, OrganizationID: o.ID}
	}
	a, b := owner("channel-A"), owner("channel-B")
	credentials := whatsapp.Credentials{AccessToken: "mock:only-synthetic", AppSecret: "mock:synthetic-app-secret", VerifyToken: "mock:synthetic-verify-token", AllowedUsers: []string{"700"}}
	connect := func(tenant catalog.Tenant, phone string) string {
		raw, e := wa.Connect(ctx, tenant, whatsapp.ConnectInput{AppID: "100", BusinessID: "200", PhoneID: phone, GraphVersion: "v0.0", Credentials: credentials})
		if e != nil {
			t.Fatal("connect", e)
		}
		if bytes.Contains(raw, []byte(credentials.AppSecret)) || bytes.Contains(raw, []byte(credentials.AccessToken)) {
			t.Fatal("secret in status")
		}
		var result struct {
			Integration struct {
				ID string `json:"id"`
			} `json:"integration"`
		}
		if json.Unmarshal(raw, &result) != nil || result.Integration.ID == "" {
			t.Fatal("integration missing")
		}
		return result.Integration.ID
	}
	ia, ib := connect(a, "300"), connect(b, "301")
	if _, e := wa.Connect(ctx, b, whatsapp.ConnectInput{AppID: "100", BusinessID: "200", PhoneID: "300", GraphVersion: "v0.0", Credentials: credentials}); e == nil {
		t.Fatal("phone stolen")
	}
	if _, e := wa.Control(ctx, catalog.Tenant{Session: b.Session, OrganizationID: a.OrganizationID}, true, true); e == nil {
		t.Fatal("foreign control")
	}
	if _, e := d.Pool.Exec(ctx, `SELECT secret_cipher FROM platform.channel_integrations`); e == nil {
		t.Fatal("cipher leaked")
	}
	member := owner("channel-staff")
	if _, e := admin.Exec(ctx, `INSERT INTO platform.organization_members(organization_id,user_id,role,can_manage_orders) VALUES($1,$2,'member',true)`, a.OrganizationID, member.Session.User.ID); e != nil {
		t.Fatal(e)
	}
	member.OrganizationID = a.OrganizationID
	if _, e := wa.Status(ctx, member); e != nil {
		t.Fatal("staff read", e)
	}
	if _, e := wa.Control(ctx, member, true, true); e == nil {
		t.Fatal("member changed channel settings")
	}
	if _, e := wa.Connect(ctx, member, whatsapp.ConnectInput{AppID: "100", BusinessID: "200", PhoneID: "302", GraphVersion: "v0.0", Credentials: credentials}); e == nil {
		t.Fatal("member connected channel")
	}
	for _, tenant := range []catalog.Tenant{a, b} {
		if _, e := wa.Control(ctx, tenant, true, true); e != nil {
			t.Fatal(e)
		}
	}
	payload := func(phone, id, typ, text string) []byte {
		message := map[string]any{"id": id, "from": "700", "timestamp": fmt.Sprint(time.Now().Unix()), "type": typ, "text": map[string]string{"body": text}}
		if typ == "interactive" {
			message["interactive"] = map[string]any{"type": "button_reply", "button_reply": map[string]string{"id": text}}
		}
		raw, _ := json.Marshal(map[string]any{"object": "whatsapp_business_account", "entry": []any{map[string]any{"id": "200", "changes": []any{map[string]any{"field": "messages", "value": map[string]any{"metadata": map[string]string{"phone_number_id": phone}, "messages": []any{message}}}}}}})
		return raw
	}
	ingress := func(id string, body []byte) int {
		t.Helper()
		n, e := wa.Ingress(ctx, id, whatsapp.Signature(credentials.AppSecret, body), body)
		if e != nil {
			t.Fatal("ingress", e)
		}
		return n
	}
	raw := payload("300", "message-1", "text", "Есть MC-S25?")
	if ingress(ia, raw) != 1 || ingress(ia, raw) != 0 {
		t.Fatal("dedup")
	}
	if _, e := wa.Ingress(ctx, ib, whatsapp.Signature(credentials.AppSecret, raw), raw); e == nil {
		t.Fatal("wrong integration")
	}
	if _, e := wa.Ingress(ctx, ia, "sha256="+strings.Repeat("0", 64), raw); e == nil {
		t.Fatal("signature bypass")
	}
	var logs bytes.Buffer
	h := httpapi.NewHandlerWithChannels(d, slog.New(slog.NewJSONHandler(&logs, nil)), 5*time.Second, 65536, identity, cfg, agent.Mock{}, config.Telegram{}, config.WhatsApp{Key: key})
	for _, test := range []struct {
		name   string
		tenant catalog.Tenant
		csrf   bool
		status int
	}{{"unauthenticated", catalog.Tenant{}, false, 401}, {"missing_csrf", a, false, 403}, {"member_config", member, true, 403}, {"owner_config", a, true, 200}} {
		req := httptest.NewRequest("POST", "/api/v1/organizations/"+a.OrganizationID+"/integrations/whatsapp/control", strings.NewReader(`{"active":true,"sending_enabled":true}`))
		req.Header.Set("Origin", "http://127.0.0.1:3000")
		req.Header.Set("Content-Type", "application/json")
		token := sessionTokens[test.tenant.Session.User.ID]
		if token[0] != "" {
			req.AddCookie(&http.Cookie{Name: "aba_session", Value: token[0]})
		}
		if test.csrf {
			req.Header.Set("X-CSRF-Token", token[1])
		}
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		if res.Code != test.status {
			t.Fatal(test.name, res.Code)
		}
	}
	r := httptest.NewRequest("POST", "/api/v1/webhooks/whatsapp/"+ia, bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Hub-Signature-256", whatsapp.Signature(credentials.AppSecret, raw))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal("webhook", w.Code, w.Body.String())
	}
	r = httptest.NewRequest("GET", "/api/v1/webhooks/whatsapp/"+ia+"?hub.mode=subscribe&hub.verify_token="+credentials.VerifyToken+"&hub.challenge=123", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 || w.Body.String() != "123" {
		t.Fatal("challenge", w.Code)
	}
	cs := catalog.Service{Pool: d.Pool}
	prod, e := cs.CreateProduct(ctx, a, catalog.Input{Name: "Synthetic MC-S25", SKU: "MC-S25", PriceAmount: 900099})
	if e != nil {
		t.Fatal(e)
	}
	var product struct {
		ID       string `json:"id"`
		Variants []struct {
			ID string `json:"id"`
		} `json:"variants"`
	}
	_ = json.Unmarshal(prod, &product)
	_, e = cs.UpdateProduct(ctx, a, product.ID, catalog.Input{ExpectedVersion: 1}, "publish")
	if e != nil {
		t.Fatal("publish", e)
	}
	// Inventory uses the existing versioned adjustment service (admin fixture seeds only synthetic stock).
	_, e = admin.Exec(ctx, `UPDATE platform.inventory SET quantity_on_hand=5 WHERE organization_id=$1`, a.OrganizationID)
	if e != nil {
		t.Fatal(e)
	}
	drain := func() {
		t.Helper()
		for i := 0; i < 10; i++ {
			found, e := wa.ProcessOne(ctx)
			if e != nil {
				t.Fatal("process", e)
			}
			if !found {
				return
			}
		}
		t.Fatal("inbox failed to drain")
	}
	drain()
	var conv string
	if e = admin.QueryRow(ctx, `SELECT conversation_id FROM platform.channel_principals WHERE integration_id=$1`, ia).Scan(&conv); e != nil {
		t.Fatal(e)
	}
	// The identical external ID in Telegram is a separate customer/conversation.
	tg := telegram.Service{Pool: d.Pool, Chat: chat, Config: config.Telegram{Enabled: true, Mode: "mock", Key: bytes.Repeat([]byte{62}, 32), Retention: 7 * 24 * time.Hour}, Transport: telegram.Mock{}}
	if _, e := tg.Connect(ctx, a, "mock:89001"); e != nil {
		t.Fatal(e)
	}
	if _, e := tg.Inject(ctx, a, telegram.Update{ID: 1, Message: &telegram.Message{ID: 1, From: telegram.User{ID: 700}, Chat: telegram.Chat{ID: 700, Type: "private"}, Text: "Есть MC-S25?"}}); e != nil {
		t.Fatal(e)
	}
	if found, e := tg.ProcessOne(ctx); e != nil || !found {
		t.Fatal("Telegram regression", e)
	}
	var tgConv string
	if e = admin.QueryRow(ctx, `SELECT conversation_id FROM platform.telegram_principals WHERE organization_id=$1 AND user_id=700`, a.OrganizationID).Scan(&tgConv); e != nil || tgConv == conv {
		t.Fatal("cross-channel principal collision", e)
	}
	ingress(ib, payload("301", "b-message-1", "text", "Есть MC-S25?"))
	drain()
	var count int
	_ = admin.QueryRow(ctx, `SELECT count(*) FROM platform.channel_principals WHERE external_user_id='700'`).Scan(&count)
	if count != 2 {
		t.Fatal("cross tenant principal collision")
	}
	if _, e := chat.Get(ctx, b, conv); e == nil {
		t.Fatal("foreign conversation")
	}
	var reserved int
	_ = admin.QueryRow(ctx, `SELECT quantity_reserved FROM platform.inventory WHERE organization_id=$1`, a.OrganizationID).Scan(&reserved)
	if reserved != 0 {
		t.Fatal("reserved without consent")
	}
	if e = admin.QueryRow(ctx, `SELECT count(*) FROM platform.orders WHERE conversation_id=$1`, conv).Scan(&count); e != nil || count != 0 {
		t.Fatal("order without consent", e, count)
	}
	ingress(ia, payload("300", "offer-1", "interactive", "offer:"+product.Variants[0].ID))
	drain()
	var quote string
	if e = admin.QueryRow(ctx, `SELECT id FROM platform.order_quotes WHERE conversation_id=$1`, conv).Scan(&quote); e != nil {
		t.Fatal("quote", e)
	}
	ingress(ia, payload("300", "confirm-1", "interactive", "confirm:"+quote))
	drain()
	ingress(ia, payload("300", "confirm-2", "interactive", "confirm:"+quote))
	drain()
	if e = admin.QueryRow(ctx, `SELECT count(*) FROM platform.orders WHERE conversation_id=$1 AND source='whatsapp' AND status='pending_confirmation'`, conv).Scan(&count); e != nil || count != 1 {
		t.Fatal("order consent/idempotency", e, count)
	}
	_ = admin.QueryRow(ctx, `SELECT quantity_reserved FROM platform.inventory WHERE organization_id=$1`, a.OrganizationID).Scan(&reserved)
	if reserved != 0 {
		t.Fatal("reserve before staff")
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := wa.SendOne(ctx); errs <- e }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal("concurrent send", e)
		}
	}
	if _, e = chat.Control(ctx, a, conv, "human", 0); e != nil {
		t.Fatal("handoff", e)
	}
	var assistantsBefore int
	if e = admin.QueryRow(ctx, `SELECT count(*) FROM platform.messages WHERE conversation_id=$1 AND role='assistant'`, conv).Scan(&assistantsBefore); e != nil {
		t.Fatal(e)
	}
	ingress(ia, payload("300", "human-1", "text", "Без автоответа"))
	drain()
	ingress(ia, payload("300", "unsupported-1", "video", ""))
	drain()
	if e = admin.QueryRow(ctx, `SELECT count(*) FROM platform.messages WHERE conversation_id=$1 AND role='assistant'`, conv).Scan(&count); e != nil || count != assistantsBefore {
		t.Fatal("handoff generated automatic reply", e, count)
	}
	if _, e = chat.Send(ctx, a, conv, conversations.SendInput{Text: "Synthetic staff reply", Role: "staff", IdempotencyKey: auth.UUID()}); e != nil {
		t.Fatal("staff", e)
	}
	// One remaining staff job, actual HTTP mock held while another worker competes.
	if _, e := wa.Control(ctx, b, false, false); e != nil {
		t.Fatal(e)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var wireCalls atomic.Int32
	wire := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wireCalls.Add(1)
		close(entered)
		<-release
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"messages":[{"id":"mock-wire-staff"}]}`))
	}))
	defer wire.Close()
	target, _ := url.Parse(wire.URL)
	realTransport := &http.Transport{Proxy: nil}
	defer realTransport.CloseIdleConnections()
	wa.Transport = whatsapp.HTTPMock{Transport: auditRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "whatsapp.mock.invalid" {
			return nil, errors.New("unexpected destination")
		}
		clone := r.Clone(r.Context())
		v := *r.URL
		v.Scheme = target.Scheme
		v.Host = target.Host
		clone.URL = &v
		return realTransport.RoundTrip(clone)
	})}
	firstDone := make(chan error, 1)
	go func() { _, e := wa.SendOne(ctx); firstDone <- e }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("wire not reached")
	}
	if found, e := wa.SendOne(ctx); e != nil || found {
		close(release)
		t.Fatal("second worker claimed same send", e, found)
	}
	close(release)
	if e := <-firstDone; e != nil || wireCalls.Load() != 1 {
		t.Fatal("wire send", e, wireCalls.Load())
	}
	wa.Transport = whatsapp.Mock{}
	// Late receipts must not downgrade read -> sent or read -> failed.
	receipt := func(state string) []byte {
		status := map[string]string{"id": "mock-wire-staff", "recipient_id": "700", "timestamp": fmt.Sprint(time.Now().Unix()), "status": state}
		value := map[string]any{"metadata": map[string]string{"phone_number_id": "300"}, "statuses": []any{status}}
		entry := map[string]any{"id": "200", "changes": []any{map[string]any{"field": "messages", "value": value}}}
		raw, _ := json.Marshal(map[string]any{"object": "whatsapp_business_account", "entry": []any{entry}})
		return raw
	}

	for _, state := range []string{"delivered", "read", "sent", "failed"} {
		ingress(ia, receipt(state))
		drain()
	}
	var deliveryState string
	if e = admin.QueryRow(ctx, `SELECT status FROM platform.channel_outbox WHERE provider_id='mock-wire-staff'`).Scan(&deliveryState); e != nil || deliveryState != "read" {
		t.Fatal("out of order status regression", e, deliveryState)
	}
	// Real subprocess death after actual loopback provider receipt, before DB settlement.
	if _, e = chat.Send(ctx, a, conv, conversations.SendInput{Text: "Synthetic crash delivery", Role: "staff", IdempotencyKey: auth.UUID()}); e != nil {
		t.Fatal(e)
	}
	var crashCalls atomic.Int32
	crashWire := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		crashCalls.Add(1)
		_, _ = w.Write([]byte(`{"messages":[{"id":"mock-crash-receipt"}]}`))
	}))
	defer crashWire.Close()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWhatsAppCrashHelper$")
	child.Env = append(os.Environ(), "ABA_WA_CRASH=after_receipt", "ABA_WA_WIRE="+crashWire.URL)
	if e = child.Run(); e == nil {
		t.Fatal("crash subprocess survived")
	} else {
		var exit *exec.ExitError
		if !errors.As(e, &exit) || exit.ExitCode() != 87 {
			t.Fatal("unexpected crash subprocess status")
		}
	}
	if crashCalls.Load() != 1 {
		t.Fatal("provider receipt not observed")
	}
	_, e = admin.Exec(ctx, `UPDATE platform.channel_outbox SET lease_until=now()-interval '1 second' WHERE integration_id=$1 AND status='sending'`, ia)
	if e != nil {
		t.Fatal(e)
	}
	restarted := whatsapp.Service{Pool: d.Pool, Key: key, Chat: chat, Transport: whatsapp.Mock{}}
	if found, e := restarted.SendOne(ctx); e != nil || found {
		t.Fatal("restart resent unknown", e, found)
	}
	var unknownID string
	var unknownVersion int64
	if e = admin.QueryRow(ctx, `SELECT id,version FROM platform.channel_outbox WHERE integration_id=$1 AND status='delivery_unknown'`, ia).Scan(&unknownID, &unknownVersion); e != nil {
		t.Fatal("lost UNKNOWN", e)
	}
	if e = wa.Resolve(ctx, a, unknownID, unknownVersion, "cancelled"); e == nil {
		t.Fatal("owner forced WhatsApp delivery without operator evidence")
	}
	// Isolate the remaining window scenario using privileged synthetic fixture cleanup.
	if _, e = admin.Exec(ctx, `UPDATE platform.channel_outbox SET status='cancelled',version=version+1 WHERE id=$1`, unknownID); e != nil {
		t.Fatal(e)
	}
	if e = wa.Resolve(ctx, a, unknownID, unknownVersion, "cancelled"); e == nil {
		t.Fatal("stale manual resolution accepted")
	}
	if crashCalls.Load() != 1 {
		t.Fatal("manual resolution resent message")
	}
	if _, e = chat.Send(ctx, a, conv, conversations.SendInput{Text: "Synthetic pending before window closes", Role: "staff", IdempotencyKey: auth.UUID()}); e != nil {
		t.Fatal(e)
	}
	_, e = admin.Exec(ctx, `UPDATE platform.channel_principals SET window_until=now()-interval '1 minute' WHERE integration_id=$1`, ia)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = chat.Send(ctx, a, conv, conversations.SendInput{Text: "Blocked outside window", Role: "staff", IdempotencyKey: auth.UUID()}); e == nil {
		t.Fatal("closed window staff bypass")
	}
	if found, e := wa.SendOne(ctx); e != nil || found {
		t.Fatal("closed window sent free text", e, found)
	}
	if e = admin.QueryRow(ctx, `SELECT count(*) FROM platform.channel_outbox WHERE integration_id=$1 AND status='needs_approved_template'`, ia).Scan(&count); e != nil || count < 1 {
		t.Fatal("missing template state", e, count)
	}
	// Reclaimed in-flight work becomes UNKNOWN and must not be resent.
	_, e = admin.Exec(ctx, `UPDATE platform.channel_outbox SET status='sending',capability=$2,lease_until=now()-interval '1 second' WHERE id=(SELECT id FROM platform.channel_outbox WHERE integration_id=$1 ORDER BY created_at DESC LIMIT 1)`, ia, auth.UUID())
	if e != nil {
		t.Fatal(e)
	}
	if _, e = wa.SendOne(ctx); e != nil {
		t.Fatal(e)
	}
	if e = admin.QueryRow(ctx, `SELECT count(*) FROM platform.channel_outbox WHERE integration_id=$1 AND status='delivery_unknown'`, ia).Scan(&count); e != nil || count != 1 {
		t.Fatal("unknown recovery", e, count)
	}
	// Real leased WA capability: no TG scope, no foreign tenant, no pool-context leak.
	ingress(ia, payload("300", "lease-scope-test", "text", "Синтетическое сообщение"))
	cap := auth.UUID()
	var claimed []byte
	if e = d.Pool.QueryRow(ctx, `SELECT platform.channel_claim($1,$2,$3,$4)`, wa.Proof(), cap, auth.UUID(), auth.UUID()).Scan(&claimed); e != nil {
		t.Fatal(e)
	}
	leased := catalog.Tenant{OrganizationID: a.OrganizationID, ChannelCapability: cap, ChannelType: "whatsapp"}
	scopeTx, e := tenancy.Begin(ctx, d.Pool, leased, true, true)
	if e != nil {
		t.Fatal("WA scope", e)
	}
	var visible int
	if e = scopeTx.QueryRow(ctx, `SELECT count(*) FROM platform.telegram_principals`).Scan(&visible); e != nil || visible != 0 {
		t.Fatal("WA scope leaked Telegram rows", e, visible)
	}
	_ = scopeTx.Rollback(ctx)
	leased.ChannelType = "telegram"
	if tx, e := tenancy.Begin(ctx, d.Pool, leased, true, true); e == nil {
		_ = tx.Rollback(ctx)
		t.Fatal("WA cap accepted as TG")
	}
	leased.ChannelType = "whatsapp"
	leased.OrganizationID = b.OrganizationID
	if tx, e := tenancy.Begin(ctx, d.Pool, leased, true, true); e == nil {
		_ = tx.Rollback(ctx)
		t.Fatal("foreign tenant cap")
	}
	if _, e = d.Pool.Exec(ctx, `SELECT platform.channel_finish($1,true)`, cap); e != nil {
		t.Fatal(e)
	}
	var leaked bool
	if e = d.Pool.QueryRow(ctx, `SELECT coalesce(current_setting('app.channel_cap',true),'')<>''`).Scan(&leaked); e != nil || leaked {
		t.Fatal("pool leaked channel context", e)
	}
	// Configured expiry erases WA payloads without deleting order snapshots/audit.
	if _, e = admin.Exec(ctx, `UPDATE platform.conversations SET expires_at=now()-interval '1 second' WHERE id=$1`, conv); e != nil {
		t.Fatal(e)
	}
	if _, e = wa.ProcessOne(ctx); e != nil {
		t.Fatal("retention maintenance", e)
	}
	if e = admin.QueryRow(ctx, `SELECT count(*) FROM platform.messages WHERE conversation_id=$1 AND (content<>'' OR data<>'{}'::jsonb)`, conv).Scan(&count); e != nil || count != 0 {
		t.Fatal("retention left message content", e, count)
	}
	if e = admin.QueryRow(ctx, `SELECT count(*) FROM platform.channel_inbox WHERE integration_id=$1 AND payload<>'{}'::jsonb`, ia).Scan(&count); e != nil || count != 0 {
		t.Fatal("retention left inbox payload", e, count)
	}
	if _, e = wa.Control(ctx, a, false, false); e != nil {
		t.Fatal(e)
	}
	if _, e = wa.Ingress(ctx, ia, whatsapp.Signature(credentials.AppSecret, raw), raw); e == nil {
		t.Fatal("disabled ingress")
	}
	tx, e := tenancy.Begin(ctx, d.Pool, b, true, true)
	if e != nil {
		t.Fatal(e)
	}
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM platform.channel_outbox WHERE organization_id=$1`, a.OrganizationID).Scan(&count); e != nil || count != 0 {
		t.Fatal("RLS", e, count)
	}
	_ = tx.Rollback(ctx)
	tx, e = d.Pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	_, e = tx.Exec(ctx, `SELECT set_config('app.session_hash',$1,true),set_config('app.organization_id',$2,true)`, hex.EncodeToString(b.Session.Hash), a.OrganizationID)
	if e != nil {
		t.Fatal(e)
	}
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM platform.channel_principals`).Scan(&count); e != nil || count != 0 {
		t.Fatal("forged tenant", e, count)
	}
	_ = tx.Rollback(ctx)
	// A local DB connection failure must not be acknowledged as a stored webhook.
	unavailable, e := db.Open(ctx, "postgresql://aba_app:synthetic@127.0.0.1:1/aba_test?sslmode=disable", 1, 100*time.Millisecond)
	if e != nil {
		t.Fatal(e)
	}
	defer unavailable.Close()
	unavailableIdentity := auth.New(unavailable.Pool, config.Auth{Origin: "http://127.0.0.1:3000", SessionTTL: time.Hour, IdleTTL: time.Hour})
	unavailableHandler := httpapi.NewHandlerWithChannels(unavailable, slog.New(slog.NewJSONHandler(&logs, nil)), time.Second, 65536, unavailableIdentity, cfg, agent.Mock{}, config.Telegram{}, config.WhatsApp{Key: key})
	failedReq := httptest.NewRequest("POST", "/api/v1/webhooks/whatsapp/"+ib, bytes.NewReader(raw))
	failedReq.Header.Set("Content-Type", "application/json")
	failedReq.Header.Set("X-Hub-Signature-256", whatsapp.Signature(credentials.AppSecret, raw))
	failedRes := httptest.NewRecorder()
	unavailableHandler.ServeHTTP(failedRes, failedReq)
	if failedRes.Code != 503 {
		t.Fatal("DB failure falsely acknowledged or misclassified", failedRes.Code)
	}
	if strings.Contains(logs.String(), credentials.AppSecret) || strings.Contains(logs.String(), credentials.AccessToken) {
		t.Fatal("secrets in logs")
	}

	t.Log("PASS: WhatsApp signed webhook/challenge, phone/tenant routing, durable dedup, shared mock agent/catalog/quote/explicit consent, no reserve, handoff/staff/window, queue recovery, encrypted secrets, runtime RLS")
}
