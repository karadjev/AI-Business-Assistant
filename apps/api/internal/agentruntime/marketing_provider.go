package agentruntime

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/agent"
	af "github.com/karadjev/AI-Business-Assistant/apps/api/internal/agentfoundation"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/specialists"
)

type marketingIntent string

const (
	marketingVerifyStock  marketingIntent = "verify_stock_first"
	marketingReviewStock  marketingIntent = "review_stock_before_offer"
	marketingPrepareOffer marketingIntent = "prepare_offer_from_stock"

	marketingToolName       = "select_marketing_draft_intent"
	marketingGeneration     = "synthetic_contextual"
	marketingFreshStock     = "fresh_available"
	marketingNoFreshStock   = "no_fresh_available"
	marketingRecentDemand   = "recent_completed_orders"
	marketingNoRecentDemand = "no_recent_completed_orders"
	marketingInstruction    = "Select one internal draft intent supported by the supplied catalog and completed-order facts."
)

var marketingTopics = map[string]string{
	"electronics":        "электронике",
	"auto_parts":         "автозапчастям",
	"building_materials": "стройматериалам",
	"furniture":          "мебели",
	"apparel":            "одежде",
	"mixed_nonfood":      "непродовольственному ассортименту",
}

type marketingSelection struct {
	Intent marketingIntent `json:"intent"`
}

func (f MarketingFacts) valid() bool {
	return marketingTopics[f.Industry] != "" &&
		(f.Inventory == marketingFreshStock || f.Inventory == marketingNoFreshStock) &&
		(f.Demand == marketingRecentDemand || f.Demand == marketingNoRecentDemand)
}

func (f MarketingFacts) intent() marketingIntent {
	if f.Inventory == marketingNoFreshStock {
		return marketingVerifyStock
	}
	if f.Demand == marketingRecentDemand {
		return marketingPrepareOffer
	}
	return marketingReviewStock
}

func buildMarketingRequest(facts MarketingFacts) (agent.Request, error) {
	if !facts.valid() {
		return agent.Request{}, af.ErrDenied
	}
	raw, err := json.Marshal(facts)
	if err != nil {
		return agent.Request{}, err
	}
	return agent.Request{
		Messages: []agent.Message{
			{Role: "system", Content: marketingInstruction},
			{Role: "user", Content: string(raw)},
		},
		Tools: []agent.Tool{{
			Type: "function",
			Function: agent.Function{
				Name:        marketingToolName,
				Description: "Select a grounded internal review intent.",
				Strict:      true,
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"intent": map[string]any{"type": "string", "enum": []marketingIntent{
							marketingVerifyStock, marketingReviewStock, marketingPrepareOffer,
						}},
					},
					"required":             []string{"intent"},
					"additionalProperties": false,
				},
			},
		}},
		// The in-process synthetic provider consumes no model tokens or money.
		InputTokens: 0, OutputTokens: 0,
	}, nil
}

// This provider is constructed locally. Worker configuration and request data
// never supply a provider, client, endpoint, credential or authorization token.
type marketingSyntheticProvider struct{}

var _ agent.Provider = marketingSyntheticProvider{}

func (marketingSyntheticProvider) Mode() string { return "mock" }

func (marketingSyntheticProvider) Generate(ctx context.Context, request agent.Request) (agent.Response, error) {
	if err := ctx.Err(); err != nil {
		return agent.Response{}, err
	}
	if request.InputTokens != 0 || request.OutputTokens != 0 ||
		len(request.Messages) != 2 || request.Messages[0].Role != "system" ||
		request.Messages[0].Content != marketingInstruction || request.Messages[1].Role != "user" ||
		len(request.Tools) != 1 || request.Tools[0].Type != "function" ||
		request.Tools[0].Function.Name != marketingToolName {
		return agent.Response{}, af.ErrDenied
	}
	var facts MarketingFacts
	if !decodeMarketingJSON([]byte(request.Messages[1].Content), &facts) || !facts.valid() {
		return agent.Response{}, af.ErrDenied
	}
	args, err := json.Marshal(marketingSelection{Intent: facts.intent()})
	if err != nil {
		return agent.Response{}, err
	}
	call := agent.Call{ID: "synthetic-marketing-intent", Type: "function"}
	call.Function.Name = marketingToolName
	call.Function.Arguments = string(args)
	return agent.Response{Calls: []agent.Call{call}, UsageKnown: true}, nil
}

