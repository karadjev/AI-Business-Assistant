package agentruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	af "github.com/karadjev/AI-Business-Assistant/apps/api/internal/agentfoundation"
	bf "github.com/karadjev/AI-Business-Assistant/apps/api/internal/businessfoundation"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/servicebooking"
)

const conversationDelegation = "delegate_conversation"

type conversationProduct struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	SKU      string `json:"sku"`
	Price    int64  `json:"price_amount"`
	Currency string `json:"currency_code"`
	Stock    *int   `json:"stock,omitempty"`
}

type conversationOrder struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type conversationOffering struct {
	ID         string `json:"id"`
	Revision   int64  `json:"revision"`
	Timezone   string `json:"timezone"`
	MinMinutes int    `json:"min_minutes"`
	MaxMinutes int    `json:"max_minutes"`
	PropertyID string `json:"property_id"`
}

type conversationLookup struct {
	Products  []conversationProduct  `json:"products"`
	Order     *conversationOrder     `json:"order"`
	Offerings []conversationOffering `json:"offerings"`
}

func (d delegationTool) conversationStep(ctx context.Context, evidence delegationContext) (delegationResult, error) {
	if d.job.Action == "delegation.conversation_intent" {
		var snapshot *struct {
			Text     string            `json:"text"`
			Previous conversationRoute `json:"previous"`
		}
		if err := d.service.Pool.QueryRow(ctx, "SELECT platform.conversation_intent_context($1,$2)", d.job.ID, d.lease).Scan(&snapshot); err != nil {
			return delegationResult{}, err
		}
		if snapshot == nil || snapshot.Text == "" {
			return delegationResult{}, af.ErrDenied
		}
		route := classifyConversation(snapshot.Text, snapshot.Previous)
		return delegationResult{State: "intent_classified", Route: &route}, nil
	}
	if d.job.Action != "delegation.conversation_plan" || len(evidence.Children) != 1 ||
		evidence.Children[0].Status != "completed" || evidence.Children[0].Result == nil || evidence.Children[0].Result.Route == nil {
		return delegationResult{}, af.ErrDenied
	}
	route := *evidence.Children[0].Result.Route
	out := delegationResult{State: "answered", Route: &route}
	if len(route.Missing) != 0 {
		return clarifyConversation(out), nil
	}
	var lookup *conversationLookup
	if err := d.service.Pool.QueryRow(ctx, "SELECT platform.conversation_plan_lookup($1,$2)", d.job.ID, d.lease).Scan(&lookup); err != nil {
		return delegationResult{}, err
	}
	if lookup == nil {
		return delegationResult{}, af.ErrDenied
	}

	f := route.Confirmed
	var input any
	switch route.Intent {
	case support:
		out.Message = "Опишите, чем помочь. Можно спросить о товаре, цене, наличии, заказе, расчёте материалов, записи или недвижимости."
	case productSearch, priceInquiry, inventory:
		out.Catalog = lookup.Products
		out.Message = productReply(route.Intent, lookup.Products)
	case orderStatus:
		if !f.validOrder() || lookup.Order == nil {
			out.Message = "Не нашёл заказ с этим номером в данном диалоге. Проверьте номер или обратитесь к сотруднику."
		} else {
			out.Order = lookup.Order
			out.Message = fmt.Sprintf("Статус заказа %s — %s.", lookup.Order.ID, orderStatusText(lookup.Order.Status))
		}
	case realEstate:
		if !f.Property.Valid() {
			route.Missing = []string{"confirmation"}
			return clarifyConversation(out), nil
		}
		route.Workflow, input = realEstateDelegation, PropertyRequirements{PropertyCriteria: f.Property}
	case compatibility:
		// Checking one unit asks only for compatibility and current stock evidence.
		requirements := AutoPartsRequirements{OEM: f.OEM, Vehicle: f.Vehicle, Quantity: 1}
		if !requirements.valid() {
			route.Missing = []string{"confirmation"}
			return clarifyConversation(out), nil
		}
		route.Workflow, input = autoPartsDelegation, requirements
	case construction:
		calculation := estimateInput(f)
		if _, err := bf.CalculateConstruction(*calculation); err != nil {
			route.Missing = []string{"dimensions", "tile_dimensions", "waste"}
			return clarifyConversation(out), nil
		}
		route.Workflow, input = constructionDelegation, ConstructionRequirements{SKU: f.SKU, Calculation: calculation}
	case serviceBooking, propertyView:
		if len(lookup.Offerings) != 1 {
			if route.Intent == serviceBooking {
				route.Missing = []string{"service"}
				return clarifyConversation(out), nil
			}
			out.State = "selection_unavailable"
			out.Message = "Не удалось подтвердить объект и условия просмотра. Сначала запросите актуальный подбор."
			return out, nil
		}
		offering := lookup.Offerings[0]
		selection, missing := appointmentSelection(f, offering)
		if missing != "" {
			route.Missing = []string{missing}
			return clarifyConversation(out), nil
		}
		if route.Intent == serviceBooking {
			r := AutoPartsRequirements{Vehicle: f.Vehicle}
			if !r.valid() {
				route.Missing = []string{"make", "model", "year"}
				return clarifyConversation(out), nil
			}
			route.Workflow, input = serviceDelegation, ServiceRequirements{Vehicle: f.Vehicle, Selection: selection}
		} else {
			route.Workflow, input = propertyViewingDelegation, PropertyViewingRequest{
				OfferTaskID: route.OfferTaskID, PropertyID: offering.PropertyID, Selection: *selection,
			}
		}
	default:
		return delegationResult{}, af.ErrDenied
	}
	if route.Workflow == "" {
		return out, nil
	}
	var err error
	route.Input, err = json.Marshal(input)
	if err != nil {
		return delegationResult{}, err
	}
	out.State = "dispatch_ready"
	return out, nil
}

