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
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/agentruntime"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/channel"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	httpapi "github.com/karadjev/AI-Business-Assistant/apps/api/internal/http"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/db"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/servicebooking"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/telegram"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/whatsapp"
)

type bookingTelegramMock struct {
	telegram.Mock
	calls       int
	destination int64
	text        string
	permanent   bool
}

func (m *bookingTelegramMock) Send(_ context.Context, _ string, chat int64, payload json.RawMessage) (telegram.SendResult, error) {
	m.calls++
	m.destination = chat
	var body struct{ Text string }
	_ = json.Unmarshal(payload, &body)
	m.text = body.Text
	if m.permanent {
		return telegram.SendResult{}, io.ErrUnexpectedEOF
	}
	if m.calls == 1 {
		return telegram.SendResult{RetryAfter: 1}, io.ErrUnexpectedEOF
	}
	return telegram.SendResult{MessageID: 887}, nil
}

type bookingWhatsAppMock struct{ messages []channel.Message }

func (m *bookingWhatsAppMock) Send(_ context.Context, _ whatsapp.Config, msg channel.Message) (channel.SendResult, error) {
	m.messages = append(m.messages, msg)
	return channel.SendResult{State: channel.Sent, ProviderID: "synthetic-booking-" + strconv.Itoa(len(m.messages))}, nil
}

