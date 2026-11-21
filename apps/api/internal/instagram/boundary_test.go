package instagram

import (
	"encoding/json"
	"strings"
	"testing"
)

// The shared repository stores numeric external identities with at most 20
// digits. Reject an incompatible event before enqueue so it cannot abort valid
// siblings in the same authenticated webhook transaction.
func TestNormalizeRejectsOversizedSenderWithoutPoisoningBatch(t *testing.T) {
	eventFrom := func(raw []byte) any {
		t.Helper()
		var value map[string]any
		if err := json.Unmarshal(raw, &value); err != nil {
			t.Fatal(err)
		}
		return value["entry"].([]any)[0].(map[string]any)["messaging"].([]any)[0]
	}
	raw, err := json.Marshal(map[string]any{"object": "instagram", "entry": []any{map[string]any{"id": "100", "messaging": []any{eventFrom(fixture("100", strings.Repeat("9", 21), "oversized", testNow)), eventFrom(fixture("100", "200", "valid", testNow))}}}})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := Normalize(binding(), raw, testNow)
	if err != nil || batch.Rejected != 1 || len(batch.Messages) != 1 || batch.Messages[0].ExternalMessageID != "valid" {
		t.Fatalf("incompatible identity poisoned batch: accepted=%d rejected=%d err=%v", len(batch.Messages), batch.Rejected, err)
	}
	boundary, err := Normalize(binding(), fixture("100", strings.Repeat("9", 20), "boundary", testNow), testNow)
	if err != nil || len(boundary.Messages) != 1 {
		t.Fatal("valid repository boundary rejected", err)
	}
}
