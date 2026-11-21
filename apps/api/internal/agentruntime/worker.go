package agentruntime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/agent"
	af "github.com/karadjev/AI-Business-Assistant/apps/api/internal/agentfoundation"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/specialists"
)

type claimed struct {
	ID, OrganizationID, Initiator, AgentID, Action, ExecutionID  string
	VariantID, ConversationID, CorrelationID, LegacyBusinessType string
	Deadline                                                     time.Time
}

// ProcessOne shares the existing worker lifecycle. Claim and finish are fenced
// by an agent-only queue lease. Every currently admitted operation is a read
// or an internal draft, so interrupted attempts have no external side effects.
func (s Service) ProcessOne(ctx context.Context) (bool, error) {
	if len(s.Key) != 32 {
		return false, auth.Unavailable()
	}
	lease := auth.UUID()
	var raw []byte
	if err := s.Pool.QueryRow(ctx, "SELECT platform.agent_claim($1,$2)", s.Proof(), lease).Scan(&raw); err != nil {
		return false, err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return false, nil
	}
	var job claimed
	if err := json.Unmarshal(raw, &job); err != nil {
		return true, err
	}
	work, stop := context.WithDeadline(ctx, job.Deadline)
	defer stop()
	result, err := s.run(work, job, lease)
	finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()
	var payload json.RawMessage
	var failure *string
	if err == nil {
		payload, err = json.Marshal(result.Results)
		if err == nil && (job.Action == "coordinate" || job.Action == "coordinate_growth") {
			var childResults json.RawMessage
			err = s.Pool.QueryRow(finish, "SELECT platform.agent_child_results($1,$2)", job.ID, lease).Scan(&childResults)
			if err == nil {
				payload, err = json.Marshal(struct {
					Plan, Children  json.RawMessage
					Recommendations []string `json:"recommendations,omitempty"`
				}{payload, childResults, growthRecommendations(job.Action, childResults)})
			}
		}
	}
	if err != nil {
		payload = nil
		code := "execution"
		var ae *auth.Error
		if errors.Is(err, af.ErrDenied) || errors.Is(err, af.ErrInvalid) || errors.Is(err, af.ErrBudget) ||
			errors.Is(err, af.ErrApproval) || errors.Is(err, af.ErrLimit) || (errors.As(err, &ae) && ae.Status < 500) {
			code = "denied"
		}
		failure = &code
	}
	if err == nil && delegationParent(job.Action) && len(result.Results) == 1 {
		var aggregate delegationResult
		if json.Unmarshal(result.Results[0], &aggregate) == nil && aggregate.State == "partial_failure" {
			code := "denied"
			failure = &code
		}
	}
	var ok bool
	finishErr := s.Pool.QueryRow(finish, "SELECT platform.agent_finish($1,$2,$3,$4::jsonb,$5,$6)", job.ID, lease, job.ExecutionID, payload, failure, failure != nil && *failure == "execution").Scan(&ok)
	if finishErr != nil {
		return true, finishErr
	}
	if !ok {
		return true, errors.New("agent task lease or audit fence rejected completion")
	}
	return true, nil
}

func (s Service) run(ctx context.Context, j claimed, lease string) (af.Result, error) {
	conversation := j.ConversationID
	if conversation == "" {
		conversation = j.ID
	} // task-scoped context for owner-only operations
	scope := af.Scope{TenantID: j.OrganizationID, PrincipalID: j.Initiator, ConversationID: conversation}
	action := j.Action
	switch action {
	case "coordinate":
		action = "plan"
	case "coordinate_growth":
		action = "plan_growth"
	}
	a := &attempt{service: s, job: j, lease: lease, scope: scope, tool: j.AgentID + "." + action}
	if strings.HasPrefix(action, "delegation.") || delegationParent(action) {
		a.permission = a.tool
		return a.runDelegation(ctx)
	}
	profile, err := a.loadProfile(ctx)
	if err != nil {
		return af.Result{}, err
	}
	if catalogAction(action) && !profile.catalogAllowed() {
		return af.Result{}, af.ErrDenied
	}
	a.profile, err = json.Marshal(profile)
	if err != nil {
		return af.Result{}, err
	}
	request := specialists.Request{Action: action, VariantID: j.VariantID}
	switch action {
	case "check_inventory":
		request.Quantity = 1
	case "get_product_details":
		binding := catalog.Tenant{OrganizationID: j.OrganizationID, AgentTaskID: j.ID, AgentLease: lease}
		productID, err := (catalog.Service{Pool: s.Pool}).Customer().ScopedProductForVariant(ctx, binding, j.VariantID)
		if err != nil {
			return af.Result{}, err
		}
		request.VariantID = ""
		request.ProductID = productID
	}
	a.request, err = json.Marshal(request)
	if err != nil {
		return af.Result{}, err
	}
	a.permission = a.tool
	if j.AgentID == "business_manager" {
		a.permission = "specialists.plan.read"
	}
	deps := profile.dependencies()
	deps.Runners, deps.Finance, deps.Analytics, deps.Growth = a, a, a, a
	deps.GrowthPlan = j.Action == "coordinate_growth"
	defs, err := specialists.Registrations(j.OrganizationID, deps)
	for i := range defs {
		defs[i].Agent.ContextKeys = append(defs[i].Agent.ContextKeys, "business_profile")
	}
	if err != nil {
		return af.Result{}, err
	}
	if err := a.bindMarketing(defs); err != nil {
		return af.Result{}, err
	}
	registry, err := af.NewRegistry(defs)
	if err != nil {
		return af.Result{}, err
	}
	router := af.Orchestrator{Registry: registry, Configurations: a, Permissions: a, Contexts: contextSource{a}, Tasks: a, Audit: a,
		Limits: af.Limits{MaxSteps: 2, MaxUnits: 0, MaxContextBytes: 4096, MaxResultBytes: 32768, Timeout: 5 * time.Second}}
	return router.RunCapability(ctx, af.Task{ID: j.ID, CorrelationID: j.CorrelationID, Scope: scope}, a.permission)
}

