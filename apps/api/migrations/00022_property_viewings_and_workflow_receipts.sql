-- +goose Up
-- +goose StatementBegin
ALTER TABLE platform.property_listings
 ADD COLUMN price_amount bigint,
 ADD COLUMN currency_code text,
 ADD COLUMN property_type text CHECK(property_type IN ('apartment','house','commercial','land','other')),
 ADD COLUMN viewing_offering_id uuid,
 ADD CONSTRAINT property_price_currency CHECK (
  (price_amount IS NULL AND currency_code IS NULL)
  OR (price_amount IS NOT NULL AND price_amount>=0 AND currency_code IS NOT NULL AND currency_code='RUB')),
 ADD CONSTRAINT property_viewing_offering_fk FOREIGN KEY (organization_id,viewing_offering_id)
  REFERENCES platform.service_offerings(organization_id,id);

CREATE FUNCTION platform.property_viewing_matches(p_org uuid,p_listing uuid,p_offering uuid) RETURNS boolean
LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
BEGIN
 IF session_user<>'aba_app' OR p_org::text IS DISTINCT FROM current_setting('app.organization_id',true)
 OR NOT coalesce(platform.is_test_org(p_org),false) OR NOT coalesce(platform.can_operate_orders(p_org),false)
 THEN RETURN false; END IF;
 PERFORM 1 FROM platform.property_listings
 WHERE organization_id=p_org AND id=p_listing AND viewing_offering_id=p_offering
 AND status='available' AND verified_until>clock_timestamp()
 FOR SHARE;
 RETURN FOUND;
