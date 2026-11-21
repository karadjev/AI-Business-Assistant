-- +goose Up
-- +goose StatementBegin
CREATE TABLE platform.channel_integrations(
 id uuid PRIMARY KEY, organization_id uuid NOT NULL UNIQUE REFERENCES platform.organizations(id),
 channel text NOT NULL DEFAULT 'whatsapp' CHECK(channel='whatsapp'), mode text NOT NULL DEFAULT 'mock' CHECK(mode='mock'),
 app_id text NOT NULL, business_id text NOT NULL, phone_id text NOT NULL UNIQUE, graph_version text NOT NULL,
 secret_cipher bytea NOT NULL, worker_hash bytea NOT NULL CHECK(octet_length(worker_hash)=32),
 active boolean NOT NULL DEFAULT true, sending_enabled boolean NOT NULL DEFAULT false,
 monetary_budget bigint NOT NULL DEFAULT 0 CHECK(monetary_budget=0), retention_days integer NOT NULL DEFAULT 7 CHECK(retention_days BETWEEN 1 AND 30),
 window_seconds integer NOT NULL DEFAULT 86400 CHECK(window_seconds BETWEEN 60 AND 86400),
 created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(organization_id,id));
CREATE TABLE platform.channel_principals(
 id uuid PRIMARY KEY,organization_id uuid NOT NULL,integration_id uuid NOT NULL,channel text NOT NULL DEFAULT 'whatsapp' CHECK(channel='whatsapp'),
 external_user_id text NOT NULL CHECK(external_user_id ~ '^[0-9]{1,20}$'), conversation_id uuid NOT NULL,
 window_until timestamptz NOT NULL,UNIQUE(integration_id,channel,external_user_id),UNIQUE(organization_id,id),UNIQUE(organization_id,integration_id,external_user_id,conversation_id),UNIQUE(organization_id,conversation_id),
 FOREIGN KEY(organization_id,integration_id) REFERENCES platform.channel_integrations(organization_id,id),
 FOREIGN KEY(organization_id,conversation_id) REFERENCES platform.conversations(organization_id,id));
CREATE TABLE platform.channel_inbox(
 id uuid PRIMARY KEY,organization_id uuid NOT NULL,integration_id uuid NOT NULL,
 external_id text NOT NULL CHECK(length(external_id) BETWEEN 1 AND 240), external_user_id text NOT NULL,
 payload jsonb NOT NULL CHECK(octet_length(payload::text)<=16384),conversation_id uuid,
 status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','processing','done','failed','cancelled')),
 attempts integer NOT NULL DEFAULT 0 CHECK(attempts BETWEEN 0 AND 5),available_at timestamptz NOT NULL DEFAULT now(),
 capability uuid,lease_until timestamptz,created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(integration_id,external_id),FOREIGN KEY(organization_id,integration_id) REFERENCES platform.channel_integrations(organization_id,id),
 FOREIGN KEY(organization_id,conversation_id) REFERENCES platform.conversations(organization_id,id));
CREATE INDEX channel_inbox_claim ON platform.channel_inbox(status,available_at,created_at,id);
CREATE TABLE platform.channel_outbox(
 id uuid PRIMARY KEY,organization_id uuid NOT NULL,integration_id uuid NOT NULL,conversation_id uuid NOT NULL,external_user_id text NOT NULL,
 payload jsonb NOT NULL CHECK(octet_length(payload::text)<=16384),role text NOT NULL CHECK(role IN ('assistant','staff')),
 status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','sending','sent','delivered','read','failed','delivery_unknown','needs_approved_template','cancelled')),
 attempts integer NOT NULL DEFAULT 0 CHECK(attempts BETWEEN 0 AND 5),available_at timestamptz NOT NULL DEFAULT now(),
 capability uuid,lease_until timestamptz,provider_id text,version bigint NOT NULL DEFAULT 1,created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,integration_id) REFERENCES platform.channel_integrations(organization_id,id),
 FOREIGN KEY(organization_id,conversation_id) REFERENCES platform.conversations(organization_id,id));
