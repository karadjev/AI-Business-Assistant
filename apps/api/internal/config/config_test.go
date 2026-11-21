package config

import (
	"strings"
	"testing"
	"time"
)

func lookup(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}
func TestLoadDefaults(t *testing.T) {
	c, err := Load(lookup(map[string]string{"DATABASE_URL": "postgresql://aba_app:secret@localhost/aba?sslmode=disable"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.HTTPAddr != ":8080" || c.DBTimeout != 2*time.Second || c.MaxBodyBytes != 1048576 || c.DBMaxConnections != 5 {
		t.Fatalf("unexpected defaults: address=%s", c.HTTPAddr)
	}
}
func TestLoadInvalid(t *testing.T) {
	for _, test := range []struct{ name, key, value string }{{"missing db", "DATABASE_URL", ""}, {"malformed db", "DATABASE_URL", "postgresql://secret:%zz@localhost/aba"}, {"superuser", "DATABASE_URL", "postgresql://postgres:secret@localhost/aba"}, {"migrator runtime", "DATABASE_URL", "postgresql://aba_migrator:secret@localhost/aba"}, {"no host", "DATABASE_URL", "postgresql://aba_app/aba"}, {"invalid env", "APP_ENV", "unknown"}, {"invalid address", "HTTP_ADDR", "localhost"}, {"invalid port", "HTTP_ADDR", ":70000"}, {"invalid duration", "DB_TIMEOUT", "oops"}, {"negative timeout", "REQUEST_TIMEOUT", "-1s"}, {"timeout order", "DB_TIMEOUT", "6s"}, {"oversize body", "MAX_BODY_BYTES", "999999999"}, {"invalid pool", "DB_MAX_CONNECTIONS", "0"}, {"invalid level", "LOG_LEVEL", "verbose"}, {"insecure production", "APP_ENV", "production"}} {
		t.Run(test.name, func(t *testing.T) {
			values := map[string]string{"DATABASE_URL": "postgresql://aba_app:secret@localhost/aba?sslmode=disable"}
			values[test.key] = test.value
			_, err := Load(lookup(values))
			if err == nil {
				t.Fatal("expected validation error")
			}
			if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "%zz") {
				t.Fatal("error leaks input")
			}
		})
	}
}
func TestLoadOverrides(t *testing.T) {
	c, err := Load(lookup(map[string]string{"APP_ENV": "production", "DATABASE_URL": "postgresql://aba_app:secret@db/aba?sslmode=verify-full", "HTTP_ADDR": "127.0.0.1:9000", "DB_TIMEOUT": "100ms", "REQUEST_TIMEOUT": "500ms", "SHUTDOWN_TIMEOUT": "2s", "MAX_BODY_BYTES": "500", "DB_MAX_CONNECTIONS": "3", "LOG_LEVEL": "warn"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.MaxBodyBytes != 500 || c.DBMaxConnections != 3 || c.LogLevel != "warn" {
		t.Fatal("overrides ignored")
	}
}
