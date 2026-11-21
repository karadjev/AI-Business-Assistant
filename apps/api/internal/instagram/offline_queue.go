//go:build instagramoffline

package instagram

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/channel"
)

type OfflineQueue struct {
	Service
	outcome string
}

// NewOfflineQueue requires the isolated database and its explicitly installed fixture.
func NewOfflineQueue(ctx context.Context, s Service, environment, outcome string) (OfflineQueue, error) {
	var isolated bool
	if environment != "test" || !s.enabled() {
		return OfflineQueue{}, ErrRejected
	}
	switch outcome {
	case "accept", "throttle", "reject", "unavailable":
	default:
		return OfflineQueue{}, ErrRejected
	}
	err := s.Pool.QueryRow(ctx, `SELECT current_database()='aba_test' AND EXISTS(
		SELECT FROM platform.test_safety WHERE singleton AND marker='aba-synthetic-isolated'
	) AND to_regprocedure('platform.instagram_offline_claim(bytea,uuid)') IS NOT NULL`).Scan(&isolated)
	if err != nil || !isolated {
		return OfflineQueue{}, ErrRejected
	}
	return OfflineQueue{Service: s, outcome: outcome}, nil
}

func (s OfflineQueue) SendOne(ctx context.Context) (bool, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))

	capability := auth.UUID()
	var raw []byte
	if err := tx.QueryRow(ctx, "SELECT platform.instagram_offline_claim($1,$2)", s.Proof(), capability).Scan(&raw); err != nil {
		return false, err
	}
	if len(raw) == 0 {
		return false, tx.Commit(ctx)
	}
	var job struct {
		ID, Organization, Integration, Conversation, Account, Recipient, Cipher, Text string
		GraphVersion                                                                  string `json:"graph_version"`
		Expires                                                                       time.Time
		LastInbound                                                                   time.Time `json:"last_inbound"`
		Attempt                                                                       int
	}
	if err := json.Unmarshal(raw, &job); err != nil {
		return false, err
	}
	binding := Binding{OrganizationID: job.Organization, IntegrationID: job.Integration, AccountID: job.Account}
	result := channel.SendResult{State: channel.Failed}
	if sealed, err := hex.DecodeString(job.Cipher); err == nil {
		if token, err := (CredentialBox{Key: s.Config.Key}).Open(binding, sealed); err == nil {
			now := time.Now()
			result, _ = (OfflineTransport{RoundTrip: s.fixture(job.ID, job.Recipient)}).Send(ctx, SendConfig{
				Binding: binding, GraphVersion: job.GraphVersion, AccessToken: string(token),
				// These fields attest to the SQL admission held by this transaction.
				Policy: Policy{Active: true, SendingEnabled: true, PermissionsVerified: true, WebhookVerified: true,
					TokenExpiresAt: job.Expires, LastInbound: job.LastInbound},
			}, channel.Message{
				OrganizationID: job.Organization, IntegrationID: job.Integration, Channel: channel.Instagram,
				ConversationID: job.Conversation, ExternalUserID: job.Recipient, ExternalChatID: job.Recipient,
				Direction: "outbound", Kind: channel.Text, Text: job.Text, CorrelationID: job.ID,
			}, now)
		}
	}
	delay := 0
	if result.State == channel.Pending {
		wait, retry := RetryDelay(job.Attempt, result.RetryAfter)
		if retry {
			delay = int(wait.Seconds())
		} else {
			result.State = channel.Failed
		}
	}
	var finished bool
	if err := tx.QueryRow(ctx, "SELECT platform.instagram_offline_finish($1,$2,$3,$4,$5)",
		s.Proof(), capability, string(result.State), delay, result.ProviderID).Scan(&finished); err != nil {
		return false, err
	}
	if !finished {
		return false, ErrRejected
	}
	return true, tx.Commit(ctx)
}

// fixture accepts only an in-memory wire request. It has no HTTP client or socket.
func (s OfflineQueue) fixture(id, recipient string) func(*http.Request) (*http.Response, error) {
	return func(req *http.Request) (*http.Response, error) {
		var wire struct {
			Recipient struct{ ID string }
			Message   struct{ Text string }
		}
		if err := json.NewDecoder(req.Body).Decode(&wire); err != nil || wire.Recipient.ID != recipient || wire.Message.Text == "" {
			return nil, ErrRejected
		}
		res := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header)}
		body := ""
		switch s.outcome {
		case "accept":
			raw, err := json.Marshal(map[string]string{"recipient_id": wire.Recipient.ID, "message_id": "mock-" + id})
			if err != nil {
				return nil, err
			}
			body = string(raw)
		case "throttle":
			res.StatusCode = http.StatusTooManyRequests
			res.Header.Set("Retry-After", "60")
			body = `{"error":{"code":4}}`
		case "reject":
			res.StatusCode = http.StatusForbidden
			body = `{"error":{"code":190}}`
		default:
			return nil, errors.New("synthetic provider unavailable")
		}
		res.Body = io.NopCloser(strings.NewReader(body))
		return res, nil
	}
}
