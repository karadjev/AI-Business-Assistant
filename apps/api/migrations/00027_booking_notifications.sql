-- +goose Up
-- +goose StatementBegin
ALTER TABLE platform.telegram_outbox ADD COLUMN booking_order_id uuid;
ALTER TABLE platform.channel_outbox ADD COLUMN booking_order_id uuid;
CREATE FUNCTION platform.booking_notification_consent(p_org uuid,p_conversation uuid) RETURNS boolean
LANGUAGE sql STABLE SET search_path=pg_catalog,platform AS $$
 SELECT coalesce((SELECT content='booking_updates:on' FROM platform.messages
 WHERE organization_id=p_org AND conversation_id=p_conversation AND role='customer'
 AND content IN ('booking_updates:on','booking_updates:off') ORDER BY created_at DESC,id DESC LIMIT 1),false)
$$;
REVOKE ALL ON FUNCTION platform.booking_notification_consent(uuid,uuid) FROM PUBLIC;

CREATE FUNCTION platform.enqueue_booking_notification() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE t platform.agent_tasks; o platform.orders; c platform.conversations;
 body jsonb:=NEW.result->0; receipt jsonb:=body->'receipt'; outcome text:='channel_unavailable';
BEGIN
 IF body->>'state' IS DISTINCT FROM 'backend_result' THEN RETURN NEW; END IF;
 SELECT * INTO t FROM platform.agent_tasks WHERE id=NEW.task_id AND organization_id=NEW.organization_id
 AND action='delegation.service_receipt' AND agent_id='business_manager';
 IF NOT FOUND THEN RETURN NEW; END IF;
 SELECT x.* INTO o FROM platform.orders x JOIN platform.order_events ev
 ON ev.organization_id=x.organization_id AND ev.order_id=x.id AND ev.id=t.idempotency_key
 WHERE x.organization_id=t.organization_id AND x.conversation_id=t.conversation_id
 AND x.idempotency_key=t.parent_id;
 IF NOT FOUND THEN RETURN NEW; END IF;
 SELECT * INTO c FROM platform.conversations WHERE organization_id=t.organization_id AND id=t.conversation_id;
 IF o.version::text IS DISTINCT FROM receipt->>'version' OR o.status IS DISTINCT FROM receipt->>'booking_status'
 OR o.id::text IS DISTINCT FROM receipt->>'order_id' OR o.status IS DISTINCT FROM (SELECT kind FROM platform.order_events WHERE id=t.idempotency_key) THEN
  outcome:='superseded';
 ELSIF c.channel='instagram' THEN
  outcome:='instagram_send_disabled';
 ELSIF c.status='open' AND c.expires_at>clock_timestamp() AND c.control_mode='agent'
 AND EXISTS(SELECT 1 FROM platform.organizations WHERE id=o.organization_id AND is_test)
 AND c.customer_id=o.customer_id
 AND ((c.channel='telegram' AND NOT EXISTS(SELECT 1 FROM platform.channel_principals WHERE organization_id=c.organization_id AND conversation_id=c.id)
      AND 1=(SELECT count(*) FROM platform.telegram_principals p WHERE p.organization_id=c.organization_id AND p.conversation_id=c.id)
      AND EXISTS(SELECT 1 FROM platform.telegram_principals p JOIN platform.telegram_integrations i ON i.organization_id=p.organization_id AND i.id=p.integration_id
          WHERE p.organization_id=c.organization_id AND p.conversation_id=c.id AND i.active AND i.mode='mock'))
   OR (c.channel='whatsapp' AND NOT EXISTS(SELECT 1 FROM platform.telegram_principals WHERE organization_id=c.organization_id AND conversation_id=c.id)
      AND EXISTS(SELECT 1 FROM platform.channel_principals p JOIN platform.channel_integrations i ON i.organization_id=p.organization_id AND i.id=p.integration_id
          WHERE p.organization_id=c.organization_id AND p.conversation_id=c.id AND p.id=c.customer_id AND p.channel='whatsapp' AND i.channel='whatsapp'
          AND i.active AND i.mode='mock' AND i.sending_enabled AND p.window_until>clock_timestamp()))) THEN
  IF NOT platform.booking_notification_consent(c.organization_id,c.id) THEN
   outcome:='consent_required';
  ELSIF length(coalesce(body->>'message','')) BETWEEN 1 AND 2000 THEN
   INSERT INTO platform.messages(id,organization_id,conversation_id,role,content,data,client_key)
   VALUES(t.id,t.organization_id,t.conversation_id,'assistant',body->>'message',
   jsonb_build_object('booking_notification',jsonb_build_object('order',o.id,'version',o.version,'status',o.status)),t.id)
   ON CONFLICT DO NOTHING;
   UPDATE platform.telegram_outbox SET booking_order_id=o.id WHERE id=t.id AND organization_id=t.organization_id;
   UPDATE platform.channel_outbox SET booking_order_id=o.id WHERE id=t.id AND organization_id=t.organization_id;
   IF EXISTS(SELECT 1 FROM platform.telegram_outbox WHERE id=t.id AND organization_id=t.organization_id)
      OR EXISTS(SELECT 1 FROM platform.channel_outbox WHERE id=t.id AND organization_id=t.organization_id) THEN outcome:='queued'; END IF;
  END IF;
 END IF;
 INSERT INTO platform.agent_audit(organization_id,task_id,execution_id,agent_id,action,result)
 VALUES(t.organization_id,t.id,NEW.id,t.agent_id,'booking.delivery',outcome);
 RETURN NEW;
