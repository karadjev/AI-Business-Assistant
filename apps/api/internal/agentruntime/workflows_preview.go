package agentruntime

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	af "github.com/karadjev/AI-Business-Assistant/apps/api/internal/agentfoundation"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
)

type WorkflowPreviewRequest struct {
	ConversationID string        `json:"conversation_id"`
	IdempotencyKey string        `json:"idempotency_key"`
	Workflow       WorkflowInput `json:"workflow"`
}

type WorkflowPreview struct {
	TaskID string `json:"task_id"`
	WorkflowResult
}

// Preview runs against the caller's staff session. The database validates
// that same session and conversation again before persisting the receipt.
func (s Service) Preview(ctx context.Context, actor catalog.Tenant, in WorkflowPreviewRequest) (WorkflowPreview, error) {
	if !organizations.ValidID(in.ConversationID) || !organizations.ValidID(in.IdempotencyKey) || ValidateWorkflowInput(in.Workflow) != nil {
		return WorkflowPreview{}, auth.Failure(422, "INVALID_AGENT_WORKFLOW", "Проверьте параметры сценария")
	}
	id := auth.UUID()
	task := af.Task{ID: id, AgentID: "customer", CorrelationID: id, Scope: af.Scope{
		TenantID: actor.OrganizationID, PrincipalID: actor.Session.User.ID, ConversationID: in.ConversationID,
	}}
	store := workflowPreviewAttempt{task: task}
	result, runErr := s.RunStaffWorkflow(ctx, actor, task, in.Workflow, WorkflowStores{Tasks: &store, Audit: &store})
	if !store.claimed {
		return WorkflowPreview{}, previewError(runErr)
	}
	if runErr == nil && (store.status != af.StatusSucceeded || !store.toolSucceeded) {
		runErr = af.ErrDenied
	}
	var raw json.RawMessage
	if runErr == nil {
		raw, runErr = json.Marshal(result)
	}
	finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()
	tx, err := tenancy.Begin(finish, s.Pool, actor, true, true)
	if err != nil {
		return WorkflowPreview{}, err
	}
	defer tx.Rollback(finish)
	_, err = tx.Exec(finish, "SELECT platform.agent_workflow_receipt($1,$2,$3,$4,$5,$6::jsonb,$7)",
		actor.OrganizationID, in.ConversationID, id, in.IdempotencyKey, in.Workflow.Scenario, raw, runErr != nil)
	if err != nil {
		var dbErr *pgconn.PgError
		if errors.As(err, &dbErr) && dbErr.Code == "54000" {
			return WorkflowPreview{}, auth.Failure(429, "RATE_LIMITED", "Повторите позже")
		}
		return WorkflowPreview{}, tenancy.Error(err)
	}
	if err = tx.Commit(finish); err != nil {
		return WorkflowPreview{}, tenancy.Error(err)
	}
	if runErr != nil {
		return WorkflowPreview{}, previewError(runErr)
	}
	return WorkflowPreview{TaskID: id, WorkflowResult: result}, nil
}

func previewError(err error) error {
	var failure *auth.Error
	switch {
	case errors.As(err, &failure):
		return failure
	case errors.Is(err, af.ErrDenied):
		return auth.Failure(403, "FORBIDDEN", "Сценарий недоступен")
	case errors.Is(err, af.ErrInvalid), errors.Is(err, af.ErrLimit):
		return auth.Failure(422, "INVALID_AGENT_WORKFLOW", "Проверьте параметры сценария")
	default:
		return auth.Unavailable()
	}
}

type workflowPreviewAttempt struct {
	task          af.Task
	claimed       bool
	toolSucceeded bool
	status        af.Status
}

func (a *workflowPreviewAttempt) Claim(ctx context.Context, task af.Task) (bool, error) {
	if a.claimed || task != a.task {
		return false, af.ErrDenied
	}
	a.claimed = true
	return true, ctx.Err()
}

func (a *workflowPreviewAttempt) Finish(_ context.Context, task af.Task, status af.Status) error {
	if task != a.task || !a.claimed {
		return af.ErrDenied
	}
	a.status = status
	return nil
}

func (a *workflowPreviewAttempt) Record(_ context.Context, event af.AuditEvent) error {
	if event.TaskHash != af.ParamsHash(json.RawMessage(a.task.ID)) ||
		event.TenantHash != af.ParamsHash(json.RawMessage(a.task.Scope.TenantID)) || !a.claimed {
		return af.ErrDenied
	}
	if event.Step == 1 && event.Status == af.StatusSucceeded {
		a.toolSucceeded = true
	}
	return nil
}
