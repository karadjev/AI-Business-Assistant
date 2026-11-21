package specialists

import (
	"testing"

	af "github.com/karadjev/AI-Business-Assistant/apps/api/internal/agentfoundation"
)

func TestNewBusinessModelDoesNotRequireIndustryCodeChange(t *testing.T) {
	registrations, err := Registrations(scope.TenantID, Dependencies{BusinessType: "custom_nonfood_business"})
	if err != nil {
		t.Fatalf("valid new business model rejected by closed industry list: %v", err)
	}
	for _, r := range registrations {
		catalogAgent := r.Agent.ID == "customer" || r.Agent.ID == "sales" || r.Agent.ID == "product_inventory"
		if catalogAgent && r.Agent.Status != af.AgentDisabled {
			t.Fatal("new model silently enabled legacy catalog specialization")
		}
		if !catalogAgent && r.Agent.Status != af.AgentEnabled {
			t.Fatal("generic adapter tied to fixed industry list")
		}
	}
}

func TestBusinessModelAndIndustryAreIndependent(t *testing.T) {
	for _, test := range []struct {
		model    string
		profiles []string
		catalog  bool
	}{
		{"trade", []string{"electronics"}, true},
		{"trade", []string{"auto_parts"}, false},
		{"service", []string{"electronics"}, false},
		{"hybrid", []string{"electronics", "vehicle_service"}, false},
		{"custom_nonfood_business", []string{"new_specialization"}, false},
		{"trade", nil, false},
	} {
		t.Run(test.model+"/"+stringList(test.profiles), func(t *testing.T) {
			registrations, err := Registrations(scope.TenantID, Dependencies{BusinessModel: test.model, IndustryProfiles: test.profiles})
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range registrations {
				catalogAgent := r.Agent.ID == "customer" || r.Agent.ID == "sales" || r.Agent.ID == "product_inventory"
				if catalogAgent {
					if (r.Agent.Status == af.AgentEnabled) != test.catalog {
						t.Fatal("legacy catalog specialization incorrectly admitted")
					}
				} else if len(r.Agent.SupportedBusinessTypes) != 1 || r.Agent.SupportedBusinessTypes[0] != test.model || stringList(r.Agent.SupportedIndustryProfiles) != stringList(test.profiles) {
					t.Fatal("business model and industry metadata conflated")
				}
			}
		})
	}
	for _, in := range []Dependencies{
		{},
		{BusinessModel: "trade", BusinessType: "service"},
		{BusinessModel: "trade", IndustryProfiles: []string{"electronics", "electronics"}},
		{BusinessModel: "trade", IndustryProfiles: []string{"invalid/profile"}},
	} {
		if _, err := Registrations(scope.TenantID, in); err == nil {
			t.Fatal("invalid or ambiguous configuration admitted")
		}
	}
}

func stringList(values []string) string {
	result := ""
	for _, value := range values {
		result += value + "/"
	}
	return result
}
