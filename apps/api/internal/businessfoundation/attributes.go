package businessfoundation

import (
	"bytes"
	"encoding/json"
	"io"
	"math/big"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

type AttributeKind string

const (
	String     AttributeKind = "string"
	Integer    AttributeKind = "integer"
	Decimal    AttributeKind = "decimal"
	Boolean    AttributeKind = "boolean"
	Enum       AttributeKind = "enum"
	Date       AttributeKind = "date"
	StringList AttributeKind = "string_list"
)

// This first bounded subset deliberately excludes arbitrary nested objects and expressions.
type AttributeDefinition struct {
	Key      string
	Kind     AttributeKind
	Required bool
	Unit     string
	Choices  []string
	Min, Max string // exact numeric bounds, no floating point
}

var decimalPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]{0,17})(\.[0-9]{1,9})?$`)
var integerPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]{0,17})$`)

func number(s string) (*big.Rat, bool) {
	if !decimalPattern.MatchString(s) {
		return nil, false
	}
	return new(big.Rat).SetString(s)
}
func validUnit(u string) bool {
	switch u {
	case "", "kg", "g", "mm", "cm", "m", "m2", "m3", "l", "GB", "month", "piece", "pack", "hour", "minute":
		return true
	}
	return false
}
func reserved(k string) bool {
	switch k {
	case "price", "stock", "organization_id", "permissions", "budget", "live", "system_prompt", "tools":
		return true
	}
	return false
}
func ValidateDefinitions(ds []AttributeDefinition) error {
	if len(ds) > 32 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, d := range ds {
		if !codePattern.MatchString(d.Key) || reserved(d.Key) || seen[d.Key] || !validUnit(d.Unit) || len(d.Choices) > 32 {
			return ErrInvalid
		}
		seen[d.Key] = true
		switch d.Kind {
		case String, Integer, Decimal, Boolean, Enum, Date, StringList:
		default:
			return ErrUnsupported
		}
		numeric := d.Kind == Integer || d.Kind == Decimal
		if !numeric && (d.Min != "" || d.Max != "" || d.Unit != "") {
			return ErrInvalid
		}
		if d.Kind != Enum && len(d.Choices) > 0 || d.Kind == Enum && len(d.Choices) == 0 {
			return ErrInvalid
		}
		choices := map[string]bool{}
		for _, c := range d.Choices {
			if c == "" || len(c) > 256 || !utf8.ValidString(c) || choices[c] {
				return ErrInvalid
			}
			choices[c] = true
		}
		for _, s := range []string{d.Min, d.Max} {
			if s != "" {
				if _, ok := number(s); !ok {
					return ErrInvalid
				}
				if d.Kind == Integer && !integerPattern.MatchString(s) {
					return ErrInvalid
				}
			}
		}
		if d.Min != "" && d.Max != "" {
			lo, _ := number(d.Min)
			hi, _ := number(d.Max)
			if lo.Cmp(hi) > 0 {
				return ErrInvalid
			}
		}
	}
	return nil
}

// AttributeValue is a tagged value. Text holds strings, enum/date and exact decimals;
// Integer and Boolean use pointers so absence cannot be mistaken for zero/false.
type AttributeValue struct {
	Kind    AttributeKind `json:"kind"`
	Text    string        `json:"text,omitempty"`
	Integer *int64        `json:"integer,omitempty"`
	Boolean *bool         `json:"boolean,omitempty"`
	List    []string      `json:"list,omitempty"`
	Unit    string        `json:"unit,omitempty"`
}

