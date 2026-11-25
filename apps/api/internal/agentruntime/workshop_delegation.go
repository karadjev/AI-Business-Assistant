package agentruntime

import (
	"context"
	"encoding/json"
	"fmt"

	af "github.com/karadjev/AI-Business-Assistant/apps/api/internal/agentfoundation"
)

const (
	workshopManager    = "delegate_workshop"
	workshopContextKey = "workshop_request"
)

func workshopDelegation(action string) bool {
	switch action {
	case workshopMarketingManager, "delegation.workshop_marketing", workshopFinanceManager, "delegation.workshop_finance", "delegation.workshop_analytics", workshopManager, "delegation.workshop_customer", "delegation.workshop_stock", "delegation.workshop_sales":
		return true
	default:
		return false
	}
}

func (d delegationTool) contextKeys() []string {
	if workshopDelegation(d.job.Action) {
		return []string{workshopContextKey}
	}
	return nil
}

type workshopContext struct{ *attempt }

func (c workshopContext) Load(ctx context.Context, scope af.Scope, keys []string, limit int) (af.ContextBundle, error) {
	if scope != c.scope || len(keys) != 1 || keys[0] != workshopContextKey {
		return af.ContextBundle{}, af.ErrDenied
	}
	var raw json.RawMessage
	query := "SELECT platform.workshop_delegation_context($1,$2)"
	if marketingDelegation(c.job.Action) {
		query = "SELECT platform.workshop_marketing_context($1,$2)"
	} else if financialDelegation(c.job.Action) {
		query = "SELECT platform.workshop_financial_context($1,$2)"
	}
	if err := c.service.Pool.QueryRow(ctx, query, c.job.ID, c.lease).Scan(&raw); err != nil {
		return af.ContextBundle{}, err
	}
	if len(raw) == 0 || len(raw) > limit || string(raw) == "null" {
		return af.ContextBundle{}, af.ErrDenied
	}
	return af.ContextBundle{Scope: scope, Values: map[string]json.RawMessage{workshopContextKey: raw}}, nil
}

type workshopPart struct {
	VariantID string `json:"variant_id"`
	ProductID string `json:"product_id"`
	Name      string `json:"name"`
	SKU       string `json:"sku"`
	Version   int64  `json:"variant_version"`
	Available *int   `json:"available"`
}

func runWorkshopStep(action string, raw json.RawMessage) (delegationResult, error) {
	if marketingDelegation(action) {
		return runWorkshopMarketing(action, raw)
	}
	if financialDelegation(action) {
		return runWorkshopFinance(action, raw)
	}
	var in struct {
		CustomerVerified bool              `json:"customer_verified"`
		Part             *workshopPart     `json:"part"`
		Quantity         int               `json:"quantity"`
		Service          string            `json:"service"`
		Vehicle          string            `json:"vehicle"`
		Children         []delegationChild `json:"children"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return delegationResult{}, err
	}
	switch action {
	case "delegation.workshop_customer":
		if !in.CustomerVerified {
			return delegationResult{State: "customer_unavailable", Message: "Карточку клиента не удалось подтвердить."}, nil
		}
		return delegationResult{State: "customer_verified", Message: "Карточка клиента принадлежит организации и привязана к заявке."}, nil
	case "delegation.workshop_stock":
		out := delegationResult{State: "part_unavailable", Message: "Активная деталь с остатком не найдена.", WorkshopPart: in.Part}
		if in.Part == nil {
			return out, nil
		}
		if in.Part.Available == nil {
			out.State, out.Message = "stock_stale", "Остаток требует свежей сверки."
		} else {
			out.State = "part_available"
			out.Message = fmt.Sprintf("Подтверждённый доступный остаток %d шт., запрошено %d шт.", *in.Part.Available, in.Quantity)
			if *in.Part.Available < in.Quantity {
				out.State = "insufficient_stock"
			}
		}
		return out, nil
	case "delegation.workshop_sales":
		customer := workshopChild(in.Children, "customer", "delegation.workshop_customer")
		stock := workshopChild(in.Children, "product_inventory", "delegation.workshop_stock")
		if customer == nil || stock == nil {
			return delegationResult{}, af.ErrDenied
		}
		if customer.State != "customer_verified" {
			return delegationResult{State: customer.State, Message: customer.Message}, nil
		}
		if stock.State != "part_available" {
			return delegationResult{State: stock.State, Message: stock.Message}, nil
		}
		if stock.WorkshopPart == nil || stock.WorkshopPart.Available == nil {
			return delegationResult{}, af.ErrDenied
		}
		return delegationResult{State: "approval_required", Message: fmt.Sprintf("Предложение по заявке «%s» для автомобиля «%s»: согласовать резерв выбранной детали. Совместимость проверяет владелец.", in.Service, in.Vehicle),
			PartName: stock.WorkshopPart.Name, SKU: stock.WorkshopPart.SKU, Available: stock.WorkshopPart.Available}, nil
	case workshopManager:
		sales := workshopChild(in.Children, "sales", "delegation.workshop_sales")
		if len(in.Children) != 3 || sales == nil || workshopChild(in.Children, "customer", "delegation.workshop_customer") == nil ||
			workshopChild(in.Children, "product_inventory", "delegation.workshop_stock") == nil {
			return delegationResult{State: "partial_failure", Children: in.Children, Message: "Проверки завершены частично. Резерв недоступен."}, nil
		}
		out := *sales
		out.Children = in.Children
		return out, nil
	default:
		return delegationResult{}, af.ErrDenied
	}
}

func workshopChild(children []delegationChild, agent, action string) *delegationResult {
	for _, child := range children {
		if child.Agent == agent && child.Action == action && child.Status == "completed" {
			return child.Result
		}
	}
	return nil
}
