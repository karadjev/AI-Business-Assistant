package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/channel"
)

type localGraph struct {
	base *url.URL
	rt   http.RoundTripper
}

func (l localGraph) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Scheme != "https" || r.URL.Host != "graph.facebook.com" {
		return nil, errors.New("unexpected origin")
	}
	c := r.Clone(r.Context())
	u := *r.URL
	u.Scheme = l.base.Scheme
	u.Host = l.base.Host
	c.URL = &u
	c.Host = l.base.Host
	return l.rt.RoundTrip(c)
}

type testAdmission struct {
	deny        bool
	window      time.Time
	approved    bool
	finishError bool
	reserves    atomic.Int32
	finishes    atomic.Int32
	state       channel.Delivery
}

func (a *testAdmission) Reserve(ctx context.Context, c Config, m channel.Message, v *Template) (string, error) {
	a.reserves.Add(1)
	if a.deny || ctx.Err() != nil || v == nil && !a.window.After(time.Now()) || v != nil && !a.approved {
		return "", errors.New("secret upstream text must never escape")
	}
	return "synthetic-reservation", nil
}
func (a *testAdmission) Finish(_ context.Context, _ string, r channel.SendResult) error {
	a.finishes.Add(1)
	a.state = r.State
	if a.finishError {
		return errors.New("synthetic failure")
	}
	return nil
}
func cloudFixture() (Config, channel.Message, CloudOptions, *testAdmission) {
	c := fixture()
	c.GraphVersion = "v99.0" // synthetic version, NOT an assertion of official support
	m := channel.Message{OrganizationID: c.OrganizationID, IntegrationID: c.IntegrationID, Channel: channel.WhatsApp, ExternalUserID: "700", ExternalChatID: "700", Direction: "outbound", Kind: channel.Text, Text: "synthetic text", CorrelationID: "outbox-synthetic-id"}
	a := &testAdmission{window: time.Now().Add(time.Hour), approved: true}
	return c, m, CloudOptions{Enabled: true, ExternalAuthorized: true, OrganizationBudget: 1, PlatformBudget: 1, SupportedVersions: []string{c.GraphVersion}, Admission: a, Timeout: time.Second}, a
}
func TestCloudWireContracts(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   channel.Delivery
		code   string
	}{
		{"text", 200, `{"messages":[{"id":"wamid.synthetic"}]}`, channel.Sent, ""},
		{"bad_token", 401, `{"error":{"code":190,"message":"SECRET"}}`, channel.Failed, "REJECTED"},
		{"expired_token", 400, `{"error":{"code":190,"error_subcode":463}}`, channel.Failed, "REJECTED"},
		{"400", 400, `{}`, channel.Failed, "REJECTED"}, {"403", 403, `{}`, channel.Failed, "REJECTED"},
		{"429", 429, `{"error":{"code":4}}`, channel.Pending, "RATE_LIMITED"},
		{"500", 500, `{}`, channel.Unknown, "DELIVERY_UNKNOWN"},
		{"malformed", 200, `{`, channel.Unknown, "DELIVERY_UNKNOWN"}, {"missing_id", 200, `{"messages":[]}`, channel.Unknown, "DELIVERY_UNKNOWN"},
		{"oversize", 200, strings.Repeat("x", 65537), channel.Unknown, "DELIVERY_UNKNOWN"},
		{"redirect", 302, `{}`, channel.Unknown, "DELIVERY_UNKNOWN"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, m, o, a := cloudFixture()
			var calls atomic.Int32
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != "POST" || r.URL.Path != "/v99.0/300/messages" || r.Header.Get("Authorization") != "Bearer "+c.AccessToken || r.Header.Get("Content-Type") != "application/json" {
					t.Error("incorrect official wire contract")
				}
				var b struct {
					Product string `json:"messaging_product"`
					To      string `json:"to"`
					Type    string `json:"type"`
					Text    struct {
						Body string `json:"body"`
					} `json:"text"`
				}
				if json.NewDecoder(r.Body).Decode(&b) != nil || b.Product != "whatsapp" || b.To != "700" || b.Type != "text" || b.Text.Body != m.Text {
					t.Error("incorrect payload")
				}
				w.Header().Set("Retry-After", "2")
				w.Header().Set("Location", "https://forbidden.invalid/")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer s.Close()
			u, _ := url.Parse(s.URL)
			tr := newCloudTransport(o, localGraph{u, s.Client().Transport})
			res, err := tr.Send(context.Background(), c, m)
			if res.State != tc.want || calls.Load() != 1 || a.finishes.Load() != 1 || a.state != tc.want {
				t.Fatalf("unexpected state %v calls=%v", res.State, calls.Load())
			}
			if tc.code == "" {
				if err != nil || res.ProviderID != "wamid.synthetic" {
					t.Fatal("success contract")
				}
			} else {
				var e *CloudError
				if !errors.As(err, &e) || e.Code != tc.code || strings.Contains(e.Error(), "SECRET") {
					t.Fatal("unsafe/untyped error")
				}
			}
			if tc.status == 429 && res.RetryAfter != 2*time.Second {
				t.Fatal("retry_after")
			}
		})
	}
}
func TestCloudTemplateAndInteractive(t *testing.T) {
	c, m, o, a := cloudFixture()
	a.window = time.Now().Add(-time.Hour)
	var kinds []string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		if json.NewDecoder(r.Body).Decode(&b) != nil {
			t.Error("JSON")
		}
		k, _ := b["type"].(string)
		kinds = append(kinds, k)
		if k == "template" {
			v := b["template"].(map[string]any)
			if v["name"] != "synthetic_approved" || v["language"].(map[string]any)["code"] != "ru" || len(v["components"].([]any)) != 1 {
				t.Error("template wire")
			}
		}
		_, _ = w.Write([]byte(`{"messages":[{"id":"wamid.template"}]}`))
	}))
	defer s.Close()
	u, _ := url.Parse(s.URL)
	tr := newCloudTransport(o, localGraph{u, s.Client().Transport})
	if _, err := tr.Send(context.Background(), c, m); err == nil {
		t.Fatal("expired text window accepted")
	}
	v := Template{Name: "synthetic_approved", Language: "ru", Parameters: []string{"synthetic parameter"}}
	if r, e := tr.SendTemplate(context.Background(), c, m, v); e != nil || r.State != channel.Sent {
		t.Fatal("approved template failed")
	}
	a.approved = false
	if _, e := tr.SendTemplate(context.Background(), c, m, v); e == nil {
		t.Fatal("unapproved template accepted")
	}
	a.window = time.Now().Add(time.Hour)
	m.Actions = []channel.Action{{ID: "offer:synthetic", Title: "Выбрать"}}
	if _, e := tr.Send(context.Background(), c, m); e != nil {
		t.Fatal(e)
	}
	if len(kinds) != 2 || kinds[0] != "template" || kinds[1] != "interactive" {
		t.Fatal("wrong types")
	}
}
func TestCloudTimeoutResetAndSettlement(t *testing.T) {
	for _, name := range []string{"timeout", "reset", "settlement"} {
		t.Run(name, func(t *testing.T) {
			c, m, o, a := cloudFixture()
			o.Timeout = 20 * time.Millisecond
			a.finishError = name == "settlement"
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch name {
				case "timeout":
					time.Sleep(60 * time.Millisecond)
					return
				case "reset":
					h, ok := w.(http.Hijacker)
					if !ok {
						t.Error("hijack")
						return
					}
					conn, _, _ := h.Hijack()
					_ = conn.Close()
					return
				}
				_, _ = w.Write([]byte(`{"messages":[{"id":"wamid.accepted"}]}`))
			}))
			defer s.Close()
			u, _ := url.Parse(s.URL)
			r, e := newCloudTransport(o, localGraph{u, s.Client().Transport}).Send(context.Background(), c, m)
			if r.State != channel.Unknown || e == nil || a.finishes.Load() != 1 {
				t.Fatal("uncertain result retried or released")
			}
		})
	}
}

