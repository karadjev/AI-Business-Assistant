package agentruntime

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGrowthRecommendationsReportsMissingAndInconsistentEvidence(t *testing.T) {
	raw := json.RawMessage(`[
	 {"agent_id":"analytics","status":"completed","result":[{"metrics":[{"name":"completed_orders_created_last_30d","value":2},{"name":"completed_orders_created_previous_30d","value":4}]}]},
	 {"agent_id":"sales","status":"completed","result":[{"metrics":[{"name":"units","value":0}]}]},
	 {"agent_id":"marketing","status":"cancelled","result":null},
	 {"agent_id":"product_inventory","status":"completed","result":[{"metrics":[{"name":"unknown","value":3}]}]}
	]`)
	got := strings.Join(growthRecommendations("coordinate_growth", raw), "\n")
	for _, want := range []string{"подтверждённым остатком", "стало меньше", "расхождение", "Часть подзадач"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
}
