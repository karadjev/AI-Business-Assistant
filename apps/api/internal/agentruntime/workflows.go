package agentruntime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	af "github.com/karadjev/AI-Business-Assistant/apps/api/internal/agentfoundation"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	bf "github.com/karadjev/AI-Business-Assistant/apps/api/internal/businessfoundation"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/orders"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/servicebooking"
)

type WorkflowScenario string

const (
	WorkflowAutoParts       WorkflowScenario = "B"
	WorkflowConstruction    WorkflowScenario = "C"
	WorkflowServiceBooking  WorkflowScenario = "D"
	WorkflowPropertyViewing WorkflowScenario = "E"

	workflowLimit   = 10
	workflowTimeout = 5 * time.Second
)

// WorkflowInput contains only scenario data. Backend-authenticated identity and
// durable task metadata are passed separately.
type WorkflowInput struct {
	Scenario        WorkflowScenario           `json:"scenario"`
	AutoParts       *WorkflowAutoPartsInput    `json:"auto_parts,omitempty"`
	Construction    *WorkflowConstructionInput `json:"construction,omitempty"`
	ServiceBooking  *WorkflowServiceInput      `json:"service_booking,omitempty"`
	PropertyViewing *WorkflowPropertyInput     `json:"property_viewing,omitempty"`
}

type WorkflowAutoPartsInput struct {
	Search    catalog.IndustrySearchFilters `json:"search"`
	Vehicle   catalog.FitmentVehicle        `json:"vehicle"`
	VariantID string                        `json:"variant_id,omitempty"`
	Quantity  int                           `json:"quantity,omitempty"`
}

type WorkflowConstructionInput struct {
	Search      catalog.IndustrySearchFilters `json:"search"`
	Calculation bf.ConstructionInput          `json:"calculation"`
	VariantID   string                        `json:"variant_id,omitempty"`
}

type WorkflowServiceInput struct {
	Query     string                    `json:"query"`
	Parts     *WorkflowAutoPartsInput   `json:"parts,omitempty"`
	Selection *servicebooking.Selection `json:"selection,omitempty"`
}

type WorkflowPropertyInput struct {
	Search    orders.PropertyFilter     `json:"search"`
	ListingID string                    `json:"listing_id,omitempty"`
	Selection *servicebooking.Selection `json:"selection,omitempty"`
}

type WorkflowProperty struct {
	ID                string      `json:"id"`
	Title             string      `json:"title"`
	Locality          string      `json:"locality"`
	Rooms             int         `json:"rooms"`
	AreaM2            json.Number `json:"area_m2"`
	Status            string      `json:"status"`
	VerifiedUntil     time.Time   `json:"verified_until"`
	PriceAmount       *int64      `json:"price_amount,omitempty"`
	CurrencyCode      string      `json:"currency_code,omitempty"`
	PropertyType      string      `json:"property_type,omitempty"`
	ViewingOfferingID string      `json:"viewing_offering_id,omitempty"`
}

type WorkflowFitment struct {
	VariantID string                `json:"variant_id"`
	Status    catalog.FitmentStatus `json:"status"`
}

type WorkflowCatalog struct {
	Items              []WorkflowCatalogItem `json:"items"`
	NextAfterVariantID string                `json:"next_after_variant_id,omitempty"`
}

type WorkflowPrice struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

type WorkflowCatalogItem struct {
	ProductID string         `json:"product_id"`
	VariantID string         `json:"variant_id"`
	Name      string         `json:"name"`
	SKU       string         `json:"sku"`
	Freshness string         `json:"freshness"`
	Available *int64         `json:"available,omitempty"`
	Price     *WorkflowPrice `json:"price,omitempty"`
}

type WorkflowOffering struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Revision   int64  `json:"revision"`
	MinMinutes int    `json:"min_minutes"`
	MaxMinutes int    `json:"max_minutes"`
}

