// Package servicebooking implements tenant-scoped service discovery and booking
// admission inside the existing orders transaction. It has no channel sender.
package servicebooking

import (
	"crypto/sha256"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	bf "github.com/karadjev/AI-Business-Assistant/apps/api/internal/businessfoundation"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
)

type Selection struct {
	OfferingID       string    `json:"offering_id"`
	OfferingRevision int64     `json:"offering_revision"`
	StartsAt         time.Time `json:"starts_at"`
	EndsAt           time.Time `json:"ends_at"`
	Timezone         string    `json:"timezone"`
	Units            int       `json:"units"`
}
type PartRule struct {
	State    string `json:"state"`
	OEM      string `json:"oem,omitempty"`
	Quantity int    `json:"quantity,omitempty"`
	Make     string `json:"make,omitempty"`
	Model    string `json:"model,omitempty"`
	Engine   string `json:"engine,omitempty"`
	Year     int    `json:"year,omitempty"`
}

type Offering struct {
	PartRule     *PartRule `json:"part_rule,omitempty"`
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Status       string    `json:"status"`
	Revision     int64     `json:"revision"`
	PriceModel   string    `json:"price_model"`
	PriceState   string    `json:"price_state"`
	AmountMinor  *int64    `json:"amount_minor"`
	Currency     string    `json:"currency"`
	MinMinutes   int       `json:"min_minutes"`
	MaxMinutes   int       `json:"max_minutes"`
	BufferBefore int       `json:"buffer_before"`
	BufferAfter  int       `json:"buffer_after"`
	Timezone     string    `json:"timezone"`
	ResourceIDs  []string  `json:"resource_ids"`
}
type Resource struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Capacity int    `json:"capacity"`
	Active   bool   `json:"active"`
	Revision int64  `json:"revision"`
}
type Window struct {
	ID         string    `json:"id"`
	OfferingID string    `json:"offering_id"`
	ResourceID string    `json:"resource_id"`
	Layer      string    `json:"layer"`
	StartsAt   time.Time `json:"starts_at"`
	EndsAt     time.Time `json:"ends_at"`
	Timezone   string    `json:"timezone"`
}
type Availability struct {
	Available bool      `json:"available"`
	State     string    `json:"state"`
	Selection Selection `json:"selection"`
}

func invalid() error {
	return tenancy.Failure(422, "VALIDATION_ERROR", "Некорректные параметры услуги")
}
func unavailable() error {
	return tenancy.Failure(409, "SERVICE_UNAVAILABLE", "Время или ресурсы услуги недоступны")
}
func conflict() error {
	return tenancy.Failure(409, "SOURCE_CHANGED", "Условия услуги изменились")
}
func validText(s string, n int) bool {
	return utf8.ValidString(s) && strings.TrimSpace(s) != "" && len(s) <= n && !strings.ContainsRune(s, 0)
}

// ValidateZonedInterval requires RFC3339 instants carrying an explicit offset
// consistent with the selected IANA zone at EACH endpoint. A DST gap cannot
// round-trip; an ambiguous fold is resolved by the caller's explicit offset.
func ValidateZonedInterval(start, end time.Time, zone string, max time.Duration) error {
	if zone == "" || zone == "Local" || len(zone) > 128 || start.IsZero() || end.IsZero() || !end.After(start) || end.Sub(start) > max || start.Year() < 2000 || end.Year() > 2200 {
		return invalid()
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return invalid()
	}
	for _, at := range []time.Time{start, end} {
		if at.Nanosecond() != 0 {
			return invalid()
		}
		if at.Format("2006-01-02T15:04:05-07:00") != at.In(loc).Format("2006-01-02T15:04:05-07:00") {
			return invalid()
		}
	}
	return nil
}
func (s Selection) validate() error {
	if !organizations.ValidID(s.OfferingID) || s.OfferingRevision < 1 || s.Units < 1 || s.Units > 10000 {
		return invalid()
	}
	return ValidateZonedInterval(s.StartsAt, s.EndsAt, s.Timezone, 7*24*time.Hour)
}
func (s Selection) hash() []byte {
	// Canonicalize instants while retaining the explicit business timezone.
	s.StartsAt = s.StartsAt.UTC()
	s.EndsAt = s.EndsAt.UTC()
	b, _ := json.Marshal(s)
	h := sha256.Sum256(b)
	return h[:]
}
func (o Offering) domain(org string) bf.Offering {
	ref := func(id string) bf.Reference { return bf.Reference{OrganizationID: org, ID: id} }
	refs := make([]bf.Reference, 0, len(o.ResourceIDs))
	for _, id := range o.ResourceIDs {
		refs = append(refs, ref(id))
	}
	return bf.Offering{Reference: ref(o.ID), Service: ref(o.ID), Status: o.Status, Price: bf.Price{Model: o.PriceModel, State: o.PriceState, Currency: o.Currency, AmountMinor: o.AmountMinor}, MinDuration: time.Duration(o.MinMinutes) * time.Minute, MaxDuration: time.Duration(o.MaxMinutes) * time.Minute, BufferBefore: time.Duration(o.BufferBefore) * time.Minute, BufferAfter: time.Duration(o.BufferAfter) * time.Minute, Resources: refs}
}
func (o Offering) validate(org string) error {
	if !validText(o.Name, 256) || o.Revision < 1 || o.MinMinutes < 0 || o.MinMinutes > 10080 || o.MaxMinutes < 0 || o.MaxMinutes > 10080 || o.BufferBefore < 0 || o.BufferBefore > 1440 || o.BufferAfter < 0 || o.BufferAfter > 1440 {
		return invalid()
	}
	if _, err := time.LoadLocation(o.Timezone); err != nil || o.Timezone == "Local" || o.Timezone == "" {
		return invalid()
	}
	if err := o.domain(org).Validate(org); err != nil {
		return invalid()
	}
	return nil
}