func testBookingNotifications(t *testing.T, d *db.Database, admin *pgx.Conn, ctx context.Context) {
	const origin = "http://127.0.0.1:3000"
	tgmock := &bookingTelegramMock{}
	wamock := &bookingWhatsAppMock{}
	tg := telegram.Service{Pool: d.Pool, Config: config.Telegram{Mode: "mock", Key: bytes.Repeat([]byte{121}, 32), Retention: 24 * time.Hour, Enabled: true}, Transport: tgmock}
	wa := whatsapp.Service{Pool: d.Pool, Key: bytes.Repeat([]byte{122}, 32), Transport: wamock}
	var telegramOwner stage23Owner
	for index, kind := range []string{"telegram", "whatsapp", "instagram"} {
		t.Run(kind, func(t *testing.T) {
			key := bytes.Repeat([]byte{byte(123 + index)}, 32)
			identity := auth.New(d.Pool, config.Auth{EmailKey: key, Origin: origin, SessionTTL: time.Hour, IdleTTL: time.Hour})
			svc := agentruntime.Service{Pool: d.Pool, Key: key}
			owner := seedStage23Owner(t, d, admin, ctx, identity, 141+index)
			if kind == "telegram" {
				telegramOwner = owner
			}
			if err := svc.UpdateProfile(ctx, owner.tenant, "service", "vehicle_service", 1); err != nil {
				t.Fatal(err)
			}
			selection, _ := stage26ConfigureBooking(t, ctx, owner.tenant, servicebooking.Service{Pool: d.Pool}, 950+index)
			org, conv := owner.tenant.OrganizationID, owner.conversation
			exec := func(q string, args ...any) {
				t.Helper()
				if _, err := admin.Exec(ctx, q, args...); err != nil {
					t.Fatal(err)
				}
			}
			exec("UPDATE platform.conversations SET channel=$1 WHERE organization_id=$2 AND id=$3", kind, org, conv)
			var integration string
			switch kind {
			case "telegram":
				if _, err := tg.Connect(ctx, owner.tenant, "mock:64027"); err != nil {
					t.Fatal(err)
				}
				if err := admin.QueryRow(ctx, "SELECT id FROM platform.telegram_integrations WHERE organization_id=$1", org).Scan(&integration); err != nil {
					t.Fatal(err)
				}
				exec("INSERT INTO platform.telegram_principals(id,organization_id,integration_id,user_id,chat_id,conversation_id) VALUES(gen_random_uuid(),$1,$2,82341,82341,$3)", org, integration, conv)
			case "whatsapp":
				credentials := whatsapp.Credentials{AccessToken: "mock:booking-wire", AppSecret: "mock:booking-app-secret", VerifyToken: "mock:booking-verify-token", AllowedUsers: []string{"82342"}}
				if _, err := wa.Connect(ctx, owner.tenant, whatsapp.ConnectInput{AppID: "100", BusinessID: "200", PhoneID: "642700", GraphVersion: "v0.0", Credentials: credentials}); err != nil {
					t.Fatal(err)
				}
				if _, err := wa.Control(ctx, owner.tenant, true, true); err != nil {
					t.Fatal(err)
				}
				if err := admin.QueryRow(ctx, "SELECT id FROM platform.channel_integrations WHERE organization_id=$1", org).Scan(&integration); err != nil {
					t.Fatal(err)
				}
				exec(`INSERT INTO platform.channel_principals(id,organization_id,integration_id,external_user_id,conversation_id,window_until)
 SELECT customer_id,organization_id,$2,'82342',id,now()+interval '1 hour' FROM platform.conversations WHERE organization_id=$1 AND id=$3`, org, integration, conv)
			}
			// A fixture for the customer command already persisted by the trusted inbound path.
			if kind != "instagram" {
				exec("INSERT INTO platform.messages(id,organization_id,conversation_id,role,content,client_key) VALUES(gen_random_uuid(),$1,$2,'customer','booking_updates:on',gen_random_uuid())", org, conv)
			}
			submit := func() string {
				t.Helper()
				parent, err := svc.SubmitService(ctx, owner.tenant, agentruntime.IndustryDelegationRequest[agentruntime.ServiceRequirements]{
					ConversationID: conv, IdempotencyKey: auth.UUID(), Requirements: agentruntime.ServiceRequirements{NoPart: true, Selection: &selection,
						Vehicle: catalog.FitmentVehicle{Make: "synthetic", Model: "service", Year: 2025}}})
				if err != nil {
					t.Fatal(err)
				}
				return parent
			}
			parent := submit()
			process := func(n int) {
				t.Helper()
				for i := 0; i < n; i++ {
					did, err := svc.ProcessOne(ctx)
					if !did || err != nil {
						t.Fatalf("worker %d: %t %v", i, did, err)
					}
				}
			}
			process(6)
			handler := httpapi.NewHandler(d, slog.New(slog.NewTextHandler(io.Discard, nil)), 3*time.Second, 65536, identity)
			post := func(path string, body any, want int) []byte {
				t.Helper()
				b, _ := json.Marshal(body)
				r := httptest.NewRequest(http.MethodPost, "/api/v1/organizations/"+org+path, bytes.NewReader(b))
				r.AddCookie(&http.Cookie{Name: "aba_session", Value: owner.token})
				r.Header.Set("Origin", origin)
				r.Header.Set("X-CSRF-Token", owner.csrf)
				r.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, r)
				if w.Code != want {
					t.Fatalf("%s status %d want %d: %s", path, w.Code, want, w.Body.String())
				}
				return w.Body.Bytes()
			}
			type preparedOrder struct {
				ID      string
				Version int64
			}
			prepare := func(task string) preparedOrder {
				t.Helper()
				var order preparedOrder
				if json.Unmarshal(post("/agent-delegations/auto-service/"+task+"/prepare", nil, 201), &order) != nil || order.ID == "" {
					t.Fatal("missing order")
				}
				return order
			}
			order := prepare(parent)
			path := "/orders/" + order.ID
			command := stage26Command(order.Version)
			post(path+"/confirm", command, 200)
			post(path+"/confirm", command, 200)
			if binary := os.Getenv("ABA_TEST_AGENT_WORKER_BINARY"); kind == "telegram" && binary != "" {
				var task string
				if err := admin.QueryRow(ctx, "SELECT id FROM platform.agent_tasks WHERE organization_id=$1 AND parent_id=$2 AND action='delegation.service_receipt'", org, parent).Scan(&task); err != nil {
					t.Fatal(err)
				}
				testStage23WorkerProcess(t, ctx, admin, binary, key, task, 1)
			} else {
				process(1)
			}
			var id, outcome string
			if err := admin.QueryRow(ctx, `SELECT t.id,a.result FROM platform.agent_tasks t JOIN platform.agent_audit a ON a.task_id=t.id AND a.organization_id=t.organization_id
   WHERE t.organization_id=$1 AND t.parent_id=$2 AND a.action='booking.delivery'`, org, parent).Scan(&id, &outcome); err != nil {
				t.Fatal(err)
			}
			if kind == "instagram" {
				if outcome != "instagram_send_disabled" {
					t.Fatal("Instagram kill switch was not represented")
				}
				post(path+"/cancel", stage26Command(order.Version+1), 200)
				process(1)
				var blocked, queued int
				if err := admin.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM platform.agent_audit a JOIN platform.agent_tasks t ON t.organization_id=a.organization_id AND t.id=a.task_id
  WHERE t.organization_id=$1 AND t.parent_id=$2 AND a.action='booking.delivery' AND a.result='instagram_send_disabled'),
 (SELECT count(*) FROM platform.channel_outbox WHERE organization_id=$1 AND conversation_id=$3)`, org, parent, conv).Scan(&blocked, &queued); err != nil || blocked != 2 || queued != 0 {
					t.Fatalf("Instagram confirmation/cancellation escaped safety block: audit=%d outbox=%d err=%v", blocked, queued, err)
				}
				return
			}
			if outcome != "queued" {
				t.Fatalf("notification is %s", outcome)
			}
			assertDelivery := func(deliveryID, state string) {
				t.Helper()
				r := httptest.NewRequest(http.MethodGet, "/api/v1/organizations/"+org+"/agent-audit?task_id="+deliveryID+"&action=booking.delivery_state", nil)
				r.AddCookie(&http.Cookie{Name: "aba_session", Value: owner.token})
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, r)
				if w.Code != 200 || !strings.Contains(w.Body.String(), `"result":"`+state+`"`) {
					t.Fatalf("delivery audit for owner: %d %s", w.Code, w.Body.String())
				}
			}
			checkText := func(text string) {
				t.Helper()
				for _, want := range []string{"Запись подтверждена.", "Услуга: Synthetic atomic service", "Время:", "UTC"} {
					if !strings.Contains(text, want) {
						t.Fatalf("missing receipt field %q", want)
					}
				}
			}
			if kind == "telegram" {
				if did, err := tg.SendOne(ctx); !did || err != nil {
					t.Fatalf("telegram retry: %t %v", did, err)
				}
				var status string
				if err := admin.QueryRow(ctx, "SELECT status FROM platform.telegram_outbox WHERE id=$1 AND organization_id=$2", id, org).Scan(&status); err != nil || status != "pending" {
					t.Fatalf("retry state %s: %v", status, err)
				}
				// A new worker instance reads the same durable pending delivery.
				restarted := telegram.Service{Pool: d.Pool, Config: tg.Config, Transport: tgmock}
				exec("UPDATE platform.telegram_outbox SET available_at=clock_timestamp() WHERE id=$1 AND organization_id=$2", id, org)
				if did, err := restarted.SendOne(ctx); !did || err != nil || tgmock.calls != 2 || tgmock.destination != 82341 {
					t.Fatalf("telegram delivery: %t %v calls=%d", did, err, tgmock.calls)
				}
				checkText(tgmock.text)
				assertDelivery(id, "sent")
				if did, err := restarted.SendOne(ctx); did || err != nil {
					t.Fatalf("duplicate telegram delivery: %t %v", did, err)
				}
				post(path+"/cancel", stage26Command(order.Version+1), 200)
				process(1)
				var cancelled string
				if err := admin.QueryRow(ctx, "SELECT id FROM platform.telegram_outbox WHERE organization_id=$1 AND id<>$2 AND booking_order_id=$3", org, id, order.ID).Scan(&cancelled); err != nil {
					t.Fatal(err)
				}
				tgmock.permanent = true
				if did, err := restarted.SendOne(ctx); !did || err != nil || tgmock.calls != 3 || !strings.Contains(tgmock.text, "Запись отменена.") {
					t.Fatalf("permanent failure on cancellation: %t %v calls=%d", did, err, tgmock.calls)
				}
				assertDelivery(cancelled, "failed")
			} else {
				foreign := httptest.NewRequest(http.MethodGet, "/api/v1/organizations/"+org+"/agent-audit?task_id="+id, nil)
				foreign.AddCookie(&http.Cookie{Name: "aba_session", Value: telegramOwner.token})
				forbidden := httptest.NewRecorder()
				handler.ServeHTTP(forbidden, foreign)
				if forbidden.Code != 404 && forbidden.Code != 403 {
					t.Fatalf("foreign delivery audit accepted: %d", forbidden.Code)
				}
				if did, err := wa.SendOne(ctx); !did || err != nil || len(wamock.messages) != 1 {
					t.Fatalf("whatsapp delivery: %t %v", did, err)
				}
				m := wamock.messages[0]
				if m.OrganizationID != org || m.ExternalUserID != "82342" || m.ConversationID != conv {
					t.Fatal("incorrect recipient binding")
				}
				checkText(m.Text)
				assertDelivery(id, "sent")
				post(path+"/cancel", stage26Command(order.Version+1), 200)
				process(1)
				exec("INSERT INTO platform.messages(id,organization_id,conversation_id,role,content,client_key) VALUES(gen_random_uuid(),$1,$2,'customer','booking_updates:off',gen_random_uuid())", org, conv)
				if did, err := wa.SendOne(ctx); !did || err != nil || len(wamock.messages) != 1 {
					t.Fatalf("revoked consent reached transport: %t %v", did, err)
				}
				parent = submit()
				process(6)
				order = prepare(parent)
				path = "/orders/" + order.ID
				post(path+"/confirm", stage26Command(order.Version), 200)
				process(1)
				if err := admin.QueryRow(ctx, `SELECT a.result FROM platform.agent_tasks t JOIN platform.agent_audit a ON a.task_id=t.id
 WHERE t.organization_id=$1 AND t.parent_id=$2 AND a.action='booking.delivery'`, org, parent).Scan(&outcome); err != nil || outcome != "consent_required" {
					t.Fatalf("missing consent: %s %v", outcome, err)
				}
				post(path+"/cancel", stage26Command(order.Version+1), 200)
				process(1)

				// Move completed fixture tasks outside the existing per-minute limit.
				exec("UPDATE platform.agent_tasks SET created_at=clock_timestamp()-interval '2 minutes' WHERE organization_id=$1 AND status='completed'", org)
				exec("INSERT INTO platform.messages(id,organization_id,conversation_id,role,content,client_key) VALUES(gen_random_uuid(),$1,$2,'customer','booking_updates:on',gen_random_uuid())", org, conv)
				parent = submit()
				process(6)
				order = prepare(parent)
				path = "/orders/" + order.ID
				post(path+"/confirm", stage26Command(order.Version), 200)
				process(1)
				if err := admin.QueryRow(ctx, "SELECT id FROM platform.channel_outbox WHERE organization_id=$1 AND booking_order_id=$2", org, order.ID).Scan(&id); err != nil {
					t.Fatal(err)
				}
				post(path+"/cancel", stage26Command(order.Version+1), 200)
				process(1)
				for i := 0; i < 2; i++ {
					if did, err := wa.SendOne(ctx); !did || err != nil {
						t.Fatalf("process confirmation and cancellation: %t %v", did, err)
					}
				}
				if len(wamock.messages) != 2 || !strings.Contains(wamock.messages[1].Text, "Запись отменена.") {
					t.Fatal("stale confirmation was delivered or cancellation was lost")
				}
				assertDelivery(id, "cancelled")
			}
		})
	}
}
