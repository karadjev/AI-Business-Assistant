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
   OR (agent_id='sales' AND action='delegation.workshop_sales' AND delegation_step IS NOT DISTINCT FROM 3))))));
CREATE OR REPLACE FUNCTION platform.workshop_task_allowed(p_org uuid,p_task uuid,p_action text) RETURNS boolean
LANGUAGE sql STABLE SET search_path=pg_catalog,platform AS $$
 SELECT platform.workshop_enabled(p_org) AND (EXISTS(SELECT 1 FROM platform.workshop_requests r JOIN platform.agent_tasks t
 ON t.organization_id=r.organization_id WHERE t.organization_id=p_org AND t.id=p_task AND t.action=p_action
 AND ((t.id=r.id AND p_action='delegate_workshop') OR (t.parent_id=r.id AND p_action IN ('delegation.workshop_customer','delegation.workshop_stock','delegation.workshop_sales')))) OR p_action IN ('workshop_inspect','workshop_reserve','workshop_release','workshop_consume')
 AND EXISTS(SELECT 1 FROM platform.workshop_requests r WHERE r.organization_id=p_org
 AND p_task=CASE p_action WHEN 'workshop_inspect' THEN r.id WHEN 'workshop_reserve' THEN r.reserve_task_id ELSE r.final_task_id END))
$$;
CREATE OR REPLACE FUNCTION platform.workshop_history(p_org uuid,p_task uuid) RETURNS boolean
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT coalesce(p_org::text=current_setting('app.organization_id',true) AND platform.has_membership(p_org,true),false)
 AND EXISTS(SELECT 1 FROM platform.workshop_requests r JOIN platform.agent_tasks t ON t.organization_id=r.organization_id
 AND (t.id IN (r.id,r.reserve_task_id) OR (t.parent_id=r.id AND t.action IN ('workshop_release','workshop_consume','delegation.workshop_customer','delegation.workshop_stock','delegation.workshop_sales')))
 WHERE r.organization_id=p_org AND t.id=p_task)
$$;
CREATE OR REPLACE FUNCTION platform.workshop_submit(p_org uuid,p_key uuid,p_customer uuid,p_variant uuid,p_quantity int,p_vehicle text,p_service text,p_hash bytea)
RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE uid uuid:=platform.current_user_id(); task uuid; r platform.workshop_requests; child uuid; step record; deadline timestamptz:=clock_timestamp()+interval '30 minutes';
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
 IF (SELECT count(*) FROM platform.agent_tasks WHERE organization_id=p_org AND created_at>clock_timestamp()-interval '1 minute') >16
 THEN RAISE EXCEPTION 'agent task rate limit' USING ERRCODE='54000'; END IF;
 IF NOT EXISTS(SELECT 1 FROM platform.workshop_customers WHERE organization_id=p_org AND id=p_customer)
 OR NOT EXISTS(SELECT 1 FROM platform.product_variants WHERE organization_id=p_org AND id=p_variant)
 THEN RAISE EXCEPTION 'workshop source unavailable' USING ERRCODE='42501'; END IF;
 task:=gen_random_uuid();
 INSERT INTO platform.agent_tasks(id,organization_id,initiator,agent_id,correlation_id,action,idempotency_key,worker_hash,status,deadline)
 VALUES(task,p_org,uid,'business_manager',task,'delegate_workshop',p_key,p_hash,'queued',deadline);
 INSERT INTO platform.workshop_requests(id,organization_id,customer_id,variant_id,quantity,vehicle,service)
 VALUES(task,p_org,p_customer,p_variant,p_quantity,btrim(p_vehicle),btrim(p_service));
 INSERT INTO platform.channel_inbox(id,organization_id,external_id,external_user_id,payload,channel,agent_task_id)
 VALUES(task,p_org,task::text,'','{}','agent',task);
 INSERT INTO platform.agent_audit(organization_id,task_id,actor_id,agent_id,action,result)
 VALUES(p_org,task,uid,'business_manager','task.submit','queued');
 FOR step IN SELECT * FROM (VALUES(1,'customer','delegation.workshop_customer'),
  (2,'product_inventory','delegation.workshop_stock'),(3,'sales','delegation.workshop_sales')) v(n,agent,action)
 LOOP
  child:=gen_random_uuid();
  INSERT INTO platform.agent_tasks(id,organization_id,initiator,agent_id,parent_id,correlation_id,action,
   idempotency_key,worker_hash,status,deadline,delegation_step)
  VALUES(child,p_org,uid,step.agent,task,task,step.action,child,p_hash,CASE WHEN step.n=1 THEN 'queued' ELSE 'pending' END,deadline,step.n);
  IF step.n=1 THEN
   INSERT INTO platform.channel_inbox(id,organization_id,external_id,external_user_id,payload,channel,agent_task_id)
   VALUES(child,p_org,child::text,'','{}','agent',child);
  END IF;
  INSERT INTO platform.agent_audit(organization_id,task_id,actor_id,agent_id,action,result)
  VALUES(p_org,child,uid,step.agent,'task.delegate',CASE WHEN step.n=1 THEN 'queued' ELSE 'pending' END);
 END LOOP;
 RETURN task;
