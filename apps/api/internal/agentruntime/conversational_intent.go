package agentruntime

import (
	"encoding/json"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	bf "github.com/karadjev/AI-Business-Assistant/apps/api/internal/businessfoundation"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/orders"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
)

type conversationIntent string

const (
	productSearch  conversationIntent = "product_search"
	priceInquiry   conversationIntent = "price_inquiry"
	inventory      conversationIntent = "inventory_inquiry"
	compatibility  conversationIntent = "auto_parts_compatibility"
	construction   conversationIntent = "construction_calculation"
	serviceBooking conversationIntent = "auto_service_booking"
	realEstate     conversationIntent = "real_estate_search"
	propertyView   conversationIntent = "property_viewing_request"
	orderStatus    conversationIntent = "order_status"
	support        conversationIntent = "customer_support"
)

type confirmedFields struct {
	Product    string                  `json:"product,omitempty"`
	OrderID    string                  `json:"order_id,omitempty"`
	OEM        string                  `json:"oem,omitempty"`
	Vehicle    catalog.FitmentVehicle  `json:"vehicle,omitzero"`
	SKU        string                  `json:"sku,omitempty"`
	Dimensions []string                `json:"dimensions,omitempty"`
	Materials  []string                `json:"materials,omitempty"`
	Waste      string                  `json:"waste_percent,omitempty"`
	Service    string                  `json:"service,omitempty"`
	Date       string                  `json:"date,omitempty"`
	Time       string                  `json:"time,omitempty"`
	Duration   int                     `json:"duration,omitempty"`
	Property   orders.PropertyCriteria `json:"property,omitzero"`
	PropertyID string                  `json:"property_id,omitempty"`
	Option     int                     `json:"option,omitempty"`
}

type conversationRoute struct {
	Intent      conversationIntent `json:"intent"`
	Confirmed   confirmedFields    `json:"confirmed"`
	Missing     []string           `json:"missing,omitempty"`
	Workflow    string             `json:"workflow,omitempty"`
	Input       json.RawMessage    `json:"input,omitempty"`
	OfferTaskID string             `json:"offer_task_id,omitempty"`
}

