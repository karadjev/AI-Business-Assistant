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
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/agent"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/agentruntime"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	httpapi "github.com/karadjev/AI-Business-Assistant/apps/api/internal/http"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/db"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/migrate"
)

func TestCommercialAdmissionIsolated(t *testing.T) {
	if os.Getenv("ABA_TEST_COMMERCIAL38") != "1" {
		t.Skip("requires isolated commercial runner")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	dsn := testDSN(t, "ABA_TEST_MIGRATION_DATABASE_URL", "aba_migrator")
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	var safe bool
	err = admin.QueryRow(ctx, `SELECT marker='aba-synthetic-isolated' AND NOT EXISTS(
		SELECT 1 FROM information_schema.tables WHERE table_schema='platform' AND table_name<>'test_safety')
		FROM platform.test_safety WHERE singleton`).Scan(&safe)
	if err != nil || !safe {
		t.Fatal("fresh isolated database required")
	}
	for _, op := range []string{"up", "down", "up"} {
		if err = migrate.Run(ctx, dsn, op); err != nil {
			t.Fatal(op, err)
		}
	}
	database, err := db.Open(ctx, testDSN(t, "ABA_TEST_DATABASE_URL", "aba_app"), 8, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err = database.Ready(ctx); err != nil {
		t.Fatal(err)
	}
	identity := auth.New(database.Pool, config.Auth{Origin: "http://127.0.0.1:3000", EmailKey: bytes.Repeat([]byte{78}, 32), SessionTTL: time.Hour, IdleTTL: time.Hour})
	ac, err := config.LoadAgent(func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	channelConfig := config.Telegram{Enabled: true, Mode: "mock", Key: bytes.Repeat([]byte{79}, 32), Retention: 24 * time.Hour}
	wa := config.WhatsApp{Key: bytes.Repeat([]byte{80}, 32)}
	handler := httpapi.NewHandlerWithChannels(database, slog.New(slog.NewTextHandler(io.Discard, nil)), 5*time.Second, 65536, identity, ac, agent.Mock{}, channelConfig, wa)
	owner := seedStage23Owner(t, database, admin, ctx, identity, 839)
	foreign := seedStage23Owner(t, database, admin, ctx, identity, 840)
	org := owner.tenant.OrganizationID
	type body = map[string]any
	type result struct {
		code int
		text string
	}
	request := func(actor stage23Owner, method, path string, payload body) result {
		raw, _ := json.Marshal(payload)
		r := httptest.NewRequest(method, "/api/v1/organizations/"+org+path, bytes.NewReader(raw)).WithContext(ctx)
		r.Header.Set("Origin", "http://127.0.0.1:3000")
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Token", actor.csrf)
		r.AddCookie(&http.Cookie{Name: "aba_session", Value: actor.token})
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, r)
		return result{rec.Code, rec.Body.String()}
	}
	expect := func(got result, code int, text string) {
		t.Helper()
		if got.code != code || !strings.Contains(got.text, text) {
			t.Fatalf("status=%d expected=%d body=%s", got.code, code, got.text)
		}
	}
	configure := body{"plan_code": "custom", "status": "active", "expected_version": 0, "staff_limit": 0, "agent_limit": 1, "channel_limit": 1, "task_limit": 1, "ai_monthly_cap": 10}
	expect(request(owner, "GET", "/commercial", nil), 200, `"can_edit":false`)
	expect(request(owner, "PUT", "/commercial", configure), 403, "FORBIDDEN")
	expect(request(foreign, "GET", "/commercial", nil), 403, "FORBIDDEN")
	if _, err = admin.Exec(ctx, "INSERT INTO platform.commercial_admin_grants(organization_id,user_id) VALUES($1,$2)", org, owner.tenant.Session.User.ID); err != nil {
		t.Fatal(err)
	}
	noCSRF := owner
	noCSRF.csrf = ""
	expect(request(noCSRF, "PUT", "/commercial", configure), 403, "CSRF")
	expect(request(owner, "PUT", "/commercial", configure), 200, `"plan_code":"custom"`)
	expect(request(owner, "PUT", "/commercial", configure), 409, "VERSION_CONFLICT")

	invite := request(owner, "POST", "/team", body{"action": "invite", "email": foreign.tenant.Session.User.Email, "operator": true})
	expect(invite, 200, "token")
	var issued map[string]string
	if err = json.Unmarshal([]byte(invite.text), &issued); err != nil {
		t.Fatal(err)
	}
	expect(request(foreign, "POST", "/team", body{"action": "accept", "token": issued["token"]}), 429, "COMMERCIAL_LIMIT")

	task := func(agentID, action, key string) result {
		return request(owner, "POST", "/agent-tasks", body{"agent_id": agentID, "action": action, "idempotency_key": key})
	}
	type response struct {
		key string
		got result
	}
	calls := make(chan response, 2)
	for i := 0; i < 2; i++ {
		go func() { key := auth.UUID(); calls <- response{key, task("business_manager", "plan", key)} }()
	}
	first, second := <-calls, <-calls
	if first.got.code != 202 {
		first, second = second, first
	}
	expect(first.got, 202, "id")
	expect(second.got, 429, "COMMERCIAL_LIMIT")
	expect(task("business_manager", "plan", first.key), 202, first.got.text)
	configure["expected_version"] = 1
	configure["task_limit"] = 4
	expect(request(owner, "PUT", "/commercial", configure), 200, `"task_limit":4`)
	expect(task("analytics", "read", auth.UUID()), 429, "агентов")
	expect(task("business_manager", "plan", auth.UUID()), 202, "id")

	// Use the real connection APIs with synthetic credentials; no provider sends or calls.
	expect(request(owner, "POST", "/integrations/telegram/connect", body{"token": "mock:839001"}), 200, "mock")
	expect(request(owner, "POST", "/integrations/whatsapp/connect", body{
		"app_id": "1893", "business_account_id": "1894", "phone_number_id": "1895", "graph_api_version": "v0.0",
		"credentials": body{"access_token": "mock:commercial-access", "app_secret": "mock:commercial-secret",
			"verify_token": "mock:commercial-verify", "allowed_users": []string{"8390"}},
	}), 429, "каналов")

	reserve := func(amount int) error {
		_, err := admin.Exec(ctx, `INSERT INTO platform.llm_reservations(id,organization_id,conversation_id,amount,input_cap,output_cap,
			attempts,rate_input,rate_output,model,day_key,month_key) VALUES(gen_random_uuid(),$1,$2,$3,1,1,1,1,1,'synthetic',
			'day:'||to_char(now() AT TIME ZONE 'UTC','YYYY-MM-DD'),'month:'||to_char(now() AT TIME ZONE 'UTC','YYYY-MM'))`, org, owner.conversation, amount)
		return err
	}
	if err = reserve(6); err != nil {
		t.Fatal(err)
	}
	if err = reserve(5); err == nil {
		t.Fatal("AI cap accepted excess reservation")
	} else if p, ok := err.(*pgconn.PgError); !ok || p.Code != "P4L01" || p.Detail != "ai" {
		t.Fatal(err)
	}
	expect(request(owner, "GET", "/commercial", nil), 200, `"ai_exposure":6`)
	configure["expected_version"] = 2
	configure["status"] = "paused"
	expect(request(owner, "PUT", "/commercial", configure), 200, `"status":"paused"`)
	expect(task("business_manager", "plan", auth.UUID()), 429, "приостановлена")
	// Model a specialist enqueue failing during completion, after the task has been claimed.
	// This trigger is installed only in the isolated test database.
	_, err = admin.Exec(ctx, `CREATE FUNCTION platform.test_reject_commercial_finish() RETURNS trigger LANGUAGE plpgsql AS $$
	BEGIN RAISE EXCEPTION 'synthetic commercial admission' USING ERRCODE='P4L01',DETAIL='tasks'; END $$;
	CREATE TRIGGER test_reject_commercial_finish AFTER UPDATE OF status ON platform.agent_executions
	FOR EACH ROW WHEN(NEW.status='completed') EXECUTE FUNCTION platform.test_reject_commercial_finish();`)
	if err != nil {
		t.Fatal(err)
	}
	worker := agentruntime.Service{Pool: database.Pool, Key: identity.Config.RetentionKey()}
	if worked, err := worker.ProcessOne(ctx); !worked || err != nil {
		t.Fatalf("quota reached after claim: worked=%t %v", worked, err)
	}
	var terminal int
	if err = admin.QueryRow(ctx, `SELECT count(*) FROM platform.agent_tasks
	WHERE organization_id=$1 AND status='failed' AND error_code='denied'`, org).Scan(&terminal); err != nil || terminal != 1 {
		t.Fatalf("expected terminal worker denial without a retry: %d %v", terminal, err)
	}
	var events int
	if err = admin.QueryRow(ctx, "SELECT count(*) FROM platform.commercial_audit WHERE organization_id=$1", org).Scan(&events); err != nil || events != 3 {
		t.Fatalf("audit=%d err=%v", events, err)
	}
}