ALTER TABLE platform.channel_outbox ADD FOREIGN KEY(organization_id,integration_id,external_user_id,conversation_id) REFERENCES platform.channel_principals(organization_id,integration_id,external_user_id,conversation_id);
CREATE UNIQUE INDEX channel_outbox_provider ON platform.channel_outbox(integration_id,provider_id) WHERE provider_id IS NOT NULL;
CREATE INDEX channel_outbox_claim ON platform.channel_outbox(status,available_at,created_at,id);
ALTER TABLE platform.channel_outbox ADD UNIQUE(organization_id,id);
CREATE TABLE platform.channel_events(
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,organization_id uuid NOT NULL,integration_id uuid NOT NULL,
 outbox_id uuid,kind text NOT NULL,actor_id uuid REFERENCES platform.users(id),created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,integration_id) REFERENCES platform.channel_integrations(organization_id,id),
 FOREIGN KEY(organization_id,outbox_id) REFERENCES platform.channel_outbox(organization_id,id));
DO $$ DECLARE t text; BEGIN FOREACH t IN ARRAY ARRAY['channel_integrations','channel_principals','channel_inbox','channel_outbox','channel_events'] LOOP
 EXECUTE format('ALTER TABLE platform.%I ENABLE ROW LEVEL SECURITY',t);
 EXECUTE format('ALTER TABLE platform.%I FORCE ROW LEVEL SECURITY',t);
 EXECUTE format('CREATE POLICY migration_owner ON platform.%I TO aba_migrator USING(true) WITH CHECK(true)',t);
 EXECUTE format('CREATE POLICY staff_read ON platform.%I FOR SELECT TO aba_app USING(organization_id::text=current_setting(''app.organization_id'',true) AND platform.can_operate_orders(organization_id) AND platform.is_test_org(organization_id))',t);
 END LOOP; END $$;
GRANT SELECT ON platform.channel_principals,platform.channel_inbox,platform.channel_outbox,platform.channel_events TO aba_app;
GRANT SELECT(id,organization_id,channel,mode,app_id,business_id,phone_id,graph_version,active,sending_enabled,monetary_budget,window_seconds,created_at) ON platform.channel_integrations TO aba_app;
ALTER TABLE platform.conversations DROP CONSTRAINT conversations_channel_check;
ALTER TABLE platform.conversations ADD CHECK(channel IN ('internal_chat','telegram','whatsapp'));
ALTER TABLE platform.orders DROP CONSTRAINT orders_source_check;
ALTER TABLE platform.orders ADD CHECK(source IN ('internal_chat','telegram','whatsapp'));
CREATE FUNCTION platform.channel_scope(p_org uuid,p_conv uuid) RETURNS boolean LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT EXISTS(SELECT 1 FROM platform.channel_inbox q JOIN platform.channel_integrations i ON i.id=q.integration_id JOIN platform.organizations o ON o.id=q.organization_id WHERE q.capability::text=current_setting('app.channel_cap',true) AND q.status='processing' AND q.lease_until>clock_timestamp() AND q.organization_id=p_org AND p_org::text=current_setting('app.organization_id',true) AND i.active AND o.is_test AND (p_conv IS NULL OR p_conv=q.conversation_id)) $$;
CREATE FUNCTION platform.channel_conversation() RETURNS uuid LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ SELECT conversation_id FROM platform.channel_inbox WHERE capability::text=current_setting('app.channel_cap',true) AND platform.channel_scope(organization_id,conversation_id) $$;
CREATE POLICY channel_read ON platform.channel_principals FOR SELECT TO aba_app USING(platform.channel_scope(organization_id,conversation_id));
CREATE POLICY channel_read ON platform.channel_outbox FOR SELECT TO aba_app USING(platform.channel_scope(organization_id,conversation_id));
-- Separate policies: a WhatsApp lease never authorizes access to Telegram storage.
DO $$ DECLARE p record; cmd text; q text; c text; BEGIN
 FOR p IN SELECT pol.*,cl.relname FROM pg_policy pol JOIN pg_class cl ON cl.oid=pol.polrelid JOIN pg_namespace ns ON ns.oid=cl.relnamespace WHERE ns.nspname='platform' AND pol.polname LIKE 'telegram_%' AND cl.relname=ANY(ARRAY['products','product_variants','inventory','conversations','messages','conversation_events','order_quotes','orders','order_items','order_events','ai_usage']) LOOP
 cmd=CASE p.polcmd WHEN 'r' THEN 'SELECT' WHEN 'a' THEN 'INSERT' WHEN 'w' THEN 'UPDATE' WHEN 'd' THEN 'DELETE' ELSE 'ALL' END;
 q=replace(replace(replace(pg_get_expr(p.polqual,p.polrelid),'telegram_scope','channel_scope'),'telegram_conversation','channel_conversation'),'''telegram''','''whatsapp''');
 c=replace(replace(replace(pg_get_expr(p.polwithcheck,p.polrelid),'telegram_scope','channel_scope'),'telegram_conversation','channel_conversation'),'''telegram''','''whatsapp''');
 EXECUTE format('CREATE POLICY %I ON platform.%I FOR %s TO aba_app%s%s',replace(p.polname,'telegram_','whatsapp_'),p.relname,cmd,CASE WHEN q IS NULL THEN '' ELSE ' USING('||q||')' END,CASE WHEN c IS NULL THEN '' ELSE ' WITH CHECK('||c||')' END);
 END LOOP;END $$;
