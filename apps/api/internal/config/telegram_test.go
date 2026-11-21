package config

import (
	"strings"
	"testing"
)

func TestTelegramConfig(t *testing.T) {
	get := func(k string) string {
		if k == "TELEGRAM_ENCRYPTION_KEY" {
			return strings.Repeat("ab", 32)
		}
		return ""
	}
	c, e := LoadTelegram(get, "development")
	if e != nil || !c.Enabled || c.Mode != "mock" {
		t.Fatal("valid config")
	}
	if _, e = LoadTelegram(get, "production"); e == nil {
		t.Fatal("production gate missing")
	}
	if _, e = LoadTelegram(func(k string) string {
		if k == "TELEGRAM_TRANSPORT" {
			return "live"
		}
		return ""
	}, "development"); e == nil {
		t.Fatal("live permitted")
	}
}
