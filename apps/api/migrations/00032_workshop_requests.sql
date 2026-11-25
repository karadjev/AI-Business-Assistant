-- +goose Up
-- +goose StatementBegin
-- Disabled for all organizations until explicitly admitted by the deployment operator.
CREATE TABLE platform.workshop_admissions (
 organization_id uuid PRIMARY KEY REFERENCES platform.organizations(id),
 admitted_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE platform.workshop_customers (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 organization_id uuid NOT NULL REFERENCES platform.organizations(id),
 name text NOT NULL CHECK(length(btrim(name)) BETWEEN 1 AND 120),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(organization_id,id)
);
CREATE TABLE platform.workshop_requests (
 id uuid PRIMARY KEY,
 organization_id uuid NOT NULL REFERENCES platform.organizations(id),
 customer_id uuid NOT NULL,
 variant_id uuid NOT NULL,
 quantity integer NOT NULL CHECK(quantity BETWEEN 1 AND 10),
 vehicle text NOT NULL CHECK(length(btrim(vehicle)) BETWEEN 1 AND 120),
 service text NOT NULL CHECK(length(btrim(service)) BETWEEN 1 AND 160),
 reserve_task_id uuid,
 reserved_at timestamptz,
 final_task_id uuid,
 closed_at timestamptz,
 close_kind text CHECK(close_kind IN ('release','consume')),
 UNIQUE(organization_id,id,variant_id),
 UNIQUE(organization_id,reserve_task_id),
 UNIQUE(organization_id,final_task_id),
 FOREIGN KEY(organization_id,customer_id) REFERENCES platform.workshop_customers(organization_id,id),
 FOREIGN KEY(organization_id,variant_id) REFERENCES platform.product_variants(organization_id,id),
 FOREIGN KEY(organization_id,id) REFERENCES platform.agent_tasks(organization_id,id),
 FOREIGN KEY(organization_id,reserve_task_id) REFERENCES platform.agent_tasks(organization_id,id),
 FOREIGN KEY(organization_id,final_task_id) REFERENCES platform.agent_tasks(organization_id,id),
 CHECK(reserved_at IS NULL OR reserve_task_id IS NOT NULL),
 CHECK((closed_at IS NULL)=(close_kind IS NULL) AND (closed_at IS NULL OR (reserved_at IS NOT NULL AND final_task_id IS NOT NULL)))
);
DO $$
DECLARE tab text;
BEGIN
 FOREACH tab IN ARRAY ARRAY['workshop_admissions','workshop_customers','workshop_requests'] LOOP
  EXECUTE format('ALTER TABLE platform.%I ENABLE ROW LEVEL SECURITY',tab);
  EXECUTE format('ALTER TABLE platform.%I FORCE ROW LEVEL SECURITY',tab);
  EXECUTE format('CREATE POLICY migration_owner ON platform.%I TO aba_migrator USING(true) WITH CHECK(true)',tab);
 END LOOP;
END $$;

CREATE FUNCTION platform.workshop_enabled(p_org uuid) RETURNS boolean
LANGUAGE sql STABLE SET search_path=pg_catalog,platform AS $$
 SELECT EXISTS(SELECT 1 FROM platform.workshop_admissions a JOIN platform.organizations o ON o.id=a.organization_id
 WHERE a.organization_id=p_org AND NOT o.is_test AND o.industry_profile='vehicle_service' AND o.business_model='hybrid')
$$;
CREATE FUNCTION platform.workshop_owner(p_org uuid) RETURNS boolean
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT coalesce(p_org::text=current_setting('app.organization_id',true) AND platform.has_membership(p_org,true)
 AND platform.workshop_enabled(p_org),false)
$$;
CREATE FUNCTION platform.workshop_task_allowed(p_org uuid,p_task uuid,p_action text) RETURNS boolean
LANGUAGE sql STABLE SET search_path=pg_catalog,platform AS $$
 SELECT p_action IN ('workshop_inspect','workshop_reserve','workshop_release','workshop_consume') AND platform.workshop_enabled(p_org)
 AND EXISTS(SELECT 1 FROM platform.workshop_requests r WHERE r.organization_id=p_org
 AND p_task=CASE p_action WHEN 'workshop_inspect' THEN r.id WHEN 'workshop_reserve' THEN r.reserve_task_id ELSE r.final_task_id END)
$$;
CREATE POLICY owner_read ON platform.workshop_customers FOR SELECT TO aba_app USING(platform.workshop_owner(organization_id));
CREATE POLICY owner_insert ON platform.workshop_customers FOR INSERT TO aba_app WITH CHECK(platform.workshop_owner(organization_id));
CREATE POLICY owner_read ON platform.workshop_requests FOR SELECT TO aba_app USING(platform.workshop_owner(organization_id));
GRANT SELECT,INSERT ON platform.workshop_customers TO aba_app;
GRANT SELECT ON platform.workshop_requests TO aba_app;
-- Visibility remains tenant- and task-scoped even after admission is revoked,
-- so previously approved actions can be reconciled by their owner.
CREATE FUNCTION platform.workshop_history(p_org uuid,p_task uuid) RETURNS boolean
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT coalesce(p_org::text=current_setting('app.organization_id',true) AND platform.has_membership(p_org,true),false)
 AND EXISTS(SELECT 1 FROM platform.workshop_requests r JOIN platform.agent_tasks t ON t.organization_id=r.organization_id
 AND (t.id IN (r.id,r.reserve_task_id) OR (t.parent_id=r.id AND t.action IN ('workshop_release','workshop_consume')))
 WHERE r.organization_id=p_org AND t.id=p_task)
$$;
CREATE POLICY workshop_read ON platform.agent_tasks FOR SELECT TO aba_app USING(platform.workshop_history(organization_id,id));
CREATE POLICY workshop_read ON platform.agent_executions FOR SELECT TO aba_app USING(platform.workshop_history(organization_id,task_id));
CREATE POLICY workshop_read ON platform.agent_approvals FOR SELECT TO aba_app USING(platform.workshop_history(organization_id,task_id));
CREATE POLICY workshop_read ON platform.agent_audit FOR SELECT TO aba_app USING(platform.workshop_history(organization_id,task_id));

ALTER TABLE platform.agent_approvals DROP CONSTRAINT agent_approvals_parameters_check;
ALTER TABLE platform.agent_approvals ADD CONSTRAINT agent_approvals_parameters_check CHECK(
 parameters='{"action":"draft"}'::jsonb OR (parameters->>'action' IN ('workshop_reserve','workshop_release','workshop_consume')
 AND parameters ?& ARRAY['request_id','variant_id','quantity']));
ALTER TABLE platform.inventory_movements ADD COLUMN workshop_request_id uuid,
 ADD FOREIGN KEY(organization_id,workshop_request_id,variant_id) REFERENCES platform.workshop_requests(organization_id,id,variant_id),
 ADD UNIQUE(organization_id,workshop_request_id,kind);
CREATE UNIQUE INDEX workshop_terminal_movement ON platform.inventory_movements(organization_id,workshop_request_id)
 WHERE workshop_request_id IS NOT NULL AND kind IN ('release','consume');
ALTER TABLE platform.inventory_movements DROP CONSTRAINT movements_kind;
ALTER TABLE platform.inventory_movements ADD CONSTRAINT movements_kind CHECK(
 (workshop_request_id IS NOT NULL AND order_id IS NULL AND (
 (kind='reserve' AND delta=0 AND delta_reserved>0)
 OR (kind='release' AND delta=0 AND delta_reserved<0)
 OR (kind='consume' AND delta=delta_reserved AND delta<0)))
 OR (workshop_request_id IS NULL AND (
 (kind='adjust' AND order_id IS NULL AND delta_reserved=0) OR (kind='reserve' AND order_id IS NOT NULL AND delta=0 AND delta_reserved>0)
 OR (kind='release' AND order_id IS NOT NULL AND delta=0 AND delta_reserved<0)
 OR (kind='consume' AND order_id IS NOT NULL AND delta=delta_reserved AND delta<0))));
CREATE POLICY workshop_worker_insert_only ON platform.inventory_movements AS RESTRICTIVE FOR INSERT TO aba_app
 WITH CHECK(workshop_request_id IS NULL);

-- Normal catalog writes keep reserved quantity unchanged. Only the scoped
-- workshop worker may change it for a non-demo organization.
CREATE FUNCTION platform.workshop_stock_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog,platform,pg_temp AS $$
BEGIN
 IF current_user='aba_app' AND NOT platform.is_test_org(NEW.organization_id) THEN
  IF (TG_OP='INSERT' AND NEW.quantity_reserved<>0)
    OR (TG_OP='UPDATE' AND NEW.quantity_reserved IS DISTINCT FROM OLD.quantity_reserved) THEN
   RAISE EXCEPTION 'real stock reservation requires approved worker operation' USING ERRCODE='42501';
  END IF;
 END IF;
 RETURN NEW;
END $$;
REVOKE ALL ON FUNCTION platform.workshop_stock_guard() FROM PUBLIC;
CREATE TRIGGER workshop_stock_guard BEFORE INSERT OR UPDATE OF quantity_reserved ON platform.inventory
 FOR EACH ROW EXECUTE FUNCTION platform.workshop_stock_guard();

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
       OR (agent_id='product_inventory' AND action IN ('workshop_reserve','workshop_release','workshop_consume') AND parent_id IS NOT NULL AND parent_id=correlation_id))));
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

