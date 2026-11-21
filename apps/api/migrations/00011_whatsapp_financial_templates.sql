-- +goose Up
-- +goose StatementBegin
-- Global platform limit: deliberately no application DML or public setter.
CREATE TABLE platform.wa_platform_budget(singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
 limit_amount bigint NOT NULL DEFAULT 0 CHECK(limit_amount BETWEEN 0 AND 1000000000000000),
 reserved bigint NOT NULL DEFAULT 0 CHECK(reserved>=0),spent bigint NOT NULL DEFAULT 0 CHECK(spent>=0),
 synthetic boolean NOT NULL DEFAULT false, CHECK(reserved+spent<=limit_amount));
INSERT INTO platform.wa_platform_budget(singleton) VALUES(true);
REVOKE ALL ON platform.wa_platform_budget FROM PUBLIC,aba_app;
CREATE TABLE platform.wa_budgets(
 organization_id uuid PRIMARY KEY REFERENCES platform.organizations(id),limit_amount bigint NOT NULL DEFAULT 0 CHECK(limit_amount BETWEEN 0 AND 1000000000000000),
 reserved bigint NOT NULL DEFAULT 0 CHECK(reserved>=0),spent bigint NOT NULL DEFAULT 0 CHECK(spent>=0),
 enabled boolean NOT NULL DEFAULT false,kill boolean NOT NULL DEFAULT true,synthetic boolean NOT NULL DEFAULT false,
 CHECK(reserved+spent<=limit_amount));
CREATE TABLE platform.wa_pricing(
 id uuid PRIMARY KEY,organization_id uuid NOT NULL,integration_id uuid NOT NULL,
 policy_version text NOT NULL CHECK(length(policy_version) BETWEEN 1 AND 100),region text NOT NULL CHECK(region ~ '^[A-Z]{2}$'),
 category text NOT NULL CHECK(category IN ('SERVICE','UTILITY','AUTHENTICATION','MARKETING')),
 message_type text NOT NULL CHECK(message_type IN ('text','template')),billing_basis text NOT NULL CHECK(length(billing_basis) BETWEEN 1 AND 100),
 currency text NOT NULL CHECK(length(currency)=3),max_charge bigint CHECK(max_charge BETWEEN 0 AND 1000000000000),verified_free boolean NOT NULL DEFAULT false,
 source text NOT NULL CHECK(length(source) BETWEEN 1 AND 300),verified_at timestamptz NOT NULL,verified_until timestamptz NOT NULL,effective_at timestamptz NOT NULL,
 verified boolean NOT NULL DEFAULT false,synthetic boolean NOT NULL DEFAULT true,
 CHECK(verified_until>verified_at),CHECK(NOT verified_free OR max_charge=0),
 UNIQUE(organization_id,id),UNIQUE(organization_id,integration_id,policy_version,region,category,message_type),
 FOREIGN KEY(organization_id,integration_id) REFERENCES platform.channel_integrations(organization_id,id));
CREATE TABLE platform.wa_templates(
 id uuid PRIMARY KEY,organization_id uuid NOT NULL,integration_id uuid NOT NULL,waba_id text NOT NULL CHECK(waba_id ~ '^[0-9]{1,20}$'),
 name text NOT NULL CHECK(name ~ '^[a-z0-9_]+$' AND length(name) BETWEEN 1 AND 512),language text NOT NULL CHECK(language ~ '^[a-z]{2,3}(_[A-Z]{2})?$'),
 category text NOT NULL CHECK(category IN ('UTILITY','AUTHENTICATION','MARKETING')),
 status text NOT NULL CHECK(status IN ('DRAFT','PENDING','APPROVED','REJECTED','PAUSED','DISABLED','UNKNOWN')),
 components jsonb NOT NULL CHECK(jsonb_typeof(components)='array' AND octet_length(components::text)<=8192),
 parameter_schema jsonb NOT NULL CHECK(jsonb_typeof(parameter_schema)='array' AND jsonb_array_length(parameter_schema)<=20),
 provider_id text CHECK(length(provider_id)<=200),verified_at timestamptz NOT NULL,verified_until timestamptz NOT NULL,
 source text NOT NULL,policy_version text NOT NULL,synthetic boolean NOT NULL DEFAULT true,
 UNIQUE(organization_id,id),UNIQUE(organization_id,integration_id,name,language),
 FOREIGN KEY(organization_id,integration_id) REFERENCES platform.channel_integrations(organization_id,id));
