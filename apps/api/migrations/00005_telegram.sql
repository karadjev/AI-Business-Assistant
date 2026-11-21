-- +goose Up
-- +goose StatementBegin
CREATE TABLE platform.telegram_integrations(
 id uuid PRIMARY KEY,organization_id uuid NOT NULL UNIQUE REFERENCES platform.organizations(id),
 bot_id bigint NOT NULL UNIQUE CHECK(bot_id>0),name text NOT NULL,username text NOT NULL,
 token_cipher bytea NOT NULL,webhook_hash bytea NOT NULL CHECK(octet_length(webhook_hash)=32),
 last_error text CHECK(last_error IN ('INVALID_BOT_TOKEN','BOT_CHECK_FAILED')),active boolean NOT NULL DEFAULT true,mode text NOT NULL DEFAULT 'mock' CHECK(mode='mock'),
 created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(organization_id,id));
CREATE TABLE platform.telegram_principals(
 id uuid PRIMARY KEY,organization_id uuid NOT NULL,integration_id uuid NOT NULL,
 user_id bigint NOT NULL CHECK(user_id>0),chat_id bigint NOT NULL CHECK(chat_id=user_id),
 conversation_id uuid NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(integration_id,user_id,chat_id),UNIQUE(organization_id,id),
 FOREIGN KEY(organization_id,integration_id) REFERENCES platform.telegram_integrations(organization_id,id),
 FOREIGN KEY(organization_id,conversation_id) REFERENCES platform.conversations(organization_id,id));
CREATE TABLE platform.telegram_inbox(
 id uuid PRIMARY KEY,organization_id uuid NOT NULL,integration_id uuid NOT NULL,update_id bigint NOT NULL CHECK(update_id>=0),
 chat_id bigint NOT NULL CHECK(chat_id>0),user_id bigint NOT NULL CHECK(user_id=chat_id),payload jsonb NOT NULL CHECK(octet_length(payload::text)<=16384),
 status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','processing','done','failed')),
 attempts integer NOT NULL DEFAULT 0,available_at timestamptz NOT NULL DEFAULT now(),lease_until timestamptz,
 capability uuid,conversation_id uuid,last_error text,created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(integration_id,update_id),FOREIGN KEY(organization_id,integration_id) REFERENCES platform.telegram_integrations(organization_id,id),
 FOREIGN KEY(organization_id,conversation_id) REFERENCES platform.conversations(organization_id,id));
CREATE INDEX telegram_inbox_pending ON platform.telegram_inbox(status,available_at,created_at);
CREATE TABLE platform.telegram_outbox(
 id uuid PRIMARY KEY,organization_id uuid NOT NULL,integration_id uuid NOT NULL,conversation_id uuid NOT NULL,
 chat_id bigint NOT NULL,message_key text NOT NULL,payload jsonb NOT NULL CHECK(octet_length(payload::text)<=16384),
 status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','sending','sent','failed','uncertain','cancelled')),
 attempts integer NOT NULL DEFAULT 0,available_at timestamptz NOT NULL DEFAULT now(),lease_until timestamptz,capability uuid,
 last_error text,telegram_message_id bigint,created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(integration_id,message_key),FOREIGN KEY(organization_id,integration_id) REFERENCES platform.telegram_integrations(organization_id,id),
 FOREIGN KEY(organization_id,conversation_id) REFERENCES platform.conversations(organization_id,id));
CREATE INDEX telegram_outbox_pending ON platform.telegram_outbox(status,available_at,created_at);
ALTER TABLE platform.conversations DROP CONSTRAINT conversations_channel_check;
ALTER TABLE platform.conversations ADD CHECK(channel IN ('internal_chat','telegram'));
ALTER TABLE platform.conversations ALTER COLUMN created_by DROP NOT NULL;
ALTER TABLE platform.conversation_events ALTER COLUMN actor_id DROP NOT NULL;
ALTER TABLE platform.order_events ALTER COLUMN actor_id DROP NOT NULL;
ALTER TABLE platform.orders DROP CONSTRAINT orders_source_check;
ALTER TABLE platform.orders ADD CHECK(source IN ('internal_chat','telegram'));
CREATE FUNCTION platform.telegram_scope(p_org uuid,p_conversation uuid) RETURNS boolean LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT EXISTS(SELECT 1 FROM platform.telegram_inbox q JOIN platform.telegram_integrations i ON i.id=q.integration_id JOIN platform.organizations o ON o.id=q.organization_id
 WHERE q.capability::text=current_setting('app.telegram_cap',true) AND q.status='processing' AND q.lease_until>now()
 AND q.organization_id=p_org AND p_org::text=current_setting('app.organization_id',true) AND i.active AND o.is_test
 AND (p_conversation IS NULL OR p_conversation=q.conversation_id)) $$;
CREATE FUNCTION platform.telegram_conversation() RETURNS uuid LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT q.conversation_id FROM platform.telegram_inbox q WHERE q.capability::text=current_setting('app.telegram_cap',true)
 AND platform.telegram_scope(q.organization_id,q.conversation_id) $$;
