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
     AND variant_id IS NULL AND delegation_step BETWEEN 1 AND 20 AND delegation_input IS NULL));
CREATE OR REPLACE FUNCTION platform.agent_delegation_profile(p_org uuid,p_action text) RETURNS boolean
LANGUAGE sql STABLE SET search_path=pg_catalog,platform AS $$
 SELECT EXISTS(SELECT 1 FROM platform.organizations o WHERE o.id=p_org AND o.is_test AND CASE p_action
 WHEN 'delegate_auto_parts' THEN o.industry_profile='auto_parts' AND o.business_model IN ('trade','hybrid')
 WHEN 'delegate_construction' THEN o.industry_profile='building_materials' AND o.business_model IN ('trade','hybrid')
 WHEN 'delegate_service' THEN (o.industry_profile='vehicle_service' AND o.business_model='service')
   OR (o.industry_profile='mixed_nonfood' AND o.business_model='hybrid')
 WHEN 'delegate_real_estate' THEN o.industry_profile='real_estate' AND o.business_model='agency'
 WHEN 'delegate_property_viewing' THEN o.industry_profile='real_estate' AND o.business_model='agency'
 WHEN 'delegate_conversation' THEN true
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
 ('delegate_service',5,'sales','delegation.service_proposal'),
 ('delegate_real_estate',1,'customer','delegation.property_requirements'),
 ('delegate_real_estate',2,'sales','delegation.property_search'),
 ('delegate_real_estate',3,'sales','delegation.property_offer'),
 ('delegate_property_viewing',1,'customer','delegation.property_selection'),
 ('delegate_property_viewing',2,'sales','delegation.property_viewing'),
 ('delegate_conversation',1,'customer','delegation.conversation_intent'),
 ('delegate_conversation',2,'sales','delegation.conversation_plan')
 ) v(workflow,step,agent,operation)

 WHERE workflow=p_action
$$;
CREATE FUNCTION platform.agent_enqueue_delegation(p_org uuid,p_id uuid,p_key uuid,p_conv uuid,p_action text,p_input jsonb,p_hash bytea,uid uuid)
RETURNS uuid LANGUAGE plpgsql SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE existing platform.agent_tasks; child record; child_id uuid;
 due timestamptz:=clock_timestamp()+interval '30 minutes'; slots integer;
BEGIN
 IF NOT EXISTS(SELECT 1 FROM platform.agent_delegation_plan(p_action))
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
CREATE OR REPLACE FUNCTION platform.agent_submit_delegation(p_org uuid,p_id uuid,p_key uuid,p_conv uuid,p_action text,p_input jsonb,p_hash bytea)
RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
BEGIN
 IF NOT platform.agent_owner(p_org) OR p_action='delegate_conversation' THEN
  RAISE EXCEPTION 'delegation unavailable' USING ERRCODE='42501';
 END IF;
 RETURN platform.agent_enqueue_delegation(p_org,p_id,p_key,p_conv,p_action,p_input,p_hash,platform.current_user_id());
END$$;
REVOKE ALL ON FUNCTION platform.agent_enqueue_delegation(uuid,uuid,uuid,uuid,text,jsonb,bytea,uuid) FROM PUBLIC;

-- The conversation is still the identity and retention boundary. Only the
-- fenced router can write its typed context or link an existing workflow.
CREATE TABLE platform.conversation_routes (
 organization_id uuid NOT NULL,
 conversation_id uuid NOT NULL,
 customer_id uuid NOT NULL,
 channel text NOT NULL CHECK(channel IN ('telegram','whatsapp','instagram')),
 turn_id uuid,
 intent text CHECK(intent IN ('product_search','price_inquiry','inventory_inquiry','auto_parts_compatibility',
  'construction_calculation','auto_service_booking','real_estate_search','property_viewing_request','order_status','customer_support')),
 confirmed jsonb NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(confirmed)='object' AND octet_length(confirmed::text)<=4096),
 missing text[] NOT NULL DEFAULT '{}' CHECK(cardinality(missing)<=12),
 active_workflow uuid,
 last_property_offer uuid,
 approval_workflow uuid,
 pending_approval uuid,
 last_result jsonb CHECK(octet_length(last_result::text)<=32768),
 PRIMARY KEY(organization_id,conversation_id),
 FOREIGN KEY(organization_id,conversation_id,customer_id) REFERENCES platform.conversations(organization_id,id,customer_id) ON DELETE CASCADE,
 FOREIGN KEY(organization_id,turn_id) REFERENCES platform.agent_tasks(organization_id,id),
 FOREIGN KEY(organization_id,active_workflow) REFERENCES platform.agent_tasks(organization_id,id),
 FOREIGN KEY(organization_id,last_property_offer) REFERENCES platform.agent_tasks(organization_id,id),
 FOREIGN KEY(organization_id,approval_workflow) REFERENCES platform.agent_tasks(organization_id,id),
 FOREIGN KEY(organization_id,pending_approval) REFERENCES platform.orders(organization_id,id)
);
ALTER TABLE platform.conversation_routes ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.conversation_routes FORCE ROW LEVEL SECURITY;
CREATE POLICY migration_owner ON platform.conversation_routes TO aba_migrator USING(true) WITH CHECK(true);
CREATE POLICY staff_read ON platform.conversation_routes FOR SELECT TO aba_app
 USING(organization_id::text=current_setting('app.organization_id',true)
 AND platform.is_test_org(organization_id) AND platform.can_operate_orders(organization_id));
GRANT SELECT ON platform.conversation_routes TO aba_app;
CREATE INDEX conversation_route_tasks ON platform.agent_tasks(organization_id,conversation_id,created_at,id)
 WHERE action='delegate_conversation' AND status IN ('queued','running');