CREATE FUNCTION platform.workshop_submit(p_org uuid,p_key uuid,p_customer uuid,p_variant uuid,p_quantity int,p_vehicle text,p_service text,p_hash bytea)
RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE uid uuid:=platform.current_user_id(); task uuid; r platform.workshop_requests;
BEGIN
 IF NOT platform.workshop_owner(p_org) THEN RAISE EXCEPTION 'workshop owner required' USING ERRCODE='42501'; END IF;
 PERFORM pg_advisory_xact_lock(hashtextextended(p_org::text,8231));
 SELECT id INTO task FROM platform.agent_tasks WHERE organization_id=p_org AND initiator=uid AND idempotency_key=p_key;
 IF FOUND THEN
  SELECT * INTO r FROM platform.workshop_requests WHERE organization_id=p_org AND id=task;
  IF NOT FOUND OR (r.customer_id,r.variant_id,r.quantity,r.vehicle,r.service)
      IS DISTINCT FROM (p_customer,p_variant,p_quantity,p_vehicle,p_service) THEN
   RAISE EXCEPTION 'idempotency conflict' USING ERRCODE='23505';
  END IF;
  RETURN task;
 END IF;
 IF (SELECT count(*) FROM platform.agent_tasks WHERE organization_id=p_org AND created_at>clock_timestamp()-interval '1 minute')>=20
 THEN RAISE EXCEPTION 'agent task rate limit' USING ERRCODE='54000'; END IF;
 IF NOT EXISTS(SELECT 1 FROM platform.workshop_customers WHERE organization_id=p_org AND id=p_customer)
 OR NOT EXISTS(SELECT 1 FROM platform.product_variants WHERE organization_id=p_org AND id=p_variant)
 THEN RAISE EXCEPTION 'workshop source unavailable' USING ERRCODE='42501'; END IF;
 task:=gen_random_uuid();
 INSERT INTO platform.agent_tasks(id,organization_id,initiator,agent_id,correlation_id,action,idempotency_key,worker_hash,status,deadline)
 VALUES(task,p_org,uid,'product_inventory',task,'workshop_inspect',p_key,p_hash,'queued',clock_timestamp()+interval '30 minutes');
 INSERT INTO platform.workshop_requests(id,organization_id,customer_id,variant_id,quantity,vehicle,service)
 VALUES(task,p_org,p_customer,p_variant,p_quantity,btrim(p_vehicle),btrim(p_service));
 INSERT INTO platform.channel_inbox(id,organization_id,external_id,external_user_id,payload,channel,agent_task_id)
 VALUES(task,p_org,task::text,'','{}','agent',task);
 INSERT INTO platform.agent_audit(organization_id,task_id,actor_id,agent_id,action,result)
 VALUES(p_org,task,uid,'product_inventory','task.submit','queued');
 RETURN task;