// WorkflowDraft is a proposal. Items describe the estimated quantities. A
// product_quote can feed the staff quote flow. For the other kinds, Kind and
// Details can feed staff request intake under a separate authenticated action
// with a server-bound conversation and idempotency key.
type WorkflowDraft struct {
	Kind           string               `json:"kind"`
	EstimatedTotal *WorkflowPrice       `json:"estimated_total,omitempty"`
	Items          []orders.CartItem    `json:"items,omitempty"`
	Details        orders.IntakeDetails `json:"details"`
}

type WorkflowResult struct {
	Scenario     WorkflowScenario             `json:"scenario"`
	State        string                       `json:"state"`
	Required     []string                     `json:"required,omitempty"`
	Catalog      *WorkflowCatalog             `json:"catalog,omitempty"`
	Fitment      *WorkflowFitment             `json:"fitment,omitempty"`
	Calculation  *bf.ConstructionResult       `json:"calculation,omitempty"`
	Offerings    []WorkflowOffering           `json:"offerings,omitempty"`
	Availability *servicebooking.Availability `json:"availability,omitempty"`
	Properties   []WorkflowProperty           `json:"properties,omitempty"`
	Draft        *WorkflowDraft               `json:"draft,omitempty"`
}

// WorkflowReader is a trusted, read-only backend adapter. CheckWorkflowScope
// must validate the exact task scope and test-organization authority. Reads
// must independently enforce tenant and resource authorization at source.
// A durable caller must bind its reader to the actual task lease.
type WorkflowReader interface {
	CheckWorkflowScope(context.Context, af.Scope, WorkflowScenario) error
	SearchIndustry(context.Context, af.Scope, catalog.IndustrySearchFilters) (catalog.IndustrySearchResult, error)
	CheckFitment(context.Context, af.Scope, catalog.FitmentQuery) (catalog.FitmentResult, error)
	SearchOfferings(context.Context, af.Scope, string, int) ([]servicebooking.Offering, error)
	CheckAvailability(context.Context, af.Scope, servicebooking.Selection) (servicebooking.Availability, error)
	SearchProperties(context.Context, af.Scope, orders.PropertyFilter) ([]WorkflowProperty, error)
}

type WorkflowStores struct {
	Tasks af.IdempotencyStore
	Audit af.AuditSink
}

type WorkflowDependencies struct {
	Reader WorkflowReader
	Stores WorkflowStores
}

// ValidateWorkflowInput checks the input before task admission. Domain read
// services still validate their own filters and check authorization at execution.
func ValidateWorkflowInput(in WorkflowInput) error {
	if (in.AutoParts != nil) != (in.Scenario == WorkflowAutoParts) ||
		(in.Construction != nil) != (in.Scenario == WorkflowConstruction) ||
		(in.ServiceBooking != nil) != (in.Scenario == WorkflowServiceBooking) ||
		(in.PropertyViewing != nil) != (in.Scenario == WorkflowPropertyViewing) {
		return af.ErrInvalid
	}
	switch in.Scenario {
	case WorkflowAutoParts:
		if !validWorkflowAutoParts(*in.AutoParts) {
			return af.ErrInvalid
		}
	case WorkflowConstruction:
		p := in.Construction
		if !validWorkflowCatalogFilter(p.Search, "building_materials") || !optionalWorkflowID(p.VariantID) {
			return af.ErrInvalid
		}
		if p.Calculation.UnitsPerPackage > 0 {
			if len(p.Search.Text) == 4 || len(p.Search.Ranges) == 4 {
				return af.ErrLimit
			}
			for _, filter := range p.Search.Text {
				if filter.Field == "selling_unit" {
					return af.ErrInvalid
				}
			}
			for _, filter := range p.Search.Ranges {
				if filter.Field == "package_quantity" {
					return af.ErrInvalid
				}
			}
		}
		if _, err := bf.CalculateConstruction(p.Calculation); err != nil {
			return af.ErrInvalid
		}
	case WorkflowServiceBooking:
		p := in.ServiceBooking
		if !workflowText(p.Query, 200) {
			return af.ErrInvalid
		}
		if !validWorkflowSelection(p.Selection) || (p.Parts != nil && !validWorkflowAutoParts(*p.Parts)) {
			return af.ErrInvalid
		}
	case WorkflowPropertyViewing:
		p := in.PropertyViewing
		if !optionalWorkflowID(p.ListingID) || !p.Search.Valid() || !validWorkflowSelection(p.Selection) ||
			(p.Selection != nil && p.ListingID == "") {
			return af.ErrInvalid
		}
	default:
		return af.ErrInvalid
	}
	return nil
}

