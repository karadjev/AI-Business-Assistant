package httpapi

import (
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/instagram"
)

func instagramRoutes(register func(string, string, func(http.ResponseWriter, *http.Request, auth.Session)), identity *auth.Service, s *instagram.Service, fail func(http.ResponseWriter, *http.Request, error)) {
	root := "/api/v1/organizations/{id}/integrations/instagram"
	for _, route := range []struct{ method, path string }{{"GET", ""}, {"POST", "/oauth/start"}, {"GET", "/oauth/callback"}, {"POST", "/disconnect"}, {"GET", "/webhook-status"}} {
		register(route.method, root+route.path, func(w http.ResponseWriter, r *http.Request, session auth.Session) {
			if err := identity.Limit(r.Context(), "instagram-config:"+session.User.ID, 60, time.Minute); err != nil {
				fail(w, r, err)
				return
			}
			tenant := catalog.Tenant{Session: session, OrganizationID: r.PathValue("id")}
			var result any
			var err error
			switch route.path {
			case "/oauth/start":
				result, err = s.Start(r.Context(), tenant)
			case "/oauth/callback":
				err = s.Callback(r.Context(), tenant, r.URL.Query())
				if err == nil {
					result, err = s.Status(r.Context(), tenant)
				}
			case "/disconnect":
				err = s.Disconnect(r.Context(), tenant)
				if err == nil {
					result, err = s.Status(r.Context(), tenant)
				}
			default:
				result, err = s.Status(r.Context(), tenant)
			}
			if err != nil {
				fail(w, r, err)
				return
			}
			writeJSON(w, http.StatusOK, result)
		})
	}
}
func instagramWebhook(mux *http.ServeMux, s *instagram.Service, identity *auth.Service) {
	path := "/api/v1/webhooks/instagram/{integration}"
	mux.HandleFunc("GET "+path, func(w http.ResponseWriter, r *http.Request) {
		if !s.Config.Enabled {
			writeError(w, r, 503, "INSTAGRAM_DISABLED", "Instagram выключен")
			return
		}
		if err := identity.Limit(r.Context(), "instagram-webhook-verify", 120, time.Minute); err != nil {
			instagramLimiterError(w, r, err)
			return
		}
		value, err := instagram.Challenge(s.Config.VerifyToken, r.URL.Query())
		if err != nil {
			writeError(w, r, 403, "WEBHOOK_REJECTED", "Запрос отклонён")
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, value)
	})
	mux.HandleFunc("POST "+path, func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, instagram.MaxPayload))
		if err != nil {
			writeError(w, r, 413, "BODY_TOO_LARGE", "Запрос отклонён")
			return
		}
		if len(r.Header.Values("X-Hub-Signature-256")) != 1 || !instagram.VerifySignature(s.Config.AppSecret, r.Header.Get("X-Hub-Signature-256"), raw) {
			writeError(w, r, 403, "WEBHOOK_REJECTED", "Запрос отклонён")
			return
		}
		if err = identity.Limit(r.Context(), "instagram-webhook-global", 1200, time.Minute); err != nil {
			instagramLimiterError(w, r, err)
			return
		}
		_, err = s.Ingress(r.Context(), r.PathValue("integration"), r.Header.Get("X-Hub-Signature-256"), raw)
		if errors.Is(err, instagram.ErrRejected) {
			writeError(w, r, 403, "WEBHOOK_REJECTED", "Запрос отклонён")
			return
		}
		if err != nil {
			writeError(w, r, 503, "QUEUE_UNAVAILABLE", "Повторите позже")
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"accepted": true})
	})
}

// A failed rate-limit repository is not evidence that the caller exceeded a
// limit. Preserve retryable unavailability without exposing database errors.
func instagramLimiterError(w http.ResponseWriter, r *http.Request, err error) {
	var limited *auth.Error
	if errors.As(err, &limited) && limited.Status == http.StatusTooManyRequests {
		w.Header().Set("Retry-After", "60")
		writeError(w, r, http.StatusTooManyRequests, "RATE_LIMITED", "Повторите позже")
		return
	}
	writeError(w, r, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Сервис временно недоступен")
}
