// Package specialists defines compiled, offline adapters for agentfoundation.
// It supplies no provider, persistence, runtime wiring, or external sender.
package specialists

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	af "github.com/karadjev/AI-Business-Assistant/apps/api/internal/agentfoundation"
)

var ErrUnavailable = errors.New("specialists: capability unavailable")

const RequestKey = "specialist_request"

var ids = [...]string{"customer", "sales", "product_inventory", "marketing", "finance", "analytics", "business_manager"}

type Request struct {
	Action    string `json:"action"`
	Query     string `json:"query,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	ProductID string `json:"product_id,omitempty"`
	VariantID string `json:"variant_id,omitempty"`
	Quantity  int    `json:"quantity,omitempty"`
	OrderID   string `json:"order_id,omitempty"`
	Reason    string `json:"reason,omitempty"`
	Target    string `json:"target,omitempty"`
}

// Reader is a trusted backend projection, not an arbitrary report/SQL executor.
// Read must enforce current principal access and resource ownership at source.
// Scope, revision and validity are independently checked before releasing facts.
type Reader interface {
	Read(context.Context, af.Scope) (ReadResult, error)
}
type Metric struct {
	Name  string `json:"name"`
	Value int64  `json:"value"`
	Unit  string `json:"unit"`
}
type ReadResult struct {
	Scope      af.Scope  `json:"scope"`
	Revision   string    `json:"revision"`
	ObservedAt time.Time `json:"observed_at"`
	ValidUntil time.Time `json:"valid_until"`
	Metrics    []Metric  `json:"metrics"`
}

// IDs returns a fresh copy of the compiled specialist identifiers.
func IDs() []string { return append([]string(nil), ids[:]...) }

type Dependencies struct {
	// Model and profiles are independently approved backend configuration, never
	// tool arguments. New valid model/profile codes do not require registry edits.
	BusinessModel          string
	CatalogProfileApproved bool // Trusted persisted capability projection for durable tasks.
	IndustryProfiles       []string
	// BusinessType is a compatibility alias for BusinessModel, NOT an industry.
	// Ambiguous conflicting values are rejected; no electronics default is inferred.
	BusinessType string
	Runners      RunnerResolver
	Finance      Reader
	Analytics    Reader
	Growth       Reader
	GrowthPlan   bool
	// BridgeCostKnown must only be set after backend admission has established the
	// cost of the existing domain executor. Unknown cost stays disabled.
	BridgeCostKnown bool
	BridgeCostUnits int64
}

type planner struct {
	tools    []af.Tool
	bridgeID string
}

func (p planner) Next(ctx context.Context, s af.Scope, snap af.Snapshot) (af.Decision, error) {
	if err := ctx.Err(); err != nil {
		return af.Decision{}, err
	}
	if snap.Context.Scope != s {
		return af.Decision{}, af.ErrDenied
	}
	if len(snap.Results) > 0 {
		return af.Decision{Done: true}, nil
	}
	raw, ok := snap.Context.Values[RequestKey]
	if !ok {
		return af.Decision{}, ErrUnavailable
	}
	tool := p.tools[0]
	if p.bridgeID != "" {
		var request struct {
			Action string `json:"action"`
		}
		// This decode selects a candidate only. The selected tool strictly
		// validates the entire request and its fixed action before execution.
		if err := json.Unmarshal(raw, &request); err != nil {
			return af.Decision{}, af.ErrInvalid
		}
		tool = nil
		for _, candidate := range p.tools {
			if candidate.Spec().Name == p.bridgeID+"."+request.Action {
				tool = candidate
				break
			}
		}
		if tool == nil {
			return af.Decision{}, af.ErrDenied
		}
	}
	canonical, err := tool.CanonicalInput(s, raw)
	if err != nil {
		return af.Decision{}, err
	}
	return af.Decision{Tool: tool.Spec().Name, Input: canonical}, nil
}

// Registrations returns seven tenant-bound definitions. Missing readers remain
// explicitly unavailable on execution; no metrics or business facts are invented.
func Registrations(tenant string, d Dependencies) ([]af.Registration, error) {
	if d.BridgeCostUnits < 0 {
		return nil, af.ErrInvalid
	}
	model := d.BusinessModel
	if model == "" {
		model = d.BusinessType
	}
	if !businessCode.MatchString(model) || (d.BusinessType != "" && d.BusinessType != model) || len(d.IndustryProfiles) > 8 {
		return nil, af.ErrInvalid
	}
	seen := map[string]bool{}
	for _, profile := range d.IndustryProfiles {
		if !businessCode.MatchString(profile) || seen[profile] {
			return nil, af.ErrInvalid
		}
		seen[profile] = true
	}
	// The existing engine is electronics-v1. An unknown/mixed specialization must
	// never silently receive its catalog workflow. Generic read/draft/plan adapters
	// stay available for approved new models, subject to normal permissions.
	catalogAdmitted := (model == "trade" || model == "hybrid") && len(d.IndustryProfiles) == 1 && (d.IndustryProfiles[0] == "electronics" || d.CatalogProfileApproved)
	out := make([]af.Registration, 0, len(ids))
	for _, id := range ids {
		var tool af.Tool
		var tools []af.Tool
		var err error
		var bridgeID string
		switch id {
		case "customer", "sales", "product_inventory":
			bridgeID = id
			tools, err = newBridgeTools(id, d)
			if err == nil && catalogAdmitted && d.Growth != nil && id != "customer" {
				action := "review_sales"
				if id == "product_inventory" {
					action = "review_inventory"
				}
				review, reviewErr := newMetricTool(id, action, d.Growth)
				err = reviewErr
				tools = append(tools, review)
			}
		case "finance":
			tool, err = newReadTool(id, d.Finance)
		case "analytics":
			tool, err = newReadTool(id, d.Analytics)
		case "marketing":
			tool, err = newDraftTool()
		case "business_manager":
			tool, err = newManagerTool(catalogAdmitted, d.GrowthPlan)
		}
		if err != nil {
			return nil, err
		}
		if tool != nil {
			tools = []af.Tool{tool}
		}
		names := make([]string, 0, len(tools))
		permissions := make([]string, 0, len(tools))
		for _, tool := range tools {
			sp := tool.Spec()
			names = append(names, sp.Name)
			permissions = append(permissions, sp.Permission)
		}
		status := af.AgentEnabled
		types := []string{model}
		profiles := append([]string(nil), d.IndustryProfiles...)
		if bridgeID != "" {
			types = []string{"trade", "hybrid"}
			profiles = []string{"electronics"}
			if catalogAdmitted {
				profiles = append([]string(nil), d.IndustryProfiles...)
			}
			if !catalogAdmitted {
				status = af.AgentDisabled
			}
		}
		out = append(out, af.Registration{
			TenantID: tenant,
			Agent: af.Agent{
				ID: id, Version: "v1", Name: id, Description: description(id),
				Capabilities: append([]string(nil), permissions...), Permissions: permissions,
				SupportedBusinessTypes: types, SupportedIndustryProfiles: profiles, Status: status,
				ExecutionLimits: af.Limits{MaxSteps: 2, MaxContextBytes: 4096, MaxResultBytes: 32768, Timeout: 5 * time.Second},
				Tools:           names, ContextKeys: []string{RequestKey},
				Planner: planner{tools: append([]af.Tool(nil), tools...), bridgeID: bridgeID}, PlanningCostKnown: true,
			},
			Tools: tools,
		})
	}
	return out, nil
}
func spec(name, permission string) af.ToolSpec {
	return af.ToolSpec{Name: name, Version: "v1", Permission: permission, CostKnown: true, Kind: af.ActionRead}
}
func onlyAction(in Request, action string) error {
	if in != (Request{Action: action}) {
		return af.ErrDenied
	}
	return nil
}
func newReadTool(id string, r Reader) (af.Tool, error) {
	return newMetricTool(id, "read", r)
}

func newMetricTool(id, action string, r Reader) (af.Tool, error) {
	return af.NewJSONTool(spec(id+"."+action, id+"."+action), func(_ af.Scope, in Request) error { return onlyAction(in, action) }, validateRead, func(ctx context.Context, s af.Scope, _ Request) (ReadResult, error) {
		if r == nil {
			return ReadResult{}, ErrUnavailable
		}
		return r.Read(ctx, s)
	})
}
func validateRead(s af.Scope, r ReadResult) error {
	now := time.Now()
	if r.Scope != s || r.Revision == "" || len(r.Revision) > 128 || r.ObservedAt.IsZero() || r.ObservedAt.After(now) || !r.ValidUntil.After(now) || !r.ValidUntil.After(r.ObservedAt) || len(r.Metrics) > 32 || r.Metrics == nil {
		return af.ErrDenied
	}
	seen := map[string]bool{}
	for _, m := range r.Metrics {
		if len(m.Name) == 0 || len(m.Name) > 64 || seen[m.Name] || len(m.Unit) == 0 || len(m.Unit) > 16 {
			return af.ErrInvalid
		}
		seen[m.Name] = true
	}
	return nil
}

type Draft struct {
	Draft                 bool   `json:"draft"`
	Text                  string `json:"text"`
	ExternalCommunication bool   `json:"external_communication"`
}

func newDraftTool() (af.Tool, error) {
	sp := spec("marketing.draft", "marketing.draft")
	sp.Kind = af.ActionCreate
	return af.NewJSONTool(sp, func(_ af.Scope, in Request) error { return onlyAction(in, "draft") }, func(_ af.Scope, d Draft) error {
		if !d.Draft || d.ExternalCommunication || d.Text != draftText {
			return af.ErrDenied
		}
		return nil
	}, func(context.Context, af.Scope, Request) (Draft, error) { return Draft{true, draftText, false}, nil })
}

const draftText = "Черновик: уточните у магазина актуальные предложения и условия. Требуется проверка сотрудником перед публикацией."

type ManagementPlan struct {
	Targets             []string `json:"targets"`
	CoordinationEnabled bool     `json:"coordination_enabled"`
	Status              string   `json:"status"`
}

func newManagerTool(catalogAdmitted, growth bool) (af.Tool, error) {
	targets := []string{"finance", "analytics"}
	if catalogAdmitted {
		targets = append([]string{"product_inventory"}, targets...)
	}

	action, status := "plan", "blocked_shared_budget"
	if growth {
		if !catalogAdmitted {
			return nil, af.ErrDenied
		}
		targets = []string{"analytics", "sales", "product_inventory", "marketing"}
		action, status = "plan_growth", "child_results_collected"
	}
	return af.NewJSONTool(spec("business_manager."+action, "specialists.plan.read"), func(_ af.Scope, in Request) error {
		if in.Action == "delegate" {
			return ErrUnavailable
		}
		return onlyAction(in, action)
	}, func(_ af.Scope, p ManagementPlan) error {
		if p.CoordinationEnabled != growth || p.Status != status || !sameTargets(p.Targets, targets) {
			return af.ErrDenied
		}
		return nil
	}, func(context.Context, af.Scope, Request) (ManagementPlan, error) {
		return ManagementPlan{append([]string(nil), targets...), growth, status}, nil
	})
}
func description(id string) string {
	switch id {
	case "customer", "sales":
		return "Existing Runner facade; no provider or independent consent engine; order creation unavailable"
	case "product_inventory":
		return "Read-only existing customer catalog facade; no inventory mutation"
	case "marketing":
		return "Deterministic review-required draft; no external communication"
	case "finance", "analytics":
		return "Read-only validated scoped projection; unavailable without injected reader"
	default:
		return "Deterministic read-work plan; delegation blocked pending shared budget and cycle enforcement"
	}
}
func bounded(s string, n int) bool { return len(s) <= n && !strings.ContainsRune(s, 0) }

var businessCode = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

func sameTargets(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
