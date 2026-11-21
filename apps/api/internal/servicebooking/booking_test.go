package servicebooking

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
)

func instant(t *testing.T, s string) time.Time {
	t.Helper()
	at, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return at
}
func TestTimezoneGapFoldAndOvernight(t *testing.T) {
	tests := []struct {
		name, start, end, zone string
		valid                  bool
	}{
		{"moscow", "2027-01-02T23:30:00+03:00", "2027-01-03T00:30:00+03:00", "Europe/Moscow", true},
		{"dst-gap", "2027-03-14T02:30:00-05:00", "2027-03-14T04:30:00-04:00", "America/New_York", false},
		{"dst-fold-early-explicit", "2027-11-07T01:15:00-04:00", "2027-11-07T01:45:00-04:00", "America/New_York", true},
		{"dst-fold-late-explicit", "2027-11-07T01:15:00-05:00", "2027-11-07T01:45:00-05:00", "America/New_York", true},
		{"fold-offset-changes", "2027-11-07T01:15:00-04:00", "2027-11-07T01:45:00-05:00", "America/New_York", true},
		{"wrong-offset", "2027-01-02T10:00:00Z", "2027-01-02T11:00:00Z", "Europe/Moscow", false},
		{"wrong-fold-offset", "2027-11-07T01:15:00-06:00", "2027-11-07T01:45:00-06:00", "America/New_York", false},
		{"local-host-forbidden", "2027-01-02T10:00:00Z", "2027-01-02T11:00:00Z", "Local", false},
		{"unknown-zone", "2027-01-02T10:00:00Z", "2027-01-02T11:00:00Z", "Mars/Olympus", false},
		{"utc", "2027-01-02T10:00:00Z", "2027-01-02T11:00:00Z", "UTC", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateZonedInterval(instant(t, tt.start), instant(t, tt.end), tt.zone, 7*24*time.Hour)
			if (err == nil) != tt.valid {
				t.Fatalf("valid=%t err=%v", tt.valid, err)
			}
		})
	}
}
func TestSelectionBoundsAndCanonicalIdempotency(t *testing.T) {
	start := instant(t, "2027-11-07T01:15:00-04:00")
	end := instant(t, "2027-11-07T01:45:00-04:00")
	s := Selection{OfferingID: "11111111-1111-4111-8111-111111111111", OfferingRevision: 1, StartsAt: start, EndsAt: end, Timezone: "America/New_York", Units: 1}
	if err := s.validate(); err != nil {
		t.Fatal(err)
	}
	copy := s
	loc, err := time.LoadLocation(s.Timezone)
	if err != nil {
		t.Fatal(err)
	}
	copy.StartsAt = copy.StartsAt.In(loc)
	copy.EndsAt = copy.EndsAt.In(loc)
	if !bytes.Equal(s.hash(), copy.hash()) {
		t.Fatal("identical instants hash differently")
	}
	changed := s
	changed.Units = 2
	if bytes.Equal(s.hash(), changed.hash()) {
		t.Fatal("units not bound")
	}
	changed = s
	changed.OfferingRevision++
	if bytes.Equal(s.hash(), changed.hash()) {
		t.Fatal("revision not bound")
	}
	for _, mutate := range []func(*Selection){func(s *Selection) { s.OfferingID = "foreign" }, func(s *Selection) { s.OfferingRevision = 0 }, func(s *Selection) { s.Units = 0 }, func(s *Selection) { s.Units = 10001 }, func(s *Selection) { s.EndsAt = s.StartsAt }, func(s *Selection) { s.EndsAt = s.StartsAt.Add(8 * 24 * time.Hour) }, func(s *Selection) { s.StartsAt = s.StartsAt.Add(time.Nanosecond) }} {
		bad := s
		mutate(&bad)
		if err := bad.validate(); err == nil {
			t.Fatal("invalid selection admitted")
		}
	}
}
func TestUnavailableDependenciesDoNotAuthorize(t *testing.T) {
	ctx := context.Background()
	s := Service{}
	if err := ValidateSelectionTx(ctx, nil, "invalid", Selection{}); err == nil {
		t.Fatal("nil tx accepted")
	}
	if err := ValidateOfferingTx(ctx, nil, "invalid", "invalid"); err == nil {
		t.Fatal("nil tx accepted")
	}
	if err := ConfirmTx(ctx, nil, "invalid", "invalid", Selection{}); err == nil {
		t.Fatal("nil tx accepted")
	}
	if err := CancelTx(ctx, nil, "invalid", "invalid"); err == nil {
		t.Fatal("nil tx accepted")
	}
	if _, err := s.Search(ctx, catalog.Tenant{}, "", 1); err == nil {
		t.Fatal("nil pool accepted")
	}
	for _, tenant := range []catalog.Tenant{{ChannelType: "telegram", ChannelCapability: "synthetic"}, {ChannelType: "whatsapp", ChannelCapability: "synthetic"}, {AgentLease: "synthetic"}, {AgentTaskID: "synthetic"}} {
		if _, err := begin(ctx, nil, tenant); err == nil {
			t.Fatal("untrusted execution boundary accepted")
		}
	}
}
func TestOfferingUnknownMoneyAndOverflow(t *testing.T) {
	org := "11111111-1111-4111-8111-111111111111"
	o := Offering{ID: org, Name: "Synthetic", Status: "active", Revision: 1, PriceModel: "on_request", PriceState: "unknown", Currency: "RUB", MinMinutes: 30, MaxMinutes: 60, Timezone: "UTC", ResourceIDs: []string{org}}
	if err := o.validate(org); err != nil {
		t.Fatal(err)
	}
	zero := int64(0)
	bad := o
	bad.AmountMinor = &zero
	if err := bad.validate(org); err == nil {
		t.Fatal("unknown money became zero")
	}
	bad = o
	bad.MinMinutes = int(^uint(0) >> 1)
	bad.MaxMinutes = bad.MinMinutes
	if err := bad.validate(org); err == nil {
		t.Fatal("duration overflow")
	}
	bad = o
	bad.ResourceIDs = []string{org, org}
	if err := bad.validate(org); err == nil {
		t.Fatal("duplicate resource")
	}
}