CREATE TABLE platform.wa_reservations(
 id uuid PRIMARY KEY,organization_id uuid NOT NULL,integration_id uuid NOT NULL,outbox_id uuid NOT NULL,attempt integer NOT NULL CHECK(attempt BETWEEN 1 AND 5),
 capability uuid NOT NULL,pricing_id uuid NOT NULL,template_id uuid,amount bigint NOT NULL CHECK(amount BETWEEN 0 AND 1000000000000),
 charged bigint NOT NULL DEFAULT 0 CHECK(charged BETWEEN 0 AND amount),
 status text NOT NULL CHECK(status IN ('reserved','unknown','charged','released')),synthetic boolean NOT NULL,
 provider_id text,version bigint NOT NULL DEFAULT 1,created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(organization_id,id),UNIQUE(outbox_id,attempt),
 FOREIGN KEY(organization_id,integration_id) REFERENCES platform.channel_integrations(organization_id,id),
 FOREIGN KEY(organization_id,outbox_id) REFERENCES platform.channel_outbox(organization_id,id),
 FOREIGN KEY(organization_id,pricing_id) REFERENCES platform.wa_pricing(organization_id,id),
 FOREIGN KEY(organization_id,template_id) REFERENCES platform.wa_templates(organization_id,id));
CREATE INDEX wa_reservations_recovery ON platform.wa_reservations(status,updated_at);
CREATE TABLE platform.wa_financial_events(
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,organization_id uuid NOT NULL REFERENCES platform.organizations(id),reservation_id uuid,template_id uuid,
 kind text NOT NULL CHECK(length(kind)<=100),actor_id uuid REFERENCES platform.users(id),evidence_hash bytea,created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,reservation_id) REFERENCES platform.wa_reservations(organization_id,id),
 FOREIGN KEY(organization_id,template_id) REFERENCES platform.wa_templates(organization_id,id));
DO $$ DECLARE t text;BEGIN FOREACH t IN ARRAY ARRAY['wa_budgets','wa_pricing','wa_templates','wa_reservations','wa_financial_events'] LOOP
 EXECUTE format('ALTER TABLE platform.%I ENABLE ROW LEVEL SECURITY',t);
 EXECUTE format('ALTER TABLE platform.%I FORCE ROW LEVEL SECURITY',t);
 EXECUTE format('CREATE POLICY migration_owner ON platform.%I TO aba_migrator USING(true) WITH CHECK(true)',t);
 EXECUTE format('CREATE POLICY owner_read ON platform.%I FOR SELECT TO aba_app USING(organization_id::text=current_setting(''app.organization_id'',true) AND platform.has_membership(organization_id,true))',t);
 EXECUTE format('REVOKE ALL ON platform.%I FROM PUBLIC,aba_app',t);
 EXECUTE format('GRANT SELECT ON platform.%I TO aba_app',t);
 END LOOP;END $$;
