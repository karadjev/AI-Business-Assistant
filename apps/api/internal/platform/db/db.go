package db

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Database struct {
	Pool    *pgxpool.Pool
	Timeout time.Duration
}

func Open(ctx context.Context, dsn string, max int32, timeout time.Duration) (*Database, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, errors.New("invalid database configuration")
	}
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		var safe bool
		err := conn.QueryRow(ctx, `SELECT current_user='aba_app' AND NOT (rolsuper OR rolbypassrls OR rolcreatedb OR rolcreaterole OR rolreplication) AND NOT has_schema_privilege(current_user,'platform','CREATE') AND NOT EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='platform' AND c.relname IN ('organizations','organization_members','products','product_categories','product_variants','inventory','inventory_movements','orders','order_items','order_events','order_quotes','conversations','messages','conversation_events','ai_usage','telegram_integrations','telegram_principals','telegram_inbox','telegram_outbox','telegram_events','ai_settings','llm_usage','llm_reservations','agent_metrics','ai_settings_events','telegram_delivery_events','llm_budget_events','channel_integrations','channel_principals','channel_inbox','channel_outbox','channel_events','instagram_connections','instagram_oauth_states','wa_budgets','wa_pricing','wa_templates','wa_reservations','wa_financial_events','wa_operator_grants','wa_provider_records','wa_reconciliation_evidence','store_profiles','catalog_imports','catalog_events','catalog_policies','catalog_source_changes','agent_tasks','agent_executions','agent_approvals','agent_audit','property_listings','catalog_fitment_evidence','service_offerings','service_resources','service_offering_resources','service_availability_windows','service_bookings','service_booking_allocations','hybrid_reservations','marketing_drafts','conversation_routes','workshop_admissions','workshop_customers','workshop_requests','workshop_financials','workshop_manager_turns','inbox_reply_drafts','team_invites','team_audit','commercial_plans','commercial_admin_grants','commercial_subscriptions','commercial_audit','service_staff_resources','service_assignments','service_receipts','customer_consent_events') AND (c.relowner=(SELECT oid FROM pg_roles WHERE rolname=current_user) OR NOT c.relrowsecurity OR NOT c.relforcerowsecurity)) FROM pg_roles WHERE rolname=current_user`).Scan(&safe)
		if err != nil || !safe {
			return errors.New("unsafe or unavailable runtime database role")
		}
		return nil
	}
	cfg.MaxConns = max
	cfg.MinConns = 0
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.ConnConfig.ConnectTimeout = timeout
	// Connection/statement errors are intentionally never exposed through this package.
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = strconv.FormatInt(timeout.Milliseconds(), 10)
	cfg.ConnConfig.RuntimeParams["application_name"] = "ai-business-assistant"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, errors.New("database pool initialization failed")
	}
	return &Database{Pool: pool, Timeout: timeout}, nil
}
func (d *Database) Ready(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, d.Timeout)
	defer cancel()
	var restricted bool
	err := d.Pool.QueryRow(ctx, `SELECT current_user='aba_app' AND NOT (rolsuper OR rolbypassrls OR rolcreatedb OR rolcreaterole OR rolreplication) AND NOT has_schema_privilege(current_user,'platform','CREATE') AND NOT EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='platform' AND c.relname IN ('organizations','organization_members','products','product_categories','product_variants','inventory','inventory_movements','orders','order_items','order_events','order_quotes','conversations','messages','conversation_events','ai_usage','telegram_integrations','telegram_principals','telegram_inbox','telegram_outbox','telegram_events','ai_settings','llm_usage','llm_reservations','agent_metrics','ai_settings_events','telegram_delivery_events','llm_budget_events','channel_integrations','channel_principals','channel_inbox','channel_outbox','channel_events','instagram_connections','instagram_oauth_states','wa_budgets','wa_pricing','wa_templates','wa_reservations','wa_financial_events','wa_operator_grants','wa_provider_records','wa_reconciliation_evidence','store_profiles','catalog_imports','catalog_events','catalog_policies','catalog_source_changes','agent_tasks','agent_executions','agent_approvals','agent_audit','property_listings','catalog_fitment_evidence','service_offerings','service_resources','service_offering_resources','service_availability_windows','service_bookings','service_booking_allocations','hybrid_reservations','marketing_drafts','conversation_routes','workshop_admissions','workshop_customers','workshop_requests','workshop_financials','workshop_manager_turns','inbox_reply_drafts','team_invites','team_audit','commercial_plans','commercial_admin_grants','commercial_subscriptions','commercial_audit','service_staff_resources','service_assignments','service_receipts','customer_consent_events') AND (NOT c.relrowsecurity OR NOT c.relforcerowsecurity OR c.relowner=(SELECT oid FROM pg_roles WHERE rolname=current_user))) FROM pg_roles WHERE rolname=current_user`).Scan(&restricted)
	if err != nil || !restricted {
		return errors.New("database unavailable or runtime role unsafe")
	}
	var version int
	if err = d.Pool.QueryRow(ctx, `SELECT version FROM platform.service_metadata WHERE singleton=true`).Scan(&version); err != nil || version != 41 {
		return errors.New("database schema unavailable")
	}
	return nil
}
func (d *Database) Close() { d.Pool.Close() }
