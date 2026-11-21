package instagram

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/channel"
)

var testNow = time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)

const testSecret = "synthetic-instagram-secret"

func binding() Binding { return Binding{"tenant-a", "integration-a", "100"} }
func fixture(account, sender, mid string, ts time.Time) []byte {
	raw, _ := json.Marshal(map[string]any{"object": "instagram", "entry": []any{map[string]any{"id": account, "messaging": []any{map[string]any{"sender": map[string]string{"id": sender}, "recipient": map[string]string{"id": account}, "timestamp": ts.UnixMilli(), "message": map[string]string{"mid": mid, "text": "Цена и наличие?"}}}}}})
	return raw
}
func signature(raw []byte) string {
	h := hmac.New(sha256.New, []byte(testSecret))
	h.Write(raw)
	return "sha256=" + hex.EncodeToString(h.Sum(nil))
}
func policy() Policy {
	return Policy{Active: true, SendingEnabled: true, PermissionsVerified: true, WebhookVerified: true, TokenExpiresAt: testNow.Add(time.Hour), LastInbound: testNow.Add(-time.Hour)}
}

func TestSignatureAndChallenge(t *testing.T) {
	raw := fixture("100", "200", "mid-1", testNow)
	if !VerifySignature(testSecret, signature(raw), raw) {
		t.Fatal("valid signature")
	}
	for _, tc := range []struct {
		secret, sig string
		raw         []byte
	}{{"", signature(raw), raw}, {testSecret, strings.Repeat("0", 71), raw}, {testSecret, signature(raw), append(raw, ' ')}, {testSecret, signature(raw), nil}, {testSecret, signature(raw), make([]byte, MaxPayload+1)}} {
		if VerifySignature(tc.secret, tc.sig, tc.raw) {
			t.Fatal("invalid signature accepted")
		}
	}
	q := url.Values{"hub.mode": {"subscribe"}, "hub.verify_token": {testSecret}, "hub.challenge": {"123"}}
	if result, err := Challenge(testSecret, q); err != nil || result != "123" {
		t.Fatal("valid challenge")
	}
	q.Add("hub.verify_token", testSecret)
	if _, err := Challenge(testSecret, q); err == nil {
		t.Fatal("duplicate token accepted")
	}
}

func TestNormalizeThreeTenantBindings(t *testing.T) {
	bindings := []Binding{binding(), {"tenant-b", "integration-b", "101"}, {"tenant-c", "integration-c", "102"}}
	for _, owner := range bindings {
		for _, target := range bindings {
			batch, err := Normalize(target, fixture(owner.AccountID, "200", "same-mid", testNow), testNow)
			if owner != target {
				if err == nil {
					t.Fatal("foreign account accepted")
				}
				continue
			}
			if err != nil || len(batch.Messages) != 1 {
				t.Fatalf("normalize: %v", err)
			}
			m := batch.Messages[0]
			if m.OrganizationID != owner.OrganizationID || m.IntegrationID != owner.IntegrationID || m.Channel != channel.Instagram || len(m.CorrelationID) != 36 {
				t.Fatal("identity binding")
			}
		}
	}
	first, _ := Normalize(bindings[0], fixture("100", "200", "same-mid", testNow), testNow)
	second, _ := Normalize(bindings[1], fixture("101", "200", "same-mid", testNow), testNow)
	if first.Messages[0].CorrelationID == second.Messages[0].CorrelationID {
		t.Fatal("cross-tenant idempotency collision")
	}
}

