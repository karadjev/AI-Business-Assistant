-- +goose Up
-- +goose StatementBegin
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';
-- No new queue: Instagram shares channel_inbox/outbox. Telegram's fenced adapter
-- keeps its existing repository, under the same channel.Queue worker lifecycle.
CREATE TABLE platform.instagram_function_backup(signature text PRIMARY KEY,definition text NOT NULL);
REVOKE ALL ON platform.instagram_function_backup FROM PUBLIC,aba_app;
INSERT INTO platform.instagram_function_backup SELECT 'platform.channel_control(uuid,boolean,boolean)',pg_get_functiondef('platform.channel_control(uuid,boolean,boolean)'::regprocedure);
INSERT INTO platform.instagram_function_backup SELECT 'platform.channel_scope(uuid,uuid)',pg_get_functiondef('platform.channel_scope(uuid,uuid)'::regprocedure);
INSERT INTO platform.instagram_function_backup SELECT 'platform.channel_ingress(uuid,bytea,jsonb)',pg_get_functiondef('platform.channel_ingress(uuid,bytea,jsonb)'::regprocedure);
INSERT INTO platform.instagram_function_backup SELECT 'platform.channel_claim(bytea,uuid,uuid,uuid)',pg_get_functiondef('platform.channel_claim(bytea,uuid,uuid,uuid)'::regprocedure);
INSERT INTO platform.instagram_function_backup SELECT 'platform.channel_order_source()',pg_get_functiondef('platform.channel_order_source()'::regprocedure);
INSERT INTO platform.instagram_function_backup SELECT 'platform.channel_control_fence()',pg_get_functiondef('platform.channel_control_fence()'::regprocedure);
INSERT INTO platform.instagram_function_backup SELECT 'platform.channel_message_outbox()',pg_get_functiondef('platform.channel_message_outbox()'::regprocedure);
ALTER TABLE platform.channel_integrations DROP CONSTRAINT channel_integrations_organization_id_key;
ALTER TABLE platform.channel_integrations ADD UNIQUE(organization_id,channel);
ALTER TABLE platform.channel_integrations DROP CONSTRAINT channel_integrations_channel_check;
ALTER TABLE platform.channel_integrations ADD CONSTRAINT channel_integrations_channel_check CHECK(channel IN ('whatsapp','instagram'));
ALTER TABLE platform.channel_principals DROP CONSTRAINT channel_principals_channel_check;
ALTER TABLE platform.channel_principals ADD CONSTRAINT channel_principals_channel_check CHECK(channel IN ('whatsapp','instagram'));
ALTER TABLE platform.conversations DROP CONSTRAINT conversations_channel_check;
ALTER TABLE platform.conversations ADD CONSTRAINT conversations_channel_check CHECK(channel IN ('internal_chat','telegram','whatsapp','instagram'));
ALTER TABLE platform.orders DROP CONSTRAINT orders_source_check;
ALTER TABLE platform.orders ADD CONSTRAINT orders_source_check CHECK(source IN ('internal_chat','telegram','whatsapp','instagram'));
ALTER TABLE platform.channel_integrations ADD CONSTRAINT channel_integrations_provider_key UNIQUE(organization_id,id,channel);
ALTER TABLE platform.channel_integrations ADD CONSTRAINT channel_integrations_account_key UNIQUE(organization_id,id,channel,phone_id);
ALTER TABLE platform.channel_principals ADD CONSTRAINT channel_principals_provider_fk FOREIGN KEY(organization_id,integration_id,channel) REFERENCES platform.channel_integrations(organization_id,id,channel);
ALTER TABLE platform.channel_inbox DROP CONSTRAINT channel_inbox_external_id_check;
ALTER TABLE platform.channel_inbox ADD CONSTRAINT channel_inbox_external_id_check CHECK(length(external_id) BETWEEN 1 AND 264);
ALTER TABLE platform.channel_inbox ADD COLUMN channel text NOT NULL DEFAULT 'whatsapp';
ALTER TABLE platform.channel_inbox ADD COLUMN external_account_id text;
UPDATE platform.channel_inbox q SET external_account_id=i.phone_id FROM platform.channel_integrations i WHERE i.id=q.integration_id;
ALTER TABLE platform.channel_inbox ALTER COLUMN external_account_id SET NOT NULL;
ALTER TABLE platform.channel_inbox ADD CONSTRAINT channel_inbox_provider_fk FOREIGN KEY(organization_id,integration_id,channel,external_account_id) REFERENCES platform.channel_integrations(organization_id,id,channel,phone_id);
ALTER TABLE platform.channel_inbox ADD COLUMN last_error_code text CHECK(last_error_code IN ('transient','permanent','lease_expired'));
CREATE INDEX channel_inbox_tenant_channel ON platform.channel_inbox(organization_id,channel,status,available_at);
CREATE TABLE platform.instagram_connections(
 organization_id uuid NOT NULL, integration_id uuid PRIMARY KEY, channel text NOT NULL DEFAULT 'instagram' CHECK(channel='instagram'),
 account_id text NOT NULL UNIQUE CHECK(account_id ~ '^[0-9]{1,20}$'), username text NOT NULL CHECK(length(username) BETWEEN 1 AND 64),
 status text NOT NULL DEFAULT 'connecting' CHECK(status IN ('connecting','connected','reauthorization_required','webhook_error','permission_error','revoked')),
 token_expires_at timestamptz, oauth_expires_at timestamptz, authorized_at timestamptz, permissions text[] NOT NULL DEFAULT '{}',
 webhook_subscribed boolean NOT NULL DEFAULT false, webhook_seen_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), revoked_at timestamptz,
 UNIQUE(organization_id,integration_id),
 CONSTRAINT instagram_connections_account_fk FOREIGN KEY(organization_id,integration_id,channel,account_id) REFERENCES platform.channel_integrations(organization_id,id,channel,phone_id));
