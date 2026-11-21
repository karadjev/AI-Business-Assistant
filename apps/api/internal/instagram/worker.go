package instagram

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/channel"
)

var _ channel.Queue = Service{}

func (s Service) ProcessOne(ctx context.Context) (bool, error) {
	if !s.enabled() {
		return false, nil
	}
	if s.Chat == nil {
		return false, ErrRejected
	}
	if _, err := s.Pool.Exec(ctx, `SELECT platform.channel_purge($1)`, s.Proof()); err != nil {
		return false, err
	}
	capability := auth.UUID()
	var raw []byte
	if err := s.Pool.QueryRow(ctx, `SELECT platform.channel_claim($1,$2,$3,$4)`, s.Proof(), capability, auth.UUID(), auth.UUID()).Scan(&raw); err != nil {
		return false, err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return false, nil
	}
	var m channel.Message
	err := json.Unmarshal(raw, &m)
	if err == nil && (m.Channel != channel.Instagram || m.OrganizationID == "" || m.IntegrationID == "" || m.ConversationID == "") {
		err = channel.ErrRejected
	}
	if err == nil {
		tenant := catalog.Tenant{OrganizationID: m.OrganizationID, ChannelCapability: capability, ChannelType: "instagram"}
		err = (channel.Engine{Chat: s.Chat}).Receive(ctx, tenant, m)
	}
	finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()
	_, finalErr := s.Pool.Exec(finish, `SELECT platform.channel_finish_classified($1,$2,$3)`, capability, err == nil, retryable(err))
	if s.Log != nil {
		s.Log.Info("instagram_job_finished", "correlation_id", m.CorrelationID, "organization_id", m.OrganizationID, "integration_id", m.IntegrationID, "success", err == nil, "finalized", finalErr == nil, "retryable", retryable(err))
	}
	if err != nil {
		return true, err
	}
	return true, finalErr
}

// Global send kill switch remains enabled. Outbox rows are auditable cancelled
// records; no API success or delivery is fabricated and no token is read here.
func (s Service) SendOne(context.Context) (bool, error) { return false, nil }
func retryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var e *auth.Error
	if errors.As(err, &e) {
		return e.Status >= 500 || e.Status == 429 || e.Code == "CONVERSATION_BUSY"
	}
	return false
}
