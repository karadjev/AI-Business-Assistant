//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
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
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/specialists"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
)

type stage23Owner struct {
	tenant                    catalog.Tenant
	token, csrf, conversation string
	product                   instagramCatalogFixture
	revenue                   int64
}

func testAgentRuntimeStage23(t *testing.T, d *db.Database, admin *pgx.Conn, ctx context.Context) {
	t.Helper()
	key := bytes.Repeat([]byte{83}, 32)
	svc := agentruntime.Service{Pool: d.Pool, Key: key}
	identity := auth.New(d.Pool, config.Auth{
		Origin: "http://127.0.0.1:3000", CatalogRetentionKey: key,
		SessionTTL: time.Hour, IdleTTL: time.Hour,
	})
	handler := httpapi.NewHandler(d, slog.New(slog.NewTextHandler(io.Discard, nil)), 5*time.Second, 16384, identity)
	owners := make([]stage23Owner, 3)
	for i := range owners {
		owners[i] = seedStage23Owner(t, d, admin, ctx, identity, i)
	}
	a, b, c := owners[0], owners[1], owners[2]

	submit := func(t *testing.T, owner stage23Owner, req agentruntime.Request) string {
		t.Helper()
		if req.IdempotencyKey == "" {
			req.IdempotencyKey = auth.UUID()
		}
		id, err := svc.Submit(ctx, owner.tenant, req)
		if err != nil || !organizations.ValidID(id) {
			t.Fatalf("submit %s/%s returned %q, %v", req.AgentID, req.Action, id, err)
		}
		return id
	}
	process := func(t *testing.T, service agentruntime.Service, want bool) {
		t.Helper()
		if worked, err := service.ProcessOne(ctx); err != nil || worked != want {
			t.Fatalf("ProcessOne = %t, %v, want %t, nil", worked, err, want)
		}
	}
	approvalPath := func(owner stage23Owner) string {
		return "/api/v1/organizations/" + owner.tenant.OrganizationID + "/agent-approvals"
	}
	request := func(t *testing.T, owner stage23Owner, method, path, body string, want int) []byte {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body)).WithContext(ctx)
		r.Header.Set("Origin", identity.Config.Origin)
		r.Header.Set("Content-Type", "application/json")
		if owner.token != "" {
			r.AddCookie(&http.Cookie{Name: "aba_session", Value: owner.token})
		}
		if owner.csrf != "" {
			r.Header.Set("X-CSRF-Token", owner.csrf)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s returned %d, want %d: %s", method, path, w.Code, want, w.Body)
		}
		return w.Body.Bytes()
	}

	t.Run("approval HTTP and one-time execution", func(t *testing.T) {
		draft := agentruntime.Request{AgentID: "marketing", Action: "draft"}
		task := submit(t, a, draft)
		foreignTask := submit(t, b, draft)
		process(t, svc, false)

		path := approvalPath(a)
		request(t, stage23Owner{}, http.MethodGet, path, "", http.StatusUnauthorized)
		request(t, c, http.MethodGet, path, "", http.StatusNotFound)
		var list struct {
			Items []struct {
				ID     string            `json:"id"`
				TaskID string            `json:"task_id"`
				Hash   string            `json:"parameters_hash"`
				Status string            `json:"status"`
				Params map[string]string `json:"parameters"`
			} `json:"items"`
		}
		if err := json.Unmarshal(request(t, a, http.MethodGet, path, "", http.StatusOK), &list); err != nil {
			t.Fatal(err)
		}
		if len(list.Items) != 1 {
			t.Fatalf("owner got %d approvals, want 1", len(list.Items))
		}
		item := list.Items[0]
		if !organizations.ValidID(item.ID) || item.TaskID != task || item.Status != "pending" ||
			len(item.Hash) != 64 || len(item.Params) != 1 || item.Params["action"] != "draft" {
			t.Fatalf("unexpected pending approval: %+v", item)
		}
		var foreignID, foreignHash string
		if err := admin.QueryRow(ctx, `SELECT id,parameters_hash FROM platform.agent_approvals WHERE task_id=$1`, foreignTask).Scan(&foreignID, &foreignHash); err != nil {
			t.Fatal(err)
		}
		decisionPath := path + "/" + item.ID + "/decision"
		body := fmt.Sprintf(`{"decision":"approved","parameters_hash":%q}`, item.Hash)
		noCSRF := a
		noCSRF.csrf = ""
		request(t, noCSRF, http.MethodPost, decisionPath, body, http.StatusForbidden)
		request(t, c, http.MethodPost, decisionPath, body, http.StatusNotFound)
		request(t, b, http.MethodPost, approvalPath(b)+"/"+item.ID+"/decision", body, http.StatusConflict)
		wrongHash := fmt.Sprintf(`{"decision":"approved","parameters_hash":%q}`, strings.Repeat("0", 64))
		request(t, a, http.MethodPost, decisionPath, wrongHash, http.StatusConflict)
		process(t, svc, false)
		request(t, a, http.MethodPost, decisionPath, body, http.StatusNoContent)
		request(t, a, http.MethodPost, decisionPath, body, http.StatusConflict)

		reject := fmt.Sprintf(`{"decision":"rejected","parameters_hash":%q}`, foreignHash)
		request(t, b, http.MethodPost, approvalPath(b)+"/"+foreignID+"/decision", reject, http.StatusNoContent)
		process(t, svc, true)
		process(t, svc, false)
		var result specialists.Draft
		stage23Result(t, admin, ctx, a, task, 1, &result)
		if !result.Draft || result.Text == "" || result.ExternalCommunication {
			t.Fatalf("unexpected marketing result: %+v", result)
		}
		for _, want := range []struct {
			id, task, approval string
			attempts           int
		}{
			{task, "completed", "executed", 1}, {foreignTask, "cancelled", "rejected", 0},
		} {
			var taskStatus, approvalStatus string
			var attempts int
			err := admin.QueryRow(ctx, `SELECT t.status,a.status,
 (SELECT count(*) FROM platform.agent_executions WHERE task_id=t.id)
 FROM platform.agent_tasks t JOIN platform.agent_approvals a ON a.task_id=t.id
 WHERE t.id=$1`, want.id).Scan(&taskStatus, &approvalStatus, &attempts)
			if err != nil || taskStatus != want.task || approvalStatus != want.approval || attempts != want.attempts {
				t.Fatalf("approval outcome = %s/%s/%d, %v; want %+v", taskStatus, approvalStatus, attempts, err, want)
			}
		}
	})

	t.Run("concurrent workers and tenant-scoped results", func(t *testing.T) {
		priceTasks := make([]string, len(owners))
		for i, id := range []string{"customer", "sales", "product_inventory"} {
			owner := owners[i]
			priceTasks[i] = submit(t, owner, agentruntime.Request{
				AgentID: id, Action: "get_product_price", ConversationID: owner.conversation, VariantID: owner.product.VariantID,
			})
		}
		finance := agentruntime.Request{AgentID: "finance", Action: "read", IdempotencyKey: auth.UUID()}
		financeID := submit(t, a, finance)
		if duplicate := submit(t, a, finance); duplicate != financeID {
			t.Fatalf("idempotent submit returned %s, want %s", duplicate, financeID)
		}
		conflict := finance
		conflict.AgentID = "analytics"
		_, err := svc.Submit(ctx, a.tenant, conflict)
		var apiErr *auth.Error
		if !errors.As(err, &apiErr) || apiErr.Status != http.StatusConflict {
			t.Fatalf("changed idempotent request returned %v, want conflict", err)
		}
		analyticsID := submit(t, b, conflict) // The same key remains available in another organization.
		managerID := submit(t, c, agentruntime.Request{AgentID: "business_manager", Action: "plan"})
		process(t, agentruntime.Service{Pool: d.Pool, Key: bytes.Repeat([]byte{84}, 32)}, false)

		type poll struct {
			worked bool
			err    error
		}
		const workers = 8
		start := make(chan struct{})
		results := make(chan poll, workers)
		for i := 0; i < workers; i++ {
			go func() {
				<-start
				worked, err := svc.ProcessOne(ctx)
				results <- poll{worked, err}
			}()
		}
		close(start)
		var worked int
		for i := 0; i < workers; i++ {
			result := <-results
			if result.err != nil {
				t.Errorf("concurrent ProcessOne: %v", result.err)
			}
			if result.worked {
				worked++
			}
		}
		if worked != 6 {
			t.Fatalf("workers processed %d jobs, want 6", worked)
		}
		process(t, svc, false)
		for i, id := range priceTasks {
			var result struct {
				JSON string `json:"result_json"`
			}
			stage23Result(t, admin, ctx, owners[i], id, 1, &result)
			var price struct {
				OK   bool `json:"ok"`
				Data struct {
					Variant  string `json:"variant_id"`
					Amount   int64  `json:"price_amount"`
					Currency string `json:"currency_code"`
				} `json:"data"`
			}
			want := owners[i].product
			if err := json.Unmarshal([]byte(result.JSON), &price); err != nil || !price.OK ||
				price.Data.Variant != want.VariantID || price.Data.Amount != want.Price || price.Data.Currency != "RUB" {
				t.Fatalf("tenant %d price result = %s, %v; want variant %s at %d RUB", i, result.JSON, err, want.VariantID, want.Price)
			}
		}
		for _, tc := range []struct {
			owner stage23Owner
			id    string
		}{{a, financeID}, {b, analyticsID}} {
			var result specialists.ReadResult
			stage23Result(t, admin, ctx, tc.owner, tc.id, 1, &result)
			metrics := make(map[string]specialists.Metric)
			for _, m := range result.Metrics {
				metrics[m.Name] = m
			}
			count := metrics["completed_orders_created_last_30d"]
			revenue := metrics["completed_order_value_last_30d"]
			if result.Scope.TenantID != tc.owner.tenant.OrganizationID || count.Value != 1 || count.Unit != "orders" ||
				revenue.Value != tc.owner.revenue || revenue.Unit != "RUB_minor" {
				t.Fatalf("unexpected tenant projection: %+v", result)
			}
		}
		var plan specialists.ManagementPlan
		stage23Result(t, admin, ctx, c, managerID, 1, &plan)
		if plan.Status != "blocked_shared_budget" || plan.CoordinationEnabled || len(plan.Targets) == 0 {
			t.Fatalf("unexpected manager plan: %+v", plan)
		}
		assertStage23RuntimeRLS(t, d, ctx, a, financeID, analyticsID)
	})

	t.Run("stale lease retry and fencing", func(t *testing.T) {
		id := submit(t, c, agentruntime.Request{AgentID: "analytics", Action: "read"})
		lease := auth.UUID()
		var raw []byte
		if err := d.Pool.QueryRow(ctx, `SELECT platform.agent_claim($1,$2)`, svc.Proof(), lease).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var claim struct{ ID, ExecutionID string }
		if err := json.Unmarshal(raw, &claim); err != nil || claim.ID != id || !organizations.ValidID(claim.ExecutionID) {
			t.Fatalf("claim = %s, %v; want task %s", raw, err, id)
		}
		process(t, svc, false)
		tag, err := admin.Exec(ctx, `UPDATE platform.channel_inbox SET lease_until=clock_timestamp()-interval '1 second'
 WHERE agent_task_id=$1 AND agent_lease=$2 AND status='processing'`, id, lease)
		if err != nil || tag.RowsAffected() != 1 {
			t.Fatalf("expire lease: %v, %v", tag, err)
		}
		assertFenced := func() {
			t.Helper()
			var accepted bool
			err := d.Pool.QueryRow(ctx, `SELECT platform.agent_finish($1,$2,$3,NULL,'execution',false)`,
				id, lease, claim.ExecutionID).Scan(&accepted)
			if err != nil || accepted {
				t.Fatalf("stale finish = %t, %v; want false, nil", accepted, err)
			}
		}
		assertFenced()
		process(t, svc, true)
		assertFenced()
		process(t, svc, false)
		var result specialists.ReadResult
		stage23Result(t, admin, ctx, c, id, 2, &result)
		var status, failure string
		err = admin.QueryRow(ctx, `SELECT status,error_code FROM platform.agent_executions WHERE id=$1`, claim.ExecutionID).Scan(&status, &failure)
		if err != nil || status != "interrupted" || failure != "interrupted" {
			t.Fatalf("original attempt = %s/%s, %v", status, failure, err)
		}
	})
}

