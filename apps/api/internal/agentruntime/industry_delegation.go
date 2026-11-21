package agentruntime

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	af "github.com/karadjev/AI-Business-Assistant/apps/api/internal/agentfoundation"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	bf "github.com/karadjev/AI-Business-Assistant/apps/api/internal/businessfoundation"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/orders"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/servicebooking"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
)

const constructionDelegation = "delegate_construction"
const serviceDelegation = "delegate_service"

func delegationParent(action string) bool {
	return action == autoPartsDelegation || action == constructionDelegation || action == serviceDelegation
}

type ConstructionRequirements struct {
	Room        string                  `json:"room,omitempty"`
	Work        string                  `json:"work,omitempty"`
	Area        *bf.ConstructionMeasure `json:"area,omitempty"`
	Notes       string                  `json:"notes,omitempty"`
	SKU         string                  `json:"sku,omitempty"`
	Calculation *bf.ConstructionInput   `json:"calculation,omitempty"`
}

type ServiceRequirements struct {
	Vehicle   catalog.FitmentVehicle    `json:"vehicle"`
	Selection *servicebooking.Selection `json:"selection,omitempty"`
	Notes     string                    `json:"notes,omitempty"`
	Part      *AutoPartsRequirements    `json:"part,omitempty"`
	NoPart    bool                      `json:"no_part,omitempty"`
}

type IndustryDelegationRequest[T any] struct {
	ConversationID string `json:"conversation_id"`
	IdempotencyKey string `json:"idempotency_key"`
	Requirements   T      `json:"requirements"`
}

func (s Service) SubmitConstruction(ctx context.Context, t catalog.Tenant, in IndustryDelegationRequest[ConstructionRequirements]) (string, error) {
	r := in.Requirements
	r.SKU = strings.ToUpper(strings.TrimSpace(r.SKU))
	if r.SKU != "" && !partReference.MatchString(r.SKU) || !workflowText(r.Room, 80) || !workflowText(r.Work, 80) || !workflowText(r.Notes, 250) {
		return "", auth.Failure(422, "INVALID_DELEGATION", "Проверьте параметры расчёта")
	}
	return s.submitDelegation(ctx, t, in.ConversationID, in.IdempotencyKey, constructionDelegation, r)
}
func (s Service) SubmitService(ctx context.Context, t catalog.Tenant, in IndustryDelegationRequest[ServiceRequirements]) (string, error) {
	r := in.Requirements
	r.Notes = strings.TrimSpace(r.Notes)
	r.Vehicle.Make, r.Vehicle.Model, r.Vehicle.Engine = strings.TrimSpace(r.Vehicle.Make), strings.TrimSpace(r.Vehicle.Model), strings.TrimSpace(r.Vehicle.Engine)
	if r.Part != nil {
		part := *r.Part
		part.Vehicle = r.Vehicle
		part.OEM = strings.ToUpper(strings.TrimSpace(part.OEM))
		r.Part = &part
	}
	if !(AutoPartsRequirements{Vehicle: r.Vehicle}).valid() || !workflowText(r.Notes, 250) || r.Selection != nil && !validWorkflowSelection(r.Selection) ||
		r.Part != nil && (r.NoPart || !r.Part.valid()) {
		return "", auth.Failure(422, "INVALID_DELEGATION", "Проверьте параметры записи")
	}
	return s.submitDelegation(ctx, t, in.ConversationID, in.IdempotencyKey, serviceDelegation, r)
}

type delegationMaterial struct {
	delegatedPart
	Manufacturer    string `json:"manufacturer,omitempty"`
	Unit            string `json:"unit,omitempty"`
	PackageQuantity string `json:"package_quantity,omitempty"`
}

type delegationOffering struct {
	WorkflowOffering
	Prices    *WorkflowPrice `json:"price,omitempty"`
	Timezone  string         `json:"timezone"`
	Resources []string       `json:"resources"`
}