func TestReplayMalformedAndMixedEvents(t *testing.T) {
	for _, raw := range [][]byte{fixture("100", "200", "mid", testNow.Add(-25*time.Hour)), fixture("100", "200", "mid", testNow.Add(6*time.Minute)), fixture("100", "bad", "mid", testNow)} {
		b, err := Normalize(binding(), raw, testNow)
		if err != nil || b.Rejected != 1 || len(b.Messages) != 0 {
			t.Fatal("bad event accepted")
		}
	}
	for _, raw := range [][]byte{[]byte(`{`), []byte(`{"object":"instagram","entry":[]} {}`), fixture("999", "200", "mid", testNow), bytes.Repeat([]byte("a"), MaxPayload+1)} {
		if _, err := Normalize(binding(), raw, testNow); err == nil {
			t.Fatal("invalid payload accepted")
		}
	}
	var w map[string]any
	_ = json.Unmarshal(fixture("100", "200", "mid", testNow), &w)
	entry := w["entry"].([]any)[0].(map[string]any)
	valid := entry["messaging"].([]any)[0]
	entry["messaging"] = []any{"bad-event", map[string]any{"unknown": "event"}, valid, valid}
	raw, _ := json.Marshal(w)
	b, err := Normalize(binding(), raw, testNow)
	if err != nil || len(b.Messages) != 1 || b.Rejected != 1 || b.Ignored != 2 {
		t.Fatalf("mixed events: %+v %v", b, err)
	}
	m := valid.(map[string]any)["message"].(map[string]any)
	m["attachments"] = []any{map[string]any{"type": "image", "payload": map[string]string{"url": "http://127.0.0.1/private"}}, map[string]string{"type": "video"}, map[string]string{"type": "future-type"}}
	entry["messaging"] = []any{valid}
	raw, _ = json.Marshal(w)
	b, err = Normalize(binding(), raw, testNow)
	if err != nil || len(b.Messages) != 1 || b.Messages[0].Kind != channel.Unsupported || len(b.Messages[0].Attachments) != 3 || b.Messages[0].Attachments[2].Type != "unsupported" {
		t.Fatal("attachment handoff")
	}
}

func TestPolicyAndRetry(t *testing.T) {
	if policy().Check(testNow) != nil {
		t.Fatal("valid policy")
	}
	for _, modify := range []func(*Policy){func(p *Policy) { p.Active = false }, func(p *Policy) { p.SendingEnabled = false }, func(p *Policy) { p.PermissionsVerified = false }, func(p *Policy) { p.WebhookVerified = false }, func(p *Policy) { p.TokenExpiresAt = testNow }, func(p *Policy) { p.LastInbound = testNow.Add(-24 * time.Hour) }, func(p *Policy) { p.LastInbound = testNow.Add(time.Second) }, func(p *Policy) { p.Blocked = true }, func(p *Policy) { p.DeliveryUnknown = true }} {
		p := policy()
		modify(&p)
		if p.Check(testNow) == nil {
			t.Fatal("unsafe send admitted")
		}
	}
	for i := 1; i < 5; i++ {
		d, ok := RetryDelay(i, 30*time.Second)
		if !ok || d < 30*time.Second {
			t.Fatal("retry delay")
		}
	}
	for _, i := range []int{-1, 0, 5, 99} {
		if _, ok := RetryDelay(i, 0); ok {
			t.Fatal("unbounded retry")
		}
	}
}

func TestCredentialIsolation(t *testing.T) {
	box := CredentialBox{Key: bytes.Repeat([]byte{7}, 32)}
	sealed, err := box.Seal(binding(), []byte("mock:synthetic-token"))
	if err != nil {
		t.Fatal(err)
	}
	opened, err := box.Open(binding(), sealed)
	if err != nil || string(opened) != "mock:synthetic-token" {
		t.Fatal("roundtrip")
	}
	for _, b := range []Binding{{"tenant-b", "integration-a", "100"}, {"tenant-c", "integration-a", "100"}, {"tenant-a", "integration-b", "100"}, {"tenant-a", "integration-a", "101"}} {
		if _, err := box.Open(b, sealed); err == nil {
			t.Fatal("foreign credentials accepted")
		}
	}
	sealed[len(sealed)-1] ^= 1
	if _, err := box.Open(binding(), sealed); err == nil {
		t.Fatal("tamper accepted")
	}
}

type memoryStates struct {
	mu      sync.Mutex
	hash    [32]byte
	binding StateBinding
	expiry  time.Time
	used    bool
}

