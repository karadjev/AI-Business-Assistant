// Package businessfoundation provides isolated, non-persistent domain validation.
// Its projections do not authorize operations or register application modules.
package businessfoundation

import (
	"errors"
	"fmt"
	"regexp"
	"time"
)

var (
	ErrInvalid     = errors.New("invalid domain input")
	ErrTenant      = errors.New("tenant mismatch")
	ErrEligibility = errors.New("business eligibility denied")
	ErrUnsupported = errors.New("unsupported capability or operation")
	// Same canonical lowercase UUID format as organizations.ValidID, without its DB dependencies.
	idPattern   = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	codePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
)

// Reference validation checks syntax and tenant-label consistency ONLY.
// It does not verify resource existence, ownership, membership or authorization.
// Repositories must resolve IDs with tenant-scoped authorization and database gates.
type Reference struct{ OrganizationID, ID string }

func (r Reference) Validate(tenant string) error {
	if !idPattern.MatchString(tenant) || !idPattern.MatchString(r.OrganizationID) || !idPattern.MatchString(r.ID) {
		return ErrInvalid
	}
	if r.OrganizationID != tenant {
		return ErrTenant
	}
	return nil
}

type Capability string

const (
	ProductSales       Capability = "product_sales"
	ServiceRequests    Capability = "service_requests"
	Scheduling         Capability = "scheduling"
	PropertyListings   Capability = "property_listings"
	CustomQuotations   Capability = "custom_quotations"
	ProductionRequests Capability = "production_requests"
	WholesalePricing   Capability = "wholesale_pricing"
)

func validCapability(c Capability) bool {
	switch c {
	case ProductSales, ServiceRequests, Scheduling, PropertyListings, CustomQuotations, ProductionRequests, WholesalePricing:
		return true
	}
	return false
}

type Binding struct {
	Code    string
	Version int
	Primary bool
}

// FoodTrade is a declared eligibility fact, independent of any custom industry label.
// An absent declaration fails closed when product sales are configured.
type Configuration struct {
	Capabilities []Capability
	FoodTrade    *bool
}
type Business struct {
	OrganizationID     string
	Revision           int
	Model              string
	SellingModes       []string
	Industries         []Binding
	Timezone, Currency string
	Configuration      Configuration
}

func (b Business) Validate() error {
	if !idPattern.MatchString(b.OrganizationID) || b.Revision < 1 || !codePattern.MatchString(b.Model) || b.Currency != "RUB" {
		return ErrInvalid
	}
	if _, err := time.LoadLocation(b.Timezone); err != nil || (b.Timezone == "" || b.Timezone == "Local") {
		return ErrInvalid
	}
	if len(b.SellingModes) > 8 || len(b.Industries) == 0 || len(b.Industries) > 8 || len(b.Configuration.Capabilities) > 16 {
		return ErrInvalid
	}
	modes := map[string]bool{}
	for _, m := range b.SellingModes {
		if !codePattern.MatchString(m) || modes[m] {
			return ErrInvalid
		}
		modes[m] = true
	}
	bindings := map[string]bool{}
	primary := 0
	for _, p := range b.Industries {
		if !codePattern.MatchString(p.Code) || p.Version < 1 || bindings[p.Code] {
			return ErrInvalid
		}
		bindings[p.Code] = true
		if p.Primary {
			primary++
		}
	}
	if primary != 1 {
		return ErrInvalid
	}
	seen := map[Capability]bool{}
	for _, c := range b.Configuration.Capabilities {
		if !validCapability(c) || seen[c] {
			return ErrUnsupported
		}
		seen[c] = true
	}
	if b.Configuration.FoodTrade != nil && *b.Configuration.FoodTrade {
		return ErrEligibility
	}
	if seen[ProductSales] && b.Configuration.FoodTrade == nil {
		return ErrEligibility
	}
	if seen[Scheduling] && !seen[ServiceRequests] {
		return ErrUnsupported
	}
	if seen[WholesalePricing] && !seen[ProductSales] {
		return ErrUnsupported
	}
	return nil
}

