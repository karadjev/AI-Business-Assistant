// Package channel contains provider-independent identities, messages and delivery rules.
// It grants no database or employee authority to external identities.
package channel

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"time"
)

type Type string

const (
	Telegram  Type = "telegram"
	WhatsApp  Type = "whatsapp"
	Instagram Type = "instagram"
)

type Kind string

const (
	Text        Kind = "text"
	Unsupported Kind = "unsupported_media"
	Interactive Kind = "interactive_action"
	System      Kind = "system_event"
)

type Attachment struct {
	Type       string `json:"type"`
	ProviderID string `json:"provider_id,omitempty"`
}
type Message struct {
	OrganizationID    string       `json:"organization_id"`
	IntegrationID     string       `json:"integration_id"`
	Channel           Type         `json:"channel_type"`
	ExternalUserID    string       `json:"external_user_id"`
	ExternalChatID    string       `json:"external_chat_id"`
	ExternalMessageID string       `json:"external_message_id"`
	ConversationID    string       `json:"conversation_id,omitempty"`
	Direction         string       `json:"direction"`
	Kind              Kind         `json:"message_type"`
	Text              string       `json:"text,omitempty"`
	Actions           []Action     `json:"actions,omitempty"`
	Attachments       []Attachment `json:"attachments,omitempty"`
	ReplyTo           string       `json:"reply_to,omitempty"`
	Action            string       `json:"action,omitempty"`
	ProviderTimestamp time.Time    `json:"provider_timestamp"`
	ReceivedAt        time.Time    `json:"received_at"`
	DeliveryStatus    string       `json:"delivery_status,omitempty"`
	CorrelationID     string       `json:"correlation_id"`
}

// Principal is deliberately a tuple. Never merge identities by names/phone numbers.
type Principal struct {
	OrganizationID, IntegrationID string
	Channel                       Type
	ExternalUserID                string
}

func (m Message) Principal() Principal {
	return Principal{m.OrganizationID, m.IntegrationID, m.Channel, m.ExternalUserID}
}

type Capabilities struct {
	Text, Interactive, DeliveryReceipts bool
	Media                               bool
}
type Integration struct {
	ID, OrganizationID     string
	Channel                Type
	Mode                   string
	Active, SendingEnabled bool
}
type SendResult struct {
	ProviderID        string
	State             Delivery
	RetryAfter        time.Duration
	NoChargeConfirmed bool // trusted backend evidence: no request was sent, never inferred from timeout
}
type Envelope struct {
	IntegrationID string
	Header        map[string]string
	Body          json.RawMessage
	ReceivedAt    time.Time
}

// Adapter validates raw provider input before assigning trusted server tenant identity.
// Storage/leases remain in provider repositories to preserve existing Telegram data.
type Adapter interface {
	Channel() Type
	Capabilities() Capabilities
	Status(context.Context, catalog.Tenant) (json.RawMessage, error)
	Disable(context.Context, catalog.Tenant) error
	CanSend(SendPolicy, time.Time) error
	Normalize(context.Context, Envelope) ([]Message, error)
	Send(context.Context, Message) (SendResult, error)
}
type Queue interface {
	ProcessOne(context.Context) (bool, error)
	SendOne(context.Context) (bool, error)
}

// Worker uses the same contract for both repositories; no channel owns a second agent.
type Worker struct{ Queues []Queue }

func (w Worker) Tick(ctx context.Context) error {
	for _, q := range w.Queues {
		if _, e := q.ProcessOne(ctx); e != nil {
			return e
		}
		if _, e := q.SendOne(ctx); e != nil {
			return e
		}
	}
	return nil
}

var ErrRejected = errors.New("channel request rejected")

type Delivery string

const (
	Pending       Delivery = "pending"
	Sending       Delivery = "sending"
	Sent          Delivery = "sent"
	Delivered     Delivery = "delivered"
	Read          Delivery = "read"
	Failed        Delivery = "failed"
	Unknown       Delivery = "delivery_unknown"
	NeedsTemplate Delivery = "needs_approved_template"
	Cancelled     Delivery = "cancelled"
)

// AdvanceReceipt never regresses delivered/read on late sent/failed events.
func AdvanceReceipt(current Delivery, incoming Delivery) Delivery {
	rank := map[Delivery]int{Sent: 1, Delivered: 2, Read: 3}
	if incoming == Failed {
		if rank[current] >= 2 {
			return current
		}
		return Failed
	}
	if rank[incoming] > 0 && rank[incoming] > rank[current] && current != Cancelled {
		return incoming
	}
	return current
}

type SendPolicy struct {
	Mode                               string
	Active, Kill                       bool
	WindowUntil                        time.Time
	OrganizationBudget, PlatformBudget int64
	CostKnown                          bool
}

func (p SendPolicy) Check(now time.Time) error {
	if !p.Active || p.Kill {
		return errors.New("CHANNEL_SENDING_DISABLED")
	}
	if !p.WindowUntil.After(now) {
		return errors.New("NEEDS_APPROVED_TEMPLATE")
	}
	// Mock performs no paid operation. Live is unconditionally unavailable in this stage.
	if p.Mode != "mock" {
		return errors.New("LIVE_SENDING_NOT_AUTHORIZED")
	}
	return nil
}