CREATE FUNCTION platform.telegram_ingress(p_id uuid,p_secret bytea,p_event uuid,p_update bigint,p_user bigint,p_chat bigint,p_payload jsonb) RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 DECLARE org uuid; BEGIN
 SELECT i.organization_id INTO org FROM platform.telegram_integrations i JOIN platform.organizations o ON o.id=i.organization_id WHERE i.id=p_id AND i.active AND o.is_test AND i.webhook_hash=p_secret;
 IF org IS NULL OR p_user<=0 OR p_chat<>p_user OR p_update<0 OR octet_length(p_payload::text)>16384 THEN RAISE EXCEPTION 'invalid ingress' USING ERRCODE='42501'; END IF;
 PERFORM pg_advisory_xact_lock(hashtextextended(p_id::text,0));
 IF EXISTS(SELECT 1 FROM platform.telegram_inbox WHERE integration_id=p_id AND update_id=p_update) THEN RETURN false;END IF;
 IF (SELECT count(*) FROM platform.telegram_inbox WHERE integration_id=p_id AND created_at>now()-interval '1 minute')>=120 THEN RAISE EXCEPTION 'ingress quota' USING ERRCODE='54000';END IF;
 INSERT INTO platform.telegram_inbox(id,organization_id,integration_id,update_id,user_id,chat_id,payload) VALUES(p_event,org,p_id,p_update,p_user,p_chat,p_payload);RETURN true;END $$;
CREATE FUNCTION platform.telegram_claim(p_cap uuid,p_conv uuid,p_customer uuid,p_principal uuid,p_days integer) RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 DECLARE q platform.telegram_inbox; conv uuid; BEGIN
 UPDATE platform.telegram_inbox SET status='failed',capability=NULL,last_error='RETRY_EXHAUSTED' WHERE status='processing' AND lease_until<now() AND attempts>=5;
 IF p_days<1 OR p_days>30 THEN RAISE EXCEPTION 'retention' USING ERRCODE='22003'; END IF;
 SELECT x.* INTO q FROM platform.telegram_inbox x JOIN platform.telegram_integrations i ON i.id=x.integration_id
 WHERE i.active AND x.attempts<5 AND ((x.status='pending' AND x.available_at<=now()) OR (x.status='processing' AND x.lease_until<now()))
 AND NOT EXISTS(SELECT 1 FROM platform.telegram_inbox older WHERE older.integration_id=x.integration_id AND older.chat_id=x.chat_id AND older.status IN ('pending','processing') AND (older.created_at,older.id)<(x.created_at,x.id))
 ORDER BY x.created_at,x.id LIMIT 1 FOR UPDATE OF x SKIP LOCKED;
 IF q.id IS NULL THEN RETURN NULL;END IF;
 SELECT conversation_id INTO conv FROM platform.telegram_principals WHERE integration_id=q.integration_id AND chat_id=q.chat_id AND user_id=q.user_id;
 IF conv IS NULL THEN
 conv=p_conv;
 INSERT INTO platform.conversations(id,organization_id,customer_id,created_by,channel,expires_at) VALUES(conv,q.organization_id,p_customer,NULL,'telegram',now()+make_interval(days=>p_days));
 INSERT INTO platform.telegram_principals(id,organization_id,integration_id,user_id,chat_id,conversation_id) VALUES(p_principal,q.organization_id,q.integration_id,q.user_id,q.chat_id,conv);
 INSERT INTO platform.conversation_events(id,organization_id,conversation_id,kind) VALUES(p_principal,q.organization_id,conv,'created');
 END IF;
 UPDATE platform.telegram_inbox SET status='processing',attempts=attempts+1,lease_until=now()+interval '30 seconds',capability=p_cap,conversation_id=conv WHERE id=q.id;
 RETURN jsonb_build_object('id',q.id,'organization_id',q.organization_id,'integration_id',q.integration_id,'conversation_id',conv,'capability',p_cap,'payload',q.payload,'chat_id',q.chat_id,'attempts',q.attempts+1);
 END $$;
CREATE FUNCTION platform.telegram_finish(p_cap uuid,p_error text) RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ BEGIN
 UPDATE platform.telegram_inbox SET status=CASE WHEN p_error IS NULL THEN 'done' WHEN attempts>=5 THEN 'failed' ELSE 'pending' END,
 available_at=now()+make_interval(secs=>least(300,power(2,attempts)::integer)),last_error=CASE WHEN p_error IS NULL THEN NULL ELSE 'PROCESSING_UNAVAILABLE' END,lease_until=NULL,capability=NULL
 WHERE capability=p_cap AND status='processing' AND lease_until>now(); END $$;
