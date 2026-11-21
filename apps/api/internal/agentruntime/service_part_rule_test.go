package agentruntime

import (
	"testing"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/servicebooking"
)

func TestResolveServicePartUsesOfferingRule(t *testing.T) {
	for _, c := range []struct {
		name                    string
		rule                    servicebooking.PartRule
		optOut                  bool
		state, requirement, oem string
	}{
		{"mandatory overrides opt-out", servicebooking.PartRule{State: "REQUIRED", OEM: "APPROVED", Quantity: 2}, true, "slot_available", "REQUIRED", "APPROVED"},
		{"optional requires choice", servicebooking.PartRule{State: "OPTIONAL", OEM: "APPROVED", Quantity: 2}, false, "part_decision_required", "OPTIONAL", ""},
		{"optional opt-out", servicebooking.PartRule{State: "OPTIONAL", OEM: "APPROVED", Quantity: 2}, true, "slot_available", "OPTIONAL", ""},
		{"not required", servicebooking.PartRule{State: "NOT_REQUIRED"}, false, "slot_available", "NOT_REQUIRED", ""},
		{"unknown", servicebooking.PartRule{State: "UNKNOWN"}, true, "part_requirement_unknown", "UNKNOWN", ""},
		{"vehicle mismatch", servicebooking.PartRule{State: "REQUIRED", OEM: "APPROVED", Quantity: 2, Model: "other"}, false, "part_requirement_unknown", "UNKNOWN", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := delegationResult{State: "slot_available", Service: &ServiceRequirements{NoPart: c.optOut, Vehicle: catalog.FitmentVehicle{Model: "synthetic", Engine: "e"}}}
			resolveServicePart(&got, &c.rule)
			if got.State != c.state || got.PartRequirement != c.requirement {
				t.Fatalf("unexpected result %s/%s", got.State, got.PartRequirement)
			}
			switch {
			case c.oem == "":
				if got.Service.Part != nil {
					t.Fatal("unexpected part")
				}
			default:
				if got.Service.NoPart || got.Service.Part == nil || got.Service.Part.OEM != c.oem || got.Service.Part.Quantity != 2 {
					t.Fatal("approved rule did not set part")
				}
			}
		})
	}
}
