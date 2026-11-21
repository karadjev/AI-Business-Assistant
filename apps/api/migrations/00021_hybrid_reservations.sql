-- +goose Up
-- +goose StatementBegin
CREATE TABLE platform.hybrid_reservations (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 organization_id uuid NOT NULL,
 order_id uuid NOT NULL,
 status text NOT NULL DEFAULT 'active' CHECK(status IN ('active','released','consumed','expired')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 expires_at timestamptz NOT NULL CHECK(isfinite(expires_at)),
 UNIQUE(organization_id,order_id),
 FOREIGN KEY(organization_id,order_id) REFERENCES platform.orders(organization_id,id) ON DELETE CASCADE
);
CREATE INDEX hybrid_reservations_due ON platform.hybrid_reservations(expires_at,organization_id,order_id) WHERE status='active';
ALTER TABLE platform.hybrid_reservations ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.hybrid_reservations FORCE ROW LEVEL SECURITY;
CREATE POLICY migration_owner ON platform.hybrid_reservations TO aba_migrator USING(true) WITH CHECK(true);
CREATE POLICY staff_read ON platform.hybrid_reservations FOR SELECT TO aba_app
 USING(organization_id::text=current_setting('app.organization_id',true) AND platform.can_operate_orders(organization_id) AND platform.is_test_org(organization_id));
CREATE POLICY staff_insert ON platform.hybrid_reservations FOR INSERT TO aba_app
 WITH CHECK(organization_id::text=current_setting('app.organization_id',true) AND platform.can_operate_orders(organization_id) AND platform.is_test_org(organization_id)
 AND status='active' AND EXISTS(SELECT 1 FROM platform.service_bookings b WHERE b.organization_id=hybrid_reservations.organization_id
 AND b.request_id=hybrid_reservations.order_id AND b.status='confirmed'
 AND EXISTS(SELECT 1 FROM platform.orders o JOIN platform.order_items i ON i.organization_id=o.organization_id AND i.order_id=o.id
 WHERE o.organization_id=b.organization_id AND o.id=b.request_id AND o.request_kind='hybrid_order' AND o.request_details ? 'selection') AND hybrid_reservations.expires_at=b.ends_at+interval '24 hours'));
CREATE POLICY staff_update ON platform.hybrid_reservations FOR UPDATE TO aba_app
 USING(organization_id::text=current_setting('app.organization_id',true) AND platform.can_operate_orders(organization_id) AND platform.is_test_org(organization_id) AND status='active')
 WITH CHECK(organization_id::text=current_setting('app.organization_id',true) AND platform.can_operate_orders(organization_id) AND platform.is_test_org(organization_id)
 AND EXISTS(SELECT 1 FROM platform.orders o WHERE o.organization_id=hybrid_reservations.organization_id AND o.id=hybrid_reservations.order_id
 AND ((o.status='cancelled' AND hybrid_reservations.status='released') OR (o.status='completed' AND hybrid_reservations.status='consumed'))));
GRANT SELECT,INSERT,UPDATE(status) ON platform.hybrid_reservations TO aba_app;
ALTER TABLE platform.service_bookings DROP CONSTRAINT service_bookings_status_check;
ALTER TABLE platform.service_bookings ADD CONSTRAINT service_bookings_status_check CHECK(status IN ('confirmed','cancelled','completed'));
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
CREATE OR REPLACE FUNCTION platform.service_tenant_write_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE org uuid; owner_only boolean;
BEGIN
 IF (current_user='aba_migrator' AND TG_OP='UPDATE' AND TG_TABLE_NAME IN ('service_bookings','service_booking_allocations')) OR session_user='aba_migrator' OR EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname=session_user AND rolsuper) THEN
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
CREATE OR REPLACE FUNCTION platform.service_allocation_guard() RETURNS trigger LANGUAGE plpgsql
SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE cap integer; b platform.service_bookings; o platform.service_offerings; occupied integer;
BEGIN
 IF TG_OP='UPDATE' THEN
  IF (NEW.organization_id,NEW.booking_id,NEW.resource_id,NEW.starts_at,NEW.ends_at,NEW.units) IS DISTINCT FROM (OLD.organization_id,OLD.booking_id,OLD.resource_id,OLD.starts_at,OLD.ends_at,OLD.units) OR NOT OLD.active THEN
   RAISE EXCEPTION 'immutable service allocation' USING ERRCODE='23514';
  END IF;
 END IF;
 -- Only the due-only expiry function performs this transition as the schema owner.
 IF current_user='aba_migrator' AND TG_OP='UPDATE' AND NOT NEW.active AND EXISTS(
 SELECT 1 FROM platform.service_bookings sb JOIN platform.hybrid_reservations h ON h.organization_id=sb.organization_id AND h.order_id=sb.request_id
 WHERE sb.organization_id=NEW.organization_id AND sb.id=NEW.booking_id AND sb.status='cancelled' AND h.status='expired' AND h.expires_at<=clock_timestamp()) THEN
  RETURN NEW;
 END IF;
 cap:=platform.service_lock_resource(NEW.organization_id,NEW.resource_id);
 -- Read binding after acquiring the serialization lock, not before a possible wait.
 SELECT * INTO STRICT b FROM platform.service_bookings WHERE organization_id=NEW.organization_id AND id=NEW.booking_id;
 SELECT * INTO STRICT o FROM platform.service_offerings WHERE organization_id=b.organization_id AND id=b.offering_id;
 IF NOT NEW.active THEN
  IF b.status NOT IN ('cancelled','completed') THEN RAISE EXCEPTION 'booking not cancelled' USING ERRCODE='23514'; END IF;
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
-- The caller supplies no tenant, order, timestamp or new state. Only the
-- database clock opens this maintenance transition. An order lock fences it
-- against staff cancellation and completion.
CREATE FUNCTION platform.expire_hybrid_reservation() RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE h platform.hybrid_reservations; actor uuid; line record; stock platform.inventory; booking uuid;
BEGIN
 SELECT x.* INTO h
 FROM platform.hybrid_reservations x JOIN platform.orders o ON o.organization_id=x.organization_id AND o.id=x.order_id
 JOIN platform.organizations org ON org.id=o.organization_id AND org.is_test
 WHERE x.status='active' AND x.expires_at<=clock_timestamp() AND o.status='confirmed' AND o.request_kind='hybrid_order'
 ORDER BY x.expires_at,x.order_id LIMIT 1 FOR UPDATE OF o SKIP LOCKED;
 IF NOT FOUND THEN RETURN false; END IF;
 SELECT confirmed_by INTO STRICT actor FROM platform.orders WHERE organization_id=h.organization_id AND id=h.order_id;
 IF actor IS NULL THEN RAISE EXCEPTION 'missing reservation authorizer'; END IF;
 FOR line IN SELECT i.variant_id,i.quantity FROM platform.order_items i
 WHERE i.organization_id=h.organization_id AND i.order_id=h.order_id ORDER BY i.variant_id LOOP
  SELECT * INTO STRICT stock FROM platform.inventory i
  WHERE i.organization_id=h.organization_id AND i.variant_id=line.variant_id FOR UPDATE;
  IF stock.quantity_reserved<line.quantity THEN RAISE EXCEPTION 'reserved quantity requires reconciliation'; END IF;
  UPDATE platform.inventory SET quantity_reserved=stock.quantity_reserved-line.quantity,version=stock.version+1,updated_at=now()
  WHERE organization_id=h.organization_id AND variant_id=line.variant_id;
  INSERT INTO platform.inventory_movements (id,organization_id,variant_id,actor_id,reason,quantity_before,quantity_after,delta,inventory_version,kind,reserved_before,reserved_after,delta_reserved,order_id)
  VALUES(gen_random_uuid(),h.organization_id,line.variant_id,actor,'Истечение резерва; actor указывает сотрудника, одобрившего резерв',stock.quantity_on_hand,stock.quantity_on_hand,0,stock.version+1,'release',
   stock.quantity_reserved,stock.quantity_reserved-line.quantity,-line.quantity,h.order_id);
 END LOOP;
 SELECT id INTO STRICT booking FROM platform.service_bookings
 WHERE organization_id=h.organization_id AND request_id=h.order_id AND status='confirmed' FOR UPDATE;
 PERFORM r.id FROM platform.service_resources r JOIN platform.service_booking_allocations a
 ON a.organization_id=r.organization_id AND a.resource_id=r.id
 WHERE a.organization_id=h.organization_id AND a.booking_id=booking ORDER BY r.id FOR UPDATE OF r;
 UPDATE platform.orders SET status='cancelled',version=version+1,updated_at=now() WHERE organization_id=h.organization_id AND id=h.order_id;
 UPDATE platform.hybrid_reservations SET status='expired' WHERE organization_id=h.organization_id AND order_id=h.order_id;
 UPDATE platform.service_bookings SET status='cancelled' WHERE organization_id=h.organization_id AND id=booking;
 UPDATE platform.service_booking_allocations SET active=false WHERE organization_id=h.organization_id AND booking_id=booking AND active;
 INSERT INTO platform.order_events(id,organization_id,order_id,actor_id,kind,reason,idempotency_key,request_hash,result)
 VALUES(gen_random_uuid(),h.organization_id,h.order_id,NULL,'cancelled','hybrid_reservation_expired',gen_random_uuid(),
 sha256(convert_to(h.id::text,'UTF8')),jsonb_build_object('id',h.order_id,'status','cancelled','reservation_status','expired'));
 RETURN true;
