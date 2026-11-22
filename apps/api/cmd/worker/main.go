package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/agent"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/agentruntime"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/channel"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/conversations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/costcontrol"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/instagram"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/orders"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/db"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/logging"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/telegram"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/whatsapp"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if execute() != nil {
		os.Exit(1)
	}
}
func execute() error {
	cfg, e := config.Load(os.Getenv)
	if e != nil {
		slog.Error("configuration_invalid")
		return e
	}
	log := logging.New(os.Stdout, cfg.LogLevel)
	ac, e := config.LoadAuth(os.Getenv, cfg.Environment)
	if e != nil {
		log.Error("authentication_configuration_invalid", "reason", e.Error())
		return e
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	database, e := db.Open(ctx, cfg.DatabaseURL, cfg.DBMaxConnections, cfg.DBTimeout)
	if e != nil {
		log.Error("database_initialization_failed")
		return e
	}
	defer database.Close()
	tc, e := config.LoadTelegram(os.Getenv, cfg.Environment)
	if e != nil {
		log.Error("telegram_configuration_invalid")
		return e
	}
	agentConfig, e := config.LoadAgent(os.Getenv)
	if e != nil {
		return e
	}
	tg := telegram.Service{Log: log, Pool: database.Pool, Config: tc, Transport: telegram.NewTransport(tc), Chat: &conversations.Service{Pool: database.Pool, Config: agentConfig, Provider: agent.NewProvider(agentConfig), Log: log}}
	agents := agentruntime.Service{Pool: database.Pool, Key: ac.RetentionKey()}
	if os.Getenv("CONVERSATIONAL_ORCHESTRATION_ENABLED") == "true" {
		if (cfg.Environment != "test" && cfg.Environment != "staging") || agentConfig.Provider != "mock" || len(agents.Key) != 32 {
			log.Error("conversational_orchestration_requires_offline_configuration")
			return errors.New("invalid conversational orchestration configuration")
		}
		proofHash := sha256.Sum256(agents.Proof())
		tg.Chat.RoutingWorkerHash = proofHash[:]
	}
	ledger := costcontrol.Service{Pool: database.Pool, Config: agentConfig}
	service := auth.New(database.Pool, ac)
	log.Info("worker_started", "mode", "dev_email_and_mock_telegram")
	waConfig, err := config.LoadWhatsApp(os.Getenv, cfg.Environment)
	if err != nil {
		log.Error("whatsapp_configuration_invalid")
		return err
	}
	wa := whatsapp.Service{Pool: database.Pool, Key: waConfig.Key, Chat: tg.Chat, Transport: whatsapp.Mock{}}

	igConfig, err := config.LoadInstagram(os.Getenv, cfg.Environment)
	if err != nil {
		log.Error("instagram_configuration_invalid")
		return err
	}
	ig, err := instagramQueue(ctx, instagram.Service{Pool: database.Pool, Config: igConfig, Chat: tg.Chat, Log: log}, cfg.Environment)
	if err != nil {
		log.Error("instagram_queue_configuration_invalid")
		return err
	}
	cleanup := catalog.Service{Pool: database.Pool}
	reservations := orders.Service{Pool: database.Pool}
	lastCleanup := time.Time{}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Info("worker_stopped")
			return nil
		case <-ticker.C:
			job, cancel := context.WithTimeout(ctx, 8*time.Second)
			if time.Since(lastCleanup) > time.Minute {
				if _, err := cleanup.Cleanup(job, catalog.WorkerProof(ac.RetentionKey())); err != nil {
					log.Warn("catalog_cleanup_unavailable")
				}
				lastCleanup = time.Now()
			}
			if _, err := reservations.ExpireOne(job); err != nil && ctx.Err() == nil {
				log.Warn("hybrid_reservation_expiry_unavailable")
			}
			if recovered, err := ledger.RecoverStale(job); err != nil && ctx.Err() == nil {
				log.Warn("budget_recovery_unavailable")
			} else if recovered > 0 {
				log.Info("budget_recovered_unknown", "operations", recovered)
			}
			for _, ch := range []struct {
				name string
				q    channel.Queue
			}{{"telegram", tg}, {"whatsapp", wa}, {"instagram", ig}} {
				if _, err := ch.q.ProcessOne(job); err != nil && ctx.Err() == nil {
					log.Warn("channel_processing_unavailable", "channel", ch.name)
				}
				if _, err := ch.q.SendOne(job); err != nil && ctx.Err() == nil {
					log.Warn("channel_delivery_unavailable", "channel", ch.name)
				}
			}
			_, e = service.DeliverOne(job)
			cancel()
			if e != nil && ctx.Err() == nil {
				log.Warn("email_delivery_unavailable")
			}
			agentJob, stopAgent := context.WithTimeout(ctx, 7*time.Second)
			if _, err := agents.ProcessOne(agentJob); err != nil && ctx.Err() == nil {
				log.Warn("agent_task_processing_unavailable")
			}
			stopAgent()
		}
	}
}
