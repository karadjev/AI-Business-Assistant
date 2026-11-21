//go:build integration

package integration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/agent"
	af "github.com/karadjev/AI-Business-Assistant/apps/api/internal/agentfoundation"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/db"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/specialists"
)

const (
	stage22InventoryAgent = "product_inventory"
	stage22PriceAction    = "get_product_price"
	stage22PriceRead      = stage22InventoryAgent + "." + stage22PriceAction
)

func testStage22SpecialistCatalog(t *testing.T, ctx context.Context, d *db.Database, tenant catalog.Tenant, conversationID, principalID string, fixtures []instagramCatalogFixture) {
	t.Helper()
	var own, foreign instagramCatalogFixture
	for _, fixture := range fixtures {
		if fixture.OrganizationID == tenant.OrganizationID {
			own = fixture
		} else {
			foreign = fixture
		}
	}
	if own.VariantID == "" || foreign.VariantID == "" || own.VariantID == foreign.VariantID {
		t.Fatal("specialist catalog requires distinct local and foreign variants")
	}
	if tenant.ChannelType != "instagram" || tenant.ChannelCapability == "" || conversationID == "" || principalID == "" {
		t.Fatal("specialist catalog requires a verified Instagram claim")
	}

	scope := af.Scope{TenantID: tenant.OrganizationID, ConversationID: conversationID, PrincipalID: principalID}
	registrations, err := specialists.Registrations(scope.TenantID, specialists.Dependencies{
		BusinessModel:    "trade",
		IndustryProfiles: []string{"electronics"},
		Runners:          stage22InstagramScopeResolver{scope: scope, tenant: tenant, catalog: catalog.Service{Pool: d.Pool}},
		BridgeCostKnown:  true, // Local PostgreSQL read with no provider use.
	})
	if err != nil {
		t.Fatal("specialist registrations", err)
	}
	for _, registration := range registrations {
		if registration.Agent.ID != stage22InventoryAgent {
			continue
		}
		for _, tool := range registration.Tools {
			if spec := tool.Spec(); spec.Kind != af.ActionRead {
				t.Fatalf("expected a read-only inventory tool, got %s (%s)", spec.Name, spec.Kind)
			}
		}
	}
	registry, err := af.NewRegistry(registrations)
	if err != nil {
		t.Fatal("specialist registry", err)
	}

	run := func(t *testing.T, request specialists.Request, wantStatus af.Status, wantFailure af.FailureCode, wantExecution bool) (af.Result, error) {
		t.Helper()
		raw, err := json.Marshal(request)
		if err != nil {
			t.Fatal("specialist request", err)
		}
		task := af.Task{ID: auth.UUID(), CorrelationID: auth.UUID(), Scope: scope}
		stores := &stage22SpecialistStores{scope: scope}
		orchestrator := af.Orchestrator{
			Registry: registry, Configurations: stores, Permissions: stores,
			Contexts: stage22SpecialistContext{scope: scope, request: raw},
			Tasks:    stores, Audit: stores,
			Limits: af.Limits{MaxSteps: 2, MaxContextBytes: 4096, MaxResultBytes: 32768, Timeout: 5 * time.Second},
		}
		result, runErr := orchestrator.RunCapability(ctx, task, stage22InventoryAgent+"."+request.Action)
		if result.UsedUnits != 0 || stores.status != wantStatus {
			t.Fatalf("specialist used %d units and finished %q, want zero units and %q (error: %v)", result.UsedUnits, stores.status, wantStatus, runErr)
		}
		hash := func(value string) string {
			sum := sha256.Sum256([]byte(value))
			return hex.EncodeToString(sum[:])
		}
		event := af.AuditEvent{
			TenantHash: hash(scope.TenantID), PrincipalHash: hash(scope.PrincipalID),
			ConversationHash: hash(scope.ConversationID), TaskHash: hash(task.ID),
			AgentHash: hash(stage22InventoryAgent), CorrelationHash: hash(scope.TenantID + "/" + task.CorrelationID),
			ApprovalStatus: af.ApprovalNone,
		}
		var wantAudit []af.AuditEvent
		if wantExecution {
			started := event
			started.ActionHash, started.ParamsHash = hash(stage22PriceRead), af.ParamsHash(raw)
			started.Step, started.Status = 1, af.StatusStarted
			finished := started
			finished.Status, finished.FailureCode = wantStatus, wantFailure
			wantAudit = append(wantAudit, started, finished)
		}
		event.ActionHash = hash("task")
		event.Status, event.FailureCode, event.Step = wantStatus, wantFailure, 1
		if wantStatus == af.StatusSucceeded {
			event.Step = 2
		}
		wantAudit = append(wantAudit, event)
		if !slices.Equal(stores.audit, wantAudit) {
			t.Fatalf("specialist audit got %+v, want %+v (error: %v)", stores.audit, wantAudit, runErr)
		}
		return result, runErr
	}

	t.Run("own_variant_price", func(t *testing.T) {
		result, err := run(t, specialists.Request{Action: stage22PriceAction, VariantID: own.VariantID}, af.StatusSucceeded, af.FailureNone, true)
		if err != nil || len(result.Results) != 1 {
			t.Fatalf("expected one catalog price result, got %d (%v)", len(result.Results), err)
		}
		var bridge struct {
			ResultJSON string `json:"result_json"`
		}
		var price struct {
			OK   bool `json:"ok"`
			Data struct {
				VariantID string `json:"variant_id"`
				Amount    int64  `json:"price_amount"`
				Currency  string `json:"currency_code"`
			} `json:"data"`
		}
		if err := json.Unmarshal(result.Results[0], &bridge); err != nil {
			t.Fatal("specialist result envelope", err)
		}
		if err := json.Unmarshal([]byte(bridge.ResultJSON), &price); err != nil {
			t.Fatal("specialist price result", err)
		}
		if !price.OK || price.Data.VariantID != own.VariantID || price.Data.Amount != own.Price || price.Data.Currency != "RUB" {
			t.Fatalf("expected own variant at %d RUB, got %+v", own.Price, price)
		}
	})

	t.Run("foreign_variant_price", func(t *testing.T) {
		result, err := run(t, specialists.Request{Action: stage22PriceAction, VariantID: foreign.VariantID}, af.StatusFailed, af.FailureExecution, true)
		var failure *auth.Error
		if !errors.As(err, &failure) || failure.Status != http.StatusNotFound || len(result.Results) != 0 {
			t.Fatalf("expected catalog 404 and zero results, got %v and %d results", err, len(result.Results))
		}
	})

	t.Run("ungranted_inventory_read", func(t *testing.T) {
		result, err := run(t, specialists.Request{Action: "check_inventory", VariantID: own.VariantID, Quantity: 1}, af.StatusFailed, af.FailureDenied, false)
		if !errors.Is(err, af.ErrDenied) || len(result.Results) != 0 {
			t.Fatalf("expected denied read and zero results, got %v and %d results", err, len(result.Results))
		}
	})
}