func (d delegationTool) industry(ctx context.Context, e delegationContext) (delegationResult, error) {
	switch d.job.Action {
	case "delegation.construction_input":
		var r ConstructionRequirements
		if json.Unmarshal(e.Input, &r) != nil {
			return delegationResult{}, af.ErrDenied
		}
		out := delegationResult{State: "requirements_collected", Construction: &r}
		if r.SKU == "" {
			out.Required = append(out.Required, "sku")
		}
		if r.Calculation == nil || r.Calculation.Length.Value == "" || r.Calculation.Width.Value == "" {
			out.Required = append(out.Required, "dimensions")
		}
		if len(out.Required) > 0 {
			out.State = "clarification_required"
		}
		return out, nil
	case "delegation.service_input":
		var r ServiceRequirements
		if json.Unmarshal(e.Input, &r) != nil {
			return delegationResult{}, af.ErrDenied
		}
		out := delegationResult{State: "requirements_collected", Service: &r}
		for _, field := range []struct{ name, value string }{{"make", r.Vehicle.Make}, {"model", r.Vehicle.Model}} {
			if field.value == "" {
				out.Required = append(out.Required, field.name)
			}
		}
		if r.Vehicle.Year == 0 {
			out.Required = append(out.Required, "year")
		}
		if r.Selection == nil {
			out.Required = append(out.Required, "selection")
		}

		if len(out.Required) > 0 {
			out.State = "clarification_required"
		}
		return out, nil
	}
	if len(e.Children) != 1 || e.Children[0].Status != "completed" || e.Children[0].Result == nil {
		return delegationResult{}, af.ErrDenied
	}
	out := *e.Children[0].Result
	switch out.State {
	case "clarification_required", "material_unavailable", "calculation_invalid", "packaging_unverified", "source_changed", "slot_unavailable", "service_unavailable", "part_unavailable", "part_requirement_unknown", "part_decision_required":
		return out, nil
	}
	if strings.HasPrefix(d.job.Action, "delegation.construction_") {
		return d.constructionStep(ctx, out)
	}
	return d.serviceStep(ctx, out)
}

func (d delegationTool) material(ctx context.Context) (*delegationMaterial, error) {
	var raw []byte
	if err := d.service.Pool.QueryRow(ctx, "SELECT platform.agent_delegation_material($1,$2)", d.job.ID, d.lease).Scan(&raw); err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, nil
	}
	var material delegationMaterial
	if err := json.Unmarshal(raw, &material); err != nil {
		return nil, err
	}
	return &material, nil
}

func (d delegationTool) constructionStep(ctx context.Context, out delegationResult) (delegationResult, error) {
	if out.Construction == nil || out.Construction.Calculation == nil {
		return delegationResult{}, af.ErrDenied
	}
	switch d.job.Action {
	case "delegation.construction_catalog":
		material, err := d.material(ctx)
		if err != nil {
			return delegationResult{}, err
		}
		out.Material = material
		out.State = "material_found"
		if material == nil {
			out.State = "material_unavailable"
		}
	case "delegation.construction_calculate":
		if out.Material == nil {
			return delegationResult{}, af.ErrDenied
		}
		calculation := *out.Construction.Calculation
		// Packaging sold by the catalog controls the conversion to purchasable units.
		pack, err := strconv.ParseInt(out.Material.PackageQuantity, 10, 64)
		if err != nil || pack < 1 || out.Material.Unit != "package" {
			out.State = "packaging_unverified"
			return out, nil
		}
		if calculation.Operation == bf.ConstructionTiles || calculation.Operation == bf.ConstructionBlocks {
			if calculation.UnitsPerPackage != 0 && calculation.UnitsPerPackage != pack {
				out.State = "packaging_unverified"
				return out, nil
			}
			calculation.UnitsPerPackage = pack
		}
		out.Construction.Calculation = &calculation
		result, err := bf.CalculateConstruction(calculation)
		if err != nil {
			out.State = "calculation_invalid"
			return out, nil
		}
		out.Calculation = &result
		out.State = "calculated"
	case "delegation.construction_offer":
		if out.Material == nil || out.Calculation == nil {
			return delegationResult{}, af.ErrDenied
		}
		current, err := d.material(ctx)
		if err != nil {
			return delegationResult{}, err
		}
		if current == nil || current.VariantID != out.Material.VariantID || current.PackageQuantity != out.Material.PackageQuantity || current.Unit != out.Material.Unit {
			out.State = "source_changed"
			return out, nil
		}
		out.Material = current
		count := out.Calculation.PackageCount
		if count <= 0 {
			out.State = "packaging_unverified"
			return out, nil
		}
		if current.Available == nil || int64(*current.Available) < count {
			out.State = "stock_unverified_or_insufficient"
			return out, nil
		}
		out.Proposal = delegationProposal(current.delegatedPart, int(count))
		out.Proposal.Kind = "quotation_request"
		out.State = "proposal_ready"
	default:
		return delegationResult{}, af.ErrDenied
	}
	return out, nil
}

