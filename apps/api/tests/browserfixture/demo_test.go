package main

import (
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"testing"
)

func TestDemoIdentitySeparation(t *testing.T) {
	run := "11111111-1111-4111-8111-111111111111"
	id := demoID(run, "store")
	if !organizations.ValidID(id) || id != demoID(run, "store") || id == demoID(run, "owner") || id == demoID("22222222-2222-4222-8222-222222222222", "store") {
		t.Fatal("fixture identity collision or unstable run")
	}
}