END $$;

-- Serialize competing owner requests on the existing workshop request. Keep
-- prior rejected/expired approvals in the audit trail; only the current one can run.
CREATE FUNCTION platform.workshop_close_propose(p_org uuid,p_request uuid,p_action text)
RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE r platform.workshop_requests; previous platform.agent_tasks; source platform.agent_tasks;
 task uuid:=gen_random_uuid(); approval uuid:=gen_random_uuid(); uid uuid:=platform.current_user_id();
 due timestamptz:=clock_timestamp()+interval '30 minutes'; params jsonb;
BEGIN
 IF NOT platform.workshop_owner(p_org) THEN RAISE EXCEPTION 'workshop owner required' USING ERRCODE='42501'; END IF;
 IF p_action IS NULL OR p_action NOT IN ('workshop_release','workshop_consume') THEN
  RAISE EXCEPTION 'invalid workshop operation' USING ERRCODE='23514';
 END IF;
 SELECT * INTO r FROM platform.workshop_requests WHERE organization_id=p_org AND id=p_request FOR UPDATE;
 IF NOT FOUND OR r.reserved_at IS NULL OR r.closed_at IS NOT NULL THEN RETURN NULL; END IF;
 IF r.final_task_id IS NOT NULL THEN
  SELECT * INTO STRICT previous FROM platform.agent_tasks WHERE organization_id=p_org AND id=r.final_task_id;
  IF previous.status IN ('waiting_approval','queued','running') THEN
   IF previous.action=p_action AND previous.deadline>clock_timestamp() THEN RETURN previous.id; END IF;
   RETURN NULL;
  END IF;
 END IF;
 PERFORM pg_advisory_xact_lock(hashtextextended(p_org::text,8231));
 IF (SELECT count(*) FROM platform.agent_tasks WHERE organization_id=p_org AND created_at>clock_timestamp()-interval '1 minute')>=20
 THEN RAISE EXCEPTION 'agent task rate limit' USING ERRCODE='54000'; END IF;
 SELECT * INTO STRICT source FROM platform.agent_tasks WHERE organization_id=p_org AND id=r.reserve_task_id AND status='completed';
 params:=jsonb_build_object('action',p_action,'request_id',r.id,'variant_id',r.variant_id,'quantity',r.quantity);
 INSERT INTO platform.agent_tasks(id,organization_id,initiator,agent_id,parent_id,correlation_id,action,idempotency_key,worker_hash,status,deadline)
 VALUES(task,p_org,uid,source.agent_id,r.id,r.id,p_action,task,source.worker_hash,'waiting_approval',due);
 UPDATE platform.workshop_requests SET final_task_id=task WHERE organization_id=p_org AND id=r.id;
 INSERT INTO platform.agent_approvals(id,organization_id,task_id,parameters,parameters_hash,expires_at,status)
 VALUES(approval,p_org,task,params,encode(sha256(convert_to(params::text,'UTF8')),'hex'),due,'pending');
 INSERT INTO platform.agent_audit(organization_id,task_id,approval_id,actor_id,agent_id,action,result)
 VALUES(p_org,task,approval,uid,source.agent_id,'approval.request','pending');
 RETURN task;
