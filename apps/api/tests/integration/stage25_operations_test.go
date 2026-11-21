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
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/orders"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/db"
)

func testStage25Operations(t *testing.T, d *db.Database, admin *pgx.Conn, ctx context.Context) {
	runtime := agentruntime.Service{Pool: d.Pool, Key: bytes.Repeat([]byte{95}, 32)}
	identity := auth.New(d.Pool, config.Auth{SessionTTL: time.Hour, IdleTTL: time.Hour})
	type profile struct {
		model, industry string
		owner           stage23Owner
	}
	profiles := []profile{
		{model: "trade", industry: "electronics"}, {model: "trade", industry: "auto_parts"},
		{model: "trade", industry: "building_materials"}, {model: "trade", industry: "furniture"},
		{model: "trade", industry: "apparel"}, {model: "service", industry: "vehicle_service"},
		{model: "service", industry: "personal_services"}, {model: "agency", industry: "real_estate"},
		{model: "hybrid", industry: "mixed_nonfood"},
	}
	for i := range profiles {
		p := &profiles[i]
		p.owner = seedStage23Owner(t, d, admin, ctx, identity, 60+i)
		if err := runtime.UpdateProfile(ctx, p.owner.tenant, p.model, p.industry, 1); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range profiles {
		t.Run(p.industry, func(t *testing.T) {
			team, err := runtime.Team(ctx, p.owner.tenant)
			if err != nil || team.Profile.Model != p.model || team.Profile.Industry != p.industry || team.Profile.Revision != 2 || len(team.Agents) != 7 {
				t.Fatalf("team %v %v", team.Profile, err)
			}
			product := p.model == "trade" || p.model == "hybrid"
			if team.Agents[0].Enabled != product {
				t.Fatal("catalog capability mismatch")
			}
			req := agentruntime.Request{AgentID: "product_inventory", Action: "check_inventory", VariantID: p.owner.product.VariantID, ConversationID: p.owner.conversation, IdempotencyKey: auth.UUID()}
			id, err := runtime.Submit(ctx, p.owner.tenant, req)
			if !product {
				if err == nil {
					t.Fatal("out-of-profile tool accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if ran, err := runtime.ProcessOne(ctx); !ran || err != nil {
				t.Fatalf("worker: %t %v", ran, err)
			}
			var result struct {
				JSON string `json:"result_json"`
			}
			stage23Result(t, admin, ctx, p.owner, id, 1, &result)
			if result.JSON == "" {
				t.Fatal("missing result")
			}
		})
	}
	a, b := profiles[0].owner, profiles[1].owner
	if err := runtime.UpdateProfile(ctx, a.tenant, "trade", "furniture", 2); err != nil {
		t.Fatal(err)
	}
	team, err := runtime.Team(ctx, b.tenant)
	if err != nil || team.Profile.Industry != "auto_parts" || team.Profile.Revision != 2 {
		t.Fatalf("other profile changed %v %v", team.Profile, err)
	}
	ordersService := orders.Service{Pool: d.Pool}
	in := orders.Intake{Kind: "quotation_request", ConversationID: a.conversation, IdempotencyKey: auth.UUID(), Details: orders.IntakeDetails{Summary: "Synthetic quotation"}}
	raw, err := ordersService.PrepareIntake(ctx, a.tenant, in)
	if err != nil {
		t.Fatal(err)
	}
	var request struct {
		ID, Status string
		Total      *int64 `json:"total_amount"`
	}
	if err = json.Unmarshal(raw, &request); err != nil || request.Status != "pending_confirmation" || request.Total != nil {
		t.Fatalf("bad intake %s %v", raw, err)
	}
	if replay, err := ordersService.PrepareIntake(ctx, a.tenant, in); err != nil || !bytes.Contains(replay, []byte(request.ID)) {
		t.Fatal("idempotent replay", err)
	}
	in.Details.Summary = "Changed parameters"
	if _, err = ordersService.PrepareIntake(ctx, a.tenant, in); err == nil {
		t.Fatal("changed replay accepted")
	}
	if _, err = ordersService.Get(ctx, b.tenant, request.ID); err == nil {
		t.Fatal("foreign request visible")
	}
	if _, err = ordersService.Change(ctx, a.tenant, request.ID, "confirm", orders.Transition{ExpectedVersion: 1, IdempotencyKey: auth.UUID()}); err != nil {
		t.Fatal(err)
	}
	agency := profiles[7].owner
	listing := auth.UUID()
	if _, err = admin.Exec(ctx, `INSERT INTO platform.property_listings(id,organization_id,title,locality,rooms,area_m2,status,verified_until)
	VALUES($1,$2,'Synthetic home','Synthetic locality',2,48.5,'available',now()+interval '1 hour')`, listing, agency.tenant.OrganizationID); err != nil {
		t.Fatal(err)
	}
	if found, err := ordersService.SearchProperties(ctx, agency.tenant, orders.PropertyFilter{}); err != nil || !bytes.Contains(found, []byte(listing)) {
		t.Fatalf("property query %s %v", found, err)
	}
	service, hybrid := profiles[5].owner, profiles[8].owner
	repairOffering, hybridOffering := auth.UUID(), auth.UUID()
	for _, v := range []struct {
		o  stage23Owner
		id string
	}{{service, repairOffering}, {hybrid, hybridOffering}} {
		if _, err = admin.Exec(ctx, `INSERT INTO platform.service_offerings(id,organization_id,name,status,price_model,price_state,min_minutes,max_minutes,timezone)
		VALUES($1,$2,'Synthetic repair','active','on_request','unknown',0,0,'UTC')`, v.id, v.o.tenant.OrganizationID); err != nil {
			t.Fatal(err)
		}
	}
	for _, caseOf := range []struct {
		o       stage23Owner
		kind    string
		details orders.IntakeDetails
	}{
		{a, "custom_order", orders.IntakeDetails{Summary: "Custom synthetic build"}},
		{agency, "property_viewing_request", orders.IntakeDetails{ListingID: listing}},
		{service, "repair_request", orders.IntakeDetails{OfferingID: repairOffering, Summary: "Synthetic repair"}},
		{hybrid, "hybrid_order", orders.IntakeDetails{OfferingID: hybridOffering, Items: []orders.CartItem{{VariantID: hybrid.product.VariantID, Quantity: 1}}}},
	} {
		out, err := ordersService.PrepareIntake(ctx, caseOf.o.tenant, orders.Intake{Kind: caseOf.kind, ConversationID: caseOf.o.conversation, IdempotencyKey: auth.UUID(), Details: caseOf.details})
		if err != nil || !bytes.Contains(out, []byte(caseOf.kind)) {
			t.Errorf("%s: %s %v", caseOf.kind, out, err)
		}
	}
	for index, owner := range []stage23Owner{a, b} {
		if _, err := admin.Exec(ctx, `INSERT INTO platform.order_items(id,organization_id,order_id,variant_id,product_id,quantity,unit_price_amount,name_snapshot,sku_snapshot)
		SELECT gen_random_uuid(),o.organization_id,o.id,v.id,v.product_id,$3,v.price_amount,'Synthetic sale',v.sku
		FROM platform.orders o JOIN platform.product_variants v ON v.organization_id=o.organization_id AND v.id=$2
		WHERE o.organization_id=$1 AND o.status='completed' AND o.request_kind='product_order'`, owner.tenant.OrganizationID, owner.product.VariantID, index+1); err != nil {
			t.Fatal(err)
		}
	}
	growth, err := runtime.Submit(ctx, a.tenant, agentruntime.Request{AgentID: "business_manager", Action: "coordinate_growth", IdempotencyKey: auth.UUID()})
	if err != nil {
		t.Fatal(err)
	}
	var approval, hash string
	if err = admin.QueryRow(ctx, `SELECT a.id,a.parameters_hash FROM platform.agent_approvals a JOIN platform.agent_tasks t ON t.id=a.task_id WHERE t.parent_id=$1`, growth).Scan(&approval, &hash); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if ran, err := runtime.ProcessOne(ctx); !ran || err != nil {
			t.Fatalf("growth evidence %t %v", ran, err)
		}
	}
	if ran, err := runtime.ProcessOne(ctx); ran || err != nil {
		t.Fatalf("approval did not block %t %v", ran, err)
	}
	if err = runtime.Decide(ctx, a.tenant, approval, "approved", hash); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if ran, err := runtime.ProcessOne(ctx); !ran || err != nil {
			t.Fatalf("growth completion %t %v", ran, err)
		}
	}
	var report struct {
		Children []struct {
			Agent  string `json:"agent_id"`
			Status string `json:"status"`
			Result []struct {
				Metrics []struct {
					Name  string
					Value int64
				}
			} `json:"result"`
		}
		Recommendations []string `json:"recommendations"`
	}
	var saved []byte
	var audit int
	if err := admin.QueryRow(ctx, `SELECT e.result,(SELECT count(*) FROM platform.agent_audit WHERE task_id=t.id AND action='coordinate_growth' AND result='succeeded')
	FROM platform.agent_executions e JOIN platform.agent_tasks t ON t.id=e.task_id AND t.organization_id=e.organization_id
	WHERE t.organization_id=$1 AND t.id=$2 AND t.status='completed' AND e.status='completed'`, a.tenant.OrganizationID, growth).Scan(&saved, &audit); err != nil || audit != 1 {
		t.Fatalf("missing growth result or audit: %d %v", audit, err)
	}
	if err := json.Unmarshal(saved, &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Children) != 4 || len(report.Recommendations) == 0 {
		t.Fatalf("incomplete growth report: %+v", report)
	}
	for _, child := range report.Children {
		if child.Status != "completed" {
			t.Fatalf("child failed: %+v", child)
		}
		if child.Agent == "sales" {
			if len(child.Result) != 1 || len(child.Result[0].Metrics) == 0 || child.Result[0].Metrics[0].Name != "units" || child.Result[0].Metrics[0].Value != 1 {
				t.Fatalf("wrong sales scope %+v", child.Result)
			}
		}
	}
}