CREATE FUNCTION platform.telegram_claim_send(p_cap uuid) RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 DECLARE q platform.telegram_outbox; cipher bytea; BEGIN
 UPDATE platform.telegram_outbox SET status='uncertain',last_error='DELIVERY_UNKNOWN',capability=NULL WHERE status='sending' AND lease_until<now();
 SELECT x.* INTO q FROM platform.telegram_outbox x JOIN platform.telegram_integrations i ON i.id=x.integration_id
 WHERE i.active AND x.status='pending' AND x.available_at<=now() AND x.attempts<5
 AND NOT EXISTS(SELECT 1 FROM platform.telegram_outbox older WHERE older.integration_id=x.integration_id AND older.chat_id=x.chat_id AND older.status IN ('pending','sending','uncertain') AND (older.created_at,older.id)<(x.created_at,x.id))
 ORDER BY x.created_at,x.id LIMIT 1 FOR UPDATE OF x SKIP LOCKED;
 IF q.id IS NULL THEN RETURN NULL;END IF;
 SELECT token_cipher INTO cipher FROM platform.telegram_integrations WHERE id=q.integration_id;
 UPDATE platform.telegram_outbox SET status='sending',attempts=attempts+1,lease_until=now()+interval '15 seconds',capability=p_cap WHERE id=q.id;
 RETURN jsonb_build_object('id',q.id,'attempts',q.attempts+1,'organization_id',q.organization_id,'capability',p_cap,'chat_id',q.chat_id,'payload',q.payload,'token_cipher',encode(cipher,'hex'));
 END $$;
CREATE FUNCTION platform.telegram_finish_send(p_cap uuid,p_state text,p_delay integer,p_message bigint) RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ BEGIN
 IF p_state NOT IN ('sent','pending','failed','uncertain') OR p_delay<0 OR p_delay>3600 THEN RAISE EXCEPTION 'invalid send result'; END IF;
 UPDATE platform.telegram_outbox SET status=CASE WHEN p_state='pending' AND attempts>=5 THEN 'failed' ELSE p_state END,
 available_at=now()+make_interval(secs=>p_delay),last_error=CASE WHEN p_state='sent' THEN NULL WHEN p_state='uncertain' THEN 'DELIVERY_UNKNOWN' ELSE 'SEND_UNAVAILABLE' END,telegram_message_id=p_message,capability=NULL,lease_until=NULL WHERE capability=p_cap AND status='sending'; END $$;
CREATE FUNCTION platform.lock_order_inventory(p_ids uuid[]) RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ BEGIN
 IF NOT (platform.can_operate_orders(nullif(current_setting('app.organization_id',true),'')::uuid) OR platform.telegram_scope(nullif(current_setting('app.organization_id',true),'')::uuid,NULL)) THEN RAISE EXCEPTION 'forbidden' USING ERRCODE='42501';END IF;
 PERFORM 1 FROM platform.inventory WHERE organization_id::text=current_setting('app.organization_id',true) AND variant_id=ANY(p_ids) ORDER BY variant_id FOR UPDATE;
 END $$;
CREATE OR REPLACE FUNCTION platform.lock_order_products(p_ids uuid[]) RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ BEGIN
 IF NOT ((platform.is_test_org(nullif(current_setting('app.organization_id',true),'')::uuid) AND platform.can_operate_orders(nullif(current_setting('app.organization_id',true),'')::uuid)) OR platform.telegram_scope(nullif(current_setting('app.organization_id',true),'')::uuid,NULL)) THEN RAISE EXCEPTION 'forbidden' USING ERRCODE='42501';END IF;
 PERFORM 1 FROM platform.products WHERE organization_id::text=current_setting('app.organization_id',true) AND id=ANY(p_ids) ORDER BY id FOR UPDATE;
 END $$;
