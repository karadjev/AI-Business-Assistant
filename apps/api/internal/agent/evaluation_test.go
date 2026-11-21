package agent

import (
	"context"
	_ "embed"
	"encoding/json"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/config"
	"testing"
)

//go:embed testdata/evaluation_v1.json
var evaluation []byte

func TestEvaluationSafetyGuards(t *testing.T) {
	var suite struct {
		Scenarios []struct {
			ID, Name, Message, Expected string
			Quality                     string `json:"natural_language_quality"`
		}
	}
	if e := json.Unmarshal(evaluation, &suite); e != nil {
		t.Fatal(e)
	}
	if len(suite.Scenarios) < 30 {
		t.Fatal("not enough scenarios")
	}
	cfg, _ := config.LoadAgent(func(string) string { return "" })
	for _, s := range suite.Scenarios {
		t.Run(s.ID+"/"+s.Name, func(t *testing.T) {
			if s.Expected == "" || s.Quality != "NOT RUN" {
				t.Fatal("missing expected behavior or invented quality result")
			}
			result, e := (Mock{}).Generate(context.Background(), Request{Messages: []Message{{Role: "system", Content: instructions}, {Role: "user", Content: s.Message}}})
			if e != nil {
				t.Fatal(e)
			}
			for _, c := range result.Calls {
				if _, e := Validate(c.Function.Name, c.Function.Arguments); e != nil {
					t.Fatal("schema", e)
				}
				if c.Function.Name != "search_products" && c.Function.Name != "handoff_to_human" {
					t.Fatal("implicit mutation from free text")
				}
			}
			cfg.Provider = "openai_compatible"
			if _, e := NewProvider(cfg).Generate(context.Background(), Request{}); e == nil {
				t.Fatal("unapproved external invocation")
			}
		})
	}
}