type countTransport struct{ calls atomic.Int32 }

func (c *countTransport) RoundTrip(*http.Request) (*http.Response, error) {
	c.calls.Add(1)
	return nil, errors.New("must not send")
}
func TestCloudFailClosedNoHTTP(t *testing.T) {
	for _, name := range []string{"disabled", "zero_org_budget", "zero_platform_budget", "kill", "no_authorization", "no_admission", "version", "tenant", "integration", "user", "token_header", "token_oversize", "cancelled", "window", "denied"} {
		t.Run(name, func(t *testing.T) {
			c, m, o, a := cloudFixture()
			ctx := context.Background()
			switch name {
			case "disabled":
				o.Enabled = false
			case "zero_org_budget":
				o.OrganizationBudget = 0
			case "zero_platform_budget":
				o.PlatformBudget = 0
			case "kill":
				o.Kill = true
			case "no_authorization":
				o.ExternalAuthorized = false
			case "no_admission":
				o.Admission = nil
			case "version":
				c.GraphVersion = "v0.0"
			case "tenant":
				m.OrganizationID = "other"
			case "integration":
				m.IntegrationID = "other"
			case "user":
				m.ExternalUserID = "701"
			case "token_oversize":
				c.AccessToken = "mock:" + strings.Repeat("x", 257)
			case "token_header":
				c.AccessToken = "mock:secret\r\nInjected: yes"
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "window":
				a.window = time.Time{}
			case "denied":
				a.deny = true
			}
			rt := &countTransport{}
			r, e := newCloudTransport(o, rt).Send(ctx, c, m)
			if e == nil || r.State != channel.Failed || rt.calls.Load() != 0 {
				t.Fatal("fail-closed bypass")
			}
			if strings.Contains(e.Error(), "secret") {
				t.Fatal("secret leak")
			}
		})
	}
	// Public constructor also performs no request at construction/default Send.
	c, m, _, _ := cloudFixture()
	if _, e := NewCloudTransport(CloudOptions{}).Send(context.Background(), c, m); e == nil {
		t.Fatal("default enabled")
	}
}

