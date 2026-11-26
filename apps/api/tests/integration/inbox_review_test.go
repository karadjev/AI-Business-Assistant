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
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/agent"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/agentruntime"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/conversations"
	httpapi "github.com/karadjev/AI-Business-Assistant/apps/api/internal/http"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/instagram"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/db"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/migrate"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/telegram"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/whatsapp"
)

func TestInboxReviewIsolated(t *testing.T) {
	if os.Getenv("ABA_TEST_INBOX37") != "1" {
		t.Skip("requires the isolated Inbox runner")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	adminDSN := testDSN(t, "ABA_TEST_MIGRATION_DATABASE_URL", "aba_migrator")
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	var marker string
	if err = admin.QueryRow(ctx, "SELECT marker FROM platform.test_safety WHERE singleton").Scan(&marker); err != nil || marker != "aba-synthetic-isolated" {
		t.Fatal("isolated database required")
	}
	for _, operation := range []string{"up", "down", "up"} {
		if err = migrate.Run(ctx, adminDSN, operation); err != nil {
			t.Fatal(operation, err)
		}
	}
	d, err := db.Open(ctx, testDSN(t, "ABA_TEST_DATABASE_URL", "aba_app"), 6, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err = d.Ready(ctx); err != nil {
		t.Fatal(err)
	}
	runSQL := func(query string, args ...any) {
		t.Helper()
		if _, err := admin.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	const origin = "http://127.0.0.1:3000"
	key := bytes.Repeat([]byte{184}, 32)
	manager := agentruntime.Service{Pool: d.Pool, Key: key}
	proof := sha256.Sum256(manager.Proof())
	identity := auth.New(d.Pool, config.Auth{Origin: origin, EmailKey: key, SessionTTL: time.Hour, IdleTTL: time.Hour})
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ac, err := config.LoadAgent(func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	tc := config.Telegram{Enabled: true, Mode: "mock", Key: bytes.Repeat([]byte{185}, 32), Retention: 24 * time.Hour}
	chat := &conversations.Service{Pool: d.Pool, Config: ac, Provider: agent.Mock{}, RoutingWorkerHash: proof[:]}
	mock := &bookingTelegramMock{calls: 1}
	tg := telegram.Service{Pool: d.Pool, Config: tc, Transport: mock, Chat: chat}
	waKey := bytes.Repeat([]byte{186}, 32)
	igConfig := config.Instagram{Enabled: true, Key: bytes.Repeat([]byte{187}, 32), AppSecret: "mock:inbox-app-secret", VerifyToken: "mock:inbox-verify-token"}
	wa := whatsapp.Service{Pool: d.Pool, Key: waKey, Chat: chat}
	ig := instagram.Service{Pool: d.Pool, Config: igConfig, Chat: chat}
	handler := httpapi.NewHandlerWithChannels(d, log, 3*time.Second, 65536, identity, ac, agent.Mock{}, tc, config.WhatsApp{Key: waKey}, igConfig)
	owner := seedStage23Owner(t, d, admin, ctx, identity, 780)
	operator := seedStage23Owner(t, d, admin, ctx, identity, 781)
	foreign := seedStage23Owner(t, d, admin, ctx, identity, 782)
	org := owner.tenant.OrganizationID
	type payload = map[string]any
	endpoint := func(who stage23Owner, method, path string, body payload, code int) payload {
		t.Helper()
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(method, "/api/v1/organizations/"+org+path, bytes.NewReader(raw)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", origin)
		req.Header.Set("X-CSRF-Token", who.csrf)
		req.AddCookie(&http.Cookie{Name: "aba_session", Value: who.token})
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != code {
			t.Fatalf("%s %s: got %d want %d: %s", method, path, res.Code, code, res.Body.String())
		}
		var out payload
		if res.Body.Len() > 0 && json.Unmarshal(res.Body.Bytes(), &out) != nil {
			t.Fatal("invalid JSON")
		}
		return out
	}
	endpoint(owner, "POST", "/inbox", payload{}, 200)
	endpoint(operator, "GET", "/team", nil, 403)
	runSQL("UPDATE platform.organizations SET staff_limit=1 WHERE id=$1", org)
	invite := func(who stage23Owner) payload {
		t.Helper()
		token := endpoint(owner, "POST", "/team", payload{"action": "invite", "email": who.tenant.Session.User.Email, "operator": true}, 200)["token"].(string)
		return payload{"action": "accept", "token": token}
	}
	accepted := invite(operator)
	endpoint(foreign, "POST", "/team", accepted, 409)
	endpoint(operator, "POST", "/team", accepted, 200)
	endpoint(operator, "POST", "/team", accepted, 409)
	endpoint(foreign, "POST", "/team", invite(foreign), 409)
	endpoint(operator, "GET", "/workspace-usage", nil, 403)

	if _, err = tg.Connect(ctx, owner.tenant, "mock:78390"); err != nil {
		t.Fatal(err)
	}
	var integration string
	if err = admin.QueryRow(ctx, "SELECT id FROM platform.telegram_integrations WHERE organization_id=$1", org).Scan(&integration); err != nil {
		t.Fatal(err)
	}
	secret := auth.Token()
	runSQL("UPDATE platform.telegram_integrations SET webhook_hash=$2 WHERE id=$1", integration, auth.Hash(secret))
	const sender = 78401
	webhook := func(number int, tag string, code int) {
		t.Helper()
		raw, _ := json.Marshal(telegram.Update{ID: int64(number), Message: &telegram.Message{ID: int64(number), From: telegram.User{ID: sender},
			Chat: telegram.Chat{ID: sender, Type: "private"}, Text: "Какие услуги доступны?"}})
		req := httptest.NewRequest("POST", "/api/v1/webhooks/telegram/"+integration, bytes.NewReader(raw)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Telegram-Bot-Api-Secret-Token", tag)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != code {
			t.Fatalf("telegram webhook: %d want %d", res.Code, code)
		}
	}
	webhook(1, "invalid", 403)
	webhook(1, secret, 200)
	webhook(1, secret, 200)
	if worked, err := tg.ProcessOne(ctx); !worked || err != nil {
		t.Fatalf("ingress: %t %v", worked, err)
	}
	if worked, err := tg.ProcessOne(ctx); worked || err != nil {
		t.Fatalf("duplicate ingress: %t %v", worked, err)
	}
	for step := 0; step < 3; step++ {
		if worked, err := manager.ProcessOne(ctx); !worked || err != nil {
			t.Fatalf("manager step %d: %t %v", step, worked, err)
		}
	}
	var conversation, task string
	if err = admin.QueryRow(ctx, "SELECT conversation_id,task_id FROM platform.inbox_reply_drafts WHERE organization_id=$1", org).Scan(&conversation, &task); err != nil {
		t.Fatal("draft required", err)
	}
	getPath := "/conversations/" + conversation + "/inbox"
	decisionPath := getPath + "/" + task
	review := endpoint(owner, "GET", getPath, nil, 200)
	if len(review["drafts"].([]any)) != 1 || len(review["delivery"].([]any)) != 0 || review["customer"].(map[string]any)["channel"] != "telegram" {
		t.Fatal("draft, customer or pre-approval delivery mismatch")
	}
	endpoint(operator, "GET", getPath, nil, 200)
	endpoint(foreign, "GET", getPath, nil, 404)
	decision := payload{"decision": "approved"}
	endpoint(operator, "POST", decisionPath, decision, 403)
	noCSRF := owner
	noCSRF.csrf = ""
	endpoint(noCSRF, "POST", decisionPath, decision, 403)
	endpoint(owner, "POST", decisionPath, decision, 204)
	endpoint(owner, "POST", decisionPath, decision, 204)
	endpoint(owner, "POST", decisionPath, payload{"decision": "rejected"}, 409)
	before := mock.calls
	if worked, err := tg.SendOne(ctx); !worked || err != nil || mock.calls != before+1 || mock.destination != sender {
		t.Fatalf("mock delivery: %t %v", worked, err)
	}
	if worked, err := tg.SendOne(ctx); worked || err != nil {
		t.Fatalf("duplicate delivery: %t %v", worked, err)
	}
	review = endpoint(owner, "GET", getPath, nil, 200)
	if review["delivery"].([]any)[0].(map[string]any)["status"] != "sent" {
		t.Fatal("expected one sent test message")
	}
	change := payload{"assignee": operator.tenant.Session.User.ID, "expected_version": review["version"]}
	endpoint(owner, "POST", getPath+"/assignee", change, 204)
	endpoint(owner, "POST", getPath+"/assignee", change, 409)
	endpoint(owner, "POST", "/team", payload{"action": "member.revoked", "target": operator.tenant.Session.User.ID}, 200)
	endpoint(operator, "GET", getPath, nil, 404)
	if reviewed := endpoint(owner, "GET", getPath, nil, 200); reviewed["assignee_id"] != nil {
		t.Fatal("revoked operator remained assigned")
	}
	usage := endpoint(owner, "GET", "/workspace-usage", nil, 200)
	if usage["tasks_30d"].(float64) < 1 || usage["staff_limit"].(float64) != 1 || usage["staff"].(float64) != 0 || usage["estimated_cost_microrub"] != nil {
		t.Fatalf("invalid usage state %v", usage)
	}
	var actions int
	if err = admin.QueryRow(ctx, "SELECT count(*) FROM platform.agent_audit WHERE task_id=$1 AND action='inbox.reply.review' AND result='approved'", task).Scan(&actions); err != nil || actions != 1 {
		t.Fatal("expected exactly one approval audit", err)
	}
	const grant = "согласен получать предложения по услугам"
	const revoke = "отзываю согласие на предложения по услугам"
	waSecrets := whatsapp.Credentials{AccessToken: "mock:inbox-access-token", AppSecret: "mock:inbox-app-secret", VerifyToken: "mock:inbox-verify-token", AllowedUsers: []string{"78502"}}
	if _, err = wa.Connect(ctx, owner.tenant, whatsapp.ConnectInput{AppID: "78510", BusinessID: "78511", PhoneID: "78512", GraphVersion: "v99.0", Credentials: waSecrets}); err != nil {
		t.Fatal(err)
	}
	injectWA := func(mid, message string) bool {
		out := endpoint(owner, "POST", "/integrations/whatsapp/synthetic-updates", payload{"user_id": "78502", "message_id": mid, "text": message}, 200)
		return out["inserted"].(float64) == 1
	}
	var waID string
	if err = admin.QueryRow(ctx, "SELECT id FROM platform.channel_integrations WHERE organization_id=$1 AND channel='whatsapp'", org).Scan(&waID); err != nil {
		t.Fatal(err)
	}
	waRaw := []byte(fmt.Sprintf(`{"object":"whatsapp_business_account","entry":[{"id":"78511","changes":[{"field":"messages","value":{"metadata":{"phone_number_id":"78512"},"messages":[{"from":"78502","id":"consent-1","timestamp":"%d","type":"text","text":{"body":%q}}]}}]}]}`, time.Now().Unix(), grant))
	signedWebhook := func(path, secret string, raw []byte) {
		t.Helper()
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(raw)
		signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		invalid := "sha256=" + hex.EncodeToString(make([]byte, sha256.Size))
		for _, test := range []struct {
			signature string
			code      int
		}{{invalid, 403}, {signature, 200}, {signature, 200}} {
			req := httptest.NewRequest("POST", path, bytes.NewReader(raw)).WithContext(ctx)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Hub-Signature-256", test.signature)
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			if res.Code != test.code {
				t.Fatalf("%s: got %d want %d: %s", path, res.Code, test.code, res.Body.String())
			}
		}
	}
	signedWebhook("/api/v1/webhooks/whatsapp/"+waID, waSecrets.AppSecret, waRaw)
	if worked, err := wa.ProcessOne(ctx); !worked || err != nil {
		t.Fatalf("WhatsApp ingress: %t %v", worked, err)
	}
	if worked, err := wa.ProcessOne(ctx); worked || err != nil {
		t.Fatalf("WhatsApp duplicate: %t %v", worked, err)
	}

	setup, err := ig.Start(ctx, owner.tenant)
	if err != nil {
		t.Fatal(err)
	}
	callback, err := url.Parse(setup["callback_url"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if err = ig.Callback(ctx, owner.tenant, callback.Query()); err != nil {
		t.Fatal(err)
	}
	igID := setup["integration_id"].(string)
	var account string
	if err = admin.QueryRow(ctx, "SELECT account_id FROM platform.instagram_connections WHERE integration_id=$1", igID).Scan(&account); err != nil {
		t.Fatal(err)
	}
	igRaw := []byte(fmt.Sprintf(`{"object":"instagram","entry":[{"id":%q,"messaging":[{"sender":{"id":"78503"},"recipient":{"id":%q},"timestamp":%d,"message":{"mid":"ig-consent-1","text":%q}}]}]}`, account, account, time.Now().UnixMilli(), grant))
	signedWebhook("/api/v1/webhooks/instagram/"+igID, igConfig.AppSecret, igRaw)
	if worked, err := ig.ProcessOne(ctx); !worked || err != nil {
		t.Fatalf("Instagram worker: %t %v", worked, err)
	}
	if worked, err := ig.ProcessOne(ctx); worked || err != nil {
		t.Fatalf("Instagram duplicate: %t %v", worked, err)
	}
	if worked, err := ig.SendOne(ctx); worked || err != nil {
		t.Fatalf("Instagram transport must stay disabled: %t %v", worked, err)
	}

	channels := map[string]string{}
	for _, item := range endpoint(owner, "GET", "/conversations", nil, 200)["items"].([]any) {
		conv := item.(map[string]any)
		channels[conv["channel"].(string)] = conv["id"].(string)
	}
	for _, name := range []string{"telegram", "whatsapp", "instagram"} {
		path := "/conversations/" + channels[name] + "/inbox"
		if channels[name] == "" {
			t.Fatalf("Inbox missing %s", name)
		}
		inspect := endpoint(owner, "GET", path, nil, 200)
		if inspect["customer"].(map[string]any)["channel"] != name {
			t.Fatalf("wrong channel for %s", name)
		}
		endpoint(foreign, "GET", path, nil, 404)
		if name == "telegram" {
			continue
		}
		events := inspect["consents"].([]any)
		if len(events) != 1 || events[0].(map[string]any)["decision"] != "granted" {
			t.Fatalf("one consent required for %s: %v", name, events)
		}
	}
	waConv := channels["whatsapp"]
	template, job, capability := auth.UUID(), auth.UUID(), auth.UUID()
	runSQL(`INSERT INTO platform.wa_templates(id,organization_id,integration_id,waba_id,name,language,category,status,components,parameter_schema,verified_at,verified_until,source,policy_version,synthetic)
	VALUES($1,$2,$3,'78511','consent_fixture','ru','MARKETING','APPROVED','[{"type":"BODY","text":"Test"}]','[]',now(),now()+interval '1 day','SYNTHETIC_MOCK','synthetic-v1',true)`, template, org, waID)
	queue := `INSERT INTO platform.channel_outbox(id,organization_id,integration_id,conversation_id,external_user_id,role,payload,status,capability,lease_until,created_at)
	VALUES($1,$2,$3,$4,'78502','staff',jsonb_build_object('template_id',$5::text,'parameters','[]'::jsonb,'text',''),'sending',$6,clock_timestamp()+interval '30 seconds',clock_timestamp())`
	runSQL(queue, job, org, waID, waConv, template, capability)
	checkDispatch := func(available bool) {
		t.Helper()
		var raw []byte
		if err := d.Pool.QueryRow(ctx, "SELECT platform.wa_dispatch_template($1,$2)", wa.Proof(), capability).Scan(&raw); err != nil || (len(raw) > 0) != available {
			t.Fatalf("template dispatch allowed=%t, want %t, err %v", len(raw) > 0, available, err)
		}
	}
	checkDispatch(true)
	if !injectWA("consent-2", revoke) {
		t.Fatal("revocation was not queued")
	}
	if worked, err := wa.ProcessOne(ctx); !worked || err != nil {
		t.Fatalf("WhatsApp revocation: %t %v", worked, err)
	}
	checkDispatch(false)
	if _, err = admin.Exec(ctx, queue, auth.UUID(), org, waID, waConv, template, auth.UUID()); err == nil {
		t.Fatal("new marketing delivery allowed after withdrawal")
	}
	if !injectWA("consent-3", grant) {
		t.Fatal("new consent was not queued")
	}
	if worked, err := wa.ProcessOne(ctx); !worked || err != nil {
		t.Fatalf("WhatsApp regrant: %t %v", worked, err)
	}
	checkDispatch(false)
	history := endpoint(owner, "GET", "/conversations/"+waConv+"/inbox", nil, 200)["consents"].([]any)
	if len(history) != 3 || history[0].(map[string]any)["decision"] != "granted" || history[1].(map[string]any)["decision"] != "revoked" {
		t.Fatalf("invalid consent history: %v", history)
	}
	t.Log("Unified Telegram, WhatsApp and Instagram Inbox; signature rejection, deduplication, staff approval, tenant isolation, consent withdrawal and no revival after regrant")

}