END $$;
REVOKE ALL ON FUNCTION platform.property_viewing_matches(uuid,uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.property_viewing_matches(uuid,uuid,uuid) TO aba_app;
CREATE OR REPLACE FUNCTION platform.service_booking_scope(p_org uuid) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE model text;
BEGIN
 IF session_user<>'aba_app' THEN RETURN false; END IF;
 IF p_org::text IS DISTINCT FROM current_setting('app.organization_id',true) OR NOT platform.is_test_org(p_org) OR NOT platform.can_operate_orders(p_org) THEN RETURN false; END IF;
 SELECT business_model INTO model FROM platform.organizations WHERE id=p_org FOR SHARE;
 RETURN coalesce(model IN ('service','hybrid','agency'),false);
END $$;
CREATE OR REPLACE FUNCTION platform.service_booking_guard() RETURNS trigger LANGUAGE plpgsql
SET search_path=pg_catalog,platform,pg_temp AS $$
BEGIN
 IF TG_OP='UPDATE' THEN
  IF (to_jsonb(NEW)-'status') IS DISTINCT FROM (to_jsonb(OLD)-'status') OR OLD.status<>'confirmed' OR NEW.status NOT IN ('cancelled','completed')
  OR (NEW.status='completed' AND (NEW.ends_at>clock_timestamp() OR NOT EXISTS(
  SELECT 1 FROM platform.orders r WHERE r.organization_id=NEW.organization_id AND r.id=NEW.request_id AND r.request_kind='hybrid_order' AND r.status='completed'))) THEN
   RAISE EXCEPTION 'immutable booking' USING ERRCODE='23514';
  END IF;
 ELSE
  PERFORM 1 FROM platform.service_offerings o WHERE o.organization_id=NEW.organization_id AND o.id=NEW.offering_id AND o.status='active' AND o.revision=NEW.offering_revision AND o.timezone=NEW.timezone AND o.min_minutes>0 AND NEW.ends_at-NEW.starts_at BETWEEN make_interval(mins=>o.min_minutes) AND make_interval(mins=>o.max_minutes) FOR SHARE;
  IF NOT FOUND OR NEW.starts_at<=clock_timestamp() THEN RAISE EXCEPTION 'service source changed' USING ERRCODE='23514'; END IF;
  IF NEW.status<>'confirmed' OR NOT EXISTS(SELECT 1 FROM platform.orders r WHERE r.organization_id=NEW.organization_id AND r.id=NEW.request_id AND r.status='confirmed' AND (r.request_kind='service_booking' OR (r.request_kind='hybrid_order' AND r.request_details ? 'selection')
   OR (r.request_kind='property_viewing_request' AND platform.property_viewing_matches(NEW.organization_id,r.listing_id,NEW.offering_id)))
   AND r.request_details->'selection'->>'offering_id'=NEW.offering_id::text
   AND (r.request_details->'selection'->>'offering_revision')::bigint=NEW.offering_revision
   AND (r.request_details->'selection'->>'starts_at')::timestamptz=NEW.starts_at
   AND (r.request_details->'selection'->>'ends_at')::timestamptz=NEW.ends_at
   AND r.request_details->'selection'->>'timezone'=NEW.timezone
   AND (r.request_details->'selection'->>'units')::int=NEW.units) THEN
   RAISE EXCEPTION 'confirmed service request required' USING ERRCODE='23514';
  END IF;
 END IF;
 RETURN NEW;
END $$;
ALTER TABLE platform.agent_tasks DROP CONSTRAINT agent_task_operation;
ALTER TABLE platform.agent_tasks ADD CONSTRAINT agent_task_operation CHECK (
 (agent_id IN ('customer','sales','product_inventory') AND action IN ('get_product_price','check_inventory','get_product_details') AND variant_id IS NOT NULL AND conversation_id IS NOT NULL)
 OR (variant_id IS NULL AND conversation_id IS NULL AND ((agent_id='marketing' AND action='draft') OR
 (agent_id IN ('finance','analytics') AND action='read') OR (agent_id='business_manager' AND action IN ('plan','coordinate','coordinate_growth'))))
 OR (variant_id IS NULL AND conversation_id IS NULL AND parent_id IS NOT NULL AND
 ((agent_id='sales' AND action='review_sales') OR (agent_id='product_inventory' AND action='review_inventory')))
 OR (agent_id='customer' AND action IN ('workflow_preview_B','workflow_preview_C','workflow_preview_D','workflow_preview_E')
     AND conversation_id IS NOT NULL AND variant_id IS NULL AND parent_id IS NULL
     AND status IN ('completed','failed') AND correlation_id=id));

CREATE FUNCTION platform.agent_workflow_receipt(p_org uuid,p_conversation uuid,p_id uuid,p_key uuid,p_scenario text,p_result jsonb,p_failed boolean)
RETURNS void
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE uid uuid:=platform.current_user_id(); ex uuid:=gen_random_uuid(); operation text:='workflow_preview_'||p_scenario;
 ts timestamptz:=clock_timestamp(); outcome text:=CASE WHEN p_failed THEN 'failed' ELSE 'completed' END;
 failure text:=CASE WHEN p_failed THEN 'execution' ELSE NULL END;
BEGIN
 IF p_scenario IS NULL OR p_scenario NOT IN ('B','C','D','E') OR p_failed IS NULL
 OR NOT EXISTS(SELECT 1 FROM platform.organizations o JOIN platform.organization_members m ON m.organization_id=o.id
               WHERE o.id=p_org AND o.id::text=current_setting('app.organization_id',true) AND o.is_test
               AND m.user_id=uid AND (m.role='owner' OR m.can_manage_orders))
 OR NOT EXISTS(SELECT 1 FROM platform.conversations c WHERE c.organization_id=p_org AND c.id=p_conversation
               AND c.status='open' AND c.expires_at>ts)
 THEN RAISE EXCEPTION 'workflow preview scope unavailable' USING ERRCODE='42501'; END IF;
 IF (p_failed AND p_result IS NOT NULL) OR (NOT p_failed AND (jsonb_typeof(p_result) IS DISTINCT FROM 'object'
     OR p_result->>'scenario' IS DISTINCT FROM p_scenario OR octet_length(p_result::text)>32768))
 THEN RAISE EXCEPTION 'invalid workflow receipt' USING ERRCODE='23514'; END IF;
 PERFORM pg_advisory_xact_lock(hashtextextended(p_org::text,8231));
 IF EXISTS(SELECT 1 FROM platform.agent_tasks WHERE organization_id=p_org AND initiator=uid AND idempotency_key=p_key)
 THEN RAISE EXCEPTION 'workflow preview already used' USING ERRCODE='23505'; END IF;
 IF (SELECT count(*) FROM platform.agent_tasks WHERE organization_id=p_org AND created_at>ts-interval '1 minute')>=20
 THEN RAISE EXCEPTION 'agent task rate limit' USING ERRCODE='54000'; END IF;
 INSERT INTO platform.agent_tasks(id,organization_id,initiator,agent_id,correlation_id,conversation_id,action,idempotency_key,worker_hash,status,deadline,started_at,completed_at,error_code)
 VALUES(p_id,p_org,uid,'customer',p_id,p_conversation,operation,p_key,sha256(uuid_send(gen_random_uuid())),outcome,ts,ts,ts,failure);
 INSERT INTO platform.agent_executions(id,organization_id,task_id,agent_version,attempt,completed_at,status,tool_calls,result,error_code)
 VALUES(ex,p_org,p_id,'workflow-v1',1,ts,outcome,1,p_result,failure);
 INSERT INTO platform.agent_audit(organization_id,task_id,execution_id,actor_id,agent_id,action,result,error_code)
 VALUES(p_org,p_id,ex,uid,'customer',operation,outcome,failure);
END $$;
REVOKE ALL ON FUNCTION platform.agent_workflow_receipt(uuid,uuid,uuid,uuid,text,jsonb,boolean) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.agent_workflow_receipt(uuid,uuid,uuid,uuid,text,jsonb,boolean) TO aba_app;
UPDATE platform.service_metadata SET version=22 WHERE singleton;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM platform.agent_tasks)
 THEN RAISE EXCEPTION 'rollback refused: preserve agent tasks and workflow preview evidence'; END IF;
