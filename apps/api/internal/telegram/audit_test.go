package telegram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAuditSend5xxIsUncertain(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || !strings.HasSuffix(r.URL.Path, "/sendMessage") {
			t.Error("unexpected HTTP operation")
		}
		var body map[string]any
		if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
			t.Error(e)
		}
		if body["chat_id"] != float64(42) || body["parse_mode"] != nil {
			t.Error("unsafe message routing/HTML")
		}
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":500,"description":"sensitive fixture"}`))
	}))
	defer server.Close()
	transport := HTTPTransport{client: server.Client(), base: server.URL}
	result, e := transport.Send(context.Background(), "123:"+strings.Repeat("a", 24), 42, json.RawMessage(`{"text":"synthetic","chat_id":777,"parse_mode":"HTML"}`))
	if e == nil || !result.Uncertain || result.RetryAfter != 0 {
		t.Fatal("ambiguous 5xx must not cause automatic resend", result, e)
	}
	if strings.Contains(e.Error(), "sensitive fixture") {
		t.Fatal("provider description leaked")
	}
}
func TestAuditNullOutboxIsRejected(t *testing.T) {
	defer func() {
		if recover() != nil {
			t.Error("null outbox crashed the worker")
		}
	}()
	transport := HTTPTransport{}
	if _, e := transport.Send(context.Background(), "synthetic", 42, json.RawMessage(`null`)); e == nil {
		t.Fatal("null payload accepted")
	}
}

func TestAuditHTTPTimeoutIsUncertain(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(100 * time.Millisecond):
		}
	}))
	defer server.Close()
	transport := HTTPTransport{client: server.Client(), base: server.URL}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	result, e := transport.Send(ctx, "123:"+strings.Repeat("a", 24), 42, json.RawMessage(`{"text":"synthetic"}`))
	if e == nil || !result.Uncertain || result.RetryAfter != 0 {
		t.Fatal("timeout must retain ambiguous delivery", result, e)
	}
}
