//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/agent"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/agentruntime"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/conversations"
	httpapi "github.com/karadjev/AI-Business-Assistant/apps/api/internal/http"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/db"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/migrate"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/telegram"
)

func TestWorkshopManagerAndTelegramIngress(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	migrationDSN := testDSN(t, "ABA_TEST_MIGRATION_DATABASE_URL", "aba_migrator")
	appDSN := testDSN(t, "ABA_TEST_DATABASE_URL", "aba_app")
	admin, err := pgx.Connect(ctx, migrationDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	var marker string
	if err = admin.QueryRow(ctx, `SELECT marker FROM platform.test_safety WHERE singleton`).Scan(&marker); err != nil || marker != "aba-synthetic-isolated" {
		t.Fatal("refusing to use database without isolation marker")
	}
	if err = migrate.Run(ctx, migrationDSN, "up"); err != nil {
		t.Fatal(err)
	}
	if err = migrate.Run(ctx, migrationDSN, "down"); err != nil {
		t.Fatalf("empty 36 → 35: %v", err)
	}
	if err = migrate.Run(ctx, migrationDSN, "up"); err != nil {
		t.Fatalf("35 → 36: %v", err)
	}
	d, err := db.Open(ctx, appDSN, 8, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err = d.Ready(ctx); err != nil {
		t.Fatal(err)
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := admin.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	key := bytes.Repeat([]byte{122}, 32)
	const origin = "http://workshop.example.test"
	identity := auth.New(d.Pool, config.Auth{Origin: origin, EmailKey: key, SessionTTL: time.Hour, IdleTTL: time.Hour})
	worker := agentruntime.Service{Pool: d.Pool, Key: key}
	handler := httpapi.NewHandler(d, slog.New(slog.NewTextHandler(io.Discard, nil)), 5*time.Second, 4096, identity)
	orgs := organizations.Service{Pool: d.Pool}
	type actor struct {
		token, csrf string
		session     auth.Session
	}
	seedActor := func() actor {
		t.Helper()
		uid := auth.UUID()
		out := actor{token: auth.Token(), csrf: auth.Token()}
		exec(`INSERT INTO platform.users(id,email,password_hash,display_name,email_verified_at) VALUES($1,$2,'synthetic','Workshop fixture',now())`, uid, uid+"@workshop.example.test")
		exec(`INSERT INTO platform.sessions(id,user_id,token_hash,csrf_hash,expires_at,idle_expires_at)
		  VALUES($1,$2,$3,$4,now()+interval '1 hour',now()+interval '1 hour')`, auth.UUID(), uid, auth.Hash(out.token), auth.Hash(out.csrf))
		var err error
		out.session, err = identity.Authenticate(ctx, out.token)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	owner, other := seedActor(), seedActor()
	create := func(user actor) organizations.Organization {
		t.Helper()
		org, err := orgs.Create(ctx, user.session, "Isolated workshop", "Europe/Moscow")
		if err != nil || org.IsTest {
			t.Fatalf("real-mode organization: %v", err)
		}
		exec(`UPDATE platform.organizations SET business_model='hybrid',industry_profile='vehicle_service' WHERE id=$1`, org.ID)
		return org
	}
	org, foreign := create(owner), create(other)
	demo, err := orgs.CreateTest(ctx, owner.session, "Isolated demo")
	if err != nil {
		t.Fatal(err)
	}
	call := func(user actor, id, method, path string, body any, want int) []byte {
		t.Helper()
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, "/api/v1/organizations/"+id+"/workshop"+path, bytes.NewReader(b)).WithContext(ctx)
		req.AddCookie(&http.Cookie{Name: "aba_session", Value: user.token})
		req.Header.Set("Origin", origin)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-CSRF-Token", user.csrf)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("%s %s: want %d got %d %s", method, path, want, w.Code, w.Body.String())
		}
		return w.Body.Bytes()
	}
	for _, id := range []string{org.ID, foreign.ID} {
		exec("INSERT INTO platform.workshop_admissions(organization_id) VALUES($1)", id)
	}
	foreignMember := seedActor()
	exec("INSERT INTO platform.organization_members(organization_id,user_id,role,can_manage_orders) VALUES($1,$2,'member',true)", org.ID, foreignMember.session.User.ID)
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(call(owner, org.ID, "POST", "/customers", map[string]string{"name": "Synthetic Customer"}, 201), &out); err != nil || out.ID == "" {
		t.Fatalf("customer: %v", err)
	}
	tenant := catalog.Tenant{OrganizationID: org.ID, Session: owner.session}
	part := seedInstagramCatalog(t, d, ctx, tenant, 3601)
	begin := agentruntime.ManagerMessage{ID: auth.UUID(), Text: "Пожалуйста, оформи новую заявку на обслуживание"}
	call(owner, org.ID, "POST", "/manager", begin, 202)
	if raw := call(owner, org.ID, "GET", "/manager", nil, 200); !bytes.Contains(raw, []byte("Уточните данные")) {
		t.Fatal("no clarification")
	}
	deny := owner
	deny.csrf = ""
	message := agentruntime.ManagerMessage{ID: auth.UUID(), Text: "Клиент: Synthetic Customer; артикул: " + part.SKU + "; автомобиль: условный автомобиль; услуга: проверка фильтра; количество: 1 шт."}
	call(deny, org.ID, "POST", "/manager", message, 403)
	call(foreignMember, org.ID, "POST", "/manager", message, 403)
	call(owner, demo.ID, "POST", "/manager", message, 403)
	call(owner, org.ID, "POST", "/manager", message, 202)
	call(owner, org.ID, "POST", "/manager", message, 202)
	message.RequestID = auth.UUID()
	call(owner, org.ID, "POST", "/manager", message, 409)
	call(owner, org.ID, "POST", "/manager", agentruntime.ManagerMessage{ID: auth.UUID(), Text: "Не нужно создавать новую заявку"}, 202)
	turns := call(owner, org.ID, "GET", "/manager", nil, 200)
	var history []struct {
		TaskID *string `json:"task_id"`
	}
	if err := json.Unmarshal(turns, &history); err != nil || len(history) != 3 || history[1].TaskID == nil || history[2].TaskID != nil {
		t.Fatalf("history rebinding or duplication: %v", err)
	}
	if raw := call(other, foreign.ID, "GET", "/manager", nil, 200); !bytes.Equal(bytes.TrimSpace(raw), []byte("[]")) {
		t.Fatal("cross tenant history")
	}
	call(other, org.ID, "GET", "/manager", nil, 404)
	// Simulate a terminal read-only specialist failure in the isolated queue.
	request := *history[1].TaskID
	exec(`UPDATE platform.agent_tasks SET status='failed',error_code='denied',completed_at=clock_timestamp()
	 WHERE organization_id=$1 AND parent_id=$2 AND delegation_step=1`, org.ID, request)
	exec(`UPDATE platform.agent_tasks SET status='failed',error_code='denied',completed_at=clock_timestamp()
	 WHERE organization_id=$1 AND id=$2`, org.ID, request)
	exec(`UPDATE platform.channel_inbox SET status='failed',attempts=1
	 WHERE organization_id=$1 AND agent_task_id IN (SELECT id FROM platform.agent_tasks WHERE organization_id=$1 AND (id=$2 OR parent_id=$2))`, org.ID, request)
	retry := agentruntime.ManagerMessage{ID: auth.UUID(), Text: "Повтори обработку заявки", RequestID: request}
	call(owner, org.ID, "POST", "/manager", retry, 202)
	call(owner, org.ID, "POST", "/manager", retry, 202)
	step := func() {
		t.Helper()
		if did, err := worker.ProcessOne(ctx); !did || err != nil {
			t.Fatalf("worker: %t %v", did, err)
		}
	}
	for range 4 {
		step()
	}
	var id, hash string
	var reserve int
	if err := admin.QueryRow(ctx, `SELECT a.id,a.parameters_hash,(SELECT quantity_reserved FROM platform.inventory WHERE organization_id=$1 AND variant_id=$3)
	 FROM platform.workshop_requests r JOIN platform.agent_approvals a ON a.organization_id=r.organization_id AND a.task_id=r.reserve_task_id
	 WHERE r.organization_id=$1 AND r.id=$2 AND a.status='pending'`, org.ID, *history[1].TaskID, part.VariantID).Scan(&id, &hash, &reserve); err != nil || reserve != 0 {
		t.Fatalf("approval: %v reserve=%d", err, reserve)
	}
	call(owner, org.ID, "POST", "/approvals/"+id+"/decision", map[string]string{"decision": "approved", "parameters_hash": hash}, 204)
	step()
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM platform.inventory_movements WHERE organization_id=$1 AND workshop_request_id=$2 AND kind='reserve'`, org.ID, *history[1].TaskID).Scan(&reserve); err != nil || reserve != 1 {
		t.Fatalf("reserve: %d %v", reserve, err)
	}
	for _, field := range []string{"overview", "tasks", "reserved", "available_skus", "paid_count"} {
		if raw := call(owner, org.ID, "GET", "", nil, 200); !bytes.Contains(raw, []byte(field)) {
			t.Fatalf("dashboard missing %s", field)
		}
	}

	call(owner, org.ID, "POST", "/manager", agentruntime.ManagerMessage{ID: auth.UUID(), Text: "Какие поступления?", RequestID: ""}, 202)
	if raw := call(owner, org.ID, "GET", "/manager", nil, 200); !bytes.Contains(raw, []byte("Подтверждённых финансовых данных пока нет")) {
		t.Fatal("unknown finance substituted")
	}
	call(owner, org.ID, "POST", "/manager", agentruntime.ManagerMessage{ID: auth.UUID(), Text: "Какой статус заявки?", RequestID: request}, 202)
	if raw := call(owner, org.ID, "GET", "/manager", nil, 200); !bytes.Contains(raw, []byte("деталь в резерве")) {
		t.Fatal("stale status")
	}

	// Existing adapter only. No live Telegram, no worker send call.
	cfg, _ := config.LoadAgent(func(string) string { return "" })
	tc := config.Telegram{Mode: "mock", Key: bytes.Repeat([]byte{81}, 32), Retention: 7 * 24 * time.Hour, Enabled: true}
	chat := &conversations.Service{Pool: d.Pool, Config: cfg, Provider: agent.Mock{}}
	tg := telegram.Service{Pool: d.Pool, Config: tc, Transport: telegram.Mock{}, Chat: chat}
	demoTenant := catalog.Tenant{OrganizationID: demo.ID, Session: owner.session}
	_, err = tg.Connect(ctx, demoTenant, "mock:61007")
	if err != nil {
		t.Fatal(err)
	}
	_ = seedInstagramCatalog(t, d, ctx, demoTenant, 3602)
	var integration string
	if err = admin.QueryRow(ctx, `SELECT id FROM platform.telegram_integrations WHERE organization_id=$1`, demo.ID).Scan(&integration); err != nil {
		t.Fatal(err)
	}
	secret := auth.Token()
	exec(`UPDATE platform.telegram_integrations SET webhook_hash=$2 WHERE organization_id=$1 AND id=$3`, demo.ID, auth.Hash(secret), integration)
	web := httpapi.NewHandlerWithAgent(d, slog.New(slog.NewTextHandler(io.Discard, nil)), 5*time.Second, 4096, identity, cfg, agent.Mock{}, tc)
	body := []byte(`{"update_id":1,"message":{"message_id":1,"from":{"id":1234},"chat":{"id":1234,"type":"private"},"text":"Есть ли SYNTH-IG-3602?"}}`)
	deliver := func(header string, want int) {
		t.Helper()
		req := httptest.NewRequest("POST", "/api/v1/webhooks/telegram/"+integration, bytes.NewReader(body)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Telegram-Bot-Api-Secret-Token", header)
		w := httptest.NewRecorder()
		web.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("Telegram response: %d", w.Code)
		}
	}
	deliver(auth.Token(), 403)
	deliver(secret, 200)
	deliver(secret, 200)
	if did, err := tg.ProcessOne(ctx); !did || err != nil {
		t.Fatalf("Telegram inbound: %t %v", did, err)
	}
	if did, err := tg.ProcessOne(ctx); did || err != nil {
		t.Fatalf("Telegram duplicate: %t %v", did, err)
	}
	var inbound, messages, sent int
	if err = admin.QueryRow(ctx, `SELECT
	  (SELECT count(*) FROM platform.telegram_inbox WHERE integration_id=$1 AND organization_id=$2 AND status='done'),
	  (SELECT count(*) FROM platform.messages WHERE organization_id=$2 AND role='customer' AND content LIKE '%SYNTH-IG-3602%'),
	  (SELECT count(*) FROM platform.telegram_outbox WHERE organization_id=$2 AND status='sent')`, integration, demo.ID).Scan(&inbound, &messages, &sent); err != nil || inbound != 1 || messages != 1 || sent != 0 {
		t.Fatalf("Telegram evidence: %d %d %d %v", inbound, messages, sent, err)
	}
}
