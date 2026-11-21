-- +goose Up
-- +goose StatementBegin
CREATE TABLE platform.agent_tasks (
 id uuid PRIMARY KEY, organization_id uuid NOT NULL REFERENCES platform.organizations(id),
 initiator uuid NOT NULL REFERENCES platform.users(id), agent_id text NOT NULL,
 parent_id uuid, correlation_id uuid NOT NULL, conversation_id uuid,
 action text NOT NULL, variant_id uuid, idempotency_key uuid NOT NULL,
 worker_hash bytea NOT NULL CHECK(octet_length(worker_hash)=32),
 priority smallint NOT NULL DEFAULT 0 CHECK(priority BETWEEN 0 AND 10),
 status text NOT NULL CHECK(status IN ('pending','queued','running','waiting_approval','completed','failed','cancelled','expired')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), started_at timestamptz, completed_at timestamptz,
 deadline timestamptz NOT NULL, retry_count integer NOT NULL DEFAULT 0 CHECK(retry_count BETWEEN 0 AND 2),
 error_code text CHECK(error_code IN ('execution','interrupted','expired','denied')),
 UNIQUE(organization_id,id), UNIQUE(organization_id,initiator,idempotency_key),
 FOREIGN KEY(organization_id,parent_id) REFERENCES platform.agent_tasks(organization_id,id),
 FOREIGN KEY(organization_id,conversation_id) REFERENCES platform.conversations(organization_id,id),
 CHECK(parent_id IS DISTINCT FROM id),
 CHECK((agent_id IN ('customer','sales','product_inventory') AND action='get_product_price' AND variant_id IS NOT NULL AND conversation_id IS NOT NULL)
    OR (variant_id IS NULL AND conversation_id IS NULL AND ((agent_id='marketing' AND action='draft') OR
        (agent_id IN ('finance','analytics') AND action='read') OR (agent_id='business_manager' AND action IN ('plan','coordinate'))))));
CREATE INDEX agent_tasks_worker ON platform.agent_tasks(worker_hash,status,deadline);
CREATE TABLE platform.agent_executions (
 id uuid PRIMARY KEY, organization_id uuid NOT NULL, task_id uuid NOT NULL,
 agent_version text NOT NULL, attempt integer NOT NULL CHECK(attempt BETWEEN 1 AND 3),
 started_at timestamptz NOT NULL DEFAULT clock_timestamp(), completed_at timestamptz,
 status text NOT NULL CHECK(status IN ('running','completed','failed','interrupted')),
 tool_calls integer NOT NULL DEFAULT 0 CHECK(tool_calls BETWEEN 0 AND 1),
 result jsonb CHECK(octet_length(result::text)<=32768),
 error_code text CHECK(error_code IN ('execution','interrupted','expired','denied')),
 UNIQUE(organization_id,task_id,id), UNIQUE(organization_id,task_id,attempt),
 FOREIGN KEY(organization_id,task_id) REFERENCES platform.agent_tasks(organization_id,id));
CREATE TABLE platform.agent_approvals (
 id uuid PRIMARY KEY, organization_id uuid NOT NULL, task_id uuid NOT NULL,
 parameters jsonb NOT NULL CHECK(parameters='{"action":"draft"}'::jsonb),
 parameters_hash text NOT NULL CHECK(parameters_hash ~ '^[0-9a-f]{64}$'),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), expires_at timestamptz NOT NULL,
 status text NOT NULL CHECK(status IN ('pending','approved','rejected','expired','cancelled','executed','execution_failed')),
 approved_by uuid REFERENCES platform.users(id), decided_by uuid REFERENCES platform.users(id), decided_at timestamptz,
 rejection_reason text CHECK(rejection_reason='owner_rejected'),
 UNIQUE(organization_id,task_id), UNIQUE(organization_id,task_id,id),
 FOREIGN KEY(organization_id,task_id) REFERENCES platform.agent_tasks(organization_id,id));