CREATE FUNCTION platform.external_channel_scope(p_org uuid,p_conv uuid) RETURNS boolean LANGUAGE sql STABLE SET search_path=pg_catalog,platform,pg_temp AS $$ SELECT platform.telegram_scope(p_org,p_conv) OR platform.channel_scope(p_org,p_conv) $$;
-- Keep original helper bodies for a lossless rollback. No tenant data is copied.
CREATE TABLE platform.channel_function_backup(signature text PRIMARY KEY,definition text NOT NULL);
REVOKE ALL ON platform.channel_function_backup FROM PUBLIC,aba_app;
DO $$ DECLARE sig text; def text; BEGIN
 FOREACH sig IN ARRAY ARRAY['platform.lock_order_inventory(uuid[])','platform.lock_order_products(uuid[])','platform.llm_authorized(uuid,uuid)'] LOOP
 def=pg_get_functiondef(sig::regprocedure);INSERT INTO platform.channel_function_backup VALUES(sig,def);
 -- These three helpers authorize shared business operations only; never Telegram queues.
 EXECUTE replace(def,'platform.telegram_scope(', 'platform.external_channel_scope(');
 END LOOP;END $$;
CREATE FUNCTION platform.channel_connect(p_id uuid,p_org uuid,p_app text,p_business text,p_phone text,p_version text,p_cipher bytea,p_proof bytea,p_window integer,p_days integer DEFAULT 7) RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ BEGIN
 IF p_org::text IS DISTINCT FROM current_setting('app.organization_id',true) OR NOT platform.has_membership(p_org,true) OR NOT platform.is_test_org(p_org) THEN RAISE EXCEPTION 'forbidden' USING ERRCODE='42501';END IF;
 IF p_app !~ '^[0-9]{1,20}$' OR p_business !~ '^[0-9]{1,20}$' OR p_phone !~ '^[0-9]{1,20}$' OR p_version !~ '^v[0-9]{1,3}\.[0-9]{1,2}$' OR octet_length(p_cipher)>8192 THEN RAISE EXCEPTION 'invalid configuration' USING ERRCODE='23514';END IF;
 INSERT INTO platform.channel_integrations(id,organization_id,app_id,business_id,phone_id,graph_version,secret_cipher,worker_hash,window_seconds,retention_days) VALUES(p_id,p_org,p_app,p_business,p_phone,p_version,p_cipher,p_proof,p_window,p_days);
 INSERT INTO platform.channel_events(organization_id,integration_id,kind,actor_id) VALUES(p_org,p_id,'connected_mock',platform.current_user_id());END $$;