END $$;

CREATE FUNCTION platform.workshop_decide(p_org uuid,p_approval uuid,p_decision text,p_hash text)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE a platform.agent_approvals; t platform.agent_tasks; uid uuid:=platform.current_user_id();
BEGIN
 IF NOT platform.workshop_owner(p_org) THEN RAISE EXCEPTION 'workshop owner required' USING ERRCODE='42501'; END IF;
 SELECT x.* INTO a FROM platform.agent_approvals x JOIN platform.workshop_requests r
 ON r.organization_id=x.organization_id AND x.task_id IN (r.reserve_task_id,r.final_task_id)
 WHERE x.organization_id=p_org AND x.id=p_approval FOR UPDATE OF x;
 IF NOT FOUND OR p_decision IS NULL OR p_decision NOT IN ('approved','rejected') OR a.status<>'pending'
 OR a.parameters_hash IS DISTINCT FROM p_hash OR a.expires_at<=clock_timestamp() THEN RETURN false; END IF;
 SELECT * INTO STRICT t FROM platform.agent_tasks WHERE organization_id=p_org AND id=a.task_id FOR UPDATE;
 IF t.status<>'waiting_approval' OR t.deadline<=clock_timestamp() OR t.action NOT IN ('workshop_reserve','workshop_release','workshop_consume') THEN RETURN false; END IF;
 UPDATE platform.agent_approvals SET status=p_decision,approved_by=CASE WHEN p_decision='approved' THEN uid END,
 decided_by=uid,decided_at=clock_timestamp(),rejection_reason=CASE WHEN p_decision='rejected' THEN 'owner_rejected' END WHERE id=a.id;
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