CREATE INDEX instagram_connections_tenant ON platform.instagram_connections(organization_id,status);
CREATE TABLE platform.instagram_oauth_states(
 state_hash bytea PRIMARY KEY CHECK(octet_length(state_hash)=32), organization_id uuid NOT NULL,
 integration_id uuid NOT NULL, user_id uuid NOT NULL REFERENCES platform.users(id), session_hash bytea NOT NULL CHECK(octet_length(session_hash)=32),
 redirect_uri text NOT NULL CHECK(length(redirect_uri) BETWEEN 1 AND 2048),
 expires_at timestamptz NOT NULL, consumed_at timestamptz, created_at timestamptz NOT NULL DEFAULT now(),
 CHECK(expires_at>created_at AND expires_at<=created_at+interval '11 minutes'),
 FOREIGN KEY(organization_id,integration_id) REFERENCES platform.instagram_connections(organization_id,integration_id));
CREATE INDEX instagram_oauth_states_expiry ON platform.instagram_oauth_states(expires_at);
CREATE INDEX instagram_oauth_states_pending ON platform.instagram_oauth_states(organization_id,integration_id) WHERE consumed_at IS NULL;
DO $$ DECLARE t text; BEGIN FOREACH t IN ARRAY ARRAY['instagram_connections','instagram_oauth_states'] LOOP
 EXECUTE format('ALTER TABLE platform.%I ENABLE ROW LEVEL SECURITY',t);
 EXECUTE format('ALTER TABLE platform.%I FORCE ROW LEVEL SECURITY',t);
 EXECUTE format('CREATE POLICY migration_owner ON platform.%I TO aba_migrator USING(true) WITH CHECK(true)',t);
 END LOOP;END $$;
CREATE POLICY owner_read ON platform.instagram_connections FOR SELECT TO aba_app USING(organization_id::text=current_setting('app.organization_id',true) AND platform.has_membership(organization_id,true) AND platform.is_test_org(organization_id));
GRANT SELECT ON platform.instagram_connections TO aba_app;
-- OAuth state hashes and credentials have no direct runtime SELECT/INSERT grants.
DO $$ DECLARE p record; cmd text;q text;c text;BEGIN
 FOR p IN SELECT pol.*,cl.relname FROM pg_policy pol JOIN pg_class cl ON cl.oid=pol.polrelid JOIN pg_namespace ns ON ns.oid=cl.relnamespace WHERE ns.nspname='platform' AND pol.polname LIKE 'whatsapp_%' AND cl.relname=ANY(ARRAY['products','product_variants','inventory','conversations','messages','conversation_events','order_quotes','orders','order_items','order_events','ai_usage']) LOOP
 cmd=CASE p.polcmd WHEN 'r' THEN 'SELECT' WHEN 'a' THEN 'INSERT' WHEN 'w' THEN 'UPDATE' WHEN 'd' THEN 'DELETE' ELSE 'ALL' END;
 q=replace(pg_get_expr(p.polqual,p.polrelid),'''whatsapp''','''instagram''');
 c=replace(pg_get_expr(p.polwithcheck,p.polrelid),'''whatsapp''','''instagram''');
 EXECUTE format('CREATE POLICY %I ON platform.%I FOR %s TO aba_app%s%s',replace(p.polname,'whatsapp_','instagram_'),p.relname,cmd,CASE WHEN q IS NULL THEN '' ELSE ' USING('||q||')' END,CASE WHEN c IS NULL THEN '' ELSE ' WITH CHECK('||c||')' END);
 END LOOP;END $$;
CREATE FUNCTION platform.channel_job_binding() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ DECLARE i platform.channel_integrations;BEGIN
 SELECT * INTO i FROM platform.channel_integrations WHERE id=NEW.integration_id AND organization_id=NEW.organization_id;
 IF i.id IS NULL OR NEW.channel IS DISTINCT FROM i.channel THEN RAISE EXCEPTION 'job binding rejected' USING ERRCODE='23514';END IF;
 IF NEW.external_account_id IS NULL THEN NEW.external_account_id=i.phone_id;END IF;
 IF NEW.external_account_id IS DISTINCT FROM i.phone_id THEN RAISE EXCEPTION 'account binding rejected' USING ERRCODE='23514';END IF;
 RETURN NEW;END $$;
CREATE TRIGGER channel_job_binding BEFORE INSERT OR UPDATE OF organization_id,integration_id,channel,external_account_id ON platform.channel_inbox FOR EACH ROW EXECUTE FUNCTION platform.channel_job_binding();
CREATE OR REPLACE FUNCTION platform.channel_control(p_org uuid,p_active boolean,p_send boolean) RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ DECLARE i uuid;BEGIN
 IF p_org::text IS DISTINCT FROM current_setting('app.organization_id',true) OR NOT platform.has_membership(p_org,true) THEN RAISE EXCEPTION 'forbidden' USING ERRCODE='42501';END IF;
 SELECT id INTO i FROM platform.channel_integrations WHERE organization_id=p_org AND channel='whatsapp'; IF i IS NULL THEN RAISE no_data_found;END IF;
 PERFORM pg_advisory_xact_lock(hashtextextended(i::text,8210));
 UPDATE platform.channel_integrations SET active=p_active,sending_enabled=p_send AND p_active WHERE id=i;
 IF NOT p_active THEN UPDATE platform.channel_inbox SET status='cancelled',payload='{}',capability=NULL WHERE integration_id=i AND status IN ('pending','processing');END IF;
 IF NOT p_active OR NOT p_send THEN UPDATE platform.channel_outbox SET status='cancelled',capability=NULL,version=version+1 WHERE integration_id=i AND status IN ('pending','needs_approved_template');END IF;
 INSERT INTO platform.channel_events(organization_id,integration_id,kind,actor_id) VALUES(p_org,i,CASE WHEN NOT p_active THEN 'disconnected' WHEN p_send THEN 'sending_enabled' ELSE 'sending_disabled' END,platform.current_user_id()); END $$;