END $$;

-- The lease determines the tenant, request, capability and projection. Names
-- from customer cards are displayed by the cabinet and do not enter agent logs.
CREATE FUNCTION platform.workshop_delegation_context(p_task uuid,p_lease uuid) RETURNS jsonb
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE t platform.agent_tasks; r platform.workshop_requests; item jsonb; children jsonb;
BEGIN
 SELECT * INTO t FROM platform.agent_lease_task(p_task,p_lease);
 IF t.id IS NULL OR t.action NOT IN ('delegate_workshop','delegation.workshop_customer','delegation.workshop_stock','delegation.workshop_sales')
 OR NOT platform.workshop_task_allowed(t.organization_id,t.id,t.action) THEN RETURN NULL; END IF;
 SELECT * INTO STRICT r FROM platform.workshop_requests WHERE organization_id=t.organization_id AND id=coalesce(t.parent_id,t.id);
 IF t.action='delegation.workshop_customer' THEN
  RETURN jsonb_build_object('customer_verified',EXISTS(SELECT 1 FROM platform.workshop_customers WHERE organization_id=r.organization_id AND id=r.customer_id));
 END IF;
 IF t.action='delegation.workshop_stock' THEN
  SELECT jsonb_build_object('variant_id',v.id,'product_id',p.id,'name',p.name,'sku',v.sku,'variant_version',v.version,
    'available',CASE WHEN platform.inventory_freshness(r.organization_id,i.manual_updated_at)='FRESH' THEN i.quantity_on_hand-i.quantity_reserved END)
  INTO item FROM platform.product_variants v JOIN platform.products p ON p.organization_id=v.organization_id AND p.id=v.product_id
  JOIN platform.inventory i ON i.organization_id=v.organization_id AND i.variant_id=v.id
  WHERE p.organization_id=r.organization_id AND v.id=r.variant_id AND p.status='active';
  RETURN jsonb_build_object('part',item,'quantity',r.quantity);
 END IF;
 SELECT coalesce(jsonb_agg(jsonb_build_object('id',c.id,'agent',c.agent_id,'action',c.action,'status',c.status,
   'error_code',coalesce(c.error_code,''),'result',e.result->0) ORDER BY c.delegation_step),'[]') INTO children
 FROM platform.agent_tasks c LEFT JOIN LATERAL (SELECT result FROM platform.agent_executions WHERE organization_id=c.organization_id
 AND task_id=c.id AND status='completed' ORDER BY attempt DESC LIMIT 1) e ON true
 WHERE c.organization_id=r.organization_id AND c.parent_id=r.id AND c.delegation_step IS NOT NULL
 AND (t.delegation_step IS NULL OR c.delegation_step<t.delegation_step);
 RETURN jsonb_build_object('children',children,'service',r.service,'vehicle',r.vehicle);
END $$;

