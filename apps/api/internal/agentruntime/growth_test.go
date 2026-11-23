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
	report, err := coordinateResult("coordinate_growth", json.RawMessage(`[]`), raw)
	if err != nil {
		t.Fatal(err)
	}
	if report.State != "partial_failure" || string(report.Children) != string(raw) {
		t.Fatalf("mixed child outcomes or results were lost: %+v", report)
	}
	got := strings.Join(report.Recommendations, "\n")
	for _, want := range []string{"подтверждённым остатком", "стало меньше", "расхождение", "Часть подзадач"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
}

func TestCoordinateResultDoesNotReportMissingSpecialistResultsAsSuccess(t *testing.T) {
	for _, tc := range []struct {
		name, children, want string
	}{
		{"all failed", `[{"status":"failed","result":null},{"status":"expired","result":null}]`, "failed"},
		{"missing result", `[{"status":"completed","result":null}]`, "failed"},
		{"all completed", `[{"status":"completed","result":[{"metrics":[]}]}]`, "completed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := json.RawMessage(tc.children)
			plan := json.RawMessage(`[{"actions":[]}]`)
			report, err := coordinateResult("coordinate", plan, raw)
			if err != nil || report.State != tc.want || string(report.Plan) != string(plan) || string(report.Children) != string(raw) {
				t.Fatalf("report = %+v, err = %v", report, err)
			}
		})
	}
	if _, err := coordinateResult("coordinate", nil, json.RawMessage(`[]`)); err == nil {
		t.Fatal("missing child projection was accepted")
	}
}

func TestOrderMetricsRequiresCompleteProjection(t *testing.T) {
	for _, data := range []string{
		"", `null`, `{}`,
		`{"count":0,"previous_count":0,"previous_value":0}`,
		`{"count":0,"revenue":null,"previous_count":0,"previous_value":0}`,
	} {
		if metrics, err := orderMetrics([]byte(data)); err == nil || len(metrics) != 0 {
			t.Fatalf("missing order evidence produced metrics %v, err = %v, input %s", metrics, err, data)
		}
	}
	metrics, err := orderMetrics([]byte(`{"count":0,"revenue":0,"previous_count":0,"previous_value":0}`))
	if err != nil || len(metrics) != 4 || metrics[0].Value != 0 {
		t.Fatalf("real zero values were discarded: %v, %v", metrics, err)
	}
}
