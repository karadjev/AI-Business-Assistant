-- +goose Up
-- +goose StatementBegin
CREATE TABLE platform.catalog_policies(
 organization_id uuid PRIMARY KEY REFERENCES platform.organizations(id),cleanup_hash bytea CHECK(cleanup_hash IS NULL OR octet_length(cleanup_hash)=32),
 fresh_hours integer NOT NULL DEFAULT 24 CHECK(fresh_hours BETWEEN 1 AND 720),
 stale_hours integer NOT NULL DEFAULT 72 CHECK(stale_hours>=fresh_hours AND stale_hours<=2160),
 low_stock integer NOT NULL DEFAULT 3 CHECK(low_stock BETWEEN 0 AND 1000000),
 preview_days integer NOT NULL DEFAULT 1 CHECK(preview_days BETWEEN 1 AND 30),
 error_days integer NOT NULL DEFAULT 7 CHECK(error_days BETWEEN 1 AND 365 AND error_days>=preview_days),
 change_days integer NOT NULL DEFAULT 90 CHECK(change_days BETWEEN 30 AND 3650),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0));
CREATE TABLE platform.catalog_source_changes(
 id uuid PRIMARY KEY,organization_id uuid NOT NULL REFERENCES platform.organizations(id),
 product_id uuid NOT NULL, target text NOT NULL CHECK(target IN ('MANUAL','EXCEL')),
 reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 500),
 snapshot jsonb NOT NULL, status text NOT NULL DEFAULT 'PREVIEW' CHECK(status IN ('PREVIEW','APPLIED')),
 actor_id uuid NOT NULL REFERENCES platform.users(id),created_at timestamptz NOT NULL DEFAULT now(),
 expires_at timestamptz NOT NULL DEFAULT now()+interval '15 minutes',applied_at timestamptz,
 UNIQUE(organization_id,id),FOREIGN KEY(organization_id,product_id) REFERENCES platform.products(organization_id,id));
CREATE FUNCTION platform.catalog_source_history_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,platform,pg_temp AS $$BEGIN
 IF (to_jsonb(NEW)-'status'-'applied_at') IS DISTINCT FROM (to_jsonb(OLD)-'status'-'applied_at') OR OLD.status='APPLIED' OR NEW.status<>'APPLIED' OR NEW.applied_at IS NULL THEN RAISE EXCEPTION 'immutable source history' USING ERRCODE='42501';END IF;RETURN NEW;END $$;
CREATE TRIGGER catalog_source_history_guard BEFORE UPDATE ON platform.catalog_source_changes FOR EACH ROW EXECUTE FUNCTION platform.catalog_source_history_guard();
REVOKE ALL ON FUNCTION platform.catalog_source_history_guard() FROM PUBLIC;
CREATE INDEX catalog_source_history ON platform.catalog_source_changes(organization_id,product_id,created_at DESC);
DO $$DECLARE t text;BEGIN FOREACH t IN ARRAY ARRAY['catalog_policies','catalog_source_changes'] LOOP
 EXECUTE format('ALTER TABLE platform.%I ENABLE ROW LEVEL SECURITY',t);EXECUTE format('ALTER TABLE platform.%I FORCE ROW LEVEL SECURITY',t);
 EXECUTE format('CREATE POLICY migration_owner ON platform.%I TO aba_migrator USING(true) WITH CHECK(true)',t);
 EXECUTE format('CREATE POLICY tenant_read ON platform.%I FOR SELECT TO aba_app USING(organization_id::text=current_setting(''app.organization_id'',true) AND platform.has_membership(organization_id,true))',t);
 EXECUTE format('CREATE POLICY tenant_insert ON platform.%I FOR INSERT TO aba_app WITH CHECK(organization_id::text=current_setting(''app.organization_id'',true) AND platform.has_membership(organization_id,true))',t);
 EXECUTE format('CREATE POLICY tenant_update ON platform.%I FOR UPDATE TO aba_app USING(organization_id::text=current_setting(''app.organization_id'',true) AND platform.has_membership(organization_id,true)) WITH CHECK(organization_id::text=current_setting(''app.organization_id'',true) AND platform.has_membership(organization_id,true))',t);
 EXECUTE format('GRANT SELECT,INSERT,UPDATE ON platform.%I TO aba_app',t);
 END LOOP;END $$;
