//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/db"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
)

// Uses the parent migration harness's safety-checked disposable aba_test only.
func testInstagramWritesAndPool(t *testing.T, d *db.Database, admin *pgx.Conn, ctx context.Context, tenants []catalog.Tenant, conversations []string) {
	tables := []string{"instagram_connections", "instagram_oauth_states", "channel_integrations", "channel_inbox", "messages"}
	fingerprint := func(org string) string {
		t.Helper()
		result := ""
		for _, table := range tables {
			var digest string
			// table is from the closed literal list above, never an HTTP value.
			query := fmt.Sprintf(`SELECT md5(coalesce(jsonb_agg(to_jsonb(x) ORDER BY to_jsonb(x)::text)::text,'[]')) FROM (SELECT * FROM platform.%s WHERE organization_id=$1) x`, table)
			if err := admin.QueryRow(ctx, query, org).Scan(&digest); err != nil {
				t.Fatal("state fingerprint", err)
			}
			result += digest
		}
		return result
	}
	for i, actor := range tenants {
		for j, target := range tenants {
			if i == j {
				continue
			}
			for _, query := range []string{
				`INSERT INTO platform.messages(id,organization_id,conversation_id,role,content,client_key) VALUES($3,$1,$2,'customer','synthetic isolation probe',$3)`,
				`UPDATE platform.messages SET content='synthetic forbidden change' WHERE organization_id=$1 AND conversation_id=$2`,
				`DELETE FROM platform.messages WHERE organization_id=$1 AND conversation_id=$2`,
				`UPDATE platform.instagram_connections SET username='synthetic_forbidden_change' WHERE organization_id=$1 AND integration_id IN (SELECT integration_id FROM platform.channel_principals WHERE conversation_id=$2)`,
				`DELETE FROM platform.instagram_connections WHERE organization_id=$1 AND integration_id IN (SELECT integration_id FROM platform.channel_principals WHERE conversation_id=$2)`,
				`UPDATE platform.channel_inbox SET status='cancelled' WHERE organization_id=$1 AND conversation_id=$2`,
				`DELETE FROM platform.channel_inbox WHERE organization_id=$1 AND conversation_id=$2`,
			} {
				before := fingerprint(target.OrganizationID)
				tx, err := tenancy.Begin(ctx, d.Pool, actor, true, true)
				if err != nil {
					t.Fatal(err)
				}
				args := []any{target.OrganizationID, conversations[j]}
				if query[0:6] == "INSERT" {
					args = append(args, auth.UUID())
				}
				tag, writeErr := tx.Exec(ctx, query, args...)
				tx.Rollback(ctx)
				if writeErr != nil {
					var denied *pgconn.PgError
					if !errors.As(writeErr, &denied) || denied.Code != "42501" {
						t.Fatalf("unexpected write failure, not an authorization proof: %v", writeErr)
					}
				} else if tag.RowsAffected() != 0 {
					t.Fatalf("P0: tenant %d mutated tenant %d rows in a transaction", i, j)
				}
				if after := fingerprint(target.OrganizationID); after != before {
					t.Fatal("P0: foreign persistent state changed after rejected write")
				}
			}
		}
	}
	// Terminal failed rows are durable DLQ evidence, visible only to their tenant.
	for i, tenant := range tenants {
		tx, err := tenancy.Begin(ctx, d.Pool, tenant, true, true)
		if err != nil {
			t.Fatal(err)
		}
		var count int
		err = tx.QueryRow(ctx, `SELECT count(*) FROM platform.channel_inbox WHERE organization_id=$1 AND channel='instagram' AND status='failed'`, tenants[0].OrganizationID).Scan(&count)
		tx.Rollback(ctx)
		if err != nil || (i == 0 && count < 1) || (i != 0 && count != 0) {
			t.Fatal("DLQ isolation or positive control failed", i, count, err)
		}
	}
	// One physical connection must not carry session/tenant/capability across users.
	reused, err := db.Open(ctx, testDSN(t, "ABA_TEST_DATABASE_URL", "aba_app"), 1, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer reused.Close()
	var backend int
	for _, tenant := range append(append([]catalog.Tenant{}, tenants...), tenants[0]) {
		tx, err := tenancy.Begin(ctx, reused.Pool, tenant, true, true)
		if err != nil {
			t.Fatal(err)
		}
		var pid, count int
		if err = tx.QueryRow(ctx, `SELECT pg_backend_pid(),(SELECT count(*) FROM platform.instagram_connections)`).Scan(&pid, &count); err != nil || count != 1 {
			tx.Rollback(ctx)
			t.Fatal("pooled owner visibility", err)
		}
		if backend != 0 && backend != pid {
			tx.Rollback(ctx)
			t.Fatal("test did not reuse a physical connection")
		}
		backend = pid
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		var residual string
		if err = reused.Pool.QueryRow(ctx, `SELECT coalesce(current_setting('app.organization_id',true),'')||coalesce(current_setting('app.session_hash',true),'')||coalesce(current_setting('app.channel_cap',true),'')||coalesce(current_setting('app.telegram_cap',true),'')`).Scan(&residual); err != nil || residual != "" {
			t.Fatal("P0: transaction context survived pool reuse", err)
		}
		if err = reused.Pool.QueryRow(ctx, `SELECT count(*) FROM platform.instagram_connections`).Scan(&count); err != nil || count != 0 {
			t.Fatal("P0: empty pooled context exposed tenant rows", err)
		}
	}
}
