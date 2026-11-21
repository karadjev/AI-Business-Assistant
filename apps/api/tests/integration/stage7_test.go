//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/agent"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/conversations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/costcontrol"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/orders"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/db"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/telegram"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
	"sync"
	"testing"
	"time"
)

func testStage7(t *testing.T, d *db.Database, admin *pgx.Conn, ctx context.Context) {
	cfg, _ := config.LoadAgent(func(string) string { return "" })
	identity := auth.New(d.Pool, config.Auth{Origin: "http://127.0.0.1:3000", SessionTTL: time.Hour, IdleTTL: time.Hour})
	makeOwner := func(name string) catalog.Tenant {
		uid, token := auth.UUID(), auth.Token()
		_, e := admin.Exec(ctx, `INSERT INTO platform.users(id,email,password_hash,display_name,email_verified_at) VALUES($1,$2,'synthetic',$3,now())`, uid, name+"@stage7.example.test", name)
		if e != nil {
			t.Fatal(e)
		}
		_, e = admin.Exec(ctx, `INSERT INTO platform.sessions(id,user_id,token_hash,csrf_hash,expires_at,idle_expires_at) VALUES($1,$2,$3,$4,now()+interval '1 hour',now()+interval '1 hour')`, auth.UUID(), uid, auth.Hash(token), auth.Hash(auth.Token()))
		if e != nil {
			t.Fatal(e)
		}
		session, e := identity.Authenticate(ctx, token)
		if e != nil {
			t.Fatal(e)
		}
		org, e := (organizations.Service{Pool: d.Pool}).CreateTest(ctx, session, name)
		if e != nil {
			t.Fatal(e)
		}
		return catalog.Tenant{Session: session, OrganizationID: org.ID}
	}
	a, b := makeOwner("cost-a"), makeOwner("cost-b")
	chat := conversations.Service{Pool: d.Pool, Config: cfg, Provider: agent.Mock{}}
	raw, e := chat.Create(ctx, a)
	if e != nil {
		t.Fatal(e)
	}
	var c struct {
		ID      string `json:"id"`
		Version int64  `json:"version"`
	}
	_ = json.Unmarshal(raw, &c)
	svc := costcontrol.Service{Pool: d.Pool, Config: cfg}
	if _, e = svc.Reserve(ctx, a, c.ID, 10, 10, 1); e == nil {
		t.Fatal("zero budget allowed")
	}
	if _, e = svc.Update(ctx, a, costcontrol.Change{Settings: costcontrol.Settings{AutoEnabled: true, LiveEnabled: true}, ExpectedVersion: 1}); e == nil {
		t.Fatal("live enabled at zero")
	}
	settings, e := svc.Update(ctx, a, costcontrol.Change{Settings: costcontrol.Settings{AutoEnabled: false}, ExpectedVersion: 1})
	if e != nil {
		t.Fatal(e)
	}
	raw, e = chat.Send(ctx, a, c.ID, conversations.SendInput{Text: "Synthetic TEST7", IdempotencyKey: auth.UUID()})
	if e != nil {
		t.Fatal(e)
	}
	var detail struct {
		Mode     string `json:"control_mode"`
		Messages []struct {
			Role string `json:"role"`
		} `json:"messages"`
	}
	_ = json.Unmarshal(raw, &detail)
	if detail.Mode != "human" {
		t.Fatal("automatic answers not stopped")
	}
	for _, m := range detail.Messages {
		if m.Role == "assistant" {
			t.Fatal("automatic answer during shutdown")
		}
	}
	// No external API: only migration-owner test fixture approves a tiny simulated ledger.
	cfg.Provider = "openai_compatible"
	cfg.LiveEnabled = true
	cfg.Model = "synthetic-model"
	cfg.Live = config.Live{ExternalAuthorized: true, BaseURL: "https://provider.example/v1", AllowedBases: []string{"https://provider.example/v1"}, InputRate: 1000000, OutputRate: 1000000, PlatformMicroRUB: 1000, Parallel: 2, DialogInput: 20000, DialogOutput: 5000, DialogRequests: 30}
	svc.Config = cfg
	_, e = admin.Exec(ctx, `UPDATE platform.llm_platform_budget SET limit_microrub=1000,kill_switch=false WHERE singleton`)
	if e != nil {
		t.Fatal(e)
	}
	_, e = svc.Update(ctx, a, costcontrol.Change{Settings: costcontrol.Settings{AutoEnabled: true, LiveEnabled: true, RequestLimit: 20, DialogLimit: 20, DayLimit: 20, MonthLimit: 20}, ExpectedVersion: settings.Version})
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	ids := make(chan string, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, e := svc.Reserve(ctx, a, c.ID, 10, 10, 1)
			if e == nil {
				ids <- id
			} else {
				errs <- e
			}
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	winners := []string{}
	for id := range ids {
		winners = append(winners, id)
	}
	if len(winners) != 1 {
		t.Fatalf("concurrent reserve admitted %d", len(winners))
	}
	for e := range errs {
		var ae *auth.Error
		if !errors.As(e, &ae) || ae.Code != "LLM_BUDGET_EXHAUSTED" {
			t.Fatal("unexpected reserve error", e)
		}
	}
	if e = svc.Settle(ctx, a, winners[0], 0, 0, false); e != nil {
		t.Fatal(e)
	}
	if e = svc.Settle(ctx, a, winners[0], 0, 0, false); e != nil {
		t.Fatal(e)
	}
	var spent, reserved, active int
	if e = admin.QueryRow(ctx, `SELECT spent_microrub,reserved_microrub,active_calls FROM platform.llm_platform_budget WHERE singleton`).Scan(&spent, &reserved, &active); e != nil || spent != 20 || reserved != 0 || active != 0 {
		t.Fatal("reconciliation/idempotency", e, spent, reserved, active)
	}
	if _, e = svc.Reserve(ctx, a, c.ID, 1, 1, 1); e == nil {
		t.Fatal("exhausted dialog budget bypassed")
	}
	if e = svc.Settle(ctx, b, winners[0], 1, 1, true); e == nil {
		t.Fatal("foreign settlement")
	}
	for _, table := range []string{"ai_settings", "llm_usage", "llm_reservations", "agent_metrics", "ai_settings_events"} {
		var force bool
		if e = admin.QueryRow(ctx, `SELECT relforcerowsecurity FROM pg_class WHERE oid=$1::regclass`, "platform."+table).Scan(&force); e != nil || !force {
			t.Fatal("RLS not forced", table)
		}
		tx, e := tenancy.Begin(ctx, d.Pool, b, true, true)
		if e != nil {
			t.Fatal(e)
		}
		var count int
		e = tx.QueryRow(ctx, `SELECT count(*) FROM platform.`+table+` WHERE organization_id=$1`, a.OrganizationID).Scan(&count)
		_ = tx.Rollback(ctx)
		if e != nil || count != 0 {
			t.Fatal("foreign ledger leak", table, e)
		}
	}
	// Cross-organization competition shares one platform budget; no HTTP calls.
	_, e = admin.Exec(ctx, `UPDATE platform.llm_platform_budget SET limit_microrub=60 WHERE singleton`)
	if e != nil {
		t.Fatal(e)
	}
	cfg.Live.Parallel = 16
	svc.Config = cfg
	current, e := svc.Read(ctx, a)
	if e != nil {
		t.Fatal(e)
	}
	limits := costcontrol.Settings{AutoEnabled: true, LiveEnabled: true, RequestLimit: 40, DialogLimit: 40, DayLimit: 40, MonthLimit: 40}
	if _, e = svc.Update(ctx, a, costcontrol.Change{Settings: limits, ExpectedVersion: current.Version}); e != nil {
		t.Fatal(e)
	}
	if _, e = svc.Update(ctx, b, costcontrol.Change{Settings: limits, ExpectedVersion: 1}); e != nil {
		t.Fatal(e)
	}
	raw, e = chat.Create(ctx, b)
	if e != nil {
		t.Fatal(e)
	}
	var bc struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(raw, &bc)
	type admission struct {
		tenant catalog.Tenant
		id     string
	}
	admitted := make(chan admission, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tenant, conv := a, c.ID
			if i%2 == 1 {
				tenant, conv = b, bc.ID
			}
			id, err := svc.Reserve(ctx, tenant, conv, 10, 10, 1)
			if err == nil {
				admitted <- admission{tenant, id}
			}
		}(i)
	}
	wg.Wait()
	close(admitted)
	total := 0
	for entry := range admitted {
		total++
		if e = svc.Settle(ctx, entry.tenant, entry.id, 0, 0, false); e != nil {
			t.Fatal(e)
		}
	}
	if total != 2 {
		t.Fatalf("platform cap admitted %d instead of 2", total)
	}
	if e = admin.QueryRow(ctx, `SELECT spent_microrub,reserved_microrub FROM platform.llm_platform_budget WHERE singleton`).Scan(&spent, &reserved); e != nil || spent != 60 || reserved != 0 {
		t.Fatal("shared platform budget", e, spent, reserved)
	}
	if _, e = svc.Reserve(ctx, b, bc.ID, 1, 1, 1); e == nil {
		t.Fatal("shared platform budget exceeded")
	}
	// Configured allowlist is enforced in SQL before a new Telegram conversation exists.
	tc := config.Telegram{Mode: "mock", Key: bytes.Repeat([]byte{17}, 32), Enabled: true, Retention: 7 * 24 * time.Hour, AllowedUsers: map[int64]bool{777: true}}
	tg := telegram.Service{Pool: d.Pool, Config: tc, Transport: telegram.Mock{}, Chat: &chat}
	if _, e = tg.Connect(ctx, b, "mock:71001"); e != nil {
		t.Fatal(e)
	}
	u := telegram.Update{ID: 1, Message: &telegram.Message{From: telegram.User{ID: 778}, Chat: telegram.Chat{ID: 778, Type: "private"}, Text: "synthetic"}}
	if _, e = tg.Inject(ctx, b, u); e == nil {
		t.Fatal("sender outside allowlist admitted")
	}
	// An already queued event from an old configuration cannot create a
	// principal after the allowlist has been tightened.
	payload, _ := json.Marshal(u)
	_, e = admin.Exec(ctx, `INSERT INTO platform.telegram_inbox(id,organization_id,integration_id,update_id,chat_id,user_id,payload) SELECT $1,$2,id,701,778,778,$3 FROM platform.telegram_integrations WHERE organization_id=$2`, auth.UUID(), b.OrganizationID, payload)
	if e != nil {
		t.Fatal(e)
	}
	var claim []byte
	e = d.Pool.QueryRow(ctx, `SELECT platform.telegram_claim($1,$2,$3,$4,7,$5,$6,'mock')`, auth.UUID(), auth.UUID(), auth.UUID(), auth.UUID(), tg.WorkerProof(), tc.AllowedIDs()).Scan(&claim)
	if e != nil || string(claim) != "null" && len(claim) != 0 {
		t.Fatal("SQL allowlist claimed forbidden sender", e, string(claim))
	}
	var principals int
	if e = admin.QueryRow(ctx, `SELECT count(*) FROM platform.telegram_principals WHERE organization_id=$1 AND user_id=778`, b.OrganizationID).Scan(&principals); e != nil || principals != 0 {
		t.Fatal("principal created outside allowlist", e)
	}
	// Exercise FK cleanup and release of a confirmed synthetic reservation.
	catalogue := catalog.Service{Pool: d.Pool}
	raw, e = catalogue.CreateProduct(ctx, a, catalog.Input{Name: "Synthetic erase device", SKU: "ERASE7", PriceAmount: 10000})
	if e != nil {
		t.Fatal(e)
	}
	var product struct {
		ID       string `json:"id"`
		Variants []struct {
			ID string `json:"id"`
		} `json:"variants"`
	}
	_ = json.Unmarshal(raw, &product)
	variant := product.Variants[0].ID
	if _, e = catalogue.UpdateProduct(ctx, a, product.ID, catalog.Input{ExpectedVersion: 1}, "publish"); e != nil {
		t.Fatal(e)
	}
	if _, e = catalogue.Adjust(ctx, a, catalog.Input{VariantID: variant, ExpectedVersion: 1, QuantityOnHand: 3, Reason: "synthetic erasure fixture"}); e != nil {
		t.Fatal(e)
	}
	orderService := orders.Service{Pool: d.Pool, StockFreshness: time.Hour}
	quote, e := orderService.PrepareQuote(ctx, a, c.ID, []orders.CartItem{{VariantID: variant, Quantity: 2}})
	if e != nil {
		t.Fatal(e)
	}
	raw, e = orderService.CreateRequest(ctx, a, orders.Consent{QuoteID: quote.ID, ConversationID: c.ID, IdempotencyKey: auth.UUID(), Accepted: true})
	if e != nil {
		t.Fatal(e)
	}
	var order struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(raw, &order)
	if _, e = orderService.Change(ctx, a, order.ID, "confirm", orders.Transition{ExpectedVersion: 1, IdempotencyKey: auth.UUID()}); e != nil {
		t.Fatal(e)
	}
	// Full synthetic erasure leaves only anonymous financial aggregates.
	raw, e = chat.Get(ctx, a, c.ID)
	if e != nil {
		t.Fatal(e)
	}
	_ = json.Unmarshal(raw, &c)
	if _, e = svc.Purge(ctx, b, c.ID, c.Version); e == nil {
		t.Fatal("foreign purge")
	}
	if _, e = svc.Purge(ctx, a, c.ID, c.Version); e != nil {
		t.Fatal("purge", e)
	}
	var onHand, stockReserved int
	if e = admin.QueryRow(ctx, `SELECT quantity_on_hand,quantity_reserved FROM platform.inventory WHERE organization_id=$1 AND variant_id=$2`, a.OrganizationID, variant).Scan(&onHand, &stockReserved); e != nil || onHand != 3 || stockReserved != 0 {
		t.Fatal("purge failed to release only reservation", e, onHand, stockReserved)
	}
	var remainingOrders int
	if e = admin.QueryRow(ctx, `SELECT count(*) FROM platform.orders WHERE organization_id=$1 AND conversation_id=$2`, a.OrganizationID, c.ID).Scan(&remainingOrders); e != nil || remainingOrders != 0 {
		t.Fatal("order FK cleanup", e)
	}
	var customerLinks int
	if e = admin.QueryRow(ctx, `SELECT count(*) FROM platform.llm_reservations WHERE organization_id=$1 AND conversation_id IS NOT NULL`, a.OrganizationID).Scan(&customerLinks); e != nil || customerLinks != 0 {
		t.Fatal("financial customer link not removed")
	}
	_, e = admin.Exec(ctx, `UPDATE platform.llm_platform_budget SET limit_microrub=0,kill_switch=true WHERE singleton`)
	if e != nil {
		t.Fatal(e)
	}
	t.Log("PASS: zero/live gates, owner shutdown, real PG concurrent ledger, unknown usage/idempotency, foreign RLS, allowlist and synthetic deletion; no external API")
}
