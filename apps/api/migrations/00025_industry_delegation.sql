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
 OR (action IN ('delegate_auto_parts','delegate_construction','delegate_service') AND agent_id='business_manager' AND parent_id IS NULL
     AND correlation_id=id AND conversation_id IS NOT NULL AND variant_id IS NULL AND delegation_input IS NOT NULL AND delegation_step IS NULL)
 OR (action LIKE 'delegation.%' AND parent_id IS NOT NULL AND conversation_id IS NOT NULL
     AND variant_id IS NULL AND delegation_step BETWEEN 1 AND 20 AND delegation_input IS NULL));
CREATE FUNCTION platform.agent_delegation_profile(p_org uuid,p_action text) RETURNS boolean
LANGUAGE sql STABLE SET search_path=pg_catalog,platform AS $$
 SELECT EXISTS(SELECT 1 FROM platform.organizations o WHERE o.id=p_org AND o.is_test AND CASE p_action
 WHEN 'delegate_auto_parts' THEN o.industry_profile='auto_parts' AND o.business_model IN ('trade','hybrid')
 WHEN 'delegate_construction' THEN o.industry_profile='building_materials' AND o.business_model IN ('trade','hybrid')
 WHEN 'delegate_service' THEN (o.industry_profile='vehicle_service' AND o.business_model='service')
   OR (o.industry_profile='mixed_nonfood' AND o.business_model='hybrid')
 ELSE false END)
$$;
REVOKE ALL ON FUNCTION platform.agent_delegation_profile(uuid,text) FROM PUBLIC;
CREATE OR REPLACE FUNCTION platform.agent_delegation_plan(p_action text)
RETURNS TABLE(step integer, agent text, operation text) LANGUAGE sql IMMUTABLE SET search_path=pg_catalog AS $$
 SELECT v.step,v.agent,v.operation FROM (VALUES
 ('delegate_auto_parts',1,'customer','delegation.requirements'),
 ('delegate_auto_parts',2,'sales','delegation.search_plan'),
 ('delegate_auto_parts',3,'product_inventory','delegation.find_parts'),
 ('delegate_auto_parts',4,'product_inventory','delegation.verify_fitment'),
 ('delegate_auto_parts',5,'sales','delegation.proposal'),
 ('delegate_construction',1,'customer','delegation.construction_input'),
 ('delegate_construction',2,'product_inventory','delegation.construction_catalog'),
 ('delegate_construction',3,'product_inventory','delegation.construction_calculate'),
 ('delegate_construction',4,'sales','delegation.construction_offer'),
 ('delegate_service',1,'customer','delegation.service_input'),
 ('delegate_service',2,'sales','delegation.service_select'),
 ('delegate_service',3,'sales','delegation.service_availability'),
 ('delegate_service',4,'product_inventory','delegation.service_parts'),
 ('delegate_service',5,'sales','delegation.service_proposal')
 ) v(workflow,step,agent,operation)

 WHERE workflow=p_action
$$;
CREATE OR REPLACE FUNCTION platform.agent_submit_delegation(p_org uuid,p_id uuid,p_key uuid,p_conv uuid,p_action text,p_input jsonb,p_hash bytea)
RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE uid uuid:=platform.current_user_id(); existing platform.agent_tasks; child record; child_id uuid;
 due timestamptz:=clock_timestamp()+interval '30 minutes'; slots integer;
