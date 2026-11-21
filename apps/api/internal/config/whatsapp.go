package config

import (
	"encoding/hex"
	"errors"
)

// Cloud wire transport exists, but live admission/billing remains prohibited at zero budget.
type WhatsApp struct{ Key []byte }

func LoadWhatsApp(get func(string) string, environment string) (WhatsApp, error) {
	c := WhatsApp{}
	if v := get("WHATSAPP_SENDING_KILL_SWITCH"); v != "" && v != "true" {
		return c, errors.New("WhatsApp sending kill switch must remain enabled")
	}
	for _, key := range []string{"WHATSAPP_LIVE_ENABLED", "WHATSAPP_EXTERNAL_REQUESTS_AUTHORIZED"} {
		v := get(key)
		if v != "" && v != "false" {
			return c, errors.New("WhatsApp live access is not authorized")
		}
	}
	if v := get("WHATSAPP_MONETARY_BUDGET_RUB"); v != "" && v != "0" {
		return c, errors.New("WhatsApp budget must remain zero")
	}
	if v := get("WHATSAPP_ENCRYPTION_KEY"); v != "" {
		var e error
		c.Key, e = hex.DecodeString(v)
		if e != nil || len(c.Key) != 32 || environment == "production" {
			return WhatsApp{}, errors.New("invalid or prohibited WhatsApp configuration")
		}
	}
	return c, nil
}
