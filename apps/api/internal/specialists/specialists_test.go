package specialists

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/agent"
	af "github.com/karadjev/AI-Business-Assistant/apps/api/internal/agentfoundation"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
)

var scope = af.Scope{TenantID: "tenant-a", PrincipalID: "principal-a", ConversationID: "chat-a"}

type fixture struct {
	scope         af.Scope
	request       json.RawMessage
	registrations []af.Registration
	deny          bool
	grant         string
	mu            sync.Mutex
	claims        map[string]bool
	calls         int
	foreign       bool
}

func (f *fixture) Load(_ context.Context, s af.Scope, id string) (af.Configuration, error) {
	if s != f.scope {
		return af.Configuration{}, af.ErrDenied
	}
	for _, r := range f.registrations {
		if r.Agent.ID == id {
			return af.Configuration{TenantID: s.TenantID, AgentID: id, Version: "v1", Enabled: true, Tools: r.Agent.Tools, ContextKeys: r.Agent.ContextKeys}, nil
		}
	}
	return af.Configuration{}, af.ErrDenied
}

type fixtureContext struct{ f *fixture }

func (c fixtureContext) Load(_ context.Context, s af.Scope, _ []string, _ int) (af.ContextBundle, error) {
	return af.ContextBundle{Scope: s, Values: map[string]json.RawMessage{RequestKey: c.f.request}}, nil
}
func (f *fixture) Check(_ context.Context, s af.Scope, permission string) error {
	if f.deny || s != f.scope || (f.grant != "" && permission != f.grant) {
		return af.ErrDenied
	}
	return nil
}
func (f *fixture) Claim(_ context.Context, t af.Task) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.claims[t.ID] {
		return false, nil
	}
	f.claims[t.ID] = true
	return true, nil
}
func (f *fixture) Finish(context.Context, af.Task, af.Status) error { return nil }
func (f *fixture) Record(context.Context, af.AuditEvent) error      { return nil }
func (f *fixture) Resolve(_ context.Context, s af.Scope) (RunnerBinding, error) {
	bound := f.scope
	if f.foreign {
		bound.PrincipalID = "foreign"
	}
	return RunnerBinding{bound, f}, nil
}
func (f *fixture) Execute(_ context.Context, name, raw string) (agent.ToolResult, error) {
	if _, err := agent.Validate(name, raw); err != nil {
		return agent.ToolResult{}, err
	}
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	// Synthetic facade facts only; never a DB or provider request.
	return agent.ToolResult{OK: true, Data: map[string]any{"source": "synthetic", "action": name}}, nil
}
func (f *fixture) Read(_ context.Context, s af.Scope) (ReadResult, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.foreign {
		s.PrincipalID = "foreign"
	}
	return ReadResult{Scope: s, Revision: "fixture-v1", ObservedAt: time.Now().Add(-time.Minute), ValidUntil: time.Now().Add(time.Minute), Metrics: []Metric{{Name: "synthetic_count", Value: 2, Unit: "count"}}}, nil
}
func setup(t *testing.T, business, raw string) (*af.Orchestrator, *fixture) {
	t.Helper()
	f := &fixture{scope: scope, request: json.RawMessage(raw), claims: map[string]bool{}}
	rs, err := Registrations(scope.TenantID, fixtureDependencies(business, f))
	if err != nil {
		t.Fatal(err)
	}
	f.registrations = rs
	registry, err := af.NewRegistry(rs)
	if err != nil {
		t.Fatal(err)
	}
	o := &af.Orchestrator{Registry: registry, Configurations: f, Permissions: f, Contexts: fixtureContext{f}, Tasks: f, Audit: f, Limits: af.Limits{MaxSteps: 4, MaxContextBytes: 4096, MaxResultBytes: 32768, MaxUnits: 0, Timeout: time.Second}}
	return o, f
}
func task(id string) af.Task { return af.Task{ID: "task-" + id, AgentID: id, Scope: scope} }
func requestFor(id string) string {
	switch id {
	case "customer", "sales", "product_inventory":
		return `{"action":"search_products","query":"synthetic","limit":1}`
	case "marketing":
		return `{"action":"draft"}`
	case "business_manager":
		return `{"action":"plan"}`
	default:
		return `{"action":"read"}`
	}
}
func TestBusinessSpecialistExecutionMatrix(t *testing.T) {
	for _, business := range businessTypes() {
		for _, id := range IDs() {
			t.Run(business+"/"+id, func(t *testing.T) {
				o, f := setup(t, business, requestFor(id))
				a, ok := o.Registry.Lookup(scope.TenantID, id)
				toolCount := 1
				switch id {
				case "customer", "sales":
					toolCount = 6
				case "product_inventory":
					toolCount = 4
				}
				if !ok || a.Name == "" || a.Description == "" || len(a.Tools) != toolCount || len(a.Permissions) != toolCount || len(a.Capabilities) != toolCount {
					t.Fatal("missing descriptor")
				}
				result, err := o.Run(context.Background(), task(id))
				catalogAgent := id == "customer" || id == "sales" || id == "product_inventory"
				if business != "electronics" && catalogAgent {
					if !errors.Is(err, af.ErrDenied) || f.calls != 0 || a.Status != af.AgentDisabled {
						t.Fatalf("unsupported domain executed: %v calls=%d", err, f.calls)
					}
					return
				}
				if err != nil || len(result.Results) != 1 || result.Steps != 2 {
					t.Fatalf("execution: %+v %v", result, err)
				}
				switch id {
				case "marketing":
					var d Draft
					if json.Unmarshal(result.Results[0], &d) != nil || !d.Draft || d.ExternalCommunication {
						t.Fatal("unsafe draft")
					}
				case "business_manager":
					var p ManagementPlan
					if json.Unmarshal(result.Results[0], &p) != nil || p.CoordinationEnabled || p.Status != "blocked_shared_budget" {
						t.Fatal("unsafe manager")
					}
				case "finance", "analytics":
					var r ReadResult
					if json.Unmarshal(result.Results[0], &r) != nil || r.Scope != scope || len(r.Metrics) != 1 {
						t.Fatal("missing scoped evidence")
					}
				default:
					var r bridgeResult
					if json.Unmarshal(result.Results[0], &r) != nil || r.ScopeHash != bridgeScopeHash(scope) || !json.Valid([]byte(r.ResultJSON)) || f.calls != 1 {
						t.Fatal("bridge not executed")
					}
				}
			})
		}
	}
}

