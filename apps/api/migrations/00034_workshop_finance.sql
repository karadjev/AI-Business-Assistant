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
CREATE TABLE platform.workshop_financials (
 organization_id uuid NOT NULL, request_id uuid NOT NULL, report_task_id uuid NOT NULL,
 service_minor bigint NOT NULL CHECK(service_minor BETWEEN 0 AND 9000000000000),
 part_cost_minor bigint NOT NULL CHECK(part_cost_minor BETWEEN 0 AND 9000000000000),
 recorded_by uuid NOT NULL REFERENCES platform.users(id),
 recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 paid_at timestamptz, paid_recorded_by uuid REFERENCES platform.users(id),
 CHECK((paid_at IS NULL)=(paid_recorded_by IS NULL)),
 PRIMARY KEY(organization_id,request_id),
 FOREIGN KEY(organization_id,report_task_id) REFERENCES platform.agent_tasks(organization_id,id),
 FOREIGN KEY(organization_id,request_id) REFERENCES platform.agent_tasks(organization_id,id)
);
ALTER TABLE platform.workshop_financials ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.workshop_financials FORCE ROW LEVEL SECURITY;
CREATE POLICY migration_owner ON platform.workshop_financials TO aba_migrator USING(true) WITH CHECK(true);
CREATE POLICY owner_read ON platform.workshop_financials FOR SELECT TO aba_app USING(platform.workshop_owner(organization_id));
GRANT SELECT ON platform.workshop_financials TO aba_app;

-- One record for one completed request. The owner records actual facts. There is
-- no payment execution and no use of the retail price as inventory cost.
CREATE FUNCTION platform.workshop_record_financials(p_org uuid,p_request uuid,p_service bigint,p_cost bigint,p_paid boolean,p_hash bytea)
RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE r platform.workshop_requests; f platform.workshop_financials; uid uuid:=platform.current_user_id();
 task uuid:=gen_random_uuid(); child uuid; step record; due timestamptz:=clock_timestamp()+interval '30 minutes';
 analytics jsonb; snapshot jsonb; recorded_now timestamptz:=clock_timestamp(); was_recorded boolean;
BEGIN
 IF NOT platform.workshop_owner(p_org) THEN RAISE EXCEPTION 'workshop owner required' USING ERRCODE='42501'; END IF;
 IF p_paid IS NULL THEN RAISE EXCEPTION 'payment status required' USING ERRCODE='23514'; END IF;
 PERFORM pg_advisory_xact_lock(hashtextextended(p_org::text,8232));
 SELECT * INTO r FROM platform.workshop_requests WHERE organization_id=p_org AND id=p_request FOR UPDATE;
 IF NOT FOUND OR r.close_kind IS DISTINCT FROM 'consume' OR NOT EXISTS(
  SELECT 1 FROM platform.inventory_movements m WHERE m.organization_id=p_org AND m.workshop_request_id=p_request
  AND m.variant_id=r.variant_id AND m.kind='consume' AND m.delta=-r.quantity
 ) THEN RETURN NULL; END IF;
 SELECT * INTO f FROM platform.workshop_financials WHERE organization_id=p_org AND request_id=p_request FOR UPDATE;
 was_recorded:=FOUND;
 IF was_recorded THEN
  IF p_service IS DISTINCT FROM f.service_minor OR p_cost IS DISTINCT FROM f.part_cost_minor
     OR (NOT p_paid AND f.paid_at IS NOT NULL) THEN RETURN NULL; END IF;
  IF (f.paid_at IS NOT NULL)=p_paid THEN RETURN f.report_task_id; END IF;
  IF EXISTS(SELECT 1 FROM platform.agent_tasks WHERE organization_id=p_org AND id=f.report_task_id
    AND status IN ('pending','queued','running')) THEN RETURN NULL; END IF;
 ELSIF p_service IS NULL OR p_cost IS NULL THEN
  RAISE EXCEPTION 'confirmed financial amounts required' USING ERRCODE='23514';
 END IF;

 IF (SELECT count(*) FROM platform.agent_tasks WHERE organization_id=p_org AND created_at>clock_timestamp()-interval '1 minute')>17
 THEN RAISE EXCEPTION 'agent task rate limit' USING ERRCODE='54000'; END IF;
 -- Compute a single consistent basis without counting the old version of this
 -- request or treating all completed services as paid.
 SELECT jsonb_build_object('recorded_count',count(*)+1,'paid_count',count(*) FILTER(WHERE x.paid_at IS NOT NULL)+p_paid::int,
  'service_minor',coalesce(sum(x.service_minor),0)+p_service,
  'parts_minor',coalesce(sum(x.part_cost_minor),0)+p_cost,
  'cash_minor',coalesce(sum(x.service_minor) FILTER(WHERE x.paid_at IS NOT NULL),0)+CASE WHEN p_paid THEN p_service ELSE 0 END)
 INTO analytics FROM platform.workshop_financials x WHERE x.organization_id=p_org AND x.request_id<>p_request;
 analytics:=analytics||jsonb_build_object('completed_count',(SELECT count(*) FROM platform.workshop_requests x
  WHERE x.organization_id=p_org AND x.close_kind='consume'));
 snapshot:=jsonb_build_object('finance',jsonb_build_object('service_minor',p_service,'parts_minor',p_cost,'paid',p_paid,'quantity',r.quantity),
  'analytics',analytics,'as_of',recorded_now);
 INSERT INTO platform.agent_tasks(id,organization_id,initiator,agent_id,parent_id,correlation_id,action,idempotency_key,worker_hash,status,deadline,delegation_input)
 VALUES(task,p_org,uid,'business_manager',p_request,p_request,'delegate_workshop_finance',task,p_hash,'queued',due,snapshot);
 IF was_recorded THEN
  UPDATE platform.workshop_financials SET paid_at=recorded_now,paid_recorded_by=uid,report_task_id=task
   WHERE organization_id=p_org AND request_id=p_request;
 ELSE
  INSERT INTO platform.workshop_financials(organization_id,request_id,report_task_id,service_minor,part_cost_minor,recorded_by,recorded_at,paid_at,paid_recorded_by)
  VALUES(p_org,p_request,task,p_service,p_cost,uid,recorded_now,CASE WHEN p_paid THEN recorded_now END,CASE WHEN p_paid THEN uid END);
 END IF;
 INSERT INTO platform.channel_inbox(id,organization_id,external_id,external_user_id,payload,channel,agent_task_id)
 VALUES(task,p_org,task::text,'','{}','agent',task);
 INSERT INTO platform.agent_audit(organization_id,task_id,actor_id,agent_id,action,result)
 VALUES(p_org,task,uid,'business_manager',CASE WHEN was_recorded THEN 'workshop.payment.record' ELSE 'workshop.financials.record' END,'recorded');
 FOR step IN SELECT * FROM (VALUES(1,'finance','delegation.workshop_finance'),(2,'analytics','delegation.workshop_analytics')) v(n,agent,action)
 LOOP
  child:=gen_random_uuid();
  INSERT INTO platform.agent_tasks(id,organization_id,initiator,agent_id,parent_id,correlation_id,action,idempotency_key,worker_hash,status,deadline,delegation_step)
  VALUES(child,p_org,uid,step.agent,task,p_request,step.action,child,p_hash,CASE WHEN step.n=1 THEN 'queued' ELSE 'pending' END,due,step.n);
  IF step.n=1 THEN
   INSERT INTO platform.channel_inbox(id,organization_id,external_id,external_user_id,payload,channel,agent_task_id)
   VALUES(child,p_org,child::text,'','{}','agent',child);
  END IF;
  INSERT INTO platform.agent_audit(organization_id,task_id,actor_id,agent_id,action,result)
  VALUES(p_org,child,uid,step.agent,'task.delegate',CASE WHEN step.n=1 THEN 'queued' ELSE 'pending' END);
 END LOOP;
 RETURN task;
