package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"time"
)

// HTTPTransport is protocol-tested locally but is deliberately not selected by
// application configuration in this stage. The endpoint cannot come from users.
type HTTPTransport struct {
	client *http.Client
	base   string
}

func NewHTTPTransport() *HTTPTransport {
	return &HTTPTransport{client: &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect forbidden") }}, base: "https://api.telegram.org"}
}

var botToken = regexp.MustCompile(`^[0-9]{1,20}:[A-Za-z0-9_-]{20,128}$`)

func (t *HTTPTransport) call(ctx context.Context, token, method string, input any) (json.RawMessage, SendResult, error) {
	if !botToken.MatchString(token) {
		return nil, SendResult{}, errors.New("invalid bot credential")
	}
	b, e := json.Marshal(input)
	if e != nil {
		return nil, SendResult{}, errors.New("invalid request")
	}
	r, e := http.NewRequestWithContext(ctx, "POST", t.base+"/bot"+token+"/"+method, bytes.NewReader(b))
	if e != nil {
		return nil, SendResult{}, errors.New("request unavailable")
	}
	r.Header.Set("Content-Type", "application/json")
	response, e := t.client.Do(r)
	if e != nil {
		return nil, SendResult{Uncertain: method == "sendMessage"}, errors.New("Telegram unavailable")
	}
	defer response.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(response.Body, 65537))
	if e != nil || len(raw) > 65536 {
		return nil, SendResult{Uncertain: method == "sendMessage"}, errors.New("invalid Telegram response")
	}
	var out struct {
		OK         bool            `json:"ok"`
		Result     json.RawMessage `json:"result"`
		Code       int             `json:"error_code"`
		Parameters struct {
			RetryAfter int `json:"retry_after"`
		} `json:"parameters"`
	}
	if json.Unmarshal(raw, &out) != nil {
		return nil, SendResult{Uncertain: method == "sendMessage"}, errors.New("invalid Telegram response")
	}
	if !out.OK || response.StatusCode != 200 {
		result := SendResult{}
		if response.StatusCode >= 500 {
			// A server-side failure may follow a committed send; never replay it blindly.
			result.Uncertain = method == "sendMessage"
			if !result.Uncertain {
				result.RetryAfter = 2
			}
		} else if response.StatusCode == 429 && out.Code == 429 {
			result.RetryAfter = max(1, min(3600, out.Parameters.RetryAfter))
		}
		return nil, result, errors.New("Telegram rejected request")
	}
	return out.Result, SendResult{}, nil
}
func (t *HTTPTransport) GetMe(ctx context.Context, token string) (Bot, error) {
	raw, _, e := t.call(ctx, token, "getMe", map[string]any{})
	if e != nil {
		return Bot{}, e
	}
	var u struct {
		ID       int64  `json:"id"`
		IsBot    bool   `json:"is_bot"`
		Name     string `json:"first_name"`
		Username string `json:"username"`
	}
	if json.Unmarshal(raw, &u) != nil || u.ID < 1 || !u.IsBot || u.Username == "" {
		return Bot{}, errors.New("invalid bot identity")
	}
	return Bot{u.ID, u.Name, u.Username}, nil
}
func (t *HTTPTransport) Send(ctx context.Context, token string, chat int64, p json.RawMessage) (SendResult, error) {
	var body map[string]json.RawMessage
	if json.Unmarshal(p, &body) != nil || body == nil {
		return SendResult{}, errors.New("invalid outbox")
	}
	body["chat_id"], _ = json.Marshal(chat)
	delete(body, "parse_mode")
	body["link_preview_options"] = json.RawMessage(`{"is_disabled":true}`)
	raw, result, e := t.call(ctx, token, "sendMessage", body)
	if e != nil {
		return result, e
	}
	var m struct {
		ID int64 `json:"message_id"`
	}
	if json.Unmarshal(raw, &m) != nil || m.ID < 1 {
		return SendResult{Uncertain: true}, errors.New("missing Telegram receipt")
	}
	return SendResult{MessageID: m.ID}, nil
}
