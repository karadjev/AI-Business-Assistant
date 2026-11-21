// Package whatsapp implements a strict offline adapter for the official Cloud API wire format.
package whatsapp

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/channel"
)

type Config struct {
	OrganizationID, IntegrationID                         string
	AppID, BusinessAccountID, PhoneNumberID, GraphVersion string
	AccessToken, AppSecret, VerifyToken                   string `json:"-"` // backend only; never JSON serialized
	AllowedUsers                                          map[string]bool
	Window                                                time.Duration
}

var digits = regexp.MustCompile(`^[0-9]{1,20}$`)
var version = regexp.MustCompile(`^v[0-9]{1,3}\.[0-9]{1,2}$`)

func (c Config) Validate() error {
	if c.OrganizationID == "" || c.IntegrationID == "" || !digits.MatchString(c.AppID) || !digits.MatchString(c.BusinessAccountID) || !digits.MatchString(c.PhoneNumberID) || !version.MatchString(c.GraphVersion) || len(c.AppSecret) < 16 || len(c.AppSecret) > 256 || len(c.VerifyToken) < 16 || len(c.VerifyToken) > 256 || !strings.HasPrefix(c.AccessToken, "mock:") || len(c.AccessToken) > 256 || c.Window <= 0 || c.Window > 24*time.Hour || len(c.AllowedUsers) == 0 || len(c.AllowedUsers) > 100 {
		return channel.ErrRejected
	}
	for u := range c.AllowedUsers {
		if !digits.MatchString(u) {
			return channel.ErrRejected
		}
	}
	return nil
}
func Signature(secret string, raw []byte) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(raw)
	return "sha256=" + hex.EncodeToString(h.Sum(nil))
}
func VerifySignature(secret, signature string, raw []byte) bool {
	if len(raw) == 0 || !utf8.Valid(raw) || len(raw) > 65536 || len(signature) != 71 || !strings.HasPrefix(signature, "sha256=") {
		return false
	}
	got, e := hex.DecodeString(signature[7:])
	if e != nil {
		return false
	}
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(raw)
	return hmac.Equal(got, h.Sum(nil))
}
func (c Config) Challenge(q url.Values) (string, error) {
	if len(q["hub.mode"]) != 1 || len(q["hub.verify_token"]) != 1 || len(q["hub.challenge"]) != 1 || q.Get("hub.mode") != "subscribe" || subtle.ConstantTimeCompare([]byte(q.Get("hub.verify_token")), []byte(c.VerifyToken)) != 1 || !digits.MatchString(q.Get("hub.challenge")) {
		return "", channel.ErrRejected
	}
	return q.Get("hub.challenge"), nil
}