-- One transaction owns stock, movement, result, approval consumption and audit.
-- It shares the normal lease and finish functions and accepts no caller-supplied business parameters.
CREATE FUNCTION platform.workshop_execute(p_task uuid,p_lease uuid,p_execution uuid)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE t platform.agent_tasks; r platform.workshop_requests; a platform.agent_approvals; stock platform.inventory;
 move_kind text; delta_on int; available int; part_name text; sku text; product_id uuid; variant_version bigint; payload jsonb; params jsonb; reserve_id uuid; approval_id uuid;
 due timestamptz; state text; failure text;
BEGIN
 PERFORM 1 FROM platform.channel_inbox WHERE id=p_task AND agent_task_id=p_task AND agent_lease=p_lease
 AND status='processing' AND lease_until>clock_timestamp() FOR UPDATE;
 IF NOT FOUND THEN RETURN false; END IF;
 SELECT * INTO t FROM platform.agent_lease_task(p_task,p_lease);
 IF t.id IS NULL OR t.action NOT IN ('workshop_inspect','workshop_reserve','workshop_release','workshop_consume') OR NOT EXISTS(
 SELECT 1 FROM platform.agent_executions WHERE organization_id=t.organization_id AND task_id=t.id AND id=p_execution AND status='running')
 THEN RETURN false; END IF;
 -- Hold the admission, profile and actor permissions through commit, including
 -- a concurrent membership, profile or pilot-admission revocation.
 PERFORM 1 FROM platform.workshop_admissions wa JOIN platform.organizations o ON o.id=wa.organization_id
 JOIN platform.organization_members m ON m.organization_id=o.id
 WHERE wa.organization_id=t.organization_id AND NOT o.is_test AND o.industry_profile='vehicle_service'
 AND o.business_model='hybrid' AND m.user_id=t.initiator AND m.role='owner' FOR SHARE OF wa,o,m;
 IF NOT FOUND THEN RETURN false; END IF;
 SELECT * INTO STRICT r FROM platform.workshop_requests WHERE organization_id=t.organization_id
 AND (CASE t.action WHEN 'workshop_inspect' THEN id WHEN 'workshop_reserve' THEN reserve_task_id ELSE final_task_id END)=t.id FOR UPDATE;
 params:=jsonb_build_object('action',CASE WHEN t.action='workshop_inspect' THEN 'workshop_reserve' ELSE t.action END,
  'request_id',r.id,'variant_id',r.variant_id,'quantity',r.quantity);
 IF t.action<>'workshop_inspect' THEN
  SELECT * INTO a FROM platform.agent_approvals WHERE organization_id=r.organization_id AND task_id=t.id FOR UPDATE;
  IF NOT FOUND OR a.status<>'approved' OR a.expires_at<=clock_timestamp() OR NOT a.parameters @> params
  OR a.parameters_hash IS DISTINCT FROM encode(sha256(convert_to(a.parameters::text,'UTF8')),'hex') OR r.closed_at IS NOT NULL
  OR (t.action='workshop_reserve') IS DISTINCT FROM (r.reserved_at IS NULL)
  THEN RETURN false; END IF;
  PERFORM 1 FROM platform.organization_members WHERE organization_id=r.organization_id AND user_id=a.approved_by AND role='owner' FOR SHARE;
  IF NOT FOUND THEN
   RETURN platform.agent_finish(p_task,p_lease,p_execution,NULL,'denied',false);
  END IF;
 END IF;
 IF t.action IN ('workshop_release','workshop_consume') THEN
  move_kind:=CASE t.action WHEN 'workshop_release' THEN 'release' ELSE 'consume' END;
  delta_on:=CASE move_kind WHEN 'release' THEN 0 ELSE -r.quantity END;
  -- These units were held already. A later catalog archive or stale stock
  -- count cannot prevent closing a real reservation.
  SELECT * INTO stock FROM platform.inventory WHERE organization_id=r.organization_id AND variant_id=r.variant_id FOR UPDATE;
  IF NOT FOUND OR stock.quantity_reserved<r.quantity OR stock.quantity_on_hand+delta_on<0
   OR a.parameters IS DISTINCT FROM params OR a.expires_at<=clock_timestamp()
   OR NOT EXISTS(SELECT 1 FROM platform.inventory_movements m WHERE m.organization_id=r.organization_id
     AND m.workshop_request_id=r.id AND m.variant_id=r.variant_id AND m.kind='reserve' AND m.delta_reserved=r.quantity)
   OR EXISTS(SELECT 1 FROM platform.inventory_movements m WHERE m.organization_id=r.organization_id
     AND m.workshop_request_id=r.id AND m.kind IN ('release','consume')) THEN
   state:='reservation_conflict';
   failure:='denied';
  ELSE
   PERFORM set_config('app.catalog_stock_move','true',true);
   UPDATE platform.inventory SET quantity_reserved=stock.quantity_reserved-r.quantity,
    quantity_on_hand=stock.quantity_on_hand+delta_on,version=version+1,updated_at=now()
   WHERE organization_id=r.organization_id AND variant_id=r.variant_id;
   PERFORM set_config('app.catalog_stock_move','false',true);
   INSERT INTO platform.inventory_movements(id,organization_id,variant_id,actor_id,reason,quantity_before,quantity_after,delta,
    inventory_version,kind,reserved_before,reserved_after,delta_reserved,workshop_request_id)
   VALUES(gen_random_uuid(),r.organization_id,r.variant_id,a.approved_by,
    CASE move_kind WHEN 'release' THEN 'Отмена заявки автосервиса' ELSE 'Обслуживание завершено' END,
    stock.quantity_on_hand,stock.quantity_on_hand+delta_on,delta_on,stock.version+1,move_kind,stock.quantity_reserved,
    stock.quantity_reserved-r.quantity,-r.quantity,r.id);
   UPDATE platform.workshop_requests SET closed_at=clock_timestamp(),close_kind=move_kind
   WHERE organization_id=r.organization_id AND id=r.id;
   state:=CASE move_kind WHEN 'release' THEN 'released' ELSE 'consumed' END;
  END IF;
 ELSE
  -- Lock catalog identity before inventory, as the existing order stock path does.
  SELECT p.name,v.sku,p.id,v.version INTO part_name,sku,product_id,variant_version FROM platform.product_variants v JOIN platform.products p
  ON p.organization_id=v.organization_id AND p.id=v.product_id
  WHERE v.organization_id=r.organization_id AND v.id=r.variant_id AND p.status='active' FOR SHARE OF p,v;
  IF FOUND THEN
   SELECT * INTO stock FROM platform.inventory WHERE organization_id=r.organization_id AND variant_id=r.variant_id FOR UPDATE;
  END IF;
  state:='part_unavailable';
  IF stock.variant_id IS NOT NULL THEN
   IF platform.inventory_freshness(r.organization_id,stock.manual_updated_at) IS DISTINCT FROM 'FRESH' THEN
    state:='stock_stale';
   ELSE
    available:=stock.quantity_on_hand-stock.quantity_reserved;
    state:=CASE WHEN available>=r.quantity THEN 'available' ELSE 'insufficient_stock' END;
   END IF;
  END IF;
  params:=params||jsonb_build_object('product_id',product_id,'name',part_name,'sku',sku,'variant_version',variant_version);
  IF t.action='workshop_reserve' AND state='available' AND a.parameters IS DISTINCT FROM params THEN
   state:='source_changed';
  END IF;
  IF t.action='workshop_inspect' AND state='available' THEN
   reserve_id:=gen_random_uuid(); approval_id:=gen_random_uuid(); due:=clock_timestamp()+interval '30 minutes';
   INSERT INTO platform.agent_tasks(id,organization_id,initiator,agent_id,parent_id,correlation_id,action,idempotency_key,worker_hash,status,deadline)
   VALUES(reserve_id,r.organization_id,t.initiator,t.agent_id,t.id,t.id,'workshop_reserve',reserve_id,t.worker_hash,'waiting_approval',due);
   UPDATE platform.workshop_requests SET reserve_task_id=reserve_id WHERE id=r.id;
   INSERT INTO platform.agent_approvals(id,organization_id,task_id,parameters,parameters_hash,expires_at,status)
   VALUES(approval_id,r.organization_id,reserve_id,params,encode(sha256(convert_to(params::text,'UTF8')),'hex'),due,'pending');
   INSERT INTO platform.agent_audit(organization_id,task_id,approval_id,actor_id,agent_id,action,result)
   VALUES(r.organization_id,reserve_id,approval_id,t.initiator,t.agent_id,'approval.request','pending');
   state:='approval_required';
  ELSIF t.action='workshop_reserve' THEN
   IF state='available' AND a.expires_at>clock_timestamp() THEN
    UPDATE platform.inventory SET quantity_reserved=stock.quantity_reserved+r.quantity,version=version+1,updated_at=now()
    WHERE organization_id=r.organization_id AND variant_id=r.variant_id;
    INSERT INTO platform.inventory_movements(id,organization_id,variant_id,actor_id,reason,quantity_before,quantity_after,delta,
     inventory_version,kind,reserved_before,reserved_after,delta_reserved,workshop_request_id)
    VALUES(gen_random_uuid(),r.organization_id,r.variant_id,a.approved_by,'Резерв по заявке автосервиса',
     stock.quantity_on_hand,stock.quantity_on_hand,0,stock.version+1,'reserve',stock.quantity_reserved,
     stock.quantity_reserved+r.quantity,r.quantity,r.id);
    UPDATE platform.workshop_requests SET reserved_at=clock_timestamp() WHERE id=r.id;
    state:='reserved';
   ELSE
    failure:='denied';
   END IF;
  END IF;
 END IF;
 payload:=jsonb_build_array(jsonb_build_object('state',state,'request_id',r.id,'part_name',part_name,'sku',sku,
 'available',available,'quantity',r.quantity,'compatibility','selected_by_owner'));
 IF NOT platform.agent_audit_tool(p_task,p_lease,p_execution,'started',NULL) THEN
  RAISE EXCEPTION 'workshop lease expired' USING ERRCODE='40001';
 END IF;
 IF NOT platform.agent_audit_tool(p_task,p_lease,p_execution,
  CASE WHEN failure IS NULL THEN 'succeeded' ELSE 'failed' END,failure) THEN
  RAISE EXCEPTION 'workshop lease expired' USING ERRCODE='40001';
 END IF;
 IF NOT platform.agent_finish(p_task,p_lease,p_execution,payload,failure,false) THEN
  RAISE EXCEPTION 'workshop lease expired' USING ERRCODE='40001';
 END IF;
 RETURN true;