END $$;
REVOKE ALL ON FUNCTION platform.enqueue_booking_notification() FROM PUBLIC;
CREATE TRIGGER enqueue_booking_notification AFTER UPDATE OF status ON platform.agent_executions FOR EACH ROW
 WHEN(OLD.status='running' AND NEW.status='completed')
 EXECUTE FUNCTION platform.enqueue_booking_notification();

CREATE FUNCTION platform.audit_booking_notification_state() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
BEGIN
 INSERT INTO platform.agent_audit(organization_id,task_id,agent_id,action,result)
 SELECT organization_id,id,agent_id,'booking.delivery_state',NEW.status
 FROM platform.agent_tasks WHERE organization_id=NEW.organization_id AND id=NEW.id
 AND action='delegation.service_receipt' AND agent_id='business_manager';
 RETURN NEW;
END $$;
REVOKE ALL ON FUNCTION platform.audit_booking_notification_state() FROM PUBLIC;
CREATE TRIGGER telegram_booking_notification_state AFTER UPDATE OF status ON platform.telegram_outbox FOR EACH ROW
 WHEN(NEW.booking_order_id IS NOT NULL AND NEW.status IS DISTINCT FROM OLD.status)
 EXECUTE FUNCTION platform.audit_booking_notification_state();
CREATE TRIGGER channel_booking_notification_state AFTER UPDATE OF status ON platform.channel_outbox FOR EACH ROW
 WHEN(NEW.booking_order_id IS NOT NULL AND NEW.status IS DISTINCT FROM OLD.status)
 EXECUTE FUNCTION platform.audit_booking_notification_state();

-- Senders pin this same order lock through the bounded provider call.
CREATE FUNCTION platform.booking_notification_transition() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF OLD.request_kind IN ('service_booking','hybrid_order') THEN
  PERFORM pg_advisory_xact_lock(hashtextextended('booking-notification:'||OLD.id::text,0));
 END IF;
 RETURN NEW;
END $$;
REVOKE ALL ON FUNCTION platform.booking_notification_transition() FROM PUBLIC;
CREATE TRIGGER booking_notification_transition BEFORE UPDATE OF status,version ON platform.orders FOR EACH ROW
 EXECUTE FUNCTION platform.booking_notification_transition();

