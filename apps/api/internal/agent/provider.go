package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Call struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}
type Message struct {
	Role       string `json:"role"`
	Content    string `json:"content,omitempty"`
	Calls      []Call `json:"tool_calls,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
}
type Request struct {
	Messages                  []Message
	Tools                     []Tool
	InputTokens, OutputTokens int
}
type Response struct {
	Calls                     []Call
	InputTokens, OutputTokens int
	UsageKnown                bool
}
type Provider interface {
	Generate(context.Context, Request) (Response, error)
	Mode() string
}
type Mock struct{}

func (Mock) Mode() string { return "mock" }

var words = regexp.MustCompile(`[\pL\pN._-]+`)

func (Mock) Generate(ctx context.Context, r Request) (Response, error) {
	if e := ctx.Err(); e != nil {
		return Response{}, e
	}
	if len(r.Messages) > 0 && r.Messages[len(r.Messages)-1].Role == "tool" {
		return Response{UsageKnown: true, InputTokens: 1, OutputTokens: 1}, nil
	}
	message := ""
	for _, m := range r.Messages {
		if m.Role == "user" {
			message = m.Content
		}
	}
	lower := strings.ToLower(message)
	name := "search_products"
	args := map[string]any{"query": "", "limit": 3}
	if strings.Contains(lower, "сотрудник") || strings.Contains(lower, "человек") {
		name = "handoff_to_human"
		args = map[string]any{"reason": "customer_request"}
	} else {
		parts := words.FindAllString(message, -1)
		for _, p := range parts {
			if regexp.MustCompile(`[0-9]`).MatchString(p) {
				args["query"] = p
				break
			}
		}
		if args["query"] == "" {
			stop := map[string]bool{"здравствуйте": true, "есть": true, "у": true, "вас": true, "сколько": true, "стоит": true, "хочу": true, "купить": true, "товар": true, "да": true, "пожалуйста": true}
			kept := []string{}
			for _, p := range parts {
				if !stop[strings.ToLower(p)] {
					kept = append(kept, p)
				}
			}
			if len(kept) > 0 {
				args["query"] = kept[len(kept)-1]
			}
		}
	}
	raw, _ := json.Marshal(args)
	call := Call{ID: "mock-call", Type: "function"}
	call.Function.Name = name
	call.Function.Arguments = string(raw)
	return Response{Calls: []Call{call}, UsageKnown: true, InputTokens: 1, OutputTokens: 1}, nil
}

// Compatible implements the Chat Completions function-tool wire protocol.
// Endpoint/client are injected only by trusted construction; no tool can set URL.
type Compatible struct {
	Key, Model, Endpoint string
	Client               *http.Client
	RequireBudget        bool
	Retries              int
}

func (Compatible) Mode() string { return "live" }
func NewProvider(c config.Agent) Provider {
	if c.Provider == "mock" {
		return Mock{}
	}
	if !c.LiveEnabled || !c.Live.Ready() || c.Key == "" || c.Model == "" {
		return Denied{}
	}
	return Compatible{Key: c.Key, Model: c.Model, Endpoint: c.Live.BaseURL + "/chat/completions", Client: SafeClient(c.Timeout), RequireBudget: true, Retries: c.Live.Retries}
}
func (p Compatible) Generate(ctx context.Context, r Request) (Response, error) {
	if p.RequireBudget && ctx.Value(budgetPermitKey{}) != true {
		return Response{}, errors.New("live request requires reserved budget")
	}
	body := map[string]any{"model": p.Model, "messages": r.Messages, "tools": r.Tools, "tool_choice": "auto", "parallel_tool_calls": false, "max_completion_tokens": r.OutputTokens, "store": false}
	raw, e := json.Marshal(body)
	if e != nil {
		return Response{}, errors.New("provider request invalid")
	}
	if len(raw) > r.InputTokens {
		return Response{}, errors.New("input budget exceeded")
	}

	didRetry := false
	var response *http.Response
	for attempt := 0; ; attempt++ {
		request, e := http.NewRequestWithContext(ctx, http.MethodPost, p.Endpoint, bytes.NewReader(raw))
		if e != nil {
			return Response{}, errors.New("provider configuration invalid")
		}
		request.Header.Set("Authorization", "Bearer "+p.Key)
		request.Header.Set("Content-Type", "application/json")
		response, e = p.Client.Do(request)
		if e != nil {
			return Response{}, errors.New("provider unavailable")
		}
		if response.StatusCode == 200 {
			break
		}
		retry := response.StatusCode == 429 && attempt < p.Retries
		delay := time.Duration(attempt+1) * 100 * time.Millisecond
		if v, err := strconv.Atoi(response.Header.Get("Retry-After")); err == nil && v > 0 {
			if v > 2 {
				retry = false
			} else {
				delay = time.Duration(v) * time.Second
			}
		}
		_ = response.Body.Close()
		if !retry {
			return Response{}, errors.New("provider rejected request")
		}
		didRetry = true
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return Response{}, ctx.Err()
		case <-timer.C:
		}
	}
	defer response.Body.Close()
	data, e := io.ReadAll(io.LimitReader(response.Body, 65537))
	if e != nil || len(data) > 65536 {
		return Response{}, errors.New("provider response too large")
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Calls []Call `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Usage *struct {
			Input  *int `json:"prompt_tokens"`
			Output *int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if e = json.Unmarshal(data, &parsed); e != nil || len(parsed.Choices) != 1 {
		return Response{}, errors.New("provider response invalid")
	}
	result := Response{Calls: parsed.Choices[0].Message.Calls}
	if len(result.Calls) > 8 {
		return Response{}, errors.New("too many provider calls")
	}
	if parsed.Usage != nil && parsed.Usage.Input != nil && parsed.Usage.Output != nil {
		result.UsageKnown = !didRetry && *parsed.Usage.Input > 0
		result.InputTokens = *parsed.Usage.Input
		result.OutputTokens = *parsed.Usage.Output
		if result.InputTokens < 0 || result.OutputTokens < 0 || result.OutputTokens > r.OutputTokens || result.InputTokens > r.InputTokens {
			return Response{}, errors.New("provider usage outside limits")
		}
	}
	for _, call := range result.Calls {
		if call.Type != "function" || call.ID == "" {
			return Response{}, errors.New("provider tool call invalid")
		}
		if _, e := Validate(call.Function.Name, call.Function.Arguments); e != nil {
			return Response{}, errors.New("provider tool arguments invalid")
		}
	}
	return result, nil
}
