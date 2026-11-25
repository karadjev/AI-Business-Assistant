package conversations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/agent"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/costcontrol"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/orders"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
	"log/slog"
	"strings"
	"time"
)

type Service struct {
	Pool              *pgxpool.Pool
	Log               *slog.Logger
	Config            config.Agent
	Provider          agent.Provider
	RoutingWorkerHash []byte
}
type SendInput struct {
	Text            string          `json:"text"`
	Role            string          `json:"role"`
	IdempotencyKey  string          `json:"idempotency_key"`
	ChannelMetadata json.RawMessage `json:"-"`
	RouteInbound    bool            `json:"-"`
}

func (s Service) begin(ctx context.Context, t catalog.Tenant) (pgx.Tx, error) {
	return tenancy.Begin(ctx, s.Pool, t, true, true)
}
func readJSON(ctx context.Context, tx pgx.Tx, sql string, args ...any) (json.RawMessage, error) {
	var raw []byte
	if e := tx.QueryRow(ctx, sql, args...).Scan(&raw); e != nil {
		return nil, tenancy.Error(e)
	}
	return json.RawMessage(raw), nil
}
func (s Service) detail(ctx context.Context, tx pgx.Tx, id string) (json.RawMessage, error) {
	return readJSON(ctx, tx, `SELECT (to_jsonb(c)||jsonb_build_object('mode',$2::text,'model',$3::text,'channel_delivery',(SELECT coalesce(jsonb_agg(to_jsonb(d)),'[]'::jsonb) FROM (SELECT id,status,provider_id,created_at FROM platform.channel_outbox WHERE conversation_id=c.id ORDER BY created_at DESC LIMIT 20)d),'channel_window_until',(SELECT window_until FROM platform.channel_principals WHERE conversation_id=c.id),'messages',(SELECT coalesce(jsonb_agg(to_jsonb(x)-'request_hash' ORDER BY x.created_at,x.id),'[]'::jsonb) FROM (SELECT * FROM platform.messages WHERE conversation_id=c.id ORDER BY created_at DESC,id LIMIT 40) x),'events',(SELECT coalesce(jsonb_agg(to_jsonb(e) ORDER BY e.created_at,e.id),'[]'::jsonb) FROM platform.conversation_events e WHERE conversation_id=c.id)))-'in_flight'-'lease_until'-'assignee_id' FROM platform.conversations c WHERE c.id=$1`, id, s.Provider.Mode(), s.Config.Model)
}
func (s Service) Create(ctx context.Context, t catalog.Tenant) (json.RawMessage, error) {
	tx, e := s.begin(ctx, t)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	var count int
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM platform.conversations WHERE status='open' AND expires_at>now()`).Scan(&count); e != nil {
		return nil, tenancy.Error(e)
	}
	if count >= 20 {
		return nil, tenancy.Failure(429, "RATE_LIMITED", "Максимум 20 открытых тестовых диалогов")
	}
	id := auth.UUID()
	_, e = tx.Exec(ctx, `INSERT INTO platform.conversations(id,organization_id,customer_id,created_by) VALUES($1,$2,$3,$4)`, id, t.OrganizationID, auth.UUID(), actor(t))
	if e != nil {
		return nil, tenancy.Error(e)
	}
	if e = event(ctx, tx, t, id, "created"); e != nil {
		return nil, e
	}
	result, e := s.detail(ctx, tx, id)
	if e != nil {
		return nil, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, tenancy.Error(e)
	}
	return result, nil
}
func (s Service) Get(ctx context.Context, t catalog.Tenant, id string) (json.RawMessage, error) {
	if !organizations.ValidID(id) {
		return nil, tenancy.Failure(404, "NOT_FOUND", "Диалог не найден")
	}
	tx, e := s.begin(ctx, t)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	return s.detail(ctx, tx, id)
}
func (s Service) List(ctx context.Context, t catalog.Tenant, page int) (json.RawMessage, error) {
	return s.ListChannel(ctx, t, page, "")
}
func (s Service) ListChannel(ctx context.Context, t catalog.Tenant, page int, channel string) (json.RawMessage, error) {
	if channel != "" && channel != "telegram" && channel != "whatsapp" && channel != "instagram" && channel != "internal_chat" {
		return nil, tenancy.Failure(422, "VALIDATION_ERROR", "Неизвестный канал")
	}

	if page < 1 || page > 10000 {
		return nil, tenancy.Failure(422, "VALIDATION_ERROR", "Некорректная страница")
	}
	tx, e := s.begin(ctx, t)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	return readJSON(ctx, tx, `SELECT jsonb_build_object('items',coalesce(jsonb_agg(to_jsonb(x)-'in_flight'-'lease_until'-'assignee_id'),'[]'::jsonb)) FROM (SELECT * FROM platform.conversations WHERE expires_at>now() AND ($2='' OR channel=$2) ORDER BY updated_at DESC,id LIMIT 20 OFFSET $1) x`, (page-1)*20, channel)
}
func event(ctx context.Context, tx pgx.Tx, t catalog.Tenant, id, kind string) error {
	_, e := tx.Exec(ctx, `INSERT INTO platform.conversation_events(id,organization_id,conversation_id,actor_id,kind) VALUES($1,$2,$3,$4,$5)`, auth.UUID(), t.OrganizationID, id, actor(t), kind)
	if e != nil {
		return tenancy.Error(e)
	}
	return nil
}
func message(ctx context.Context, tx pgx.Tx, t catalog.Tenant, id, role, text, key string, hash []byte, data any) error {
	raw, _ := json.Marshal(data)
	if data == nil {
		raw = []byte(`{}`)
	}
	_, e := tx.Exec(ctx, `INSERT INTO platform.messages(id,organization_id,conversation_id,role,content,data,client_key,request_hash,actor_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,CASE WHEN $4='staff' THEN $9::uuid ELSE NULL END)`, auth.UUID(), t.OrganizationID, id, role, text, raw, key, hash, actor(t))
	if e != nil {
		return tenancy.Error(e)
	}
	return nil
}
func (s Service) Control(ctx context.Context, t catalog.Tenant, id, mode string, version int64) (json.RawMessage, error) {
	if t.ChannelCapability != "" && mode != "human" {
		return nil, tenancy.Failure(403, "FORBIDDEN", "Channel cannot resume or erase conversations")
	}
	if !organizations.ValidID(id) || (mode != "human" && mode != "agent" && mode != "delete") {
		return nil, tenancy.Failure(422, "VALIDATION_ERROR", "Некорректная команда")
	}
	tx, e := s.begin(ctx, t)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	var current, status string
	var v int64
	e = tx.QueryRow(ctx, `SELECT control_mode,status,version FROM platform.conversations WHERE id=$1 FOR UPDATE`, id).Scan(&current, &status, &v)
	if e != nil {
		return nil, tenancy.Error(e)
	}
	if version > 0 && version != v {
		return nil, tenancy.Failure(409, "VERSION_CONFLICT", "Диалог изменился")
	}
	if status == "closed" && mode != "delete" {
		return nil, tenancy.Failure(409, "CONVERSATION_CLOSED", "Диалог закрыт")
	}
	kind := "handoff"
	if mode == "agent" {
		kind = "resumed"
	}
	if mode == "delete" {
		kind = "deleted"
		mode = "human"
		_, e = tx.Exec(ctx, `UPDATE platform.messages SET content='',data='{}',request_hash=NULL WHERE conversation_id=$1`, id)
		if e != nil {
			return nil, tenancy.Error(e)
		}
		status = "closed"
	}
	if current == mode && status != "closed" {
		return s.detail(ctx, tx, id)
	}
	_, e = tx.Exec(ctx, `UPDATE platform.conversations SET control_mode=$2,status=$3,version=version+1,in_flight=NULL,lease_until=NULL,updated_at=now() WHERE id=$1`, id, mode, status)
	if e != nil {
		return nil, tenancy.Error(e)
	}
	if e = event(ctx, tx, t, id, kind); e != nil {
		return nil, e
	}
	result, e := s.detail(ctx, tx, id)
	if e != nil {
		return nil, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, tenancy.Error(e)
	}
	return result, nil
}
func (s Service) State(ctx context.Context, t catalog.Tenant) (map[string]any, error) {
	tx, e := s.begin(ctx, t)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	return map[string]any{"mode": s.Provider.Mode(), "provider": s.Config.Provider, "model": s.Config.Model, "max_tool_calls": s.Config.MaxCalls, "max_iterations": 2, "input_tokens": s.Config.InputTokens, "output_tokens": s.Config.OutputTokens, "daily_requests": s.Config.DailyRequests, "daily_tokens": s.Config.DailyTokens, "quote_ttl_seconds": 300, "inventory_freshness_seconds": int(s.Config.StockFreshness.Seconds()), "retention_days": 7}, nil
}
func (s Service) Send(ctx context.Context, t catalog.Tenant, id string, in SendInput) (json.RawMessage, error) {
	if in.Role == "" {
		in.Role = "customer"
	}
	in.Text = strings.TrimSpace(in.Text)
	if !organizations.ValidID(id) || !organizations.ValidID(in.IdempotencyKey) || (in.Role != "customer" && in.Role != "staff") || len([]rune(in.Text)) < 1 || len([]rune(in.Text)) > 2000 || strings.ContainsRune(in.Text, 0) {
		return nil, tenancy.Failure(422, "VALIDATION_ERROR", "Сообщение 1–2000 символов, корректный ключ и роль")
	}
	tx, e := s.begin(ctx, t)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	if in.Role == "staff" {
		var closedWindow bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM platform.channel_principals p WHERE p.conversation_id=$1 AND p.window_until<=clock_timestamp())`, id).Scan(&closedWindow); e != nil {
			return nil, tenancy.Error(e)
		}
		if closedWindow {
			var provider string
			if e = tx.QueryRow(ctx, `SELECT channel FROM platform.conversations WHERE id=$1`, id).Scan(&provider); e != nil {
				return nil, tenancy.Error(e)
			}
			if provider == "instagram" {
				return nil, tenancy.Failure(409, "INSTAGRAM_MESSAGING_WINDOW_CLOSED", "Окно Instagram 24 часа закрыто. Автоматические исключения не применяются.")
			}
			return nil, tenancy.Failure(409, "NEEDS_APPROVED_TEMPLATE", "Окно WhatsApp закрыто. Свободный ответ запрещён; шаблоны не отправляются.")
		}
	}
	if t.ChannelCapability == "" && in.Role == "customer" {
		var channel string
		if e = tx.QueryRow(ctx, `SELECT channel FROM platform.conversations WHERE id=$1`, id).Scan(&channel); e != nil {
			return nil, tenancy.Error(e)
		}
		if channel != "internal_chat" {
			return nil, tenancy.Failure(403, "CUSTOMER_PRINCIPAL_REQUIRED", "Telegram customer messages arrive only through inbox")
		}
	}
	var mode, status string
	var version int64
	var expiry time.Time
	var count int
	var lease *time.Time
	e = tx.QueryRow(ctx, `SELECT control_mode,status,version,expires_at,message_count,lease_until FROM platform.conversations WHERE id=$1 FOR UPDATE`, id).Scan(&mode, &status, &version, &expiry, &count, &lease)
	if e != nil {
		return nil, tenancy.Error(e)
	}
	fingerprint := auth.Hash(in.Role + ":" + in.Text)
	var old []byte
	e = tx.QueryRow(ctx, `SELECT request_hash FROM platform.messages WHERE conversation_id=$1 AND client_key=$2`, id, in.IdempotencyKey).Scan(&old)
	if e == nil {
		if string(old) != string(fingerprint) {
			return nil, tenancy.Failure(409, "IDEMPOTENCY_CONFLICT", "Ключ связан с другим сообщением")
		}
		// A committed customer message is not proof that its interrupted run finished.
		// Never rerun model side effects blindly after a worker crash: fence and hand off.
		if mode == "agent" && lease != nil && lease.After(time.Now()) {
			return nil, tenancy.Failure(409, "CONVERSATION_BUSY", "Ответ ещё обрабатывается; повторите позже")
		}
		if mode == "agent" && lease != nil && !lease.After(time.Now()) {
			_, e = tx.Exec(ctx, `UPDATE platform.conversations SET control_mode='human',in_flight=NULL,lease_until=NULL,version=version+1 WHERE id=$1`, id)
			if e != nil {
				return nil, tenancy.Error(e)
			}
			if e = event(ctx, tx, t, id, "provider_error"); e != nil {
				return nil, e
			}
			notice := "Предыдущий ответ прерван. Диалог передан сотруднику; автоматические ответы остановлены."
			if s.Provider.Mode() == "mock" {
				notice = "[MOCK — тестовый сценарий] " + notice
			}
			if e = message(ctx, tx, t, id, "assistant", notice, auth.UUID(), nil, map[string]any{"mode": s.Provider.Mode(), "handoff_notice": true}); e != nil {
				return nil, e
			}
			if e = tx.Commit(ctx); e != nil {
				return nil, tenancy.Error(e)
			}
			return s.Get(ctx, t, id)
		}
		return s.detail(ctx, tx, id)
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return nil, tenancy.Error(e)
	}
	if status != "open" || !expiry.After(time.Now()) {
		return nil, tenancy.Failure(409, "CONVERSATION_CLOSED", "Диалог закрыт или срок хранения истёк")
	}
	if lease != nil && lease.After(time.Now()) && mode == "agent" {
		return nil, tenancy.Failure(409, "CONVERSATION_BUSY", "Предыдущее сообщение ещё обрабатывается")
	}
	if count >= 100 {
		return nil, tenancy.Failure(429, "RATE_LIMITED", "Лимит 100 сообщений: создайте новый тестовый диалог")
	}
	if in.Role == "staff" && mode != "human" {
		return nil, tenancy.Failure(409, "INVALID_STATE", "Сначала передайте диалог сотруднику")
	}
	var channelData any
	if t.ChannelCapability != "" && len(in.ChannelMetadata) > 0 {
		channelData = in.ChannelMetadata
	}
	if e = message(ctx, tx, t, id, in.Role, in.Text, in.IdempotencyKey, fingerprint, channelData); e != nil {
		return nil, e
	}
	runID := auth.UUID()
	bookingPreference := t.ChannelCapability != "" && (in.Text == "booking_updates:on" || in.Text == "booking_updates:off")
	routedInbound := len(s.RoutingWorkerHash) == 32 && t.ChannelCapability != "" && in.RouteInbound &&
		in.Role == "customer" && !bookingPreference
	if routedInbound && mode == "agent" {
		// The stored message and its durable task commit together. The database
		// independently verifies the inbox capability, customer and policy.
		if _, e = tx.Exec(ctx, `SELECT platform.conversation_route_submit($1,$2,$3,$4)`,
			t.OrganizationID, id, in.IdempotencyKey, s.RoutingWorkerHash); e != nil {
			return nil, tenancy.Error(e)
		}
	}
	auto := mode == "agent" && in.Role == "customer" && !bookingPreference && !routedInbound
	var autoEnabled bool
	if e = tx.QueryRow(ctx, `SELECT coalesce((SELECT auto_enabled FROM platform.ai_settings WHERE organization_id=$1),true)`, t.OrganizationID).Scan(&autoEnabled); e != nil {
		return nil, tenancy.Error(e)
	}
	if auto && !autoEnabled {
		auto = false
		mode = "human"
	}
	reservation := 2 * (s.Config.InputTokens + s.Config.OutputTokens)
	budgetOK := true
	usageDay := ""
	if auto {
		if reservation > s.Config.DailyTokens {
			budgetOK = false
			mode = "human"
			auto = false
		} else {
			var n int
			e = tx.QueryRow(ctx, `INSERT INTO platform.ai_usage(organization_id,day,requests,reserved_tokens) VALUES($1,current_date,1,$2) ON CONFLICT(organization_id,day) DO UPDATE SET requests=ai_usage.requests+1,reserved_tokens=ai_usage.reserved_tokens+$2 WHERE ai_usage.requests<$3 AND ai_usage.used_tokens+ai_usage.reserved_tokens+$2<=$4 RETURNING requests,day::text`, t.OrganizationID, reservation, s.Config.DailyRequests, s.Config.DailyTokens).Scan(&n, &usageDay)
			if errors.Is(e, pgx.ErrNoRows) {
				budgetOK = false
				mode = "human"
				auto = false
			} else if e != nil {
				return nil, tenancy.Error(e)
			}
		}
	}

	_, e = tx.Exec(ctx, `UPDATE platform.conversations SET version=version+1,control_mode=$2,message_count=message_count+1,updated_at=now(),in_flight=CASE WHEN $3 THEN $4::uuid ELSE NULL END,lease_until=CASE WHEN $3 THEN now()+interval '10 seconds' ELSE NULL END WHERE id=$1`, id, mode, auto, runID)
	if e != nil {
		return nil, tenancy.Error(e)
	}
	version++
	if !budgetOK {
		if e = event(ctx, tx, t, id, "limit"); e != nil {
			return nil, e
		}
		if e = message(ctx, tx, t, id, "system", "Лимит агента достигнут. Диалог передан сотруднику.", auth.UUID(), nil, nil); e != nil {
			return nil, e
		}
	}
	history := []agent.Message{}
	if auto {
		rows, e := tx.Query(ctx, `SELECT role,content FROM (SELECT role,content,created_at,id FROM platform.messages WHERE conversation_id=$1 AND role IN ('customer','assistant') ORDER BY created_at DESC,id LIMIT 10) x ORDER BY created_at,id`, id)
		if e != nil {
			return nil, tenancy.Error(e)
		}
		for rows.Next() {
			var m agent.Message
			if e = rows.Scan(&m.Role, &m.Content); e != nil {
				rows.Close()
				return nil, tenancy.Error(e)
			}
			if m.Role == "customer" {
				m.Role = "user"
			}
			history = append(history, m)
		}
		rows.Close()
		if rows.Err() != nil {
			return nil, tenancy.Error(rows.Err())
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, tenancy.Error(e)
	}
	if !auto {
		return s.Get(ctx, t, id)
	}
	// No database transaction is held during network/model calls.
	runner := agent.Runner{Catalog: catalog.Service{Pool: s.Pool}, Orders: orders.Service{Pool: s.Pool, StockFreshness: s.Config.StockFreshness}, Tenant: t, ConversationID: id, Freshness: s.Config.StockFreshness, Handoff: func(ctx context.Context, _ string) error { _, e := s.Control(ctx, t, id, "human", 0); return e }}
	modelCtx, cancel := context.WithTimeout(ctx, s.Config.Timeout)
	started := time.Now()
	if s.Log != nil {
		s.Log.Info("agent_cycle_started", "correlation_id", runID, "mode", s.Provider.Mode())
	}
	provider := agent.Metered{Provider: s.Provider, Ledger: costcontrol.Service{Pool: s.Pool, Config: s.Config}, Tenant: t, ConversationID: id}
	outcome, runError := agent.Run(modelCtx, provider, s.Config, history, &runner)
	elapsed := time.Since(started).Milliseconds()
	if s.Log != nil {
		s.Log.Info("agent_cycle_finished", "correlation_id", runID, "mode", s.Provider.Mode(), "duration_ms", elapsed, "tool_calls", outcome.ToolCalls, "success", runError == nil)
	}
	cancel()
	// Independent bounded finalization permits safe handoff even if caller cancelled.
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer finishCancel()
	tx, e = s.begin(finishCtx, t)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(finishCtx)
	_, e = tx.Exec(finishCtx, `INSERT INTO platform.agent_metrics(organization_id,mode,cycles,successes,errors,tool_calls,total_latency_ms) VALUES($1,$2,1,$3,$4,$5,$6) ON CONFLICT(organization_id,mode) DO UPDATE SET cycles=agent_metrics.cycles+1,successes=agent_metrics.successes+$3,errors=agent_metrics.errors+$4,tool_calls=agent_metrics.tool_calls+$5,total_latency_ms=agent_metrics.total_latency_ms+$6`, t.OrganizationID, s.Provider.Mode(), boolInt(runError == nil), boolInt(runError != nil), outcome.ToolCalls, elapsed)
	if e != nil {
		return nil, tenancy.Error(e)
	}
	if outcome.UsageKnown && runError == nil {
		_, e = tx.Exec(finishCtx, `UPDATE platform.ai_usage SET reserved_tokens=reserved_tokens-$2,used_tokens=used_tokens+$3 WHERE organization_id=$1 AND day=$4::date AND reserved_tokens>=$2`, t.OrganizationID, reservation, outcome.Tokens, usageDay)
		if e != nil {
			return nil, tenancy.Error(e)
		}
	}
	var actualVersion int64
	var actualMode, statusNow string
	var activeRun *string
	e = tx.QueryRow(finishCtx, `SELECT version,control_mode,status,in_flight::text FROM platform.conversations WHERE id=$1 FOR UPDATE`, id).Scan(&actualVersion, &actualMode, &statusNow, &activeRun)
	if e != nil {
		return nil, tenancy.Error(e)
	}
	if actualVersion != version || actualMode != "agent" || statusNow != "open" || activeRun == nil || *activeRun != runID {
		if e = tx.Commit(finishCtx); e != nil {
			return nil, tenancy.Error(e)
		}
		return s.Get(finishCtx, t, id)
	}
	reply := "Проверил каталог. Выберите вариант и количество, затем подтвердите показанную заявку. До подтверждения сотрудником резерва нет."
	if len(outcome.Cards) == 0 {
		reply = "Достоверных данных для ответа нет. Уточните SKU или обратитесь к сотруднику."
	}
	kind := ""
	if runError != nil || outcome.Handoff {
		actualMode = "human"
		kind = "provider_error"
		reply = "Агент не смог безопасно завершить ответ. Диалог передан сотруднику; автоматические ответы остановлены."
	}
	if s.Provider.Mode() == "mock" {
		reply = "[MOCK — тестовый сценарий, не настоящий интеллект] " + reply
	}
	if e = message(finishCtx, tx, t, id, "assistant", reply, auth.UUID(), nil, map[string]any{"cards": outcome.Cards, "mode": s.Provider.Mode(), "handoff_notice": actualMode == "human"}); e != nil {
		return nil, e
	}
	_, e = tx.Exec(finishCtx, `UPDATE platform.conversations SET control_mode=$2,in_flight=NULL,lease_until=NULL,version=version+1,updated_at=now() WHERE id=$1`, id, actualMode)
	if e != nil {
		return nil, tenancy.Error(e)
	}
	if kind != "" {
		if e = event(finishCtx, tx, t, id, kind); e != nil {
			return nil, e
		}
	}
	result, e := s.detail(finishCtx, tx, id)
	if e != nil {
		return nil, e
	}
	if e = tx.Commit(finishCtx); e != nil {
		return nil, tenancy.Error(e)
	}
	return result, nil
}
func (s Service) SetOrderAccess(ctx context.Context, t catalog.Tenant, userID string, allowed bool) error {
	if !organizations.ValidID(userID) {
		return tenancy.Failure(422, "VALIDATION_ERROR", "Некорректный участник")
	}
	tx, e := tenancy.Begin(ctx, s.Pool, t, false, false)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	_, e = tx.Exec(ctx, `SELECT platform.set_order_access($1,$2,$3)`, t.OrganizationID, userID, allowed)
	if e != nil {
		var pe interface{ SQLState() string }
		if errors.As(e, &pe) && pe.SQLState() == "P0002" {
			return tenancy.Failure(404, "NOT_FOUND", "Участник не найден")
		}
		return tenancy.Error(e)
	}
	if e = tx.Commit(ctx); e != nil {
		return tenancy.Error(e)
	}
	return nil
}

// PurgeExpired erases test message payloads only; order/audit identifiers remain.
// Runs under migration-owner? No: invoked per authenticated test tenant, with RLS.
func (s Service) PurgeExpired(ctx context.Context, t catalog.Tenant) error {
	tx, e := s.begin(ctx, t)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	_, e = tx.Exec(ctx, `UPDATE platform.messages SET content='',data='{}',request_hash=NULL WHERE conversation_id IN (SELECT id FROM platform.conversations WHERE expires_at<=now())`)
	if e != nil {
		return tenancy.Error(e)
	}
	_, e = tx.Exec(ctx, `UPDATE platform.conversations SET status='closed',control_mode='human',in_flight=NULL,lease_until=NULL,version=version+1 WHERE expires_at<=now() AND status<>'closed'`)
	if e != nil {
		return tenancy.Error(e)
	}
	if e = tx.Commit(ctx); e != nil {
		return tenancy.Error(e)
	}
	return nil
}
func (s Service) String() string { return fmt.Sprintf("agent mode=%s", s.Provider.Mode()) }

func actor(t catalog.Tenant) any {
	if t.ChannelCapability != "" {
		return nil
	}
	return t.Session.User.ID
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
