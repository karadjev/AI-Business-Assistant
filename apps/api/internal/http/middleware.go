package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type requestKey struct{}

func requestID(r *http.Request) string { id, _ := r.Context().Value(requestKey{}).(string); return id }

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (w *responseWriter) WriteHeader(s int) {
	if w.status == 0 {
		w.status = s
		// Official provider verification returns the raw text challenge. Keep the
		// existing JSON default (including TimeoutHandler's HTML-labelled error body).
		if w.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
		}
		w.ResponseWriter.WriteHeader(s)
	}
}
func (w *responseWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

func recoverHandler(next http.Handler, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tracked := &responseWriter{ResponseWriter: w}
		defer func() {
			if recover() != nil {
				log.Error("request_failed", "code", "INTERNAL_ERROR", "request_id", requestID(r))
				if tracked.status == 0 {
					writeError(tracked, r, 500, "INTERNAL_ERROR", "Internal server error")
				}
			}
		}()
		next.ServeHTTP(tracked, r)
	})
}
func middleware(next http.Handler, log *slog.Logger, timeout time.Duration, maxBody int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var bytes [16]byte
		if _, err := rand.Read(bytes[:]); err != nil {
			writeJSON(w, 500, ErrorResponse{Error: ErrorDetail{Code: "INTERNAL_ERROR", Message: "Internal server error"}})
			return
		}
		id := hex.EncodeToString(bytes[:])
		r = r.WithContext(context.WithValue(r.Context(), requestKey{}, id))
		w.Header().Set("X-Request-ID", id)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		tracked := &responseWriter{ResponseWriter: w}
		start := time.Now()
		defer func() {
			log.Info("http_request", "method", r.Method, "status", tracked.status, "request_id", id, "duration_ms", time.Since(start).Milliseconds())
		}()
		limit := maxBody
		parts := strings.Split(r.URL.Path, "/")
		if r.Method == "POST" && len(parts) == 6 && parts[1] == "api" && parts[2] == "v1" && parts[3] == "organizations" && organizations.ValidID(parts[4]) && parts[5] == "catalog-imports" {
			limit = 1 << 20
		}
		if r.ContentLength > limit {
			writeError(tracked, r, 413, "BODY_TOO_LARGE", "Request body is too large")
			return
		}
		r.Body = http.MaxBytesReader(tracked, r.Body, limit)
		body, _ := json.Marshal(ErrorResponse{Error: ErrorDetail{Code: "REQUEST_TIMEOUT", Message: "Request timed out", RequestID: id}})
		http.TimeoutHandler(recoverHandler(next, log), timeout, string(body)).ServeHTTP(tracked, r)
	})
}
