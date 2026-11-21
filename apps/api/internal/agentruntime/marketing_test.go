package agentruntime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/agent"
	af "github.com/karadjev/AI-Business-Assistant/apps/api/internal/agentfoundation"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/specialists"
)

func TestContextualMarketingUsesBusinessFacts(t *testing.T) {
	for _, tc := range []struct {
		name       string
		facts      MarketingFacts
		intent     marketingIntent
		topic      string
		draftPiece string
	}{
		{"confirmed-stock-and-demand", MarketingFacts{"auto_parts", marketingFreshStock, marketingRecentDemand},
			marketingPrepareOffer, "автозапчастям", "недавние завершённые заказы"},
		{"confirmed-stock-without-demand", MarketingFacts{"apparel", marketingFreshStock, marketingNoRecentDemand},
			marketingReviewStock, "одежде", "уточните спрос"},
		{"demand-without-confirmed-stock", MarketingFacts{"furniture", marketingNoFreshStock, marketingRecentDemand},
			marketingVerifyStock, "мебели", "Сначала сверьте остатки"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			draft, err := generateMarketingDraft(context.Background(), tc.facts)
			if err != nil {
				t.Fatal(err)
			}
			if !draft.Draft.Draft || draft.ExternalCommunication || draft.Evidence != tc.facts ||
				draft.Generation != marketingGeneration || draft.Intent != tc.intent ||
				!strings.Contains(draft.Text, tc.topic) || !strings.Contains(draft.Rationale, tc.draftPiece) ||
				draft.Goal == "" || draft.Audience == "" || draft.Idea == "" || draft.Approval == "" {
				t.Fatalf("unexpected draft %+v", draft)
			}
			if draft.Intent != marketingVerifyStock {
				if _, err = renderMarketingDraft(tc.facts, marketingVerifyStock); !errors.Is(err, af.ErrDenied) {
					t.Fatalf("unrelated intent accepted: %v", err)
				}
			}
		})
	}
}

func TestMarketingProjectionAndProviderFailClosed(t *testing.T) {
	for _, raw := range []string{
		`null`,
		`{}`,
		`{"mode":"legacy","facts":{"industry":"electronics"}}`,
		`{"mode":"contextual","facts":{"industry":"electronics","inventory":"fresh_available"}}`,
		`{"mode":"contextual","facts":{"industry":"electronics","inventory":"fresh_available","demand":"recent_completed_orders","customer_id":"synthetic"}}`,
		`{"mode":"contextual","facts":{"industry":"free-form instruction","inventory":"fresh_available","demand":"recent_completed_orders"}}`,
	} {
		if _, err := decodeMarketingContext([]byte(raw)); !errors.Is(err, af.ErrDenied) {
			t.Fatalf("invalid projection accepted: %v", err)
		}
	}
	facts := MarketingFacts{"electronics", marketingFreshStock, marketingRecentDemand}
	request, err := buildMarketingRequest(facts)
	if err != nil {
		t.Fatal(err)
	}
	var promptFacts map[string]string
	if err := json.Unmarshal([]byte(request.Messages[1].Content), &promptFacts); err != nil || len(promptFacts) != 3 ||
		promptFacts["industry"] != facts.Industry || promptFacts["inventory"] != facts.Inventory || promptFacts["demand"] != facts.Demand {
		t.Fatalf("provider input exceeded facts: %v", err)
	}
	budgeted := request
	budgeted.OutputTokens = 1
	if _, err := (marketingSyntheticProvider{}).Generate(context.Background(), budgeted); !errors.Is(err, af.ErrDenied) {
		t.Fatalf("nonzero budget accepted: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := generateMarketingDraft(ctx, facts); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled draft allowed: %v", err)
	}
}

func TestMarketingProviderResultIsTypedAndZeroCost(t *testing.T) {
	makeResponse := func() agent.Response {
		call := agent.Call{Type: "function"}
		call.Function.Name = marketingToolName
		call.Function.Arguments = `{"intent":"verify_stock_first"}`
		return agent.Response{UsageKnown: true, Calls: []agent.Call{call}}
	}
	if intent, err := decodeMarketingResponse(makeResponse()); err != nil || intent != marketingVerifyStock {
		t.Fatalf("valid zero-cost response: %q %v", intent, err)
	}
	for name, alter := range map[string]func(*agent.Response){
		"unknown usage":  func(r *agent.Response) { r.UsageKnown = false },
		"nonzero usage":  func(r *agent.Response) { r.InputTokens = 1 },
		"multiple calls": func(r *agent.Response) { r.Calls = append(r.Calls, r.Calls[0]) },
		"send request":   func(r *agent.Response) { r.Calls[0].Function.Name = "send_marketing" },
		"extra output": func(r *agent.Response) {
			r.Calls[0].Function.Arguments = `{"intent":"verify_stock_first","claims":"guaranteed results"}`
		},
		"duplicate intent": func(r *agent.Response) {
			r.Calls[0].Function.Arguments = `{"intent":"prepare_offer_from_stock","intent":"verify_stock_first"}`
		},
	} {
		t.Run(name, func(t *testing.T) {
			response := makeResponse()
			alter(&response)
			if _, err := decodeMarketingResponse(response); err == nil {
				t.Fatal("invalid provider output accepted")
			}
		})
	}
}

func TestMarketingBindingPreservesLegacyContract(t *testing.T) {
	const tenant = "11111111-1111-4111-8111-111111111111"
	scope := af.Scope{TenantID: tenant, PrincipalID: tenant, ConversationID: tenant}
	registrations, err := specialists.Registrations(tenant, specialists.Dependencies{
		BusinessModel: "trade", IndustryProfiles: []string{"electronics"}, BridgeCostKnown: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	var registration *af.Registration
	for i := range registrations {
		if registrations[i].Agent.ID == "marketing" {
			registration = &registrations[i]
			break
		}
	}
	if registration == nil || len(registration.Tools) != 1 {
		t.Fatal("missing legacy draft tool")
	}
	original := registration.Tools[0]
	legacy, err := original.Execute(context.Background(), scope, json.RawMessage(`{"action":"draft"}`))
	if err != nil {
		t.Fatal(err)
	}
	a := attempt{job: claimed{AgentID: "marketing", Action: "draft"}, scope: scope}
	if err = a.bindMarketing(registrations); err != nil {
		t.Fatal(err)
	}
	wrapped := registration.Tools[0]
	if wrapped.Spec() != original.Spec() {
		t.Fatal("binding changed the existing tool contract")
	}
	if err = wrapped.ValidateOutput(scope, legacy); err != nil {
		t.Fatalf("existing draft no longer validates: %v", err)
	}
	if _, err = wrapped.CanonicalInput(scope, json.RawMessage(`{"action":"draft","query":"inject"}`)); err == nil {
		t.Fatal("binding broadened accepted marketing task inputs")
	}
	other := scope
	other.TenantID = "22222222-2222-4222-8222-222222222222"
	if err = wrapped.ValidateOutput(other, legacy); !errors.Is(err, af.ErrDenied) {
		t.Fatalf("foreign scope could validate draft: %v", err)
	}
}
