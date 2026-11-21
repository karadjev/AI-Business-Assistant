package telegram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPProtocolLocalOnly(t *testing.T) {
	var received string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "getMe") {
			_, _ = w.Write([]byte(`{"ok":true,"result":{"id":123,"is_bot":true,"first_name":"Fixture","username":"fixture_bot"}}`))
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		received = body["text"].(string)
		if body["chat_id"].(float64) != 42 {
			t.Error("chat binding")
		}
		if _, ok := body["parse_mode"]; ok {
			t.Error("unsafe parse mode")
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":55}}`))
	}))
	defer server.Close()
	transport := &HTTPTransport{client: server.Client(), base: server.URL}
	token := "123:abcdefghijklmnopqrstuv"
	bot, e := transport.GetMe(context.Background(), token)
	if e != nil || bot.ID != 123 {
		t.Fatal("getMe", e)
	}
	result, e := transport.Send(context.Background(), token, 42, json.RawMessage(`{"text":"<script>synthetic</script>","parse_mode":"HTML"}`))
	if e != nil || result.MessageID != 55 || received != "<script>synthetic</script>" {
		t.Fatal("send", e)
	}
}
func TestHTTP429AndErrorRedaction(t *testing.T) {
	secret := "123:abcdefghijklmnopqrstuv"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":429,"description":"123:abcdefghijklmnopqrstuv","parameters":{"retry_after":17}}`))
	}))
	defer server.Close()
	transport := &HTTPTransport{client: server.Client(), base: server.URL}
	res, e := transport.Send(context.Background(), secret, 42, json.RawMessage(`{"text":"synthetic"}`))
	if e == nil || res.RetryAfter != 17 || strings.Contains(e.Error(), secret) {
		t.Fatal("429/redaction")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res, e = transport.Send(ctx, secret, 42, json.RawMessage(`{"text":"synthetic"}`))
	if e == nil || !res.Uncertain || strings.Contains(e.Error(), secret) {
		t.Fatal("ambiguous network send not classified")
	}
}
