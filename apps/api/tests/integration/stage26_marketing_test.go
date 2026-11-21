//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/agentruntime"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/db"
)

func testStage26Marketing(t *testing.T, d *db.Database, admin *pgx.Conn, ctx context.Context) {
	key := bytes.Repeat([]byte{106}, 32)
	runtime := agentruntime.Service{Pool: d.Pool, Key: key}
	identity := auth.New(d.Pool, config.Auth{SessionTTL: time.Hour, IdleTTL: time.Hour})
	owner := seedStage23Owner(t, d, admin, ctx, identity, 72)
	outsider := seedStage23Owner(t, d, admin, ctx, identity, 73)
	for _, decision := range []string{"approved", "rejected"} {
		t.Run(decision, func(t *testing.T) {
			parent, err := runtime.Submit(ctx, owner.tenant, agentruntime.Request{
				AgentID: "business_manager", Action: "coordinate_growth", IdempotencyKey: auth.UUID(),
			})
			if err != nil {
				t.Fatal(err)
			}
			var approval, task, hash string
			err = admin.QueryRow(ctx, `SELECT a.id,c.id,a.parameters_hash FROM platform.agent_tasks c
			JOIN platform.agent_approvals a ON a.organization_id=c.organization_id AND a.task_id=c.id
			WHERE c.organization_id=$1 AND c.parent_id=$2 AND c.agent_id='marketing'`, owner.tenant.OrganizationID, parent).Scan(&approval, &task, &hash)
			if err != nil {
				t.Fatal(err)
			}
			var projection []byte
			err = d.Pool.QueryRow(ctx, "SELECT platform.agent_marketing_context($1,$2)", task, auth.UUID()).Scan(&projection)
			if err != nil || len(projection) != 0 {
				t.Fatalf("unleased projection leaked: %v", err)
			}
			if err = runtime.Decide(ctx, outsider.tenant, approval, decision, hash); err == nil {
				t.Fatal("foreign approval accepted")
			}
			if err = runtime.Decide(ctx, owner.tenant, approval, decision, hash); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 5; i++ {
				worked, err := runtime.ProcessOne(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if !worked {
					break
				}
			}
			var state string
			var raw []byte
			if err = admin.QueryRow(ctx, `SELECT t.status,e.result FROM platform.agent_tasks t
			 LEFT JOIN platform.agent_executions e ON e.organization_id=t.organization_id AND e.task_id=t.id WHERE t.id=$1`, task).Scan(&state, &raw); err != nil {
				t.Fatal(err)
			}
			var draftCount int
			if err := admin.QueryRow(ctx, "SELECT count(*) FROM platform.marketing_drafts WHERE organization_id=$1 AND id=$2", owner.tenant.OrganizationID, task).Scan(&draftCount); err != nil {
				t.Fatal(err)
			}
			if decision == "rejected" {
				if state != "cancelled" || len(raw) != 0 || draftCount != 0 {
					t.Fatal("rejected draft had an execution result")
				}
				return
			}
			var result []struct {
				Generation            string
				Evidence              agentruntime.MarketingFacts
				ExternalCommunication bool `json:"external_communication"`
			}
			if state != "completed" || json.Unmarshal(raw, &result) != nil || len(result) != 1 ||
				result[0].Generation != "synthetic_contextual" || result[0].ExternalCommunication ||
				result[0].Evidence.Industry != "electronics" || result[0].Evidence.Inventory != "fresh_available" ||
				result[0].Evidence.Demand != "recent_completed_orders" {
				t.Fatalf("contextual draft not recorded; task status %s", state)
			}
			if draftCount != 1 {
				t.Fatalf("draft persisted %d times", draftCount)
			}
			reason := "Synthetic review declined"
			reject := agentruntime.MarketingDraftAction{Version: 1, Action: "rejected", Reason: &reason}
			if err := runtime.ChangeMarketingDraft(ctx, outsider.tenant, task, reject); err == nil {
				t.Fatal("cross-tenant review accepted")
			}
			if err := runtime.ChangeMarketingDraft(ctx, owner.tenant, task, reject); err != nil {
				t.Fatal(err)
			}
			if err := runtime.ChangeMarketingDraft(ctx, owner.tenant, task, reject); err == nil {
				t.Fatal("review replay accepted")
			}
			revised := "Synthetic revised material"
			if err := runtime.ChangeMarketingDraft(ctx, owner.tenant, task, agentruntime.MarketingDraftAction{Version: 1, Action: "revise", Content: &revised}); err != nil {
				t.Fatal(err)
			}
			if err := runtime.ChangeMarketingDraft(ctx, owner.tenant, task, agentruntime.MarketingDraftAction{Version: 2, Action: "approved"}); err != nil {
				t.Fatal(err)
			}
			drafts, err := runtime.ListMarketingDrafts(ctx, owner.tenant)
			var list struct {
				Items []struct {
					ID, Status, Reason, Content string
					Version                     int
				}
			}
			if err != nil || json.Unmarshal(drafts, &list) != nil || len(list.Items) != 2 ||
				list.Items[0].ID != task || list.Items[0].Version != 2 || list.Items[0].Status != "approved" ||
				list.Items[0].Content != revised || list.Items[1].Status != "rejected" || list.Items[1].Reason != reason ||
				list.Items[1].Content == "" || list.Items[1].Content == revised {
				t.Fatalf("draft version or review history missing: %v", err)
			}
			if err := admin.QueryRow(ctx, "SELECT count(*) FROM platform.agent_audit WHERE organization_id=$1 AND task_id=$2 AND action='draft.review'", owner.tenant.OrganizationID, task).Scan(&draftCount); err != nil || draftCount != 3 {
				t.Fatalf("review audit count %d: %v", draftCount, err)
			}
		})
	}
}
