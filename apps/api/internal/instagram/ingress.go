package instagram

import (
	"context"
	"encoding/json"
	"time"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
)

// Ingress verifies raw bytes before any binding lookup, normalization or enqueue.
// Account and tenant are resolved from protected SQL mapping, never event fields.
func (s Service) Ingress(ctx context.Context, integration, signature string, raw []byte) (int, error) {
	if !s.enabled() || !organizations.ValidID(integration) || !VerifySignature(s.Config.AppSecret, signature, raw) {
		return 0, ErrRejected
	}
	var mapping []byte
	if err := s.Pool.QueryRow(ctx, `SELECT platform.instagram_webhook_binding($1,$2)`, integration, s.Proof()).Scan(&mapping); err != nil {
		return 0, tenancy.Error(err)
	}
	var b Binding
	if len(mapping) == 0 || json.Unmarshal(mapping, &b) != nil || b.IntegrationID != integration {
		return 0, ErrRejected
	}
	batch, err := Normalize(b, raw, time.Now().UTC())
	if err != nil {
		return 0, err
	}
	if len(batch.Messages) == 0 {
		return 0, nil
	}
	// Atomic acknowledgement boundary: all accepted siblings persist or none do.
	payload, err := json.Marshal(batch.Messages)
	if err != nil {
		return 0, ErrRejected
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return 0, tenancy.Error(err)
	}
	defer tx.Rollback(ctx)
	var inserted int
	if err = tx.QueryRow(ctx, `SELECT platform.channel_ingress($1,$2,$3)`, integration, s.Proof(), payload).Scan(&inserted); err != nil {
		return 0, tenancy.Error(err)
	}
	if _, err = tx.Exec(ctx, `SELECT platform.instagram_observe($1,$2)`, integration, s.Proof()); err != nil {
		return 0, tenancy.Error(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, tenancy.Error(err)
	}
	if s.Log != nil {
		s.Log.Info("instagram_ingress_committed", "integration_id", integration, "inserted", inserted, "ignored", batch.Ignored, "rejected", batch.Rejected)
	}
	return inserted, nil
}
