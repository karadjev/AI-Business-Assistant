-- +goose Up
-- +goose StatementBegin
ALTER TABLE platform.telegram_outbox ADD COLUMN version bigint NOT NULL DEFAULT 1 CHECK(version>0);
ALTER TABLE platform.telegram_outbox ADD CONSTRAINT telegram_outbox_tenant_id UNIQUE(organization_id,id);
ALTER TABLE platform.llm_reservations ADD CONSTRAINT llm_reservations_tenant_id UNIQUE(organization_id,id);
CREATE TABLE platform.telegram_delivery_events(
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,organization_id uuid NOT NULL,
 outbox_id uuid NOT NULL,attempt integer NOT NULL CHECK(attempt>=0),old_status text,new_status text NOT NULL,
 version bigint NOT NULL,actor_id uuid REFERENCES platform.users(id),decision text,request_key uuid,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(organization_id,outbox_id,request_key),
 FOREIGN KEY(organization_id,outbox_id) REFERENCES platform.telegram_outbox(organization_id,id) ON DELETE CASCADE);
CREATE TABLE platform.llm_budget_events(
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,organization_id uuid NOT NULL,reservation_id uuid NOT NULL,
 status text NOT NULL,amount bigint NOT NULL,charged bigint,actor_id uuid REFERENCES platform.users(id),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(organization_id,reservation_id) REFERENCES platform.llm_reservations(organization_id,id));
ALTER TABLE platform.telegram_delivery_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.telegram_delivery_events FORCE ROW LEVEL SECURITY;
CREATE POLICY migration_owner ON platform.telegram_delivery_events TO aba_migrator USING(true) WITH CHECK(true);
CREATE POLICY staff_read ON platform.telegram_delivery_events FOR SELECT TO aba_app USING(organization_id::text=current_setting('app.organization_id',true) AND platform.can_operate_orders(organization_id));
ALTER TABLE platform.llm_budget_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.llm_budget_events FORCE ROW LEVEL SECURITY;
CREATE POLICY migration_owner ON platform.llm_budget_events TO aba_migrator USING(true) WITH CHECK(true);
CREATE POLICY scoped_read ON platform.llm_budget_events FOR SELECT TO aba_app USING(platform.llm_authorized(organization_id,NULL));
GRANT SELECT ON platform.telegram_delivery_events,platform.llm_budget_events TO aba_app;
CREATE FUNCTION platform.telegram_delivery_version() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,platform,pg_temp AS $$ BEGIN NEW.version=OLD.version+1;RETURN NEW;END $$;
CREATE TRIGGER delivery_version BEFORE UPDATE ON platform.telegram_outbox FOR EACH ROW EXECUTE FUNCTION platform.telegram_delivery_version();
CREATE FUNCTION platform.telegram_delivery_audit() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ BEGIN
 IF TG_OP='INSERT' OR OLD.status IS DISTINCT FROM NEW.status THEN
 INSERT INTO platform.telegram_delivery_events(organization_id,outbox_id,attempt,old_status,new_status,version) VALUES(NEW.organization_id,NEW.id,NEW.attempts,CASE WHEN TG_OP='INSERT' THEN NULL ELSE OLD.status END,NEW.status,NEW.version);
 END IF;RETURN NEW;END $$;
CREATE TRIGGER delivery_audit AFTER INSERT OR UPDATE ON platform.telegram_outbox FOR EACH ROW EXECUTE FUNCTION platform.telegram_delivery_audit();
CREATE FUNCTION platform.llm_budget_audit() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ BEGIN
 IF TG_OP='INSERT' OR OLD.status IS DISTINCT FROM NEW.status THEN
 INSERT INTO platform.llm_budget_events(organization_id,reservation_id,status,amount,charged,actor_id) VALUES(NEW.organization_id,NEW.id,NEW.status,NEW.amount,NEW.charged,platform.current_user_id());
 END IF;RETURN NEW;END $$;
