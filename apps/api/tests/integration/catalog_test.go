//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	httpapi "github.com/karadjev/AI-Business-Assistant/apps/api/internal/http"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/db"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func testCatalog(t *testing.T, database *db.Database, admin *pgx.Conn, ctx context.Context) {
	s := catalog.Service{Pool: database.Pool}
	orgs := organizations.Service{Pool: database.Pool}
	identity := auth.New(database.Pool, config.Auth{Origin: "http://127.0.0.1:3000", SessionTTL: time.Hour, IdleTTL: time.Hour})
	h := httpapi.NewHandler(database, slog.New(slog.NewJSONHandler(io.Discard, nil)), 5*time.Second, 16384, identity)
	type fixture struct {
		tenant      catalog.Tenant
		token, csrf string
	}
	makeFixture := func(name string) fixture {
		uid := auth.UUID()
		token, csrf := auth.Token(), auth.Token()
		_, e := admin.Exec(ctx, `INSERT INTO platform.users(id,email,password_hash,display_name,email_verified_at) VALUES($1,$2,'not-a-login-hash',$3,now())`, uid, name+"@catalog.example.test", name)
		if e != nil {
			t.Fatal(e)
		}
		_, e = admin.Exec(ctx, `INSERT INTO platform.sessions(id,user_id,token_hash,csrf_hash,expires_at,idle_expires_at) VALUES($1,$2,$3,$4,now()+interval '1 hour',now()+interval '1 hour')`, auth.UUID(), uid, auth.Hash(token), auth.Hash(csrf))
		if e != nil {
			t.Fatal(e)
		}
		session, e := identity.Authenticate(ctx, token)
		if e != nil {
			t.Fatal(e)
		}
		o, e := orgs.Create(ctx, session, name, "")
		if e != nil {
			t.Fatal(e)
		}
		return fixture{catalog.Tenant{Session: session, OrganizationID: o.ID}, token, csrf}
	}
	a, b := makeFixture("catalog-a"), makeFixture("catalog-b")
	req := func(f fixture, method, path string, body any, expected int) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(method, "/api/v1/organizations/"+f.tenant.OrganizationID+path, bytes.NewReader(raw))
		r.AddCookie(&http.Cookie{Name: "aba_session", Value: f.token})
		r.Header.Set("Origin", identity.Config.Origin)
		r.Header.Set("X-CSRF-Token", f.csrf)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != expected {
			t.Fatalf("%s %s expected %d got %d: %s", method, path, expected, w.Code, w.Body.String())
		}
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return out
	}
	getID := func(out map[string]any) string { return out["id"].(string) }
	cat := req(a, "POST", "/categories", map[string]any{"name": "Телефоны"}, 201)
	category := getID(cat)
	req(a, "PATCH", "/categories/"+category, map[string]any{"name": "Смартфоны", "expected_version": 1}, 200)
	req(a, "POST", "/categories", map[string]any{"name": "Смартфоны"}, 409)
	req(a, "GET", "/categories", nil, 200)
	create := func(f fixture, sku string, price int64) map[string]any {
		return req(f, "POST", "/products", map[string]any{"name": "Samsung Galaxy", "brand": "Samsung", "sku": sku, "price_amount": price}, 201)
	}
	p := create(a, "s25", 12345)
	id := getID(p)
	variants := p["variants"].([]any)
	if len(variants) != 1 {
		t.Fatal("default variant missing")
	}
	v := variants[0].(map[string]any)
	vid := getID(v)
	if v["price_amount"].(float64) != 12345 || v["sku"] != "S25" {
		t.Fatal("minor price or SKU normalization")
	}
	spoofOrg := a
	spoofOrg.tenant.OrganizationID = b.tenant.OrganizationID
	req(spoofOrg, "GET", "/products", nil, 404)
	req(spoofOrg, "POST", "/categories", map[string]any{"name": "Foreign"}, 404)
	noSession := a
	noSession.token = ""
	req(noSession, "GET", "/products", nil, 401)
	noCSRF := a
	noCSRF.csrf = ""
	req(noCSRF, "POST", "/categories", map[string]any{"name": "CSRF"}, 403)
	foreign := create(b, "S25", 45600)
	foreignID := getID(foreign)
	req(a, "GET", "/products/"+foreignID, nil, 404)
	req(a, "PATCH", "/products/"+foreignID, map[string]any{"name": "Hack", "expected_version": 1}, 404)
	req(a, "POST", "/products", map[string]any{"name": "Spoof", "sku": "BAD", "price_amount": 1, "organization_id": b.tenant.OrganizationID}, 422)
	req(a, "POST", "/products", map[string]any{"name": "Duplicate", "sku": "S25", "price_amount": 1}, 409)
	req(a, "POST", "/products", map[string]any{"name": "Invalid", "sku": "NEG", "price_amount": -1}, 422)
	req(a, "POST", "/products", map[string]any{"name": "Invalid", "sku": "DECIMAL", "price_amount": 1.5}, 400)
	req(a, "POST", "/products", map[string]any{"name": "Invalid", "sku": "ARRAY", "price_amount": 1, "attributes": []string{"x"}}, 400)
	req(a, "PATCH", "/products/"+id, map[string]any{"name": "Galaxy S25", "brand": "Samsung", "category_id": category, "description": "<script>text only</script>", "expected_version": 1}, 200)
	req(a, "POST", "/products/"+id+"/publish", map[string]any{"expected_version": 1}, 409)
	if _, e := s.Customer().GetProductDetails(ctx, a.tenant, id); e == nil {
		t.Fatal("draft exposed")
	}
	if _, e := s.Customer().GetProductPrice(ctx, a.tenant, vid); e == nil {
		t.Fatal("draft price exposed")
	}
	req(a, "POST", "/products/"+id+"/publish", map[string]any{"expected_version": 2}, 200)
	req(a, "PATCH", "/variants/"+vid, map[string]any{"sku": "S25", "price_amount": 22345, "attributes": map[string]string{"memory": "256GB"}, "expected_version": 1}, 200)
	req(a, "PATCH", "/variants/"+vid, map[string]any{"sku": "S25", "price_amount": 22345, "expected_version": 1}, 409)
	req(a, "PATCH", "/variants/"+vid, map[string]any{"sku": "S25", "price_amount": 0, "expected_version": 2}, 422)
	req(a, "POST", "/products/"+id+"/variants", map[string]any{"sku": "S25-512", "price_amount": 32345, "variant_name": "512GB"}, 201)
	req(a, "GET", "/products/"+id+"/variants", nil, 200)
	stock := req(a, "POST", "/inventory/adjustments", map[string]any{"variant_id": vid, "quantity_on_hand": 10, "expected_version": 1, "reason": "Начальный остаток"}, 200)
	if stock["available"].(float64) != 10 {
		t.Fatal("stock")
	}
	req(a, "POST", "/inventory/adjustments", map[string]any{"variant_id": vid, "quantity_on_hand": -1, "expected_version": 2, "reason": "bad"}, 422)
	var wg sync.WaitGroup
	out := make(chan error, 2)
	start := make(chan struct{})
	for _, qty := range []int64{11, 12} {
		wg.Add(1)
		go func(q int64) {
			defer wg.Done()
			<-start
			_, e := s.Adjust(ctx, a.tenant, catalog.Input{VariantID: vid, QuantityOnHand: q, ExpectedVersion: 2, Reason: "Concurrent"})
			out <- e
		}(qty)
	}
	close(start)
	wg.Wait()
	close(out)
	ok, conflicts := 0, 0
	for e := range out {
		if e == nil {
			ok++
		} else {
			var ae *auth.Error
			if errors.As(e, &ae) && ae.Status == 409 {
				conflicts++
			} else {
				t.Fatal(e)
			}
		}
	}
	if ok != 1 || conflicts != 1 {
		t.Fatalf("lost update: successes %d conflicts %d", ok, conflicts)
	}
	_, e := admin.Exec(ctx, `UPDATE platform.inventory SET quantity_reserved=5 WHERE variant_id=$1`, vid)
	if e != nil {
		t.Fatal(e)
	}
	req(a, "POST", "/inventory/adjustments", map[string]any{"variant_id": vid, "quantity_on_hand": 4, "expected_version": 3, "reason": "below reserved"}, 422)
	movements := req(a, "GET", "/inventory/movements", nil, 200)["items"].([]any)
	if len(movements) != 2 {
		t.Fatal("history not atomic")
	}
	for _, m := range movements {
		row := m.(map[string]any)
		if row["actor_id"] != a.tenant.Session.User.ID || row["delta"].(float64) != row["quantity_after"].(float64)-row["quantity_before"].(float64) {
			t.Fatal("audit incorrect")
		}
	}
	req(a, "GET", "/inventory", nil, 200)
	found := req(a, "GET", "/products?q=S25-512&category_id="+category+"&status=active&stock=in_stock&sort=price_asc&page_size=1", nil, 200)
	if found["total"].(float64) != 1 {
		t.Fatal("search/filter")
	}
	if len(req(a, "GET", "/products?q=absent", nil, 200)["items"].([]any)) != 0 {
		t.Fatal("search")
	}
	req(a, "GET", "/products?page_size=101", nil, 422)
	req(a, "GET", "/products?page=0", nil, 422)
	for _, read := range []func() (json.RawMessage, error){func() (json.RawMessage, error) { return s.Customer().SearchProducts(ctx, a.tenant, catalog.Filters{}) }, func() (json.RawMessage, error) { return s.Customer().GetProductDetails(ctx, a.tenant, id) }, func() (json.RawMessage, error) { return s.Customer().GetProductPrice(ctx, a.tenant, vid) }, func() (json.RawMessage, error) { return s.Customer().CheckInventory(ctx, a.tenant, vid) }} {
		if _, e := read(); e != nil {
			t.Fatal(e)
		}
	}
	// Real member fixture: B remains owner of B, read-only member of A.
	_, e = admin.Exec(ctx, `INSERT INTO platform.organization_members(organization_id,user_id,role) VALUES($1,$2,'member')`, a.tenant.OrganizationID, b.tenant.Session.User.ID)
	if e != nil {
		t.Fatal(e)
	}
	member := b
	member.tenant.OrganizationID = a.tenant.OrganizationID
	req(member, "GET", "/products/"+id, nil, 200)
	req(member, "POST", "/categories", map[string]any{"name": "Forbidden"}, 403)
	req(member, "POST", "/inventory/adjustments", map[string]any{"variant_id": vid, "quantity_on_hand": 20, "expected_version": 3, "reason": "member"}, 403)
	org2, e := orgs.Create(ctx, a.tenant.Session, "A second", "")
	if e != nil {
		t.Fatal(e)
	}
	second := a
	second.tenant.OrganizationID = org2.ID
	create(second, "S25", 1)
	req(second, "GET", "/products/"+id, nil, 404)
	scoped := func(f fixture) pgx.Tx {
		tx, e := database.Pool.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		_, e = tx.Exec(ctx, `SELECT set_config('app.session_hash',$1,true),set_config('app.organization_id',$2,true)`, hex.EncodeToString(f.tenant.Session.Hash), f.tenant.OrganizationID)
		if e != nil {
			t.Fatal(e)
		}
		return tx
	}
	for _, table := range []string{"product_categories", "products", "product_variants", "inventory", "inventory_movements"} {
		var force bool
		e = admin.QueryRow(ctx, `SELECT relrowsecurity AND relforcerowsecurity AND pg_get_userbyid(relowner)='aba_migrator' FROM pg_class WHERE oid=('platform.'||$1)::regclass`, table).Scan(&force)
		if e != nil || !force {
			t.Fatal("unsafe table", table, e)
		}
		tx := scoped(a)
		var n int
		e = tx.QueryRow(ctx, `SELECT count(*) FROM platform.`+table+` WHERE organization_id=$1`, b.tenant.OrganizationID).Scan(&n)
		if e != nil || n != 0 {
			t.Fatal("foreign SQL read", table, e)
		}
		tag, e := tx.Exec(ctx, `UPDATE platform.`+table+` SET organization_id=organization_id WHERE organization_id=$1`, b.tenant.OrganizationID)
		if table == "inventory_movements" {
			if e == nil {
				t.Fatal("mutable audit")
			}
		} else if e != nil || tag.RowsAffected() != 0 {
			t.Fatal("foreign update", table, e)
		}
		_ = tx.Rollback(ctx)
		tx = scoped(a)
		if _, e = tx.Exec(ctx, `DELETE FROM platform.`+table); e == nil {
			t.Fatal("runtime DELETE permitted", table)
		}
		_ = tx.Rollback(ctx)
	}
	for _, sql := range []string{`INSERT INTO platform.products(id,organization_id,name) VALUES($1,$2,'spoof')`, `UPDATE platform.products SET organization_id=$2 WHERE id=$1`} {
		tx := scoped(a)
		pid := id
		if sql[0] == 'I' {
			pid = auth.UUID()
		}
		if _, e = tx.Exec(ctx, sql, pid, b.tenant.OrganizationID); e == nil {
			t.Fatal("tenant spoof accepted")
		}
		_ = tx.Rollback(ctx)
	}
	// Cross-tenant references must fail even when both organizations are owned by same user.

	// INSERT WITH CHECK applies to every tenant-owned table, not only products.
	foreignVariant := foreign["variants"].([]any)[0].(map[string]any)["id"].(string)
	attacks := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO platform.product_categories(id,organization_id,name) VALUES($1,$2,'spoof category')`, []any{auth.UUID(), b.tenant.OrganizationID}},
		{`INSERT INTO platform.product_variants(id,organization_id,product_id,sku,variant_name,price_amount) VALUES($1,$2,$3,'SPOOF','spoof',1)`, []any{auth.UUID(), b.tenant.OrganizationID, foreignID}},
		{`INSERT INTO platform.inventory(organization_id,variant_id) VALUES($1,$2)`, []any{b.tenant.OrganizationID, foreignVariant}},
		{`INSERT INTO platform.inventory_movements(id,organization_id,variant_id,actor_id,reason,quantity_before,quantity_after,delta,inventory_version) VALUES($1,$2,$3,$4,'spoof',0,1,1,99)`, []any{auth.UUID(), b.tenant.OrganizationID, foreignVariant, a.tenant.Session.User.ID}},
	}
	for _, attack := range attacks {
		tx := scoped(a)
		if _, e = tx.Exec(ctx, attack.sql, attack.args...); e == nil {
			t.Fatal("foreign INSERT permitted")
		}
		_ = tx.Rollback(ctx)
	}
	for _, table := range []string{"product_categories", "product_variants", "inventory"} {
		tx := scoped(a)
		if _, e = tx.Exec(ctx, `UPDATE platform.`+table+` SET organization_id=$1`, b.tenant.OrganizationID); e == nil {
			t.Fatal("foreign UPDATE WITH CHECK permitted", table)
		}
		_ = tx.Rollback(ctx)
	}
	for _, quantity := range []int{-1, 4} {
		tx := scoped(a)
		if _, e = tx.Exec(ctx, `UPDATE platform.inventory SET quantity_on_hand=$2 WHERE variant_id=$1`, vid, quantity); e == nil {
			t.Fatal("inventory CHECK failed")
		}
		_ = tx.Rollback(ctx)
	}
	// Hold both physical connections simultaneously; SET LOCAL must reset on both paths.
	conn1, e := database.Pool.Acquire(ctx)
	if e != nil {
		t.Fatal(e)
	}
	conn2, e := database.Pool.Acquire(ctx)
	if e != nil {
		t.Fatal(e)
	}
	for i, c := range []*pgxpool.Conn{conn1, conn2} {
		tx, e := c.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		f := a
		if i == 1 {
			f = b
		}
		_, e = tx.Exec(ctx, `SELECT set_config('app.session_hash',$1,true),set_config('app.organization_id',$2,true)`, hex.EncodeToString(f.tenant.Session.Hash), f.tenant.OrganizationID)
		if e != nil {
			t.Fatal(e)
		}
		var count int
		if e = tx.QueryRow(ctx, `SELECT count(*) FROM platform.products WHERE organization_id=$1`, f.tenant.OrganizationID).Scan(&count); e != nil || count != 1 {
			t.Fatal("physical tenant query", e, count)
		}
		if i == 0 {
			e = tx.Commit(ctx)
		} else {
			e = tx.Rollback(ctx)
		}
		if e != nil {
			t.Fatal(e)
		}
		if e = c.QueryRow(ctx, `SELECT count(*) FROM platform.products`).Scan(&count); e != nil || count != 0 {
			t.Fatal("physical connection context leak")
		}
	}
	conn1.Release()
	conn2.Release()
	tx := scoped(second)
	if _, e = tx.Exec(ctx, `INSERT INTO platform.products(id,organization_id,category_id,name) VALUES($1,$2,$3,'cross FK')`, auth.UUID(), org2.ID, category); e == nil {
		t.Fatal("cross FK accepted")
	}
	_ = tx.Rollback(ctx)
	tx = scoped(member)
	tag, e := tx.Exec(ctx, `UPDATE platform.products SET name='member' WHERE id=$1`, id)
	if e != nil || tag.RowsAffected() != 0 {
		t.Fatal("member SQL write")
	}
	_ = tx.Rollback(ctx)
	for _, f := range []fixture{a, b, second} {
		tx = scoped(f)
		var n int
		_ = tx.QueryRow(ctx, `SELECT count(*) FROM platform.products`).Scan(&n)
		if n == 0 {
			t.Fatal("fixture missing")
		}
		if e = tx.Commit(ctx); e != nil {
			t.Fatal(e)
		}
		if e = database.Pool.QueryRow(ctx, `SELECT count(*) FROM platform.products`).Scan(&n); e != nil || n != 0 {
			t.Fatal("tenant context leak")
		}
	}
	req(a, "POST", "/products/"+id+"/archive", map[string]any{"expected_version": 3}, 200)
	if _, e = s.Customer().GetProductDetails(ctx, a.tenant, id); e == nil {
		t.Fatal("archive exposed")
	}
	if _, e = s.Customer().CheckInventory(ctx, a.tenant, vid); e == nil {
		t.Fatal("archive stock exposed")
	}
	reads, e := s.Customer().SearchProducts(ctx, a.tenant, catalog.Filters{})
	if e != nil {
		t.Fatal(e)
	}
	var empty struct{ Items []any }
	_ = json.Unmarshal(reads, &empty)
	if len(empty.Items) != 0 {
		t.Fatal("archive in search")
	}
	req(a, "POST", "/products", map[string]any{"name": "Reuse archived SKU", "sku": "S25", "price_amount": 1}, 409)
	req(a, "PATCH", "/products/"+id, map[string]any{"name": "restore", "expected_version": 4}, 409)
	zero := create(a, "ZERO", 0)
	req(a, "POST", "/products/"+getID(zero)+"/publish", map[string]any{"expected_version": 1}, 422)
	page1 := req(a, "GET", "/products?page_size=1&page=1", nil, 200)
	page2 := req(a, "GET", "/products?page_size=1&page=2", nil, 200)
	if page1["total"].(float64) != 2 || len(page1["items"].([]any)) != 1 || len(page2["items"].([]any)) != 1 || page1["items"].([]any)[0].(map[string]any)["id"] == page2["items"].([]any)[0].(map[string]any)["id"] {
		t.Fatal("pagination")
	}
	t.Log("PASS: catalog API, FORCE RLS, two tenants/member, integer prices, concurrent inventory and customer reads")
}