CREATE OR REPLACE FUNCTION platform.channel_scope(p_org uuid,p_conv uuid) RETURNS boolean LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT EXISTS(SELECT 1 FROM platform.channel_inbox q JOIN platform.channel_integrations i ON i.id=q.integration_id JOIN platform.organizations o ON o.id=q.organization_id WHERE q.capability::text=current_setting('app.channel_cap',true) AND q.status='processing' AND q.lease_until>clock_timestamp() AND q.organization_id=p_org AND p_org::text=current_setting('app.organization_id',true) AND i.active AND o.is_test AND i.channel=coalesce(nullif(current_setting('app.channel_type',true),''),'whatsapp') AND (i.channel<>'instagram' OR EXISTS(SELECT 1 FROM platform.instagram_connections ic WHERE ic.integration_id=i.id AND ic.organization_id=i.organization_id AND ic.revoked_at IS NULL AND ic.webhook_subscribed AND ic.token_expires_at>clock_timestamp() AND ic.permissions @> ARRAY['instagram_business_basic','instagram_business_manage_messages'])) AND EXISTS(SELECT 1 FROM platform.conversations c WHERE c.id=q.conversation_id AND c.organization_id=q.organization_id AND c.channel=i.channel) AND (p_conv IS NULL OR p_conv=q.conversation_id)) $$;
CREATE OR REPLACE FUNCTION platform.channel_ingress(p_id uuid,p_proof bytea,p_events jsonb) RETURNS integer LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 DECLARE i platform.channel_integrations; ev jsonb; k text; n integer=0; BEGIN
 SELECT * INTO i FROM platform.channel_integrations WHERE id=p_id AND active AND worker_hash=sha256(p_proof) FOR UPDATE;
 IF i.id IS NULL OR NOT EXISTS(SELECT 1 FROM platform.organizations o WHERE o.id=i.organization_id AND o.is_test) THEN RAISE EXCEPTION 'forbidden' USING ERRCODE='42501';END IF;
 IF jsonb_typeof(p_events) IS DISTINCT FROM 'array' OR jsonb_array_length(p_events)>100 OR octet_length(p_events::text)>131072 THEN RAISE EXCEPTION 'invalid events' USING ERRCODE='23514';END IF;
 IF (SELECT count(*) FROM platform.channel_inbox WHERE integration_id=p_id AND created_at>now()-interval '1 minute')>=120 THEN RAISE EXCEPTION 'rate limited' USING ERRCODE='54000';END IF;
 FOR ev IN SELECT value FROM jsonb_array_elements(p_events) LOOP
 IF ev->>'organization_id' IS DISTINCT FROM i.organization_id::text OR ev->>'integration_id' IS DISTINCT FROM i.id::text OR ev->>'channel_type' IS DISTINCT FROM i.channel OR coalesce(ev->>'external_user_id','') !~ '^[0-9]{1,20}$' OR (ev->>'conversation_id' IS NOT NULL AND ev->>'conversation_id'<>'') OR ev->>'direction' IS DISTINCT FROM 'inbound' OR length(coalesce(ev->>'external_message_id','')) NOT BETWEEN 1 AND 256 OR (i.channel='instagram' AND (coalesce(ev->>'message_type','') NOT IN ('text','unsupported_media') OR ev->>'provider_timestamp' IS NULL OR (ev->>'provider_timestamp')::timestamptz NOT BETWEEN clock_timestamp()-interval '24 hours' AND clock_timestamp()+interval '5 minutes')) THEN RAISE EXCEPTION 'invalid event' USING ERRCODE='23514';END IF;
 k=CASE WHEN ev->>'message_type'='system_event' THEN 'status:'||(ev->>'external_message_id')||':'||(ev->>'delivery_status') ELSE 'message:'||(ev->>'external_message_id') END;
 INSERT INTO platform.channel_inbox(id,organization_id,integration_id,external_id,external_user_id,payload,channel,external_account_id) VALUES((ev->>'correlation_id')::uuid,i.organization_id,i.id,k,ev->>'external_user_id',ev,i.channel,i.phone_id) ON CONFLICT(integration_id,external_id) DO NOTHING;
 IF FOUND THEN n=n+1;END IF; END LOOP;RETURN n;END $$;
