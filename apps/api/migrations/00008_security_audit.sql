-- +goose Up
-- +goose StatementBegin
-- Serialize the actual send attempt with disconnect/handoff using a session lock.
-- No SQL transaction remains open during network I/O. Caller MUST pin/release its connection.
CREATE FUNCTION platform.telegram_prepare_send(p_cap uuid,p_worker bytea,p_mode text,p_allowed bigint[]) RETURNS bigint LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE integration uuid; key bigint; valid boolean; BEGIN
 SELECT i.id INTO integration FROM platform.telegram_outbox q JOIN platform.telegram_integrations i ON i.id=q.integration_id WHERE q.capability=p_cap AND q.status='sending' AND i.worker_hash=sha256(p_worker);
 IF integration IS NULL THEN RETURN NULL;END IF;
 key=hashtextextended('telegram-send:'||integration::text,0);
 PERFORM pg_advisory_lock(key);
 SELECT EXISTS(SELECT 1 FROM platform.telegram_outbox q JOIN platform.telegram_integrations i ON i.id=q.integration_id JOIN platform.conversations c ON c.organization_id=q.organization_id AND c.id=q.conversation_id LEFT JOIN platform.messages m ON m.id::text=q.message_key AND m.organization_id=q.organization_id AND m.conversation_id=q.conversation_id
 WHERE q.capability=p_cap AND q.status='sending' AND q.lease_until>clock_timestamp() AND i.active AND i.mode=p_mode AND i.worker_hash=sha256(p_worker) AND EXISTS(SELECT 1 FROM platform.organizations o WHERE o.id=i.organization_id AND o.is_test)
 AND (p_mode='mock' AND p_allowed IS NULL OR q.chat_id=ANY(p_allowed))
 AND (p_mode='mock' OR EXISTS(SELECT 1 FROM platform.ai_settings s WHERE s.organization_id=q.organization_id AND s.telegram_sending_enabled))
 AND c.status='open' AND c.expires_at>clock_timestamp()
 AND (m.role='staff' OR m.data->>'handoff_notice'='true' OR (c.control_mode='agent' AND coalesce((SELECT auto_enabled FROM platform.ai_settings s WHERE s.organization_id=q.organization_id),true)))) INTO valid;
 IF NOT valid THEN
 UPDATE platform.telegram_outbox SET status='cancelled',capability=NULL,lease_until=NULL WHERE capability=p_cap AND status='sending';
 PERFORM pg_advisory_unlock(key);RETURN NULL;
 END IF;
 RETURN key;
EXCEPTION WHEN OTHERS THEN
 -- Prevent session-lock leakage even when a statement timeout is caught.
 IF key IS NOT NULL THEN PERFORM pg_advisory_unlock(key);END IF;
 RAISE;
END $$;
REVOKE ALL ON FUNCTION platform.telegram_prepare_send(uuid,bytea,text,bigint[]) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.telegram_prepare_send(uuid,bytea,text,bigint[]) TO aba_app;
CREATE OR REPLACE FUNCTION platform.telegram_disconnect(p_org uuid) RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE integration uuid; BEGIN
 IF NOT coalesce(p_org::text=current_setting('app.organization_id',true) AND platform.has_membership(p_org,true),false) THEN RAISE EXCEPTION 'forbidden' USING ERRCODE='42501';END IF;
 SELECT id INTO integration FROM platform.telegram_integrations WHERE organization_id=p_org;
 IF integration IS NOT NULL THEN PERFORM pg_advisory_xact_lock(hashtextextended('telegram-send:'||integration::text,0));END IF;
 UPDATE platform.telegram_integrations SET active=false,token_cipher=''::bytea,webhook_secret_cipher=NULL,webhook_state='disabled',updated_at=now() WHERE organization_id=p_org;
 UPDATE platform.telegram_outbox SET status='cancelled',capability=NULL,lease_until=NULL WHERE organization_id=p_org AND status IN ('pending','sending');
 UPDATE platform.telegram_inbox SET status='failed',capability=NULL,lease_until=NULL,last_error='INTEGRATION_DISABLED' WHERE organization_id=p_org AND status IN ('pending','processing');
