//go:build integration

package integration

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"
)

// testInstagramSchemaStage22 inspects the schema after an existing migration step.
// Call with 17 after the initial up, 16 immediately after down, and 17 after reapply.
func testInstagramSchemaStage22(t *testing.T, conn *pgx.Conn, ctx context.Context, wantVersion int) {
	t.Helper()
	if wantVersion < 16 || wantVersion > 29 {
		t.Fatalf("unsupported Instagram schema checkpoint: %d", wantVersion)
	}
	var version int
	if err := conn.QueryRow(ctx, `SELECT version FROM platform.service_metadata WHERE singleton`).Scan(&version); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if version != wantVersion {
		t.Fatalf("schema version = %d, want %d", version, wantVersion)
	}

	for _, table := range []string{"channel_integrations", "channel_principals", "channel_inbox"} {
		assertInstagramSchemaRLS(t, conn, ctx, table)
	}
	if wantVersion == 16 {
		assertInstagramSchemaRollback(t, conn, ctx)
		return
	}
	for _, table := range []string{"instagram_connections", "instagram_oauth_states"} {
		assertInstagramSchemaRLS(t, conn, ctx, table)
	}

	for _, index := range []instagramSchemaIndex{
		{"channel_integrations", "channel_integrations_organization_id_channel_key", []string{"organization_id", "channel"}, true, ""},
		{"channel_integrations", "channel_integrations_provider_key", []string{"organization_id", "id", "channel"}, true, ""},
		{"channel_integrations", "channel_integrations_account_key", []string{"organization_id", "id", "channel", "phone_id"}, true, ""},
		{"channel_inbox", "channel_inbox_tenant_channel", []string{"organization_id", "channel", "status", "available_at"}, false, ""},
		{"instagram_connections", "instagram_connections_pkey", []string{"integration_id"}, true, ""},
		{"instagram_connections", "instagram_connections_account_id_key", []string{"account_id"}, true, ""},
		{"instagram_connections", "instagram_connections_organization_id_integration_id_key", []string{"organization_id", "integration_id"}, true, ""},
		{"instagram_connections", "instagram_connections_tenant", []string{"organization_id", "status"}, false, ""},
		{"instagram_oauth_states", "instagram_oauth_states_pkey", []string{"state_hash"}, true, ""},
		{"instagram_oauth_states", "instagram_oauth_states_expiry", []string{"expires_at"}, false, ""},
		{"instagram_oauth_states", "instagram_oauth_states_pending", []string{"organization_id", "integration_id"}, false, "(consumed_at IS NULL)"},
	} {
		assertInstagramSchemaIndex(t, conn, ctx, index)
	}

	for _, fk := range []struct {
		table, name, referencedTable string
		columns, referencedColumns   []string
	}{
		{"channel_principals", "channel_principals_provider_fk", "channel_integrations", []string{"organization_id", "integration_id", "channel"}, []string{"organization_id", "id", "channel"}},
		{"channel_inbox", "channel_inbox_provider_fk", "channel_integrations", []string{"organization_id", "integration_id", "channel", "external_account_id"}, []string{"organization_id", "id", "channel", "phone_id"}},
		{"instagram_connections", "instagram_connections_account_fk", "channel_integrations", []string{"organization_id", "integration_id", "channel", "account_id"}, []string{"organization_id", "id", "channel", "phone_id"}},
		{"instagram_oauth_states", "instagram_oauth_states_organization_id_integration_id_fkey", "instagram_connections", []string{"organization_id", "integration_id"}, []string{"organization_id", "integration_id"}},
	} {
		var columns, referencedColumns []string
		var validated, targetMatches bool
		err := conn.QueryRow(ctx, `
			SELECT c.convalidated, c.confrelid = to_regclass('platform.' || $3),
			       ARRAY(SELECT a.attname::text FROM unnest(c.conkey) WITH ORDINALITY AS k(attnum, position)
			             JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = k.attnum ORDER BY k.position),
			       ARRAY(SELECT a.attname::text FROM unnest(c.confkey) WITH ORDINALITY AS k(attnum, position)
			             JOIN pg_attribute a ON a.attrelid = c.confrelid AND a.attnum = k.attnum ORDER BY k.position)
			FROM pg_constraint c
			WHERE c.conrelid = to_regclass('platform.' || $1) AND c.conname::text = $2 AND c.contype = 'f'`,
			fk.table, fk.name, fk.referencedTable).Scan(&validated, &targetMatches, &columns, &referencedColumns)
		if err != nil {
			t.Fatalf("read foreign key %s.%s: %v", fk.table, fk.name, err)
		}
		if !validated || !targetMatches || !slices.Equal(columns, fk.columns) || !slices.Equal(referencedColumns, fk.referencedColumns) {
			t.Fatalf("foreign key %s.%s: columns=%v, referenced columns=%v, target is platform.%s=%t, validated=%t; want %v -> %v, matching target, and validation",
				fk.table, fk.name, columns, referencedColumns, fk.referencedTable, targetMatches, validated, fk.columns, fk.referencedColumns)
		}
	}
}

