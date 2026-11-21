//go:build integration

package integration

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/agent"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/conversations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/orders"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/db"
)

func testStoreImport(t *testing.T, d *db.Database, admin *pgx.Conn, ctx context.Context) {
	t.Helper()
	s := catalog.Service{Pool: d.Pool}
	execSQL := func(q string, args ...any) {
		t.Helper()
		if _, e := admin.Exec(ctx, q, args...); e != nil {
			t.Fatal(e)
		}
	}
	uid, token := auth.UUID(), auth.Token()
	execSQL(`INSERT INTO platform.users(id,email,password_hash,display_name,email_verified_at) VALUES($1,$2,'synthetic','Import owner',now())`, uid, uid+"@import.example.test")
	execSQL(`INSERT INTO platform.sessions(id,user_id,token_hash,csrf_hash,expires_at,idle_expires_at) VALUES($1,$2,$3,$4,now()+interval '1 hour',now()+interval '1 hour')`, auth.UUID(), uid, auth.Hash(token), auth.Hash(auth.Token()))
	identity := auth.New(d.Pool, config.Auth{SessionTTL: time.Hour, IdleTTL: time.Hour})
	session, e := identity.Authenticate(ctx, token)
	if e != nil {
		t.Fatal(e)
	}
	org, e := (organizations.Service{Pool: d.Pool}).CreateTest(ctx, session, "SYNTHETIC Import A")
	if e != nil {
		t.Fatal(e)
	}
	other, e := (organizations.Service{Pool: d.Pool}).CreateTest(ctx, session, "SYNTHETIC Import B")
	if e != nil {
		t.Fatal(e)
	}
	tenant := catalog.Tenant{Session: session, OrganizationID: org.ID}
	alien := tenant
	alien.OrganizationID = other.ID
	staffID, staffToken := auth.UUID(), auth.Token()
	execSQL(`INSERT INTO platform.users(id,email,password_hash,display_name,email_verified_at) VALUES($1,$2,'synthetic','Import staff',now())`, staffID, staffID+"@import.example.test")
	execSQL(`INSERT INTO platform.sessions(id,user_id,token_hash,csrf_hash,expires_at,idle_expires_at) VALUES($1,$2,$3,$4,now()+interval '1 hour',now()+interval '1 hour')`, auth.UUID(), staffID, auth.Hash(staffToken), auth.Hash(auth.Token()))
	execSQL(`INSERT INTO platform.organization_members(organization_id,user_id,role,can_manage_orders) VALUES($1,$2,'member',true)`, org.ID, staffID)
	staffSession, e := identity.Authenticate(ctx, staffToken)
	if e != nil {
		t.Fatal(e)
	}
	staff := catalog.Tenant{Session: staffSession, OrganizationID: org.ID}
	fixture, e := os.ReadFile("../../internal/catalog/testdata/synthetic-electronics.xlsx")
	if e != nil {
		t.Fatal(e)
	}
	type Job struct {
		ID      string                `json:"id"`
		Status  string                `json:"status"`
		Version int64                 `json:"version"`
		Preview catalog.ImportPreview `json:"preview"`
	}
	decode := func(raw json.RawMessage, e error) Job {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
		var j Job
		if json.Unmarshal(raw, &j) != nil {
			t.Fatal("bad import JSON")
		}
		return j
	}
	upload := func(b []byte) Job { return decode(s.UploadXLSX(ctx, tenant, auth.UUID(), b)) }
	mapping := map[string]int{"sku": 0, "name": 1, "category": 2, "brand": 3, "model": 4, "description": 5, "price": 6, "currency": 7, "quantity": 8, "availability": 9, "attributes": 10}
	preview := func(j Job) Job { return decode(s.MapImport(ctx, tenant, j.ID, j.Version, mapping)) }
	count := func() int {
		var n int
		if e := admin.QueryRow(ctx, `SELECT count(*) FROM platform.products WHERE organization_id=$1`, org.ID).Scan(&n); e != nil {
			t.Fatal(e)
		}
		return n
	}
	edit := func(b []byte, from, to string) []byte {
		z, _ := zip.NewReader(bytes.NewReader(b), int64(len(b)))
		var out bytes.Buffer
		w := zip.NewWriter(&out)
		for _, f := range z.File {
			r, _ := f.Open()
			v, _ := io.ReadAll(r)
			_ = r.Close()
			if f.Name == "xl/worksheets/sheet1.xml" {
				v = []byte(strings.ReplaceAll(string(v), from, to))
			}
			p, _ := w.Create(f.Name)
			_, _ = p.Write(v)
		}
		_ = w.Close()
		return out.Bytes()
	}
	profile := catalog.StoreProfile{Name: "SYNTHETIC Electronics", BusinessCategory: "Электроника", Description: "Тестовые цены и остатки", Address: "Синтетический адрес", OpeningHours: "Тест 10–18", Contacts: "synthetic@example.test", DeliveryTerms: "Демонстрация", PaymentMethods: "Тестовая оплата", ReturnTerms: "Тестовый возврат", AIInstructions: "Использовать только каталог; не выдумывать цены"}
	t.Run("10A_profile_owner_and_no_implicit_tenant", func(t *testing.T) {
		if _, e := s.SaveProfile(ctx, staff, profile); e == nil {
			t.Fatal("staff profile write")
		}
		if _, e := s.SaveProfile(ctx, tenant, profile); e != nil {
			t.Fatal(e)
		}
		if _, e := s.SaveProfile(ctx, tenant, profile); e == nil {
			t.Fatal("stale profile accepted")
		}
		if _, e := s.OnboardingStep(ctx, tenant, "ACTIVE", 1, true); e == nil {
			t.Fatal("activated without catalog/review/channel")
		}
	})
	if _, e := s.Onboarding(ctx, tenant); e != nil {
		t.Fatal("onboarding state", e)
	}
	var a, b Job
	t.Run("10A_upload_mapping_no_catalog_write_and_auth", func(t *testing.T) {
		key := auth.UUID()
		a = decode(s.UploadXLSX(ctx, tenant, key, fixture))
		again := decode(s.UploadXLSX(ctx, tenant, key, fixture))
		if again.ID != a.ID {
			t.Fatal("upload not idempotent")
		}
		if _, e := s.UploadXLSX(ctx, tenant, key, []byte("different")); e == nil {
			t.Fatal("key reused with different bytes")
		}
		if _, e := s.UploadXLSX(ctx, staff, auth.UUID(), fixture); e == nil {
			t.Fatal("staff upload")
		}
		if _, e := s.ImportResult(ctx, alien, a.ID); e == nil {
			t.Fatal("foreign import read")
		}
		if _, e := s.ConfirmImport(ctx, alien, a.ID, a.Version, true); e == nil {
			t.Fatal("foreign confirm")
		}
		a = preview(a)
		if a.Preview.New != 6 || a.Preview.Errors != 0 || count() != 0 {
			t.Fatal("preview wrote catalog or wrong rows", a.Preview)
		}
		b = preview(upload(fixture))
		if count() != 0 {
			t.Fatal("mapping mutated catalog")
		}
	})
	t.Run("10A_rollback_worker_crash_before_commit", func(t *testing.T) {
		// A transaction-scoped fault models process/connection loss during a batch: no partial changes survive.
		execSQL(`CREATE FUNCTION platform.test_import_fail() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN IF NEW.sku='DEMO-CHARGER' THEN RAISE EXCEPTION 'synthetic transaction fault';END IF;RETURN NEW;END $$`)
		execSQL(`CREATE TRIGGER test_import_fail BEFORE INSERT ON platform.product_variants FOR EACH ROW EXECUTE FUNCTION platform.test_import_fail()`)
		if _, e := s.ConfirmImport(ctx, tenant, a.ID, a.Version, true); e == nil {
			t.Fatal("fault not injected")
		}
		if count() != 0 {
			t.Fatal("partial import survived rollback")
		}
		same := decode(s.ImportResult(ctx, tenant, a.ID))
		if same.Status != "PREVIEW" || same.Version != a.Version {
			t.Fatal("job committed partial state")
		}
		execSQL(`DROP TRIGGER test_import_fail ON platform.product_variants`)
		execSQL(`DROP FUNCTION platform.test_import_fail()`)
	})
	t.Run("10A_concurrent_imports_and_duplicate_confirmation", func(t *testing.T) {
		var wg sync.WaitGroup
		type Result struct {
			job Job
			e   error
		}
		results := make(chan Result, 2)
		for _, j := range []Job{a, b} {
			wg.Add(1)
			go func(j Job) {
				defer wg.Done()
				_, e := s.ConfirmImport(ctx, tenant, j.ID, j.Version, true)
				results <- Result{j, e}
			}(j)
		}
		wg.Wait()
		close(results)
		success := 0
		var winner Job
		for r := range results {
			if r.e == nil {
				success++
				winner = r.job
			}
		}
		if success != 1 || count() != 6 {
			t.Fatal("concurrent import safety", success, count())
		}
		for n := 0; n < 2; n++ {
			if _, e := s.ConfirmImport(ctx, tenant, winner.ID, winner.Version, true); e != nil {
				t.Fatal("double-click replay", e)
			}
		}
		// Clear the unused stale preview without changing catalog.
		loser := a
		if winner.ID == a.ID {
			loser = b
		}
		if _, e := s.CancelImport(ctx, tenant, loser.ID, loser.Version); e != nil {
			t.Fatal(e)
		}
	})
	t.Run("10A_repeat_unchanged_and_source_policy", func(t *testing.T) {
		j := preview(upload(fixture))
		if j.Preview.Unchanged != 6 || j.Preview.New != 0 {
			t.Fatal("repeated import duplicates", j.Preview)
		}
		if _, e := s.ConfirmImport(ctx, tenant, j.ID, j.Version, true); e != nil {
			t.Fatal(e)
		}
		var vid string
		var v int64
		_ = admin.QueryRow(ctx, `SELECT id,version FROM platform.product_variants WHERE organization_id=$1 AND sku='DEMO-IPHONE'`, org.ID).Scan(&vid, &v)
		if _, e := s.Variant(ctx, tenant, "", vid, catalog.Input{SKU: "DEMO-IPHONE", PriceAmount: 1, ExpectedVersion: v}); e == nil {
			t.Fatal("manual overwrote EXCEL price")
		}
		if _, e := s.CreateProduct(ctx, tenant, catalog.Input{Name: "Manual stays supported", SKU: "MANUAL-NEW", PriceAmount: 100, CurrencyCode: "RUB"}); e != nil {
			t.Fatal("manual regression", e)
		}
	})
	t.Run("10A_stale_preview_then_updated_prices_stock", func(t *testing.T) {
		changed := edit(fixture, "59990.00", "60000.00")
		j := preview(upload(changed))
		if j.Preview.Updated != 1 {
			t.Fatal("updated price not detected", j.Preview)
		}
		if _, e := s.CreateProduct(ctx, tenant, catalog.Input{Name: "Concurrent manual", SKU: "MANUAL-CONCURRENT", PriceAmount: 100}); e != nil {
			t.Fatal(e)
		}
		if _, e := s.ConfirmImport(ctx, tenant, j.ID, j.Version, true); e == nil {
			t.Fatal("stale preview applied")
		}
		j = preview(j)
		if _, e := s.ConfirmImport(ctx, tenant, j.ID, j.Version, true); e != nil {
			t.Fatal(e)
		}
	})
	for _, mode := range []string{"duplicate", "negative_stock", "negative_price", "currency", "manual_conflict"} {
		t.Run("10A_invalid_"+mode, func(t *testing.T) {
			bad := fixture
			switch mode {
			case "duplicate":
				bad = edit(bad, "DEMO-SAMSUNG", "DEMO-IPHONE")
			case "negative_stock":
				bad = edit(bad, ">5<", ">-1<")
			case "negative_price":
				bad = edit(bad, "59990.00", "-5")
			case "currency":
				bad = edit(bad, ">RUB<", ">USD<")
			case "manual_conflict":
				bad = edit(bad, "DEMO-IPHONE", "MANUAL-NEW")
			}
			j := preview(upload(bad))
			if j.Preview.Errors+j.Preview.Conflicts+j.Preview.Duplicates == 0 {
				t.Fatal("invalid row unflagged", mode)
			}
			before := count()
			if _, e := s.ConfirmImport(ctx, tenant, j.ID, j.Version, true); e == nil || count() != before {
				t.Fatal("invalid batch applied")
			}
			if _, e := s.CancelImport(ctx, tenant, j.ID, j.Version); e != nil {
				t.Fatal(e)
			}
		})
	}
	t.Run("10A_reserved_stock_and_future_ONE_C", func(t *testing.T) {
		var vid string
		_ = admin.QueryRow(ctx, `SELECT id FROM platform.product_variants WHERE organization_id=$1 AND sku='DEMO-IPHONE'`, org.ID).Scan(&vid)
		execSQL(`UPDATE platform.inventory SET quantity_reserved=2 WHERE variant_id=$1`, vid)
		bad := edit(fixture, ">5<", ">1<")
		j := preview(upload(bad))
		if j.Preview.Conflicts == 0 {
			t.Fatal("reserved stock overwritten")
		}
		_, _ = s.CancelImport(ctx, tenant, j.ID, j.Version)
		execSQL(`UPDATE platform.inventory SET quantity_reserved=0 WHERE variant_id=$1`, vid)
		// ONE_C is a reserved source contract only. No adapter/network is invoked.
		p, v := auth.UUID(), auth.UUID()
		execSQL(`SELECT set_config('app.catalog_writer','ONE_C',false)`)
		execSQL(`INSERT INTO platform.products(id,organization_id,name,catalog_source) VALUES($1,$2,'Synthetic ONE_C','ONE_C')`, p, org.ID)
		execSQL(`INSERT INTO platform.product_variants(id,organization_id,product_id,sku,variant_name,price_amount,catalog_source) VALUES($1,$2,$3,'ONEC-TEST','Основной',100,'ONE_C')`, v, org.ID, p)
		execSQL(`INSERT INTO platform.inventory(organization_id,variant_id,catalog_source) VALUES($1,$2,'ONE_C')`, org.ID, v)
		execSQL(`SELECT set_config('app.catalog_writer','MANUAL',false)`)
		if _, e := s.Variant(ctx, tenant, "", v, catalog.Input{SKU: "ONEC-TEST", PriceAmount: 500, ExpectedVersion: 1}); e == nil {
			t.Fatal("manual overwrote ONE_C")
		}
		j = preview(upload(edit(fixture, "DEMO-IPHONE", "ONEC-TEST")))
		if j.Preview.Conflicts == 0 {
			t.Fatal("Excel overwrote ONE_C")
		}
		_, _ = s.CancelImport(ctx, tenant, j.ID, j.Version)
	})
	t.Run("10A_imported_catalog_grounding_consent_staff_reserve_consume", func(t *testing.T) {
		var pid, vid string
		var pv int64
		if e := admin.QueryRow(ctx, `SELECT p.id,v.id,p.version FROM platform.products p JOIN platform.product_variants v ON v.product_id=p.id WHERE p.organization_id=$1 AND v.sku='DEMO-SAMSUNG'`, org.ID).Scan(&pid, &vid, &pv); e != nil {
			t.Fatal(e)
		}
		if _, e := s.UpdateProduct(ctx, tenant, pid, catalog.Input{ExpectedVersion: pv}, "publish"); e != nil {
			t.Fatal(e)
		}
		raw, e := s.Customer().GetProductPrice(ctx, tenant, vid)
		if e != nil || !strings.Contains(string(raw), "3999000") {
			t.Fatal("grounding imported price", e)
		}
		cfg, _ := config.LoadAgent(func(string) string { return "" })
		chat := conversations.Service{Pool: d.Pool, Config: cfg, Provider: agent.Mock{}}
		raw, e = chat.Create(ctx, tenant)
		if e != nil {
			t.Fatal(e)
		}
		var conv struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(raw, &conv)
		orderSvc := orders.Service{Pool: d.Pool, StockFreshness: 24 * time.Hour}
		quote, e := orderSvc.PrepareQuote(ctx, tenant, conv.ID, []orders.CartItem{{VariantID: vid, Quantity: 1}})
		if e != nil {
			t.Fatal(e)
		}
		consent := orders.Consent{QuoteID: quote.ID, ConversationID: conv.ID, IdempotencyKey: auth.UUID(), Accepted: false}
		if _, e = orderSvc.CreateRequest(ctx, tenant, consent); e == nil {
			t.Fatal("consent bypass")
		}
		consent.Accepted = true
		raw, e = orderSvc.CreateRequest(ctx, tenant, consent)
		if e != nil {
			t.Fatal(e)
		}
		var order struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(raw, &order)
		if _, e = orderSvc.Change(ctx, staff, order.ID, "confirm", orders.Transition{ExpectedVersion: 1, IdempotencyKey: auth.UUID()}); e != nil {
			t.Fatal(e)
		}
		if _, e = orderSvc.Change(ctx, staff, order.ID, "complete", orders.Transition{ExpectedVersion: 2, IdempotencyKey: auth.UUID()}); e != nil {
			t.Fatal(e)
		}
		var on, res int
		var source string
		_ = admin.QueryRow(ctx, `SELECT quantity_on_hand,quantity_reserved,catalog_source FROM platform.inventory WHERE variant_id=$1`, vid).Scan(&on, &res, &source)
		if on != 6 || res != 0 || source != "EXCEL" {
			t.Fatal("external inventory consumption", on, res, source)
		}
	})
	testCatalogControl(t, d, admin, ctx, tenant, alien, staff)
	testCatalogScale(t, d, admin, ctx, tenant, alien, staff)
	t.Log("PASS10A: owner profile, XLSX upload/mapping/preview, rollback, concurrent stale fencing, replay, source conflicts, reserved stock, tenant/RLS; no external API")
}
