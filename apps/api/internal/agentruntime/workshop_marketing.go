package agentruntime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
)

const workshopMarketingManager = "delegate_workshop_marketing"

func marketingDelegation(action string) bool {
	return action == workshopMarketingManager || action == "delegation.workshop_marketing"
}

type WorkshopMarketingInput struct {
	Action    string  `json:"action"`
	Reference *string `json:"reference,omitempty"`
	DraftID   string  `json:"draft_id,omitempty"`
	Version   int     `json:"version,omitempty"`
}

func (s Service) WorkshopMarketingAction(ctx context.Context, t catalog.Tenant, request string, in WorkshopMarketingInput) (string, error) {
	invalid := auth.Failure(422, "INVALID_MARKETING_ACTION", "Проверьте параметры маркетинга и подтверждение согласия")
	if !organizations.ValidID(request) {
		return "", invalid
	}
	switch in.Action {
	case "consent":
		if in.Reference == nil || strings.TrimSpace(*in.Reference) == "" || !workflowText(*in.Reference, 120) ||
			in.DraftID != "" || in.Version != 0 {
			return "", invalid
		}
	case "revoke", "prepare":
		if in.Reference != nil || in.DraftID != "" || in.Version != 0 {
			return "", invalid
		}
	case "approved", "rejected":
		if in.Reference != nil || !organizations.ValidID(in.DraftID) || in.Version != 1 {
			return "", invalid
		}
	default:
		return "", invalid
	}
	tx, err := tenancy.Begin(ctx, s.Pool, t, true, false)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var ok bool
	id := &request
	switch in.Action {
	case "consent", "revoke":
		var reference *string
		if in.Reference != nil {
			value := strings.TrimSpace(*in.Reference)
			reference = &value
		}
		err = tx.QueryRow(ctx, "SELECT platform.workshop_record_consent($1,$2,$3,$4)", t.OrganizationID, request, in.Action == "consent", reference).Scan(&ok)
	case "prepare":
		if len(s.Key) != 32 {
			return "", auth.Unavailable()
		}
		err = tx.QueryRow(ctx, "SELECT platform.workshop_request_marketing($1,$2,sha256($3::bytea))", t.OrganizationID, request, s.Proof()).Scan(&id)
		ok = id != nil
	case "approved", "rejected":
		err = tx.QueryRow(ctx, `SELECT CASE WHEN EXISTS(SELECT 1 FROM platform.agent_tasks WHERE organization_id=$1 AND id=$2
    AND correlation_id=$3 AND action='delegation.workshop_marketing')
    THEN platform.marketing_draft_action($1,$2,$4,$5,NULL,NULL) ELSE false END`,
			t.OrganizationID, in.DraftID, request, in.Version, in.Action).Scan(&ok)
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "54000" {
			return "", auth.Failure(429, "RATE_LIMITED", "Слишком много задач")
		}
		return "", tenancy.Error(err)
	}
	if !ok {
		return "", auth.Failure(409, "MARKETING_UNAVAILABLE", "Проверьте согласие клиента, результаты заявки и состояние предложения")
	}
	if err = tx.Commit(ctx); err != nil {
		return "", tenancy.Error(err)
	}
	return *id, nil
}

type WorkshopMarketingDraft struct {
	Generation            string `json:"generation"`
	ExternalCommunication bool   `json:"external_communication"`
	Goal                  string `json:"goal"`
	Audience              string `json:"audience"`
	Channel               string `json:"channel"`
	Text                  string `json:"text,omitempty"`
}

type workshopMarketingEvidence struct {
	CustomerCompleted int64                    `json:"customer_completed"`
	ActiveServices    int64                    `json:"active_services"`
	AvailableParts    int64                    `json:"available_parts"`
	Financial         *workshopFinancialResult `json:"financial"`
}

func runWorkshopMarketing(action string, raw json.RawMessage) (delegationResult, error) {
	var in struct {
		workshopMarketingEvidence
		Proposal *delegationResult `json:"proposal"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return delegationResult{}, err
	}
	if action == workshopMarketingManager {
		if in.Proposal == nil || in.Proposal.MarketingEvidence == nil {
			return delegationResult{State: "marketing_unavailable", Message: "Предложение недоступно. Проверьте согласие и результаты заявки."}, nil
		}
		return delegationResult{State: "marketing_review", Message: in.Proposal.Message, MarketingEvidence: in.Proposal.MarketingEvidence}, nil
	}
	if in.CustomerCompleted < 1 || in.Financial == nil || in.Financial.Finance == nil || in.Financial.Analytics == nil {
		return delegationResult{State: "marketing_unavailable"}, nil
	}
	goal := "Запросить обратную связь о завершённом обслуживании"
	body := "Добрый день! Спасибо, что обратились в наш автосервис. Подскажите, пожалуйста, всё ли вас устроило после обслуживания?"
	message := "Рекомендую запросить обратную связь. История подтверждает завершённое обслуживание. Для предложения конкретной услуги или детали нужно проверить возможности."
	if in.ActiveServices > 0 && in.AvailableParts > 0 && in.Financial.Finance.GrossMinor > 0 && in.Financial.Analytics.GrossMinor > 0 {
		goal = "Уточнить потребность клиента в следующем визите"
		body = "Добрый день! Спасибо, что обратились в наш автосервис. Если вам нужен следующий визит, напишите, какую работу планируете. Мы уточним подходящую услугу и наличие деталей."
		message = "Рекомендую предложить клиенту обсудить следующий визит. В каталоге есть активные услуги и детали со свежим остатком. Валовая прибыль по заявке и учтённым услугам положительна. Черновик оставляет сроки, совместимость и цену для отдельной проверки."
	}
	evidence := in.workshopMarketingEvidence
	return delegationResult{State: "marketing_draft_ready", Message: message, MarketingEvidence: &evidence,
		WorkshopMarketingDraft: &WorkshopMarketingDraft{Generation: "workshop_contextual", Goal: goal, Audience: "Клиент завершённой заявки с подтверждённым согласием",
			Channel: "internal_review", Text: body}}, nil
}