// stage22InstagramScopeResolver is a test fixture. Its scope and lease come
// from the verified Instagram claim supplied by the calling integration test.
type stage22InstagramScopeResolver struct {
	scope   af.Scope
	tenant  catalog.Tenant
	catalog catalog.Service
}

func (r stage22InstagramScopeResolver) Resolve(ctx context.Context, scope af.Scope) (specialists.RunnerBinding, error) {
	if err := ctx.Err(); err != nil {
		return specialists.RunnerBinding{}, err
	}
	if scope != r.scope {
		return specialists.RunnerBinding{}, af.ErrDenied
	}
	return specialists.BindRunner(scope, &agent.Runner{
		Catalog: r.catalog, Tenant: r.tenant, ConversationID: r.scope.ConversationID,
	})
}

type stage22SpecialistContext struct {
	scope   af.Scope
	request json.RawMessage
}

func (c stage22SpecialistContext) Load(ctx context.Context, scope af.Scope, keys []string, maxBytes int) (af.ContextBundle, error) {
	if err := ctx.Err(); err != nil {
		return af.ContextBundle{}, err
	}
	if scope != c.scope || !slices.Equal(keys, []string{specialists.RequestKey}) {
		return af.ContextBundle{}, af.ErrDenied
	}
	if len(specialists.RequestKey)+len(c.request) > maxBytes {
		return af.ContextBundle{}, af.ErrLimit
	}
	return af.ContextBundle{Scope: c.scope, Values: map[string]json.RawMessage{specialists.RequestKey: c.request}}, nil
}

// Each run owns one in-memory task and an audit sink for its scoped tool and task events.
type stage22SpecialistStores struct {
	scope  af.Scope
	task   af.Task
	status af.Status
	audit  []af.AuditEvent
}

func (s *stage22SpecialistStores) Load(ctx context.Context, scope af.Scope, agentID string) (af.Configuration, error) {
	if err := ctx.Err(); err != nil {
		return af.Configuration{}, err
	}
	if scope != s.scope || agentID != stage22InventoryAgent {
		return af.Configuration{}, af.ErrDenied
	}
	return af.Configuration{
		TenantID: s.scope.TenantID, AgentID: stage22InventoryAgent, Version: "v1", Enabled: true,
		Tools: []string{stage22PriceRead}, ContextKeys: []string{specialists.RequestKey},
	}, nil
}

func (s *stage22SpecialistStores) Check(ctx context.Context, scope af.Scope, permission string) error {
	if scope != s.scope || permission != stage22PriceRead {
		return af.ErrDenied
	}
	return ctx.Err()
}

func (s *stage22SpecialistStores) Claim(ctx context.Context, task af.Task) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if task.Scope != s.scope || task.AgentID != stage22InventoryAgent {
		return false, af.ErrDenied
	}
	if s.task.ID != "" {
		return false, nil
	}
	s.task = task
	return true, nil
}

func (s *stage22SpecialistStores) Finish(ctx context.Context, task af.Task, status af.Status) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.task.ID == "" || s.task != task {
		return af.ErrDenied
	}
	s.status = status
	return nil
}

func (s *stage22SpecialistStores) Record(ctx context.Context, event af.AuditEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.audit = append(s.audit, event)
	return nil
}
