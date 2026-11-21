package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/channel"
)

// CloudError deliberately excludes upstream bodies, URLs, credentials and recipient IDs.
type CloudError struct {
	Code         string
	HTTPStatus   int
	ProviderCode int
}

func (e *CloudError) Error() string { return "WHATSAPP_" + e.Code }

// SendAdmission is a trusted backend boundary, never populated from a message/LLM.
// Reserve must atomically check current tenant/integration status, window or approved
// template, consent, known cost and platform/tenant budgets, keyed by CorrelationID.
// Finish must be durable/idempotent; Unknown retains the disputed reservation.
// FinancialAdmission supplies the durable backend; live flags/budgets remain disabled.
type SendAdmission interface {
	Reserve(context.Context, Config, channel.Message, *Template) (string, error)
	Finish(context.Context, string, channel.SendResult) error
}
type Template struct {
	Name       string
	Language   string
	Parameters []string
}
type CloudOptions struct {
	Enabled, ExternalAuthorized, Kill  bool
	OrganizationBudget, PlatformBudget int64
	SupportedVersions                  []string // operator-reviewed official versions; no default
	Admission                          SendAdmission
	Timeout                            time.Duration
	AllowVerifiedFree                  bool // explicit; DB admission must still verify a zero-cost rule
}
type CloudTransport struct {
	options   CloudOptions
	client    *http.Client
	localOnly bool
}

var _ Transport = (*CloudTransport)(nil)

// NewCloudTransport has a fixed official origin and no proxy/redirect support.
// Construction performs no DNS/network calls. Zero-value options fail closed.
func NewCloudTransport(o CloudOptions) *CloudTransport {
	tr := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 3 * time.Second}).DialContext, TLSHandshakeTimeout: 3 * time.Second, ResponseHeaderTimeout: 5 * time.Second, MaxIdleConns: 8, IdleConnTimeout: 30 * time.Second}
	t := newCloudTransport(o, tr)
	t.localOnly = false
	return t
}

