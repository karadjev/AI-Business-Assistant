package whatsapp

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/channel"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
	"time"
)

var _ channel.Queue = Service{}
var _ channel.Adapter = Adapter{}

func (s Service) ProcessOne(ctx context.Context) (bool, error) {
	if len(s.Key) != 32 {
		return false, nil
	}
	if _, e := s.Pool.Exec(ctx, `SELECT platform.channel_purge($1)`, s.Proof()); e != nil {
		return false, e
	}
	cap := auth.UUID()
	var raw []byte
	e := s.Pool.QueryRow(ctx, `SELECT platform.channel_claim($1,$2,$3,$4)`, s.Proof(), cap, auth.UUID(), auth.UUID()).Scan(&raw)
	if e != nil {
		return false, e
	}
	if len(raw) == 0 || string(raw) == "null" {
		return false, nil
	}
	var m channel.Message
	if json.Unmarshal(raw, &m) != nil {
		return true, channel.ErrRejected
	}
	if m.IntegrationID == "" {
		return true, nil
	} // receipt processed atomically in claim
	t := catalog.Tenant{OrganizationID: m.OrganizationID, ChannelCapability: cap, ChannelType: "whatsapp"}
	switch m.Kind {
	case channel.Text, channel.Unsupported:
		e = (channel.Engine{Chat: s.Chat}).Receive(ctx, t, m)
	case channel.Interactive:
		e = s.record(ctx, t, m, "customer", "Нажатие кнопки WhatsApp mock", map[string]any{"action": m.Action}, m.CorrelationID)
		if e == nil {
			var response channel.Response
			response, e = (channel.Engine{Chat: s.Chat}).Action(ctx, t, m.ConversationID, m.Action)
			if e == nil && response.Text != "" {
				data := map[string]any{"channel_payload": response, "mode": "mock"}
				if response.OrderID != "" {
					data["order_id"] = response.OrderID
				}
				e = s.record(ctx, t, m, "assistant", response.Text, data, replyID(m.CorrelationID))
			}
		}
	default:
		e = channel.ErrRejected
	}
	finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()
	_, fe := s.Pool.Exec(finish, `SELECT platform.channel_finish($1,$2)`, cap, e == nil)
	if e != nil {
		return true, e
	}
	return true, fe
}
func replyID(id string) string {
	b := auth.Hash("whatsapp/reply/" + id)
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmtUUID(b[:16])
}
func (s Service) record(ctx context.Context, t catalog.Tenant, m channel.Message, role, text string, data any, key string) error {
	tx, e := tenancy.Begin(ctx, s.Pool, t, true, true)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var count int
	var mode, status string
	var expiry time.Time
	e = tx.QueryRow(ctx, `SELECT message_count,control_mode,status,expires_at FROM platform.conversations WHERE id=$1 FOR UPDATE`, m.ConversationID).Scan(&count, &mode, &status, &expiry)
	if e != nil {
		return tenancy.Error(e)
	}
	if status != "open" || !expiry.After(time.Now()) {
		return channel.ErrRejected
	}
	if role == "assistant" && mode != "agent" {
		return nil
	}
	var exists bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM platform.messages WHERE conversation_id=$1 AND client_key=$2)`, m.ConversationID, key).Scan(&exists); e != nil {
		return e
	}
	if exists {
		return nil
	}
	if count >= 100 {
		return channel.ErrRejected
	}
	raw, _ := json.Marshal(data)
	_, e = tx.Exec(ctx, `INSERT INTO platform.messages(id,organization_id,conversation_id,role,content,data,client_key) VALUES($1,$2,$3,$4,$5,$6,$1)`, key, t.OrganizationID, m.ConversationID, role, text, raw)
	if e != nil {
		return tenancy.Error(e)
	}
	_, e = tx.Exec(ctx, `UPDATE platform.conversations SET message_count=message_count+1,version=version+1,updated_at=now() WHERE id=$1`, m.ConversationID)
	if e != nil {
		return tenancy.Error(e)
	}
	return tx.Commit(ctx)
}
func (s Service) SendOne(ctx context.Context) (bool, error) {
	if len(s.Key) != 32 {
		return false, nil
	}
	if e := s.RecoverFinancial(ctx); e != nil {
		return false, e
	}
	cap := auth.UUID()
	var raw []byte
	e := s.Pool.QueryRow(ctx, `SELECT platform.channel_claim_send($1,$2)`, s.Proof(), cap).Scan(&raw)
	if e != nil {
		return false, e
	}
	if len(raw) == 0 || string(raw) == "null" {
		return false, nil
	}
	var j struct {
		ID           string           `json:"id"`
		Organization string           `json:"organization_id"`
		Integration  string           `json:"integration_id"`
		Conversation string           `json:"conversation_id"`
		User         string           `json:"external_user_id"`
		Text         string           `json:"text"`
		Actions      []channel.Action `json:"actions"`
		Attempts     int              `json:"attempts"`
		TemplateID   string           `json:"template_id"`
	}
	if json.Unmarshal(raw, &j) != nil {
		return true, channel.ErrRejected
	}
	c, configurationError := s.configuration(ctx, j.Integration)
	// Pin a session lock across HTTP, without holding an SQL transaction. Disconnect/handoff
	// use the same lock. A second worker cannot reclaim an in-flight lease as pending.
	conn, e := s.Pool.Acquire(ctx)
	if e != nil {
		return true, e
	}
	locked := false
	var bookingKey *int64
	var consentLocked bool
	discard := false
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()
		if discard || !channel.UnlockBookingNotification(cleanup, conn, bookingKey) {
			_ = conn.Hijack().Close(cleanup)
			return
		}
		if consentLocked {
			var ok bool
			if conn.QueryRow(cleanup, `SELECT pg_advisory_unlock(hashtextextended($1,4101))`, j.Conversation).Scan(&ok) != nil || !ok {
				_ = conn.Hijack().Close(cleanup)
				return
			}
		}
		if locked {
			var ok bool
			if conn.QueryRow(cleanup, `SELECT pg_advisory_unlock(hashtextextended($1,8210))`, j.Integration).Scan(&ok) != nil || !ok {
				_ = conn.Hijack().Close(cleanup)
				return
			}
		}
		conn.Release()
	}()
	if _, e = conn.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended($1,8210))`, j.Integration); e != nil {
		return true, e
	}
	locked = true
	// A revocation and template dispatch serialize on the same conversation.
	if j.TemplateID != "" {
		if _, e = conn.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended($1,4101))`, j.Conversation); e != nil {
			discard = true
			return true, e
		}
		consentLocked = true
	}

	var admitted bool
	if e = conn.QueryRow(ctx, `SELECT platform.channel_admit_send($1)`, cap).Scan(&admitted); e != nil {
		return true, e
	}
	if admitted {
		admitted, bookingKey, e = channel.AdmitBookingNotification(ctx, conn, cap, channel.WhatsApp)
		if e != nil {
			discard = true
			return true, e
		}
	}
	result := channel.SendResult{State: channel.Cancelled}
	if admitted {
		if configurationError != nil {
			result.State = channel.Failed
		} else {
			send, cancel := context.WithTimeout(ctx, 5*time.Second)
			message := channel.Message{OrganizationID: j.Organization, IntegrationID: j.Integration, Channel: channel.WhatsApp, ConversationID: j.Conversation, ExternalUserID: j.User, ExternalChatID: j.User, Direction: "outbound", Kind: channel.Text, Text: j.Text, CorrelationID: j.ID, Actions: j.Actions}
			admission := &FinancialAdmission{Conn: conn, Proof: s.Proof(), Capability: cap, Simulation: true}
			transport := s.Transport
			if cloud, ok := transport.(*CloudTransport); ok {
				admission.Simulation = cloud.localOnly
				transport = cloud.withAdmission(admission)
			}
			if j.TemplateID != "" {
				var rawTemplate []byte
				e = conn.QueryRow(send, `SELECT platform.wa_dispatch_template($1,$2)`, s.Proof(), cap).Scan(&rawTemplate)
				var template Template
				if e == nil && len(rawTemplate) == 0 {
					e = channel.ErrRejected
				}
				if e == nil {
					e = json.Unmarshal(rawTemplate, &template)
				}
				if e == nil {
					if cloud, ok := transport.(*CloudTransport); ok {
						result, e = cloud.SendTemplate(send, c, message, template)
					} else {
						var reservation string
						reservation, e = admission.Reserve(send, c, message, &template)
						if e == nil {
							if sender, ok := transport.(TemplateSender); ok {
								result, e = sender.SendTemplate(send, c, message, template)
							} else {
								result.State = channel.Failed
								result.NoChargeConfirmed = true
								e = channel.ErrRejected
							}
							finish, cancelFinish := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
							if admission.Finish(finish, reservation, result) != nil {
								result.State = channel.Unknown
							}
							cancelFinish()
						} else {
							result.State = channel.Failed
						}
					}
				} else {
					result.State = channel.Failed
				}
			} else {
				result, e = (Adapter{Config: c, Transport: transport}).Send(send, message)
			}
			cancel()
			if e != nil && result.State == "" {
				result.State = channel.Unknown
			}
		}
	}
	delay := 0
	if result.State == channel.Pending {
		delay = min(3600, max(int(result.RetryAfter.Seconds()), 1<<min(j.Attempts, 8)))
	}
	finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()
	_, e = conn.Exec(finish, `SELECT platform.channel_finish_send($1,$2,$3,$4)`, cap, string(result.State), delay, result.ProviderID)
	return true, e
}

func fmtUUID(b []byte) string {
	h := hex.EncodeToString(b)
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}
