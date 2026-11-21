package agentruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	af "github.com/karadjev/AI-Business-Assistant/apps/api/internal/agentfoundation"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/orders"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/servicebooking"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
)

const realEstateDelegation = "delegate_real_estate"
const propertyViewingDelegation = "delegate_property_viewing"

type PropertyRequirements struct {
	orders.PropertyCriteria
	Wishes string `json:"wishes,omitempty"`
}

type RealEstateRequest struct {
	IndustryDelegationRequest[PropertyRequirements]
	PreviousTaskID string `json:"previous_task_id,omitempty"`
}

type PropertyViewingRequest struct {
	ConversationID string                   `json:"conversation_id"`
	IdempotencyKey string                   `json:"idempotency_key"`
	OfferTaskID    string                   `json:"offer_task_id"`
	PropertyID     string                   `json:"property_id"`
	Selection      servicebooking.Selection `json:"selection"`
	Comment        string                   `json:"comment,omitempty"`
}

func invalidPropertyRequest() error {
	return auth.Failure(422, "INVALID_DELEGATION", "Проверьте параметры недвижимости или просмотра")
}

func (s Service) SubmitRealEstate(ctx context.Context, t catalog.Tenant, in RealEstateRequest) (string, error) {
	r := in.Requirements
	r.Normalize()
	r.Wishes = strings.TrimSpace(r.Wishes)
	if !optionalWorkflowID(in.PreviousTaskID) || !r.Valid() || !workflowText(r.Wishes, 350) {
		return "", invalidPropertyRequest()
	}
	if in.PreviousTaskID != "" {
		tx, err := s.ownerTx(ctx, t)
		if err != nil {
			return "", err
		}
		defer tx.Rollback(ctx)
		patch, err := json.Marshal(r)
		if err != nil {
			return "", err
		}
		// A prior completed result is immutable and belongs to this exact conversation.
		var merged PropertyRequirements
		err = tx.QueryRow(ctx, `SELECT (e.result->0->'property_requirements') || $4::jsonb
		 FROM platform.agent_tasks a JOIN platform.agent_executions e
		   ON e.organization_id=a.organization_id AND e.task_id=a.id AND e.status='completed'
		 WHERE a.organization_id=$1 AND a.conversation_id=$2 AND a.id=$3
		 AND a.action='delegate_real_estate' AND a.status='completed'
		 AND jsonb_typeof(e.result->0->'property_requirements')='object'`,
			t.OrganizationID, in.ConversationID, in.PreviousTaskID, patch).Scan(&merged)
		if err != nil {
			return "", tenancy.Error(err)
		}
		r = merged
		if !r.Valid() {
			return "", invalidPropertyRequest()
		}
		if err = tx.Rollback(ctx); err != nil {
			return "", tenancy.Error(err)
		}
	}
	return s.submitDelegation(ctx, t, in.ConversationID, in.IdempotencyKey, realEstateDelegation, r)
}

func (s Service) SubmitPropertyViewing(ctx context.Context, t catalog.Tenant, in PropertyViewingRequest) (string, error) {
	in.Comment = strings.TrimSpace(in.Comment)
	if !organizations.ValidID(in.PropertyID) || !organizations.ValidID(in.OfferTaskID) ||
		!validWorkflowSelection(&in.Selection) || in.Selection.Units != 1 || !workflowText(in.Comment, 500) {
		return "", invalidPropertyRequest()
	}
	return s.submitDelegation(ctx, t, in.ConversationID, in.IdempotencyKey, propertyViewingDelegation, struct {
		OfferTaskID string                   `json:"offer_task_id"`
		PropertyID  string                   `json:"property_id"`
		Selection   servicebooking.Selection `json:"selection"`
		Comment     string                   `json:"comment,omitempty"`
	}{in.OfferTaskID, in.PropertyID, in.Selection, in.Comment})
}

func collectPropertyRequirements(raw json.RawMessage) (delegationResult, error) {
	var r PropertyRequirements
	if json.Unmarshal(raw, &r) != nil || !r.Valid() || !workflowText(r.Wishes, 350) {
		return delegationResult{}, af.ErrDenied
	}
	out := delegationResult{State: "requirements_collected", PropertyRequirements: &r}
	var questions []string
	for _, field := range []struct{ key, value, question string }{
		{"transaction_type", string(r.Transaction), "Вы покупаете или арендуете? Если арендуете, на какой срок?"},
		{"property_type", string(r.Type), "Какой тип недвижимости вы ищете?"},
		{"city", r.City, "В каком городе искать?"},
		{"currency_code", r.Currency, "В какой валюте указан бюджет?"},
	} {
		if field.value == "" {
			out.Required = append(out.Required, field.key)
			questions = append(questions, field.question)
		}
	}
	if r.MaxPrice == nil {
		out.Required = append(out.Required, "max_price_amount")
		questions = append(questions, "На какой максимальный бюджет ориентироваться?")
	}
	if len(out.Required) > 0 {
		out.State = "clarification_required"
		out.Message = strings.Join(questions, " ")
	}
	return out, nil
}