// Only package tests inject a local HTTP emulator; endpoints never come from users.
func newCloudTransport(o CloudOptions, rt http.RoundTripper) *CloudTransport {
	if o.Timeout <= 0 || o.Timeout > 30*time.Second {
		o.Timeout = 5 * time.Second
	}
	o.SupportedVersions = append([]string(nil), o.SupportedVersions...)
	return &CloudTransport{localOnly: true, options: o, client: &http.Client{Transport: rt, Timeout: o.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func validAccessToken(s string) bool {
	if len(s) == 0 || len(s) > 256 || strings.ContainsAny(s, " \t\r\n\x00") {
		return false
	}
	return strings.HasPrefix(s, "mock:") || regexp.MustCompile(`^[A-Za-z0-9_.|\-]{16,256}$`).MatchString(s)
}

// ValidateConfig leaves the public mock configuration validator unchanged.
func (*CloudTransport) ValidateConfig(c Config) error {
	if !validAccessToken(c.AccessToken) {
		return channel.ErrRejected
	}
	c.AccessToken = "mock:validated-for-cloud"
	return c.Validate()
}

func (t *CloudTransport) Send(ctx context.Context, c Config, m channel.Message) (channel.SendResult, error) {
	return t.send(ctx, c, m, nil)
}

// Templates are a separate typed operation. Approval is established by Admission,
// not by a boolean or a template name supplied by the caller.
func (t *CloudTransport) SendTemplate(ctx context.Context, c Config, m channel.Message, v Template) (channel.SendResult, error) {
	return t.send(ctx, c, m, &v)
}

var templateName = regexp.MustCompile(`^[a-z0-9_]{1,512}$`)
var templateLanguage = regexp.MustCompile(`^[a-z]{2,3}(_[A-Z]{2})?$`)

func (t *CloudTransport) send(ctx context.Context, c Config, m channel.Message, v *Template) (channel.SendResult, error) {
	fail := func(code string) (channel.SendResult, error) {
		return channel.SendResult{State: channel.Failed}, &CloudError{Code: code}
	}
	if t == nil || t.client == nil || !t.options.Enabled || !t.options.ExternalAuthorized || t.options.Kill {
		return fail("DISABLED")
	}
	if t.options.Admission == nil || t.options.OrganizationBudget < 0 || t.options.PlatformBudget < 0 || ((t.options.OrganizationBudget == 0 || t.options.PlatformBudget == 0) && !t.options.AllowVerifiedFree) {
		return fail("BUDGET_BLOCKED")
	}
	if !t.localOnly {
		if _, ok := t.options.Admission.(*FinancialAdmission); !ok {
			return fail("DURABLE_ADMISSION_REQUIRED")
		}
	}
	if a, ok := t.options.Admission.(interface{ OfflineOnly() bool }); ok && a.OfflineOnly() && !t.localOnly {
		return fail("SYNTHETIC_ADMISSION_PROHIBITED")
	}
	supported := false
	for _, x := range t.options.SupportedVersions {
		if x == c.GraphVersion && x != "v0.0" && version.MatchString(x) {
			supported = true
		}
	}
	if !supported || t.ValidateConfig(c) != nil || m.Channel != channel.WhatsApp || m.OrganizationID != c.OrganizationID || m.IntegrationID != c.IntegrationID || m.Direction != "outbound" || m.ExternalChatID != m.ExternalUserID || !digits.MatchString(m.ExternalChatID) || !c.AllowedUsers[m.ExternalUserID] || m.CorrelationID == "" || len(m.CorrelationID) > 200 {
		return fail("INVALID_REQUEST")
	}
	if ctx.Err() != nil {
		return fail("CANCELLED_BEFORE_SEND")
	}
	body := map[string]any{"messaging_product": "whatsapp", "recipient_type": "individual", "to": m.ExternalChatID}
	if v != nil {
		if !templateName.MatchString(v.Name) || !templateLanguage.MatchString(v.Language) || len(v.Parameters) > 20 || len(m.Actions) > 0 {
			return fail("INVALID_TEMPLATE")
		}
		params := []map[string]string{}
		for _, s := range v.Parameters {
			if strings.TrimSpace(s) == "" || len(s) > 1024 || strings.ContainsRune(s, 0) {
				return fail("INVALID_TEMPLATE")
			}
			params = append(params, map[string]string{"type": "text", "text": s})
		}
		spec := map[string]any{"name": v.Name, "language": map[string]string{"code": v.Language}}
		if len(params) > 0 {
			spec["components"] = []any{map[string]any{"type": "body", "parameters": params}}
		}
		body["type"] = "template"
		body["template"] = spec
	} else {
		if m.Kind != channel.Text || len([]rune(m.Text)) < 1 || len([]rune(m.Text)) > 2000 || strings.ContainsRune(m.Text, 0) || len(m.Actions) > 3 {
			return fail("INVALID_REQUEST")
		}
		body["type"] = "text"
		body["text"] = map[string]any{"body": m.Text, "preview_url": false}
		if len(m.Actions) > 0 {
			buttons := []any{}
			seen := map[string]bool{}
			for _, a := range m.Actions {
				if len(a.ID) < 1 || len(a.ID) > 64 || len([]rune(a.Title)) < 1 || len([]rune(a.Title)) > 20 || strings.ContainsAny(a.ID, "\r\n\x00") || seen[a.ID] {
					return fail("INVALID_REQUEST")
				}
				seen[a.ID] = true
				buttons = append(buttons, map[string]any{"type": "reply", "reply": map[string]string{"id": a.ID, "title": a.Title}})
			}
			delete(body, "text")
			body["type"] = "interactive"
			body["interactive"] = map[string]any{"type": "button", "body": map[string]string{"text": m.Text}, "action": map[string]any{"buttons": buttons}}
		}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return fail("INVALID_REQUEST")
	}
	requestCtx, cancel := context.WithTimeout(ctx, t.options.Timeout)
	defer cancel()
	// Admission immediately precedes HTTP; it must recheck policy under the worker
	// integration lock. Existing outbox still owns lease/fencing and crash recovery.
	reservation, err := t.options.Admission.Reserve(requestCtx, c, m, v)
	if err != nil || reservation == "" {
		return fail("ADMISSION_DENIED")
	}
	result := channel.SendResult{State: channel.Unknown}
	finish := func(e error) (channel.SendResult, error) {
		settle, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()
		if t.options.Admission.Finish(settle, reservation, result) != nil {
			result.State = channel.Unknown
			return result, &CloudError{Code: "SETTLEMENT_UNKNOWN"}
		}
		return result, e
	}
	if requestCtx.Err() != nil {
		result.State = channel.Failed
		result.NoChargeConfirmed = true
		return finish(&CloudError{Code: "CANCELLED_BEFORE_SEND"})
	}
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, "https://graph.facebook.com/"+c.GraphVersion+"/"+c.PhoneNumberID+"/messages", bytes.NewReader(raw))
	if err != nil {
		result.State = channel.Failed
		result.NoChargeConfirmed = true
		return finish(&CloudError{Code: "INVALID_REQUEST"})
	}
	req.Header.Set("Authorization", "Bearer "+c.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return finish(&CloudError{Code: "DELIVERY_UNKNOWN"})
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 65537))
	if err != nil || len(b) > 65536 {
		return finish(&CloudError{Code: "DELIVERY_UNKNOWN"})
	}
	var out struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
		Error *struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	valid := json.Unmarshal(b, &out) == nil
	e := &CloudError{HTTPStatus: resp.StatusCode, Code: "DELIVERY_UNKNOWN"}
	if valid && out.Error != nil {
		e.ProviderCode = out.Error.Code
	}
	if resp.StatusCode == 429 {
		result.State = channel.Pending
		result.RetryAfter = time.Minute
		if n, x := strconv.Atoi(resp.Header.Get("Retry-After")); x == nil && n > 0 {
			result.RetryAfter = time.Duration(min(n, 3600)) * time.Second
		}
		e.Code = "RATE_LIMITED"
		return finish(e)
	}
	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		result.State = channel.Failed
		e.Code = "REJECTED"
		return finish(e)
	}
	if resp.StatusCode != 200 || !valid || out.Error != nil || len(out.Messages) != 1 || strings.TrimSpace(out.Messages[0].ID) == "" || len(out.Messages[0].ID) > 200 || strings.ContainsAny(out.Messages[0].ID, "\r\n\x00") {
		return finish(e)
	}
	result.State = channel.Sent
	result.ProviderID = out.Messages[0].ID
	return finish(nil)
}

