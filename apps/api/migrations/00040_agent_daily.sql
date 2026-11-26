-- +goose Up
-- +goose StatementBegin
ALTER TABLE platform.agent_tasks DROP CONSTRAINT agent_task_operation;
ALTER TABLE platform.agent_tasks ADD CONSTRAINT agent_task_operation CHECK (
 (agent_id IN ('customer','sales','product_inventory') AND action IN ('get_product_price','check_inventory','get_product_details') AND variant_id IS NOT NULL AND conversation_id IS NOT NULL)
 OR (variant_id IS NULL AND conversation_id IS NULL AND ((agent_id='marketing' AND action='draft') OR
 (agent_id IN ('finance','analytics') AND action='read') OR (agent_id='business_manager' AND action IN ('plan','coordinate','coordinate_growth'))))
 OR (variant_id IS NULL AND conversation_id IS NULL AND parent_id IS NOT NULL AND
 ((agent_id='sales' AND action='review_sales') OR (agent_id='product_inventory' AND action='review_inventory')))
 OR (agent_id='customer' AND action IN ('workflow_preview_B','workflow_preview_C','workflow_preview_D','workflow_preview_E')
     AND conversation_id IS NOT NULL AND variant_id IS NULL AND parent_id IS NULL
     AND status IN ('completed','failed') AND correlation_id=id)
 OR (action IN ('delegate_auto_parts','delegate_construction','delegate_service','delegate_real_estate','delegate_property_viewing','delegate_conversation') AND agent_id='business_manager' AND parent_id IS NULL
     AND correlation_id=id AND conversation_id IS NOT NULL AND variant_id IS NULL AND delegation_input IS NOT NULL AND delegation_step IS NULL)
 OR (action LIKE 'delegation.%' AND parent_id IS NOT NULL AND conversation_id IS NOT NULL
     AND variant_id IS NULL AND delegation_step BETWEEN 1 AND 20 AND delegation_input IS NULL)
 OR (conversation_id IS NULL AND variant_id IS NULL AND delegation_input IS NULL AND delegation_step IS NULL
     AND ((agent_id='product_inventory' AND action='workshop_inspect' AND parent_id IS NULL AND correlation_id=id)
       OR (agent_id='product_inventory' AND action IN ('workshop_reserve','workshop_release','workshop_consume') AND parent_id IS NOT NULL AND parent_id=correlation_id)))
 OR (conversation_id IS NULL AND variant_id IS NULL AND delegation_input IS NULL AND (
  (agent_id='business_manager' AND action='delegate_workshop' AND parent_id IS NULL AND correlation_id=id AND delegation_step IS NULL)
  OR (parent_id IS NOT NULL AND parent_id=correlation_id AND (
   (agent_id='customer' AND action='delegation.workshop_customer' AND delegation_step IS NOT DISTINCT FROM 1)
   OR (agent_id='product_inventory' AND action='delegation.workshop_stock' AND delegation_step IS NOT DISTINCT FROM 2)
   OR (agent_id='sales' AND action='delegation.workshop_sales' AND delegation_step IS NOT DISTINCT FROM 3)))))
 OR (conversation_id IS NULL AND variant_id IS NULL AND parent_id IS NOT NULL AND (
  (agent_id='business_manager' AND action='delegate_workshop_finance' AND parent_id=correlation_id
   AND delegation_step IS NULL AND delegation_input IS NOT NULL)
  OR (delegation_input IS NULL AND (
   (agent_id='finance' AND action='delegation.workshop_finance' AND delegation_step IS NOT DISTINCT FROM 1)
   OR (agent_id='analytics' AND action='delegation.workshop_analytics' AND delegation_step IS NOT DISTINCT FROM 2)))))
 OR (conversation_id IS NULL AND variant_id IS NULL AND parent_id IS NOT NULL AND (
  (agent_id='business_manager' AND action='delegate_workshop_marketing' AND parent_id=correlation_id
   AND delegation_step IS NULL AND delegation_input IS NOT NULL)
  OR (agent_id='marketing' AND action='delegation.workshop_marketing'
   AND delegation_step IS NOT DISTINCT FROM 1 AND delegation_input IS NULL)))
 OR (conversation_id IS NULL AND variant_id IS NULL AND delegation_input IS NULL AND delegation_step IS NULL AND
  ((agent_id='business_manager' AND action='coordinate_daily' AND parent_id IS NULL AND correlation_id=id)
   OR (agent_id IN ('customer','sales','product_inventory','marketing') AND action='daily_review' AND parent_id IS NOT NULL AND parent_id=correlation_id))));