CREATE FUNCTION platform.conversation_route_allowed(p_org uuid,p_conv uuid) RETURNS boolean
LANGUAGE sql VOLATILE SET search_path=pg_catalog,platform AS $$
 SELECT EXISTS(SELECT 1 FROM platform.conversations c
 JOIN platform.organizations z ON z.id=c.organization_id AND z.is_test
 WHERE (c.organization_id,c.id)=(p_org,p_conv) AND c.status='open' AND c.control_mode='agent'
 AND c.expires_at>clock_timestamp()
 AND coalesce((SELECT a.auto_enabled FROM platform.ai_settings a WHERE a.organization_id=p_org),true)
 AND (
  (c.channel='telegram' AND 1=(SELECT count(*) FROM platform.telegram_principals p WHERE (p.organization_id,p.conversation_id)=(p_org,p_conv))
   AND EXISTS(SELECT 1 FROM platform.telegram_principals p JOIN platform.telegram_integrations i
      ON (i.organization_id,i.id)=(p.organization_id,p.integration_id)
      WHERE (p.organization_id,p.conversation_id)=(p_org,p_conv) AND i.active AND i.mode='mock'))
  OR (c.channel IN ('whatsapp','instagram') AND EXISTS(SELECT 1 FROM platform.channel_principals p JOIN platform.channel_integrations i
      ON (i.organization_id,i.id,i.channel)=(p.organization_id,p.integration_id,p.channel)
      LEFT JOIN platform.instagram_connections x ON (x.organization_id,x.integration_id)=(i.organization_id,i.id)
      WHERE (p.organization_id,p.conversation_id,p.id,p.channel)=(p_org,p_conv,c.customer_id,c.channel)
      AND p.window_until>clock_timestamp() AND i.active AND i.mode='mock'
      AND ((i.channel='whatsapp' AND i.sending_enabled)
       OR (i.channel='instagram' AND NOT i.sending_enabled AND x.status='connected' AND x.revoked_at IS NULL
        AND x.webhook_subscribed AND x.webhook_seen_at IS NOT NULL AND x.token_expires_at>clock_timestamp()
        AND x.permissions @> ARRAY['instagram_business_basic','instagram_business_manage_messages']))))))
$$;

CREATE OR REPLACE FUNCTION platform.agent_delegation_context(p_task uuid,p_lease uuid) RETURNS jsonb
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT jsonb_build_object('conversation_routed', origin.routed,'input',CASE WHEN t.delegation_step=1 THEN parent.delegation_input END,'children',coalesce((
  SELECT jsonb_agg(jsonb_build_object('id',c.id,'agent',c.agent_id,'action',c.action,'status',c.status,'error_code',c.error_code,'result',e.result->0) ORDER BY c.delegation_step)
  FROM platform.agent_tasks c LEFT JOIN platform.agent_executions e ON e.organization_id=c.organization_id AND e.task_id=c.id AND e.status='completed'
  WHERE c.organization_id=t.organization_id AND c.parent_id=parent.id AND (t.delegation_step IS NULL OR c.delegation_step=t.delegation_step-1)
 ),'[]'::jsonb))
 FROM platform.agent_lease_task(p_task,p_lease) t
 JOIN platform.agent_tasks parent ON parent.organization_id=t.organization_id AND parent.id=coalesce(t.parent_id,t.id)
 JOIN platform.organizations o ON o.id=t.organization_id AND platform.agent_delegation_profile(o.id,parent.action)
 JOIN platform.conversations c ON c.organization_id=t.organization_id AND c.id=t.conversation_id AND c.status='open' AND c.expires_at>statement_timestamp()
 CROSS JOIN LATERAL (SELECT EXISTS(SELECT 1 FROM platform.agent_tasks root
  WHERE (root.organization_id,root.id,root.conversation_id)=(parent.organization_id,parent.idempotency_key,parent.conversation_id)
  AND root.action='delegate_conversation') AS routed) origin
 WHERE (NOT origin.routed OR platform.conversation_route_allowed(t.organization_id,t.conversation_id))
 AND parent.delegation_input IS NOT NULL AND (t.id=parent.id OR (parent.status='queued' AND EXISTS(
 SELECT 1 FROM platform.agent_delegation_plan(parent.action) p WHERE (p.step,p.agent,p.operation)=(t.delegation_step,t.agent_id,t.action))))$$;
CREATE FUNCTION platform.conversation_route_submit(p_org uuid,p_conv uuid,p_key uuid,p_hash bytea)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE c platform.conversations; src text; stored text; owner_id uuid; task_id uuid; wait_turn boolean;
BEGIN
 IF session_user<>'aba_app' OR p_org::text IS DISTINCT FROM current_setting('app.organization_id',true)
 OR octet_length(p_hash) IS DISTINCT FROM 32 THEN RETURN false; END IF;
 SELECT * INTO c FROM platform.conversations WHERE (organization_id,id)=(p_org,p_conv) FOR UPDATE;
 IF c.id IS NULL OR NOT platform.conversation_route_allowed(p_org,p_conv) THEN RETURN false; END IF;
 IF c.channel='telegram' AND platform.telegram_scope(p_org,p_conv) THEN
  SELECT q.payload->'message'->>'text' INTO src
  FROM platform.telegram_inbox q JOIN platform.telegram_principals p
   ON (p.organization_id,p.integration_id,p.conversation_id,p.user_id,p.chat_id)
      =(q.organization_id,q.integration_id,q.conversation_id,q.user_id,q.chat_id)
  WHERE q.id=p_key AND q.organization_id=p_org AND q.conversation_id=p_conv
  AND q.capability::text=current_setting('app.telegram_cap',true);
 ELSIF c.channel IN ('whatsapp','instagram') AND platform.channel_scope(p_org,p_conv) THEN
  SELECT q.payload->>'text' INTO src
  FROM platform.channel_inbox q JOIN platform.channel_principals p
   ON (p.organization_id,p.integration_id,p.conversation_id,p.external_user_id,p.channel)
      =(q.organization_id,q.integration_id,q.conversation_id,q.external_user_id,q.channel)
  WHERE q.id=p_key AND q.organization_id=p_org AND q.conversation_id=p_conv AND p.id=c.customer_id
  AND q.capability::text=current_setting('app.channel_cap',true) AND q.payload->>'message_type'='text';
 END IF;
 SELECT content INTO stored FROM platform.messages
 WHERE (organization_id,conversation_id,client_key)=(p_org,p_conv,p_key) AND role='customer';
 IF src IS NULL OR stored IS DISTINCT FROM regexp_replace(src,'^[[:space:]]+|[[:space:]]+$','','g') THEN RETURN false; END IF;
 SELECT user_id INTO owner_id FROM platform.organization_members WHERE organization_id=p_org AND role='owner' ORDER BY user_id LIMIT 1;
 IF owner_id IS NULL THEN RETURN false; END IF;
 IF EXISTS(SELECT 1 FROM platform.agent_tasks WHERE organization_id=p_org AND conversation_id=p_conv
  AND idempotency_key=p_key AND action='delegate_conversation') THEN RETURN true; END IF;
 SELECT EXISTS(SELECT 1 FROM platform.agent_tasks WHERE organization_id=p_org AND conversation_id=p_conv
  AND action='delegate_conversation' AND status IN ('queued','running')) INTO wait_turn;
 BEGIN
  task_id:=platform.agent_enqueue_delegation(p_org,gen_random_uuid(),p_key,p_conv,'delegate_conversation',
   jsonb_build_object('message_key',p_key),p_hash,owner_id);
 EXCEPTION WHEN program_limit_exceeded THEN
  RETURN false;
 END;
 INSERT INTO platform.conversation_routes(organization_id,conversation_id,customer_id,channel)
 VALUES(p_org,p_conv,c.customer_id,c.channel) ON CONFLICT DO NOTHING;
 IF wait_turn THEN
  UPDATE platform.channel_inbox q SET available_at='infinity' FROM platform.agent_tasks t
   WHERE (q.organization_id,q.agent_task_id)=(t.organization_id,t.id)
   AND t.organization_id=p_org AND t.parent_id=task_id AND t.delegation_step=1;
 END IF;
 RETURN true;
