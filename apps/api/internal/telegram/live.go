package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/network"
	"net/url"
	"strings"
	"time"
)

type manualOperation struct{}

func ConfirmedOperation(ctx context.Context, confirmed bool) context.Context {
	if confirmed {
		return context.WithValue(ctx, manualOperation{}, true)
	}
	return ctx
}

type Guarded struct {
	Config config.Telegram
	Inner  Transport
}

func NewTransport(c config.Telegram) Transport {
	if c.Mode == "mock" {
		return Mock{}
	}
	return Guarded{c, &HTTPTransport{client: network.SafeClient(5 * time.Second), base: "https://api.telegram.org"}}
}
func (g Guarded) GetMe(ctx context.Context, token string) (Bot, error) {
	if g.Config.Mode != "live" || !g.Config.ExternalAuthorized || ctx.Value(manualOperation{}) != true {
		return Bot{}, errors.New("external Telegram operation not authorized")
	}
	return g.Inner.GetMe(ctx, token)
}
func (g Guarded) Send(ctx context.Context, token string, chat int64, p json.RawMessage) (SendResult, error) {
	if !g.Config.CanSendLive() || !g.Config.Allowed(chat) {
		return SendResult{}, errors.New("external Telegram sending disabled or test user not allowed")
	}
	return g.Inner.Send(ctx, token, chat, p)
}

type WebhookInfo struct {
	URL           string `json:"url"`
	Pending       int    `json:"pending_update_count"`
	LastErrorDate int64  `json:"last_error_date"`
}

func (t *HTTPTransport) SetWebhook(ctx context.Context, token, endpoint, secret string) error {
	u, e := url.Parse(endpoint)
	if e != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !strings.HasPrefix(u.Path, "/api/v1/webhooks/telegram/") || len(secret) != 43 {
		return errors.New("invalid webhook configuration")
	}
	_, _, e = t.call(ctx, token, "setWebhook", map[string]any{"url": endpoint, "secret_token": secret, "allowed_updates": []string{"message", "callback_query"}, "drop_pending_updates": false})
	return e
}
func (t *HTTPTransport) GetWebhookInfo(ctx context.Context, token string) (WebhookInfo, error) {
	raw, _, e := t.call(ctx, token, "getWebhookInfo", map[string]any{})
	var info WebhookInfo
	if e == nil {
		e = json.Unmarshal(raw, &info)
	}
	if e != nil {
		return info, errors.New("webhook status unavailable")
	}
	return info, nil
}
func (t *HTTPTransport) DeleteWebhook(ctx context.Context, token string) error {
	_, _, e := t.call(ctx, token, "deleteWebhook", map[string]any{"drop_pending_updates": false})
	return e
}
func (t *HTTPTransport) AnswerCallback(ctx context.Context, token, id string) error {
	if id == "" || len(id) > 128 {
		return errors.New("invalid callback")
	}
	_, _, e := t.call(ctx, token, "answerCallbackQuery", map[string]any{"callback_query_id": id, "text": "Запрос принят"})
	return e
}