type instagramSchemaIndex struct {
	table, name string
	columns     []string
	unique      bool
	predicate   string
}

func assertInstagramSchemaIndex(t *testing.T, conn *pgx.Conn, ctx context.Context, want instagramSchemaIndex) {
	t.Helper()
	var columns []string
	var unique, usable, defaultShape bool
	var method, predicate string
	err := conn.QueryRow(ctx, `
		SELECT am.amname::text, i.indisunique, i.indisvalid AND i.indisready AND i.indislive,
		       i.indnkeyatts = i.indnatts AND i.indexprs IS NULL
		           AND NOT EXISTS (SELECT 1 FROM unnest(i.indoption) AS o(flags) WHERE o.flags <> 0),
		       coalesce(pg_get_expr(i.indpred, i.indrelid), ''),
		       ARRAY(SELECT a.attname::text FROM unnest(i.indkey) WITH ORDINALITY AS k(attnum, position)
		             JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = k.attnum ORDER BY k.position)
		FROM pg_index i
		JOIN pg_class idx ON idx.oid = i.indexrelid
		JOIN pg_am am ON am.oid = idx.relam
		WHERE i.indrelid = to_regclass('platform.' || $1) AND i.indexrelid = to_regclass('platform.' || $2)`,
		want.table, want.name).Scan(&method, &unique, &usable, &defaultShape, &predicate, &columns)
	if err != nil {
		t.Fatalf("read index %s on %s: %v", want.name, want.table, err)
	}
	if method != "btree" || unique != want.unique || !usable || !defaultShape || predicate != want.predicate || !slices.Equal(columns, want.columns) {
		t.Fatalf("index %s on %s: method=%s, columns=%v, unique=%t, predicate=%q, usable=%t, default plain keys=%t; want usable btree with default plain keys, columns=%v, unique=%t, predicate=%q",
			want.name, want.table, method, columns, unique, predicate, usable, defaultShape, want.columns, want.unique, want.predicate)
	}
}

func assertInstagramSchemaRLS(t *testing.T, conn *pgx.Conn, ctx context.Context, table string) {
	t.Helper()
	var enabled, forced bool
	err := conn.QueryRow(ctx, `SELECT relrowsecurity, relforcerowsecurity FROM pg_class
		WHERE oid = to_regclass('platform.' || $1) AND relkind = 'r'`, table).Scan(&enabled, &forced)
	if err != nil {
		t.Fatalf("read RLS flags for platform.%s: %v", table, err)
	}
	if !enabled || !forced {
		t.Fatalf("platform.%s RLS enabled=%t forced=%t, want both true", table, enabled, forced)
	}
}

func assertInstagramSchemaRollback(t *testing.T, conn *pgx.Conn, ctx context.Context) {
	t.Helper()
	var remaining []string
	err := conn.QueryRow(ctx, `SELECT ARRAY(
		SELECT name FROM unnest(ARRAY['instagram_connections', 'instagram_oauth_states', 'instagram_function_backup',
		    'channel_inbox_tenant_channel', 'channel_integrations_organization_id_channel_key',
		    'channel_integrations_provider_key', 'channel_integrations_account_key']) AS objects(name)
		WHERE to_regclass('platform.' || name) IS NOT NULL
		UNION ALL
		SELECT conname::text FROM pg_constraint
		WHERE (conrelid = 'platform.channel_inbox'::regclass AND conname = 'channel_inbox_provider_fk')
		   OR (conrelid = 'platform.channel_principals'::regclass AND conname = 'channel_principals_provider_fk')
		UNION ALL
		SELECT attname::text FROM pg_attribute
		WHERE attrelid = 'platform.channel_inbox'::regclass AND NOT attisdropped
		  AND attname IN ('channel', 'external_account_id', 'last_error_code')
		ORDER BY 1)`).Scan(&remaining)
	if err != nil {
		t.Fatalf("read Instagram schema after down to version 16: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("version 17 schema objects remain after rollback: %v", remaining)
	}
	assertInstagramSchemaIndex(t, conn, ctx, instagramSchemaIndex{
		"channel_integrations", "channel_integrations_organization_id_key", []string{"organization_id"}, true, "",
	})
}
