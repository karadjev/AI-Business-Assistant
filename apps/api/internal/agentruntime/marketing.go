package agentruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"io"

	af "github.com/karadjev/AI-Business-Assistant/apps/api/internal/agentfoundation"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/specialists"
)

type marketingContext struct {
	Mode  string          `json:"mode"`
	Facts *MarketingFacts `json:"facts,omitempty"`
}

type MarketingFacts struct {
	Industry  string `json:"industry"`
	Inventory string `json:"inventory"`
	Demand    string `json:"demand"`
}

type MarketingDraft struct {
	specialists.Draft
	Generation string          `json:"generation"`
	Intent     marketingIntent `json:"intent"`
	Evidence   MarketingFacts  `json:"evidence"`
	Goal       string          `json:"goal"`
	Audience   string          `json:"audience"`
	Channel    string          `json:"channel"`
	Idea       string          `json:"idea"`
	Rationale  string          `json:"rationale"`
	Assumption string          `json:"assumptions"`
	Approval   string          `json:"required_approvals"`
}

func (a *attempt) bindMarketing(registrations []af.Registration) error {
	if a.job.AgentID != "marketing" || a.job.Action != "draft" {
		return nil
	}
	for i := range registrations {
		registration := &registrations[i]
		if registration.Agent.ID != "marketing" {
			continue
		}
		for j, tool := range registration.Tools {
			if tool.Spec().Name == "marketing.draft" {
				registration.Tools[j] = marketingTool{Tool: tool, attempt: a}
				return nil
			}
		}
	}
	return af.ErrDenied
}

type marketingTool struct {
	af.Tool
	attempt *attempt
}

func (t marketingTool) Execute(ctx context.Context, scope af.Scope, input json.RawMessage) (json.RawMessage, error) {
	if scope != t.attempt.scope || t.attempt.job.AgentID != "marketing" || t.attempt.job.Action != "draft" {
		return nil, af.ErrDenied
	}
	input, err := t.Tool.CanonicalInput(scope, input)
	if err != nil {
		return nil, err
	}
	var raw []byte
	if err := t.attempt.service.Pool.QueryRow(ctx, "SELECT platform.agent_marketing_context($1,$2)",
		t.attempt.job.ID, t.attempt.lease).Scan(&raw); err != nil {
		return nil, err
	}
	source, err := decodeMarketingContext(raw)
	if err != nil {
		return nil, err
	}
	if source.Mode == "legacy" {
		return t.Tool.Execute(ctx, scope, input)
	}
	draft, err := generateMarketingDraft(ctx, *source.Facts)
	if err != nil {
		return nil, err
	}
	return json.Marshal(draft)
}

func (t marketingTool) ValidateOutput(scope af.Scope, output json.RawMessage) error {
	if scope != t.attempt.scope {
		return af.ErrDenied
	}
	if err := t.Tool.ValidateOutput(scope, output); err == nil {
		return nil
	}
	var draft MarketingDraft
	if !decodeMarketingJSON(output, &draft) {
		return af.ErrDenied
	}
	expected, err := renderMarketingDraft(draft.Evidence, draft.Intent)
	if err != nil || draft != expected {
		return af.ErrDenied
	}
	return nil
}

func decodeMarketingContext(raw []byte) (marketingContext, error) {
	var source marketingContext
	if !decodeMarketingJSON(raw, &source) {
		return marketingContext{}, af.ErrDenied
	}
	switch source.Mode {
	case "legacy":
		if source.Facts == nil {
			return source, nil
		}
	case "contextual":
		if source.Facts != nil && source.Facts.valid() {
			return source, nil
		}
	}
	return marketingContext{}, af.ErrDenied
}

func decodeMarketingJSON(raw []byte, target any) bool {
	if len(raw) == 0 || len(raw) > 2048 {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target) == nil && decoder.Decode(new(any)) == io.EOF
}
