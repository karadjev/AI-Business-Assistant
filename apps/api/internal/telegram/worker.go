package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/channel"
	"strconv"
	"time"
)

type Job struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	IntegrationID  string `json:"integration_id"`
	ConversationID string `json:"conversation_id"`
	Capability     string `json:"capability"`
	ChatID         int64  `json:"chat_id"`
	Payload        Update `json:"payload"`
}

func (s Service) ProcessOne(ctx context.Context) (bool, error) {
	if !s.Config.Enabled {
		return false, nil
	}
	if _, e := s.Pool.Exec(ctx, `SELECT platform.telegram_purge_expired($1)`, s.WorkerProof()); e != nil {
		return false, e
	}
	var raw []byte
	cap := auth.UUID()
	e := s.Pool.QueryRow(ctx, `SELECT platform.telegram_claim($1,$2,$3,$4,$5,$6,$7,$8)`, cap, auth.UUID(), auth.UUID(), auth.UUID(), int(s.Config.Retention.Hours()/24), s.WorkerProof(), s.Config.AllowedIDs(), s.Config.Mode).Scan(&raw)
	if e != nil {
		return false, e
	}
	if len(raw) == 0 || string(raw) == "null" {
		return false, nil
	}
	var job Job
	if e = json.Unmarshal(raw, &job); e != nil {
		return true, e
	}
	if s.Log != nil {
		s.Log.Info("telegram_inbox_started", "correlation_id", job.ID, "mode", s.Config.Mode)
	}
	e = s.process(ctx, job)
	if s.Log != nil {
		s.Log.Info("telegram_inbox_finished", "correlation_id", job.ID, "mode", s.Config.Mode, "success", e == nil)
	}
	var failure any
	if e != nil {
		failure = "PROCESSING_UNAVAILABLE"
	}
	finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()
	_, fe := s.Pool.Exec(finish, `SELECT platform.telegram_finish($1,$2)`, cap, failure)
	if e != nil {
		return true, e
	}
	return true, fe
}
func (s Service) process(ctx context.Context, j Job) error {
	t := catalog.Tenant{OrganizationID: j.OrganizationID, ChannelCapability: j.Capability}
	if j.Payload.Message != nil {
		return (channel.Engine{Chat: s.Chat}).Receive(ctx, t, channel.Message{OrganizationID: j.OrganizationID, IntegrationID: j.IntegrationID, Channel: channel.Telegram, ConversationID: j.ConversationID, Direction: "inbound", Kind: channel.Text, Text: j.Payload.Message.Text, CorrelationID: j.ID, ExternalUserID: strconv.FormatInt(j.ChatID, 10), ExternalChatID: strconv.FormatInt(j.ChatID, 10), ExternalMessageID: strconv.FormatInt(j.Payload.ID, 10)})
	}
	if j.Payload.Callback == nil {
		return fmt.Errorf("unsupported update")
	}
	if e := s.record(ctx, t, j.ConversationID, j.ID, "customer", "Нажатие кнопки тестового Telegram", map[string]any{"callback": j.Payload.Callback.Data}); e != nil {
		return e
	}
	response, e := (channel.Engine{Chat: s.Chat}).Action(ctx, t, j.ConversationID, j.Payload.Callback.Data)
	if e != nil {
		return e
	}
	if response.Text == "" {
		return nil
	}
	payload := map[string]any{"text": response.Text}
	if response.OrderID != "" {
		payload["order_id"] = response.OrderID
	}
	if len(response.Actions) > 0 {
		buttons := [][]map[string]string{}
		for _, a := range response.Actions {
			buttons = append(buttons, []map[string]string{{"text": a.Title, "callback_data": a.ID}})
		}
		payload["reply_markup"] = map[string]any{"inline_keyboard": buttons}
	}
	return s.queue(ctx, t, j.ConversationID, j.IntegrationID, j.ChatID, j.ID, payload)
}

func (s Service) SendOne(ctx context.Context) (bool, error) {
	if !s.Config.Enabled {
		return false, nil
	}
	if s.Config.Mode == "live" && !s.Config.CanSendLive() {
		return false, nil
	}
	cap := auth.UUID()
	var raw []byte
	e := s.Pool.QueryRow(ctx, `SELECT platform.telegram_claim_send($1,$2,$3,$4)`, cap, s.WorkerProof(), s.Config.AllowedIDs(), s.Config.Mode).Scan(&raw)
	if e != nil {
		return false, e
	}
	if len(raw) == 0 || string(raw) == "null" {
		return false, nil
	}
	var j struct {
		ID             string          `json:"id"`
		Attempts       int             `json:"attempts"`
		ChatID         int64           `json:"chat_id"`
		OrganizationID string          `json:"organization_id"`
		Payload        json.RawMessage `json:"payload"`
		Cipher         string          `json:"token_cipher"`
	}
	if e = json.Unmarshal(raw, &j); e != nil {
		return true, e
	}
	if !s.Config.Allowed(j.ChatID) {
		return true, fmt.Errorf("test sender not allowed")
	}
	cipher, e := decodeCipher(j.Cipher)
	token := ""
	if e == nil {
		token, e = decrypt(s.Config.Key, cipher, j.OrganizationID)
	}
	result := SendResult{}
	var guard *sendGuard
	if e == nil {
		guard, e = s.prepareSend(ctx, cap)
		if e != nil {
			return true, e
		}
		if guard == nil {
			return true, nil
		}
		defer guard.close(ctx)
	}
	if e == nil {
		send, cancel := context.WithTimeout(ctx, 5*time.Second)
		out, se := (Adapter{Service: &s, OrganizationID: j.OrganizationID, token: token, guard: guard, payload: j.Payload}).Send(send, channel.Message{OrganizationID: j.OrganizationID, Direction: "outbound", Channel: channel.Telegram, ExternalUserID: strconv.FormatInt(j.ChatID, 10), ExternalChatID: strconv.FormatInt(j.ChatID, 10), CorrelationID: j.ID})
		e = se
		messageID, _ := strconv.ParseInt(out.ProviderID, 10, 64)
		result = SendResult{MessageID: messageID, RetryAfter: int(out.RetryAfter.Seconds()), Uncertain: out.State == channel.Unknown}
		cancel()
	}
	state := "sent"
	delay := 0
	if e != nil {
		state = "failed"
		if result.Uncertain {
			state = "uncertain"
		} else if result.RetryAfter > 0 {
			state = "pending"
			delay = min(3600, max(result.RetryAfter, 1<<min(j.Attempts, 8)))
		}
	}
	finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()
	var fe error
	if guard != nil {
		_, fe = guard.conn.Exec(finish, `SELECT platform.telegram_finish_send($1,$2,$3,$4)`, cap, state, delay, result.MessageID)
	} else {
		_, fe = s.Pool.Exec(finish, `SELECT platform.telegram_finish_send($1,$2,$3,$4)`, cap, state, delay, result.MessageID)
	}
	if s.Log != nil {
		s.Log.Info("telegram_outbox_finished", "correlation_id", j.ID, "mode", s.Config.Mode, "delivery_status", state)
	}
	return true, fe
}
