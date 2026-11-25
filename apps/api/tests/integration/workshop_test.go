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
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/agentruntime"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	httpapi "github.com/karadjev/AI-Business-Assistant/apps/api/internal/http"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/db"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/migrate"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
)

func TestRealWorkshop(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	migrationDSN := testDSN(t, "ABA_TEST_MIGRATION_DATABASE_URL", "aba_migrator")
	appDSN := testDSN(t, "ABA_TEST_DATABASE_URL", "aba_app")
	admin, err := pgx.Connect(ctx, migrationDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	var marker string
	if err = admin.QueryRow(ctx, `SELECT marker FROM platform.test_safety WHERE singleton`).Scan(&marker); err != nil || marker != "aba-synthetic-isolated" {
		t.Fatal("refusing to use database without isolation marker")
	}
	if err = migrate.Run(ctx, migrationDSN, "up"); err != nil {
		t.Fatal(err)
	}
	if err = migrate.Run(ctx, migrationDSN, "down"); err != nil {
		t.Fatalf("undo latest empty migration: %v", err)
	}
	if err = migrate.Run(ctx, migrationDSN, "up"); err != nil {
		t.Fatalf("reapply latest empty migration: %v", err)
	}
	d, err := db.Open(ctx, appDSN, 8, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err = d.Ready(ctx); err != nil {
		t.Fatal(err)
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := admin.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	key := bytes.Repeat([]byte{122}, 32)
	const origin = "http://workshop.example.test"
	identity := auth.New(d.Pool, config.Auth{Origin: origin, EmailKey: key, SessionTTL: time.Hour, IdleTTL: time.Hour})
	worker := agentruntime.Service{Pool: d.Pool, Key: key}
	handler := httpapi.NewHandler(d, slog.New(slog.NewTextHandler(io.Discard, nil)), 5*time.Second, 4096, identity)
	orgs := organizations.Service{Pool: d.Pool}
	type actor struct {
		token, csrf string
		session     auth.Session
	}
	seedActor := func() actor {
		t.Helper()
		uid := auth.UUID()
		out := actor{token: auth.Token(), csrf: auth.Token()}
		exec(`INSERT INTO platform.users(id,email,password_hash,display_name,email_verified_at) VALUES($1,$2,'synthetic','Workshop fixture',now())`, uid, uid+"@workshop.example.test")
		exec(`INSERT INTO platform.sessions(id,user_id,token_hash,csrf_hash,expires_at,idle_expires_at)
		  VALUES($1,$2,$3,$4,now()+interval '1 hour',now()+interval '1 hour')`, auth.UUID(), uid, auth.Hash(out.token), auth.Hash(out.csrf))
		var err error
		out.session, err = identity.Authenticate(ctx, out.token)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	owner, other := seedActor(), seedActor()
	create := func(user actor) organizations.Organization {
		t.Helper()
		org, err := orgs.Create(ctx, user.session, "Isolated workshop", "Europe/Moscow")
		if err != nil || org.IsTest {
			t.Fatalf("real-mode organization: %v", err)
		}
		exec(`UPDATE platform.organizations SET business_model='hybrid',industry_profile='vehicle_service' WHERE id=$1`, org.ID)
		return org
	}
	org, foreign := create(owner), create(other)
	demo, err := orgs.CreateTest(ctx, owner.session, "Isolated demo")
	if err != nil {
		t.Fatal(err)
	}
	call := func(user actor, id, method, path string, body any, want int) []byte {
		t.Helper()
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, "/api/v1/organizations/"+id+"/workshop"+path, bytes.NewReader(b)).WithContext(ctx)
		req.AddCookie(&http.Cookie{Name: "aba_session", Value: user.token})
		req.Header.Set("Origin", origin)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-CSRF-Token", user.csrf)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("%s %s: want %d got %d %s", method, path, want, w.Code, w.Body.String())
		}
		return w.Body.Bytes()
	}
	customerBody := map[string]string{"name": "Synthetic Pilot Customer"}
	call(owner, org.ID, "POST", "/customers", customerBody, 403)
	for _, id := range []string{org.ID, foreign.ID, demo.ID} {
		exec(`INSERT INTO platform.workshop_admissions(organization_id) VALUES($1)`, id)
	}
	call(owner, demo.ID, "POST", "/customers", customerBody, 403)
	noCSRF := owner
	noCSRF.csrf = ""
	call(noCSRF, org.ID, "POST", "/customers", customerBody, 403)
	call(other, org.ID, "GET", "", nil, 404)
	foreignMember := seedActor()
	exec(`INSERT INTO platform.organization_members(organization_id,user_id,role,can_manage_orders) VALUES($1,$2,'member',true)`, org.ID, foreignMember.session.User.ID)
	call(foreignMember, org.ID, "POST", "/customers", customerBody, 403)
	decodeID := func(raw []byte) string {
		t.Helper()
		var x struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(raw, &x); err != nil || x.ID == "" {
			t.Fatalf("expected id: %s", raw)
		}
		return x.ID
	}
	customer := decodeID(call(owner, org.ID, "POST", "/customers", customerBody, 201))
	foreignCustomer := decodeID(call(other, foreign.ID, "POST", "/customers", customerBody, 201))
	tenant := catalog.Tenant{OrganizationID: org.ID, Session: owner.session}
	foreignTenant := catalog.Tenant{OrganizationID: foreign.ID, Session: other.session}
	createPart := func(user catalog.Tenant, index int) string {
		t.Helper()
		part := seedInstagramCatalog(t, d, ctx, user, index)
		stock := catalog.Service{Pool: d.Pool}
		if _, err := stock.Adjust(ctx, user, catalog.Input{VariantID: part.VariantID, ExpectedVersion: 2, QuantityOnHand: 1, Reason: "One synthetic part"}); err != nil {
			t.Fatal(err)
		}
		return part.VariantID
	}
	part, foreignPart := createPart(tenant, 3201), createPart(foreignTenant, 3202)
	tx, err := tenancy.Begin(ctx, d.Pool, tenant, true, false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, "UPDATE platform.inventory SET quantity_reserved=1 WHERE variant_id=$1", part)
	_ = tx.Rollback(ctx)
	if err == nil {
		t.Fatal("real stock changed without an approved worker operation")
	}
	in := agentruntime.WorkshopRequest{CustomerID: foreignCustomer, VariantID: part, Quantity: 1, Vehicle: "Synthetic Car 2020", Service: "Replace filter", IdempotencyKey: auth.UUID()}
	call(owner, org.ID, "POST", "/requests", in, 403)
	in.CustomerID, in.VariantID = customer, foreignPart
	call(owner, org.ID, "POST", "/requests", in, 403)
	in.VariantID = part
	first := decodeID(call(owner, org.ID, "POST", "/requests", in, 202))
	if again := decodeID(call(owner, org.ID, "POST", "/requests", in, 202)); again != first {
		t.Fatal("request retried with a different ID")
	}
	in.IdempotencyKey = auth.UUID()
	second := decodeID(call(owner, org.ID, "POST", "/requests", in, 202))
	step := func() {
		t.Helper()
		if worked, err := worker.ProcessOne(ctx); err != nil || !worked {
			t.Fatalf("worker: worked=%t %v", worked, err)
		}
	}
	for range 8 {
		step()
	}
	var reserved int
	stock := func(want int) {
		t.Helper()
		if err := admin.QueryRow(ctx, `SELECT quantity_reserved FROM platform.inventory WHERE organization_id=$1 AND variant_id=$2`, org.ID, part).Scan(&reserved); err != nil || reserved != want {
			t.Fatalf("reserved=%d want=%d err=%v", reserved, want, err)
		}
	}
	stock(0)
	type approval struct{ id, hash string }
	lookup := func(request string) approval {
		t.Helper()
		var out approval
		if err := admin.QueryRow(ctx, `SELECT a.id,a.parameters_hash FROM platform.agent_approvals a JOIN platform.workshop_requests r
		  ON r.organization_id=a.organization_id AND coalesce(r.final_task_id,r.reserve_task_id)=a.task_id WHERE r.id=$1 AND a.status='pending'`, request).Scan(&out.id, &out.hash); err != nil {
			t.Fatal(err)
		}
		return out
	}
	a, b := lookup(first), lookup(second)
	if worked, err := worker.ProcessOne(ctx); err != nil || worked {
		t.Fatalf("worker processed unapproved task: %t %v", worked, err)
	}
	decide := func(user actor, orgID string, a approval, hash string, want int) {
		t.Helper()
		call(user, orgID, "POST", "/approvals/"+a.id+"/decision", map[string]string{"decision": "approved", "parameters_hash": hash}, want)
	}
	decide(other, foreign.ID, a, a.hash, 409)
	decide(foreignMember, org.ID, a, a.hash, 403)
	decide(owner, org.ID, a, b.hash, 409)
	decide(owner, org.ID, a, a.hash, 204)
	decide(owner, org.ID, a, a.hash, 409)
	decide(owner, org.ID, b, b.hash, 204)
	results := make(chan error, 2)
	for range 2 {
		go func() {
			worked, err := worker.ProcessOne(ctx)
			if err == nil && !worked {
				err = io.ErrUnexpectedEOF
			}
			results <- err
		}()
	}
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	stock(1)
	var holds, accepted, dispatched int
	if err = admin.QueryRow(ctx, `SELECT
	   (SELECT count(*) FROM platform.inventory_movements WHERE organization_id=$1 AND workshop_request_id IS NOT NULL),
	   (SELECT count(*) FROM platform.agent_approvals WHERE organization_id=$1 AND status='executed'),
	   (SELECT count(*) FROM platform.agent_audit WHERE organization_id=$1 AND action='task.finish' AND result='completed')`, org.ID).Scan(&holds, &accepted, &dispatched); err != nil || holds != 1 || accepted != 1 || dispatched != 9 {
		t.Fatalf("holds=%d accepted=%d completed=%d err=%v", holds, accepted, dispatched, err)
	}
	raw := call(owner, org.ID, "GET", "", nil, 200)
	for _, want := range []string{"Synthetic Pilot Customer", "approval.decision", "reserved", "insufficient_stock", "customer_verified", "delegation.workshop_customer", "delegation.workshop_stock", "delegation.workshop_sales", "Предложение по заявке"} {
		if !bytes.Contains(raw, []byte(want)) {
			t.Fatalf("missing %s from dashboard payload", want)
		}
	}
	if raw = call(other, foreign.ID, "GET", "", nil, 200); bytes.Contains(raw, []byte(first)) {
		t.Fatal("request exposed across organizations")
	}
	var active string
	if err := admin.QueryRow(ctx, `SELECT id FROM platform.workshop_requests WHERE organization_id=$1 AND reserved_at IS NOT NULL`, org.ID).Scan(&active); err != nil {
		t.Fatal(err)
	}
	closePath := func(id string) string { return "/requests/" + id + "/close" }
	operation := func(op string) map[string]string { return map[string]string{"operation": op} }
	call(foreignMember, org.ID, "POST", closePath(active), operation("consume"), 403)
	call(noCSRF, org.ID, "POST", closePath(active), operation("consume"), 403)
	call(other, foreign.ID, "POST", closePath(active), operation("consume"), 409)
	type competing struct {
		kind, id string
		err      error
	}
	choices := make(chan competing, 2)
	for _, kind := range []string{"release", "consume"} {
		go func() {
			id, err := worker.ProposeWorkshopClose(ctx, tenant, active, kind)
			choices <- competing{kind, id, err}
		}()
	}
	var chosen competing
	for range 2 {
		result := <-choices
		if result.err == nil {
			if chosen.id != "" {
				t.Fatal("two competing close operations admitted")
			}
			chosen = result
		} else if ae, ok := result.err.(*auth.Error); !ok || ae.Status != 409 {
			t.Fatal(result.err)
		}
	}
	if chosen.id == "" {
		t.Fatal("neither close proposal succeeded")
	}
	if chosen.kind == "consume" {
		c := lookup(active)
		call(owner, org.ID, "POST", "/approvals/"+c.id+"/decision", map[string]string{"decision": "rejected", "parameters_hash": c.hash}, 204)
	}
	release := decodeID(call(owner, org.ID, "POST", closePath(active), operation("release"), 202))
	if duplicate := decodeID(call(owner, org.ID, "POST", closePath(active), operation("release"), 202)); duplicate != release {
		t.Fatal("release retry created another task")
	}
	call(owner, org.ID, "POST", closePath(active), operation("consume"), 409)
	if worked, err := worker.ProcessOne(ctx); err != nil || worked {
		t.Fatalf("unapproved release processed: %t %v", worked, err)
	}
	stock(1)
	c := lookup(active)
	decide(owner, org.ID, c, c.hash, 204)
	step()
	decide(owner, org.ID, c, c.hash, 409)
	stock(0)
	var onHand int
	checkOnHand := func(want int) {
		t.Helper()
		if err := admin.QueryRow(ctx, `SELECT quantity_on_hand FROM platform.inventory WHERE organization_id=$1 AND variant_id=$2`, org.ID, part).Scan(&onHand); err != nil || onHand != want {
			t.Fatalf("on hand=%d want=%d err=%v", onHand, want, err)
		}
	}
	checkOnHand(1)
	call(owner, org.ID, "POST", closePath(active), operation("consume"), 409)
	in.IdempotencyKey = auth.UUID()
	consumed := decodeID(call(owner, org.ID, "POST", "/requests", in, 202))
	for range 4 {
		step()
	}
	c = lookup(consumed)
	decide(owner, org.ID, c, c.hash, 204)
	step()
	stock(1)
	call(owner, org.ID, "POST", closePath(consumed), operation("consume"), 202)
	c = lookup(consumed)
	decide(owner, org.ID, c, c.hash, 204)
	step()
	decide(owner, org.ID, c, c.hash, 409)
	call(owner, org.ID, "POST", closePath(consumed), operation("consume"), 409)
	stock(0)
	checkOnHand(0)
	var releaseCount, consumeCount int
	if err := admin.QueryRow(ctx, `SELECT count(*) FILTER(WHERE kind='release'),count(*) FILTER(WHERE kind='consume')
	 FROM platform.inventory_movements WHERE organization_id=$1 AND workshop_request_id IS NOT NULL`, org.ID).Scan(&releaseCount, &consumeCount); err != nil || releaseCount != 1 || consumeCount != 1 {
		t.Fatalf("release=%d consume=%d err=%v", releaseCount, consumeCount, err)
	}
	raw = call(owner, org.ID, "GET", "", nil, 200)
	for _, want := range []string{"workshop_release", "workshop_consume", "released", "consumed"} {
		if !bytes.Contains(raw, []byte(want)) {
			t.Fatalf("missing terminal result %s", want)
		}
	}
	// Move fixture setup outside of the existing minute-wide admission window.
	exec("UPDATE platform.agent_tasks SET created_at=created_at-interval '2 minutes' WHERE organization_id=$1", org.ID)
	financePath := func(id string) string { return "/requests/" + id + "/financials" }
	bill := func(paid bool) map[string]any {
		return map[string]any{"service_minor": 120050, "part_cost_minor": 40525, "paid": paid}
	}
	call(owner, org.ID, "POST", financePath(active), bill(false), 409)
	call(other, foreign.ID, "POST", financePath(consumed), bill(false), 409)
	call(foreignMember, org.ID, "POST", financePath(consumed), bill(false), 403)
	call(noCSRF, org.ID, "POST", financePath(consumed), bill(false), 403)
	call(owner, org.ID, "POST", financePath(consumed), map[string]any{"service_minor": 120050, "paid": false}, 422)
	unpaid := decodeID(call(owner, org.ID, "POST", financePath(consumed), bill(false), 202))
	if again := decodeID(call(owner, org.ID, "POST", financePath(consumed), bill(false), 202)); again != unpaid {
		t.Fatal("duplicate unpaid report")
	}
	call(owner, org.ID, "POST", financePath(consumed), bill(true), 409)
	report := func(id string, cash int64, paidCount int64) {
		t.Helper()
		for range 3 {
			step()
		}
		var raw []byte
		if err := admin.QueryRow(ctx, `SELECT result FROM platform.agent_executions WHERE organization_id=$1 AND task_id=$2 AND status='completed'`, org.ID, id).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var results []struct {
			Financial struct {
				Finance struct {
					Cash  int64 `json:"cash_minor"`
					Gross int64 `json:"gross_minor"`
				} `json:"finance"`
				Analytics struct {
					Cash     int64 `json:"cash_minor"`
					Gross    int64 `json:"gross_minor"`
					Recorded int64 `json:"recorded_count"`
					Paid     int64 `json:"paid_count"`
				} `json:"analytics"`
			} `json:"financial"`
		}
		if err := json.Unmarshal(raw, &results); err != nil || len(results) != 1 {
			t.Fatalf("financial report: %v", err)
		}
		f, m := results[0].Financial.Finance, results[0].Financial.Analytics
		if f.Cash != cash || f.Gross != 79525 || m.Cash != cash || m.Gross != 79525 || m.Recorded != 1 || m.Paid != paidCount {
			t.Fatalf("unexpected financial results: %+v %+v", f, m)
		}
	}
	report(unpaid, 0, 0)
	payment := decodeID(call(owner, org.ID, "POST", financePath(consumed), bill(true), 202))
	if payment == unpaid || decodeID(call(owner, org.ID, "POST", financePath(consumed), bill(true), 202)) != payment {
		t.Fatal("payment counted twice")
	}
	call(owner, org.ID, "POST", financePath(consumed), map[string]any{"service_minor": 120051, "part_cost_minor": 40525, "paid": true}, 409)
	report(payment, 120050, 1)
	raw = call(owner, org.ID, "GET", "", nil, 200)
	for _, want := range []string{"financial_report_ready", "workshop.payment.record", "delegation.workshop_finance", "delegation.workshop_analytics", "delegate_workshop_finance"} {
		if !bytes.Contains(raw, []byte(want)) {
			t.Fatalf("missing finance evidence %s", want)
		}
	}
	raw = call(other, foreign.ID, "GET", "", nil, 200)
	if bytes.Contains(raw, []byte(payment)) || bytes.Contains(raw, []byte("120050")) || bytes.Contains(raw, []byte("financial_report_ready")) {
		t.Fatal("financial report exposed across tenants")
	}
	var ledgerCount int
	if err := admin.QueryRow(ctx, "SELECT count(*) FROM platform.workshop_financials WHERE organization_id=$1 AND paid_at IS NOT NULL", org.ID).Scan(&ledgerCount); err != nil || ledgerCount != 1 {
		t.Fatalf("duplicate ledger: %d %v", ledgerCount, err)
	}
	marketingPath := "/requests/" + consumed + "/marketing"
	prepare := map[string]any{"action": "prepare"}
	consent := map[string]any{"action": "consent", "reference": "synthetic-consent-reference"}
	call(owner, org.ID, "POST", marketingPath, prepare, 409)
	call(other, foreign.ID, "POST", marketingPath, consent, 409)
	call(foreignMember, org.ID, "POST", marketingPath, consent, 409)
	call(noCSRF, org.ID, "POST", marketingPath, consent, 403)
	call(owner, org.ID, "POST", marketingPath, map[string]any{"action": "consent"}, 422)
	call(owner, org.ID, "POST", marketingPath, consent, 202)
	_ = createPart(tenant, 3203)
	exec(`INSERT INTO platform.service_offerings(organization_id,name,status,price_model,price_state,min_minutes,max_minutes,timezone)
	 VALUES($1,'Synthetic workshop service','active','on_request','unknown',30,60,'Europe/Moscow')`, org.ID)
	marketing := decodeID(call(owner, org.ID, "POST", marketingPath, prepare, 202))
	if decodeID(call(owner, org.ID, "POST", marketingPath, prepare, 202)) != marketing {
		t.Fatal("duplicate marketing task")
	}
	step()
	step()
	var draft string
	var channel, status string
	if err := admin.QueryRow(ctx, `SELECT d.id,d.channel,d.status FROM platform.marketing_drafts d JOIN platform.agent_tasks t ON t.organization_id=d.organization_id AND t.id=d.id
	 WHERE t.organization_id=$1 AND t.parent_id=$2`, org.ID, marketing).Scan(&draft, &channel, &status); err != nil || channel != "internal_review" || status != "pending" {
		t.Fatalf("internal draft: %s %s %v", channel, status, err)
	}
	raw = call(owner, org.ID, "GET", "", nil, 200)
	for _, want := range []string{"marketing_review", "Уточнить потребность клиента", "customer_completed", "active_services", "available_parts", "79525", "draft.persist"} {
		if !bytes.Contains(raw, []byte(want)) {
			t.Fatalf("missing marketing evidence %s", want)
		}
	}
	approveMarketing := map[string]any{"action": "approved", "version": 1, "draft_id": draft}
	call(other, foreign.ID, "POST", marketingPath, approveMarketing, 409)
	call(owner, org.ID, "POST", marketingPath, approveMarketing, 202)
	call(owner, org.ID, "POST", marketingPath, approveMarketing, 409)
	call(owner, org.ID, "POST", marketingPath, map[string]any{"action": "revoke"}, 202)
	raw = call(owner, org.ID, "GET", "", nil, 200)
	if bytes.Contains(raw, []byte("Добрый день! Спасибо")) {
		t.Fatal("draft visible after consent revocation")
	}
	call(owner, org.ID, "POST", marketingPath, prepare, 409)
	var sent int
	if err := admin.QueryRow(ctx, `SELECT
	 (SELECT count(*) FROM platform.channel_outbox WHERE organization_id=$1)
	 +(SELECT count(*) FROM platform.telegram_outbox WHERE organization_id=$1)`, org.ID).Scan(&sent); err != nil || sent != 0 {
		t.Fatalf("outbound activity: %d %v", sent, err)
	}

	// The workshop marketing history belongs to migration 35. Later migrations
	// can roll back on this fixture, which contains no data owned by them.
	const marketingHistoryVersion = 35
	currentVersion := func() int {
		t.Helper()
		var version int
		if err := admin.QueryRow(ctx, `SELECT max(version_id) FROM platform.goose_db_version WHERE is_applied`).Scan(&version); err != nil {
			t.Fatal("cannot determine isolated migration version", err)
		}
		return version
	}
	defer func() {
		if err := migrate.Run(ctx, migrationDSN, "up"); err != nil {
			t.Error("restore latest isolated schema", err)
		}
	}()
	for version := currentVersion(); version > marketingHistoryVersion; version = currentVersion() {
		if err := migrate.Run(ctx, migrationDSN, "down"); err != nil {
			t.Fatalf("cannot reach populated workshop marketing schema from version %d: %v", version, err)
		}
		if currentVersion() >= version {
			t.Fatalf("migration version %d did not roll back", version)
		}
	}
	if currentVersion() != marketingHistoryVersion {
		t.Fatal("unexpected schema before testing workshop history rollback")
	}
	if err = migrate.Run(ctx, migrationDSN, "down"); err == nil || currentVersion() != marketingHistoryVersion {
		t.Fatal("populated workshop marketing migration must refuse rollback")
	}
}
