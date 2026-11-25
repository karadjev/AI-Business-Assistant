package organizations

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
)

type TeamCommand struct {
	Email    string `json:"email,omitempty"`
	Token    string `json:"token,omitempty"`
	Action   string `json:"action,omitempty"`
	Target   string `json:"target,omitempty"`
	Operator *bool  `json:"operator,omitempty"`
}

func teamError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "42501":
			return auth.Failure(403, "FORBIDDEN", "Раздел доступен владельцу организации")
		case "P4L01":
			return auth.CommercialAllowance(pgErr.Detail)
		case "23503", "23514":
			return auth.Failure(422, "VALIDATION_ERROR", "Некорректный тариф или лимит")
		}
	}
	return auth.Unavailable()
}

func (s Service) Team(ctx context.Context, session auth.Session, org string, usage bool) (json.RawMessage, error) {
	if !ValidID(org) {
		return nil, auth.Failure(404, "NOT_FOUND", "Организация не найдена")
	}
	tx, err := s.transaction(ctx, session, org)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	query := "SELECT platform.team_state($1)"
	if usage {
		query = "SELECT platform.workspace_usage($1)"
	}
	var result json.RawMessage
	if err = tx.QueryRow(ctx, query, org).Scan(&result); err != nil {
		return nil, teamError(err)
	}
	return result, nil
}

func (s Service) ManageTeam(ctx context.Context, session auth.Session, org string, in TeamCommand) (map[string]string, error) {
	if !ValidID(org) {
		return nil, auth.Failure(404, "NOT_FOUND", "Организация не найдена")
	}
	tx, err := s.transaction(ctx, session, org)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	result := map[string]string{"status": "ok"}
	switch in.Action {
	case "invite":
		email, parseErr := auth.Email(in.Email)
		if parseErr != nil || in.Operator == nil {
			return nil, auth.Failure(422, "VALIDATION_ERROR", "Укажите адрес электронной почты и роль")
		}
		token := auth.Token()
		_, err = tx.Exec(ctx, "SELECT platform.team_invite($1,$2,$3,$4)", org, email, auth.Hash(token), *in.Operator)
		result["token"] = token
	case "accept":
		if len(in.Token) != 43 {
			return nil, auth.Failure(422, "INVALID_INVITE", "Приглашение недействительно")
		}
		var accepted string
		err = tx.QueryRow(ctx, "SELECT platform.team_accept($1,$2)", org, auth.Hash(in.Token)).Scan(&accepted)
		if err == nil {
			switch accepted {
			case "accepted":
			case "limit":
				return nil, auth.Failure(409, "SEAT_LIMIT", "Свободных мест для сотрудников нет")
			case "member":
				return nil, auth.Failure(409, "ALREADY_MEMBER", "Пользователь уже состоит в организации")
			default:
				return nil, auth.Failure(409, "INVALID_INVITE", "Приглашение истекло, отозвано или вы вошли с другим email")
			}
		}
	case "member.role", "member.revoked", "invite.revoked":
		if !ValidID(in.Target) || in.Action == "member.role" && in.Operator == nil {
			return nil, auth.Failure(422, "VALIDATION_ERROR", "Проверьте участника и роль")
		}
		var changed bool
		err = tx.QueryRow(ctx, "SELECT platform.team_change($1,$2,$3,$4)", org, in.Target, in.Action, in.Operator != nil && *in.Operator).Scan(&changed)
		if err == nil && !changed {
			return nil, auth.Failure(404, "NOT_FOUND", "Участник или приглашение недоступны")
		}
	default:
		return nil, auth.Failure(422, "VALIDATION_ERROR", "Неизвестное действие")
	}
	if err != nil {
		return nil, teamError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, teamError(err)
	}
	return result, nil
}
