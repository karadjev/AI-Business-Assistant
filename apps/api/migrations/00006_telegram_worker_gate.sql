-- +goose Up
-- +goose StatementBegin
-- Additive worker authentication: legacy integrations require owner reconnect.
ALTER TABLE platform.telegram_integrations ADD COLUMN worker_hash bytea CHECK(worker_hash IS NULL OR octet_length(worker_hash)=32);
DROP FUNCTION platform.telegram_claim(uuid,uuid,uuid,uuid,integer);
DROP FUNCTION platform.telegram_claim_send(uuid);
DROP FUNCTION platform.telegram_connect(uuid,uuid,bigint,text,text,bytea,bytea);
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
DROP FUNCTION platform.telegram_purge_expired();
CREATE FUNCTION platform.telegram_purge_expired(p_worker bytea) RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ BEGIN
 UPDATE platform.messages m SET content='',data='{}',request_hash=NULL WHERE EXISTS(SELECT 1 FROM platform.conversations c WHERE c.organization_id=m.organization_id AND c.id=m.conversation_id AND c.channel='telegram' AND c.expires_at<=now() AND c.status='open' AND EXISTS(SELECT 1 FROM platform.telegram_integrations i WHERE i.organization_id=c.organization_id AND i.worker_hash=sha256(p_worker)));
 UPDATE platform.conversations SET status='closed',control_mode='human',version=version+1,in_flight=NULL,lease_until=NULL WHERE channel='telegram' AND expires_at<=now() AND status='open' AND EXISTS(SELECT 1 FROM platform.telegram_integrations i WHERE i.organization_id=conversations.organization_id AND i.worker_hash=sha256(p_worker));
 END $$;
REVOKE ALL ON FUNCTION platform.telegram_purge_expired(bytea) FROM PUBLIC;GRANT EXECUTE ON FUNCTION platform.telegram_purge_expired(bytea) TO aba_app;
UPDATE platform.service_metadata SET version=6 WHERE singleton;
-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN IF EXISTS(SELECT 1 FROM platform.telegram_integrations) THEN RAISE EXCEPTION 'Telegram data exists: worker gate rollback refused';END IF;END $$;
DROP FUNCTION platform.telegram_claim(uuid,uuid,uuid,uuid,integer,bytea);
DROP FUNCTION platform.telegram_claim_send(uuid,bytea);
DROP FUNCTION platform.telegram_connect(uuid,uuid,bigint,text,text,bytea,bytea,bytea);
ALTER TABLE platform.telegram_integrations DROP COLUMN worker_hash;
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
CREATE FUNCTION platform.telegram_connect(p_id uuid,p_org uuid,p_bot bigint,p_name text,p_username text,p_cipher bytea,p_hash bytea) RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ BEGIN
 IF NOT (p_org::text=current_setting('app.organization_id',true) AND platform.has_membership(p_org,true) AND platform.is_test_org(p_org)) THEN RAISE EXCEPTION 'forbidden' USING ERRCODE='42501';END IF;
 PERFORM pg_advisory_xact_lock(hashtextextended('telegram-config:'||p_org::text,0));
 IF EXISTS(SELECT 1 FROM platform.telegram_integrations WHERE organization_id=p_org AND bot_id<>p_bot) THEN RAISE EXCEPTION 'bot replacement' USING ERRCODE='23505';END IF;
 INSERT INTO platform.telegram_integrations(id,organization_id,bot_id,name,username,token_cipher,webhook_hash) VALUES(p_id,p_org,p_bot,p_name,p_username,p_cipher,p_hash) ON CONFLICT(organization_id) DO UPDATE SET active=true,last_error=NULL,token_cipher=excluded.token_cipher,webhook_hash=excluded.webhook_hash,updated_at=now();END $$;
REVOKE ALL ON FUNCTION platform.telegram_claim(uuid,uuid,uuid,uuid,integer) FROM PUBLIC;GRANT EXECUTE ON FUNCTION platform.telegram_claim(uuid,uuid,uuid,uuid,integer) TO aba_app;
REVOKE ALL ON FUNCTION platform.telegram_claim_send(uuid) FROM PUBLIC;GRANT EXECUTE ON FUNCTION platform.telegram_claim_send(uuid) TO aba_app;
REVOKE ALL ON FUNCTION platform.telegram_connect(uuid,uuid,bigint,text,text,bytea,bytea) FROM PUBLIC;GRANT EXECUTE ON FUNCTION platform.telegram_connect(uuid,uuid,bigint,text,text,bytea,bytea) TO aba_app;
DROP FUNCTION platform.telegram_purge_expired(bytea);
CREATE FUNCTION platform.telegram_purge_expired() RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ BEGIN
 UPDATE platform.messages m SET content='',data='{}',request_hash=NULL WHERE EXISTS(SELECT 1 FROM platform.conversations c WHERE c.organization_id=m.organization_id AND c.id=m.conversation_id AND c.channel='telegram' AND c.expires_at<=now() AND c.status='open');
 UPDATE platform.conversations SET status='closed',control_mode='human',version=version+1,in_flight=NULL,lease_until=NULL WHERE channel='telegram' AND expires_at<=now() AND status='open';
 END $$;
REVOKE ALL ON FUNCTION platform.telegram_purge_expired() FROM PUBLIC;GRANT EXECUTE ON FUNCTION platform.telegram_purge_expired() TO aba_app;
UPDATE platform.service_metadata SET version=5 WHERE singleton;
-- +goose StatementEnd
