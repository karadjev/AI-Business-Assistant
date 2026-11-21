package channel

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/conversations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/orders"
	"strings"
)

// Engine reuses the existing catalog/agent/orders services. Adapters only encode transport UI.
type Engine struct{ Chat *conversations.Service }
type Action struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}
type Response struct {
	Text    string   `json:"text"`
	Actions []Action `json:"actions,omitempty"`
	OrderID string   `json:"order_id,omitempty"`
}

func (e Engine) Action(ctx context.Context, t catalog.Tenant, conv, action string) (Response, error) {
	detail, err := e.Chat.Get(ctx, t, conv)
	if err != nil {
		return Response{}, err
	}
	var state struct {
		Mode string `json:"control_mode"`
	}
	if json.Unmarshal(detail, &state) != nil {
		return Response{}, ErrRejected
	}
	if state.Mode != "agent" {
		return Response{}, nil
	}
	parts := strings.Split(action, ":")
	if len(parts) != 2 {
		return Response{}, ErrRejected
	}
	svc := orders.Service{Pool: e.Chat.Pool, StockFreshness: e.Chat.Config.StockFreshness}
	switch parts[0] {
	case "offer":
		q, err := svc.PrepareQuote(ctx, t, conv, []orders.CartItem{{VariantID: parts[1], Quantity: 1}})
		if err != nil {
			return Response{Text: "Цена или наличие изменились. Запросите товар снова; резерв не выполнен."}, nil
		}
		text := "[MOCK — тестовый сценарий] Предложение на 5 минут. Заявка не резервирует товар.\n"
		for _, i := range q.Items {
			text += fmt.Sprintf("%s · %s · %d шт. · %d.%02d RUB\n", i.Name, i.SKU, i.Quantity, i.UnitPrice/100, i.UnitPrice%100)
		}
		return Response{Text: text + "Подтвердить создание заявки на показанных условиях?", Actions: []Action{{ID: "confirm:" + q.ID, Title: "Подтверждаю заявку"}}}, nil
	case "confirm":
		created, err := svc.CreateRequest(ctx, t, orders.Consent{QuoteID: parts[1], ConversationID: conv, IdempotencyKey: parts[1], Accepted: true})
		if err != nil {
			return Response{Text: "Предложение недействительно либо цена/наличие изменились. Запросите новое предложение и подтвердите снова."}, nil
		}
		var o struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(created, &o) != nil {
			return Response{}, ErrRejected
		}
		return Response{Text: "[MOCK] Заявка создана. Ожидает сотрудника; товар ещё не зарезервирован.", OrderID: o.ID}, nil
	}
	return Response{}, ErrRejected
}

// Receive is the common channel-to-conversation boundary. Tenant is trusted from a
// leased inbox job, never from provider JSON. Staff sessions cannot impersonate it.
func (e Engine) Receive(ctx context.Context, t catalog.Tenant, m Message) error {
	if t.ChannelCapability == "" || m.OrganizationID != t.OrganizationID || m.IntegrationID == "" || m.ConversationID == "" || m.Direction != "inbound" || (m.Channel != Telegram && m.Channel != WhatsApp && m.Channel != Instagram) {
		return ErrRejected
	}
	text := m.Text
	if m.Kind == Unsupported {
		text = "[Неподдерживаемое вложение: требуется сотрудник]"
		if _, err := e.Chat.Control(ctx, t, m.ConversationID, "human", 0); err != nil {
			return err
		}
	} else if m.Kind != Text {
		return ErrRejected
	}
	var metadata json.RawMessage
	if m.Channel == Instagram {
		metadata, _ = json.Marshal(map[string]any{"channel": "instagram", "external_message_id": m.ExternalMessageID, "sender_id": m.ExternalUserID, "provider_timestamp": m.ProviderTimestamp, "attachments": m.Attachments, "reply_to": m.ReplyTo, "message_type": m.Kind})
	}
	_, err := e.Chat.Send(ctx, t, m.ConversationID, conversations.SendInput{Text: text, Role: "customer", IdempotencyKey: m.CorrelationID, ChannelMetadata: metadata})

	return err
}