END $$;

CREATE FUNCTION platform.conversation_intent_context(p_task uuid,p_lease uuid) RETURNS jsonb
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT jsonb_strip_nulls(jsonb_build_object('text',m.content,'previous',
  jsonb_build_object('intent',r.intent,'confirmed',r.confirmed,'missing',r.missing,'offer_task_id',r.last_property_offer)))
 FROM platform.agent_lease_task(p_task,p_lease) t
 JOIN platform.agent_tasks parent ON (parent.organization_id,parent.id)=(t.organization_id,t.parent_id)
 JOIN platform.conversation_routes r ON (r.organization_id,r.conversation_id)=(t.organization_id,t.conversation_id)
 JOIN platform.messages m ON (m.organization_id,m.conversation_id,m.client_key)
   =(t.organization_id,t.conversation_id,(parent.delegation_input->>'message_key')::uuid) AND m.role='customer'
 WHERE (t.agent_id,t.action,parent.action)=('customer','delegation.conversation_intent','delegate_conversation')
 AND platform.agent_delegation_context(p_task,p_lease) IS NOT NULL
 AND platform.conversation_route_allowed(t.organization_id,t.conversation_id)
$$;

CREATE FUNCTION platform.conversation_plan_lookup(p_task uuid,p_lease uuid) RETURNS jsonb
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 WITH bound AS MATERIALIZED (
  SELECT t.organization_id,t.conversation_id,r.customer_id,r.last_property_offer,
   platform.agent_delegation_context(p_task,p_lease)->'children'->0->'result'->'route' AS route
  FROM platform.agent_lease_task(p_task,p_lease) t
  JOIN platform.conversation_routes r ON (r.organization_id,r.conversation_id)=(t.organization_id,t.conversation_id)
  WHERE (t.agent_id,t.action)=('sales','delegation.conversation_plan')
  AND platform.conversation_route_allowed(t.organization_id,t.conversation_id)
 ), request AS (
  SELECT b.*,route->>'intent' AS intent,route->'confirmed' AS attrs
  FROM bound b WHERE route IS NOT NULL AND route->'missing' IS NULL
 ), catalog_cards AS (
  SELECT v.id,p.name,v.sku,v.price_amount,v.currency_code,
   CASE WHEN platform.inventory_freshness(v.organization_id,i.manual_updated_at)='FRESH' THEN i.quantity_on_hand-i.quantity_reserved END AS stock
  FROM request r JOIN platform.organizations z ON z.id=r.organization_id AND z.business_model IN ('trade','hybrid')
  JOIN platform.products p ON p.organization_id=r.organization_id AND p.status='active'
  JOIN platform.product_variants v ON (v.organization_id,v.product_id)=(p.organization_id,p.id)
  JOIN platform.inventory i ON (i.organization_id,i.variant_id)=(v.organization_id,v.id)
  WHERE r.intent IN ('product_search','price_inquiry','inventory_inquiry') AND length(r.attrs->>'product') BETWEEN 1 AND 100
  AND (lower(v.sku)=lower(r.attrs->>'product') OR position(lower(r.attrs->>'product') in lower(p.name))>0)
  ORDER BY (lower(v.sku)=lower(r.attrs->>'product')) DESC,p.name,v.sku,v.id LIMIT 3
 ), owned_order AS (
  SELECT o.id,o.status FROM request r JOIN platform.orders o
  ON (o.organization_id,o.conversation_id,o.customer_id)=(r.organization_id,r.conversation_id,r.customer_id)
  WHERE r.intent='order_status' AND o.id::text=r.attrs->>'order_id'
 ), selected_property AS (
  SELECT p.id,p.viewing_offering_id
  FROM request r JOIN platform.agent_tasks a ON (a.organization_id,a.id,a.conversation_id)
    =(r.organization_id,r.last_property_offer,r.conversation_id)
  JOIN platform.agent_executions e ON (e.organization_id,e.task_id)=(a.organization_id,a.id) AND e.status='completed'
  CROSS JOIN LATERAL jsonb_array_elements(e.result->0->'properties') WITH ORDINALITY x(item,pos)
  JOIN platform.property_listings p ON p.organization_id=r.organization_id AND p.id::text=x.item->>'id'
  WHERE r.intent='property_viewing_request' AND a.action='delegate_real_estate' AND a.status='completed'
  AND a.deadline>clock_timestamp() AND e.result->0->>'state'='proposal_ready'
  AND (CASE WHEN r.attrs->>'property_id' IS NOT NULL THEN p.id::text=r.attrs->>'property_id'
       ELSE x.pos=(r.attrs->>'option')::int END)
  AND p.status='available' AND p.property_status='active' AND p.verified_until>clock_timestamp()
 ), offering AS (
  SELECT o.id,o.revision,o.timezone,o.min_minutes,o.max_minutes,p.id AS property_id
  FROM request r JOIN platform.service_offerings o ON o.organization_id=r.organization_id AND o.status='active'
  LEFT JOIN selected_property p ON p.viewing_offering_id=o.id
  WHERE (r.intent='auto_service_booking' AND platform.agent_delegation_profile(r.organization_id,'delegate_service')
    AND lower(o.name)=lower(r.attrs->>'service'))
   OR (r.intent='property_viewing_request' AND p.id IS NOT NULL)
  ORDER BY o.id LIMIT 2
 )
 SELECT CASE WHEN EXISTS(SELECT 1 FROM bound WHERE route IS NOT NULL) THEN jsonb_build_object(
  'products',coalesce((SELECT jsonb_agg(to_jsonb(p)) FROM catalog_cards p),'[]'),
  'order',(SELECT to_jsonb(o) FROM owned_order o),
  'offerings',coalesce((SELECT jsonb_agg(to_jsonb(s)) FROM offering s),'[]')) END
