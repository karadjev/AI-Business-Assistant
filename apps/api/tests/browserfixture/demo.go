package main

// Privileged, isolated fixture only: no HTTP endpoint and no production binary.
import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/orders"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/db"
	"net/url"
	"os"
	"time"
)

func demoID(run, label string) string {
	b := sha256.Sum256([]byte("synthetic-demo-v1:" + run + ":" + label))
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	s := hex.EncodeToString(b[:16])
	return s[:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:]
}
func seedDemo(ctx context.Context, c *pgx.Conn, run string) error {
	if !organizations.ValidID(run) {
		return fmt.Errorf("demo requires a synthetic run UUID")
	}
	var marker string
	if e := c.QueryRow(ctx, `SELECT marker FROM platform.test_safety WHERE singleton`).Scan(&marker); e != nil || marker != "aba-synthetic-isolated" {
		return fmt.Errorf("isolated safety marker required")
	}
	if _, e := c.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended($1,10104))`, run); e != nil {
		return e
	}
	defer c.Exec(context.Background(), `SELECT pg_advisory_unlock(hashtextextended($1,10104))`, run)
	tx, e := c.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,10104))`, run); e != nil {
		return e
	}
	org := demoID(run, "store")
	pw := "Synthetic-Demo-Only-Password-123!"
	users := map[string]User{}
	for _, role := range []string{"owner", "staff", "readonly"} {
		u := User{ID: demoID(run, role), Email: "demo-" + role + "-" + run + "@example.test", Password: pw}
		users[role] = u
		if _, e = tx.Exec(ctx, `INSERT INTO platform.users(id,email,password_hash,display_name,email_verified_at) VALUES($1,$2,$3,$4,now()) ON CONFLICT DO NOTHING`, u.ID, u.Email, auth.PasswordHash(pw), "SYNTHETIC demo "+role); e != nil {
			return e
		}
	}
	if _, e = tx.Exec(ctx, `INSERT INTO platform.organizations(id,name,is_test) VALUES($1,'SYNTHETIC · Электроника · демонстрация',true) ON CONFLICT DO NOTHING`, org); e != nil {
		return e
	}
	var safe bool
	if e = tx.QueryRow(ctx, `SELECT is_test AND name='SYNTHETIC · Электроника · демонстрация' FROM platform.organizations WHERE id=$1`, org).Scan(&safe); e != nil || !safe {
		return fmt.Errorf("demo scope mismatch")
	}
	for role, u := range users {
		dbrole := "member"
		if role == "owner" {
			dbrole = "owner"
		}
		if _, e = tx.Exec(ctx, `INSERT INTO platform.organization_members(organization_id,user_id,role,can_manage_orders) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, org, u.ID, dbrole, role != "readonly"); e != nil {
			return e
		}
	}
	type item struct {
		sku, name, category, brand, description, attrs string
		price                                          int64
		stock                                          int
		age                                            string
	}
	items := []item{
		{"DEMO-S25", "Galaxy S25 256 ГБ · SYNTHETIC", "Смартфоны", "Samsung", "Тестовая модель: AMOLED, 256 ГБ, гарантия магазина", `{"memory":"256 ГБ","color":"синий"}`, 6999000, 8, "0 hours"},
		{"DEMO-IPHONE", "iPhone 16 128 ГБ · SYNTHETIC", "Смартфоны", "Apple", "Только демонстрация цены и устаревшего наличия", `{"memory":"128 ГБ","color":"чёрный"}`, 7999000, 4, "100 hours"},
		{"DEMO-CASE", "Чехол Galaxy S25 · SYNTHETIC", "Аксессуары", "Demo", "Нет на складе", `{"material":"силикон"}`, 99000, 0, "0 hours"},
		{"DEMO-CHARGER", "Зарядное устройство 30W · SYNTHETIC", "Аксессуары", "Demo", "USB-C Power Delivery, кабель отдельно", `{"power":"30W"}`, 249000, 12, "0 hours"},
		{"DEMO-AUDIO", "Беспроводные наушники · SYNTHETIC", "Аудио", "Demo", "Шумоподавление, Bluetooth", `{"connection":"Bluetooth"}`, 499000, 6, "0 hours"},
		{"DEMO-TABLET", "Планшет 11 дюймов · SYNTHETIC", "Планшеты", "Demo", "Неизвестная дата сверки", `{"screen":"11 дюймов"}`, 2999000, 2, ""},
		{"DEMO-CABLE", "Кабель USB-C · SYNTHETIC", "Аксессуары", "", "", `{}`, 0, 5, "0 hours"},
	}
	for _, it := range items {
		cat := demoID(run, it.category)
		pid := demoID(run, "product:"+it.sku)
		vid := demoID(run, "variant:"+it.sku)
		if _, e = tx.Exec(ctx, `INSERT INTO platform.product_categories(id,organization_id,name) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, cat, org, it.category); e != nil {
			return e
		}
		status := "active"
		if it.price == 0 {
			status = "draft"
		}
		if _, e = tx.Exec(ctx, `INSERT INTO platform.products(id,organization_id,category_id,name,brand,description,status) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING`, pid, org, cat, it.name, it.brand, it.description, status); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO platform.product_variants(id,organization_id,product_id,sku,variant_name,attributes,price_amount) VALUES($1,$2,$3,$4,'Демонстрационный',$5,$6) ON CONFLICT DO NOTHING`, vid, org, pid, it.sku, it.attrs, it.price); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO platform.inventory(organization_id,variant_id,quantity_on_hand,manual_updated_at) VALUES($1,$2,$3,CASE WHEN $4='' THEN NULL ELSE now()-$4::interval END) ON CONFLICT DO NOTHING`, org, vid, it.stock, it.age); e != nil {
			return e
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return e
	}
	output := func() error {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"org": org, "owner": users["owner"], "staff": users["staff"], "member": users["readonly"], "variant": demoID(run, "variant:DEMO-S25"), "run": run})
	}
	var complete bool
	if e = c.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM platform.catalog_events WHERE organization_id=$1 AND kind='DEMO_FIXTURE_READY' AND resource_id=$2)`, org, run).Scan(&complete); e != nil {
		return e
	}
	if complete {
		return output()
	}
	// Sample statuses use the real consent/order/reservation service, never direct order writes.
	parsed, e := url.Parse(os.Getenv("DATABASE_URL"))
	if e != nil || parsed.Hostname() != "postgres" || parsed.Path != "/aba_test" || (func() string {
		if parsed.User == nil {
			return ""
		}
		return parsed.User.Username()
	})() != "aba_app" {
		return fmt.Errorf("isolated application DSN required")
	}
	database, e := db.Open(ctx, parsed.String(), 2, 2*time.Second)
	if e != nil {
		return e
	}
	defer database.Close()
	token := auth.Token()
	hash := auth.Hash(token)
	if _, e = c.Exec(ctx, `INSERT INTO platform.sessions(id,user_id,token_hash,csrf_hash,expires_at,idle_expires_at) VALUES($1,$2,$3,$4,now()+interval '1 hour',now()+interval '1 hour')`, auth.UUID(), users["owner"].ID, hash, auth.Hash(auth.Token())); e != nil {
		return e
	}
	tenant := catalog.Tenant{OrganizationID: org, Session: auth.Session{Hash: hash, User: auth.User{ID: users["owner"].ID}}}
	svc := orders.Service{Pool: database.Pool, StockFreshness: 24 * time.Hour}
	for _, state := range []string{"pending_confirmation", "rejected", "confirmed", "completed"} {
		conv := demoID(run, "sample:"+state)
		customer := demoID(run, "customer:"+state)
		if _, e = c.Exec(ctx, `INSERT INTO platform.conversations(id,organization_id,customer_id,created_by) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, conv, org, customer, users["owner"].ID); e != nil {
			return e
		}
		var oid, status string
		var version int64
		e = c.QueryRow(ctx, `SELECT id,status,version FROM platform.orders WHERE organization_id=$1 AND conversation_id=$2`, org, conv).Scan(&oid, &status, &version)
		if e == pgx.ErrNoRows {
			q, err := svc.PrepareQuote(ctx, tenant, conv, []orders.CartItem{{VariantID: demoID(run, "variant:DEMO-CHARGER"), Quantity: 1}})
			if err != nil {
				return err
			}
			raw, err := svc.CreateRequest(ctx, tenant, orders.Consent{QuoteID: q.ID, ConversationID: conv, Accepted: true, IdempotencyKey: demoID(run, "request:"+state)})
			if err != nil {
				return err
			}
			var order struct {
				ID      string
				Status  string
				Version int64
			}
			if err = json.Unmarshal(raw, &order); err != nil {
				return err
			}
			oid, status, version = order.ID, order.Status, order.Version
		} else if e != nil {
			return e
		}
		actions := []string{}
		if state == "rejected" && status == "pending_confirmation" {
			actions = append(actions, "reject")
		}
		if (state == "confirmed" || state == "completed") && status == "pending_confirmation" {
			actions = append(actions, "confirm")
		}
		if state == "completed" && status != "completed" {
			actions = append(actions, "complete")
		}
		for _, action := range actions {
			if _, e = svc.Change(ctx, tenant, oid, action, orders.Transition{ExpectedVersion: version, IdempotencyKey: demoID(run, state+":"+action), Reason: "SYNTHETIC: подготовленный пример, не результат показа"}); e != nil {
				return e
			}
			version++
		}
	}
	if _, e = c.Exec(ctx, `INSERT INTO platform.catalog_events(organization_id,actor_id,resource_id,kind) VALUES($1,$2,$3,'DEMO_FIXTURE_READY')`, org, users["owner"].ID, run); e != nil {
		return e
	}
	return output()
}
