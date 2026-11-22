package costcontrol

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
)

type Service struct {
	Pool               *pgxpool.Pool
	Config             config.Agent
	TelegramAuthorized bool
}
type Settings struct {
	TelegramSendingEnabled bool  `json:"telegram_sending_enabled"`
	AutoEnabled            bool  `json:"auto_enabled"`
	LiveEnabled            bool  `json:"live_enabled"`
	RequestLimit           int64 `json:"request_limit_microrub"`
	DialogLimit            int64 `json:"dialog_limit_microrub"`
	DayLimit               int64 `json:"day_limit_microrub"`
	MonthLimit             int64 `json:"month_limit_microrub"`
	Version                int64 `json:"version"`
}
type Change struct {
	Settings
	ExpectedVersion int64 `json:"expected_version"`
}

func Estimate(in, out int, rateIn, rateOut int64) (int64, error) {
	if in < 0 || out < 0 || in > 96000 || out > 96000 || rateIn <= 0 || rateOut <= 0 || rateIn > 1000000000000 || rateOut > 1000000000000 {
		return 0, errors.New("invalid cost inputs")
	}
	n := int64(in)*rateIn + int64(out)*rateOut
	return (n + 999999) / 1000000, nil
}
func (s Service) Read(ctx context.Context, t catalog.Tenant) (Settings, error) {
	tx, e := tenancy.Begin(ctx, s.Pool, t, true, true)
	if e != nil {
		return Settings{}, e
	}
	defer tx.Rollback(ctx)
	var b []byte
	e = tx.QueryRow(ctx, `SELECT coalesce((SELECT jsonb_build_object('telegram_sending_enabled',telegram_sending_enabled,'auto_enabled',auto_enabled,'live_enabled',live_enabled,'request_limit_microrub',request_limit,'dialog_limit_microrub',dialog_limit,'day_limit_microrub',day_limit,'month_limit_microrub',month_limit,'version',version) FROM platform.ai_settings WHERE organization_id=$1),'{"auto_enabled":true,"live_enabled":false,"request_limit_microrub":0,"dialog_limit_microrub":0,"day_limit_microrub":0,"month_limit_microrub":0,"version":1}'::jsonb)`, t.OrganizationID).Scan(&b)
	var c Settings
	if e == nil {
		e = json.Unmarshal(b, &c)
	}
	if e != nil {
		return c, tenancy.Error(e)
	}
	return c, nil
}
func (s Service) Update(ctx context.Context, t catalog.Tenant, c Change) (Settings, error) {
	if c.TelegramSendingEnabled && !s.TelegramAuthorized {
		return Settings{}, auth.Failure(403, "TELEGRAM_SENDING_NOT_AUTHORIZED", "Live Telegram sending currently NOT AUTHORIZED")
	}
	if c.LiveEnabled && (s.Config.Provider == "mock" || !s.Config.LiveEnabled || !s.Config.Live.Ready()) {
		return Settings{}, auth.Failure(403, "LIVE_NOT_AUTHORIZED", "Live LLM не согласован; бюджет платформы по умолчанию 0 RUB")
	}
	if c.Version != 0 {
		return Settings{}, auth.Failure(422, "VALIDATION_ERROR", "Используйте expected_version")
	}
	tx, e := tenancy.Begin(ctx, s.Pool, t, true, true)
	if e != nil {
		return Settings{}, e
	}
	defer tx.Rollback(ctx)
	_, e = tx.Exec(ctx, `SELECT platform.ai_set_settings($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, t.OrganizationID, c.RequestLimit, c.DialogLimit, c.DayLimit, c.MonthLimit, c.LiveEnabled, c.AutoEnabled, c.TelegramSendingEnabled, c.ExpectedVersion, auth.UUID())
	if e != nil {
		var state interface{ SQLState() string }
		if errors.As(e, &state) && state.SQLState() == "40001" {
			return Settings{}, auth.Failure(409, "VERSION_CONFLICT", "Настройки изменились")
		}
		return Settings{}, tenancy.Error(e)
	}
	if e = tx.Commit(ctx); e != nil {
		return Settings{}, tenancy.Error(e)
	}
	return s.Read(ctx, t)
}
func (s Service) Reserve(ctx context.Context, t catalog.Tenant, conv string, in, out, attempts int) (string, error) {
	if !s.Config.LiveEnabled || !s.Config.Live.Ready() {
		return "", auth.Failure(403, "LIVE_BUDGET_BLOCKED", "Live LLM отключён или бюджет равен 0")
	}
	amount, e := Estimate(in*attempts, out*attempts, s.Config.Live.InputRate, s.Config.Live.OutputRate)
	if e != nil {
		return "", e
	}
	tx, e := tenancy.Begin(ctx, s.Pool, t, true, true)
	if e != nil {
		return "", e
	}
	defer tx.Rollback(ctx)
	id := auth.UUID()
	c := s.Config.Live
	_, e = tx.Exec(ctx, `SELECT platform.llm_reserve($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, t.OrganizationID, conv, id, amount, in, out, attempts, c.InputRate, c.OutputRate, s.Config.Model, c.PlatformMicroRUB, c.Parallel, c.DialogInput, c.DialogOutput, c.DialogRequests)
	if e != nil {
		var state interface{ SQLState() string }
		if errors.As(e, &state) && state.SQLState() == "23514" {
			return "", auth.Failure(429, "LLM_BUDGET_EXHAUSTED", "Бюджет или лимит параллельности исчерпан")
		}
		return "", tenancy.Error(e)
	}
	if e = tx.Commit(ctx); e != nil {
		return "", tenancy.Error(e)
	}
	return id, nil
}
func (s Service) Settle(ctx context.Context, t catalog.Tenant, id string, in, out int, known bool) error {
	tx, e := tenancy.Begin(ctx, s.Pool, t, true, true)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	_, e = tx.Exec(ctx, `SELECT platform.llm_settle($1,$2,$3,$4,$5)`, t.OrganizationID, id, in, out, known)
	if e != nil {
		return tenancy.Error(e)
	}
	return tx.Commit(ctx)
}
func (s Service) State(ctx context.Context, t catalog.Tenant) (json.RawMessage, error) {
	c, e := s.Read(ctx, t)
	if e != nil {
		return nil, e
	}
	tx, e := tenancy.Begin(ctx, s.Pool, t, true, true)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	var b []byte
	e = tx.QueryRow(ctx, `SELECT jsonb_build_object('platform',platform.llm_platform_state(),'estimated_live_spent_microrub',coalesce((SELECT sum(charged) FROM platform.llm_reservations WHERE organization_id=$1),0),'reserved_live_microrub',coalesce((SELECT sum(amount) FROM platform.llm_reservations WHERE organization_id=$1 AND status='pending'),0),'metrics',coalesce((SELECT jsonb_agg(to_jsonb(m)) FROM platform.agent_metrics m WHERE organization_id=$1),'[]'::jsonb),'conversations',(SELECT count(*) FROM platform.conversations WHERE organization_id=$1),'inbound_messages',(SELECT count(*) FROM platform.telegram_inbox WHERE organization_id=$1),'orders',(SELECT count(*) FROM platform.orders WHERE organization_id=$1),'handoff',(SELECT count(*) FROM platform.conversation_events WHERE organization_id=$1 AND kind='handoff'),'telegram_errors',(SELECT count(*) FROM platform.telegram_outbox WHERE organization_id=$1 AND last_error IS NOT NULL),'inbox_pending',(SELECT count(*) FROM platform.telegram_inbox WHERE organization_id=$1 AND status IN ('pending','processing')),'outbox_pending',(SELECT count(*) FROM platform.telegram_outbox WHERE organization_id=$1 AND status IN ('pending','sending')),'retry_messages',(SELECT count(*) FROM platform.telegram_outbox WHERE organization_id=$1 AND status='pending' AND attempts>0),'inbox_lag_seconds',coalesce((SELECT extract(epoch FROM now()-min(created_at)) FROM platform.telegram_inbox WHERE organization_id=$1 AND status IN ('pending','processing')),0),'outbox_lag_seconds',coalesce((SELECT extract(epoch FROM now()-min(created_at)) FROM platform.telegram_outbox WHERE organization_id=$1 AND status='pending'),0))`, t.OrganizationID).Scan(&b)
	if e != nil {
		return nil, tenancy.Error(e)
	}
	var state map[string]any
	_ = json.Unmarshal(b, &state)
	state["settings"] = c
	state["mode"] = s.Config.Provider
	state["model"] = s.Config.Model
	state["provider_selected"] = s.Config.Provider == "openai_compatible"
	state["live_config_ready"] = s.Config.LiveEnabled && s.Config.Live.Ready()
	state["currency"] = "RUB"
	state["cost_unit"] = "microRUB"
	state["live_e2e"] = "NOT RUN"
	state["instructions_version"] = "electronics-v1"
	state["limits"] = map[string]int{"dialog_input_tokens": s.Config.Live.DialogInput, "dialog_output_tokens": s.Config.Live.DialogOutput, "dialog_requests": s.Config.Live.DialogRequests, "tool_calls": s.Config.MaxCalls, "parallel": s.Config.Live.Parallel}
	return json.Marshal(state)
}
func (s Service) Purge(ctx context.Context, t catalog.Tenant, id string, version int64) (json.RawMessage, error) {
	tx, e := tenancy.Begin(ctx, s.Pool, t, true, true)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	var b []byte
	e = tx.QueryRow(ctx, `SELECT platform.purge_test_conversation($1,$2,$3,$4)`, t.OrganizationID, id, version, auth.UUID()).Scan(&b)
	if e != nil {
		var state interface{ SQLState() string }
		if errors.As(e, &state) {
			if state.SQLState() == "40001" {
				return nil, auth.Failure(409, "ACTIVE_OR_CHANGED", "Диалог изменился или активная операция ещё не завершена")
			}
			if state.SQLState() == "P0002" {
				return nil, auth.Failure(404, "NOT_FOUND", "Диалог не найден")
			}
		}
		return nil, tenancy.Error(e)
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, tenancy.Error(e)
	}
	return b, nil
}