ALTER TABLE platform.telegram_integrations ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.telegram_integrations FORCE ROW LEVEL SECURITY;
CREATE POLICY migration_owner ON platform.telegram_integrations TO aba_migrator USING(true) WITH CHECK(true);
CREATE POLICY staff_read ON platform.telegram_integrations FOR SELECT TO aba_app USING(organization_id::text=current_setting('app.organization_id',true) AND platform.can_operate_orders(organization_id));
ALTER TABLE platform.telegram_principals ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.telegram_principals FORCE ROW LEVEL SECURITY;
CREATE POLICY migration_owner ON platform.telegram_principals TO aba_migrator USING(true) WITH CHECK(true);
CREATE POLICY staff_read ON platform.telegram_principals FOR SELECT TO aba_app USING(organization_id::text=current_setting('app.organization_id',true) AND platform.can_operate_orders(organization_id));
ALTER TABLE platform.telegram_inbox ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.telegram_inbox FORCE ROW LEVEL SECURITY;
CREATE POLICY migration_owner ON platform.telegram_inbox TO aba_migrator USING(true) WITH CHECK(true);
CREATE POLICY staff_read ON platform.telegram_inbox FOR SELECT TO aba_app USING(organization_id::text=current_setting('app.organization_id',true) AND platform.can_operate_orders(organization_id));
ALTER TABLE platform.telegram_outbox ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.telegram_outbox FORCE ROW LEVEL SECURITY;
CREATE POLICY migration_owner ON platform.telegram_outbox TO aba_migrator USING(true) WITH CHECK(true);
CREATE POLICY staff_read ON platform.telegram_outbox FOR SELECT TO aba_app USING(organization_id::text=current_setting('app.organization_id',true) AND platform.can_operate_orders(organization_id));
GRANT SELECT(id,organization_id,bot_id,name,username,active,mode,last_error,created_at,updated_at),INSERT,UPDATE ON platform.telegram_integrations TO aba_app;
CREATE POLICY owner_insert ON platform.telegram_integrations FOR INSERT TO aba_app WITH CHECK(platform.has_membership(organization_id,true) AND organization_id::text=current_setting('app.organization_id',true) AND platform.is_test_org(organization_id));
CREATE POLICY owner_update ON platform.telegram_integrations FOR UPDATE TO aba_app USING(platform.has_membership(organization_id,true) AND organization_id::text=current_setting('app.organization_id',true)) WITH CHECK(platform.has_membership(organization_id,true) AND organization_id::text=current_setting('app.organization_id',true));
GRANT SELECT ON platform.telegram_principals,platform.telegram_inbox TO aba_app;
GRANT SELECT,INSERT ON platform.telegram_outbox TO aba_app;
CREATE POLICY channel_outbox_read ON platform.telegram_outbox FOR SELECT TO aba_app USING(platform.telegram_scope(organization_id,conversation_id));
CREATE POLICY channel_outbox ON platform.telegram_outbox FOR INSERT TO aba_app WITH CHECK(platform.telegram_scope(organization_id,conversation_id) AND EXISTS(SELECT 1 FROM platform.telegram_principals p WHERE p.organization_id=telegram_outbox.organization_id AND p.integration_id=telegram_outbox.integration_id AND p.conversation_id=telegram_outbox.conversation_id AND p.chat_id=telegram_outbox.chat_id));
CREATE POLICY staff_outbox ON platform.telegram_outbox FOR INSERT TO aba_app WITH CHECK(organization_id::text=current_setting('app.organization_id',true) AND platform.can_operate_orders(organization_id) AND EXISTS(SELECT 1 FROM platform.telegram_principals p WHERE p.organization_id=telegram_outbox.organization_id AND p.integration_id=telegram_outbox.integration_id AND p.conversation_id=telegram_outbox.conversation_id AND p.chat_id=telegram_outbox.chat_id));
CREATE POLICY channel_principal ON platform.telegram_principals FOR SELECT TO aba_app USING(platform.telegram_scope(organization_id,conversation_id));
CREATE POLICY telegram_read ON platform.products FOR SELECT TO aba_app USING(platform.telegram_scope(organization_id,NULL) AND status='active');
CREATE POLICY telegram_read ON platform.product_variants FOR SELECT TO aba_app USING(platform.telegram_scope(organization_id,NULL) AND EXISTS(SELECT 1 FROM platform.products p WHERE p.organization_id=product_variants.organization_id AND p.id=product_variants.product_id AND p.status='active'));
CREATE POLICY telegram_read ON platform.inventory FOR SELECT TO aba_app USING(platform.telegram_scope(organization_id,NULL) AND EXISTS(SELECT 1 FROM platform.product_variants v WHERE v.organization_id=inventory.organization_id AND v.id=inventory.variant_id));
CREATE POLICY telegram_read ON platform.conversations FOR SELECT TO aba_app USING(platform.telegram_scope(organization_id,id));
CREATE POLICY telegram_update ON platform.conversations FOR UPDATE TO aba_app USING(platform.telegram_scope(organization_id,id)) WITH CHECK(platform.telegram_scope(organization_id,id) AND channel='telegram' AND created_by IS NULL);
CREATE POLICY telegram_read ON platform.messages FOR SELECT TO aba_app USING(platform.telegram_scope(organization_id,conversation_id));
CREATE POLICY telegram_insert ON platform.messages FOR INSERT TO aba_app WITH CHECK(platform.telegram_scope(organization_id,conversation_id) AND (role IN ('customer','assistant','system','tool')) AND actor_id IS NULL);
CREATE POLICY telegram_update ON platform.messages FOR UPDATE TO aba_app USING(platform.telegram_scope(organization_id,conversation_id)) WITH CHECK(platform.telegram_scope(organization_id,conversation_id));
CREATE POLICY telegram_read ON platform.conversation_events FOR SELECT TO aba_app USING(platform.telegram_scope(organization_id,conversation_id));
CREATE POLICY telegram_insert ON platform.conversation_events FOR INSERT TO aba_app WITH CHECK(platform.telegram_scope(organization_id,conversation_id) AND actor_id IS NULL);
CREATE POLICY telegram_read ON platform.order_quotes FOR SELECT TO aba_app USING(platform.telegram_scope(organization_id,conversation_id));
CREATE POLICY telegram_insert ON platform.order_quotes FOR INSERT TO aba_app WITH CHECK(platform.telegram_scope(organization_id,conversation_id));
CREATE POLICY telegram_update ON platform.order_quotes FOR UPDATE TO aba_app USING(platform.telegram_scope(organization_id,conversation_id)) WITH CHECK(platform.telegram_scope(organization_id,conversation_id));
CREATE POLICY telegram_read ON platform.orders FOR SELECT TO aba_app USING(platform.telegram_scope(organization_id,conversation_id));
CREATE POLICY telegram_insert ON platform.orders FOR INSERT TO aba_app WITH CHECK(platform.telegram_scope(organization_id,conversation_id) AND status='pending_confirmation' AND confirmed_by IS NULL AND source='telegram');
CREATE POLICY telegram_update ON platform.orders FOR UPDATE TO aba_app USING(platform.telegram_scope(organization_id,conversation_id) AND status='pending_confirmation') WITH CHECK(platform.telegram_scope(organization_id,conversation_id) AND status='pending_confirmation' AND confirmed_by IS NULL);
CREATE POLICY telegram_read ON platform.order_items FOR SELECT TO aba_app USING(EXISTS(SELECT 1 FROM platform.orders o WHERE o.organization_id=order_items.organization_id AND o.id=order_items.order_id AND platform.telegram_scope(o.organization_id,o.conversation_id)));
CREATE POLICY telegram_insert ON platform.order_items FOR INSERT TO aba_app WITH CHECK(EXISTS(SELECT 1 FROM platform.orders o WHERE o.organization_id=order_items.organization_id AND o.id=order_items.order_id AND platform.telegram_scope(o.organization_id,o.conversation_id)));
CREATE POLICY telegram_read ON platform.order_events FOR SELECT TO aba_app USING(EXISTS(SELECT 1 FROM platform.orders o WHERE o.organization_id=order_events.organization_id AND o.id=order_events.order_id AND platform.telegram_scope(o.organization_id,o.conversation_id)));
CREATE POLICY telegram_insert ON platform.order_events FOR INSERT TO aba_app WITH CHECK(EXISTS(SELECT 1 FROM platform.orders o WHERE o.organization_id=order_events.organization_id AND o.id=order_events.order_id AND platform.telegram_scope(o.organization_id,o.conversation_id)) AND kind='created' AND actor_id IS NULL);
CREATE POLICY telegram_select ON platform.ai_usage FOR SELECT TO aba_app USING(platform.telegram_scope(organization_id,NULL)) ;
CREATE POLICY telegram_insert ON platform.ai_usage FOR INSERT TO aba_app WITH CHECK(platform.telegram_scope(organization_id,NULL));
CREATE POLICY telegram_update ON platform.ai_usage FOR UPDATE TO aba_app USING(platform.telegram_scope(organization_id,NULL)) WITH CHECK(platform.telegram_scope(organization_id,NULL));
CREATE FUNCTION platform.telegram_order_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,platform,pg_temp AS $$ BEGIN
 IF platform.telegram_scope(NEW.organization_id,NEW.conversation_id) THEN
 IF TG_OP='UPDATE' THEN RAISE EXCEPTION 'channel cannot update orders' USING ERRCODE='42501';END IF;
 NEW.source='telegram';END IF;RETURN NEW;END $$;