CREATE OR REPLACE FUNCTION platform.channel_claim(p_proof bytea,p_cap uuid,p_conv uuid,p_customer uuid) RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 DECLARE q platform.channel_inbox;p platform.channel_principals;i platform.channel_integrations;cutoff timestamptz;BEGIN
 UPDATE platform.channel_inbox x SET status='cancelled',capability=NULL,lease_until=NULL,last_error_code='permanent' FROM platform.channel_integrations cfg JOIN platform.instagram_connections ic ON ic.integration_id=cfg.id AND ic.organization_id=cfg.organization_id WHERE x.integration_id=cfg.id AND cfg.channel='instagram' AND cfg.worker_hash=sha256(p_proof) AND x.status IN ('pending','processing') AND (ic.revoked_at IS NOT NULL OR ic.token_expires_at<=clock_timestamp() OR NOT ic.permissions @> ARRAY['instagram_business_basic','instagram_business_manage_messages']);
 UPDATE platform.channel_inbox x SET status='failed',capability=NULL WHERE status='processing' AND lease_until<now() AND attempts>=5 AND EXISTS(SELECT 1 FROM platform.channel_integrations t WHERE t.id=x.integration_id AND t.worker_hash=sha256(p_proof));
 SELECT x.* INTO q FROM platform.channel_inbox x JOIN platform.channel_integrations cfg ON cfg.id=x.integration_id WHERE cfg.active AND (cfg.channel<>'instagram' OR EXISTS(SELECT 1 FROM platform.instagram_connections ic WHERE ic.integration_id=cfg.id AND ic.organization_id=cfg.organization_id AND ic.revoked_at IS NULL AND ic.webhook_subscribed AND ic.token_expires_at>clock_timestamp() AND ic.permissions @> ARRAY['instagram_business_basic','instagram_business_manage_messages'])) AND cfg.worker_hash=sha256(p_proof) AND x.attempts<5 AND ((x.status='pending' AND x.available_at<=now()) OR (x.status='processing' AND x.lease_until<now())) AND NOT EXISTS(SELECT 1 FROM platform.channel_inbox older WHERE older.integration_id=x.integration_id AND older.external_user_id=x.external_user_id AND older.status IN ('pending','processing') AND (older.created_at,older.id)<(x.created_at,x.id)) ORDER BY x.created_at,x.id LIMIT 1 FOR UPDATE OF x SKIP LOCKED;
 IF q.id IS NULL THEN RETURN NULL;END IF;
 SELECT * INTO i FROM platform.channel_integrations WHERE id=q.integration_id;
 IF q.channel IS DISTINCT FROM i.channel OR q.external_account_id IS DISTINCT FROM i.phone_id THEN RAISE EXCEPTION 'job binding mismatch' USING ERRCODE='23514';END IF;
 IF q.payload->>'message_type'='system_event' THEN
 UPDATE platform.channel_outbox SET status=CASE WHEN status='read' THEN 'read' WHEN status='delivered' AND q.payload->>'delivery_status' IN ('sent','failed') THEN 'delivered' ELSE q.payload->>'delivery_status' END,version=version+1 WHERE integration_id=q.integration_id AND external_user_id=q.external_user_id AND provider_id=q.payload->>'external_message_id' AND status IN ('sent','delivered','read','delivery_unknown');
 UPDATE platform.channel_inbox SET status='done' WHERE id=q.id;RETURN jsonb_build_object('system',true);END IF;
 SELECT * INTO p FROM platform.channel_principals WHERE integration_id=q.integration_id AND external_user_id=q.external_user_id;
 cutoff=least((q.payload->>'provider_timestamp')::timestamptz,q.created_at)+make_interval(secs=>i.window_seconds);
 IF p.id IS NULL THEN
 INSERT INTO platform.conversations(id,organization_id,customer_id,created_by,channel,expires_at) VALUES(p_conv,q.organization_id,p_customer,NULL,i.channel,now()+make_interval(days=>i.retention_days));
 INSERT INTO platform.channel_principals(id,organization_id,integration_id,channel,external_user_id,conversation_id,window_until) VALUES(p_customer,q.organization_id,q.integration_id,i.channel,q.external_user_id,p_conv,cutoff) RETURNING * INTO p;
 ELSE UPDATE platform.channel_principals SET window_until=greatest(window_until,cutoff) WHERE id=p.id;END IF;
 UPDATE platform.channel_inbox SET status='processing',attempts=attempts+1,capability=p_cap,lease_until=now()+interval '30 seconds',conversation_id=p.conversation_id WHERE id=q.id;
 RETURN q.payload||jsonb_build_object('organization_id',q.organization_id,'integration_id',q.integration_id,'channel_type',i.channel,'external_user_id',q.external_user_id,'external_chat_id',q.external_user_id,'direction','inbound','correlation_id',q.id,'conversation_id',p.conversation_id,'capability',p_cap);END $$;
CREATE OR REPLACE FUNCTION platform.channel_order_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,platform,pg_temp AS $$ BEGIN
 IF platform.channel_scope(NEW.organization_id,NEW.conversation_id) THEN IF TG_OP='UPDATE' THEN RAISE EXCEPTION 'customer cannot update order' USING ERRCODE='42501';END IF;NEW.source=(SELECT channel FROM platform.channel_principals WHERE organization_id=NEW.organization_id AND conversation_id=NEW.conversation_id);END IF;RETURN NEW;END $$;
CREATE OR REPLACE FUNCTION platform.channel_control_fence() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ DECLARE i uuid;BEGIN
 IF NEW.channel IN ('whatsapp','instagram') AND (NEW.control_mode IS DISTINCT FROM OLD.control_mode OR NEW.status IS DISTINCT FROM OLD.status) THEN
 SELECT integration_id INTO i FROM platform.channel_principals WHERE conversation_id=NEW.id;IF i IS NOT NULL THEN
 PERFORM pg_advisory_xact_lock(hashtextextended(i::text,8210));
 UPDATE platform.channel_outbox SET status='cancelled',capability=NULL,version=version+1 WHERE conversation_id=NEW.id AND role='assistant' AND status IN ('pending','needs_approved_template');
 IF NEW.status='closed' THEN UPDATE platform.channel_inbox SET payload='{}',status=CASE WHEN status IN ('pending','processing') THEN 'cancelled' ELSE status END,capability=NULL WHERE integration_id=i AND external_user_id=(SELECT external_user_id FROM platform.channel_principals WHERE conversation_id=NEW.id);UPDATE platform.channel_outbox SET payload='{}',status=CASE WHEN status IN ('pending','needs_approved_template') THEN 'cancelled' ELSE status END WHERE conversation_id=NEW.id;END IF;
 END IF;END IF;RETURN NEW;END $$;
