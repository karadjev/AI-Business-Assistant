package businessfoundation

// Readiness evidence must be bound to the current tenant/profile revision.
// Channel facts stay separated from configuration and never enable live traffic.
type ChannelReadiness struct {
	Channel                                          string
	MockConfigured, LiveAuthorized, ProviderVerified bool
}
type ReadinessEvidence struct {
	OrganizationID                                                                   string
	ProfileRevision                                                                  int
	Validated, OfflineScenarioPassed                                                 bool
	ProductsReady, ServicesReady, SchedulesReady, ListingsReady, SpecificationsReady bool
	PoliciesReady, PublicContextReady                                                bool
	Channels                                                                         []ChannelReadiness
}
type Readiness struct {
	ConfigurationValidated, OfflineScenarioPassed bool
	Blockers                                      []string
	Channels                                      []ChannelReadiness
}

func ProjectReadiness(b Business, a Admission, profiles []Profile, e ReadinessEvidence) (Readiness, error) {
	var out Readiness
	if err := b.Validate(); err != nil {
		return out, err
	}
	if e.OrganizationID != b.OrganizationID {
		return out, ErrTenant
	}
	if e.ProfileRevision != b.Revision {
		return out, ErrInvalid
	}
	caps, err := Effective(b, a)
	if err != nil {
		return out, err
	}
	if err = ValidateBindings(b, profiles, caps); err != nil {
		return out, err
	}
	out.Blockers = []string{}
	if !e.Validated {
		out.Blockers = append(out.Blockers, "configuration_not_validated")
	}
	if len(caps) != len(b.Configuration.Capabilities) {
		out.Blockers = append(out.Blockers, "capability_unavailable")
	}
	for _, c := range caps {
		ready := true
		switch c {
		case ProductSales:
			ready = e.ProductsReady
		case ServiceRequests:
			ready = e.ServicesReady
		case Scheduling:
			ready = e.SchedulesReady
		case PropertyListings:
			ready = e.ListingsReady
		case CustomQuotations, ProductionRequests:
			ready = e.SpecificationsReady
		}
		if !ready {
			out.Blockers = append(out.Blockers, string(c)+"_not_ready")
		}
	}
	if !e.PoliciesReady {
		out.Blockers = append(out.Blockers, "policies_not_ready")
	}
	if !e.PublicContextReady {
		out.Blockers = append(out.Blockers, "public_context_not_ready")
	}
	if len(e.Channels) > 3 {
		return Readiness{}, ErrInvalid
	}
	seen := map[string]bool{}
	for _, c := range e.Channels {
		switch c.Channel {
		case "telegram", "whatsapp", "instagram":
		default:
			return Readiness{}, ErrUnsupported
		}
		if seen[c.Channel] {
			return Readiness{}, ErrInvalid
		}
		seen[c.Channel] = true
	}
	out.ConfigurationValidated = len(out.Blockers) == 0
	out.OfflineScenarioPassed = out.ConfigurationValidated && e.OfflineScenarioPassed
	out.Channels = append([]ChannelReadiness(nil), e.Channels...)
	return out, nil
}
