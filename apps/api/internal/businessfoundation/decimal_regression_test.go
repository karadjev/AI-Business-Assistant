package businessfoundation

import "testing"

func TestExactDecimalPhysicalAttributes(t *testing.T) {
	defs := []AttributeDefinition{{Key: "weight", Kind: Decimal, Unit: "kg", Min: "0.001", Max: "1000000.000"}}
	for _, value := range []string{"0.001", "12.500", "1000000.000"} {
		if err := ValidateAttributes(defs, map[string]AttributeValue{"weight": {Kind: Decimal, Text: value, Unit: "kg"}}); err != nil {
			t.Fatalf("valid exact decimal %q rejected: %v", value, err)
		}
	}
	for _, value := range []string{`12\x500`, "1e3", "0.000", "1000000.001", "NaN", "12,5"} {
		if err := ValidateAttributes(defs, map[string]AttributeValue{"weight": {Kind: Decimal, Text: value, Unit: "kg"}}); err == nil {
			t.Fatalf("malformed/out-of-bounds decimal %q accepted", value)
		}
	}
}