func validWorkflowSelection(s *servicebooking.Selection) bool {
	return s == nil || (organizations.ValidID(s.OfferingID) && s.OfferingRevision >= 1 && s.Units >= 1 && s.Units <= 100 &&
		servicebooking.ValidateZonedInterval(s.StartsAt, s.EndsAt, s.Timezone, 7*24*time.Hour) == nil)
}

func validWorkflowAutoParts(p WorkflowAutoPartsInput) bool {
	if !validWorkflowCatalogFilter(p.Search, "auto_parts") || !optionalWorkflowID(p.VariantID) ||
		(p.VariantID == "") != (p.Quantity == 0) || p.Quantity < 0 || p.Quantity > 10 ||
		(p.Vehicle.Year != 0 && p.Vehicle.Year < 1886) || p.Vehicle.Year > 2100 {
		return false
	}
	for _, value := range []string{p.Vehicle.Make, p.Vehicle.Model, p.Vehicle.Engine} {
		if !workflowText(value, 80) {
			return false
		}
	}
	return true
}

func optionalWorkflowID(id string) bool {
	return id == "" || organizations.ValidID(id)
}

func workflowText(value string, max int) bool {
	return len(value) <= max && utf8.ValidString(value) && !strings.ContainsFunc(value, unicode.IsControl)
}

func validWorkflowCatalogFilter(f catalog.IndustrySearchFilters, industry string) bool {
	if (f.Industry != "" && f.Industry != industry) || f.Vehicle != nil ||
		f.Limit < 0 || f.Limit > workflowLimit || len(f.Text) > 4 || len(f.Ranges) > 4 ||
		!optionalWorkflowID(f.CategoryID) || !optionalWorkflowID(f.AfterVariantID) {
		return false
	}
	switch f.Availability {
	case "", catalog.IndustryInStock, catalog.IndustryOutOfStock, catalog.IndustryUnknown:
	default:
		return false
	}
	for _, filter := range f.Text {
		if !workflowText(filter.Field, 32) || !workflowText(filter.Value, 100) ||
			(filter.Match != catalog.IndustryExact && filter.Match != catalog.IndustryPartial) {
			return false
		}
	}
	for _, filter := range f.Ranges {
		if !workflowText(filter.Field, 32) || !workflowText(filter.Min, 28) || !workflowText(filter.Max, 28) {
			return false
		}
	}
	return true
}