func TestBridgeActionPermissionsAndEffects(t *testing.T) {
	const uuid = "00000000-0000-4000-8000-000000000001"
	actions := []struct {
		name, input string
		kind        af.ActionKind
	}{
		{"search_products", requestFor("customer"), af.ActionRead},
		{"get_product_details", `{"action":"get_product_details","product_id":"` + uuid + `"}`, af.ActionRead},
		{"get_product_price", `{"action":"get_product_price","variant_id":"` + uuid + `"}`, af.ActionRead},
		{"check_inventory", `{"action":"check_inventory","variant_id":"` + uuid + `","quantity":1}`, af.ActionRead},
		{"get_order_status", `{"action":"get_order_status","order_id":"` + uuid + `"}`, af.ActionRead},
		{"handoff_to_human", `{"action":"handoff_to_human","reason":"customer_request"}`, af.ActionUpdate},
	}
	for index, id := range []string{"customer", "sales", "product_inventory"} {
		supported := actions
		if id == "product_inventory" {
			supported = actions[:4]
		}
		for i, action := range supported {
			t.Run(id+"/"+action.name, func(t *testing.T) {
				other := supported[(i+1)%len(supported)]
				o, f := setup(t, "electronics", other.input)
				f.grant = id + "." + action.name
				r := f.registrations[index]
				var names []string
				for _, expected := range supported {
					names = append(names, id+"."+expected.name)
				}
				if r.Agent.ID != id || !slices.Equal(r.Agent.Tools, names) || !slices.Equal(r.Agent.Permissions, names) || !slices.Equal(r.Agent.Capabilities, names) {
					t.Fatalf("unexpected descriptor: %+v", r.Agent)
				}
				tool := r.Tools[i]
				sp := tool.Spec()
				if sp.Name != f.grant || sp.Permission != f.grant || sp.Kind != action.kind || sp.RequiresApproval {
					t.Fatalf("unexpected tool spec: %+v", sp)
				}
				if _, err := tool.Execute(context.Background(), scope, json.RawMessage(other.input)); !errors.Is(err, af.ErrDenied) || f.calls != 0 {
					t.Fatalf("tool allowed a different action: %v calls=%d", err, f.calls)
				}
				if result, err := o.Run(context.Background(), task(id)); !errors.Is(err, af.ErrDenied) || len(result.Results) != 0 || f.calls != 0 {
					t.Fatalf("ungranted %s ran with %s granted: %v calls=%d", other.name, action.name, err, f.calls)
				}
				f.request = json.RawMessage(action.input)
				allowed := task(id)
				allowed.ID += "-allowed"
				result, err := o.Run(context.Background(), allowed)
				if err != nil || len(result.Results) != 1 || f.calls != 1 {
					t.Fatalf("granted action failed: %v calls=%d", err, f.calls)
				}
				var output bridgeResult
				if err := json.Unmarshal(result.Results[0], &output); err != nil {
					t.Fatal(err)
				}
				var domainResult struct {
					Data struct{ Action string }
				}
				if err := json.Unmarshal([]byte(output.ResultJSON), &domainResult); err != nil || domainResult.Data.Action != action.name {
					t.Fatalf("wrong Runner dispatch: %q %v", domainResult.Data.Action, err)
				}
			})
		}
	}
}

