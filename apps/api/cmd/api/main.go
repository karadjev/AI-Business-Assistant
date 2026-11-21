package main

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/agent"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	httpapi "github.com/karadjev/AI-Business-Assistant/apps/api/internal/http"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/db"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/logging"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/run"
)

func main() {
	if execute() != nil {
		os.Exit(1)
	}
}
func execute() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("configuration_invalid", "reason", err.Error())
		return err
	}
	log := logging.New(os.Stdout, cfg.LogLevel)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	database, err := db.Open(ctx, cfg.DatabaseURL, cfg.DBMaxConnections, cfg.DBTimeout)
	if err != nil {
		log.Error("database_initialization_failed")
		return err
	}
	defer database.Close()
	authConfig, err := config.LoadAuth(os.Getenv, cfg.Environment)
	if err != nil {
		log.Error("authentication_configuration_invalid", "reason", err.Error())
		return err
	}
	agentConfig, err := config.LoadAgent(os.Getenv)
	if err != nil {
		log.Error("agent_configuration_invalid")
		return err
	}
	telegramConfig, err := config.LoadTelegram(os.Getenv, cfg.Environment)
	if err != nil {
		log.Error("telegram_configuration_invalid")
		return err
	}
	whatsappConfig, err := config.LoadWhatsApp(os.Getenv, cfg.Environment)
	if err != nil {
		log.Error("whatsapp_configuration_invalid")
		return err
	}
	instagramConfig, err := config.LoadInstagram(os.Getenv, cfg.Environment)
	if err != nil {
		log.Error("instagram_configuration_invalid")
		return err
	}
	identity := auth.New(database.Pool, authConfig)
	listener, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		log.Error("listener_failed")
		return err
	}
	server := &http.Server{Handler: httpapi.NewHandlerWithChannels(database, log, cfg.RequestTimeout, cfg.MaxBodyBytes, identity, agentConfig, agent.NewProvider(agentConfig), telegramConfig, whatsappConfig, instagramConfig), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: cfg.RequestTimeout + time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	log.Info("api_started", "address", listener.Addr().String(), "api_version", "v1")
	if err = run.Serve(ctx, server, listener, cfg.ShutdownTimeout, log); err != nil {
		log.Error("api_shutdown_failed")
		return err
	}
	return nil
}