CREATE TABLE platform.agent_audit (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), organization_id uuid NOT NULL, task_id uuid NOT NULL,
 execution_id uuid, approval_id uuid, actor_id uuid REFERENCES platform.users(id), agent_id text NOT NULL,
 action text NOT NULL CHECK(length(action)<=100), result text NOT NULL CHECK(length(result)<=30),
 error_code text CHECK(length(error_code)<=30), created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(organization_id,task_id) REFERENCES platform.agent_tasks(organization_id,id),
 FOREIGN KEY(organization_id,task_id,execution_id) REFERENCES platform.agent_executions(organization_id,task_id,id),
 FOREIGN KEY(organization_id,task_id,approval_id) REFERENCES platform.agent_approvals(organization_id,task_id,id));
CREATE INDEX agent_audit_filter ON platform.agent_audit(organization_id,agent_id,task_id,created_at DESC,action);

DO $$
DECLARE tab text;
BEGIN
 FOR tab IN SELECT unnest(ARRAY['agent_tasks','agent_executions','agent_approvals','agent_audit']) LOOP
  EXECUTE format('ALTER TABLE platform.%I ENABLE ROW LEVEL SECURITY',tab);
  EXECUTE format('ALTER TABLE platform.%I FORCE ROW LEVEL SECURITY',tab);
  EXECUTE format('CREATE POLICY migration_owner ON platform.%I TO aba_migrator USING(true) WITH CHECK(true)',tab);
  EXECUTE format('CREATE POLICY owner_read ON platform.%I FOR SELECT TO aba_app USING(organization_id::text=current_setting(''app.organization_id'',true) AND platform.has_membership(organization_id,true) AND platform.is_test_org(organization_id))',tab);
 END LOOP;
END $$;
GRANT SELECT (id,organization_id,initiator,agent_id,parent_id,correlation_id,conversation_id,action,variant_id,idempotency_key,priority,status,created_at,started_at,completed_at,deadline,retry_count,error_code) ON platform.agent_tasks TO aba_app;
GRANT SELECT ON platform.agent_executions,platform.agent_approvals,platform.agent_audit TO aba_app;

ALTER TABLE platform.channel_inbox ALTER COLUMN integration_id DROP NOT NULL, ALTER COLUMN external_account_id DROP NOT NULL;
ALTER TABLE platform.channel_inbox ADD COLUMN agent_task_id uuid, ADD COLUMN agent_lease uuid,
 ADD CONSTRAINT agent_job_task_fk FOREIGN KEY(organization_id,agent_task_id) REFERENCES platform.agent_tasks(organization_id,id),
 ADD CONSTRAINT agent_job_identity CHECK(
 (agent_task_id IS NULL AND channel IN ('whatsapp','instagram') AND integration_id IS NOT NULL AND external_account_id IS NOT NULL)
 OR (agent_task_id IS NOT NULL AND agent_task_id=id AND capability IS NULL AND channel='agent' AND integration_id IS NULL AND external_account_id IS NULL AND conversation_id IS NULL AND external_user_id='' AND payload='{}'::jsonb));
CREATE POLICY agent_job_hidden ON platform.channel_inbox AS RESTRICTIVE FOR SELECT TO aba_app USING(agent_task_id IS NULL);
CREATE INDEX channel_inbox_agent_claim ON platform.channel_inbox(status,available_at,lease_until) WHERE agent_task_id IS NOT NULL;
CREATE OR REPLACE FUNCTION platform.channel_job_binding() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE i platform.channel_integrations;
BEGIN
 IF NEW.agent_task_id IS NOT NULL THEN RETURN NEW; END IF;
 SELECT * INTO i FROM platform.channel_integrations WHERE id=NEW.integration_id AND organization_id=NEW.organization_id;
 IF i.id IS NULL OR NEW.channel IS DISTINCT FROM i.channel THEN RAISE EXCEPTION 'job binding rejected' USING ERRCODE='23514';END IF;
 IF NEW.external_account_id IS NULL THEN NEW.external_account_id=i.phone_id;END IF;
 IF NEW.external_account_id IS DISTINCT FROM i.phone_id THEN RAISE EXCEPTION 'account binding rejected' USING ERRCODE='23514';END IF;
 RETURN NEW;
