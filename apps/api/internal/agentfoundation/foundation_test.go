package agentfoundation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type plannerFunc func(context.Context, Scope, Snapshot) (Decision, error)

func (f plannerFunc) Next(ctx context.Context, s Scope, b Snapshot) (Decision, error) {
	return f(ctx, s, b)
}

type configFunc func(context.Context, Scope, string) (Configuration, error)

func (f configFunc) Load(c context.Context, s Scope, a string) (Configuration, error) {
	return f(c, s, a)
}

type permissionFunc func(context.Context, Scope, string) error

func (f permissionFunc) Check(c context.Context, s Scope, p string) error { return f(c, s, p) }

type contextFunc func(context.Context, Scope, []string, int) (ContextBundle, error)

func (f contextFunc) Load(c context.Context, s Scope, k []string, n int) (ContextBundle, error) {
	return f(c, s, k, n)
}

type auditFunc func(context.Context, AuditEvent) error

func (f auditFunc) Record(c context.Context, e AuditEvent) error { return f(c, e) }

// Memory storage deliberately exists ONLY in this test file.
type taskMemory struct {
	mu       sync.Mutex
	claimed  map[string]Task
	finishes int
}

func (m *taskMemory) Claim(_ context.Context, t Task) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := t.Scope.TenantID + "/" + t.ID
	if _, ok := m.claimed[k]; ok {
		return false, nil
	}
	m.claimed[k] = t
	return true, nil
}
func (m *taskMemory) Finish(_ context.Context, _ Task, _ Status) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.finishes++
	return nil
}

type approvalMemory struct {
	mu        sync.Mutex
	record    Approval
	used      bool
	afterLoad func()
}

func (m *approvalMemory) Load(_ context.Context, s Scope, id string) (Approval, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.record.ID != id || m.record.Binding.Scope != s {
		return Approval{}, ErrApproval
	}
	a := m.record
	if m.afterLoad != nil {
		m.afterLoad()
	}
	return a, nil
}
func (m *approvalMemory) Consume(_ context.Context, a Approval, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.used || a != m.record || !now.Before(a.ExpiresAt) {
		return ErrApproval
	}
	m.used = true
	return nil
}

type input struct {
	ResourceID string `json:"resource_id"`
	Quantity   int    `json:"quantity"`
}
type output struct {
	TenantID string `json:"tenant_id"`
	Quantity int    `json:"quantity"`
}

var testScope = Scope{"tenant-a", "principal-a", "conversation-a"}

