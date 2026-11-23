package agentruntime

import "testing"

func TestVehicleServiceMayUseHybridForPartBearingAppointments(t *testing.T) {
	for _, tc := range []struct {
		model, industry string
		valid           bool
	}{
		{"service", "vehicle_service", true},
		{"hybrid", "vehicle_service", true},
		{"hybrid", "mixed_nonfood", true},
		{"trade", "vehicle_service", false},
		{"agency", "vehicle_service", false},
		{"hybrid", "real_estate", false},
		{"", "", false},
		{"trade", "unregistered", false},
	} {
		t.Run(tc.model+"/"+tc.industry, func(t *testing.T) {
			got := (BusinessProfile{Model: tc.model, Industry: tc.industry, Revision: 1}).valid()
			if got != tc.valid {
				t.Fatalf("profile validity=%v want %v", got, tc.valid)
			}
		})
	}
}
