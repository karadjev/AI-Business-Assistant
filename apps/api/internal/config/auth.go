package config

import (
	"encoding/hex"
	"errors"
	"net/url"
	"time"
)

type Auth struct {
	Origin                               string
	EmailKey                             []byte
	CatalogRetentionKey                  []byte
	SMTPAddr                             string
	SMTP                                 SMTP
	Secure                               bool
	EmailActionsDisabled                 bool
	SessionTTL, IdleTTL, VerificationTTL time.Duration
}

func LoadAuth(get func(string) string, env string) (Auth, error) {
	c := Auth{Origin: value(get, "PUBLIC_ORIGIN", "http://127.0.0.1:3000"), SMTPAddr: value(get, "SMTP_ADDR", "127.0.0.1:1025"), Secure: requiresTLS(env), SessionTTL: 7 * 24 * time.Hour, IdleTTL: 24 * time.Hour, VerificationTTL: 24 * time.Hour}
	switch get("ABA_DISABLE_EMAIL_ACTIONS") {
	case "", "false":
	case "true":
		c.EmailActionsDisabled = true
	default:
		return Auth{}, errors.New("ABA_DISABLE_EMAIL_ACTIONS must be true or false")
	}
	u, e := url.Parse(c.Origin)
	if e != nil || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || (c.Secure && u.Scheme != "https") {
		return Auth{}, errors.New("PUBLIC_ORIGIN is invalid")
	}
	c.EmailKey, e = hex.DecodeString(get("AUTH_EMAIL_KEY"))
	if e != nil || len(c.EmailKey) != 32 {
		return Auth{}, errors.New("AUTH_EMAIL_KEY must be 32 bytes encoded as hex")
	}
	if raw := get("CATALOG_RETENTION_KEY"); raw != "" {
		c.CatalogRetentionKey, e = hex.DecodeString(raw)
		if e != nil || len(c.CatalogRetentionKey) != 32 {
			return Auth{}, errors.New("CATALOG_RETENTION_KEY must be 32 bytes encoded as hex")
		}
	}
	c.SMTP, e = loadSMTP(get, c.SMTPAddr, c.Secure)
	if e != nil {
		return Auth{}, e
	}
	for _, entry := range []struct {
		key    string
		target *time.Duration
	}{{"SESSION_TTL", &c.SessionTTL}, {"SESSION_IDLE_TTL", &c.IdleTTL}, {"VERIFICATION_TTL", &c.VerificationTTL}} {
		if raw := get(entry.key); raw != "" {
			v, err := time.ParseDuration(raw)
			if err != nil || v < time.Second || v > 7*24*time.Hour {
				return Auth{}, errors.New(entry.key + " must be between 1s and 168h")
			}
			*entry.target = v
		}
	}
	if c.IdleTTL > 24*time.Hour || c.IdleTTL > c.SessionTTL {
		return Auth{}, errors.New("SESSION_IDLE_TTL must not exceed 24h or SESSION_TTL")
	}
	return c, nil
}

// RetentionKey preserves legacy derivation until a dedicated key is supplied.
func (c Auth) RetentionKey() []byte {
	if len(c.CatalogRetentionKey) > 0 {
		return c.CatalogRetentionKey
	}
	return c.EmailKey
}
