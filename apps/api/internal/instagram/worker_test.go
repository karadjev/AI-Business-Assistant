package instagram

import (
	"context"
	"testing"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
)

func TestWorkerDisabledAndNoOutbound(t *testing.T) {
	s := Service{}
	if worked, err := s.ProcessOne(context.Background()); err != nil || worked {
		t.Fatal("disabled worker touched repository")
	}
	if sent, err := s.SendOne(context.Background()); err != nil || sent {
		t.Fatal("outbound must remain disabled")
	}
}
func TestRetryClassification(t *testing.T) {
	for _, err := range []error{context.Canceled, context.DeadlineExceeded, auth.Unavailable(), auth.Failure(429, "RATE_LIMITED", "synthetic"), auth.Failure(409, "CONVERSATION_BUSY", "synthetic")} {
		if !retryable(err) {
			t.Fatal("transient error dead-lettered")
		}
	}
	for _, err := range []error{nil, ErrRejected, auth.Failure(403, "FORBIDDEN", "synthetic"), auth.Failure(422, "VALIDATION_ERROR", "synthetic")} {
		if retryable(err) {
			t.Fatal("permanent error retried")
		}
	}
}
