//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/db"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

func testCatalogScale(t *testing.T, d *db.Database, admin *pgx.Conn, ctx context.Context, tenant, alien, staff catalog.Tenant) {
	execFor := func(t *testing.T) func(string, ...any) {
		return func(q string, args ...any) {
			t.Helper()
			if _, e := admin.Exec(ctx, q, args...); e != nil {
				t.Fatal(e)
			}
		}
	}
	t.Run("10C_rotation_fencing_rollback_scope", func(t *testing.T) {
		exec := execFor(t)
		oldProof := catalog.WorkerProof(bytes.Repeat([]byte{11}, 32))
		newProof := catalog.WorkerProof(bytes.Repeat([]byte{12}, 32))
		svc := catalog.Service{Pool: d.Pool, CleanupProof: newProof}
		var version int64
		if e := admin.QueryRow(ctx, `SELECT version FROM platform.catalog_policies WHERE organization_id=$1`, tenant.OrganizationID).Scan(&version); e != nil {
			t.Fatal(e)
		}
		exec(`UPDATE platform.catalog_policies SET cleanup_hash=sha256($2) WHERE organization_id=$1`, tenant.OrganizationID, oldProof)
		if _, e := svc.RebindCleanup(ctx, staff, version, true); e == nil {
			t.Fatal("staff rebind")
		}
		outsider := staff
		outsider.OrganizationID = alien.OrganizationID
		if _, e := svc.RebindCleanup(ctx, outsider, version, true); e == nil {
			t.Fatal("foreign rebind")
		}
		if _, e := svc.RebindCleanup(ctx, tenant, version, false); e == nil {
			t.Fatal("missing confirmation")
		}
		exec(`CREATE FUNCTION platform.test_rebind_rollback() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN IF NEW.kind='CATALOG_CLEANUP_REBIND_CONFIRMED' THEN RAISE EXCEPTION 'synthetic rebind rollback';END IF;RETURN NEW;END $$`)
		exec(`CREATE TRIGGER test_rebind_rollback BEFORE INSERT ON platform.catalog_events FOR EACH ROW EXECUTE FUNCTION platform.test_rebind_rollback()`)
		if _, e := svc.RebindCleanup(ctx, tenant, version, true); e == nil {
			t.Fatal("missing rollback failure")
		}
		exec(`DROP TRIGGER test_rebind_rollback ON platform.catalog_events`)
		exec(`DROP FUNCTION platform.test_rebind_rollback()`)
		var same bool
		if e := admin.QueryRow(ctx, `SELECT cleanup_hash=sha256($2) AND version=$3 FROM platform.catalog_policies WHERE organization_id=$1`, tenant.OrganizationID, oldProof, version).Scan(&same); e != nil || !same {
			t.Fatal("partial rebind", e)
		}
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); _, e := svc.RebindCleanup(ctx, tenant, version, true); errs <- e }()
		}
		wg.Wait()
		close(errs)
		successes := 0
		for e := range errs {
			if e == nil {
				successes++
			} else if ae, ok := e.(*auth.Error); !ok || ae.Code != "VERSION_CONFLICT" {
				t.Fatal(e)
			}
		}
		if successes != 1 {
			t.Fatal("concurrent confirmations", successes)
		}
		if _, e := svc.RebindCleanup(ctx, tenant, version, true); e == nil {
			t.Fatal("stale replay")
		}
		// Actual capability discovery must revoke the old proof for this tenant.
		for _, v := range []struct {
			proof []byte
			want  bool
		}{{oldProof, false}, {newProof, true}} {
			var found bool
			if e := d.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM platform.catalog_cleanup_orgs($1) x WHERE x=$2::uuid)`, v.proof, tenant.OrganizationID).Scan(&found); e != nil || found != v.want {
				t.Fatal("proof scope", found, e)
			}
		}
		var alienBound bool
		if e := admin.QueryRow(ctx, `SELECT cleanup_hash=sha256($2) FROM platform.catalog_policies WHERE organization_id=$1`, alien.OrganizationID, newProof).Scan(&alienBound); e != nil || alienBound {
			t.Fatal("global rebinding", e)
		}
		raw, e := svc.Control(ctx, tenant)
		if e != nil || bytes.Contains(raw, []byte("cleanup_hash")) {
			t.Fatal("capability exposure", e)
		}
	})
	t.Run("10C_6001_variants_235_receipts", func(t *testing.T) {
		exec := execFor(t)
		// One isolated owner tenant, no channels required; tied timestamps test stable ordering.
		pid := auth.UUID()
		exec(`SELECT set_config('app.catalog_writer','MANUAL',false)`)
		exec(`INSERT INTO platform.products(id,organization_id,name,description,brand) VALUES($1,$2,'SYNTHETIC SCALE','Synthetic','Synthetic')`, pid, alien.OrganizationID)
		exec(`INSERT INTO platform.product_variants(id,organization_id,product_id,sku,variant_name,attributes,price_amount,currency_code) SELECT gen_random_uuid(),$1,$2,'SCALE-'||lpad(n::text,5,'0'),'Synthetic','{"synthetic":"true"}',10000,'RUB' FROM generate_series(1,6001) n`, alien.OrganizationID, pid)
		exec(`INSERT INTO platform.inventory(organization_id,variant_id,quantity_on_hand,manual_updated_at) SELECT $1,id,10,now() FROM platform.product_variants WHERE product_id=$2`, alien.OrganizationID, pid)
		exec(`INSERT INTO platform.catalog_imports(id,organization_id,request_key,file_digest,grid,status,created_by,created_at,minimized_at) SELECT gen_random_uuid(),$1,gen_random_uuid(),sha256('synthetic scale'::bytea),'[]','FAILED',$2,'2020-01-01'::timestamptz,now() FROM generate_series(1,235)`, alien.OrganizationID, alien.Session.User.ID)
		svc := catalog.Service{Pool: d.Pool}
		seenRows := map[string]bool{}
		seenImports := map[string]bool{}
		start := time.Now()
		var latencies []time.Duration
		maxBytes := 0
		// All pages exercise the real RLS-scoped service and exact aggregates.
		for page := 1; page <= 121; page++ {
			q := catalog.ControlQuery{Page: page}
			if page <= 3 {
				q.ImportPage = page
			}
			requestStart := time.Now()
			raw, e := svc.ControlPage(ctx, alien, q)
			latencies = append(latencies, time.Since(requestStart))
			if len(raw) > maxBytes {
				maxBytes = len(raw)
			}
			if e != nil {
				t.Fatalf("page %d: %v", page, e)
			}
			var result struct {
				Rows []struct {
					ID string `json:"variant_id"`
				}
				Imports []struct {
					ID string `json:"id"`
				}
				Total       int  `json:"rows_total"`
				ImportTotal int  `json:"imports_total"`
				Score       *int `json:"health_score"`
				Metrics     struct {
					Variants int
					Products int
				}
			}
			if e = json.Unmarshal(raw, &result); e != nil {
				t.Fatal(e)
			}
			if result.Total != 6001 || result.Metrics.Variants != 6001 || result.Metrics.Products != 1 || len(result.Rows) > 50 || result.ImportTotal != 236 || result.Score == nil || *result.Score != 100 {
				t.Fatalf("incorrect aggregate/page: total=%d variants=%d products=%d imports=%d score=%v", result.Total, result.Metrics.Variants, result.Metrics.Products, result.ImportTotal, result.Score)
			}
			for _, r := range result.Rows {
				if seenRows[r.ID] {
					t.Fatal("duplicate variant")
				}
				seenRows[r.ID] = true
			}
			if page <= 3 {
				for _, r := range result.Imports {
					if seenImports[r.ID] {
						t.Fatal("duplicate receipt")
					}
					seenImports[r.ID] = true
				}
			}
		}
		if len(seenRows) != 6001 || len(seenImports) != 236 {
			t.Fatal("truncated pagination", len(seenRows), len(seenImports))
		}
		if _, e := svc.Control(ctx, alien); e == nil {
			t.Fatal("legacy no-query limit changed")
		}
		raw, e := svc.ControlPage(ctx, alien, catalog.ControlQuery{Query: "SCALE-06001"})
		if e != nil {
			t.Fatal(e)
		}
		var filtered struct {
			Total int `json:"rows_total"`
		}
		if json.Unmarshal(raw, &filtered) != nil || filtered.Total != 1 {
			t.Fatal("search beyond first page")
		}
		raw, e = svc.Control(ctx, tenant)
		if e != nil || bytes.Contains(raw, []byte("SCALE-")) {
			t.Fatal("cross tenant scale leak", e)
		}
		sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
		t.Logf("LOAD: 6001 variants, 236 receipts, 121 RLS-scoped requests elapsed=%s p50=%s p95=%s max=%s max_response_bytes=%d", time.Since(start), latencies[len(latencies)/2], latencies[len(latencies)*95/100], latencies[len(latencies)-1], maxBytes)
	})
	t.Run("11A_accumulated_catalogs_concurrent_reads", func(t *testing.T) {
		exec := execFor(t)
		svc := catalog.Service{Pool: d.Pool}
		smallBefore, e := svc.ControlPage(ctx, tenant, catalog.ControlQuery{})
		if e != nil {
			t.Fatal(e)
		}
		type page struct {
			Total   int                    `json:"rows_total"`
			Imports int                    `json:"imports_total"`
			Rows    []struct{ SKU string } `json:"rows"`
			History []struct{ ID string }  `json:"imports"`
		}
		var small page
		if e = json.Unmarshal(smallBefore, &small); e != nil {
			t.Fatal(e)
		}
		org, e := (organizations.Service{Pool: d.Pool}).CreateTest(ctx, tenant.Session, "SYNTHETIC11A accumulated")
		if e != nil {
			t.Fatal(e)
		}
		large := tenant
		large.OrganizationID = org.ID
		pid := auth.UUID()
		exec(`SELECT set_config('app.catalog_writer','MANUAL',false)`)
		exec(`INSERT INTO platform.products(id,organization_id,name,description,brand) VALUES($1,$2,'SYNTHETIC11A','Synthetic','Synthetic')`, pid, org.ID)
		exec(`INSERT INTO platform.product_variants(id,organization_id,product_id,sku,variant_name,attributes,price_amount,currency_code) SELECT gen_random_uuid(),$1,$2,'HARDEN11A-'||lpad(n::text,5,'0'),'Synthetic','{"synthetic":"true"}',10000,'RUB' FROM generate_series(1,6001) n`, org.ID, pid)
		exec(`INSERT INTO platform.inventory(organization_id,variant_id,quantity_on_hand,manual_updated_at) SELECT $1,id,10,now() FROM platform.product_variants WHERE product_id=$2`, org.ID, pid)
		exec(`INSERT INTO platform.catalog_imports(id,organization_id,request_key,file_digest,grid,status,created_by,created_at,minimized_at) SELECT gen_random_uuid(),$1,gen_random_uuid(),sha256('synthetic11A'::bytea),'[]','FAILED',$2,'2020-01-01'::timestamptz,now() FROM generate_series(1,237)`, org.ID, tenant.Session.User.ID)
		// Start immediately after the bulk insert; do not wait for auto-analyze or
		// increase the existing one-second statement deadline to obtain a pass.
		pool, e := db.Open(ctx, d.Pool.Config().ConnString(), 8, time.Second)
		if e != nil {
			t.Fatal(e)
		}
		defer pool.Close()
		svc.Pool = pool.Pool
		type result struct {
			elapsed time.Duration
			err     error
			page    page
			which   int
		}
		results := make(chan result, 24)
		start := time.Now()
		var wg sync.WaitGroup
		for worker := 0; worker < 4; worker++ {
			wg.Add(1)
			go func(worker int) {
				defer wg.Done()
				for i := 0; i < 6; i++ {
					which := (worker + i) % 3
					who := []catalog.Tenant{tenant, alien, large}[which]
					at := time.Now()
					raw, err := svc.ControlPage(ctx, who, catalog.ControlQuery{Page: 1 + i%2, ImportPage: 1 + i%3})
					r := result{elapsed: time.Since(at), err: err, which: which}
					if err == nil {
						r.err = json.Unmarshal(raw, &r.page)
					}
					results <- r
				}
			}(worker)
		}
		wg.Wait()
		close(results)
		var latencies []time.Duration
		for r := range results {
			if r.err != nil {
				t.Fatal("concurrent RLS catalog read", r.err)
			}
			want := []int{small.Total, 6001, 6001}[r.which]
			if r.page.Total != want || len(r.page.Rows) > 50 || (r.which == 2 && r.page.Imports != 237) {
				t.Fatalf("incorrect accumulated catalog totals: tenant=%d total=%d want=%d imports=%d", r.which, r.page.Total, want, r.page.Imports)
			}
			for _, row := range r.page.Rows {
				if (r.which == 2) != strings.HasPrefix(row.SKU, "HARDEN11A-") {
					t.Fatal("cross-tenant accumulated catalog row")
				}
			}
			latencies = append(latencies, r.elapsed)
		}
		seen := map[string]bool{}
		for n := 1; n <= 3; n++ {
			raw, err := svc.ControlPage(ctx, large, catalog.ControlQuery{ImportPage: n})
			var p page
			if err != nil || json.Unmarshal(raw, &p) != nil {
				t.Fatal("accumulated import history", err)
			}
			for _, receipt := range p.History {
				if seen[receipt.ID] {
					t.Fatal("duplicate accumulated receipt")
				}
				seen[receipt.ID] = true
			}
		}
		if len(seen) != 237 {
			t.Fatal("truncated accumulated receipt history", len(seen))
		}
		outsider := staff
		outsider.OrganizationID = org.ID
		if _, err := svc.ControlPage(ctx, outsider, catalog.ControlQuery{}); err == nil {
			t.Fatal("foreign staff accessed accumulated catalog")
		}
		t.Run("owner_readers_coexist_and_fence_writer", func(t *testing.T) {
			// Hold the last metadata relation so real ControlPage transactions
			// remain open. Both readers must arrive there together; the writer
			// must remain behind their tenant locks until they finish.
			lock, err := admin.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer lock.Rollback(context.Background())
			if _, err = lock.Exec(ctx, `LOCK TABLE platform.catalog_source_changes IN ACCESS EXCLUSIVE MODE`); err != nil {
				t.Fatal(err)
			}
			reads := make(chan error, 2)
			for i := 0; i < 2; i++ {
				go func() { _, err := svc.ControlPage(ctx, large, catalog.ControlQuery{}); reads <- err }()
			}
			wait := func(query string) {
				t.Helper()
				deadline := time.Now().Add(750 * time.Millisecond)
				for {
					var ready bool
					if err := lock.QueryRow(ctx, query).Scan(&ready); err != nil {
						t.Fatal(err)
					}
					if ready {
						return
					}
					if time.Now().After(deadline) {
						t.Fatal("expected catalog lock state was not observed")
					}
					time.Sleep(10 * time.Millisecond)
				}
			}
			wait(`SELECT count(*)>=2 FROM pg_locks WHERE locktype='relation' AND relation='platform.catalog_source_changes'::regclass AND NOT granted`)
			writes := make(chan error, 1)
			go func() {
				_, err := svc.Category(ctx, large, "", catalog.Input{Name: "SYNTHETIC11A fenced writer"})
				writes <- err
			}()
			wait(`SELECT EXISTS(SELECT 1 FROM pg_locks WHERE locktype='advisory' AND NOT granted)`)
			select {
			case err := <-writes:
				t.Fatalf("writer passed open catalog readers: %v", err)
			default:
			}
			if err = lock.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				if err := <-reads; err != nil {
					t.Fatal("fenced reader failed", err)
				}
			}
			if err := <-writes; err != nil {
				t.Fatal("writer did not resume after readers", err)
			}
		})
		sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
		t.Logf("LOAD11A: two 6001-variant tenants plus small tenant, 24 reads/4 workers, 237 receipts/3 pages; elapsed=%s p50=%s p95=%s max=%s; SQL deadline=1s", time.Since(start), latencies[len(latencies)/2], latencies[len(latencies)*95/100], latencies[len(latencies)-1])
		t.Run("small_tenant_source_preview_with_large_foreign_catalogs", func(t *testing.T) {
			var product, source string
			if err := admin.QueryRow(ctx, `SELECT id,catalog_source FROM platform.products WHERE organization_id=$1 AND status<>'archived' ORDER BY id LIMIT 1`, tenant.OrganizationID).Scan(&product, &source); err != nil {
				t.Fatal(err)
			}
			target := "EXCEL"
			if source == target {
				target = "MANUAL"
			}
			began := time.Now()
			raw, err := svc.ChangeSource(ctx, tenant, catalog.SourceChange{OperationID: auth.UUID(), ProductID: product, Target: target, Reason: "SYNTHETIC11A bounded source preview"})
			var operation struct{ Status string }
			if err != nil || json.Unmarshal(raw, &operation) != nil || operation.Status != "PREVIEW" {
				t.Fatal("small tenant preview failed with large foreign inventories", err)
			}
			t.Logf("SOURCE_PREVIEW11A: elapsed=%s SQL deadline=1s; foreign variants>=12002", time.Since(began))
		})
		t.Run("small_tenant_import_fingerprint_with_large_foreign_catalogs", func(t *testing.T) {
			fixture, err := os.ReadFile("../../internal/catalog/testdata/synthetic-electronics.xlsx")
			if err != nil {
				t.Fatal(err)
			}
			type job struct {
				ID, Status string
				Version    int64
			}
			var uploaded, preview job
			raw, err := svc.UploadXLSX(ctx, tenant, auth.UUID(), fixture)
			if err != nil || json.Unmarshal(raw, &uploaded) != nil {
				t.Fatal("accumulated import upload", err)
			}
			mapping := map[string]int{"sku": 0, "name": 1, "category": 2, "brand": 3, "model": 4, "description": 5, "price": 6, "currency": 7, "quantity": 8, "availability": 9, "attributes": 10}
			began := time.Now()
			raw, err = svc.MapImport(ctx, tenant, uploaded.ID, uploaded.Version, mapping)
			if err != nil || json.Unmarshal(raw, &preview) != nil || preview.Status != "PREVIEW" {
				t.Fatal("accumulated import fingerprint", err)
			}
			t.Logf("IMPORT_PREVIEW11A: elapsed=%s SQL deadline=1s; foreign variants>=12002", time.Since(began))
			if _, err := svc.CancelImport(ctx, tenant, preview.ID, preview.Version); err != nil {
				t.Fatal("cancel synthetic preview", err)
			}
		})
		t.Run("small_tenant_product_lists_with_large_foreign_catalogs", func(t *testing.T) {
			exec(`INSERT INTO platform.products(id,organization_id,name,description,brand) SELECT gen_random_uuid(),$1,'SYNTHETIC foreign empty product','Synthetic','Synthetic' FROM generate_series(1,200)`, large.OrganizationID)
			cfg := d.Pool.Config()
			cfg.MaxConns = 1
			cfg.ConnConfig.RuntimeParams["plan_cache_mode"] = "force_generic_plan"
			pool, err := pgxpool.NewWithConfig(ctx, cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			bounded := catalog.Service{Pool: pool}
			for _, customer := range []bool{false, true} {
				want := map[string]bool{}
				rows, err := admin.Query(ctx, `SELECT id FROM platform.products WHERE organization_id=$1 AND (NOT $2::boolean OR status='active')`, tenant.OrganizationID, customer)
				if err != nil {
					t.Fatal(err)
				}
				for rows.Next() {
					var id string
					if err = rows.Scan(&id); err != nil {
						t.Fatal(err)
					}
					want[id] = true
				}
				err = rows.Err()
				rows.Close()
				if err != nil {
					t.Fatal(err)
				}
				if len(want) == 0 || len(want) > 100 {
					t.Fatal("expected a bounded nonempty synthetic tenant")
				}
				began := time.Now()
				raw, err := bounded.Products(ctx, tenant, catalog.Filters{PageSize: 100}, customer)
				var page struct {
					Items []struct{ ID string }
					Total int
				}
				if err != nil || json.Unmarshal(raw, &page) != nil || page.Total != len(want) || len(page.Items) != len(want) {
					t.Fatal("bounded tenant product listing", customer, err)
				}
				for _, item := range page.Items {
					if !want[item.ID] {
						t.Fatal("foreign product visible")
					}
					delete(want, item.ID)
				}
				if len(want) != 0 {
					t.Fatal("own products missing")
				}
				t.Logf("PRODUCTS11A: customer=%t generic_plan elapsed=%s; foreign products>=200, foreign variants>=12002, SQL deadline=1s", customer, time.Since(began))
			}
		})

	})
	t.Run("11A_cached_generic_before_bulk_point_lookup", func(t *testing.T) {
		exec := execFor(t)
		org, err := (organizations.Service{Pool: d.Pool}).CreateTest(ctx, tenant.Session, "SYNTHETIC11A cached generic")
		if err != nil {
			t.Fatal(err)
		}
		who := tenant
		who.OrganizationID = org.ID
		pid, first := auth.UUID(), auth.UUID()
		exec(`SELECT set_config('app.catalog_writer','MANUAL',false)`)
		exec(`INSERT INTO platform.products(id,organization_id,name,description,brand) VALUES($1,$2,'SYNTHETIC cached','Synthetic','Synthetic')`, pid, org.ID)
		exec(`INSERT INTO platform.product_variants(id,organization_id,product_id,sku,variant_name,attributes,price_amount,currency_code) VALUES($1,$2,$3,'CACHED-BASE','Synthetic','{"synthetic":"true"}',10000,'RUB')`, first, org.ID, pid)
		exec(`INSERT INTO platform.inventory(organization_id,variant_id,quantity_on_hand,manual_updated_at) VALUES($1,$2,10,now())`, org.ID, first)
		// Prevent auto-analyze from invalidating the warmed plan. This lock is
		// held by a separate test migration connection, permits normal reads and
		// writes, and is released on rollback. No server settings are changed.
		locker, err := pgx.Connect(ctx, admin.Config().ConnString())
		if err != nil {
			t.Fatal(err)
		}
		defer locker.Close(context.Background())
		lock, err := locker.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer lock.Rollback(context.Background())
		if _, err = lock.Exec(ctx, `LOCK TABLE platform.products,platform.product_variants,platform.inventory IN SHARE UPDATE EXCLUSIVE MODE`); err != nil {
			t.Fatal(err)
		}
		cfg := d.Pool.Config()
		cfg.MaxConns = 1
		cfg.ConnConfig.RuntimeParams["plan_cache_mode"] = "force_generic_plan"
		pool, err := pgxpool.NewWithConfig(ctx, cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer pool.Close()
		svc := catalog.Service{Pool: pool}
		read := func(q catalog.ControlQuery, total, rows int) time.Duration {
			t.Helper()
			began := time.Now()
			raw, err := svc.ControlPage(ctx, who, q)
			var page struct {
				Total int               `json:"rows_total"`
				Rows  []json.RawMessage `json:"rows"`
			}
			if err != nil || json.Unmarshal(raw, &page) != nil || page.Total != total || len(page.Rows) != rows {
				t.Fatalf("cached catalog page: total=%d rows=%d: %v", page.Total, len(page.Rows), err)
			}
			return time.Since(began)
		}
		read(catalog.ControlQuery{}, 1, 1)
		var before, after uint32
		if err = pool.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&before); err != nil {
			t.Fatal(err)
		}
		exec(`INSERT INTO platform.product_variants(id,organization_id,product_id,sku,variant_name,attributes,price_amount,currency_code) SELECT gen_random_uuid(),$1,$2,'CACHEDBULK-'||lpad(n::text,5,'0'),'Synthetic','{"synthetic":"true"}',10000,'RUB' FROM generate_series(1,6001) n`, org.ID, pid)
		exec(`INSERT INTO platform.inventory(organization_id,variant_id,quantity_on_hand,manual_updated_at) SELECT $1,id,10,now() FROM platform.product_variants WHERE organization_id=$1 AND sku LIKE 'CACHEDBULK-%'`, org.ID)
		elapsed := read(catalog.ControlQuery{Page: 1}, 6002, 50)
		read(catalog.ControlQuery{Page: 121}, 6002, 2)
		read(catalog.ControlQuery{Query: "CACHEDBULK-06001"}, 1, 1)
		if err = pool.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&after); err != nil || after != before {
			t.Fatal("cached-plan regression did not reuse the warmed connection", err)
		}
		if _, err := svc.Control(ctx, who); err == nil {
			t.Fatal("legacy catalog size limit bypassed")
		}
		t.Logf("CACHED_PLAN11A: same connection, generic plan warmed at1 then6002 variants, first_page=%s, last_page/search complete; SQL deadline=1s", elapsed)
	})
	t.Run("11A_catalog_jit_scope_survives_commit_and_rollback", func(t *testing.T) {
		cfg := d.Pool.Config()
		cfg.MaxConns = 1
		cfg.ConnConfig.RuntimeParams["jit"] = "on"
		probe := &catalogJITProbe{}
		cfg.ConnConfig.Tracer = probe
		pool, err := pgxpool.NewWithConfig(ctx, cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer pool.Close()
		svc := catalog.Service{Pool: pool}
		var originalPID uint32
		var setting string
		if err = pool.QueryRow(ctx, `SELECT pg_backend_pid(),current_setting('jit')`).Scan(&originalPID, &setting); err != nil || setting != "on" {
			t.Fatal("initial JIT configuration", err)
		}
		checkReset := func() {
			t.Helper()
			var pid uint32
			if err := pool.QueryRow(ctx, `SELECT pg_backend_pid(),current_setting('jit')`).Scan(&pid, &setting); err != nil || setting != "on" || pid != originalPID {
				t.Fatal("transaction setting leaked or pooled connection replaced", err)
			}
		}
		if _, err := svc.ControlPage(ctx, tenant, catalog.ControlQuery{}); err != nil {
			t.Fatal(err)
		}
		checkReset()
		// The over-limit legacy request returns after the aggregate and rolls
		// back. The next tenant must reuse the same connection with JIT restored.
		if _, err := svc.Control(ctx, alien); err == nil {
			t.Fatal("legacy limit bypassed")
		}
		checkReset()
		if _, err := svc.ControlPage(ctx, alien, catalog.ControlQuery{Page: 121}); err != nil {
			t.Fatal(err)
		}
		checkReset()
		if probe.err != nil || probe.aggregates != 3 || probe.pages != 2 {
			t.Fatalf("JIT transaction coverage: aggregates=%d pages=%d: %v", probe.aggregates, probe.pages, probe.err)
		}
		t.Log("JIT_SCOPE11A: aggregate/page JIT off only inside ControlPage; same connection restored to on after commit and rollback across tenants")
	})
}

// Observe actual server configuration, without changing the measured query or
// exporting its parameters. SHOW does not match the prefix, preventing recursion.
type catalogJITProbe struct {
	aggregates, pages int
	err               error
}

func (p *catalogJITProbe) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if !strings.HasPrefix(data.SQL, "WITH r AS (") {
		return ctx
	}
	var setting string
	if err := conn.QueryRow(ctx, `SHOW jit`).Scan(&setting); err != nil {
		p.err = err
	} else if setting != "off" {
		p.err = fmt.Errorf("interactive catalog compiled with JIT enabled")
	}
	if strings.Contains(data.SQL, "matched AS") {
		p.pages++
	} else {
		p.aggregates++
	}
	return ctx
}
func (*catalogJITProbe) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}
