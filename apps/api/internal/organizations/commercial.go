package organizations

import (
	"context"
	"encoding/json"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
)

type CommercialSettings struct {
	PlanCode        string `json:"plan_code"`
	Status          string `json:"status"`
	ExpectedVersion int64  `json:"expected_version"`
	StaffLimit      *int   `json:"staff_limit"`
	AgentLimit      *int   `json:"agent_limit"`
	ChannelLimit    *int   `json:"channel_limit"`
	TaskLimit       *int   `json:"task_limit"`
	AIMonthlyCap    *int64 `json:"ai_monthly_cap"`
}

func (s Service) Commercial(ctx context.Context, session auth.Session, org string, change *CommercialSettings) (json.RawMessage, error) {
	if !ValidID(org) {
		return nil, auth.Failure(404, "NOT_FOUND", "Организация не найдена")
	}
	tx, err := s.transaction(ctx, session, org)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if change != nil {
		if change.ExpectedVersion < 0 || (change.Status != "active" && change.Status != "paused") ||
			(change.PlanCode != "pilot" && change.PlanCode != "standard" && change.PlanCode != "custom") {
			return nil, auth.Failure(422, "VALIDATION_ERROR", "Проверьте тариф, статус и версию")
		}
		var saved bool
		err = tx.QueryRow(ctx, "SELECT platform.commercial_configure($1,$2,$3,$4,$5,$6,$7,$8,$9)", org, change.ExpectedVersion,
			change.PlanCode, change.Status, change.StaffLimit, change.AgentLimit, change.ChannelLimit, change.TaskLimit, change.AIMonthlyCap).Scan(&saved)
		if err != nil {
			return nil, teamError(err)
		}
		if !saved {
			return nil, auth.Failure(409, "VERSION_CONFLICT", "Подписка изменилась. Обновите страницу")
		}
	}
	var result json.RawMessage
	if err = tx.QueryRow(ctx, "SELECT platform.commercial_state($1)", org).Scan(&result); err != nil {
		return nil, teamError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, teamError(err)
	}
	return result, nil
}
