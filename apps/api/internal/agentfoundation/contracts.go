// Package agentfoundation provides offline, tenant-scoped orchestration boundaries.
// It supplies no LLM, storage, network, shell, SQL, or runtime wiring. Scope must
// come from authenticated backend state, never from tool arguments.
package agentfoundation

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrAmbiguous = errors.New("agentfoundation: ambiguous capability")
	ErrInvalid   = errors.New("agentfoundation: invalid contract")
	ErrDenied    = errors.New("agentfoundation: denied")
	ErrBudget    = errors.New("agentfoundation: budget exhausted or unknown")
	ErrDuplicate = errors.New("agentfoundation: task already claimed")
	ErrCycle     = errors.New("agentfoundation: repeated action")
	ErrApproval  = errors.New("agentfoundation: approval required or invalid")
	ErrLimit     = errors.New("agentfoundation: limit exceeded")
)

type Scope struct{ TenantID, PrincipalID, ConversationID string }
type Task struct {
	ID, AgentID, CorrelationID string
	Scope                      Scope
}
type Configuration struct {
	TenantID, AgentID, Version string
	Enabled                    bool
	Tools, ContextKeys         []string
}
type ConfigurationStore interface {
	Load(context.Context, Scope, string) (Configuration, error)
}
type PermissionChecker interface {
	Check(context.Context, Scope, string) error
}

// Load must retrieve only the requested keys for the exact authenticated scope,
// minimizing data at source. The orchestrator rejects extra keys and oversize data.
type ContextProvider interface {
	Load(context.Context, Scope, []string, int) (ContextBundle, error)
}
type ContextBundle struct {
	Scope  Scope
	Values map[string]json.RawMessage
}
type Snapshot struct {
	Context        ContextBundle
	Results        []json.RawMessage
	RemainingUnits int64
}
type Decision struct {
	Done       bool
	Tool       string
	Input      json.RawMessage
	ApprovalID string
}

// Planner is a trusted backend adapter, not a second LLM or an authorization source.
// All callbacks must honor cancellation. No goroutine can forcibly cancel effects.
type Planner interface {
	Next(context.Context, Scope, Snapshot) (Decision, error)
}
type AgentStatus string

const (
	AgentEnabled  AgentStatus = "enabled"
	AgentDisabled AgentStatus = "disabled"
)

type Agent struct {
	ID, Version                                                                  string
	Name, Description                                                            string
	Capabilities, Permissions, SupportedBusinessTypes, SupportedIndustryProfiles []string
	ExecutionLimits                                                              Limits
	Status                                                                       AgentStatus
	// Configuration is bounded descriptive metadata, never authority or instructions.
	Configuration      json.RawMessage
	Tools, ContextKeys []string
	Planner            Planner
	PlanningCostUnits  int64
	PlanningCostKnown  bool
}

// ActionKind describes the actual backend effect, never a model-supplied label.
// Empty means read for trusted legacy registrations. Create/update/execute still
// require domain authorization/consent; the four high-effect kinds mandate a
// foundation approval as an additional gate. Draft creation is not publication.
type ActionKind string

const (
	ActionRead                  ActionKind = "read"
	ActionCreate                ActionKind = "create"
	ActionUpdate                ActionKind = "update"
	ActionExecute               ActionKind = "execute"
	ActionExternalCommunication ActionKind = "external_communication"
	ActionFinancial             ActionKind = "financial"
	ActionCriticalSettings      ActionKind = "critical_settings"
	ActionIrreversible          ActionKind = "irreversible"
)

type ToolSpec struct {
	Kind                      ActionKind
	Name, Version, Permission string
	RequiresApproval          bool
	CostUnits                 int64
	CostKnown                 bool
}

// Tool implementations are trusted compiled adapters. Input/output validators
// must check resource ownership against Scope, not merely compare tenant strings.
// Execute must recheck domain consent, source revision, freshness and admission
// atomically with mutations. A foundation approval cannot replace these checks.
type Tool interface {
	Spec() ToolSpec
	CanonicalInput(Scope, json.RawMessage) (json.RawMessage, error)
	ValidateOutput(Scope, json.RawMessage) error
	Execute(context.Context, Scope, json.RawMessage) (json.RawMessage, error)
}
type Registration struct {
	TenantID string
	Agent    Agent
	Tools    []Tool
}
type Limits struct {
	// MaxUnits are abstract offline work units, never money. On Agent.ExecutionLimits,
	// MaxUnitsSet distinguishes explicit deny-zero from zero/inherit. Global MaxUnits
	// always applies, including zero; unknown costs are rejected regardless.
	MaxUnitsSet                               bool
	MaxSteps, MaxContextBytes, MaxResultBytes int
	MaxUnits                                  int64
	Timeout                                   time.Duration
}
type Result struct {
	Results   []json.RawMessage
	Steps     int
	UsedUnits int64
}

type ApprovalBinding struct {
	Scope                                                                            Scope
	TaskID, AgentID, Action, ParamsHash, Version, AgentVersion, ConfigurationVersion string
}
type Approval struct {
	ID        string
	Binding   ApprovalBinding
	ExpiresAt time.Time
}

// ApprovalStore must be durable. Consume MUST atomically compare the entire
// approval (including scope, binding, expiry), ensure now < expiry, and mark used
// exactly once. Revoked/changed/consumed records fail closed. Issuance is outside
// the agent API. Expiration and revocation must also be checked in the domain
// transaction where required. Consumed approvals are never automatically restored.
type ApprovalStore interface {
	Load(context.Context, Scope, string) (Approval, error)
	Consume(context.Context, Approval, time.Time) error
}
type Status string

const (
	StatusStarted   Status = "started"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
)

// Claim must atomically reserve tenant+task ID durably across processes, binding
// principal/conversation/agent too. Existing claims (including failed/unknown)
// return false; never automatically release after timeout or an uncertain effect.
// Finish must preserve the claim even on persistence error. No retries here.
type IdempotencyStore interface {
	Claim(context.Context, Task) (bool, error)
	Finish(context.Context, Task, Status) error
}

// AuditEvent deliberately has no free text, parameters, results, or error field.
// Identifiers are SHA-256 hashes to avoid accidental PII/secret logging.
type ApprovalStatus string

const (
	ApprovalNone     ApprovalStatus = "none"
	ApprovalRequired ApprovalStatus = "required"
	ApprovalConsumed ApprovalStatus = "consumed"
)

type AuditEvent struct {
	TenantHash, PrincipalHash, ConversationHash, TaskHash, AgentHash, ActionHash, CorrelationHash string
	// ParamsHash is the canonical typed parameter hash on tool events, empty on task events.
	ParamsHash     string
	FailureCode    FailureCode
	ApprovalStatus ApprovalStatus
	Status         Status
	Step           int
}
type AuditSink interface {
	Record(context.Context, AuditEvent) error
}

// FailureCode is a bounded audit classification. Backend error text may contain sensitive data.
type FailureCode string

const (
	FailureNone      FailureCode = ""
	FailureDenied    FailureCode = "denied"
	FailureInvalid   FailureCode = "invalid"
	FailureTimeout   FailureCode = "timeout"
	FailureCanceled  FailureCode = "canceled"
	FailureApproval  FailureCode = "approval"
	FailureLimit     FailureCode = "limit"
	FailureExecution FailureCode = "execution"
)
