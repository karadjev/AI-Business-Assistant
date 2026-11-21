package telegram

import (
	"context"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
)

type LiveOperation struct {
	Operation string `json:"operation"`
	Confirm   bool   `json:"confirm_external_request"`
	Token     string `json:"token,omitempty"`
}

func (s Service) LiveOperation(ctx context.Context, t catalog.Tenant, in LiveOperation) (map[string]any, error) {
	if !s.Config.ExternalAuthorized || s.Config.Mode != "live" || !in.Confirm {
		return nil, auth.Failure(403, "EXTERNAL_REQUEST_NOT_AUTHORIZED", "Telegram external requests NOT AUTHORIZED; требуется отдельное разрешение и подтверждение")
	}
	if len(in.Token) > 256 || in.Operation == "verify_token" && in.Token == "" {
		return nil, auth.Failure(422, "VALIDATION_ERROR", "Некорректный токен")
	}
	tx, e := s.begin(ctx, t, true)
	if e != nil {
		return nil, e
	}
	_ = tx.Rollback(ctx)
	guarded, ok := s.Transport.(Guarded)
	if !ok {
		return nil, auth.Unavailable()
	}
	transport, ok := guarded.Inner.(*HTTPTransport)
	if !ok {
		return nil, auth.Unavailable()
	}
	ctx = ConfirmedOperation(ctx, true)
	if in.Operation == "verify_token" {
		bot, e := guarded.GetMe(ctx, in.Token)
		if e != nil {
			return nil, auth.Failure(422, "INVALID_BOT_TOKEN", "Не удалось проверить тестового бота")
		}
		cipher, e := encrypt(s.Config.Key, in.Token, t.OrganizationID)
		if e != nil {
			return nil, auth.Unavailable()
		}
		tx, e = s.begin(ctx, t, true)
		if e != nil {
			return nil, e
		}
		defer tx.Rollback(ctx)
		_, e = tx.Exec(ctx, `SELECT platform.telegram_connect($1,$2,$3,$4,$5,$6,$7,$8,'live')`, auth.UUID(), t.OrganizationID, bot.ID, bot.Name, bot.Username, cipher, auth.Hash(auth.Token()), sha256Sum(s.WorkerProof()))
		if e != nil {
			return nil, tenancy.Error(e)
		}
		if e = tx.Commit(ctx); e != nil {
			return nil, tenancy.Error(e)
		}
		return map[string]any{"verified": true, "bot": bot, "webhook_state": "not_set"}, nil
	}
	if in.Token != "" {
		return nil, auth.Failure(422, "VALIDATION_ERROR", "Токен принимается только verify_token")
	}
	tx, e = s.begin(ctx, t, true)
	if e != nil {
		return nil, e
	}
	var encrypted []byte
	var id string
	e = tx.QueryRow(ctx, `SELECT id,platform.telegram_owner_cipher($1) FROM platform.telegram_integrations WHERE organization_id=$1 AND mode='live'`, t.OrganizationID).Scan(&id, &encrypted)
	_ = tx.Rollback(ctx)
	if e != nil {
		return nil, tenancy.Error(e)
	}
	token, e := decrypt(s.Config.Key, encrypted, t.OrganizationID)
	if e != nil {
		return nil, auth.Unavailable()
	}
	switch in.Operation {
	case "webhook_info":
		info, e := transport.GetWebhookInfo(ctx, token)
		if e != nil {
			return nil, auth.Unavailable()
		}
		return map[string]any{"pending_update_count": info.Pending, "last_error_date": info.LastErrorDate, "endpoint_matches": info.URL == s.Config.WebhookBase+"/api/v1/webhooks/telegram/"+id}, nil
	case "set_webhook":
		if s.Config.WebhookBase == "" {
			return nil, auth.Failure(403, "HTTPS_ENDPOINT_NOT_APPROVED", "Нет согласованного HTTPS endpoint")
		}
		secret := auth.Token()
		cipher, e := encrypt(s.Config.Key, secret, t.OrganizationID+":webhook")
		if e != nil {
			return nil, auth.Unavailable()
		}
		// Persist candidate secret first; ingress validates this exact hash. Failure is
		// reported, never a fabricated successful webhook; previous secret is replaced.
		tx, e = s.begin(ctx, t, true)
		if e != nil {
			return nil, e
		}
		_, e = tx.Exec(ctx, `SELECT platform.telegram_save_webhook($1,$2,$3,'disabled')`, t.OrganizationID, auth.Hash(secret), cipher)
		if e == nil {
			e = tx.Commit(ctx)
		} else {
			_ = tx.Rollback(ctx)
		}
		if e != nil {
			return nil, tenancy.Error(e)
		}
		if e = transport.SetWebhook(ctx, token, s.Config.WebhookBase+"/api/v1/webhooks/telegram/"+id, secret); e != nil {
			return nil, auth.Unavailable()
		}
		tx, e = s.begin(ctx, t, true)
		if e != nil {
			return nil, e
		}
		defer tx.Rollback(ctx)
		_, e = tx.Exec(ctx, `SELECT platform.telegram_save_webhook($1,NULL,$2,'configured')`, t.OrganizationID, cipher)
		if e == nil {
			e = tx.Commit(ctx)
		}
		if e != nil {
			return nil, tenancy.Error(e)
		}
		return map[string]any{"webhook_state": "configured"}, nil
	case "delete_webhook":
		if e = transport.DeleteWebhook(ctx, token); e != nil {
			return nil, auth.Unavailable()
		}
		tx, e = s.begin(ctx, t, true)
		if e != nil {
			return nil, e
		}
		defer tx.Rollback(ctx)
		_, e = tx.Exec(ctx, `SELECT platform.telegram_save_webhook($1,NULL,NULL,'disabled')`, t.OrganizationID)
		if e == nil {
			e = tx.Commit(ctx)
		}
		if e != nil {
			return nil, tenancy.Error(e)
		}
		return map[string]any{"webhook_state": "disabled"}, nil
	default:
		return nil, auth.Failure(422, "VALIDATION_ERROR", "Неизвестная операция")
	}
}
