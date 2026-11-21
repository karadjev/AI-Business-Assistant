package specialists

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/agent"
	af "github.com/karadjev/AI-Business-Assistant/apps/api/internal/agentfoundation"
)

type executor interface {
	Execute(context.Context, string, string) (agent.ToolResult, error)
}

// RunnerBinding can only be constructed from an existing Runner after checking
// tenant and conversation. Resolver must authenticate principal ownership and
// return a per-execution Runner: Runner.Cards is mutable and cannot be shared.
type RunnerBinding struct {
	scope  af.Scope
	runner executor
}
type RunnerResolver interface {
	Resolve(context.Context, af.Scope) (RunnerBinding, error)
}

func BindRunner(s af.Scope, r *agent.Runner) (RunnerBinding, error) {
	if r == nil || s.TenantID == "" || s.PrincipalID == "" || s.ConversationID == "" || r.Tenant.OrganizationID != s.TenantID || r.ConversationID != s.ConversationID || (r.Tenant.Session.User.ID != "" && r.Tenant.Session.User.ID != s.PrincipalID) {
		return RunnerBinding{}, af.ErrDenied
	}
	return RunnerBinding{s, r}, nil
}

type bridgeResult struct {
	ScopeHash  string `json:"scope_hash"`
	ResultJSON string `json:"result_json"`
}

// Scope identifiers are already validated by the JSON tool before this is used.
func bridgeScopeHash(s af.Scope) string {
	sum := sha256.Sum256([]byte(s.TenantID + "\x00" + s.PrincipalID + "\x00" + s.ConversationID))
	return hex.EncodeToString(sum[:])
}

func bridgeArgs(in Request) (string, error) {
	var selected Request
	var args any
	switch in.Action {
	case "search_products":
		if !bounded(in.Query, 800) || in.Limit < 1 || in.Limit > 3 {
			return "", af.ErrInvalid
		}
		selected = Request{Action: in.Action, Query: in.Query, Limit: in.Limit}
		args = struct {
			Query string `json:"query"`
			Limit int    `json:"limit"`
		}{in.Query, in.Limit}
	case "get_product_details":
		selected = Request{Action: in.Action, ProductID: in.ProductID}
		args = struct {
			ID string `json:"product_id"`
		}{in.ProductID}
	case "get_product_price":
		selected = Request{Action: in.Action, VariantID: in.VariantID}
		args = struct {
			ID string `json:"variant_id"`
		}{in.VariantID}
	case "check_inventory":
		selected = Request{Action: in.Action, VariantID: in.VariantID, Quantity: in.Quantity}
		args = struct {
			ID       string `json:"variant_id"`
			Quantity int    `json:"quantity"`
		}{in.VariantID, in.Quantity}
	case "get_order_status":
		selected = Request{Action: in.Action, OrderID: in.OrderID}
		args = struct {
			ID string `json:"order_id"`
		}{in.OrderID}
	case "handoff_to_human":
		selected = Request{Action: in.Action, Reason: in.Reason}
		args = struct {
			Reason string `json:"reason"`
		}{in.Reason}
	default:
		return "", af.ErrDenied
	}
	if selected != in {
		return "", af.ErrDenied
	}
	raw, err := json.Marshal(args)
	if err != nil {
		return "", af.ErrInvalid
	}
	if _, err = agent.Validate(in.Action, string(raw)); err != nil {
		return "", af.ErrInvalid
	}
	return string(raw), nil
}
func newBridgeTools(id string, d Dependencies) ([]af.Tool, error) {
	actions := []string{"search_products", "get_product_details", "get_product_price", "check_inventory"}
	if id == "customer" || id == "sales" {
		actions = append(actions, "get_order_status", "handoff_to_human")
	}
	tools := make([]af.Tool, 0, len(actions))
	for _, action := range actions {
		tool, err := newBridgeTool(id, action, d)
		if err != nil {
			return nil, err
		}
		tools = append(tools, tool)
	}
	return tools, nil
}

func newBridgeTool(id, action string, d Dependencies) (af.Tool, error) {
	name := id + "." + action
	sp := spec(name, name)
	if action == "handoff_to_human" {
		sp.Kind = af.ActionUpdate
	}
	sp.CostKnown = d.BridgeCostKnown
	sp.CostUnits = d.BridgeCostUnits
	return af.NewJSONTool(sp, func(_ af.Scope, in Request) error {
		if in.Action != action {
			return af.ErrDenied
		}
		_, err := bridgeArgs(in)
		return err
	}, func(s af.Scope, out bridgeResult) error {
		if bridgeScopeHash(s) != out.ScopeHash || len(out.ResultJSON) > 32768 || !json.Valid([]byte(out.ResultJSON)) {
			return af.ErrDenied
		}
		return nil
	}, func(ctx context.Context, s af.Scope, in Request) (bridgeResult, error) {
		if d.Runners == nil {
			return bridgeResult{}, ErrUnavailable
		}
		args, err := bridgeArgs(in)
		if err != nil {
			return bridgeResult{}, err
		}
		b, err := d.Runners.Resolve(ctx, s)
		if err != nil {
			return bridgeResult{}, err
		}
		if b.scope != s || b.runner == nil {
			return bridgeResult{}, af.ErrDenied
		}
		// Defense in depth against a resolver reusing or mutating a bound Runner.
		if r, ok := b.runner.(*agent.Runner); ok && (r.Tenant.OrganizationID != s.TenantID || r.ConversationID != s.ConversationID || (r.Tenant.Session.User.ID != "" && r.Tenant.Session.User.ID != s.PrincipalID)) {
			return bridgeResult{}, af.ErrDenied
		}
		result, err := b.runner.Execute(ctx, action, args)
		if err != nil {
			return bridgeResult{}, err
		}
		raw, err := json.Marshal(result)
		if err != nil {
			return bridgeResult{}, af.ErrInvalid
		}
		return bridgeResult{bridgeScopeHash(s), string(raw)}, nil
	})
}
