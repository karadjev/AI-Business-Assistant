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
   AND delegation_step IS NOT DISTINCT FROM 1 AND delegation_input IS NULL))));
CREATE OR REPLACE FUNCTION platform.workshop_task_allowed(p_org uuid,p_task uuid,p_action text) RETURNS boolean
LANGUAGE sql STABLE SET search_path=pg_catalog,platform AS $$
 SELECT platform.workshop_enabled(p_org) AND (EXISTS(SELECT 1 FROM platform.workshop_requests r JOIN platform.agent_tasks t
 ON t.organization_id=r.organization_id WHERE t.organization_id=p_org AND t.id=p_task AND t.action=p_action
 AND ((t.correlation_id=r.id AND p_action IN ('delegate_workshop_marketing','delegation.workshop_marketing')
       AND EXISTS(SELECT 1 FROM platform.agent_tasks parent WHERE parent.organization_id=r.organization_id
         AND parent.id=CASE WHEN p_action='delegate_workshop_marketing' THEN t.id ELSE t.parent_id END
         AND parent.parent_id=r.id AND parent.action='delegate_workshop_marketing'))
 OR (t.correlation_id=r.id AND p_action IN ('delegate_workshop_finance','delegation.workshop_finance','delegation.workshop_analytics')
       AND EXISTS(SELECT 1 FROM platform.agent_tasks parent WHERE parent.organization_id=r.organization_id
         AND parent.id=CASE WHEN p_action='delegate_workshop_finance' THEN t.id ELSE t.parent_id END
         AND parent.parent_id=r.id AND parent.action='delegate_workshop_finance'))
 OR (t.id=r.id AND p_action='delegate_workshop') OR (t.parent_id=r.id AND p_action IN ('delegation.workshop_customer','delegation.workshop_stock','delegation.workshop_sales')))) OR p_action IN ('workshop_inspect','workshop_reserve','workshop_release','workshop_consume')
 AND EXISTS(SELECT 1 FROM platform.workshop_requests r WHERE r.organization_id=p_org
 AND p_task=CASE p_action WHEN 'workshop_inspect' THEN r.id WHEN 'workshop_reserve' THEN r.reserve_task_id ELSE r.final_task_id END))
$$;
CREATE OR REPLACE FUNCTION platform.workshop_history(p_org uuid,p_task uuid) RETURNS boolean
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT coalesce(p_org::text=current_setting('app.organization_id',true) AND platform.has_membership(p_org,true),false)
 AND EXISTS(SELECT 1 FROM platform.workshop_requests r JOIN platform.agent_tasks t ON t.organization_id=r.organization_id
 AND (t.id IN (r.id,r.reserve_task_id) OR (t.correlation_id=r.id AND t.action IN ('delegate_workshop_finance','delegation.workshop_finance','delegation.workshop_analytics','delegate_workshop_marketing','delegation.workshop_marketing')) OR (t.parent_id=r.id AND t.action IN ('workshop_release','workshop_consume','delegation.workshop_customer','delegation.workshop_stock','delegation.workshop_sales')))
 WHERE r.organization_id=p_org AND t.id=p_task)
$$;
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

ALTER TABLE platform.workshop_customers ADD COLUMN marketing_consent_version integer NOT NULL DEFAULT 0 CHECK(marketing_consent_version>=0),
 ADD COLUMN marketing_consent_ref text CHECK(length(btrim(marketing_consent_ref)) BETWEEN 1 AND 120);

-- Track only an internal evidence reference, never contact details or an inferred opt-in.
CREATE FUNCTION platform.workshop_record_consent(p_org uuid,p_request uuid,p_granted boolean,p_ref text)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE customer platform.workshop_customers; target uuid; uid uuid:=platform.current_user_id();
BEGIN
 IF NOT platform.workshop_owner(p_org) OR p_granted IS NULL THEN RETURN false; END IF;
 SELECT customer_id INTO target FROM platform.workshop_requests WHERE organization_id=p_org AND id=p_request;
 IF NOT FOUND THEN RETURN false; END IF;
 IF (p_granted AND (p_ref IS NULL OR length(btrim(p_ref)) NOT BETWEEN 1 AND 120))
 OR (NOT p_granted AND p_ref IS NOT NULL) THEN RETURN false; END IF;
 SELECT * INTO STRICT customer FROM platform.workshop_customers WHERE organization_id=p_org AND id=target FOR UPDATE;
 IF customer.marketing_consent_ref IS NOT DISTINCT FROM p_ref THEN RETURN true; END IF;
 UPDATE platform.workshop_customers SET marketing_consent_ref=p_ref,marketing_consent_version=marketing_consent_version+1
 WHERE organization_id=p_org AND id=target;
 INSERT INTO platform.agent_audit(organization_id,task_id,actor_id,agent_id,action,result)
 VALUES(p_org,p_request,uid,'marketing','workshop.consent',CASE WHEN p_granted THEN 'granted' ELSE 'revoked' END);
 RETURN true;