CREATE OR REPLACE FUNCTION platform.channel_message_outbox() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 DECLARE p platform.channel_principals;i platform.channel_integrations;body jsonb;facts text; BEGIN
 IF NEW.role NOT IN ('assistant','staff') OR NEW.content='' THEN RETURN NEW;END IF;
 SELECT * INTO p FROM platform.channel_principals WHERE organization_id=NEW.organization_id AND conversation_id=NEW.conversation_id;IF p.id IS NULL THEN RETURN NEW;END IF;
 SELECT * INTO i FROM platform.channel_integrations WHERE id=p.integration_id;
 PERFORM pg_advisory_xact_lock(hashtextextended(i.id::text,8210));
 IF NOT i.active OR (NEW.role='assistant' AND NOT EXISTS(SELECT 1 FROM platform.conversations WHERE id=p.conversation_id AND control_mode='agent')) THEN RETURN NEW;END IF;
 IF i.channel='instagram' THEN body=jsonb_build_object('text',NEW.content);
 INSERT INTO platform.channel_outbox(id,organization_id,integration_id,conversation_id,external_user_id,payload,role,status) VALUES(NEW.id,NEW.organization_id,p.integration_id,p.conversation_id,p.external_user_id,body,NEW.role,'cancelled') ON CONFLICT(id) DO NOTHING;RETURN NEW;END IF;
 IF NEW.role='staff' AND p.window_until<=now() THEN RAISE EXCEPTION 'NEEDS_APPROVED_TEMPLATE' USING ERRCODE='23514';END IF;
 body=jsonb_build_object('text',NEW.content);
 IF jsonb_typeof(NEW.data->'channel_payload')='object' THEN body=NEW.data->'channel_payload';END IF;
 IF NEW.role='assistant' AND jsonb_typeof(NEW.data->'cards')='array' THEN
 SELECT string_agg((card->>'name')||' · '||(card->>'sku')||' · '||((card->>'price_amount')::bigint/100)::text||'.'||lpad(((card->>'price_amount')::bigint%100)::text,2,'0')||' RUB · '||CASE WHEN card->>'fresh'='true' THEN 'доступно: '||coalesce(card->>'available','неизвестно') ELSE 'наличие требует подтверждения магазина' END,E'\n') INTO facts FROM jsonb_array_elements(NEW.data->'cards') card;
 body=body||jsonb_build_object('text',left(NEW.content||E'\n'||coalesce(facts,''),2000),'actions',(SELECT coalesce(jsonb_agg(jsonb_build_object('id','offer:'||(card->>'variant_id'),'title',left(card->>'sku',20))),'[]') FROM (SELECT value card FROM jsonb_array_elements(NEW.data->'cards') LIMIT 3) cards));END IF;
 INSERT INTO platform.channel_outbox(id,organization_id,integration_id,conversation_id,external_user_id,payload,role,status) VALUES(NEW.id,NEW.organization_id,p.integration_id,p.conversation_id,p.external_user_id,body,NEW.role,CASE WHEN NOT i.sending_enabled THEN 'cancelled' WHEN p.window_until<=now() THEN 'needs_approved_template' ELSE 'pending' END) ON CONFLICT(id) DO NOTHING;RETURN NEW;END $$;
-- Pin provider-specific credentials, templates and financial helpers to WhatsApp.
-- Save exact definitions so Down restores the predecessor, including 00011/12 changes.
DO $$ DECLARE f record; d text; patched text;BEGIN
 FOR f IN SELECT p.oid::regprocedure sig FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='platform' AND (left(p.proname,3)='wa_' OR p.proname IN ('channel_credentials','channel_claim_send','channel_admit_send')) LOOP
 d=pg_get_functiondef(f.sig);
 patched=replace(d,'FROM platform.channel_integrations WHERE ','FROM platform.channel_integrations WHERE channel=''whatsapp'' AND ');
 patched=replace(patched,'FROM platform.channel_integrations i WHERE ','FROM platform.channel_integrations i WHERE i.channel=''whatsapp'' AND ');
 patched=replace(patched,'JOIN platform.channel_integrations i ON ','JOIN platform.channel_integrations i ON i.channel=''whatsapp'' AND ');
 IF patched<>d THEN INSERT INTO platform.instagram_function_backup VALUES(f.sig::text,d);EXECUTE patched;END IF;
 END LOOP;END $$;
-- WhatsApp manual resolution cannot acknowledge another channel's delivery.
INSERT INTO platform.instagram_function_backup SELECT 'platform.channel_resolve(uuid,uuid,bigint,text)',pg_get_functiondef('platform.channel_resolve(uuid,uuid,bigint,text)'::regprocedure);
DO $$ DECLARE d text;BEGIN
 d=pg_get_functiondef('platform.channel_resolve(uuid,uuid,bigint,text)'::regprocedure);
 d=replace(d,'WHERE id=p_id AND organization_id=p_org','WHERE id=p_id AND organization_id=p_org AND integration_id IN (SELECT id FROM platform.channel_integrations WHERE channel=''whatsapp'')');
 EXECUTE d;END $$;
