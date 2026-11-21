//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/agentruntime"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	httpapi "github.com/karadjev/AI-Business-Assistant/apps/api/internal/http"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/db"
)

func testStage23CollaborationHTTP(t *testing.T, d *db.Database, admin *pgx.Conn, ctx context.Context) {
	const origin = "http://127.0.0.1:3000"
	key := bytes.Repeat([]byte{81}, 32)
	identity := auth.New(d.Pool, config.Auth{EmailKey: key, Origin: origin, SessionTTL: time.Hour, IdleTTL: time.Hour})
	user, token, csrf := auth.UUID(), auth.Token(), auth.Token()
	if _, err := admin.Exec(ctx, `INSERT INTO platform.users(id,email,password_hash,display_name,email_verified_at) VALUES($1,$2,'synthetic','Agent approval',now())`, user, user+"@agent-approval.example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO platform.sessions(id,user_id,token_hash,csrf_hash,expires_at,idle_expires_at) VALUES($1,$2,$3,$4,now()+interval '1 hour',now()+interval '1 hour')`, auth.UUID(), user, auth.Hash(token), auth.Hash(csrf)); err != nil {
		t.Fatal(err)
	}
	session, err := identity.Authenticate(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	org, err := (organizations.Service{Pool: d.Pool}).CreateTest(ctx, session, "Synthetic manager")
	if err != nil {
		t.Fatal(err)
	}
	h := httpapi.NewHandler(d, slog.New(slog.NewTextHandler(io.Discard, nil)), 3*time.Second, 16384, identity)
	root := "/api/v1/organizations/" + org.ID
	request := func(method, path string, input any, authorizedCSRF bool, want int) []byte {
		t.Helper()
		body, _ := json.Marshal(input)
		r := httptest.NewRequest(method, root+path, bytes.NewReader(body))
		r.AddCookie(&http.Cookie{Name: "aba_session", Value: token})
		r.Header.Set("Origin", origin)
		r.Header.Set("Content-Type", "application/json")
		if authorizedCSRF {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s got %d want %d: %s", method, path, w.Code, want, w.Body.String())
		}
		return w.Body.Bytes()
	}
	body := agentruntime.Request{AgentID: "business_manager", Action: "coordinate", IdempotencyKey: auth.UUID()}
	var task struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(request("POST", "/agent-tasks", body, true, 202), &task); err != nil || task.ID == "" {
		t.Fatal("missing parent task", err)
	}
	request("POST", "/agent-tasks", body, true, 202)
	var approvals struct {
		Items []struct {
			ID     string `json:"id"`
			Hash   string `json:"parameters_hash"`
			Status string `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(request("GET", "/agent-approvals", nil, false, 200), &approvals); err != nil || len(approvals.Items) != 1 || approvals.Items[0].Status != "pending" {
		t.Fatal("expected pending approval", err)
	}
	svc := agentruntime.Service{Pool: d.Pool, Key: key}
	run := func(want bool) {
		t.Helper()
		if ran, err := svc.ProcessOne(ctx); err != nil || ran != want {
			t.Fatalf("worker ran=%t want=%t error=%v", ran, want, err)
		}
	}
	run(true)  // analytics
	run(false) // parent waits for approval
	a := approvals.Items[0]
	decision := map[string]string{"decision": "rejected", "parameters_hash": a.Hash}
	path := "/agent-approvals/" + a.ID + "/decision"
	request("POST", path, decision, false, 403)
	request("POST", path, decision, true, 204)
	request("POST", path, decision, true, 409)
	run(true) // parent aggregates completed analytics and cancelled marketing
	run(false)
	var status string
	var result json.RawMessage
	if err := admin.QueryRow(ctx, `SELECT t.status,e.result FROM platform.agent_tasks t JOIN platform.agent_executions e ON e.task_id=t.id WHERE t.id=$1`, task.ID).Scan(&status, &result); err != nil || status != "completed" {
		t.Fatalf("parent status %s: %v", status, err)
	}
	var output struct {
		Children []struct {
			AgentID string `json:"agent_id"`
			Status  string `json:"status"`
		}
	}
	if err := json.Unmarshal(result, &output); err != nil || len(output.Children) != 2 || output.Children[0].AgentID != "analytics" || output.Children[0].Status != "completed" || output.Children[1].AgentID != "marketing" || output.Children[1].Status != "cancelled" {
		t.Fatalf("unexpected child outcomes: %s %v", result, err)
	}
	request("GET", "/agent-audit?task_id="+task.ID, nil, false, 200)

	t.Run("expired approval and mandatory audit", func(t *testing.T) {
		request("POST", "/agent-tasks", agentruntime.Request{AgentID: "marketing", Action: "draft", IdempotencyKey: auth.UUID()}, true, 202)
		var id, hash string
		if err := admin.QueryRow(ctx, `UPDATE platform.agent_approvals SET expires_at=clock_timestamp()-interval '1 second'
 WHERE organization_id=$1 AND status='pending' RETURNING id,parameters_hash`, org.ID).Scan(&id, &hash); err != nil {
			t.Fatal(err)
		}
		request("POST", "/agent-approvals/"+id+"/decision", map[string]string{"decision": "approved", "parameters_hash": hash}, true, 409)
		run(false)
		if err := admin.QueryRow(ctx, "SELECT status FROM platform.agent_approvals WHERE id=$1", id).Scan(&status); err != nil || status != "expired" {
			t.Fatalf("approval expiry %s: %v", status, err)
		}

		request("POST", "/agent-tasks", agentruntime.Request{AgentID: "business_manager", Action: "plan", IdempotencyKey: auth.UUID()}, true, 202)
		lease := auth.UUID()
		var claimed json.RawMessage
		if err := d.Pool.QueryRow(ctx, "SELECT platform.agent_claim($1,$2)", svc.Proof(), lease).Scan(&claimed); err != nil {
			t.Fatal(err)
		}
		var claim struct{ ID, ExecutionID string }
		if err := json.Unmarshal(claimed, &claim); err != nil || claim.ID == "" || claim.ExecutionID == "" {
			t.Fatal("no lease claim", err)
		}
		var accepted bool
		if err := d.Pool.QueryRow(ctx, `SELECT platform.agent_finish($1,$2,$3,'[{}]'::jsonb,NULL,false)`, claim.ID, lease, claim.ExecutionID).Scan(&accepted); err != nil || accepted {
			t.Fatalf("completion without successful audit accepted=%t err=%v", accepted, err)
		}
		if _, err := admin.Exec(ctx, "UPDATE platform.channel_inbox SET lease_until=now()-interval '1 second' WHERE agent_task_id=$1", claim.ID); err != nil {
			t.Fatal(err)
		}
		if binary := os.Getenv("ABA_TEST_AGENT_WORKER_BINARY"); binary != "" {
			testStage23WorkerProcess(t, ctx, admin, binary, key, claim.ID, 2)
		} else {
			t.Log("compiled worker process not provided; exercising the worker handler")
			run(true)
		}
	})
}

func testStage23WorkerProcess(t *testing.T, ctx context.Context, admin *pgx.Conn, binary string, key []byte, taskID string, attempt int) {
	t.Helper()
	lifetime, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(lifetime, binary)
	cmd.Env = []string{
		"APP_ENV=test", "DATABASE_URL=" + testDSN(t, "ABA_TEST_DATABASE_URL", "aba_app"),
		"AUTH_EMAIL_KEY=" + hex.EncodeToString(key), "PUBLIC_ORIGIN=http://127.0.0.1:3000",
		"LLM_PROVIDER=mock", "TELEGRAM_TRANSPORT=mock", "TELEGRAM_SENDING_ENABLED=false",
		"WHATSAPP_LIVE_ENABLED=false", "WHATSAPP_SENDING_KILL_SWITCH=true", "WHATSAPP_MONETARY_BUDGET_RUB=0",
		"INSTAGRAM_ENABLED=false", "INSTAGRAM_LIVE_ENABLED=false",
	}
	var logs bytes.Buffer
	cmd.Stdout, cmd.Stderr = &logs, &logs
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			t.Fatalf("worker exited before completion: %v\n%s", err, logs.String())
		case <-lifetime.Done():
			err := <-done
			t.Fatalf("worker did not recover task: %v\n%s", err, logs.String())
		case <-ticker.C:
			var count int
			err := admin.QueryRow(lifetime, "SELECT count(*) FROM platform.agent_executions WHERE task_id=$1 AND attempt=$2 AND status='completed'", taskID, attempt).Scan(&count)
			if err != nil || count != 1 {
				continue
			}
			if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
				cancel()
				<-done
				t.Fatal(err)
			}
			if err := <-done; err != nil {
				t.Fatalf("worker did not shut down cleanly: %v\n%s", err, logs.String())
			}
			t.Logf("compiled worker committed attempt %d", attempt)
			return
		}
	}
}
