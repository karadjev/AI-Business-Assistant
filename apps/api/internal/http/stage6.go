package httpapi

import (
	"encoding/json"
	"errors"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/telegram"
	"io"
	"net/http"
	"strings"
	"time"
)

type telegramUpdateInput struct {
	ID       *int64             `json:"update_id"`
	Message  *telegram.Message  `json:"message,omitempty"`
	Callback *telegram.Callback `json:"callback_query,omitempty"`
}

func (i telegramUpdateInput) update() telegram.Update {
	return telegram.Update{ID: *i.ID, Message: i.Message, Callback: i.Callback}
}
func stage6Routes(register func(string, string, func(http.ResponseWriter, *http.Request, auth.Session)), identity *auth.Service, tg *telegram.Service, decode func(http.ResponseWriter, *http.Request, any) bool, fail func(http.ResponseWriter, *http.Request, error)) {
	base := "/api/v1/organizations/{id}/integrations/telegram"
	for _, route := range []struct{ method, suffix string }{{"GET", ""}, {"POST", "/connect"}, {"POST", "/check"}, {"POST", "/disconnect"}, {"POST", "/synthetic-updates"}} {
		register(route.method, base+route.suffix, func(w http.ResponseWriter, r *http.Request, session auth.Session) {
			if e := identity.Limit(r.Context(), "telegram-config:"+session.User.ID, 30, time.Minute); e != nil {
				fail(w, r, e)
				return
			}
			t := catalog.Tenant{Session: session, OrganizationID: r.PathValue("id")}
			var result any
			var e error
			switch route.suffix {
			case "":
				result, e = tg.Status(r.Context(), t)
			case "/connect":
				var in struct {
					Token string `json:"token"`
				}
				if !decode(w, r, &in) {
					return
				}
				if len(in.Token) > 256 {
					writeError(w, r, 422, "INVALID_BOT_TOKEN", "Некорректный токен")
					return
				}
				result, e = tg.Connect(r.Context(), t, in.Token)
			case "/check":
				result, e = tg.Check(r.Context(), t)
			case "/disconnect":
				result, e = tg.Disconnect(r.Context(), t)
			case "/synthetic-updates":
				if !tg.Config.Enabled {
					writeError(w, r, 503, "TELEGRAM_DISABLED", "Интеграция отключена")
					return
				}
				var u telegramUpdateInput
				if !decode(w, r, &u) {
					return
				}
				if u.ID == nil {
					writeError(w, r, 422, "INVALID_UPDATE", "update_id обязателен")
					return
				}
				var inserted bool
				inserted, e = tg.Inject(r.Context(), t, u.update())
				result = map[string]any{"accepted": true, "inserted": inserted}
			}
			if e != nil {
				fail(w, r, e)
				return
			}
			writeJSON(w, 200, result)
		})
	}
}
func telegramWebhook(mux *http.ServeMux, tg *telegram.Service) {
	// Rate limiting needs only the pool, never an Argon2 password hash.
	limiter := &auth.Service{Pool: tg.Pool}
	mux.HandleFunc("POST /api/v1/webhooks/telegram/{integration}", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			writeError(w, r, 415, "UNSUPPORTED_MEDIA_TYPE", "Требуется JSON")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16384)
		d := json.NewDecoder(r.Body)
		var u telegramUpdateInput
		// Telegram may add metadata fields; only typed security-relevant fields are used.
		if e := d.Decode(&u); e != nil {
			var size *http.MaxBytesError
			if errors.As(e, &size) {
				writeError(w, r, 413, "BODY_TOO_LARGE", "Событие превышает 16 KiB")
			} else {
				writeError(w, r, 400, "INVALID_UPDATE", "Некорректное событие")
			}
			return
		}
		var tail any
		if d.Decode(&tail) != io.EOF {
			writeError(w, r, 400, "INVALID_UPDATE", "Некорректное событие")
			return
		}
		if u.ID == nil {
			writeError(w, r, 422, "INVALID_UPDATE", "update_id обязателен")
			return
		}
		secret := r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
		if len(secret) != 43 {
			writeError(w, r, 403, "INVALID_WEBHOOK", "Webhook отклонён")
			return
		}
		if e := limiter.Limit(r.Context(), "telegram-webhook-global", 1200, time.Minute); e != nil {
			var ae *auth.Error
			if errors.As(e, &ae) {
				writeError(w, r, ae.Status, ae.Code, ae.Message)
			} else {
				writeError(w, r, 503, "SERVICE_UNAVAILABLE", "Сервис недоступен")
			}
			return
		}
		_, e := tg.Ingress(r.Context(), r.PathValue("integration"), secret, u.update())
		if e != nil {
			var ae *auth.Error
			if errors.As(e, &ae) {
				writeError(w, r, ae.Status, ae.Code, ae.Message)
			} else {
				writeError(w, r, 503, "SERVICE_UNAVAILABLE", "Сервис недоступен")
			}
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
}