ALTER TABLE platform.channel_principals ADD recipient_region text CHECK(recipient_region ~ '^[A-Z]{2}$');
-- Backup only definitions, not tenant records. Restore exact predecessor on down.
CREATE TABLE platform.wa_function_backup(signature text PRIMARY KEY,definition text NOT NULL);
REVOKE ALL ON platform.wa_function_backup FROM PUBLIC,aba_app;
DO $$ DECLARE s text;d text;BEGIN
 FOREACH s IN ARRAY ARRAY['platform.channel_claim_send(bytea,uuid)','platform.channel_admit_send(uuid)'] LOOP
 d=pg_get_functiondef(s::regprocedure);INSERT INTO platform.wa_function_backup VALUES(s,d);
 -- Window exception only permits a template to reach financial admission; never bypasses it.
 d=replace(d,'p.window_until>now()','(p.window_until>now() OR x.payload ? ''template_id'')');
 d=replace(d,'p.window_until<=now()','p.window_until<=now() AND NOT (x.payload ? ''template_id'')');
 d=replace(d,'''attempts'',q.attempts+1','''attempts'',q.attempts+1,''template_id'',q.payload->>''template_id'',''parameters'',q.payload->''parameters''');
 EXECUTE d;
 END LOOP;END $$;
CREATE FUNCTION platform.wa_components_valid(c jsonb,s jsonb) RETURNS boolean LANGUAGE plpgsql IMMUTABLE SET search_path=pg_catalog,platform,pg_temp AS $$
 DECLARE body text;n integer;k integer;BEGIN
 IF jsonb_typeof(c) IS DISTINCT FROM 'array' OR jsonb_array_length(c)<>1 OR c->0->>'type' IS DISTINCT FROM 'BODY' OR jsonb_typeof(c->0->'text') IS DISTINCT FROM 'string' OR jsonb_typeof(s) IS DISTINCT FROM 'array' OR jsonb_array_length(s)>20 OR EXISTS(SELECT 1 FROM jsonb_array_elements_text(s)x WHERE x IS DISTINCT FROM 'text') THEN RETURN false;END IF;
 body=c->0->>'text';IF length(body) NOT BETWEEN 1 AND 4096 THEN RETURN false;END IF;
 n=jsonb_array_length(s);
 SELECT count(*) INTO k FROM regexp_matches(body,'\{\{[0-9]+\}\}','g');IF k<>n THEN RETURN false;END IF;
 FOR k IN 1..n LOOP IF position('{{'||k::text||'}}' IN body)=0 THEN RETURN false;END IF;body=replace(body,'{{'||k::text||'}}','');END LOOP;
 RETURN position('{{' IN body)=0 AND position('}}' IN body)=0;END $$;
CREATE FUNCTION platform.wa_template_check(p_org uuid,p_integration uuid,p_id uuid,p_name text,p_language text,p_params jsonb,p_synthetic boolean) RETURNS boolean
 LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 DECLARE t platform.wa_templates;i platform.channel_integrations;v jsonb;n integer;BEGIN
 SELECT * INTO t FROM platform.wa_templates WHERE id=p_id AND organization_id=p_org AND integration_id=p_integration;
 SELECT * INTO i FROM platform.channel_integrations WHERE id=p_integration AND organization_id=p_org;
 IF t.id IS NULL OR i.id IS NULL OR t.waba_id<>i.business_id OR t.name<>p_name OR t.language<>p_language OR t.status<>'APPROVED' OR t.verified_at>clock_timestamp() OR t.verified_until<=clock_timestamp()+interval '6 seconds' OR t.synthetic<>p_synthetic OR (NOT p_synthetic AND t.source<>'META_VERIFIED') OR t.policy_version='' THEN RETURN false;END IF;
 IF NOT platform.wa_components_valid(t.components,t.parameter_schema) OR jsonb_typeof(p_params) IS DISTINCT FROM 'array' OR jsonb_array_length(p_params)<>jsonb_array_length(t.parameter_schema) OR jsonb_array_length(t.components)<>1 OR t.components->0->>'type' IS DISTINCT FROM 'BODY' OR jsonb_typeof(t.components->0->'text') IS DISTINCT FROM 'string' THEN RETURN false;END IF;
 FOR n IN 0..jsonb_array_length(t.parameter_schema)-1 LOOP
 IF t.parameter_schema->>n IS DISTINCT FROM 'text' OR jsonb_typeof(p_params->n) IS DISTINCT FROM 'string' OR length(p_params->>n) NOT BETWEEN 1 AND 1024 THEN RETURN false;END IF;
 END LOOP;
 RETURN true;END $$;
-- Worker proof + current fenced outbox, never organization IDs alone, authorize reserve.
CREATE FUNCTION platform.wa_reserve(p_proof bytea,p_cap uuid,p_job uuid,p_org uuid,p_integration uuid,p_phone text,p_waba text,p_version text,p_user text,p_text text,p_actions jsonb,p_name text,p_language text,p_params jsonb,p_synthetic boolean) RETURNS uuid
 LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 DECLARE q platform.channel_outbox;i platform.channel_integrations;b platform.wa_budgets;g platform.wa_platform_budget;p platform.channel_principals;r platform.wa_pricing;t platform.wa_templates;op uuid;kind text='text';cat text='SERVICE';why text;BEGIN
 SELECT * INTO q FROM platform.channel_outbox WHERE id=p_job;
 SELECT * INTO i FROM platform.channel_integrations WHERE id=q.integration_id AND worker_hash=sha256(p_proof);
 IF i.id IS NULL OR i.phone_id IS DISTINCT FROM p_phone OR i.business_id IS DISTINCT FROM p_waba OR i.graph_version IS DISTINCT FROM p_version OR q.organization_id IS DISTINCT FROM p_org OR q.integration_id IS DISTINCT FROM p_integration OR q.external_user_id IS DISTINCT FROM p_user OR coalesce(q.payload->>'text','') IS DISTINCT FROM p_text OR coalesce(q.payload->'actions','[]'::jsonb) IS DISTINCT FROM p_actions THEN RAISE EXCEPTION 'forbidden' USING ERRCODE='42501';END IF;
 PERFORM pg_advisory_xact_lock(hashtextextended(i.id::text,8210));
 SELECT * INTO g FROM platform.wa_platform_budget WHERE singleton FOR UPDATE;
 SELECT * INTO b FROM platform.wa_budgets WHERE organization_id=i.organization_id FOR UPDATE;
 SELECT * INTO q FROM platform.channel_outbox WHERE id=p_job FOR UPDATE;
 SELECT * INTO i FROM platform.channel_integrations WHERE id=q.integration_id;
 SELECT * INTO p FROM platform.channel_principals WHERE organization_id=q.organization_id AND integration_id=q.integration_id AND conversation_id=q.conversation_id AND external_user_id=q.external_user_id;
 IF q.status<>'sending' OR q.capability IS DISTINCT FROM p_cap OR q.lease_until<=clock_timestamp()+interval '1 second' OR NOT platform.channel_admit_send(p_cap) OR NOT i.active OR NOT i.sending_enabled OR p.id IS NULL THEN why='STALE_OR_DISABLED';
 ELSIF b.organization_id IS NULL OR NOT b.enabled OR b.kill OR b.synthetic<>p_synthetic OR g.synthetic<>p_synthetic OR (p_synthetic AND NOT EXISTS(SELECT 1 FROM platform.organizations WHERE id=q.organization_id AND is_test)) OR (NOT p_synthetic AND i.mode<>'live') THEN why='LIVE_DISABLED';
 ELSIF EXISTS(SELECT 1 FROM platform.wa_reservations WHERE outbox_id=q.id AND (status<>'released' OR attempt>=q.attempts)) THEN why='DUPLICATE_OR_UNKNOWN';
 END IF;
 IF why IS NULL AND q.payload ? 'template_id' THEN
 kind='template';SELECT * INTO t FROM platform.wa_templates WHERE id=(q.payload->>'template_id')::uuid AND organization_id=q.organization_id;
 IF t.id IS NULL OR NOT platform.wa_template_check(q.organization_id,i.id,t.id,p_name,p_language,p_params,p_synthetic) OR p_params IS DISTINCT FROM q.payload->'parameters' THEN why='TEMPLATE_BLOCKED';ELSE cat=t.category;END IF;
 ELSIF why IS NULL AND (p_name<>'' OR p.window_until<=clock_timestamp()+interval '6 seconds') THEN why='WINDOW_CLOSED';END IF;
 IF why IS NULL THEN
 SELECT * INTO r FROM platform.wa_pricing WHERE organization_id=q.organization_id AND integration_id=i.id AND region=p.recipient_region AND category=cat AND message_type=kind AND currency='RUB' AND verified AND effective_at<=clock_timestamp() AND verified_at<=clock_timestamp() AND verified_until>clock_timestamp()+interval '6 seconds' AND synthetic=p_synthetic AND (p_synthetic OR source='META_VERIFIED') AND max_charge IS NOT NULL AND billing_basis IN ('per_message','per_conversation','per_event','verified_maximum') ORDER BY verified_at DESC,id LIMIT 1;
 IF r.id IS NULL OR (r.max_charge=0 AND NOT r.verified_free) THEN why='PRICING_UNVERIFIED';
 ELSIF r.max_charge>b.limit_amount-b.reserved-b.spent OR r.max_charge>g.limit_amount-g.reserved-g.spent THEN why='BUDGET_EXHAUSTED';END IF;
 END IF;
 IF why IS NOT NULL THEN INSERT INTO platform.wa_financial_events(organization_id,kind) VALUES(q.organization_id,why);RETURN NULL;END IF;
 op=gen_random_uuid();
 INSERT INTO platform.wa_reservations(id,organization_id,integration_id,outbox_id,attempt,capability,pricing_id,template_id,amount,status,synthetic) VALUES(op,q.organization_id,i.id,q.id,q.attempts,p_cap,r.id,t.id,r.max_charge,'reserved',p_synthetic);
 UPDATE platform.wa_budgets SET reserved=reserved+r.max_charge WHERE organization_id=q.organization_id;
 UPDATE platform.wa_platform_budget SET reserved=reserved+r.max_charge WHERE singleton;
 INSERT INTO platform.wa_financial_events(organization_id,reservation_id,kind) VALUES(q.organization_id,op,'RESERVED');RETURN op;END $$;
CREATE FUNCTION platform.wa_finish(p_proof bytea,p_id uuid,p_state text,p_provider text,p_no_charge boolean) RETURNS void
 LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 DECLARE r platform.wa_reservations;s text;charge bigint=0;BEGIN
 SELECT * INTO r FROM platform.wa_reservations WHERE id=p_id;
 IF r.id IS NULL OR NOT EXISTS(SELECT 1 FROM platform.channel_integrations WHERE id=r.integration_id AND worker_hash=sha256(p_proof)) THEN RAISE EXCEPTION 'forbidden' USING ERRCODE='42501';END IF;
 PERFORM 1 FROM platform.wa_platform_budget WHERE singleton FOR UPDATE;
 PERFORM 1 FROM platform.wa_budgets WHERE organization_id=r.organization_id FOR UPDATE;
 SELECT * INTO r FROM platform.wa_reservations WHERE id=p_id FOR UPDATE;
 IF r.status<>'reserved' THEN RETURN;END IF;
 s=CASE WHEN p_state='sent' THEN 'charged' WHEN p_state IN ('failed','cancelled','pending') AND p_no_charge IS TRUE THEN 'released' ELSE 'unknown' END;
 IF s='charged' THEN charge=r.amount;END IF;
 IF s<>'unknown' THEN
 UPDATE platform.wa_budgets SET reserved=reserved-r.amount,spent=spent+charge WHERE organization_id=r.organization_id;
 UPDATE platform.wa_platform_budget SET reserved=reserved-r.amount,spent=spent+charge WHERE singleton;END IF;
 UPDATE platform.wa_reservations SET status=s,charged=charge,provider_id=left(p_provider,200),version=version+1,updated_at=clock_timestamp() WHERE id=r.id;
 INSERT INTO platform.wa_financial_events(organization_id,reservation_id,kind) VALUES(r.organization_id,r.id,CASE WHEN s='charged' THEN 'CHARGED_CONSERVATIVE_ESTIMATE' ELSE upper(s) END);END $$;
-- Recovery retains money, never equates timeout with a free operation.
CREATE FUNCTION platform.wa_recover(p_proof bytea) RETURNS integer LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 DECLARE r platform.wa_reservations;n integer=0;BEGIN
 FOR r IN SELECT x.* FROM platform.wa_reservations x JOIN platform.channel_integrations i ON i.id=x.integration_id JOIN platform.channel_outbox q ON q.id=x.outbox_id WHERE x.status='reserved' AND i.worker_hash=sha256(p_proof) AND (q.lease_until IS NULL OR q.lease_until<=clock_timestamp() OR q.status<>'sending') ORDER BY x.created_at,x.id LIMIT 100 FOR UPDATE OF x SKIP LOCKED LOOP
 UPDATE platform.wa_reservations SET status='unknown',version=version+1,updated_at=clock_timestamp() WHERE id=r.id;
 INSERT INTO platform.wa_financial_events(organization_id,reservation_id,kind) VALUES(r.organization_id,r.id,'RECOVERED_UNKNOWN');n=n+1;END LOOP;RETURN n;END $$;
CREATE FUNCTION platform.wa_reconcile(p_org uuid,p_id uuid,p_version bigint,p_charge bigint,p_evidence text) RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 DECLARE r platform.wa_reservations;BEGIN
 IF p_org::text IS DISTINCT FROM current_setting('app.organization_id',true) OR NOT platform.has_membership(p_org,true) THEN RAISE EXCEPTION 'forbidden' USING ERRCODE='42501';END IF;
 SELECT * INTO r FROM platform.wa_reservations WHERE organization_id=p_org AND id=p_id;
 IF r.id IS NULL THEN RAISE no_data_found;END IF;
 PERFORM 1 FROM platform.wa_platform_budget WHERE singleton FOR UPDATE;PERFORM 1 FROM platform.wa_budgets WHERE organization_id=p_org FOR UPDATE;
 SELECT * INTO r FROM platform.wa_reservations WHERE id=p_id FOR UPDATE;
 IF r.status IN ('released','charged') AND EXISTS(SELECT 1 FROM platform.wa_financial_events WHERE reservation_id=r.id AND kind='OWNER_RECONCILED' AND evidence_hash=sha256(convert_to(p_evidence,'UTF8'))) AND r.charged=p_charge THEN RETURN;END IF;
 IF r.status<>'unknown' OR p_version IS NULL OR p_version<1 OR r.version<>p_version THEN RAISE EXCEPTION 'financial version conflict' USING ERRCODE='40001';END IF;
 IF p_charge IS NULL OR p_evidence IS NULL OR p_charge<0 OR p_charge>r.amount OR length(p_evidence) NOT BETWEEN 10 AND 1000 THEN RAISE EXCEPTION 'invalid reconciliation' USING ERRCODE='23514';END IF;
 UPDATE platform.wa_budgets SET reserved=reserved-r.amount,spent=spent+p_charge WHERE organization_id=p_org;
 UPDATE platform.wa_platform_budget SET reserved=reserved-r.amount,spent=spent+p_charge WHERE singleton;
 UPDATE platform.wa_reservations SET status=CASE WHEN p_charge=0 THEN 'released' ELSE 'charged' END,charged=p_charge,version=version+1,updated_at=clock_timestamp() WHERE id=r.id;
 INSERT INTO platform.wa_financial_events(organization_id,reservation_id,kind,actor_id,evidence_hash) VALUES(p_org,p_id,'OWNER_RECONCILED',platform.current_user_id(),sha256(convert_to(p_evidence,'UTF8')));END $$;
-- Only the backend proof can persist verifier output; mock cannot create real approval.
CREATE FUNCTION platform.wa_verify_template(p_proof bytea,p_org uuid,p_integration uuid,p_id uuid,p_name text,p_lang text,p_category text,p_status text,p_components jsonb,p_schema jsonb,p_until timestamptz) RETURNS void
 LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ DECLARE i platform.channel_integrations;verified_id uuid;BEGIN
 SELECT * INTO i FROM platform.channel_integrations WHERE id=p_integration AND organization_id=p_org AND worker_hash=sha256(p_proof);
 IF i.id IS NULL OR NOT EXISTS(SELECT 1 FROM platform.organizations WHERE id=p_org AND is_test) THEN RAISE EXCEPTION 'forbidden' USING ERRCODE='42501';END IF;
 PERFORM pg_advisory_xact_lock(hashtextextended(i.id::text,8210));
 IF NOT platform.wa_components_valid(p_components,p_schema) OR p_until<=clock_timestamp() OR p_until>clock_timestamp()+interval '7 days' OR jsonb_array_length(p_components)<>1 OR p_components->0->>'type' IS DISTINCT FROM 'BODY' OR jsonb_typeof(p_components->0->'text') IS DISTINCT FROM 'string' OR EXISTS(SELECT 1 FROM jsonb_array_elements_text(p_schema) x WHERE x IS DISTINCT FROM 'text') THEN RAISE EXCEPTION 'invalid template' USING ERRCODE='23514';END IF;
 INSERT INTO platform.wa_templates(id,organization_id,integration_id,waba_id,name,language,category,status,components,parameter_schema,verified_at,verified_until,source,policy_version,synthetic) VALUES(p_id,p_org,p_integration,i.business_id,p_name,p_lang,p_category,p_status,p_components,p_schema,clock_timestamp(),p_until,'SYNTHETIC_MOCK','synthetic-v1',true)
 ON CONFLICT(organization_id,integration_id,name,language) DO UPDATE SET status=excluded.status,components=excluded.components,parameter_schema=excluded.parameter_schema,verified_at=excluded.verified_at,verified_until=excluded.verified_until,source='SYNTHETIC_MOCK',synthetic=true RETURNING id INTO verified_id;
 INSERT INTO platform.wa_financial_events(organization_id,template_id,kind) VALUES(p_org,verified_id,'SYNTHETIC_TEMPLATE_'||p_status);END $$;
CREATE FUNCTION platform.wa_enqueue_template(p_org uuid,p_conv uuid,p_template uuid,p_params jsonb,p_id uuid) RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 DECLARE t platform.wa_templates;p platform.channel_principals;old platform.channel_outbox;BEGIN
 IF p_org::text IS DISTINCT FROM current_setting('app.organization_id',true) OR NOT platform.has_membership(p_org,true) OR NOT platform.is_test_org(p_org) THEN RAISE EXCEPTION 'forbidden' USING ERRCODE='42501';END IF;
 SELECT * INTO p FROM platform.channel_principals WHERE organization_id=p_org AND conversation_id=p_conv;
 SELECT * INTO t FROM platform.wa_templates WHERE organization_id=p_org AND id=p_template AND integration_id=p.integration_id;
 IF t.id IS NULL OR NOT platform.wa_template_check(p_org,p.integration_id,t.id,t.name,t.language,p_params,true) THEN RAISE EXCEPTION 'template unavailable' USING ERRCODE='23514';END IF;
 SELECT * INTO old FROM platform.channel_outbox WHERE id=p_id;
 IF old.id IS NOT NULL THEN IF old.organization_id=p_org AND old.conversation_id=p_conv AND old.payload=jsonb_build_object('template_id',t.id,'parameters',p_params,'text','') THEN RETURN p_id;END IF;RAISE EXCEPTION 'idempotency conflict' USING ERRCODE='23505';END IF;
 INSERT INTO platform.channel_outbox(id,organization_id,integration_id,conversation_id,external_user_id,role,payload) VALUES(p_id,p_org,p.integration_id,p_conv,p.external_user_id,'staff',jsonb_build_object('template_id',t.id,'parameters',p_params,'text',''));RETURN p_id;END $$;
CREATE FUNCTION platform.wa_dispatch_template(p_proof bytea,p_cap uuid) RETURNS jsonb LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT jsonb_build_object('Name',t.name,'Language',t.language,'Parameters',q.payload->'parameters') FROM platform.channel_outbox q JOIN platform.channel_integrations i ON i.id=q.integration_id JOIN platform.wa_templates t ON t.id=(q.payload->>'template_id')::uuid AND t.organization_id=q.organization_id AND t.integration_id=q.integration_id WHERE q.capability=p_cap AND q.status='sending' AND q.lease_until>clock_timestamp() AND i.worker_hash=sha256(p_proof) $$;
DO $$ DECLARE f record;BEGIN FOR f IN SELECT p.oid::regprocedure sig,p.proname FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='platform' AND p.proname LIKE 'wa_%' LOOP
 EXECUTE format('REVOKE ALL ON FUNCTION %s FROM PUBLIC',f.sig);
 IF f.proname<>'wa_template_check' THEN EXECUTE format('GRANT EXECUTE ON FUNCTION %s TO aba_app',f.sig);END IF;
 END LOOP;END $$;
UPDATE platform.service_metadata SET version=11 WHERE singleton;
-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DO $$BEGIN IF EXISTS(SELECT 1 FROM platform.channel_principals WHERE recipient_region IS NOT NULL) OR EXISTS(SELECT 1 FROM platform.wa_templates) OR EXISTS(SELECT 1 FROM platform.wa_reservations) OR EXISTS(SELECT 1 FROM platform.wa_pricing) OR EXISTS(SELECT 1 FROM platform.wa_budgets) OR EXISTS(SELECT 1 FROM platform.wa_financial_events) THEN RAISE EXCEPTION 'rollback refused: preserve WhatsApp financial data';END IF;END $$;
DO $$DECLARE f record;BEGIN FOR f IN SELECT definition FROM platform.wa_function_backup LOOP EXECUTE f.definition;END LOOP;END $$;
DROP FUNCTION platform.wa_dispatch_template(bytea,uuid),platform.wa_enqueue_template(uuid,uuid,uuid,jsonb,uuid),platform.wa_verify_template(bytea,uuid,uuid,uuid,text,text,text,text,jsonb,jsonb,timestamptz),platform.wa_reconcile(uuid,uuid,bigint,bigint,text),platform.wa_recover(bytea),platform.wa_finish(bytea,uuid,text,text,boolean),platform.wa_reserve(bytea,uuid,uuid,uuid,uuid,text,text,text,text,text,jsonb,text,text,jsonb,boolean),platform.wa_template_check(uuid,uuid,uuid,text,text,jsonb,boolean),platform.wa_components_valid(jsonb,jsonb);
DROP TABLE platform.wa_financial_events,platform.wa_reservations,platform.wa_templates,platform.wa_pricing,platform.wa_budgets,platform.wa_platform_budget,platform.wa_function_backup;
ALTER TABLE platform.channel_principals DROP COLUMN recipient_region;
UPDATE platform.service_metadata SET version=10 WHERE singleton;
-- +goose StatementEnd