type attempt struct {
	service    Service
	job        claimed
	lease      string
	scope      af.Scope
	tool       string
	permission string
	request    json.RawMessage
	profile    json.RawMessage
	claimed    bool
}

func (a *attempt) Load(ctx context.Context, s af.Scope, id string) (af.Configuration, error) {
	if s != a.scope || id != a.job.AgentID {
		return af.Configuration{}, af.ErrDenied
	}
	return af.Configuration{TenantID: s.TenantID, AgentID: id, Version: "stage23-v1", Enabled: true, Tools: []string{a.tool}, ContextKeys: []string{specialists.RequestKey, "business_profile"}}, ctx.Err()
}
func (a *attempt) Check(ctx context.Context, s af.Scope, permission string) error {
	if s != a.scope || permission != a.permission {
		return af.ErrDenied
	}
	return ctx.Err()
}
func (a *attempt) Claim(ctx context.Context, t af.Task) (bool, error) {
	if t.ID != a.job.ID || t.Scope != a.scope || t.AgentID != a.job.AgentID || a.claimed {
		return false, af.ErrDenied
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	a.claimed = true // SQL claimed this exact attempt before constructing the adapter.
	return true, nil
}
func (a *attempt) Finish(_ context.Context, t af.Task, _ af.Status) error {
	if t.ID != a.job.ID || t.Scope != a.scope || t.AgentID != a.job.AgentID || !a.claimed {
		return af.ErrDenied
	}
	return nil // ProcessOne persists result, attempt, approval and queue completion together.
}
func (a *attempt) Record(ctx context.Context, event af.AuditEvent) error {
	if event.Step < 1 || event.ActionHash == af.ParamsHash(json.RawMessage("task")) {
		return nil
	}
	var ok bool
	err := a.service.Pool.QueryRow(ctx, "SELECT platform.agent_audit_tool($1,$2,$3,$4,nullif($5,''))", a.job.ID, a.lease, a.job.ExecutionID, string(event.Status), string(event.FailureCode)).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return af.ErrDenied
	}
	return nil
}
func (a *attempt) Resolve(ctx context.Context, s af.Scope) (specialists.RunnerBinding, error) {
	if s != a.scope || !catalogAction(a.job.Action) || a.job.ConversationID == "" {
		return specialists.RunnerBinding{}, af.ErrDenied
	}
	if err := ctx.Err(); err != nil {
		return specialists.RunnerBinding{}, err
	}
	return specialists.BindRunner(s, &agent.Runner{
		Catalog:        catalog.Service{Pool: a.service.Pool},
		ConversationID: a.job.ConversationID,
		Tenant:         catalog.Tenant{OrganizationID: a.job.OrganizationID, AgentTaskID: a.job.ID, AgentLease: a.lease},
	})
}
func (a *attempt) Read(ctx context.Context, s af.Scope) (specialists.ReadResult, error) {
	if s != a.scope {
		return specialists.ReadResult{}, af.ErrDenied
	}
	if a.job.Action == "review_sales" || a.job.Action == "review_inventory" {
		return a.readGrowth(ctx, s)
	}
	if a.job.AgentID != "analytics" && a.job.AgentID != "finance" {
		return specialists.ReadResult{}, af.ErrDenied
	}
	var data []byte
	if err := a.service.Pool.QueryRow(ctx, "SELECT platform.agent_metrics_projection($1,$2)", a.job.ID, a.lease).Scan(&data); err != nil {
		return specialists.ReadResult{}, err
	}
	var metrics struct {
		Count, Revenue int64
		PreviousCount  int64 `json:"previous_count"`
		PreviousValue  int64 `json:"previous_value"`
	}
	if err := json.Unmarshal(data, &metrics); err != nil {
		return specialists.ReadResult{}, err
	}
	now := time.Now()
	values := []specialists.Metric{
		{Name: "completed_orders_created_last_30d", Value: metrics.Count, Unit: "orders"},
		{Name: "completed_order_value_last_30d", Value: metrics.Revenue, Unit: "RUB_minor"},
		{Name: "completed_orders_created_previous_30d", Value: metrics.PreviousCount, Unit: "orders"},
		{Name: "completed_order_value_previous_30d", Value: metrics.PreviousValue, Unit: "RUB_minor"},
	}
	if metrics.Count > 0 {
		values = append(values, specialists.Metric{Name: "average_completed_order_value_last_30d", Value: metrics.Revenue / metrics.Count, Unit: "RUB_minor"})
	}
	return specialists.ReadResult{Scope: s, Revision: a.job.ExecutionID, ObservedAt: now, ValidUntil: now.Add(time.Minute), Metrics: values}, nil
}

type contextSource struct{ a *attempt }

func (c contextSource) Load(ctx context.Context, s af.Scope, keys []string, max int) (af.ContextBundle, error) {
	if s != c.a.scope || len(keys) != 2 || keys[0] != specialists.RequestKey || keys[1] != "business_profile" || len(c.a.request)+len(c.a.profile)+len(keys[0])+len(keys[1]) > max {
		return af.ContextBundle{}, af.ErrDenied
	}
	return af.ContextBundle{Scope: s, Values: map[string]json.RawMessage{specialists.RequestKey: c.a.request, "business_profile": c.a.profile}}, ctx.Err()
}