// Admission contains caller-supplied server facts. Industry configuration cannot
// create these facts. Effective is a projection, never an authorization token.
type Admission struct{ Implemented, Enabled, Authorized, PolicyAllowed map[Capability]bool }

func Effective(b Business, a Admission) ([]Capability, error) {
	if err := b.Validate(); err != nil {
		return nil, err
	}
	out := []Capability{}
	for _, c := range b.Configuration.Capabilities {
		if a.Implemented[c] && a.Enabled[c] && a.Authorized[c] && a.PolicyAllowed[c] {
			out = append(out, c)
		}
	}
	// Scheduling and wholesale depend on their base capability at every gate.
	has := func(w Capability) bool {
		for _, c := range out {
			if c == w {
				return true
			}
		}
		return false
	}
	filtered := []Capability{}
	for _, c := range out {
		if c == Scheduling && !has(ServiceRequests) || c == WholesalePricing && !has(ProductSales) {
			continue
		}
		filtered = append(filtered, c)
	}
	return filtered, nil
}

type Profile struct {
	Code                 string
	Version              int
	RequiredCapabilities []Capability
	Attributes           []AttributeDefinition
}

func (p Profile) Validate() error {
	if !codePattern.MatchString(p.Code) || p.Version < 1 || len(p.RequiredCapabilities) > 16 {
		return ErrInvalid
	}
	seen := map[Capability]bool{}
	for _, c := range p.RequiredCapabilities {
		if !validCapability(c) || seen[c] {
			return ErrUnsupported
		}
		seen[c] = true
	}
	return ValidateDefinitions(p.Attributes)
}

// ValidateBindings resolves exact versions; it never silently upgrades a binding.
func ValidateBindings(b Business, profiles []Profile, effective []Capability) error {
	if err := b.Validate(); err != nil {
		return err
	}
	if len(profiles) > 64 {
		return ErrInvalid
	}
	registry := map[string]Profile{}
	for _, p := range profiles {
		if err := p.Validate(); err != nil {
			return err
		}
		key := fmt.Sprintf("%s/%d", p.Code, p.Version)
		if _, ok := registry[key]; ok {
			return ErrInvalid
		}
		registry[key] = p
	}
	caps := map[Capability]bool{}
	for _, c := range effective {
		caps[c] = true
	}
	definitions := []AttributeDefinition{}
	for _, binding := range b.Industries {
		p, ok := registry[fmt.Sprintf("%s/%d", binding.Code, binding.Version)]
		if !ok {
			return ErrUnsupported
		}
		for _, c := range p.RequiredCapabilities {
			if !caps[c] {
				return ErrUnsupported
			}
		}
		definitions = append(definitions, p.Attributes...)
	}
	// Conflicts require an explicit mapping; no last-wins schema merge.
	return ValidateDefinitions(definitions)
}

// ConfigurationReferences is a facade over canonical organization/store/AI data
// and future subordinate policy/location revisions. It stores no duplicate fields
// and does not assert that the referenced rows or APIs currently exist.
type ConfigurationReferences struct {
	Organization             Reference
	StoreProfile, AISettings *Reference
	Locations, Policies      []Reference
}

func (c ConfigurationReferences) Validate(tenant string) error {
	if err := c.Organization.Validate(tenant); err != nil {
		return err
	}
	if c.Organization.ID != tenant {
		return ErrInvalid
	}
	if len(c.Locations) > 64 || len(c.Policies) > 32 {
		return ErrInvalid
	}
	for _, r := range []*Reference{c.StoreProfile, c.AISettings} {
		if r != nil {
			if err := r.Validate(tenant); err != nil {
				return err
			}
		}
	}
	for _, refs := range [][]Reference{c.Locations, c.Policies} {
		seen := map[string]bool{}
		for _, r := range refs {
			if err := r.Validate(tenant); err != nil {
				return err
			}
			if seen[r.ID] {
				return ErrInvalid
			}
			seen[r.ID] = true
		}
	}
	return nil
}
