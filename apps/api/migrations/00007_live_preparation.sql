-- +goose Up
-- +goose StatementBegin
CREATE TABLE platform.llm_platform_budget(singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),limit_microrub bigint NOT NULL DEFAULT 0 CHECK(limit_microrub>=0),spent_microrub bigint NOT NULL DEFAULT 0 CHECK(spent_microrub>=0),reserved_microrub bigint NOT NULL DEFAULT 0 CHECK(reserved_microrub>=0),active_calls integer NOT NULL DEFAULT 0 CHECK(active_calls>=0),kill_switch boolean NOT NULL DEFAULT true);
INSERT INTO platform.llm_platform_budget(singleton) VALUES(true);
CREATE TABLE platform.ai_settings(organization_id uuid PRIMARY KEY REFERENCES platform.organizations(id),auto_enabled boolean NOT NULL DEFAULT true,live_enabled boolean NOT NULL DEFAULT false,request_limit bigint NOT NULL DEFAULT 0 CHECK(request_limit BETWEEN 0 AND 1000000000000),dialog_limit bigint NOT NULL DEFAULT 0 CHECK(dialog_limit BETWEEN 0 AND 1000000000000),day_limit bigint NOT NULL DEFAULT 0 CHECK(day_limit BETWEEN 0 AND 1000000000000),month_limit bigint NOT NULL DEFAULT 0 CHECK(month_limit BETWEEN 0 AND 1000000000000),version bigint NOT NULL DEFAULT 1 CHECK(version>0),updated_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE platform.llm_usage(organization_id uuid NOT NULL REFERENCES platform.organizations(id),window_key text NOT NULL,spent bigint NOT NULL DEFAULT 0 CHECK(spent>=0),reserved bigint NOT NULL DEFAULT 0 CHECK(reserved>=0),input_tokens bigint NOT NULL DEFAULT 0 CHECK(input_tokens>=0),output_tokens bigint NOT NULL DEFAULT 0 CHECK(output_tokens>=0),requests integer NOT NULL DEFAULT 0 CHECK(requests>=0),PRIMARY KEY(organization_id,window_key));
CREATE TABLE platform.llm_reservations(id uuid PRIMARY KEY,organization_id uuid NOT NULL,conversation_id uuid,amount bigint NOT NULL CHECK(amount>0),input_cap integer NOT NULL CHECK(input_cap>0),output_cap integer NOT NULL CHECK(output_cap>0),attempts integer NOT NULL CHECK(attempts BETWEEN 1 AND 3),rate_input bigint NOT NULL CHECK(rate_input>0),rate_output bigint NOT NULL CHECK(rate_output>0),usage_input integer,usage_output integer,charged bigint,status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','settled','uncertain')),day_key text NOT NULL,month_key text NOT NULL,model text NOT NULL CHECK(length(model) BETWEEN 1 AND 128),created_at timestamptz NOT NULL DEFAULT now(),completed_at timestamptz,FOREIGN KEY(organization_id,conversation_id) REFERENCES platform.conversations(organization_id,id));
CREATE INDEX llm_reservation_pending ON platform.llm_reservations(status,created_at);
CREATE TABLE platform.agent_metrics(organization_id uuid NOT NULL REFERENCES platform.organizations(id),mode text NOT NULL CHECK(mode IN ('mock','live')),cycles bigint NOT NULL DEFAULT 0,successes bigint NOT NULL DEFAULT 0,errors bigint NOT NULL DEFAULT 0,tool_calls bigint NOT NULL DEFAULT 0,total_latency_ms bigint NOT NULL DEFAULT 0,PRIMARY KEY(organization_id,mode));
CREATE TABLE platform.ai_settings_events(id uuid PRIMARY KEY,organization_id uuid NOT NULL REFERENCES platform.organizations(id),actor_id uuid NOT NULL REFERENCES platform.users(id),kind text NOT NULL CHECK(kind IN ('settings_changed','test_data_deleted')),created_at timestamptz NOT NULL DEFAULT now());
CREATE FUNCTION platform.llm_authorized(p_org uuid,p_conv uuid) RETURNS boolean LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT (p_org::text=current_setting('app.organization_id',true) AND platform.is_test_org(p_org) AND platform.can_operate_orders(p_org) AND (p_conv IS NULL OR EXISTS(SELECT 1 FROM platform.conversations WHERE organization_id=p_org AND id=p_conv))) OR platform.telegram_scope(p_org,p_conv) $$;
ALTER TABLE platform.ai_settings ENABLE ROW LEVEL SECURITY;ALTER TABLE platform.ai_settings FORCE ROW LEVEL SECURITY;
CREATE POLICY migration_owner ON platform.ai_settings TO aba_migrator USING(true) WITH CHECK(true);
CREATE POLICY scoped_read ON platform.ai_settings FOR SELECT TO aba_app USING(platform.llm_authorized(organization_id,NULL));
GRANT SELECT ON platform.ai_settings TO aba_app;
ALTER TABLE platform.llm_usage ENABLE ROW LEVEL SECURITY;ALTER TABLE platform.llm_usage FORCE ROW LEVEL SECURITY;
CREATE POLICY migration_owner ON platform.llm_usage TO aba_migrator USING(true) WITH CHECK(true);
CREATE POLICY scoped_read ON platform.llm_usage FOR SELECT TO aba_app USING(platform.llm_authorized(organization_id,NULL));
GRANT SELECT ON platform.llm_usage TO aba_app;
ALTER TABLE platform.llm_reservations ENABLE ROW LEVEL SECURITY;ALTER TABLE platform.llm_reservations FORCE ROW LEVEL SECURITY;
CREATE POLICY migration_owner ON platform.llm_reservations TO aba_migrator USING(true) WITH CHECK(true);
CREATE POLICY scoped_read ON platform.llm_reservations FOR SELECT TO aba_app USING(platform.llm_authorized(organization_id,NULL));
GRANT SELECT ON platform.llm_reservations TO aba_app;
ALTER TABLE platform.agent_metrics ENABLE ROW LEVEL SECURITY;ALTER TABLE platform.agent_metrics FORCE ROW LEVEL SECURITY;
CREATE POLICY migration_owner ON platform.agent_metrics TO aba_migrator USING(true) WITH CHECK(true);
CREATE POLICY scoped_read ON platform.agent_metrics FOR SELECT TO aba_app USING(platform.llm_authorized(organization_id,NULL));
GRANT SELECT ON platform.agent_metrics TO aba_app;
ALTER TABLE platform.ai_settings_events ENABLE ROW LEVEL SECURITY;ALTER TABLE platform.ai_settings_events FORCE ROW LEVEL SECURITY;
CREATE POLICY migration_owner ON platform.ai_settings_events TO aba_migrator USING(true) WITH CHECK(true);
CREATE POLICY scoped_read ON platform.ai_settings_events FOR SELECT TO aba_app USING(platform.llm_authorized(organization_id,NULL));
GRANT SELECT ON platform.ai_settings_events TO aba_app;
CREATE POLICY metric_write ON platform.agent_metrics FOR INSERT TO aba_app WITH CHECK(platform.llm_authorized(organization_id,NULL));
CREATE POLICY metric_update ON platform.agent_metrics FOR UPDATE TO aba_app USING(platform.llm_authorized(organization_id,NULL)) WITH CHECK(platform.llm_authorized(organization_id,NULL));
GRANT INSERT,UPDATE ON platform.agent_metrics TO aba_app;
CREATE FUNCTION platform.ai_set_settings(p_org uuid,p_request bigint,p_dialog bigint,p_day bigint,p_month bigint,p_live boolean,p_auto boolean,p_sending boolean,p_version bigint,p_event uuid) RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 DECLARE cap bigint; current_version bigint; BEGIN
 IF p_org::text IS DISTINCT FROM current_setting('app.organization_id',true) OR NOT platform.is_test_org(p_org) OR NOT platform.has_membership(p_org,true) THEN RAISE EXCEPTION 'forbidden' USING ERRCODE='42501';END IF;
 SELECT limit_microrub INTO cap FROM platform.llm_platform_budget WHERE singleton FOR UPDATE;
 IF least(p_request,p_dialog,p_day,p_month)<0 OR greatest(p_request,p_dialog,p_day,p_month)>cap OR (p_live AND least(p_request,p_dialog,p_day,p_month)=0) THEN RAISE EXCEPTION 'budget not approved' USING ERRCODE='23514';END IF;
 INSERT INTO platform.ai_settings(organization_id) VALUES(p_org) ON CONFLICT DO NOTHING;
 SELECT version INTO current_version FROM platform.ai_settings WHERE organization_id=p_org FOR UPDATE;
 IF p_version<>current_version THEN RAISE EXCEPTION 'version conflict' USING ERRCODE='40001';END IF;
 UPDATE platform.ai_settings SET request_limit=p_request,dialog_limit=p_dialog,day_limit=p_day,month_limit=p_month,live_enabled=p_live,auto_enabled=p_auto,telegram_sending_enabled=p_sending,version=version+1,updated_at=now() WHERE organization_id=p_org;
 IF NOT p_auto THEN UPDATE platform.conversations SET control_mode='human',in_flight=NULL,lease_until=NULL,version=version+1,updated_at=now() WHERE organization_id=p_org AND status='open' AND control_mode='agent';END IF;
 INSERT INTO platform.ai_settings_events(id,organization_id,actor_id,kind) VALUES(p_event,p_org,platform.current_user_id(),'settings_changed');
 END $$;
CREATE FUNCTION platform.llm_reserve(p_org uuid,p_conv uuid,p_id uuid,p_amount bigint,p_in integer,p_out integer,p_attempts integer,p_rate_in bigint,p_rate_out bigint,p_model text,p_global_cap bigint,p_parallel integer,p_dialog_in integer,p_dialog_out integer,p_requests integer) RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 DECLARE global_row platform.llm_platform_budget; cfg platform.ai_settings; bucket platform.llm_usage; key text; ceiling bigint; day_key text; month_key text; BEGIN
 IF NOT coalesce(platform.llm_authorized(p_org,p_conv),false) THEN RAISE EXCEPTION 'forbidden' USING ERRCODE='42501';END IF;
 IF p_amount<=0 OR p_amount<>ceil((p_in::numeric*p_attempts*p_rate_in+p_out::numeric*p_attempts*p_rate_out)/1000000)::bigint OR p_in NOT BETWEEN 1 AND 32000 OR p_out NOT BETWEEN 1 AND 32000 OR p_attempts NOT BETWEEN 1 AND 3 OR p_rate_in<=0 OR p_rate_out<=0 OR p_parallel NOT BETWEEN 1 AND 16 OR p_global_cap<=0 THEN RAISE EXCEPTION 'live budget denied' USING ERRCODE='23514';END IF;
 SELECT * INTO global_row FROM platform.llm_platform_budget WHERE singleton FOR UPDATE;
 IF global_row.kill_switch OR global_row.spent_microrub+global_row.reserved_microrub+p_amount>least(global_row.limit_microrub,p_global_cap) OR global_row.active_calls>=p_parallel THEN RAISE EXCEPTION 'platform budget denied' USING ERRCODE='23514';END IF;
 SELECT * INTO cfg FROM platform.ai_settings WHERE organization_id=p_org FOR UPDATE;
 IF cfg.organization_id IS NULL OR NOT cfg.live_enabled OR NOT cfg.auto_enabled OR p_amount>cfg.request_limit THEN RAISE EXCEPTION 'organization budget denied' USING ERRCODE='23514';END IF;
 day_key='day:'||to_char(now() AT TIME ZONE 'UTC','YYYY-MM-DD');month_key='month:'||to_char(now() AT TIME ZONE 'UTC','YYYY-MM');
 FOREACH key IN ARRAY ARRAY['dialog:'||p_conv::text,day_key,month_key] LOOP
 INSERT INTO platform.llm_usage(organization_id,window_key) VALUES(p_org,key) ON CONFLICT DO NOTHING;
 SELECT * INTO bucket FROM platform.llm_usage WHERE organization_id=p_org AND window_key=key FOR UPDATE;
 ceiling=CASE WHEN key=day_key THEN cfg.day_limit WHEN key=month_key THEN cfg.month_limit ELSE cfg.dialog_limit END;
 IF bucket.spent+bucket.reserved+p_amount>ceiling THEN RAISE EXCEPTION 'budget exhausted' USING ERRCODE='23514';END IF;
 IF key='dialog:'||p_conv::text AND (bucket.input_tokens+p_in*p_attempts>p_dialog_in OR bucket.output_tokens+p_out*p_attempts>p_dialog_out OR bucket.requests+p_attempts>p_requests) THEN RAISE EXCEPTION 'dialog token quota' USING ERRCODE='23514';END IF;
 UPDATE platform.llm_usage SET reserved=reserved+p_amount,input_tokens=input_tokens+p_in*p_attempts,output_tokens=output_tokens+p_out*p_attempts,requests=requests+p_attempts WHERE organization_id=p_org AND window_key=key;
 END LOOP;
 INSERT INTO platform.llm_reservations(id,organization_id,conversation_id,amount,input_cap,output_cap,attempts,rate_input,rate_output,model,day_key,month_key) VALUES(p_id,p_org,p_conv,p_amount,p_in,p_out,p_attempts,p_rate_in,p_rate_out,p_model,day_key,month_key);
 UPDATE platform.llm_platform_budget SET reserved_microrub=reserved_microrub+p_amount,active_calls=active_calls+1 WHERE singleton;
 END $$;
CREATE FUNCTION platform.llm_settle(p_org uuid,p_id uuid,p_in integer,p_out integer,p_known boolean) RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 DECLARE entry platform.llm_reservations; cost bigint; actual_in integer; actual_out integer; key text; BEGIN
 IF NOT coalesce(platform.llm_authorized(p_org,NULL),false) THEN RAISE EXCEPTION 'forbidden' USING ERRCODE='42501';END IF;
 PERFORM 1 FROM platform.llm_platform_budget WHERE singleton FOR UPDATE;
 SELECT * INTO entry FROM platform.llm_reservations WHERE id=p_id AND organization_id=p_org FOR UPDATE;
 IF entry.id IS NULL OR NOT coalesce(platform.llm_authorized(p_org,entry.conversation_id),false) THEN RAISE EXCEPTION 'forbidden' USING ERRCODE='42501';END IF;
 IF entry.status<>'pending' THEN RETURN;END IF;
 IF p_known AND p_in BETWEEN 0 AND entry.input_cap*entry.attempts AND p_out BETWEEN 0 AND entry.output_cap*entry.attempts THEN
 actual_in=p_in;actual_out=p_out;cost=ceil((p_in::numeric*entry.rate_input+p_out::numeric*entry.rate_output)/1000000)::bigint;
 ELSE p_known=false;actual_in=entry.input_cap*entry.attempts;actual_out=entry.output_cap*entry.attempts;cost=entry.amount;END IF;
 IF cost>entry.amount THEN p_known=false;cost=entry.amount;actual_in=entry.input_cap*entry.attempts;actual_out=entry.output_cap*entry.attempts;END IF;
 FOREACH key IN ARRAY ARRAY['dialog:'||entry.conversation_id::text,entry.day_key,entry.month_key] LOOP
 UPDATE platform.llm_usage SET reserved=reserved-entry.amount,spent=spent+cost,input_tokens=input_tokens-entry.input_cap*entry.attempts+actual_in,output_tokens=output_tokens-entry.output_cap*entry.attempts+actual_out WHERE organization_id=p_org AND window_key=key;
 END LOOP;
 UPDATE platform.llm_platform_budget SET reserved_microrub=reserved_microrub-entry.amount,spent_microrub=spent_microrub+cost,active_calls=active_calls-1 WHERE singleton;
 UPDATE platform.llm_reservations SET status=CASE WHEN p_known THEN 'settled' ELSE 'uncertain' END,charged=cost,usage_input=actual_in,usage_output=actual_out,completed_at=now() WHERE id=p_id;
 END $$;
CREATE FUNCTION platform.llm_platform_state() RETURNS jsonb LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ SELECT jsonb_build_object('limit_microrub',limit_microrub,'kill_switch',kill_switch) FROM platform.llm_platform_budget WHERE singleton $$;
REVOKE ALL ON FUNCTION platform.llm_authorized(uuid,uuid) FROM PUBLIC;GRANT EXECUTE ON FUNCTION platform.llm_authorized(uuid,uuid) TO aba_app;
REVOKE ALL ON FUNCTION platform.ai_set_settings(uuid,bigint,bigint,bigint,bigint,boolean,boolean,boolean,bigint,uuid) FROM PUBLIC;GRANT EXECUTE ON FUNCTION platform.ai_set_settings(uuid,bigint,bigint,bigint,bigint,boolean,boolean,boolean,bigint,uuid) TO aba_app;
REVOKE ALL ON FUNCTION platform.llm_reserve(uuid,uuid,uuid,bigint,integer,integer,integer,bigint,bigint,text,bigint,integer,integer,integer,integer) FROM PUBLIC;GRANT EXECUTE ON FUNCTION platform.llm_reserve(uuid,uuid,uuid,bigint,integer,integer,integer,bigint,bigint,text,bigint,integer,integer,integer,integer) TO aba_app;
REVOKE ALL ON FUNCTION platform.llm_settle(uuid,uuid,integer,integer,boolean) FROM PUBLIC;GRANT EXECUTE ON FUNCTION platform.llm_settle(uuid,uuid,integer,integer,boolean) TO aba_app;
REVOKE ALL ON FUNCTION platform.llm_platform_state() FROM PUBLIC;GRANT EXECUTE ON FUNCTION platform.llm_platform_state() TO aba_app;
ALTER TABLE platform.ai_settings ADD COLUMN telegram_sending_enabled boolean NOT NULL DEFAULT false;
ALTER TABLE platform.telegram_integrations DROP CONSTRAINT telegram_integrations_mode_check;
ALTER TABLE platform.telegram_integrations ADD CHECK(mode IN ('mock','live'));
ALTER TABLE platform.telegram_integrations ADD COLUMN webhook_state text NOT NULL DEFAULT 'synthetic_only' CHECK(webhook_state IN ('synthetic_only','not_set','configured','disabled')),ADD COLUMN webhook_secret_cipher bytea;
GRANT SELECT(webhook_state) ON platform.telegram_integrations TO aba_app;
DROP FUNCTION platform.telegram_claim(uuid,uuid,uuid,uuid,integer,bytea);
DROP FUNCTION platform.telegram_claim_send(uuid,bytea);
DROP FUNCTION platform.telegram_connect(uuid,uuid,bigint,text,text,bytea,bytea,bytea);
CREATE FUNCTION platform.telegram_claim(p_cap uuid,p_conv uuid,p_customer uuid,p_principal uuid,p_days integer,p_worker bytea,p_allowed bigint[],p_mode text) RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 DECLARE q platform.telegram_inbox; conv uuid; BEGIN
 UPDATE platform.telegram_inbox expired_job SET status='failed',capability=NULL,last_error='RETRY_EXHAUSTED' WHERE status='processing' AND lease_until<now() AND attempts>=5 AND EXISTS(SELECT 1 FROM platform.telegram_integrations i WHERE i.id=expired_job.integration_id AND i.worker_hash=sha256(p_worker));
 IF p_days<1 OR p_days>30 THEN RAISE EXCEPTION 'retention' USING ERRCODE='22003'; END IF;
 SELECT x.* INTO q FROM platform.telegram_inbox x JOIN platform.telegram_integrations i ON i.id=x.integration_id
 WHERE i.mode=p_mode AND (p_mode='mock' AND p_allowed IS NULL OR x.user_id=ANY(p_allowed)) AND i.active AND i.worker_hash=sha256(p_worker) AND x.attempts<5 AND ((x.status='pending' AND x.available_at<=now()) OR (x.status='processing' AND x.lease_until<now()))
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
CREATE FUNCTION platform.telegram_claim_send(p_cap uuid,p_worker bytea,p_allowed bigint[],p_mode text) RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 DECLARE q platform.telegram_outbox; cipher bytea; BEGIN
 UPDATE platform.telegram_outbox expired_job SET status='uncertain',last_error='DELIVERY_UNKNOWN',capability=NULL WHERE status='sending' AND lease_until<now() AND EXISTS(SELECT 1 FROM platform.telegram_integrations i WHERE i.id=expired_job.integration_id AND i.worker_hash=sha256(p_worker));
 SELECT x.* INTO q FROM platform.telegram_outbox x JOIN platform.telegram_integrations i ON i.id=x.integration_id
 WHERE i.mode=p_mode AND (p_mode='mock' AND p_allowed IS NULL OR x.chat_id=ANY(p_allowed)) AND (p_mode='mock' OR EXISTS(SELECT 1 FROM platform.ai_settings settings WHERE settings.organization_id=x.organization_id AND settings.telegram_sending_enabled)) AND i.active AND i.worker_hash=sha256(p_worker) AND x.status='pending' AND x.available_at<=now() AND x.attempts<5
 AND NOT EXISTS(SELECT 1 FROM platform.telegram_outbox older WHERE older.integration_id=x.integration_id AND older.chat_id=x.chat_id AND older.status IN ('pending','sending','uncertain') AND (older.created_at,older.id)<(x.created_at,x.id))
 ORDER BY x.created_at,x.id LIMIT 1 FOR UPDATE OF x SKIP LOCKED;
 IF q.id IS NULL THEN RETURN NULL;END IF;
 SELECT token_cipher INTO cipher FROM platform.telegram_integrations WHERE id=q.integration_id;
 UPDATE platform.telegram_outbox SET status='sending',attempts=attempts+1,lease_until=now()+interval '15 seconds',capability=p_cap WHERE id=q.id;
 RETURN jsonb_build_object('id',q.id,'attempts',q.attempts+1,'organization_id',q.organization_id,'capability',p_cap,'chat_id',q.chat_id,'payload',q.payload,'token_cipher',encode(cipher,'hex'));
 END $$;
CREATE FUNCTION platform.telegram_connect(p_id uuid,p_org uuid,p_bot bigint,p_name text,p_username text,p_cipher bytea,p_hash bytea,p_worker_hash bytea,p_mode text) RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ BEGIN
 IF p_mode NOT IN ('mock','live') THEN RAISE EXCEPTION 'invalid integration mode';END IF;
 IF NOT (p_org::text=current_setting('app.organization_id',true) AND platform.has_membership(p_org,true) AND platform.is_test_org(p_org)) THEN RAISE EXCEPTION 'forbidden' USING ERRCODE='42501';END IF;
 PERFORM pg_advisory_xact_lock(hashtextextended('telegram-config:'||p_org::text,0));
 IF EXISTS(SELECT 1 FROM platform.telegram_integrations WHERE organization_id=p_org AND bot_id<>p_bot) THEN RAISE EXCEPTION 'bot replacement' USING ERRCODE='23505';END IF;
 INSERT INTO platform.telegram_integrations(id,organization_id,bot_id,name,username,token_cipher,webhook_hash,worker_hash,mode,webhook_state) VALUES(p_id,p_org,p_bot,p_name,p_username,p_cipher,p_hash,p_worker_hash,p_mode,CASE WHEN p_mode='mock' THEN 'synthetic_only' ELSE 'not_set' END) ON CONFLICT(organization_id) DO UPDATE SET active=true,mode=excluded.mode,webhook_state=excluded.webhook_state,last_error=NULL,worker_hash=excluded.worker_hash,token_cipher=excluded.token_cipher,webhook_hash=excluded.webhook_hash,updated_at=now();END $$;
REVOKE ALL ON FUNCTION platform.telegram_claim(uuid,uuid,uuid,uuid,integer,bytea,bigint[],text) FROM PUBLIC;GRANT EXECUTE ON FUNCTION platform.telegram_claim(uuid,uuid,uuid,uuid,integer,bytea,bigint[],text) TO aba_app;
REVOKE ALL ON FUNCTION platform.telegram_claim_send(uuid,bytea,bigint[],text) FROM PUBLIC;GRANT EXECUTE ON FUNCTION platform.telegram_claim_send(uuid,bytea,bigint[],text) TO aba_app;
REVOKE ALL ON FUNCTION platform.telegram_connect(uuid,uuid,bigint,text,text,bytea,bytea,bytea,text) FROM PUBLIC;GRANT EXECUTE ON FUNCTION platform.telegram_connect(uuid,uuid,bigint,text,text,bytea,bytea,bytea,text) TO aba_app;
CREATE FUNCTION platform.telegram_save_webhook(p_org uuid,p_hash bytea,p_cipher bytea,p_state text) RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ BEGIN
 IF p_org::text IS DISTINCT FROM current_setting('app.organization_id',true) OR NOT platform.has_membership(p_org,true) OR NOT platform.is_test_org(p_org) THEN RAISE EXCEPTION 'forbidden' USING ERRCODE='42501';END IF;
 IF p_state NOT IN ('configured','disabled') THEN RAISE EXCEPTION 'invalid webhook state';END IF;
 UPDATE platform.telegram_integrations SET webhook_hash=coalesce(p_hash,webhook_hash),webhook_secret_cipher=p_cipher,webhook_state=p_state WHERE organization_id=p_org AND mode='live';END $$;
REVOKE ALL ON FUNCTION platform.telegram_save_webhook(uuid,bytea,bytea,text) FROM PUBLIC;GRANT EXECUTE ON FUNCTION platform.telegram_save_webhook(uuid,bytea,bytea,text) TO aba_app;
CREATE FUNCTION platform.purge_test_conversation(p_org uuid,p_conv uuid,p_version bigint,p_audit uuid) RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 DECLARE actual bigint; stock record; released bigint=0; orders_count integer; BEGIN
 IF p_org::text IS DISTINCT FROM current_setting('app.organization_id',true) OR NOT platform.is_test_org(p_org) OR NOT platform.has_membership(p_org,true) THEN RAISE EXCEPTION 'forbidden' USING ERRCODE='42501';END IF;
 PERFORM pg_advisory_xact_lock(hashtextextended('purge:'||p_org::text,0));
 SELECT version INTO actual FROM platform.conversations WHERE organization_id=p_org AND id=p_conv AND is_test FOR UPDATE;
 IF actual IS NULL THEN RAISE EXCEPTION 'not found' USING ERRCODE='P0002';END IF;
 IF actual<>p_version THEN RAISE EXCEPTION 'version conflict' USING ERRCODE='40001';END IF;
 IF EXISTS(SELECT 1 FROM platform.llm_reservations WHERE organization_id=p_org AND conversation_id=p_conv AND status='pending') OR EXISTS(SELECT 1 FROM platform.telegram_inbox WHERE organization_id=p_org AND conversation_id=p_conv AND status='processing' AND lease_until>now()) OR EXISTS(SELECT 1 FROM platform.telegram_outbox WHERE organization_id=p_org AND conversation_id=p_conv AND status='sending' AND lease_until>now()) OR EXISTS(SELECT 1 FROM platform.conversations WHERE id=p_conv AND lease_until>now() AND in_flight IS NOT NULL) THEN RAISE EXCEPTION 'active operation' USING ERRCODE='40001';END IF;
 PERFORM id FROM platform.orders WHERE organization_id=p_org AND conversation_id=p_conv ORDER BY id FOR UPDATE;
 SELECT count(*) INTO orders_count FROM platform.orders WHERE organization_id=p_org AND conversation_id=p_conv;
 FOR stock IN SELECT items.variant_id,sum(items.quantity)::integer qty FROM platform.order_items items JOIN platform.orders o ON o.organization_id=items.organization_id AND o.id=items.order_id WHERE o.organization_id=p_org AND o.conversation_id=p_conv AND o.status='confirmed' GROUP BY items.variant_id ORDER BY items.variant_id LOOP
 PERFORM 1 FROM platform.inventory WHERE organization_id=p_org AND variant_id=stock.variant_id FOR UPDATE;
 UPDATE platform.inventory SET quantity_reserved=quantity_reserved-stock.qty,version=version+1,updated_at=now() WHERE organization_id=p_org AND variant_id=stock.variant_id AND quantity_reserved>=stock.qty;
 IF NOT FOUND THEN RAISE EXCEPTION 'stock reconciliation required' USING ERRCODE='23514';END IF;released=released+stock.qty;
 END LOOP;
 DELETE FROM platform.inventory_movements m WHERE m.organization_id=p_org AND m.order_id IN (SELECT id FROM platform.orders WHERE organization_id=p_org AND conversation_id=p_conv);
 DELETE FROM platform.order_events e WHERE e.organization_id=p_org AND e.order_id IN (SELECT id FROM platform.orders WHERE organization_id=p_org AND conversation_id=p_conv);
 DELETE FROM platform.order_items items WHERE items.organization_id=p_org AND items.order_id IN (SELECT id FROM platform.orders WHERE organization_id=p_org AND conversation_id=p_conv);
 DELETE FROM platform.orders WHERE organization_id=p_org AND conversation_id=p_conv;
 DELETE FROM platform.order_quotes WHERE organization_id=p_org AND conversation_id=p_conv;
 DELETE FROM platform.telegram_outbox WHERE organization_id=p_org AND conversation_id=p_conv;
 DELETE FROM platform.telegram_inbox WHERE organization_id=p_org AND conversation_id=p_conv;
 DELETE FROM platform.telegram_principals WHERE organization_id=p_org AND conversation_id=p_conv;
 DELETE FROM platform.messages WHERE organization_id=p_org AND conversation_id=p_conv;
 DELETE FROM platform.conversation_events WHERE organization_id=p_org AND conversation_id=p_conv;
 UPDATE platform.llm_reservations SET conversation_id=NULL WHERE organization_id=p_org AND conversation_id=p_conv;
 DELETE FROM platform.llm_usage WHERE organization_id=p_org AND window_key='dialog:'||p_conv::text;
 DELETE FROM platform.conversations WHERE organization_id=p_org AND id=p_conv;
 INSERT INTO platform.ai_settings_events(id,organization_id,actor_id,kind) VALUES(p_audit,p_org,platform.current_user_id(),'test_data_deleted');
 RETURN jsonb_build_object('deleted',true,'orders_deleted',orders_count,'reserved_released',released,'financial_aggregates_retained',true);
 END $$;
REVOKE ALL ON FUNCTION platform.purge_test_conversation(uuid,uuid,bigint,uuid) FROM PUBLIC;GRANT EXECUTE ON FUNCTION platform.purge_test_conversation(uuid,uuid,bigint,uuid) TO aba_app;
UPDATE platform.service_metadata SET version=7 WHERE singleton;
-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN IF EXISTS(SELECT 1 FROM platform.telegram_integrations WHERE mode='live') OR EXISTS(SELECT 1 FROM platform.llm_reservations) OR EXISTS(SELECT 1 FROM platform.ai_settings) OR EXISTS(SELECT 1 FROM platform.agent_metrics) OR EXISTS(SELECT 1 FROM platform.ai_settings_events) THEN RAISE EXCEPTION 'stage7 data exists: rollback refused';END IF;END $$;
DROP FUNCTION platform.telegram_save_webhook(uuid,bytea,bytea,text);
DROP FUNCTION platform.telegram_claim(uuid,uuid,uuid,uuid,integer,bytea,bigint[],text);
DROP FUNCTION platform.telegram_claim_send(uuid,bytea,bigint[],text);
DROP FUNCTION platform.telegram_connect(uuid,uuid,bigint,text,text,bytea,bytea,bytea,text);
CREATE FUNCTION platform.telegram_claim(p_cap uuid,p_conv uuid,p_customer uuid,p_principal uuid,p_days integer,p_worker bytea) RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 DECLARE q platform.telegram_inbox; conv uuid; BEGIN
 UPDATE platform.telegram_inbox expired_job SET status='failed',capability=NULL,last_error='RETRY_EXHAUSTED' WHERE status='processing' AND lease_until<now() AND attempts>=5 AND EXISTS(SELECT 1 FROM platform.telegram_integrations i WHERE i.id=expired_job.integration_id AND i.worker_hash=sha256(p_worker));
 IF p_days<1 OR p_days>30 THEN RAISE EXCEPTION 'retention' USING ERRCODE='22003'; END IF;
 SELECT x.* INTO q FROM platform.telegram_inbox x JOIN platform.telegram_integrations i ON i.id=x.integration_id
 WHERE i.active AND i.worker_hash=sha256(p_worker) AND x.attempts<5 AND ((x.status='pending' AND x.available_at<=now()) OR (x.status='processing' AND x.lease_until<now()))
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
CREATE FUNCTION platform.telegram_claim_send(p_cap uuid,p_worker bytea) RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 DECLARE q platform.telegram_outbox; cipher bytea; BEGIN
 UPDATE platform.telegram_outbox expired_job SET status='uncertain',last_error='DELIVERY_UNKNOWN',capability=NULL WHERE status='sending' AND lease_until<now() AND EXISTS(SELECT 1 FROM platform.telegram_integrations i WHERE i.id=expired_job.integration_id AND i.worker_hash=sha256(p_worker));
 SELECT x.* INTO q FROM platform.telegram_outbox x JOIN platform.telegram_integrations i ON i.id=x.integration_id
 WHERE i.active AND i.worker_hash=sha256(p_worker) AND x.status='pending' AND x.available_at<=now() AND x.attempts<5
 AND NOT EXISTS(SELECT 1 FROM platform.telegram_outbox older WHERE older.integration_id=x.integration_id AND older.chat_id=x.chat_id AND older.status IN ('pending','sending','uncertain') AND (older.created_at,older.id)<(x.created_at,x.id))
 ORDER BY x.created_at,x.id LIMIT 1 FOR UPDATE OF x SKIP LOCKED;
 IF q.id IS NULL THEN RETURN NULL;END IF;
 SELECT token_cipher INTO cipher FROM platform.telegram_integrations WHERE id=q.integration_id;
 UPDATE platform.telegram_outbox SET status='sending',attempts=attempts+1,lease_until=now()+interval '15 seconds',capability=p_cap WHERE id=q.id;
 RETURN jsonb_build_object('id',q.id,'attempts',q.attempts+1,'organization_id',q.organization_id,'capability',p_cap,'chat_id',q.chat_id,'payload',q.payload,'token_cipher',encode(cipher,'hex'));
 END $$;
CREATE FUNCTION platform.telegram_connect(p_id uuid,p_org uuid,p_bot bigint,p_name text,p_username text,p_cipher bytea,p_hash bytea,p_worker_hash bytea) RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ BEGIN
 IF NOT (p_org::text=current_setting('app.organization_id',true) AND platform.has_membership(p_org,true) AND platform.is_test_org(p_org)) THEN RAISE EXCEPTION 'forbidden' USING ERRCODE='42501';END IF;
 PERFORM pg_advisory_xact_lock(hashtextextended('telegram-config:'||p_org::text,0));
 IF EXISTS(SELECT 1 FROM platform.telegram_integrations WHERE organization_id=p_org AND bot_id<>p_bot) THEN RAISE EXCEPTION 'bot replacement' USING ERRCODE='23505';END IF;
 INSERT INTO platform.telegram_integrations(id,organization_id,bot_id,name,username,token_cipher,webhook_hash,worker_hash) VALUES(p_id,p_org,p_bot,p_name,p_username,p_cipher,p_hash,p_worker_hash) ON CONFLICT(organization_id) DO UPDATE SET active=true,last_error=NULL,worker_hash=excluded.worker_hash,token_cipher=excluded.token_cipher,webhook_hash=excluded.webhook_hash,updated_at=now();END $$;
REVOKE ALL ON FUNCTION platform.telegram_claim(uuid,uuid,uuid,uuid,integer,bytea) FROM PUBLIC;GRANT EXECUTE ON FUNCTION platform.telegram_claim(uuid,uuid,uuid,uuid,integer,bytea) TO aba_app;
REVOKE ALL ON FUNCTION platform.telegram_claim_send(uuid,bytea) FROM PUBLIC;GRANT EXECUTE ON FUNCTION platform.telegram_claim_send(uuid,bytea) TO aba_app;
REVOKE ALL ON FUNCTION platform.telegram_connect(uuid,uuid,bigint,text,text,bytea,bytea,bytea) FROM PUBLIC;GRANT EXECUTE ON FUNCTION platform.telegram_connect(uuid,uuid,bigint,text,text,bytea,bytea,bytea) TO aba_app;
ALTER TABLE platform.telegram_integrations DROP COLUMN webhook_state,DROP COLUMN webhook_secret_cipher,DROP CONSTRAINT telegram_integrations_mode_check;
ALTER TABLE platform.telegram_integrations ADD CHECK(mode='mock');
DROP FUNCTION platform.purge_test_conversation(uuid,uuid,bigint,uuid);
DROP FUNCTION platform.ai_set_settings(uuid,bigint,bigint,bigint,bigint,boolean,boolean,boolean,bigint,uuid),platform.llm_reserve(uuid,uuid,uuid,bigint,integer,integer,integer,bigint,bigint,text,bigint,integer,integer,integer,integer),platform.llm_settle(uuid,uuid,integer,integer,boolean),platform.llm_platform_state();
DROP TABLE platform.ai_settings_events,platform.agent_metrics,platform.llm_reservations,platform.llm_usage,platform.ai_settings;
DROP FUNCTION platform.llm_authorized(uuid,uuid);
DROP TABLE platform.llm_platform_budget;
UPDATE platform.service_metadata SET version=6 WHERE singleton;
-- +goose StatementEnd
