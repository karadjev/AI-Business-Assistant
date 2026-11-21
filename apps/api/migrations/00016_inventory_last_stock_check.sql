-- +goose Up
-- +goose StatementBegin
-- Keep the tenant-scoped MAX bounded to the newest non-null timestamp. Without
-- an ordered index, the RLS scan's estimated cost can trigger expensive JIT
-- compilation even for a small tenant in an accumulated multi-tenant catalog.
CREATE INDEX inventory_last_stock_check ON platform.inventory(organization_id,manual_updated_at DESC) WHERE manual_updated_at IS NOT NULL;
-- The row tenant equality and identical owner/staff predicates remain in
-- USING and WITH CHECK. Session, membership and order permission are checked
-- per statement, never cached across statements or tenants.
ALTER POLICY tenant_update ON platform.inventory USING(organization_id::text=current_setting('app.organization_id',true) AND (SELECT platform.has_membership(CASE WHEN current_setting('app.organization_id',true) ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' THEN current_setting('app.organization_id',true)::uuid ELSE NULL END,true))) WITH CHECK(organization_id::text=current_setting('app.organization_id',true) AND (SELECT platform.has_membership(CASE WHEN current_setting('app.organization_id',true) ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' THEN current_setting('app.organization_id',true)::uuid ELSE NULL END,true)));
ALTER POLICY staff_stock ON platform.inventory USING(organization_id::text=current_setting('app.organization_id',true) AND (SELECT platform.is_test_org(CASE WHEN current_setting('app.organization_id',true) ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' THEN current_setting('app.organization_id',true)::uuid ELSE NULL END) AND platform.can_operate_orders(CASE WHEN current_setting('app.organization_id',true) ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' THEN current_setting('app.organization_id',true)::uuid ELSE NULL END))) WITH CHECK(organization_id::text=current_setting('app.organization_id',true) AND (SELECT platform.is_test_org(CASE WHEN current_setting('app.organization_id',true) ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' THEN current_setting('app.organization_id',true)::uuid ELSE NULL END) AND platform.can_operate_orders(CASE WHEN current_setting('app.organization_id',true) ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' THEN current_setting('app.organization_id',true)::uuid ELSE NULL END)));
UPDATE platform.service_metadata SET version=16 WHERE singleton;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Preserve the latest migration's existing populated-catalog rollback refusal.
DO $$BEGIN IF EXISTS(SELECT 1 FROM platform.products) OR EXISTS(SELECT 1 FROM platform.catalog_policies) OR EXISTS(SELECT 1 FROM platform.catalog_events) OR EXISTS(SELECT 1 FROM platform.channel_principals) THEN RAISE EXCEPTION 'rollback refused: preserve populated catalog and provider state';END IF;END $$;
ALTER POLICY tenant_update ON platform.inventory USING(organization_id::text=current_setting('app.organization_id',true) AND platform.has_membership(organization_id,true)) WITH CHECK(organization_id::text=current_setting('app.organization_id',true) AND platform.has_membership(organization_id,true));
ALTER POLICY staff_stock ON platform.inventory USING(organization_id::text=current_setting('app.organization_id',true) AND platform.is_test_org(organization_id) AND platform.can_operate_orders(organization_id)) WITH CHECK(organization_id::text=current_setting('app.organization_id',true) AND platform.is_test_org(organization_id) AND platform.can_operate_orders(organization_id));
DROP INDEX platform.inventory_last_stock_check;
UPDATE platform.service_metadata SET version=15 WHERE singleton;
-- +goose StatementEnd
