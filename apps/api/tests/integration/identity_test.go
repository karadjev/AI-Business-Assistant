//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	httpapi "github.com/karadjev/AI-Business-Assistant/apps/api/internal/http"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/platform/db"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

func testIdentity(t *testing.T, database *db.Database, admin *pgx.Conn, ctx context.Context) {
	t.Helper()
	smtp := os.Getenv("TEST_SMTP_ADDR")
	if smtp == "" {
		smtp = "mailpit:1025"
	}
	cfg := config.Auth{Origin: "http://127.0.0.1:3000", EmailKey: bytes.Repeat([]byte{7}, 32), SMTPAddr: smtp, SessionTTL: time.Hour, IdleTTL: time.Hour, VerificationTTL: time.Hour}
	s := auth.New(database.Pool, cfg)
	h := httpapi.NewHandler(database, slog.New(slog.NewJSONHandler(io.Discard, nil)), 5*time.Second, 16384, s)
	type client struct {
		cookies []*http.Cookie
		csrf    string
	}
	a, b := &client{}, &client{}
	request := func(c *client, method, path string, body any, origin bool, csrf bool) *httptest.ResponseRecorder {
		t.Helper()
		data, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(data))
		r.RemoteAddr = "127.0.0.1:12345"
		r.Header.Set("Content-Type", "application/json")
		if origin {
			r.Header.Set("Origin", cfg.Origin)
		}
		for _, cookie := range c.cookies {
			r.AddCookie(cookie)
		}
		if csrf {
			r.Header.Set("X-CSRF-Token", c.csrf)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	expect := func(w *httptest.ResponseRecorder, status int) {
		t.Helper()
		if w.Code != status {
			t.Fatalf("expected %d got %d: %s", status, w.Code, w.Body.String())
		}
		for _, secret := range []string{"password_hash", "token_hash", "csrf_hash"} {
			if strings.Contains(w.Body.String(), secret) {
				t.Fatal("sensitive field exposed")
			}
		}
	}
	password := "Synthetic-Password-123!"
	emailA := "test-a@example.test"
	emailB := "test-b@example.test"
	register := func(email, name string) {
		expect(request(a, "POST", "/api/v1/auth/register", map[string]string{"email": email, "password": password, "display_name": name}, true, false), 202)
	}
	expect(request(a, "POST", "/api/v1/auth/register", map[string]string{"email": "bad", "password": "short", "display_name": "A"}, true, false), 422)
	register("  TEST-A@EXAMPLE.TEST ", "<img src=x onerror=alert(1)>")
	register(emailB, "User B")
	duplicate := request(a, "POST", "/api/v1/auth/register", map[string]string{"email": emailA, "password": password, "display_name": "Other"}, true, false)
	expect(duplicate, 202)
	if strings.Contains(duplicate.Body.String(), "exists") {
		t.Fatal("account enumeration")
	}
	for i := 0; i < 2; i++ {
		if _, e := s.DeliverOne(ctx); e != nil {
			t.Fatal(e)
		}
	}
	mailToken := func(email, linkPath string) string {
		t.Helper()
		base := os.Getenv("TEST_MAILPIT_URL")
		if base == "" {
			base = "http://mailpit:8025"
		}
		response, e := http.Get(base + "/api/v1/messages")
		if e != nil {
			t.Fatal("mail sink unavailable")
		}
		defer response.Body.Close()
		var list struct {
			Messages []struct {
				ID string
				To []struct{ Address string }
			}
		}
		if e = json.NewDecoder(response.Body).Decode(&list); e != nil {
			t.Fatal(e)
		}
		for _, message := range list.Messages {
			for _, to := range message.To {
				if to.Address == email {
					res, e := http.Get(base + "/api/v1/message/" + message.ID)
					if e != nil {
						t.Fatal(e)
					}
					var detail struct{ Text string }
					e = json.NewDecoder(res.Body).Decode(&detail)
					res.Body.Close()
					if e != nil {
						t.Fatal(e)
					}
					if !strings.Contains(detail.Text, linkPath) {
						continue
					}
					match := regexp.MustCompile(`token=([A-Za-z0-9_-]{43})`).FindStringSubmatch(detail.Text)
					if len(match) == 2 {
						return match[1]
					}
				}
			}
		}
		t.Fatal("verification email missing")
		return ""
	}
	tokenA, tokenB := mailToken(emailA, "verify-email"), mailToken(emailB, "verify-email")
	expect(request(a, "POST", "/api/v1/auth/login", map[string]string{"email": emailA, "password": password}, true, false), 401)
	expect(request(a, "POST", "/api/v1/auth/verify-email", map[string]string{"token": tokenA}, true, false), 204)
	expect(request(a, "POST", "/api/v1/auth/verify-email", map[string]string{"token": tokenA}, true, false), 422)
	if _, e := admin.Exec(ctx, `UPDATE platform.email_verification_tokens SET expires_at=now()-interval '1 second' WHERE token_hash=$1`, auth.Hash(tokenB)); e != nil {
		t.Fatal(e)
	}
	expect(request(b, "POST", "/api/v1/auth/verify-email", map[string]string{"token": tokenB}, true, false), 422)
	expect(request(b, "POST", "/api/v1/auth/resend-verification", map[string]string{"email": emailB}, true, false), 202)
	if _, e := s.DeliverOne(ctx); e != nil {
		t.Fatal(e)
	}
	tokenB = mailToken(emailB, "verify-email")
	expect(request(b, "POST", "/api/v1/auth/verify-email", map[string]string{"token": tokenB}, true, false), 204)
	expect(request(a, "POST", "/api/v1/auth/login", map[string]string{"email": emailA, "password": "Wrong-Password-123!"}, true, false), 401)
	login := func(c *client, email string) {
		w := request(c, "POST", "/api/v1/auth/login", map[string]string{"email": email, "password": password}, true, false)
		expect(w, 200)
		c.cookies = w.Result().Cookies()
		for _, cookie := range c.cookies {
			if cookie.Name == "aba_csrf" {
				c.csrf = cookie.Value
			}
			if cookie.Name == "aba_session" && (!cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode) {
				t.Fatal("unsafe session cookie")
			}
		}
	}
	login(a, emailA)
	login(b, emailB)
	expect(request(a, "GET", "/api/v1/auth/me", nil, false, false), 200)
	expect(request(&client{}, "GET", "/api/v1/auth/me", nil, false, false), 401)
	old := &client{cookies: a.cookies, csrf: a.csrf}
	login(a, emailA)
	expect(request(old, "GET", "/api/v1/auth/me", nil, false, false), 401)
	expect(request(a, "POST", "/api/v1/organizations", map[string]string{"name": "Shop A", "timezone": "Europe/Moscow"}, false, true), 403)
	expect(request(a, "POST", "/api/v1/organizations", map[string]string{"name": "Shop A", "timezone": "Europe/Moscow"}, true, false), 403)
	create := func(c *client, name string) string {
		w := request(c, "POST", "/api/v1/organizations", map[string]string{"name": name, "timezone": "Europe/Moscow"}, true, true)
		expect(w, 201)
		var o struct{ ID, Role string }
		if e := json.Unmarshal(w.Body.Bytes(), &o); e != nil {
			t.Fatal(e)
		}
		if o.Role != "owner" {
			t.Fatal("owner not assigned")
		}
		return o.ID
	}
	orgA, orgB := create(a, "Shop A"), create(b, "Shop B")
	orgA2 := create(a, "Shop A2")
	expect(request(a, "GET", "/api/v1/organizations/"+orgA2, nil, false, false), 200)
	expect(request(a, "GET", "/api/v1/organizations/"+orgB, nil, false, false), 404)
	edit := map[string]any{"name": "Edited A", "timezone": "Europe/Moscow", "expected_version": 1}
	expect(request(a, "PATCH", "/api/v1/organizations/"+orgB, edit, true, true), 404)
	expect(request(a, "PATCH", "/api/v1/organizations/"+orgA, edit, true, true), 200)
	expect(request(a, "PATCH", "/api/v1/organizations/"+orgA, edit, true, true), 409)
	var uidB string
	if e := admin.QueryRow(ctx, `SELECT id FROM platform.users WHERE email=$1`, emailB).Scan(&uidB); e != nil {
		t.Fatal(e)
	}
	if _, e := admin.Exec(ctx, `INSERT INTO platform.organization_members VALUES($1,$2,'member')`, orgA, uidB); e != nil {
		t.Fatal(e)
	}
	expect(request(b, "GET", "/api/v1/organizations/"+orgA, nil, false, false), 200)
	expect(request(b, "PATCH", "/api/v1/organizations/"+orgA, edit, true, true), 403)
	list := request(a, "GET", "/api/v1/organizations", nil, false, false)
	expect(list, 200)
	if strings.Contains(list.Body.String(), orgB) {
		t.Fatal("list leaked foreign org")
	}
	var sessionToken string
	for _, c := range a.cookies {
		if c.Name == "aba_session" {
			sessionToken = c.Value
		}
	}
	for _, target := range []string{orgA, orgB} {
		tx, e := database.Pool.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		_, e = tx.Exec(ctx, `SELECT set_config('app.session_hash',$1,true),set_config('app.organization_id',$2,true)`, hex.EncodeToString(auth.Hash(sessionToken)), target)
		if e != nil {
			t.Fatal(e)
		}
		var count int
		e = tx.QueryRow(ctx, `SELECT count(*) FROM platform.organizations`).Scan(&count)
		if e != nil {
			t.Fatal(e)
		}
		expected := 1
		if target == orgB {
			expected = 0
		}
		if count != expected {
			t.Fatal("RLS isolation failed")
		}
		tag, e := tx.Exec(ctx, `UPDATE platform.organizations SET name='SQL tamper' WHERE id=$1`, orgB)
		if e != nil || tag.RowsAffected() != 0 {
			t.Fatal("RLS write bypass")
		}
		if e = tx.Commit(ctx); e != nil {
			t.Fatal(e)
		}
	}
	for i := 0; i < 6; i++ {
		var count int
		if e := database.Pool.QueryRow(ctx, `SELECT count(*) FROM platform.organizations`).Scan(&count); e != nil || count != 0 {
			t.Fatal("tenant context leaked between pool transactions")
		}
	}
	var bToken string
	for _, cookie := range b.cookies {
		if cookie.Name == "aba_session" {
			bToken = cookie.Value
		}
	}
	tx, err := database.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `SELECT set_config('app.session_hash',$1,true),set_config('app.organization_id',$2,true)`, hex.EncodeToString(auth.Hash(bToken)), orgA); err != nil {
		t.Fatal(err)
	}
	tag, err := tx.Exec(ctx, `UPDATE platform.organizations SET name='member tamper' WHERE id=$1`, orgA)
	if err != nil || tag.RowsAffected() != 0 {
		t.Fatal("member bypassed owner RLS")
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var left string
	if err = database.Pool.QueryRow(ctx, `SELECT coalesce(current_setting('app.session_hash',true),'')`).Scan(&left); err != nil || left != "" {
		t.Fatal("rollback leaked session context")
	}
	func() {
		c1, e := database.Pool.Acquire(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer c1.Release()
		c2, e := database.Pool.Acquire(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer c2.Release()
		t1, e := c1.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer t1.Rollback(ctx)
		t2, e := c2.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer t2.Rollback(ctx)
		for i, transaction := range []pgx.Tx{t1, t2} {
			tokens := []string{sessionToken, bToken}
			ids := []string{orgA, orgB}
			if _, e = transaction.Exec(ctx, `SELECT set_config('app.session_hash',$1,true),set_config('app.organization_id',$2,true)`, hex.EncodeToString(auth.Hash(tokens[i])), ids[i]); e != nil {
				t.Fatal(e)
			}
			var n int
			if e = transaction.QueryRow(ctx, `SELECT count(*) FROM platform.organizations`).Scan(&n); e != nil || n != 1 {
				t.Fatal("separate pool connections mixed tenants")
			}
		}
		if e = t1.Commit(ctx); e != nil {
			t.Fatal(e)
		}
		if e = t2.Rollback(ctx); e != nil {
			t.Fatal(e)
		}
		for _, connection := range []interface {
			QueryRow(context.Context, string, ...any) pgx.Row
		}{c1, c2} {
			var n int
			if e = connection.QueryRow(ctx, `SELECT count(*) FROM platform.organizations`).Scan(&n); e != nil || n != 0 {
				t.Fatal("context leaked on reused physical connection")
			}
		}
	}()
	var guarded int
	if err = admin.QueryRow(ctx, `SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace JOIN pg_roles r ON r.oid=c.relowner WHERE n.nspname='platform' AND c.relname IN ('organizations','organization_members') AND c.relrowsecurity AND c.relforcerowsecurity AND r.rolname='aba_migrator'`).Scan(&guarded); err != nil || guarded != 2 {
		t.Fatal("tenant table ownership/FORCE RLS invalid")
	}
	if _, e := database.Pool.Exec(ctx, `SELECT * FROM platform.sessions`); e == nil {
		t.Fatal("runtime can enumerate session hashes")
	}
	var limited bool
	for i := 0; i < 11; i++ {
		w := request(a, "POST", "/api/v1/auth/login", map[string]string{"email": "missing@example.test", "password": password}, true, false)
		if w.Code == 429 {
			limited = true
		} else {
			expect(w, 401)
		}
	}
	if !limited {
		t.Fatal("login limiter missing")
	}
	// Fresh account key; allow two adjacent fixed-minute buckets instead of
	// depending on a resend performed much earlier in this integration test.
	limited = false
	for i := 0; i < 7; i++ {
		w := request(b, "POST", "/api/v1/auth/resend-verification", map[string]string{"email": "resend-limiter@example.test"}, true, false)
		if w.Code == 429 {
			limited = true
		} else {
			expect(w, 202)
		}
	}
	if !limited {
		t.Fatal("resend limiter missing")
	}
	expect(request(a, "POST", "/api/v1/auth/logout", nil, true, false), 403)
	expect(request(a, "POST", "/api/v1/auth/logout", nil, true, true), 204)
	expect(request(a, "GET", "/api/v1/auth/me", nil, false, false), 401)
	production := cfg
	production.Secure = true
	production.Origin = "https://platform.example.test"
	secureHandler := httpapi.NewHandler(database, slog.New(slog.NewJSONHandler(io.Discard, nil)), 5*time.Second, 16384, auth.New(database.Pool, production))
	body, _ := json.Marshal(map[string]string{"email": emailB, "password": password})
	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.2:1234"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", production.Origin)
	record := httptest.NewRecorder()
	secureHandler.ServeHTTP(record, req)
	expect(record, 200)
	var secureToken string
	for _, cookie := range record.Result().Cookies() {
		if !cookie.Secure {
			t.Fatal("production cookie missing Secure")
		}
		if cookie.Name == "__Host-aba_session" {
			secureToken = cookie.Value
			if !cookie.HttpOnly || cookie.Path != "/" || cookie.Domain != "" {
				t.Fatal("production session cookie unsafe")
			}
		}
	}
	if secureToken == "" {
		t.Fatal("missing __Host session cookie")
	}
	if _, err := admin.Exec(ctx, `UPDATE platform.sessions SET expires_at=now()-interval '1 second' WHERE token_hash=$1`, auth.Hash(secureToken)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(ctx, secureToken); err == nil {
		t.Fatal("absolute expiry ignored")
	}
	var bHash []byte
	for _, c := range b.cookies {
		if c.Name == "aba_session" {
			bHash = auth.Hash(c.Value)
		}
	}
	if _, e := admin.Exec(ctx, `UPDATE platform.sessions SET idle_expires_at=now()-interval '1 second' WHERE token_hash=$1`, bHash); e != nil {
		t.Fatal(e)
	}
	expect(request(b, "GET", "/api/v1/auth/me", nil, false, false), 401)
	activeLogin := request(b, "POST", "/api/v1/auth/login", map[string]string{"email": emailB, "password": password}, true, false)
	expect(activeLogin, 200)
	b.cookies = activeLogin.Result().Cookies()
	expect(request(b, "GET", "/api/v1/auth/me", nil, false, false), 200)
	unknownReset := request(a, "POST", "/api/v1/auth/password-reset", map[string]string{"email": "unknown-reset@example.test"}, true, false)
	expect(unknownReset, 202)
	knownReset := request(a, "POST", "/api/v1/auth/password-reset", map[string]string{"email": emailB}, true, false)
	expect(knownReset, 202)
	if knownReset.Body.String() != unknownReset.Body.String() {
		t.Fatal("password reset permits account enumeration")
	}
	for _, status := range []int{202, 202, 429} {
		expect(request(a, "POST", "/api/v1/auth/password-reset", map[string]string{"email": "unknown-reset@example.test"}, true, false), status)
	}
	if _, e := s.DeliverOne(ctx); e != nil {
		t.Fatal(e)
	}
	resetToken := mailToken(emailB, "reset-password")
	newPassword := "Synthetic-Changed-Password-456!"
	if _, e := admin.Exec(ctx, `UPDATE platform.password_reset_tokens SET expires_at=now()-interval '1 second' WHERE token_hash=$1`, auth.Hash(resetToken)); e != nil {
		t.Fatal(e)
	}
	expect(request(a, "POST", "/api/v1/auth/password-reset/confirm", map[string]string{"token": resetToken, "password": newPassword}, true, false), 422)
	expect(request(b, "GET", "/api/v1/auth/me", nil, false, false), 200)
	expect(request(a, "POST", "/api/v1/auth/password-reset", map[string]string{"email": emailB}, true, false), 202)
	if _, e := s.DeliverOne(ctx); e != nil {
		t.Fatal(e)
	}
	resetToken = mailToken(emailB, "reset-password")
	expect(request(a, "POST", "/api/v1/auth/password-reset/confirm", map[string]string{"token": resetToken, "password": newPassword}, true, false), 204)
	expect(request(a, "POST", "/api/v1/auth/password-reset/confirm", map[string]string{"token": resetToken, "password": password}, true, false), 422)
	expect(request(b, "GET", "/api/v1/auth/me", nil, false, false), 401)
	expect(request(a, "POST", "/api/v1/auth/login", map[string]string{"email": emailB, "password": newPassword}, true, false), 200)
}