END $$;
CREATE FUNCTION platform.telegram_control_fence() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE integration uuid; BEGIN
 IF NEW.channel='telegram' AND ((NEW.control_mode='human' AND OLD.control_mode IS DISTINCT FROM NEW.control_mode) OR NEW.status='closed') THEN
 SELECT id INTO integration FROM platform.telegram_integrations WHERE organization_id=NEW.organization_id;
 IF integration IS NOT NULL THEN PERFORM pg_advisory_xact_lock(hashtextextended('telegram-send:'||integration::text,0));END IF;
 UPDATE platform.telegram_outbox q SET status='cancelled',capability=NULL,lease_until=NULL WHERE q.organization_id=NEW.organization_id AND q.conversation_id=NEW.id AND q.status IN ('pending','sending') AND
 (NEW.status='closed' OR EXISTS(SELECT 1 FROM platform.messages m WHERE m.id::text=q.message_key AND m.organization_id=q.organization_id AND m.conversation_id=q.conversation_id AND m.role='assistant' AND coalesce(m.data->>'handoff_notice','false')<>'true'));
 END IF;RETURN NEW;
END $$;
CREATE TRIGGER telegram_control_fence BEFORE UPDATE OF control_mode,status ON platform.conversations FOR EACH ROW EXECUTE FUNCTION platform.telegram_control_fence();
REVOKE ALL ON FUNCTION platform.telegram_control_fence() FROM PUBLIC;
CREATE OR REPLACE FUNCTION platform.telegram_message_outbox() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 DECLARE p platform.telegram_principals; buttons jsonb; body jsonb; facts text; BEGIN
 IF NEW.role NOT IN ('assistant','staff') OR NEW.content='' THEN RETURN NEW;END IF;
 SELECT * INTO p FROM platform.telegram_principals WHERE organization_id=NEW.organization_id AND conversation_id=NEW.conversation_id;
 IF p.id IS NULL THEN RETURN NEW;END IF;
 PERFORM pg_advisory_xact_lock(hashtextextended('telegram-send:'||p.integration_id::text,0));
 IF NOT EXISTS(SELECT 1 FROM platform.telegram_integrations WHERE id=p.integration_id AND active) THEN RETURN NEW;END IF;
 body=jsonb_build_object('text',NEW.content);
 IF jsonb_typeof(NEW.data->'telegram_payload')='object' THEN body=NEW.data->'telegram_payload';END IF;
 IF NEW.role='assistant' AND jsonb_typeof(NEW.data->'cards')='array' THEN
 SELECT string_agg((card->>'name')||' · '||(card->>'sku')||' · '||((card->>'price_amount')::bigint/100)::text||'.'||lpad(((card->>'price_amount')::bigint%100)::text,2,'0')||' RUB · '||CASE WHEN card->>'fresh'='true' THEN 'доступно: '||coalesce(card->>'available','неизвестно') ELSE 'наличие требует подтверждения магазина' END||' · обновление: '||coalesce(card->>'manual_updated_at','неизвестно'),E'\n') INTO facts FROM jsonb_array_elements(NEW.data->'cards') card;
 IF facts IS NOT NULL THEN body=jsonb_build_object('text',left(NEW.content||E'\n'||facts,4000));END IF;
 SELECT jsonb_agg(jsonb_build_array(jsonb_build_object('text','Заявка · '||left(card->>'sku',40),'callback_data','offer:'||(card->>'variant_id')))) INTO buttons FROM jsonb_array_elements(NEW.data->'cards') card;
 IF buttons IS NOT NULL THEN body=body||jsonb_build_object('reply_markup',jsonb_build_object('inline_keyboard',buttons));END IF;
 END IF;
 INSERT INTO platform.telegram_outbox(id,organization_id,integration_id,conversation_id,chat_id,message_key,payload) VALUES(NEW.id,NEW.organization_id,p.integration_id,NEW.conversation_id,p.chat_id,NEW.id::text,body) ON CONFLICT(integration_id,message_key) DO NOTHING;
 RETURN NEW;END $$;
