package agentfoundation

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

type Orchestrator struct {
	Registry       *Registry
	Configurations ConfigurationStore
	Permissions    PermissionChecker
	Contexts       ContextProvider
	Approvals      ApprovalStore
	Tasks          IdempotencyStore
	Audit          AuditSink
	Limits         Limits
}

func intersect(a, b Limits) Limits {
	minInt := func(x, y int) int {
		if y > 0 && y < x {
			return y
		}
		return x
	}
	a.MaxSteps = minInt(a.MaxSteps, b.MaxSteps)
	a.MaxContextBytes = minInt(a.MaxContextBytes, b.MaxContextBytes)
	a.MaxResultBytes = minInt(a.MaxResultBytes, b.MaxResultBytes)
	if (b.MaxUnitsSet || b.MaxUnits > 0) && b.MaxUnits < a.MaxUnits {
		a.MaxUnits = b.MaxUnits
	}
	if b.Timeout > 0 && b.Timeout < a.Timeout {
		a.Timeout = b.Timeout
	}
	return a
}
func (o *Orchestrator) configuration(ctx context.Context, t Task, e entry) (Configuration, error) {
	c, err := o.Configurations.Load(ctx, t.Scope, t.AgentID)
	if err != nil {
		return c, err
	}
	if !c.Enabled || c.TenantID != t.Scope.TenantID || c.AgentID != t.AgentID || !valid(c.Version) || !unique(c.Tools) || !unique(c.ContextKeys) {
		return c, ErrDenied
	}
	for _, name := range c.Tools {
		if !contains(e.agent.Tools, name) {
			return c, ErrDenied
		}
	}
	for _, key := range c.ContextKeys {
		if !contains(e.agent.ContextKeys, key) {
			return c, ErrDenied
		}
	}
	c.Tools = append([]string(nil), c.Tools...)
	c.ContextKeys = append([]string(nil), c.ContextKeys...)
	return c, nil
}
func cloneBundle(b ContextBundle) ContextBundle {
	out := ContextBundle{Scope: b.Scope, Values: make(map[string]json.RawMessage, len(b.Values))}
	for k, v := range b.Values {
		out.Values[k] = append(json.RawMessage(nil), v...)
	}
	return out
}
func cloneResults(rs []json.RawMessage) []json.RawMessage {
	out := make([]json.RawMessage, len(rs))
	for i, v := range rs {
		out[i] = append(json.RawMessage(nil), v...)
	}
	return out
}
func (o *Orchestrator) loadContext(ctx context.Context, t Task, c Configuration, max int) (ContextBundle, error) {
	b, err := o.Contexts.Load(ctx, t.Scope, append([]string(nil), c.ContextKeys...), max)
	if err != nil {
		return ContextBundle{}, err
	}
	if b.Scope != t.Scope {
		return ContextBundle{}, ErrDenied
	}
	size := 0
	for k, v := range b.Values {
		if !contains(c.ContextKeys, k) {
			return ContextBundle{}, ErrDenied
		}
		if !json.Valid(v) {
			return ContextBundle{}, ErrInvalid
		}
		size += len(k) + len(v)
		if size > max {
			return ContextBundle{}, ErrLimit
		}
	}
	return cloneBundle(b), nil
}
func auditFailure(err error) FailureCode {
	switch {
	case err == nil:
		return FailureNone
	case errors.Is(err, context.DeadlineExceeded):
		return FailureTimeout
	case errors.Is(err, context.Canceled):
		return FailureCanceled
	case errors.Is(err, ErrDenied):
		return FailureDenied
	case errors.Is(err, ErrInvalid):
		return FailureInvalid
	case errors.Is(err, ErrApproval):
		return FailureApproval
	case errors.Is(err, ErrLimit), errors.Is(err, ErrBudget), errors.Is(err, ErrCycle):
		return FailureLimit
	default:
		return FailureExecution
	}
}

func (o *Orchestrator) record(ctx context.Context, t Task, action string, status Status, step int, paramsHash string, approvalStatus ApprovalStatus, failure error) error {
	correlation := t.CorrelationID
	if correlation == "" {
		correlation = t.ID
	}
	return o.Audit.Record(ctx, AuditEvent{
		TenantHash: hash(t.Scope.TenantID), PrincipalHash: hash(t.Scope.PrincipalID),
		ConversationHash: hash(t.Scope.ConversationID), TaskHash: hash(t.ID),
		AgentHash: hash(t.AgentID), ActionHash: hash(action),
		CorrelationHash: hash(t.Scope.TenantID + "/" + correlation),
		ParamsHash:      paramsHash, ApprovalStatus: approvalStatus, Status: status,
		Step: step, FailureCode: auditFailure(failure),
	})
}

