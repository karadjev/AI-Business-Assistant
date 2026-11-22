package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMountedSecretAdmission(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "db")
	for _, tc := range []struct {
		name, contents string
		mode           os.FileMode
		want           bool
	}{
		{"private", "synthetic\n", 0600, true},
		{"group_readable", "synthetic", 0640, true},
		{"public", "synthetic", 0644, false},
		{"empty", "", 0600, false},
		{"multiline", "first\nsecond", 0600, false},
		{"nul", "first\x00second", 0600, false},
		{"oversized", strings.Repeat("x", 4097), 0600, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(p, []byte(tc.contents), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(p, tc.mode); err != nil {
				t.Fatal(err)
			}
			_, err := readSecret(p)
			if (err == nil) != tc.want {
				t.Fatalf("admission mismatch: %v", err)
			}
		})
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(p, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readSecret(link); err == nil {
		t.Fatal("symlink accepted")
	}
	if _, err := readSecret(dir); err == nil {
		t.Fatal("directory accepted")
	}
}

func TestSecretConflictAndMountBoundary(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "db")
	if err := os.WriteFile(p, []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	get := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	values, err := secretEnvironment(get(map[string]string{"DATABASE_URL_FILE": p}), dir)
	if err != nil || values["DATABASE_URL"] != "synthetic" {
		t.Fatal("private mount not loaded")
	}
	for _, m := range []map[string]string{
		{"DATABASE_URL_FILE": p, "DATABASE_URL": "conflict"},
		{"DATABASE_URL_FILE": filepath.Join(dir, "sub", "db")},
		{"DATABASE_URL_FILE": dir + "/../db"},
	} {
		if _, err := secretEnvironment(get(m), dir); err == nil {
			t.Fatal("unsafe mount accepted")
		}
	}
}

func TestPrivateRuntimeSecurityAdmission(t *testing.T) {
	safe := map[string]string{
		"APP_ENV": "staging", "LLM_PROVIDER": "mock", "TELEGRAM_TRANSPORT": "mock",
		"LLM_ENABLE_LIVE": "false", "LLM_EXTERNAL_REQUESTS_AUTHORIZED": "false",
		"LLM_KILL_SWITCH": "true", "LLM_PLATFORM_BUDGET_MICRORUB": "0",
		"TELEGRAM_EXTERNAL_REQUESTS_AUTHORIZED": "false", "TELEGRAM_SENDING_ENABLED": "false",
		"TELEGRAM_SENDING_KILL_SWITCH": "true", "WHATSAPP_LIVE_ENABLED": "false",
		"WHATSAPP_EXTERNAL_REQUESTS_AUTHORIZED": "false",
		"WHATSAPP_SENDING_KILL_SWITCH":          "true", "WHATSAPP_MONETARY_BUDGET_RUB": "0",
		"PUBLIC_ORIGIN":     "https://staging.invalid:8443",
		"INSTAGRAM_ENABLED": "false", "INSTAGRAM_LIVE_ENABLED": "false",
		"INSTAGRAM_EXTERNAL_REQUESTS_AUTHORIZED": "false", "INSTAGRAM_SENDING_KILL_SWITCH": "true",
		"INSTAGRAM_MONETARY_BUDGET_RUB": "0",
	}
	for _, part := range []string{"api", "worker"} {
		t.Run(part, func(t *testing.T) {
			get := func(key string) string { return safe[key] }
			if err := runtimeAdmission(part, get); err != nil {
				t.Fatal("complete private configuration rejected", err)
			}
			for key, expected := range safe {
				t.Run("missing_"+key, func(t *testing.T) {
					if err := runtimeAdmission(part, func(k string) string {
						if k == key {
							return ""
						}
						return get(k)
					}); err == nil {
						t.Fatal("missing control admitted")
					}
				})
				t.Run("unsafe_"+key, func(t *testing.T) {
					wrong := map[string]string{"staging": "test", "mock": "live", "false": "true", "true": "false", "0": "1"}[expected]
					if key == "PUBLIC_ORIGIN" {
						wrong = "http://staging.invalid"
					}
					if err := runtimeAdmission(part, func(k string) string {
						if k == key {
							return wrong
						}
						return get(k)
					}); err == nil {
						t.Fatal("unsafe control admitted")
					}
				})
			}
		})
	}
	for _, part := range []string{"migrate", "healthcheck", "worker-live"} {
		if err := runtimeAdmission(part, func(string) string { return "" }); err != nil {
			t.Fatal("non-application command requires unrelated flags", part, err)
		}
	}
}