func TestRuntimeCredentialsRemainSynthetic(t *testing.T) {
	if stringsSynthetic(Credentials{AccessToken: "syntheticRealFormatToken123", AppSecret: "mock:synthetic-secret", VerifyToken: "mock:synthetic-verify"}) {
		t.Fatal("runtime accepted non-mock access token")
	}
}

type offlineAdmissionFixture struct{ *testAdmission }

func (offlineAdmissionFixture) OfflineOnly() bool { return true }
func TestPublicCloudBoundaries(t *testing.T) {
	c, m, o, a := cloudFixture()
	// A fake/in-memory approval can never authorize the real Graph client.
	r, e := NewCloudTransport(o).Send(context.Background(), c, m)
	var ce *CloudError
	if !errors.As(e, &ce) || ce.Code != "DURABLE_ADMISSION_REQUIRED" || r.State != channel.Failed || a.reserves.Load() != 0 {
		t.Fatal("real transport accepted non-durable admission")
	}
	o.Admission = offlineAdmissionFixture{a}
	for _, endpoint := range []string{"https://graph.facebook.com", "http://example.invalid", "http://localhost:80", "http://127.0.0.1:80/path", "http://user:pass@127.0.0.1:80", "http://127.0.0.1:80?query=1"} {
		if _, e := NewOfflineCloudTransport(o, endpoint); e == nil {
			t.Fatal("unsafe offline origin accepted")
		}
	}
	if _, e := NewOfflineCloudTransport(o, "http://127.0.0.1:1"); e != nil {
		t.Fatal("local constructor", e)
	}
}