BEGIN
 IF NOT platform.agent_owner(p_org) OR NOT EXISTS(SELECT 1 FROM platform.agent_delegation_plan(p_action))
 OR NOT platform.agent_delegation_profile(p_org,p_action)
 OR NOT EXISTS(SELECT 1 FROM platform.conversations WHERE organization_id=p_org AND id=p_conv AND status='open' AND expires_at>clock_timestamp())
 THEN RAISE EXCEPTION 'delegation unavailable' USING ERRCODE='42501'; END IF;
 IF jsonb_typeof(p_input) IS DISTINCT FROM 'object' OR octet_length(p_input::text)>2048
 THEN RAISE EXCEPTION 'invalid delegation input' USING ERRCODE='23514'; END IF;
 PERFORM pg_advisory_xact_lock(hashtextextended(p_org::text,8231));
 SELECT * INTO existing FROM platform.agent_tasks WHERE organization_id=p_org AND initiator=uid AND idempotency_key=p_key;
 IF FOUND THEN
  IF (existing.action,existing.conversation_id,existing.delegation_input) IS DISTINCT FROM (p_action,p_conv,p_input)
  THEN RAISE EXCEPTION 'idempotency conflict' USING ERRCODE='23505'; END IF;
  RETURN existing.id;
 END IF;
 SELECT count(*) INTO slots FROM platform.agent_delegation_plan(p_action);
 IF (SELECT count(*) FROM platform.agent_tasks WHERE organization_id=p_org AND created_at>clock_timestamp()-interval '1 minute')+slots+1>20
 THEN RAISE EXCEPTION 'agent task rate limit' USING ERRCODE='54000'; END IF;
 INSERT INTO platform.agent_tasks(id,organization_id,initiator,agent_id,correlation_id,conversation_id,action,idempotency_key,worker_hash,status,deadline,delegation_input)
 VALUES(p_id,p_org,uid,'business_manager',p_id,p_conv,p_action,p_key,p_hash,'queued',due,p_input);
 INSERT INTO platform.channel_inbox(id,organization_id,external_id,external_user_id,payload,channel,agent_task_id)
 VALUES(p_id,p_org,p_id::text,'','{}','agent',p_id);
 INSERT INTO platform.agent_audit(organization_id,task_id,actor_id,agent_id,action,result)
 VALUES(p_org,p_id,uid,'business_manager','task.submit','queued');
 FOR child IN SELECT * FROM platform.agent_delegation_plan(p_action) ORDER BY step LOOP
  child_id:=gen_random_uuid();
  INSERT INTO platform.agent_tasks(id,organization_id,initiator,agent_id,parent_id,correlation_id,conversation_id,action,idempotency_key,worker_hash,status,deadline,delegation_step)
  VALUES(child_id,p_org,uid,child.agent,p_id,p_id,p_conv,child.operation,child_id,p_hash,CASE WHEN child.step=1 THEN 'queued' ELSE 'pending' END,due,child.step);
  IF child.step=1 THEN
   INSERT INTO platform.channel_inbox(id,organization_id,external_id,external_user_id,payload,channel,agent_task_id)
   VALUES(child_id,p_org,child_id::text,'','{}','agent',child_id);
  END IF;
  INSERT INTO platform.agent_audit(organization_id,task_id,actor_id,agent_id,action,result)
  VALUES(p_org,child_id,uid,child.agent,'task.delegate',CASE WHEN child.step=1 THEN 'queued' ELSE 'pending' END);
 END LOOP;
 RETURN p_id;
END$$;
CREATE OR REPLACE FUNCTION platform.agent_delegation_context(p_task uuid,p_lease uuid) RETURNS jsonb
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT jsonb_build_object('input',CASE WHEN t.delegation_step=1 THEN parent.delegation_input END,'children',coalesce((
  SELECT jsonb_agg(jsonb_build_object('id',c.id,'agent',c.agent_id,'action',c.action,'status',c.status,'error_code',c.error_code,'result',e.result->0) ORDER BY c.delegation_step)
  FROM platform.agent_tasks c LEFT JOIN platform.agent_executions e ON e.organization_id=c.organization_id AND e.task_id=c.id AND e.status='completed'
  WHERE c.organization_id=t.organization_id AND c.parent_id=parent.id AND (t.delegation_step IS NULL OR c.delegation_step=t.delegation_step-1)
 ),'[]'::jsonb))
 FROM platform.agent_lease_task(p_task,p_lease) t
 JOIN platform.agent_tasks parent ON parent.organization_id=t.organization_id AND parent.id=coalesce(t.parent_id,t.id)
 JOIN platform.organizations o ON o.id=t.organization_id AND platform.agent_delegation_profile(o.id,parent.action)
 JOIN platform.conversations c ON c.organization_id=t.organization_id AND c.id=t.conversation_id AND c.status='open' AND c.expires_at>statement_timestamp()
 WHERE parent.delegation_input IS NOT NULL AND (t.id=parent.id OR (parent.status='queued' AND EXISTS(
 SELECT 1 FROM platform.agent_delegation_plan(parent.action) p WHERE (p.step,p.agent,p.operation)=(t.delegation_step,t.agent_id,t.action))))$$;
