package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBrevoSMTPConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic-key")
	if err := os.WriteFile(path, []byte("synthetic-test-key\n"), 0400); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{
		"AUTH_EMAIL_KEY": strings.Repeat("01", 32), "PUBLIC_ORIGIN": "https://example.test",
		"SMTP_MODE": "starttls", "SMTP_ADDR": "smtp-relay.brevo.com:587",
		"SMTP_TLS_SERVER_NAME": "smtp-relay.brevo.com", "SMTP_USERNAME": "example@smtp-brevo.com",
		"SMTP_PASSWORD_FILE": path, "SMTP_FROM_EMAIL": "noreply@example.test", "SMTP_FROM_NAME": "Example",
	}
	cfg, err := LoadAuth(lookup(env), "staging")
	if err != nil || cfg.SMTP.ServerName != "smtp-relay.brevo.com" || cfg.SMTP.Password != "synthetic-test-key" || cfg.SMTP.From.Address != "noreply@example.test" {
		t.Fatal("valid STARTTLS config refused")
	}
	for key, bad := range map[string]string{
		"SMTP_MODE": "plain", "SMTP_ADDR": "smtp-relay.brevo.com:25", "SMTP_TLS_SERVER_NAME": "different.example.test",
		"SMTP_FROM_NAME": "newline\r\n", "SMTP_PASSWORD": "synthetic-test-key", "SMTP_PASSWORD_FILE": "/not-found/synthetic-test-key",
	} {
		old := env[key]
		env[key] = bad
		_, err = LoadAuth(lookup(env), "staging")
		if err == nil || strings.Contains(err.Error(), "synthetic-test-key") {
			t.Fatalf("unsafe configuration was accepted or emitted a secret for %s", key)
		}
		env[key] = old
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAuth(lookup(env), "staging"); err == nil {
		t.Fatal("readable password file accepted")
	}
}