END $$;

-- Consent and the financial source version are checked again at materialization
-- and approval, with the same lock that serializes owner consent changes.
CREATE FUNCTION platform.workshop_marketing_current(p_org uuid,p_task uuid,p_lock boolean)
RETURNS boolean LANGUAGE plpgsql SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE customer platform.workshop_customers; binding record;
BEGIN
 SELECT r.customer_id,parent.delegation_input AS snapshot,f.report_task_id INTO binding
 FROM platform.agent_tasks t JOIN platform.agent_tasks parent ON parent.organization_id=t.organization_id AND parent.id=t.parent_id
 JOIN platform.workshop_requests r ON r.organization_id=t.organization_id AND r.id=parent.parent_id
 JOIN platform.workshop_financials f ON f.organization_id=r.organization_id AND f.request_id=r.id
 WHERE t.organization_id=p_org AND t.id=p_task AND t.action='delegation.workshop_marketing'
 AND parent.action='delegate_workshop_marketing' AND r.close_kind='consume';
 IF NOT FOUND OR NOT platform.workshop_enabled(p_org) THEN RETURN false; END IF;
 IF p_lock THEN
  SELECT * INTO STRICT customer FROM platform.workshop_customers WHERE organization_id=p_org AND id=binding.customer_id FOR SHARE;
  PERFORM 1 FROM platform.workshop_financials WHERE organization_id=p_org AND request_id=(binding.snapshot->>'request_id')::uuid
   AND report_task_id=binding.report_task_id FOR SHARE;
  IF NOT FOUND THEN RETURN false; END IF;
 ELSE
  SELECT * INTO STRICT customer FROM platform.workshop_customers WHERE organization_id=p_org AND id=binding.customer_id;
 END IF;
 RETURN customer.marketing_consent_ref IS NOT NULL
  AND customer.marketing_consent_version::text=binding.snapshot->>'consent_version'
  AND binding.report_task_id::text=binding.snapshot->>'financial_task';
END $$;

CREATE FUNCTION platform.workshop_marketing_read(p_org uuid,p_task uuid) RETURNS boolean
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT platform.workshop_owner(p_org) AND platform.workshop_marketing_current(p_org,p_task,false)
$$;
CREATE POLICY workshop_owner_read ON platform.marketing_drafts FOR SELECT TO aba_app USING(platform.workshop_marketing_read(organization_id,id));

CREATE FUNCTION platform.workshop_request_marketing(p_org uuid,p_request uuid,p_hash bytea)
RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE r platform.workshop_requests; c platform.workshop_customers; f platform.workshop_financials;
 uid uuid:=platform.current_user_id(); task uuid:=gen_random_uuid(); child uuid:=gen_random_uuid(); existing uuid; snapshot jsonb;
 due timestamptz:=clock_timestamp()+interval '30 minutes';