CREATE OR REPLACE FUNCTION platform.agent_claim(p_proof bytea,p_lease uuid) RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE q platform.channel_inbox; t platform.agent_tasks; ex uuid:=gen_random_uuid(); previous_execution uuid; apr uuid; reason text; terminal text;
BEGIN
 IF p_lease IS NULL OR octet_length(p_proof) IS DISTINCT FROM 32 THEN RETURN NULL; END IF;
 -- Expire at most one unapproved request per tick, with audit in the same transaction.
 SELECT a.id,a.task_id INTO apr,t.id FROM platform.agent_approvals a JOIN platform.agent_tasks x ON x.id=a.task_id AND x.organization_id=a.organization_id
 WHERE a.status='pending' AND a.expires_at<=clock_timestamp() AND x.worker_hash=sha256(p_proof)
 ORDER BY a.expires_at LIMIT 1 FOR UPDATE OF a SKIP LOCKED;
 IF apr IS NOT NULL THEN
  UPDATE platform.agent_tasks SET status='expired',completed_at=clock_timestamp(),error_code='expired' WHERE id=t.id AND status='waiting_approval' RETURNING * INTO t;
  IF FOUND THEN
   UPDATE platform.agent_approvals SET status='expired' WHERE id=apr;
   INSERT INTO platform.agent_audit(organization_id,task_id,approval_id,agent_id,action,result)
   VALUES(t.organization_id,t.id,apr,t.agent_id,'approval.expire','expired');
  END IF;
 END IF;
 SELECT x.* INTO q FROM platform.channel_inbox x JOIN platform.agent_tasks j ON j.id=x.agent_task_id AND j.organization_id=x.organization_id
 WHERE j.worker_hash=sha256(p_proof)
 AND (j.deadline<=clock_timestamp() OR NOT EXISTS(SELECT 1 FROM platform.agent_tasks c WHERE c.organization_id=j.organization_id AND c.parent_id=j.id AND c.status IN ('pending','queued','running','waiting_approval')))
 AND ((x.status='pending' AND x.available_at<=clock_timestamp()) OR (x.status='processing' AND x.lease_until<=clock_timestamp()))
 ORDER BY j.priority DESC,x.available_at,x.created_at,x.id LIMIT 1 FOR UPDATE OF x SKIP LOCKED;
 IF NOT FOUND THEN RETURN NULL; END IF;
 SELECT * INTO t FROM platform.agent_tasks WHERE id=q.agent_task_id AND organization_id=q.organization_id FOR UPDATE;
 UPDATE platform.agent_executions SET status='interrupted',error_code='interrupted',completed_at=clock_timestamp()
 WHERE task_id=t.id AND status='running' RETURNING id INTO previous_execution;
 IF previous_execution IS NOT NULL THEN
  INSERT INTO platform.agent_audit(organization_id,task_id,execution_id,actor_id,agent_id,action,result,error_code)
  VALUES(t.organization_id,t.id,previous_execution,t.initiator,t.agent_id,'task.interrupt','interrupted','interrupted');
 END IF;
 apr:=NULL;
 IF t.agent_id='marketing' AND t.action NOT IN ('delegation.workshop_marketing','daily_review') THEN
  SELECT id INTO apr FROM platform.agent_approvals WHERE task_id=t.id AND status='approved' AND expires_at>clock_timestamp()
  AND parameters_hash=encode(sha256(convert_to('{"action":"draft"}','UTF8')),'hex');
 END IF;
 IF t.deadline<=clock_timestamp() THEN reason:='expired'; terminal:='expired';
 ELSIF q.attempts>=3 THEN reason:='interrupted';terminal:='failed';
 ELSIF (t.agent_id='marketing' AND t.action NOT IN ('delegation.workshop_marketing','daily_review') AND apr IS NULL) OR NOT EXISTS(
  SELECT 1 FROM platform.organization_members m JOIN platform.organizations o ON o.id=m.organization_id
  WHERE m.organization_id=t.organization_id AND m.user_id=t.initiator AND m.role='owner' AND ((o.is_test AND t.action NOT IN ('workshop_inspect','workshop_reserve','workshop_release','workshop_consume')) OR platform.workshop_task_allowed(t.organization_id,t.id,t.action)))
 THEN reason:='denied';terminal:='cancelled'; END IF;
 IF terminal IS NOT NULL THEN
  UPDATE platform.channel_inbox SET status='failed',agent_lease=NULL,lease_until=NULL WHERE id=q.id;
  UPDATE platform.agent_tasks SET status=terminal,completed_at=clock_timestamp(),error_code=reason WHERE id=t.id;
  UPDATE platform.agent_approvals SET status=CASE WHEN terminal='expired' THEN 'expired' ELSE 'execution_failed' END WHERE task_id=t.id AND status='approved' RETURNING id INTO apr;
  INSERT INTO platform.agent_audit(organization_id,task_id,approval_id,agent_id,action,result,error_code)
  VALUES(t.organization_id,t.id,apr,t.agent_id,'task.recover',terminal,reason);
  RETURN NULL;
 END IF;
 UPDATE platform.channel_inbox SET status='processing',agent_lease=p_lease,lease_until=least(t.deadline,clock_timestamp()+interval '30 seconds'),attempts=attempts+1 WHERE id=q.id;
 UPDATE platform.agent_tasks SET status='running',started_at=coalesce(started_at,clock_timestamp()),retry_count=q.attempts WHERE id=t.id;
 INSERT INTO platform.agent_executions(id,organization_id,task_id,agent_version,attempt,status)
 VALUES(ex,t.organization_id,t.id,'v1',q.attempts+1,'running');
 INSERT INTO platform.agent_audit(organization_id,task_id,execution_id,approval_id,actor_id,agent_id,action,result)
 VALUES(t.organization_id,t.id,ex,apr,t.initiator,t.agent_id,'task.claim','running');
 RETURN jsonb_build_object('ID',t.id,'OrganizationID',t.organization_id,'Initiator',t.initiator,'AgentID',t.agent_id,
  'LegacyBusinessType',(SELECT business_type FROM platform.organizations WHERE id=t.organization_id),'Action',t.action,'ParentAction',(SELECT action FROM platform.agent_tasks WHERE organization_id=t.organization_id AND id=t.parent_id),'CorrelationID',t.correlation_id,'VariantID',t.variant_id,'ConversationID',t.conversation_id,'ExecutionID',ex,'Deadline',t.deadline);
