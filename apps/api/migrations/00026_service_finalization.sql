-- +goose Up
-- +goose StatementBegin
ALTER TABLE platform.service_offerings ADD COLUMN part_rule jsonb
 CHECK(part_rule IS NULL OR (jsonb_typeof(part_rule)='object' AND octet_length(part_rule::text)<=1024));
CREATE OR REPLACE FUNCTION platform.agent_delegation_parts(p_task uuid,p_lease uuid) RETURNS jsonb
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 WITH bound AS MATERIALIZED (
  SELECT t.organization_id,t.action,CASE WHEN parent.action='delegate_service' THEN jsonb_build_object('vehicle',parent.delegation_input->'vehicle','oem',
    coalesce(o.part_rule->>'oem',parent.delegation_input->'part'->>'oem')) ELSE parent.delegation_input END AS request FROM platform.agent_lease_task(p_task,p_lease) t
  JOIN platform.agent_tasks parent ON parent.organization_id=t.organization_id AND parent.id=t.parent_id AND parent.action IN ('delegate_auto_parts','delegate_service')
  LEFT JOIN platform.service_offerings o ON parent.action='delegate_service' AND o.organization_id=t.organization_id
     AND o.id=(parent.delegation_input->'selection'->>'offering_id')::uuid
  WHERE (parent.action<>'delegate_service' OR o.revision=(parent.delegation_input->'selection'->>'offering_revision')::bigint)
  AND t.agent_id='product_inventory' AND t.action IN ('delegation.find_parts','delegation.verify_fitment','delegation.service_parts')
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

-- The existing Order API inserts its event after booking and inventory writes.
-- Enqueue in that transaction so a rolled back confirmation never generates a receipt.
CREATE FUNCTION platform.enqueue_service_receipt() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE source platform.agent_tasks; revision bigint; task_id uuid:=gen_random_uuid();
BEGIN
 SELECT t.* INTO source
 FROM platform.orders o JOIN platform.agent_tasks t
 ON t.organization_id=o.organization_id AND t.id=o.idempotency_key AND t.conversation_id=o.conversation_id
 WHERE o.organization_id=NEW.organization_id AND o.id=NEW.order_id AND o.status=NEW.kind
 AND o.request_kind IN ('service_booking','hybrid_order')
 AND t.action='delegate_service' AND t.status='completed'
 AND EXISTS(SELECT 1 FROM platform.agent_executions e WHERE e.organization_id=t.organization_id
  AND e.task_id=t.id AND e.status='completed' AND e.result->0->>'state'='approval_required'
  AND e.result->0->'proposal'->'details'=o.request_details AND e.result->0->'proposal'->>'kind'=o.request_kind);
 IF NOT FOUND THEN RETURN NEW; END IF;
 SELECT version INTO STRICT revision FROM platform.orders WHERE organization_id=NEW.organization_id AND id=NEW.order_id;
 INSERT INTO platform.agent_tasks(id,organization_id,initiator,agent_id,parent_id,correlation_id,conversation_id,action,idempotency_key,worker_hash,status,deadline,delegation_step)
 VALUES(task_id,source.organization_id,source.initiator,'business_manager',source.id,source.correlation_id,
 source.conversation_id,'delegation.service_receipt',NEW.id,source.worker_hash,'queued',clock_timestamp()+interval '24 hours',5+revision)
 ON CONFLICT DO NOTHING;
 IF NOT FOUND THEN RETURN NEW; END IF;
 INSERT INTO platform.channel_inbox(id,organization_id,external_id,external_user_id,payload,channel,agent_task_id)
 VALUES(task_id,source.organization_id,task_id::text,'','{}','agent',task_id);
 INSERT INTO platform.agent_audit(organization_id,task_id,actor_id,agent_id,action,result)
 VALUES(source.organization_id,task_id,NEW.actor_id,'business_manager','task.delegate','queued');
 RETURN NEW;
END $$;
REVOKE ALL ON FUNCTION platform.enqueue_service_receipt() FROM PUBLIC;
CREATE TRIGGER enqueue_service_receipt AFTER INSERT ON platform.order_events FOR EACH ROW
 WHEN(NEW.kind IN ('confirmed','rejected','cancelled','completed'))
 EXECUTE FUNCTION platform.enqueue_service_receipt();

CREATE FUNCTION platform.service_receipt_context(p_task uuid,p_lease uuid) RETURNS jsonb
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT jsonb_strip_nulls(jsonb_build_object(
  'order_id',o.id,'version',o.version,'booking_status',o.status,
  'reservation_status',h.status,'reservation_expires_at',h.expires_at,
  'booking_id',b.id,'starts_at',coalesce(to_jsonb(b.starts_at),o.request_details->'selection'->'starts_at'),
  'ends_at',coalesce(to_jsonb(b.ends_at),o.request_details->'selection'->'ends_at'),
  'timezone',o.request_details->'selection'->>'timezone',
  'service',e.result->0->'offering'->>'name',
  'total_amount',o.total_amount,'currency',o.currency_code,
  'parts',coalesce((SELECT jsonb_agg(jsonb_build_object('name',i.name_snapshot,'sku',i.sku_snapshot,'quantity',i.quantity,'unit_price',i.unit_price_amount))
   FROM platform.order_items i WHERE i.organization_id=o.organization_id AND i.order_id=o.id),'[]'::jsonb),
  'specialists',coalesce((SELECT jsonb_agg(r.name ORDER BY r.id) FROM platform.service_booking_allocations a JOIN platform.service_resources r
   ON r.organization_id=a.organization_id AND r.id=a.resource_id AND r.kind='staff'
   WHERE a.organization_id=o.organization_id AND a.booking_id=b.id),'[]'::jsonb)
 ))
 FROM platform.agent_lease_task(p_task,p_lease) t
 JOIN platform.agent_tasks p ON p.organization_id=t.organization_id AND p.id=t.parent_id AND p.action='delegate_service'
 JOIN platform.agent_executions e ON e.organization_id=p.organization_id AND e.task_id=p.id AND e.status='completed'
 JOIN platform.order_events ev ON ev.organization_id=t.organization_id AND ev.id=t.idempotency_key
 JOIN platform.orders o ON o.organization_id=ev.organization_id AND o.id=ev.order_id AND o.idempotency_key=p.id AND o.conversation_id=p.conversation_id
 LEFT JOIN platform.service_bookings b ON b.organization_id=o.organization_id AND b.request_id=o.id
 LEFT JOIN platform.hybrid_reservations h ON h.organization_id=o.organization_id AND h.order_id=o.id
 WHERE t.agent_id='business_manager' AND t.action='delegation.service_receipt' AND o.status<>'pending_confirmation'
 AND e.result->0->'proposal'->'details'=o.request_details
 AND (o.status<>'confirmed' OR (b.status='confirmed' AND (o.request_kind<>'hybrid_order' OR h.status='active')))
$$;
REVOKE ALL ON FUNCTION platform.service_receipt_context(uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.service_receipt_context(uuid,uuid) TO aba_app;
UPDATE platform.service_metadata SET version=26 WHERE singleton;
-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN IF EXISTS(SELECT 1 FROM platform.agent_tasks) OR EXISTS(SELECT 1 FROM platform.service_offerings WHERE part_rule IS NOT NULL) THEN
 RAISE EXCEPTION 'rollback refused: preserve service delegation history and rules'; END IF; END $$;
DROP TRIGGER enqueue_service_receipt ON platform.order_events;
DROP FUNCTION platform.enqueue_service_receipt(),platform.service_receipt_context(uuid,uuid);
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
ALTER TABLE platform.service_offerings DROP COLUMN part_rule;
UPDATE platform.service_metadata SET version=25 WHERE singleton;
-- +goose StatementEnd