// Run makes no retry or delegation decision. Any repeated canonical action in a
// task is a cycle. Callbacks must honor deadlines and must not spawn background
// effects; domain mutations still own atomic authorization/financial gates.
func (o *Orchestrator) Run(ctx context.Context, t Task) (result Result, err error) {
	if o == nil || o.Registry == nil || o.Configurations == nil || o.Permissions == nil || o.Contexts == nil || o.Tasks == nil || o.Audit == nil || !validScope(t.Scope) || !valid(t.ID) || !valid(t.AgentID) || (t.CorrelationID != "" && !valid(t.CorrelationID)) {
		return result, ErrInvalid
	}
	l := o.Limits
	if l.MaxSteps <= 0 || l.MaxSteps > 1024 || l.MaxContextBytes <= 0 || l.MaxContextBytes > MaxJSONBytes || l.MaxResultBytes <= 0 || l.MaxResultBytes > MaxJSONBytes || l.MaxUnits < 0 || l.Timeout <= 0 {
		return result, ErrInvalid
	}
	e, ok := o.Registry.entries[registryKey{t.Scope.TenantID, t.AgentID}]
	if !ok || e.agent.Status == AgentDisabled {
		return result, ErrDenied
	}
	l = intersect(l, e.agent.ExecutionLimits)
	ctx, cancel := context.WithTimeout(ctx, l.Timeout)
	defer cancel()
	if err = ctx.Err(); err != nil {
		return result, err
	}
	if _, err = o.configuration(ctx, t, e); err != nil {
		return result, err
	}
	claimed, err := o.Tasks.Claim(ctx, t)
	if err != nil {
		return result, err
	}
	if !claimed {
		return result, ErrDuplicate
	}
	// Bounded detached finalization preserves failure evidence after cancellation.
	// Stores must keep claims even if Finish/Audit fails; uncertain effects stay claimed.
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()
		status := StatusSucceeded
		if err != nil {
			status = StatusFailed
		}
		err = errors.Join(err, o.Tasks.Finish(cleanup, t, status), o.record(cleanup, t, "task", status, result.Steps, "", ApprovalNone, err))
	}()
	seen := map[string]bool{}
	charge := func(known bool, units int64) error {
		if !known || units < 0 || units > l.MaxUnits-result.UsedUnits {
			return ErrBudget
		}
		result.UsedUnits += units
		return nil
	}
	for step := 0; step < l.MaxSteps; step++ {
		if err = ctx.Err(); err != nil {
			return result, err
		}
		c, loadErr := o.configuration(ctx, t, e)
		if loadErr != nil {
			return result, loadErr
		}
		bundle, loadErr := o.loadContext(ctx, t, c, l.MaxContextBytes)
		if loadErr != nil {
			return result, loadErr
		}
		if err = charge(e.agent.PlanningCostKnown, e.agent.PlanningCostUnits); err != nil {
			return result, err
		}
		decision, nextErr := e.agent.Planner.Next(ctx, t.Scope, Snapshot{Context: bundle, Results: cloneResults(result.Results), RemainingUnits: l.MaxUnits - result.UsedUnits})
		result.Steps++
		if nextErr != nil {
			return result, nextErr
		}
		if err = ctx.Err(); err != nil {
			return result, err
		}
		if decision.Done {
			if decision.Tool != "" || len(decision.Input) > 0 || decision.ApprovalID != "" {
				return result, ErrInvalid
			}
			return result, nil
		}
		tool, exists := e.tools[decision.Tool]
		if !exists || !contains(e.agent.Permissions, tool.spec.Permission) {
			return result, ErrDenied
		}
		// Re-read configuration after planning. Planner/context snapshots confer no grant.
		c, loadErr = o.configuration(ctx, t, e)
		if loadErr != nil {
			return result, loadErr
		}
		if !contains(c.Tools, decision.Tool) {
			return result, ErrDenied
		}
		if err = o.Permissions.Check(ctx, t.Scope, tool.spec.Permission); err != nil {
			return result, err
		}
		canonical, canonErr := tool.tool.CanonicalInput(t.Scope, decision.Input)
		if canonErr != nil {
			return result, canonErr
		}
		if !validJSONObject(canonical) {
			return result, ErrInvalid
		}
		key := tool.spec.Name + ":" + ParamsHash(canonical)
		if seen[key] {
			return result, ErrCycle
		}
		seen[key] = true
		if err = charge(tool.spec.CostKnown, tool.spec.CostUnits); err != nil {
			return result, err
		}
		var approval Approval
		if tool.spec.RequiresApproval {
			if o.Approvals == nil || !valid(decision.ApprovalID) {
				return result, ErrApproval
			}
			approval, err = o.Approvals.Load(ctx, t.Scope, decision.ApprovalID)
			if err != nil {
				return result, err
			}
			expected := ApprovalBinding{Scope: t.Scope, TaskID: t.ID, AgentID: t.AgentID, Action: tool.spec.Name, ParamsHash: ParamsHash(canonical), Version: tool.spec.Version, AgentVersion: e.agent.Version, ConfigurationVersion: c.Version}
			if approval.ID != decision.ApprovalID || approval.Binding != expected || !time.Now().Before(approval.ExpiresAt) {
				return result, ErrApproval
			}
		} else if decision.ApprovalID != "" {
			return result, ErrInvalid
		}
		// Audit intent before any effect. The sink receives hashes only.
		approvalStatus := ApprovalNone
		if tool.spec.RequiresApproval {
			approvalStatus = ApprovalRequired
		}
		if err = o.record(ctx, t, tool.spec.Name, StatusStarted, result.Steps, ParamsHash(canonical), approvalStatus, nil); err != nil {
			return result, err
		}
		// Revocation/config drift during validation or approval lookup must fail closed.
		current, configErr := o.configuration(ctx, t, e)
		if configErr != nil {
			return result, configErr
		}
		if current.Version != c.Version || !contains(current.Tools, tool.spec.Name) {
			return result, ErrDenied
		}
		if err = o.Permissions.Check(ctx, t.Scope, tool.spec.Permission); err != nil {
			return result, err
		}
		if err = ctx.Err(); err != nil {
			return result, err
		}
		if tool.spec.RequiresApproval {
			if !time.Now().Before(approval.ExpiresAt) {
				return result, ErrApproval
			}
			if err = o.Approvals.Consume(ctx, approval, time.Now()); err != nil {
				return result, err
			}
			if err = o.record(ctx, t, tool.spec.Name, StatusStarted, result.Steps, ParamsHash(canonical), ApprovalConsumed, nil); err != nil {
				return result, err
			}
		}
		if err = ctx.Err(); err != nil {
			return result, err
		}
		if tool.spec.RequiresApproval {
			approvalStatus = ApprovalConsumed
		}
		output, execErr := executeTool(ctx, tool.tool, t.Scope, canonical, result.Results, l.MaxResultBytes)
		status := StatusSucceeded
		if execErr != nil {
			status = StatusFailed
		}
		auditCtx, cancelAudit := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		auditErr := o.record(auditCtx, t, tool.spec.Name, status, result.Steps, ParamsHash(canonical), approvalStatus, execErr)
		cancelAudit()
		if failure := errors.Join(execErr, auditErr); failure != nil {
			return result, failure
		}
		result.Results = append(result.Results, append(json.RawMessage(nil), output...))
	}
	return result, ErrLimit
}

