//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/agentruntime"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	httpapi "github.com/karadjev/AI-Business-Assistant/apps/api/internal/http"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/orders"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/db"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/migrate"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/servicebooking"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/telegram"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/whatsapp"
)

func testStage263RealEstate(t *testing.T, d *db.Database, admin *pgx.Conn, ctx context.Context) {
	const origin = "http://127.0.0.1:3000"
	key := bytes.Repeat([]byte{177}, 32)
	identity := auth.New(d.Pool, config.Auth{EmailKey: key, Origin: origin, SessionTTL: time.Hour, IdleTTL: time.Hour})
	handler := httpapi.NewHandler(d, slog.New(slog.NewTextHandler(io.Discard, nil)), 3*time.Second, 65536, identity)
	svc := agentruntime.Service{Pool: d.Pool, Key: key}
	var forcedTables int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
	 WHERE n.nspname='platform' AND c.relkind='r' AND c.relrowsecurity AND c.relforcerowsecurity`).Scan(&forcedTables); err != nil || forcedTables != 61 {
		t.Fatalf("FORCE RLS tables=%d: %v", forcedTables, err)
	}

	var tenants [3]stage23Owner
	for i := range tenants {
		tenants[i] = seedStage23Owner(t, d, admin, ctx, identity, 182+i)
		if err := svc.UpdateProfile(ctx, tenants[i].tenant, "agency", "real_estate", 1); err != nil {
			t.Fatal(err)
		}
	}
	owner := tenants[0]
	org := owner.tenant.OrganizationID
	selection, _ := stage26ConfigureBooking(t, ctx, owner.tenant, servicebooking.Service{Pool: d.Pool}, 990)
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := admin.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	post := func(actor stage23Owner, path string, body any, want int) []byte {
		t.Helper()
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodPost, "/api/v1/organizations/"+actor.tenant.OrganizationID+path, bytes.NewReader(b)).WithContext(ctx)
		r.AddCookie(&http.Cookie{Name: "aba_session", Value: actor.token})
		r.Header.Set("Origin", origin)
		r.Header.Set("X-CSRF-Token", actor.csrf)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s: got %d, want %d: %s", path, w.Code, want, w.Body.String())
		}
		return w.Body.Bytes()
	}
	seed := func(tenant stage23Owner, title, currency, transaction, market, availability string, rooms, price int, offering *string) string {
		t.Helper()
		id := auth.UUID()
		exec(`INSERT INTO platform.property_listings(id,organization_id,title,locality,city,district,area_m2,rooms,bedrooms,
		  status,verified_until,price_amount,currency_code,property_type,transaction_type,property_status,viewing_offering_id,
		  floor,building_type,renovation_status,has_parking,published_at,updated_at)
		  VALUES($1,$2,$3,'Synthetic locality','Synthetic City','Center',48.5,$4,1,$5,now()+interval '2 hours',$6,$7,
		   'apartment',$8,$9,$10,3,'brick','renovated',true,now(),now())`,
			id, tenant.tenant.OrganizationID, title, rooms, availability, price, currency, transaction, market, offering)
		return id
	}
	selected := seed(owner, "Synthetic central apartment", "RUB", "sale", "active", "available", 2, 500000, &selection.OfferingID)
	second := seed(owner, "Synthetic second apartment", "RUB", "sale", "active", "available", 2, 550000, nil)
	seed(owner, "Archived apartment", "RUB", "sale", "archived", "unavailable", 2, 400000, nil)
	seed(owner, "Foreign currency", "USD", "sale", "active", "available", 2, 100, nil)
	seed(owner, "Rental apartment", "RUB", "long_term_rental", "active", "available", 2, 100, nil)
	foreign := seed(tenants[1], "Tenant two apartment", "RUB", "sale", "active", "available", 2, 100, nil)
	seed(tenants[2], "Tenant three apartment", "RUB", "sale", "active", "available", 2, 100, nil)

	rooms, beds, floor, budget := 2, 1, 3, int64(600000)
	criteria := orders.PropertyCriteria{City: "Synthetic City", District: "Center", Type: "apartment", Currency: "RUB",
		Rooms: &rooms, MaxPrice: &budget, Bedrooms: &beds, Floor: &floor, BuildingType: "brick", Renovation: "renovated"}
	criteria.Transaction = "sale"
	minArea, maxArea := 48.4, 48.6
	criteria.MinArea, criteria.MaxArea = &minArea, &maxArea
	var page orders.PropertyPage
	decoded := post(owner, "/operations/properties/discover", orders.PropertySearch{PropertyCriteria: criteria, Sort: "price_asc"}, 200)
	if json.Unmarshal(decoded, &page) != nil || len(page.Items) != 2 || page.Items[0].ID != selected || page.Items[1].ID != second {
		t.Fatalf("typed search: %s", decoded)
	}
	post(owner, "/operations/properties/discover", map[string]any{"max_price_amount": 500}, 422)
	post(owner, "/operations/properties/discover", map[string]any{"currency_code": "RUB", "min_price_amount": 800, "max_price_amount": 500}, 422)

	t.Run("deterministic-pagination", func(t *testing.T) {
		exec(`INSERT INTO platform.property_listings(id,organization_id,title,locality,city,rooms,area_m2,status,verified_until)
		 SELECT gen_random_uuid(),$1,'Synthetic page '||n,'Paging','Paging',1,n,'available',now()+interval '1 hour'
		 FROM generate_series(1,21) AS n`, org)
		input := orders.PropertySearch{PropertyCriteria: orders.PropertyCriteria{City: "Paging"}, Sort: "area_desc"}
		raw := post(owner, "/operations/properties/discover", input, 200)
		var first orders.PropertyPage
		if json.Unmarshal(raw, &first) != nil || len(first.Items) != 20 || !first.HasMore || first.Items[0].Area != "21.00" {
			t.Fatalf("first page: %s", raw)
		}
		input.Page = 2
		raw = post(owner, "/operations/properties/discover", input, 200)
		var next orders.PropertyPage
		if json.Unmarshal(raw, &next) != nil || len(next.Items) != 1 || next.HasMore || next.Items[0].Area != "1.00" {
			t.Fatalf("second page: %s", raw)
		}
	})

	process := func(n int) {
		t.Helper()
		for i := 0; i < n; i++ {
			if worked, err := svc.ProcessOne(ctx); !worked || err != nil {
				t.Fatalf("worker %d: worked=%t error=%v", i, worked, err)
			}
		}
	}
	submit := func(path string, body any) string {
		t.Helper()
		var result struct{ ID string }
		raw := post(owner, "/agent-delegations/"+path, body, 202)
		if json.Unmarshal(raw, &result) != nil || result.ID == "" {
			t.Fatalf("submit: %s", raw)
		}
		return result.ID
	}
	result := func(id, state string) []byte {
		t.Helper()
		var raw []byte
		var status, actual string
		err := admin.QueryRow(ctx, `SELECT a.status,coalesce(e.result->0->>'state',''),e.result->0
		 FROM platform.agent_tasks a JOIN platform.agent_executions e ON (e.organization_id,e.task_id)=(a.organization_id,a.id)
		 WHERE a.id=$1 ORDER BY e.attempt DESC LIMIT 1`, id).Scan(&status, &actual, &raw)
		if err != nil || status != "completed" || actual != state {
			t.Fatalf("result %s: %s/%s (%s): %v", id, status, actual, raw, err)
		}
		return raw
	}
	search := agentruntime.RealEstateRequest{IndustryDelegationRequest: agentruntime.IndustryDelegationRequest[agentruntime.PropertyRequirements]{
		ConversationID: owner.conversation, IdempotencyKey: auth.UUID(), Requirements: agentruntime.PropertyRequirements{PropertyCriteria: criteria},
	}}
	search.Requirements.Transaction, search.Requirements.MaxPrice = "", nil
	missing := submit("real-estate", search)
	process(4)
	if raw := result(missing, "clarification_required"); !bytes.Contains(raw, []byte("max_price_amount")) || bytes.Contains(raw, []byte(selected)) {
		t.Fatalf("unexpected clarification: %s", raw)
	}
	search.IdempotencyKey, search.PreviousTaskID = auth.UUID(), missing
	search.Requirements = agentruntime.PropertyRequirements{PropertyCriteria: orders.PropertyCriteria{Transaction: "sale", MaxPrice: &budget}}
	offer := submit("real-estate", search)
	process(2)
	exec("UPDATE platform.property_listings SET status='unavailable',property_status='sold' WHERE id=$1", second)
	process(2)
	if raw := result(offer, "proposal_ready"); !bytes.Contains(raw, []byte(selected)) || strings.Contains(string(raw), foreign) {
		t.Fatalf("offer missing selected listing: %s", raw)
	} else {
		var top struct{ Properties []orders.Property }
		if json.Unmarshal(raw, &top) != nil || len(top.Properties) != 1 || top.Properties[0].ID != selected {
			t.Fatalf("stale listing was offered: %s", raw)
		}
	}
	search.IdempotencyKey = auth.UUID()
	search.Requirements.City = "Missing City"
	none := submit("real-estate", search)
	process(4)
	result(none, "no_properties")

	t.Run("large-property-cards-fit-the-durable-result", func(t *testing.T) {
		tenant := tenants[1]
		exec(`INSERT INTO platform.property_listings(id,organization_id,title,locality,city,district,country,region,address,
		 building_type,area_m2,rooms,status,verified_until,price_amount,currency_code,property_type,transaction_type)
		 SELECT gen_random_uuid(),$1,repeat(chr(60),160),repeat(chr(60),120),'Synthetic large cards',repeat(chr(60),120),
		  repeat(chr(60),120),repeat(chr(60),120),repeat(chr(60),240),repeat(chr(60),120),48.5,2,'available',
		  now()+interval '1 hour',100+n,'RUB','apartment','sale' FROM generate_series(1,4) n`, tenant.tenant.OrganizationID)
		input := agentruntime.RealEstateRequest{IndustryDelegationRequest: agentruntime.IndustryDelegationRequest[agentruntime.PropertyRequirements]{
			ConversationID: tenant.conversation, IdempotencyKey: auth.UUID(),
			Requirements: agentruntime.PropertyRequirements{PropertyCriteria: orders.PropertyCriteria{
				City: "Synthetic large cards", Type: "apartment", Transaction: "sale", Currency: "RUB", MaxPrice: &budget,
			}},
		}}
		var task struct{ ID string }
		if err := json.Unmarshal(post(tenant, "/agent-delegations/real-estate", input, 202), &task); err != nil || task.ID == "" {
			t.Fatalf("large card task: %v", err)
		}
		process(4)
		var offer struct{ Properties []orders.Property }
		if err := json.Unmarshal(result(task.ID, "proposal_ready"), &offer); err != nil || len(offer.Properties) != 3 {
			t.Fatalf("large card offer has %d properties: %v", len(offer.Properties), err)
		}
	})

	view := agentruntime.PropertyViewingRequest{ConversationID: owner.conversation, IdempotencyKey: auth.UUID(),
		OfferTaskID: offer, PropertyID: foreign, Selection: selection, Comment: "Synthetic customer comment"}
	notOurs := submit("property-viewing", view)
	process(3)
	result(notOurs, "property_unavailable")
	post(owner, "/agent-delegations/property-viewing/"+notOurs+"/prepare", nil, 409)
	post(tenants[1], "/agent-delegations/property-viewing/"+notOurs+"/prepare", nil, 404)
	view.IdempotencyKey, view.PropertyID = auth.UUID(), selected
	// Keep the fixture under the per-tenant submission rate budget.
	exec("UPDATE platform.agent_tasks SET created_at=created_at-interval '2 minutes' WHERE organization_id=$1", org)

	tgMock := &bookingTelegramMock{calls: 1}
	tg := telegram.Service{Pool: d.Pool, Config: config.Telegram{Mode: "mock", Key: bytes.Repeat([]byte{178}, 32), Retention: 24 * time.Hour, Enabled: true}, Transport: tgMock}
	if _, err := tg.Connect(ctx, owner.tenant, "mock:64263"); err != nil {
		t.Fatal(err)
	}
	exec("UPDATE platform.conversations SET channel='telegram' WHERE organization_id=$1 AND id=$2", org, owner.conversation)
	exec(`INSERT INTO platform.telegram_principals(id,organization_id,integration_id,user_id,chat_id,conversation_id)
	 SELECT gen_random_uuid(),organization_id,id,8263,8263,$2 FROM platform.telegram_integrations WHERE organization_id=$1`, org, owner.conversation)
	exec(`INSERT INTO platform.messages(id,organization_id,conversation_id,role,content,client_key)
	 VALUES(gen_random_uuid(),$1,$2,'customer','booking_updates:on',gen_random_uuid())`, org, owner.conversation)
	sendTelegram := func(want string) {
		t.Helper()
		before := tgMock.calls
		if worked, err := tg.SendOne(ctx); !worked || err != nil || tgMock.calls != before+1 ||
			tgMock.destination != 8263 || !strings.Contains(tgMock.text, want) {
			t.Fatalf("telegram worked=%t calls=%d text=%q error=%v", worked, tgMock.calls, tgMock.text, err)
		}
	}
	task := submit("property-viewing", view)
	if replay := submit("property-viewing", view); replay != task {
		t.Fatal("duplicate viewing submit changed parent")
	}
	process(3)
	result(task, "approval_required")
	type request struct {
		ID      string
		Version int64
	}
	prepare := func(task string) request {
		t.Helper()
		var order request
		raw := post(owner, "/agent-delegations/property-viewing/"+task+"/prepare", nil, 201)
		if json.Unmarshal(raw, &order) != nil || order.ID == "" || order.Version != 1 {
			t.Fatalf("prepare: %s", raw)
		}
		return order
	}
	order := prepare(task)
	if prepare(task) != order {
		t.Fatal("duplicate prepare changed viewing ID")
	}
	process(1)
	sendTelegram("Заявка на просмотр принята")
	orderPath := "/orders/" + order.ID
	post(tenants[1], orderPath+"/confirm", stage26Command(1), 404)
	exec("UPDATE platform.property_listings SET verified_until=now()-interval '1 second' WHERE id=$1", selected)
	post(owner, orderPath+"/confirm", stage26Command(1), 409)
	exec("UPDATE platform.property_listings SET verified_until=now()+interval '2 hours' WHERE id=$1", selected)
	command := stage26Command(1)
	post(owner, orderPath+"/confirm", command, 200)
	post(owner, orderPath+"/confirm", command, 200)
	post(owner, orderPath+"/confirm", stage26Command(1), 409)

	var raw []byte
	if err := d.Pool.QueryRow(ctx, "SELECT platform.agent_claim($1,$2)", svc.Proof(), auth.UUID()).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var claimed struct{ ID string }
	if json.Unmarshal(raw, &claimed) != nil || claimed.ID == "" {
		t.Fatalf("claim interrupted receipt: %s", raw)
	}
	var deniedRead []byte
	if err := d.Pool.QueryRow(ctx, "SELECT platform.property_receipt_context($1,$2)", claimed.ID, auth.UUID()).Scan(&deniedRead); err != nil || len(deniedRead) != 0 {
		t.Fatalf("viewing receipt visible without lease: %s %v", deniedRead, err)
	}

	exec("UPDATE platform.channel_inbox SET lease_until=clock_timestamp()-interval '1 second' WHERE id=$1", claimed.ID)
	if binary := os.Getenv("ABA_TEST_AGENT_WORKER_BINARY"); binary != "" {
		testStage23WorkerProcess(t, ctx, admin, binary, key, claimed.ID, 2)
	} else {
		process(1)
	}
	result(claimed.ID, "backend_result")
	sendTelegram("Просмотр подтверждён")
	post(owner, orderPath+"/cancel", stage26Command(2), 200)
	process(1)
	sendTelegram("Просмотр отменён")

	view.IdempotencyKey = auth.UUID()
	rejectTask := submit("property-viewing", view)
	process(3)
	rejected := prepare(rejectTask)
	process(1)
	sendTelegram("Заявка на просмотр принята")
	employee := tenants[1]
	employee.tenant.OrganizationID = org
	employeeID := employee.tenant.Session.User.ID
	exec(`INSERT INTO platform.organization_members(organization_id,user_id,role,can_manage_orders)
	 VALUES($1,$2,'member',false)`, org, employeeID)
	post(employee, "/orders/"+rejected.ID+"/reject", stage26Command(1), 403)
	exec("UPDATE platform.organization_members SET can_manage_orders=true WHERE organization_id=$1 AND user_id=$2", org, employeeID)
	post(employee, "/orders/"+rejected.ID+"/reject", stage26Command(1), 200)
	var decisionBy string
	if err := admin.QueryRow(ctx, "SELECT actor_id FROM platform.order_events WHERE organization_id=$1 AND order_id=$2 AND kind='rejected'",
		org, rejected.ID).Scan(&decisionBy); err != nil || decisionBy != employeeID {
		t.Fatalf("rejection audit actor=%s: %v", decisionBy, err)
	}
	process(1)
	sendTelegram("Сотрудник отклонил заявку")
	if worked, err := tg.SendOne(ctx); worked || err != nil {
		t.Fatalf("duplicate delivery: %t %v", worked, err)
	}

	var availability, marketStatus string
	if err := admin.QueryRow(ctx, "SELECT status,property_status FROM platform.property_listings WHERE id=$1", selected).Scan(&availability, &marketStatus); err != nil || availability != "available" || marketStatus != "active" {
		t.Fatalf("viewing altered property: %s/%s %v", availability, marketStatus, err)
	}

	for _, stranger := range tenants[1:] {
		tx, err := tenancy.Begin(ctx, d.Pool, stranger.tenant, true, true)
		if err != nil {
			t.Fatal(err)
		}
		var visible int
		err = tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM platform.property_listings WHERE id=$1) +
		  (SELECT count(*) FROM platform.orders WHERE id=$2) + (SELECT count(*) FROM platform.agent_audit WHERE task_id=$3)`,
			selected, order.ID, claimed.ID).Scan(&visible)
		_ = tx.Rollback(ctx)
		if err != nil || visible != 0 {
			t.Fatalf("cross-tenant rows=%d error=%v", visible, err)
		}
	}

	waOwner := tenants[2]
	waMock := &bookingWhatsAppMock{}
	wa := whatsapp.Service{Pool: d.Pool, Key: bytes.Repeat([]byte{179}, 32), Transport: waMock}
	if _, err := wa.Connect(ctx, waOwner.tenant, whatsapp.ConnectInput{AppID: "100", BusinessID: "200", PhoneID: "642630", GraphVersion: "v0.0", Credentials: whatsapp.Credentials{AccessToken: "mock:property-wire", AppSecret: "mock:property-app", VerifyToken: "mock:property-verify", AllowedUsers: []string{"8264"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := wa.Control(ctx, waOwner.tenant, true, true); err != nil {
		t.Fatal(err)
	}
	waOrg := waOwner.tenant.OrganizationID
	exec("UPDATE platform.conversations SET channel='whatsapp' WHERE organization_id=$1 AND id=$2", waOrg, waOwner.conversation)
	exec(`INSERT INTO platform.channel_principals(id,organization_id,integration_id,external_user_id,conversation_id,window_until)
	 SELECT c.customer_id,c.organization_id,i.id,'8264',c.id,now()+interval '1 hour'
	 FROM platform.conversations c JOIN platform.channel_integrations i ON i.organization_id=c.organization_id AND i.channel='whatsapp'
	 WHERE c.organization_id=$1 AND c.id=$2`, waOrg, waOwner.conversation)
	exec(`INSERT INTO platform.messages(id,organization_id,conversation_id,role,content,client_key)
	 VALUES(gen_random_uuid(),$1,$2,'customer','booking_updates:on',gen_random_uuid())`, waOrg, waOwner.conversation)
	search.ConversationID, search.PreviousTaskID, search.IdempotencyKey = waOwner.conversation, "", auth.UUID()
	search.Requirements = agentruntime.PropertyRequirements{PropertyCriteria: criteria}
	post(waOwner, "/agent-delegations/real-estate", search, 202)
	process(4)
	if worked, err := wa.SendOne(ctx); !worked || err != nil || len(waMock.messages) != 1 {
		t.Fatalf("WhatsApp delivery worked=%t calls=%d error=%v", worked, len(waMock.messages), err)
	}
	if m := waMock.messages[0]; m.OrganizationID != waOrg || m.ConversationID != waOwner.conversation || m.ExternalUserID != "8264" {
		t.Fatal("WhatsApp offer reached the wrong tenant or recipient")
	}
	search.IdempotencyKey = auth.UUID()
	post(waOwner, "/agent-delegations/real-estate", search, 202)
	process(4)
	exec("UPDATE platform.property_listings SET verified_until=now()-interval '1 minute' WHERE organization_id=$1", waOrg)
	if worked, err := wa.SendOne(ctx); !worked || err != nil || len(waMock.messages) != 1 {
		t.Fatalf("stale property reached WhatsApp provider: worked=%t calls=%d error=%v", worked, len(waMock.messages), err)
	}

}

// TestRealEstateIsolated runs the focused HTTP and worker proof on a fresh database.
func TestRealEstateIsolated(t *testing.T) {
	if os.Getenv("ABA_TEST_ONLY_REAL_ESTATE") != "1" {
		t.Skip("requires a fresh isolated fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	migrationDSN := testDSN(t, "ABA_TEST_MIGRATION_DATABASE_URL", "aba_migrator")
	admin, err := pgx.Connect(ctx, migrationDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	var safe bool
	err = admin.QueryRow(ctx, `SELECT marker='aba-synthetic-isolated' AND
	 NOT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema='platform' AND table_name<>'test_safety')
	 FROM platform.test_safety WHERE singleton`).Scan(&safe)
	if err != nil || !safe {
		t.Fatal("fresh isolated schema required")
	}
	if err = migrate.Run(ctx, migrationDSN, "up"); err != nil {
		t.Fatal(err)
	}
	database, err := db.Open(ctx, testDSN(t, "ABA_TEST_DATABASE_URL", "aba_app"), 2, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	testStage263RealEstate(t, database, admin, ctx)
}