DROP POLICY tenant_read ON platform.catalog_policies;
CREATE POLICY tenant_read ON platform.catalog_policies FOR SELECT TO aba_app USING(organization_id::text=current_setting('app.organization_id',true) AND platform.has_membership(organization_id,false));
DROP POLICY tenant_insert ON platform.catalog_source_changes;
CREATE POLICY tenant_insert ON platform.catalog_source_changes FOR INSERT TO aba_app WITH CHECK(organization_id::text=current_setting('app.organization_id',true) AND platform.has_membership(organization_id,true) AND actor_id=platform.current_user_id());
CREATE POLICY channel_read ON platform.catalog_policies FOR SELECT TO aba_app USING(platform.telegram_scope(organization_id,NULL) OR platform.channel_scope(organization_id,NULL));
CREATE FUNCTION platform.inventory_freshness(p_org uuid,p_updated timestamptz) RETURNS text LANGUAGE sql STABLE SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT CASE WHEN p_updated IS NULL OR p_updated>now() THEN 'UNKNOWN'
 WHEN p_updated>=now()-make_interval(hours=>coalesce((SELECT fresh_hours FROM platform.catalog_policies WHERE organization_id=p_org),24)) THEN 'FRESH'
 WHEN p_updated>=now()-make_interval(hours=>coalesce((SELECT stale_hours FROM platform.catalog_policies WHERE organization_id=p_org),72)) THEN 'AGING' ELSE 'STALE' END $$;
REVOKE ALL ON FUNCTION platform.inventory_freshness(uuid,timestamptz) FROM PUBLIC;GRANT EXECUTE ON FUNCTION platform.inventory_freshness(uuid,timestamptz) TO aba_app;
ALTER TABLE platform.product_variants ADD price_updated_at timestamptz;
-- Historic product.updated_at cannot establish price verification.
CREATE FUNCTION platform.catalog_price_time() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,platform,pg_temp AS $$BEGIN
 IF TG_OP='INSERT' OR (NEW.price_amount,NEW.currency_code) IS DISTINCT FROM (OLD.price_amount,OLD.currency_code) THEN NEW.price_updated_at=now();END IF;RETURN NEW;END $$;
