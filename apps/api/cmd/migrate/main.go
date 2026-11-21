package main

import (
	"context"
	"log/slog"
	"net/url"
	"os"
	"time"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/migrate"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(log)
	if len(os.Args) != 2 {
		log.Error("usage: migrate up|down|status")
		os.Exit(1)
	}
	dsn := os.Getenv("MIGRATION_DATABASE_URL")
	u, err := url.Parse(dsn)
	if err != nil || u == nil || u.User == nil || u.User.Username() != "aba_migrator" || u.Hostname() == "" || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		log.Error("MIGRATION_DATABASE_URL must use aba_migrator")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err = migrate.Run(ctx, dsn, os.Args[1]); err != nil {
		log.Error("migration_failed", "reason", err.Error())
		os.Exit(1)
	}
	log.Info("migration_command_completed", "command", os.Args[1])
}