func (d delegationTool) serviceStep(ctx context.Context, out delegationResult) (delegationResult, error) {
	if out.Service == nil || out.Service.Selection == nil {
		return delegationResult{}, af.ErrDenied
	}
	selection := out.Service.Selection
	switch d.job.Action {
	case "delegation.service_select", "delegation.service_availability":
		check := d.job.Action == "delegation.service_availability"
		service := servicebooking.Service{Pool: d.service.Pool}
		offering, err := service.DelegatedAvailability(ctx, catalog.Tenant{OrganizationID: d.scope.TenantID, AgentTaskID: d.job.ID, AgentLease: d.lease}, *selection, check)
		if err != nil {
			switch {
			case workflowDomainError(err, 409, "SERVICE_UNAVAILABLE"):
				out.State = "slot_unavailable"
			case workflowDomainError(err, 409, "SOURCE_CHANGED"):
				out.State = "source_changed"
			case workflowDomainError(err, 404, "NOT_FOUND"):
				out.State = "service_unavailable"
			default:
				return delegationResult{}, err
			}
			return out, nil
		}
		out.Offering = &delegationOffering{WorkflowOffering: WorkflowOffering{ID: offering.ID, Name: offering.Name, Revision: offering.Revision, MinMinutes: offering.MinMinutes, MaxMinutes: offering.MaxMinutes}, Timezone: offering.Timezone, Resources: offering.ResourceIDs}
		if offering.AmountMinor != nil && offering.PriceState == "known" {
			out.Offering.Prices = &WorkflowPrice{Amount: *offering.AmountMinor, Currency: offering.Currency}
		}
		out.State = "service_selected"
		if check {
			out.State = "slot_available"
			resolveServicePart(&out, offering.PartRule)
		}
	case "delegation.service_parts":
		out.State = "part_not_required"
		if out.Service.NoPart {
			return out, nil
		}
		var raw []byte
		if err := d.service.Pool.QueryRow(ctx, "SELECT platform.agent_delegation_parts($1,$2)", d.job.ID, d.lease).Scan(&raw); err != nil {
			return delegationResult{}, err
		}
		var parts []delegatedPart
		if len(raw) == 0 {
			out.State = "source_changed"
			return out, nil
		}
		if json.Unmarshal(raw, &parts) != nil || parts == nil {
			return delegationResult{}, af.ErrDenied
		}
		out.Parts = parts
		out.State = "part_unavailable"
		for _, part := range parts {
			if part.Compatibility == "compatible" && part.Available != nil && *part.Available >= out.Service.Part.Quantity {
				out.Parts = []delegatedPart{part}
				out.State = "part_verified"
				break
			}
		}
	case "delegation.service_proposal":
		details := orders.IntakeDetails{Selection: selection, Vehicle: &out.Service.Vehicle, Summary: out.Service.Notes}
		kind := "service_booking"
		if !out.Service.NoPart {
			if out.Service.Part == nil || len(out.Parts) != 1 || out.State != "part_verified" {
				return delegationResult{}, af.ErrDenied
			}
			kind = "hybrid_order"
			details.Items = []orders.CartItem{{VariantID: out.Parts[0].VariantID, Quantity: out.Service.Part.Quantity}}
		}
		out.Proposal = &WorkflowDraft{Kind: kind, Details: details, Items: details.Items}
		out.State = "approval_required"
	default:
		return delegationResult{}, af.ErrDenied
	}
	return out, nil
}