CREATE OR REPLACE FUNCTION platform.agent_delegation_parts(p_task uuid,p_lease uuid) RETURNS jsonb
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 WITH bound AS MATERIALIZED (
  SELECT t.organization_id,t.action,CASE WHEN parent.action='delegate_service' THEN parent.delegation_input->'part' ELSE parent.delegation_input END AS request FROM platform.agent_lease_task(p_task,p_lease) t
  JOIN platform.agent_tasks parent ON parent.organization_id=t.organization_id AND parent.id=t.parent_id AND parent.action IN ('delegate_auto_parts','delegate_service')
  WHERE t.agent_id='product_inventory' AND t.action IN ('delegation.find_parts','delegation.verify_fitment','delegation.service_parts')
  AND platform.agent_delegation_context(p_task,p_lease) IS NOT NULL
 ), matches AS (
 SELECT v.id AS variant_id,p.name,v.sku,v.price_amount,v.currency_code,
 CASE WHEN platform.inventory_freshness_at(i.manual_updated_at,coalesce(policy.fresh_hours,24),coalesce(policy.stale_hours,72))='FRESH'
 THEN i.quantity_on_hand-i.quantity_reserved END AS available,
 CASE WHEN b.action IN ('delegation.verify_fitment','delegation.service_parts') THEN (
  SELECT CASE WHEN bool_or(f.compatible) AND NOT bool_or(NOT f.compatible) THEN 'compatible'
     WHEN bool_or(NOT f.compatible) AND NOT bool_or(f.compatible) THEN 'incompatible' ELSE 'unknown' END
  FROM platform.catalog_fitment_evidence f WHERE f.organization_id=b.organization_id AND f.variant_id=v.id
   AND f.make=lower(b.request->'vehicle'->>'make') AND f.model=lower(b.request->'vehicle'->>'model')
   AND f.engine=lower(b.request->'vehicle'->>'engine') AND (b.request->'vehicle'->>'year')::integer BETWEEN f.year_from AND f.year_to
   AND f.revoked_at IS NULL AND f.verified_at<=statement_timestamp() AND f.valid_until>statement_timestamp()
 ) END AS compatibility
 FROM bound b JOIN platform.product_variants v ON v.organization_id=b.organization_id
 JOIN platform.products p ON p.organization_id=b.organization_id AND p.id=v.product_id AND p.status='active'
 LEFT JOIN platform.inventory i ON i.organization_id=b.organization_id AND i.variant_id=v.id
 LEFT JOIN platform.catalog_policies policy ON policy.organization_id=b.organization_id
 WHERE lower(coalesce(v.attributes->>'oem',''))=lower(b.request->>'oem')
 ORDER BY v.id LIMIT 10)
 SELECT CASE WHEN EXISTS(SELECT 1 FROM bound) THEN coalesce((SELECT jsonb_agg(to_jsonb(m) ORDER BY variant_id) FROM matches m),'[]'::jsonb) END$$;

CREATE FUNCTION platform.agent_delegation_material(p_task uuid,p_lease uuid) RETURNS jsonb
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 WITH bound AS MATERIALIZED (
  SELECT t.organization_id,parent.delegation_input->>'sku' AS sku FROM platform.agent_lease_task(p_task,p_lease) t
  JOIN platform.agent_tasks parent ON parent.organization_id=t.organization_id AND parent.id=t.parent_id AND parent.action='delegate_construction'
  WHERE ((t.agent_id='product_inventory' AND t.action='delegation.construction_catalog')
       OR (t.agent_id='sales' AND t.action='delegation.construction_offer'))
  AND platform.agent_delegation_context(p_task,p_lease) IS NOT NULL
 )
 SELECT (SELECT jsonb_build_object('variant_id',v.id,'name',p.name,'sku',v.sku,'manufacturer',p.brand,
 'unit',v.attributes->>'selling_unit','package_quantity',v.attributes->>'package_quantity',
 'price_amount',v.price_amount,'currency_code',v.currency_code,'available',
 CASE WHEN platform.inventory_freshness_at(i.manual_updated_at,coalesce(policy.fresh_hours,24),coalesce(policy.stale_hours,72))='FRESH'
 THEN i.quantity_on_hand-i.quantity_reserved END)
 FROM bound b JOIN platform.product_variants v ON v.organization_id=b.organization_id AND v.sku=b.sku
 JOIN platform.products p ON p.organization_id=b.organization_id AND p.id=v.product_id AND p.status='active'
 LEFT JOIN platform.inventory i ON i.organization_id=v.organization_id AND i.variant_id=v.id
 LEFT JOIN platform.catalog_policies policy ON policy.organization_id=v.organization_id)
