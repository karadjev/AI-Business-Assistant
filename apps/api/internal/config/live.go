package config

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
)

type Live struct {
	ExternalAuthorized, Kill                                     bool
	BaseURL                                                      string
	AllowedBases                                                 []string
	PlatformMicroRUB, InputRate, OutputRate                      int64
	Parallel, Retries, DialogInput, DialogOutput, DialogRequests int
}

func LoadLive(get func(string) string) (Live, error) {
	c := Live{BaseURL: get("LLM_BASE_URL"), DialogInput: 20000, DialogOutput: 5000, DialogRequests: 30, Parallel: 2}
	var e error
	c.ExternalAuthorized, e = strconv.ParseBool(value(get, "LLM_EXTERNAL_REQUESTS_AUTHORIZED", "false"))
	if e != nil {
		return c, errors.New("invalid LLM external authorization")
	}
	c.Kill, e = strconv.ParseBool(value(get, "LLM_KILL_SWITCH", "true"))
	if e != nil {
		return c, errors.New("invalid LLM kill switch")
	}
	if raw := get("LLM_ALLOWED_BASE_URLS"); raw != "" {
		c.AllowedBases = strings.Split(raw, ",")
	}
	for _, p := range []struct {
		name string
		dst  *int64
	}{{"LLM_PLATFORM_BUDGET_MICRORUB", &c.PlatformMicroRUB}, {"LLM_INPUT_MICRORUB_PER_MILLION", &c.InputRate}, {"LLM_OUTPUT_MICRORUB_PER_MILLION", &c.OutputRate}} {
		*p.dst, e = strconv.ParseInt(value(get, p.name, "0"), 10, 64)
		if e != nil || *p.dst < 0 || *p.dst > 1000000000000 {
			return c, errors.New(p.name + " invalid")
		}
	}
	for _, p := range []struct {
		name, def string
		dst       *int
		max       int
	}{{"LLM_MAX_PARALLEL", "2", &c.Parallel, 16}, {"LLM_RETRIES", "0", &c.Retries, 2}, {"LLM_DIALOG_INPUT_TOKENS", "20000", &c.DialogInput, 20000}, {"LLM_DIALOG_OUTPUT_TOKENS", "5000", &c.DialogOutput, 5000}, {"LLM_DIALOG_REQUESTS", "30", &c.DialogRequests, 30}} {
		*p.dst, e = strconv.Atoi(value(get, p.name, p.def))
		if e != nil || *p.dst < 0 || *p.dst > p.max || p.name != "LLM_RETRIES" && *p.dst == 0 {
			return c, errors.New(p.name + " invalid")
		}
	}
	if c.BaseURL != "" {
		if e = ValidateBaseURL(c.BaseURL, c.AllowedBases); e != nil {
			return c, e
		}
	}
	return c, nil
}
func ValidateBaseURL(raw string, allow []string) error {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Host == "" || u.Port() != "" && u.Port() != "443" || u.Path != "/v1" || u.RawPath != "" {
		return errors.New("LLM base URL must be exact allowlisted HTTPS /v1")
	}
	host := strings.ToLower(u.Hostname())
	if net.ParseIP(host) != nil || host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || !strings.Contains(host, ".") {
		return errors.New("private LLM host forbidden")
	}
	for _, a := range allow {
		if raw == strings.TrimSpace(a) {
			return nil
		}
	}
	return errors.New("LLM base URL not approved")
}
func (c Live) Ready() bool {
	return c.ExternalAuthorized && !c.Kill && c.PlatformMicroRUB > 0 && c.InputRate > 0 && c.OutputRate > 0 && ValidateBaseURL(c.BaseURL, c.AllowedBases) == nil
}
