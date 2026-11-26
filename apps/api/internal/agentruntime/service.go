package agentruntime

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
)

type Service struct {
	Pool *pgxpool.Pool
	Key  []byte
}

type Request struct {
	AgentID        string `json:"agent_id"`
	Action         string `json:"action"`
	VariantID      string `json:"variant_id,omitempty"`
	ConversationID string `json:"conversation_id,omitempty"`
	IdempotencyKey string `json:"idempotency_key"`
}

func (s Service) Proof() []byte {
	mac := hmac.New(sha256.New, s.Key)
	mac.Write([]byte("aba:agent-runtime:v1"))
	return mac.Sum(nil)
}

func (s Service) ownerTx(ctx context.Context, t catalog.Tenant) (pgx.Tx, error) {
	if t.ChannelCapability != "" || t.AgentLease != "" || t.AgentTaskID != "" || len(t.Session.Hash) != sha256.Size {
		return nil, denied()
	}
	tx, err := tenancy.Begin(ctx, s.Pool, t, false, true)
	if err != nil {
		return nil, err
	}
	var ok bool
	if err = tx.QueryRow(ctx, "SELECT platform.agent_owner($1)", t.OrganizationID).Scan(&ok); err != nil || !ok {
		tx.Rollback(ctx)
		if err != nil {
			return nil, auth.Unavailable()
		}
		return nil, denied()
	}
	return tx, nil
}

func denied() error {
	return auth.Failure(403, "FORBIDDEN", "Для действия нужны права владельца тестового бизнеса")
}

func (r Request) valid() bool {
	if !organizations.ValidID(r.IdempotencyKey) {
		return false
	}
	switch r.AgentID {
	case "customer", "sales", "product_inventory":
		return catalogAction(r.Action) && organizations.ValidID(r.ConversationID) && organizations.ValidID(r.VariantID)
	case "marketing":
		return r.Action == "draft" && r.VariantID == "" && r.ConversationID == ""
	case "finance", "analytics":
		return r.Action == "read" && r.VariantID == "" && r.ConversationID == ""
	case "business_manager":
		return (r.Action == "plan" || r.Action == "coordinate" || r.Action == "coordinate_growth" || r.Action == dailyCoordination) && r.VariantID == "" && r.ConversationID == ""
	}
	return false
}

func (s Service) Submit(ctx context.Context, t catalog.Tenant, r Request) (string, error) {
	if !r.valid() {
		return "", auth.Failure(422, "INVALID_AGENT_TASK", "Задача не поддерживается")
	}
	if len(s.Key) != 32 {
		return "", auth.Unavailable()
	}
	if r.Action == dailyCoordination {
		return s.submitDaily(ctx, t)
	}
	tx, err := s.ownerTx(ctx, t)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	if catalogAction(r.Action) {
		var model string
		if err := tx.QueryRow(ctx, "SELECT business_model FROM platform.organizations WHERE id=$1", t.OrganizationID).Scan(&model); err != nil {
			return "", auth.Unavailable()
		}
		if model != "trade" && model != "hybrid" {
			return "", auth.Failure(403, "CATALOG_ACTION_UNAVAILABLE", "Каталог товаров недоступен для этого профиля")
		}
	}
	hash := sha256.Sum256(s.Proof())
	var id string
	err = tx.QueryRow(ctx, "SELECT platform.agent_submit($1,$2,$3,$4,$5,nullif($6,'')::uuid,nullif($7,'')::uuid,$8)",
		t.OrganizationID, auth.UUID(), r.IdempotencyKey, r.AgentID, r.Action, r.VariantID, r.ConversationID, hash[:]).Scan(&id)
	if err != nil {
		return "", submitError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return "", auth.Unavailable()
	}
	return id, nil
}

func submitError(err error) error {
	var dbErr *pgconn.PgError
	if errors.As(err, &dbErr) && dbErr.Code == "54000" {
		return auth.Failure(429, "RATE_LIMITED", "Повторите позже")
	}
	return tenancy.Error(err)
}