-- Message attribution is immutable; the only channel update is retention erasure.
REVOKE UPDATE ON platform.messages FROM aba_app;
GRANT UPDATE(content,data,request_hash) ON platform.messages TO aba_app;
DROP POLICY telegram_update ON platform.messages;
CREATE POLICY telegram_update ON platform.messages FOR UPDATE TO aba_app
 USING(platform.telegram_scope(organization_id,conversation_id))
 WITH CHECK(platform.telegram_scope(organization_id,conversation_id) AND content='' AND data='{}'::jsonb AND request_hash IS NULL);
UPDATE platform.service_metadata SET version=8 WHERE singleton;
-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN IF EXISTS(SELECT 1 FROM platform.organizations) THEN RAISE EXCEPTION 'refusing security downgrade with organization data';END IF;END $$;
REVOKE UPDATE(content,data,request_hash) ON platform.messages FROM aba_app;
GRANT UPDATE ON platform.messages TO aba_app;
DROP POLICY telegram_update ON platform.messages;
CREATE POLICY telegram_update ON platform.messages FOR UPDATE TO aba_app USING(platform.telegram_scope(organization_id,conversation_id)) WITH CHECK(platform.telegram_scope(organization_id,conversation_id));
DROP TRIGGER telegram_control_fence ON platform.conversations;
DROP FUNCTION platform.telegram_control_fence();
DROP FUNCTION platform.telegram_prepare_send(uuid,bytea,text,bigint[]);
CREATE OR REPLACE FUNCTION platform.telegram_disconnect(p_org uuid) RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ BEGIN
 IF NOT (p_org::text=current_setting('app.organization_id',true) AND platform.has_membership(p_org,true)) THEN RAISE EXCEPTION 'forbidden' USING ERRCODE='42501';END IF;
 UPDATE platform.telegram_integrations SET active=false,token_cipher=''::bytea,updated_at=now() WHERE organization_id=p_org;
 UPDATE platform.telegram_outbox SET status='cancelled',capability=NULL WHERE organization_id=p_org AND status='pending';
 END $$;
CREATE OR REPLACE FUNCTION platform.telegram_message_outbox() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 DECLARE p platform.telegram_principals; buttons jsonb; body jsonb; facts text; BEGIN
 IF NEW.role NOT IN ('assistant','staff') OR NEW.content='' THEN RETURN NEW;END IF;
 SELECT * INTO p FROM platform.telegram_principals WHERE organization_id=NEW.organization_id AND conversation_id=NEW.conversation_id;
 IF p.id IS NULL THEN RETURN NEW;END IF;
 body=jsonb_build_object('text',NEW.content);
 IF jsonb_typeof(NEW.data->'telegram_payload')='object' THEN body=NEW.data->'telegram_payload';END IF;
 IF NEW.role='assistant' AND jsonb_typeof(NEW.data->'cards')='array' THEN
 SELECT string_agg((card->>'name')||' · '||(card->>'sku')||' · '||((card->>'price_amount')::bigint/100)::text||'.'||lpad(((card->>'price_amount')::bigint%100)::text,2,'0')||' RUB · '||CASE WHEN card->>'fresh'='true' THEN 'доступно: '||coalesce(card->>'available','неизвестно') ELSE 'наличие требует подтверждения магазина' END||' · обновление: '||coalesce(card->>'manual_updated_at','неизвестно'),E'\n') INTO facts FROM jsonb_array_elements(NEW.data->'cards') card;
 IF facts IS NOT NULL THEN body=jsonb_build_object('text',left(NEW.content||E'\n'||facts,4000));END IF;
 SELECT jsonb_agg(jsonb_build_array(jsonb_build_object('text','Заявка · '||left(card->>'sku',40),'callback_data','offer:'||(card->>'variant_id')))) INTO buttons FROM jsonb_array_elements(NEW.data->'cards') card;
 IF buttons IS NOT NULL THEN body=body||jsonb_build_object('reply_markup',jsonb_build_object('inline_keyboard',buttons));END IF;
 END IF;
 INSERT INTO platform.telegram_outbox(id,organization_id,integration_id,conversation_id,chat_id,message_key,payload) VALUES(NEW.id,NEW.organization_id,p.integration_id,NEW.conversation_id,p.chat_id,NEW.id::text,body) ON CONFLICT(integration_id,message_key) DO NOTHING;
 RETURN NEW;END $$;
UPDATE platform.service_metadata SET version=7 WHERE singleton;
-- +goose StatementEnd
