package telegram

import (
	"context"
	"encoding/json"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type spyTransport struct{ calls int }

func (s *spyTransport) GetMe(context.Context, string) (Bot, error) { s.calls++; return Bot{ID: 1}, nil }
func (s *spyTransport) Send(context.Context, string, int64, json.RawMessage) (SendResult, error) {
	s.calls++
	return SendResult{MessageID: 1}, nil
}
func TestExternalGatesAndAllowlist(t *testing.T) {
	c, _ := config.LoadTelegram(func(string) string { return "" }, "development")
	spy := &spyTransport{}
	g := Guarded{Config: c, Inner: spy}
	if _, e := g.GetMe(ConfirmedOperation(context.Background(), true), "synthetic"); e == nil {
		t.Fatal("unauthorized getMe")
	}
	if _, e := g.Send(context.Background(), "synthetic", 1, json.RawMessage(`{}`)); e == nil || spy.calls != 0 {
		t.Fatal("unauthorized send")
	}
	service := Service{Config: c, Transport: spy}
	if _, e := service.LiveOperation(context.Background(), catalog.Tenant{}, LiveOperation{Operation: "set_webhook", Confirm: true}); e == nil || spy.calls != 0 {
		t.Fatal("unauthorized operation")
	}
	c.Mode = "live"
	c.ExternalAuthorized = true
	c.AllowedUsers = map[int64]bool{42: true}
	g.Config = c
	if _, e := g.GetMe(context.Background(), "synthetic"); e == nil {
		t.Fatal("automatic getMe")
	}
	if _, e := g.Send(context.Background(), "synthetic", 42, json.RawMessage(`{}`)); e == nil {
		t.Fatal("sending off default")
	}
	g.Config.SendingEnabled = true
	g.Config.KillSending = true
	if _, e := g.Send(context.Background(), "synthetic", 42, json.RawMessage(`{}`)); e == nil {
		t.Fatal("kill switch")
	}
	g.Config.KillSending = false
	if _, e := g.Send(context.Background(), "synthetic", 43, json.RawMessage(`{}`)); e == nil {
		t.Fatal("foreign test account")
	}
	if _, e := g.Send(context.Background(), "synthetic", 42, json.RawMessage(`{}`)); e != nil || spy.calls != 1 {
		t.Fatal("authorized synthetic spy", e)
	}
}
func TestWebhookOperationsLocalFixture(t *testing.T) {
	seen := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		seen[method]++
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if method == "setWebhook" {
			if body["drop_pending_updates"] != false || len(body["allowed_updates"].([]any)) != 2 {
				t.Error("destructive webhook config")
			}
		}
		if method == "getWebhookInfo" {
			_, _ = w.Write([]byte(`{"ok":true,"result":{"url":"https://approved.example/api/v1/webhooks/telegram/fixture","pending_update_count":2,"last_error_date":0,"last_error_message":"sensitive ignored"}}`))
		} else {
			_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
		}
	}))
	defer server.Close()
	p := HTTPTransport{client: server.Client(), base: server.URL}
	token := "123:abcdefghijklmnopqrstuv"
	if e := p.SetWebhook(context.Background(), token, "https://approved.example/api/v1/webhooks/telegram/fixture", strings.Repeat("a", 43)); e != nil {
		t.Fatal(e)
	}
	info, e := p.GetWebhookInfo(context.Background(), token)
	if e != nil || info.Pending != 2 {
		t.Fatal(e)
	}
	if e = p.DeleteWebhook(context.Background(), token); e != nil {
		t.Fatal(e)
	}
	if e = p.AnswerCallback(context.Background(), token, "callback-fixture"); e != nil {
		t.Fatal(e)
	}
	for _, method := range []string{"setWebhook", "getWebhookInfo", "deleteWebhook", "answerCallbackQuery"} {
		if seen[method] != 1 {
			t.Fatal("missing protocol operation", method)
		}
	}
}
