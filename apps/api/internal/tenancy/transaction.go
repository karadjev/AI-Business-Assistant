package tenancy

import (
	"context"
	"encoding/hex"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
)

func Failure(status int, code, message string) error { return auth.Failure(status, code, message) }
func Error(e error) error {
	if errors.Is(e, pgx.ErrNoRows) {
		return Failure(404, "NOT_FOUND", "Ресурс не найден")
	}
	var p *pgconn.PgError
	if errors.As(e, &p) {
		switch p.Code {
		case "23505":
			return Failure(409, "IDEMPOTENCY_CONFLICT", "Повторный ключ или ресурс")
		case "23503", "23514", "22003", "22P02":
			return Failure(422, "VALIDATION_ERROR", "Некорректные данные")
		case "42501":
			return Failure(403, "FORBIDDEN", "Операция запрещена")
		}
	}
	return auth.Unavailable()
}

// Begin installs only transaction-local context derived from an authenticated session.
func Begin(ctx context.Context, pool *pgxpool.Pool, t catalog.Tenant, staff, test bool) (pgx.Tx, error) {
	if !organizations.ValidID(t.OrganizationID) {
		return nil, Failure(404, "NOT_FOUND", "Организация не найдена")
	}
	tx, e := pool.Begin(ctx)
	if e != nil {
		return nil, Error(e)
	}
	rollback := func(e error) (pgx.Tx, error) { _ = tx.Rollback(ctx); return nil, e }
	if _, e = tx.Exec(ctx, `SELECT set_config('app.session_hash',$1,true),set_config('app.organization_id',$2,true)`, hex.EncodeToString(t.Session.Hash), t.OrganizationID); e != nil {
		return rollback(Error(e))
	}
	if t.ChannelCapability != "" {
		setting, scope := "app.telegram_cap", "platform.telegram_scope"
		if t.ChannelType == "whatsapp" || t.ChannelType == "instagram" {
			setting, scope = "app.channel_cap", "platform.channel_scope"
		}
		_, e = tx.Exec(ctx, `SELECT set_config($1,$2,true),set_config('app.channel_type',$3,true)`, setting, t.ChannelCapability, t.ChannelType)
		var ok bool
		if e == nil {
			e = tx.QueryRow(ctx, `SELECT `+scope+`($1,NULL)`, t.OrganizationID).Scan(&ok)
		}
		if e != nil || !ok {
			return rollback(Failure(404, "NOT_FOUND", "Channel scope unavailable"))
		}
		return tx, nil
	}
	var allowed, isTest bool
	e = tx.QueryRow(ctx, `SELECT (m.role='owner' OR m.can_manage_orders),o.is_test FROM platform.organizations o JOIN platform.organization_members m ON m.organization_id=o.id AND m.user_id=platform.current_user_id() WHERE o.id=$1`, t.OrganizationID).Scan(&allowed, &isTest)
	if e != nil {
		return rollback(Error(e))
	}
	if staff && !allowed {
		return rollback(Failure(403, "FORBIDDEN", "Нужно право staff для заявок и чата"))
	}
	if test && !isTest {
		return rollback(Failure(403, "TEST_ORGANIZATION_REQUIRED", "Тестовый чат и заявки доступны только в отдельной тестовой организации"))
	}
	return tx, nil
}