CREATE FUNCTION platform.booking_notification_admit(p_cap uuid,p_channel text)
RETURNS TABLE(allowed boolean,lock_key bigint)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE item record; key bigint; marker jsonb;
BEGIN
 IF p_channel='telegram' THEN
  SELECT q.id,q.organization_id,q.conversation_id,q.lease_until,q.booking_order_id,q.integration_id,q.chat_id::text AS recipient,i.mode INTO item
  FROM platform.telegram_outbox q JOIN platform.telegram_integrations i ON i.id=q.integration_id AND i.organization_id=q.organization_id
  WHERE q.capability=p_cap AND q.status='sending';
 ELSIF p_channel='whatsapp' THEN
  SELECT q.id,q.organization_id,q.conversation_id,q.lease_until,q.booking_order_id,q.integration_id,q.external_user_id AS recipient,i.mode INTO item
  FROM platform.channel_outbox q JOIN platform.channel_integrations i ON i.id=q.integration_id AND i.organization_id=q.organization_id
  WHERE q.capability=p_cap AND q.status='sending' AND i.channel='whatsapp';
 ELSE RETURN QUERY SELECT false,NULL::bigint; RETURN;
 END IF;
 IF item.id IS NULL THEN RETURN QUERY SELECT false,NULL::bigint; RETURN; END IF;
 SELECT m.data->'booking_notification' INTO marker FROM platform.messages m
 WHERE m.id=item.id AND m.organization_id=item.organization_id AND m.conversation_id=item.conversation_id;
 IF item.booking_order_id IS NULL THEN RETURN QUERY SELECT marker IS NULL,NULL::bigint; RETURN; END IF;
 key:=hashtextextended('booking-notification:'||item.booking_order_id::text,0);
 PERFORM pg_advisory_lock(key);
 IF item.mode='mock' AND item.lease_until>clock_timestamp()+interval '6 seconds'
 AND EXISTS(SELECT 1 FROM platform.orders o JOIN platform.conversations c ON c.organization_id=o.organization_id AND c.id=o.conversation_id
  WHERE o.organization_id=item.organization_id AND o.conversation_id=item.conversation_id
  AND o.id=item.booking_order_id AND o.id::text=marker->>'order' AND o.version::text=marker->>'version' AND o.status=marker->>'status'
  AND c.customer_id=o.customer_id AND c.channel=p_channel AND c.status='open' AND c.expires_at>clock_timestamp()
  AND ((p_channel='telegram' AND EXISTS(SELECT 1 FROM platform.telegram_principals p
    WHERE p.organization_id=c.organization_id AND p.conversation_id=c.id
    AND p.integration_id=item.integration_id AND p.chat_id::text=item.recipient))
   OR (p_channel='whatsapp' AND EXISTS(SELECT 1 FROM platform.channel_principals p
    WHERE p.organization_id=c.organization_id AND p.conversation_id=c.id AND p.id=c.customer_id
    AND p.integration_id=item.integration_id AND p.external_user_id=item.recipient AND p.channel='whatsapp'
    AND p.window_until>clock_timestamp())))
  AND platform.booking_notification_consent(c.organization_id,c.id)
  AND EXISTS(SELECT 1 FROM platform.organizations x WHERE x.id=o.organization_id AND x.is_test)
  AND (o.status<>'confirmed' OR (EXISTS(SELECT 1 FROM platform.service_bookings b WHERE b.organization_id=o.organization_id AND b.request_id=o.id AND b.status='confirmed')
   AND (o.request_kind<>'hybrid_order' OR EXISTS(SELECT 1 FROM platform.hybrid_reservations h
    WHERE h.organization_id=o.organization_id AND h.order_id=o.id AND h.status='active' AND h.expires_at>clock_timestamp()+interval '6 seconds'))))) THEN
  RETURN QUERY SELECT true,key;
 ELSE
  PERFORM pg_advisory_unlock(key);
  IF p_channel='telegram' THEN
   UPDATE platform.telegram_outbox SET status='cancelled',capability=NULL,lease_until=NULL WHERE id=item.id AND capability=p_cap;
  END IF;
  RETURN QUERY SELECT false,NULL::bigint;
 END IF;
EXCEPTION WHEN query_canceled OR OTHERS THEN
 IF key IS NOT NULL THEN PERFORM pg_advisory_unlock(key); END IF;
 RAISE;
END $$;
REVOKE ALL ON FUNCTION platform.booking_notification_admit(uuid,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.booking_notification_admit(uuid,text) TO aba_app;
UPDATE platform.service_metadata SET version=27 WHERE singleton;
-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN IF EXISTS(SELECT 1 FROM platform.messages WHERE data ? 'booking_notification')
 OR EXISTS(SELECT 1 FROM platform.telegram_outbox WHERE booking_order_id IS NOT NULL)
 OR EXISTS(SELECT 1 FROM platform.channel_outbox WHERE booking_order_id IS NOT NULL) THEN
 RAISE EXCEPTION 'rollback refused: preserve booking notifications'; END IF; END $$;
DROP TRIGGER enqueue_booking_notification ON platform.agent_executions;
DROP TRIGGER booking_notification_transition ON platform.orders;
DROP TRIGGER telegram_booking_notification_state ON platform.telegram_outbox;
DROP TRIGGER channel_booking_notification_state ON platform.channel_outbox;
DROP FUNCTION platform.booking_notification_admit(uuid,text),platform.booking_notification_transition(),platform.enqueue_booking_notification(),platform.booking_notification_consent(uuid,uuid),platform.audit_booking_notification_state();
ALTER TABLE platform.telegram_outbox DROP COLUMN booking_order_id;
ALTER TABLE platform.channel_outbox DROP COLUMN booking_order_id;
UPDATE platform.service_metadata SET version=26 WHERE singleton;
-- +goose StatementEnd
