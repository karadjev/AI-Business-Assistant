package businessfoundation

import (
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

type Price struct {
	Model, State, Currency string
	AmountMinor            *int64
	UpperMinor             *int64
}

func (p Price) Validate() error {
	if p.Currency != "RUB" {
		return ErrUnsupported
	}
	switch p.Model {
	case "fixed", "hourly":
		if p.State != "known" {
			return ErrInvalid
		}
	case "on_request":
		if p.State != "unknown" {
			return ErrInvalid
		}
	case "custom_estimate":
		if p.State != "unknown" && p.State != "estimated" {
			return ErrInvalid
		}
	default:
		return ErrUnsupported
	}
	if p.State == "unknown" {
		if p.AmountMinor != nil || p.UpperMinor != nil {
			return ErrInvalid
		}
		return nil
	}
	if p.AmountMinor == nil || *p.AmountMinor < 0 {
		return ErrInvalid
	}
	if p.UpperMinor != nil && (p.State != "estimated" || *p.UpperMinor < *p.AmountMinor) {
		return ErrInvalid
	}
	return nil
}

type Provider struct {
	Reference
	StaffMembership *Reference
}

func (p Provider) Validate(tenant string) error {
	if err := p.Reference.Validate(tenant); err != nil {
		return err
	}
	if p.StaffMembership != nil {
		return p.StaffMembership.Validate(tenant)
	}
	return nil
}

type Resource struct {
	Reference
	Kind     string
	Capacity int
	Provider *Provider
}

func (r Resource) Validate(tenant string) error {
	if err := r.Reference.Validate(tenant); err != nil {
		return err
	}
	switch r.Kind {
	case "staff", "equipment", "room", "capacity":
	default:
		return ErrInvalid
	}
	if r.Capacity < 1 || r.Capacity > 10000 {
		return ErrInvalid
	}
	if r.Provider != nil {
		return r.Provider.Validate(tenant)
	}
	return nil
}

type Offering struct {
	Reference
	Service                   Reference
	Status                    string
	Price                     Price
	MinDuration, MaxDuration  time.Duration // zero/zero means unknown
	BufferBefore, BufferAfter time.Duration
	Resources                 []Reference
	Location                  *Reference
}

func (o Offering) Validate(tenant string) error {
	if err := o.Reference.Validate(tenant); err != nil {
		return err
	}
	if err := o.Service.Validate(tenant); err != nil {
		return err
	}
	switch o.Status {
	case "draft", "active", "archived":
	default:
		return ErrInvalid
	}
	if err := o.Price.Validate(); err != nil {
		return err
	}
	if o.MinDuration < 0 || o.MaxDuration < o.MinDuration || o.MaxDuration > 7*24*time.Hour || (o.MinDuration == 0) != (o.MaxDuration == 0) || o.BufferBefore < 0 || o.BufferAfter < 0 || o.BufferBefore > 24*time.Hour || o.BufferAfter > 24*time.Hour || len(o.Resources) > 16 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, r := range o.Resources {
		if err := r.Validate(tenant); err != nil {
			return err
		}
		if seen[r.ID] {
			return ErrInvalid
		}
		seen[r.ID] = true
	}
	if o.Location != nil {
		return o.Location.Validate(tenant)
	}
	return nil
}

type Interval struct{ Start, End time.Time }

func (i Interval) valid() bool {
	return !i.Start.IsZero() && !i.End.IsZero() && i.End.After(i.Start) && i.End.Sub(i.Start) <= 31*24*time.Hour
}
func (i Interval) contains(other Interval) bool {
	return !other.Start.Before(i.Start) && !other.End.After(i.End)
}
func (i Interval) overlaps(other Interval) bool {
	return i.Start.Before(other.End) && other.Start.Before(i.End)
}

// All intervals are absolute instants, supplied after timezone/DST resolution.
// Missing Known means unknown. Each layer must cover the buffered slot in full.
type Availability struct {
	Resource                  Reference
	Timezone                  string
	Known                     bool
	Business, Offering, Shift []Interval
	Exceptions                []Interval
}

func (a Availability) Validate(tenant string) error {
	if err := a.Resource.Validate(tenant); err != nil {
		return err
	}
	if a.Timezone == "" || a.Timezone == "Local" {
		return ErrInvalid
	}
	if _, err := time.LoadLocation(a.Timezone); err != nil {
		return ErrInvalid
	}
	for _, layer := range [][]Interval{a.Business, a.Offering, a.Shift, a.Exceptions} {
		if len(layer) > 128 {
			return ErrInvalid
		}
		for _, i := range layer {
			if !i.valid() {
				return ErrInvalid
			}
		}
	}
	return nil
}
func (a Availability) allows(slot Interval) bool {
	if !a.Known {
		return false
	}
	for _, layer := range [][]Interval{a.Business, a.Offering, a.Shift} {
		covered := false
		for _, i := range layer {
			if i.contains(slot) {
				covered = true
			}
		}
		if !covered {
			return false
		}
	}
	for _, i := range a.Exceptions {
		if i.overlaps(slot) {
			return false
		}
	}
	return true
}

type Allocation struct {
	Resource Reference
	Interval Interval
	Units    int
}

// BookingRequest validates a non-binding proposal tied to an EXISTING order.
// It does not create a hold, confirm an order, lock resources or guarantee capacity.
type BookingRequest struct {
	Order    Reference
	Offering Reference
	Slot     Interval
	Units    int
}

func ValidateBooking(tenant string, b BookingRequest, o Offering, resources []Resource, schedules []Availability, allocations []Allocation) error {
	for _, r := range []Reference{b.Order, b.Offering} {
		if err := r.Validate(tenant); err != nil {
			return err
		}
	}
	if err := o.Validate(tenant); err != nil {
		return err
	}
	if b.Offering != o.Reference || o.Status != "active" || o.MinDuration == 0 || !b.Slot.valid() || b.Slot.End.Sub(b.Slot.Start) < o.MinDuration || b.Slot.End.Sub(b.Slot.Start) > o.MaxDuration || b.Units < 1 || b.Units > 10000 || len(o.Resources) == 0 {
		return ErrInvalid
	}
	if len(resources) > 64 || len(schedules) > 64 || len(allocations) > 1024 {
		return ErrInvalid
	}
	rs := map[string]Resource{}
	for _, r := range resources {
		if err := r.Validate(tenant); err != nil {
			return err
		}
		if _, ok := rs[r.ID]; ok {
			return ErrInvalid
		}
		rs[r.ID] = r
	}
	av := map[string]Availability{}
	for _, a := range schedules {
		if err := a.Validate(tenant); err != nil {
			return err
		}
		if _, ok := av[a.Resource.ID]; ok {
			return ErrInvalid
		}
		av[a.Resource.ID] = a
	}
	for _, a := range allocations {
		if err := a.Resource.Validate(tenant); err != nil {
			return err
		}
		r, ok := rs[a.Resource.ID]
		if !ok || !a.Interval.valid() || a.Units < 1 || a.Units > r.Capacity {
			return ErrInvalid
		}
	}
	slot := Interval{b.Slot.Start.Add(-o.BufferBefore), b.Slot.End.Add(o.BufferAfter)}
	for _, ref := range o.Resources {
		r, ok := rs[ref.ID]
		if !ok {
			return ErrInvalid
		}
		a, ok := av[ref.ID]
		if !ok || !a.allows(slot) {
			return ErrUnsupported
		}
		// Half-open intervals: an end at the next start does not overlap.
		type event struct {
			at    time.Time
			delta int
		}
		events := []event{{slot.Start, b.Units}, {slot.End, -b.Units}}
		for _, allocation := range allocations {
			if allocation.Resource == ref && allocation.Interval.overlaps(slot) {
				start, end := allocation.Interval.Start, allocation.Interval.End
				if start.Before(slot.Start) {
					start = slot.Start
				}
				if end.After(slot.End) {
					end = slot.End
				}
				events = append(events, event{start, allocation.Units}, event{end, -allocation.Units})
			}
		}
		sort.Slice(events, func(i, j int) bool {
			if events[i].at.Equal(events[j].at) {
				return events[i].delta < events[j].delta
			}
			return events[i].at.Before(events[j].at)
		})
		used := 0
		for _, e := range events {
			used += e.delta
			if used > r.Capacity {
				return ErrUnsupported
			}
		}
	}
	return nil
}

// Service is a standalone service descriptor, never a product or stock record.
// Offering.Service references this ID; repositories must verify existence and ownership.
type Service struct {
	Reference
	Name     string
	Status   string
	Revision int
}

func (s Service) Validate(tenant string) error {
	if err := s.Reference.Validate(tenant); err != nil {
		return err
	}
	if len(s.Name) == 0 || len(s.Name) > 256 || !utf8.ValidString(s.Name) || strings.TrimSpace(s.Name) == "" || s.Revision < 1 {
		return ErrInvalid
	}
	switch s.Status {
	case "draft", "active", "archived":
		return nil
	}
	return ErrInvalid
}

type BookingStatus string

const (
	BookingDraft           BookingStatus = "draft"
	BookingNeedsAssessment BookingStatus = "needs_assessment"
	BookingProposed        BookingStatus = "proposed"
)

// BookingProposal has no confirmed/held/paid state and performs no transitions.
// Its ID identifies a proposal only; Order is always an existing order reference.
// A future repository must bind revisions and recheck admission transactionally.
type BookingProposal struct {
	Reference
	Request  BookingRequest
	Status   BookingStatus
	Revision int
}

func (p BookingProposal) Validate(tenant string, o Offering, resources []Resource, schedules []Availability, allocations []Allocation) error {
	if err := p.Reference.Validate(tenant); err != nil {
		return err
	}
	if p.Revision < 1 {
		return ErrInvalid
	}
	switch p.Status {
	case BookingDraft, BookingNeedsAssessment, BookingProposed:
	default:
		return ErrUnsupported
	}
	if p.Status == BookingProposed {
		return ValidateBooking(tenant, p.Request, o, resources, schedules, allocations)
	}
	if err := p.Request.Order.Validate(tenant); err != nil {
		return err
	}
	if err := p.Request.Offering.Validate(tenant); err != nil {
		return err
	}
	if err := o.Validate(tenant); err != nil {
		return err
	}
	if p.Request.Offering != o.Reference {
		return ErrInvalid
	}
	// Draft/assessment requests must not contain a fabricated slot or allocation.
	if !p.Request.Slot.Start.IsZero() || !p.Request.Slot.End.IsZero() || p.Request.Units != 0 {
		return ErrInvalid
	}
	return nil
}