CREATE TRIGGER catalog_price_time BEFORE INSERT OR UPDATE ON platform.product_variants FOR EACH ROW EXECUTE FUNCTION platform.catalog_price_time();
ALTER TABLE platform.catalog_imports ADD file_name text NOT NULL DEFAULT 'catalog.xlsx' CHECK(length(file_name)<=120),ADD applied_at timestamptz,ADD retention_hold boolean NOT NULL DEFAULT false,ADD minimized_at timestamptz,ADD summary jsonb NOT NULL DEFAULT '{}';
UPDATE platform.catalog_imports j SET applied_at=(SELECT max(created_at) FROM platform.catalog_events e WHERE e.import_id=j.id AND e.organization_id=j.organization_id AND kind='IMPORT_APPLIED'),summary=preview-'lines' WHERE status='APPLIED';
ALTER TABLE platform.catalog_imports DROP CONSTRAINT catalog_imports_status_check;
ALTER TABLE platform.catalog_imports ADD CHECK(status IN ('UPLOADED','PREVIEW','APPLIED','CANCELLED','EXPIRED','FAILED'));
-- Capability permits cleanup of imports only in integrations bound to this worker proof.
CREATE FUNCTION platform.catalog_cleanup_orgs(p_proof bytea) RETURNS SETOF uuid LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$SELECT organization_id FROM platform.catalog_policies WHERE cleanup_hash=sha256(p_proof) UNION SELECT organization_id FROM platform.telegram_integrations WHERE worker_hash=sha256(p_proof) $$;
REVOKE ALL ON FUNCTION platform.catalog_cleanup_orgs(bytea) FROM PUBLIC;GRANT EXECUTE ON FUNCTION platform.catalog_cleanup_orgs(bytea) TO aba_app;
CREATE FUNCTION platform.catalog_cleanup_scope(p_org uuid) RETURNS boolean LANGUAGE sql STABLE SET search_path=pg_catalog,platform,pg_temp AS $$ SELECT p_org::text=current_setting('app.organization_id',true) AND p_org IN (SELECT platform.catalog_cleanup_orgs(decode(nullif(current_setting('app.catalog_cleanup_proof',true),''),'hex'))) $$;
REVOKE ALL ON FUNCTION platform.catalog_cleanup_scope(uuid) FROM PUBLIC;GRANT EXECUTE ON FUNCTION platform.catalog_cleanup_scope(uuid) TO aba_app;
CREATE POLICY cleanup_read ON platform.catalog_policies FOR SELECT TO aba_app USING(platform.catalog_cleanup_scope(organization_id));
CREATE POLICY cleanup_read ON platform.catalog_imports FOR SELECT TO aba_app USING(platform.catalog_cleanup_scope(organization_id));
CREATE POLICY cleanup_update ON platform.catalog_imports FOR UPDATE TO aba_app USING(platform.catalog_cleanup_scope(organization_id)) WITH CHECK(platform.catalog_cleanup_scope(organization_id));
CREATE POLICY cleanup_event ON platform.catalog_events FOR INSERT TO aba_app WITH CHECK(platform.catalog_cleanup_scope(organization_id) AND actor_id IS NULL AND kind='IMPORT_RETENTION_MINIMIZED');
CREATE OR REPLACE FUNCTION platform.catalog_source_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,platform,pg_temp AS $$
 DECLARE writer text=coalesce(nullif(current_setting('app.catalog_writer',true),''),'MANUAL');changed boolean;BEGIN
 IF current_user='aba_app' AND (NEW.organization_id::text IS DISTINCT FROM current_setting('app.organization_id',true) OR NOT platform.has_membership(NEW.organization_id,false)) THEN RAISE EXCEPTION 'forbidden' USING ERRCODE='42501';END IF;
 IF TG_OP='INSERT' THEN
 IF NEW.catalog_source<>writer THEN RAISE EXCEPTION 'catalog source conflict' USING ERRCODE='P0010';END IF;RETURN NEW;END IF;
 IF TG_TABLE_NAME='products' THEN changed=(NEW.name,NEW.brand,NEW.description,NEW.category_id) IS DISTINCT FROM (OLD.name,OLD.brand,OLD.description,OLD.category_id);
 ELSIF TG_TABLE_NAME='product_variants' THEN changed=(NEW.sku,NEW.variant_name,NEW.attributes,NEW.price_amount,NEW.currency_code) IS DISTINCT FROM (OLD.sku,OLD.variant_name,OLD.attributes,OLD.price_amount,OLD.currency_code);
 ELSE changed=NEW.quantity_on_hand<>OLD.quantity_on_hand AND coalesce(current_setting('app.catalog_stock_move',true),'')<>'true';END IF;
 IF coalesce(current_setting('app.catalog_handoff',true),'')='true' AND platform.has_membership(NEW.organization_id,true) AND NEW.catalog_source IN ('MANUAL','EXCEL') AND (to_jsonb(NEW)-'catalog_source'-'version'-'updated_at')=(to_jsonb(OLD)-'catalog_source'-'version'-'updated_at') THEN RETURN NEW;END IF;
 IF NEW.catalog_source<>OLD.catalog_source OR (changed AND OLD.catalog_source<>writer) THEN RAISE EXCEPTION 'catalog source conflict' USING ERRCODE='P0010';END IF;
 RETURN NEW;END $$;

-- Recheck queued stock facts at dispatch as well as tool/quote time.
CREATE FUNCTION platform.catalog_dispatch_freshness() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE cards jsonb;BEGIN
 IF NEW.status<>'sending' THEN RETURN NEW;END IF;
 SELECT data->'cards' INTO cards FROM platform.messages WHERE organization_id=NEW.organization_id AND id=NEW.id AND role='assistant';
 IF jsonb_typeof(cards)='array' AND EXISTS(SELECT 1 FROM jsonb_array_elements(cards) c LEFT JOIN platform.inventory i ON i.organization_id=NEW.organization_id AND i.variant_id=(c->>'variant_id')::uuid WHERE c->>'fresh'='true' AND (i.variant_id IS NULL OR platform.inventory_freshness(i.organization_id,i.manual_updated_at)<>'FRESH' OR (c->>'manual_updated_at')::timestamptz IS DISTINCT FROM i.manual_updated_at OR (c->>'available')::integer IS DISTINCT FROM i.quantity_on_hand-i.quantity_reserved)) THEN
 NEW.payload=jsonb_build_object('text','Наличие требует подтверждения магазина: данные изменились или устарели после подготовки ответа. Запросите актуальные сведения у сотрудника.');
 INSERT INTO platform.catalog_events(organization_id,resource_id,kind) VALUES(NEW.organization_id,NEW.id,'CATALOG_DISPATCH_RECHECK');
 END IF;RETURN NEW;END $$;