CREATE FUNCTION platform.instagram_owner(p_org uuid) RETURNS boolean LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT p_org::text=current_setting('app.organization_id',true) AND platform.has_membership(p_org,true) AND platform.is_test_org(p_org) AND coalesce(current_setting('app.channel_cap',true),'')='' AND coalesce(current_setting('app.telegram_cap',true),'')='' $$;
CREATE FUNCTION platform.instagram_prepare(p_org uuid,p_id uuid,p_account text,p_cipher bytea,p_proof bytea) RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ BEGIN
 IF NOT coalesce(platform.instagram_owner(p_org),false) THEN RAISE EXCEPTION 'forbidden' USING ERRCODE='42501';END IF;
 IF p_account IS NULL OR p_account !~ '^[0-9]{1,20}$' OR p_cipher IS NULL OR octet_length(p_cipher) NOT BETWEEN 29 AND 8192 OR p_proof IS NULL OR octet_length(p_proof)<>32 THEN RAISE EXCEPTION 'invalid configuration' USING ERRCODE='23514';END IF;
 PERFORM pg_advisory_xact_lock(hashtextextended(p_org::text,8217));
 IF EXISTS(SELECT 1 FROM platform.channel_integrations WHERE organization_id=p_org AND channel='instagram' AND (id<>p_id OR phone_id<>p_account)) THEN RAISE EXCEPTION 'identity mismatch' USING ERRCODE='23514';END IF;
 PERFORM pg_advisory_xact_lock(hashtextextended(p_id::text,8210));
 INSERT INTO platform.channel_integrations(id,organization_id,channel,app_id,business_id,phone_id,graph_version,secret_cipher,worker_hash,active,sending_enabled,window_seconds) VALUES(p_id,p_org,'instagram','0',p_account,p_account,'v27.0',p_cipher,p_proof,false,false,86400)
 ON CONFLICT(organization_id,channel) DO UPDATE SET secret_cipher=excluded.secret_cipher,worker_hash=excluded.worker_hash,active=false,sending_enabled=false;
 INSERT INTO platform.instagram_connections(organization_id,integration_id,account_id,username) VALUES(p_org,p_id,p_account,'mock_'||p_account)
 ON CONFLICT(integration_id) DO UPDATE SET status='connecting',permissions='{}',token_expires_at=NULL,webhook_subscribed=false,webhook_seen_at=NULL,revoked_at=NULL,updated_at=now();
 UPDATE platform.instagram_connections SET oauth_expires_at=now()+interval '10 minutes',authorized_at=NULL WHERE integration_id=p_id;
 INSERT INTO platform.channel_events(organization_id,integration_id,kind,actor_id) VALUES(p_org,p_id,'instagram_mock_connecting',platform.current_user_id());
 UPDATE platform.instagram_oauth_states SET consumed_at=now() WHERE integration_id=p_id AND consumed_at IS NULL;
 UPDATE platform.channel_inbox SET status='cancelled',capability=NULL,lease_until=NULL WHERE integration_id=p_id AND status IN ('pending','processing');
 UPDATE platform.channel_outbox SET status=CASE WHEN status='sending' THEN 'delivery_unknown' ELSE 'cancelled' END,capability=NULL,lease_until=NULL,version=version+1 WHERE integration_id=p_id AND status IN ('pending','sending','needs_approved_template');
 END $$;
CREATE FUNCTION platform.instagram_state_create(p_org uuid,p_id uuid,p_hash bytea,p_session bytea,p_redirect text,p_expiry timestamptz) RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ BEGIN
 IF NOT coalesce(platform.instagram_owner(p_org),false) OR encode(p_session,'hex') IS DISTINCT FROM current_setting('app.session_hash',true) THEN RAISE EXCEPTION 'forbidden' USING ERRCODE='42501';END IF;
 INSERT INTO platform.instagram_oauth_states(state_hash,organization_id,integration_id,user_id,session_hash,redirect_uri,expires_at) VALUES(p_hash,p_org,p_id,platform.current_user_id(),p_session,p_redirect,p_expiry);END $$;
CREATE FUNCTION platform.instagram_state_consume(p_org uuid,p_hash bytea,p_session bytea,p_redirect text) RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ BEGIN
 IF NOT coalesce(platform.instagram_owner(p_org),false) OR encode(p_session,'hex') IS DISTINCT FROM current_setting('app.session_hash',true) THEN RETURN false;END IF;
 UPDATE platform.instagram_oauth_states SET consumed_at=clock_timestamp() WHERE state_hash=p_hash AND organization_id=p_org AND user_id=platform.current_user_id() AND session_hash=p_session AND redirect_uri=p_redirect AND consumed_at IS NULL AND expires_at>clock_timestamp();RETURN FOUND;END $$;
CREATE FUNCTION platform.instagram_authorize(p_org uuid,p_id uuid,p_cipher bytea,p_expiry timestamptz,p_permissions text[]) RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ BEGIN
 IF NOT coalesce(platform.instagram_owner(p_org),false) THEN RAISE EXCEPTION 'forbidden' USING ERRCODE='42501';END IF;
 IF p_cipher IS NULL OR octet_length(p_cipher) NOT BETWEEN 29 AND 8192 OR p_expiry IS NULL OR p_expiry<=clock_timestamp() OR p_permissions IS NULL THEN RAISE EXCEPTION 'invalid token metadata' USING ERRCODE='23514';END IF;
 UPDATE platform.instagram_connections SET token_expires_at=p_expiry,oauth_expires_at=NULL,authorized_at=date_trunc('milliseconds',clock_timestamp()),permissions=p_permissions,webhook_subscribed=true,status=CASE WHEN p_permissions @> ARRAY['instagram_business_basic','instagram_business_manage_messages'] THEN 'connecting' ELSE 'permission_error' END,updated_at=now() WHERE organization_id=p_org AND integration_id=p_id AND revoked_at IS NULL AND status='connecting';
 IF NOT FOUND THEN RAISE EXCEPTION 'not found' USING ERRCODE='P0002';END IF;
 UPDATE platform.channel_integrations SET secret_cipher=p_cipher,active=p_permissions @> ARRAY['instagram_business_basic','instagram_business_manage_messages'],sending_enabled=false WHERE organization_id=p_org AND id=p_id AND channel='instagram';
 INSERT INTO platform.channel_events(organization_id,integration_id,kind,actor_id) VALUES(p_org,p_id,'instagram_mock_authorized',platform.current_user_id());END $$;
