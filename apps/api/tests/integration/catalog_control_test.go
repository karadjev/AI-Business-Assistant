//go:build integration

package integration

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/db"
	"sync"
	"testing"
	"time"
)

func testCatalogControl(t *testing.T, d *db.Database, admin *pgx.Conn, ctx context.Context, tenant, alien, staff catalog.Tenant) {
	s := catalog.Service{Pool: d.Pool}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, e := admin.Exec(ctx, q, args...); e != nil {
			t.Fatal(e)
		}
	}
	t.Run("10B_control_source_freshness", func(t *testing.T) {
		raw, e := s.Control(ctx, tenant)
		if e != nil {
			t.Fatal(e)
		}
		var overview map[string]any
		_ = json.Unmarshal(raw, &overview)
		if overview["health_score"] == nil {
			t.Fatal("populated score missing")
		}
		if _, e = s.Control(ctx, staff); e == nil {
			t.Fatal("staff control mutation scope")
		}
		policy := catalog.Policy{FreshHours: 2, StaleHours: 4, LowStock: 3, PreviewDays: 1, ErrorDays: 7, ChangeDays: 30}
		if _, e = s.SavePolicy(ctx, tenant, policy); e != nil {
			t.Fatal(e)
		}
		if _, e = s.SavePolicy(ctx, tenant, policy); e == nil {
			t.Fatal("stale policy")
		}
		var pid, vid string
		var price int64
		var on int
		exec(`SELECT set_config('app.catalog_writer','EXCEL',false)`)
		if e = admin.QueryRow(ctx, `SELECT p.id,v.id,v.price_amount,i.quantity_on_hand FROM platform.products p JOIN platform.product_variants v ON v.product_id=p.id JOIN platform.inventory i ON i.variant_id=v.id WHERE p.organization_id=$1 AND p.status='active' LIMIT 1`, tenant.OrganizationID).Scan(&pid, &vid, &price, &on); e != nil {
			t.Fatal(e)
		}
		exec(`UPDATE platform.inventory SET manual_updated_at=now()-interval '3 hours' WHERE variant_id=$1`, vid)
		raw, e = s.Customer().CheckInventory(ctx, tenant, vid)
		if e != nil {
			t.Fatal(e)
		}
		var stock map[string]any
		_ = json.Unmarshal(raw, &stock)
		if stock["freshness"] != "AGING" || stock["available"] != nil {
			t.Fatal("aging promise", string(raw))
		}
		// Common reserve guard rejects AGING even when the legacy environment cap is24h.
		guardTx, err := d.Pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		_, err = guardTx.Exec(ctx, `SELECT set_config('app.organization_id',$1,true),set_config('app.session_hash',$2,true)`, tenant.OrganizationID, hex.EncodeToString(tenant.Session.Hash))
		if err != nil {
			t.Fatal(err)
		}
		err = catalog.MoveOrderStock(ctx, guardTx, tenant, auth.UUID(), vid, "reserve", 1)
		_ = guardTx.Rollback(ctx)
		if ae, ok := err.(*auth.Error); !ok || ae.Code != "STOCK_STALE" {
			t.Fatal("aging reserve admitted", err)
		}
		exec(`UPDATE platform.inventory SET manual_updated_at=now()+interval '1 hour' WHERE variant_id=$1`, vid)
		raw, e = s.Customer().CheckInventory(ctx, tenant, vid)
		if e != nil {
			t.Fatal(e)
		}
		_ = json.Unmarshal(raw, &stock)
		if stock["freshness"] != "UNKNOWN" || stock["available"] != nil {
			t.Fatal("future stock trusted")
		}
		exec(`UPDATE platform.inventory SET manual_updated_at=now()-interval '5 hours' WHERE variant_id=$1`, vid)
		raw, e = s.Customer().CheckInventory(ctx, tenant, vid)
		if e != nil {
			t.Fatal(e)
		}
		_ = json.Unmarshal(raw, &stock)
		if stock["freshness"] != "STALE" || stock["available"] != nil {
			t.Fatal("stale promise")
		}
		change := catalog.SourceChange{OperationID: auth.UUID(), ProductID: pid, Target: "MANUAL", Reason: "Synthetic controlled handoff"}
		if _, e = s.ChangeSource(ctx, alien, change); e == nil {
			t.Fatal("foreign handoff")
		}
		if _, e = s.ChangeSource(ctx, staff, change); e == nil {
			t.Fatal("staff handoff")
		}
		if _, e = s.ChangeSource(ctx, tenant, change); e != nil {
			t.Fatal(e)
		}
		change.Confirm = true
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); _, e := s.ChangeSource(ctx, tenant, change); errs <- e }()
		}
		wg.Wait()
		close(errs)
		for e := range errs {
			if e != nil {
				t.Fatal("duplicate concurrent apply", e)
			}
		}
		var gotPrice int64
		var gotOn int
		var source string
		_ = admin.QueryRow(ctx, `SELECT v.price_amount,i.quantity_on_hand,v.catalog_source FROM platform.product_variants v JOIN platform.inventory i ON i.variant_id=v.id WHERE v.id=$1`, vid).Scan(&gotPrice, &gotOn, &source)
		if gotPrice != price || gotOn != on || source != "MANUAL" {
			t.Fatal("handoff changed values")
		}
		// A product edit after preview fences the transfer.
		next := catalog.SourceChange{OperationID: auth.UUID(), ProductID: pid, Target: "EXCEL", Reason: "stale preview"}
		if _, e = s.ChangeSource(ctx, tenant, next); e != nil {
			t.Fatal(e)
		}
		exec(`UPDATE platform.products SET version=version+1 WHERE id=$1`, pid)
		next.Confirm = true
		if _, e = s.ChangeSource(ctx, tenant, next); e == nil {
			t.Fatal("stale source preview")
		}
		// Actual DB rollback leaves neither source nor applied history behind.
		tx, e := d.Pool.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		_, e = tx.Exec(ctx, `SELECT set_config('app.organization_id',$1,true),set_config('app.session_hash',$2,true),set_config('app.catalog_handoff','true',true)`, tenant.OrganizationID, hex.EncodeToString(tenant.Session.Hash))
		if e != nil {
			t.Fatal(e)
		}
		_, e = tx.Exec(ctx, `UPDATE platform.products SET catalog_source='EXCEL',version=version+1 WHERE id=$1`, pid)
		if e != nil {
			t.Fatal(e)
		}
		_ = tx.Rollback(ctx)
		exec(`UPDATE platform.inventory SET manual_updated_at=now() WHERE variant_id=$1`, vid)
		raw, e = s.Customer().CheckInventory(ctx, tenant, vid)
		if e != nil {
			t.Fatal(e)
		}
		_ = json.Unmarshal(raw, &stock)
		if stock["freshness"] != "FRESH" || stock["available"] == nil {
			t.Fatal("stock verification not fresh")
		}
	})
	t.Run("10B_retention_capability_hold_concurrency", func(t *testing.T) {
		proof := []byte("synthetic-worker-proof-32-bytes!!")
		proof = proof[:32]
		// Existing integration schema is reused; only proof binding is synthetic.
		var integration string
		e := admin.QueryRow(ctx, `SELECT id FROM platform.telegram_integrations LIMIT 1`).Scan(&integration)
		if e != nil {
			t.Fatal(e)
		}
		exec(`UPDATE platform.telegram_integrations SET worker_hash=sha256($1) WHERE id=$2`, proof, integration)
		var org string
		_ = admin.QueryRow(ctx, `SELECT organization_id FROM platform.telegram_integrations WHERE id=$1`, integration).Scan(&org)
		var owner string
		_ = admin.QueryRow(ctx, `SELECT user_id FROM platform.organization_members WHERE organization_id=$1 AND role='owner'`, org).Scan(&owner)
		insert := func(hold bool, age string, status string) string {
			id := auth.UUID()
			exec(`INSERT INTO platform.catalog_imports(id,organization_id,request_key,file_digest,grid,preview,created_by,created_at,expires_at,status,retention_hold) VALUES($1,$2,$3,sha256('synthetic'::bytea),'[["sensitive synthetic"]]','{"total":1,"errors":1,"lines":[{"reason":"synthetic"}]}',$4,now()-$5::interval,now()-interval '1 day',$6,$7)`, id, org, auth.UUID(), owner, age, status, hold)
			return id
		}
		expired := insert(false, "10 days", "PREVIEW")
		// Inject failure after UPDATE, before audit INSERT: the whole cleanup must roll back.
		exec(`CREATE FUNCTION platform.test_cleanup_rollback() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN IF NEW.kind='IMPORT_RETENTION_MINIMIZED' THEN RAISE EXCEPTION 'synthetic rollback';END IF;RETURN NEW;END $$`)
		exec(`CREATE TRIGGER test_cleanup_rollback BEFORE INSERT ON platform.catalog_events FOR EACH ROW EXECUTE FUNCTION platform.test_cleanup_rollback()`)
		if _, e = s.Cleanup(ctx, proof); e == nil {
			t.Fatal("injected cleanup failure missing")
		}
		var cleared bool
		_ = admin.QueryRow(ctx, `SELECT minimized_at IS NOT NULL FROM platform.catalog_imports WHERE id=$1`, expired).Scan(&cleared)
		if cleared {
			t.Fatal("partial cleanup after rollback")
		}
		exec(`DROP TRIGGER test_cleanup_rollback ON platform.catalog_events`)
		exec(`DROP FUNCTION platform.test_cleanup_rollback()`)

		held := insert(true, "10 days", "PREVIEW")
		active := insert(false, "10 days", "PREVIEW")
		exec(`UPDATE platform.catalog_imports SET expires_at=now()+interval '1 hour' WHERE id=$1`, active)
		inUse := insert(false, "10 days", "PREVIEW")
		locked, e := admin.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = locked.Exec(ctx, `SELECT id FROM platform.catalog_imports WHERE id=$1 FOR UPDATE`, inUse); e != nil {
			t.Fatal(e)
		}
		if _, e = s.Cleanup(ctx, proof); e != nil {
			t.Fatal(e)
		}
		var inUseCleared bool
		_ = admin.QueryRow(ctx, `SELECT minimized_at IS NOT NULL FROM platform.catalog_imports WHERE id=$1`, inUse).Scan(&inUseCleared)
		if inUseCleared {
			t.Fatal("in-use import removed")
		}
		_ = locked.Rollback(ctx)
		applied := insert(false, "100 days", "APPLIED")
		if n, e := s.Cleanup(ctx, []byte("wrong")); e == nil || n != 0 {
			t.Fatal("unauthorized cleanup")
		}
		errs := make(chan error, 2)
		for i := 0; i < 2; i++ {
			go func() { _, e := s.Cleanup(ctx, proof); errs <- e }()
		}
		for i := 0; i < 2; i++ {
			if e := <-errs; e != nil {
				t.Fatal(e)
			}
		}
		for id, want := range map[string]bool{expired: true, held: false, active: false, applied: true, inUse: true} {
			var done bool
			_ = admin.QueryRow(ctx, `SELECT minimized_at IS NOT NULL FROM platform.catalog_imports WHERE id=$1`, id).Scan(&done)
			if done != want {
				t.Fatal("retention incorrect", id, done)
			}
		}
		var events int
		_ = admin.QueryRow(ctx, `SELECT count(*) FROM platform.catalog_events WHERE import_id=$1 AND kind='IMPORT_RETENTION_MINIMIZED'`, expired).Scan(&events)
		if events != 1 {
			t.Fatal("cleanup audit duplicate", events)
		}
		if _, e = s.Cleanup(ctx, proof); e != nil {
			t.Fatal(e)
		}
		_ = time.Second
	})
	t.Run("10B_cleanup_without_any_channel", func(t *testing.T) {
		proof := catalog.WorkerProof([]byte("SYNTHETIC-local-auth-key-32bytes"))
		if len(proof) != 32 {
			t.Fatal("synthetic key invalid")
		}
		scoped := catalog.Service{Pool: d.Pool, CleanupProof: proof}
		if _, e := scoped.SavePolicy(ctx, alien, catalog.Policy{FreshHours: 24, StaleHours: 72, LowStock: 3, PreviewDays: 1, ErrorDays: 7, ChangeDays: 90}); e != nil {
			t.Fatal(e)
		}
		job := auth.UUID()
		exec(`INSERT INTO platform.catalog_imports(id,organization_id,request_key,file_digest,grid,created_by,status,created_at,expires_at) VALUES($1,$2,$3,sha256('synthetic'::bytea),'[]',$4,'FAILED',now()-interval '10 days',now()-interval '2 days')`, job, alien.OrganizationID, auth.UUID(), alien.Session.User.ID)
		if n, e := scoped.Cleanup(ctx, proof); e != nil || n != 1 {
			t.Fatal("pre-channel cleanup", n, e)
		}
		raw, e := scoped.Control(ctx, alien)
		if e != nil {
			t.Fatal(e)
		}
		var result map[string]any
		_ = json.Unmarshal(raw, &result)
		if _, exists := result["policy"].(map[string]any)["cleanup_hash"]; exists {
			t.Fatal("internal capability hash in API")
		}
	})
	t.Run("10B_delayed_dispatch_rechecks_both_outboxes", func(t *testing.T) {
		for _, table := range []string{"telegram_outbox", "channel_outbox"} {
			tx, e := admin.Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			var id, org string
			var cards []byte
			e = tx.QueryRow(ctx, `SELECT q.id,q.organization_id,m.data->'cards' FROM platform.`+table+` q JOIN platform.messages m ON m.id=q.id AND m.organization_id=q.organization_id WHERE jsonb_typeof(m.data->'cards')='array' AND jsonb_array_length(m.data->'cards')>0 AND m.data->'cards'->0->>'fresh'='true' LIMIT 1`).Scan(&id, &org, &cards)
			if e != nil {
				_ = tx.Rollback(ctx)
				t.Fatal("missing actual grounded outbox", table, e)
			}
			_, e = tx.Exec(ctx, `UPDATE platform.inventory SET manual_updated_at=now()-interval '100 hours' WHERE organization_id=$1`, org)
			if e != nil {
				t.Fatal(e)
			}
			var payload []byte
			e = tx.QueryRow(ctx, `UPDATE platform.`+table+` SET status='sending' WHERE id=$1 RETURNING payload`, id).Scan(&payload)
			if e != nil {
				_ = tx.Rollback(ctx)
				t.Fatal(e)
			}
			var p map[string]any
			_ = json.Unmarshal(payload, &p)
			if p["text"] != "Наличие требует подтверждения магазина: данные изменились или устарели после подготовки ответа. Запросите актуальные сведения у сотрудника." {
				t.Fatal("queued stale promise", string(payload))
			}
			_ = tx.Rollback(ctx)
		}
	})
	t.Log("PASS10B: backend health, policy, owner/tenant, stale masking, fresh verification, source handoff concurrency/replay/stale preview/rollback, scoped cleanup/hold/audit")
}
