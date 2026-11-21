package migrate

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/karadjev/AI-Business-Assistant/apps/api/migrations"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

func Run(ctx context.Context, dsn, command string) error {
	if command != "up" && command != "down" && command != "status" {
		return errors.New("migration command must be up, down or status")
	}
	database, err := sql.Open("pgx", dsn)
	if err != nil {
		return errors.New("migration database configuration invalid")
	}
	defer database.Close()
	database.SetMaxOpenConns(2)
	database.SetMaxIdleConns(2)
	database.SetConnMaxLifetime(10 * time.Minute)
	var safe bool
	if err = database.QueryRowContext(ctx, `SELECT NOT (rolsuper OR rolbypassrls OR rolcreatedb OR rolcreaterole) AND current_user='aba_migrator' FROM pg_roles WHERE rolname=current_user`).Scan(&safe); err != nil || !safe {
		return errors.New("migration role is unavailable or unsafe")
	}
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return errors.New("migration lock initialization failed")
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, database, migrations.Files, goose.WithSessionLocker(locker), goose.WithTableName("platform.goose_db_version"))
	if err != nil {
		return errors.New("migration provider initialization failed")
	}
	switch command {
	case "up":
		_, err = provider.Up(ctx)
	case "down":
		_, err = provider.Down(ctx)
	case "status":
		var statuses []*goose.MigrationStatus
		statuses, err = provider.Status(ctx)
		if err == nil {
			for _, status := range statuses {
				slog.Info("migration_status", "version", status.Source.Version, "state", status.State)
			}
		}
	}
	if err != nil {
		return errors.New("migration failed; inspect database roles and schema using a privileged local session")
	}
	return nil
}
