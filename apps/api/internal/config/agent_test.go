package config

import "testing"

func TestProductionRejectsMockProviders(t *testing.T) {
	values := map[string]string{"APP_ENV": "production", "LLM_PROVIDER": "mock", "TELEGRAM_TRANSPORT": "mock"}
	if _, err := LoadAgent(lookup(values)); err == nil {
		t.Fatal("production accepted a mock LLM")
	}
	if _, err := LoadTelegram(lookup(values), "production"); err == nil {
		t.Fatal("production accepted a mock Telegram transport")
	}
	delete(values, "LLM_PROVIDER")
	delete(values, "TELEGRAM_TRANSPORT")
	if agent, err := LoadAgent(lookup(values)); err != nil || agent.Provider != "disabled" {
		t.Fatal("production LLM default must be disabled")
	}
	if telegram, err := LoadTelegram(lookup(values), "production"); err != nil || telegram.Mode != "disabled" || telegram.Enabled {
		t.Fatal("production Telegram default must be disabled")
	}
}

func TestAgentConfiguration(t *testing.T) {
	c, e := LoadAgent(func(string) string { return "" })
	if e != nil || c.Provider != "mock" || c.LiveEnabled {
		t.Fatal("default must not spend", e)
	}
	for _, env := range []map[string]string{{"LLM_PROVIDER": "openai_compatible"}, {"LLM_MAX_TOOL_CALLS": "9"}, {"LLM_TIMEOUT": "10s"}, {"INVENTORY_FRESHNESS": "0h"}, {"LLM_ENABLE_LIVE": "invalid"}} {
		if _, e = LoadAgent(func(k string) string { return env[k] }); e == nil {
			t.Fatal("invalid config", env)
		}
	}
}