-- Proof is a server HMAC-derived capability, not a client credential or webhook header.
CREATE FUNCTION platform.channel_credentials(p_id uuid,p_proof bytea) RETURNS jsonb LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT jsonb_build_object('id',id,'organization_id',organization_id,'app_id',app_id,'business_id',business_id,'phone_id',phone_id,'graph_version',graph_version,'window_seconds',window_seconds,'cipher',encode(secret_cipher,'hex')) FROM platform.channel_integrations i WHERE i.id=p_id AND i.active AND i.worker_hash=sha256(p_proof) AND EXISTS(SELECT 1 FROM platform.organizations o WHERE o.id=i.organization_id AND o.is_test) $$;
CREATE FUNCTION platform.channel_control(p_org uuid,p_active boolean,p_send boolean) RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ DECLARE i uuid;BEGIN
 IF p_org::text IS DISTINCT FROM current_setting('app.organization_id',true) OR NOT platform.has_membership(p_org,true) THEN RAISE EXCEPTION 'forbidden' USING ERRCODE='42501';END IF;
 SELECT id INTO i FROM platform.channel_integrations WHERE organization_id=p_org; IF i IS NULL THEN RAISE no_data_found;END IF;
 PERFORM pg_advisory_xact_lock(hashtextextended(i::text,8210));
 UPDATE platform.channel_integrations SET active=p_active,sending_enabled=p_send AND p_active WHERE id=i;
 IF NOT p_active THEN UPDATE platform.channel_inbox SET status='cancelled',payload='{}',capability=NULL WHERE integration_id=i AND status IN ('pending','processing');END IF;
 IF NOT p_active OR NOT p_send THEN UPDATE platform.channel_outbox SET status='cancelled',capability=NULL,version=version+1 WHERE integration_id=i AND status IN ('pending','needs_approved_template');END IF;
 INSERT INTO platform.channel_events(organization_id,integration_id,kind,actor_id) VALUES(p_org,i,CASE WHEN NOT p_active THEN 'disconnected' WHEN p_send THEN 'sending_enabled' ELSE 'sending_disabled' END,platform.current_user_id()); END $$;
CREATE FUNCTION platform.channel_ingress(p_id uuid,p_proof bytea,p_events jsonb) RETURNS integer LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 DECLARE i platform.channel_integrations; ev jsonb; k text; n integer=0; BEGIN
 SELECT * INTO i FROM platform.channel_integrations WHERE id=p_id AND active AND worker_hash=sha256(p_proof) FOR UPDATE;
 IF i.id IS NULL OR NOT EXISTS(SELECT 1 FROM platform.organizations o WHERE o.id=i.organization_id AND o.is_test) THEN RAISE EXCEPTION 'forbidden' USING ERRCODE='42501';END IF;
 IF jsonb_typeof(p_events)<>'array' OR jsonb_array_length(p_events)>100 OR octet_length(p_events::text)>131072 THEN RAISE EXCEPTION 'invalid events' USING ERRCODE='23514';END IF;
 IF (SELECT count(*) FROM platform.channel_inbox WHERE integration_id=p_id AND created_at>now()-interval '1 minute')>=120 THEN RAISE EXCEPTION 'rate limited' USING ERRCODE='54000';END IF;
 FOR ev IN SELECT value FROM jsonb_array_elements(p_events) LOOP
 IF ev->>'organization_id'<>i.organization_id::text OR ev->>'integration_id'<>i.id::text OR ev->>'channel_type'<>'whatsapp' OR ev->>'external_user_id' !~ '^[0-9]{1,20}$' THEN RAISE EXCEPTION 'invalid event' USING ERRCODE='23514';END IF;
 k=CASE WHEN ev->>'message_type'='system_event' THEN 'status:'||(ev->>'external_message_id')||':'||(ev->>'delivery_status') ELSE 'message:'||(ev->>'external_message_id') END;
 INSERT INTO platform.channel_inbox(id,organization_id,integration_id,external_id,external_user_id,payload) VALUES((ev->>'correlation_id')::uuid,i.organization_id,i.id,k,ev->>'external_user_id',ev) ON CONFLICT(integration_id,external_id) DO NOTHING;
 IF FOUND THEN n=n+1;END IF; END LOOP;RETURN n;END $$;