END $$;

REVOKE ALL ON FUNCTION platform.workshop_enabled(uuid),platform.workshop_history(uuid,uuid),platform.workshop_owner(uuid),
 platform.workshop_task_allowed(uuid,uuid,text),platform.workshop_submit(uuid,uuid,uuid,uuid,int,text,text,bytea),
 platform.workshop_close_propose(uuid,uuid,text),platform.workshop_decide(uuid,uuid,text,text),platform.workshop_execute(uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.workshop_history(uuid,uuid),platform.workshop_owner(uuid),
 platform.workshop_submit(uuid,uuid,uuid,uuid,int,text,text,bytea),platform.workshop_close_propose(uuid,uuid,text),platform.workshop_decide(uuid,uuid,text,text),
 platform.workshop_execute(uuid,uuid,uuid) TO aba_app;
UPDATE platform.service_metadata SET version=32 WHERE singleton;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM platform.workshop_requests) OR EXISTS(SELECT 1 FROM platform.workshop_customers) THEN
  RAISE EXCEPTION 'rollback refused: preserve workshop customers and request history';
 END IF;
END $$;
DROP FUNCTION platform.workshop_execute(uuid,uuid,uuid),platform.workshop_close_propose(uuid,uuid,text),platform.workshop_decide(uuid,uuid,text,text),
 platform.workshop_submit(uuid,uuid,uuid,uuid,int,text,text,bytea);
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
CREATE OR REPLACE FUNCTION platform.agent_lease_task(p_id uuid,p_lease uuid) RETURNS platform.agent_tasks
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT t FROM platform.agent_tasks t JOIN platform.channel_inbox q ON q.agent_task_id=t.id AND q.organization_id=t.organization_id
 JOIN platform.organization_members m ON m.organization_id=t.organization_id AND m.user_id=t.initiator AND m.role='owner'
 JOIN platform.organizations o ON o.id=t.organization_id AND o.is_test
 WHERE t.id=p_id AND t.status='running' AND q.agent_lease=p_lease AND q.status='processing'
 AND q.lease_until>clock_timestamp() AND t.deadline>clock_timestamp()
 AND (t.agent_id<>'marketing' OR EXISTS(SELECT 1 FROM platform.agent_approvals a WHERE a.task_id=t.id AND a.status='approved' AND a.expires_at>clock_timestamp()
 AND a.parameters_hash=encode(sha256(convert_to('{"action":"draft"}','UTF8')),'hex')))
