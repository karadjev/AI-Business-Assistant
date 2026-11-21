package httpapi

import (
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/telegram"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAuditWebhookRejectsMalformedBeforeDatabase(t *testing.T) {
	mux := http.NewServeMux()
	telegramWebhook(mux, &telegram.Service{}) // No database: malformed ingress must not reach it.
	handler := middleware(mux, slog.New(slog.NewJSONHandler(io.Discard, nil)), time.Second, 1<<20)
	for _, tc := range []struct {
		body, secret string
		want         int
	}{{`{"update_id":1}`, "", 403}, {`{`, strings.Repeat("x", 43), 400}, {strings.Repeat(" ", 16385), strings.Repeat("x", 43), 413}} {
		req := httptest.NewRequest("POST", "/api/v1/webhooks/telegram/00000000-0000-4000-8000-000000000001", strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Telegram-Bot-Api-Secret-Token", tc.secret)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != tc.want {
			t.Errorf("expected %d before DB, got %d", tc.want, res.Code)
		}
	}
}