CREATE FUNCTION platform.instagram_auth_failure(p_org uuid,p_id uuid) RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ BEGIN
 IF NOT coalesce(platform.instagram_owner(p_org),false) THEN RAISE EXCEPTION 'forbidden' USING ERRCODE='42501';END IF;
 UPDATE platform.instagram_connections SET status='reauthorization_required',oauth_expires_at=NULL,authorized_at=NULL,token_expires_at=NULL,permissions='{}',webhook_subscribed=false,webhook_seen_at=NULL,updated_at=now() WHERE organization_id=p_org AND integration_id=p_id AND revoked_at IS NULL AND status='connecting';
 IF NOT FOUND THEN RAISE EXCEPTION 'not found' USING ERRCODE='P0002';END IF;
 UPDATE platform.channel_integrations SET active=false,sending_enabled=false,secret_cipher='\x'::bytea WHERE organization_id=p_org AND id=p_id AND channel='instagram';
 INSERT INTO platform.channel_events(organization_id,integration_id,kind,actor_id) VALUES(p_org,p_id,'instagram_mock_authorization_rejected',platform.current_user_id());END $$;
CREATE FUNCTION platform.instagram_disconnect(p_org uuid) RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ DECLARE target uuid;BEGIN
 IF NOT coalesce(platform.instagram_owner(p_org),false) THEN RAISE EXCEPTION 'forbidden' USING ERRCODE='42501';END IF;
 PERFORM pg_advisory_xact_lock(hashtextextended(p_org::text,8217));
 SELECT integration_id INTO target FROM platform.instagram_connections WHERE organization_id=p_org FOR UPDATE;
 IF target IS NULL THEN RETURN;END IF;
 PERFORM pg_advisory_xact_lock(hashtextextended(target::text,8210));
 UPDATE platform.channel_integrations SET active=false,sending_enabled=false,secret_cipher='\x'::bytea WHERE id=target;
 UPDATE platform.instagram_connections SET status='revoked',oauth_expires_at=NULL,authorized_at=NULL,permissions='{}',token_expires_at=NULL,webhook_subscribed=false,webhook_seen_at=NULL,revoked_at=now(),updated_at=now() WHERE integration_id=target;
 UPDATE platform.instagram_oauth_states SET consumed_at=now() WHERE integration_id=target AND consumed_at IS NULL;
 UPDATE platform.channel_inbox SET status='cancelled',capability=NULL,lease_until=NULL,payload='{}' WHERE integration_id=target AND status IN ('pending','processing');
 UPDATE platform.channel_outbox SET status=CASE WHEN status='sending' THEN 'delivery_unknown' ELSE 'cancelled' END,capability=NULL,lease_until=NULL,version=version+1 WHERE integration_id=target AND status IN ('pending','sending','needs_approved_template');
 INSERT INTO platform.channel_events(organization_id,integration_id,kind,actor_id) VALUES(p_org,target,'instagram_mock_revoked',platform.current_user_id());END $$;
CREATE FUNCTION platform.instagram_webhook_binding(p_id uuid,p_proof bytea) RETURNS jsonb LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT jsonb_build_object('organization_id',i.organization_id,'integration_id',i.id,'account_id',c.account_id) FROM platform.channel_integrations i JOIN platform.instagram_connections c ON c.organization_id=i.organization_id AND c.integration_id=i.id JOIN platform.organizations o ON o.id=i.organization_id WHERE i.id=p_id AND i.channel='instagram' AND i.mode='mock' AND i.active AND NOT i.sending_enabled AND i.worker_hash=sha256(p_proof) AND c.revoked_at IS NULL AND c.webhook_subscribed AND c.token_expires_at>clock_timestamp() AND c.permissions @> ARRAY['instagram_business_basic','instagram_business_manage_messages'] AND o.is_test $$;
CREATE FUNCTION platform.instagram_observe(p_id uuid,p_proof bytea) RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ BEGIN
 IF platform.instagram_webhook_binding(p_id,p_proof) IS NULL THEN RAISE EXCEPTION 'unknown connection' USING ERRCODE='42501';END IF;
 UPDATE platform.instagram_connections c SET webhook_seen_at=clock_timestamp(),updated_at=clock_timestamp(),status='connected' WHERE integration_id=p_id AND authorized_at IS NOT NULL AND EXISTS(SELECT 1 FROM platform.channel_inbox q WHERE q.integration_id=c.integration_id AND q.organization_id=c.organization_id AND q.channel='instagram' AND q.created_at>=c.authorized_at AND (q.payload->>'provider_timestamp')::timestamptz>=c.authorized_at);END $$;