$$;

CREATE FUNCTION platform.agent_delegation_offering(p_org uuid) RETURNS uuid
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT (p.delegation_input->'selection'->>'offering_id')::uuid
 FROM platform.agent_lease_task(nullif(current_setting('app.agent_task',true),'')::uuid,
 nullif(current_setting('app.agent_lease',true),'')::uuid) t
 JOIN platform.agent_tasks p ON p.organization_id=t.organization_id AND p.id=t.parent_id AND p.action='delegate_service'
 WHERE t.organization_id=p_org AND t.agent_id='sales' AND t.action IN ('delegation.service_select','delegation.service_availability')
 AND platform.agent_delegation_context(t.id,nullif(current_setting('app.agent_lease',true),'')::uuid) IS NOT NULL
$$;
REVOKE ALL ON FUNCTION platform.agent_delegation_material(uuid,uuid),platform.agent_delegation_offering(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.agent_delegation_material(uuid,uuid),platform.agent_delegation_offering(uuid) TO aba_app;
CREATE POLICY delegation_read ON platform.service_offerings FOR SELECT TO aba_app
 USING (CASE WHEN nullif(current_setting('app.agent_lease',true),'') IS NOT NULL THEN id=platform.agent_delegation_offering(organization_id) ELSE false END);
CREATE POLICY delegation_read ON platform.service_offering_resources FOR SELECT TO aba_app
 USING (CASE WHEN nullif(current_setting('app.agent_lease',true),'') IS NOT NULL THEN offering_id=platform.agent_delegation_offering(organization_id) ELSE false END);
CREATE POLICY delegation_read ON platform.service_availability_windows FOR SELECT TO aba_app
 USING (CASE WHEN nullif(current_setting('app.agent_lease',true),'') IS NOT NULL THEN offering_id=platform.agent_delegation_offering(organization_id) ELSE false END);
CREATE POLICY delegation_read ON platform.service_resources FOR SELECT TO aba_app
 USING (CASE WHEN nullif(current_setting('app.agent_lease',true),'') IS NOT NULL THEN EXISTS(
 SELECT 1 FROM platform.service_offering_resources r WHERE r.organization_id=service_resources.organization_id
 AND r.resource_id=service_resources.id AND r.offering_id=platform.agent_delegation_offering(r.organization_id)) ELSE false END);
CREATE POLICY delegation_read ON platform.service_booking_allocations FOR SELECT TO aba_app
 USING (CASE WHEN nullif(current_setting('app.agent_lease',true),'') IS NOT NULL THEN EXISTS(
 SELECT 1 FROM platform.service_offering_resources r WHERE r.organization_id=service_booking_allocations.organization_id
 AND r.resource_id=service_booking_allocations.resource_id AND r.offering_id=platform.agent_delegation_offering(r.organization_id)) ELSE false END);
UPDATE platform.service_metadata SET version=25 WHERE singleton;
-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN IF EXISTS(SELECT 1 FROM platform.agent_tasks) THEN RAISE EXCEPTION 'rollback refused: preserve agent history'; END IF; END $$;
DROP POLICY delegation_read ON platform.service_booking_allocations;
DROP POLICY delegation_read ON platform.service_resources;
DROP POLICY delegation_read ON platform.service_availability_windows;
DROP POLICY delegation_read ON platform.service_offering_resources;
DROP POLICY delegation_read ON platform.service_offerings;
DROP FUNCTION platform.agent_delegation_material(uuid,uuid),platform.agent_delegation_offering(uuid);
CREATE OR REPLACE FUNCTION platform.agent_delegation_parts(p_task uuid,p_lease uuid) RETURNS jsonb
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 WITH bound AS MATERIALIZED (
  SELECT t.organization_id,t.action,parent.delegation_input AS request FROM platform.agent_lease_task(p_task,p_lease) t
  JOIN platform.agent_tasks parent ON parent.organization_id=t.organization_id AND parent.id=t.parent_id AND parent.action='delegate_auto_parts'
  WHERE t.agent_id='product_inventory' AND t.action IN ('delegation.find_parts','delegation.verify_fitment')
  AND platform.agent_delegation_context(p_task,p_lease) IS NOT NULL
 ), matches AS (
 SELECT v.id AS variant_id,p.name,v.sku,v.price_amount,v.currency_code,
 CASE WHEN platform.inventory_freshness_at(i.manual_updated_at,coalesce(policy.fresh_hours,24),coalesce(policy.stale_hours,72))='FRESH'
 THEN i.quantity_on_hand-i.quantity_reserved END AS available,
 CASE WHEN b.action='delegation.verify_fitment' THEN (
  SELECT CASE WHEN bool_or(f.compatible) AND NOT bool_or(NOT f.compatible) THEN 'compatible'
     WHEN bool_or(NOT f.compatible) AND NOT bool_or(f.compatible) THEN 'incompatible' ELSE 'unknown' END
  FROM platform.catalog_fitment_evidence f WHERE f.organization_id=b.organization_id AND f.variant_id=v.id
   AND f.make=lower(b.request->'vehicle'->>'make') AND f.model=lower(b.request->'vehicle'->>'model')
   AND f.engine=lower(b.request->'vehicle'->>'engine') AND (b.request->'vehicle'->>'year')::integer BETWEEN f.year_from AND f.year_to
   AND f.revoked_at IS NULL AND f.verified_at<=statement_timestamp() AND f.valid_until>statement_timestamp()
 ) END AS compatibility
 FROM bound b JOIN platform.product_variants v ON v.organization_id=b.organization_id
 JOIN platform.products p ON p.organization_id=b.organization_id AND p.id=v.product_id AND p.status='active'
 LEFT JOIN platform.inventory i ON i.organization_id=b.organization_id AND i.variant_id=v.id
 LEFT JOIN platform.catalog_policies policy ON policy.organization_id=b.organization_id
 WHERE lower(coalesce(v.attributes->>'oem',''))=lower(b.request->>'oem')
 ORDER BY v.id LIMIT 10)
 SELECT CASE WHEN EXISTS(SELECT 1 FROM bound) THEN coalesce((SELECT jsonb_agg(to_jsonb(m) ORDER BY variant_id) FROM matches m),'[]'::jsonb) END$$;
CREATE OR REPLACE FUNCTION platform.agent_delegation_context(p_task uuid,p_lease uuid) RETURNS jsonb
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT jsonb_build_object('input',CASE WHEN t.delegation_step=1 THEN parent.delegation_input END,'children',coalesce((
  SELECT jsonb_agg(jsonb_build_object('id',c.id,'agent',c.agent_id,'action',c.action,'status',c.status,'error_code',c.error_code,'result',e.result->0) ORDER BY c.delegation_step)
  FROM platform.agent_tasks c LEFT JOIN platform.agent_executions e ON e.organization_id=c.organization_id AND e.task_id=c.id AND e.status='completed'
  WHERE c.organization_id=t.organization_id AND c.parent_id=parent.id AND (t.delegation_step IS NULL OR c.delegation_step=t.delegation_step-1)
 ),'[]'::jsonb))
 FROM platform.agent_lease_task(p_task,p_lease) t
 JOIN platform.agent_tasks parent ON parent.organization_id=t.organization_id AND parent.id=coalesce(t.parent_id,t.id)
 JOIN platform.organizations o ON o.id=t.organization_id AND o.industry_profile='auto_parts' AND o.business_model IN ('trade','hybrid')
 JOIN platform.conversations c ON c.organization_id=t.organization_id AND c.id=t.conversation_id AND c.status='open' AND c.expires_at>statement_timestamp()
 WHERE parent.delegation_input IS NOT NULL AND (t.id=parent.id OR (parent.status='queued' AND EXISTS(
 SELECT 1 FROM platform.agent_delegation_plan(parent.action) p WHERE (p.step,p.agent,p.operation)=(t.delegation_step,t.agent_id,t.action))))$$;
CREATE OR REPLACE FUNCTION platform.agent_submit_delegation(p_org uuid,p_id uuid,p_key uuid,p_conv uuid,p_action text,p_input jsonb,p_hash bytea)
RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE uid uuid:=platform.current_user_id(); existing platform.agent_tasks; child record; child_id uuid;
 due timestamptz:=clock_timestamp()+interval '30 minutes'; slots integer;
BEGIN
 IF NOT platform.agent_owner(p_org) OR p_action IS DISTINCT FROM 'delegate_auto_parts'
 OR NOT EXISTS(SELECT 1 FROM platform.organizations WHERE id=p_org AND industry_profile='auto_parts' AND business_model IN ('trade','hybrid'))
 OR NOT EXISTS(SELECT 1 FROM platform.conversations WHERE organization_id=p_org AND id=p_conv AND status='open' AND expires_at>clock_timestamp())
 THEN RAISE EXCEPTION 'delegation unavailable' USING ERRCODE='42501'; END IF;
 IF jsonb_typeof(p_input) IS DISTINCT FROM 'object' OR octet_length(p_input::text)>2048
 THEN RAISE EXCEPTION 'invalid delegation input' USING ERRCODE='23514'; END IF;
 PERFORM pg_advisory_xact_lock(hashtextextended(p_org::text,8231));
 SELECT * INTO existing FROM platform.agent_tasks WHERE organization_id=p_org AND initiator=uid AND idempotency_key=p_key;
 IF FOUND THEN
  IF (existing.action,existing.conversation_id,existing.delegation_input) IS DISTINCT FROM (p_action,p_conv,p_input)
  THEN RAISE EXCEPTION 'idempotency conflict' USING ERRCODE='23505'; END IF;
  RETURN existing.id;
 END IF;
 SELECT count(*) INTO slots FROM platform.agent_delegation_plan(p_action);
 IF (SELECT count(*) FROM platform.agent_tasks WHERE organization_id=p_org AND created_at>clock_timestamp()-interval '1 minute')+slots+1>20
 THEN RAISE EXCEPTION 'agent task rate limit' USING ERRCODE='54000'; END IF;
 INSERT INTO platform.agent_tasks(id,organization_id,initiator,agent_id,correlation_id,conversation_id,action,idempotency_key,worker_hash,status,deadline,delegation_input)
 VALUES(p_id,p_org,uid,'business_manager',p_id,p_conv,p_action,p_key,p_hash,'queued',due,p_input);
 INSERT INTO platform.channel_inbox(id,organization_id,external_id,external_user_id,payload,channel,agent_task_id)
 VALUES(p_id,p_org,p_id::text,'','{}','agent',p_id);
 INSERT INTO platform.agent_audit(organization_id,task_id,actor_id,agent_id,action,result)
 VALUES(p_org,p_id,uid,'business_manager','task.submit','queued');
 FOR child IN SELECT * FROM platform.agent_delegation_plan(p_action) ORDER BY step LOOP
  child_id:=gen_random_uuid();
  INSERT INTO platform.agent_tasks(id,organization_id,initiator,agent_id,parent_id,correlation_id,conversation_id,action,idempotency_key,worker_hash,status,deadline,delegation_step)
  VALUES(child_id,p_org,uid,child.agent,p_id,p_id,p_conv,child.operation,child_id,p_hash,CASE WHEN child.step=1 THEN 'queued' ELSE 'pending' END,due,child.step);
  IF child.step=1 THEN
   INSERT INTO platform.channel_inbox(id,organization_id,external_id,external_user_id,payload,channel,agent_task_id)
   VALUES(child_id,p_org,child_id::text,'','{}','agent',child_id);
  END IF;
  INSERT INTO platform.agent_audit(organization_id,task_id,actor_id,agent_id,action,result)
  VALUES(p_org,child_id,uid,child.agent,'task.delegate',CASE WHEN child.step=1 THEN 'queued' ELSE 'pending' END);
 END LOOP;
 RETURN p_id;
END$$;
CREATE OR REPLACE FUNCTION platform.agent_delegation_plan(p_action text)
RETURNS TABLE(step integer, agent text, operation text) LANGUAGE sql IMMUTABLE SET search_path=pg_catalog AS $$
 SELECT * FROM (VALUES
 (1,'customer','delegation.requirements'),(2,'sales','delegation.search_plan'),
 (3,'product_inventory','delegation.find_parts'),(4,'product_inventory','delegation.verify_fitment'),
 (5,'sales','delegation.proposal')) v WHERE p_action='delegate_auto_parts'$$;
DROP FUNCTION platform.agent_delegation_profile(uuid,text);
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
 OR (action='delegate_auto_parts' AND agent_id='business_manager' AND parent_id IS NULL
     AND correlation_id=id AND conversation_id IS NOT NULL AND variant_id IS NULL AND delegation_input IS NOT NULL AND delegation_step IS NULL)
 OR (action LIKE 'delegation.%' AND parent_id IS NOT NULL AND conversation_id IS NOT NULL
     AND variant_id IS NULL AND delegation_step BETWEEN 1 AND 20 AND delegation_input IS NULL));
UPDATE platform.service_metadata SET version=24 WHERE singleton;
-- +goose StatementEnd
