package whatsapp

import (
	"context"
	"encoding/json"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/channel"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func fixture() Config {
	return Config{OrganizationID: "org-A", IntegrationID: "integration-A", AppID: "100", BusinessAccountID: "200", PhoneNumberID: "300", GraphVersion: "v0.0", AccessToken: "mock:synthetic", AppSecret: "mock:synthetic-secret", VerifyToken: "mock:synthetic-verify", AllowedUsers: map[string]bool{"700": true}, Window: 24 * time.Hour}
}
func body(now time.Time) []byte {
	return []byte(`{"object":"whatsapp_business_account","entry":[{"id":"200","changes":[{"field":"messages","value":{"metadata":{"phone_number_id":"300"},"messages":[{"from":"700","id":"synthetic-1","timestamp":"` + fmtUnix(now) + `","type":"text","text":{"body":"Есть TEST-S25?"}}]}}]}]}`)
}
func fmtUnix(t time.Time) string {
	return json.Number(strings.TrimSpace(string(mustJSON(t.Unix())))).String()
}
func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
func TestWebhookContract(t *testing.T) {
	now := time.Now().UTC()
	c := fixture()
	raw := body(now)
	for _, tc := range []struct {
		name   string
		modify func(*channel.Envelope)
		valid  bool
	}{
		{"valid", func(*channel.Envelope) {}, true}, {"wrong_signature", func(e *channel.Envelope) { e.Header["X-Hub-Signature-256"] = Signature("other", e.Body) }, false},
		{"wrong_integration", func(e *channel.Envelope) { e.IntegrationID = "integration-B" }, false},
		{"wrong_phone", func(e *channel.Envelope) {
			e.Body = []byte(strings.ReplaceAll(string(raw), `"300"`, `"301"`))
			e.Header["X-Hub-Signature-256"] = Signature(c.AppSecret, e.Body)
		}, false},
		{"wrong_account", func(e *channel.Envelope) {
			e.Body = []byte(strings.ReplaceAll(string(raw), `"200"`, `"201"`))
			e.Header["X-Hub-Signature-256"] = Signature(c.AppSecret, e.Body)
		}, false},
		{"allowlist", func(e *channel.Envelope) {
			e.Body = []byte(strings.ReplaceAll(string(raw), `"700"`, `"701"`))
			e.Header["X-Hub-Signature-256"] = Signature(c.AppSecret, e.Body)
		}, false},
		{"media", func(e *channel.Envelope) {
			e.Body = []byte(strings.ReplaceAll(string(raw), `"type":"text"`, `"type":"video"`))
			e.Header["X-Hub-Signature-256"] = Signature(c.AppSecret, e.Body)
		}, true},
		{"trailing_json", func(e *channel.Envelope) {
			e.Body = append(e.Body, []byte(`{}`)...)
			e.Header["X-Hub-Signature-256"] = Signature(c.AppSecret, e.Body)
		}, false},
		{"oversize", func(e *channel.Envelope) {
			e.Body = []byte(strings.Repeat("x", 65537))
			e.Header["X-Hub-Signature-256"] = Signature(c.AppSecret, e.Body)
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := channel.Envelope{IntegrationID: c.IntegrationID, Header: map[string]string{"X-Hub-Signature-256": Signature(c.AppSecret, raw)}, Body: append([]byte{}, raw...), ReceivedAt: now}
			tc.modify(&e)
			m, err := (Adapter{Config: c}).Normalize(context.Background(), e)
			if (err == nil) != tc.valid {
				t.Fatal("validation outcome", err)
			}
			if tc.valid && (len(m) != 1 || m[0].OrganizationID != c.OrganizationID || m[0].Channel != channel.WhatsApp) {
				t.Fatal("routing")
			}
		})
	}
	q := url.Values{"hub.mode": {"subscribe"}, "hub.verify_token": {c.VerifyToken}, "hub.challenge": {"12345"}}
	if v, e := c.Challenge(q); v != "12345" || e != nil {
		t.Fatal("challenge", e)
	}
	q.Set("hub.verify_token", "wrong")
	if _, e := c.Challenge(q); e == nil {
		t.Fatal("challenge bypass")
	}
}

type rewrite struct {
	base string
	t    http.RoundTripper
}