var (
	injection = regexp.MustCompile(`(?i)(ignore (all |the |previous )?(instructions|rules)|system prompt|developer message|tool_call|permissions|игнорируй|системн[^ ]* (промпт|инструкц)|роль администратора)`)
	uncertain = regexp.MustCompile(`(?i)(?:вероятно|возможно|наверное|не уверен|может быть|probably|maybe|not sure|approximately)`)
	intents   = []struct {
		intent conversationIntent
		text   *regexp.Regexp
	}{
		{propertyView, regexp.MustCompile(`(?i)(просмотр|посмотреть квартир|показ объект|viewing|view (the |this )?property)`)},
		{orderStatus, regexp.MustCompile(`(?i)(статус (моего )?заказ|где (мой )?заказ|что с заказ|order status|track (my )?order)`)},
		{serviceBooking, regexp.MustCompile(`(?i)(автосервис|записа[^ ]* на (диагностик|ремонт|то)|запись на (то|диагностик)|service booking|book (a |an )?(car service|inspection))`)},
		{compatibility, regexp.MustCompile(`(?i)(подойд[её]т|совместим|запчаст|fitment|compatible|auto part|\boem\b)`)},
		{construction, regexp.MustCompile(`(?i)(сколько (плит|блок)|рассчита[^ ]* (расход|плит|материал)|расч[её]т (плит|материал)|construction|calculate (the )?(tiles|materials))`)},
		{realEstate, regexp.MustCompile(`(?i)(квартир|недвижим|земельн[^ ]* участ|новострой|аренд[^ ]* (дом|помещ)|купить (дом|помещ)|apartment|real estate|rent a (house|flat)|commercial property)`)},
		{priceInquiry, regexp.MustCompile(`(?i)(цена|сколько стоит|стоимость|\bprice\b|how much)`)},
		{inventory, regexp.MustCompile(`(?i)(в наличии|наличи[еи]|остаток|есть ли|in stock|inventory|available stock)`)},
		{support, regexp.MustCompile(`(?i)(гаранти|возврат|сотрудник|консультац|поддержк|warranty|customer support|talk to (an |a )?agent)`)},
		{productSearch, regexp.MustCompile(`(?i)(найд[иу]|ищу|подбер[иу]|нужен|нужна|нужны|покажи|find|looking for|search for)`)},
	}
	productPrefix = regexp.MustCompile(`(?i)^(?:подскажите[, ]+|пожалуйста[, ]+)*(?:цена|сколько стоит|стоимость|в наличии|наличие|остаток|есть ли|найди|найдите|ищу|подбери|нужен|нужна|нужны|покажи|price(?: of)?|how much(?: is)?|in stock|inventory|find|looking for|search for)\s*[: ]*`)
	oemValue      = regexp.MustCompile(`(?i)\boem\s*[:=]?\s*([a-z0-9][a-z0-9_.-]{0,63})\b`)
	skuValue      = regexp.MustCompile(`(?i)(?:\bsku|артикул)\s*[:=]?\s*([a-z0-9][a-z0-9_.-]{0,63})\b`)
	vehicleValue  = regexp.MustCompile(`(?i)(?:автомобиль|машина|vehicle|car)\s*[:=]?\s*([a-zа-яё0-9-]+)\s+([a-zа-яё0-9-]+)\s+(19\d{2}|20\d{2})`)
	yearValue     = regexp.MustCompile(`\b(19\d{2}|20\d{2})\b`)
	engineValue   = regexp.MustCompile(`(?i)(?:двигатель|мотор|engine)\s*[:=]?\s*([a-zа-яё0-9][a-zа-яё0-9.,_-]{0,39})`)
	makeValue     = regexp.MustCompile(`(?i)(?:марка|make)\s*[:=]?\s*([a-zа-яё0-9-]+)`)
	modelValue    = regexp.MustCompile(`(?i)(?:модель|model)\s*[:=]?\s*([a-zа-яё0-9-]+)`)
	dimsValue     = regexp.MustCompile(`(?i)(?:комната|размеры|пол|room)?\s*(\d{1,4}(?:[.,]\d{1,4})?)\s*[xх×]\s*(\d{1,4}(?:[.,]\d{1,4})?)\s*(мм|см|м|mm|cm|m)(?:\s|[.,;!?]|$)`)
	itemValue     = regexp.MustCompile(`(?i)(?:плитка|плитки|tiles?)\s*(\d{1,4}(?:[.,]\d{1,4})?)\s*[xх×]\s*(\d{1,4}(?:[.,]\d{1,4})?)\s*(мм|см|м|mm|cm|m)(?:\s|[.,;!?]|$)`)
	wasteValue    = regexp.MustCompile(`(?i)(?:запас|отходы|waste)\s*[:=]?\s*(\d{1,3}(?:[.,]\d{1,4})?)\s*%`)
	dateValue     = regexp.MustCompile(`\b(20\d{2}-\d{2}-\d{2})\b`)
	timeValue     = regexp.MustCompile(`\b([01]\d|2[0-3]):([0-5]\d)\b`)
	durationValue = regexp.MustCompile(`(?i)(\d{1,3})\s*(?:минут|мин\.|minutes)`)
	serviceValue  = regexp.MustCompile(`(?i)(?:услуга|service)\s*[:=]\s*([^;\n,]+)`)
	cityValue     = regexp.MustCompile(`(?i)(?:город\s*[:=]?\s*|city\s*[:=]\s*)([a-zа-яё][a-zа-яё -]{0,70}?)(?:[,;.!?]|\s+(?:до|бюджет|район|за|budget)\s|$)`)
	cityIn        = regexp.MustCompile(`(?i)(?:\sв\s|\sin\s)([a-zа-яё][a-zа-яё -]{0,50}?)(?:[,;.!?]|\s+(?:до|бюджет|район|за|budget)\s|$)`)
	districtValue = regexp.MustCompile(`(?i)(?:район|district)\s*[:=]?\s*([a-zа-яё][a-zа-яё 0-9-]{0,70}?)(?:[,;.!?]|\s+(?:до|бюджет|budget)\s|$)`)
	budgetValue   = regexp.MustCompile(`(?i)(?:бюджет|до|budget|up to)\s*[:=]?\s*(\d{1,14}(?:[.,]\d{1,2})?)\s*(млн|тыс|million|thousand)?\s*(₽|руб(?:лей|ля|ль)?|rub|eur|usd|€|\$)?`)
	roomsValue    = regexp.MustCompile(`(?i)([0-9]{1,2})[- ]?(?:комнат|rooms?)`)
	bedsValue     = regexp.MustCompile(`(?i)([0-9]{1,2})\s*(?:спальн|bedrooms?)`)
	areaValue     = regexp.MustCompile(`(?i)(?:от|min(?:imum)?)\s*(\d{1,6}(?:[.,]\d{1,2})?)\s*(?:м²|м2|m2)`)
	idValue       = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)
	optionValue   = regexp.MustCompile(`(?i)(?:вариант|объект|option)\s*(?:№|#)?\s*([1-3])\b`)
)