$$;

CREATE FUNCTION platform.conversation_current_reply(p_org uuid,p_conv uuid,p_task uuid) RETURNS boolean
LANGUAGE sql VOLATILE SET search_path=pg_catalog,platform AS $$
 SELECT platform.conversation_route_allowed(p_org,p_conv) AND EXISTS(
  SELECT 1 FROM platform.conversation_routes r
  JOIN platform.agent_tasks t ON t.organization_id=r.organization_id AND t.conversation_id=r.conversation_id
   AND (t.id=r.turn_id OR t.id=r.active_workflow) AND t.id=p_task AND t.deadline>clock_timestamp()
  LEFT JOIN platform.agent_executions e ON (e.organization_id,e.task_id)=(t.organization_id,t.id) AND e.status='completed'
  WHERE (r.organization_id,r.conversation_id)=(p_org,p_conv)
  AND (e.id IS NOT NULL OR t.status IN ('failed','expired','cancelled'))
  AND NOT EXISTS (
   SELECT 1 FROM jsonb_array_elements(e.result->0->'catalog') card WHERE NOT EXISTS (
    SELECT 1 FROM platform.product_variants v JOIN platform.products p ON (p.organization_id,p.id)=(v.organization_id,v.product_id)
    JOIN platform.inventory i ON (i.organization_id,i.variant_id)=(v.organization_id,v.id)
    WHERE v.organization_id=p_org AND v.id::text=card->>'id' AND p.status='active' AND p.name=card->>'name' AND v.sku=card->>'sku'
    AND (r.intent='inventory_inquiry' OR (v.price_amount::text=card->>'price_amount' AND v.currency_code=card->>'currency_code'))
    AND (r.intent='price_inquiry' OR (CASE WHEN platform.inventory_freshness(p_org,i.manual_updated_at)='FRESH'
         THEN (i.quantity_on_hand-i.quantity_reserved)::text END) IS NOT DISTINCT FROM card->>'stock')))
  AND NOT EXISTS (
   SELECT 1 FROM jsonb_array_elements(e.result->0->'properties') item WHERE NOT EXISTS (
    SELECT 1 FROM platform.property_listings p WHERE p.organization_id=p_org AND p.id::text=item->>'id'
    AND p.status='available' AND p.property_status='active' AND p.verified_until>clock_timestamp()+interval '6 seconds'
    AND p.price_amount::text IS NOT DISTINCT FROM item->>'price_amount' AND p.currency_code IS NOT DISTINCT FROM item->>'currency_code'
    AND p.title=item->>'title' AND p.area_m2=(item->>'area_m2')::numeric
    AND p.locality=item->>'locality' AND coalesce(p.city,'')=coalesce(item->>'city','')
    AND coalesce(p.district,'')=coalesce(item->>'district','') AND p.rooms=(item->>'rooms')::int))
  AND (e.result->0->'order' IS NULL OR EXISTS(
   SELECT 1 FROM platform.orders o WHERE (o.organization_id,o.conversation_id,o.customer_id)=(p_org,p_conv,r.customer_id)
    AND o.id::text=e.result->0->'order'->>'id' AND o.status=e.result->0->'order'->>'status')))
$$;

CREATE FUNCTION platform.conversation_publish(p_task platform.agent_tasks,p_execution uuid,p_body text) RETURNS void
LANGUAGE plpgsql SET search_path=pg_catalog,platform AS $$
DECLARE outcome text:='suppressed';
BEGIN
 IF octet_length(p_body) BETWEEN 1 AND 1000 AND
    platform.conversation_current_reply(p_task.organization_id,p_task.conversation_id,p_task.id) THEN
  INSERT INTO platform.messages(id,organization_id,conversation_id,role,content,data,client_key)
   VALUES(p_task.id,p_task.organization_id,p_task.conversation_id,'assistant',p_body,
    '{"conversation_route":true}',p_task.id) ON CONFLICT DO NOTHING;
  IF EXISTS(SELECT 1 FROM platform.telegram_outbox WHERE id=p_task.id AND organization_id=p_task.organization_id)
   OR EXISTS(SELECT 1 FROM platform.channel_outbox WHERE id=p_task.id AND organization_id=p_task.organization_id
     AND status<>'cancelled') THEN outcome:='queued';
  ELSIF EXISTS(SELECT 1 FROM platform.channel_outbox WHERE id=p_task.id AND organization_id=p_task.organization_id) THEN
    outcome:='instagram_send_disabled';
  END IF;
 END IF;
 INSERT INTO platform.agent_audit(organization_id,task_id,execution_id,agent_id,action,result)
 VALUES(p_task.organization_id,p_task.id,p_execution,p_task.agent_id,'conversation.reply',outcome);
END $$;

CREATE FUNCTION platform.finish_conversation_execution() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE t platform.agent_tasks; route jsonb; body jsonb:=NEW.result->0; workflow text; next_id uuid;
 response text; source_turn boolean; intended_action text;
