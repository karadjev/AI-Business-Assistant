package businessfoundation

// Intent is validation input for an existing order flow, with no transition or
// financial confirmation methods. Reference existence/freshness remains a repository gate.
type Intent struct {
	ExistingOrder, Customer, Conversation Reference
	Type                                  string
	Products                              []Reference // existing variant IDs; never creates replacement products
	Offerings                             []Reference
	Specification, Listing                *Reference
	RequestFinancialConfirmation          bool
}

func (i Intent) Validate(tenant string, effective []Capability) error {
	if i.RequestFinancialConfirmation {
		return ErrUnsupported
	}
	for _, r := range []Reference{i.ExistingOrder, i.Customer, i.Conversation} {
		if err := r.Validate(tenant); err != nil {
			return err
		}
	}
	if len(i.Products) > 10 || len(i.Offerings) > 10 {
		return ErrInvalid
	}
	for _, refs := range [][]Reference{i.Products, i.Offerings} {
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
	for _, r := range []*Reference{i.Specification, i.Listing} {
		if r != nil {
			if err := r.Validate(tenant); err != nil {
				return err
			}
		}
	}
	caps := map[Capability]bool{}
	for _, c := range effective {
		caps[c] = true
	}
	p, s, spec, listing := len(i.Products) > 0, len(i.Offerings) > 0, i.Specification != nil, i.Listing != nil
	switch i.Type {
	case "product_sale":
		if !caps[ProductSales] {
			return ErrUnsupported
		}
		if !p || s || spec || listing {
			return ErrInvalid
		}
	case "service_booking":
		if !caps[ServiceRequests] || !caps[Scheduling] {
			return ErrUnsupported
		}
		if p || !s || spec || listing {
			return ErrInvalid
		}
	case "repair_request":
		if !caps[ServiceRequests] {
			return ErrUnsupported
		}
		if p || !s || spec || listing {
			return ErrInvalid
		}
	case "quotation_request":
		if !caps[CustomQuotations] {
			return ErrUnsupported
		}
		if p || s || !spec || listing {
			return ErrInvalid
		}
	case "custom_manufacturing":
		if !caps[ProductionRequests] || !caps[CustomQuotations] {
			return ErrUnsupported
		}
		if p || s || !spec || listing {
			return ErrInvalid
		}
	case "property_inquiry":
		if !caps[PropertyListings] {
			return ErrUnsupported
		}
		if p || s || spec || !listing {
			return ErrInvalid
		}
	case "hybrid":
		if !caps[ProductSales] || !caps[ServiceRequests] {
			return ErrUnsupported
		}
		if !p || !s || spec || listing {
			return ErrInvalid
		}
	default:
		return ErrUnsupported
	}
	return nil
}