END $$;

CREATE FUNCTION platform.agent_owner(p_org uuid) RETURNS boolean LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT coalesce(p_org::text=current_setting('app.organization_id',true) AND platform.has_membership(p_org,true) AND platform.is_test_org(p_org),false)
$$;
CREATE FUNCTION platform.agent_submit(p_org uuid,p_id uuid,p_key uuid,p_agent text,p_action text,p_variant uuid,p_conv uuid,p_hash bytea) RETURNS uuid
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE uid uuid:=platform.current_user_id(); existing platform.agent_tasks; approval uuid; child uuid; child_agent text; child_action text; due timestamptz:=clock_timestamp()+interval '30 minutes';
BEGIN
 IF NOT platform.agent_owner(p_org) THEN RAISE EXCEPTION 'agent owner required' USING ERRCODE='42501'; END IF;
 PERFORM pg_advisory_xact_lock(hashtextextended(p_org::text,8231));
 SELECT * INTO existing FROM platform.agent_tasks WHERE organization_id=p_org AND initiator=uid AND idempotency_key=p_key;
 IF FOUND THEN
  IF (existing.agent_id,existing.action,existing.variant_id,existing.conversation_id) IS DISTINCT FROM (p_agent,p_action,p_variant,p_conv)
    THEN RAISE EXCEPTION 'idempotency conflict' USING ERRCODE='23505'; END IF;
  RETURN existing.id;
 END IF;
 IF (SELECT count(*) FROM platform.agent_tasks WHERE organization_id=p_org AND created_at>clock_timestamp()-interval '1 minute')>=20 THEN
  RAISE EXCEPTION 'agent task rate limit' USING ERRCODE='54000';
 END IF;
 IF p_conv IS NOT NULL AND NOT EXISTS(SELECT 1 FROM platform.conversations WHERE organization_id=p_org AND id=p_conv AND status='open' AND expires_at>clock_timestamp()) THEN
  RAISE EXCEPTION 'conversation unavailable' USING ERRCODE='42501';
 END IF;
 INSERT INTO platform.agent_tasks(id,organization_id,initiator,agent_id,correlation_id,conversation_id,action,variant_id,idempotency_key,worker_hash,status,deadline)
 VALUES(p_id,p_org,uid,p_agent,p_id,p_conv,p_action,p_variant,p_key,p_hash,CASE WHEN p_agent='marketing' THEN 'waiting_approval' ELSE 'queued' END,due);
 IF p_agent='marketing' THEN
  approval:=gen_random_uuid();
  INSERT INTO platform.agent_approvals(id,organization_id,task_id,parameters,parameters_hash,expires_at,status)
  VALUES(approval,p_org,p_id,'{"action":"draft"}',encode(sha256(convert_to('{"action":"draft"}','UTF8')),'hex'),due,'pending');
 ELSE
  INSERT INTO platform.channel_inbox(id,organization_id,external_id,external_user_id,payload,channel,agent_task_id)
  VALUES(p_id,p_org,p_id::text,'','{}','agent',p_id);
 END IF;
 IF p_agent='business_manager' AND p_action='coordinate' THEN
  FOREACH child_agent IN ARRAY ARRAY['analytics','marketing'] LOOP
   child:=gen_random_uuid();
   child_action:=CASE WHEN child_agent='analytics' THEN 'read' ELSE 'draft' END;
   INSERT INTO platform.agent_tasks(id,organization_id,initiator,agent_id,parent_id,correlation_id,action,idempotency_key,worker_hash,status,deadline)
   VALUES(child,p_org,uid,child_agent,p_id,p_id,child_action,child,p_hash,CASE WHEN child_agent='marketing' THEN 'waiting_approval' ELSE 'queued' END,due);
   IF child_agent='marketing' THEN
    approval:=gen_random_uuid();
    INSERT INTO platform.agent_approvals(id,organization_id,task_id,parameters,parameters_hash,expires_at,status)
    VALUES(approval,p_org,child,'{"action":"draft"}',encode(sha256(convert_to('{"action":"draft"}','UTF8')),'hex'),due,'pending');
   ELSE
    approval:=NULL;
    INSERT INTO platform.channel_inbox(id,organization_id,external_id,external_user_id,payload,channel,agent_task_id)
    VALUES(child,p_org,child::text,'','{}','agent',child);
   END IF;
   INSERT INTO platform.agent_audit(organization_id,task_id,approval_id,actor_id,agent_id,action,result)
   VALUES(p_org,child,approval,uid,child_agent,'task.delegate',CASE WHEN approval IS NULL THEN 'queued' ELSE 'pending' END);
  END LOOP;
  approval:=NULL;
 END IF;
 INSERT INTO platform.agent_audit(organization_id,task_id,approval_id,actor_id,agent_id,action,result)
 VALUES(p_org,p_id,approval,uid,p_agent,'task.submit',CASE WHEN approval IS NULL THEN 'queued' ELSE 'pending' END);
 RETURN p_id;
