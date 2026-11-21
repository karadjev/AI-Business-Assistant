//go:build integration

package integration

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/agent"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/agentruntime"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/conversations"
	httpapi "github.com/karadjev/AI-Business-Assistant/apps/api/internal/http"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/instagram"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/orders"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/db"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/migrate"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/servicebooking"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
)

func TestInstagramOfflineIsolated(t *testing.T) {
	binary := os.Getenv("ABA_TEST_INSTAGRAM_WORKER")
	if binary == "" {
		t.Skip("requires the isolated Instagram offline runner")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	migrationDSN := testDSN(t, "ABA_TEST_MIGRATION_DATABASE_URL", "aba_migrator")
	appDSN := testDSN(t, "ABA_TEST_DATABASE_URL", "aba_app")
	admin, err := pgx.Connect(ctx, migrationDSN)
	if err != nil {
		t.Fatal("cannot connect to isolated database")
	}
	defer admin.Close(context.Background())
	var clean bool
	if err = admin.QueryRow(ctx, `SELECT marker='aba-synthetic-isolated' AND NOT EXISTS(
 SELECT FROM information_schema.tables WHERE table_schema='platform' AND table_name<>'test_safety')
 FROM platform.test_safety WHERE singleton`).Scan(&clean); err != nil || !clean {
		t.Fatal("fresh isolated database required")
	}
	if err = migrate.Run(ctx, migrationDSN, "up"); err != nil {
		t.Fatal(err)
	}
	d, err := db.Open(ctx, appDSN, 4, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	const origin = "http://127.0.0.1:3000"
	key := bytes.Repeat([]byte{101}, 32)
	ig := config.Instagram{Enabled: true, Key: bytes.Repeat([]byte{102}, 32),
		AppSecret: "mock:instagram-offline-app-secret", VerifyToken: "mock:instagram-offline-verify-token"}
	worker := func(bin, outcome, env string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, bin)
		cmd.Env = []string{
			"APP_ENV=" + env, "DATABASE_URL=" + appDSN, "AUTH_EMAIL_KEY=" + hex.EncodeToString(key),
			"PUBLIC_ORIGIN=" + origin, "LLM_PROVIDER=mock", "TELEGRAM_TRANSPORT=mock",
			"INSTAGRAM_ENABLED=true", "INSTAGRAM_ENCRYPTION_KEY=" + hex.EncodeToString(ig.Key),
			"INSTAGRAM_APP_SECRET=" + ig.AppSecret, "INSTAGRAM_VERIFY_TOKEN=" + ig.VerifyToken,
			"ABA_INSTAGRAM_OFFLINE_PROVIDER=" + outcome,
		}
		return cmd
	}
	if log, err := worker(binary, "accept", "test").CombinedOutput(); err == nil || !bytes.Contains(log, []byte("instagram_queue_configuration_invalid")) {
		t.Fatal("worker started without the database test permission")
	}
	sql, err := os.ReadFile("testdata/instagram_offline.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = admin.Exec(ctx, string(sql)); err != nil {
		t.Fatal(err)
	}

	if log, err := worker(binary, "accept", "development").CombinedOutput(); err == nil || !bytes.Contains(log, []byte("instagram_queue_configuration_invalid")) {
		t.Fatal("offline provider started outside the test runtime")
	}

	start := func(bin, outcome string) func() {
		t.Helper()
		cmd := worker(bin, outcome, "test")
		var log bytes.Buffer
		cmd.Stdout, cmd.Stderr = &log, &log
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		return func() {
			t.Helper()
			if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
				t.Error(err)
			}
			if err := cmd.Wait(); err != nil {
				t.Errorf("worker exit: %v\n%s", err, log.String())
			}
		}
	}
	execSQL := func(query string, args ...any) {
		t.Helper()
		if _, err := admin.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	waitFor := func(check func() bool, description string) {
		t.Helper()
		for deadline := time.Now().Add(9 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
			if check() {
				return
			}
		}
		t.Fatalf("timed out: %s", description)
	}

	ac, err := config.LoadAgent(func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	identity := auth.New(d.Pool, config.Auth{SessionTTL: time.Hour, IdleTTL: time.Hour, Origin: origin, EmailKey: key})
	handler := httpapi.NewHandlerWithChannels(d, logger, 3*time.Second, 65536, identity, ac, agent.Mock{}, config.Telegram{}, config.WhatsApp{}, ig)
	instagramService := instagram.Service{Config: ig, Pool: d.Pool, Chat: &conversations.Service{Pool: d.Pool, Config: ac, Provider: agent.Mock{}}}
	agents := agentruntime.Service{Pool: d.Pool, Key: key}
	type fixture struct {
		owner                                                  stage23Owner
		org, integration, conversation, parent, order, receipt string
		version                                                int64
	}
	post := func(f fixture, route string, data any, want int) []byte {
		t.Helper()
		body, _ := json.Marshal(data)
		r := httptest.NewRequest(http.MethodPost, "/api/v1/organizations/"+f.org+route, bytes.NewReader(body))
		r.AddCookie(&http.Cookie{Name: "aba_session", Value: f.owner.token})
		r.Header.Set("Origin", origin)
		r.Header.Set("X-CSRF-Token", f.owner.csrf)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s status %d: %s", route, w.Code, w.Body.String())
		}
		return w.Body.Bytes()
	}
	receive := func(f fixture, eventID, inbound string) {
		t.Helper()
		var account string
		if err := admin.QueryRow(ctx, "SELECT account_id FROM platform.instagram_connections WHERE integration_id=$1", f.integration).Scan(&account); err != nil {
			t.Fatal(err)
		}
		raw := []byte(fmt.Sprintf(`{"object":"instagram","entry":[{"id":%q,"messaging":[{"sender":{"id":"9700"},"recipient":{"id":%q},"timestamp":%d,"message":{"mid":%q,"text":%q}}]}]}`,
			account, account, time.Now().UnixMilli(), eventID, inbound))
		mac := hmac.New(sha256.New, []byte(ig.AppSecret))
		mac.Write(raw)
		r := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/instagram/"+f.integration, bytes.NewReader(raw))
		r.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("signed inbound status %d: %s", w.Code, w.Body.String())
		}
		if worked, err := instagramService.ProcessOne(ctx); err != nil || !worked {
			t.Fatalf("inbound worker: %t %v", worked, err)
		}
	}
	connect := func(index int, consent bool, model, industry string) fixture {
		t.Helper()
		f := fixture{owner: seedStage23Owner(t, d, admin, ctx, identity, 310+index)}
		f.org = f.owner.tenant.OrganizationID
		if err := agents.UpdateProfile(ctx, f.owner.tenant, model, industry, 1); err != nil {
			t.Fatal(err)
		}
		settings, err := instagramService.Start(ctx, f.owner.tenant)
		if err != nil {
			t.Fatal(err)
		}
		f.integration = settings["integration_id"].(string)
		callback, err := url.Parse(settings["callback_url"].(string))
		if err != nil {
			t.Fatal(err)
		}
		if err = instagramService.Callback(ctx, f.owner.tenant, callback.Query()); err != nil {
			t.Fatal(err)
		}
		inbound := "Нужна запись"
		if consent {
			inbound = "booking_updates:on"
		}
		receive(f, "customer-first", inbound)
		if err := admin.QueryRow(ctx, "SELECT conversation_id FROM platform.channel_principals WHERE integration_id=$1", f.integration).Scan(&f.conversation); err != nil {
			t.Fatal(err)
		}
		return f
	}
	prep := func(index int, consent bool) fixture {
		t.Helper()
		f := connect(index, consent, "service", "vehicle_service")
		selection, _ := stage26ConfigureBooking(t, ctx, f.owner.tenant, servicebooking.Service{Pool: d.Pool}, 1050+index)
		var task struct{ ID string }
		if err := json.Unmarshal(post(f, "/agent-delegations/auto-service", agentruntime.IndustryDelegationRequest[agentruntime.ServiceRequirements]{
			ConversationID: f.conversation, IdempotencyKey: auth.UUID(),
			Requirements: agentruntime.ServiceRequirements{NoPart: true, Selection: &selection,
				Vehicle: catalog.FitmentVehicle{Make: "synthetic", Model: "service", Year: 2025}},
		}, 202), &task); err != nil || task.ID == "" {
			t.Fatal("missing parent task")
		}
		f.parent = task.ID
		for range 6 {
			if worked, err := agents.ProcessOne(ctx); err != nil || !worked {
				t.Fatalf("workflow step: %t %v", worked, err)
			}
		}
		var order struct {
			ID      string
			Version int64
		}
		if err := json.Unmarshal(post(f, "/agent-delegations/auto-service/"+f.parent+"/prepare", nil, 201), &order); err != nil || order.ID == "" {
			t.Fatal("missing prepared order")
		}
		f.order, f.version = order.ID, order.Version
		command := stage26Command(f.version)
		post(f, "/orders/"+f.order+"/confirm", command, 200)
		post(f, "/orders/"+f.order+"/confirm", command, 200)
		if err := admin.QueryRow(ctx, "SELECT id FROM platform.agent_tasks WHERE organization_id=$1 AND parent_id=$2 AND action='delegation.service_receipt'", f.org, f.parent).Scan(&f.receipt); err != nil {
			t.Fatal(err)
		}
		return f
	}
	assertState := func(f fixture, state string, attempts int) {
		t.Helper()
		waitFor(func() bool {
			var count int
			return admin.QueryRow(ctx, "SELECT count(*) FROM platform.channel_outbox WHERE id=$1 AND organization_id=$2 AND status=$3 AND attempts=$4",
				f.receipt, f.org, state, attempts).Scan(&count) == nil && count == 1
		}, state)
	}
	assertAudit := func(f fixture, action, result string) {
		t.Helper()
		var count int
		if err := admin.QueryRow(ctx, "SELECT count(*) FROM platform.agent_audit WHERE organization_id=$1 AND task_id=$2 AND action=$3 AND result=$4",
			f.org, f.receipt, action, result).Scan(&count); err != nil || count != 1 {
			t.Fatalf("expected one audit result %s %s, got %d: %v", action, result, count, err)
		}
	}
	normal := os.Getenv("ABA_TEST_AGENT_WORKER_BINARY")
	if normal == "" {
		t.Fatal("normal worker binary required")
	}
	f := prep(0, true)
	stop := start(normal, "accept")
	assertState(f, "pending", 0)
	time.Sleep(1200 * time.Millisecond)
	stop()
	assertState(f, "pending", 0)
	assertAudit(f, "booking.delivery", "queued")
	t.Log("normal worker preserved sending_enabled=false with a queued fixture")

	stop = start(binary, "throttle")
	assertState(f, "pending", 1)
	stop()
	assertAudit(f, "booking.delivery_state", "pending")
	execSQL("UPDATE platform.channel_outbox SET available_at=clock_timestamp() WHERE id=$1", f.receipt)
	stop = start(binary, "accept")
	assertState(f, "sent", 2)
	stop()
	assertAudit(f, "booking.delivery_state", "sent")
	var matches int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM platform.channel_outbox q JOIN platform.channel_integrations i
 ON (i.organization_id,i.id)=(q.organization_id,q.integration_id) WHERE q.id=$1 AND q.organization_id=$2
 AND q.conversation_id=$3 AND q.external_user_id='9700' AND q.provider_id='mock-'||q.id::text AND NOT i.sending_enabled
 AND q.payload->>'text' LIKE 'Запись подтверждена.%Synthetic atomic service%'`,
		f.receipt, f.org, f.conversation).Scan(&matches); err != nil || matches != 1 {
		t.Fatalf("unexpected sent row or content: %d %v", matches, err)
	}
	t.Log("signed consent, real API, Business Manager receipt, PostgreSQL outbox, adapter acceptance and worker process recovery verified")

	absent := prep(1, false)
	stop = start(binary, "accept")
	waitFor(func() bool {
		var count int
		return admin.QueryRow(ctx, "SELECT count(*) FROM platform.agent_audit WHERE task_id=$1 AND result='consent_required'", absent.receipt).Scan(&count) == nil && count == 1
	}, "missing consent audit")
	stop()
	assertAudit(absent, "booking.delivery", "consent_required")
	assertNoOutbox := func(f fixture) {
		t.Helper()
		var count int
		if err := admin.QueryRow(ctx, "SELECT count(*) FROM platform.channel_outbox WHERE id=$1", f.receipt).Scan(&count); err != nil || count != 0 {
			t.Fatalf("unexpected outbox row: %d %v", count, err)
		}
	}
	assertNoOutbox(absent)
	tx, err := tenancy.Begin(ctx, d.Pool, absent.owner.tenant, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM platform.channel_outbox WHERE id=$1", f.receipt).Scan(&matches); err != nil || matches != 0 {
		t.Fatalf("foreign outbox visible: %d %v", matches, err)
	}
	tx.Rollback(ctx)

	stale := prep(2, true)
	if did, err := agents.ProcessOne(ctx); !did || err != nil {
		t.Fatalf("queue confirmation: %t %v", did, err)
	}
	assertState(stale, "pending", 0)
	if _, err := admin.Exec(ctx, "UPDATE platform.channel_outbox SET external_user_id='9999' WHERE id=$1", stale.receipt); err == nil {
		t.Fatal("mismatched recipient accepted by PostgreSQL")
	}
	post(stale, "/orders/"+stale.order+"/cancel", stage26Command(stale.version+1), 200)
	stop = start(binary, "accept")
	assertState(stale, "cancelled", 0)
	waitFor(func() bool {
		var count int
		return admin.QueryRow(ctx, `SELECT count(*) FROM platform.channel_outbox WHERE booking_order_id=$1 AND status='sent' AND payload->>'status'='cancelled'`, stale.order).Scan(&count) == nil && count == 1
	}, "fresh cancellation accepted")
	stop()
	assertAudit(stale, "booking.delivery_state", "cancelled")

	revoked := prep(5, true)
	if did, err := agents.ProcessOne(ctx); !did || err != nil {
		t.Fatalf("queue before opt-out: %t %v", did, err)
	}
	assertState(revoked, "pending", 0)
	receive(revoked, "customer-opt-out", "booking_updates:off")
	stop = start(binary, "accept")
	assertState(revoked, "cancelled", 0)
	stop()
	assertAudit(revoked, "booking.delivery_state", "cancelled")

	for i, tc := range []struct{ mode, state string }{{"reject", "failed"}, {"unavailable", "delivery_unknown"}} {
		next := prep(3+i, true)
		stop = start(binary, tc.mode)
		assertState(next, tc.state, 1)
		stop()
		assertAudit(next, "booking.delivery_state", tc.state)
		stop = start(binary, "accept")
		time.Sleep(1200 * time.Millisecond)
		stop()
		assertState(next, tc.state, 1)
	}
	t.Run("real-estate-offer-and-viewing-decision", func(t *testing.T) {
		f := connect(6, true, "agency", "real_estate")
		selection, _ := stage26ConfigureBooking(t, ctx, f.owner.tenant, servicebooking.Service{Pool: d.Pool}, 1060)
		listing := auth.UUID()
		execSQL(`INSERT INTO platform.property_listings(id,organization_id,title,locality,city,rooms,area_m2,status,
		 verified_until,price_amount,currency_code,property_type,transaction_type,viewing_offering_id)
		 VALUES($1,$2,'Synthetic Instagram apartment','Synthetic city','Synthetic city',2,51,'available',
		  now()+interval '1 day',9500000,'RUB','apartment','sale',$3)`, listing, f.org, selection.OfferingID)
		submit := func(path string, body any) string {
			t.Helper()
			var task struct{ ID string }
			if json.Unmarshal(post(f, path, body, 202), &task) != nil || task.ID == "" {
				t.Fatal("expected persisted parent")
			}
			return task.ID
		}
		process := func(n int) {
			t.Helper()
			for range n {
				if worked, err := agents.ProcessOne(ctx); !worked || err != nil {
					t.Fatalf("real estate worker %t: %v", worked, err)
				}
			}
		}
		budget := int64(10000000)
		offer := submit("/agent-delegations/real-estate", agentruntime.RealEstateRequest{
			IndustryDelegationRequest: agentruntime.IndustryDelegationRequest[agentruntime.PropertyRequirements]{
				ConversationID: f.conversation, IdempotencyKey: auth.UUID(),
				Requirements: agentruntime.PropertyRequirements{PropertyCriteria: orders.PropertyCriteria{
					City: "Synthetic city", Type: "apartment", Transaction: "sale", Currency: "RUB", MaxPrice: &budget,
				}},
			},
		})
		process(4)
		f.receipt = offer
		assertState(f, "pending", 0)
		stop := start(normal, "accept")
		time.Sleep(1200 * time.Millisecond)
		stop()
		assertState(f, "pending", 0)
		stop = start(binary, "accept")
		assertState(f, "sent", 1)
		stop()
		assertAudit(f, "property.delivery_state", "sent")
		viewing := submit("/agent-delegations/property-viewing", agentruntime.PropertyViewingRequest{
			ConversationID: f.conversation, IdempotencyKey: auth.UUID(), OfferTaskID: offer, PropertyID: listing, Selection: selection,
		})
		process(3)
		var order struct {
			ID      string
			Version int64
		}
		if json.Unmarshal(post(f, "/agent-delegations/property-viewing/"+viewing+"/prepare", nil, 201), &order) != nil || order.ID == "" {
			t.Fatal("expected prepared viewing request")
		}
		process(1)
		post(f, "/orders/"+order.ID+"/confirm", stage26Command(order.Version), 200)
		process(1)
		if err := admin.QueryRow(ctx, `SELECT a.id FROM platform.agent_tasks a JOIN platform.order_events e
		 ON (e.organization_id,e.id)=(a.organization_id,a.idempotency_key)
		 WHERE a.parent_id=$1 AND a.action='delegation.property_receipt' AND e.kind='confirmed'`, viewing).Scan(&f.receipt); err != nil {
			t.Fatal(err)
		}
		stop = start(binary, "accept")
		assertState(f, "sent", 1)
		stop()
		assertAudit(f, "property.delivery_state", "sent")
		var correct bool
		err := admin.QueryRow(ctx, `SELECT q.external_user_id='9700' AND q.conversation_id=$3
		 AND q.payload->>'text' LIKE 'Просмотр подтверждён.%' AND q.provider_id='mock-'||q.id::text
		 FROM platform.channel_outbox q WHERE (q.organization_id,q.id)=($1,$2)`, f.org, f.receipt, f.conversation).Scan(&correct)
		if err != nil || !correct {
			t.Fatalf("wrong recipient or viewing receipt: %v", err)
		}
	})
	if _, err := admin.Exec(ctx, `DO $$ BEGIN IF EXISTS(SELECT FROM platform.channel_outbox q JOIN platform.channel_integrations i ON
 (i.organization_id,i.id)=(q.organization_id,q.integration_id) WHERE i.channel='instagram'
 AND ((q.status='sent' AND coalesce(q.provider_id,'') NOT LIKE 'mock-%') OR i.sending_enabled)) THEN
 RAISE EXCEPTION 'non-mock Instagram delivery'; END IF; END $$`); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(binary) == normal {
		t.Fatal("offline and normal worker paths must differ")
	}
}