func captured(pattern *regexp.Regexp, text string) string {
	match := pattern.FindStringSubmatch(text)
	if len(match) == 0 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func keepValue(field *string, value string, limit int) {
	if value != "" && workflowText(value, limit) {
		*field = value
	}
}

func isProductIntent(intent conversationIntent) bool {
	return intent == productSearch || intent == priceInquiry || intent == inventory
}

func namedIntent(text string) conversationIntent {
	for _, rule := range intents {
		if rule.text.MatchString(text) {
			return rule.intent
		}
	}
	return ""
}

func classifyConversation(text string, previous conversationRoute) conversationRoute {
	text = strings.TrimSpace(text)
	if injection.MatchString(text) {
		return conversationRoute{Intent: support}
	}
	intent := namedIntent(text)
	if intent == "" {
		intent = previous.Intent
		if intent == "" || intent == support || (len(previous.Missing) == 0 && !propertyRevision(intent, text)) {
			return conversationRoute{Intent: support}
		}
	}
	route := conversationRoute{Intent: intent, OfferTaskID: previous.OfferTaskID}
	if previous.Intent == intent && (len(previous.Missing) > 0 || propertyRevision(intent, text)) {
		route.Confirmed = previous.Confirmed
	} else if isProductIntent(intent) && isProductIntent(previous.Intent) {
		route.Confirmed.Product = previous.Confirmed.Product
	}
	if uncertain.MatchString(text) || ambiguousProperty.MatchString(text) {
		if len(previous.Missing) > 0 && previous.Intent == intent {
			route.Missing = previous.Missing
		} else {
			route.Missing = []string{"confirmation"}
		}
		return route
	}
	fields := &route.Confirmed
	switch intent {
	case productSearch, priceInquiry, inventory:
		term := strings.Trim(productPrefix.ReplaceAllString(text, ""), " \t:,.!?")
		if namedIntent(text) == "" || term != strings.Trim(text, " \t:,.!?") {
			keepValue(&fields.Product, term, 100)
		}
		route.need("product", fields.Product == "")
	case orderStatus:
		fields.OrderID = strings.ToLower(idValue.FindString(text))
		route.need("order_id", fields.OrderID == "")
	case compatibility, serviceBooking:
		readVehicle(fields, text)
		if intent == compatibility {
			oem := captured(oemValue, text)
			if oem == "" && slices.Equal(previous.Missing, []string{"oem"}) && bareReference.MatchString(text) {
				oem = text
			}
			keepValue(&fields.OEM, strings.ToUpper(oem), 64)
			route.need("oem", fields.OEM == "")
			route.need("engine", fields.Vehicle.Engine == "")
		} else {
			readAppointment(fields, text)
			keepValue(&fields.Service, captured(serviceValue, text), 140)
			route.need("service", fields.Service == "")
			route.need("date", fields.Date == "")
			route.need("time", fields.Time == "")
		}
		route.need("make", fields.Vehicle.Make == "")
		route.need("model", fields.Vehicle.Model == "")
		route.need("year", fields.Vehicle.Year == 0)
	case construction:
		keepValue(&fields.SKU, strings.ToUpper(captured(skuValue, text)), 64)
		material := itemValue.FindStringSubmatch(text)
		text = itemValue.ReplaceAllString(text, "")
		if len(material) == 4 {
			fields.Materials = material[1:]
		}
		if match := dimsValue.FindStringSubmatch(text); len(match) == 4 {
			fields.Dimensions = match[1:]
		}
		keepValue(&fields.Waste, captured(wasteValue, text), 8)
		route.need("sku", fields.SKU == "")
		route.need("dimensions", len(fields.Dimensions) != 3)
		route.need("tile_dimensions", len(fields.Materials) != 3)
		route.need("waste", fields.Waste == "")
	case realEstate:
		readProperty(&fields.Property, text, previous.Missing)
		route.need("transaction_type", fields.Property.Transaction == "")
		route.need("property_type", fields.Property.Type == "")
		route.need("city", fields.Property.City == "")
		route.need("budget", fields.Property.MaxPrice == nil)
		route.need("currency", fields.Property.Currency == "")
	case propertyView:
		readAppointment(fields, text)
		keepValue(&fields.PropertyID, strings.ToLower(idValue.FindString(text)), 36)
		index := captured(optionValue, text)
		if index == "" && slices.Equal(previous.Missing, []string{"property"}) && len(text) == 1 {
			index = text
		}
		if n, err := strconv.Atoi(index); err == nil && n >= 1 && n <= 3 {
			fields.Option = n
		}
		route.need("property", fields.PropertyID == "" && fields.Option == 0)
		route.need("date", fields.Date == "")
		route.need("time", fields.Time == "")
	}
	return route
}

func (r *conversationRoute) need(key string, missing bool) {
	if missing {
		r.Missing = append(r.Missing, key)
	}
}

var bareReference = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,63}$`)

func propertyRevision(intent conversationIntent, text string) bool {
	return intent == realEstate && (budgetValue.MatchString(text) || cityValue.MatchString(text) ||
		districtValue.MatchString(text) || roomsValue.MatchString(text) || areaValue.MatchString(text))
}

func readVehicle(f *confirmedFields, text string) {
	if match := vehicleValue.FindStringSubmatch(text); len(match) == 4 {
		keepValue(&f.Vehicle.Make, match[1], 80)
		keepValue(&f.Vehicle.Model, match[2], 80)
		f.Vehicle.Year, _ = strconv.Atoi(match[3])
	}
	keepValue(&f.Vehicle.Make, captured(makeValue, text), 80)
	keepValue(&f.Vehicle.Model, captured(modelValue, text), 80)
	keepValue(&f.Vehicle.Engine, captured(engineValue, text), 80)
	if y := captured(yearValue, text); y != "" && captured(dateValue, text) == "" {
		f.Vehicle.Year, _ = strconv.Atoi(y)
	}
}

func readAppointment(f *confirmedFields, text string) {
	if value := captured(dateValue, text); value != "" {
		if _, err := time.Parse(time.DateOnly, value); err == nil {
			f.Date = value
		}
	}
	if value := timeValue.FindString(text); value != "" {
		f.Time = value
	}
	if value := captured(durationValue, text); value != "" {
		f.Duration, _ = strconv.Atoi(value)
	}
}

func readProperty(f *orders.PropertyCriteria, text string, missing []string) {
	lower := strings.ToLower(text)
	for _, pair := range []struct {
		words []string
		value orders.PropertyType
	}{
		{[]string{"новостро", "new development"}, "new_development"},
		{[]string{"квартир", "apartment", "flat"}, "apartment"},
		{[]string{"участ", "land"}, "land"},
		{[]string{"помещ", "коммерческ", "commercial"}, "commercial"},
		{[]string{"дом", "house"}, "house"},
	} {
		if containsPhrase(lower, pair.words...) {
			f.Type = pair.value
			break
		}
	}
	switch {
	case containsPhrase(lower, "посуточ", "краткосроч", "short term", "short-term"):
		f.Transaction = "short_term_rental"
	case containsPhrase(lower, "долгосроч", "надолго", "long term", "long-term"):
		f.Transaction = "long_term_rental"
	case containsPhrase(lower, "купить", "куплю", "купим", "покуп", "продаж", "buy", "sale"):
		f.Transaction = "sale"
	}
	if city := captured(cityValue, text); city != "" {
		keepValue(&f.City, propertyCity(city), 120)
	} else if city = captured(cityIn, text); city != "" && namedIntent(text) == realEstate {
		keepValue(&f.City, propertyCity(city), 120)
	} else if len(missing) == 1 && missing[0] == "city" && regexpCityOnly.MatchString(text) {
		keepValue(&f.City, strings.Trim(text, " .,!"), 120)
	}
	keepValue(&f.District, captured(districtValue, text), 120)
	budgetText := text
	if slices.Contains(missing, "budget") && bareBudget.MatchString(text) {
		budgetText = "до " + text
	}
	if match := budgetValue.FindStringSubmatch(budgetText); len(match) == 4 {
		n := strings.Split(strings.ReplaceAll(match[1], ",", "."), ".")
		whole, _ := strconv.ParseInt(n[0], 10, 64)
		minor := int64(0)
		if len(n) == 2 {
			minor, _ = strconv.ParseInt((n[1] + "0")[:2], 10, 64)
		}
		scale := int64(1)
		switch strings.ToLower(match[2]) {
		case "млн", "million":
			scale = 1000000
		case "тыс", "thousand":
			scale = 1000
		}
		if whole <= (1<<63-1)/100/scale-1 && (whole > 0 || minor > 0) {
			budget := (whole*100 + minor) * scale
			f.MaxPrice = &budget
		}
		currency := match[3]
		switch strings.ToLower(currency) {
		case "₽", "rub", "руб", "рублей", "рубля", "рубль":
			f.Currency = "RUB"
		case "€", "eur":
			f.Currency = "EUR"
		case "$", "usd":
			f.Currency = "USD"
		}
	} else {
		switch strings.ToLower(strings.Trim(text, " .,!?")) {
		case "rub", "рубли", "₽":
			f.Currency = "RUB"
		case "eur", "евро":
			f.Currency = "EUR"
		case "usd", "доллары":
			f.Currency = "USD"
		}
	}
	for _, value := range []struct {
		pattern *regexp.Regexp
		field   **int
	}{{roomsValue, &f.Rooms}, {bedsValue, &f.Bedrooms}} {
		if n, err := strconv.Atoi(captured(value.pattern, text)); err == nil && n <= 50 {
			*value.field = &n
		}
	}
	if n, err := strconv.ParseFloat(strings.ReplaceAll(captured(areaValue, text), ",", "."), 64); err == nil && n > 0 {
		f.MinArea = &n
	}
	f.Normalize()
}

var bareBudget = regexp.MustCompile(`(?i)^\d{1,14}(?:[.,]\d{1,2})?(?:\s*(млн|тыс|million|thousand))?(?:\s*(₽|руб(?:лей|ля|ль)?|rub|eur|usd|€|\$))?[.!]?$\z`)
var ambiguousProperty = regexp.MustCompile(`(?i)(купить|покупка)\s+или\s+(аренд|снят)|(?:buy|rent)\s+or\s+(?:buy|rent)`)
var regexpCityOnly = regexp.MustCompile(`(?i)^[a-zа-яё][a-zа-яё -]{1,40}[.!]?$\z`)

func propertyCity(value string) string {
	switch strings.ToLower(value) {
	case "москве":
		return "Москва"
	case "казани":
		return "Казань"
	case "санкт-петербурге":
		return "Санкт-Петербург"
	}
	return value
}

func containsPhrase(text string, values ...string) bool {
	for _, value := range values {
		if strings.Contains(text, value) {
			return true
		}
	}
	return false
}

func estimateInput(f confirmedFields) *bf.ConstructionInput {
	measure := func(value, unit string) bf.ConstructionMeasure {
		switch unit {
		case "м":
			unit = "m"
		case "см":
			unit = "cm"
		case "мм":
			unit = "mm"
		}
		return bf.ConstructionMeasure{Value: strings.ReplaceAll(value, ",", "."), Unit: strings.ToLower(unit)}
	}
	return &bf.ConstructionInput{
		Operation: bf.ConstructionTiles,
		Length:    measure(f.Dimensions[0], f.Dimensions[2]), Width: measure(f.Dimensions[1], f.Dimensions[2]),
		ItemLength: measure(f.Materials[0], f.Materials[2]), ItemWidth: measure(f.Materials[1], f.Materials[2]),
		WastePercent: strings.ReplaceAll(f.Waste, ",", "."),
	}
}

func (f confirmedFields) validOrder() bool { return organizations.ValidID(f.OrderID) }