END $$;
CREATE OR REPLACE FUNCTION platform.agent_lease_task(p_id uuid,p_lease uuid) RETURNS platform.agent_tasks
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT t FROM platform.agent_tasks t JOIN platform.channel_inbox q ON q.agent_task_id=t.id AND q.organization_id=t.organization_id
 JOIN platform.organization_members m ON m.organization_id=t.organization_id AND m.user_id=t.initiator AND m.role='owner'
 JOIN platform.organizations o ON o.id=t.organization_id AND ((o.is_test AND t.action NOT IN ('workshop_inspect','workshop_reserve','workshop_release','workshop_consume')) OR platform.workshop_task_allowed(t.organization_id,t.id,t.action))
 WHERE t.id=p_id AND t.status='running' AND q.agent_lease=p_lease AND q.status='processing'
 AND q.lease_until>clock_timestamp() AND t.deadline>clock_timestamp()
 AND (t.agent_id<>'marketing' OR t.action IN ('delegation.workshop_marketing','daily_review') OR EXISTS(SELECT 1 FROM platform.agent_approvals a WHERE a.task_id=t.id AND a.status='approved' AND a.expires_at>clock_timestamp()
 AND a.parameters_hash=encode(sha256(convert_to('{"action":"draft"}','UTF8')),'hex')))
$$;
CREATE OR REPLACE FUNCTION platform.agent_child_results(p_task uuid,p_lease uuid) RETURNS jsonb
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT coalesce(jsonb_agg(jsonb_build_object('agent_id',c.agent_id,'status',c.status,'error_code',c.error_code,'result',e.result) ORDER BY c.agent_id),'[]')
 FROM platform.agent_lease_task(p_task,p_lease) t
 JOIN platform.agent_tasks c ON c.organization_id=t.organization_id AND c.parent_id=t.id
 LEFT JOIN LATERAL (SELECT x.result FROM platform.agent_executions x
 WHERE x.organization_id=c.organization_id AND x.task_id=c.id AND x.status='completed' ORDER BY x.attempt DESC LIMIT 1)e ON true
 WHERE t.agent_id='business_manager' AND t.action IN ('coordinate','coordinate_growth','coordinate_daily')
$$;
-- One immutable owner-initiated snapshot per local day. The native queue
-- and commercial triggers still account for the parent and every child.
CREATE UNIQUE INDEX agent_daily_single ON platform.agent_tasks(organization_id,idempotency_key)
 WHERE action='coordinate_daily' AND parent_id IS NULL;

CREATE FUNCTION platform.agent_daily_submit(p_org uuid,p_id uuid,p_hash bytea) RETURNS uuid
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE zone text; key uuid; root uuid;
BEGIN
 IF NOT platform.agent_owner(p_org) THEN RAISE EXCEPTION 'agent owner required' USING ERRCODE='42501'; END IF;
 PERFORM pg_advisory_xact_lock(hashtextextended(p_org::text,8231));
 SELECT timezone INTO STRICT zone FROM platform.organizations WHERE id=p_org;
 key:=md5('daily-v1:'||p_org::text||':'||((statement_timestamp() AT TIME ZONE zone)::date)::text)::uuid;
 SELECT id INTO root FROM platform.agent_tasks WHERE organization_id=p_org AND action='coordinate_daily' AND idempotency_key=key;
 IF FOUND THEN RETURN root; END IF;
 RETURN platform.agent_submit(p_org,p_id,key,'business_manager','coordinate_daily',NULL,NULL,p_hash);
END $$;

