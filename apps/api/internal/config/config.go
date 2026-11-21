package config

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Environment      string
	HTTPAddr         string
	DatabaseURL      string
	DBTimeout        time.Duration
	RequestTimeout   time.Duration
	ShutdownTimeout  time.Duration
	MaxBodyBytes     int64
	DBMaxConnections int32
	LogLevel         string
}

// Load accepts a lookup function so tests never mutate process environment.
func Load(get func(string) string) (Config, error) {
	if err := ValidateInstagram(get); err != nil {
		return Config{}, err
	}
	c := Config{Environment: value(get, "APP_ENV", "development"), HTTPAddr: value(get, "HTTP_ADDR", ":8080"), DatabaseURL: get("DATABASE_URL"), LogLevel: value(get, "LOG_LEVEL", "info")}
	if c.Environment != "development" && c.Environment != "test" && c.Environment != "production" {
		return Config{}, errors.New("APP_ENV is invalid")
	}
	if _, port, err := net.SplitHostPort(c.HTTPAddr); err != nil || port == "" {
		return Config{}, errors.New("HTTP_ADDR must contain host and port")
	} else if n, e := strconv.Atoi(port); e != nil || n < 1 || n > 65535 {
		return Config{}, errors.New("HTTP_ADDR port is invalid")
	}
	u, err := url.Parse(c.DatabaseURL)
	if err != nil || u == nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Hostname() == "" || u.User == nil || u.User.Username() == "" || strings.Trim(u.Path, "/") == "" {
		return Config{}, errors.New("DATABASE_URL must be a PostgreSQL URL with host, user and database")
	}
	if u.User.Username() != "aba_app" {
		return Config{}, errors.New("DATABASE_URL must use aba_app runtime role")
	}
	if c.Environment == "production" && u.Query().Get("sslmode") != "verify-full" {
		return Config{}, errors.New("production DATABASE_URL requires sslmode=verify-full")
	}
	if c.LogLevel != "debug" && c.LogLevel != "info" && c.LogLevel != "warn" && c.LogLevel != "error" {
		return Config{}, errors.New("LOG_LEVEL is invalid")
	}
	for _, entry := range []struct {
		name     string
		target   *time.Duration
		fallback string
	}{{"DB_TIMEOUT", &c.DBTimeout, "2s"}, {"REQUEST_TIMEOUT", &c.RequestTimeout, "5s"}, {"SHUTDOWN_TIMEOUT", &c.ShutdownTimeout, "10s"}} {
		v, e := time.ParseDuration(value(get, entry.name, entry.fallback))
		if e != nil || v < time.Millisecond || v > time.Minute {
			return Config{}, errors.New(entry.name + " must be between 1ms and 1m")
		}
		*entry.target = v
	}
	if c.DBTimeout >= c.RequestTimeout {
		return Config{}, errors.New("DB_TIMEOUT must be less than REQUEST_TIMEOUT")
	}
	n, e := strconv.ParseInt(value(get, "MAX_BODY_BYTES", "1048576"), 10, 64)
	if e != nil || n < 1 || n > 10485760 {
		return Config{}, errors.New("MAX_BODY_BYTES must be between 1 and 10485760")
	}
	c.MaxBodyBytes = n
	n, e = strconv.ParseInt(value(get, "DB_MAX_CONNECTIONS", "5"), 10, 32)
	if e != nil || n < 1 || n > 100 {
		return Config{}, errors.New("DB_MAX_CONNECTIONS must be between 1 and 100")
	}
	c.DBMaxConnections = int32(n)
	return c, nil
}
func value(get func(string) string, key, fallback string) string {
	if v := get(key); v != "" {
		return v
	}
	return fallback
}