BEGIN
 IF NOT platform.workshop_owner(p_org) THEN RAISE EXCEPTION 'workshop owner required' USING ERRCODE='42501'; END IF;
 PERFORM pg_advisory_xact_lock(hashtextextended(p_org::text,8233));
 SELECT * INTO r FROM platform.workshop_requests WHERE organization_id=p_org AND id=p_request AND close_kind='consume' FOR SHARE;
 IF NOT FOUND THEN RETURN NULL; END IF;
 SELECT * INTO STRICT c FROM platform.workshop_customers WHERE organization_id=p_org AND id=r.customer_id FOR SHARE;
 IF c.marketing_consent_ref IS NULL THEN RETURN NULL; END IF;
 SELECT * INTO f FROM platform.workshop_financials WHERE organization_id=p_org AND request_id=r.id FOR SHARE;
 IF NOT FOUND OR NOT EXISTS(SELECT 1 FROM platform.agent_executions e WHERE e.organization_id=p_org AND e.task_id=f.report_task_id
  AND e.status='completed' AND e.result->0->>'state'='financial_report_ready') THEN RETURN NULL; END IF;
 snapshot:=jsonb_build_object('request_id',r.id,'consent_version',c.marketing_consent_version,'financial_task',f.report_task_id);
 SELECT id INTO existing FROM platform.agent_tasks WHERE organization_id=p_org AND parent_id=r.id AND action='delegate_workshop_marketing'
  AND delegation_input=snapshot AND status NOT IN ('failed','cancelled','expired') ORDER BY created_at DESC LIMIT 1;
 IF FOUND THEN RETURN existing; END IF;
 IF (SELECT count(*) FROM platform.agent_tasks WHERE organization_id=p_org AND created_at>clock_timestamp()-interval '1 minute')>18
  THEN RAISE EXCEPTION 'agent task rate limit' USING ERRCODE='54000'; END IF;
 INSERT INTO platform.agent_tasks(id,organization_id,initiator,agent_id,parent_id,correlation_id,action,idempotency_key,worker_hash,status,deadline,delegation_input)
 VALUES(task,p_org,uid,'business_manager',r.id,r.id,'delegate_workshop_marketing',task,p_hash,'queued',due,snapshot);
 INSERT INTO platform.agent_tasks(id,organization_id,initiator,agent_id,parent_id,correlation_id,action,idempotency_key,worker_hash,status,deadline,delegation_step)
 VALUES(child,p_org,uid,'marketing',task,r.id,'delegation.workshop_marketing',child,p_hash,'queued',due,1);
 INSERT INTO platform.channel_inbox(id,organization_id,external_id,external_user_id,payload,channel,agent_task_id)
 VALUES(task,p_org,task::text,'','{}','agent',task),(child,p_org,child::text,'','{}','agent',child);
 INSERT INTO platform.agent_audit(organization_id,task_id,actor_id,agent_id,action,result)
 VALUES(p_org,task,uid,'business_manager','task.submit','queued'),(p_org,child,uid,'marketing','task.delegate','queued');
 RETURN task;
END $$;

CREATE FUNCTION platform.workshop_marketing_context(p_task uuid,p_lease uuid) RETURNS jsonb
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE t platform.agent_tasks; r platform.workshop_requests; parent platform.agent_tasks; finance jsonb; result jsonb;
BEGIN
 SELECT * INTO t FROM platform.agent_lease_task(p_task,p_lease);
 IF t.id IS NULL OR t.action NOT IN ('delegate_workshop_marketing','delegation.workshop_marketing')
  OR NOT platform.workshop_task_allowed(t.organization_id,t.id,t.action) THEN RETURN NULL; END IF;
 SELECT * INTO STRICT r FROM platform.workshop_requests WHERE organization_id=t.organization_id AND id=t.correlation_id;
 IF t.action='delegate_workshop_marketing' THEN
  SELECT (e.result->0)-'text' INTO result
  FROM platform.agent_tasks c JOIN platform.agent_executions e ON e.organization_id=c.organization_id AND e.task_id=c.id AND e.status='completed'
  JOIN platform.marketing_drafts d ON d.organization_id=e.organization_id AND d.id=e.task_id AND d.source_execution_id=e.id
  WHERE c.organization_id=r.organization_id AND c.parent_id=t.id AND c.action='delegation.workshop_marketing'
  AND platform.workshop_marketing_current(c.organization_id,c.id,false);
  RETURN jsonb_build_object('proposal',result);
 END IF;
 IF NOT platform.workshop_marketing_current(t.organization_id,t.id,false) THEN RETURN '{}'::jsonb; END IF;
 SELECT * INTO STRICT parent FROM platform.agent_tasks WHERE organization_id=r.organization_id AND id=t.parent_id;
 SELECT e.result->0->'financial' INTO finance FROM platform.agent_executions e
 WHERE e.organization_id=r.organization_id AND e.task_id=(parent.delegation_input->>'financial_task')::uuid AND e.status='completed';
 IF finance IS NULL OR 3<>(
  SELECT count(*) FROM platform.agent_tasks c JOIN platform.agent_executions e ON e.organization_id=c.organization_id AND e.task_id=c.id AND e.status='completed'
  WHERE c.organization_id=r.organization_id AND c.parent_id=r.id AND (
   (c.agent_id='customer' AND c.action='delegation.workshop_customer' AND e.result->0->>'state'='customer_verified')
   OR (c.agent_id='product_inventory' AND c.action='delegation.workshop_stock' AND e.result->0->>'state'='part_available')
   OR (c.agent_id='sales' AND c.action='delegation.workshop_sales' AND e.result->0->>'state'='approval_required'))
 ) THEN RETURN '{}'::jsonb; END IF;
 RETURN jsonb_build_object('financial',finance,
  'customer_completed',(SELECT count(*) FROM platform.workshop_requests x WHERE x.organization_id=r.organization_id AND x.customer_id=r.customer_id AND x.close_kind='consume'),
  'active_services',(SELECT count(*) FROM platform.service_offerings o WHERE o.organization_id=r.organization_id AND o.status='active'),
  'available_parts',(SELECT count(*) FROM platform.inventory i JOIN platform.product_variants v ON v.organization_id=i.organization_id AND v.id=i.variant_id
    JOIN platform.products p ON p.organization_id=v.organization_id AND p.id=v.product_id AND p.status='active'
    WHERE i.organization_id=r.organization_id AND i.quantity_on_hand>i.quantity_reserved
    AND platform.inventory_freshness(i.organization_id,i.manual_updated_at)='FRESH'));
