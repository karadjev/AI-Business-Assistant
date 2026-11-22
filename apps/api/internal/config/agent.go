package config

import (
	"errors"
	"strconv"
	"time"
)

type Agent struct {
	Live                                                            Live
	Provider, Model, Key                                            string
	LiveEnabled                                                     bool
	Timeout, StockFreshness                                         time.Duration
	MaxCalls, InputTokens, OutputTokens, DailyRequests, DailyTokens int
}

func LoadAgent(get func(string) string) (Agent, error) {
	production := get("APP_ENV") == "production"
	defaultProvider := "mock"
	if production {
		defaultProvider = "disabled"
	}
	c := Agent{Provider: value(get, "LLM_PROVIDER", defaultProvider), Model: get("LLM_MODEL"), Key: get("LLM_API_KEY")}
	var e error
	c.Live, e = LoadLive(get)
	if e != nil {
		return c, e
	}
	enabled, e := strconv.ParseBool(value(get, "LLM_ENABLE_LIVE", "false"))
	if e != nil {
		return c, errors.New("LLM_ENABLE_LIVE invalid")
	}
	c.LiveEnabled = enabled
	if c.Provider != "mock" && c.Provider != "openai_compatible" && c.Provider != "disabled" {
		return c, errors.New("LLM_PROVIDER invalid")
	}
	if production && c.Provider == "mock" {
		return c, errors.New("mock LLM is prohibited in production")
	}
	if c.Provider == "disabled" && (enabled || c.Key != "" || c.Live.ExternalAuthorized) {
		return c, errors.New("disabled LLM cannot load live credentials or authorization")
	}
	if c.Provider == "openai_compatible" && (!enabled || c.Model == "" || c.Key == "" || !c.Live.Ready()) {
		return c, errors.New("live LLM requires explicit opt-in, model and key")
	}
	if c.Provider == "mock" {
		c.Model = "deterministic-test"
	}
	for _, entry := range []struct {
		name, def string
		target    *int
		max       int
	}{{"LLM_MAX_TOOL_CALLS", "3", &c.MaxCalls, 3}, {"LLM_INPUT_TOKENS", "16000", &c.InputTokens, 32000}, {"LLM_OUTPUT_TOKENS", "512", &c.OutputTokens, 2048}, {"LLM_DAILY_REQUESTS", "40", &c.DailyRequests, 1000}, {"LLM_DAILY_TOKENS", "2000000", &c.DailyTokens, 10000000}} {
		n, e := strconv.Atoi(value(get, entry.name, entry.def))
		if e != nil || n < 1 || n > entry.max {
			return c, errors.New(entry.name + " invalid")
		}
		*entry.target = n
	}
	c.Timeout, e = time.ParseDuration(value(get, "LLM_TIMEOUT", "2s"))
	if e != nil || c.Timeout < time.Millisecond || c.Timeout > 3*time.Second {
		return c, errors.New("LLM_TIMEOUT must be 1ms..3s")
	}
	c.StockFreshness, e = time.ParseDuration(value(get, "INVENTORY_FRESHNESS", "24h"))
	if e != nil || c.StockFreshness < time.Hour || c.StockFreshness > 7*24*time.Hour {
		return c, errors.New("INVENTORY_FRESHNESS must be 1h..168h")
	}
	return c, nil
}
