package httpapi

import (
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/conversations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/costcontrol"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/telegram"
	"net/http"
	"time"
)

func recoveryRoutes(register func(string, string, func(http.ResponseWriter, *http.Request, auth.Session)), identity *auth.Service, chat *conversations.Service, tg *telegram.Service, decode func(http.ResponseWriter, *http.Request, any) bool, fail func(http.ResponseWriter, *http.Request, error)) {
	ledger := costcontrol.Service{Pool: identity.Pool, Config: chat.Config}
	for _, route := range []struct{ method, path string }{{"GET", "/integrations/telegram/deliveries"}, {"POST", "/integrations/telegram/deliveries/{resource}/resolve"}, {"GET", "/llm-reservations"}, {"POST", "/llm-reservations/{resource}/recover"}} {
		register(route.method, "/api/v1/organizations/{id}"+route.path, func(w http.ResponseWriter, r *http.Request, session auth.Session) {
			if e := identity.Limit(r.Context(), "recovery:"+session.User.ID, 30, time.Minute); e != nil {
				fail(w, r, e)
				return
			}
			tenant := catalog.Tenant{Session: session, OrganizationID: r.PathValue("id")}
			var result any
			var e error
			switch route.path {
			case "/integrations/telegram/deliveries":
				if tg == nil {
					writeError(w, r, 503, "TELEGRAM_DISABLED", "Интеграция недоступна")
					return
				}
				result, e = tg.Deliveries(r.Context(), tenant)
			case "/integrations/telegram/deliveries/{resource}/resolve":
				if tg == nil {
					writeError(w, r, 503, "TELEGRAM_DISABLED", "Интеграция недоступна")
					return
				}
				var in telegram.DeliveryResolution
				if !decode(w, r, &in) {
					return
				}
				result, e = tg.ResolveDelivery(r.Context(), tenant, r.PathValue("resource"), in)
			case "/llm-reservations":
				result, e = ledger.Reservations(r.Context(), tenant)
			case "/llm-reservations/{resource}/recover":
				var in struct {
					Confirm bool `json:"confirm_conservative_charge"`
				}
				if !decode(w, r, &in) {
					return
				}
				if !in.Confirm {
					writeError(w, r, 422, "CONFIRMATION_REQUIRED", "Подтвердите полное консервативное списание резерва")
					return
				}
				e = ledger.Recover(r.Context(), tenant, r.PathValue("resource"))
				result = map[string]bool{"reconciled": true}
			}
			if e != nil {
				fail(w, r, e)
				return
			}
			writeJSON(w, 200, result)
		})
	}
}
