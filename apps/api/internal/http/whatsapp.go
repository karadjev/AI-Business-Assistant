package httpapi

import (
	"errors"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/whatsapp"
	"io"
	"net/http"
	"strings"
	"time"
)

func whatsappRoutes(register func(string, string, func(http.ResponseWriter, *http.Request, auth.Session)), identity *auth.Service, s *whatsapp.Service, decode func(http.ResponseWriter, *http.Request, any) bool, fail func(http.ResponseWriter, *http.Request, error)) {
	base := "/api/v1/organizations/{id}/integrations/whatsapp"
	for _, route := range []struct{ method, suffix string }{{"GET", "/operator"}, {"GET", "/provider"}, {"POST", "/provider/{resource}/revoke"}, {"GET", "/financial"}, {"GET", "/templates"}, {"POST", "/financial/{resource}/reconcile"}, {"POST", "/financial/check"}, {"GET", ""}, {"POST", "/connect"}, {"POST", "/control"}, {"POST", "/synthetic-updates"}, {"POST", "/deliveries/{resource}/resolve"}} {
		register(route.method, base+route.suffix, func(w http.ResponseWriter, r *http.Request, session auth.Session) {
			limitKey, limit := "whatsapp-config:", 30
			if route.method == "GET" && (route.suffix == "/financial" || route.suffix == "/templates" || route.suffix == "/provider" || route.suffix == "/operator") {
				limitKey = "whatsapp-financial-read:"
				limit = 60
			}
			if e := identity.Limit(r.Context(), limitKey+session.User.ID, limit, time.Minute); e != nil {
				fail(w, r, e)
				return
			}
			t := catalog.Tenant{Session: session, OrganizationID: r.PathValue("id")}
			var result any
			var e error
			switch route.suffix {
			case "/financial":
				result, e = s.FinancialState(r.Context(), t)
			case "/templates":
				result, e = s.Templates(r.Context(), t)
			case "/financial/check":
				// A diagnostic denial is never a financial authorization or a send operation.
				_, e = s.FinancialState(r.Context(), t)
				if e == nil {
					e = auth.Failure(403, "LIVE_DISABLED", "Live WhatsApp выключен; нет подтверждённой реальной тарифной политики")
				}
			case "/operator":
				result, e = s.OperatorState(r.Context(), t)
			case "/provider":
				result, e = s.ProviderState(r.Context(), t)
			case "/provider/{resource}/revoke":
				var in struct {
					Version int64 `json:"expected_version"`
					Confirm bool  `json:"confirm_revocation"`
				}
				if !decode(w, r, &in) {
					return
				}
				if !in.Confirm {
					writeError(w, r, 422, "VALIDATION_ERROR", "Требуется подтверждение отзыва")
					return
				}
				e = s.RevokeProvider(r.Context(), t, r.PathValue("resource"), in.Version)
				result = map[string]bool{"revoked": e == nil}
			case "/financial/{resource}/reconcile":
				var in whatsapp.ReconciliationInput
				if !decode(w, r, &in) {
					return
				}
				result, e = s.ReconcileEvidence(r.Context(), t, r.PathValue("resource"), in)

			case "":
				result, e = s.Status(r.Context(), t)
			case "/connect":
				var in whatsapp.ConnectInput
				if !decode(w, r, &in) {
					return
				}
				result, e = s.Connect(r.Context(), t, in)
			case "/control":
				var in struct {
					Active  *bool `json:"active"`
					Sending *bool `json:"sending_enabled"`
				}
				if !decode(w, r, &in) {
					return
				}
				if in.Active == nil || in.Sending == nil {
					writeError(w, r, 422, "VALIDATION_ERROR", "Укажите active и sending_enabled")
					return
				}
				result, e = s.Control(r.Context(), t, *in.Active, *in.Sending)
			case "/synthetic-updates":
				var in struct {
					User   string `json:"user_id"`
					ID     string `json:"message_id"`
					Text   string `json:"text"`
					Action string `json:"action"`
				}
				if !decode(w, r, &in) {
					return
				}
				var n int
				n, e = s.Inject(r.Context(), t, in.User, in.ID, in.Text, in.Action)
				result = map[string]any{"accepted": e == nil, "inserted": n}
			default:
				var in struct {
					Version   int64  `json:"expected_version"`
					State     string `json:"state"`
					Confirmed bool   `json:"confirm_resolution"`
				}
				if !decode(w, r, &in) {
					return
				}
				if !in.Confirmed || in.Version < 1 {
					writeError(w, r, 422, "VALIDATION_ERROR", "Требуется явное подтверждение")
					return
				}
				e = s.Resolve(r.Context(), t, r.PathValue("resource"), in.Version, in.State)
				result = map[string]bool{"resolved": e == nil}
			}
			if e != nil {
				fail(w, r, e)
				return
			}
			writeJSON(w, 200, result)
		})
	}
}
func whatsappWebhook(mux *http.ServeMux, s *whatsapp.Service) {
	limiter := &auth.Service{Pool: s.Pool}
	path := "/api/v1/webhooks/whatsapp/{integration}"
	reject := func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, 403, "INVALID_WEBHOOK", "Webhook отклонён")
	}
	upstreamError := func(w http.ResponseWriter, r *http.Request, e error) {
		var ae *auth.Error
		if errors.As(e, &ae) {
			writeError(w, r, ae.Status, ae.Code, ae.Message)
		} else {
			reject(w, r)
		}
	}
	mux.HandleFunc("GET "+path, func(w http.ResponseWriter, r *http.Request) {
		if e := limiter.Limit(r.Context(), "whatsapp-challenge-global", 120, time.Minute); e != nil {
			upstreamError(w, r, e)
			return
		}
		challenge, e := s.Challenge(r.Context(), r.PathValue("integration"), r.URL.Query())
		if e != nil {
			upstreamError(w, r, e)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(challenge))
	})
	mux.HandleFunc("POST "+path, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			writeError(w, r, 415, "UNSUPPORTED_MEDIA_TYPE", "Требуется JSON")
			return
		}
		signature := r.Header.Get("X-Hub-Signature-256")
		if len(signature) != 71 || !strings.HasPrefix(signature, "sha256=") {
			reject(w, r)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 65536)
		raw, e := io.ReadAll(r.Body)
		if e != nil {
			var size *http.MaxBytesError
			if errors.As(e, &size) {
				writeError(w, r, 413, "BODY_TOO_LARGE", "Webhook превышает 64 KiB")
			} else {
				writeError(w, r, 400, "INVALID_UPDATE", "Некорректное событие")
			}
			return
		}
		if e := limiter.Limit(r.Context(), "whatsapp-webhook-global", 1200, time.Minute); e != nil {
			upstreamError(w, r, e)
			return
		}
		if _, e = s.Ingress(r.Context(), r.PathValue("integration"), signature, raw); e != nil {
			var ae *auth.Error
			if errors.As(e, &ae) {
				writeError(w, r, ae.Status, ae.Code, ae.Message)
			} else {
				reject(w, r)
			}
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
}
