//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/agentruntime"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/db"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/migrate"
)

func testStage261Delegation(t *testing.T, d *db.Database, admin *pgx.Conn, ctx context.Context) {
	svc := agentruntime.Service{Pool: d.Pool, Key: bytes.Repeat([]byte{107}, 32)}
	identity := auth.New(d.Pool, config.Auth{SessionTTL: time.Hour, IdleTTL: time.Hour})
	owner := seedStage23Owner(t, d, admin, ctx, identity, 74)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := admin.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("UPDATE platform.organizations SET industry_profile='auto_parts' WHERE id=$1", owner.tenant.OrganizationID)
	in := agentruntime.DelegationRequest{ConversationID: owner.conversation, IdempotencyKey: auth.UUID()}
	parent, err := svc.SubmitAutoParts(ctx, owner.tenant, in)
	if err != nil {
		t.Fatal(err)
	}
	step := func(want bool) {
		t.Helper()
		worked, err := svc.ProcessOne(ctx)
		if err != nil || worked != want {
			t.Fatalf("worker worked=%v want=%v error=%v", worked, want, err)
		}
	}
	claim := func() string {
		t.Helper()
		var raw []byte
		if err := d.Pool.QueryRow(ctx, "SELECT platform.agent_claim($1,$2)", svc.Proof(), auth.UUID()).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var job struct{ ID string }
		if json.Unmarshal(raw, &job) != nil || job.ID == "" {
			t.Fatal("expected claim")
		}
		return job.ID
	}
	first := claim()
	var raw []byte
	if err := d.Pool.QueryRow(ctx, "SELECT platform.agent_delegation_context($1,$2)", first, auth.UUID()).Scan(&raw); err != nil || len(raw) != 0 {
		t.Fatalf("foreign lease exposed data: %v", err)
	}
	exec("UPDATE platform.channel_inbox SET lease_until=clock_timestamp()-interval '1 second' WHERE id=$1", first)
	step(true)
	var attempts, completed int
	if err := admin.QueryRow(ctx, "SELECT count(*),count(*) FILTER(WHERE status='completed') FROM platform.agent_executions WHERE task_id=$1", first).Scan(&attempts, &completed); err != nil || attempts != 2 || completed != 1 {
		t.Fatalf("retry attempts=%d completed=%d: %v", attempts, completed, err)
	}
	second := claim()
	exec("UPDATE platform.channel_inbox SET attempts=3,lease_until=clock_timestamp()-interval '1 second' WHERE id=$1", second)
	step(false)
	step(true)
	step(false)
	var parentStatus, queueStatus string
	err = admin.QueryRow(ctx, `SELECT t.status,q.status,e.result FROM platform.agent_tasks t
 JOIN platform.channel_inbox q ON q.id=t.id JOIN platform.agent_executions e ON e.task_id=t.id WHERE t.id=$1`, parent).Scan(&parentStatus, &queueStatus, &raw)
	var aggregate []struct {
		State    string
		Children []struct {
			Status string
			Result json.RawMessage
		}
	}
	if err != nil || parentStatus != "failed" || queueStatus != "failed" || json.Unmarshal(raw, &aggregate) != nil || len(aggregate) != 1 || aggregate[0].State != "partial_failure" || len(aggregate[0].Children) != 5 || aggregate[0].Children[0].Status != "completed" || len(aggregate[0].Children[0].Result) == 0 || aggregate[0].Children[1].Status != "failed" || aggregate[0].Children[4].Status != "cancelled" {
		var audit, states string
		_ = admin.QueryRow(ctx, `SELECT coalesce(string_agg(action||':'||result||':'||coalesce(error_code,''),',' ORDER BY created_at),'') FROM platform.agent_audit WHERE task_id=$1`, parent).Scan(&audit)
		_ = admin.QueryRow(ctx, `SELECT coalesce(string_agg(action||':'||status,',' ORDER BY delegation_step),'') FROM platform.agent_tasks WHERE parent_id=$1`, parent).Scan(&states)
		t.Logf("audit=%s child states=%s", audit, states)
		t.Fatalf("failed parent aggregation state=%s queue=%s error=%v result=%s", parentStatus, queueStatus, err, raw)
	}
}

// TestDelegationIsolated permits a focused recovery regression on a disposable schema.
func TestDelegationIsolated(t *testing.T) {
	if os.Getenv("ABA_TEST_ONLY_DELEGATION") != "1" {
		t.Skip("requires the focused disposable runner")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	migrationDSN := testDSN(t, "ABA_TEST_MIGRATION_DATABASE_URL", "aba_migrator")
	admin, err := pgx.Connect(ctx, migrationDSN)
	if err != nil {
		t.Fatal("cannot open isolated fixture")
	}
	defer admin.Close(context.Background())
	var safe bool
	err = admin.QueryRow(ctx, `SELECT marker='aba-synthetic-isolated' AND
 NOT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema='platform' AND table_name<>'test_safety')
 FROM platform.test_safety WHERE singleton`).Scan(&safe)
	if err != nil || !safe {
		t.Fatal("fresh isolated schema required")
	}
	if err = migrate.Run(ctx, migrationDSN, "up"); err != nil {
		t.Fatal(err)
	}
	d, err := db.Open(ctx, testDSN(t, "ABA_TEST_DATABASE_URL", "aba_app"), 2, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	testStage261Delegation(t, d, admin, ctx)
}
