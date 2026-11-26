-- +goose Up
-- +goose StatementBegin
CREATE TABLE platform.customer_consent_events (
 organization_id uuid NOT NULL,
 conversation_id uuid NOT NULL,
 customer_id uuid NOT NULL,
 channel text NOT NULL CHECK (channel IN ('internal_chat','telegram','whatsapp','instagram')),
 purpose text NOT NULL CHECK (purpose='marketing_service_offers'),
 source text NOT NULL CHECK (source='customer_message'),
 source_message_id uuid NOT NULL REFERENCES platform.messages(id) ON DELETE CASCADE,
 source_hash bytea NOT NULL CHECK (octet_length(source_hash)=32),
 occurred_at timestamptz NOT NULL,
 recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 decision text NOT NULL CHECK (decision IN ('granted','revoked')),
 research_only boolean NOT NULL DEFAULT true CHECK (research_only),
 PRIMARY KEY (organization_id,source_message_id),
 FOREIGN KEY (organization_id,conversation_id,customer_id) REFERENCES platform.conversations(organization_id,id,customer_id) ON DELETE CASCADE
);
CREATE INDEX customer_consent_history ON platform.customer_consent_events
 (organization_id,conversation_id,purpose,occurred_at DESC,decision DESC,source_message_id DESC);
ALTER TABLE platform.customer_consent_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.customer_consent_events FORCE ROW LEVEL SECURITY;
CREATE POLICY migration_owner ON platform.customer_consent_events TO aba_migrator USING(true) WITH CHECK(true);
CREATE POLICY operator_read ON platform.customer_consent_events FOR SELECT TO aba_app USING (
 organization_id::text=current_setting('app.organization_id',true) AND platform.is_test_org(organization_id) AND platform.can_operate_orders(organization_id));
GRANT SELECT (organization_id,conversation_id,customer_id,channel,purpose,source,source_message_id,occurred_at,recorded_at,decision,research_only)
 ON platform.customer_consent_events TO aba_app;

CREATE FUNCTION platform.capture_customer_consent() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE outcome text; c platform.conversations;
BEGIN
 outcome:=CASE lower(btrim(NEW.content))
  WHEN 'согласен получать предложения по услугам' THEN 'granted'
  WHEN 'отзываю согласие на предложения по услугам' THEN 'revoked'
 END;
 IF outcome IS NULL THEN RETURN NEW; END IF;
 SELECT * INTO c FROM platform.conversations WHERE (organization_id,id)=(NEW.organization_id,NEW.conversation_id) AND is_test;
 IF NOT FOUND THEN RETURN NEW; END IF;
 PERFORM pg_advisory_xact_lock(hashtextextended(c.id::text,4101));
 INSERT INTO platform.customer_consent_events(organization_id,conversation_id,customer_id,channel,purpose,source,source_message_id,source_hash,occurred_at,decision)
 VALUES(c.organization_id,c.id,c.customer_id,c.channel,'marketing_service_offers','customer_message',NEW.id,
  sha256(convert_to(NEW.content,'UTF8')),NEW.created_at,outcome);
 RETURN NEW;
END $$;
CREATE TRIGGER capture_customer_consent AFTER INSERT ON platform.messages
 FOR EACH ROW WHEN (NEW.role='customer') EXECUTE FUNCTION platform.capture_customer_consent();

CREATE FUNCTION platform.customer_marketing_consent(p_org uuid,p_conv uuid,p_channel text) RETURNS boolean
LANGUAGE sql STABLE SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT coalesce((SELECT e.decision='granted' AND e.research_only AND c.is_test AND c.status='open' AND c.expires_at>statement_timestamp()
 FROM platform.customer_consent_events e JOIN platform.conversations c
 ON (c.organization_id,c.id,c.customer_id,c.channel)=(e.organization_id,e.conversation_id,e.customer_id,e.channel)
 WHERE (e.organization_id,e.conversation_id,e.channel)=(p_org,p_conv,p_channel) AND e.purpose='marketing_service_offers'
 ORDER BY e.occurred_at DESC,e.decision DESC,e.source_message_id DESC LIMIT 1),false)