END $$;
REVOKE ALL ON FUNCTION platform.workshop_marketing_current(uuid,uuid,boolean),platform.workshop_marketing_read(uuid,uuid),
 platform.workshop_marketing_context(uuid,uuid),platform.workshop_request_marketing(uuid,uuid,bytea),
 platform.workshop_record_consent(uuid,uuid,boolean,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.workshop_marketing_read(uuid,uuid),platform.workshop_marketing_context(uuid,uuid),
 platform.workshop_request_marketing(uuid,uuid,bytea),platform.workshop_record_consent(uuid,uuid,boolean,text) TO aba_app;

CREATE OR REPLACE FUNCTION platform.capture_marketing_draft() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE draft jsonb;
BEGIN
 IF NEW.status='completed' AND OLD.status='running' THEN
  draft:=NEW.result->0;
  IF draft->>'external_communication'='false' AND (
     (draft->>'generation'='synthetic_contextual'
      AND EXISTS(SELECT 1 FROM platform.agent_tasks WHERE organization_id=NEW.organization_id AND id=NEW.task_id AND agent_id='marketing' AND action='draft'))
     OR (draft->>'generation'='workshop_contextual' AND draft->>'channel'='internal_review'
         AND platform.workshop_marketing_current(NEW.organization_id,NEW.task_id,true)))
     AND draft ?& ARRAY['goal','audience','channel','text'] THEN
   INSERT INTO platform.marketing_drafts(id,organization_id,source_execution_id,version,goal,audience,channel,content)
   VALUES(NEW.task_id,NEW.organization_id,NEW.id,1,draft->>'goal',draft->>'audience',draft->>'channel',draft->>'text');
   INSERT INTO platform.agent_audit(organization_id,task_id,execution_id,agent_id,action,result)
   VALUES(NEW.organization_id,NEW.task_id,NEW.id,'marketing','draft.persist','pending');
  END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE OR REPLACE FUNCTION platform.marketing_draft_action(p_org uuid,p_id uuid,p_version integer,p_action text,p_content text,p_reason text)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE d platform.marketing_drafts; uid uuid:=platform.current_user_id();
BEGIN
 IF uid IS NULL THEN RETURN false; END IF;
 IF NOT platform.agent_owner(p_org) THEN
  IF p_action NOT IN ('approved','rejected') OR NOT platform.workshop_owner(p_org)
     OR NOT platform.workshop_marketing_current(p_org,p_id,true) OR NOT EXISTS(SELECT 1 FROM platform.agent_tasks x JOIN platform.agent_tasks m ON m.organization_id=x.organization_id AND m.id=x.parent_id WHERE x.organization_id=p_org AND x.id=p_id AND m.status='completed') THEN RETURN false; END IF;
 END IF;
 PERFORM pg_advisory_xact_lock(hashtextextended(p_org::text||p_id::text,30823));
 SELECT * INTO d FROM platform.marketing_drafts WHERE organization_id=p_org AND id=p_id ORDER BY version DESC LIMIT 1 FOR UPDATE;
 IF NOT FOUND OR d.version<>p_version THEN RETURN false; END IF;
 IF p_action IN ('approved','rejected') AND d.status='pending' AND p_content IS NULL
    AND (p_reason IS NULL OR (p_action='rejected' AND length(btrim(p_reason)) BETWEEN 1 AND 500)) THEN
  UPDATE platform.marketing_drafts SET status=p_action,reason=p_reason,decided_by=uid,decided_at=clock_timestamp()
  WHERE organization_id=p_org AND id=p_id AND version=p_version;
 ELSIF p_action='revise' AND d.status='rejected' AND p_reason IS NULL AND length(btrim(p_content)) BETWEEN 1 AND 8000 THEN
  INSERT INTO platform.marketing_drafts(id,organization_id,source_execution_id,version,goal,audience,channel,content)
  VALUES(d.id,d.organization_id,d.source_execution_id,d.version+1,d.goal,d.audience,d.channel,p_content);
 ELSE RETURN false;
 END IF;
 INSERT INTO platform.agent_audit(organization_id,task_id,execution_id,actor_id,agent_id,action,result)
 VALUES(p_org,p_id,d.source_execution_id,uid,'marketing','draft.review',p_action);
 RETURN true;
END $$;
UPDATE platform.service_metadata SET version=35 WHERE singleton;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION platform.capture_marketing_draft() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE draft jsonb;
BEGIN
 IF NEW.status='completed' AND OLD.status='running' THEN
  draft:=NEW.result->0;
  IF draft->>'generation'='synthetic_contextual' AND draft->>'external_communication'='false'
     AND EXISTS(SELECT 1 FROM platform.agent_tasks WHERE organization_id=NEW.organization_id AND id=NEW.task_id AND agent_id='marketing' AND action='draft')
     AND draft ?& ARRAY['goal','audience','channel','text'] THEN
   INSERT INTO platform.marketing_drafts(id,organization_id,source_execution_id,version,goal,audience,channel,content)
   VALUES(NEW.task_id,NEW.organization_id,NEW.id,1,draft->>'goal',draft->>'audience',draft->>'channel',draft->>'text');
   INSERT INTO platform.agent_audit(organization_id,task_id,execution_id,agent_id,action,result)
   VALUES(NEW.organization_id,NEW.task_id,NEW.id,'marketing','draft.persist','pending');
  END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE OR REPLACE FUNCTION platform.marketing_draft_action(p_org uuid,p_id uuid,p_version integer,p_action text,p_content text,p_reason text)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE d platform.marketing_drafts; uid uuid:=platform.current_user_id();
BEGIN
 IF NOT platform.agent_owner(p_org) OR uid IS NULL THEN RETURN false; END IF;
 PERFORM pg_advisory_xact_lock(hashtextextended(p_org::text||p_id::text,30823));
 SELECT * INTO d FROM platform.marketing_drafts WHERE organization_id=p_org AND id=p_id ORDER BY version DESC LIMIT 1 FOR UPDATE;
 IF NOT FOUND OR d.version<>p_version THEN RETURN false; END IF;
 IF p_action IN ('approved','rejected') AND d.status='pending' AND p_content IS NULL
    AND (p_reason IS NULL OR (p_action='rejected' AND length(btrim(p_reason)) BETWEEN 1 AND 500)) THEN
  UPDATE platform.marketing_drafts SET status=p_action,reason=p_reason,decided_by=uid,decided_at=clock_timestamp()
  WHERE organization_id=p_org AND id=p_id AND version=p_version;
 ELSIF p_action='revise' AND d.status='rejected' AND p_reason IS NULL AND length(btrim(p_content)) BETWEEN 1 AND 8000 THEN
  INSERT INTO platform.marketing_drafts(id,organization_id,source_execution_id,version,goal,audience,channel,content)
  VALUES(d.id,d.organization_id,d.source_execution_id,d.version+1,d.goal,d.audience,d.channel,p_content);
 ELSE RETURN false;
 END IF;
 INSERT INTO platform.agent_audit(organization_id,task_id,execution_id,actor_id,agent_id,action,result)
 VALUES(p_org,p_id,d.source_execution_id,uid,'marketing','draft.review',p_action);
 RETURN true;
END $$;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM platform.workshop_customers WHERE marketing_consent_version<>0)
 OR EXISTS(SELECT 1 FROM platform.agent_tasks WHERE action='delegate_workshop_marketing') THEN
  RAISE EXCEPTION 'rollback refused: preserve workshop consent and marketing history';
 END IF;
