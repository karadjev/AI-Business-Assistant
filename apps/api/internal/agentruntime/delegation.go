package agentruntime

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	af "github.com/karadjev/AI-Business-Assistant/apps/api/internal/agentfoundation"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	bf "github.com/karadjev/AI-Business-Assistant/apps/api/internal/businessfoundation"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/orders"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
)

const autoPartsDelegation = "delegate_auto_parts"

type AutoPartsRequirements struct {
	OEM      string                 `json:"oem"`
	Vehicle  catalog.FitmentVehicle `json:"vehicle"`
	Quantity int                    `json:"quantity"`
}

type DelegationRequest struct {
	ConversationID string                `json:"conversation_id"`
	IdempotencyKey string                `json:"idempotency_key"`
	Requirements   AutoPartsRequirements `json:"requirements"`
}

var partReference = regexp.MustCompile(`^[[:alnum:]_. /-]{1,80}$`)

func (r AutoPartsRequirements) valid() bool {
	if r.OEM != "" && !partReference.MatchString(r.OEM) || r.Quantity < 0 || r.Quantity > 10 {
		return false
	}
	return ValidateWorkflowInput(WorkflowInput{Scenario: WorkflowAutoParts, AutoParts: &WorkflowAutoPartsInput{Vehicle: r.Vehicle}}) == nil
}

func (s Service) SubmitAutoParts(ctx context.Context, tenant catalog.Tenant, in DelegationRequest) (string, error) {
	in.Requirements.OEM = strings.ToUpper(strings.TrimSpace(in.Requirements.OEM))
	vehicle := &in.Requirements.Vehicle
	vehicle.Make, vehicle.Model, vehicle.Engine = strings.TrimSpace(vehicle.Make), strings.TrimSpace(vehicle.Model), strings.TrimSpace(vehicle.Engine)
	if !organizations.ValidID(in.ConversationID) || !organizations.ValidID(in.IdempotencyKey) || !in.Requirements.valid() {
		return "", auth.Failure(422, "INVALID_DELEGATION", "Проверьте параметры подбора")
	}
	return s.submitDelegation(ctx, tenant, in.ConversationID, in.IdempotencyKey, autoPartsDelegation, in.Requirements)
}