END $$;
DROP FUNCTION platform.agent_workflow_receipt(uuid,uuid,uuid,uuid,text,jsonb,boolean);
ALTER TABLE platform.agent_tasks DROP CONSTRAINT agent_task_operation;
ALTER TABLE platform.agent_tasks ADD CONSTRAINT agent_task_operation CHECK (
 (agent_id IN ('customer','sales','product_inventory') AND action IN ('get_product_price','check_inventory','get_product_details') AND variant_id IS NOT NULL AND conversation_id IS NOT NULL)
 OR (variant_id IS NULL AND conversation_id IS NULL AND ((agent_id='marketing' AND action='draft') OR
 (agent_id IN ('finance','analytics') AND action='read') OR (agent_id='business_manager' AND action IN ('plan','coordinate','coordinate_growth'))))
 OR (variant_id IS NULL AND conversation_id IS NULL AND parent_id IS NOT NULL AND
 ((agent_id='sales' AND action='review_sales') OR (agent_id='product_inventory' AND action='review_inventory'))));
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM platform.property_listings WHERE price_amount IS NOT NULL OR property_type IS NOT NULL OR viewing_offering_id IS NOT NULL)
 OR EXISTS(SELECT 1 FROM platform.orders WHERE request_kind='property_viewing_request' AND request_details ? 'selection')
 THEN RAISE EXCEPTION 'rollback refused: preserve property viewing configuration and history'; END IF;
END $$;
CREATE OR REPLACE FUNCTION platform.service_booking_guard() RETURNS trigger LANGUAGE plpgsql
SET search_path=pg_catalog,platform,pg_temp AS $$
BEGIN
 IF TG_OP='UPDATE' THEN
  IF (to_jsonb(NEW)-'status') IS DISTINCT FROM (to_jsonb(OLD)-'status') OR OLD.status<>'confirmed' OR NEW.status NOT IN ('cancelled','completed')
  OR (NEW.status='completed' AND (NEW.ends_at>clock_timestamp() OR NOT EXISTS(
  SELECT 1 FROM platform.orders r WHERE r.organization_id=NEW.organization_id AND r.id=NEW.request_id AND r.request_kind='hybrid_order' AND r.status='completed'))) THEN
   RAISE EXCEPTION 'immutable booking' USING ERRCODE='23514';
  END IF;
 ELSE
  PERFORM 1 FROM platform.service_offerings o WHERE o.organization_id=NEW.organization_id AND o.id=NEW.offering_id AND o.status='active' AND o.revision=NEW.offering_revision AND o.timezone=NEW.timezone AND o.min_minutes>0 AND NEW.ends_at-NEW.starts_at BETWEEN make_interval(mins=>o.min_minutes) AND make_interval(mins=>o.max_minutes) FOR SHARE;
  IF NOT FOUND OR NEW.starts_at<=clock_timestamp() THEN RAISE EXCEPTION 'service source changed' USING ERRCODE='23514'; END IF;
  IF NEW.status<>'confirmed' OR NOT EXISTS(SELECT 1 FROM platform.orders r WHERE r.organization_id=NEW.organization_id AND r.id=NEW.request_id AND r.status='confirmed' AND (r.request_kind='service_booking' OR (r.request_kind='hybrid_order' AND r.request_details ? 'selection'))
   AND r.request_details->'selection'->>'offering_id'=NEW.offering_id::text
   AND (r.request_details->'selection'->>'offering_revision')::bigint=NEW.offering_revision
   AND (r.request_details->'selection'->>'starts_at')::timestamptz=NEW.starts_at
   AND (r.request_details->'selection'->>'ends_at')::timestamptz=NEW.ends_at
   AND r.request_details->'selection'->>'timezone'=NEW.timezone
   AND (r.request_details->'selection'->>'units')::int=NEW.units) THEN
   RAISE EXCEPTION 'confirmed service request required' USING ERRCODE='23514';
  END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE OR REPLACE FUNCTION platform.service_booking_scope(p_org uuid) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE model text;
BEGIN
 IF session_user<>'aba_app' THEN RETURN false; END IF;
 IF p_org::text IS DISTINCT FROM current_setting('app.organization_id',true) OR NOT platform.is_test_org(p_org) OR NOT platform.can_operate_orders(p_org) THEN RETURN false; END IF;
 SELECT business_model INTO model FROM platform.organizations WHERE id=p_org FOR SHARE;
 RETURN coalesce(model IN ('service','hybrid'),false);
END $$;
DROP FUNCTION platform.property_viewing_matches(uuid,uuid,uuid);
ALTER TABLE platform.property_listings DROP COLUMN viewing_offering_id, DROP COLUMN property_type,
 DROP COLUMN currency_code, DROP COLUMN price_amount;
UPDATE platform.service_metadata SET version=21 WHERE singleton;
-- +goose StatementEnd