CREATE FUNCTION platform.agent_daily_children() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE child uuid; specialist text; operation text; zone text;
BEGIN
 IF NOT platform.agent_owner(NEW.organization_id) THEN RAISE EXCEPTION 'agent owner required' USING ERRCODE='42501'; END IF;
 SELECT timezone INTO STRICT zone FROM platform.organizations WHERE id=NEW.organization_id;
 IF NEW.idempotency_key IS DISTINCT FROM md5('daily-v1:'||NEW.organization_id::text||':'||((statement_timestamp() AT TIME ZONE zone)::date)::text)::uuid THEN
  RAISE EXCEPTION 'daily scope denied' USING ERRCODE='42501';
 END IF;
 -- agent_submit accounts for 20/minute. Reserve the complete batch atomically.
 IF (SELECT count(*) FROM platform.agent_tasks WHERE organization_id=NEW.organization_id AND created_at>clock_timestamp()-interval '1 minute')>14 THEN
  RAISE EXCEPTION 'agent task rate limit' USING ERRCODE='54000';
 END IF;
 FOR specialist,operation IN SELECT * FROM (VALUES
  ('customer','daily_review'),('sales','daily_review'),('product_inventory','daily_review'),
  ('marketing','daily_review'),('finance','read'),('analytics','read')) AS work(agent_name,operation_name)
 LOOP
  child:=gen_random_uuid();
  INSERT INTO platform.agent_tasks(id,organization_id,initiator,agent_id,parent_id,correlation_id,action,idempotency_key,worker_hash,status,deadline)
  VALUES(child,NEW.organization_id,NEW.initiator,specialist,NEW.id,NEW.id,operation,child,NEW.worker_hash,'queued',NEW.deadline);
  INSERT INTO platform.channel_inbox(id,organization_id,external_id,external_user_id,payload,channel,agent_task_id)
  VALUES(child,NEW.organization_id,child::text,'','{}','agent',child);
  INSERT INTO platform.agent_audit(organization_id,task_id,actor_id,agent_id,action,result)
  VALUES(NEW.organization_id,child,NEW.initiator,specialist,'task.delegate','queued');
 END LOOP;
 RETURN NEW;
END $$;
CREATE TRIGGER agent_daily_children AFTER INSERT ON platform.agent_tasks
 FOR EACH ROW WHEN (NEW.agent_id='business_manager' AND NEW.action='coordinate_daily' AND NEW.parent_id IS NULL)
 EXECUTE FUNCTION platform.agent_daily_children();

