package config

import (
	"strings"
	"testing"
)

func TestInstagramRuntimeCannotBeEnabled(t *testing.T) {
	defaults := map[string]string{"DATABASE_URL": "postgresql://aba_app:synthetic@localhost/aba?sslmode=disable"}
	if _, err := Load(lookup(defaults)); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ key, value string }{
		{"INSTAGRAM_ENABLED", "true"}, {"INSTAGRAM_LIVE_ENABLED", "true"}, {"INSTAGRAM_EXTERNAL_REQUESTS_AUTHORIZED", "true"}, {"INSTAGRAM_SENDING_KILL_SWITCH", "false"}, {"INSTAGRAM_MONETARY_BUDGET_RUB", "1"}, {"INSTAGRAM_APP_SECRET", "synthetic-secret"}, {"INSTAGRAM_ACCESS_TOKEN", "synthetic-token"}, {"INSTAGRAM_ENCRYPTION_KEY", "synthetic-key"}, {"INSTAGRAM_VERIFY_TOKEN", "synthetic-verify"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			values := map[string]string{"DATABASE_URL": defaults["DATABASE_URL"], tc.key: tc.value}
			_, err := Load(lookup(values))
			if err == nil {
				t.Fatal("unsafe configuration accepted")
			}
			if strings.Contains(err.Error(), tc.value) {
				t.Fatal("configuration leaked")
			}
		})
	}
}

func TestInstagramOnlySyntheticNonProductionRuntime(t *testing.T) {
	values := map[string]string{"INSTAGRAM_ENABLED": "true", "INSTAGRAM_ENCRYPTION_KEY": strings.Repeat("49", 32), "INSTAGRAM_APP_SECRET": "mock:synthetic-instagram-secret", "INSTAGRAM_VERIFY_TOKEN": "mock:synthetic-instagram-verify"}
	cfg, err := LoadInstagram(lookup(values), "test")
	if err != nil || !cfg.Enabled || len(cfg.Key) != 32 {
		t.Fatal("synthetic configuration rejected", err)
	}
	if _, err := LoadInstagram(lookup(values), "production"); err == nil {
		t.Fatal("production enabled")
	}
	values["INSTAGRAM_SENDING_KILL_SWITCH"] = "false"
	if _, err := LoadInstagram(lookup(values), "test"); err == nil {
		t.Fatal("valid mock connection allowed disabling the send kill switch")
	}
	delete(values, "INSTAGRAM_SENDING_KILL_SWITCH")
	values["INSTAGRAM_APP_SECRET"] = "not-a-mock-secret"
	if _, err := LoadInstagram(lookup(values), "test"); err == nil {
		t.Fatal("non-synthetic secret accepted")
	}
}