END $$;

CREATE FUNCTION platform.agent_decide(p_org uuid,p_id uuid,p_decision text,p_hash text) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE a platform.agent_approvals; t platform.agent_tasks; uid uuid:=platform.current_user_id();
BEGIN
 IF NOT platform.agent_owner(p_org) THEN RAISE EXCEPTION 'agent owner required' USING ERRCODE='42501'; END IF;
 SELECT * INTO a FROM platform.agent_approvals WHERE organization_id=p_org AND id=p_id FOR UPDATE;
 IF NOT FOUND OR a.status<>'pending' OR a.expires_at<=clock_timestamp() OR a.parameters_hash IS DISTINCT FROM p_hash
    OR p_decision NOT IN ('approved','rejected') THEN RETURN false; END IF;
 SELECT * INTO t FROM platform.agent_tasks WHERE organization_id=p_org AND id=a.task_id FOR UPDATE;
 IF t.status<>'waiting_approval' OR t.deadline<=clock_timestamp() OR t.agent_id<>'marketing' OR t.action<>'draft' THEN RETURN false; END IF;
 UPDATE platform.agent_approvals SET status=p_decision,approved_by=CASE WHEN p_decision='approved' THEN uid END,decided_by=uid,decided_at=clock_timestamp(),
 rejection_reason=CASE WHEN p_decision='rejected' THEN 'owner_rejected' END WHERE id=a.id;
 UPDATE platform.agent_tasks SET status=CASE WHEN p_decision='approved' THEN 'queued' ELSE 'cancelled' END,
  completed_at=CASE WHEN p_decision='rejected' THEN clock_timestamp() END WHERE id=t.id;
 IF p_decision='approved' THEN
  INSERT INTO platform.channel_inbox(id,organization_id,external_id,external_user_id,payload,channel,agent_task_id)
  VALUES(t.id,p_org,t.id::text,'','{}','agent',t.id);
 END IF;
 INSERT INTO platform.agent_audit(organization_id,task_id,approval_id,actor_id,agent_id,action,result)
 VALUES(p_org,t.id,a.id,uid,t.agent_id,'approval.decision',p_decision);
 RETURN true;
END $$;

CREATE FUNCTION platform.agent_claim(p_proof bytea,p_lease uuid) RETURNS jsonb
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
  WHERE m.organization_id=t.organization_id AND m.user_id=t.initiator AND m.role='owner' AND o.is_test)
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