CREATE FUNCTION platform.agent_daily_context(p_task uuid,p_lease uuid) RETURNS jsonb
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 WITH principal AS MATERIALIZED (
  SELECT t.*,p.created_at AS cutoff,o.timezone,o.business_model,p.created_at AT TIME ZONE o.timezone AS local_time
  FROM platform.agent_lease_task(p_task,p_lease) t
  JOIN platform.agent_tasks p ON p.organization_id=t.organization_id AND p.id=coalesce(t.parent_id,t.id)
   AND p.agent_id='business_manager' AND p.action='coordinate_daily'
  JOIN platform.organizations o ON o.id=t.organization_id AND o.is_test
  WHERE (t.action='daily_review' AND t.agent_id IN ('customer','sales','product_inventory','marketing'))
     OR (t.action='read' AND t.agent_id IN ('finance','analytics'))
     OR (t.action='coordinate_daily' AND t.agent_id='business_manager')
 ), stock AS MATERIALIZED (
  SELECT CASE WHEN platform.inventory_freshness(i.organization_id,i.manual_updated_at)='FRESH'
   THEN i.quantity_on_hand-i.quantity_reserved END AS available
  FROM principal t JOIN platform.products p ON p.organization_id=t.organization_id AND p.status='active'
  JOIN platform.product_variants v ON v.organization_id=t.organization_id AND v.product_id=p.id
  LEFT JOIN platform.inventory i ON i.organization_id=v.organization_id AND i.variant_id=v.id
  WHERE t.agent_id='product_inventory' AND t.business_model IN ('trade','hybrid')
 ), marketing_candidates AS MATERIALIZED (
  SELECT DISTINCT ON (c.id) c.id AS conversation_id, f.name AS offering_name
  FROM principal t JOIN platform.conversations c ON c.organization_id=t.organization_id
   AND c.channel='internal_chat' AND c.status='open' AND c.expires_at>t.cutoff
  JOIN LATERAL (
   SELECT lower(btrim(m.content)) AS decision FROM platform.messages m
   WHERE m.organization_id=c.organization_id AND m.conversation_id=c.id AND m.role='customer' AND m.created_at<=t.cutoff
    AND lower(btrim(m.content)) IN ('согласен получать предложения по услугам','отзываю согласие на предложения по услугам')
   ORDER BY m.created_at DESC,m.id DESC LIMIT 1
  ) consent ON consent.decision='согласен получать предложения по услугам'
  JOIN platform.orders o ON o.organization_id=c.organization_id AND o.conversation_id=c.id AND o.status='completed' AND o.request_kind='service_booking'
  JOIN platform.service_receipts r ON r.organization_id=o.organization_id AND r.order_id=o.id AND r.recorded_at<=t.cutoff
  JOIN platform.service_bookings b ON b.organization_id=o.organization_id AND b.request_id=o.id
  JOIN platform.service_offerings f ON f.organization_id=b.organization_id AND f.id=b.offering_id AND f.status='active'
  WHERE t.agent_id='marketing'
  ORDER BY c.id,r.recorded_at DESC,r.order_id
 )
 SELECT jsonb_build_object('date',t.local_time::date,'timezone',t.timezone,'as_of',t.cutoff,
 'proposals',CASE WHEN t.agent_id='marketing' THEN (SELECT coalesce(jsonb_agg(to_jsonb(x) ORDER BY x.conversation_id),'[]')
  FROM (SELECT * FROM marketing_candidates ORDER BY conversation_id LIMIT 8)x) END,'metrics',
 CASE t.agent_id
 WHEN 'customer' THEN jsonb_build_object(
  'open_conversations',(SELECT count(*) FROM platform.conversations c WHERE c.organization_id=t.organization_id AND c.status='open' AND c.expires_at>t.cutoff),
  'awaiting_staff',(SELECT count(*) FROM platform.conversations c WHERE c.organization_id=t.organization_id AND c.status='open' AND c.expires_at>t.cutoff
   AND EXISTS(SELECT 1 FROM platform.messages m WHERE m.organization_id=c.organization_id AND m.conversation_id=c.id AND m.role='customer'
    AND m.content<>'' AND m.created_at<=t.cutoff
    AND NOT EXISTS(SELECT 1 FROM platform.messages a WHERE a.organization_id=c.organization_id AND a.conversation_id=c.id AND a.role='staff'
     AND a.content<>'' AND a.created_at>=m.created_at AND a.created_at<=t.cutoff))))
 WHEN 'sales' THEN jsonb_build_object(
  'requests_today',(SELECT count(*) FROM platform.orders o WHERE o.organization_id=t.organization_id AND o.created_at<=t.cutoff AND (o.created_at AT TIME ZONE t.timezone)::date=t.local_time::date),
  'pending_confirmation',(SELECT count(*) FROM platform.orders o WHERE o.organization_id=t.organization_id AND o.created_at<=t.cutoff AND o.status='pending_confirmation'),
  'confirmed_open',(SELECT count(*) FROM platform.orders o WHERE o.organization_id=t.organization_id AND o.created_at<=t.cutoff AND o.status='confirmed'))
 WHEN 'product_inventory' THEN jsonb_build_object(
  'stock_zero',(SELECT count(*) FROM stock WHERE available=0),
  'stock_unknown',(SELECT count(*) FROM stock WHERE available IS NULL),
  'unassigned_bookings',(SELECT count(*) FROM platform.service_bookings b JOIN platform.orders o ON o.organization_id=b.organization_id AND o.id=b.request_id
   WHERE b.organization_id=t.organization_id AND o.request_kind='service_booking' AND o.status='confirmed'
   AND b.ends_at>t.cutoff AND (b.starts_at AT TIME ZONE t.timezone)::date=t.local_time::date
   AND NOT EXISTS(SELECT 1 FROM platform.service_assignments a WHERE a.organization_id=b.organization_id AND a.booking_id=b.id)))
 WHEN 'marketing' THEN jsonb_build_object('eligible_service_conversations',(SELECT count(*) FROM marketing_candidates))
 WHEN 'finance' THEN (SELECT jsonb_build_object(
  'paid_services_today',count(*),'received_minor',sum(received_minor),'cost_minor',sum(cost_minor),
  'ai_settled_microrub',(SELECT sum(charged) FROM platform.llm_reservations l WHERE l.organization_id=t.organization_id
   AND l.status='settled' AND l.completed_at<=t.cutoff AND (l.completed_at AT TIME ZONE t.timezone)::date=t.local_time::date),
  'ai_unsettled_count',(SELECT count(*) FROM platform.llm_reservations l WHERE l.organization_id=t.organization_id
   AND l.status IN ('pending','uncertain') AND l.created_at<=t.cutoff AND (l.created_at AT TIME ZONE t.timezone)::date=t.local_time::date))
 FROM platform.service_receipts r WHERE r.organization_id=t.organization_id AND r.recorded_at<=t.cutoff
  AND (r.recorded_at AT TIME ZONE t.timezone)::date=t.local_time::date)
 WHEN 'analytics' THEN (SELECT jsonb_build_object(
  'bookings_today',count(*),'assigned_today',count(a.booking_id) FILTER (WHERE o.status IN ('confirmed','completed')),
  'employees_booked_today',count(DISTINCT a.user_id) FILTER (WHERE o.status IN ('confirmed','completed')),
  'booked_minutes_today',coalesce(sum(extract(epoch FROM b.ends_at-b.starts_at)::bigint / 60) FILTER (WHERE a.booking_id IS NOT NULL AND o.status IN ('confirmed','completed')),0))
 FROM platform.service_bookings b JOIN platform.orders o ON o.organization_id=b.organization_id AND o.id=b.request_id
 LEFT JOIN platform.service_assignments a ON a.organization_id=b.organization_id AND a.booking_id=b.id
 WHERE b.organization_id=t.organization_id AND b.status='confirmed' AND (b.starts_at AT TIME ZONE t.timezone)::date=t.local_time::date)
 ELSE '{}'::jsonb END) FROM principal t