CREATE FUNCTION platform.channel_claim(p_proof bytea,p_cap uuid,p_conv uuid,p_customer uuid) RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 DECLARE q platform.channel_inbox;p platform.channel_principals;i platform.channel_integrations;cutoff timestamptz;BEGIN
 UPDATE platform.channel_inbox x SET status='failed',capability=NULL WHERE status='processing' AND lease_until<now() AND attempts>=5 AND EXISTS(SELECT 1 FROM platform.channel_integrations t WHERE t.id=x.integration_id AND t.worker_hash=sha256(p_proof));
 SELECT x.* INTO q FROM platform.channel_inbox x JOIN platform.channel_integrations cfg ON cfg.id=x.integration_id WHERE cfg.active AND cfg.worker_hash=sha256(p_proof) AND x.attempts<5 AND ((x.status='pending' AND x.available_at<=now()) OR (x.status='processing' AND x.lease_until<now())) AND NOT EXISTS(SELECT 1 FROM platform.channel_inbox older WHERE older.integration_id=x.integration_id AND older.external_user_id=x.external_user_id AND older.status IN ('pending','processing') AND (older.created_at,older.id)<(x.created_at,x.id)) ORDER BY x.created_at,x.id LIMIT 1 FOR UPDATE OF x SKIP LOCKED;
 IF q.id IS NULL THEN RETURN NULL;END IF;
 SELECT * INTO i FROM platform.channel_integrations WHERE id=q.integration_id;
 IF q.payload->>'message_type'='system_event' THEN
 UPDATE platform.channel_outbox SET status=CASE WHEN status='read' THEN 'read' WHEN status='delivered' AND q.payload->>'delivery_status' IN ('sent','failed') THEN 'delivered' ELSE q.payload->>'delivery_status' END,version=version+1 WHERE integration_id=q.integration_id AND external_user_id=q.external_user_id AND provider_id=q.payload->>'external_message_id' AND status IN ('sent','delivered','read','delivery_unknown');
 UPDATE platform.channel_inbox SET status='done' WHERE id=q.id;RETURN jsonb_build_object('system',true);END IF;
 SELECT * INTO p FROM platform.channel_principals WHERE integration_id=q.integration_id AND external_user_id=q.external_user_id;
 cutoff=least((q.payload->>'provider_timestamp')::timestamptz,q.created_at)+make_interval(secs=>i.window_seconds);
 IF p.id IS NULL THEN
 INSERT INTO platform.conversations(id,organization_id,customer_id,created_by,channel,expires_at) VALUES(p_conv,q.organization_id,p_customer,NULL,'whatsapp',now()+make_interval(days=>i.retention_days));
 INSERT INTO platform.channel_principals(id,organization_id,integration_id,external_user_id,conversation_id,window_until) VALUES(p_customer,q.organization_id,q.integration_id,q.external_user_id,p_conv,cutoff) RETURNING * INTO p;
 ELSE UPDATE platform.channel_principals SET window_until=greatest(window_until,cutoff) WHERE id=p.id;END IF;
 UPDATE platform.channel_inbox SET status='processing',attempts=attempts+1,capability=p_cap,lease_until=now()+interval '30 seconds',conversation_id=p.conversation_id WHERE id=q.id;
 RETURN q.payload||jsonb_build_object('correlation_id',q.id,'conversation_id',p.conversation_id,'capability',p_cap);END $$;
CREATE FUNCTION platform.channel_finish(p_cap uuid,p_success boolean) RETURNS void LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 UPDATE platform.channel_inbox SET status=CASE WHEN p_success THEN 'done' WHEN attempts>=5 THEN 'failed' ELSE 'pending' END,capability=NULL,lease_until=NULL,available_at=now()+make_interval(secs=>least(300,power(2,attempts)::integer)) WHERE capability=p_cap AND status='processing' AND lease_until>now() $$;
CREATE FUNCTION platform.channel_order_source() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,platform,pg_temp AS $$ BEGIN
 IF platform.channel_scope(NEW.organization_id,NEW.conversation_id) THEN IF TG_OP='UPDATE' THEN RAISE EXCEPTION 'customer cannot update order' USING ERRCODE='42501';END IF;NEW.source='whatsapp';END IF;RETURN NEW;END $$;