func appointmentSelection(f confirmedFields, offering conversationOffering) (*servicebooking.Selection, string) {
	zone, err := time.LoadLocation(offering.Timezone)
	if err != nil || offering.Timezone == "Local" {
		return nil, "service"
	}
	duration := f.Duration
	if duration == 0 && offering.MinMinutes == offering.MaxMinutes {
		duration = offering.MinMinutes
	}
	if duration <= 0 || duration < offering.MinMinutes || duration > offering.MaxMinutes {
		return nil, "duration"
	}
	local := f.Date + " " + f.Time
	start, err := time.ParseInLocation("2006-01-02 15:04", local, zone)
	if err != nil || !start.After(time.Now()) || start.In(zone).Format("2006-01-02 15:04") != local {
		return nil, "date_and_time"
	}
	// An ambiguous wall time needs an explicit offset. Neither default is consent.
	for _, offset := range []time.Duration{-2 * time.Hour, -time.Hour, -30 * time.Minute, 30 * time.Minute, time.Hour, 2 * time.Hour} {
		if start.Add(offset).In(zone).Format("2006-01-02 15:04") == local {
			return nil, "date_and_time"
		}
	}
	selection := &servicebooking.Selection{OfferingID: offering.ID, OfferingRevision: offering.Revision,
		StartsAt: start, EndsAt: start.Add(time.Duration(duration) * time.Minute).In(zone), Timezone: offering.Timezone, Units: 1}
	if !validWorkflowSelection(selection) {
		return nil, "date_and_time"
	}
	return selection, ""
}