END $$;
REVOKE ALL ON FUNCTION platform.expire_hybrid_reservation() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.expire_hybrid_reservation() TO aba_app;
-- An active booking must remain cancellable under the profile that authorized it.
CREATE FUNCTION platform.retain_active_hybrid_profile() RETURNS trigger
 LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
BEGIN
 IF EXISTS(SELECT 1 FROM platform.hybrid_reservations h WHERE h.organization_id=OLD.id AND h.status='active') THEN
  RAISE EXCEPTION 'active hybrid reservations prevent profile change' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
REVOKE ALL ON FUNCTION platform.retain_active_hybrid_profile() FROM PUBLIC;
CREATE TRIGGER retain_active_hybrid_profile BEFORE UPDATE OF business_model ON platform.organizations
 FOR EACH ROW WHEN (OLD.business_model='hybrid' AND NEW.business_model IS DISTINCT FROM OLD.business_model)
 EXECUTE FUNCTION platform.retain_active_hybrid_profile();
-- Only the current approved marketing lease can read this fixed projection.
-- Return no identifiers, customer data, catalog text, quantities or money.
CREATE FUNCTION platform.agent_marketing_context(p_task uuid,p_lease uuid) RETURNS jsonb
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT CASE
 WHEN parent.action IS DISTINCT FROM 'coordinate_growth' THEN jsonb_build_object('mode','legacy')
 WHEN parent.agent_id='business_manager' AND parent.initiator=t.initiator
 AND parent.correlation_id=t.correlation_id AND parent.status IN ('queued','running')
 AND parent.deadline>clock_timestamp() AND o.business_model IN ('trade','hybrid')
 THEN jsonb_build_object('mode','contextual', 'facts',jsonb_build_object(
  'industry',o.industry_profile,
  'inventory',CASE WHEN EXISTS(
   SELECT 1 FROM platform.inventory i
   JOIN platform.product_variants v ON v.organization_id=i.organization_id AND v.id=i.variant_id
   JOIN platform.products p ON p.organization_id=v.organization_id AND p.id=v.product_id
   LEFT JOIN platform.catalog_policies policy ON policy.organization_id=i.organization_id
   WHERE i.organization_id=t.organization_id AND p.status='active' AND i.quantity_on_hand>i.quantity_reserved
   AND platform.inventory_freshness_at(i.manual_updated_at,coalesce(policy.fresh_hours,24),coalesce(policy.stale_hours,72))='FRESH'
  ) THEN 'fresh_available' ELSE 'no_fresh_available' END,
  'demand',CASE WHEN EXISTS(
   SELECT 1 FROM platform.orders sales
   WHERE sales.organization_id=t.organization_id AND sales.request_kind='product_order' AND sales.status='completed'
   AND sales.created_at>=now()-interval '30 days' AND sales.created_at<=now()
  ) THEN 'recent_completed_orders' ELSE 'no_recent_completed_orders' END
 ))
 END
 FROM platform.agent_lease_task(p_task,p_lease) t
 JOIN platform.organizations o ON o.id=t.organization_id
 LEFT JOIN platform.agent_tasks parent ON parent.organization_id=t.organization_id AND parent.id=t.parent_id
 WHERE t.agent_id='marketing' AND t.action='draft'