// RunWorkflow runs one compiled customer workflow through Registry,
// Orchestrator and a typed ActionRead tool. Each scenario has at most two
// domain reads. Partial output is discarded if orchestration or auditing fails.
func RunWorkflow(ctx context.Context, task af.Task, in WorkflowInput, deps WorkflowDependencies) (WorkflowResult, error) {
	if deps.Reader == nil || deps.Stores.Tasks == nil || deps.Stores.Audit == nil ||
		!organizations.ValidID(task.Scope.TenantID) || !organizations.ValidID(task.Scope.PrincipalID) ||
		!organizations.ValidID(task.Scope.ConversationID) || !organizations.ValidID(task.ID) ||
		!optionalWorkflowID(task.CorrelationID) || (task.AgentID != "" && task.AgentID != "customer") {
		return WorkflowResult{}, af.ErrInvalid
	}
	if err := ValidateWorkflowInput(in); err != nil {
		return WorkflowResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, workflowTimeout)
	defer cancel()
	if err := deps.Reader.CheckWorkflowScope(ctx, task.Scope, in.Scenario); err != nil {
		return WorkflowResult{}, err
	}

	name := "customer.workflow." + string(in.Scenario) + ".read"
	params, err := json.Marshal(in)
	if err != nil {
		return WorkflowResult{}, af.ErrInvalid
	}
	bound := workflowBinding{task.Scope, in.Scenario, name, params, deps.Reader}
	tool, err := af.NewJSONTool(af.ToolSpec{Name: name, Permission: name, Version: "v1", Kind: af.ActionRead, CostKnown: true},
		bound.validateInput, bound.validateOutput, bound.execute)
	if err != nil {
		return WorkflowResult{}, err
	}
	registry, err := af.NewRegistry([]af.Registration{{
		TenantID: task.Scope.TenantID,
		Agent: af.Agent{ID: "customer", Version: "workflow-v1", Status: af.AgentEnabled, Name: "Customer",
			Capabilities: []string{name}, Tools: []string{name}, Permissions: []string{name},
			Planner: bound, PlanningCostKnown: true},
		Tools: []af.Tool{tool},
	}})
	if err != nil {
		return WorkflowResult{}, err
	}
	router := af.Orchestrator{Registry: registry, Configurations: bound, Permissions: bound,
		Contexts: workflowContext{bound.scope}, Tasks: deps.Stores.Tasks, Audit: deps.Stores.Audit,
		Limits: af.Limits{MaxSteps: 2, MaxContextBytes: 256, MaxResultBytes: 32_768, Timeout: workflowTimeout}}
	task.AgentID = "customer"
	result, err := router.Run(ctx, task)
	if err != nil {
		return WorkflowResult{}, err
	}
	var out WorkflowResult
	if len(result.Results) != 1 || json.Unmarshal(result.Results[0], &out) != nil {
		return WorkflowResult{}, af.ErrInvalid
	}
	return out, nil
}

type workflowBinding struct {
	scope    af.Scope
	scenario WorkflowScenario
	name     string
	params   json.RawMessage
	reader   WorkflowReader
}

func (b workflowBinding) Load(ctx context.Context, s af.Scope, agentID string) (af.Configuration, error) {
	if s != b.scope || agentID != "customer" {
		return af.Configuration{}, af.ErrDenied
	}
	return af.Configuration{TenantID: s.TenantID, AgentID: agentID, Version: "workflow-v1", Enabled: true, Tools: []string{b.name}}, ctx.Err()
}

func (b workflowBinding) Check(ctx context.Context, s af.Scope, permission string) error {
	if s != b.scope || permission != b.name {
		return af.ErrDenied
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return b.reader.CheckWorkflowScope(ctx, s, b.scenario)
}

func (b workflowBinding) Next(ctx context.Context, s af.Scope, snapshot af.Snapshot) (af.Decision, error) {
	if s != b.scope {
		return af.Decision{}, af.ErrDenied
	}
	switch len(snapshot.Results) {
	case 0:
		return af.Decision{Tool: b.name, Input: b.params}, ctx.Err()
	case 1:
		return af.Decision{Done: true}, ctx.Err()
	default:
		return af.Decision{}, af.ErrLimit
	}
}

func (b workflowBinding) validateInput(s af.Scope, in WorkflowInput) error {
	if s != b.scope || in.Scenario != b.scenario {
		return af.ErrDenied
	}
	return ValidateWorkflowInput(in)
}

func (b workflowBinding) validateOutput(s af.Scope, out WorkflowResult) error {
	if s != b.scope || out.Scenario != b.scenario {
		return af.ErrDenied
	}
	return nil
}

type workflowContext struct{ scope af.Scope }

func (c workflowContext) Load(ctx context.Context, s af.Scope, keys []string, _ int) (af.ContextBundle, error) {
	if s != c.scope || len(keys) != 0 {
		return af.ContextBundle{}, af.ErrDenied
	}
	return af.ContextBundle{Scope: s}, ctx.Err()
}

func workflowDomainError(err error, status int, code string) bool {
	var failure *auth.Error
	return errors.As(err, &failure) && failure.Status == status && failure.Code == code
}
