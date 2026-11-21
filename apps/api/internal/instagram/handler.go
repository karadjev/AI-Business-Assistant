package instagram

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/channel"
)

// Webhook is a handler contract, not a registered runtime endpoint. Binding must
// be resolved server-side. Enqueue must durably deduplicate integration+message
// ID and commit before returning nil; nil for a duplicate is success. Never run
// AI inside Enqueue. Do not mount until the shared repository implements this.
type Webhook struct {
	Binding                Binding
	AppSecret, VerifyToken string
	Now                    func() time.Time
	Enqueue                func(context.Context, channel.Message) error
}

func (h Webhook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodGet {
		challenge, err := Challenge(h.VerifyToken, r.URL.Query())
		if err != nil {
			http.Error(w, "verification rejected", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, challenge)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method rejected", http.StatusMethodNotAllowed)
		return
	}
	if h.Now == nil || h.Enqueue == nil || len(h.AppSecret) < 16 {
		http.Error(w, "integration unavailable", http.StatusServiceUnavailable)
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxPayload))
	if err != nil {
		http.Error(w, "payload rejected", http.StatusRequestEntityTooLarge)
		return
	}
	if len(r.Header.Values("X-Hub-Signature-256")) != 1 || !VerifySignature(h.AppSecret, r.Header.Get("X-Hub-Signature-256"), raw) {
		http.Error(w, "signature rejected", http.StatusForbidden)
		return
	}
	batch, err := Normalize(h.Binding, raw, h.Now())
	if err != nil {
		http.Error(w, "event rejected", http.StatusBadRequest)
		return
	}
	failed := false
	for _, m := range batch.Messages {
		if h.Enqueue(r.Context(), m) != nil {
			failed = true
		}
	}
	if failed {
		http.Error(w, "queue unavailable", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
}
