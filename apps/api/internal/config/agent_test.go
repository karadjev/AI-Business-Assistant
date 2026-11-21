package config

import "testing"

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
