package db

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestInvalidConfiguration(t *testing.T) {
	_, err := Open(context.Background(), "postgresql://password:%zz@localhost/db", 1, time.Second)
	if err == nil {
		t.Fatal("expected invalid DSN")
	}
	if strings.Contains(err.Error(), "password") {
		t.Fatal("configuration leak")
	}
}
