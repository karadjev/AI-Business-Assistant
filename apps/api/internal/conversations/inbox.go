package conversations

import (
	"context"
	"encoding/json"

	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
)

func (s Service) InboxReview(ctx context.Context, t catalog.Tenant, enable bool) (bool, error) {
	tx, err := s.begin(ctx, t)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	if enable {
		if _, err = tx.Exec(ctx, "SELECT platform.set_inbox_review($1)", t.OrganizationID); err != nil {
			return false, tenancy.Error(err)
		}
	}
	var enabled bool
	if err = tx.QueryRow(ctx, "SELECT inbox_review_enabled FROM platform.organizations WHERE id=$1", t.OrganizationID).Scan(&enabled); err != nil {
		return false, tenancy.Error(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return false, tenancy.Error(err)
	}
	return enabled, nil
}

func (s Service) DecideInboxReply(ctx context.Context, t catalog.Tenant, conversationID, taskID, decision string) error {
	if !organizations.ValidID(conversationID) || !organizations.ValidID(taskID) || (decision != "approved" && decision != "rejected") {
		return auth.Failure(422, "VALIDATION_ERROR", "Проверьте идентификатор черновика и решение")
	}
	tx, err := s.begin(ctx, t)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var status string
	if err = tx.QueryRow(ctx, "SELECT platform.decide_inbox_reply($1,$2,$3,$4)", t.OrganizationID, conversationID, taskID, decision).Scan(&status); err != nil {
		return tenancy.Error(err)
	}
	switch status {
	case decision:
		if err = tx.Commit(ctx); err != nil {
			return tenancy.Error(err)
		}
		return nil
	case "missing":
		return auth.Failure(404, "NOT_FOUND", "Черновик не найден")
	default:
		return auth.Failure(409, "DRAFT_UNAVAILABLE", "Черновик устарел или решение уже принято")
	}
}

func (s Service) InboxDetail(ctx context.Context, t catalog.Tenant, id string) (json.RawMessage, error) {
	if !organizations.ValidID(id) {
		return nil, auth.Failure(404, "NOT_FOUND", "Диалог не найден")
	}
	tx, err := s.begin(ctx, t)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	return readJSON(ctx, tx, `SELECT jsonb_build_object(
	 'customer',jsonb_build_object('id',c.customer_id,'channel',c.channel),
	 'requests',coalesce((SELECT jsonb_agg(to_jsonb(x) ORDER BY x.created_at DESC,x.id) FROM (
	   SELECT id,status,created_at FROM platform.orders WHERE (organization_id,conversation_id,customer_id)=(c.organization_id,c.id,c.customer_id)
	   ORDER BY created_at DESC,id LIMIT 20)x),'[]'::jsonb),
	 'consents',coalesce((SELECT jsonb_agg(to_jsonb(x) ORDER BY x.occurred_at DESC,x.decision DESC,x.source_message_id DESC) FROM (
	   SELECT channel,purpose,source,source_message_id,occurred_at,recorded_at,decision,research_only
	   FROM platform.customer_consent_events WHERE (organization_id,conversation_id,customer_id)=(c.organization_id,c.id,c.customer_id)
	   ORDER BY occurred_at DESC,decision DESC,source_message_id DESC LIMIT 20)x),'[]'::jsonb),
	 'review_enabled',o.inbox_review_enabled,
     'version',c.version,'assignee_id',c.assignee_id,
     'operators',CASE WHEN platform.has_membership(c.organization_id,true) THEN
       (SELECT coalesce(jsonb_agg(jsonb_build_object('id',m->>'id','name',m->>'display_name')),'[]'::jsonb)
         FROM jsonb_array_elements(platform.team_state(c.organization_id)->'members') m WHERE m->>'operator'='true') ELSE '[]'::jsonb END,
	 'intent',r.intent,'processing_state',r.last_result->>'state',
	 'delivery',coalesce((SELECT jsonb_agg(to_jsonb(v) ORDER BY v.created_at DESC,v.id) FROM (
	   SELECT id,status,attempts,created_at FROM platform.telegram_outbox WHERE (organization_id,conversation_id)=(c.organization_id,c.id)
	   UNION ALL
	   SELECT id,status,attempts,created_at FROM platform.channel_outbox WHERE (organization_id,conversation_id)=(c.organization_id,c.id)
	   ORDER BY created_at DESC,id LIMIT 20)v),'[]'::jsonb),
	 'drafts',coalesce((SELECT jsonb_agg(to_jsonb(d) ORDER BY d.expires_at DESC,d.task_id) FROM (
	   SELECT task_id,body,expires_at,decided_at,CASE WHEN status='pending' AND expires_at<=clock_timestamp()
	     THEN 'expired' ELSE status END AS status
	   FROM platform.inbox_reply_drafts WHERE (organization_id,conversation_id)=(c.organization_id,c.id)
	   AND c.status='open' AND c.expires_at>clock_timestamp() ORDER BY expires_at DESC,task_id LIMIT 20)d),'[]'::jsonb))
	 FROM platform.conversations c JOIN platform.organizations o ON o.id=c.organization_id
	 LEFT JOIN platform.conversation_routes r ON (r.organization_id,r.conversation_id)=(c.organization_id,c.id)
	 WHERE (c.organization_id,c.id)=($1,$2)`, t.OrganizationID, id)
}

func (s Service) AssignInbox(ctx context.Context, t catalog.Tenant, id string, assignee *string, version int64) error {
	if !organizations.ValidID(id) || assignee != nil && !organizations.ValidID(*assignee) || version < 1 {
		return auth.Failure(422, "VALIDATION_ERROR", "Укажите сотрудника и версию диалога")
	}
	tx, err := s.begin(ctx, t)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var changed bool
	if err = tx.QueryRow(ctx, "SELECT platform.inbox_assign($1,$2,$3,$4)", t.OrganizationID, id, assignee, version).Scan(&changed); err != nil {
		return tenancy.Error(err)
	}
	if !changed {
		return auth.Failure(409, "ASSIGNMENT_UNAVAILABLE", "Диалог изменился или у сотрудника нет доступа")
	}
	if err = tx.Commit(ctx); err != nil {
		return tenancy.Error(err)
	}
	return nil
}