CREATE FUNCTION platform.agent_lease_task(p_id uuid,p_lease uuid) RETURNS platform.agent_tasks
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT t FROM platform.agent_tasks t JOIN platform.channel_inbox q ON q.agent_task_id=t.id AND q.organization_id=t.organization_id
 JOIN platform.organization_members m ON m.organization_id=t.organization_id AND m.user_id=t.initiator AND m.role='owner'
 JOIN platform.organizations o ON o.id=t.organization_id AND o.is_test
 WHERE t.id=p_id AND t.status='running' AND q.agent_lease=p_lease AND q.status='processing'
 AND q.lease_until>clock_timestamp() AND t.deadline>clock_timestamp()
 AND (t.agent_id<>'marketing' OR EXISTS(SELECT 1 FROM platform.agent_approvals a WHERE a.task_id=t.id AND a.status='approved' AND a.expires_at>clock_timestamp()
 AND a.parameters_hash=encode(sha256(convert_to('{"action":"draft"}','UTF8')),'hex')))
$$;
CREATE FUNCTION platform.agent_catalog_scope(p_org uuid,p_variant uuid DEFAULT NULL,p_product uuid DEFAULT NULL) RETURNS boolean LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT EXISTS(SELECT 1 FROM platform.agent_lease_task(
 nullif(current_setting('app.agent_task',true),'')::uuid,nullif(current_setting('app.agent_lease',true),'')::uuid) t
 WHERE t.organization_id=p_org AND t.agent_id IN ('customer','sales','product_inventory') AND t.action='get_product_price'
 AND p_org::text=current_setting('app.organization_id',true)
 AND (p_variant IS NULL OR p_variant=t.variant_id)
 AND (p_product IS NULL OR EXISTS(SELECT 1 FROM platform.product_variants v WHERE v.organization_id=p_org AND v.id=t.variant_id AND v.product_id=p_product))
 AND EXISTS(SELECT 1 FROM platform.conversations c WHERE c.organization_id=p_org AND c.id=t.conversation_id AND c.status='open' AND c.expires_at>clock_timestamp()))
$$;
CREATE POLICY agent_price_products ON platform.products FOR SELECT TO aba_app USING(CASE WHEN nullif(current_setting('app.agent_lease',true),'') IS NOT NULL THEN status='active' AND platform.agent_catalog_scope(organization_id,NULL,id) ELSE false END);
CREATE POLICY agent_price_variants ON platform.product_variants FOR SELECT TO aba_app USING(CASE WHEN nullif(current_setting('app.agent_lease',true),'') IS NOT NULL THEN platform.agent_catalog_scope(organization_id,id,NULL) ELSE false END);
CREATE POLICY agent_price_inventory ON platform.inventory FOR SELECT TO aba_app USING(CASE WHEN nullif(current_setting('app.agent_lease',true),'') IS NOT NULL THEN platform.agent_catalog_scope(organization_id,variant_id,NULL) ELSE false END);

CREATE FUNCTION platform.agent_metrics_projection(p_task uuid,p_lease uuid) RETURNS jsonb
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT jsonb_build_object('count',count(o.id),'revenue',coalesce(sum(o.total_amount),0))
 FROM platform.agent_lease_task(p_task,p_lease) t
 LEFT JOIN platform.orders o ON o.organization_id=t.organization_id AND o.status='completed' AND o.created_at>=now()-interval '30 days'
 WHERE t.agent_id IN ('analytics','finance') GROUP BY t.id
