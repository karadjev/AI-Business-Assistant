package telegram

import (
	"context"
	"encoding/json"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/channel"
	"strconv"
	"time"
)

// Adapter preserves legacy webhook/inbox/outbox storage and its fencing/recovery guarantees.
type Adapter struct {
	guard          *sendGuard
	payload        json.RawMessage
	Service        *Service
	OrganizationID string
	IntegrationID  string
	token          string
}

var _ channel.Adapter = Adapter{}

func (Adapter) Channel() channel.Type { return channel.Telegram }
func (Adapter) Capabilities() channel.Capabilities {
	return channel.Capabilities{Text: true, Interactive: true}
}
func (a Adapter) Normalize(ctx context.Context, e channel.Envelope) ([]channel.Message, error) {
	if a.Service == nil || e.IntegrationID != a.IntegrationID {
		return nil, channel.ErrRejected
	}
	var u Update
	if json.Unmarshal(e.Body, &u) != nil {
		return nil, channel.ErrRejected
	}
	user, chat, err := u.Principal()
	if err != nil {
		return nil, channel.ErrRejected
	}
	// Ingress authenticates webhook, derives authoritative tenant in SQL and deduplicates durably.
	if _, err = a.Service.Ingress(ctx, e.IntegrationID, e.Header["X-Telegram-Bot-Api-Secret-Token"], u); err != nil {
		return nil, err
	}
	m := channel.Message{IntegrationID: e.IntegrationID, Channel: channel.Telegram, ExternalUserID: strconv.FormatInt(user, 10), ExternalChatID: strconv.FormatInt(chat, 10), ExternalMessageID: strconv.FormatInt(u.ID, 10), Direction: "inbound", ReceivedAt: e.ReceivedAt}
	// OrganizationID is intentionally absent: the claimed inbox job is the trusted routing source.
	if u.Message != nil {
		m.Kind = channel.Text
		m.Text = u.Message.Text
	} else {
		m.Kind = channel.Interactive
		m.Action = u.Callback.Data
	}
	return []channel.Message{m}, nil
}
func (a Adapter) Send(ctx context.Context, m channel.Message) (channel.SendResult, error) {
	// Only Service.SendOne can construct a bound adapter with the private pinned guard.
	if a.Service == nil || a.guard == nil || a.guard.conn == nil || m.OrganizationID != a.OrganizationID || m.Direction != "outbound" || m.ExternalUserID != m.ExternalChatID {
		return channel.SendResult{}, channel.ErrRejected
	}
	chat, e := strconv.ParseInt(m.ExternalChatID, 10, 64)
	if e != nil || !a.Service.Config.Allowed(chat) {
		return channel.SendResult{}, channel.ErrRejected
	}
	r, e := a.Service.Transport.Send(ctx, a.token, chat, a.payload)
	state := channel.Sent
	if e != nil {
		state = channel.Failed
		if r.Uncertain {
			state = channel.Unknown
		} else if r.RetryAfter > 0 {
			state = channel.Pending
		}
	}
	return channel.SendResult{ProviderID: strconv.FormatInt(r.MessageID, 10), State: state, RetryAfter: time.Duration(r.RetryAfter) * time.Second}, e
}

func (a Adapter) Status(ctx context.Context, t catalog.Tenant) (json.RawMessage, error) {
	if a.Service == nil {
		return nil, channel.ErrRejected
	}
	return a.Service.Status(ctx, t)
}
func (a Adapter) Disable(ctx context.Context, t catalog.Tenant) error {
	if a.Service == nil {
		return channel.ErrRejected
	}
	_, e := a.Service.Disconnect(ctx, t)
	return e
}
func (a Adapter) CanSend(p channel.SendPolicy, now time.Time) error {
	if a.Service == nil || !p.Active || p.Kill || p.Mode != "mock" {
		return channel.ErrRejected
	}
	return nil
}
