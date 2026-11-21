package agent

import (
	"context"
	"fmt"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/costcontrol"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuditIncompleteUsageIsUnknown(t *testing.T) {
	for _, usage := range []string{`{}`, `{"prompt_tokens":10}`, `{"completion_tokens":1}`, `{"prompt_tokens":null,"completion_tokens":1}`, `{"prompt_tokens":0,"completion_tokens":0}`} {
		t.Run(usage, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintf(w, `{"choices":[{"message":{}}],"usage":%s}`, usage)
			}))
			defer server.Close()
			p := Compatible{Endpoint: server.URL, Client: server.Client()}
			response, e := p.Generate(context.Background(), Request{InputTokens: 1000, OutputTokens: 100})
			if e != nil {
				t.Fatal(e)
			}
			if response.UsageKnown {
				t.Fatal("missing/zero usage may not release budget as known zero")
			}
		})
	}
}

type auditNoNetworkTransport struct{ calls int }

func (t *auditNoNetworkTransport) RoundTrip(*http.Request) (*http.Response, error) {
	t.calls++
	return nil, fmt.Errorf("network disabled in test")
}
func TestAuditZeroBudgetStopsActualAdapterTransport(t *testing.T) {
	cfg, _ := config.LoadAgent(func(string) string { return "" })
	cfg.LiveEnabled = true
	cfg.Provider = "openai_compatible"
	cfg.Live = config.Live{ExternalAuthorized: true, BaseURL: "https://provider.invalid/v1", AllowedBases: []string{"https://provider.invalid/v1"}, PlatformMicroRUB: 0, InputRate: 1, OutputRate: 1}
	transport := &auditNoNetworkTransport{}
	adapter := Compatible{Endpoint: "https://provider.invalid/v1/chat/completions", Client: &http.Client{Transport: transport}, Key: "synthetic", Model: "synthetic", RequireBudget: true}
	metered := Metered{Provider: adapter, Ledger: costcontrol.Service{Config: cfg}}
	if _, e := metered.Generate(context.Background(), Request{InputTokens: 1000, OutputTokens: 100}); e == nil {
		t.Fatal("zero budget admitted")
	}
	if transport.calls != 0 {
		t.Fatal("actual adapter HTTP transport called at zero budget")
	}
}