$$;

-- Keep registered templates and all financial checks. Gate marketing at both
-- insertion and dispatch, and leave previously uncertain deliveries untouched.
CREATE FUNCTION platform.wa_consent_enqueue() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
BEGIN
 IF NEW.payload ? 'template_id' AND EXISTS (SELECT 1 FROM platform.wa_templates t
  WHERE (t.organization_id,t.integration_id,t.id)=(NEW.organization_id,NEW.integration_id,(NEW.payload->>'template_id')::uuid)
   AND upper(t.category)='MARKETING') THEN
   PERFORM pg_advisory_xact_lock(hashtextextended(NEW.conversation_id::text,4101));
   IF NOT platform.customer_marketing_consent(NEW.organization_id,NEW.conversation_id,'whatsapp') THEN
    RAISE EXCEPTION 'marketing consent required' USING ERRCODE='42501';
   END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER wa_consent_enqueue BEFORE INSERT ON platform.channel_outbox FOR EACH ROW EXECUTE FUNCTION platform.wa_consent_enqueue();

ALTER FUNCTION platform.wa_dispatch_template(bytea,uuid) RENAME TO wa_dispatch_template_previous;
REVOKE EXECUTE ON FUNCTION platform.wa_dispatch_template_previous(bytea,uuid) FROM aba_app;
CREATE FUNCTION platform.wa_dispatch_template(p_proof bytea,p_cap uuid) RETURNS jsonb
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT platform.wa_dispatch_template_previous(p_proof,p_cap)
 FROM platform.channel_outbox q JOIN platform.channel_integrations i ON (i.organization_id,i.id)=(q.organization_id,q.integration_id)
 JOIN platform.wa_templates t ON (t.organization_id,t.integration_id,t.id)=(q.organization_id,q.integration_id,(q.payload->>'template_id')::uuid)
 WHERE q.capability=p_cap AND q.status='sending' AND q.lease_until>clock_timestamp() AND i.worker_hash=sha256(p_proof)
 AND (upper(t.category)<>'MARKETING' OR (i.mode='mock' AND t.synthetic AND platform.customer_marketing_consent(q.organization_id,q.conversation_id,'whatsapp')
  AND NOT EXISTS (SELECT 1 FROM platform.customer_consent_events e
    WHERE (e.organization_id,e.conversation_id,e.channel)=(q.organization_id,q.conversation_id,'whatsapp')
      AND e.decision='revoked' AND e.recorded_at>=q.created_at)
  AND EXISTS (SELECT 1 FROM platform.customer_consent_events e
    WHERE (e.organization_id,e.conversation_id,e.channel)=(q.organization_id,q.conversation_id,'whatsapp')
      AND e.decision='granted' AND e.recorded_at<=q.created_at)))
