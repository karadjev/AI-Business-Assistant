package config

import (
	"encoding/hex"
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Telegram struct {
	Mode                                            string
	Key                                             []byte
	Retention                                       time.Duration
	Enabled                                         bool
	ExternalAuthorized, SendingEnabled, KillSending bool
	AllowedUsers                                    map[int64]bool
	WebhookBase                                     string
}

func LoadTelegram(get func(string) string, environment string) (Telegram, error) {
	c := Telegram{Mode: value(get, "TELEGRAM_TRANSPORT", "mock"), Retention: 7 * 24 * time.Hour}
	if c.Mode != "mock" && c.Mode != "live" {
		return c, errors.New("invalid Telegram transport")
	}
	var err error
	c.ExternalAuthorized, err = strconv.ParseBool(value(get, "TELEGRAM_EXTERNAL_REQUESTS_AUTHORIZED", "false"))
	if err != nil {
		return c, errors.New("invalid Telegram external gate")
	}
	c.SendingEnabled, err = strconv.ParseBool(value(get, "TELEGRAM_SENDING_ENABLED", "false"))
	if err != nil {
		return c, errors.New("invalid Telegram sending flag")
	}
	c.KillSending, err = strconv.ParseBool(value(get, "TELEGRAM_SENDING_KILL_SWITCH", "true"))
	if err != nil {
		return c, errors.New("invalid Telegram sending kill switch")
	}
	c.AllowedUsers = map[int64]bool{}
	if raw := get("TELEGRAM_TEST_USER_IDS"); raw != "" {
		for _, v := range strings.Split(raw, ",") {
			id, e := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
			if e != nil || id <= 0 {
				return c, errors.New("invalid test Telegram user ID")
			}
			c.AllowedUsers[id] = true
		}
	}
	c.WebhookBase = get("TELEGRAM_WEBHOOK_BASE_URL")
	if c.WebhookBase != "" {
		u, e := url.Parse(c.WebhookBase)
		if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Port() != "" && u.Port() != "443" || net.ParseIP(u.Hostname()) != nil || u.Hostname() == "localhost" || strings.HasSuffix(u.Hostname(), ".local") {
			return c, errors.New("approved webhook base must be public HTTPS origin")
		}
		if c.WebhookBase != get("TELEGRAM_APPROVED_WEBHOOK_BASE_URL") {
			return c, errors.New("webhook origin not approved")
		}
	}
	if c.Mode == "live" && (!c.ExternalAuthorized || len(c.AllowedUsers) == 0) {
		return c, errors.New("live Telegram requires explicit authorization and test user allowlist")
	}
	raw := get("TELEGRAM_ENCRYPTION_KEY")
	if raw != "" {
		var e error
		c.Key, e = hex.DecodeString(raw)
		if e != nil || len(c.Key) != 32 {
			return c, errors.New("TELEGRAM_ENCRYPTION_KEY must be 32 bytes hex")
		}
		c.Enabled = true
	}
	days, e := strconv.Atoi(value(get, "TELEGRAM_RETENTION_DAYS", "7"))
	if e != nil || days < 1 || days > 30 {
		return c, errors.New("TELEGRAM_RETENTION_DAYS must be 1..30")
	}
	c.Retention = time.Duration(days) * 24 * time.Hour
	if environment == "production" && c.Enabled {
		return c, errors.New("production Telegram processing requires approved privacy policy")
	}
	if c.Mode == "live" && !c.Enabled {
		return c, errors.New("live Telegram requires encryption key")
	}
	return c, nil
}
func (c Telegram) Allowed(id int64) bool {
	if id <= 0 {
		return false
	}
	if c.Mode == "mock" && len(c.AllowedUsers) == 0 {
		return true
	}
	return c.AllowedUsers[id]
}
func (c Telegram) AllowedIDs() []int64 {
	if c.Mode == "mock" && len(c.AllowedUsers) == 0 {
		return nil
	}
	ids := []int64{}
	for id := range c.AllowedUsers {
		ids = append(ids, id)
	}
	return ids
}
func (c Telegram) CanSendLive() bool {
	return c.Mode == "live" && c.ExternalAuthorized && c.SendingEnabled && !c.KillSending && len(c.AllowedUsers) > 0
}
