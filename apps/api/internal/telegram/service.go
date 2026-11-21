package telegram

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/conversations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
	"log/slog"
	"time"
)

type Service struct {
	Log       *slog.Logger
	Pool      *pgxpool.Pool
	Config    config.Telegram
	Transport Transport
	Chat      *conversations.Service
}

func (s Service) begin(ctx context.Context, t catalog.Tenant, owner bool) (pgx.Tx, error) {
	tx, e := tenancy.Begin(ctx, s.Pool, t, true, true)
	if e != nil {
		return nil, e
	}
	if owner {
		var ok bool
		e = tx.QueryRow(ctx, `SELECT platform.has_membership($1,true)`, t.OrganizationID).Scan(&ok)
		if e != nil || !ok {
			_ = tx.Rollback(ctx)
			return nil, auth.Failure(403, "FORBIDDEN", "Только владелец управляет интеграцией")
		}
	}
	return tx, nil
}
func (s Service) Status(ctx context.Context, t catalog.Tenant) (json.RawMessage, error) {
	tx, e := s.begin(ctx, t, false)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	var b []byte
	e = tx.QueryRow(ctx, `SELECT jsonb_build_object('enabled',$1::boolean,'mode',$3::text,'integration',(SELECT jsonb_build_object('id',i.id,'bot_id',i.bot_id,'name',i.name,'username',i.username,'active',i.active,'webhook',i.webhook_state,'last_error',coalesce(i.last_error,(SELECT last_error FROM platform.telegram_inbox WHERE integration_id=i.id AND last_error IS NOT NULL ORDER BY created_at DESC LIMIT 1)),'pending_inbox',(SELECT count(*) FROM platform.telegram_inbox WHERE integration_id=i.id AND status IN ('pending','processing')),'deliveries',(SELECT coalesce(jsonb_agg(to_jsonb(x)),'[]'::jsonb) FROM (SELECT id,status,last_error,payload,created_at FROM platform.telegram_outbox WHERE integration_id=i.id ORDER BY created_at DESC,id LIMIT 10) x),'failed_outbox',(SELECT count(*) FROM platform.telegram_outbox WHERE integration_id=i.id AND status IN ('failed','uncertain'))) FROM platform.telegram_integrations i WHERE organization_id=$2))`, s.Config.Enabled, t.OrganizationID, s.Config.Mode).Scan(&b)
	return b, tenancyError(e)
}
func tenancyError(e error) error {
	if e == nil {
		return nil
	}
	return tenancy.Error(e)
}
func (s Service) Connect(ctx context.Context, t catalog.Tenant, token string) (json.RawMessage, error) {
	if s.Config.Mode != "mock" {
		return nil, auth.Failure(403, "MANUAL_LIVE_OPERATION_REQUIRED", "Используйте отдельную подтверждённую live-операцию")
	}
	if !s.Config.Enabled {
		return nil, auth.Failure(503, "TELEGRAM_DISABLED", "Настройте серверный ключ шифрования")
	}
	tx, e := s.begin(ctx, t, true)
	if e != nil {
		return nil, e
	}
	_ = tx.Rollback(ctx)
	bot, e := s.Transport.GetMe(ctx, token)
	if e != nil {
		s.recordError(ctx, t, "INVALID_BOT_TOKEN")
		return nil, auth.Failure(422, "INVALID_BOT_TOKEN", "Не удалось проверить синтетического бота")
	}
	cipher, e := encrypt(s.Config.Key, token, t.OrganizationID)
	if e != nil {
		return nil, auth.Unavailable()
	}
	secret := auth.Token()
	tx, e = s.begin(ctx, t, true)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	var old int64
	e = tx.QueryRow(ctx, `SELECT bot_id FROM platform.telegram_integrations WHERE organization_id=$1`, t.OrganizationID).Scan(&old)
	if e == nil && old != bot.ID {
		return nil, auth.Failure(409, "BOT_REPLACEMENT_UNSUPPORTED", "Для организации уже назначен другой бот")
	}
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return nil, tenancy.Error(e)
	}
	_, e = tx.Exec(ctx, `SELECT platform.telegram_connect($1,$2,$3,$4,$5,$6,$7,$8,$9)`, auth.UUID(), t.OrganizationID, bot.ID, bot.Name, bot.Username, cipher, auth.Hash(secret), sha256Sum(s.WorkerProof()), s.Config.Mode)
	if e != nil {
		return nil, tenancy.Error(e)
	}
	if e = s.audit(ctx, tx, t, "connected"); e != nil {
		return nil, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, tenancy.Error(e)
	}
	return s.Status(ctx, t)
}
func (s Service) Disconnect(ctx context.Context, t catalog.Tenant) (json.RawMessage, error) {
	tx, e := s.begin(ctx, t, true)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	_, e = tx.Exec(ctx, `SELECT platform.telegram_disconnect($1)`, t.OrganizationID)
	if e != nil {
		return nil, tenancy.Error(e)
	}
	if e = s.audit(ctx, tx, t, "disconnected"); e != nil {
		return nil, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, tenancy.Error(e)
	}
	return s.Status(ctx, t)
}
func (s Service) Ingress(ctx context.Context, id, secret string, u Update) (bool, error) {
	if !organizations.ValidID(id) {
		return false, auth.Failure(403, "INVALID_WEBHOOK", "Webhook отклонён")
	}
	var expected []byte
	if e := s.Pool.QueryRow(ctx, `SELECT platform.telegram_webhook_hash($1)`, id).Scan(&expected); e != nil {
		if errors.Is(e, pgx.ErrNoRows) {
			return false, auth.Failure(403, "INVALID_WEBHOOK", "Webhook отклонён")
		}
		return false, auth.Unavailable()
	}
	if subtle.ConstantTimeCompare(expected, auth.Hash(secret)) != 1 {
		return false, auth.Failure(403, "INVALID_WEBHOOK", "Webhook отклонён")
	}
	user, chat, e := u.Principal()
	if e != nil {
		return false, auth.Failure(422, "INVALID_UPDATE", "Поддерживаются только личные текстовые диалоги")
	}
	if !s.Config.Allowed(user) {
		return false, auth.Failure(403, "TEST_USER_NOT_ALLOWED", "Отправитель отсутствует в тестовом allowlist")
	}
	raw, _ := json.Marshal(u)
	var inserted bool
	e = s.Pool.QueryRow(ctx, `SELECT platform.telegram_ingress($1,$2,$3,$4,$5,$6,$7)`, id, auth.Hash(secret), auth.UUID(), u.ID, user, chat, raw).Scan(&inserted)
	if e != nil {
		var pe *pgconn.PgError
		if errors.As(e, &pe) {
			if pe.Code == "54000" {
				return false, auth.Failure(429, "RATE_LIMITED", "Повторите позже")
			}
			if pe.Code == "42501" {
				return false, auth.Failure(403, "INVALID_WEBHOOK", "Webhook отклонён")
			}
		}
		return false, auth.Unavailable()
	}
	return inserted, nil
}