func (r rewrite) RoundTrip(req *http.Request) (*http.Response, error) {
	u, _ := url.Parse(r.base)
	copy := req.Clone(req.Context())
	copy.URL.Scheme = u.Scheme
	copy.URL.Host = u.Host
	return r.t.RoundTrip(copy)
}
func TestHTTPWireContract(t *testing.T) {
	c := fixture()
	for _, test := range []struct {
		name     string
		code     int
		response string
		state    channel.Delivery
	}{{"sent", 200, `{"messages":[{"id":"mock-wire-1"}]}`, channel.Sent}, {"429", 429, `{}`, channel.Pending}, {"500", 500, `{}`, channel.Unknown}, {"rejected", 401, `{}`, channel.Failed}, {"bad_json", 200, `{}`, channel.Unknown}} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "POST" || r.URL.Path != "/v0.0/300/messages" || r.Header.Get("Authorization") != "Bearer "+c.AccessToken {
					t.Error("wrong wire request")
				}
				b, _ := io.ReadAll(r.Body)
				var data map[string]any
				if json.Unmarshal(b, &data) != nil || data["messaging_product"] != "whatsapp" || data["to"] != "700" {
					t.Error("wrong request body")
				}
				w.WriteHeader(test.code)
				_, _ = w.Write([]byte(test.response))
			}))
			defer s.Close()
			transport := HTTPMock{Transport: rewrite{base: s.URL, t: s.Client().Transport}}
			result, e := transport.Send(context.Background(), c, channel.Message{Text: "Synthetic", ExternalChatID: "700"})
			if result.State != test.state || calls != 1 || (test.code == 200 && test.name == "sent" && e != nil) {
				t.Fatal("wire result", result, e, calls)
			}
		})
	}
}
func TestPrincipalNamespaceAndPolicy(t *testing.T) {
	m := channel.Message{OrganizationID: "a", IntegrationID: "i", Channel: channel.WhatsApp, ExternalUserID: "700"}
	p := m.Principal()
	m.Channel = channel.Telegram
	if p == m.Principal() {
		t.Fatal("cross-channel collision")
	}
	m.Channel = channel.WhatsApp
	m.IntegrationID = "j"
	if p == m.Principal() {
		t.Fatal("cross-integration collision")
	}
	now := time.Now()
	pcy := channel.SendPolicy{Mode: "mock", Active: true, WindowUntil: now.Add(time.Hour)}
	if pcy.Check(now) != nil {
		t.Fatal("mock blocked")
	}
	pcy.Mode = "live"
	if pcy.Check(now) == nil {
		t.Fatal("live zero budget bypass")
	}
	pcy.Mode = "mock"
	pcy.Kill = true
	if pcy.Check(now) == nil {
		t.Fatal("kill bypass")
	}
	pcy.Kill = false
	pcy.WindowUntil = now
	if e := pcy.Check(now); e == nil || e.Error() != "NEEDS_APPROVED_TEMPLATE" {
		t.Fatal("closed window")
	}
	if channel.AdvanceReceipt(channel.Read, channel.Sent) != channel.Read || channel.AdvanceReceipt(channel.Delivered, channel.Failed) != channel.Delivered {
		t.Fatal("status regression")
	}
}
func TestCipherIsolation(t *testing.T) {
	key := []byte(strings.Repeat("s", 32))
	raw := []byte("mock:synthetic-secret")
	sealed, e := seal(key, "a", raw)
	if e != nil || strings.Contains(string(sealed), string(raw)) {
		t.Fatal("cipher")
	}
	if _, e = open(key, "b", sealed); e == nil {
		t.Fatal("tenant AAD")
	}
	if _, e = open(key, "a", sealed); e != nil {
		t.Fatal(e)
	}
}

func TestTimeoutAndCancelledContextAreUnknown(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(200 * time.Millisecond):
		}
	}))
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	result, e := (HTTPMock{Transport: rewrite{base: s.URL, t: s.Client().Transport}}).Send(ctx, fixture(), channel.Message{Text: "Synthetic", ExternalChatID: "700"})
	if e == nil || result.State != channel.Unknown {
		t.Fatal("timeout treated as safe retry", result, e)
	}
}
func TestNoCredentialJSONAndNoLiveOrDisabledSend(t *testing.T) {
	c := fixture()
	raw, e := json.Marshal(c)
	if e != nil || strings.Contains(string(raw), c.AccessToken) || strings.Contains(string(raw), c.AppSecret) || strings.Contains(string(raw), c.VerifyToken) {
		t.Fatal("credential serialization")
	}
	c.AccessToken = "non-mock-prohibited"
	if _, e := (Adapter{Config: c, Transport: Mock{}}).Send(context.Background(), channel.Message{OrganizationID: c.OrganizationID, IntegrationID: c.IntegrationID, Channel: channel.WhatsApp, ExternalUserID: "700", ExternalChatID: "700", Direction: "outbound", Text: "Synthetic"}); e == nil {
		t.Fatal("real credential accepted")
	}
	// Disabled application instance has no pool/transport calls at all.
	if found, e := (Service{}).SendOne(context.Background()); e != nil || found {
		t.Fatal("disabled sending attempted work", e)
	}
}