$$;
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
     AND variant_id IS NULL AND delegation_step BETWEEN 1 AND 20 AND delegation_input IS NULL));

DROP TRIGGER workshop_stock_guard ON platform.inventory;
DROP FUNCTION platform.workshop_stock_guard();
DROP POLICY workshop_worker_insert_only ON platform.inventory_movements;
ALTER TABLE platform.inventory_movements DROP CONSTRAINT movements_kind,DROP COLUMN workshop_request_id;
ALTER TABLE platform.inventory_movements ADD CONSTRAINT movements_kind CHECK(
 (kind='adjust' AND order_id IS NULL AND delta_reserved=0) OR (kind='reserve' AND order_id IS NOT NULL AND delta=0 AND delta_reserved>0)
 OR (kind='release' AND order_id IS NOT NULL AND delta=0 AND delta_reserved<0)
 OR (kind='consume' AND order_id IS NOT NULL AND delta=delta_reserved AND delta<0));
ALTER TABLE platform.agent_approvals DROP CONSTRAINT agent_approvals_parameters_check;
ALTER TABLE platform.agent_approvals ADD CONSTRAINT agent_approvals_parameters_check CHECK(parameters='{"action":"draft"}'::jsonb);
DROP POLICY workshop_read ON platform.agent_tasks;
DROP POLICY workshop_read ON platform.agent_executions;
DROP POLICY workshop_read ON platform.agent_approvals;
DROP POLICY workshop_read ON platform.agent_audit;
DROP TABLE platform.workshop_requests,platform.workshop_customers,platform.workshop_admissions;
DROP FUNCTION platform.workshop_history(uuid,uuid),platform.workshop_owner(uuid),platform.workshop_task_allowed(uuid,uuid,text),platform.workshop_enabled(uuid);
UPDATE platform.service_metadata SET version=31 WHERE singleton;

-- +goose StatementEnd
