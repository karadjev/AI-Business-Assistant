-- +goose Up
-- +goose StatementBegin
CREATE TABLE platform.store_profiles(
 organization_id uuid PRIMARY KEY REFERENCES platform.organizations(id),business_category text NOT NULL DEFAULT '' CHECK(length(business_category)<=100),
 description text NOT NULL DEFAULT '' CHECK(length(description)<=2000),address text NOT NULL DEFAULT '' CHECK(length(address)<=500),
 opening_hours text NOT NULL DEFAULT '' CHECK(length(opening_hours)<=500),contacts text NOT NULL DEFAULT '' CHECK(length(contacts)<=500),
 delivery_terms text NOT NULL DEFAULT '' CHECK(length(delivery_terms)<=2000),payment_methods text NOT NULL DEFAULT '' CHECK(length(payment_methods)<=1000),
 return_terms text NOT NULL DEFAULT '' CHECK(length(return_terms)<=2000),ai_instructions text NOT NULL DEFAULT '' CHECK(length(ai_instructions)<=2000),
 status text NOT NULL DEFAULT 'IN_PROGRESS' CHECK(status IN ('NOT_STARTED','IN_PROGRESS','CATALOG_READY','CHANNELS_PENDING','READY_FOR_REVIEW','ACTIVE')),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),updated_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE platform.catalog_imports(
 id uuid PRIMARY KEY,organization_id uuid NOT NULL REFERENCES platform.organizations(id),request_key uuid NOT NULL,
 file_digest bytea NOT NULL CHECK(octet_length(file_digest)=32),grid jsonb NOT NULL CHECK(jsonb_typeof(grid)='array' AND octet_length(grid::text)<=2097152),
 mapping jsonb NOT NULL DEFAULT '{}',preview jsonb NOT NULL DEFAULT '{}' CHECK(octet_length(preview::text)<=4194304),catalog_fingerprint text,
 status text NOT NULL DEFAULT 'UPLOADED' CHECK(status IN ('UPLOADED','PREVIEW','APPLIED','CANCELLED')),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),created_by uuid NOT NULL REFERENCES platform.users(id),created_at timestamptz NOT NULL DEFAULT now(),expires_at timestamptz NOT NULL DEFAULT now()+interval '1 day',
 UNIQUE(organization_id,id),UNIQUE(organization_id,request_key));
CREATE INDEX catalog_imports_recent ON platform.catalog_imports(organization_id,created_at DESC);
CREATE TABLE platform.catalog_events(
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,organization_id uuid NOT NULL REFERENCES platform.organizations(id),import_id uuid,actor_id uuid REFERENCES platform.users(id),
 kind text NOT NULL CHECK(length(kind)<=80),resource_id uuid,created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,import_id) REFERENCES platform.catalog_imports(organization_id,id));
CREATE INDEX catalog_events_recent ON platform.catalog_events(organization_id,created_at DESC,id);
DO $$DECLARE t text;BEGIN FOREACH t IN ARRAY ARRAY['store_profiles','catalog_imports','catalog_events'] LOOP
 EXECUTE format('ALTER TABLE platform.%I ENABLE ROW LEVEL SECURITY',t);EXECUTE format('ALTER TABLE platform.%I FORCE ROW LEVEL SECURITY',t);
 EXECUTE format('CREATE POLICY migration_owner ON platform.%I TO aba_migrator USING(true) WITH CHECK(true)',t);
 EXECUTE format('CREATE POLICY tenant_read ON platform.%I FOR SELECT TO aba_app USING(organization_id::text=current_setting(''app.organization_id'',true) AND platform.has_membership(organization_id,true))',t);
 EXECUTE format('GRANT SELECT,INSERT ON platform.%I TO aba_app',t);
 EXECUTE format('CREATE POLICY tenant_insert ON platform.%I FOR INSERT TO aba_app WITH CHECK(organization_id::text=current_setting(''app.organization_id'',true) AND platform.has_membership(organization_id,true))',t);
 IF t<>'catalog_events' THEN
 EXECUTE format('GRANT UPDATE ON platform.%I TO aba_app',t);
 EXECUTE format('CREATE POLICY tenant_update ON platform.%I FOR UPDATE TO aba_app USING(organization_id::text=current_setting(''app.organization_id'',true) AND platform.has_membership(organization_id,true)) WITH CHECK(organization_id::text=current_setting(''app.organization_id'',true) AND platform.has_membership(organization_id,true))',t);
 END IF;END LOOP;END $$;
