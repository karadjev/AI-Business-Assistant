package config

import "testing"

func TestWhatsAppOfflineOnly(t *testing.T) {
	for _, key := range []string{"WHATSAPP_LIVE_ENABLED", "WHATSAPP_EXTERNAL_REQUESTS_AUTHORIZED", "WHATSAPP_MONETARY_BUDGET_RUB", "WHATSAPP_SENDING_KILL_SWITCH"} {
		if _, e := LoadWhatsApp(func(k string) string {
			if k == key {
				return "1"
			}
			return ""
		}, "development"); e == nil {
			t.Fatal("unsafe configuration accepted", key)
		}
	}
	if c, e := LoadWhatsApp(func(string) string { return "" }, "development"); e != nil || len(c.Key) != 0 {
		t.Fatal("safe default")
	}
}