func (d delegationTool) propertyStep(ctx context.Context, e delegationContext) (delegationResult, error) {
	switch d.job.Action {
	case "delegation.property_requirements":
		return collectPropertyRequirements(e.Input)
	case "delegation.property_selection":
		var selected bool
		err := d.service.Pool.QueryRow(ctx, `SELECT platform.agent_property_selection($1,$2)`, d.job.ID, d.lease).Scan(&selected)
		if err != nil {
			return delegationResult{}, err
		}
		if !selected {
			return delegationResult{State: "property_unavailable", Message: "Выбранный объект недоступен для просмотра."}, nil
		}
		var in PropertyViewingRequest
		if json.Unmarshal(e.Input, &in) != nil || !validWorkflowSelection(&in.Selection) {
			return delegationResult{}, af.ErrDenied
		}
		return delegationResult{State: "property_selected", Proposal: &WorkflowDraft{
			Kind: "property_viewing_request", Details: orders.IntakeDetails{ListingID: in.PropertyID, Selection: &in.Selection, Summary: in.Comment},
		}}, nil
	}

	if len(e.Children) != 1 || e.Children[0].Status != "completed" || e.Children[0].Result == nil {
		return delegationResult{}, af.ErrDenied
	}
	out := *e.Children[0].Result
	switch out.State {
	case "clarification_required", "no_properties", "property_unavailable":
		return out, nil
	}
	var properties []orders.Property
	if err := d.service.Pool.QueryRow(ctx, `SELECT platform.agent_delegation_properties($1,$2)`, d.job.ID, d.lease).Scan(&properties); err != nil {
		return delegationResult{}, err
	}
	if properties == nil {
		return delegationResult{}, af.ErrDenied
	}
	out.Properties = properties
	switch d.job.Action {
	case "delegation.property_search", "delegation.property_offer":
		out.State = "properties_found"
		if len(properties) == 0 {
			out.State = "no_properties"
			out.Message = "По заданным критериям доступных объектов нет. Можно уточнить район, площадь или бюджет."
		} else if d.job.Action == "delegation.property_offer" {
			out.State = "proposal_ready"
			out.Message = propertyComparison(properties)
		}
	case "delegation.property_viewing":
		if len(properties) != 1 || out.Proposal == nil || properties[0].ViewingOfferingID == "" ||
			properties[0].ViewingOfferingID != out.Proposal.Details.Selection.OfferingID {
			out.State = "property_unavailable"
			out.Proposal = nil
			out.Message = "Объект или выбранное время просмотра недоступны."
		} else {
			out.State = "approval_required"
			out.Message = "Заявку на просмотр должен подтвердить сотрудник."
		}
	default:
		return delegationResult{}, af.ErrDenied
	}
	return out, nil
}

func propertyComparison(properties []orders.Property) string {
	var lines []string
	for _, p := range properties {
		location := p.Locality
		if p.City != "" {
			location = p.City
			if p.District != "" {
				location += ", " + p.District
			}
		}
		price := fmt.Sprintf("%d мин. ед. %s", *p.Price, p.Currency)
		switch p.Currency {
		case "RUB", "EUR", "USD":
			price = fmt.Sprintf("%d.%02d %s", *p.Price/100, *p.Price%100, p.Currency)
		}
		lines = append(lines, fmt.Sprintf("%s · %s · %s · %s м² · комнат %d", p.Title, location, price, p.Area, p.Rooms))
	}
	const heading = "Подходящие объекты по возрастанию цены."
	message := heading
	for _, line := range lines {
		if len(message)+len(line)+1 > 925 {
			return message + "\nВсе варианты сохранены в предложении."
		}
		message += "\n" + line
	}
	return message
}

func (s Service) PreparePropertyViewing(ctx context.Context, t catalog.Tenant, id string) (json.RawMessage, error) {
	return s.prepareDelegatedIntake(ctx, t, id, propertyViewingDelegation)
}

func (d delegationTool) propertyReceipt(ctx context.Context) (delegationResult, error) {
	var raw []byte
	if err := d.service.Pool.QueryRow(ctx, `SELECT platform.property_receipt_context($1,$2)`, d.job.ID, d.lease).Scan(&raw); err != nil {
		return delegationResult{}, err
	}
	var receipt struct {
		Status      string `json:"booking_status"`
		EventStatus string `json:"event_status"`
		Available   bool   `json:"property_available"`
	}
	if json.Unmarshal(raw, &receipt) != nil {
		return delegationResult{}, af.ErrDenied
	}
	if receipt.Status != receipt.EventStatus {
		return delegationResult{State: "superseded", Receipt: raw}, nil
	}
	message := map[string]string{
		"pending_confirmation": "Заявка на просмотр принята. Ожидается решение сотрудника.",
		"confirmed":            "Просмотр подтверждён.",
		"rejected":             "Сотрудник отклонил заявку на просмотр.",
		"cancelled":            "Просмотр отменён.",
		"completed":            "Просмотр завершён.",
	}[receipt.Status]
	if message == "" {
		return delegationResult{}, af.ErrDenied
	}
	if !receipt.Available && (receipt.Status == "pending_confirmation" || receipt.Status == "confirmed") {
		message = "Объект стал недоступен. Уточните у сотрудника статус просмотра."
	}
	return delegationResult{State: "backend_result", Receipt: raw, Message: bookingMessage(message, raw)}, nil
}