-- Parent result and the new approval are committed together. The stock
-- executor independently validates the snapshot and approval before writing.
CREATE FUNCTION platform.workshop_manager_finish(p_task uuid,p_lease uuid,p_execution uuid,p_result jsonb) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE t platform.agent_tasks; r platform.workshop_requests; source jsonb; params jsonb; task uuid; approval uuid; due timestamptz;
BEGIN
 PERFORM 1 FROM platform.channel_inbox WHERE id=p_task AND agent_task_id=p_task AND agent_lease=p_lease AND status='processing'
 AND lease_until>clock_timestamp() FOR UPDATE;
 IF NOT FOUND THEN RETURN false; END IF;
 SELECT * INTO t FROM platform.agent_lease_task(p_task,p_lease);
 IF t.id IS NULL OR t.action<>'delegate_workshop' THEN RETURN false; END IF;
 SELECT * INTO STRICT r FROM platform.workshop_requests WHERE organization_id=t.organization_id AND id=t.id FOR UPDATE;
 IF p_result->0->>'state'='approval_required' THEN
  SELECT e.result->0->'workshop_part' INTO source FROM platform.agent_tasks c JOIN platform.agent_executions e
  ON e.organization_id=c.organization_id AND e.task_id=c.id AND e.status='completed'
  WHERE c.organization_id=t.organization_id AND c.parent_id=t.id AND c.action='delegation.workshop_stock' AND c.status='completed';
  IF source IS NULL OR source->>'variant_id' IS DISTINCT FROM r.variant_id::text
   OR coalesce((source->>'available')::int,-1)<r.quantity OR r.reserve_task_id IS NOT NULL OR
   (SELECT count(*) FROM platform.agent_tasks c WHERE c.organization_id=t.organization_id AND c.parent_id=t.id
    AND c.delegation_step IS NOT NULL AND c.status='completed')<>3 THEN RETURN false; END IF;
  task:=gen_random_uuid(); approval:=gen_random_uuid(); due:=clock_timestamp()+interval '30 minutes';
  params:=jsonb_build_object('action','workshop_reserve','request_id',r.id,'variant_id',r.variant_id,'quantity',r.quantity)
    || (source-'variant_id'-'available');
  INSERT INTO platform.agent_tasks(id,organization_id,initiator,agent_id,parent_id,correlation_id,action,idempotency_key,worker_hash,status,deadline)
  VALUES(task,r.organization_id,t.initiator,'product_inventory',t.id,t.id,'workshop_reserve',task,t.worker_hash,'waiting_approval',due);
  UPDATE platform.workshop_requests SET reserve_task_id=task WHERE id=r.id;
  INSERT INTO platform.agent_approvals(id,organization_id,task_id,parameters,parameters_hash,expires_at,status)
  VALUES(approval,r.organization_id,task,params,encode(sha256(convert_to(params::text,'UTF8')),'hex'),due,'pending');
  INSERT INTO platform.agent_audit(organization_id,task_id,approval_id,actor_id,agent_id,action,result)
  VALUES(r.organization_id,task,approval,t.initiator,'product_inventory','approval.request','pending');
 END IF;
 IF NOT platform.agent_finish(p_task,p_lease,p_execution,p_result,NULL,false) THEN
  RAISE EXCEPTION 'workshop manager lease expired' USING ERRCODE='40001';
 END IF;
 RETURN true;
END $$;
REVOKE ALL ON FUNCTION platform.workshop_delegation_context(uuid,uuid),platform.workshop_manager_finish(uuid,uuid,uuid,jsonb) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.workshop_delegation_context(uuid,uuid),platform.workshop_manager_finish(uuid,uuid,uuid,jsonb) TO aba_app;
UPDATE platform.service_metadata SET version=33 WHERE singleton;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM platform.agent_tasks WHERE action='delegate_workshop') THEN
  RAISE EXCEPTION 'rollback refused: preserve workshop delegation history';
 END IF;
END $$;
DROP FUNCTION platform.workshop_manager_finish(uuid,uuid,uuid,jsonb), platform.workshop_delegation_context(uuid,uuid);
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
CREATE OR REPLACE FUNCTION platform.workshop_task_allowed(p_org uuid,p_task uuid,p_action text) RETURNS boolean
LANGUAGE sql STABLE SET search_path=pg_catalog,platform AS $$
 SELECT p_action IN ('workshop_inspect','workshop_reserve','workshop_release','workshop_consume') AND platform.workshop_enabled(p_org)
 AND EXISTS(SELECT 1 FROM platform.workshop_requests r WHERE r.organization_id=p_org
 AND p_task=CASE p_action WHEN 'workshop_inspect' THEN r.id WHEN 'workshop_reserve' THEN r.reserve_task_id ELSE r.final_task_id END)
$$;
CREATE OR REPLACE FUNCTION platform.workshop_history(p_org uuid,p_task uuid) RETURNS boolean
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT coalesce(p_org::text=current_setting('app.organization_id',true) AND platform.has_membership(p_org,true),false)
 AND EXISTS(SELECT 1 FROM platform.workshop_requests r JOIN platform.agent_tasks t ON t.organization_id=r.organization_id
 AND (t.id IN (r.id,r.reserve_task_id) OR (t.parent_id=r.id AND t.action IN ('workshop_release','workshop_consume')))
 WHERE r.organization_id=p_org AND t.id=p_task)
$$;
CREATE OR REPLACE FUNCTION platform.workshop_submit(p_org uuid,p_key uuid,p_customer uuid,p_variant uuid,p_quantity int,p_vehicle text,p_service text,p_hash bytea)
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

UPDATE platform.service_metadata SET version=32 WHERE singleton;
-- +goose StatementEnd