$$;
REVOKE ALL ON FUNCTION platform.wa_dispatch_template(bytea,uuid),platform.wa_consent_enqueue(),
 platform.capture_customer_consent(),platform.customer_marketing_consent(uuid,uuid,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.wa_dispatch_template(bytea,uuid) TO aba_app;
CREATE OR REPLACE FUNCTION platform.agent_daily_context(p_task uuid,p_lease uuid) RETURNS jsonb
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 WITH principal AS MATERIALIZED (
  SELECT t.*,p.created_at AS cutoff,o.timezone,o.business_model,p.created_at AT TIME ZONE o.timezone AS local_time
  FROM platform.agent_lease_task(p_task,p_lease) t
  JOIN platform.agent_tasks p ON p.organization_id=t.organization_id AND p.id=coalesce(t.parent_id,t.id)
   AND p.agent_id='business_manager' AND p.action='coordinate_daily'
  JOIN platform.organizations o ON o.id=t.organization_id AND o.is_test
  WHERE (t.action='daily_review' AND t.agent_id IN ('customer','sales','product_inventory','marketing'))
     OR (t.action='read' AND t.agent_id IN ('finance','analytics'))
     OR (t.action='coordinate_daily' AND t.agent_id='business_manager')
 ), stock AS MATERIALIZED (
  SELECT CASE WHEN platform.inventory_freshness(i.organization_id,i.manual_updated_at)='FRESH'
   THEN i.quantity_on_hand-i.quantity_reserved END AS available
  FROM principal t JOIN platform.products p ON p.organization_id=t.organization_id AND p.status='active'
  JOIN platform.product_variants v ON v.organization_id=t.organization_id AND v.product_id=p.id
  LEFT JOIN platform.inventory i ON i.organization_id=v.organization_id AND i.variant_id=v.id
  WHERE t.agent_id='product_inventory' AND t.business_model IN ('trade','hybrid')
 ), marketing_candidates AS MATERIALIZED (
  SELECT DISTINCT ON (c.id) c.id AS conversation_id, f.name AS offering_name
  FROM principal t JOIN platform.conversations c ON c.organization_id=t.organization_id
   AND c.channel='internal_chat' AND c.status='open' AND c.expires_at>t.cutoff
  JOIN platform.orders o ON o.organization_id=c.organization_id AND o.conversation_id=c.id AND o.status='completed' AND o.request_kind='service_booking'
  JOIN platform.service_receipts r ON r.organization_id=o.organization_id AND r.order_id=o.id AND r.recorded_at<=t.cutoff
  JOIN platform.service_bookings b ON b.organization_id=o.organization_id AND b.request_id=o.id
  JOIN platform.service_offerings f ON f.organization_id=b.organization_id AND f.id=b.offering_id AND f.status='active'
  WHERE t.agent_id='marketing' AND platform.customer_marketing_consent(c.organization_id,c.id,c.channel)
  ORDER BY c.id,r.recorded_at DESC,r.order_id
 )
 SELECT jsonb_build_object('date',t.local_time::date,'timezone',t.timezone,'as_of',t.cutoff,
 'proposals',CASE WHEN t.agent_id='marketing' THEN (SELECT coalesce(jsonb_agg(to_jsonb(x) ORDER BY x.conversation_id),'[]')
  FROM (SELECT * FROM marketing_candidates ORDER BY conversation_id LIMIT 8)x) END,'metrics',
 CASE t.agent_id
 WHEN 'customer' THEN jsonb_build_object(
  'open_conversations',(SELECT count(*) FROM platform.conversations c WHERE c.organization_id=t.organization_id AND c.status='open' AND c.expires_at>t.cutoff),
  'awaiting_staff',(SELECT count(*) FROM platform.conversations c WHERE c.organization_id=t.organization_id AND c.status='open' AND c.expires_at>t.cutoff
   AND EXISTS(SELECT 1 FROM platform.messages m WHERE m.organization_id=c.organization_id AND m.conversation_id=c.id AND m.role='customer'
    AND m.content<>'' AND m.created_at<=t.cutoff
    AND NOT EXISTS(SELECT 1 FROM platform.messages a WHERE a.organization_id=c.organization_id AND a.conversation_id=c.id AND a.role='staff'
     AND a.content<>'' AND a.created_at>=m.created_at AND a.created_at<=t.cutoff))))
 WHEN 'sales' THEN jsonb_build_object(
  'requests_today',(SELECT count(*) FROM platform.orders o WHERE o.organization_id=t.organization_id AND o.created_at<=t.cutoff AND (o.created_at AT TIME ZONE t.timezone)::date=t.local_time::date),
  'pending_confirmation',(SELECT count(*) FROM platform.orders o WHERE o.organization_id=t.organization_id AND o.created_at<=t.cutoff AND o.status='pending_confirmation'),
  'confirmed_open',(SELECT count(*) FROM platform.orders o WHERE o.organization_id=t.organization_id AND o.created_at<=t.cutoff AND o.status='confirmed'))
 WHEN 'product_inventory' THEN jsonb_build_object(
  'stock_zero',(SELECT count(*) FROM stock WHERE available=0),
  'stock_unknown',(SELECT count(*) FROM stock WHERE available IS NULL),
  'unassigned_bookings',(SELECT count(*) FROM platform.service_bookings b JOIN platform.orders o ON o.organization_id=b.organization_id AND o.id=b.request_id
   WHERE b.organization_id=t.organization_id AND o.request_kind='service_booking' AND o.status='confirmed'
   AND b.ends_at>t.cutoff AND (b.starts_at AT TIME ZONE t.timezone)::date=t.local_time::date
   AND NOT EXISTS(SELECT 1 FROM platform.service_assignments a WHERE a.organization_id=b.organization_id AND a.booking_id=b.id)))
 WHEN 'marketing' THEN jsonb_build_object('eligible_service_conversations',(SELECT count(*) FROM marketing_candidates))
 WHEN 'finance' THEN (SELECT jsonb_build_object(
  'paid_services_today',count(*),'received_minor',sum(received_minor),'cost_minor',sum(cost_minor),
  'ai_settled_microrub',(SELECT sum(charged) FROM platform.llm_reservations l WHERE l.organization_id=t.organization_id
   AND l.status='settled' AND l.completed_at<=t.cutoff AND (l.completed_at AT TIME ZONE t.timezone)::date=t.local_time::date),
  'ai_unsettled_count',(SELECT count(*) FROM platform.llm_reservations l WHERE l.organization_id=t.organization_id
   AND l.status IN ('pending','uncertain') AND l.created_at<=t.cutoff AND (l.created_at AT TIME ZONE t.timezone)::date=t.local_time::date))
 FROM platform.service_receipts r WHERE r.organization_id=t.organization_id AND r.recorded_at<=t.cutoff
  AND (r.recorded_at AT TIME ZONE t.timezone)::date=t.local_time::date)
 WHEN 'analytics' THEN (SELECT jsonb_build_object(
  'bookings_today',count(*),'assigned_today',count(a.booking_id) FILTER (WHERE o.status IN ('confirmed','completed')),
  'employees_booked_today',count(DISTINCT a.user_id) FILTER (WHERE o.status IN ('confirmed','completed')),
  'booked_minutes_today',coalesce(sum(extract(epoch FROM b.ends_at-b.starts_at)::bigint / 60) FILTER (WHERE a.booking_id IS NOT NULL AND o.status IN ('confirmed','completed')),0))
 FROM platform.service_bookings b JOIN platform.orders o ON o.organization_id=b.organization_id AND o.id=b.request_id
 LEFT JOIN platform.service_assignments a ON a.organization_id=b.organization_id AND a.booking_id=b.id
 WHERE b.organization_id=t.organization_id AND b.status='confirmed' AND (b.starts_at AT TIME ZONE t.timezone)::date=t.local_time::date)
 ELSE '{}'::jsonb END) FROM principal t
$$;

UPDATE platform.service_metadata SET version=41 WHERE singleton;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM platform.customer_consent_events) THEN RAISE EXCEPTION 'rollback refused: preserve customer consent history'; END IF;
END $$;
DROP TRIGGER capture_customer_consent ON platform.messages;
DROP TRIGGER wa_consent_enqueue ON platform.channel_outbox;
DROP FUNCTION platform.capture_customer_consent(),platform.wa_consent_enqueue(),platform.wa_dispatch_template(bytea,uuid);
ALTER FUNCTION platform.wa_dispatch_template_previous(bytea,uuid) RENAME TO wa_dispatch_template;
GRANT EXECUTE ON FUNCTION platform.wa_dispatch_template(bytea,uuid) TO aba_app;
CREATE OR REPLACE FUNCTION platform.agent_daily_context(p_task uuid,p_lease uuid) RETURNS jsonb
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 WITH principal AS MATERIALIZED (
  SELECT t.*,p.created_at AS cutoff,o.timezone,o.business_model,p.created_at AT TIME ZONE o.timezone AS local_time
  FROM platform.agent_lease_task(p_task,p_lease) t
  JOIN platform.agent_tasks p ON p.organization_id=t.organization_id AND p.id=coalesce(t.parent_id,t.id)
   AND p.agent_id='business_manager' AND p.action='coordinate_daily'
  JOIN platform.organizations o ON o.id=t.organization_id AND o.is_test
  WHERE (t.action='daily_review' AND t.agent_id IN ('customer','sales','product_inventory','marketing'))
     OR (t.action='read' AND t.agent_id IN ('finance','analytics'))
     OR (t.action='coordinate_daily' AND t.agent_id='business_manager')
 ), stock AS MATERIALIZED (
  SELECT CASE WHEN platform.inventory_freshness(i.organization_id,i.manual_updated_at)='FRESH'
   THEN i.quantity_on_hand-i.quantity_reserved END AS available
  FROM principal t JOIN platform.products p ON p.organization_id=t.organization_id AND p.status='active'
  JOIN platform.product_variants v ON v.organization_id=t.organization_id AND v.product_id=p.id
  LEFT JOIN platform.inventory i ON i.organization_id=v.organization_id AND i.variant_id=v.id
  WHERE t.agent_id='product_inventory' AND t.business_model IN ('trade','hybrid')
 ), marketing_candidates AS MATERIALIZED (
  SELECT DISTINCT ON (c.id) c.id AS conversation_id, f.name AS offering_name
  FROM principal t JOIN platform.conversations c ON c.organization_id=t.organization_id
   AND c.channel='internal_chat' AND c.status='open' AND c.expires_at>t.cutoff
  JOIN LATERAL (
   SELECT lower(btrim(m.content)) AS decision FROM platform.messages m
   WHERE m.organization_id=c.organization_id AND m.conversation_id=c.id AND m.role='customer' AND m.created_at<=t.cutoff
    AND lower(btrim(m.content)) IN ('согласен получать предложения по услугам','отзываю согласие на предложения по услугам')
   ORDER BY m.created_at DESC,m.id DESC LIMIT 1
  ) consent ON consent.decision='согласен получать предложения по услугам'
  JOIN platform.orders o ON o.organization_id=c.organization_id AND o.conversation_id=c.id AND o.status='completed' AND o.request_kind='service_booking'
  JOIN platform.service_receipts r ON r.organization_id=o.organization_id AND r.order_id=o.id AND r.recorded_at<=t.cutoff
  JOIN platform.service_bookings b ON b.organization_id=o.organization_id AND b.request_id=o.id
  JOIN platform.service_offerings f ON f.organization_id=b.organization_id AND f.id=b.offering_id AND f.status='active'
  WHERE t.agent_id='marketing'
  ORDER BY c.id,r.recorded_at DESC,r.order_id
 )
 SELECT jsonb_build_object('date',t.local_time::date,'timezone',t.timezone,'as_of',t.cutoff,
 'proposals',CASE WHEN t.agent_id='marketing' THEN (SELECT coalesce(jsonb_agg(to_jsonb(x) ORDER BY x.conversation_id),'[]')
  FROM (SELECT * FROM marketing_candidates ORDER BY conversation_id LIMIT 8)x) END,'metrics',
 CASE t.agent_id
 WHEN 'customer' THEN jsonb_build_object(
  'open_conversations',(SELECT count(*) FROM platform.conversations c WHERE c.organization_id=t.organization_id AND c.status='open' AND c.expires_at>t.cutoff),
  'awaiting_staff',(SELECT count(*) FROM platform.conversations c WHERE c.organization_id=t.organization_id AND c.status='open' AND c.expires_at>t.cutoff
   AND EXISTS(SELECT 1 FROM platform.messages m WHERE m.organization_id=c.organization_id AND m.conversation_id=c.id AND m.role='customer'
    AND m.content<>'' AND m.created_at<=t.cutoff
    AND NOT EXISTS(SELECT 1 FROM platform.messages a WHERE a.organization_id=c.organization_id AND a.conversation_id=c.id AND a.role='staff'
     AND a.content<>'' AND a.created_at>=m.created_at AND a.created_at<=t.cutoff))))
 WHEN 'sales' THEN jsonb_build_object(
  'requests_today',(SELECT count(*) FROM platform.orders o WHERE o.organization_id=t.organization_id AND o.created_at<=t.cutoff AND (o.created_at AT TIME ZONE t.timezone)::date=t.local_time::date),
  'pending_confirmation',(SELECT count(*) FROM platform.orders o WHERE o.organization_id=t.organization_id AND o.created_at<=t.cutoff AND o.status='pending_confirmation'),
  'confirmed_open',(SELECT count(*) FROM platform.orders o WHERE o.organization_id=t.organization_id AND o.created_at<=t.cutoff AND o.status='confirmed'))
 WHEN 'product_inventory' THEN jsonb_build_object(
  'stock_zero',(SELECT count(*) FROM stock WHERE available=0),
  'stock_unknown',(SELECT count(*) FROM stock WHERE available IS NULL),
  'unassigned_bookings',(SELECT count(*) FROM platform.service_bookings b JOIN platform.orders o ON o.organization_id=b.organization_id AND o.id=b.request_id
   WHERE b.organization_id=t.organization_id AND o.request_kind='service_booking' AND o.status='confirmed'
   AND b.ends_at>t.cutoff AND (b.starts_at AT TIME ZONE t.timezone)::date=t.local_time::date
   AND NOT EXISTS(SELECT 1 FROM platform.service_assignments a WHERE a.organization_id=b.organization_id AND a.booking_id=b.id)))
 WHEN 'marketing' THEN jsonb_build_object('eligible_service_conversations',(SELECT count(*) FROM marketing_candidates))
 WHEN 'finance' THEN (SELECT jsonb_build_object(
  'paid_services_today',count(*),'received_minor',sum(received_minor),'cost_minor',sum(cost_minor),
  'ai_settled_microrub',(SELECT sum(charged) FROM platform.llm_reservations l WHERE l.organization_id=t.organization_id
   AND l.status='settled' AND l.completed_at<=t.cutoff AND (l.completed_at AT TIME ZONE t.timezone)::date=t.local_time::date),
  'ai_unsettled_count',(SELECT count(*) FROM platform.llm_reservations l WHERE l.organization_id=t.organization_id
   AND l.status IN ('pending','uncertain') AND l.created_at<=t.cutoff AND (l.created_at AT TIME ZONE t.timezone)::date=t.local_time::date))
 FROM platform.service_receipts r WHERE r.organization_id=t.organization_id AND r.recorded_at<=t.cutoff
  AND (r.recorded_at AT TIME ZONE t.timezone)::date=t.local_time::date)
 WHEN 'analytics' THEN (SELECT jsonb_build_object(
  'bookings_today',count(*),'assigned_today',count(a.booking_id) FILTER (WHERE o.status IN ('confirmed','completed')),
  'employees_booked_today',count(DISTINCT a.user_id) FILTER (WHERE o.status IN ('confirmed','completed')),
  'booked_minutes_today',coalesce(sum(extract(epoch FROM b.ends_at-b.starts_at)::bigint / 60) FILTER (WHERE a.booking_id IS NOT NULL AND o.status IN ('confirmed','completed')),0))
 FROM platform.service_bookings b JOIN platform.orders o ON o.organization_id=b.organization_id AND o.id=b.request_id
 LEFT JOIN platform.service_assignments a ON a.organization_id=b.organization_id AND a.booking_id=b.id
 WHERE b.organization_id=t.organization_id AND b.status='confirmed' AND (b.starts_at AT TIME ZONE t.timezone)::date=t.local_time::date)
 ELSE '{}'::jsonb END) FROM principal t
$$;

DROP FUNCTION platform.customer_marketing_consent(uuid,uuid,text);
DROP TABLE platform.customer_consent_events;
UPDATE platform.service_metadata SET version=40 WHERE singleton;
-- +goose StatementEnd