func TestBridgeResultScopeBinding(t *testing.T) {
	o, f := setup(t, "electronics", requestFor("customer"))
	result, err := o.Run(context.Background(), task("customer"))
	if err != nil || len(result.Results) != 1 {
		t.Fatalf("bridge result: %v", err)
	}
	raw := result.Results[0]
	tool := f.registrations[0].Tools[0]
	if err := tool.ValidateOutput(scope, raw); err != nil {
		t.Fatal(err)
	}
	for _, foreign := range []af.Scope{
		{TenantID: "foreign", PrincipalID: scope.PrincipalID, ConversationID: scope.ConversationID},
		{TenantID: scope.TenantID, PrincipalID: "foreign", ConversationID: scope.ConversationID},
		{TenantID: scope.TenantID, PrincipalID: scope.PrincipalID, ConversationID: "foreign"},
	} {
		if err := tool.ValidateOutput(foreign, raw); !errors.Is(err, af.ErrDenied) {
			t.Fatalf("accepted foreign result: %v", err)
		}
	}
	for _, id := range []string{scope.TenantID, scope.PrincipalID, scope.ConversationID} {
		if strings.Contains(string(raw), id) {
			t.Fatal("bridge serialized a raw scope identifier")
		}
	}
}

func TestEachSpecialistDeniesUnauthorizedAndInjectedScope(t *testing.T) {
	for _, id := range IDs() {
		t.Run(id, func(t *testing.T) {
			o, f := setup(t, "electronics", requestFor(id))
			f.deny = true
			if _, err := o.Run(context.Background(), task(id)); !errors.Is(err, af.ErrDenied) || f.calls != 0 {
				t.Fatalf("permission bypass %v", err)
			}
			for _, raw := range []string{`{"action":"read","tenant_id":"foreign"}`, `{"action":"read","action":"draft"}`, `{"action":"read","principal_id":"foreign"}`, `{"Action":"read"}`, `{"action":null}`} {
				o, f = setup(t, "electronics", raw)
				if _, err := o.Run(context.Background(), task(id)); err == nil || f.calls != 0 {
					t.Fatalf("accepted %s: %v", raw, err)
				}
			}
			o, f = setup(t, "electronics", requestFor(id))
			foreign := task(id)
			foreign.Scope.TenantID = "foreign"
			if _, err := o.Run(context.Background(), foreign); !errors.Is(err, af.ErrDenied) || f.calls != 0 {
				t.Fatalf("foreign tenant %v", err)
			}
		})
	}
}
func TestBridgeForeignBindingAndMutations(t *testing.T) {
	for _, id := range []string{"customer", "sales", "product_inventory"} {
		t.Run(id, func(t *testing.T) {
			o, f := setup(t, "electronics", requestFor(id))
			f.foreign = true
			if _, err := o.Run(context.Background(), task(id)); !errors.Is(err, af.ErrDenied) || f.calls != 0 {
				t.Fatalf("foreign principal binding %v", err)
			}
			requests := []string{
				`{"action":"create_order_request","quote_id":"00000000-0000-4000-8000-000000000001"}`,
				`{"action":"admin"}`,
				`{"action":"search_products","query":"x","limit":1,"target":"finance"}`,
				`{"action":"search_products","query":"x","limit":1,"tenant_id":"foreign"}`,
				`{"action":"handoff_to_human","action":"search_products","query":"x","limit":1}`,
			}
			if id == "product_inventory" {
				requests = append(requests, `{"action":"handoff_to_human","reason":"customer_request"}`, `{"action":"get_order_status","order_id":"00000000-0000-4000-8000-000000000001"}`)
			}
			for _, raw := range requests {
				o, f = setup(t, "electronics", raw)
				if _, err := o.Run(context.Background(), task(id)); err == nil || f.calls != 0 {
					t.Fatalf("mutation accepted: %v", err)
				}
			}
		})
	}
}
func TestBindRunnerChecksSessionAndConversation(t *testing.T) {
	r := &agent.Runner{Tenant: catalog.Tenant{OrganizationID: scope.TenantID, Session: auth.Session{User: auth.User{ID: scope.PrincipalID}}}, ConversationID: scope.ConversationID}
	if _, err := BindRunner(scope, r); err != nil {
		t.Fatal(err)
	}
	for _, s := range []af.Scope{{TenantID: "foreign", PrincipalID: scope.PrincipalID, ConversationID: scope.ConversationID}, {TenantID: scope.TenantID, PrincipalID: "foreign", ConversationID: scope.ConversationID}, {TenantID: scope.TenantID, PrincipalID: scope.PrincipalID, ConversationID: "foreign"}} {
		if _, err := BindRunner(s, r); !errors.Is(err, af.ErrDenied) {
			t.Fatal("accepted foreign binding")
		}
	}
	if _, err := BindRunner(scope, nil); !errors.Is(err, af.ErrDenied) {
		t.Fatal("accepted nil runner")
	}
}
func TestReadEvidenceRejectsForeignStaleAndMissing(t *testing.T) {
	for _, id := range []string{"finance", "analytics"} {
		t.Run(id, func(t *testing.T) {
			o, f := setup(t, "electronics", requestFor(id))
			f.foreign = true
			if result, err := o.Run(context.Background(), task(id)); !errors.Is(err, af.ErrDenied) || len(result.Results) != 0 {
				t.Fatalf("foreign evidence released %v", err)
			}
			tool, err := newReadTool(id, nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = tool.Execute(context.Background(), scope, []byte(`{"action":"read"}`)); !errors.Is(err, ErrUnavailable) {
				t.Fatal("invented missing facts")
			}
		})
	}
	valid := ReadResult{Scope: scope, Revision: "v1", ObservedAt: time.Now().Add(-time.Minute), ValidUntil: time.Now().Add(time.Minute), Metrics: []Metric{}}
	for _, mutate := range []func(*ReadResult){func(r *ReadResult) { r.ValidUntil = time.Now().Add(-time.Second) }, func(r *ReadResult) { r.ObservedAt = time.Now().Add(time.Hour) }, func(r *ReadResult) { r.Revision = "" }, func(r *ReadResult) { r.Scope.ConversationID = "foreign" }, func(r *ReadResult) { r.Metrics = nil }} {
		r := valid
		mutate(&r)
		if validateRead(scope, r) == nil {
			t.Fatal("accepted unverified evidence")
		}
	}
}
func TestManagerAndMarketingCannotSendOrDelegate(t *testing.T) {
	for _, id := range []string{"marketing", "business_manager"} {
		for _, action := range []string{"send", "publish", "delegate", "admin"} {
			t.Run(id+"/"+action, func(t *testing.T) {
				o, f := setup(t, "electronics", fmt.Sprintf(`{"action":%q}`, action))
				if _, err := o.Run(context.Background(), task(id)); err == nil || f.calls != 0 {
					t.Fatal("effect allowed")
				}
			})
		}
	}
}
func TestBridgeUnknownCostUnavailableAndDuplicate(t *testing.T) {
	for _, id := range []string{"customer", "sales", "product_inventory"} {
		t.Run(id, func(t *testing.T) {
			o, f := setup(t, "electronics", requestFor(id))
			rs, err := Registrations(scope.TenantID, Dependencies{BusinessModel: "trade", IndustryProfiles: []string{"electronics"}, Runners: f})
			if err != nil {
				t.Fatal(err)
			}
			o.Registry, err = af.NewRegistry(rs)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = o.Run(context.Background(), task(id)); !errors.Is(err, af.ErrBudget) || f.calls != 0 {
				t.Fatalf("unknown cost admitted %v", err)
			}
			o, f = setup(t, "electronics", requestFor(id))
			if _, err = o.Run(context.Background(), task(id)); err != nil {
				t.Fatal(err)
			}
			if _, err = o.Run(context.Background(), task(id)); !errors.Is(err, af.ErrDuplicate) || f.calls != 1 {
				t.Fatalf("duplicate dispatched %v", err)
			}
			tools, err := newBridgeTools(id, Dependencies{})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = tools[0].Execute(context.Background(), scope, []byte(requestFor(id))); !errors.Is(err, ErrUnavailable) {
				t.Fatal("missing runner accepted")
			}
		})
	}
}
func TestCancellationAndDescriptorCopies(t *testing.T) {
	names := IDs()
	names[0] = "admin"
	if IDs()[0] != "customer" {
		t.Fatal("mutable identifiers")
	}
	for _, id := range IDs() {
		o, f := setup(t, "electronics", requestFor(id))
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := o.Run(ctx, task(id)); !errors.Is(err, context.Canceled) || f.calls != 0 {
			t.Fatalf("%s cancellation bypass %v", id, err)
		}
	}
	if _, err := Registrations(scope.TenantID, Dependencies{BusinessModel: "invalid/model"}); !errors.Is(err, af.ErrInvalid) {
		t.Fatal("invalid business code accepted")
	}
}

// Fixture industries are examples, never a closed production model registry.
func businessTypes() []string {
	return []string{"electronics", "autoparts", "building", "furniture", "vehicle_service", "service_company", "real_estate", "hybrid"}
}
func fixtureDependencies(industry string, f *fixture) Dependencies {
	model := "trade"
	switch industry {
	case "vehicle_service", "service_company":
		model = "service"
	case "real_estate":
		model = "agency"
	case "hybrid":
		model = "hybrid"
	}
	return Dependencies{BusinessModel: model, IndustryProfiles: []string{industry}, Runners: f, Finance: f, Analytics: f, BridgeCostKnown: true}
}
