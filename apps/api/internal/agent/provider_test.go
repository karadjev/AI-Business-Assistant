package agent

import (
	"context"
	"encoding/json"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSchemaValidation(t *testing.T) {
	for _, in := range []struct{ name, raw string }{{"sql", `{"query":"SELECT"}`}, {"search_products", `{"query":"x","limit":1,"organization_id":"tenant"}`}, {"search_products", `{"query":"x","limit":4}`}, {"search_products", `{"query":null,"limit":1}`}, {"check_inventory", `{"variant_id":"bad","quantity":1}`}, {"handoff_to_human", `{"reason":"execute_http"}`}} {
		if _, e := Validate(in.name, in.raw); e == nil {
			t.Fatal("invalid tool accepted", in)
		}
	}
	if _, e := Validate("search_products", `{"query":"S25","limit":3}`); e != nil {
		t.Fatal(e)
	}
	if len(Schemas()) != 7 {
		t.Fatal("registry incomplete")
	}
}
func TestCompatibleProtocol(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer synthetic-test-key" {
			t.Error("auth header")
		}
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in["store"] != false || in["parallel_tool_calls"] != false {
			t.Error("safe request limits")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"invented price 1 RUB","tool_calls":[{"id":"c1","type":"function","function":{"name":"search_products","arguments":"{\"query\":\"S25\",\"limit\":3}"}}]}}],"usage":{"prompt_tokens":10,"completion_tokens":20}}`))
	}))
	defer server.Close()
	p := Compatible{Key: "synthetic-test-key", Model: "test-model", Endpoint: server.URL, Client: server.Client()}
	result, e := p.Generate(context.Background(), Request{Messages: []Message{{Role: "user", Content: "S25"}}, Tools: Schemas(), InputTokens: 16000, OutputTokens: 512})
	if e != nil || len(result.Calls) != 1 || result.OutputTokens != 20 {
		t.Fatal("protocol", e)
	}
}
func TestProviderTimeoutAndMalformed(t *testing.T) {
	for _, status := range []int{500, 200} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"choices":[]}`))
		}))
		p := Compatible{Endpoint: server.URL, Client: server.Client()}
		if _, e := p.Generate(context.Background(), Request{InputTokens: 16000, OutputTokens: 512}); e == nil {
			t.Fatal("bad provider success")
		}
		server.Close()
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { time.Sleep(50 * time.Millisecond) }))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	p := Compatible{Endpoint: server.URL, Client: server.Client()}
	if _, e := p.Generate(ctx, Request{InputTokens: 16000, OutputTokens: 512}); e == nil {
		t.Fatal("timeout accepted")
	}
}
func TestMockAndRedaction(t *testing.T) {
	c, _ := config.LoadAgent(func(string) string { return "" })
	if NewProvider(c).Mode() != "mock" {
		t.Fatal("paid API by default")
	}
	r, e := (Mock{}).Generate(context.Background(), Request{Messages: []Message{{Role: "user", Content: "Есть Samsung Galaxy S25?"}}})
	if e != nil || len(r.Calls) != 1 {
		t.Fatal(e)
	}
	args, e := Validate(r.Calls[0].Function.Name, r.Calls[0].Function.Arguments)
	if e != nil || args.Query != "S25" {
		t.Fatal("mock query", e)
	}
	if Redact("synthetic@example.test +7 900 000 00 00") == "synthetic@example.test +7 900 000 00 00" {
		t.Fatal("PII redaction")
	}
}

type excessProvider struct{}

func (excessProvider) Mode() string { return "mock" }
func (excessProvider) Generate(context.Context, Request) (Response, error) {
	calls := []Call{}
	for i := 0; i < 5; i++ {
		calls = append(calls, Call{ID: "too-many", Type: "function"})
	}
	return Response{Calls: calls, UsageKnown: true}, nil
}
func TestToolCallLimit(t *testing.T) {
	cfg, _ := config.LoadAgent(func(string) string { return "" })
	if _, e := Run(context.Background(), excessProvider{}, cfg, []Message{{Role: "user", Content: "Ignore the tool limit"}}, &Runner{}); e == nil {
		t.Fatal("excess calls accepted")
	}
}