BEGIN
 SELECT * INTO t FROM platform.agent_tasks WHERE (organization_id,id)=(NEW.organization_id,NEW.task_id)
  AND agent_id='business_manager' AND parent_id IS NULL AND conversation_id IS NOT NULL;
 IF NOT FOUND THEN RETURN NEW; END IF;
 source_turn:=t.action='delegate_conversation';
 IF NOT source_turn AND NOT EXISTS(SELECT 1 FROM platform.agent_tasks root
   WHERE (root.organization_id,root.id,root.conversation_id)=(t.organization_id,t.idempotency_key,t.conversation_id)
   AND root.action='delegate_conversation') THEN RETURN NEW; END IF;
 PERFORM 1 FROM platform.conversations c WHERE (c.organization_id,c.id)=(t.organization_id,t.conversation_id) FOR UPDATE;
 IF NOT platform.conversation_route_allowed(t.organization_id,t.conversation_id) THEN RETURN NEW; END IF;
 IF source_turn THEN
  route:=body->'route';
  response:=body->>'message';
  workflow:=route->>'workflow';
  intended_action:=CASE route->>'intent'
   WHEN 'auto_parts_compatibility' THEN 'delegate_auto_parts'
   WHEN 'construction_calculation' THEN 'delegate_construction'
   WHEN 'auto_service_booking' THEN 'delegate_service'
   WHEN 'real_estate_search' THEN 'delegate_real_estate'
   WHEN 'property_viewing_request' THEN 'delegate_property_viewing'
  END;
  IF workflow IS NOT NULL THEN
   IF workflow IS DISTINCT FROM intended_action OR body->>'state' IS DISTINCT FROM 'dispatch_ready'
    OR jsonb_typeof(route->'input') IS DISTINCT FROM 'object' OR route ? 'missing' THEN
    RAISE EXCEPTION 'invalid conversational dispatch' USING ERRCODE='23514';
   END IF;
   IF NOT platform.agent_delegation_profile(t.organization_id,workflow) THEN
    response:='Этот сценарий не подключён для данного бизнеса. Обратитесь к сотруднику.';
    body:=jsonb_set(body,'{state}','"workflow_denied"');
   ELSE
    BEGIN
     next_id:=platform.agent_enqueue_delegation(t.organization_id,gen_random_uuid(),t.id,t.conversation_id,
      workflow,route->'input',t.worker_hash,t.initiator);
    EXCEPTION WHEN program_limit_exceeded THEN
     response:='Достигнут лимит запросов. Повторите чуть позже.';
     body:=jsonb_set(body,'{state}','"rate_limited"');
    END;
   END IF;
  END IF;
  IF route IS NULL THEN RETURN NEW; END IF;
  UPDATE platform.conversation_routes
   SET turn_id=t.id,intent=route->>'intent',confirmed=route->'confirmed',
    missing=ARRAY(SELECT jsonb_array_elements_text(coalesce(route->'missing','[]'))),
    active_workflow=next_id,last_result=body-'children'
   WHERE (organization_id,conversation_id)=(t.organization_id,t.conversation_id);
 ELSE
  UPDATE platform.conversation_routes SET last_result=body-'children',
   last_property_offer=CASE WHEN t.action='delegate_real_estate' AND body->>'state'='proposal_ready' THEN t.id
     WHEN t.action='delegate_real_estate' THEN NULL ELSE last_property_offer END,
   approval_workflow=CASE WHEN body->>'state'='approval_required' THEN t.id ELSE approval_workflow END
  WHERE (organization_id,conversation_id,active_workflow)=(t.organization_id,t.conversation_id,t.id);
  IF NOT FOUND THEN RETURN NEW; END IF;
  response:=body->>'conversation_reply';
 END IF;
 IF response IS NOT NULL THEN PERFORM platform.conversation_publish(t,NEW.id,response); END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER conversation_execution_finished AFTER UPDATE OF status ON platform.agent_executions
 FOR EACH ROW WHEN(OLD.status='running' AND NEW.status='completed')
 EXECUTE FUNCTION platform.finish_conversation_execution();

CREATE FUNCTION platform.advance_conversation_turn() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE next_id uuid;
BEGIN
 IF NEW.action<>'delegate_conversation' THEN
  IF NEW.status='completed' THEN RETURN NEW; END IF;
  UPDATE platform.conversation_routes r SET last_result=jsonb_build_object('state',NEW.status)
   WHERE (r.organization_id,r.conversation_id,r.active_workflow)=(NEW.organization_id,NEW.conversation_id,NEW.id);
  IF FOUND THEN
   PERFORM platform.conversation_publish(NEW,NULL,'Не удалось завершить запрос. Повторите его или обратитесь к сотруднику.');
  END IF;
  RETURN NEW;
 END IF;
 PERFORM 1 FROM platform.conversations c WHERE (c.organization_id,c.id)=(NEW.organization_id,NEW.conversation_id) FOR UPDATE;
 IF NEW.status<>'completed' THEN
  -- The existing worker will expire any unfinished children through its normal lease path.
  UPDATE platform.channel_inbox q SET available_at=clock_timestamp()
   FROM platform.agent_tasks t WHERE (q.organization_id,q.agent_task_id)=(t.organization_id,t.id)
   AND t.parent_id=NEW.id AND q.status='pending' AND q.available_at='infinity';
  IF platform.conversation_route_allowed(NEW.organization_id,NEW.conversation_id) THEN
   UPDATE platform.conversation_routes SET turn_id=NEW.id,active_workflow=NULL,
    last_result=jsonb_build_object('state',NEW.status)
   WHERE (organization_id,conversation_id)=(NEW.organization_id,NEW.conversation_id);
   PERFORM platform.conversation_publish(NEW,NULL,'Не удалось обработать сообщение. Повторите запрос чуть позже.');
  END IF;
 END IF;
 SELECT id INTO next_id FROM platform.agent_tasks
  WHERE (organization_id,conversation_id,action)=(NEW.organization_id,NEW.conversation_id,'delegate_conversation')
   AND status IN ('queued','running') ORDER BY created_at,id LIMIT 1;
 UPDATE platform.channel_inbox q SET available_at=clock_timestamp()
  FROM platform.agent_tasks t WHERE (q.organization_id,q.agent_task_id)=(t.organization_id,t.id)
  AND t.parent_id=next_id AND t.delegation_step=1 AND q.status='pending' AND q.available_at='infinity';
 RETURN NEW;
