package instagram

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/channel"
)

var graphVersion = regexp.MustCompile(`^v[0-9]{1,3}\.[0-9]{1,2}$`)

// OfflineTransport calls only the injected fixture function. There is no default
// HTTP client, live constructor or configuration path enabling network access.
// Production dispatch additionally requires durable principal/outbox admission.
type OfflineTransport struct {
	RoundTrip func(*http.Request) (*http.Response, error)
}
type SendConfig struct {
	Binding      Binding
	GraphVersion string
	AccessToken  string `json:"-"`
	Policy       Policy
}

func (t OfflineTransport) Send(ctx context.Context, c SendConfig, m channel.Message, now time.Time) (channel.SendResult, error) {
	failed := channel.SendResult{State: channel.Failed}
	if ctx.Err() != nil {
		return failed, ctx.Err()
	}
	if err := c.Policy.Check(now); err != nil {
		return failed, err
	}
	if t.RoundTrip == nil || !strings.HasPrefix(c.AccessToken, "mock:") || len(c.AccessToken) > 256 || strings.ContainsAny(c.AccessToken, "\r\n\x00") || !graphVersion.MatchString(c.GraphVersion) || !digits.MatchString(c.Binding.AccountID) || c.Binding.OrganizationID == "" || c.Binding.IntegrationID == "" || m.Channel != channel.Instagram || m.OrganizationID != c.Binding.OrganizationID || m.IntegrationID != c.Binding.IntegrationID || m.Direction != "outbound" || m.Kind != channel.Text || !digits.MatchString(m.ExternalUserID) || m.ExternalUserID != m.ExternalChatID || m.ExternalUserID == c.Binding.AccountID || strings.TrimSpace(m.Text) == "" || len(m.Text) > 1000 || !utf8.ValidString(m.Text) || strings.ContainsRune(m.Text, 0) || len(m.Actions) > 0 || len(m.Attachments) > 0 || m.ReplyTo != "" {
		return failed, ErrRejected
	}
	body, _ := json.Marshal(map[string]any{"recipient": map[string]string{"id": m.ExternalUserID}, "message": map[string]string{"text": m.Text}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://graph.instagram.com/"+c.GraphVersion+"/"+c.Binding.AccountID+"/messages", bytes.NewReader(body))
	if err != nil {
		return failed, ErrRejected
	}
	req.Header.Set("Authorization", "Bearer "+c.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	response, err := t.RoundTrip(req)
	unknown := channel.SendResult{State: channel.Unknown}
	if err != nil || response == nil {
		if response != nil && response.Body != nil {
			response.Body.Close()
		}
		return unknown, ErrRejected
	}
	if response.Body == nil {
		return unknown, ErrRejected
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, MaxPayload+1))
	if err != nil || len(raw) > MaxPayload {
		return unknown, ErrRejected
	}
	var result struct {
		ID        string `json:"message_id"`
		Recipient string `json:"recipient_id"`
		Error     *struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &result) != nil {
		return unknown, ErrRejected
	}
	if response.StatusCode == 200 && result.Error == nil && identifier(result.ID) && result.Recipient == m.ExternalUserID {
		return channel.SendResult{State: channel.Sent, ProviderID: result.ID}, nil
	}
	if response.StatusCode >= 500 || (response.StatusCode >= 200 && response.StatusCode < 400) || result.Error == nil {
		return unknown, ErrRejected
	}
	if response.StatusCode == 429 {
		seconds, err := strconv.Atoi(response.Header.Get("Retry-After"))
		if err != nil || seconds < 1 || seconds > 3600 {
			return failed, ErrRejected
		}
		return channel.SendResult{State: channel.Pending, RetryAfter: time.Duration(seconds) * time.Second}, nil
	}
	return failed, ErrRejected
}