CREATE TRIGGER channel_order_source BEFORE INSERT OR UPDATE ON platform.orders FOR EACH ROW EXECUTE FUNCTION platform.channel_order_source();
CREATE FUNCTION platform.channel_message_outbox() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 DECLARE p platform.channel_principals;i platform.channel_integrations;body jsonb;facts text; BEGIN
 IF NEW.role NOT IN ('assistant','staff') OR NEW.content='' THEN RETURN NEW;END IF;
 SELECT * INTO p FROM platform.channel_principals WHERE organization_id=NEW.organization_id AND conversation_id=NEW.conversation_id;IF p.id IS NULL THEN RETURN NEW;END IF;
 SELECT * INTO i FROM platform.channel_integrations WHERE id=p.integration_id;
 PERFORM pg_advisory_xact_lock(hashtextextended(i.id::text,8210));
 IF NOT i.active OR (NEW.role='assistant' AND NOT EXISTS(SELECT 1 FROM platform.conversations WHERE id=p.conversation_id AND control_mode='agent')) THEN RETURN NEW;END IF;
 IF NEW.role='staff' AND p.window_until<=now() THEN RAISE EXCEPTION 'NEEDS_APPROVED_TEMPLATE' USING ERRCODE='23514';END IF;
 body=jsonb_build_object('text',NEW.content);
 IF jsonb_typeof(NEW.data->'channel_payload')='object' THEN body=NEW.data->'channel_payload';END IF;
 IF NEW.role='assistant' AND jsonb_typeof(NEW.data->'cards')='array' THEN
 SELECT string_agg((card->>'name')||' · '||(card->>'sku')||' · '||((card->>'price_amount')::bigint/100)::text||'.'||lpad(((card->>'price_amount')::bigint%100)::text,2,'0')||' RUB · '||CASE WHEN card->>'fresh'='true' THEN 'доступно: '||coalesce(card->>'available','неизвестно') ELSE 'наличие требует подтверждения магазина' END,E'\n') INTO facts FROM jsonb_array_elements(NEW.data->'cards') card;
 body=body||jsonb_build_object('text',left(NEW.content||E'\n'||coalesce(facts,''),2000),'actions',(SELECT coalesce(jsonb_agg(jsonb_build_object('id','offer:'||(card->>'variant_id'),'title',left(card->>'sku',20))),'[]') FROM (SELECT value card FROM jsonb_array_elements(NEW.data->'cards') LIMIT 3) cards));END IF;
 INSERT INTO platform.channel_outbox(id,organization_id,integration_id,conversation_id,external_user_id,payload,role,status) VALUES(NEW.id,NEW.organization_id,p.integration_id,p.conversation_id,p.external_user_id,body,NEW.role,CASE WHEN NOT i.sending_enabled THEN 'cancelled' WHEN p.window_until<=now() THEN 'needs_approved_template' ELSE 'pending' END) ON CONFLICT(id) DO NOTHING;RETURN NEW;END $$;
CREATE TRIGGER channel_message_outbox AFTER INSERT ON platform.messages FOR EACH ROW EXECUTE FUNCTION platform.channel_message_outbox();
CREATE FUNCTION platform.channel_claim_send(p_proof bytea,p_cap uuid) RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 DECLARE q platform.channel_outbox; BEGIN
 UPDATE platform.channel_outbox x SET status='delivery_unknown',capability=NULL,version=version+1 WHERE status='sending' AND lease_until<now() AND EXISTS(SELECT 1 FROM platform.channel_integrations i WHERE i.id=x.integration_id AND i.worker_hash=sha256(p_proof));
 UPDATE platform.channel_outbox x SET status='needs_approved_template',version=version+1 WHERE status='pending' AND EXISTS(SELECT 1 FROM platform.channel_principals p JOIN platform.channel_integrations i ON i.id=p.integration_id WHERE p.conversation_id=x.conversation_id AND p.window_until<=now() AND i.worker_hash=sha256(p_proof));
 SELECT x.* INTO q FROM platform.channel_outbox x JOIN platform.channel_integrations i ON i.id=x.integration_id JOIN platform.channel_principals p ON p.integration_id=x.integration_id AND p.conversation_id=x.conversation_id JOIN platform.conversations c ON c.id=x.conversation_id WHERE i.active AND i.sending_enabled AND i.mode='mock' AND i.worker_hash=sha256(p_proof) AND p.window_until>now() AND c.status='open' AND c.expires_at>now() AND (x.role='staff' OR c.control_mode='agent') AND x.status='pending' AND x.attempts<5 AND x.available_at<=now() AND NOT EXISTS(SELECT 1 FROM platform.channel_outbox older WHERE older.integration_id=x.integration_id AND older.external_user_id=x.external_user_id AND older.status IN ('pending','sending','delivery_unknown') AND (older.created_at,older.id)<(x.created_at,x.id)) ORDER BY x.created_at,x.id LIMIT 1 FOR UPDATE OF x SKIP LOCKED;
 IF q.id IS NULL THEN RETURN NULL;END IF;
 UPDATE platform.channel_outbox SET status='sending',attempts=attempts+1,capability=p_cap,lease_until=now()+interval '15 seconds',version=version+1 WHERE id=q.id;
 RETURN jsonb_build_object('id',q.id,'organization_id',q.organization_id,'integration_id',q.integration_id,'conversation_id',q.conversation_id,'external_user_id',q.external_user_id,'text',q.payload->>'text','actions',q.payload->'actions','attempts',q.attempts+1);END $$;
