package agentfoundation

import (
	"encoding/json"
	"regexp"
)

var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)

func valid(s string) bool { return identifier.MatchString(s) }
func validScope(s Scope) bool {
	return valid(s.TenantID) && valid(s.PrincipalID) && valid(s.ConversationID)
}
func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
func unique(xs []string) bool {
	if len(xs) > 128 {
		return false
	}
	seen := map[string]bool{}
	for _, x := range xs {
		if !valid(x) || seen[x] {
			return false
		}
		seen[x] = true
	}
	return true
}

type registryKey struct{ tenant, agent string }
type entry struct {
	agent Agent
	tools map[string]registeredTool
}
type registeredTool struct {
	tool Tool
	spec ToolSpec
}

// Registry is immutable after construction and safe for concurrent readers.
// Callback implementations must themselves be concurrency safe.
type Registry struct{ entries map[registryKey]entry }

func NewRegistry(rs []Registration) (*Registry, error) {
	r := &Registry{entries: make(map[registryKey]entry)}
	for _, reg := range rs {
		a := reg.Agent
		if !valid(reg.TenantID) || !valid(a.ID) || !valid(a.Version) || a.Planner == nil || !unique(a.Tools) || !unique(a.ContextKeys) || a.PlanningCostUnits < 0 {
			return nil, ErrInvalid
		}
		if len(a.Name) > 128 || len(a.Description) > 2048 || !unique(a.Capabilities) || !unique(a.Permissions) || !unique(a.SupportedBusinessTypes) || !unique(a.SupportedIndustryProfiles) || (a.Status != "" && a.Status != AgentEnabled && a.Status != AgentDisabled) || !nonnegativeLimits(a.ExecutionLimits) {
			return nil, ErrInvalid
		}
		if len(a.Configuration) > 4096 || (len(a.Configuration) > 0 && !json.Valid(a.Configuration)) {
			return nil, ErrInvalid
		}
		k := registryKey{reg.TenantID, a.ID}
		if _, ok := r.entries[k]; ok {
			return nil, ErrInvalid
		}
		e := entry{agent: a, tools: make(map[string]registeredTool)}
		e.agent = cloneAgent(a)
		for _, t := range reg.Tools {
			if t == nil {
				return nil, ErrInvalid
			}
			s := t.Spec()
			if !valid(s.Name) || !valid(s.Version) || !valid(s.Permission) || s.CostUnits < 0 || !validEffect(s) || !contains(a.Tools, s.Name) {
				return nil, ErrInvalid
			}
			if _, ok := e.tools[s.Name]; ok {
				return nil, ErrInvalid
			}
			if len(a.Permissions) > 0 && !contains(a.Permissions, s.Permission) {
				return nil, ErrDenied
			}
			if !contains(e.agent.Permissions, s.Permission) {
				e.agent.Permissions = append(e.agent.Permissions, s.Permission)
			}
			e.tools[s.Name] = registeredTool{t, s}
		}
		if len(e.tools) != len(a.Tools) {
			return nil, ErrInvalid
		}
		r.entries[k] = e
	}
	return r, nil
}

func cloneAgent(a Agent) Agent {
	a.Tools = append([]string(nil), a.Tools...)
	a.ContextKeys = append([]string(nil), a.ContextKeys...)
	a.Capabilities = append([]string(nil), a.Capabilities...)
	a.Permissions = append([]string(nil), a.Permissions...)
	a.SupportedBusinessTypes = append([]string(nil), a.SupportedBusinessTypes...)
	a.SupportedIndustryProfiles = append([]string(nil), a.SupportedIndustryProfiles...)
	a.Configuration = append(json.RawMessage(nil), a.Configuration...)
	return a
}

// Lookup returns a deep copy of descriptor data, never registry maps. Trusted
// Planner and Tool callbacks are referenced, not cloned; they must be immutable.
func (r *Registry) Lookup(tenantID, agentID string) (Agent, bool) {
	if r == nil {
		return Agent{}, false
	}
	e, ok := r.entries[registryKey{tenantID, agentID}]
	if !ok {
		return Agent{}, false
	}
	return cloneAgent(e.agent), true
}
func nonnegativeLimits(l Limits) bool {
	return l.MaxSteps >= 0 && l.MaxContextBytes >= 0 && l.MaxResultBytes >= 0 && l.MaxUnits >= 0 && l.Timeout >= 0
}

// Select is tenant-local deterministic capability routing, not authorization.
// Multiple compiled matches fail closed even if one happens to be enabled in a
// mutable configuration. No priority, fallback tenant, or extra grants are implied.
func (r *Registry) Select(tenantID, capability string) (Agent, error) {
	if r == nil || !valid(tenantID) || !valid(capability) {
		return Agent{}, ErrInvalid
	}
	var selected Agent
	found := false
	for key, e := range r.entries {
		if key.tenant != tenantID || e.agent.Status == AgentDisabled || !contains(e.agent.Capabilities, capability) {
			continue
		}
		if found {
			return Agent{}, ErrAmbiguous
		}
		selected = e.agent
		found = true
	}
	if !found {
		return Agent{}, ErrDenied
	}
	return cloneAgent(selected), nil
}

func validEffect(s ToolSpec) bool {
	switch s.Kind {
	case "", ActionRead, ActionCreate, ActionUpdate, ActionExecute:
		return true
	case ActionExternalCommunication, ActionFinancial, ActionCriticalSettings, ActionIrreversible:
		return s.RequiresApproval
	default:
		return false
	}
}