func (s *memoryStates) Create(_ context.Context, h [32]byte, b StateBinding, e time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hash = h
	s.binding = b
	s.expiry = e
	s.used = false
	return nil
}
func (s *memoryStates) Consume(_ context.Context, h [32]byte, b StateBinding, n time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.used || h != s.hash || b != s.binding || !n.Before(s.expiry) {
		return ErrRejected
	}
	s.used = true
	return nil
}
func TestOAuthStateBindingExpiryAndReplay(t *testing.T) {
	ctx := context.Background()
	store := &memoryStates{}
	b := StateBinding{"tenant-a", "user-a", "session-hash", "https://avero.example/instagram/callback"}
	o := OAuth{AppID: "123", RedirectURI: b.RedirectURI, States: store}
	start := func() url.Values {
		t.Helper()
		link, err := o.Start(ctx, b, testNow)
		if err != nil {
			t.Fatal(err)
		}
		u, _ := url.Parse(link)
		if u.Host != "www.instagram.com" || u.Query().Get("scope") != Permissions {
			t.Fatal("wrong auth model")
		}
		return url.Values{"state": {u.Query().Get("state")}, "code": {"synthetic-code"}}
	}
	q := start()
	for _, foreign := range []StateBinding{{"tenant-b", b.UserID, b.SessionID, b.RedirectURI}, {"tenant-c", b.UserID, b.SessionID, b.RedirectURI}, {b.OrganizationID, "user-b", b.SessionID, b.RedirectURI}, {b.OrganizationID, b.UserID, "other-session", b.RedirectURI}} {
		if _, err := o.Callback(ctx, foreign, q, testNow); err == nil {
			t.Fatal("foreign OAuth state")
		}
	}
	if _, err := o.Callback(ctx, b, q, testNow); err != nil {
		t.Fatal(err)
	}
	if _, err := o.Callback(ctx, b, q, testNow); err == nil {
		t.Fatal("replay")
	}
	q = start()
	if _, err := o.Callback(ctx, b, q, testNow.Add(10*time.Minute)); err == nil {
		t.Fatal("expired state")
	}
	q = start()
	q.Add("state", q.Get("state"))
	if _, err := o.Callback(ctx, b, q, testNow); err == nil {
		t.Fatal("duplicate state")
	}
	q = start()
	q.Set("error", "access_denied")
	if _, err := o.Callback(ctx, b, q, testNow); err == nil {
		t.Fatal("denied OAuth")
	}
	q.Del("error")
	if _, err := o.Callback(ctx, b, q, testNow); err == nil {
		t.Fatal("cancelled state reused")
	}
}