END $$;
CREATE TRIGGER conversation_turn_finished AFTER UPDATE OF status ON platform.agent_tasks
 FOR EACH ROW WHEN(NEW.parent_id IS NULL AND NEW.action IN ('delegate_conversation','delegate_auto_parts',
  'delegate_construction','delegate_service','delegate_real_estate','delegate_property_viewing') AND OLD.status IN ('running','queued')
  AND NEW.status IN ('completed','expired','failed','cancelled'))
 EXECUTE FUNCTION platform.advance_conversation_turn();

CREATE FUNCTION platform.conversation_approval_state() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
BEGIN
 UPDATE platform.conversation_routes r
 SET pending_approval=CASE WHEN o.status='pending_confirmation' THEN o.id ELSE NULL END,
 approval_workflow=CASE WHEN o.status='pending_confirmation' THEN o.idempotency_key ELSE NULL END
 FROM platform.orders o JOIN platform.agent_tasks t
   ON (t.organization_id,t.id,t.conversation_id)=(o.organization_id,o.idempotency_key,o.conversation_id)
 JOIN platform.agent_tasks root
   ON (root.organization_id,root.id,root.conversation_id)=(t.organization_id,t.idempotency_key,t.conversation_id)
 WHERE (o.organization_id,o.id)=(NEW.organization_id,NEW.order_id)
  AND (r.organization_id,r.conversation_id,r.customer_id)=(o.organization_id,o.conversation_id,o.customer_id)
  AND t.action IN ('delegate_service','delegate_property_viewing') AND root.action='delegate_conversation'
  AND (o.status='pending_confirmation' OR r.pending_approval=o.id);
 RETURN NEW;
END $$;
CREATE TRIGGER conversation_approval_state AFTER INSERT ON platform.order_events
 FOR EACH ROW WHEN(NEW.kind IN ('created','confirmed','rejected','cancelled','completed'))
 EXECUTE FUNCTION platform.conversation_approval_state();

ALTER FUNCTION platform.booking_notification_admit(uuid,text) RENAME TO prior_booking_notification_admit;
REVOKE ALL ON FUNCTION platform.prior_booking_notification_admit(uuid,text) FROM aba_app;

CREATE FUNCTION platform.booking_notification_admit(p_cap uuid,p_channel text)
RETURNS TABLE(allowed boolean,lock_key bigint)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE item record; marker boolean;
BEGIN
 IF p_channel='telegram' THEN
  SELECT q.id,q.organization_id,q.conversation_id,q.integration_id,q.lease_until,q.chat_id::text AS recipient INTO item
  FROM platform.telegram_outbox q WHERE q.capability=p_cap AND q.status='sending';
 ELSIF p_channel='whatsapp' THEN
  SELECT q.id,q.organization_id,q.conversation_id,q.integration_id,q.lease_until,q.external_user_id AS recipient INTO item
  FROM platform.channel_outbox q JOIN platform.channel_integrations i ON (i.organization_id,i.id)=(q.organization_id,q.integration_id)
  WHERE q.capability=p_cap AND q.status='sending' AND i.channel='whatsapp';
 ELSE RETURN QUERY SELECT false,NULL::bigint; RETURN;
 END IF;
 SELECT m.role='assistant' AND m.data @> '{"conversation_route":true}' INTO marker FROM platform.messages m
 WHERE (m.organization_id,m.conversation_id,m.id)=(item.organization_id,item.conversation_id,item.id);
 IF NOT coalesce(marker,false) THEN
  RETURN QUERY SELECT a.allowed,a.lock_key FROM platform.prior_booking_notification_admit(p_cap,p_channel) a;
  RETURN;
 END IF;
 IF item.lease_until>clock_timestamp()+interval '6 seconds'
  AND platform.conversation_current_reply(item.organization_id,item.conversation_id,item.id)
  AND EXISTS(SELECT 1 FROM platform.conversations c WHERE (c.organization_id,c.id)=(item.organization_id,item.conversation_id) AND c.channel=p_channel
   AND ((p_channel='telegram' AND EXISTS(SELECT 1 FROM platform.telegram_principals p JOIN platform.telegram_integrations i
     ON (i.organization_id,i.id)=(p.organization_id,p.integration_id)
     WHERE (p.organization_id,p.conversation_id,p.integration_id)=(c.organization_id,c.id,item.integration_id)
      AND p.chat_id::text=item.recipient AND i.mode='mock'))
    OR (p_channel='whatsapp' AND EXISTS(SELECT 1 FROM platform.channel_principals p JOIN platform.channel_integrations i
      ON (i.organization_id,i.id,i.channel)=(p.organization_id,p.integration_id,p.channel)
      WHERE (p.organization_id,p.conversation_id,p.integration_id,p.id)=(c.organization_id,c.id,item.integration_id,c.customer_id)
       AND p.external_user_id=item.recipient AND i.mode='mock')))) THEN
  RETURN QUERY SELECT true,NULL::bigint;
 ELSE
  IF p_channel='telegram' THEN
   UPDATE platform.telegram_outbox SET status='cancelled',capability=NULL,lease_until=NULL WHERE id=item.id AND capability=p_cap;
  END IF;
  RETURN QUERY SELECT false,NULL::bigint;
 END IF;
END $$;

CREATE FUNCTION platform.audit_conversation_delivery() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
BEGIN
 INSERT INTO platform.agent_audit(organization_id,task_id,agent_id,action,result)
 SELECT t.organization_id,t.id,t.agent_id,'conversation.delivery_state',NEW.status
 FROM platform.agent_tasks t JOIN platform.messages m ON (m.organization_id,m.conversation_id,m.id)=(t.organization_id,t.conversation_id,t.id)
 WHERE (t.organization_id,t.id)=(NEW.organization_id,NEW.id) AND m.data @> '{"conversation_route":true}';
 RETURN NEW;
END $$;
CREATE TRIGGER conversational_telegram_delivery AFTER UPDATE OF status ON platform.telegram_outbox
 FOR EACH ROW WHEN(OLD.status IS DISTINCT FROM NEW.status) EXECUTE FUNCTION platform.audit_conversation_delivery();
CREATE TRIGGER conversational_channel_delivery AFTER UPDATE OF status ON platform.channel_outbox
 FOR EACH ROW WHEN(OLD.status IS DISTINCT FROM NEW.status) EXECUTE FUNCTION platform.audit_conversation_delivery();