$$;

-- Staff receive a fixed allowlist of operational child results. No financial
-- data, recipient details or parent result can cross this boundary.
CREATE FUNCTION platform.agent_daily_staff(p_org uuid) RETURNS jsonb
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 WITH latest AS (
  SELECT id FROM platform.agent_tasks WHERE organization_id=p_org AND action='coordinate_daily'
  ORDER BY created_at DESC,id LIMIT 1
 )
 SELECT coalesce(jsonb_agg(jsonb_build_object('agent_id',t.agent_id,'status',t.status,'result',e.result) ORDER BY t.agent_id),'[]')
 FROM latest p JOIN platform.agent_tasks t ON t.organization_id=p_org AND t.parent_id=p.id AND t.action='daily_review'
  AND t.agent_id IN ('customer','sales','product_inventory')
 LEFT JOIN LATERAL (SELECT result FROM platform.agent_executions WHERE organization_id=p_org AND task_id=t.id AND status='completed' ORDER BY attempt DESC LIMIT 1) e ON true
 WHERE p_org::text=current_setting('app.organization_id',true) AND platform.is_test_org(p_org) AND platform.can_operate_orders(p_org)
$$;

REVOKE ALL ON FUNCTION platform.agent_daily_submit(uuid,uuid,bytea),platform.agent_daily_children(),
 platform.agent_daily_context(uuid,uuid),platform.agent_daily_staff(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.agent_daily_submit(uuid,uuid,bytea),platform.agent_daily_context(uuid,uuid),
 platform.agent_daily_staff(uuid) TO aba_app;
UPDATE platform.service_metadata SET version=40 WHERE singleton;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM platform.agent_tasks WHERE action IN ('coordinate_daily','daily_review')) THEN
  RAISE EXCEPTION 'rollback refused: preserve daily agent history';
 END IF;
END $$;
DROP TRIGGER agent_daily_children ON platform.agent_tasks;
DROP FUNCTION platform.agent_daily_submit(uuid,uuid,bytea),platform.agent_daily_children(),platform.agent_daily_context(uuid,uuid),platform.agent_daily_staff(uuid);
DROP INDEX platform.agent_daily_single;
ALTER TABLE platform.agent_tasks DROP CONSTRAINT agent_task_operation;
ALTER TABLE platform.agent_tasks ADD CONSTRAINT agent_task_operation CHECK (
 (agent_id IN ('customer','sales','product_inventory') AND action IN ('get_product_price','check_inventory','get_product_details') AND variant_id IS NOT NULL AND conversation_id IS NOT NULL)
 OR (variant_id IS NULL AND conversation_id IS NULL AND ((agent_id='marketing' AND action='draft') OR
 (agent_id IN ('finance','analytics') AND action='read') OR (agent_id='business_manager' AND action IN ('plan','coordinate','coordinate_growth'))))
 OR (variant_id IS NULL AND conversation_id IS NULL AND parent_id IS NOT NULL AND
 ((agent_id='sales' AND action='review_sales') OR (agent_id='product_inventory' AND action='review_inventory')))
 OR (agent_id='customer' AND action IN ('workflow_preview_B','workflow_preview_C','workflow_preview_D','workflow_preview_E')
     AND conversation_id IS NOT NULL AND variant_id IS NULL AND parent_id IS NULL
     AND status IN ('completed','failed') AND correlation_id=id)
 OR (action IN ('delegate_auto_parts','delegate_construction','delegate_service','delegate_real_estate','delegate_property_viewing','delegate_conversation') AND agent_id='business_manager' AND parent_id IS NULL
     AND correlation_id=id AND conversation_id IS NOT NULL AND variant_id IS NULL AND delegation_input IS NOT NULL AND delegation_step IS NULL)
 OR (action LIKE 'delegation.%' AND parent_id IS NOT NULL AND conversation_id IS NOT NULL
     AND variant_id IS NULL AND delegation_step BETWEEN 1 AND 20 AND delegation_input IS NULL)
 OR (conversation_id IS NULL AND variant_id IS NULL AND delegation_input IS NULL AND delegation_step IS NULL
     AND ((agent_id='product_inventory' AND action='workshop_inspect' AND parent_id IS NULL AND correlation_id=id)
       OR (agent_id='product_inventory' AND action IN ('workshop_reserve','workshop_release','workshop_consume') AND parent_id IS NOT NULL AND parent_id=correlation_id)))
 OR (conversation_id IS NULL AND variant_id IS NULL AND delegation_input IS NULL AND (
  (agent_id='business_manager' AND action='delegate_workshop' AND parent_id IS NULL AND correlation_id=id AND delegation_step IS NULL)
  OR (parent_id IS NOT NULL AND parent_id=correlation_id AND (
   (agent_id='customer' AND action='delegation.workshop_customer' AND delegation_step IS NOT DISTINCT FROM 1)
   OR (agent_id='product_inventory' AND action='delegation.workshop_stock' AND delegation_step IS NOT DISTINCT FROM 2)
   OR (agent_id='sales' AND action='delegation.workshop_sales' AND delegation_step IS NOT DISTINCT FROM 3)))))
 OR (conversation_id IS NULL AND variant_id IS NULL AND parent_id IS NOT NULL AND (
  (agent_id='business_manager' AND action='delegate_workshop_finance' AND parent_id=correlation_id
   AND delegation_step IS NULL AND delegation_input IS NOT NULL)
  OR (delegation_input IS NULL AND (
   (agent_id='finance' AND action='delegation.workshop_finance' AND delegation_step IS NOT DISTINCT FROM 1)
   OR (agent_id='analytics' AND action='delegation.workshop_analytics' AND delegation_step IS NOT DISTINCT FROM 2)))))
 OR (conversation_id IS NULL AND variant_id IS NULL AND parent_id IS NOT NULL AND (
  (agent_id='business_manager' AND action='delegate_workshop_marketing' AND parent_id=correlation_id
   AND delegation_step IS NULL AND delegation_input IS NOT NULL)
  OR (agent_id='marketing' AND action='delegation.workshop_marketing'
   AND delegation_step IS NOT DISTINCT FROM 1 AND delegation_input IS NULL))));
