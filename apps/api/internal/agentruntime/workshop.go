package agentruntime

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/auth"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/catalog"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/organizations"
	"github.com/karadjev/AI-Business-Assistant/apps/api/internal/tenancy"
)

type WorkshopRequest struct {
	CustomerID     string `json:"customer_id"`
	VariantID      string `json:"variant_id"`
	Quantity       int    `json:"quantity"`
	Vehicle        string `json:"vehicle"`
	Service        string `json:"service"`
	IdempotencyKey string `json:"idempotency_key"`
}

func (s Service) Workshop(ctx context.Context, t catalog.Tenant) (json.RawMessage, error) {
	tx, err := tenancy.Begin(ctx, s.Pool, t, true, false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var out json.RawMessage
	err = tx.QueryRow(ctx, `WITH finance_scope AS (
	 SELECT timezone,date_trunc('month',CURRENT_TIMESTAMP AT TIME ZONE timezone) AS month
	 FROM platform.organizations WHERE id=$1 AND platform.workshop_owner($1)
	), recorded AS (
	 SELECT date_trunc('month',f.recorded_at AT TIME ZONE s.timezone) AS month,
	  sum(f.service_minor) AS revenue_minor,sum(f.part_cost_minor) AS cost_minor,
	  sum(f.service_minor-f.part_cost_minor) AS gross_minor
	 FROM platform.workshop_financials f CROSS JOIN finance_scope s
	 WHERE f.organization_id=$1 AND f.recorded_at >= (s.month-interval '11 months') AT TIME ZONE s.timezone
	  AND f.recorded_at<=CURRENT_TIMESTAMP
	 GROUP BY 1
	), paid AS (
	 SELECT date_trunc('month',f.paid_at AT TIME ZONE s.timezone) AS month,sum(f.service_minor) AS cash_minor
	 FROM platform.workshop_financials f CROSS JOIN finance_scope s
	 WHERE f.organization_id=$1 AND f.paid_at >= (s.month-interval '11 months') AT TIME ZONE s.timezone
	  AND f.paid_at<=CURRENT_TIMESTAMP
	 GROUP BY 1
	)
	SELECT jsonb_build_object(
	 'enabled',platform.workshop_owner($1),
	 'overview',CASE WHEN platform.workshop_owner($1) THEN jsonb_build_object(
	  'customers',(SELECT count(*) FROM platform.workshop_customers WHERE organization_id=$1),
	  'requests',(SELECT count(*) FROM platform.workshop_requests WHERE organization_id=$1),
	  'completed',(SELECT count(*) FROM platform.workshop_requests WHERE organization_id=$1 AND close_kind='consume'),
	  'financial_count',(SELECT count(*) FROM platform.workshop_financials WHERE organization_id=$1),
	  'paid_count',(SELECT count(*) FROM platform.workshop_financials WHERE organization_id=$1 AND paid_at IS NOT NULL),
	  'cash_minor',(SELECT coalesce(sum(service_minor) FILTER (WHERE paid_at IS NOT NULL),0) FROM platform.workshop_financials WHERE organization_id=$1),
	  'gross_minor',(SELECT sum(service_minor-part_cost_minor) FROM platform.workshop_financials WHERE organization_id=$1),
	  'available_skus',(SELECT count(*) FROM platform.inventory i JOIN platform.product_variants v
	    ON v.organization_id=i.organization_id AND v.id=i.variant_id JOIN platform.products p ON p.organization_id=v.organization_id AND p.id=v.product_id
	    WHERE i.organization_id=$1 AND p.status='active' AND i.quantity_on_hand>i.quantity_reserved
	    AND platform.inventory_freshness(i.organization_id,i.manual_updated_at)='FRESH'),
	  'approvals',(SELECT count(*) FROM platform.agent_approvals WHERE organization_id=$1 AND status='pending' AND expires_at>clock_timestamp()),
	   'marketing_pending',(SELECT count(*) FROM platform.marketing_drafts d JOIN platform.agent_tasks c ON c.id=d.id AND c.organization_id=d.organization_id
	    JOIN platform.agent_tasks m ON m.id=c.parent_id AND m.organization_id=c.organization_id AND m.status='completed'
	    WHERE d.organization_id=$1 AND d.status='pending')) END,
	 'finance_periods',coalesce((SELECT jsonb_agg(jsonb_build_object(
	  'month',to_char(m.month,'YYYY-MM'),'revenue_minor',r.revenue_minor,
	  'cost_minor',r.cost_minor,'gross_minor',r.gross_minor,'cash_minor',p.cash_minor) ORDER BY m.month)
	  FROM finance_scope s CROSS JOIN LATERAL generate_series(s.month-interval '11 months',s.month,interval '1 month') m(month)
	  LEFT JOIN recorded r ON r.month=m.month LEFT JOIN paid p ON p.month=m.month),'[]'),
	 'tasks',coalesce((SELECT jsonb_agg(y ORDER BY y.created_at DESC,y.id DESC) FROM (
	  SELECT id,correlation_id AS request_id,agent_id,action,status,created_at,error_code FROM platform.agent_tasks
	  WHERE organization_id=$1 AND platform.workshop_owner($1) ORDER BY created_at DESC,id DESC LIMIT 30)y),'[]'),
	 'customers',coalesce((SELECT jsonb_agg(x ORDER BY x.name,x.id) FROM
	   (SELECT id,name FROM platform.workshop_customers WHERE organization_id=$1 ORDER BY name,id LIMIT 200)x),'[]'),
	 'parts',coalesce((SELECT jsonb_agg(x ORDER BY x.name,x.sku,x.id) FROM
	   (SELECT v.id,p.name,v.sku FROM platform.product_variants v JOIN platform.products p
	    ON p.organization_id=v.organization_id AND p.id=v.product_id
	    WHERE p.organization_id=$1 AND p.status='active' AND platform.workshop_owner($1)
	    ORDER BY p.name,v.sku,v.id LIMIT 200)x),'[]'),
	 'requests',coalesce((SELECT jsonb_agg(x ORDER BY x.created_at DESC,x.id) FROM
	   (SELECT r.id,r.reserve_task_id,r.final_task_id,r.close_kind,r.reserved_at,ft.action AS final_action,r.vehicle,r.service,r.quantity,c.name AS customer, j.created_at,j.status,
	     c.marketing_consent_ref IS NOT NULL AS marketing_consent,
	     mt.status AS marketing_status, me.result->0 AS marketing_report,
	     CASE WHEN md.id IS NOT NULL THEN jsonb_build_object('id',md.id,'version',md.version,'status',md.status,
	       'content',md.content,'goal',md.goal) END AS marketing_draft,
	     j.action='delegate_workshop' AS marketing_supported,
	     f.service_minor AS financial_service_minor,f.part_cost_minor AS financial_part_cost_minor,
	     f.paid_at AS financial_paid_at,ftask.status AS financial_status,fre.result->0 AS financial_report,
	     ie.result->0 AS proposal,re.result->0 AS outcome,fe.result->0 AS final_outcome,a.id AS approval_id,a.parameters_hash,a.expires_at,
	     CASE WHEN a.status='pending' AND a.expires_at<=clock_timestamp() THEN 'expired' ELSE a.status END AS approval_status
	    FROM platform.workshop_requests r JOIN platform.workshop_customers c ON c.organization_id=r.organization_id AND c.id=r.customer_id
	    JOIN platform.agent_tasks j ON j.organization_id=r.organization_id AND j.id=r.id
	    LEFT JOIN platform.workshop_financials f ON f.organization_id=r.organization_id AND f.request_id=r.id
	    LEFT JOIN platform.agent_tasks ftask ON ftask.organization_id=r.organization_id AND ftask.id=f.report_task_id
	    LEFT JOIN LATERAL (SELECT e.result FROM platform.agent_executions e WHERE e.organization_id=r.organization_id
	      AND e.task_id=f.report_task_id AND e.result IS NOT NULL ORDER BY e.attempt DESC LIMIT 1) fre ON true
	    LEFT JOIN LATERAL (SELECT m.id,m.status FROM platform.agent_tasks m WHERE m.organization_id=r.organization_id
	     AND m.parent_id=r.id AND m.action='delegate_workshop_marketing' ORDER BY m.created_at DESC,m.id LIMIT 1) mt ON true
	    LEFT JOIN LATERAL (SELECT e.result FROM platform.agent_executions e WHERE e.organization_id=r.organization_id
	      AND e.task_id=mt.id AND e.result IS NOT NULL ORDER BY e.attempt DESC LIMIT 1) me ON true
	    LEFT JOIN platform.agent_tasks mc ON mc.organization_id=r.organization_id AND mc.parent_id=mt.id AND mc.action='delegation.workshop_marketing'
	    LEFT JOIN platform.marketing_drafts md ON md.organization_id=r.organization_id AND md.id=mc.id AND md.version=1 AND mt.status='completed'
	    LEFT JOIN platform.agent_tasks ft ON ft.organization_id=r.organization_id AND ft.id=r.final_task_id
	    LEFT JOIN platform.agent_approvals a ON a.organization_id=r.organization_id AND a.task_id=coalesce(r.final_task_id,r.reserve_task_id)
	    LEFT JOIN LATERAL (SELECT e.result FROM platform.agent_executions e WHERE e.organization_id=r.organization_id
	      AND e.task_id=r.final_task_id ORDER BY e.attempt DESC LIMIT 1) fe ON true
	    LEFT JOIN LATERAL (SELECT e.result FROM platform.agent_executions e WHERE e.organization_id=r.organization_id
	      AND e.task_id=r.id AND e.result IS NOT NULL ORDER BY e.attempt DESC LIMIT 1) ie ON true
	    LEFT JOIN LATERAL (SELECT e.result FROM platform.agent_executions e WHERE e.organization_id=r.organization_id
	      AND e.task_id=r.reserve_task_id ORDER BY e.attempt DESC LIMIT 1) re ON true
	    WHERE r.organization_id=$1 ORDER BY j.created_at DESC,j.id LIMIT 50)x),'[]'),
	 'audit',coalesce((SELECT jsonb_agg(x ORDER BY x.created_at DESC,x.id) FROM
	   (SELECT e.id,e.task_id,t.correlation_id AS request_id,e.action,e.result,e.created_at
	    FROM platform.agent_audit e JOIN platform.agent_tasks t ON t.organization_id=e.organization_id AND t.id=e.task_id
	    WHERE e.organization_id=$1
	    AND platform.workshop_history(e.organization_id,e.task_id) ORDER BY e.created_at DESC,e.id LIMIT 100)x),'[]'))`,
		t.OrganizationID).Scan(&out)
	if err != nil {
		return nil, tenancy.Error(err)
	}
	return out, nil
}

func (s Service) CreateWorkshopCustomer(ctx context.Context, t catalog.Tenant, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || !workflowText(name, 120) {
		return "", auth.Failure(422, "INVALID_CUSTOMER", "Укажите имя клиента")
	}
	tx, err := tenancy.Begin(ctx, s.Pool, t, true, false)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var id string
	if err = tx.QueryRow(ctx, `INSERT INTO platform.workshop_customers(organization_id,name) VALUES($1,$2) RETURNING id`, t.OrganizationID, name).Scan(&id); err != nil {
		return "", tenancy.Error(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return "", tenancy.Error(err)
	}
	return id, nil
}

func (s Service) SubmitWorkshop(ctx context.Context, t catalog.Tenant, in WorkshopRequest) (string, error) {
	in.Vehicle, in.Service = strings.TrimSpace(in.Vehicle), strings.TrimSpace(in.Service)
	if !organizations.ValidID(in.CustomerID) || !organizations.ValidID(in.VariantID) || !organizations.ValidID(in.IdempotencyKey) ||
		in.Quantity < 1 || in.Quantity > 10 || in.Vehicle == "" || in.Service == "" || !workflowText(in.Vehicle, 120) || !workflowText(in.Service, 160) {
		return "", auth.Failure(422, "INVALID_REQUEST", "Проверьте карточку клиента, автомобиль, услугу, деталь и количество")
	}
	if len(s.Key) != 32 {
		return "", auth.Unavailable()
	}
	tx, err := tenancy.Begin(ctx, s.Pool, t, true, false)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var id string
	err = tx.QueryRow(ctx, `SELECT platform.workshop_submit($1,$2,$3,$4,$5,$6,$7,sha256($8::bytea))`,
		t.OrganizationID, in.IdempotencyKey, in.CustomerID, in.VariantID, in.Quantity, in.Vehicle, in.Service, s.Proof()).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "54000" {
			return "", auth.Failure(429, "RATE_LIMITED", "Слишком много задач")
		}
		return "", tenancy.Error(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return "", tenancy.Error(err)
	}
	return id, nil
}

func (s Service) ProposeWorkshopClose(ctx context.Context, t catalog.Tenant, requestID, operation string) (string, error) {
	if !organizations.ValidID(requestID) || (operation != "release" && operation != "consume") {
		return "", auth.Failure(422, "INVALID_WORKSHOP_OPERATION", "Проверьте заявку и действие")
	}
	tx, err := tenancy.Begin(ctx, s.Pool, t, true, false)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var id *string
	if err = tx.QueryRow(ctx, `SELECT platform.workshop_close_propose($1,$2,$3)`,
		t.OrganizationID, requestID, "workshop_"+operation).Scan(&id); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "54000" {
			return "", auth.Failure(429, "RATE_LIMITED", "Слишком много задач")
		}
		return "", tenancy.Error(err)
	}
	if id == nil {
		return "", auth.Failure(409, "RESERVATION_UNAVAILABLE", "Нет активного резерва или уже ожидается другое решение")
	}
	if err = tx.Commit(ctx); err != nil {
		return "", tenancy.Error(err)
	}
	return *id, nil
}

func (s Service) DecideWorkshop(ctx context.Context, t catalog.Tenant, id, decision, hash string) error {
	_, err := hex.DecodeString(hash)
	if !organizations.ValidID(id) || len(hash) != 64 || err != nil || (decision != "approved" && decision != "rejected") {
		return auth.Failure(422, "INVALID_DECISION", "Неверное решение")
	}
	tx, err := tenancy.Begin(ctx, s.Pool, t, true, false)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var ok bool
	if err = tx.QueryRow(ctx, `SELECT platform.workshop_decide($1,$2,$3,$4)`, t.OrganizationID, id, decision, hash).Scan(&ok); err != nil {
		return tenancy.Error(err)
	}
	if !ok {
		return auth.Failure(409, "APPROVAL_UNAVAILABLE", "Запрос изменился или больше недоступен")
	}
	if err = tx.Commit(ctx); err != nil {
		return tenancy.Error(err)
	}
	return nil
}