func (s Service) submitDelegation(ctx context.Context, tenant catalog.Tenant, conversation, key, action string, input any) (string, error) {
	if !organizations.ValidID(conversation) || !organizations.ValidID(key) {
		return "", auth.Failure(422, "INVALID_DELEGATION", "Проверьте параметры запроса")
	}
	if len(s.Key) != 32 {
		return "", auth.Unavailable()
	}
	tx, err := s.ownerTx(ctx, tenant)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	payload, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	var id string
	err = tx.QueryRow(ctx, `SELECT platform.agent_submit_delegation($1,$2,$3,$4,$5,$6::jsonb,sha256($7))`,
		tenant.OrganizationID, auth.UUID(), key, conversation, action, payload, s.Proof()).Scan(&id)
	if err != nil {
		var p *pgconn.PgError
		if errors.As(err, &p) && p.Code == "54000" {
			return "", auth.Failure(429, "RATE_LIMITED", "Повторите позже")
		}
		return "", tenancy.Error(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return "", tenancy.Error(err)
	}
	return id, nil
}

type delegationChild struct {
	ID        string            `json:"id"`
	Agent     string            `json:"agent"`
	Action    string            `json:"action"`
	Status    string            `json:"status"`
	ErrorCode string            `json:"error_code,omitempty"`
	Result    *delegationResult `json:"result,omitempty"`
}

type delegatedPart struct {
	VariantID     string `json:"variant_id"`
	Name          string `json:"name"`
	SKU           string `json:"sku"`
	Price         int64  `json:"price_amount"`
	Currency      string `json:"currency_code"`
	Available     *int   `json:"available,omitempty"`
	Compatibility string `json:"compatibility"`
}
type delegationResult struct {
	*WorkshopMarketingDraft
	MarketingEvidence    *workshopMarketingEvidence `json:"marketing_evidence,omitempty"`
	Financial            *workshopFinancialResult   `json:"financial,omitempty"`
	WorkshopPart         *workshopPart              `json:"workshop_part,omitempty"`
	PartName             string                     `json:"part_name,omitempty"`
	SKU                  string                     `json:"sku,omitempty"`
	Available            *int                       `json:"available,omitempty"`
	Route                *conversationRoute         `json:"route,omitempty"`
	Catalog              []conversationProduct      `json:"catalog,omitempty"`
	Order                *conversationOrder         `json:"order,omitempty"`
	ConversationReply    string                     `json:"conversation_reply,omitempty"`
	PropertyRequirements *PropertyRequirements      `json:"property_requirements,omitempty"`
	Properties           []orders.Property          `json:"properties,omitempty"`
	Message              string                     `json:"message,omitempty"`
	PartRequirement      string                     `json:"part_requirement,omitempty"`
	Receipt              json.RawMessage            `json:"receipt,omitempty"`
	Construction         *ConstructionRequirements  `json:"construction,omitempty"`
	Service              *ServiceRequirements       `json:"service,omitempty"`
	Calculation          *bf.ConstructionResult     `json:"calculation,omitempty"`
	Material             *delegationMaterial        `json:"material,omitempty"`
	Offering             *delegationOffering        `json:"offering,omitempty"`
	State                string                     `json:"state"`
	Required             []string                   `json:"required,omitempty"`
	Requirements         *AutoPartsRequirements     `json:"requirements,omitempty"`
	Parts                []delegatedPart            `json:"parts,omitempty"`
	Proposal             *WorkflowDraft             `json:"proposal,omitempty"`
	Children             []delegationChild          `json:"children,omitempty"`
}

type delegationContext struct {
	ConversationRouted bool              `json:"conversation_routed"`
	Input              json.RawMessage   `json:"input"`
	Children           []delegationChild `json:"children"`
}

type delegationTool struct{ *attempt }

func (d delegationTool) Load(ctx context.Context, scope af.Scope, agentID string) (af.Configuration, error) {
	if scope != d.scope || agentID != d.job.AgentID {
		return af.Configuration{}, af.ErrDenied
	}
	return af.Configuration{TenantID: scope.TenantID, AgentID: agentID, Version: "delegation-v1", Enabled: true, Tools: []string{d.tool}, ContextKeys: d.contextKeys()}, ctx.Err()
}

func (d delegationTool) Next(ctx context.Context, scope af.Scope, snapshot af.Snapshot) (af.Decision, error) {
	if scope != d.scope {
		return af.Decision{}, af.ErrDenied
	}
	if len(snapshot.Results) > 0 {
		return af.Decision{Done: true}, ctx.Err()
	}
	if workshopDelegation(d.job.Action) {
		d.workshopEvidence = snapshot.Context.Values[workshopContextKey]
	}
	return af.Decision{Tool: d.tool, Input: json.RawMessage("{}")}, ctx.Err()
}

func (d delegationTool) execute(ctx context.Context, scope af.Scope, _ struct{}) (delegationResult, error) {
	if workshopDelegation(d.job.Action) {
		return runWorkshopStep(d.job.Action, d.workshopEvidence)
	}
	if d.job.Action == "delegation.property_receipt" {
		if scope != d.scope {
			return delegationResult{}, af.ErrDenied
		}
		return d.propertyReceipt(ctx)
	}
	if d.job.Action == "delegation.service_receipt" {
		var raw []byte
		if err := d.service.Pool.QueryRow(ctx, "SELECT platform.service_receipt_context($1,$2)", d.job.ID, d.lease).Scan(&raw); err != nil {
			return delegationResult{}, err
		}
		if len(raw) == 0 || scope != d.scope {
			return delegationResult{}, af.ErrDenied
		}
		var receipt struct {
			BookingStatus     string `json:"booking_status"`
			ReservationStatus string `json:"reservation_status"`
		}
		if json.Unmarshal(raw, &receipt) != nil {
			return delegationResult{}, af.ErrDenied
		}
		var message string
		switch receipt.BookingStatus {
		case "confirmed":
			message = "Запись подтверждена."
		case "rejected":
			message = "Сотрудник отклонил заявку."
		case "cancelled":
			message = "Запись отменена."
			if receipt.ReservationStatus == "expired" {
				message = "Срок резерва истёк, запись отменена."
			}
		case "completed":
			message = "Обслуживание завершено."
		default:
			return delegationResult{}, af.ErrDenied
		}
		return delegationResult{State: "backend_result", Receipt: raw, Message: bookingMessage(message, raw)}, nil
	}
	var evidence delegationContext
	var raw []byte
	if err := d.service.Pool.QueryRow(ctx, "SELECT platform.agent_delegation_context($1,$2)", d.job.ID, d.lease).Scan(&raw); err != nil {
		return delegationResult{}, err
	}
	if scope != d.scope || json.Unmarshal(raw, &evidence) != nil {
		return delegationResult{}, af.ErrDenied
	}
	if delegationParent(d.job.Action) {
		if len(evidence.Children) == 0 {
			return delegationResult{}, af.ErrDenied
		}
		out := delegationResult{State: "partial_failure", Children: evidence.Children}
		last := evidence.Children[len(evidence.Children)-1]
		if last.Status == "completed" && last.Result != nil {
			for _, c := range evidence.Children {
				if c.Status != "completed" {
					return out, nil
				}
			}
			out.State, out.Required, out.Proposal = last.Result.State, last.Result.Required, last.Result.Proposal
			out.PartRequirement = last.Result.PartRequirement
			out.PropertyRequirements, out.Properties = last.Result.PropertyRequirements, last.Result.Properties
			out.Message = last.Result.Message
			out.Route, out.Catalog, out.Order = last.Result.Route, last.Result.Catalog, last.Result.Order
			out.Calculation, out.Material, out.Offering = last.Result.Calculation, last.Result.Material, last.Result.Offering
			if evidence.ConversationRouted {
				out.ConversationReply = conversationalWorkflowReply(d.job.Action, out)
			}
		}
		if d.job.Action == realEstateDelegation && len(out.Properties) != 0 {
			// The child executions keep their original snapshots. The parent carries
			// each verified listing once so large cards fit the persisted result limit.
			for i := range out.Children {
				if child := out.Children[i].Result; child != nil && len(child.Properties) != 0 {
					summary := *child
					summary.Properties = nil
					out.Children[i].Result = &summary
				}
			}
		}
		return out, nil
	}
	if strings.HasPrefix(d.job.Action, "delegation.conversation_") {
		return d.conversationStep(ctx, evidence)
	}
	if strings.HasPrefix(d.job.Action, "delegation.property_") {
		return d.propertyStep(ctx, evidence)
	}
	if strings.HasPrefix(d.job.Action, "delegation.construction_") || strings.HasPrefix(d.job.Action, "delegation.service_") {
		return d.industry(ctx, evidence)
	}
	if d.job.Action == "delegation.requirements" {
		var requirements AutoPartsRequirements
		if json.Unmarshal(evidence.Input, &requirements) != nil || !requirements.valid() {
			return delegationResult{}, af.ErrDenied
		}
		r := &requirements
		out := delegationResult{State: "requirements_collected", Requirements: r}
		for _, field := range []struct{ name, value string }{{"oem", r.OEM}, {"make", r.Vehicle.Make}, {"model", r.Vehicle.Model}, {"engine", r.Vehicle.Engine}} {
			if strings.TrimSpace(field.value) == "" {
				out.Required = append(out.Required, field.name)
			}
		}
		if r.Vehicle.Year == 0 {
			out.Required = append(out.Required, "year")
		}
		if r.Quantity == 0 {
			out.Required = append(out.Required, "quantity")
		}
		if len(out.Required) > 0 {
			out.State = "clarification_required"
		}
		return out, nil
	}
	if len(evidence.Children) != 1 || evidence.Children[0].Status != "completed" || evidence.Children[0].Result == nil {
		return delegationResult{}, af.ErrDenied
	}
	out := *evidence.Children[0].Result
	if out.State == "clarification_required" {
		return out, nil
	}
	if out.Requirements == nil || !out.Requirements.valid() {
		return delegationResult{}, af.ErrDenied
	}
	switch d.job.Action {
	case "delegation.search_plan":
		out.State = "search_planned"
	case "delegation.find_parts", "delegation.verify_fitment":
		if err := d.service.Pool.QueryRow(ctx, "SELECT platform.agent_delegation_parts($1,$2)", d.job.ID, d.lease).Scan(&raw); err != nil {
			return delegationResult{}, err
		}
		var candidates []delegatedPart
		if json.Unmarshal(raw, &candidates) != nil || candidates == nil {
			return delegationResult{}, af.ErrDenied
		}
		if d.job.Action == "delegation.verify_fitment" {
			previous := map[string]bool{}
			for _, p := range out.Parts {
				previous[p.VariantID] = true
			}
			out.Parts = nil
			for _, p := range candidates {
				if previous[p.VariantID] {
					out.Parts = append(out.Parts, p)
				}
			}
			out.State = "fitment_checked"
		} else {
			out.Parts = candidates
			out.State = "parts_found"
		}
	case "delegation.proposal":
		out.State = "no_verified_offer"
		for _, part := range out.Parts {
			if part.Compatibility != "compatible" || part.Available == nil || *part.Available < out.Requirements.Quantity {
				continue
			}
			out.Proposal = delegationProposal(part, out.Requirements.Quantity)
			out.State = "proposal_ready"
			break
		}
	default:
		return delegationResult{}, af.ErrDenied
	}
	return out, nil
}

func (a *attempt) runDelegation(ctx context.Context) (af.Result, error) {
	d := delegationTool{a}
	tool, err := af.NewJSONTool(af.ToolSpec{Name: a.tool, Permission: a.tool, Version: "v1", Kind: af.ActionRead, CostKnown: true},
		func(scope af.Scope, _ struct{}) error {
			if scope != a.scope {
				return af.ErrDenied
			}
			return nil
		},
		func(scope af.Scope, out delegationResult) error {
			if scope != a.scope || out.State == "" {
				return af.ErrDenied
			}
			return nil
		}, d.execute)
	if err != nil {
		return af.Result{}, err
	}
	registry, err := af.NewRegistry([]af.Registration{{TenantID: a.scope.TenantID,
		Agent: af.Agent{ID: a.job.AgentID, Name: a.job.AgentID, Status: af.AgentEnabled, Version: "delegation-v1", Capabilities: []string{a.tool},
			Tools: []string{a.tool}, Permissions: []string{a.tool}, ContextKeys: d.contextKeys(), Planner: d, PlanningCostKnown: true}, Tools: []af.Tool{tool}}})
	if err != nil {
		return af.Result{}, err
	}
	var contexts af.ContextProvider = workflowContext{a.scope}
	if workshopDelegation(a.job.Action) {
		contexts = workshopContext{a}
	}
	router := af.Orchestrator{Registry: registry, Configurations: d, Permissions: a, Contexts: contexts, Tasks: a, Audit: a,
		Limits: af.Limits{MaxSteps: 2, MaxContextBytes: 8192, MaxResultBytes: 32768, Timeout: workflowTimeout}}
	return router.Run(ctx, af.Task{ID: a.job.ID, AgentID: a.job.AgentID, CorrelationID: a.job.CorrelationID, Scope: a.scope})
}

func delegationProposal(p delegatedPart, quantity int) *WorkflowDraft {
	return &WorkflowDraft{Kind: "product_quote", EstimatedTotal: estimateWorkflowPrice(&WorkflowPrice{Amount: p.Price, Currency: p.Currency}, int64(quantity)),
		Items: []orders.CartItem{{VariantID: p.VariantID, Quantity: quantity}}}
}