// PrepareDelegatedBooking accepts only the saved proposal and uses the parent as the intake idempotency key.
func (s Service) PrepareDelegatedBooking(ctx context.Context, t catalog.Tenant, id string) (json.RawMessage, error) {
	if !organizations.ValidID(id) {
		return nil, auth.Failure(404, "NOT_FOUND", "Задача не найдена")
	}
	tx, err := s.ownerTx(ctx, t)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var conversation string
	var payload []byte
	err = tx.QueryRow(ctx, `SELECT j.conversation_id,e.result FROM platform.agent_tasks j
 JOIN platform.agent_executions e ON e.organization_id=j.organization_id AND e.task_id=j.id AND e.status='completed'
 WHERE j.organization_id=$1 AND j.id=$2 AND j.action='delegate_service' AND j.status='completed' AND j.deadline>clock_timestamp()`, t.OrganizationID, id).Scan(&conversation, &payload)
	if err != nil {
		return nil, tenancy.Error(err)
	}
	var result []delegationResult
	if json.Unmarshal(payload, &result) != nil || len(result) != 1 || result[0].State != "approval_required" || result[0].Proposal == nil {
		return nil, auth.Failure(409, "DELEGATION_UNAVAILABLE", "Подтверждаемое предложение отсутствует")
	}
	proposal := result[0].Proposal
	if err = tx.Commit(ctx); err != nil {
		return nil, tenancy.Error(err)
	}
	return (orders.Service{Pool: s.Pool}).PrepareServiceDelegation(ctx, t, orders.Intake{ConversationID: conversation, IdempotencyKey: id, Kind: proposal.Kind, Details: proposal.Details})
}

func resolveServicePart(out *delegationResult, rule *servicebooking.PartRule) {
	r := out.Service
	if rule == nil {
		// Existing proposals using a human's explicit choice remain compatible.
		switch {
		case r.NoPart:
			out.PartRequirement = "NOT_REQUIRED"
		case r.Part != nil && r.Part.OEM != "" && r.Part.Quantity > 0:
			out.PartRequirement = "REQUIRED"
		default:
			out.PartRequirement = "UNKNOWN"
		}
	} else {
		out.PartRequirement = rule.State
		v := r.Vehicle
		for _, pair := range [][2]string{{rule.Make, v.Make}, {rule.Model, v.Model}, {rule.Engine, v.Engine}} {
			if pair[0] != "" && !strings.EqualFold(pair[0], pair[1]) {
				out.PartRequirement = "UNKNOWN"
			}
		}
		if rule.Year != 0 && rule.Year != v.Year {
			out.PartRequirement = "UNKNOWN"
		}
		switch out.PartRequirement {
		case "NOT_REQUIRED":
			r.NoPart, r.Part = true, nil
		case "REQUIRED", "OPTIONAL":
			if out.PartRequirement == "OPTIONAL" && !r.NoPart && r.Part == nil {
				out.State = "part_decision_required"
				out.Required = []string{"part_choice"}
				return
			}
			if out.PartRequirement == "REQUIRED" || !r.NoPart {
				r.NoPart = false
				r.Part = &AutoPartsRequirements{OEM: rule.OEM, Vehicle: v, Quantity: rule.Quantity}
			}
		}
	}
	if out.PartRequirement == "UNKNOWN" {
		out.State = "part_requirement_unknown"
		out.Required = []string{"service_diagnosis"}
		return
	}
	if !r.NoPart && r.Vehicle.Engine == "" {
		out.State = "clarification_required"
		out.Required = []string{"engine"}
	}
}
