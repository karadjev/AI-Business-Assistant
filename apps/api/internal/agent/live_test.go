package agent

import (
	"context"
	"encoding/json"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/costcontrol"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

type countLive struct{ calls atomic.Int32 }

func (*countLive) Mode() string { return "live" }
func (p *countLive) Generate(context.Context, Request) (Response, error) {
	p.calls.Add(1)
	return Response{}, nil
}
func TestUnapprovedOrZeroBudgetNeverCallsProvider(t *testing.T) {
	for _, cfg := range []config.Agent{{}, {Provider: "openai_compatible", LiveEnabled: true, Key: "synthetic-key", Model: "synthetic-model", Live: config.Live{ExternalAuthorized: true, BaseURL: "https://provider.example/v1", AllowedBases: []string{"https://provider.example/v1"}, InputRate: 1, OutputRate: 1, PlatformMicroRUB: 0}}} {
		inner := &countLive{}
		metered := Metered{Provider: inner, Ledger: costcontrol.Service{Config: cfg}}
		if _, e := metered.Generate(context.Background(), Request{InputTokens: 16000, OutputTokens: 512}); e == nil || inner.calls.Load() != 0 {
			t.Fatal("external call reached without budget", e)
		}
		cfg.Provider = "openai_compatible"
		if _, e := NewProvider(cfg).Generate(context.Background(), Request{}); e == nil {
			t.Fatal("factory failed to deny invalid live configuration")
		}
	}
}
func TestCompatibleRetryAndArgumentDefense(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(429)
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"id":"fixture","type":"function","function":{"name":"search_products","arguments":"{\"query\":\"S25\",\"limit\":1}"}}]}}],"usage":{"prompt_tokens":10,"completion_tokens":2}}`))
	}))
	defer server.Close()
	p := Compatible{Endpoint: server.URL, Client: server.Client(), Retries: 1}
	r, e := p.Generate(context.Background(), Request{InputTokens: 16000, OutputTokens: 512})
	if e != nil || calls.Load() != 2 || r.UsageKnown {
		t.Fatal("retry must remain conservatively uncertain", e)
	}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"id":"bad","type":"function","function":{"name":"search_products","arguments":"{\"query\":\"S25\",\"limit\":1,\"organization_id\":\"foreign\"}"}}]}}]}`))
	}))
	defer bad.Close()
	p.Endpoint = bad.URL
	p.Client = bad.Client()
	if _, e = p.Generate(context.Background(), Request{InputTokens: 16000, OutputTokens: 512}); e == nil {
		t.Fatal("model-selected organization accepted")
	}
}
func TestAuthorizationErrorsAndMandatoryReservation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":"synthetic-private-key"}`))
	}))
	defer server.Close()
	p := Compatible{Endpoint: server.URL, Client: server.Client(), Key: "synthetic-private-key", RequireBudget: true}
	if _, e := p.Generate(context.Background(), Request{InputTokens: 16000, OutputTokens: 512}); e == nil {
		t.Fatal("missing reservation allowed")
	}
	p.RequireBudget = false
	if _, e := p.Generate(context.Background(), Request{InputTokens: 16000, OutputTokens: 512}); e == nil || strings.Contains(e.Error(), "synthetic-private-key") {
		t.Fatal("provider secret leaked")
	}
	var zero Response
	data, _ := json.Marshal(zero)
	if strings.Contains(string(data), "instructions") {
		t.Fatal("system prompt exposed")
	}
}
