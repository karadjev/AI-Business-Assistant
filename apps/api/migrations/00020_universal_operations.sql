-- +goose Up
-- +goose StatementBegin
CREATE TABLE platform.property_listings (
 id uuid PRIMARY KEY,
 organization_id uuid NOT NULL REFERENCES platform.organizations(id),
 title text NOT NULL CHECK(length(btrim(title)) BETWEEN 1 AND 160),
 locality text NOT NULL CHECK(length(btrim(locality)) BETWEEN 1 AND 120),
 rooms integer NOT NULL CHECK(rooms BETWEEN 0 AND 50),
 area_m2 numeric(10,2) NOT NULL CHECK(area_m2 > 0),
 status text NOT NULL CHECK(status IN ('available','unavailable','unknown')),
 verified_until timestamptz NOT NULL,
 UNIQUE (organization_id,id)
);
CREATE INDEX property_listings_filter ON platform.property_listings(organization_id,rooms,area_m2,id);
ALTER TABLE platform.property_listings ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.property_listings FORCE ROW LEVEL SECURITY;
CREATE POLICY migration_owner ON platform.property_listings TO aba_migrator USING(true) WITH CHECK(true);
CREATE POLICY staff_read ON platform.property_listings FOR SELECT TO aba_app
 USING(organization_id::text=current_setting('app.organization_id',true) AND platform.can_operate_orders(organization_id) AND platform.is_test_org(organization_id));
GRANT SELECT ON platform.property_listings TO aba_app;

ALTER TABLE platform.orders
 ALTER COLUMN quote_id DROP NOT NULL,
 ALTER COLUMN total_amount DROP NOT NULL,
 ADD COLUMN request_kind text NOT NULL DEFAULT 'product_order',
 ADD COLUMN request_details jsonb,
 ADD COLUMN listing_id uuid,
 ADD CONSTRAINT orders_listing_tenant_fk FOREIGN KEY(organization_id,listing_id) REFERENCES platform.property_listings(organization_id,id),
 ADD CONSTRAINT orders_conversation_tenant_fk FOREIGN KEY(organization_id,conversation_id,customer_id) REFERENCES platform.conversations(organization_id,id,customer_id),
 ADD CONSTRAINT orders_request_shape CHECK (
 (request_kind='product_order' AND quote_id IS NOT NULL AND total_amount IS NOT NULL AND request_details IS NULL AND listing_id IS NULL)
 OR (request_kind IN ('service_booking','repair_request','quotation_request','property_viewing_request','custom_order','hybrid_order')
 AND quote_id IS NULL AND total_amount IS NULL AND source='internal_chat' AND request_details IS NOT NULL
 AND jsonb_typeof(request_details)='object' AND octet_length(request_details::text)<=4096
 AND (listing_id IS NOT NULL)=(request_kind='property_viewing_request')));
CREATE POLICY universal_order_insert ON platform.orders AS RESTRICTIVE FOR INSERT TO aba_app WITH CHECK (
 request_kind='product_order' OR (organization_id::text=current_setting('app.organization_id',true)
 AND platform.can_operate_orders(organization_id) AND platform.is_test_org(organization_id)
 AND status='pending_confirmation' AND confirmed_by IS NULL));
CREATE POLICY universal_order_update ON platform.orders AS RESTRICTIVE FOR UPDATE TO aba_app USING (
 request_kind='product_order' OR (organization_id::text=current_setting('app.organization_id',true)
 AND platform.can_operate_orders(organization_id) AND platform.is_test_org(organization_id))) WITH CHECK (
 request_kind='product_order' OR (organization_id::text=current_setting('app.organization_id',true)
 AND platform.can_operate_orders(organization_id) AND platform.is_test_org(organization_id)));
CREATE FUNCTION platform.order_request_immutable() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog,platform,pg_temp AS $$
BEGIN
 IF (OLD.request_kind,OLD.request_details,OLD.listing_id) IS DISTINCT FROM
    (NEW.request_kind,NEW.request_details,NEW.listing_id)
 OR (OLD.request_kind<>'product_order' AND
    (OLD.organization_id,OLD.conversation_id,OLD.customer_id,OLD.idempotency_key) IS DISTINCT FROM
    (NEW.organization_id,NEW.conversation_id,NEW.customer_id,NEW.idempotency_key)) THEN
  RAISE EXCEPTION 'request parameters are immutable' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
REVOKE ALL ON FUNCTION platform.order_request_immutable() FROM PUBLIC;
CREATE TRIGGER order_request_immutable BEFORE UPDATE ON platform.orders
 FOR EACH ROW EXECUTE FUNCTION platform.order_request_immutable();
-- Share the organization lock with profile changes, including for order staff
-- who cannot update the organization's configuration.
CREATE FUNCTION platform.order_request_business_model(p_org uuid) RETURNS text
 LANGUAGE sql VOLATILE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT o.business_model FROM platform.organizations o
 WHERE o.id=p_org AND o.is_test
 AND p_org::text=current_setting('app.organization_id',true)
 AND platform.can_operate_orders(p_org)
 FOR SHARE
