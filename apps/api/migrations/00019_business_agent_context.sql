-- +goose Up
-- +goose StatementBegin
ALTER TABLE platform.organizations
 ADD COLUMN business_model text NOT NULL DEFAULT 'trade',
 ADD COLUMN industry_profile text NOT NULL DEFAULT 'electronics',
 ADD COLUMN business_profile_revision integer NOT NULL DEFAULT 1 CHECK(business_profile_revision>0),
 ADD CONSTRAINT industry_model_pair CHECK (
   (business_model='trade' AND industry_profile IN ('electronics','auto_parts','building_materials','furniture','apparel'))
   OR (business_model='service' AND industry_profile IN ('vehicle_service','personal_services'))
   OR (business_model='agency' AND industry_profile='real_estate')
   OR (business_model='hybrid' AND industry_profile='mixed_nonfood'));
-- Preserve the earlier self-parent constraint and replace only the operation admission check.
DO $$ DECLARE rule_name name; BEGIN
 SELECT conname INTO STRICT rule_name FROM pg_constraint
  WHERE conrelid='platform.agent_tasks'::regclass AND contype='c' AND pg_get_constraintdef(oid) LIKE '%get_product_price%';
 EXECUTE format('ALTER TABLE platform.agent_tasks DROP CONSTRAINT %I',rule_name);
END $$;
ALTER TABLE platform.agent_tasks ADD CONSTRAINT agent_task_operation CHECK (
 (agent_id IN ('customer','sales','product_inventory') AND action IN ('get_product_price','check_inventory','get_product_details') AND variant_id IS NOT NULL AND conversation_id IS NOT NULL)
 OR (variant_id IS NULL AND conversation_id IS NULL AND ((agent_id='marketing' AND action='draft') OR
 (agent_id IN ('finance','analytics') AND action='read') OR (agent_id='business_manager' AND action IN ('plan','coordinate')))));
CREATE FUNCTION platform.agent_update_business_profile(p_org uuid,p_model text,p_industry text,p_revision integer) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
BEGIN
 IF NOT platform.agent_owner(p_org) THEN RAISE EXCEPTION 'profile write denied' USING ERRCODE='42501'; END IF;
 UPDATE platform.organizations
 SET business_model=p_model,industry_profile=p_industry,business_profile_revision=business_profile_revision+1
 WHERE id=p_org AND business_profile_revision=p_revision;
 RETURN FOUND;
END $$;
REVOKE ALL ON FUNCTION platform.agent_update_business_profile(uuid,text,text,integer) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.agent_update_business_profile(uuid,text,text,integer) TO aba_app;
CREATE FUNCTION platform.agent_business_profile(p_task uuid,p_lease uuid) RETURNS jsonb
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT jsonb_build_object('model',o.business_model,'industry',o.industry_profile,'revision',o.business_profile_revision,'timezone',o.timezone)
 FROM platform.agent_lease_task(p_task,p_lease) t JOIN platform.organizations o ON o.id=t.organization_id