REVOKE ALL ON FUNCTION platform.conversation_route_allowed(uuid,uuid),platform.conversation_route_submit(uuid,uuid,uuid,bytea),
 platform.conversation_intent_context(uuid,uuid),platform.conversation_plan_lookup(uuid,uuid),platform.conversation_current_reply(uuid,uuid,uuid),
 platform.conversation_publish(platform.agent_tasks,uuid,text),platform.finish_conversation_execution(),
 platform.advance_conversation_turn(),platform.conversation_approval_state(),platform.booking_notification_admit(uuid,text),
 platform.audit_conversation_delivery() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.conversation_route_submit(uuid,uuid,uuid,bytea),
 platform.conversation_intent_context(uuid,uuid),platform.conversation_plan_lookup(uuid,uuid),
 platform.booking_notification_admit(uuid,text) TO aba_app;
CREATE OR REPLACE FUNCTION platform.enqueue_real_estate_notification() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE t platform.agent_tasks; c platform.conversations; outcome text:='channel_unavailable'; body text:=NEW.result->0->>'message';
BEGIN
 SELECT * INTO t FROM platform.agent_tasks WHERE organization_id=NEW.organization_id AND id=NEW.task_id
 AND agent_id='business_manager' AND action IN ('delegate_real_estate','delegate_property_viewing','delegation.property_receipt');
 IF NOT FOUND OR body IS NULL THEN RETURN NEW; END IF;
 -- Conversational offers use the reply to the verified inbound request.
 -- Approval receipts continue through the existing consent-gated path.
 IF t.action IN ('delegate_real_estate','delegate_property_viewing') AND EXISTS(
  SELECT 1 FROM platform.agent_tasks root WHERE (root.organization_id,root.id,root.conversation_id)
   =(t.organization_id,t.idempotency_key,t.conversation_id) AND root.action='delegate_conversation') THEN RETURN NEW; END IF;
 IF t.action='delegate_property_viewing' AND NEW.result->0->>'state'<>'property_unavailable' THEN RETURN NEW; END IF;
 SELECT * INTO c FROM platform.conversations WHERE organization_id=t.organization_id AND id=t.conversation_id;
 IF c.channel='instagram' THEN
  outcome:='instagram_send_disabled';
 ELSIF NOT platform.booking_notification_consent(c.organization_id,c.id) AND c.channel<>'internal_chat' THEN
  outcome:='consent_required';
 ELSIF length(body) BETWEEN 1 AND 2000 AND platform.real_estate_notification_current(t.id,t.organization_id,t.conversation_id)
 AND ((c.channel='telegram' AND NOT EXISTS(SELECT 1 FROM platform.channel_principals WHERE organization_id=c.organization_id AND conversation_id=c.id)
      AND 1=(SELECT count(*) FROM platform.telegram_principals p WHERE p.organization_id=c.organization_id AND p.conversation_id=c.id)
      AND EXISTS(SELECT 1 FROM platform.telegram_principals p JOIN platform.telegram_integrations i ON (i.organization_id,i.id)=(p.organization_id,p.integration_id)
          WHERE p.organization_id=c.organization_id AND p.conversation_id=c.id AND i.active AND i.mode='mock'))
   OR (c.channel='whatsapp' AND NOT EXISTS(SELECT 1 FROM platform.telegram_principals WHERE organization_id=c.organization_id AND conversation_id=c.id)
      AND EXISTS(SELECT 1 FROM platform.channel_principals p JOIN platform.channel_integrations i ON (i.organization_id,i.id)=(p.organization_id,p.integration_id)
          WHERE (p.organization_id,p.conversation_id,p.id)=(c.organization_id,c.id,c.customer_id) AND p.channel='whatsapp' AND i.channel='whatsapp'
          AND i.active AND i.mode='mock' AND i.sending_enabled AND p.window_until>clock_timestamp()))) THEN
  INSERT INTO platform.messages(id,organization_id,conversation_id,role,content,data,client_key)
   VALUES(t.id,t.organization_id,t.conversation_id,'assistant',body,jsonb_build_object('real_estate_notification',true),t.id)
   ON CONFLICT DO NOTHING;
  IF EXISTS(SELECT 1 FROM platform.telegram_outbox WHERE id=t.id AND organization_id=t.organization_id)
   OR EXISTS(SELECT 1 FROM platform.channel_outbox WHERE id=t.id AND organization_id=t.organization_id) THEN outcome:='queued'; END IF;
 END IF;
 INSERT INTO platform.agent_audit(organization_id,task_id,execution_id,agent_id,action,result)
 VALUES(t.organization_id,t.id,NEW.id,t.agent_id,'property.delivery',outcome);
 RETURN NEW;
END $$;
UPDATE platform.service_metadata SET version=29 WHERE singleton;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM platform.conversation_routes)
 OR EXISTS(SELECT 1 FROM platform.agent_tasks WHERE action='delegate_conversation')
 OR EXISTS(SELECT 1 FROM platform.messages WHERE data ? 'booking_notification')
 OR EXISTS(SELECT 1 FROM platform.telegram_outbox WHERE booking_order_id IS NOT NULL)
 OR EXISTS(SELECT 1 FROM platform.channel_outbox WHERE booking_order_id IS NOT NULL)
 THEN RAISE EXCEPTION 'rollback refused: preserve conversations and booking history'; END IF;
END $$;
DROP TRIGGER conversational_channel_delivery ON platform.channel_outbox;
DROP TRIGGER conversational_telegram_delivery ON platform.telegram_outbox;
DROP TRIGGER conversation_approval_state ON platform.order_events;
DROP TRIGGER conversation_turn_finished ON platform.agent_tasks;
DROP TRIGGER conversation_execution_finished ON platform.agent_executions;
DROP FUNCTION platform.booking_notification_admit(uuid,text);
ALTER FUNCTION platform.prior_booking_notification_admit(uuid,text) RENAME TO booking_notification_admit;
GRANT EXECUTE ON FUNCTION platform.booking_notification_admit(uuid,text) TO aba_app;
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
DROP FUNCTION platform.audit_conversation_delivery(),platform.conversation_approval_state(),platform.advance_conversation_turn(),
 platform.finish_conversation_execution(),platform.conversation_publish(platform.agent_tasks,uuid,text),
 platform.conversation_current_reply(uuid,uuid,uuid),platform.conversation_plan_lookup(uuid,uuid),
 platform.conversation_intent_context(uuid,uuid),platform.conversation_route_submit(uuid,uuid,uuid,bytea),
 platform.conversation_route_allowed(uuid,uuid);