func generateMarketingDraft(ctx context.Context, facts MarketingFacts) (MarketingDraft, error) {
	request, err := buildMarketingRequest(facts)
	if err != nil {
		return MarketingDraft{}, err
	}
	var provider agent.Provider = marketingSyntheticProvider{}
	response, err := provider.Generate(ctx, request)
	if err != nil {
		return MarketingDraft{}, err
	}
	intent, err := decodeMarketingResponse(response)
	if err != nil {
		return MarketingDraft{}, err
	}
	return renderMarketingDraft(facts, intent)
}

func decodeMarketingResponse(response agent.Response) (marketingIntent, error) {
	if !response.UsageKnown || response.InputTokens != 0 || response.OutputTokens != 0 || len(response.Calls) != 1 {
		return "", af.ErrBudget
	}
	call := response.Calls[0]
	var selection marketingSelection
	raw := []byte(call.Function.Arguments)
	if call.Type != "function" || call.Function.Name != marketingToolName || !decodeMarketingJSON(raw, &selection) {
		return "", af.ErrDenied
	}
	canonical, err := json.Marshal(selection)
	if err != nil || !bytes.Equal(raw, canonical) {
		return "", af.ErrDenied
	}
	switch selection.Intent {
	case marketingVerifyStock, marketingReviewStock, marketingPrepareOffer:
		return selection.Intent, nil
	}
	return "", af.ErrDenied
}

func renderMarketingDraft(facts MarketingFacts, intent marketingIntent) (MarketingDraft, error) {
	if !facts.valid() || intent != facts.intent() {
		return MarketingDraft{}, af.ErrDenied
	}
	goal := "Проверить готовность предложения"
	audience := "Потенциальные покупатели, которые изучают ассортимент"
	channel := "internal_review"
	idea := "Уточнить потребность клиента перед подготовкой предложения"
	var guidance, copy string
	switch intent {
	case marketingVerifyStock:
		guidance = "Сначала сверьте остатки и исключите неподтверждённые позиции из предложения."
		copy = "Расскажите, что вы ищете. Мы уточним доступные варианты и сообщим условия."
	case marketingReviewStock:
		guidance = "Проверьте позиции с подтверждённым наличием и уточните спрос перед подготовкой предложения."
		goal = "Собрать вопросы об ассортименте"
		channel = "instagram"
		idea = "Публикация с визуальным обзором и приглашением задать вопрос"
		copy = "Выбираете товар? Напишите, что для вас важно при выборе. Подготовим обзор по вашим вопросам."
	case marketingPrepareOffer:
		guidance = "При подготовке предложения учтите недавние завершённые заказы и используйте только позиции с подтверждённым наличием."
		goal = "Подготовить обзор подтверждённого ассортимента"
		channel = "telegram"
		idea = "Короткий обзор ассортимента с приглашением уточнить наличие"
		copy = "Нужна помощь с выбором? Расскажите, для какой задачи подбираете товар. Уточним наличие подходящих вариантов."
	}
	return MarketingDraft{
		Draft: specialists.Draft{
			Draft:                 true,
			Text:                  "Черновик по " + marketingTopics[facts.Industry] + ". " + copy,
			ExternalCommunication: false,
		},
		Generation: marketingGeneration,
		Intent:     intent,
		Evidence:   facts,
		Goal:       goal,
		Audience:   audience,
		Channel:    channel,
		Idea:       idea,
		Rationale:  guidance,
		Assumption: "Аудитория предложена как гипотеза. Доступность канала и согласие получателей требуют проверки. WhatsApp подходит только для разрешённого диалога или проверенного шаблона.",
		Approval:   "Перед публикацией, отправкой или запуском требуется решение владельца.",
	}, nil
}
