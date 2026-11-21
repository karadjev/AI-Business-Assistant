package httpapi

import (
	"bytes"
	"encoding/json"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEmailHTTPBodyAdmission(t *testing.T) {
	const origin = "http://127.0.0.1:3000"
	const marker = "synthetic-email-body-must-not-be-logged"
	for _, endpoint := range []string{"register", "resend-verification", "login"} {
		for _, unknownLength := range []bool{false, true} {
			name := endpoint + "/known_length"
			if unknownLength {
				name = endpoint + "/streamed_length"
			}
			t.Run(name, func(t *testing.T) {
				var logs bytes.Buffer
				service := &auth.Service{Config: config.Auth{Origin: origin}}
				h := NewHandler(&fakeDB{}, logger(&logs), time.Second, 1024, service)
				body := `{"email":"` + marker + strings.Repeat("a", 2048) + `@example.test"}`
				r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/"+endpoint, strings.NewReader(body))
				r.Header.Set("Content-Type", "application/json")
				r.Header.Set("Origin", origin)
				if unknownLength {
					r.ContentLength = -1
				}
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				var response ErrorResponse
				if w.Code != http.StatusRequestEntityTooLarge || json.Unmarshal(w.Body.Bytes(), &response) != nil || response.Error.Code != "BODY_TOO_LARGE" {
					t.Fatal("oversized auth request was not rejected predictably")
				}
				if strings.Contains(logs.String(), marker) || strings.Contains(w.Body.String(), marker) {
					t.Fatal("email input leaked into logs or response")
				}
			})
		}
	}
}