CREATE OR REPLACE FUNCTION platform.agent_claim(p_proof bytea,p_lease uuid) RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE q platform.channel_inbox; t platform.agent_tasks; ex uuid:=gen_random_uuid(); previous_execution uuid; apr uuid; reason text; terminal text;
BEGIN
 IF p_lease IS NULL OR octet_length(p_proof) IS DISTINCT FROM 32 THEN RETURN NULL; END IF;
 -- Expire at most one unapproved request per tick, with audit in the same transaction.
 SELECT a.id,a.task_id INTO apr,t.id FROM platform.agent_approvals a JOIN platform.agent_tasks x ON x.id=a.task_id AND x.organization_id=a.organization_id
 WHERE a.status='pending' AND a.expires_at<=clock_timestamp() AND x.worker_hash=sha256(p_proof)
 ORDER BY a.expires_at LIMIT 1 FOR UPDATE OF a SKIP LOCKED;
 IF apr IS NOT NULL THEN
  UPDATE platform.agent_tasks SET status='expired',completed_at=clock_timestamp(),error_code='expired' WHERE id=t.id AND status='waiting_approval' RETURNING * INTO t;
  IF FOUND THEN
   UPDATE platform.agent_approvals SET status='expired' WHERE id=apr;
   INSERT INTO platform.agent_audit(organization_id,task_id,approval_id,agent_id,action,result)
   VALUES(t.organization_id,t.id,apr,t.agent_id,'approval.expire','expired');
  END IF;
 END IF;
 SELECT x.* INTO q FROM platform.channel_inbox x JOIN platform.agent_tasks j ON j.id=x.agent_task_id AND j.organization_id=x.organization_id
 WHERE j.worker_hash=sha256(p_proof)
 AND (j.deadline<=clock_timestamp() OR NOT EXISTS(SELECT 1 FROM platform.agent_tasks c WHERE c.organization_id=j.organization_id AND c.parent_id=j.id AND c.status IN ('pending','queued','running','waiting_approval')))
 AND ((x.status='pending' AND x.available_at<=clock_timestamp()) OR (x.status='processing' AND x.lease_until<=clock_timestamp()))
 ORDER BY j.priority DESC,x.available_at,x.created_at,x.id LIMIT 1 FOR UPDATE OF x SKIP LOCKED;
 IF NOT FOUND THEN RETURN NULL; END IF;
 SELECT * INTO t FROM platform.agent_tasks WHERE id=q.agent_task_id AND organization_id=q.organization_id FOR UPDATE;
 UPDATE platform.agent_executions SET status='interrupted',error_code='interrupted',completed_at=clock_timestamp()
 WHERE task_id=t.id AND status='running' RETURNING id INTO previous_execution;
 IF previous_execution IS NOT NULL THEN
  INSERT INTO platform.agent_audit(organization_id,task_id,execution_id,actor_id,agent_id,action,result,error_code)
  VALUES(t.organization_id,t.id,previous_execution,t.initiator,t.agent_id,'task.interrupt','interrupted','interrupted');
 END IF;
 apr:=NULL;
 IF t.agent_id='marketing' AND t.action<>'delegation.workshop_marketing' THEN
  SELECT id INTO apr FROM platform.agent_approvals WHERE task_id=t.id AND status='approved' AND expires_at>clock_timestamp()
  AND parameters_hash=encode(sha256(convert_to('{"action":"draft"}','UTF8')),'hex');
 END IF;
 IF t.deadline<=clock_timestamp() THEN reason:='expired'; terminal:='expired';
 ELSIF q.attempts>=3 THEN reason:='interrupted';terminal:='failed';
 ELSIF (t.agent_id='marketing' AND t.action<>'delegation.workshop_marketing' AND apr IS NULL) OR NOT EXISTS(
  SELECT 1 FROM platform.organization_members m JOIN platform.organizations o ON o.id=m.organization_id
  WHERE m.organization_id=t.organization_id AND m.user_id=t.initiator AND m.role='owner' AND ((o.is_test AND t.action NOT IN ('workshop_inspect','workshop_reserve','workshop_release','workshop_consume')) OR platform.workshop_task_allowed(t.organization_id,t.id,t.action)))
 THEN reason:='denied';terminal:='cancelled'; END IF;
 IF terminal IS NOT NULL THEN
  UPDATE platform.channel_inbox SET status='failed',agent_lease=NULL,lease_until=NULL WHERE id=q.id;
  UPDATE platform.agent_tasks SET status=terminal,completed_at=clock_timestamp(),error_code=reason WHERE id=t.id;
  UPDATE platform.agent_approvals SET status=CASE WHEN terminal='expired' THEN 'expired' ELSE 'execution_failed' END WHERE task_id=t.id AND status='approved' RETURNING id INTO apr;
  INSERT INTO platform.agent_audit(organization_id,task_id,approval_id,agent_id,action,result,error_code)
  VALUES(t.organization_id,t.id,apr,t.agent_id,'task.recover',terminal,reason);
  RETURN NULL;
 END IF;
 UPDATE platform.channel_inbox SET status='processing',agent_lease=p_lease,lease_until=least(t.deadline,clock_timestamp()+interval '30 seconds'),attempts=attempts+1 WHERE id=q.id;
 UPDATE platform.agent_tasks SET status='running',started_at=coalesce(started_at,clock_timestamp()),retry_count=q.attempts WHERE id=t.id;
 INSERT INTO platform.agent_executions(id,organization_id,task_id,agent_version,attempt,status)
 VALUES(ex,t.organization_id,t.id,'v1',q.attempts+1,'running');
 INSERT INTO platform.agent_audit(organization_id,task_id,execution_id,approval_id,actor_id,agent_id,action,result)
 VALUES(t.organization_id,t.id,ex,apr,t.initiator,t.agent_id,'task.claim','running');
 RETURN jsonb_build_object('ID',t.id,'OrganizationID',t.organization_id,'Initiator',t.initiator,'AgentID',t.agent_id,
  'LegacyBusinessType',(SELECT business_type FROM platform.organizations WHERE id=t.organization_id),'Action',t.action,'CorrelationID',t.correlation_id,'VariantID',t.variant_id,'ConversationID',t.conversation_id,'ExecutionID',ex,'Deadline',t.deadline);
