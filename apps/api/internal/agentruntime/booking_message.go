package agentruntime

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func bookingMessage(summary string, raw json.RawMessage) string {
	var receipt struct {
		Service     string    `json:"service"`
		Start       time.Time `json:"starts_at"`
		End         time.Time `json:"ends_at"`
		Zone        string    `json:"timezone"`
		Specialists []string  `json:"specialists"`
		Reservation string    `json:"reservation_status"`
		Total       *int64    `json:"total_amount"`
		Currency    string    `json:"currency"`
		Parts       []struct {
			Name     string `json:"name"`
			SKU      string `json:"sku"`
			Quantity int    `json:"quantity"`
			Price    *int64 `json:"unit_price"`
		} `json:"parts"`
	}
	if json.Unmarshal(raw, &receipt) != nil {
		return summary
	}
	lines := []string{summary}
	if receipt.Service != "" {
		lines = append(lines, "Услуга: "+receipt.Service)
	}
	if zone, err := time.LoadLocation(receipt.Zone); err == nil && !receipt.Start.IsZero() && !receipt.End.IsZero() {
		lines = append(lines, fmt.Sprintf("Время: %s — %s (%s).", receipt.Start.In(zone).Format("02.01.2006 15:04"), receipt.End.In(zone).Format("02.01.2006 15:04"), receipt.Zone))
	}
	if len(receipt.Specialists) > 0 {
		lines = append(lines, "Специалисты: "+strings.Join(receipt.Specialists, ", "))
	}
	if receipt.Total != nil && receipt.Currency != "" {
		lines = append(lines, fmt.Sprintf("Стоимость: %d.%02d %s.", *receipt.Total/100, *receipt.Total%100, receipt.Currency))
	}
	switch receipt.Reservation {
	case "active":
		lines = append(lines, "Запчасти зарезервированы.")
	case "released":
		lines = append(lines, "Резерв освобождён.")
	case "expired":
		lines = append(lines, "Срок резерва истёк.")
	case "consumed":
		lines = append(lines, "Резерв использован.")
	}
	if summary == "Запись подтверждена." {
		lines = append(lines, "Для изменения или отмены записи обратитесь к сотруднику в этом диалоге.")
	}
	if len(receipt.Parts) > 0 {
		lines = append(lines, "Запчасти:")
		for _, p := range receipt.Parts {
			line := fmt.Sprintf("%s (%s) × %d", p.Name, p.SKU, p.Quantity)
			if p.Price != nil && receipt.Currency != "" {
				line += fmt.Sprintf(" · %d.%02d %s/шт.", *p.Price/100, *p.Price%100, receipt.Currency)
			}
			lines = append(lines, line)
		}
	}
	text := []rune(strings.Join(lines, "\n"))
	if len(text) > 2000 {
		text = append(text[:1999], '…')
	}
	return string(text)
}
