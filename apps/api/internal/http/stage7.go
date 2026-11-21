package httpapi

import (
	"encoding/json"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/conversations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/costcontrol"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/telegram"
	"net/http"
	"strings"
	"time"
)

func stage7Routes(register func(string, string, func(http.ResponseWriter, *http.Request, auth.Session)), identity *auth.Service, chat *conversations.Service, tg *telegram.Service, decode func(http.ResponseWriter, *http.Request, any) bool, fail func(http.ResponseWriter, *http.Request, error)) {
	recoveryRoutes(register, identity, chat, tg, decode, fail)
	svc := costcontrol.Service{Pool: identity.Pool, Config: chat.Config}
	if tg != nil {
		svc.TelegramAuthorized = tg.Config.CanSendLive()
	}
	root := "/api/v1/organizations/{id}"
	for _, route := range []struct{ method, path string }{{"GET", "/ai-settings"}, {"PATCH", "/ai-settings"}, {"GET", "/agent/metrics"}, {"POST", "/integrations/telegram/live-operations"}, {"POST", "/conversations/{resource}/purge-test-data"}} {
		register(route.method, root+route.path, func(w http.ResponseWriter, r *http.Request, session auth.Session) {
			if e := identity.Limit(r.Context(), "stage7:"+session.User.ID, 60, time.Minute); e != nil {
				fail(w, r, e)
				return
			}
			t := catalog.Tenant{OrganizationID: r.PathValue("id"), Session: session}
			var result any
			var e error
			switch route.path {
			case "/ai-settings":
				if r.Method == "PATCH" {
					var in costcontrol.Change
					if !decode(w, r, &in) {
						return
					}
					result, e = svc.Update(r.Context(), t, in)
				} else {
					state, err := svc.State(r.Context(), t)
					e = err
					if err == nil {
						var data map[string]any
						_ = json.Unmarshal(state, &data)
						tgState := map[string]any{"mode": "mock", "external_authorized": false, "sending_enabled": false, "kill_switch": true, "webhook": "not configured"}
						if tg != nil {
							tgState["mode"] = tg.Config.Mode
							tgState["external_authorized"] = tg.Config.ExternalAuthorized
							tgState["sending_enabled"] = tg.Config.SendingEnabled
							tgState["kill_switch"] = tg.Config.KillSending
							if st, se := tg.Status(r.Context(), t); se == nil {
								var status struct {
									Integration *struct {
										Webhook string `json:"webhook"`
									} `json:"integration"`
								}
								_ = json.Unmarshal(st, &status)
								if status.Integration != nil {
									tgState["webhook"] = status.Integration.Webhook
								}
							}
						}
						data["telegram"] = tgState
						result = data
					}
				}
			case "/agent/metrics":
				result, e = svc.State(r.Context(), t)
			case "/integrations/telegram/live-operations":
				var in telegram.LiveOperation
				if !decode(w, r, &in) {
					return
				}
				if tg == nil {
					writeError(w, r, 403, "EXTERNAL_REQUEST_NOT_AUTHORIZED", "Telegram NOT AUTHORIZED")
					return
				}
				if tg.Config.ExternalAuthorized && !strings.HasPrefix(identity.Config.Origin, "https://") {
					writeError(w, r, 403, "HTTPS_REQUIRED", "Real token operation requires approved HTTPS origin")
					return
				}
				result, e = tg.LiveOperation(r.Context(), t, in)
			case "/conversations/{resource}/purge-test-data":
				var in struct {
					Confirm         bool  `json:"confirm_delete_synthetic_data"`
					ExpectedVersion int64 `json:"expected_version"`
				}
				if !decode(w, r, &in) {
					return
				}
				if !in.Confirm {
					writeError(w, r, 422, "CONFIRMATION_REQUIRED", "Нужно отдельное подтверждение полного удаления тестовых данных")
					return
				}
				result, e = svc.Purge(r.Context(), t, r.PathValue("resource"), in.ExpectedVersion)
			}
			if e != nil {
				fail(w, r, e)
				return
			}
			writeJSON(w, 200, result)
		})
	}
}