CREATE FUNCTION platform.channel_admit_send(p_cap uuid) RETURNS boolean LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ SELECT EXISTS(SELECT 1 FROM platform.channel_outbox x JOIN platform.channel_integrations i ON i.id=x.integration_id JOIN platform.channel_principals p ON p.conversation_id=x.conversation_id JOIN platform.conversations c ON c.id=x.conversation_id WHERE x.capability=p_cap AND x.status='sending' AND x.lease_until>now()+interval '6 seconds' AND i.active AND i.sending_enabled AND i.mode='mock' AND p.window_until>now() AND c.status='open' AND c.expires_at>now() AND (x.role='staff' OR c.control_mode='agent')) $$;
CREATE FUNCTION platform.channel_finish_send(p_cap uuid,p_state text,p_delay integer,p_provider text) RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ BEGIN
 IF p_state NOT IN ('sent','pending','failed','delivery_unknown','cancelled') OR p_delay<0 OR p_delay>3600 OR length(p_provider)>200 THEN RAISE EXCEPTION 'invalid result';END IF;
 UPDATE platform.channel_outbox SET status=CASE WHEN p_state='pending' AND attempts>=5 THEN 'failed' ELSE p_state END,provider_id=p_provider,capability=NULL,lease_until=NULL,version=version+1,available_at=now()+make_interval(secs=>p_delay) WHERE capability=p_cap AND status='sending' AND lease_until>now(); END $$;
CREATE FUNCTION platform.channel_delivery_audit() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ BEGIN
 IF TG_OP='INSERT' OR NEW.status IS DISTINCT FROM OLD.status OR NEW.attempts IS DISTINCT FROM OLD.attempts THEN INSERT INTO platform.channel_events(organization_id,integration_id,outbox_id,kind,actor_id) VALUES(NEW.organization_id,NEW.integration_id,NEW.id,NEW.status,platform.current_user_id());END IF;RETURN NEW;END $$;
CREATE TRIGGER channel_delivery_audit AFTER INSERT OR UPDATE ON platform.channel_outbox FOR EACH ROW EXECUTE FUNCTION platform.channel_delivery_audit();
CREATE FUNCTION platform.channel_control_fence() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ DECLARE i uuid;BEGIN
 IF NEW.channel='whatsapp' AND (NEW.control_mode IS DISTINCT FROM OLD.control_mode OR NEW.status IS DISTINCT FROM OLD.status) THEN
 SELECT integration_id INTO i FROM platform.channel_principals WHERE conversation_id=NEW.id;IF i IS NOT NULL THEN
 PERFORM pg_advisory_xact_lock(hashtextextended(i::text,8210));
 UPDATE platform.channel_outbox SET status='cancelled',capability=NULL,version=version+1 WHERE conversation_id=NEW.id AND role='assistant' AND status IN ('pending','needs_approved_template');
 IF NEW.status='closed' THEN UPDATE platform.channel_inbox SET payload='{}',status=CASE WHEN status IN ('pending','processing') THEN 'cancelled' ELSE status END,capability=NULL WHERE integration_id=i AND external_user_id=(SELECT external_user_id FROM platform.channel_principals WHERE conversation_id=NEW.id);UPDATE platform.channel_outbox SET payload='{}',status=CASE WHEN status IN ('pending','needs_approved_template') THEN 'cancelled' ELSE status END WHERE conversation_id=NEW.id;END IF;
 END IF;END IF;RETURN NEW;END $$;
CREATE TRIGGER channel_control_fence AFTER UPDATE OF control_mode,status ON platform.conversations FOR EACH ROW EXECUTE FUNCTION platform.channel_control_fence();
CREATE FUNCTION platform.channel_purge(p_proof bytea) RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ BEGIN
 UPDATE platform.conversations c SET status='closed',control_mode='human',in_flight=NULL,lease_until=NULL,version=version+1 WHERE expires_at<=now() AND status='open' AND EXISTS(SELECT 1 FROM platform.channel_principals p JOIN platform.channel_integrations i ON i.id=p.integration_id WHERE p.conversation_id=c.id AND i.worker_hash=sha256(p_proof));
 UPDATE platform.messages m SET content='',data='{}',request_hash=NULL WHERE EXISTS(SELECT 1 FROM platform.channel_principals p JOIN platform.channel_integrations i ON i.id=p.integration_id JOIN platform.conversations c ON c.id=p.conversation_id WHERE c.id=m.conversation_id AND (c.expires_at<=now() OR c.status='closed') AND i.worker_hash=sha256(p_proof));
 UPDATE platform.channel_inbox q SET payload='{}',status=CASE WHEN status IN ('pending','processing') THEN 'cancelled' ELSE status END,capability=NULL WHERE EXISTS(SELECT 1 FROM platform.channel_integrations i WHERE i.id=q.integration_id AND i.worker_hash=sha256(p_proof) AND q.created_at<now()-make_interval(days=>i.retention_days));
 END $$;