// Inject is owner-authenticated and remains disabled in production by configuration.
func (s Service) Inject(ctx context.Context, t catalog.Tenant, u Update) (bool, error) {
	user, chat, e := u.Principal()
	if e != nil {
		return false, auth.Failure(422, "INVALID_UPDATE", "Некорректное синтетическое сообщение")
	}
	tx, e := s.begin(ctx, t, true)
	if e != nil {
		return false, e
	}
	defer tx.Rollback(ctx)
	if !s.Config.Allowed(user) {
		return false, auth.Failure(403, "TEST_USER_NOT_ALLOWED", "Отправитель отсутствует в тестовом allowlist")
	}
	raw, _ := json.Marshal(u)
	var id string
	e = tx.QueryRow(ctx, `SELECT id FROM platform.telegram_integrations WHERE active AND organization_id=$1`, t.OrganizationID).Scan(&id)
	if e != nil {
		return false, tenancy.Error(e)
	}
	tag, e := tx.Exec(ctx, `INSERT INTO platform.telegram_inbox(id,organization_id,integration_id,update_id,user_id,chat_id,payload) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(integration_id,update_id) DO NOTHING`, auth.UUID(), t.OrganizationID, id, u.ID, user, chat, raw)
	if e != nil {
		return false, tenancy.Error(e)
	}
	if e = tx.Commit(ctx); e != nil {
		return false, tenancy.Error(e)
	}
	return tag.RowsAffected() == 1, nil
}
func (s Service) queue(ctx context.Context, t catalog.Tenant, conv, integration string, chat int64, key string, payload any) error {
	raw, _ := json.Marshal(payload)
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	var text string
	_ = json.Unmarshal(fields["text"], &text)
	order := fields["order_id"]
	delete(fields, "order_id")
	data := map[string]any{"telegram_payload": fields, "mode": "mock"}
	if len(order) > 0 {
		data["order_id"] = order
	}
	return s.record(ctx, t, conv, stableID(key+"/reply"), "assistant", text, data)
}
func stableID(key string) string {
	b := auth.Hash(key)[:16]
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	h := hex.EncodeToString(b)
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}
func (s Service) record(ctx context.Context, t catalog.Tenant, conv, key, role, text string, data any) error {
	tx, e := tenancy.Begin(ctx, s.Pool, t, true, true)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var count int
	var mode, status string
	var expiry time.Time
	if e = tx.QueryRow(ctx, `SELECT message_count,control_mode,status,expires_at FROM platform.conversations WHERE id=$1 FOR UPDATE`, conv).Scan(&count, &mode, &status, &expiry); e != nil {
		return tenancy.Error(e)
	}
	if status != "open" || !expiry.After(time.Now()) {
		return auth.Failure(409, "CONVERSATION_CLOSED", "Диалог закрыт")
	}
	if role == "assistant" && mode != "agent" {
		return nil
	} // A handoff raced a callback: discard late automation.

	var exists bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM platform.messages WHERE conversation_id=$1 AND client_key=$2)`, conv, key).Scan(&exists); e != nil {
		return tenancy.Error(e)
	}
	if exists {
		return nil
	}
	if count >= 100 {
		return auth.Failure(429, "CONVERSATION_LIMIT", "Диалог достиг лимита сообщений")
	}
	raw, _ := json.Marshal(data)
	_, e = tx.Exec(ctx, `INSERT INTO platform.messages(id,organization_id,conversation_id,role,content,data,client_key) VALUES($1,$2,$3,$4,$5,$6,$1)`, key, t.OrganizationID, conv, role, text, raw)
	if e != nil {
		return tenancy.Error(e)
	}
	_, e = tx.Exec(ctx, `UPDATE platform.conversations SET message_count=message_count+1,version=version+1,updated_at=now() WHERE id=$1`, conv)
	if e != nil {
		return tenancy.Error(e)
	}
	return tx.Commit(ctx)
}
func decodeCipher(s string) ([]byte, error) { return hex.DecodeString(s) }

func (s Service) audit(ctx context.Context, tx pgx.Tx, t catalog.Tenant, kind string) error {
	_, e := tx.Exec(ctx, `INSERT INTO platform.telegram_events(id,organization_id,integration_id,actor_id,kind) SELECT $1,$2,id,$3,$4 FROM platform.telegram_integrations WHERE organization_id=$2`, auth.UUID(), t.OrganizationID, t.Session.User.ID, kind)
	return tenancyError(e)
}
func (s Service) Check(ctx context.Context, t catalog.Tenant) (json.RawMessage, error) {
	if s.Config.Mode != "mock" {
		return nil, auth.Failure(403, "MANUAL_LIVE_OPERATION_REQUIRED", "Проверка только явной live-операцией")
	}
	tx, e := s.begin(ctx, t, true)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	var cipher []byte
	e = tx.QueryRow(ctx, `SELECT platform.telegram_owner_cipher($1)`, t.OrganizationID).Scan(&cipher)
	if e != nil || len(cipher) == 0 {
		return nil, auth.Failure(409, "INTEGRATION_INACTIVE", "Нет активной интеграции")
	}
	token, e := decrypt(s.Config.Key, cipher, t.OrganizationID)
	if e != nil {
		return nil, auth.Unavailable()
	}
	if _, e = s.Transport.GetMe(ctx, token); e != nil {
		_, _ = tx.Exec(ctx, `SELECT platform.telegram_connection_error($1,$2)`, t.OrganizationID, "BOT_CHECK_FAILED")
		_ = tx.Commit(ctx)
		return nil, auth.Failure(503, "BOT_CHECK_FAILED", "Проверка недоступна")
	}
	if _, e = tx.Exec(ctx, `SELECT platform.telegram_connection_error($1,NULL)`, t.OrganizationID); e != nil {
		return nil, tenancy.Error(e)
	}
	if e = s.audit(ctx, tx, t, "checked"); e != nil {
		return nil, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, tenancy.Error(e)
	}
	return s.Status(ctx, t)
}

func (s Service) recordError(ctx context.Context, t catalog.Tenant, code string) {
	tx, e := s.begin(ctx, t, true)
	if e != nil {
		return
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT platform.telegram_connection_error($1,$2)`, t.OrganizationID, code); e == nil {
		_ = tx.Commit(ctx)
	}
}

// WorkerProof is only for the internal queue boundary, never HTTP/model output.
func (s Service) WorkerProof() []byte {
	h := hmac.New(sha256.New, s.Config.Key)
	_, _ = h.Write([]byte("aba:telegram:queue-worker:v1"))
	return h.Sum(nil)
}
func sha256Sum(b []byte) []byte { h := sha256.Sum256(b); return h[:] }