$$;
CREATE FUNCTION platform.agent_audit_tool(p_task uuid,p_lease uuid,p_execution uuid,p_result text,p_error text) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE t platform.agent_tasks;
BEGIN
 PERFORM 1 FROM platform.channel_inbox WHERE id=p_task AND agent_task_id=p_task AND agent_lease=p_lease FOR UPDATE;
 IF NOT FOUND THEN RETURN false; END IF;
 SELECT * INTO t FROM platform.agent_lease_task(p_task,p_lease);
 IF t.id IS NULL OR p_result IS NULL OR p_result NOT IN ('started','succeeded','failed')
 OR (p_error IS NOT NULL AND p_error NOT IN ('denied','invalid','timeout','canceled','approval','limit','execution'))
 OR NOT EXISTS(SELECT 1 FROM platform.agent_executions WHERE id=p_execution AND task_id=t.id AND status='running')
 THEN RETURN false; END IF;
 INSERT INTO platform.agent_audit(organization_id,task_id,execution_id,approval_id,actor_id,agent_id,action,result,error_code)
 VALUES(t.organization_id,t.id,p_execution,(SELECT id FROM platform.agent_approvals WHERE task_id=t.id),t.initiator,t.agent_id,t.action,p_result,p_error);
 IF p_result='started' THEN
  UPDATE platform.agent_executions SET tool_calls=1 WHERE id=p_execution;
 END IF;
 RETURN true;
END $$;
CREATE FUNCTION platform.agent_child_results(p_task uuid,p_lease uuid) RETURNS jsonb
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT coalesce(jsonb_agg(jsonb_build_object('agent_id',c.agent_id,'status',c.status,'error_code',c.error_code,'result',e.result) ORDER BY c.agent_id),'[]')
 FROM platform.agent_lease_task(p_task,p_lease) t
 JOIN platform.agent_tasks c ON c.organization_id=t.organization_id AND c.parent_id=t.id
 LEFT JOIN LATERAL (SELECT x.result FROM platform.agent_executions x
 WHERE x.organization_id=c.organization_id AND x.task_id=c.id AND x.status='completed' ORDER BY x.attempt DESC LIMIT 1)e ON true
 WHERE t.agent_id='business_manager' AND t.action='coordinate'
$$;
CREATE FUNCTION platform.agent_finish(p_task uuid,p_lease uuid,p_execution uuid,p_result jsonb,p_error text,p_retry boolean) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE q platform.channel_inbox; t platform.agent_tasks; final_status text; apr uuid;
BEGIN
 SELECT * INTO q FROM platform.channel_inbox WHERE id=p_task AND agent_task_id=p_task AND agent_lease=p_lease
 AND status='processing' AND lease_until>clock_timestamp() FOR UPDATE;
 IF NOT FOUND THEN RETURN false; END IF;
 SELECT * INTO t FROM platform.agent_lease_task(p_task,p_lease);
 IF t.id IS NULL THEN RETURN false; END IF;
 IF NOT EXISTS(SELECT 1 FROM platform.agent_executions WHERE id=p_execution AND task_id=t.id AND status='running') THEN RETURN false; END IF;
 IF p_error IS NULL AND (p_result IS NULL OR NOT EXISTS(SELECT 1 FROM platform.agent_audit
 WHERE task_id=t.id AND execution_id=p_execution AND action=t.action AND result='succeeded')) THEN RETURN false; END IF;
 final_status:=CASE WHEN p_error IS NULL THEN 'completed' WHEN p_retry AND q.attempts<3 THEN 'queued' ELSE 'failed' END;
 UPDATE platform.agent_executions SET status=CASE WHEN p_error IS NULL THEN 'completed' ELSE 'failed' END,
 completed_at=clock_timestamp(),result=p_result,error_code=p_error WHERE id=p_execution;
 UPDATE platform.agent_tasks SET status=final_status,error_code=p_error,completed_at=CASE WHEN final_status<>'queued' THEN clock_timestamp() END WHERE id=t.id;
 UPDATE platform.channel_inbox SET status=CASE final_status WHEN 'completed' THEN 'done' WHEN 'queued' THEN 'pending' ELSE 'failed' END,
 agent_lease=NULL,lease_until=NULL,available_at=clock_timestamp()+make_interval(secs=>power(2,q.attempts)::int) WHERE id=q.id;
 UPDATE platform.agent_approvals SET status=CASE final_status WHEN 'completed' THEN 'executed' ELSE 'execution_failed' END
 WHERE task_id=t.id AND status='approved' AND final_status<>'queued' RETURNING id INTO apr;
 INSERT INTO platform.agent_audit(organization_id,task_id,execution_id,approval_id,actor_id,agent_id,action,result,error_code)
 VALUES(t.organization_id,t.id,p_execution,apr,t.initiator,t.agent_id,'task.finish',final_status,p_error);
 RETURN true;