DROP INDEX platform.conversation_route_tasks;
DROP TABLE platform.conversation_routes;
CREATE OR REPLACE FUNCTION platform.enqueue_real_estate_notification() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE t platform.agent_tasks; c platform.conversations; outcome text:='channel_unavailable'; body text:=NEW.result->0->>'message';
BEGIN
 SELECT * INTO t FROM platform.agent_tasks WHERE organization_id=NEW.organization_id AND id=NEW.task_id
 AND agent_id='business_manager' AND action IN ('delegate_real_estate','delegate_property_viewing','delegation.property_receipt');
 IF NOT FOUND OR body IS NULL THEN RETURN NEW; END IF;
 IF t.action='delegate_property_viewing' AND NEW.result->0->>'state'<>'property_unavailable' THEN RETURN NEW; END IF;
 SELECT * INTO c FROM platform.conversations WHERE organization_id=t.organization_id AND id=t.conversation_id;
 IF c.channel='instagram' THEN
  outcome:='instagram_send_disabled';
 ELSIF NOT platform.booking_notification_consent(c.organization_id,c.id) AND c.channel<>'internal_chat' THEN
  outcome:='consent_required';
 ELSIF length(body) BETWEEN 1 AND 2000 AND platform.real_estate_notification_current(t.id,t.organization_id,t.conversation_id)
 AND ((c.channel='telegram' AND NOT EXISTS(SELECT 1 FROM platform.channel_principals WHERE organization_id=c.organization_id AND conversation_id=c.id)
      AND 1=(SELECT count(*) FROM platform.telegram_principals p WHERE p.organization_id=c.organization_id AND p.conversation_id=c.id)
      AND EXISTS(SELECT 1 FROM platform.telegram_principals p JOIN platform.telegram_integrations i ON (i.organization_id,i.id)=(p.organization_id,p.integration_id)
          WHERE p.organization_id=c.organization_id AND p.conversation_id=c.id AND i.active AND i.mode='mock'))
   OR (c.channel='whatsapp' AND NOT EXISTS(SELECT 1 FROM platform.telegram_principals WHERE organization_id=c.organization_id AND conversation_id=c.id)
      AND EXISTS(SELECT 1 FROM platform.channel_principals p JOIN platform.channel_integrations i ON (i.organization_id,i.id)=(p.organization_id,p.integration_id)
          WHERE (p.organization_id,p.conversation_id,p.id)=(c.organization_id,c.id,c.customer_id) AND p.channel='whatsapp' AND i.channel='whatsapp'
          AND i.active AND i.mode='mock' AND i.sending_enabled AND p.window_until>clock_timestamp()))) THEN
  INSERT INTO platform.messages(id,organization_id,conversation_id,role,content,data,client_key)
   VALUES(t.id,t.organization_id,t.conversation_id,'assistant',body,jsonb_build_object('real_estate_notification',true),t.id)
   ON CONFLICT DO NOTHING;
  IF EXISTS(SELECT 1 FROM platform.telegram_outbox WHERE id=t.id AND organization_id=t.organization_id)
   OR EXISTS(SELECT 1 FROM platform.channel_outbox WHERE id=t.id AND organization_id=t.organization_id) THEN outcome:='queued'; END IF;
 END IF;
 INSERT INTO platform.agent_audit(organization_id,task_id,execution_id,agent_id,action,result)
 VALUES(t.organization_id,t.id,NEW.id,t.agent_id,'property.delivery',outcome);
 RETURN NEW;
END $$;
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
 OR (action IN ('delegate_auto_parts','delegate_construction','delegate_service','delegate_real_estate','delegate_property_viewing') AND agent_id='business_manager' AND parent_id IS NULL
     AND correlation_id=id AND conversation_id IS NOT NULL AND variant_id IS NULL AND delegation_input IS NOT NULL AND delegation_step IS NULL)
 OR (action LIKE 'delegation.%' AND parent_id IS NOT NULL AND conversation_id IS NOT NULL
     AND variant_id IS NULL AND delegation_step BETWEEN 1 AND 20 AND delegation_input IS NULL));
CREATE OR REPLACE FUNCTION platform.agent_delegation_profile(p_org uuid,p_action text) RETURNS boolean
LANGUAGE sql STABLE SET search_path=pg_catalog,platform AS $$
 SELECT EXISTS(SELECT 1 FROM platform.organizations o WHERE o.id=p_org AND o.is_test AND CASE p_action
 WHEN 'delegate_auto_parts' THEN o.industry_profile='auto_parts' AND o.business_model IN ('trade','hybrid')
 WHEN 'delegate_construction' THEN o.industry_profile='building_materials' AND o.business_model IN ('trade','hybrid')
 WHEN 'delegate_service' THEN (o.industry_profile='vehicle_service' AND o.business_model='service')
   OR (o.industry_profile='mixed_nonfood' AND o.business_model='hybrid')
 WHEN 'delegate_real_estate' THEN o.industry_profile='real_estate' AND o.business_model='agency'
 WHEN 'delegate_property_viewing' THEN o.industry_profile='real_estate' AND o.business_model='agency'
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
 ('delegate_service',5,'sales','delegation.service_proposal'),
 ('delegate_real_estate',1,'customer','delegation.property_requirements'),
 ('delegate_real_estate',2,'sales','delegation.property_search'),
 ('delegate_real_estate',3,'sales','delegation.property_offer'),
 ('delegate_property_viewing',1,'customer','delegation.property_selection'),
 ('delegate_property_viewing',2,'sales','delegation.property_viewing')
 ) v(workflow,step,agent,operation)

 WHERE workflow=p_action
$$;
DROP FUNCTION platform.agent_enqueue_delegation(uuid,uuid,uuid,uuid,text,jsonb,bytea,uuid);
UPDATE platform.service_metadata SET version=28 WHERE singleton;
-- +goose StatementEnd
