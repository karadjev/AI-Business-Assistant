-- +goose Up
-- +goose StatementBegin
-- Equivalent tenant SELECT authorization, evaluated as an InitPlan per statement.
-- Row tenant equality, live session/membership checks, channel policies, FORCE RLS
-- and write policies are preserved. No cached authorization survives a statement.
DO $$DECLARE t text;owner_only boolean;BEGIN
 FOREACH t IN ARRAY ARRAY['products','product_variants','inventory','catalog_policies'] LOOP
 owner_only=(t='catalog_policies');
 EXECUTE format('ALTER POLICY tenant_read ON platform.%I USING (
 organization_id::text=current_setting(''app.organization_id'',true) AND
 (SELECT platform.has_membership(CASE WHEN current_setting(''app.organization_id'',true) ~ ''^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$''
 THEN current_setting(''app.organization_id'',true)::uuid ELSE NULL END,%L)))',t,owner_only);
 END LOOP;
END $$;
-- An absent principal already makes scope false. Test it before expensive scope
-- evaluation; with a capability present the original lease/tenant checks still run.
DO $$DECLARE p record;setting text;BEGIN
 FOR p IN SELECT pol.polname,cl.relname,pg_get_expr(pol.polqual,pol.polrelid) expr FROM pg_policy pol JOIN pg_class cl ON cl.oid=pol.polrelid JOIN pg_namespace ns ON ns.oid=cl.relnamespace WHERE ns.nspname='platform' AND cl.relname IN ('products','product_variants','inventory') AND pol.polname IN ('telegram_read','whatsapp_read') LOOP
 setting=CASE WHEN p.polname='telegram_read' THEN 'app.telegram_cap' ELSE 'app.channel_cap' END;
 EXECUTE format('ALTER POLICY %I ON platform.%I USING (nullif(current_setting(%L,true),'''') IS NOT NULL AND (%s))',p.polname,p.relname,setting,p.expr);
 END LOOP;
END $$;
-- Shared freshness calculation: the centre can load one policy per statement,
-- while reserve/customer/dispatch keep resolving the policy in the old facade.
CREATE FUNCTION platform.inventory_freshness_at(p_updated timestamptz,p_fresh integer,p_stale integer)
RETURNS text LANGUAGE sql STABLE AS $$
 SELECT CASE WHEN p_updated IS NULL OR p_updated>pg_catalog.now() THEN 'UNKNOWN'
 WHEN p_updated>=pg_catalog.now()-pg_catalog.make_interval(hours=>p_fresh) THEN 'FRESH'
 WHEN p_updated>=pg_catalog.now()-pg_catalog.make_interval(hours=>p_stale) THEN 'AGING' ELSE 'STALE' END $$;
REVOKE ALL ON FUNCTION platform.inventory_freshness_at(timestamptz,integer,integer) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.inventory_freshness_at(timestamptz,integer,integer) TO aba_app;
CREATE OR REPLACE FUNCTION platform.inventory_freshness(p_org uuid,p_updated timestamptz)
RETURNS text LANGUAGE sql STABLE SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT platform.inventory_freshness_at(p_updated,
 coalesce((SELECT fresh_hours FROM platform.catalog_policies WHERE organization_id=p_org),24),
 coalesce((SELECT stale_hours FROM platform.catalog_policies WHERE organization_id=p_org),72)) $$;
UPDATE platform.service_metadata SET version=15 WHERE singleton;
-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
-- Keep the existing latest-migration refusal on populated catalog/provider state.
DO $$BEGIN IF EXISTS(SELECT 1 FROM platform.products) OR EXISTS(SELECT 1 FROM platform.catalog_policies) OR EXISTS(SELECT 1 FROM platform.catalog_events) OR EXISTS(SELECT 1 FROM platform.channel_principals) THEN RAISE EXCEPTION 'rollback refused: preserve populated catalog and provider state';END IF;END $$;
DO $$DECLARE t text;BEGIN
 FOREACH t IN ARRAY ARRAY['products','product_variants','inventory'] LOOP
 EXECUTE format('ALTER POLICY tenant_read ON platform.%I USING(organization_id::text=current_setting(''app.organization_id'',true) AND platform.has_membership(organization_id,false))',t);
 END LOOP;
END $$;
ALTER POLICY tenant_read ON platform.catalog_policies USING(organization_id::text=current_setting('app.organization_id',true) AND platform.has_membership(organization_id,true));
CREATE OR REPLACE FUNCTION platform.inventory_freshness(p_org uuid,p_updated timestamptz) RETURNS text LANGUAGE sql STABLE SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT CASE WHEN p_updated IS NULL OR p_updated>now() THEN 'UNKNOWN'
 WHEN p_updated>=now()-make_interval(hours=>coalesce((SELECT fresh_hours FROM platform.catalog_policies WHERE organization_id=p_org),24)) THEN 'FRESH'
 WHEN p_updated>=now()-make_interval(hours=>coalesce((SELECT stale_hours FROM platform.catalog_policies WHERE organization_id=p_org),72)) THEN 'AGING' ELSE 'STALE' END $$;
-- Restore the unchanged predicates from00005 and their00010 WhatsApp copies.
DO $$DECLARE n text;scope text;BEGIN
 FOREACH n IN ARRAY ARRAY['telegram_read','whatsapp_read'] LOOP
 scope=CASE WHEN n='telegram_read' THEN 'platform.telegram_scope' ELSE 'platform.channel_scope' END;
 EXECUTE format('ALTER POLICY %I ON platform.products USING(%s(organization_id,NULL) AND status=''active'')',n,scope);
 EXECUTE format('ALTER POLICY %I ON platform.product_variants USING(%s(organization_id,NULL) AND EXISTS(SELECT 1 FROM platform.products p WHERE p.organization_id=product_variants.organization_id AND p.id=product_variants.product_id AND p.status=''active''))',n,scope);
 EXECUTE format('ALTER POLICY %I ON platform.inventory USING(%s(organization_id,NULL) AND EXISTS(SELECT 1 FROM platform.product_variants v WHERE v.organization_id=inventory.organization_id AND v.id=inventory.variant_id))',n,scope);
 END LOOP;
END $$;
DROP FUNCTION platform.inventory_freshness_at(timestamptz,integer,integer);
UPDATE platform.service_metadata SET version=14 WHERE singleton;
-- +goose StatementEnd