func TestWebhookAcknowledgement(t *testing.T) {
	calls := 0
	h := Webhook{Binding: binding(), AppSecret: testSecret, VerifyToken: testSecret, Now: func() time.Time { return testNow }, Enqueue: func(_ context.Context, m channel.Message) error {
		calls++
		if m.OrganizationID != "tenant-a" {
			t.Fatal("tenant spoof")
		}
		return nil
	}}
	raw := fixture("100", "200", "mid", testNow)
	post := func(body []byte, sig string) int {
		t.Helper()
		r := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
		r.Header.Set("X-Hub-Signature-256", sig)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	if post(raw, "invalid") != 403 || calls != 0 {
		t.Fatal("unverified queue access")
	}
	if post(raw, signature(raw)) != 200 || calls != 1 {
		t.Fatal("valid enqueue")
	}
	h.Enqueue = func(context.Context, channel.Message) error { return errors.New("synthetic database unavailable") }
	if post(raw, signature(raw)) != 503 {
		t.Fatal("acknowledged lost event")
	}
	if post(bytes.Repeat([]byte("x"), MaxPayload+1), "invalid") != 413 {
		t.Fatal("size limit")
	}
}

func TestOfflineSendClassificationAndIsolation(t *testing.T) {
	c := SendConfig{Binding: binding(), GraphVersion: "v27.0", AccessToken: "mock:synthetic-token", Policy: policy()}
	m := channel.Message{OrganizationID: "tenant-a", IntegrationID: "integration-a", Channel: channel.Instagram, Direction: "outbound", Kind: channel.Text, Text: "Ответ", ExternalUserID: "200", ExternalChatID: "200"}
	for _, tc := range []struct {
		name        string
		status      int
		body, after string
		want        channel.Delivery
	}{{"confirmed", 200, `{"recipient_id":"200","message_id":"mid"}`, "", channel.Sent}, {"wrong-recipient", 200, `{"recipient_id":"999","message_id":"mid"}`, "", channel.Unknown}, {"timeout-shaped", 502, `{"error":{"code":2}}`, "", channel.Unknown}, {"throttled", 429, `{"error":{"code":4}}`, "60", channel.Pending}, {"no-retry-proof", 429, `{"error":{"code":4}}`, "", channel.Failed}, {"expired-token", 400, `{"error":{"code":190}}`, "", channel.Failed}, {"malformed", 200, `{`, "", channel.Unknown}} {
		t.Run(tc.name, func(t *testing.T) {
			tr := OfflineTransport{RoundTrip: func(r *http.Request) (*http.Response, error) {
				if r.URL.Host != "graph.instagram.com" || r.URL.Path != "/v27.0/100/messages" || r.Header.Get("Authorization") != "Bearer mock:synthetic-token" {
					t.Fatal("wrong endpoint/token model")
				}
				return &http.Response{StatusCode: tc.status, Header: http.Header{"Retry-After": {tc.after}}, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			}}
			result, _ := tr.Send(context.Background(), c, m, testNow)
			if result.State != tc.want {
				t.Fatalf("got %s want %s", result.State, tc.want)
			}
		})
	}
	called := false
	tr := OfflineTransport{RoundTrip: func(*http.Request) (*http.Response, error) {
		called = true
		return nil, errors.New("synthetic timeout")
	}}
	for _, tenant := range []string{"tenant-b", "tenant-c"} {
		foreign := m
		foreign.OrganizationID = tenant
		if _, err := tr.Send(context.Background(), c, foreign, testNow); err == nil || called {
			t.Fatal("foreign send")
		}
	}
	oversize := m
	oversize.Text = strings.Repeat("я", 501)
	if _, err := tr.Send(context.Background(), c, oversize, testNow); err == nil || called {
		t.Fatal("1000 byte limit")
	}
	result, _ := tr.Send(context.Background(), c, m, testNow)
	if result.State != channel.Unknown {
		t.Fatal("timeout retried")
	}
}

func TestOfflineTransportRefusesUnsafeDeliveryBeforeFixture(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*SendConfig, *channel.Message)
	}{
		{"disabled", func(c *SendConfig, _ *channel.Message) { c.Policy.SendingEnabled = false }},
		{"disconnected", func(c *SendConfig, _ *channel.Message) { c.Policy.Active = false }},
		{"missing-permissions", func(c *SendConfig, _ *channel.Message) { c.Policy.PermissionsVerified = false }},
		{"unverified-webhook", func(c *SendConfig, _ *channel.Message) { c.Policy.WebhookVerified = false }},
		{"expired-token", func(c *SendConfig, _ *channel.Message) { c.Policy.TokenExpiresAt = testNow }},
		{"closed-window", func(c *SendConfig, _ *channel.Message) { c.Policy.LastInbound = testNow.Add(-24 * time.Hour) }},
		{"blocked-recipient", func(c *SendConfig, _ *channel.Message) { c.Policy.Blocked = true }},
		{"unknown-delivery", func(c *SendConfig, _ *channel.Message) { c.Policy.DeliveryUnknown = true }},
		{"non-mock-token", func(c *SendConfig, _ *channel.Message) { c.AccessToken = "synthetic-without-mock-prefix" }},
		{"wrong-integration", func(_ *SendConfig, m *channel.Message) { m.IntegrationID = "integration-b" }},
		{"wrong-chat", func(_ *SendConfig, m *channel.Message) { m.ExternalChatID = "201" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := SendConfig{Binding: binding(), GraphVersion: "v27.0", AccessToken: "mock:synthetic-token", Policy: policy()}
			m := channel.Message{OrganizationID: c.Binding.OrganizationID, IntegrationID: c.Binding.IntegrationID,
				Channel: channel.Instagram, Direction: "outbound", Kind: channel.Text, Text: "Ответ", ExternalUserID: "200", ExternalChatID: "200"}
			tc.edit(&c, &m)
			tr := OfflineTransport{RoundTrip: func(*http.Request) (*http.Response, error) {
				t.Fatal("unsafe delivery reached the provider fixture")
				return nil, nil
			}}
			if result, err := tr.Send(context.Background(), c, m, testNow); err == nil || result.State != channel.Failed {
				t.Fatalf("got %s, %v, want failed and an error", result.State, err)
			}
		})
	}
}