END $$;
DROP POLICY workshop_owner_read ON platform.marketing_drafts;
DROP FUNCTION platform.workshop_marketing_read(uuid,uuid),platform.workshop_marketing_context(uuid,uuid),
 platform.workshop_request_marketing(uuid,uuid,bytea),platform.workshop_record_consent(uuid,uuid,boolean,text),
 platform.workshop_marketing_current(uuid,uuid,boolean);
ALTER TABLE platform.workshop_customers DROP COLUMN marketing_consent_version,DROP COLUMN marketing_consent_ref;
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
   OR (agent_id='analytics' AND action='delegation.workshop_analytics' AND delegation_step IS NOT DISTINCT FROM 2))))));
CREATE OR REPLACE FUNCTION platform.workshop_task_allowed(p_org uuid,p_task uuid,p_action text) RETURNS boolean
LANGUAGE sql STABLE SET search_path=pg_catalog,platform AS $$
 SELECT platform.workshop_enabled(p_org) AND (EXISTS(SELECT 1 FROM platform.workshop_requests r JOIN platform.agent_tasks t
 ON t.organization_id=r.organization_id WHERE t.organization_id=p_org AND t.id=p_task AND t.action=p_action
 AND ((t.correlation_id=r.id AND p_action IN ('delegate_workshop_finance','delegation.workshop_finance','delegation.workshop_analytics')
       AND EXISTS(SELECT 1 FROM platform.agent_tasks parent WHERE parent.organization_id=r.organization_id
         AND parent.id=CASE WHEN p_action='delegate_workshop_finance' THEN t.id ELSE t.parent_id END
         AND parent.parent_id=r.id AND parent.action='delegate_workshop_finance'))
 OR (t.id=r.id AND p_action='delegate_workshop') OR (t.parent_id=r.id AND p_action IN ('delegation.workshop_customer','delegation.workshop_stock','delegation.workshop_sales')))) OR p_action IN ('workshop_inspect','workshop_reserve','workshop_release','workshop_consume')
 AND EXISTS(SELECT 1 FROM platform.workshop_requests r WHERE r.organization_id=p_org
 AND p_task=CASE p_action WHEN 'workshop_inspect' THEN r.id WHEN 'workshop_reserve' THEN r.reserve_task_id ELSE r.final_task_id END))
