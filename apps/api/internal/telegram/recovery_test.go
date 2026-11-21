package telegram

import (
	"context"
	"encoding/json"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestAcceptedSendLosesReceiptWithoutAutomaticRetry(t *testing.T) {
	var accepted atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if e := json.NewDecoder(r.Body).Decode(&body); e != nil || body["chat_id"] != float64(42) {
			t.Error("actual wire payload", e)
		}
		accepted.Add(1)
		conn, _, e := w.(http.Hijacker).Hijack()
		if e != nil {
			t.Error(e)
			return
		}
		_ = conn.Close()
	}))
	defer server.Close()
	transport := HTTPTransport{client: server.Client(), base: server.URL}
	result, e := transport.Send(context.Background(), "123:"+strings.Repeat("a", 24), 42, json.RawMessage(`{"text":"synthetic"}`))
	if e == nil || !result.Uncertain || result.RetryAfter != 0 || accepted.Load() != 1 {
		t.Fatal("lost receipt replayed or misclassified", e, result, accepted.Load())
	}
}
func TestDeliveryResolutionRequiresExplicitValidAction(t *testing.T) {
	service := Service{}
	for _, in := range []DeliveryResolution{{}, {Decision: "resend", Confirm: true}, {Decision: "delivered", ExpectedVersion: 1, Confirm: false}} {
		if _, e := service.ResolveDelivery(context.Background(), catalog.Tenant{}, "invalid", in); e == nil {
			t.Fatal("invalid operator command accepted")
		}
	}
}

func TestHTTP5xxCannotBeReplayedFromContradictory429Body(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":429,"parameters":{"retry_after":1}}`))
	}))
	defer server.Close()
	transport := HTTPTransport{client: server.Client(), base: server.URL}
	result, e := transport.Send(context.Background(), "123:"+strings.Repeat("a", 24), 42, json.RawMessage(`{"text":"synthetic"}`))
	if e == nil || !result.Uncertain || result.RetryAfter != 0 {
		t.Fatal("5xx became unsafe retry through response body", result, e)
	}
}