func clarifyConversation(out delegationResult) delegationResult {
	labels := map[string]string{
		"product": "название или артикул товара", "order_id": "номер заказа (UUID из заявки)",
		"oem": "номер OEM", "make": "марка автомобиля", "model": "модель автомобиля", "year": "год выпуска", "engine": "двигатель",
		"sku": "артикул материала (SKU)", "dimensions": "размеры помещения (например, 4 × 3 м)",
		"tile_dimensions": "размеры плитки (например, плитка 30 × 30 см)", "waste": "процент запаса",
		"service": "точное название услуги (например, Услуга: Диагностика)", "date": "дата (ГГГГ-ММ-ДД)", "time": "время (ЧЧ:ММ)",
		"duration": "желаемая длительность в минутах", "date_and_time": "будущие дата и время без неоднозначного перевода часов",
		"transaction_type": "покупка, долгосрочная или краткосрочная аренда", "property_type": "тип недвижимости",
		"city": "город", "budget": "верхняя граница бюджета (например, до 5 млн рублей)", "currency": "валюта бюджета",
		"property": "номер варианта из предложения или ID объекта", "confirmation": "подтвердите точные параметры запроса",
	}
	var missing []string
	for _, key := range out.Route.Missing {
		missing = append(missing, labels[key])
	}
	out.State = "clarification_required"
	out.Required = out.Route.Missing
	out.Message = "Уточните, пожалуйста: " + strings.Join(missing, ", ") + "."
	return out
}

func orderStatusText(status string) string {
	switch status {
	case "pending_confirmation":
		return "ожидает решения сотрудника"
	case "confirmed":
		return "подтверждён"
	case "rejected":
		return "отклонён"
	case "cancelled":
		return "отменён"
	case "completed":
		return "завершён"
	default:
		return "требует уточнения у сотрудника"
	}
}

func productReply(intent conversationIntent, rows []conversationProduct) string {
	if len(rows) == 0 {
		return "Подходящих записей в текущем каталоге не найдено. Уточните название или артикул."
	}
	lines := make([]string, 0, len(rows))
	for _, p := range rows {
		line := fmt.Sprintf("%s (%s)", p.Name, p.SKU)
		if intent != inventory {
			line += fmt.Sprintf(" · %d.%02d %s", p.Price/100, p.Price%100, p.Currency)
		}
		if intent != priceInquiry {
			if p.Stock == nil {
				line += " · наличие требует проверки"
			} else {
				line += fmt.Sprintf(" · доступно %d шт.", *p.Stock)
			}
		}
		lines = append(lines, line)
	}
	return shortChannelReply(strings.Join(lines, "\n"))
}

func shortChannelReply(s string) string {
	if len(s) <= 950 {
		return s
	}
	end := 947
	for !utf8.RuneStart(s[end]) {
		end--
	}
	return s[:end] + "…"
}

func conversationalWorkflowReply(action string, out delegationResult) string {
	if action == realEstateDelegation && len(out.Properties) > 0 && out.State == "proposal_ready" {
		lines := []string{"Подходящие объекты:"}
		for i, p := range out.Properties {
			lines = append(lines, fmt.Sprintf("Вариант %d · %s · %s м² · ID %s", i+1, p.Title, p.Area, p.ID))
		}
		return shortChannelReply(strings.Join(lines, "\n") + "\n" + out.Message)
	}
	if out.Message != "" {
		return shortChannelReply(out.Message)
	}
	switch {
	case out.State == "approval_required":
		return "Условия проверены. Сотрудник должен оформить заявку и принять решение."
	case action == autoPartsDelegation && out.State == "proposal_ready":
		return "Совместимость подтверждена по каталогу, доступность проверена. Резервирование требует решения сотрудника."
	case action == autoPartsDelegation:
		return "Не найдено подтверждённого варианта для указанного автомобиля и номера OEM."
	case action == constructionDelegation && out.Calculation != nil &&
		(out.State == "proposal_ready" || out.State == "stock_unverified_or_insufficient"):
		r := out.Calculation
		message := fmt.Sprintf("По расчёту потребуется %s %s, упаковок: %d. Формула: %s.", r.Quantity, r.Unit, r.PackageCount, r.Formula)
		if out.State == "stock_unverified_or_insufficient" {
			message += " Наличие требует уточнения."
		}
		return shortChannelReply(message)
	case action == serviceDelegation && (out.State == "part_requirement_unknown" || out.State == "part_decision_required"):
		return "Для записи сотруднику нужно уточнить необходимость запчастей."
	default:
		return "Проверенный вариант по указанным параметрам недоступен. Уточните запрос или обратитесь к сотруднику."
	}
}
