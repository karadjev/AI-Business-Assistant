package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeDB struct {
	err   error
	calls int
}

func (d *fakeDB) Ready(context.Context) error { d.calls++; return d.err }
func logger(out *bytes.Buffer) *slog.Logger   { return slog.New(slog.NewJSONHandler(out, nil)) }
func TestPublicEndpoints(t *testing.T) {
	database := &fakeDB{}
	var logs bytes.Buffer
	handler := NewHandler(database, logger(&logs), time.Second, 1024)
	for _, test := range []struct{ path, expected string }{{"/health/live", "alive"}, {"/api/v1/status", "running"}} {
		t.Run(test.path, func(t *testing.T) {
			req := httptest.NewRequest("GET", test.path, nil)
			req.Header.Set("X-Request-ID", "untrusted-secret")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != 200 {
				t.Fatal(w.Code)
			}
			var data map[string]string
			if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
				t.Fatal(err)
			}
			if data["status"] != test.expected {
				t.Fatal(data)
			}
			if test.path == "/api/v1/status" && (data["service"] != "ai-business-assistant" || data["api_version"] != "v1") {
				t.Fatal(data)
			}
			if len(w.Header().Get("X-Request-ID")) != 32 {
				t.Fatal("missing generated request ID")
			}
			if w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("missing security headers")
			}
			if w.Header().Get("Access-Control-Allow-Origin") != "" {
				t.Fatal("unexpected CORS")
			}
		})
	}
	if database.calls != 0 {
		t.Fatal("live/status queried database")
	}
	if strings.Contains(logs.String(), "untrusted-secret") {
		t.Fatal("logged untrusted request ID")
	}
}
func TestReadiness(t *testing.T) {
	for _, unavailable := range []bool{false, true} {
		var logs bytes.Buffer
		d := &fakeDB{}
		expected := 200
		if unavailable {
			d.err = errors.New("postgresql://secret@private-host")
			expected = 503
		}
		w := httptest.NewRecorder()
		NewHandler(d, logger(&logs), time.Second, 1024).ServeHTTP(w, httptest.NewRequest("GET", "/health/ready", nil))
		if w.Code != expected {
			t.Fatal(w.Code)
		}
		if strings.Contains(w.Body.String()+logs.String(), "secret") {
			t.Fatal("database error leaked")
		}
		if d.calls != 1 {
			t.Fatal("readiness did not query db")
		}
	}
}
func TestErrors(t *testing.T) {
	for _, tc := range []struct {
		method, path string
		body         string
		status       int
		code         string
	}{{"GET", "/unknown", "", 404, "NOT_FOUND"}, {"POST", "/api/v1/status", "", 405, "METHOD_NOT_ALLOWED"}, {"GET", "/health/live", "123456", 413, "BODY_TOO_LARGE"}} {
		var logs bytes.Buffer
		w := httptest.NewRecorder()
		NewHandler(&fakeDB{}, logger(&logs), time.Second, 5).ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
		if w.Code != tc.status {
			t.Fatal(w.Code)
		}
		var e ErrorResponse
		if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil {
			t.Fatal(err)
		}
		if e.Error.Code != tc.code || e.Error.RequestID != w.Header().Get("X-Request-ID") {
			t.Fatal(e)
		}
	}
}
func TestRecovery(t *testing.T) {
	var logs bytes.Buffer
	handler := middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("private-password") }), logger(&logs), time.Second, 1024)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 500 {
		t.Fatal(w.Code)
	}
	var e ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil {
		t.Fatal(err)
	}
	if e.Error.Code != "INTERNAL_ERROR" || e.Error.RequestID == "" {
		t.Fatal(e)
	}
	if strings.Contains(w.Body.String()+logs.String(), "private-password") {
		t.Fatal("panic leaked")
	}
}
func TestTimeout(t *testing.T) {
	var logs bytes.Buffer
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }), logger(&logs), 10*time.Millisecond, 1024)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
	var e ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil {
		t.Fatal(err)
	}
	if e.Error.Code != "REQUEST_TIMEOUT" {
		t.Fatal(e)
	}
	if !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
		t.Fatal("timeout is not JSON")
	}
}
