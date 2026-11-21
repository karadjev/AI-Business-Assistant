//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/agentruntime"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/db"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/specialists"
)

func testStage24IndustryProfiles(t *testing.T, d *db.Database, admin *pgx.Conn, ctx context.Context) {
	key := bytes.Repeat([]byte{91}, 32)
	svc := agentruntime.Service{Pool: d.Pool, Key: key}
	identity := auth.New(d.Pool, config.Auth{SessionTTL: time.Hour, IdleTTL: time.Hour})
	a := seedStage23Owner(t, d, admin, ctx, identity, 40)
	b := seedStage23Owner(t, d, admin, ctx, identity, 41)
	revision := 1
	requireStatus := func(t *testing.T, err error, want int) {
		t.Helper()
		var e *auth.Error
		if !errors.As(err, &e) || e.Status != want {
			t.Fatalf("wanted %d, got %v", want, err)
		}
	}
	spoof := a.tenant
	spoof.OrganizationID = b.tenant.OrganizationID
	if _, err := svc.Team(ctx, spoof); err == nil {
		t.Fatal("foreign profile read")
	}
	if err := svc.UpdateProfile(ctx, spoof, "trade", "auto_parts", 1); err == nil {
		t.Fatal("foreign profile write")
	}
	requireStatus(t, svc.UpdateProfile(ctx, a.tenant, "trade", "groceries", 1), http.StatusUnprocessableEntity)

	for _, profile := range []struct {
		model, industry string
		catalog         bool
	}{
		{"trade", "electronics", true}, {"trade", "auto_parts", true}, {"trade", "building_materials", true},
		{"trade", "furniture", true}, {"trade", "apparel", true}, {"service", "vehicle_service", false},
		{"service", "personal_services", false}, {"agency", "real_estate", false}, {"hybrid", "mixed_nonfood", true},
	} {
		t.Run(profile.industry, func(t *testing.T) {
			if err := svc.UpdateProfile(ctx, a.tenant, profile.model, profile.industry, revision); err != nil {
				t.Fatal(err)
			}
			revision++
			team, err := svc.Team(ctx, a.tenant)
			if err != nil || team.Profile.Model != profile.model || team.Profile.Industry != profile.industry || team.Profile.Revision != revision || len(team.Agents) != 7 {
				t.Fatalf("persisted profile %+v: %v", team, err)
			}
			if team.Agents[0].Enabled != profile.catalog {
				t.Fatal("customer availability does not match catalog capability")
			}
			req := agentruntime.Request{AgentID: "product_inventory", Action: "check_inventory", VariantID: a.product.VariantID,
				ConversationID: a.conversation, IdempotencyKey: auth.UUID()}
			id, err := svc.Submit(ctx, a.tenant, req)
			if !profile.catalog {
				requireStatus(t, err, http.StatusForbidden)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if ran, err := svc.ProcessOne(ctx); !ran || err != nil {
				t.Fatalf("process %t %v", ran, err)
			}
			var bridge struct {
				JSON string `json:"result_json"`
			}
			stage23Result(t, admin, ctx, a, id, 1, &bridge)
			var stock struct {
				Data struct {
					Available *int `json:"available"`
					Fresh     bool `json:"fresh"`
				} `json:"data"`
			}
			if err := json.Unmarshal([]byte(bridge.JSON), &stock); err != nil || !stock.Data.Fresh || stock.Data.Available == nil || *stock.Data.Available != 7 {
				t.Fatalf("wrong stock %s: %v", bridge.JSON, err)
			}
		})
	}
	requireStatus(t, svc.UpdateProfile(ctx, a.tenant, "trade", "electronics", revision-1), http.StatusConflict)
	team, err := svc.Team(ctx, b.tenant)
	if err != nil || team.Profile.Revision != 1 || team.Profile.Industry != "electronics" {
		t.Fatal("foreign profile changed", err)
	}
	// A detail job can only project the selected variant's product and attributes.
	details := agentruntime.Request{AgentID: "sales", Action: "get_product_details", VariantID: a.product.VariantID,
		ConversationID: a.conversation, IdempotencyKey: auth.UUID()}
	id, err := svc.Submit(ctx, a.tenant, details)
	if err != nil {
		t.Fatal(err)
	}
	if ran, err := svc.ProcessOne(ctx); !ran || err != nil {
		t.Fatalf("details %t %v", ran, err)
	}
	var bridge struct {
		JSON string `json:"result_json"`
	}
	stage23Result(t, admin, ctx, a, id, 1, &bridge)
	var result struct {
		Data []struct {
			VariantID string `json:"variant_id"`
			Price     int64  `json:"price_amount"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(bridge.JSON), &result); err != nil || len(result.Data) != 1 || result.Data[0].VariantID != a.product.VariantID || result.Data[0].Price != a.product.Price {
		t.Fatalf("unexpected details %s: %v", bridge.JSON, err)
	}
	details.VariantID = b.product.VariantID
	details.IdempotencyKey = auth.UUID()
	foreign, err := svc.Submit(ctx, a.tenant, details)
	if err != nil {
		t.Fatal(err)
	}
	if ran, err := svc.ProcessOne(ctx); !ran || err != nil {
		t.Fatalf("foreign %t %v", ran, err)
	}
	var state string
	var hasResult bool
	if err := admin.QueryRow(ctx, `SELECT t.status,e.result IS NOT NULL FROM platform.agent_tasks t
 JOIN platform.agent_executions e ON e.organization_id=t.organization_id AND e.task_id=t.id WHERE t.id=$1`, foreign).Scan(&state, &hasResult); err != nil || state != "failed" || hasResult {
		t.Fatalf("foreign task %s result=%t %v", state, hasResult, err)
	}
	id, err = svc.Submit(ctx, a.tenant, agentruntime.Request{AgentID: "finance", Action: "read", IdempotencyKey: auth.UUID()})
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := svc.ProcessOne(ctx); !ok || err != nil {
		t.Fatalf("finance %t %v", ok, err)
	}
	var report specialists.ReadResult
	stage23Result(t, admin, ctx, a, id, 1, &report)
	metrics := map[string]int64{}
	for _, m := range report.Metrics {
		metrics[m.Name] = m.Value
	}
	if metrics["average_completed_order_value_last_30d"] != a.revenue || metrics["completed_orders_created_last_30d"] != 1 {
		t.Fatalf("unexpected computed report %+v", metrics)
	}
	if count, exists := metrics["completed_orders_created_previous_30d"]; !exists || count != 0 {
		t.Fatal("unexpected previous period")
	}
}