func ValidateAttributes(ds []AttributeDefinition, values map[string]AttributeValue) error {
	if err := ValidateDefinitions(ds); err != nil {
		return err
	}
	if len(values) > 32 {
		return ErrInvalid
	}
	raw, err := json.Marshal(values)
	if err != nil || len(raw) > 4096 {
		return ErrInvalid
	}
	defs := map[string]AttributeDefinition{}
	for _, d := range ds {
		defs[d.Key] = d
		if _, ok := values[d.Key]; d.Required && !ok {
			return ErrInvalid
		}
	}
	for k, v := range values {
		d, ok := defs[k]
		if !ok || v.Kind != d.Kind || v.Unit != d.Unit {
			return ErrInvalid
		}
		if v.Kind != Integer && v.Integer != nil || v.Kind != Boolean && v.Boolean != nil || v.Kind != StringList && v.List != nil {
			return ErrInvalid
		}
		if (v.Kind == Integer || v.Kind == Boolean || v.Kind == StringList) && v.Text != "" {
			return ErrInvalid
		}
		if d.Required && ((v.Kind == String && strings.TrimSpace(v.Text) == "") || (v.Kind == StringList && len(v.List) == 0)) {
			return ErrInvalid
		}
		if !utf8.ValidString(v.Text) || len(v.Text) > 256 {
			return ErrInvalid
		}
		var n *big.Rat
		switch v.Kind {
		case Integer:
			if v.Integer == nil {
				return ErrInvalid
			}
			n = new(big.Rat).SetInt64(*v.Integer)
		case Decimal:
			var ok bool
			n, ok = number(v.Text)
			if !ok {
				return ErrInvalid
			}
		case Boolean:
			if v.Boolean == nil {
				return ErrInvalid
			}
		case Date:
			if _, e := time.Parse("2006-01-02", v.Text); e != nil {
				return ErrInvalid
			}
		case Enum:
			found := false
			for _, c := range d.Choices {
				if c == v.Text {
					found = true
				}
			}
			if !found {
				return ErrInvalid
			}
		case StringList:
			if v.List == nil || len(v.List) > 16 {
				return ErrInvalid
			}
			for _, s := range v.List {
				if len(s) > 256 || !utf8.ValidString(s) {
					return ErrInvalid
				}
			}
		}
		if n != nil {
			if d.Min != "" {
				lo, _ := number(d.Min)
				if n.Cmp(lo) < 0 {
					return ErrInvalid
				}
			}
			if d.Max != "" {
				hi, _ := number(d.Max)
				if n.Cmp(hi) > 0 {
					return ErrInvalid
				}
			}
		}
	}
	return nil
}

// DecodeProfile accepts only the declarative schema, rejecting unknown executable
// fields, trailing payloads and oversized configuration before decoding.
func DecodeProfile(raw []byte) (Profile, error) {
	var p Profile
	if len(raw) > 16384 || !utf8.Valid(raw) {
		return p, ErrInvalid
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return Profile{}, ErrInvalid
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return Profile{}, ErrInvalid
	}
	if err := p.Validate(); err != nil {
		return Profile{}, err
	}
	return p, nil
}

// ProductAttributes is a sidecar reference, not a new product/stock/price identity.
type ProductAttributes struct {
	Product        Reference
	Variant        *Reference
	SchemaRevision int
	Values         map[string]AttributeValue
}

func (p ProductAttributes) Validate(tenant string, ds []AttributeDefinition) error {
	if err := p.Product.Validate(tenant); err != nil {
		return err
	}
	if p.Variant != nil {
		if err := p.Variant.Validate(tenant); err != nil {
			return err
		}
	}
	if p.SchemaRevision < 1 {
		return ErrInvalid
	}
	return ValidateAttributes(ds, p.Values)
}

// AttributeRange declares a bounded cross-field comparison such as year_from <=
// year_to. Missing optional values stay unknown; requiredness belongs to definitions.
type AttributeRange struct{ Lower, Upper string }

func ValidateAttributeRanges(ds []AttributeDefinition, values map[string]AttributeValue, ranges []AttributeRange) error {
	if err := ValidateAttributes(ds, values); err != nil {
		return err
	}
	if len(ranges) > 16 {
		return ErrInvalid
	}
	defs := map[string]AttributeDefinition{}
	for _, d := range ds {
		defs[d.Key] = d
	}
	seen := map[AttributeRange]bool{}
	for _, r := range ranges {
		lo, lok := defs[r.Lower]
		hi, hok := defs[r.Upper]
		if !lok || !hok || r.Lower == r.Upper || seen[r] || lo.Kind != hi.Kind || lo.Unit != hi.Unit || (lo.Kind != Integer && lo.Kind != Decimal) {
			return ErrInvalid
		}
		seen[r] = true
		l, lp := values[r.Lower]
		h, hp := values[r.Upper]
		if !lp || !hp {
			continue
		}
		var ln, hn *big.Rat
		if lo.Kind == Integer {
			ln = new(big.Rat).SetInt64(*l.Integer)
			hn = new(big.Rat).SetInt64(*h.Integer)
		} else {
			ln, _ = number(l.Text)
			hn, _ = number(h.Text)
		}
		if ln.Cmp(hn) > 0 {
			return ErrInvalid
		}
	}
	return nil
}