END $$;
CREATE OR REPLACE FUNCTION platform.agent_lease_task(p_id uuid,p_lease uuid) RETURNS platform.agent_tasks
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT t FROM platform.agent_tasks t JOIN platform.channel_inbox q ON q.agent_task_id=t.id AND q.organization_id=t.organization_id
 JOIN platform.organization_members m ON m.organization_id=t.organization_id AND m.user_id=t.initiator AND m.role='owner'
 JOIN platform.organizations o ON o.id=t.organization_id AND ((o.is_test AND t.action NOT IN ('workshop_inspect','workshop_reserve','workshop_release','workshop_consume')) OR platform.workshop_task_allowed(t.organization_id,t.id,t.action))
 WHERE t.id=p_id AND t.status='running' AND q.agent_lease=p_lease AND q.status='processing'
 AND q.lease_until>clock_timestamp() AND t.deadline>clock_timestamp()
 AND (t.agent_id<>'marketing' OR t.action='delegation.workshop_marketing' OR EXISTS(SELECT 1 FROM platform.agent_approvals a WHERE a.task_id=t.id AND a.status='approved' AND a.expires_at>clock_timestamp()
 AND a.parameters_hash=encode(sha256(convert_to('{"action":"draft"}','UTF8')),'hex')))
$$;
CREATE OR REPLACE FUNCTION platform.agent_child_results(p_task uuid,p_lease uuid) RETURNS jsonb
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT coalesce(jsonb_agg(jsonb_build_object('agent_id',c.agent_id,'status',c.status,'error_code',c.error_code,'result',e.result) ORDER BY c.agent_id),'[]')
 FROM platform.agent_lease_task(p_task,p_lease) t
 JOIN platform.agent_tasks c ON c.organization_id=t.organization_id AND c.parent_id=t.id
 LEFT JOIN LATERAL (SELECT x.result FROM platform.agent_executions x
 WHERE x.organization_id=c.organization_id AND x.task_id=c.id AND x.status='completed' ORDER BY x.attempt DESC LIMIT 1)e ON true
 WHERE t.agent_id='business_manager' AND t.action IN ('coordinate','coordinate_growth')
$$;
UPDATE platform.service_metadata SET version=39 WHERE singleton;
-- +goose StatementEnd