CREATE FUNCTION platform.channel_resolve(p_org uuid,p_id uuid,p_version bigint,p_state text) RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ BEGIN
 IF p_org::text IS DISTINCT FROM current_setting('app.organization_id',true) OR NOT platform.is_test_org(p_org) OR NOT platform.can_operate_orders(p_org) THEN RAISE EXCEPTION 'forbidden' USING ERRCODE='42501';END IF;
 IF NOT EXISTS(SELECT 1 FROM platform.channel_outbox WHERE id=p_id AND organization_id=p_org) THEN RAISE EXCEPTION 'not found' USING ERRCODE='P0002';END IF;
 IF p_state NOT IN ('sent','cancelled') THEN RAISE EXCEPTION 'invalid resolution' USING ERRCODE='23514';END IF;
 UPDATE platform.channel_outbox SET status=p_state,version=version+1 WHERE id=p_id AND organization_id=p_org AND version=p_version AND status='delivery_unknown';IF NOT FOUND THEN RAISE EXCEPTION 'version conflict' USING ERRCODE='40001';END IF; END $$;
DO $$ DECLARE f record; BEGIN FOR f IN SELECT p.oid::regprocedure sig,p.proname FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='platform' AND (p.proname LIKE 'channel_%' OR p.proname='external_channel_scope') LOOP
 EXECUTE format('REVOKE ALL ON FUNCTION %s FROM PUBLIC',f.sig);
 IF f.proname NOT IN ('channel_order_source','channel_message_outbox','channel_delivery_audit','channel_control_fence') THEN EXECUTE format('GRANT EXECUTE ON FUNCTION %s TO aba_app',f.sig);END IF;
 END LOOP;END $$;
UPDATE platform.service_metadata SET version=10 WHERE singleton;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN IF EXISTS(SELECT 1 FROM platform.channel_integrations) OR EXISTS(SELECT 1 FROM platform.conversations WHERE channel='whatsapp') THEN RAISE EXCEPTION 'rollback refused: preserve channel data';END IF;END $$;
DROP TRIGGER channel_control_fence ON platform.conversations;
DROP TRIGGER channel_message_outbox ON platform.messages;
DROP TRIGGER channel_order_source ON platform.orders;
DO $$ DECLARE x record;BEGIN FOR x IN SELECT definition FROM platform.channel_function_backup LOOP EXECUTE x.definition;END LOOP;
 FOR x IN SELECT polname,relname FROM pg_policy p JOIN pg_class c ON c.oid=p.polrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='platform' AND polname LIKE 'whatsapp_%' LOOP EXECUTE format('DROP POLICY %I ON platform.%I',x.polname,x.relname);END LOOP;END $$;
DROP TABLE platform.channel_events,platform.channel_outbox,platform.channel_inbox,platform.channel_principals,platform.channel_integrations,platform.channel_function_backup;
DO $$ DECLARE f record;BEGIN FOR f IN SELECT p.oid::regprocedure sig FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='platform' AND (p.proname LIKE 'channel_%' OR p.proname='external_channel_scope') LOOP EXECUTE format('DROP FUNCTION %s',f.sig);END LOOP;END $$;
ALTER TABLE platform.conversations DROP CONSTRAINT conversations_channel_check;
ALTER TABLE platform.conversations ADD CHECK(channel IN ('internal_chat','telegram'));
ALTER TABLE platform.orders DROP CONSTRAINT orders_source_check;
ALTER TABLE platform.orders ADD CHECK(source IN ('internal_chat','telegram'));
UPDATE platform.service_metadata SET version=9 WHERE singleton;
-- +goose StatementEnd