func (s Service) ListApprovals(ctx context.Context, t catalog.Tenant) (json.RawMessage, error) {
	return s.list(ctx, t, `SELECT jsonb_build_object('items',coalesce(jsonb_agg(x ORDER BY x.expires_at,x.id),'[]')) FROM (
 SELECT a.id,a.task_id,j.agent_id,j.action,a.parameters,a.parameters_hash,a.expires_at,
 CASE WHEN a.status='pending' AND a.expires_at<=clock_timestamp() THEN 'expired' ELSE a.status END status,
 'Проверка параметров перед созданием черновика' reason
 FROM platform.agent_approvals a JOIN platform.agent_tasks j ON j.organization_id=a.organization_id AND j.id=a.task_id
 WHERE a.organization_id=$1 ORDER BY (a.status='pending' AND a.expires_at>clock_timestamp()) DESC,a.created_at DESC,a.id LIMIT 50)x`)
}

func (s Service) ListTasks(ctx context.Context, t catalog.Tenant) (json.RawMessage, error) {
	return s.list(ctx, t, `SELECT jsonb_build_object('items',coalesce(jsonb_agg(x ORDER BY x.created_at DESC,x.id),'[]')) FROM (
 SELECT j.id,j.parent_id,j.correlation_id,j.agent_id,j.action,j.status,j.priority,j.created_at,j.updated_at,j.started_at,j.completed_at,j.deadline,j.retry_count,j.error_code,e.result
 FROM platform.agent_tasks j LEFT JOIN LATERAL (SELECT result FROM platform.agent_executions WHERE organization_id=j.organization_id AND task_id=j.id
 ORDER BY attempt DESC LIMIT 1)e ON true WHERE j.organization_id=$1 ORDER BY j.created_at DESC,j.id LIMIT 50)x`)
}

func (s Service) list(ctx context.Context, t catalog.Tenant, query string) (json.RawMessage, error) {
	tx, err := s.ownerTx(ctx, t)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var out json.RawMessage
	if err = tx.QueryRow(ctx, query, t.OrganizationID).Scan(&out); err != nil {
		return nil, auth.Unavailable()
	}
	return out, nil
}

func (s Service) Decide(ctx context.Context, t catalog.Tenant, id, decision, hash string) error {
	_, validHash := hex.DecodeString(hash)
	if !organizations.ValidID(id) || len(hash) != 64 || validHash != nil || (decision != "approved" && decision != "rejected") {
		return auth.Failure(422, "INVALID_DECISION", "Неверное решение")
	}
	tx, err := s.ownerTx(ctx, t)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var ok bool
	if err = tx.QueryRow(ctx, "SELECT platform.agent_decide($1,$2,$3,$4)", t.OrganizationID, id, decision, hash).Scan(&ok); err != nil {
		return tenancy.Error(err)
	}
	if !ok {
		return auth.Failure(409, "APPROVAL_UNAVAILABLE", "Запрос изменился или больше недоступен")
	}
	if err = tx.Commit(ctx); err != nil {
		return auth.Unavailable()
	}
	return nil
}

// ListAudit returns an owner-scoped timeline. It never returns inputs, credential
// material, free-form provider errors or worker leases.
func (s Service) ListAudit(ctx context.Context, t catalog.Tenant, taskID, agentID, action string, since time.Time) (json.RawMessage, error) {
	if (taskID != "" && !organizations.ValidID(taskID)) || len(agentID) > 40 || len(action) > 100 {
		return nil, auth.Failure(422, "INVALID_FILTER", "Неверный фильтр журнала")
	}
	tx, err := s.ownerTx(ctx, t)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var raw json.RawMessage
	err = tx.QueryRow(ctx, `SELECT jsonb_build_object('items',coalesce(jsonb_agg(x ORDER BY x.created_at DESC,x.id),'[]')) FROM (
 SELECT id,task_id,execution_id,approval_id,actor_id,agent_id,action,result,error_code,created_at
 FROM platform.agent_audit WHERE organization_id=$1 AND (nullif($2,'') IS NULL OR task_id=nullif($2,'')::uuid)
 AND ($3='' OR agent_id=$3) AND ($4='' OR action=$4) AND created_at>=$5
 ORDER BY created_at DESC,id LIMIT 100)x`, t.OrganizationID, taskID, agentID, action, since).Scan(&raw)
	if err != nil {
		return nil, auth.Unavailable()
	}
	return raw, nil
}