CREATE FUNCTION platform.channel_finish_classified(p_cap uuid,p_success boolean,p_retryable boolean) RETURNS void LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 UPDATE platform.channel_inbox SET status=CASE WHEN p_success THEN 'done' WHEN NOT p_retryable OR attempts>=5 THEN 'failed' ELSE 'pending' END,last_error_code=CASE WHEN p_success THEN NULL WHEN p_retryable THEN 'transient' ELSE 'permanent' END,capability=NULL,lease_until=NULL,available_at=clock_timestamp()+make_interval(secs=>least(300,power(2,attempts)::integer)) WHERE capability=p_cap AND status='processing' AND lease_until>clock_timestamp() $$;
-- All new privileged helpers are closed to PUBLIC. Trigger/owner helpers are not callable by runtime.
REVOKE ALL ON FUNCTION platform.channel_job_binding() FROM PUBLIC,aba_app;
REVOKE ALL ON FUNCTION platform.instagram_owner(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.instagram_owner(uuid) TO aba_app;
DO $$ DECLARE f record;BEGIN FOR f IN SELECT p.oid::regprocedure sig FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='platform' AND p.proname IN ('instagram_prepare','instagram_state_create','instagram_state_consume','instagram_authorize','instagram_auth_failure','instagram_disconnect','instagram_webhook_binding','instagram_observe','channel_finish_classified') LOOP
 EXECUTE format('REVOKE ALL ON FUNCTION %s FROM PUBLIC',f.sig);EXECUTE format('GRANT EXECUTE ON FUNCTION %s TO aba_app',f.sig);END LOOP;END $$;
UPDATE platform.service_metadata SET version=17 WHERE singleton;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Never erase connection/state/conversation/catalog audit to make rollback pass.
DO $$ BEGIN IF EXISTS(SELECT 1 FROM platform.instagram_connections) OR EXISTS(SELECT 1 FROM platform.instagram_oauth_states) OR EXISTS(SELECT 1 FROM platform.channel_integrations WHERE channel='instagram') OR EXISTS(SELECT 1 FROM platform.conversations WHERE channel='instagram') OR EXISTS(SELECT 1 FROM platform.orders WHERE source='instagram') OR EXISTS(SELECT 1 FROM platform.products) OR EXISTS(SELECT 1 FROM platform.catalog_policies) OR EXISTS(SELECT 1 FROM platform.catalog_events) OR EXISTS(SELECT 1 FROM platform.channel_principals) THEN RAISE EXCEPTION 'rollback refused: preserve provider/catalog data';END IF;END $$;
DO $$ DECLARE p record;BEGIN FOR p IN SELECT pol.polname,cl.relname FROM pg_policy pol JOIN pg_class cl ON cl.oid=pol.polrelid JOIN pg_namespace ns ON ns.oid=cl.relnamespace WHERE ns.nspname='platform' AND pol.polname LIKE 'instagram_%' LOOP EXECUTE format('DROP POLICY %I ON platform.%I',p.polname,p.relname);END LOOP;END $$;
DO $$ DECLARE f record;BEGIN FOR f IN SELECT definition FROM platform.instagram_function_backup LOOP EXECUTE f.definition;END LOOP;END $$;
DROP TRIGGER channel_job_binding ON platform.channel_inbox;
DROP FUNCTION platform.channel_job_binding();
DROP FUNCTION platform.instagram_auth_failure(uuid,uuid);
DROP FUNCTION platform.instagram_prepare(uuid,uuid,text,bytea,bytea),platform.instagram_state_create(uuid,uuid,bytea,bytea,text,timestamptz),platform.instagram_state_consume(uuid,bytea,bytea,text),platform.instagram_authorize(uuid,uuid,bytea,timestamptz,text[]),platform.instagram_disconnect(uuid),platform.instagram_observe(uuid,bytea),platform.instagram_webhook_binding(uuid,bytea),platform.channel_finish_classified(uuid,boolean,boolean);
DROP TABLE platform.instagram_oauth_states,platform.instagram_connections;
DROP FUNCTION platform.instagram_owner(uuid);
DROP TABLE platform.instagram_function_backup;
ALTER TABLE platform.channel_inbox DROP CONSTRAINT channel_inbox_external_id_check;
ALTER TABLE platform.channel_inbox ADD CONSTRAINT channel_inbox_external_id_check CHECK(length(external_id) BETWEEN 1 AND 240);
ALTER TABLE platform.channel_inbox DROP CONSTRAINT channel_inbox_provider_fk;
ALTER TABLE platform.channel_inbox DROP COLUMN channel,DROP COLUMN external_account_id,DROP COLUMN last_error_code;
ALTER TABLE platform.channel_principals DROP CONSTRAINT channel_principals_provider_fk;
ALTER TABLE platform.channel_integrations DROP CONSTRAINT channel_integrations_provider_key;
ALTER TABLE platform.channel_integrations DROP CONSTRAINT channel_integrations_account_key;
ALTER TABLE platform.channel_integrations DROP CONSTRAINT channel_integrations_organization_id_channel_key;
ALTER TABLE platform.channel_integrations ADD UNIQUE(organization_id);
ALTER TABLE platform.channel_integrations DROP CONSTRAINT channel_integrations_channel_check;
ALTER TABLE platform.channel_integrations ADD CONSTRAINT channel_integrations_channel_check CHECK(channel='whatsapp');
ALTER TABLE platform.channel_principals DROP CONSTRAINT channel_principals_channel_check;
ALTER TABLE platform.channel_principals ADD CONSTRAINT channel_principals_channel_check CHECK(channel='whatsapp');
ALTER TABLE platform.conversations DROP CONSTRAINT conversations_channel_check;
ALTER TABLE platform.conversations ADD CONSTRAINT conversations_channel_check CHECK(channel IN ('internal_chat','telegram','whatsapp'));
ALTER TABLE platform.orders DROP CONSTRAINT orders_source_check;
ALTER TABLE platform.orders ADD CONSTRAINT orders_source_check CHECK(source IN ('internal_chat','telegram','whatsapp'));
UPDATE platform.service_metadata SET version=16 WHERE singleton;
-- +goose StatementEnd