CREATE TRIGGER budget_audit AFTER INSERT OR UPDATE ON platform.llm_reservations FOR EACH ROW EXECUTE FUNCTION platform.llm_budget_audit();
REVOKE ALL ON FUNCTION platform.telegram_delivery_version(),platform.telegram_delivery_audit(),platform.llm_budget_audit() FROM PUBLIC;
-- Local operator acknowledgement only. Intentionally no resend action.
CREATE FUNCTION platform.telegram_resolve_delivery(p_org uuid,p_id uuid,p_version bigint,p_decision text,p_key uuid) RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE q platform.telegram_outbox; prior text; integration uuid; BEGIN
 IF NOT coalesce(p_org::text=current_setting('app.organization_id',true) AND platform.can_operate_orders(p_org) AND platform.is_test_org(p_org),false) THEN RAISE EXCEPTION 'forbidden' USING ERRCODE='42501';END IF;
 IF p_decision IS NULL OR p_decision NOT IN ('delivered','abandon') OR p_key IS NULL OR p_version IS NULL THEN RAISE EXCEPTION 'invalid resolution' USING ERRCODE='23514';END IF;
 SELECT integration_id INTO integration FROM platform.telegram_outbox WHERE id=p_id AND organization_id=p_org;
 IF integration IS NULL THEN RAISE EXCEPTION 'not found' USING ERRCODE='P0002';END IF;
 PERFORM pg_advisory_xact_lock(hashtextextended('telegram-send:'||integration::text,0));
 SELECT * INTO q FROM platform.telegram_outbox WHERE id=p_id AND organization_id=p_org FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION 'not found' USING ERRCODE='P0002';END IF;
 SELECT decision INTO prior FROM platform.telegram_delivery_events WHERE organization_id=p_org AND outbox_id=p_id AND request_key=p_key;
 IF FOUND THEN
 IF prior<>p_decision THEN RAISE EXCEPTION 'idempotency conflict' USING ERRCODE='40001';END IF;
 RETURN jsonb_build_object('id',q.id,'status',q.status,'version',q.version);
 END IF;
 IF q.status<>'uncertain' OR q.version<>p_version THEN RAISE EXCEPTION 'version or state conflict' USING ERRCODE='40001';END IF;
 UPDATE platform.telegram_outbox SET status=CASE WHEN p_decision='delivered' THEN 'sent' ELSE 'cancelled' END,last_error=NULL,capability=NULL,lease_until=NULL WHERE id=p_id RETURNING * INTO q;
 INSERT INTO platform.telegram_delivery_events(organization_id,outbox_id,attempt,old_status,new_status,version,actor_id,decision,request_key) VALUES(p_org,q.id,q.attempts,'uncertain',q.status,q.version,platform.current_user_id(),p_decision,p_key);
 RETURN jsonb_build_object('id',q.id,'status',q.status,'version',q.version);