END $$;

CREATE FUNCTION platform.workshop_financial_context(p_task uuid,p_lease uuid) RETURNS jsonb
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE t platform.agent_tasks; parent platform.agent_tasks; children jsonb;
BEGIN
 SELECT * INTO t FROM platform.agent_lease_task(p_task,p_lease);
 IF t.id IS NULL OR t.action NOT IN ('delegate_workshop_finance','delegation.workshop_finance','delegation.workshop_analytics')
  OR NOT platform.workshop_task_allowed(t.organization_id,t.id,t.action) THEN RETURN NULL; END IF;
 SELECT * INTO STRICT parent FROM platform.agent_tasks WHERE organization_id=t.organization_id
  AND id=CASE WHEN t.action='delegate_workshop_finance' THEN t.id ELSE t.parent_id END AND action='delegate_workshop_finance';
 CASE t.action
 WHEN 'delegation.workshop_finance' THEN RETURN jsonb_build_object('finance',parent.delegation_input->'finance');
 WHEN 'delegation.workshop_analytics' THEN RETURN jsonb_build_object('analytics',parent.delegation_input->'analytics');
 ELSE
  SELECT coalesce(jsonb_agg(jsonb_build_object('id',c.id,'agent',c.agent_id,'action',c.action,'status',c.status,'result',e.result->0)
     ORDER BY c.delegation_step),'[]') INTO children
   FROM platform.agent_tasks c LEFT JOIN LATERAL (SELECT result FROM platform.agent_executions WHERE organization_id=c.organization_id
    AND task_id=c.id AND status='completed' ORDER BY attempt DESC LIMIT 1) e ON true
   WHERE c.organization_id=t.organization_id AND c.parent_id=t.id AND c.delegation_step IS NOT NULL;
  RETURN jsonb_build_object('children',children,'as_of',parent.delegation_input->'as_of');
 END CASE;
END $$;
REVOKE ALL ON FUNCTION platform.workshop_record_financials(uuid,uuid,bigint,bigint,boolean,bytea),platform.workshop_financial_context(uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.workshop_record_financials(uuid,uuid,bigint,bigint,boolean,bytea),platform.workshop_financial_context(uuid,uuid) TO aba_app;
UPDATE platform.service_metadata SET version=34 WHERE singleton;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM platform.workshop_financials) THEN
  RAISE EXCEPTION 'rollback refused: preserve workshop financial history';
 END IF;
END $$;
DROP FUNCTION platform.workshop_record_financials(uuid,uuid,bigint,bigint,boolean,bytea),
 platform.workshop_financial_context(uuid,uuid);
DROP TABLE platform.workshop_financials;
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
UPDATE platform.service_metadata SET version=33 WHERE singleton;
-- +goose StatementEnd
