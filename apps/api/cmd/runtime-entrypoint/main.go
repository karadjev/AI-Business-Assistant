// runtime-entrypoint loads explicitly mounted secrets before replacing itself.
// It is used only by the private deployment images, not by developer runners.
package main

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

var secretKeys = []string{"DATABASE_URL", "MIGRATION_DATABASE_URL", "AUTH_EMAIL_KEY", "CATALOG_RETENTION_KEY", "TELEGRAM_ENCRYPTION_KEY", "WHATSAPP_ENCRYPTION_KEY"}

// These images are private staging only. Admission is required at execution,
// including when an operator starts an image without the Compose validator.
func runtimeAdmission(part string, get func(string) string) error {
	if part != "api" && part != "worker" {
		return nil
	}
	for _, control := range [][2]string{
		{"APP_ENV", "production"}, {"LLM_PROVIDER", "mock"}, {"TELEGRAM_TRANSPORT", "mock"},
		{"LLM_ENABLE_LIVE", "false"}, {"LLM_EXTERNAL_REQUESTS_AUTHORIZED", "false"},
		{"LLM_KILL_SWITCH", "true"}, {"LLM_PLATFORM_BUDGET_MICRORUB", "0"},
		{"TELEGRAM_EXTERNAL_REQUESTS_AUTHORIZED", "false"}, {"TELEGRAM_SENDING_ENABLED", "false"},
		{"TELEGRAM_SENDING_KILL_SWITCH", "true"}, {"WHATSAPP_LIVE_ENABLED", "false"},
		{"WHATSAPP_EXTERNAL_REQUESTS_AUTHORIZED", "false"},
		{"WHATSAPP_SENDING_KILL_SWITCH", "true"}, {"WHATSAPP_MONETARY_BUDGET_RUB", "0"},
	} {
		if get(control[0]) != control[1] {
			return errors.New("private runtime security configuration required")
		}
	}
	origin, err := url.Parse(get("PUBLIC_ORIGIN"))
	if err != nil || origin.Scheme != "https" || origin.Hostname() == "" || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" || (origin.Path != "" && origin.Path != "/") {
		return errors.New("private runtime HTTPS origin required")
	}
	return nil
}

func readSecret(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0007 != 0 {
		return "", errors.New("secret must be a regular file without access for others")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", errors.New("secret unavailable")
	}
	defer f.Close()
	actual, err := f.Stat()
	if err != nil || !os.SameFile(info, actual) {
		return "", errors.New("secret changed while opening")
	}
	b, err := io.ReadAll(io.LimitReader(f, 4097))
	if err != nil || len(b) > 4096 {
		return "", errors.New("secret unreadable or too large")
	}
	value := strings.TrimSuffix(string(b), "\n")
	if value == "" || strings.ContainsAny(value, "\x00\r\n") {
		return "", errors.New("secret must contain one nonempty line")
	}
	return value, nil
}

func secretEnvironment(get func(string) string, root string) (map[string]string, error) {
	out := map[string]string{}
	for _, key := range secretKeys {
		path := get(key + "_FILE")
		if path == "" {
			continue
		}
		if get(key) != "" || filepath.Dir(path) != root || filepath.Clean(path) != path {
			return nil, errors.New("conflicting secret value or invalid mount path")
		}
		value, err := readSecret(path)
		if err != nil {
			return nil, err
		}
		out[key] = value
	}
	return out, nil
}

func execute() error {
	if len(os.Args) < 2 {
		return errors.New("missing runtime command")
	}
	part := os.Args[1]
	if part == "worker-live" {
		b, err := os.ReadFile("/proc/1/comm")
		if err != nil || strings.TrimSpace(string(b)) != "worker" {
			return errors.New("worker process unavailable")
		}
		return nil
	}
	if part != "api" && part != "worker" && part != "migrate" && part != "healthcheck" {
		return errors.New("unsupported runtime command")
	}
	if err := runtimeAdmission(part, os.Getenv); err != nil {
		return err
	}
	values, err := secretEnvironment(os.Getenv, "/run/secrets")
	if err != nil {
		return err
	}
	for key, value := range values {
		if err = os.Setenv(key, value); err != nil {
			return errors.New("cannot load runtime secret")
		}
		_ = os.Unsetenv(key + "_FILE")
	}
	args := append([]string{"/app/" + part}, os.Args[2:]...)
	return syscall.Exec(args[0], args, os.Environ())
}

func main() {
	if execute() != nil {
		// No underlying error/path/value is printed: mount paths can be sensitive.
		fmt.Fprintln(os.Stderr, "runtime entrypoint refused configuration or execution")
		os.Exit(1)
	}
}