$$;
REVOKE ALL ON FUNCTION platform.order_request_business_model(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.order_request_business_model(uuid) TO aba_app;
-- Existing catalog attributes remain string-valued. Read only keys with explicit
-- units/types, and index those projections without rewriting product records.
CREATE FUNCTION platform.industry_catalog_text(p_attributes jsonb,p_key text)
RETURNS text LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE SET search_path=pg_catalog,pg_temp AS $$
 SELECT CASE WHEN jsonb_typeof(p_attributes->p_key)='string'
             AND length(btrim(p_attributes->>p_key)) BETWEEN 1 AND 100
             THEN lower(btrim(p_attributes->>p_key)) END
$$;
CREATE FUNCTION platform.industry_catalog_number(p_attributes jsonb,p_key text,p_integer boolean)
RETURNS numeric LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE SET search_path=pg_catalog,pg_temp AS $$
 SELECT CASE WHEN jsonb_typeof(p_attributes->p_key)='string' AND
  CASE WHEN p_integer THEN (p_attributes->>p_key) ~ '^(0|[1-9][0-9]{0,17})$'
  ELSE (p_attributes->>p_key) ~ '^(0|[1-9][0-9]{0,17})([.][0-9]{1,9})?$' END
  THEN (p_attributes->>p_key)::numeric END
$$;
REVOKE ALL ON FUNCTION platform.industry_catalog_text(jsonb,text),platform.industry_catalog_number(jsonb,text,boolean) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.industry_catalog_text(jsonb,text),platform.industry_catalog_number(jsonb,text,boolean) TO aba_app;

CREATE INDEX industry_active_name ON platform.products(organization_id,(lower(name) COLLATE "C"),id) WHERE status='active';
CREATE INDEX industry_active_brand ON platform.products(organization_id,(lower(brand) COLLATE "C"),id) WHERE status='active';
CREATE INDEX industry_variant_sku ON platform.product_variants(organization_id,(lower(sku) COLLATE "C"),id);
CREATE INDEX industry_variant_name ON platform.product_variants(organization_id,(lower(variant_name) COLLATE "C"),id);
CREATE INDEX industry_variant_price ON platform.product_variants(organization_id,price_amount,id);

DO $$
DECLARE attr text;
BEGIN
 FOREACH attr IN ARRAY ARRAY['oem','manufacturer','model','color','material','size','size_system','selling_unit'] LOOP
  EXECUTE format('CREATE INDEX %I ON platform.product_variants(organization_id,(platform.industry_catalog_text(attributes,%L) COLLATE "C"),id) WHERE platform.industry_catalog_text(attributes,%L) IS NOT NULL',
   'industry_text_'||attr,attr,attr);
 END LOOP;
 FOREACH attr IN ARRAY ARRAY['memory_gb','warranty_months','weight_kg','volume_l','length_mm','width_mm','height_mm','package_quantity'] LOOP
  EXECUTE format('CREATE INDEX %I ON platform.product_variants(organization_id,platform.industry_catalog_number(attributes,%L,%L),id) WHERE platform.industry_catalog_number(attributes,%L,%L) IS NOT NULL',
   'industry_number_'||attr,attr,attr IN ('memory_gb','warranty_months','package_quantity'),
   attr,attr IN ('memory_gb','warranty_months','package_quantity'));
 END LOOP;
END $$;

-- Narrow profile read for existing server-bound catalog principals. Profiles
-- do not grant access to products or expand an agent lease beyond its variant.
CREATE FUNCTION platform.industry_catalog_profile(p_org uuid)
RETURNS text LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT o.industry_profile FROM platform.organizations o
 WHERE o.id=p_org AND p_org::text=current_setting('app.organization_id',true)
 AND o.business_model IN ('trade','hybrid')
 AND CASE
  WHEN nullif(current_setting('app.agent_lease',true),'') IS NOT NULL THEN platform.agent_catalog_scope(p_org)
  WHEN nullif(current_setting('app.channel_cap',true),'') IS NOT NULL THEN platform.channel_scope(p_org,NULL)
  WHEN nullif(current_setting('app.telegram_cap',true),'') IS NOT NULL THEN platform.telegram_scope(p_org,NULL)
  ELSE platform.has_membership(p_org,false)
 END
$$;
REVOKE ALL ON FUNCTION platform.industry_catalog_profile(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.industry_catalog_profile(uuid) TO aba_app;

-- Protected verification evidence is an additive sidecar on existing variants.
-- Only a trusted migration/operator path can attest, expire, or revoke it.
CREATE TABLE platform.catalog_fitment_evidence (
 id uuid PRIMARY KEY,
 organization_id uuid NOT NULL,
 variant_id uuid NOT NULL,
 make text NOT NULL CHECK(length(make) BETWEEN 1 AND 80 AND make=lower(btrim(make)) AND make !~ '[[:cntrl:]]'),
 model text NOT NULL CHECK(length(model) BETWEEN 1 AND 80 AND model=lower(btrim(model)) AND model !~ '[[:cntrl:]]'),
 engine text NOT NULL CHECK(length(engine) BETWEEN 1 AND 80 AND engine=lower(btrim(engine)) AND engine !~ '[[:cntrl:]]'),
 year_from smallint NOT NULL CHECK(year_from BETWEEN 1886 AND 2100),
 year_to smallint NOT NULL CHECK(year_to BETWEEN year_from AND 2100),
 compatible boolean NOT NULL,
 source_ref text NOT NULL CHECK(source_ref ~ '^[a-zA-Z0-9][a-zA-Z0-9._:/-]{0,119}$'),
 verified_at timestamptz NOT NULL CHECK(isfinite(verified_at)),
 valid_until timestamptz NOT NULL CHECK(isfinite(valid_until) AND valid_until>verified_at),
 revoked_at timestamptz CHECK(isfinite(revoked_at) AND revoked_at>=verified_at),
 FOREIGN KEY(organization_id,variant_id) REFERENCES platform.product_variants(organization_id,id) ON DELETE RESTRICT
);
CREATE INDEX industry_fitment_lookup ON platform.catalog_fitment_evidence
 (organization_id,variant_id,make,model,engine,compatible,verified_at DESC,id)
 INCLUDE(year_from,year_to,valid_until,source_ref) WHERE revoked_at IS NULL;
ALTER TABLE platform.catalog_fitment_evidence ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.catalog_fitment_evidence FORCE ROW LEVEL SECURITY;
CREATE POLICY migration_owner ON platform.catalog_fitment_evidence TO aba_migrator USING(true) WITH CHECK(true);
CREATE POLICY published_variant_read ON platform.catalog_fitment_evidence FOR SELECT TO aba_app
 USING(organization_id::text=current_setting('app.organization_id',true)
 AND (SELECT platform.industry_catalog_profile(nullif(current_setting('app.organization_id',true),'')::uuid))='auto_parts'
 AND revoked_at IS NULL AND verified_at<=statement_timestamp() AND valid_until>statement_timestamp()
 AND EXISTS(
  SELECT 1 FROM platform.product_variants v
  JOIN platform.products p ON p.organization_id=v.organization_id AND p.id=v.product_id
  WHERE v.organization_id=catalog_fitment_evidence.organization_id AND v.id=catalog_fitment_evidence.variant_id AND p.status='active'
 ));
GRANT SELECT ON platform.catalog_fitment_evidence TO aba_app;
-- Booking fragment: assemble AFTER the universal orders alterations.
-- No extensions. Absolute windows are explicit bounded schedule evidence, not
-- implicit weekly recurrence. Missing any layer means availability unknown.
-- Fail with an authorization SQLSTATE before any BEFORE-trigger domain lookup.
-- SECURITY DEFINER changes current_user, so only session_user identifies the
-- database login. aba_migrator and actual cluster administrators retain their maintenance path;
-- aba_app must still prove a live session and actual tenant membership.
CREATE FUNCTION platform.service_write_authorize(p_org uuid,p_owner boolean) RETURNS void
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
BEGIN
 IF session_user='aba_migrator' OR EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname=session_user AND rolsuper) THEN RETURN; END IF;
 IF session_user<>'aba_app'
 OR p_org IS NULL
 OR p_org::text IS DISTINCT FROM current_setting('app.organization_id',true)
 OR platform.current_user_id() IS NULL
 OR NOT coalesce(platform.is_test_org(p_org),false)
 OR (p_owner AND NOT coalesce(platform.has_membership(p_org,true),false))
 OR (NOT p_owner AND NOT coalesce(platform.can_operate_orders(p_org),false)) THEN
  RAISE EXCEPTION 'service write denied' USING ERRCODE='42501';
 END IF;
END $$;
REVOKE ALL ON FUNCTION platform.service_write_authorize(uuid,boolean) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.service_write_authorize(uuid,boolean) TO aba_app;
-- Lock profile through a narrow authenticated function, without granting staff
-- organization UPDATE authority. The lock prevents a profile switch mid-booking.
CREATE FUNCTION platform.service_booking_scope(p_org uuid) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE model text;
BEGIN
 IF session_user<>'aba_app' THEN RETURN false; END IF;
 IF p_org::text IS DISTINCT FROM current_setting('app.organization_id',true) OR NOT platform.is_test_org(p_org) OR NOT platform.can_operate_orders(p_org) THEN RETURN false; END IF;
 SELECT business_model INTO model FROM platform.organizations WHERE id=p_org FOR SHARE;
 RETURN coalesce(model IN ('service','hybrid'),false);
END $$;
REVOKE ALL ON FUNCTION platform.service_booking_scope(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.service_booking_scope(uuid) TO aba_app;
CREATE TABLE platform.service_offerings (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 organization_id uuid NOT NULL REFERENCES platform.organizations(id),
 name text NOT NULL CHECK(length(btrim(name)) BETWEEN 1 AND 256),
 status text NOT NULL CHECK(status IN ('draft','active','archived')),
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 price_model text NOT NULL CHECK(price_model IN ('fixed','hourly','on_request','custom_estimate')),
 price_state text NOT NULL CHECK(price_state IN ('known','unknown','estimated')),
 amount_minor bigint CHECK(amount_minor>=0),
 currency text NOT NULL DEFAULT 'RUB' CHECK(currency='RUB'),
 min_minutes integer NOT NULL CHECK(min_minutes BETWEEN 0 AND 10080),
 max_minutes integer NOT NULL CHECK(max_minutes BETWEEN min_minutes AND 10080),
 buffer_before integer NOT NULL DEFAULT 0 CHECK(buffer_before BETWEEN 0 AND 1440),
 buffer_after integer NOT NULL DEFAULT 0 CHECK(buffer_after BETWEEN 0 AND 1440),
 timezone text NOT NULL CHECK(length(timezone) BETWEEN 1 AND 128 AND timezone<>'Local'),
 CHECK((min_minutes=0)=(max_minutes=0)),
 CHECK((price_state='unknown' AND amount_minor IS NULL) OR (price_state<>'unknown' AND amount_minor IS NOT NULL)),
 CHECK((price_model IN ('fixed','hourly') AND price_state='known') OR (price_model='on_request' AND price_state='unknown') OR (price_model='custom_estimate' AND price_state IN ('unknown','estimated'))),
 UNIQUE(organization_id,id)
);
CREATE TABLE platform.service_resources (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 organization_id uuid NOT NULL REFERENCES platform.organizations(id),
 name text NOT NULL CHECK(length(btrim(name)) BETWEEN 1 AND 128),
 kind text NOT NULL CHECK(kind IN ('staff','equipment','room','capacity')),
 capacity integer NOT NULL CHECK(capacity BETWEEN 1 AND 10000),
 active boolean NOT NULL DEFAULT true,
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 UNIQUE(organization_id,id)
);
-- A staff resource is a scheduling resource, not a new login identity.
CREATE TABLE platform.service_offering_resources (
 organization_id uuid NOT NULL,
 offering_id uuid NOT NULL,
 resource_id uuid NOT NULL,
 PRIMARY KEY(organization_id,offering_id,resource_id),
 FOREIGN KEY(organization_id,offering_id) REFERENCES platform.service_offerings(organization_id,id) ON DELETE CASCADE,
 FOREIGN KEY(organization_id,resource_id) REFERENCES platform.service_resources(organization_id,id)
);
CREATE TABLE platform.service_availability_windows (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 organization_id uuid NOT NULL,
 offering_id uuid NOT NULL,
 resource_id uuid NOT NULL,
 layer text NOT NULL CHECK(layer IN ('business','offering','shift','exception')),
 starts_at timestamptz NOT NULL,
 ends_at timestamptz NOT NULL,
 timezone text NOT NULL CHECK(length(timezone) BETWEEN 1 AND 128 AND timezone<>'Local'),
 CHECK(ends_at>starts_at AND ends_at-starts_at<=interval '31 days'),
 FOREIGN KEY(organization_id,offering_id,resource_id) REFERENCES platform.service_offering_resources(organization_id,offering_id,resource_id) ON DELETE CASCADE
);
CREATE INDEX service_window_lookup ON platform.service_availability_windows(organization_id,offering_id,resource_id,starts_at);
CREATE TABLE platform.service_bookings (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 organization_id uuid NOT NULL,
 request_id uuid NOT NULL,
 offering_id uuid NOT NULL,
 offering_revision bigint NOT NULL CHECK(offering_revision>0),
 starts_at timestamptz NOT NULL,
 ends_at timestamptz NOT NULL,
 timezone text NOT NULL,
 units integer NOT NULL CHECK(units BETWEEN 1 AND 10000),
 selection_hash bytea NOT NULL CHECK(octet_length(selection_hash)=32),
 status text NOT NULL DEFAULT 'confirmed' CHECK(status IN ('confirmed','cancelled')),
 UNIQUE(organization_id,id), UNIQUE(organization_id,request_id),
 CHECK(ends_at>starts_at AND ends_at-starts_at<=interval '7 days'),
 FOREIGN KEY(organization_id,request_id) REFERENCES platform.orders(organization_id,id) ON DELETE CASCADE,
 FOREIGN KEY(organization_id,offering_id) REFERENCES platform.service_offerings(organization_id,id)
);
CREATE TABLE platform.service_booking_allocations (
 organization_id uuid NOT NULL,
 booking_id uuid NOT NULL,
 resource_id uuid NOT NULL,
 starts_at timestamptz NOT NULL,
 ends_at timestamptz NOT NULL,
 units integer NOT NULL CHECK(units BETWEEN 1 AND 10000),
 active boolean NOT NULL DEFAULT true,
 PRIMARY KEY(organization_id,booking_id,resource_id),
 CHECK(ends_at>starts_at AND ends_at-starts_at<=interval '9 days'),
 FOREIGN KEY(organization_id,booking_id) REFERENCES platform.service_bookings(organization_id,id) ON DELETE CASCADE,
 FOREIGN KEY(organization_id,resource_id) REFERENCES platform.service_resources(organization_id,id)
);
CREATE INDEX service_allocation_overlap ON platform.service_booking_allocations(organization_id,resource_id,starts_at,ends_at) WHERE active;
DO $$ DECLARE tab text; BEGIN
 FOREACH tab IN ARRAY ARRAY['service_offerings','service_resources','service_offering_resources','service_availability_windows','service_bookings','service_booking_allocations'] LOOP
  EXECUTE format('ALTER TABLE platform.%I ENABLE ROW LEVEL SECURITY',tab);
  EXECUTE format('ALTER TABLE platform.%I FORCE ROW LEVEL SECURITY',tab);
  EXECUTE format('CREATE POLICY migration_owner ON platform.%I TO aba_migrator USING(true) WITH CHECK(true)',tab);
  EXECUTE format('CREATE POLICY staff_read ON platform.%I FOR SELECT TO aba_app USING(organization_id::text=current_setting(''app.organization_id'',true) AND platform.is_test_org(organization_id) AND platform.can_operate_orders(organization_id))',tab);
  IF tab IN ('service_bookings','service_booking_allocations') THEN
   EXECUTE format('CREATE POLICY staff_insert ON platform.%I FOR INSERT TO aba_app WITH CHECK(organization_id::text=current_setting(''app.organization_id'',true) AND platform.is_test_org(organization_id) AND platform.can_operate_orders(organization_id))',tab);
   EXECUTE format('CREATE POLICY staff_update ON platform.%I FOR UPDATE TO aba_app USING(organization_id::text=current_setting(''app.organization_id'',true) AND platform.is_test_org(organization_id) AND platform.can_operate_orders(organization_id)) WITH CHECK(organization_id::text=current_setting(''app.organization_id'',true) AND platform.is_test_org(organization_id) AND platform.can_operate_orders(organization_id))',tab);
  ELSE
   EXECUTE format('CREATE POLICY owner_write ON platform.%I FOR ALL TO aba_app USING(organization_id::text=current_setting(''app.organization_id'',true) AND platform.is_test_org(organization_id) AND platform.has_membership(organization_id,true)) WITH CHECK(organization_id::text=current_setting(''app.organization_id'',true) AND platform.is_test_org(organization_id) AND platform.has_membership(organization_id,true))',tab);
  END IF;
 END LOOP;
END $$;
-- Row locking requires UPDATE USING visibility, but WITH CHECK false keeps
-- delegated order staff from actually changing offering/resource configuration.
CREATE POLICY staff_lock ON platform.service_offerings FOR UPDATE TO aba_app USING(organization_id::text=current_setting('app.organization_id',true) AND platform.is_test_org(organization_id) AND platform.can_operate_orders(organization_id)) WITH CHECK(false);
CREATE POLICY staff_lock ON platform.service_resources FOR UPDATE TO aba_app USING(organization_id::text=current_setting('app.organization_id',true) AND platform.is_test_org(organization_id) AND platform.can_operate_orders(organization_id)) WITH CHECK(false);
GRANT SELECT,INSERT,UPDATE ON platform.service_bookings,platform.service_booking_allocations TO aba_app;
GRANT SELECT,INSERT,UPDATE,DELETE ON platform.service_offerings,platform.service_resources,platform.service_offering_resources,platform.service_availability_windows TO aba_app;

-- Alphabetically first on each relation, ahead of domain guards. An owner
-- cannot move a visible row between tenants, even if both tenants are owned.
CREATE FUNCTION platform.service_tenant_write_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE org uuid; owner_only boolean;
BEGIN
 IF session_user='aba_migrator' OR EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname=session_user AND rolsuper) THEN
  IF TG_OP='DELETE' THEN RETURN OLD; END IF;
  RETURN NEW;
 END IF;
 IF TG_OP='UPDATE' AND NEW.organization_id IS DISTINCT FROM OLD.organization_id THEN
  RAISE EXCEPTION 'service tenant substitution denied' USING ERRCODE='42501';
 END IF;
 IF TG_OP='DELETE' THEN org:=OLD.organization_id; ELSE org:=NEW.organization_id; END IF;
 owner_only:=TG_TABLE_NAME NOT IN ('service_bookings','service_booking_allocations');
 -- The admitted allocation lock performs an exact no-op UPDATE through a
 -- definer. Staff may take that lock but cannot change configuration fields.
 IF TG_TABLE_NAME='service_resources' AND TG_OP='UPDATE' AND NEW IS NOT DISTINCT FROM OLD THEN owner_only:=false; END IF;
 PERFORM platform.service_write_authorize(org,owner_only);
 IF TG_OP='DELETE' THEN RETURN OLD; END IF;
 RETURN NEW;
END $$;
REVOKE ALL ON FUNCTION platform.service_tenant_write_guard() FROM PUBLIC;
DO $$ DECLARE tab text; BEGIN
 FOREACH tab IN ARRAY ARRAY['service_offerings','service_resources','service_offering_resources','service_availability_windows','service_bookings','service_booking_allocations'] LOOP
  EXECUTE format('CREATE TRIGGER aaa_service_tenant_write_guard BEFORE INSERT OR UPDATE OR DELETE ON platform.%I FOR EACH ROW EXECUTE FUNCTION platform.service_tenant_write_guard()',tab);
 END LOOP;
END $$;

-- A no-op MVCC write makes REPEATABLE READ writers fail serialization instead
-- of admitting allocations from a stale snapshot after waiting on a row lock.
CREATE FUNCTION platform.service_lock_resource(p_org uuid,p_resource uuid) RETURNS integer
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE cap integer;
BEGIN
 PERFORM platform.service_write_authorize(p_org,false);
 IF session_user='aba_app' AND NOT platform.service_booking_scope(p_org) THEN RAISE EXCEPTION 'resource lock denied' USING ERRCODE='42501'; END IF;
 UPDATE platform.service_resources SET revision=revision WHERE organization_id=p_org AND id=p_resource AND active RETURNING capacity INTO STRICT cap;
 RETURN cap;
END $$;
REVOKE ALL ON FUNCTION platform.service_lock_resource(uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.service_lock_resource(uuid,uuid) TO aba_app;
-- Every allocation writer serializes against the same tenant/resource row, even
-- callers bypassing Go. Event-point sweep supports capacity N without summing
-- disjoint appointments as if they overlapped. Half-open intervals permit adjacency.
CREATE FUNCTION platform.service_allocation_guard() RETURNS trigger LANGUAGE plpgsql
SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE cap integer; b platform.service_bookings; o platform.service_offerings; occupied integer;
BEGIN
 IF TG_OP='UPDATE' THEN
  IF (NEW.organization_id,NEW.booking_id,NEW.resource_id,NEW.starts_at,NEW.ends_at,NEW.units) IS DISTINCT FROM (OLD.organization_id,OLD.booking_id,OLD.resource_id,OLD.starts_at,OLD.ends_at,OLD.units) OR NOT OLD.active THEN
   RAISE EXCEPTION 'immutable service allocation' USING ERRCODE='23514';
  END IF;
 END IF;
 cap:=platform.service_lock_resource(NEW.organization_id,NEW.resource_id);
 -- Read binding after acquiring the serialization lock, not before a possible wait.
 SELECT * INTO STRICT b FROM platform.service_bookings WHERE organization_id=NEW.organization_id AND id=NEW.booking_id;
 SELECT * INTO STRICT o FROM platform.service_offerings WHERE organization_id=b.organization_id AND id=b.offering_id;
 IF NOT NEW.active THEN
  IF b.status<>'cancelled' THEN RAISE EXCEPTION 'booking not cancelled' USING ERRCODE='23514'; END IF;
  RETURN NEW;
 END IF;
 IF b.status<>'confirmed' OR NEW.units<>b.units OR NEW.starts_at<>b.starts_at-make_interval(mins=>o.buffer_before) OR NEW.ends_at<>b.ends_at+make_interval(mins=>o.buffer_after)
 OR NOT EXISTS(SELECT 1 FROM platform.service_offering_resources WHERE organization_id=NEW.organization_id AND offering_id=b.offering_id AND resource_id=NEW.resource_id) THEN
  RAISE EXCEPTION 'allocation binding mismatch' USING ERRCODE='23514';
 END IF;
 IF EXISTS(SELECT 1 FROM unnest(ARRAY['business','offering','shift']) AS required(layer) WHERE NOT EXISTS(
 SELECT 1 FROM platform.service_availability_windows w WHERE w.organization_id=NEW.organization_id AND w.offering_id=b.offering_id AND w.resource_id=NEW.resource_id AND w.layer=required.layer AND w.timezone=o.timezone AND w.starts_at<=NEW.starts_at AND w.ends_at>=NEW.ends_at))
 OR EXISTS(SELECT 1 FROM platform.service_availability_windows w WHERE w.organization_id=NEW.organization_id AND w.offering_id=b.offering_id AND w.resource_id=NEW.resource_id AND w.layer='exception' AND w.starts_at<NEW.ends_at AND w.ends_at>NEW.starts_at) THEN
  RAISE EXCEPTION 'service schedule unavailable' USING ERRCODE='23P01';
 END IF;
 WITH points AS (
  SELECT NEW.starts_at AS at
  UNION SELECT a.starts_at FROM platform.service_booking_allocations a WHERE a.organization_id=NEW.organization_id AND a.resource_id=NEW.resource_id AND a.active AND a.booking_id<>NEW.booking_id AND a.starts_at>=NEW.starts_at AND a.starts_at<NEW.ends_at
 ) SELECT coalesce(max(NEW.units+(SELECT coalesce(sum(a.units),0) FROM platform.service_booking_allocations a WHERE a.organization_id=NEW.organization_id AND a.resource_id=NEW.resource_id AND a.active AND a.booking_id<>NEW.booking_id AND a.starts_at<=p.at AND a.ends_at>p.at)),NEW.units) INTO occupied FROM points p;
 IF occupied>cap THEN RAISE EXCEPTION 'service capacity unavailable' USING ERRCODE='23P01'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER service_allocation_guard BEFORE INSERT OR UPDATE ON platform.service_booking_allocations FOR EACH ROW EXECUTE FUNCTION platform.service_allocation_guard();

CREATE FUNCTION platform.service_booking_guard() RETURNS trigger LANGUAGE plpgsql
SET search_path=pg_catalog,platform,pg_temp AS $$
BEGIN
 IF TG_OP='UPDATE' THEN
  IF (to_jsonb(NEW)-'status') IS DISTINCT FROM (to_jsonb(OLD)-'status') OR OLD.status<>'confirmed' OR NEW.status<>'cancelled' THEN
   RAISE EXCEPTION 'immutable booking' USING ERRCODE='23514';
  END IF;
 ELSE
  PERFORM 1 FROM platform.service_offerings o WHERE o.organization_id=NEW.organization_id AND o.id=NEW.offering_id AND o.status='active' AND o.revision=NEW.offering_revision AND o.timezone=NEW.timezone AND o.min_minutes>0 AND NEW.ends_at-NEW.starts_at BETWEEN make_interval(mins=>o.min_minutes) AND make_interval(mins=>o.max_minutes) FOR SHARE;
  IF NOT FOUND OR NEW.starts_at<=clock_timestamp() THEN RAISE EXCEPTION 'service source changed' USING ERRCODE='23514'; END IF;
  IF NEW.status<>'confirmed' OR NOT EXISTS(SELECT 1 FROM platform.orders r WHERE r.organization_id=NEW.organization_id AND r.id=NEW.request_id AND r.status='confirmed' AND r.request_kind='service_booking') THEN
   RAISE EXCEPTION 'confirmed service request required' USING ERRCODE='23514';
  END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER service_booking_guard BEFORE INSERT OR UPDATE ON platform.service_bookings FOR EACH ROW EXECUTE FUNCTION platform.service_booking_guard();
-- Config mutations use the same row locks as admission. Resource shape cannot
-- change while active allocations exist; cancelling releases this restriction.
CREATE FUNCTION platform.service_resource_guard() RETURNS trigger LANGUAGE plpgsql
SET search_path=pg_catalog,platform,pg_temp AS $$
BEGIN
 IF NEW IS NOT DISTINCT FROM OLD THEN RETURN NEW; END IF;
 IF NEW.organization_id<>OLD.organization_id OR NEW.id<>OLD.id OR EXISTS(SELECT 1 FROM platform.service_booking_allocations WHERE organization_id=OLD.organization_id AND resource_id=OLD.id AND active) THEN
  RAISE EXCEPTION 'resource has active allocations' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER service_resource_guard BEFORE UPDATE ON platform.service_resources FOR EACH ROW EXECUTE FUNCTION platform.service_resource_guard();
CREATE FUNCTION platform.service_window_guard() RETURNS trigger LANGUAGE plpgsql
SET search_path=pg_catalog,platform,pg_temp AS $$
BEGIN
 IF TG_OP='UPDATE' THEN RAISE EXCEPTION 'replace schedule window explicitly' USING ERRCODE='23514'; END IF;
 IF TG_OP='DELETE' THEN
  UPDATE platform.service_offerings SET revision=revision+1 WHERE organization_id=OLD.organization_id AND id=OLD.offering_id;
  RETURN OLD;
 END IF;
 UPDATE platform.service_offerings SET revision=revision+1 WHERE organization_id=NEW.organization_id AND id=NEW.offering_id;
 RETURN NEW;
END $$;
CREATE TRIGGER service_window_guard BEFORE INSERT OR UPDATE OR DELETE ON platform.service_availability_windows FOR EACH ROW EXECUTE FUNCTION platform.service_window_guard();
REVOKE ALL ON FUNCTION platform.service_allocation_guard(),platform.service_booking_guard(),platform.service_resource_guard(),platform.service_window_guard() FROM PUBLIC;

CREATE TRIGGER service_assignment_guard BEFORE INSERT OR UPDATE OR DELETE ON platform.service_offering_resources FOR EACH ROW EXECUTE FUNCTION platform.service_window_guard();
ALTER TABLE platform.agent_tasks DROP CONSTRAINT agent_task_operation;
ALTER TABLE platform.agent_tasks ADD CONSTRAINT agent_task_operation CHECK (
 (agent_id IN ('customer','sales','product_inventory') AND action IN ('get_product_price','check_inventory','get_product_details') AND variant_id IS NOT NULL AND conversation_id IS NOT NULL)
 OR (variant_id IS NULL AND conversation_id IS NULL AND ((agent_id='marketing' AND action='draft') OR
 (agent_id IN ('finance','analytics') AND action='read') OR (agent_id='business_manager' AND action IN ('plan','coordinate','coordinate_growth'))))
 OR (variant_id IS NULL AND conversation_id IS NULL AND parent_id IS NOT NULL AND
 ((agent_id='sales' AND action='review_sales') OR (agent_id='product_inventory' AND action='review_inventory'))));
CREATE FUNCTION platform.agent_growth_children() RETURNS trigger
 LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE child uuid; approval uuid; specialist text; operation text;
BEGIN
 IF NOT platform.agent_owner(NEW.organization_id)
 OR NOT EXISTS(SELECT 1 FROM platform.organizations o WHERE o.id=NEW.organization_id AND o.business_model IN ('trade','hybrid')) THEN
  RAISE EXCEPTION 'growth review unavailable' USING ERRCODE='42501';
 END IF;
 IF (SELECT count(*) FROM platform.agent_tasks WHERE organization_id=NEW.organization_id AND created_at>clock_timestamp()-interval '1 minute')>16 THEN
  RAISE EXCEPTION 'agent task rate limit' USING ERRCODE='54000';
 END IF;
 FOR specialist,operation IN SELECT * FROM (VALUES ('analytics','read'),('sales','review_sales'),
 ('product_inventory','review_inventory'),('marketing','draft')) AS work(agent_name,operation_name)
 LOOP
  child:=gen_random_uuid();
  INSERT INTO platform.agent_tasks(id,organization_id,initiator,agent_id,parent_id,correlation_id,action,idempotency_key,worker_hash,status,deadline)
  VALUES(child,NEW.organization_id,NEW.initiator,specialist,NEW.id,NEW.correlation_id,operation,child,NEW.worker_hash,
   CASE WHEN specialist='marketing' THEN 'waiting_approval' ELSE 'queued' END,NEW.deadline);
  approval:=NULL;
  IF specialist='marketing' THEN
   approval:=gen_random_uuid();
   INSERT INTO platform.agent_approvals(id,organization_id,task_id,parameters,parameters_hash,expires_at,status)
   VALUES(approval,NEW.organization_id,child,'{"action":"draft"}',encode(sha256(convert_to('{"action":"draft"}','UTF8')),'hex'),NEW.deadline,'pending');
  ELSE
   INSERT INTO platform.channel_inbox(id,organization_id,external_id,external_user_id,payload,channel,agent_task_id)
   VALUES(child,NEW.organization_id,child::text,'','{}','agent',child);
  END IF;
  INSERT INTO platform.agent_audit(organization_id,task_id,approval_id,actor_id,agent_id,action,result)
  VALUES(NEW.organization_id,child,approval,NEW.initiator,specialist,'task.delegate',CASE WHEN approval IS NULL THEN 'queued' ELSE 'pending' END);
 END LOOP;
 RETURN NEW;
END $$;
REVOKE ALL ON FUNCTION platform.agent_growth_children() FROM PUBLIC;
CREATE TRIGGER agent_growth_children AFTER INSERT ON platform.agent_tasks
 FOR EACH ROW WHEN (NEW.agent_id='business_manager' AND NEW.action='coordinate_growth' AND NEW.parent_id IS NULL)
 EXECUTE FUNCTION platform.agent_growth_children();

CREATE FUNCTION platform.agent_growth_projection(p_task uuid,p_lease uuid) RETURNS jsonb
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 WITH principal AS MATERIALIZED (
 SELECT t.* FROM platform.agent_lease_task(p_task,p_lease) t JOIN platform.organizations o ON o.id=t.organization_id
 JOIN platform.agent_tasks parent ON parent.organization_id=t.organization_id AND parent.id=t.parent_id AND parent.action='coordinate_growth'
 WHERE o.business_model IN ('trade','hybrid') AND ((t.agent_id='sales' AND t.action='review_sales') OR
 (t.agent_id='product_inventory' AND t.action='review_inventory'))
 ), stock AS MATERIALIZED (
  SELECT t.id,CASE WHEN platform.inventory_freshness(i.organization_id,i.manual_updated_at)='FRESH' THEN i.quantity_on_hand-i.quantity_reserved END AS available
  FROM principal t JOIN platform.product_variants v ON v.organization_id=t.organization_id
  JOIN platform.products p ON p.id=v.product_id AND p.organization_id=v.organization_id AND p.status='active'
  LEFT JOIN platform.inventory i ON i.variant_id=v.id AND i.organization_id=v.organization_id
  WHERE t.agent_id='product_inventory'
 )
 SELECT CASE WHEN t.agent_id='sales' THEN jsonb_build_object(
 'units',coalesce(sum(i.quantity),0),'variants',count(DISTINCT i.variant_id),'categories',count(DISTINCT p.category_id))
 ELSE jsonb_build_object('in_stock',(SELECT count(*) FROM stock WHERE available>0),
 'out_of_stock',(SELECT count(*) FROM stock WHERE available=0),'unknown',(SELECT count(*) FROM stock WHERE available IS NULL)) END
 FROM principal t
 LEFT JOIN platform.orders o ON t.agent_id='sales' AND o.organization_id=t.organization_id AND o.request_kind='product_order'
 AND o.status='completed' AND o.created_at>=now()-interval '30 days' AND o.created_at<=now()
 LEFT JOIN platform.order_items i ON i.organization_id=o.organization_id AND i.order_id=o.id
 LEFT JOIN platform.products p ON p.organization_id=i.organization_id AND p.id=i.product_id GROUP BY t.id,t.agent_id
$$;
REVOKE ALL ON FUNCTION platform.agent_growth_projection(uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.agent_growth_projection(uuid,uuid) TO aba_app;
CREATE OR REPLACE FUNCTION platform.agent_child_results(p_task uuid,p_lease uuid) RETURNS jsonb
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT coalesce(jsonb_agg(jsonb_build_object('agent_id',c.agent_id,'status',c.status,'error_code',c.error_code,'result',e.result) ORDER BY c.agent_id),'[]')
 FROM platform.agent_lease_task(p_task,p_lease) t
 JOIN platform.agent_tasks c ON c.organization_id=t.organization_id AND c.parent_id=t.id
 LEFT JOIN LATERAL (SELECT x.result FROM platform.agent_executions x
 WHERE x.organization_id=c.organization_id AND x.task_id=c.id AND x.status='completed' ORDER BY x.attempt DESC LIMIT 1)e ON true
 WHERE t.agent_id='business_manager' AND t.action IN ('coordinate','coordinate_growth')
$$;
CREATE OR REPLACE FUNCTION platform.agent_metrics_projection(p_task uuid,p_lease uuid) RETURNS jsonb
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT jsonb_build_object(
 'count',count(o.id) FILTER(WHERE o.created_at>=now()-interval '30 days'),
 'revenue',coalesce(sum(o.total_amount) FILTER(WHERE o.created_at>=now()-interval '30 days'),0),
 'previous_count',count(o.id) FILTER(WHERE o.created_at<now()-interval '30 days'),
 'previous_value',coalesce(sum(o.total_amount) FILTER(WHERE o.created_at<now()-interval '30 days'),0))
 FROM platform.agent_lease_task(p_task,p_lease) t
 LEFT JOIN platform.orders o ON o.organization_id=t.organization_id AND o.status='completed' AND o.request_kind='product_order' AND o.created_at>=now()-interval '60 days' AND o.created_at<=now()
 WHERE t.agent_id IN ('analytics','finance') GROUP BY t.id
$$;
UPDATE platform.service_metadata SET version=20 WHERE singleton;
-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM platform.agent_tasks)
 OR EXISTS(SELECT 1 FROM platform.orders WHERE request_kind<>'product_order')
 OR EXISTS(SELECT 1 FROM platform.property_listings)
 OR EXISTS(SELECT 1 FROM platform.service_offerings)
 OR EXISTS(SELECT 1 FROM platform.service_resources) THEN
  RAISE EXCEPTION 'rollback refused: preserve orders, listings and agent history';
 END IF;
END $$;
DROP TRIGGER agent_growth_children ON platform.agent_tasks;
DROP FUNCTION platform.agent_growth_children(),platform.agent_growth_projection(uuid,uuid);
ALTER TABLE platform.agent_tasks DROP CONSTRAINT agent_task_operation;
ALTER TABLE platform.agent_tasks ADD CONSTRAINT agent_task_operation CHECK (
 (agent_id IN ('customer','sales','product_inventory') AND action IN ('get_product_price','check_inventory','get_product_details') AND variant_id IS NOT NULL AND conversation_id IS NOT NULL)
 OR (variant_id IS NULL AND conversation_id IS NULL AND ((agent_id='marketing' AND action='draft') OR
 (agent_id IN ('finance','analytics') AND action='read') OR (agent_id='business_manager' AND action IN ('plan','coordinate')))));
CREATE OR REPLACE FUNCTION platform.agent_child_results(p_task uuid,p_lease uuid) RETURNS jsonb
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT coalesce(jsonb_agg(jsonb_build_object('agent_id',c.agent_id,'status',c.status,'error_code',c.error_code,'result',e.result) ORDER BY c.agent_id),'[]')
 FROM platform.agent_lease_task(p_task,p_lease) t
 JOIN platform.agent_tasks c ON c.organization_id=t.organization_id AND c.parent_id=t.id
 LEFT JOIN LATERAL (SELECT x.result FROM platform.agent_executions x
 WHERE x.organization_id=c.organization_id AND x.task_id=c.id AND x.status='completed' ORDER BY x.attempt DESC LIMIT 1)e ON true
 WHERE t.agent_id='business_manager' AND t.action='coordinate'
$$;
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
-- Booking rollback fragment; no silent loss of confirmed/cancelled history.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM platform.service_bookings) THEN RAISE EXCEPTION 'rollback refused: preserve service booking history'; END IF;
END $$;
DROP TABLE platform.service_booking_allocations,platform.service_bookings;
DROP TABLE platform.service_availability_windows,platform.service_offering_resources,platform.service_resources,platform.service_offerings;
DROP FUNCTION platform.service_allocation_guard(),platform.service_booking_guard(),platform.service_resource_guard(),platform.service_window_guard();

DROP FUNCTION platform.service_lock_resource(uuid,uuid),platform.service_booking_scope(uuid);

DROP FUNCTION platform.service_tenant_write_guard(),platform.service_write_authorize(uuid,boolean);
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM platform.catalog_fitment_evidence) THEN
  RAISE EXCEPTION 'rollback refused: preserve catalog fitment evidence';
 END IF;