func callDecision() Decision {
	return Decision{Tool: "read", Input: json.RawMessage(`{"resource_id":"owned","quantity":1}`)}
}
func setup(t *testing.T, approve bool) (*Orchestrator, Task, *atomic.Int64) {
	t.Helper()
	count := &atomic.Int64{}
	tool, err := NewJSONTool(ToolSpec{Name: "read", Version: "v1", Permission: "read.owned", RequiresApproval: approve, CostKnown: true, CostUnits: 2}, func(s Scope, i input) error {
		if s != testScope || i.ResourceID != "owned" || i.Quantity <= 0 {
			return ErrDenied
		}
		return nil
	}, func(s Scope, o output) error {
		if o.TenantID != s.TenantID {
			return ErrDenied
		}
		return nil
	}, func(_ context.Context, s Scope, i input) (output, error) {
		count.Add(1)
		return output{s.TenantID, i.Quantity}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	planner := plannerFunc(func(_ context.Context, _ Scope, s Snapshot) (Decision, error) {
		if len(s.Results) > 0 {
			return Decision{Done: true}, nil
		}
		d := callDecision()
		if approve {
			d.ApprovalID = "approval-1"
		}
		return d, nil
	})
	r, err := NewRegistry([]Registration{{TenantID: testScope.TenantID, Agent: Agent{ID: "customer", Version: "v1", Tools: []string{"read"}, ContextKeys: []string{"public"}, Capabilities: []string{"customer_help"}, Planner: planner, PlanningCostKnown: true}, Tools: []Tool{tool}}})
	if err != nil {
		t.Fatal(err)
	}
	task := Task{ID: "task-1", AgentID: "customer", Scope: testScope}
	o := &Orchestrator{Registry: r, Configurations: configFunc(func(_ context.Context, s Scope, a string) (Configuration, error) {
		return Configuration{TenantID: s.TenantID, AgentID: a, Version: "cfg1", Enabled: true, Tools: []string{"read"}, ContextKeys: []string{"public"}}, nil
	}), Permissions: permissionFunc(func(context.Context, Scope, string) error { return nil }), Contexts: contextFunc(func(_ context.Context, s Scope, keys []string, _ int) (ContextBundle, error) {
		if len(keys) != 1 || keys[0] != "public" {
			return ContextBundle{}, ErrDenied
		}
		return ContextBundle{Scope: s, Values: map[string]json.RawMessage{"public": json.RawMessage(`{"name":"synthetic"}`)}}, nil
	}), Tasks: &taskMemory{claimed: map[string]Task{}}, Audit: auditFunc(func(_ context.Context, e AuditEvent) error {
		if len(e.TenantHash) != 64 || len(e.TaskHash) != 64 || len(e.ActionHash) != 64 || e.PrincipalHash != hash(testScope.PrincipalID) || e.ConversationHash != hash(testScope.ConversationID) {
			t.Error("audit not hashed")
		}
		return nil
	}), Limits: Limits{MaxSteps: 4, MaxUnits: 10, Timeout: time.Second, MaxContextBytes: 1024, MaxResultBytes: 1024}}
	canonical, err := tool.CanonicalInput(testScope, callDecision().Input)
	if err != nil {
		t.Fatal(err)
	}
	o.Approvals = &approvalMemory{record: Approval{ID: "approval-1", Binding: ApprovalBinding{Scope: testScope, TaskID: task.ID, AgentID: task.AgentID, Action: "read", ParamsHash: ParamsHash(canonical), Version: "v1", AgentVersion: "v1", ConfigurationVersion: "cfg1"}, ExpiresAt: time.Now().Add(time.Minute)}}
	return o, task, count
}
func TestSuccessDuplicateAndTenantIsolation(t *testing.T) {
	o, task, count := setup(t, false)
	result, err := o.Run(context.Background(), task)
	if err != nil || len(result.Results) != 1 || count.Load() != 1 {
		t.Fatalf("result=%+v err=%v calls=%d", result, err, count.Load())
	}
	if _, err = o.Run(context.Background(), task); !errors.Is(err, ErrDuplicate) {
		t.Fatal(err)
	}
	task.Scope.TenantID = "tenant-b"
	if _, err = o.Run(context.Background(), task); !errors.Is(err, ErrDenied) {
		t.Fatal(err)
	}
	if count.Load() != 1 {
		t.Fatal("cross tenant or duplicate executed")
	}
}
func TestConcurrentClaim(t *testing.T) {
	o, task, count := setup(t, false)
	var wg sync.WaitGroup
	var successes atomic.Int64
	for range 20 {
		wg.Go(func() {
			_, err := o.Run(context.Background(), task)
			if err == nil {
				successes.Add(1)
			} else if !errors.Is(err, ErrDuplicate) {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if count.Load() != 1 || successes.Load() != 1 {
		t.Fatal(count.Load(), successes.Load())
	}
}
func TestAuthorizationAndContext(t *testing.T) {
	cases := []struct {
		name   string
		change func(*Orchestrator)
		want   error
	}{
		{"permission", func(o *Orchestrator) {
			o.Permissions = permissionFunc(func(context.Context, Scope, string) error { return ErrDenied })
		}, ErrDenied},
		{"disabled", func(o *Orchestrator) {
			base := o.Configurations
			o.Configurations = configFunc(func(c context.Context, s Scope, a string) (Configuration, error) {
				v, e := base.Load(c, s, a)
				v.Enabled = false
				return v, e
			})
		}, ErrDenied},
		{"wrong tenant config", func(o *Orchestrator) {
			base := o.Configurations
			o.Configurations = configFunc(func(c context.Context, s Scope, a string) (Configuration, error) {
				v, e := base.Load(c, s, a)
				v.TenantID = "tenant-b"
				return v, e
			})
		}, ErrDenied},
		{"tool denied", func(o *Orchestrator) {
			base := o.Configurations
			o.Configurations = configFunc(func(c context.Context, s Scope, a string) (Configuration, error) {
				v, e := base.Load(c, s, a)
				v.Tools = nil
				return v, e
			})
		}, ErrDenied},
		{"cross tenant context", func(o *Orchestrator) {
			o.Contexts = contextFunc(func(context.Context, Scope, []string, int) (ContextBundle, error) {
				return ContextBundle{Scope: Scope{"tenant-b", "p", "c"}}, nil
			})
		}, ErrDenied},
		{"extra context", func(o *Orchestrator) {
			o.Contexts = contextFunc(func(_ context.Context, s Scope, _ []string, _ int) (ContextBundle, error) {
				return ContextBundle{Scope: s, Values: map[string]json.RawMessage{"private": json.RawMessage(`"secret"`)}}, nil
			})
		}, ErrDenied},
		{"oversize context", func(o *Orchestrator) { o.Limits.MaxContextBytes = 1 }, ErrLimit},
		{"budget", func(o *Orchestrator) { o.Limits.MaxUnits = 1 }, ErrBudget},
		{"zero budget", func(o *Orchestrator) { o.Limits.MaxUnits = 0 }, ErrBudget},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o, task, count := setup(t, false)
			tc.change(o)
			_, err := o.Run(context.Background(), task)
			if !errors.Is(err, tc.want) || count.Load() != 0 {
				t.Fatalf("err=%v calls=%d", err, count.Load())
			}
		})
	}
}
func TestApprovalBindingDriftReplayAndRecheck(t *testing.T) {
	changes := map[string]func(*Approval){
		"tenant": func(a *Approval) { a.Binding.Scope.TenantID = "tenant-b" }, "principal": func(a *Approval) { a.Binding.Scope.PrincipalID = "other" }, "conversation": func(a *Approval) { a.Binding.Scope.ConversationID = "other" }, "task": func(a *Approval) { a.Binding.TaskID = "other" }, "agent": func(a *Approval) { a.Binding.AgentID = "other" }, "action": func(a *Approval) { a.Binding.Action = "other" }, "params": func(a *Approval) { a.Binding.ParamsHash = "other" }, "tool version": func(a *Approval) { a.Binding.Version = "v2" }, "agent version": func(a *Approval) { a.Binding.AgentVersion = "v2" }, "config version": func(a *Approval) { a.Binding.ConfigurationVersion = "v2" }, "expired": func(a *Approval) { a.ExpiresAt = time.Now().Add(-time.Second) }}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			o, task, count := setup(t, true)
			change(&o.Approvals.(*approvalMemory).record)
			_, err := o.Run(context.Background(), task)
			if !errors.Is(err, ErrApproval) || count.Load() != 0 {
				t.Fatal(err, count.Load())
			}
		})
	}
	t.Run("replay", func(t *testing.T) {
		o, task, count := setup(t, true)
		if _, err := o.Run(context.Background(), task); err != nil {
			t.Fatal(err)
		}
		o.Tasks = &taskMemory{claimed: map[string]Task{}}
		if _, err := o.Run(context.Background(), task); !errors.Is(err, ErrApproval) || count.Load() != 1 {
			t.Fatal(err, count.Load())
		}
	})
	t.Run("revoke after load", func(t *testing.T) {
		o, task, count := setup(t, true)
		m := o.Approvals.(*approvalMemory)
		m.afterLoad = func() { m.used = true }
		_, err := o.Run(context.Background(), task)
		if !errors.Is(err, ErrApproval) || count.Load() != 0 {
			t.Fatal(err, count.Load())
		}
	})
	t.Run("permission revoked after lookup", func(t *testing.T) {
		o, task, count := setup(t, true)
		var revoked bool
		o.Approvals.(*approvalMemory).afterLoad = func() { revoked = true }
		o.Permissions = permissionFunc(func(context.Context, Scope, string) error {
			if revoked {
				return ErrDenied
			}
			return nil
		})
		_, err := o.Run(context.Background(), task)
		if !errors.Is(err, ErrDenied) || count.Load() != 0 {
			t.Fatal(err, count.Load())
		}
	})
}
func replaceAgent(o *Orchestrator, change func(*Agent)) {
	k := registryKey{testScope.TenantID, "customer"}
	e := o.Registry.entries[k]
	change(&e.agent)
	o.Registry.entries[k] = e
}
func TestCyclesStepsUnknownCostsDeadline(t *testing.T) {
	t.Run("canonical cycle", func(t *testing.T) {
		o, task, count := setup(t, false)
		replaceAgent(o, func(a *Agent) {
			a.Planner = plannerFunc(func(_ context.Context, _ Scope, s Snapshot) (Decision, error) {
				d := callDecision()
				if len(s.Results) > 0 {
					d.Input = json.RawMessage(`{"quantity":1,"resource_id":"owned"}`)
				}
				return d, nil
			})
		})
		_, err := o.Run(context.Background(), task)
		if !errors.Is(err, ErrCycle) || count.Load() != 1 {
			t.Fatal(err, count.Load())
		}
	})
	t.Run("step limit", func(t *testing.T) {
		o, task, _ := setup(t, false)
		o.Limits.MaxSteps = 1
		_, err := o.Run(context.Background(), task)
		if !errors.Is(err, ErrLimit) {
			t.Fatal(err)
		}
	})
	t.Run("unknown planning cost", func(t *testing.T) {
		o, task, count := setup(t, false)
		replaceAgent(o, func(a *Agent) { a.PlanningCostKnown = false })
		_, err := o.Run(context.Background(), task)
		if !errors.Is(err, ErrBudget) || count.Load() != 0 {
			t.Fatal(err)
		}
	})
	t.Run("unknown tool cost", func(t *testing.T) {
		o, task, count := setup(t, false)
		k := registryKey{testScope.TenantID, "customer"}
		e := o.Registry.entries[k]
		tool := e.tools["read"]
		tool.spec.CostKnown = false
		e.tools["read"] = tool
		_, err := o.Run(context.Background(), task)
		if !errors.Is(err, ErrBudget) || count.Load() != 0 {
			t.Fatal(err)
		}
	})
	t.Run("deadline", func(t *testing.T) {
		o, task, count := setup(t, false)
		o.Limits.Timeout = 10 * time.Millisecond
		replaceAgent(o, func(a *Agent) {
			a.Planner = plannerFunc(func(ctx context.Context, _ Scope, _ Snapshot) (Decision, error) {
				<-ctx.Done()
				return callDecision(), nil
			})
		})
		_, err := o.Run(context.Background(), task)
		if !errors.Is(err, context.DeadlineExceeded) || count.Load() != 0 {
			t.Fatal(err)
		}
	})
	t.Run("agent deny zero", func(t *testing.T) {
		o, task, count := setup(t, false)
		replaceAgent(o, func(a *Agent) { a.ExecutionLimits = Limits{MaxUnitsSet: true} })
		_, err := o.Run(context.Background(), task)
		if !errors.Is(err, ErrBudget) || count.Load() != 0 {
			t.Fatal(err)
		}
	})
}
func TestStrictTypedJSON(t *testing.T) {
	o, _, _ := setup(t, false)
	tool := o.Registry.entries[registryKey{testScope.TenantID, "customer"}].tools["read"].tool
	bad := []string{`null`, `[]`, `{"resource_id":"owned","quantity":1,"tenant_id":"tenant-b"}`, `{"resource_id":"owned","resource_id":"owned","quantity":1}`, `{"resource_id":null,"quantity":1}`, `{"Resource_ID":"owned","quantity":1}`, `{"resource_id":"owned","quantity":1} {}`, `{"resource_id":"foreign","quantity":1}`, `{"resource_id":"owned","quantity":0}`, `{"resource_id":"owned","quantity":"1"}`, strings.Repeat("x", MaxJSONBytes+1)}
	for _, raw := range bad {
		if _, err := tool.CanonicalInput(testScope, json.RawMessage(raw)); err == nil {
			t.Errorf("accepted %s", raw[:min(len(raw), 100)])
		}
	}
	if err := tool.ValidateOutput(testScope, json.RawMessage(`{"tenant_id":"tenant-b","quantity":1}`)); !errors.Is(err, ErrDenied) {
		t.Fatal(err)
	}
	if err := tool.ValidateOutput(testScope, json.RawMessage(`{"tenant_id":"tenant-a","quantity":1,"secret":"x"}`)); err == nil {
		t.Fatal("accepted unknown output")
	}
}
func TestRegistryRoutingCopiesAndDisabled(t *testing.T) {
	o, task, _ := setup(t, false)
	a, ok := o.Registry.Lookup(task.Scope.TenantID, task.AgentID)
	if !ok {
		t.Fatal("missing")
	}
	a.Tools[0] = "shell"
	a.ContextKeys[0] = "private"
	a.Capabilities[0] = "escalate"
	selected, err := o.Select(context.Background(), testScope, "customer_help")
	if err != nil || selected.ID != "customer" || selected.Tools[0] != "read" {
		t.Fatal(selected, err)
	}
	if _, err = o.Registry.Select("tenant-b", "customer_help"); !errors.Is(err, ErrDenied) {
		t.Fatal(err)
	}
	e := o.Registry.entries[registryKey{testScope.TenantID, "customer"}]
	other := e
	other.agent = cloneAgent(e.agent)
	other.agent.ID = "sales"
	o.Registry.entries[registryKey{testScope.TenantID, "sales"}] = other
	if _, err = o.Registry.Select(testScope.TenantID, "customer_help"); !errors.Is(err, ErrAmbiguous) {
		t.Fatal(err)
	}
	other.agent.Status = AgentDisabled
	o.Registry.entries[registryKey{testScope.TenantID, "sales"}] = other
	if _, err = o.Registry.Select(testScope.TenantID, "customer_help"); err != nil {
		t.Fatal(err)
	}
	replaceAgent(o, func(a *Agent) { a.Status = AgentDisabled })
	if _, err = o.Run(context.Background(), task); !errors.Is(err, ErrDenied) {
		t.Fatal(err)
	}
}

func TestUnregisteredToolCannotSelfEscalate(t *testing.T) {
	o, task, count := setup(t, false)
	replaceAgent(o, func(a *Agent) {
		a.Planner = plannerFunc(func(context.Context, Scope, Snapshot) (Decision, error) {
			return Decision{Tool: "shell", Input: json.RawMessage(`{"command":"noop"}`)}, nil
		})
	})
	_, err := o.Run(context.Background(), task)
	if !errors.Is(err, ErrDenied) || count.Load() != 0 {
		t.Fatal(err, count.Load())
	}
}
func TestConfigurationRevocationDuringApproval(t *testing.T) {
	o, task, count := setup(t, true)
	revoked := false
	base := o.Configurations
	o.Approvals.(*approvalMemory).afterLoad = func() { revoked = true }
	o.Configurations = configFunc(func(c context.Context, s Scope, a string) (Configuration, error) {
		v, e := base.Load(c, s, a)
		if revoked {
			v.Version = "cfg2"
		}
		return v, e
	})
	_, err := o.Run(context.Background(), task)
	if !errors.Is(err, ErrDenied) || count.Load() != 0 {
		t.Fatal(err, count.Load())
	}
	if o.Approvals.(*approvalMemory).used {
		t.Fatal("approval consumed despite config drift")
	}
}
func TestAuditFailureBlocksEffectAndKeepsClaim(t *testing.T) {
	o, task, count := setup(t, false)
	failure := errors.New("synthetic sink unavailable")
	o.Audit = auditFunc(func(context.Context, AuditEvent) error { return failure })
	_, err := o.Run(context.Background(), task)
	if !errors.Is(err, failure) || count.Load() != 0 {
		t.Fatal(err, count.Load())
	}
	if _, err = o.Run(context.Background(), task); !errors.Is(err, ErrDuplicate) {
		t.Fatal(err)
	}
}
func TestScopeAndRegistryInputValidation(t *testing.T) {
	for _, s := range []Scope{{}, {TenantID: "a", PrincipalID: "p"}, {TenantID: "a", ConversationID: "c"}, {TenantID: "a", PrincipalID: "p", ConversationID: "\n"}} {
		o, task, _ := setup(t, false)
		task.Scope = s
		if _, err := o.Run(context.Background(), task); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	}
	o, _, _ := setup(t, false)
	e := o.Registry.entries[registryKey{testScope.TenantID, "customer"}]
	a := cloneAgent(e.agent)
	a.Configuration = json.RawMessage(`{"enabled":true}`)
	a.SupportedBusinessTypes = []string{"retail"}
	regs := []Registration{{TenantID: testScope.TenantID, Agent: a, Tools: []Tool{e.tools["read"].tool}}}
	r, err := NewRegistry(regs)
	if err != nil {
		t.Fatal(err)
	}
	a.Tools[0] = "shell"
	a.SupportedBusinessTypes[0] = "other"
	a.Configuration[0] = '!'
	copied, _ := r.Lookup(testScope.TenantID, "customer")
	if copied.Tools[0] != "read" || copied.SupportedBusinessTypes[0] != "retail" || !json.Valid(copied.Configuration) {
		t.Fatal("constructor alias")
	}
	copied.Configuration[0] = '!'
	again, _ := r.Lookup(testScope.TenantID, "customer")
	if !json.Valid(again.Configuration) {
		t.Fatal("lookup alias")
	}
	regs[0].Agent = cloneAgent(e.agent)
	regs[0].Agent.Permissions = []string{"admin"}
	if _, err = NewRegistry(regs); !errors.Is(err, ErrDenied) {
		t.Fatal(err)
	}
}
func TestLimitsIntersectAndResultsBound(t *testing.T) {
	a := Limits{MaxSteps: 5, MaxContextBytes: 100, MaxResultBytes: 100, MaxUnits: 10, Timeout: time.Second}
	b := Limits{MaxSteps: 2, MaxContextBytes: 200, MaxResultBytes: 50, MaxUnits: 20, Timeout: 2 * time.Second}
	v := intersect(a, b)
	if v.MaxSteps != 2 || v.MaxContextBytes != 100 || v.MaxResultBytes != 50 || v.MaxUnits != 10 || v.Timeout != time.Second {
		t.Fatal(v)
	}
	if intersect(a, Limits{MaxUnitsSet: true}).MaxUnits != 0 {
		t.Fatal("zero not honored")
	}
	if intersect(a, Limits{}).MaxUnits != 10 {
		t.Fatal("inherit not honored")
	}
	o, task, count := setup(t, false)
	o.Limits.MaxResultBytes = 1
	_, err := o.Run(context.Background(), task)
	if !errors.Is(err, ErrLimit) || count.Load() != 1 {
		t.Fatal(err, count.Load())
	}
	if _, err = o.Run(context.Background(), task); !errors.Is(err, ErrDuplicate) {
		t.Fatal("uncertain effect retried", err)
	}
}
func TestSnapshotCannotMutatePriorResults(t *testing.T) {
	o, task, _ := setup(t, false)
	replaceAgent(o, func(a *Agent) {
		a.Planner = plannerFunc(func(_ context.Context, _ Scope, s Snapshot) (Decision, error) {
			if len(s.Results) == 0 {
				return callDecision(), nil
			}
			s.Results[0][0] = '!'
			return Decision{Done: true}, nil
		})
	})
	result, err := o.Run(context.Background(), task)
	if err != nil || !json.Valid(result.Results[0]) {
		t.Fatal(err, result)
	}
}

type specOverride struct {
	Tool
	descriptor ToolSpec
}

func (t specOverride) Spec() ToolSpec { return t.descriptor }
func TestEffectApprovalInvariant(t *testing.T) {
	inputValidator := func(Scope, input) error { return nil }
	outputValidator := func(Scope, output) error { return nil }
	execute := func(context.Context, Scope, input) (output, error) { return output{}, nil }
	for _, kind := range []ActionKind{ActionExternalCommunication, ActionFinancial, ActionCriticalSettings, ActionIrreversible, ActionKind("unknown")} {
		t.Run(string(kind), func(t *testing.T) {
			sp := ToolSpec{Name: "dangerous", Version: "v1", Permission: "dangerous", Kind: kind, CostKnown: true}
			if _, err := NewJSONTool(sp, inputValidator, outputValidator, execute); !errors.Is(err, ErrInvalid) {
				t.Fatal("constructor accepted", err)
			}
			sp.RequiresApproval = true
			_, err := NewJSONTool(sp, inputValidator, outputValidator, execute)
			if kind == "unknown" {
				if !errors.Is(err, ErrInvalid) {
					t.Fatal(err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			o, _, _ := setup(t, false)
			e := o.Registry.entries[registryKey{testScope.TenantID, "customer"}]
			custom := specOverride{e.tools["read"].tool, e.tools["read"].spec}
			custom.descriptor.Kind = kind
			if _, err := NewRegistry([]Registration{{TenantID: testScope.TenantID, Agent: e.agent, Tools: []Tool{custom}}}); !errors.Is(err, ErrInvalid) {
				t.Fatal("registry accepted unsafe compiled adapter", err)
			}
		})
	}
	for _, kind := range []ActionKind{"", ActionRead, ActionCreate, ActionUpdate, ActionExecute} {
		if _, err := NewJSONTool(ToolSpec{Name: "safe", Version: "v1", Permission: "safe", Kind: kind}, inputValidator, outputValidator, execute); err != nil {
			t.Fatal(kind, err)
		}
	}
}

func TestAuditAttributionApprovalAndCanonicalParams(t *testing.T) {
	for _, requires := range []bool{false, true} {
		o, task, _ := setup(t, requires)
		var events []AuditEvent
		o.Audit = auditFunc(func(_ context.Context, e AuditEvent) error { events = append(events, e); return nil })
		if _, err := o.Run(context.Background(), task); err != nil {
			t.Fatal(err)
		}
		expectedParams := o.Approvals.(*approvalMemory).record.Binding.ParamsHash
		required, consumed, intents := 0, 0, 0
		for _, e := range events {
			if e.PrincipalHash != hash(task.Scope.PrincipalID) || e.ConversationHash != hash(task.Scope.ConversationID) || e.TenantHash != hash(task.Scope.TenantID) {
				t.Fatal("missing hashed initiator", e)
			}
			if e.ActionHash == hash("read") && e.Status != StatusStarted {
				continue // Outcome is asserted by TestCapabilityRunEmitsCorrelatedToolOutcome.
			}
			if e.ActionHash == hash("read") {
				intents++
				if e.ParamsHash != expectedParams {
					t.Fatal("noncanonical audit parameters", e)
				}
				switch e.ApprovalStatus {
				case ApprovalNone:
					if requires {
						t.Fatal("approval omitted")
					}
				case ApprovalRequired:
					required++
				case ApprovalConsumed:
					consumed++
				default:
					t.Fatal("unbounded approval status")
				}
			} else if e.ParamsHash != "" || e.ApprovalStatus != ApprovalNone {
				t.Fatal("task event carries tool metadata")
			}
		}
		if requires && (required != 1 || consumed != 1 || intents != 2) {
			t.Fatal("missing consumed audit", events)
		}
		if !requires && (intents != 1 || required != 0 || consumed != 0) {
			t.Fatal(events)
		}
	}
}

func TestCapabilityRunEmitsCorrelatedToolOutcome(t *testing.T) {
	for _, rejected := range []bool{false, true} {
		o, task, count := setup(t, false)
		task.AgentID = ""
		task.CorrelationID = "synthetic-request"
		if rejected {
			key := registryKey{testScope.TenantID, "customer"}
			entry := o.Registry.entries[key]
			selected := entry.tools["read"]
			selected.tool = failingTool{Tool: selected.tool}
			entry.tools["read"] = selected
		}
		var events []AuditEvent
		o.Audit = auditFunc(func(_ context.Context, event AuditEvent) error {
			events = append(events, event)
			return nil
		})
		result, err := o.RunCapability(context.Background(), task, "customer_help")
		wantStatus, wantFailure := StatusSucceeded, FailureNone
		if rejected {
			wantStatus, wantFailure = StatusFailed, FailureExecution
			if err == nil || len(result.Results) != 0 || count.Load() != 0 {
				t.Fatal("failed execution produced a result")
			}
		} else if err != nil || len(result.Results) != 1 || count.Load() != 1 {
			t.Fatalf("capability execution: %v", err)
		}
		if len(events) != 3 {
			t.Fatalf("expected intent, outcome, task finalization, got %d", len(events))
		}
		first, outcome := events[0], events[1]
		if first.Status != StatusStarted || outcome.ActionHash != first.ActionHash ||
			outcome.ParamsHash != first.ParamsHash || outcome.Status != wantStatus || outcome.FailureCode != wantFailure ||
			events[2].Status != wantStatus || events[2].FailureCode != wantFailure {
			t.Fatal("missing or incorrect execution outcome", events)
		}
		for _, event := range events {
			if event.CorrelationHash != hash(task.Scope.TenantID+"/"+task.CorrelationID) {
				t.Fatal("missing tenant-scoped correlation")
			}
		}
		encoded, err := json.Marshal(events)
		if err != nil || strings.Contains(string(encoded), "synthetic-private-error") || strings.Contains(string(encoded), task.CorrelationID) {
			t.Fatal("raw audit data leaked")
		}
	}
}

type failingTool struct{ Tool }

func (f failingTool) Execute(context.Context, Scope, json.RawMessage) (json.RawMessage, error) {
	return nil, errors.New("synthetic-private-error")
}

func TestCapabilityRunBoundsSelectionTime(t *testing.T) {
	o, task, count := setup(t, false)
	task.AgentID = ""
	o.Limits.Timeout = 20 * time.Millisecond
	o.Configurations = configFunc(func(ctx context.Context, _ Scope, _ string) (Configuration, error) {
		select {
		case <-ctx.Done():
			return Configuration{}, ctx.Err()
		case <-time.After(time.Second):
			return Configuration{}, errors.New("selection exceeded its deadline")
		}
	})
	if _, err := o.RunCapability(context.Background(), task, "customer_help"); !errors.Is(err, context.DeadlineExceeded) || count.Load() != 0 {
		t.Fatalf("selection deadline did not stop work: %v", err)
	}
}
