-- This fixture is installed only by the offline test runner after normal migrations.
BEGIN;
DO $$
BEGIN
 IF current_database()<>'aba_test' OR NOT EXISTS(
   SELECT FROM platform.test_safety WHERE singleton AND marker='aba-synthetic-isolated'
 ) THEN RAISE EXCEPTION 'isolated test database required'; END IF;
END $$;

CREATE FUNCTION platform.instagram_offline_enqueue() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE job record;
        property_delivery boolean:=NEW.action='property.delivery';
BEGIN
 SELECT t.organization_id,t.conversation_id,i.id AS integration_id,p.external_user_id,
        o.id AS order_id,o.version,o.status,e.result->0->>'message' AS body INTO job
 FROM platform.agent_tasks t
 JOIN platform.agent_executions e ON (e.organization_id,e.task_id,e.id)=(t.organization_id,t.id,NEW.execution_id)
 LEFT JOIN platform.order_events ev ON ev.organization_id=t.organization_id AND ev.id=t.idempotency_key
 LEFT JOIN platform.orders o ON (o.organization_id,o.id,o.conversation_id,o.idempotency_key)=(ev.organization_id,ev.order_id,t.conversation_id,t.parent_id)
 JOIN platform.conversations c ON (c.organization_id,c.id)=(t.organization_id,t.conversation_id)
 JOIN platform.channel_principals p ON (p.organization_id,p.conversation_id,p.id)=(c.organization_id,c.id,c.customer_id)
 JOIN platform.channel_integrations i ON (i.organization_id,i.id)=(p.organization_id,p.integration_id)
 JOIN platform.instagram_connections x ON (x.organization_id,x.integration_id)=(i.organization_id,i.id)
 JOIN platform.organizations z ON z.id=t.organization_id
 WHERE t.organization_id=NEW.organization_id AND t.id=NEW.task_id
 AND t.agent_id='business_manager' AND e.status='completed'
 AND ((property_delivery AND platform.real_estate_notification_current(t.id,t.organization_id,t.conversation_id))
 OR (NOT property_delivery AND t.action='delegation.service_receipt' AND c.customer_id=o.customer_id
 AND ev.kind=o.status AND o.version::text=e.result->0->'receipt'->>'version'))
 AND c.channel='instagram' AND p.channel='instagram' AND i.channel='instagram' AND i.mode='mock'
 AND z.is_test AND i.active AND NOT i.sending_enabled AND x.status='connected' AND x.revoked_at IS NULL
 AND x.webhook_subscribed AND x.webhook_seen_at IS NOT NULL AND x.token_expires_at>clock_timestamp()
 AND x.permissions @> ARRAY['instagram_business_basic','instagram_business_manage_messages']
 AND c.status='open' AND c.control_mode='agent' AND c.expires_at>clock_timestamp() AND p.window_until>clock_timestamp();
 IF NOT FOUND THEN RETURN NEW; END IF;
 IF NOT platform.booking_notification_consent(job.organization_id,job.conversation_id) THEN
   NEW.result:='consent_required';
 ELSIF octet_length(job.body) BETWEEN 1 AND 1000 THEN
   INSERT INTO platform.channel_outbox(id,organization_id,integration_id,conversation_id,external_user_id,payload,role,booking_order_id)
   VALUES(NEW.task_id,job.organization_id,job.integration_id,job.conversation_id,job.external_user_id,
          jsonb_build_object('text',job.body,'version',job.version,'status',job.status,'real_estate',property_delivery),'assistant',job.order_id)
   ON CONFLICT (id) DO NOTHING;
   NEW.result:='queued';
 END IF;
 RETURN NEW;
END $$;
REVOKE ALL ON FUNCTION platform.instagram_offline_enqueue() FROM PUBLIC;
CREATE TRIGGER instagram_offline_enqueue BEFORE INSERT ON platform.agent_audit FOR EACH ROW
 WHEN(NEW.action IN ('booking.delivery','property.delivery') AND NEW.result='instagram_send_disabled')
 EXECUTE FUNCTION platform.instagram_offline_enqueue();

