package agentruntime

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	af "github.com/karadjev/AI-Business-Assistant/apps/api/internal/agentfoundation"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
)

const dailyCoordination = "coordinate_daily"

type serviceOfferDraft struct {
	ConversationID string `json:"conversation_id"`
	OfferingName   string `json:"offering_name"`
	Text           string `json:"text,omitempty"`
}

type dailyChild struct {
	Agent  string          `json:"agent_id"`
	Status string          `json:"status"`
	Error  string          `json:"error_code,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
}

type dailyReport struct {
	Date            string              `json:"date"`
	Timezone        string              `json:"timezone"`
	AsOf            string              `json:"as_of"`
	Metrics         map[string]*int64   `json:"metrics"`
	Proposals       []serviceOfferDraft `json:"proposals,omitempty"`
	State           string              `json:"state"`
	Children        []dailyChild        `json:"children,omitempty"`
	Recommendations []string            `json:"recommendations,omitempty"`
}

func (s Service) submitDaily(ctx context.Context, t catalog.Tenant) (string, error) {
	tx, err := s.ownerTx(ctx, t)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	hash := sha256.Sum256(s.Proof())
	var id string
	if err = tx.QueryRow(ctx, "SELECT platform.agent_daily_submit($1,$2,$3)", t.OrganizationID, auth.UUID(), hash[:]).Scan(&id); err != nil {
		return "", submitError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return "", auth.Unavailable()
	}
	return id, nil
}

func (s Service) DailyForStaff(ctx context.Context, t catalog.Tenant) (json.RawMessage, error) {
	tx, err := tenancy.Begin(ctx, s.Pool, t, true, true)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var result json.RawMessage
	if err = tx.QueryRow(ctx, "SELECT platform.agent_daily_staff($1)", t.OrganizationID).Scan(&result); err != nil {
		return nil, tenancy.Error(err)
	}
	return result, nil
}

type dailyPlanner struct{ *attempt }

func (d dailyPlanner) Load(ctx context.Context, scope af.Scope, id string) (af.Configuration, error) {
	if scope != d.scope || id != d.job.AgentID {
		return af.Configuration{}, af.ErrDenied
	}
	return af.Configuration{TenantID: scope.TenantID, AgentID: id, Version: "daily-v1", Enabled: true, Tools: []string{d.tool}}, ctx.Err()
}

func (d dailyPlanner) Next(ctx context.Context, scope af.Scope, data af.Snapshot) (af.Decision, error) {
	if scope != d.scope {
		return af.Decision{}, af.ErrDenied
	}
	if len(data.Results) > 0 {
		return af.Decision{Done: true}, ctx.Err()
	}
	return af.Decision{Tool: d.tool, Input: json.RawMessage("{}")}, ctx.Err()
}

func (a *attempt) runDaily(ctx context.Context) (af.Result, error) {
	planner := dailyPlanner{a}
	tool, err := af.NewJSONTool(af.ToolSpec{Name: a.tool, Permission: a.tool, Version: "v1", Kind: af.ActionRead, CostKnown: true},
		func(scope af.Scope, _ struct{}) error {
			if scope != a.scope {
				return af.ErrDenied
			}
			return nil
		},
		func(scope af.Scope, out dailyReport) error {
			if scope != a.scope || out.Date == "" || out.Timezone == "" || out.AsOf == "" || out.State == "" {
				return af.ErrDenied
			}
			return nil
		}, a.dailySnapshot)
	if err != nil {
		return af.Result{}, err
	}
	registry, err := af.NewRegistry([]af.Registration{{TenantID: a.scope.TenantID,
		Agent: af.Agent{ID: a.job.AgentID, Name: a.job.AgentID, Status: af.AgentEnabled, Version: "daily-v1",
			Tools: []string{a.tool}, Permissions: []string{a.tool}, Planner: planner, PlanningCostKnown: true}, Tools: []af.Tool{tool}}})
	if err != nil {
		return af.Result{}, err
	}
	router := af.Orchestrator{Registry: registry, Configurations: planner, Permissions: a, Contexts: workflowContext{a.scope}, Tasks: a, Audit: a,
		Limits: af.Limits{MaxSteps: 2, MaxUnits: 0, MaxContextBytes: 1024, MaxResultBytes: 24576, Timeout: workflowTimeout}}
	return router.Run(ctx, af.Task{ID: a.job.ID, AgentID: a.job.AgentID, CorrelationID: a.job.CorrelationID, Scope: a.scope})
}

func (a *attempt) dailySnapshot(ctx context.Context, _ af.Scope, _ struct{}) (dailyReport, error) {
	var raw []byte
	if err := a.service.Pool.QueryRow(ctx, "SELECT platform.agent_daily_context($1,$2)", a.job.ID, a.lease).Scan(&raw); err != nil {
		return dailyReport{}, err
	}
	if len(raw) == 0 {
		return dailyReport{}, af.ErrDenied
	}
	var result dailyReport
	if err := json.Unmarshal(raw, &result); err != nil {
		return dailyReport{}, err
	}
	result.State = "completed"
	for key, value := range result.Metrics {
		if value == nil {
			delete(result.Metrics, key) // Tool outputs omit unavailable metrics.
		}
	}
	for i := range result.Proposals {
		draft := &result.Proposals[i]
		draft.Text = fmt.Sprintf("Здравствуйте! Если вам снова нужна услуга «%s», можем подобрать удобное время.", draft.OfferingName)
	}
	if a.job.Action != dailyCoordination {
		return result, nil
	}
	if err := a.service.Pool.QueryRow(ctx, "SELECT platform.agent_child_results($1,$2)", a.job.ID, a.lease).Scan(&raw); err != nil {
		return dailyReport{}, err
	}
	if err := json.Unmarshal(raw, &result.Children); err != nil {
		return dailyReport{}, err
	}
	for i := range result.Children {
		child := &result.Children[i]
		if string(child.Result) == "null" {
			child.Result = nil
		}
	}
	result.State, result.Recommendations = dailyRecommendations(result.Children)
	return result, nil
}

func dailyRecommendations(children []dailyChild) (string, []string) {
	facts := make(map[string]map[string]*int64)
	for _, child := range children {
		var result []dailyReport
		if child.Status == "completed" && json.Unmarshal(child.Result, &result) == nil && len(result) == 1 && result[0].State == "completed" && result[0].Metrics != nil {
			facts[child.Agent] = result[0].Metrics
		}
	}
	state := "completed"
	var advice []string
	if len(facts) != 6 || len(children) != 6 {
		state = "partial_failure"
		advice = append(advice, "Проверьте статусы специалистов. Сводка содержит только доступные результаты.")
	}
	for _, item := range []struct{ agent, key, action string }{
		{"customer", "awaiting_staff", "Проверьте в Inbox обращения без ответа сотрудника: %d."},
		{"sales", "pending_confirmation", "Рассмотрите заявки, ожидающие подтверждения: %d."},
		{"product_inventory", "stock_zero", "Проверьте позиции без доступного остатка: %d."},
		{"product_inventory", "stock_unknown", "Сверьте позиции с неизвестным остатком: %d."},
		{"product_inventory", "unassigned_bookings", "Назначьте исполнителей на сегодняшние записи: %d."},
		{"finance", "ai_unsettled_count", "Сверьте неподтверждённые AI-операции: %d."},
		{"marketing", "eligible_service_conversations", "Проверьте внутренние черновики предложений по услугам для клиентов с согласием: %d."},
	} {
		if value := facts[item.agent][item.key]; value != nil && *value > 0 {
			advice = append(advice, fmt.Sprintf(item.action, *value))
		}
	}
	return state, advice
}
