package httpapi

import (
	"context"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/agent"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/conversations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/instagram"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/telegram"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/whatsapp"
	"log/slog"
	"net/http"
	"time"
)

type Readiness interface{ Ready(context.Context) error }
type StatusResponse struct {
	Service    string `json:"service"`
	Status     string `json:"status"`
	APIVersion string `json:"api_version"`
}
type HealthResponse struct {
	Status string `json:"status"`
}

func NewHandler(db Readiness, log *slog.Logger, timeout time.Duration, maxBody int64, identity ...*auth.Service) http.Handler {
	c, _ := config.LoadAgent(func(string) string { return "" })
	var service *auth.Service
	if len(identity) > 0 {
		service = identity[0]
	}
	return NewHandlerWithAgent(db, log, timeout, maxBody, service, c, agent.Mock{})
}
func NewHandlerWithAgent(db Readiness, log *slog.Logger, timeout time.Duration, maxBody int64, identity *auth.Service, c config.Agent, p agent.Provider, tgConfig ...config.Telegram) http.Handler {
	var tg config.Telegram
	if len(tgConfig) > 0 {
		tg = tgConfig[0]
	}
	return NewHandlerWithChannels(db, log, timeout, maxBody, identity, c, p, tg, config.WhatsApp{})
}
func NewHandlerWithChannels(db Readiness, log *slog.Logger, timeout time.Duration, maxBody int64, identity *auth.Service, c config.Agent, p agent.Provider, tgConfig config.Telegram, waConfig config.WhatsApp, igConfigs ...config.Instagram) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { writeError(w, r, 404, "NOT_FOUND", "Resource not found") })
	register := func(path string, h http.HandlerFunc) {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" {
				w.Header().Set("Allow", "GET")
				writeError(w, r, 405, "METHOD_NOT_ALLOWED", "Method not allowed")
				return
			}
			h(w, r)
		})
	}
	register("/health/live", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, HealthResponse{Status: "alive"}) })
	register("/health/ready", func(w http.ResponseWriter, r *http.Request) {
		if err := db.Ready(r.Context()); err != nil {
			log.Warn("readiness_failed", "code", "DATABASE_UNAVAILABLE", "request_id", requestID(r))
			writeError(w, r, 503, "DATABASE_UNAVAILABLE", "Service is not ready")
			return
		}
		writeJSON(w, 200, HealthResponse{Status: "ready"})
	})
	register("/api/v1/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, StatusResponse{Service: "ai-business-assistant", Status: "running", APIVersion: "v1"})
	})
	if identity != nil {
		chat := &conversations.Service{Pool: identity.Pool, Config: c, Provider: p, Log: log}
		tg := &telegram.Service{Log: log, Pool: identity.Pool, Config: tgConfig, Transport: telegram.NewTransport(tgConfig), Chat: chat}
		if tg.Config.Mode == "" {
			tg.Config.Mode = "mock"
		}
		if tg.Config.Enabled {
			telegramWebhook(mux, tg)
		}
		wa := &whatsapp.Service{Pool: identity.Pool, Key: waConfig.Key, Chat: chat, Transport: whatsapp.Mock{}}
		if len(waConfig.Key) == 32 {
			whatsappWebhook(mux, wa)
		}
		var igConfig config.Instagram
		if len(igConfigs) > 0 {
			igConfig = igConfigs[0]
		}
		ig := &instagram.Service{Pool: identity.Pool, Config: igConfig, Chat: chat, Log: log}
		instagramWebhook(mux, ig, identity)
		identityRoutes(mux, identity, chat, tg, wa, ig)
	}
	return middleware(mux, log, timeout, maxBody)
}
