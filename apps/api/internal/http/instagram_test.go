package httpapi

import (
	"bytes"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/agent"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
)

func TestInstagramEndpointsRegisteredAndFailClosed(t *testing.T) {
	var logs bytes.Buffer
	identity := &auth.Service{Config: config.Auth{Origin: "http://127.0.0.1:3000"}}
	ac, _ := config.LoadAgent(func(string) string { return "" })
	h := NewHandlerWithChannels(&fakeDB{}, logger(&logs), time.Second, 65536, identity, ac, agent.Mock{}, config.Telegram{}, config.WhatsApp{})
	root := "/api/v1/organizations/11111111-1111-4111-8111-111111111111/integrations/instagram"
	for _, tc := range []struct {
		method, path string
		status       int
	}{{"GET", root, 401}, {"POST", root + "/oauth/start", 401}, {"GET", root + "/oauth/callback?state=invalid&code=synthetic", 401}, {"POST", root + "/disconnect", 401}, {"GET", root + "/webhook-status", 401}, {"GET", "/api/v1/webhooks/instagram/11111111-1111-4111-8111-111111111111", 503}, {"POST", "/api/v1/webhooks/instagram/11111111-1111-4111-8111-111111111111", 403}} {
		r := httptest.NewRequest(tc.method, tc.path, nil)
		r.Header.Set("Origin", identity.Config.Origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%s %s: %d want %d", tc.method, tc.path, w.Code, tc.status)
		}
	}
	if bytes.Contains(logs.Bytes(), []byte("state=invalid")) || bytes.Contains(logs.Bytes(), []byte("code=synthetic")) {
		t.Fatal("OAuth query leaked into logs")
	}
}

func TestWebhookChallengeContentTypeAndJSONTimeoutDefault(t *testing.T) {
	for _, tc := range []struct {
		provided, want string
		status         int
	}{{"text/plain; charset=utf-8", "text/plain; charset=utf-8", 200}, {"", "application/json; charset=utf-8", 200}, {"text/html; charset=utf-8", "application/json; charset=utf-8", 503}} {
		recorder := httptest.NewRecorder()
		writer := &responseWriter{ResponseWriter: recorder}
		writer.Header().Set("Content-Type", tc.provided)
		writer.WriteHeader(tc.status)
		if got := recorder.Result().Header.Get("Content-Type"); got != tc.want {
			t.Fatalf("content type %q want %q", got, tc.want)
		}
	}
}

// Nil DB dependencies deliberately make any pre-auth lookup fail. Rejection
// must happen before the rate limiter, account resolution or enqueue touches DB.
func TestInstagramSignedWebhookRejectsUntrustedInputBeforeDatabase(t *testing.T) {
	var logs bytes.Buffer
	identity := &auth.Service{Config: config.Auth{Origin: "http://127.0.0.1:3000"}}
	ac, _ := config.LoadAgent(func(string) string { return "" })
	cfg := config.Instagram{Enabled: true, Key: bytes.Repeat([]byte{49}, 32), AppSecret: "mock:synthetic-stage21-secret", VerifyToken: "mock:synthetic-stage21-verify"}
	h := NewHandlerWithChannels(&fakeDB{}, logger(&logs), time.Second, 65536, identity, ac, agent.Mock{}, config.Telegram{}, config.WhatsApp{}, cfg)
	for _, tc := range []struct {
		name    string
		body    []byte
		headers []string
		want    int
	}{
		{"missing", []byte(`{}`), nil, 403},
		{"forged", []byte(`{}`), []string{"sha256=" + strings.Repeat("0", 64)}, 403},
		{"duplicate", []byte(`{}`), []string{"sha256=" + strings.Repeat("0", 64), "sha256=" + strings.Repeat("0", 64)}, 403},
		{"oversized", bytes.Repeat([]byte("x"), 65537), nil, 413},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/api/v1/webhooks/instagram/11111111-1111-4111-8111-111111111111", bytes.NewReader(tc.body))
			for _, v := range tc.headers {
				r.Header.Add("X-Hub-Signature-256", v)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("untrusted request reached a database dependency: %d want %d", w.Code, tc.want)
			}
		})
	}
}

func TestInstagramLimiterDistinguishesThrottlingFromUnavailableStorage(t *testing.T) {
	for _, tc := range []struct {
		name  string
		err   error
		want  int
		retry string
	}{
		{"limit", auth.Failure(429, "RATE_LIMITED", "synthetic"), 429, "60"},
		{"database", auth.Unavailable(), 503, ""},
		{"unexpected", errors.New("postgresql://synthetic_sensitive@private"), 503, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("POST", "/webhook", nil)
			instagramLimiterError(w, r, tc.err)
			if w.Code != tc.want || w.Header().Get("Retry-After") != tc.retry {
				t.Fatalf("status=%d retry=%q", w.Code, w.Header().Get("Retry-After"))
			}
			if strings.Contains(w.Body.String(), "synthetic_sensitive") || strings.Contains(w.Body.String(), "private") {
				t.Fatal("storage error leaked")
			}
		})
	}
}