END $$;
REVOKE ALL ON FUNCTION platform.telegram_resolve_delivery(uuid,uuid,bigint,text,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.telegram_resolve_delivery(uuid,uuid,bigint,text,uuid) TO aba_app;
-- Preserve exact v8 definitions for a clean, guarded rollback.
ALTER FUNCTION platform.llm_settle(uuid,uuid,integer,integer,boolean) RENAME TO llm_settle_v8;
REVOKE ALL ON FUNCTION platform.llm_settle_v8(uuid,uuid,integer,integer,boolean) FROM aba_app,PUBLIC;
CREATE FUNCTION platform.llm_settle_core(p_org uuid,p_id uuid,p_in integer,p_out integer,p_known boolean) RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 DECLARE entry platform.llm_reservations; cost bigint; actual_in integer; actual_out integer; key text; BEGIN
 PERFORM 1 FROM platform.llm_platform_budget WHERE singleton FOR UPDATE;
 SELECT * INTO entry FROM platform.llm_reservations WHERE id=p_id AND organization_id=p_org FOR UPDATE;
 IF entry.id IS NULL THEN RAISE EXCEPTION 'forbidden' USING ERRCODE='42501';END IF;
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
REVOKE ALL ON FUNCTION platform.llm_settle_core(uuid,uuid,integer,integer,boolean) FROM PUBLIC;
CREATE FUNCTION platform.llm_settle(p_org uuid,p_id uuid,p_in integer,p_out integer,p_known boolean) RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE conv uuid; BEGIN
 IF NOT coalesce(platform.llm_authorized(p_org,NULL),false) THEN RAISE EXCEPTION 'forbidden' USING ERRCODE='42501';END IF;
 SELECT conversation_id INTO conv FROM platform.llm_reservations WHERE id=p_id AND organization_id=p_org;
 IF NOT FOUND OR NOT coalesce(platform.llm_authorized(p_org,conv),false) THEN RAISE EXCEPTION 'forbidden' USING ERRCODE='42501';END IF;
 PERFORM platform.llm_settle_core(p_org,p_id,p_in,p_out,p_known);
END $$;
REVOKE ALL ON FUNCTION platform.llm_settle(uuid,uuid,integer,integer,boolean) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.llm_settle(uuid,uuid,integer,integer,boolean) TO aba_app;
-- Fixed server-side 5 minute horizon exceeds the bounded provider cycle.
-- This maintenance capability can only consume stale reservations, never refund,
-- enable live, increase a limit, return row data or contact a provider.
CREATE FUNCTION platform.llm_recover_stale() RETURNS integer LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE r record; recovered integer=0; BEGIN
 PERFORM 1 FROM platform.llm_platform_budget WHERE singleton FOR UPDATE;
 FOR r IN SELECT id,organization_id FROM platform.llm_reservations WHERE status='pending' AND created_at<clock_timestamp()-interval '5 minutes' ORDER BY created_at,id LIMIT 100 FOR UPDATE LOOP
 PERFORM platform.llm_settle_core(r.organization_id,r.id,0,0,false);recovered=recovered+1;
 END LOOP;RETURN recovered;
END $$;
REVOKE ALL ON FUNCTION platform.llm_recover_stale() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.llm_recover_stale() TO aba_app;
CREATE FUNCTION platform.llm_recover_one(p_org uuid,p_id uuid) RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE r platform.llm_reservations; BEGIN
 IF NOT coalesce(p_org::text=current_setting('app.organization_id',true) AND platform.has_membership(p_org,true) AND platform.is_test_org(p_org),false) THEN RAISE EXCEPTION 'forbidden' USING ERRCODE='42501';END IF;
 PERFORM 1 FROM platform.llm_platform_budget WHERE singleton FOR UPDATE;
 SELECT * INTO r FROM platform.llm_reservations WHERE id=p_id AND organization_id=p_org FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION 'not found' USING ERRCODE='P0002';END IF;
 IF r.status<>'pending' THEN RETURN;END IF;
 IF r.created_at>=clock_timestamp()-interval '5 minutes' THEN RAISE EXCEPTION 'active reservation' USING ERRCODE='40001';END IF;
 PERFORM platform.llm_settle_core(p_org,p_id,0,0,false);
END $$;
REVOKE ALL ON FUNCTION platform.llm_recover_one(uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.llm_recover_one(uuid,uuid) TO aba_app;
UPDATE platform.service_metadata SET version=9 WHERE singleton;
-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN IF EXISTS(SELECT 1 FROM platform.organizations) THEN RAISE EXCEPTION 'refusing reliability downgrade with organization data';END IF;END $$;
DROP FUNCTION platform.telegram_resolve_delivery(uuid,uuid,bigint,text,uuid);
DROP FUNCTION platform.llm_recover_one(uuid,uuid),platform.llm_recover_stale();
DROP FUNCTION platform.llm_settle(uuid,uuid,integer,integer,boolean);
DROP FUNCTION platform.llm_settle_core(uuid,uuid,integer,integer,boolean);
ALTER FUNCTION platform.llm_settle_v8(uuid,uuid,integer,integer,boolean) RENAME TO llm_settle;
GRANT EXECUTE ON FUNCTION platform.llm_settle(uuid,uuid,integer,integer,boolean) TO aba_app;
DROP TRIGGER budget_audit ON platform.llm_reservations;
DROP TRIGGER delivery_audit ON platform.telegram_outbox;
DROP TRIGGER delivery_version ON platform.telegram_outbox;
DROP FUNCTION platform.llm_budget_audit(),platform.telegram_delivery_audit(),platform.telegram_delivery_version();
DROP TABLE platform.llm_budget_events,platform.telegram_delivery_events;
ALTER TABLE platform.llm_reservations DROP CONSTRAINT llm_reservations_tenant_id;
ALTER TABLE platform.telegram_outbox DROP CONSTRAINT telegram_outbox_tenant_id;
ALTER TABLE platform.telegram_outbox DROP COLUMN version;
UPDATE platform.service_metadata SET version=8 WHERE singleton;
-- +goose StatementEnd