$$;
REVOKE ALL ON FUNCTION platform.agent_marketing_context(uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.agent_marketing_context(uuid,uuid) TO aba_app;
UPDATE platform.service_metadata SET version=21 WHERE singleton;
-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM platform.hybrid_reservations) OR EXISTS(SELECT 1 FROM platform.orders WHERE request_kind='hybrid_order' AND request_details ? 'selection') OR EXISTS(SELECT 1 FROM platform.agent_tasks) THEN
  RAISE EXCEPTION 'rollback refused: preserve hybrid history and agent tasks';
 END IF;
END $$;
DROP FUNCTION platform.expire_hybrid_reservation();
DROP TRIGGER retain_active_hybrid_profile ON platform.organizations;
DROP FUNCTION platform.retain_active_hybrid_profile();
DROP FUNCTION platform.agent_marketing_context(uuid,uuid);
CREATE OR REPLACE FUNCTION platform.service_allocation_guard() RETURNS trigger LANGUAGE plpgsql
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
CREATE OR REPLACE FUNCTION platform.service_tenant_write_guard() RETURNS trigger
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
CREATE OR REPLACE FUNCTION platform.service_booking_guard() RETURNS trigger LANGUAGE plpgsql
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
ALTER TABLE platform.service_bookings DROP CONSTRAINT service_bookings_status_check;
ALTER TABLE platform.service_bookings ADD CONSTRAINT service_bookings_status_check CHECK(status IN ('confirmed','cancelled'));
DROP TABLE platform.hybrid_reservations;
UPDATE platform.service_metadata SET version=20 WHERE singleton;
-- +goose StatementEnd