CREATE TRIGGER telegram_order_source BEFORE INSERT OR UPDATE ON platform.orders FOR EACH ROW EXECUTE FUNCTION platform.telegram_order_source();
REVOKE ALL ON FUNCTION platform.telegram_scope(uuid,uuid) FROM PUBLIC;GRANT EXECUTE ON FUNCTION platform.telegram_scope(uuid,uuid) TO aba_app;
REVOKE ALL ON FUNCTION platform.telegram_conversation() FROM PUBLIC;GRANT EXECUTE ON FUNCTION platform.telegram_conversation() TO aba_app;
REVOKE ALL ON FUNCTION platform.telegram_ingress(uuid,bytea,uuid,bigint,bigint,bigint,jsonb) FROM PUBLIC;GRANT EXECUTE ON FUNCTION platform.telegram_ingress(uuid,bytea,uuid,bigint,bigint,bigint,jsonb) TO aba_app;
REVOKE ALL ON FUNCTION platform.telegram_claim(uuid,uuid,uuid,uuid,integer) FROM PUBLIC;GRANT EXECUTE ON FUNCTION platform.telegram_claim(uuid,uuid,uuid,uuid,integer) TO aba_app;
REVOKE ALL ON FUNCTION platform.telegram_finish(uuid,text) FROM PUBLIC;GRANT EXECUTE ON FUNCTION platform.telegram_finish(uuid,text) TO aba_app;
REVOKE ALL ON FUNCTION platform.telegram_claim_send(uuid) FROM PUBLIC;GRANT EXECUTE ON FUNCTION platform.telegram_claim_send(uuid) TO aba_app;
REVOKE ALL ON FUNCTION platform.telegram_finish_send(uuid,text,integer,bigint) FROM PUBLIC;GRANT EXECUTE ON FUNCTION platform.telegram_finish_send(uuid,text,integer,bigint) TO aba_app;
REVOKE ALL ON FUNCTION platform.lock_order_inventory(uuid[]) FROM PUBLIC;GRANT EXECUTE ON FUNCTION platform.lock_order_inventory(uuid[]) TO aba_app;
CREATE POLICY owner_inject ON platform.telegram_inbox FOR INSERT TO aba_app WITH CHECK(organization_id::text=current_setting('app.organization_id',true) AND platform.has_membership(organization_id,true) AND platform.is_test_org(organization_id));
GRANT INSERT ON platform.telegram_inbox TO aba_app;
CREATE FUNCTION platform.telegram_message_outbox() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
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
CREATE TRIGGER telegram_message_outbox AFTER INSERT ON platform.messages FOR EACH ROW EXECUTE FUNCTION platform.telegram_message_outbox();
REVOKE ALL ON FUNCTION platform.telegram_message_outbox() FROM PUBLIC;
CREATE FUNCTION platform.telegram_webhook_hash(p_id uuid) RETURNS bytea LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ SELECT webhook_hash FROM platform.telegram_integrations i JOIN platform.organizations o ON o.id=i.organization_id WHERE i.id=p_id AND i.active AND o.is_test $$;
REVOKE ALL ON FUNCTION platform.telegram_webhook_hash(uuid) FROM PUBLIC;GRANT EXECUTE ON FUNCTION platform.telegram_webhook_hash(uuid) TO aba_app;
CREATE FUNCTION platform.telegram_connect(p_id uuid,p_org uuid,p_bot bigint,p_name text,p_username text,p_cipher bytea,p_hash bytea) RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ BEGIN
 IF NOT (p_org::text=current_setting('app.organization_id',true) AND platform.has_membership(p_org,true) AND platform.is_test_org(p_org)) THEN RAISE EXCEPTION 'forbidden' USING ERRCODE='42501';END IF;
 PERFORM pg_advisory_xact_lock(hashtextextended('telegram-config:'||p_org::text,0));
 IF EXISTS(SELECT 1 FROM platform.telegram_integrations WHERE organization_id=p_org AND bot_id<>p_bot) THEN RAISE EXCEPTION 'bot replacement' USING ERRCODE='23505';END IF;
 INSERT INTO platform.telegram_integrations(id,organization_id,bot_id,name,username,token_cipher,webhook_hash) VALUES(p_id,p_org,p_bot,p_name,p_username,p_cipher,p_hash) ON CONFLICT(organization_id) DO UPDATE SET active=true,last_error=NULL,token_cipher=excluded.token_cipher,webhook_hash=excluded.webhook_hash,updated_at=now();END $$;