// Ensure errors remain usable with errors.As without exposing upstream strings.
var _ error = (*CloudError)(nil)

// NewOfflineCloudTransport accepts only a literal loopback HTTP emulator URL.
// It cannot send to Graph even if configured with synthetic positive budgets.
func NewOfflineCloudTransport(o CloudOptions, endpoint string) (*CloudTransport, error) {
	a, ok := o.Admission.(interface{ OfflineOnly() bool })
	u, e := url.Parse(endpoint)
	if e != nil || u.Scheme != "http" || (u.Hostname() != "127.0.0.1" && u.Hostname() != "::1") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || !ok || !a.OfflineOnly() {
		return nil, channel.ErrRejected
	}
	rt := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 3 * time.Second}).DialContext, ResponseHeaderTimeout: 5 * time.Second}
	return newCloudTransport(o, loopbackCloud{target: u, transport: rt}), nil
}

type loopbackCloud struct {
	target    *url.URL
	transport http.RoundTripper
}

func (l loopbackCloud) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Scheme != "https" || r.URL.Host != "graph.facebook.com" {
		return nil, channel.ErrRejected
	}
	c := r.Clone(r.Context())
	u := *r.URL
	u.Scheme = l.target.Scheme
	u.Host = l.target.Host
	c.URL = &u
	c.Host = u.Host
	return l.transport.RoundTrip(c)
}

func (t *CloudTransport) withAdmission(a SendAdmission) *CloudTransport {
	clone := *t
	clone.options = t.options
	clone.options.Admission = a
	return &clone
}