// RunCapability selects from an explicit capability supplied by the backend. It
// uses the same task claim, permissions and limits as an explicit-agent task.
func (o *Orchestrator) RunCapability(ctx context.Context, task Task, capability string) (Result, error) {
	if o == nil || o.Limits.Timeout <= 0 || task.AgentID != "" || !valid(task.ID) ||
		(task.CorrelationID != "" && !valid(task.CorrelationID)) {
		return Result{}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, o.Limits.Timeout)
	defer cancel()
	agent, err := o.Select(ctx, task.Scope, capability)
	if err != nil {
		return Result{}, err
	}
	task.AgentID = agent.ID
	return o.Run(ctx, task)
}

func executeTool(ctx context.Context, tool Tool, scope Scope, canonical json.RawMessage, prior []json.RawMessage, maxBytes int) (json.RawMessage, error) {
	output, err := tool.Execute(ctx, scope, append(json.RawMessage(nil), canonical...))
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	size := len(output)
	for _, previous := range prior {
		size += len(previous)
	}
	if size > maxBytes {
		return nil, ErrLimit
	}
	if !validJSONObject(output) {
		return nil, ErrInvalid
	}
	if err := tool.ValidateOutput(scope, output); err != nil {
		return nil, err
	}
	return output, nil
}

// Select resolves a unique compiled capability then checks current tenant config.
// Selection confers no tool grant; Run performs admission and per-call checks.
func (o *Orchestrator) Select(ctx context.Context, s Scope, capability string) (Agent, error) {
	if o == nil || o.Registry == nil || o.Configurations == nil || !validScope(s) {
		return Agent{}, ErrInvalid
	}
	a, err := o.Registry.Select(s.TenantID, capability)
	if err != nil {
		return Agent{}, err
	}
	e := o.Registry.entries[registryKey{s.TenantID, a.ID}]
	if _, err = o.configuration(ctx, Task{AgentID: a.ID, Scope: s}, e); err != nil {
		return Agent{}, err
	}
	return a, nil
}
