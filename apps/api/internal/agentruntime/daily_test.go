package agentruntime

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDailyRecommendationsDistinguishMissingResults(t *testing.T) {
	children := []dailyChild{
		{Agent: "sales", Status: "completed", Result: json.RawMessage(`[{"state":"completed","metrics":{"pending_confirmation":2}}]`)},
		{Agent: "customer", Status: "failed", Result: json.RawMessage(`[{"state":"completed","metrics":{"awaiting_staff":99}}]`)},
		{Agent: "finance", Status: "completed", Result: json.RawMessage(`[{"state":"completed","metrics":{"ai_unsettled_count":null}}]`)},
	}
	state, notes := dailyRecommendations(children)
	if state != "partial_failure" || len(notes) != 2 || !strings.Contains(notes[1], ": 2.") {
		t.Fatalf("state=%q notes=%q", state, notes)
	}
}
