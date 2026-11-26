//go:build integration

package integration

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	httpapi "github.com/karadjev/AI-Business-Assistant/apps/api/internal/http"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/db"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/migrate"
)

func testDSN(t *testing.T, key, role string) string {
	t.Helper()
	dsn := os.Getenv(key)
	u, e := url.Parse(dsn)
	if e != nil || u == nil || u.Path != "/aba_test" || u.User == nil || u.User.Username() != role {
		t.Fatalf("%s must point to isolated aba_test with role %s", key, role)
	}
	return dsn
}
func TestDatabaseAndMigrations(t *testing.T) {
	appDSN := testDSN(t, "ABA_TEST_DATABASE_URL", "aba_app")
	migrationDSN := testDSN(t, "ABA_TEST_MIGRATION_DATABASE_URL", "aba_migrator")
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second) // Includes the 121-page synthetic load sweep; SQL timeout remains 1s.
	defer cancel()
	conn, e := pgx.Connect(ctx, migrationDSN)
	if e != nil {
		t.Fatal("cannot connect to isolated database")
	}
	defer conn.Close(context.Background())
	var marker string
	if e = conn.QueryRow(ctx, `SELECT marker FROM platform.test_safety WHERE singleton=true`).Scan(&marker); e != nil || marker != "aba-synthetic-isolated" {
		t.Fatal("test safety marker missing; refusing migrations")
	}
	var count int
	if e = conn.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema='platform' AND table_name <> 'test_safety'`).Scan(&count); e != nil || count != 0 {
		t.Fatal("test schema is not clean; refusing to alter existing data")
	}
	database, e := db.Open(ctx, appDSN, 2, time.Second)
	if e != nil {
		t.Fatal(e)
	}
	if dir := os.Getenv("ABA_CAPTURE_CATALOG_PLAN_DIR"); dir != "" {
		attachCatalogPlanTrace(t, database, dir)
	}
	defer database.Close()
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	handler := httpapi.NewHandler(database, log, 3*time.Second, 1024)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/health/ready", nil))
	if w.Code != 503 {
		t.Fatal("schema missing should mean not ready")
	}
	if e = migrate.Run(ctx, migrationDSN, "up"); e != nil {
		t.Fatal(e)
	}
	t.Run("instagram-schema-up", func(t *testing.T) { testInstagramSchemaStage22(t, conn, ctx, 40) })
	if e = migrate.Run(ctx, migrationDSN, "up"); e != nil {
		t.Fatal("repeat up failed", e)
	}
	// Concurrent migration attempts must serialize through advisory lock.
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { results <- migrate.Run(ctx, migrationDSN, "up") }()
	}
	for i := 0; i < 2; i++ {
		if e = <-results; e != nil {
			t.Fatal(e)
		}
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/health/ready", nil))
	if w.Code != 200 {
		t.Fatalf("ready status %d", w.Code)
	}
	if _, e = database.Pool.Exec(ctx, `UPDATE platform.service_metadata SET version=2`); e == nil {
		t.Fatal("runtime can mutate metadata")
	}
	if _, e = database.Pool.Exec(ctx, `CREATE TABLE platform.forbidden (id integer)`); e == nil {
		t.Fatal("runtime can perform DDL")
	}
	if e = migrate.Run(ctx, migrationDSN, "down"); e != nil {
		t.Fatal("daily agent schema down", e)
	}
	if e = migrate.Run(ctx, migrationDSN, "down"); e != nil {
		t.Fatal("service work schema down", e)
	}
	if e = migrate.Run(ctx, migrationDSN, "down"); e != nil {
		t.Fatal(e)
	}
	if e = migrate.Run(ctx, migrationDSN, "down"); e != nil {
		t.Fatal(e)
	}
	if e = migrate.Run(ctx, migrationDSN, "down"); e != nil {
		t.Fatal(e)
	}
	if e = migrate.Run(ctx, migrationDSN, "down"); e != nil {
		t.Fatal(e)
	}
	if e = migrate.Run(ctx, migrationDSN, "down"); e != nil {
		t.Fatal(e)
	}
	if e = migrate.Run(ctx, migrationDSN, "down"); e != nil {
		t.Fatal(e)
	}
	if e = migrate.Run(ctx, migrationDSN, "down"); e != nil {
		t.Fatal("inbox schema down", e)
	}
	if e = migrate.Run(ctx, migrationDSN, "down"); e != nil {
		t.Fatal("commercial schema down", e)
	}
	t.Run("vehicle-service-hybrid-schema-down", func(t *testing.T) {
		var version int
		if err := conn.QueryRow(ctx, "SELECT version FROM platform.service_metadata WHERE singleton").Scan(&version); err != nil || version != 30 {
			t.Fatalf("vehicle service hybrid rollback: version=%d err=%v", version, err)
		}
	})
	// Continue down from schema 30 to schema 28.
	if e = migrate.Run(ctx, migrationDSN, "down"); e != nil {
		t.Fatal(e)
	}
	if e = migrate.Run(ctx, migrationDSN, "down"); e != nil {
		t.Fatal(e)
	}
	t.Run("conversational-schema-down", func(t *testing.T) {
		var version int
		var removed bool
		if err := conn.QueryRow(ctx, `SELECT version,to_regclass('platform.conversation_routes') IS NULL
		 FROM platform.service_metadata WHERE singleton`).Scan(&version, &removed); err != nil || version != 28 || !removed {
			t.Fatalf("conversational rollback: %d %t %v", version, removed, err)
		}
	})
	if e = migrate.Run(ctx, migrationDSN, "down"); e != nil {
		t.Fatal(e)
	}
	t.Run("real-estate-schema-down", func(t *testing.T) {
		var version int
		if err := conn.QueryRow(ctx, "SELECT version FROM platform.service_metadata WHERE singleton").Scan(&version); err != nil || version != 27 {
			t.Fatalf("real estate rollback: %d %v", version, err)
		}
	})
	if e = migrate.Run(ctx, migrationDSN, "down"); e != nil {
		t.Fatal(e)
	}
	t.Run("booking-notification-schema-down", func(t *testing.T) {
		var version int
		if err := conn.QueryRow(ctx, "SELECT version FROM platform.service_metadata WHERE singleton").Scan(&version); err != nil || version != 26 {
			t.Fatalf("booking notification rollback: %d %v", version, err)
		}
	})
	if e = migrate.Run(ctx, migrationDSN, "down"); e != nil {
		t.Fatal(e)
	}
	t.Run("service-finalization-schema-down", func(t *testing.T) {
		var version int
		if err := conn.QueryRow(ctx, "SELECT version FROM platform.service_metadata WHERE singleton").Scan(&version); err != nil || version != 25 {
			t.Fatalf("service finalization rollback: %d %v", version, err)
		}
	})
	if e = migrate.Run(ctx, migrationDSN, "down"); e != nil {
		t.Fatal(e)
	}
	t.Run("industry-delegation-schema-down", func(t *testing.T) {
		var version int
		if err := conn.QueryRow(ctx, "SELECT version FROM platform.service_metadata WHERE singleton").Scan(&version); err != nil || version != 24 {
			t.Fatalf("industry delegation rollback: %d %v", version, err)
		}
	})
	if e = migrate.Run(ctx, migrationDSN, "down"); e != nil {
		t.Fatal(e)
	}
	t.Run("delegation-schema-down", func(t *testing.T) {
		var version int
		if err := conn.QueryRow(ctx, "SELECT version FROM platform.service_metadata WHERE singleton").Scan(&version); err != nil || version != 23 {
			t.Fatalf("delegation rollback: %d %v", version, err)
		}
	})
	if e = migrate.Run(ctx, migrationDSN, "down"); e != nil {
		t.Fatal(e)
	}
	t.Run("marketing-schema-down", func(t *testing.T) {
		var version int
		var removed bool
		err := conn.QueryRow(ctx, "SELECT version,to_regclass('platform.marketing_drafts') IS NULL FROM platform.service_metadata WHERE singleton").Scan(&version, &removed)
		if err != nil || version != 22 || !removed {
			t.Fatalf("marketing rollback: %d %t %v", version, removed, err)
		}
	})
	if e = migrate.Run(ctx, migrationDSN, "down"); e != nil {
		t.Fatal(e)
	}
	t.Run("workflow-schema-down", func(t *testing.T) {
		var version int
		var removed bool
		err := conn.QueryRow(ctx, `SELECT version,
		 to_regprocedure('platform.agent_workflow_receipt(uuid,uuid,uuid,uuid,text,jsonb,boolean)') IS NULL
		 AND NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='platform'
		 AND table_name='property_listings' AND column_name='viewing_offering_id')
		 FROM platform.service_metadata WHERE singleton`).Scan(&version, &removed)
		if err != nil || version != 21 || !removed {
			t.Fatalf("property and workflow rollback: %d %t %v", version, removed, err)
		}
	})
	if e = migrate.Run(ctx, migrationDSN, "down"); e != nil {
		t.Fatal(e)
	}
	t.Run("reservation-schema-down", func(t *testing.T) {
		var version int
		var removed bool
		err := conn.QueryRow(ctx, "SELECT version,to_regclass('platform.hybrid_reservations') IS NULL FROM platform.service_metadata WHERE singleton").Scan(&version, &removed)
		if err != nil || version != 20 || !removed {
			t.Fatalf("reservation rollback: %d %t %v", version, removed, err)
		}
	})
	if e = migrate.Run(ctx, migrationDSN, "down"); e != nil {
		t.Fatal(e)
	}
	t.Run("operations-schema-down", func(t *testing.T) {
		var version int
		var removed bool
		if err := conn.QueryRow(ctx, "SELECT version,to_regclass('platform.service_bookings') IS NULL FROM platform.service_metadata WHERE singleton").Scan(&version, &removed); err != nil || version != 19 || !removed {
			t.Fatalf("operations rollback: version=%d removed=%t err=%v", version, removed, err)
		}
	})
	if e = migrate.Run(ctx, migrationDSN, "down"); e != nil {
		t.Fatal(e)
	}
	t.Run("business-schema-down", func(t *testing.T) {
		var version int
		if err := conn.QueryRow(ctx, "SELECT version FROM platform.service_metadata WHERE singleton").Scan(&version); err != nil || version != 18 {
			t.Fatalf("business rollback: version=%d err=%v", version, err)
		}
	})
	if e = migrate.Run(ctx, migrationDSN, "down"); e != nil {
		t.Fatal(e)
	}
	t.Run("agent-schema-down", func(t *testing.T) {
		var version int
		var removed bool
		if err := conn.QueryRow(ctx, "SELECT version,to_regclass('platform.agent_tasks') IS NULL FROM platform.service_metadata WHERE singleton").Scan(&version, &removed); err != nil || version != 17 || !removed {
			t.Fatalf("agent rollback: version=%d removed=%t err=%v", version, removed, err)
		}
	})
	if e = migrate.Run(ctx, migrationDSN, "down"); e != nil {
		t.Fatal(e)
	}
	t.Run("instagram-schema-down", func(t *testing.T) { testInstagramSchemaStage22(t, conn, ctx, 16) })
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/health/ready", nil))
	if w.Code != 503 {
		t.Fatal("readiness after rollback")
	}
	// After down17 ->16, roundtrip16..2 to1 on an empty isolated DB.
	for i := 0; i < 15; i++ {
		if e = migrate.Run(ctx, migrationDSN, "down"); e != nil {
			t.Fatal(e)
		}
	}
	if e = migrate.Run(ctx, migrationDSN, "up"); e != nil {
		t.Fatal(e)
	}
	t.Run("instagram-schema-reapply", func(t *testing.T) { testInstagramSchemaStage22(t, conn, ctx, 40) })
	testIdentity(t, database, conn, ctx)
	testCatalog(t, database, conn, ctx)
	testStage5(t, database, conn, ctx)
	testStage6(t, database, conn, ctx)
	testStage7(t, database, conn, ctx)
	testReliability(t, database, conn, ctx)
	t.Run("agent-runtime", func(t *testing.T) { testAgentRuntimeStage23(t, database, conn, ctx) })
	t.Run("agent-collaboration-http", func(t *testing.T) { testStage23CollaborationHTTP(t, database, conn, ctx) })
	testInstagramDurable(t, database, conn, ctx)
	testMultichannel(t, database, conn, ctx)
	testPreliveMatrix(t, database, conn, ctx)
	t.Run("stage26-marketing", func(t *testing.T) { testStage26Marketing(t, database, conn, ctx) })
	t.Run("booking-notifications", func(t *testing.T) { testBookingNotifications(t, database, conn, ctx) })
	testWhatsAppFinancial(t, database, conn, ctx)
	testStoreImport(t, database, conn, ctx)
	t.Run("stage24-industry-profiles", func(t *testing.T) { testStage24IndustryProfiles(t, database, conn, ctx) })
	t.Run("stage25-operations", func(t *testing.T) { testStage25Operations(t, database, conn, ctx) })
	t.Run("stage25-catalog", func(t *testing.T) { testStage25Catalog(t, database, conn, ctx) })
	t.Run("stage25-booking", func(t *testing.T) { testStage25Booking(t, database, conn, ctx) })
	t.Run("stage26-reservation", func(t *testing.T) { testStage26Reservation(t, database, conn, ctx) })
	t.Run("stage263-real-estate", func(t *testing.T) { testStage263RealEstate(t, database, conn, ctx) })
	// Refuse a populated draft schema rollback and preserve catalog data.
	var before, after string
	const snapshot = `SELECT md5(coalesce((SELECT jsonb_agg(to_jsonb(x) ORDER BY id)::text FROM platform.products x),''))`
	if e = conn.QueryRow(ctx, snapshot).Scan(&before); e != nil {
		t.Fatal(e)
	}
	// Remove unused workshop migrations and the password-reset migration first.
	if e = migrate.Run(ctx, migrationDSN, "down"); e != nil {
		t.Fatalf("empty manager schema rollback: %v", e)
	}
	if e = migrate.Run(ctx, migrationDSN, "down"); e != nil {
		t.Fatalf("empty marketing schema rollback: %v", e)
	}
	if e = migrate.Run(ctx, migrationDSN, "down"); e != nil {
		t.Fatalf("empty finance schema rollback: %v", e)
	}
	if e = migrate.Run(ctx, migrationDSN, "down"); e != nil {
		t.Fatalf("empty delegation schema rollback: %v", e)
	}
	if e = migrate.Run(ctx, migrationDSN, "down"); e != nil {
		t.Fatalf("empty workshop schema rollback: %v", e)
	}
	if e = migrate.Run(ctx, migrationDSN, "down"); e != nil {
		t.Fatalf("password-reset schema rollback: %v", e)
	}
	if e = migrate.Run(ctx, migrationDSN, "down"); e != nil {
		t.Fatalf("additive vehicle-service schema rollback: %v", e)
	}
	if e = migrate.Run(ctx, migrationDSN, "down"); e == nil {
		t.Fatal("populated draft history rollback allowed")
	}
	if e = migrate.Run(ctx, migrationDSN, "up"); e != nil {
		t.Fatalf("restore latest schema after rollback refusal: %v", e)
	}
	if e = conn.QueryRow(ctx, snapshot).Scan(&after); e != nil || before != after {
		t.Fatal("rollback changed catalog", e)
	}
	t.Run("stage261-delegation", func(t *testing.T) { testStage261Delegation(t, database, conn, ctx) })
	t.Run("stage262-industry", func(t *testing.T) { testStage262Industry(t, database, conn, ctx) })
	t.Run("stage262-service-closure", func(t *testing.T) { testStage262ServiceClosure(t, database, conn, ctx) })
	testAudit8A(t, database, conn, ctx)
	if err := migrate.Run(ctx, migrationDSN, "down"); err != nil {
		t.Fatalf("empty manager schema rollback: %v", err)
	}
	if err := migrate.Run(ctx, migrationDSN, "down"); err != nil {
		t.Fatalf("empty marketing schema rollback: %v", err)
	}
	if err := migrate.Run(ctx, migrationDSN, "down"); err != nil {
		t.Fatalf("empty finance schema rollback: %v", err)
	}
	if err := migrate.Run(ctx, migrationDSN, "down"); err != nil {
		t.Fatalf("empty delegation schema rollback: %v", err)
	}
	if err := migrate.Run(ctx, migrationDSN, "down"); err != nil {
		t.Fatalf("empty workshop schema rollback: %v", err)
	}
	if err := migrate.Run(ctx, migrationDSN, "down"); err != nil {
		t.Fatalf("password-reset rollback: %v", err)
	}
	if err := migrate.Run(ctx, migrationDSN, "down"); err == nil {
		t.Fatal("vehicle-service capability rollback should refuse when profile data exists")
	}
	unreachable, e := db.Open(ctx, "postgresql://aba_app:synthetic@127.0.0.1:1/aba_test?sslmode=disable", 1, 100*time.Millisecond)
	if e != nil {
		t.Fatal(e)
	}
	defer unreachable.Close()
	w = httptest.NewRecorder()
	httpapi.NewHandler(unreachable, log, time.Second, 1024).ServeHTTP(w, httptest.NewRequest("GET", "/health/ready", nil))
	if w.Code != 503 {
		t.Fatal("unavailable database must return 503")
	}
}