DROP POLICY tenant_insert ON platform.catalog_events;
CREATE POLICY tenant_insert ON platform.catalog_events FOR INSERT TO aba_app WITH CHECK(organization_id::text=current_setting('app.organization_id',true) AND platform.has_membership(organization_id,true) AND actor_id=platform.current_user_id());
DROP POLICY tenant_insert ON platform.catalog_imports;
CREATE POLICY tenant_insert ON platform.catalog_imports FOR INSERT TO aba_app WITH CHECK(organization_id::text=current_setting('app.organization_id',true) AND platform.has_membership(organization_id,true) AND created_by=platform.current_user_id());
GRANT USAGE ON SEQUENCE platform.catalog_events_id_seq TO aba_app;
ALTER TABLE platform.products ADD catalog_source text NOT NULL DEFAULT 'MANUAL' CHECK(catalog_source IN ('MANUAL','EXCEL','ONE_C'));
ALTER TABLE platform.product_variants ADD catalog_source text NOT NULL DEFAULT 'MANUAL' CHECK(catalog_source IN ('MANUAL','EXCEL','ONE_C'));
ALTER TABLE platform.inventory ADD catalog_source text NOT NULL DEFAULT 'MANUAL' CHECK(catalog_source IN ('MANUAL','EXCEL','ONE_C'));
CREATE FUNCTION platform.catalog_source_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,platform,pg_temp AS $$
 DECLARE writer text=coalesce(nullif(current_setting('app.catalog_writer',true),''),'MANUAL');changed boolean;BEGIN
 IF current_user='aba_app' AND (NEW.organization_id::text IS DISTINCT FROM current_setting('app.organization_id',true) OR NOT platform.has_membership(NEW.organization_id,false)) THEN RAISE EXCEPTION 'forbidden' USING ERRCODE='42501';END IF;
 IF TG_OP='INSERT' THEN
 IF NEW.catalog_source<>writer THEN RAISE EXCEPTION 'catalog source conflict' USING ERRCODE='P0010';END IF;RETURN NEW;END IF;
 IF TG_TABLE_NAME='products' THEN changed=(NEW.name,NEW.brand,NEW.description,NEW.category_id) IS DISTINCT FROM (OLD.name,OLD.brand,OLD.description,OLD.category_id);
 ELSIF TG_TABLE_NAME='product_variants' THEN changed=(NEW.sku,NEW.variant_name,NEW.attributes,NEW.price_amount,NEW.currency_code) IS DISTINCT FROM (OLD.sku,OLD.variant_name,OLD.attributes,OLD.price_amount,OLD.currency_code);
 ELSE changed=NEW.quantity_on_hand<>OLD.quantity_on_hand AND coalesce(current_setting('app.catalog_stock_move',true),'')<>'true';END IF;
 IF NEW.catalog_source<>OLD.catalog_source OR (changed AND OLD.catalog_source<>writer) THEN RAISE EXCEPTION 'catalog source conflict' USING ERRCODE='P0010';END IF;
 RETURN NEW;END $$;
CREATE TRIGGER catalog_source_guard BEFORE INSERT OR UPDATE ON platform.products FOR EACH ROW EXECUTE FUNCTION platform.catalog_source_guard();
CREATE TRIGGER catalog_source_guard BEFORE INSERT OR UPDATE ON platform.product_variants FOR EACH ROW EXECUTE FUNCTION platform.catalog_source_guard();
CREATE TRIGGER catalog_source_guard BEFORE INSERT OR UPDATE ON platform.inventory FOR EACH ROW EXECUTE FUNCTION platform.catalog_source_guard();
CREATE FUNCTION platform.catalog_change_audit() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$BEGIN
 IF TG_OP='INSERT' OR to_jsonb(NEW) IS DISTINCT FROM to_jsonb(OLD) THEN
 INSERT INTO platform.catalog_events(organization_id,kind,resource_id,actor_id) VALUES(NEW.organization_id,TG_TABLE_NAME||'_'||TG_OP,coalesce(to_jsonb(NEW)->>'variant_id',to_jsonb(NEW)->>'id')::uuid,platform.current_user_id());END IF;RETURN NEW;END $$;
CREATE TRIGGER catalog_change_audit AFTER INSERT OR UPDATE ON platform.products FOR EACH ROW EXECUTE FUNCTION platform.catalog_change_audit();
CREATE TRIGGER catalog_change_audit AFTER INSERT OR UPDATE ON platform.product_variants FOR EACH ROW EXECUTE FUNCTION platform.catalog_change_audit();
CREATE TRIGGER catalog_change_audit AFTER INSERT OR UPDATE ON platform.inventory FOR EACH ROW EXECUTE FUNCTION platform.catalog_change_audit();
REVOKE ALL ON FUNCTION platform.catalog_source_guard(),platform.catalog_change_audit() FROM PUBLIC;
UPDATE platform.service_metadata SET version=13 WHERE singleton;
-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DO $$BEGIN IF EXISTS(SELECT 1 FROM platform.store_profiles) OR EXISTS(SELECT 1 FROM platform.catalog_imports) OR EXISTS(SELECT 1 FROM platform.catalog_events) OR EXISTS(SELECT 1 FROM platform.products WHERE catalog_source<>'MANUAL') OR EXISTS(SELECT 1 FROM platform.product_variants WHERE catalog_source<>'MANUAL') OR EXISTS(SELECT 1 FROM platform.inventory WHERE catalog_source<>'MANUAL') THEN RAISE EXCEPTION 'rollback refused: preserve onboarding/import/source audit';END IF;END $$;
DROP TRIGGER catalog_source_guard ON platform.products;DROP TRIGGER catalog_source_guard ON platform.product_variants;DROP TRIGGER catalog_source_guard ON platform.inventory;
DROP TRIGGER catalog_change_audit ON platform.products;DROP TRIGGER catalog_change_audit ON platform.product_variants;DROP TRIGGER catalog_change_audit ON platform.inventory;
DROP FUNCTION platform.catalog_source_guard(),platform.catalog_change_audit();
ALTER TABLE platform.products DROP COLUMN catalog_source;ALTER TABLE platform.product_variants DROP COLUMN catalog_source;ALTER TABLE platform.inventory DROP COLUMN catalog_source;
DROP TABLE platform.catalog_events,platform.catalog_imports,platform.store_profiles;
UPDATE platform.service_metadata SET version=12 WHERE singleton;
-- +goose StatementEnd