CREATE FUNCTION platform.instagram_offline_claim(p_proof bytea,p_cap uuid) RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE q platform.channel_outbox; c platform.conversations; admitted jsonb;
BEGIN
 SELECT x.* INTO q FROM platform.channel_outbox x JOIN platform.channel_integrations i
 ON (i.organization_id,i.id)=(x.organization_id,x.integration_id)
 WHERE i.channel='instagram' AND i.worker_hash=sha256(p_proof)
 AND (x.booking_order_id IS NOT NULL OR x.payload->>'real_estate'='true') AND x.status='pending' AND x.available_at<=clock_timestamp()
 ORDER BY x.created_at,x.id LIMIT 1 FOR UPDATE OF x SKIP LOCKED;
 IF NOT FOUND THEN RETURN NULL; END IF;

 -- Keep consent, channel controls and booking status stable through the in-memory provider call.
 SELECT * INTO c FROM platform.conversations WHERE organization_id=q.organization_id AND id=q.conversation_id FOR SHARE NOWAIT;
 IF NOT pg_try_advisory_xact_lock(hashtextextended(q.integration_id::text,8210))
 OR NOT pg_try_advisory_xact_lock(CASE WHEN q.payload->>'real_estate'='true'
 THEN hashtextextended('real-estate-notification:'||q.organization_id::text,0)
 ELSE hashtextextended('booking-notification:'||q.booking_order_id::text,0) END) THEN
   RETURN NULL;
 END IF;
 SELECT jsonb_build_object('id',q.id,'organization',q.organization_id,'integration',q.integration_id,
   'conversation',q.conversation_id,'account',x.account_id,'recipient',p.external_user_id,
   'graph_version',i.graph_version,'cipher',encode(i.secret_cipher,'hex'),'expires',x.token_expires_at,
   'last_inbound',p.window_until-interval '24 hours','text',q.payload->>'text','attempt',q.attempts+1) INTO admitted
 FROM platform.channel_integrations i
 JOIN platform.instagram_connections x ON (x.organization_id,x.integration_id)=(i.organization_id,i.id)
 JOIN platform.channel_principals p ON (p.organization_id,p.integration_id,p.conversation_id,p.external_user_id)
       =(q.organization_id,q.integration_id,q.conversation_id,q.external_user_id)
 JOIN platform.organizations z ON z.id=i.organization_id
 LEFT JOIN platform.orders o ON (o.organization_id,o.id,o.conversation_id,o.customer_id)=(q.organization_id,q.booking_order_id,c.id,c.customer_id)
 WHERE (i.organization_id,i.id)=(q.organization_id,q.integration_id) AND i.channel='instagram'
 AND z.is_test AND i.mode='mock' AND i.active AND NOT i.sending_enabled AND i.worker_hash=sha256(p_proof)
 AND x.status='connected' AND x.revoked_at IS NULL AND x.webhook_subscribed AND x.webhook_seen_at IS NOT NULL
 AND x.permissions @> ARRAY['instagram_business_basic','instagram_business_manage_messages'] AND x.token_expires_at>clock_timestamp()
 AND p.id=c.customer_id AND p.channel='instagram' AND p.window_until>clock_timestamp()
 AND c.channel='instagram' AND c.status='open' AND c.control_mode='agent' AND c.expires_at>clock_timestamp()
 AND platform.booking_notification_consent(c.organization_id,c.id)
 AND ((q.payload->>'real_estate'='true' AND platform.real_estate_notification_current(q.id,q.organization_id,q.conversation_id))
 OR ((q.payload->>'real_estate' IS DISTINCT FROM 'true') AND o.version::text=q.payload->>'version' AND o.status=q.payload->>'status'
 AND (o.status<>'confirmed' OR (EXISTS(SELECT 1 FROM platform.service_bookings b
   WHERE b.organization_id=o.organization_id AND b.request_id=o.id AND b.status='confirmed')
   AND (o.request_kind<>'hybrid_order' OR EXISTS(SELECT 1 FROM platform.hybrid_reservations h
   WHERE h.organization_id=o.organization_id AND h.order_id=o.id AND h.status='active' AND h.expires_at>clock_timestamp()))))));
 IF admitted IS NULL THEN
   UPDATE platform.channel_outbox SET status='cancelled',version=version+1 WHERE id=q.id;
 ELSE
   UPDATE platform.channel_outbox SET status='sending',attempts=attempts+1,capability=p_cap,version=version+1 WHERE id=q.id;
 END IF;
 RETURN admitted;
EXCEPTION WHEN lock_not_available THEN RETURN NULL;
END $$;

CREATE FUNCTION platform.instagram_offline_finish(p_proof bytea,p_cap uuid,p_state text,p_delay integer,p_id text) RETURNS boolean
LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 WITH changed AS (
   UPDATE platform.channel_outbox q SET status=p_state,available_at=clock_timestamp()+make_interval(secs=>p_delay),
     capability=NULL,provider_id=nullif(p_id,''),version=version+1
   FROM platform.channel_integrations i
   WHERE (i.organization_id,i.id)=(q.organization_id,q.integration_id) AND i.channel='instagram'
   AND i.mode='mock' AND i.worker_hash=sha256(p_proof) AND (q.booking_order_id IS NOT NULL OR q.payload->>'real_estate'='true')
   AND q.status='sending' AND q.capability=p_cap
   AND p_state IN ('sent','failed','delivery_unknown','pending')
   AND (p_state<>'pending' OR (q.attempts<5 AND p_delay BETWEEN 1 AND 3600))
   AND (p_state='sent')=(nullif(p_id,'') IS NOT NULL)
   RETURNING q.id
 ) SELECT EXISTS(SELECT FROM changed)
$$;
REVOKE ALL ON FUNCTION platform.instagram_offline_claim(bytea,uuid),platform.instagram_offline_finish(bytea,uuid,text,integer,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.instagram_offline_claim(bytea,uuid),platform.instagram_offline_finish(bytea,uuid,text,integer,text) TO aba_app;
COMMIT;