$$;
REVOKE ALL ON FUNCTION platform.agent_business_profile(uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.agent_business_profile(uuid,uuid) TO aba_app;
CREATE OR REPLACE FUNCTION platform.agent_catalog_scope(p_org uuid,p_variant uuid DEFAULT NULL,p_product uuid DEFAULT NULL) RETURNS boolean LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT EXISTS(SELECT 1 FROM platform.agent_lease_task(
 nullif(current_setting('app.agent_task',true),'')::uuid,nullif(current_setting('app.agent_lease',true),'')::uuid) t
 WHERE t.organization_id=p_org AND t.agent_id IN ('customer','sales','product_inventory') AND t.action IN ('get_product_price','check_inventory','get_product_details')
 AND p_org::text=current_setting('app.organization_id',true)
 AND EXISTS(SELECT 1 FROM platform.organizations o WHERE o.id=p_org AND o.business_model IN ('trade','hybrid'))
 AND (p_variant IS NULL OR p_variant=t.variant_id)
 AND (p_product IS NULL OR EXISTS(SELECT 1 FROM platform.product_variants v WHERE v.organization_id=p_org AND v.id=t.variant_id AND v.product_id=p_product))
 AND EXISTS(SELECT 1 FROM platform.conversations c WHERE c.organization_id=p_org AND c.id=t.conversation_id AND c.status='open' AND c.expires_at>clock_timestamp()))
$$;
CREATE POLICY agent_catalog_policy_read ON platform.catalog_policies FOR SELECT TO aba_app
 USING(CASE WHEN nullif(current_setting('app.agent_lease',true),'') IS NOT NULL
 THEN platform.agent_catalog_scope(organization_id) ELSE false END);
CREATE OR REPLACE FUNCTION platform.agent_metrics_projection(p_task uuid,p_lease uuid) RETURNS jsonb
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT jsonb_build_object(
 'count',count(o.id) FILTER(WHERE o.created_at>=now()-interval '30 days'),
 'revenue',coalesce(sum(o.total_amount) FILTER(WHERE o.created_at>=now()-interval '30 days'),0),
 'previous_count',count(o.id) FILTER(WHERE o.created_at<now()-interval '30 days'),
 'previous_value',coalesce(sum(o.total_amount) FILTER(WHERE o.created_at<now()-interval '30 days'),0))
 FROM platform.agent_lease_task(p_task,p_lease) t
 LEFT JOIN platform.orders o ON o.organization_id=t.organization_id AND o.status='completed' AND o.created_at>=now()-interval '60 days' AND o.created_at<=now()
 WHERE t.agent_id IN ('analytics','finance') GROUP BY t.id
$$;
UPDATE platform.service_metadata SET version=19 WHERE singleton;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM platform.organizations WHERE business_profile_revision<>1 OR business_model<>'trade' OR industry_profile<>'electronics')
 OR EXISTS(SELECT 1 FROM platform.agent_tasks) THEN
  RAISE EXCEPTION 'rollback refused: preserve universal profile and agent history';
 END IF;
END $$;
DROP POLICY agent_catalog_policy_read ON platform.catalog_policies;
DROP FUNCTION platform.agent_business_profile(uuid,uuid),platform.agent_update_business_profile(uuid,text,text,integer);
ALTER TABLE platform.agent_tasks DROP CONSTRAINT agent_task_operation;
ALTER TABLE platform.agent_tasks ADD CONSTRAINT agent_task_operation CHECK (
 (agent_id IN ('customer','sales','product_inventory') AND action='get_product_price' AND variant_id IS NOT NULL AND conversation_id IS NOT NULL)
 OR (variant_id IS NULL AND conversation_id IS NULL AND ((agent_id='marketing' AND action='draft') OR
 (agent_id IN ('finance','analytics') AND action='read') OR (agent_id='business_manager' AND action IN ('plan','coordinate')))));
CREATE OR REPLACE FUNCTION platform.agent_catalog_scope(p_org uuid,p_variant uuid DEFAULT NULL,p_product uuid DEFAULT NULL) RETURNS boolean LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT EXISTS(SELECT 1 FROM platform.agent_lease_task(
 nullif(current_setting('app.agent_task',true),'')::uuid,nullif(current_setting('app.agent_lease',true),'')::uuid) t
 WHERE t.organization_id=p_org AND t.agent_id IN ('customer','sales','product_inventory') AND t.action='get_product_price'
 AND p_org::text=current_setting('app.organization_id',true)
 AND (p_variant IS NULL OR p_variant=t.variant_id)
 AND (p_product IS NULL OR EXISTS(SELECT 1 FROM platform.product_variants v WHERE v.organization_id=p_org AND v.id=t.variant_id AND v.product_id=p_product))
 AND EXISTS(SELECT 1 FROM platform.conversations c WHERE c.organization_id=p_org AND c.id=t.conversation_id AND c.status='open' AND c.expires_at>clock_timestamp()))
$$;
CREATE OR REPLACE FUNCTION platform.agent_metrics_projection(p_task uuid,p_lease uuid) RETURNS jsonb
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT jsonb_build_object('count',count(o.id),'revenue',coalesce(sum(o.total_amount),0))
 FROM platform.agent_lease_task(p_task,p_lease) t
 LEFT JOIN platform.orders o ON o.organization_id=t.organization_id AND o.status='completed' AND o.created_at>=now()-interval '30 days'
 WHERE t.agent_id IN ('analytics','finance') GROUP BY t.id
$$;
ALTER TABLE platform.organizations DROP CONSTRAINT industry_model_pair,DROP COLUMN business_model,DROP COLUMN industry_profile,DROP COLUMN business_profile_revision;
UPDATE platform.service_metadata SET version=18 WHERE singleton;
-- +goose StatementEnd
