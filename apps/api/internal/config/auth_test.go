package config

import (
	"strings"
	"testing"
)

func TestAuthConfiguration(t *testing.T) {
	v := map[string]string{"AUTH_EMAIL_KEY": strings.Repeat("01", 32)}
	c, e := LoadAuth(lookup(v), "development")
	if e != nil || c.Secure {
		t.Fatal("dev auth config")
	}
	v["PUBLIC_ORIGIN"] = "https://example.test"
	c, e = LoadAuth(lookup(v), "production")
	if e != nil || !c.Secure {
		t.Fatal("production cookies")
	}
	c, e = LoadAuth(lookup(v), "staging")
	if e != nil || !c.Secure {
		t.Fatal("staging cookies must be secure")
	}
	v["PUBLIC_ORIGIN"] = "http://example.test"
	if _, e = LoadAuth(lookup(v), "staging"); e == nil {
		t.Fatal("staging accepted insecure origin")
	}
	for _, key := range []string{"AUTH_EMAIL_KEY", "PUBLIC_ORIGIN", "SMTP_ADDR", "SESSION_TTL"} {
		bad := map[string]string{"AUTH_EMAIL_KEY": strings.Repeat("01", 32)}
		bad[key] = "invalid-secret"
		_, e = LoadAuth(lookup(bad), "development")
		if e == nil || strings.Contains(e.Error(), "invalid-secret") {
			t.Fatal("invalid config or secret leak", key)
		}
	}
}

func TestDedicatedRetentionKey(t *testing.T) {
	base := map[string]string{"AUTH_EMAIL_KEY": strings.Repeat("01", 32)}
	get := func(k string) string { return base[k] }
	c, e := LoadAuth(get, "development")
	if e != nil || string(c.RetentionKey()) != string(c.EmailKey) {
		t.Fatal("legacy retention derivation", e)
	}
	base["CATALOG_RETENTION_KEY"] = strings.Repeat("02", 32)
	c, e = LoadAuth(get, "development")
	if e != nil || len(c.RetentionKey()) != 32 || string(c.RetentionKey()) == string(c.EmailKey) {
		t.Fatal("independent retention key", e)
	}
	for _, bad := range []string{"invalid", strings.Repeat("01", 31), strings.Repeat("01", 33)} {
		base["CATALOG_RETENTION_KEY"] = bad
		if _, e = LoadAuth(get, "development"); e == nil {
			t.Fatal("invalid retention key accepted")
		}
	}
}