REVOKE ALL ON FUNCTION platform.catalog_dispatch_freshness() FROM PUBLIC;
CREATE TRIGGER catalog_dispatch_freshness BEFORE UPDATE OF status ON platform.telegram_outbox FOR EACH ROW EXECUTE FUNCTION platform.catalog_dispatch_freshness();
CREATE TRIGGER catalog_dispatch_freshness BEFORE UPDATE OF status ON platform.channel_outbox FOR EACH ROW EXECUTE FUNCTION platform.catalog_dispatch_freshness();
UPDATE platform.service_metadata SET version=14 WHERE singleton;
-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DO $$BEGIN IF EXISTS(SELECT 1 FROM platform.catalog_policies) OR EXISTS(SELECT 1 FROM platform.catalog_source_changes) OR EXISTS(SELECT 1 FROM platform.catalog_imports) OR EXISTS(SELECT 1 FROM platform.catalog_events) THEN RAISE EXCEPTION 'rollback refused: preserve catalog control history';END IF;END $$;
DROP POLICY cleanup_read ON platform.catalog_policies;DROP POLICY cleanup_read ON platform.catalog_imports;DROP POLICY cleanup_update ON platform.catalog_imports;DROP POLICY cleanup_event ON platform.catalog_events;
DROP TRIGGER catalog_dispatch_freshness ON platform.telegram_outbox;DROP TRIGGER catalog_dispatch_freshness ON platform.channel_outbox;DROP FUNCTION platform.catalog_dispatch_freshness();
DROP FUNCTION platform.catalog_cleanup_scope(uuid),platform.catalog_cleanup_orgs(bytea),platform.inventory_freshness(uuid,timestamptz);
DROP TRIGGER catalog_price_time ON platform.product_variants;DROP FUNCTION platform.catalog_price_time();ALTER TABLE platform.product_variants DROP COLUMN price_updated_at;
ALTER TABLE platform.catalog_imports DROP COLUMN file_name,DROP COLUMN applied_at,DROP COLUMN retention_hold,DROP COLUMN minimized_at,DROP COLUMN summary;
ALTER TABLE platform.catalog_imports DROP CONSTRAINT catalog_imports_status_check;ALTER TABLE platform.catalog_imports ADD CHECK(status IN ('UPLOADED','PREVIEW','APPLIED','CANCELLED'));
DROP TRIGGER catalog_source_history_guard ON platform.catalog_source_changes;DROP FUNCTION platform.catalog_source_history_guard();
DROP TABLE platform.catalog_source_changes,platform.catalog_policies;
CREATE OR REPLACE FUNCTION platform.catalog_source_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,platform,pg_temp AS $$
 DECLARE writer text=coalesce(nullif(current_setting('app.catalog_writer',true),''),'MANUAL');changed boolean;BEGIN
 IF current_user='aba_app' AND (NEW.organization_id::text IS DISTINCT FROM current_setting('app.organization_id',true) OR NOT platform.has_membership(NEW.organization_id,false)) THEN RAISE EXCEPTION 'forbidden' USING ERRCODE='42501';END IF;
 IF TG_OP='INSERT' THEN
 IF NEW.catalog_source<>writer THEN RAISE EXCEPTION 'catalog source conflict' USING ERRCODE='P0010';END IF;RETURN NEW;END IF;
 IF TG_TABLE_NAME='products' THEN changed=(NEW.name,NEW.brand,NEW.description,NEW.category_id) IS DISTINCT FROM (OLD.name,OLD.brand,OLD.description,OLD.category_id);
 ELSIF TG_TABLE_NAME='product_variants' THEN changed=(NEW.sku,NEW.variant_name,NEW.attributes,NEW.price_amount,NEW.currency_code) IS DISTINCT FROM (OLD.sku,OLD.variant_name,OLD.attributes,OLD.price_amount,OLD.currency_code);
 ELSE changed=NEW.quantity_on_hand<>OLD.quantity_on_hand AND coalesce(current_setting('app.catalog_stock_move',true),'')<>'true';END IF;
 IF NEW.catalog_source<>OLD.catalog_source OR (changed AND OLD.catalog_source<>writer) THEN RAISE EXCEPTION 'catalog source conflict' USING ERRCODE='P0010';END IF;
 RETURN NEW;END $$;

UPDATE platform.service_metadata SET version=13 WHERE singleton;
-- +goose StatementEnd