$$;
CREATE OR REPLACE FUNCTION platform.workshop_history(p_org uuid,p_task uuid) RETURNS boolean
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT coalesce(p_org::text=current_setting('app.organization_id',true) AND platform.has_membership(p_org,true),false)
 AND EXISTS(SELECT 1 FROM platform.workshop_requests r JOIN platform.agent_tasks t ON t.organization_id=r.organization_id
 AND (t.id IN (r.id,r.reserve_task_id) OR (t.correlation_id=r.id AND t.action IN ('delegate_workshop_finance','delegation.workshop_finance','delegation.workshop_analytics')) OR (t.parent_id=r.id AND t.action IN ('workshop_release','workshop_consume','delegation.workshop_customer','delegation.workshop_stock','delegation.workshop_sales')))
 WHERE r.organization_id=p_org AND t.id=p_task)
$$;
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
 IF t.agent_id='marketing' THEN
  SELECT id INTO apr FROM platform.agent_approvals WHERE task_id=t.id AND status='approved' AND expires_at>clock_timestamp()
  AND parameters_hash=encode(sha256(convert_to('{"action":"draft"}','UTF8')),'hex');
 END IF;
 IF t.deadline<=clock_timestamp() THEN reason:='expired'; terminal:='expired';
 ELSIF q.attempts>=3 THEN reason:='interrupted';terminal:='failed';
 ELSIF (t.agent_id='marketing' AND apr IS NULL) OR NOT EXISTS(
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
 AND (t.agent_id<>'marketing' OR EXISTS(SELECT 1 FROM platform.agent_approvals a WHERE a.task_id=t.id AND a.status='approved' AND a.expires_at>clock_timestamp()
 AND a.parameters_hash=encode(sha256(convert_to('{"action":"draft"}','UTF8')),'hex')))
$$;

UPDATE platform.service_metadata SET version=34 WHERE singleton;
-- +goose StatementEnd