func seedStage23Owner(t *testing.T, d *db.Database, admin *pgx.Conn, ctx context.Context, identity *auth.Service, i int) stage23Owner {
	t.Helper()
	uid := auth.UUID()
	owner := stage23Owner{token: auth.Token(), csrf: auth.Token(), revenue: int64(1234 * (i + 1))}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := admin.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO platform.users(id,email,password_hash,display_name,email_verified_at) VALUES($1,$2,'synthetic','Stage 2.3 fixture',now())`,
		uid, uid+"@stage23.example.test")
	exec(`INSERT INTO platform.sessions(id,user_id,token_hash,csrf_hash,expires_at,idle_expires_at)
 VALUES($1,$2,$3,$4,now()+interval '1 hour',now()+interval '1 hour')`, auth.UUID(), uid, auth.Hash(owner.token), auth.Hash(owner.csrf))
	session, err := identity.Authenticate(ctx, owner.token)
	if err != nil {
		t.Fatal(err)
	}
	org, err := (organizations.Service{Pool: d.Pool}).CreateTest(ctx, session, fmt.Sprintf("Synthetic Stage 2.3 %d", i+1))
	if err != nil {
		t.Fatal(err)
	}
	owner.tenant = catalog.Tenant{OrganizationID: org.ID, Session: session}
	chat := conversations.Service{Pool: d.Pool, Provider: agent.Mock{}}
	raw, err := chat.Create(ctx, owner.tenant)
	if err != nil {
		t.Fatal(err)
	}
	var conversation struct{ ID, CustomerID string }
	var created struct {
		ID         string `json:"id"`
		CustomerID string `json:"customer_id"`
	}
	if err = json.Unmarshal(raw, &created); err != nil || !organizations.ValidID(created.ID) || !organizations.ValidID(created.CustomerID) {
		t.Fatalf("seed conversation: %v", err)
	}
	conversation.ID, conversation.CustomerID = created.ID, created.CustomerID
	owner.conversation = conversation.ID
	owner.product = seedInstagramCatalog(t, d, ctx, owner.tenant, 230+i)
	quote := auth.UUID()
	exec(`INSERT INTO platform.order_quotes(id,organization_id,conversation_id,customer_id,items)
 VALUES($1,$2,$3,$4,'[{}]')`, quote, org.ID, conversation.ID, conversation.CustomerID)
	exec(`INSERT INTO platform.orders(id,organization_id,conversation_id,customer_id,quote_id,status,idempotency_key,total_amount)
 VALUES($1,$2,$3,$4,$5,'completed',$6,$7)`, auth.UUID(), org.ID, conversation.ID, conversation.CustomerID, quote, auth.UUID(), owner.revenue)
	return owner
}

func stage23Result(t *testing.T, admin *pgx.Conn, ctx context.Context, owner stage23Owner, id string, attempt int, result any) {
	t.Helper()
	var task, execution, queue string
	var attempts, retries, calls, successes, finishes int
	var raw []byte
	err := admin.QueryRow(ctx, `SELECT t.status,e.status,q.status,q.attempts,t.retry_count,e.tool_calls,e.result,
 (SELECT count(*) FROM platform.agent_audit WHERE task_id=t.id AND execution_id=e.id AND action=t.action AND result='succeeded'),
 (SELECT count(*) FROM platform.agent_audit WHERE task_id=t.id AND action='task.finish' AND result='completed')
 FROM platform.agent_tasks t JOIN platform.agent_executions e ON e.task_id=t.id AND e.organization_id=t.organization_id
 JOIN platform.channel_inbox q ON q.agent_task_id=t.id AND q.organization_id=t.organization_id
 WHERE t.organization_id=$1 AND t.initiator=$2 AND t.id=$3 AND e.attempt=$4`,
		owner.tenant.OrganizationID, owner.tenant.Session.User.ID, id, attempt).
		Scan(&task, &execution, &queue, &attempts, &retries, &calls, &raw, &successes, &finishes)
	if err != nil || task != "completed" || execution != "completed" || queue != "done" ||
		attempts != attempt || retries != attempt-1 || calls != 1 || successes != 1 || finishes != 1 {
		t.Fatalf("task %s result: task/execution/queue %s/%s/%s, attempts/retries %d/%d, calls/successes/finishes %d/%d/%d, error %v",
			id, task, execution, queue, attempts, retries, calls, successes, finishes, err)
	}
	var outputs []json.RawMessage
	if err = json.Unmarshal(raw, &outputs); err != nil || len(outputs) != 1 {
		t.Fatalf("task %s returned %s, %v; want one tool result", id, raw, err)
	}
	if err = json.Unmarshal(outputs[0], result); err != nil {
		t.Fatalf("decode result for task %s: %v", id, err)
	}
}

func assertStage23RuntimeRLS(t *testing.T, d *db.Database, ctx context.Context, owner stage23Owner, ownID, foreignID string) {
	t.Helper()
	tx, err := tenancy.Begin(ctx, d.Pool, owner.tenant, false, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var own, foreign int
	err = tx.QueryRow(ctx, `SELECT count(*) FILTER (WHERE id=$1),count(*) FILTER (WHERE id=$2)
 FROM platform.agent_tasks WHERE id IN ($1,$2)`, ownID, foreignID).Scan(&own, &foreign)
	if err != nil || own != 1 || foreign != 0 {
		t.Fatalf("tenant task visibility = %d own, %d foreign, %v", own, foreign, err)
	}
}
