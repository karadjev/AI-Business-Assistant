package config

import (
	"encoding/hex"
	"errors"
	"strings"
)

type Instagram struct {
	Enabled                bool
	Key                    []byte `json:"-"`
	AppSecret, VerifyToken string `json:"-"`
}

// LoadInstagram permits only synthetic, non-production runtime. No live client
// is selected by configuration, and the send kill switch remains mandatory.
func LoadInstagram(get func(string) string, environment string) (Instagram, error) {
	var c Instagram
	for _, key := range []string{"INSTAGRAM_LIVE_ENABLED", "INSTAGRAM_EXTERNAL_REQUESTS_AUTHORIZED"} {
		if v := get(key); v != "" && v != "false" {
			return c, errors.New("Instagram live access is not authorized")
		}
	}
	if v := get("INSTAGRAM_SENDING_KILL_SWITCH"); v != "" && v != "true" {
		return c, errors.New("Instagram sending kill switch must remain enabled")
	}
	if v := get("INSTAGRAM_MONETARY_BUDGET_RUB"); v != "" && v != "0" {
		return c, errors.New("Instagram budget must remain zero")
	}
	if get("INSTAGRAM_ACCESS_TOKEN") != "" {
		return c, errors.New("Instagram runtime tokens must come from encrypted storage")
	}
	enabled := get("INSTAGRAM_ENABLED")
	if enabled != "" && enabled != "false" && enabled != "true" {
		return c, errors.New("invalid Instagram mode")
	}
	if enabled != "true" {
		for _, key := range []string{"INSTAGRAM_APP_SECRET", "INSTAGRAM_ENCRYPTION_KEY", "INSTAGRAM_VERIFY_TOKEN"} {
			if get(key) != "" {
				return c, errors.New("Instagram credentials require explicit mock runtime configuration")
			}
		}
		return c, nil
	}
	if requiresTLS(environment) {
		return c, errors.New("Instagram mock runtime is restricted to isolated development and testing")
	}
	key, err := hex.DecodeString(get("INSTAGRAM_ENCRYPTION_KEY"))
	secret, verify := get("INSTAGRAM_APP_SECRET"), get("INSTAGRAM_VERIFY_TOKEN")
	if err != nil || len(key) != 32 || !strings.HasPrefix(secret, "mock:") || !strings.HasPrefix(verify, "mock:") || len(secret) < 16 || len(secret) > 256 || len(verify) < 16 || len(verify) > 256 || strings.ContainsAny(secret+verify, "\r\n\x00") {
		return c, errors.New("invalid synthetic Instagram configuration")
	}
	return Instagram{Enabled: true, Key: key, AppSecret: secret, VerifyToken: verify}, nil
}

// ValidateInstagram is the common startup gate; credentials never enable live access.
func ValidateInstagram(get func(string) string) error {
	_, err := LoadInstagram(get, value(get, "APP_ENV", "development"))
	return err
}