// Provider adds fields over time; only typed fields are trusted. Bodies are size/batch bounded.
type webhook struct {
	Object  string `json:"object"`
	Entries []struct {
		ID      string `json:"id"`
		Changes []struct {
			Field string `json:"field"`
			Value struct {
				Metadata struct {
					PhoneID string `json:"phone_number_id"`
				} `json:"metadata"`
				Messages []struct {
					From      string `json:"from"`
					ID        string `json:"id"`
					Timestamp string `json:"timestamp"`
					Type      string `json:"type"`
					Text      struct {
						Body string `json:"body"`
					} `json:"text"`
					Context struct {
						ID string `json:"id"`
					} `json:"context"`
					Interactive struct {
						Type   string `json:"type"`
						Button struct {
							ID string `json:"id"`
						} `json:"button_reply"`
					} `json:"interactive"`
				} `json:"messages"`
				Statuses []struct {
					ID        string `json:"id"`
					Status    string `json:"status"`
					Recipient string `json:"recipient_id"`
					Timestamp string `json:"timestamp"`
				} `json:"statuses"`
			} `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}

type Adapter struct {
	Manager   *Service
	Config    Config
	Transport Transport
}

func (Adapter) Channel() channel.Type { return channel.WhatsApp }
func (Adapter) Capabilities() channel.Capabilities {
	return channel.Capabilities{Text: true, Interactive: true, DeliveryReceipts: true}
}
func (a Adapter) Normalize(ctx context.Context, e channel.Envelope) ([]channel.Message, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if a.Config.Validate() != nil || e.IntegrationID != a.Config.IntegrationID || !VerifySignature(a.Config.AppSecret, e.Header["X-Hub-Signature-256"], e.Body) {
		return nil, channel.ErrRejected
	}
	var w webhook
	d := json.NewDecoder(bytes.NewReader(e.Body))
	if d.Decode(&w) != nil || w.Object != "whatsapp_business_account" || len(w.Entries) < 1 || len(w.Entries) > 10 {
		return nil, channel.ErrRejected
	}
	var tail any
	if d.Decode(&tail) != io.EOF {
		return nil, channel.ErrRejected
	}
	result := []channel.Message{}
	base := func(user, id, ts string) (channel.Message, error) {
		n, err := strconv.ParseInt(ts, 10, 64)
		if err != nil || n < 1 || n > e.ReceivedAt.Add(5*time.Minute).Unix() || !digits.MatchString(user) || !a.Config.AllowedUsers[user] || len(id) < 1 || len(id) > 200 || strings.ContainsRune(id, 0) {
			return channel.Message{}, channel.ErrRejected
		}
		return channel.Message{OrganizationID: a.Config.OrganizationID, IntegrationID: a.Config.IntegrationID, Channel: channel.WhatsApp, ExternalUserID: user, ExternalChatID: user, ExternalMessageID: id, Direction: "inbound", ProviderTimestamp: time.Unix(n, 0).UTC(), ReceivedAt: e.ReceivedAt, CorrelationID: id}, nil
	}
	for _, entry := range w.Entries {
		if entry.ID != a.Config.BusinessAccountID || len(entry.Changes) > 10 {
			return nil, channel.ErrRejected
		}
		for _, change := range entry.Changes {
			v := change.Value
			if change.Field != "messages" || v.Metadata.PhoneID != a.Config.PhoneNumberID || len(v.Messages)+len(v.Statuses) > 50 {
				return nil, channel.ErrRejected
			}
			for _, in := range v.Messages {
				m, err := base(in.From, in.ID, in.Timestamp)
				if err != nil {
					return nil, err
				}
				m.ReplyTo = in.Context.ID
				switch in.Type {
				case "text":
					m.Kind = channel.Text
					m.Text = in.Text.Body
					if strings.TrimSpace(m.Text) == "" || len([]rune(m.Text)) > 2000 || strings.ContainsRune(m.Text, 0) {
						return nil, channel.ErrRejected
					}
				case "interactive":
					m.Kind = channel.Interactive
					m.Action = in.Interactive.Button.ID
					if in.Interactive.Type != "button_reply" || len(m.Action) > 64 || !regexp.MustCompile(`^(offer|confirm):[0-9a-f-]{36}$`).MatchString(m.Action) {
						return nil, channel.ErrRejected
					}
				default:
					m.Kind = channel.Unsupported
					m.Attachments = []channel.Attachment{{Type: "unsupported"}}
				}
				result = append(result, m)
			}
			for _, in := range v.Statuses {
				m, err := base(in.Recipient, in.ID, in.Timestamp)
				if err != nil {
					return nil, err
				}
				m.Kind = channel.System
				m.DeliveryStatus = in.Status
				switch channel.Delivery(in.Status) {
				case channel.Sent, channel.Delivered, channel.Read, channel.Failed:
				default:
					return nil, channel.ErrRejected
				}
				result = append(result, m)
			}
		}
	}
	if len(result) == 0 || len(result) > 100 {
		return nil, channel.ErrRejected
	}
	return result, nil
}

// Transport is injected. Runtime remains mock until separately authorized.
type Transport interface {
	Send(context.Context, Config, channel.Message) (channel.SendResult, error)
}

func (a Adapter) Send(ctx context.Context, m channel.Message) (channel.SendResult, error) {
	validation := a.Config.Validate()
	if validator, ok := a.Transport.(interface{ ValidateConfig(Config) error }); ok {
		validation = validator.ValidateConfig(a.Config)
	}
	if validation != nil || m.Channel != channel.WhatsApp || m.OrganizationID != a.Config.OrganizationID || m.IntegrationID != a.Config.IntegrationID || !a.Config.AllowedUsers[m.ExternalUserID] || m.ExternalUserID != m.ExternalChatID || m.Direction != "outbound" || len([]rune(m.Text)) < 1 || len([]rune(m.Text)) > 2000 || a.Transport == nil {
		return channel.SendResult{}, channel.ErrRejected
	}
	return a.Transport.Send(ctx, a.Config, m)
}

type Mock struct{}

func (Mock) Send(ctx context.Context, c Config, m channel.Message) (channel.SendResult, error) {
	if ctx.Err() != nil {
		return channel.SendResult{}, ctx.Err()
	}
	return channel.SendResult{ProviderID: "mock:" + m.CorrelationID, State: channel.Sent}, nil
}

// HTTPMock sends actual wire requests only through the explicitly injected RoundTripper.
// There is no default client and no URL sourced from messages or configuration.
type HTTPMock struct{ Transport http.RoundTripper }

func (t HTTPMock) Send(ctx context.Context, c Config, m channel.Message) (channel.SendResult, error) {
	if t.Transport == nil {
		return channel.SendResult{}, channel.ErrRejected
	}
	raw, _ := json.Marshal(map[string]any{"messaging_product": "whatsapp", "to": m.ExternalChatID, "type": "text", "text": map[string]string{"body": m.Text}})
	if len(m.Actions) > 0 {
		if len(m.Actions) > 3 {
			return channel.SendResult{}, channel.ErrRejected
		}
		buttons := []map[string]any{}
		for _, a := range m.Actions {
			buttons = append(buttons, map[string]any{"type": "reply", "reply": map[string]string{"id": a.ID, "title": a.Title}})
		}
		raw, _ = json.Marshal(map[string]any{"messaging_product": "whatsapp", "to": m.ExternalChatID, "type": "interactive", "interactive": map[string]any{"type": "button", "body": map[string]string{"text": m.Text}, "action": map[string]any{"buttons": buttons}}})
	}
	req, err := http.NewRequestWithContext(ctx, "POST", "http://whatsapp.mock.invalid/"+c.GraphVersion+"/"+c.PhoneNumberID+"/messages", bytes.NewReader(raw))
	if err != nil {
		return channel.SendResult{}, channel.ErrRejected
	}
	req.Header.Set("Authorization", "Bearer "+c.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.Transport.RoundTrip(req)
	if err != nil {
		return channel.SendResult{State: channel.Unknown}, errors.New("WHATSAPP_DELIVERY_UNKNOWN")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16385))
	if err != nil || len(body) > 16384 {
		return channel.SendResult{State: channel.Unknown}, errors.New("WHATSAPP_DELIVERY_UNKNOWN")
	}
	if resp.StatusCode == 429 {
		delay := 60
		if n, e := strconv.Atoi(resp.Header.Get("Retry-After")); e == nil && n > 0 {
			delay = min(3600, max(delay, n))
		}
		return channel.SendResult{State: channel.Pending, RetryAfter: time.Duration(delay) * time.Second}, errors.New("WHATSAPP_RATE_LIMITED")
	}
	if resp.StatusCode >= 500 {
		return channel.SendResult{State: channel.Unknown}, errors.New("WHATSAPP_DELIVERY_UNKNOWN")
	}
	if resp.StatusCode != 200 {
		return channel.SendResult{State: channel.Failed}, errors.New("WHATSAPP_REJECTED")
	}
	var out struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
	}
	if json.Unmarshal(body, &out) != nil || len(out.Messages) != 1 || out.Messages[0].ID == "" || len(out.Messages[0].ID) > 200 {
		return channel.SendResult{State: channel.Unknown}, errors.New("WHATSAPP_DELIVERY_UNKNOWN")
	}
	return channel.SendResult{ProviderID: out.Messages[0].ID, State: channel.Sent}, nil
}

func (a Adapter) Status(ctx context.Context, t catalog.Tenant) (json.RawMessage, error) {
	if a.Manager == nil {
		return nil, channel.ErrRejected
	}
	return a.Manager.Status(ctx, t)
}
func (a Adapter) Disable(ctx context.Context, t catalog.Tenant) error {
	if a.Manager == nil {
		return channel.ErrRejected
	}
	_, e := a.Manager.Control(ctx, t, false, false)
	return e
}
func (a Adapter) CanSend(p channel.SendPolicy, now time.Time) error { return p.Check(now) }

// TemplateSender is separate from free text: a template must never silently fall
// back to Send outside the customer service window.
type TemplateSender interface {
	SendTemplate(context.Context, Config, channel.Message, Template) (channel.SendResult, error)
}

func (Mock) SendTemplate(ctx context.Context, c Config, m channel.Message, v Template) (channel.SendResult, error) {
	if c.Validate() != nil || m.Channel != channel.WhatsApp || m.OrganizationID != c.OrganizationID || m.IntegrationID != c.IntegrationID || m.Direction != "outbound" || !c.AllowedUsers[m.ExternalUserID] || m.ExternalChatID != m.ExternalUserID || !templateName.MatchString(v.Name) || !templateLanguage.MatchString(v.Language) {
		return channel.SendResult{State: channel.Failed}, channel.ErrRejected
	}
	return (Mock{}).Send(ctx, c, m)
}