END $$;
REVOKE ALL ON FUNCTION platform.agent_owner(uuid),platform.agent_submit(uuid,uuid,uuid,text,text,uuid,uuid,bytea),
 platform.agent_decide(uuid,uuid,text,text),platform.agent_claim(bytea,uuid),platform.agent_lease_task(uuid,uuid),platform.agent_catalog_scope(uuid,uuid,uuid),
 platform.agent_metrics_projection(uuid,uuid),platform.agent_child_results(uuid,uuid),platform.agent_audit_tool(uuid,uuid,uuid,text,text),platform.agent_finish(uuid,uuid,uuid,jsonb,text,boolean) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.agent_owner(uuid),platform.agent_submit(uuid,uuid,uuid,text,text,uuid,uuid,bytea),
 platform.agent_decide(uuid,uuid,text,text),platform.agent_claim(bytea,uuid),platform.agent_catalog_scope(uuid,uuid,uuid),
 platform.agent_metrics_projection(uuid,uuid),platform.agent_child_results(uuid,uuid),platform.agent_audit_tool(uuid,uuid,uuid,text,text),platform.agent_finish(uuid,uuid,uuid,jsonb,text,boolean) TO aba_app;
UPDATE platform.service_metadata SET version=18 WHERE singleton;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN IF EXISTS(SELECT 1 FROM platform.agent_tasks) THEN RAISE EXCEPTION 'rollback refused: preserve durable agent history'; END IF; END $$;
DROP POLICY agent_price_products ON platform.products;
DROP POLICY agent_price_variants ON platform.product_variants;
DROP POLICY agent_price_inventory ON platform.inventory;
DROP FUNCTION platform.agent_finish(uuid,uuid,uuid,jsonb,text,boolean),platform.agent_audit_tool(uuid,uuid,uuid,text,text),platform.agent_metrics_projection(uuid,uuid),platform.agent_child_results(uuid,uuid),platform.agent_catalog_scope(uuid,uuid,uuid);
DROP FUNCTION platform.agent_lease_task(uuid,uuid),platform.agent_claim(bytea,uuid),platform.agent_decide(uuid,uuid,text,text),platform.agent_submit(uuid,uuid,uuid,text,text,uuid,uuid,bytea),platform.agent_owner(uuid);
CREATE OR REPLACE FUNCTION platform.channel_job_binding() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE i platform.channel_integrations;
BEGIN
 SELECT * INTO i FROM platform.channel_integrations WHERE id=NEW.integration_id AND organization_id=NEW.organization_id;
 IF i.id IS NULL OR NEW.channel IS DISTINCT FROM i.channel THEN RAISE EXCEPTION 'job binding rejected' USING ERRCODE='23514';END IF;
 IF NEW.external_account_id IS NULL THEN NEW.external_account_id=i.phone_id;END IF;
 IF NEW.external_account_id IS DISTINCT FROM i.phone_id THEN RAISE EXCEPTION 'account binding rejected' USING ERRCODE='23514';END IF;
 RETURN NEW;
END $$;
DROP POLICY agent_job_hidden ON platform.channel_inbox;
ALTER TABLE platform.channel_inbox DROP CONSTRAINT agent_job_identity,DROP COLUMN agent_task_id,DROP COLUMN agent_lease,
 ALTER COLUMN integration_id SET NOT NULL,ALTER COLUMN external_account_id SET NOT NULL;
DROP TABLE platform.agent_audit,platform.agent_approvals,platform.agent_executions,platform.agent_tasks;
UPDATE platform.service_metadata SET version=17 WHERE singleton;
-- +goose StatementEnd