END $$;
DROP TABLE platform.catalog_fitment_evidence;
DROP FUNCTION platform.industry_catalog_profile(uuid);
DROP INDEX platform.industry_active_name,platform.industry_active_brand,
 platform.industry_variant_sku,platform.industry_variant_name,platform.industry_variant_price;
DO $$
DECLARE attr text;
BEGIN
 FOREACH attr IN ARRAY ARRAY['oem','manufacturer','model','color','material','size','size_system','selling_unit'] LOOP
  EXECUTE format('DROP INDEX platform.%I','industry_text_'||attr);
 END LOOP;
 FOREACH attr IN ARRAY ARRAY['memory_gb','warranty_months','weight_kg','volume_l','length_mm','width_mm','height_mm','package_quantity'] LOOP
  EXECUTE format('DROP INDEX platform.%I','industry_number_'||attr);
 END LOOP;
END $$;
DROP FUNCTION platform.industry_catalog_text(jsonb,text),platform.industry_catalog_number(jsonb,text,boolean);
DROP FUNCTION platform.order_request_business_model(uuid);
DROP TRIGGER order_request_immutable ON platform.orders;
DROP FUNCTION platform.order_request_immutable();
DROP POLICY universal_order_update ON platform.orders;
DROP POLICY universal_order_insert ON platform.orders;
ALTER TABLE platform.orders
 DROP CONSTRAINT orders_request_shape,
 DROP CONSTRAINT orders_conversation_tenant_fk,
 DROP CONSTRAINT orders_listing_tenant_fk,
 DROP COLUMN listing_id,
 DROP COLUMN request_details,
 DROP COLUMN request_kind,
 ALTER COLUMN total_amount SET NOT NULL,
 ALTER COLUMN quote_id SET NOT NULL;
DROP TABLE platform.property_listings;
UPDATE platform.service_metadata SET version=19 WHERE singleton;
-- +goose StatementEnd