CREATE FUNCTION platform.telegram_disconnect(p_org uuid) RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ BEGIN
 IF NOT (p_org::text=current_setting('app.organization_id',true) AND platform.has_membership(p_org,true)) THEN RAISE EXCEPTION 'forbidden' USING ERRCODE='42501';END IF;
 UPDATE platform.telegram_integrations SET active=false,token_cipher=''::bytea,updated_at=now() WHERE organization_id=p_org;
 UPDATE platform.telegram_outbox SET status='cancelled',capability=NULL WHERE organization_id=p_org AND status='pending';
 END $$;
REVOKE INSERT,UPDATE ON platform.telegram_integrations FROM aba_app;
REVOKE ALL ON FUNCTION platform.telegram_connect(uuid,uuid,bigint,text,text,bytea,bytea),platform.telegram_disconnect(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.telegram_connect(uuid,uuid,bigint,text,text,bytea,bytea),platform.telegram_disconnect(uuid) TO aba_app;
CREATE FUNCTION platform.telegram_erase_payload() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ BEGIN
 IF NEW.channel='telegram' AND NEW.status='closed' THEN
 UPDATE platform.telegram_inbox SET payload='{}',status=CASE WHEN status='done' THEN status ELSE 'failed' END,capability=NULL,lease_until=NULL WHERE organization_id=NEW.organization_id AND conversation_id=NEW.id;
 UPDATE platform.telegram_outbox SET payload='{}',status=CASE WHEN status='sent' THEN status ELSE 'cancelled' END,capability=NULL,lease_until=NULL WHERE organization_id=NEW.organization_id AND conversation_id=NEW.id;
 END IF;RETURN NEW;END $$;
CREATE TRIGGER telegram_erase_payload AFTER UPDATE OF status ON platform.conversations FOR EACH ROW EXECUTE FUNCTION platform.telegram_erase_payload();
REVOKE ALL ON FUNCTION platform.telegram_erase_payload() FROM PUBLIC;
CREATE FUNCTION platform.telegram_purge_expired() RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ BEGIN
 UPDATE platform.messages m SET content='',data='{}',request_hash=NULL WHERE EXISTS(SELECT 1 FROM platform.conversations c WHERE c.organization_id=m.organization_id AND c.id=m.conversation_id AND c.channel='telegram' AND c.expires_at<=now() AND c.status='open');
 UPDATE platform.conversations SET status='closed',control_mode='human',version=version+1,in_flight=NULL,lease_until=NULL WHERE channel='telegram' AND expires_at<=now() AND status='open';
 END $$;
REVOKE ALL ON FUNCTION platform.telegram_purge_expired() FROM PUBLIC;GRANT EXECUTE ON FUNCTION platform.telegram_purge_expired() TO aba_app;
CREATE TABLE platform.telegram_events(id uuid PRIMARY KEY,organization_id uuid NOT NULL REFERENCES platform.organizations(id),integration_id uuid NOT NULL,actor_id uuid NOT NULL REFERENCES platform.users(id),kind text NOT NULL CHECK(kind IN ('connected','checked','disconnected')),created_at timestamptz NOT NULL DEFAULT now(),FOREIGN KEY(organization_id,integration_id) REFERENCES platform.telegram_integrations(organization_id,id));
ALTER TABLE platform.telegram_events ENABLE ROW LEVEL SECURITY;ALTER TABLE platform.telegram_events FORCE ROW LEVEL SECURITY;
CREATE POLICY migration_owner ON platform.telegram_events TO aba_migrator USING(true) WITH CHECK(true);
CREATE POLICY owner_read ON platform.telegram_events FOR SELECT TO aba_app USING(organization_id::text=current_setting('app.organization_id',true) AND platform.has_membership(organization_id,true));
CREATE POLICY owner_insert ON platform.telegram_events FOR INSERT TO aba_app WITH CHECK(organization_id::text=current_setting('app.organization_id',true) AND platform.has_membership(organization_id,true) AND actor_id=platform.current_user_id());
GRANT SELECT,INSERT ON platform.telegram_events TO aba_app;
CREATE FUNCTION platform.telegram_owner_cipher(p_org uuid) RETURNS bytea LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ SELECT token_cipher FROM platform.telegram_integrations WHERE organization_id=p_org AND active AND p_org::text=current_setting('app.organization_id',true) AND platform.has_membership(p_org,true) $$;
REVOKE ALL ON FUNCTION platform.telegram_owner_cipher(uuid) FROM PUBLIC;GRANT EXECUTE ON FUNCTION platform.telegram_owner_cipher(uuid) TO aba_app;
CREATE FUNCTION platform.telegram_connection_error(p_org uuid,p_code text) RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ BEGIN
 IF NOT (p_org::text=current_setting('app.organization_id',true) AND platform.has_membership(p_org,true)) THEN RAISE EXCEPTION 'forbidden' USING ERRCODE='42501';END IF;
 IF p_code IS NOT NULL AND p_code NOT IN ('INVALID_BOT_TOKEN','BOT_CHECK_FAILED') THEN RAISE EXCEPTION 'invalid error';END IF;
 UPDATE platform.telegram_integrations SET last_error=p_code WHERE organization_id=p_org;END $$;
REVOKE ALL ON FUNCTION platform.telegram_connection_error(uuid,text) FROM PUBLIC;GRANT EXECUTE ON FUNCTION platform.telegram_connection_error(uuid,text) TO aba_app;
UPDATE platform.service_metadata SET version=5 WHERE singleton=true;
-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN IF EXISTS(SELECT 1 FROM platform.telegram_integrations) THEN RAISE EXCEPTION 'Telegram data exists: rollback refused';END IF; END $$;
DROP TRIGGER telegram_erase_payload ON platform.conversations;
DROP FUNCTION platform.telegram_erase_payload();
DROP FUNCTION platform.telegram_purge_expired();
DROP TRIGGER telegram_message_outbox ON platform.messages;
DROP FUNCTION platform.telegram_message_outbox();
DROP TRIGGER telegram_order_source ON platform.orders;
DROP FUNCTION platform.telegram_order_source();
DROP POLICY IF EXISTS telegram_read ON platform.products;
DROP POLICY IF EXISTS telegram_insert ON platform.products;
DROP POLICY IF EXISTS telegram_update ON platform.products;
DROP POLICY IF EXISTS telegram_select ON platform.products;
DROP POLICY IF EXISTS telegram_read ON platform.product_variants;
DROP POLICY IF EXISTS telegram_insert ON platform.product_variants;
DROP POLICY IF EXISTS telegram_update ON platform.product_variants;
DROP POLICY IF EXISTS telegram_select ON platform.product_variants;
DROP POLICY IF EXISTS telegram_read ON platform.inventory;
DROP POLICY IF EXISTS telegram_insert ON platform.inventory;
DROP POLICY IF EXISTS telegram_update ON platform.inventory;
DROP POLICY IF EXISTS telegram_select ON platform.inventory;
DROP POLICY IF EXISTS telegram_read ON platform.conversations;
DROP POLICY IF EXISTS telegram_insert ON platform.conversations;
DROP POLICY IF EXISTS telegram_update ON platform.conversations;
DROP POLICY IF EXISTS telegram_select ON platform.conversations;
DROP POLICY IF EXISTS telegram_read ON platform.messages;
DROP POLICY IF EXISTS telegram_insert ON platform.messages;
DROP POLICY IF EXISTS telegram_update ON platform.messages;
DROP POLICY IF EXISTS telegram_select ON platform.messages;
DROP POLICY IF EXISTS telegram_read ON platform.conversation_events;
DROP POLICY IF EXISTS telegram_insert ON platform.conversation_events;
DROP POLICY IF EXISTS telegram_update ON platform.conversation_events;
DROP POLICY IF EXISTS telegram_select ON platform.conversation_events;
DROP POLICY IF EXISTS telegram_read ON platform.order_quotes;
DROP POLICY IF EXISTS telegram_insert ON platform.order_quotes;
DROP POLICY IF EXISTS telegram_update ON platform.order_quotes;
DROP POLICY IF EXISTS telegram_select ON platform.order_quotes;
DROP POLICY IF EXISTS telegram_read ON platform.orders;
DROP POLICY IF EXISTS telegram_insert ON platform.orders;
DROP POLICY IF EXISTS telegram_update ON platform.orders;
DROP POLICY IF EXISTS telegram_select ON platform.orders;
DROP POLICY IF EXISTS telegram_read ON platform.order_items;
DROP POLICY IF EXISTS telegram_insert ON platform.order_items;
DROP POLICY IF EXISTS telegram_update ON platform.order_items;
DROP POLICY IF EXISTS telegram_select ON platform.order_items;
DROP POLICY IF EXISTS telegram_read ON platform.order_events;
DROP POLICY IF EXISTS telegram_insert ON platform.order_events;
DROP POLICY IF EXISTS telegram_update ON platform.order_events;
DROP POLICY IF EXISTS telegram_select ON platform.order_events;
DROP POLICY IF EXISTS telegram_read ON platform.ai_usage;
DROP POLICY IF EXISTS telegram_insert ON platform.ai_usage;
DROP POLICY IF EXISTS telegram_update ON platform.ai_usage;
DROP POLICY IF EXISTS telegram_select ON platform.ai_usage;
DROP FUNCTION platform.telegram_connection_error(uuid,text);
DROP TABLE platform.telegram_events;
DROP FUNCTION platform.telegram_owner_cipher(uuid);
DROP FUNCTION platform.telegram_connect(uuid,uuid,bigint,text,text,bytea,bytea);
DROP FUNCTION platform.telegram_disconnect(uuid);
DROP FUNCTION platform.telegram_webhook_hash(uuid);
DROP TABLE platform.telegram_outbox,platform.telegram_principals,platform.telegram_inbox,platform.telegram_integrations;
DROP FUNCTION platform.telegram_conversation();
DROP FUNCTION platform.telegram_scope(uuid,uuid);
DROP FUNCTION platform.telegram_ingress(uuid,bytea,uuid,bigint,bigint,bigint,jsonb);
DROP FUNCTION platform.telegram_claim(uuid,uuid,uuid,uuid,integer);
DROP FUNCTION platform.telegram_finish(uuid,text);
DROP FUNCTION platform.telegram_claim_send(uuid);
DROP FUNCTION platform.telegram_finish_send(uuid,text,integer,bigint);
DROP FUNCTION platform.lock_order_inventory(uuid[]);
CREATE OR REPLACE FUNCTION platform.lock_order_products(p_ids uuid[]) RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ BEGIN
 IF NOT (platform.is_test_org(nullif(current_setting('app.organization_id',true),'')::uuid) AND platform.can_operate_orders(nullif(current_setting('app.organization_id',true),'')::uuid)) THEN RAISE EXCEPTION 'forbidden' USING ERRCODE='42501';END IF;
 PERFORM 1 FROM platform.products WHERE organization_id::text=current_setting('app.organization_id',true) AND id=ANY(p_ids) ORDER BY id FOR UPDATE;END $$;
ALTER TABLE platform.conversations DROP CONSTRAINT conversations_channel_check;
ALTER TABLE platform.conversations ADD CHECK(channel='internal_chat');
ALTER TABLE platform.orders DROP CONSTRAINT orders_source_check;
ALTER TABLE platform.orders ADD CHECK(source='internal_chat');
ALTER TABLE platform.conversations ALTER COLUMN created_by SET NOT NULL;
ALTER TABLE platform.conversation_events ALTER COLUMN actor_id SET NOT NULL;
ALTER TABLE platform.order_events ALTER COLUMN actor_id SET NOT NULL;
UPDATE platform.service_metadata SET version=4 WHERE singleton=true;
-- +goose StatementEnd
