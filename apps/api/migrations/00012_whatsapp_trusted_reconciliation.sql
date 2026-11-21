-- +goose Up
-- +goose StatementBegin
-- Explicit tenant assignments are provisioned by the migration/control-plane role,
-- never by an owner-facing endpoint. No global administrative RLS bypass.
CREATE TABLE platform.wa_operator_grants(
 organization_id uuid NOT NULL REFERENCES platform.organizations(id),user_id uuid NOT NULL REFERENCES platform.users(id),
 expires_at timestamptz NOT NULL,revoked_at timestamptz,assigned_by text NOT NULL CHECK(assigned_by ~ '^control:[A-Za-z0-9_-]{1,80}$'),
 PRIMARY KEY(organization_id,user_id));
CREATE FUNCTION platform.wa_is_operator(p_org uuid) RETURNS boolean LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT p_org::text=current_setting('app.organization_id',true) AND EXISTS(SELECT 1 FROM platform.wa_operator_grants WHERE organization_id=p_org AND user_id=platform.current_user_id() AND revoked_at IS NULL AND expires_at>clock_timestamp()) $$;
CREATE FUNCTION platform.wa_lock_operator(p_org uuid) RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$BEGIN
 IF p_org::text IS DISTINCT FROM current_setting('app.organization_id',true) THEN RETURN false;END IF;
 PERFORM 1 FROM platform.wa_operator_grants WHERE organization_id=p_org AND user_id=platform.current_user_id() AND revoked_at IS NULL AND expires_at>clock_timestamp() FOR SHARE;
 RETURN FOUND;END $$;
REVOKE ALL ON FUNCTION platform.wa_lock_operator(uuid) FROM PUBLIC;
CREATE TABLE platform.wa_provider_records(
 id uuid PRIMARY KEY,organization_id uuid NOT NULL,integration_id uuid NOT NULL,pricing_id uuid,template_id uuid,
 source_type text NOT NULL CHECK(source_type IN ('SYNTHETIC_TEST','MANUAL_UNVERIFIED','VERIFIED_OPERATOR','OFFICIAL_PROVIDER')),
 source_identity text NOT NULL CHECK(source_identity ~ '^[A-Za-z0-9:_-]{1,100}$'),verification_method text NOT NULL CHECK(verification_method IN ('OFFLINE_DIGEST','OPERATOR_REVIEW','NONE')),
 verified_at timestamptz NOT NULL,expires_at timestamptz NOT NULL,policy_version text NOT NULL CHECK(length(policy_version) BETWEEN 1 AND 100),
 integrity_digest bytea NOT NULL CHECK(octet_length(integrity_digest)=32),payload_digest bytea NOT NULL CHECK(octet_length(payload_digest)=32),
 approval_status text NOT NULL CHECK(approval_status IN ('VERIFIED','UNVERIFIED')),revoked_at timestamptz,version bigint NOT NULL DEFAULT 1,
 CHECK((pricing_id IS NULL)<>(template_id IS NULL)),CHECK(expires_at>verified_at),
 -- No official verifier exists in this offline release. A label cannot manufacture provenance.
 CHECK(source_type<>'OFFICIAL_PROVIDER'),
 UNIQUE(organization_id,id),UNIQUE(organization_id,pricing_id),UNIQUE(organization_id,template_id),
 FOREIGN KEY(organization_id,integration_id) REFERENCES platform.channel_integrations(organization_id,id),
 FOREIGN KEY(organization_id,pricing_id) REFERENCES platform.wa_pricing(organization_id,id),
 FOREIGN KEY(organization_id,template_id) REFERENCES platform.wa_templates(organization_id,id));
CREATE INDEX wa_provider_expiry ON platform.wa_provider_records(expires_at) WHERE revoked_at IS NULL;
CREATE TABLE platform.wa_reconciliation_evidence(
 id uuid PRIMARY KEY,organization_id uuid NOT NULL,reservation_id uuid NOT NULL,integration_id uuid NOT NULL,outbox_id uuid NOT NULL,
 provider_id text CHECK(provider_id ~ '^[A-Za-z0-9._:-]{1,200}$'),
 evidence_type text NOT NULL CHECK(evidence_type IN ('SYNTHETIC_TEST','OPERATOR_REVIEW')),
 evidence_reference text NOT NULL CHECK(evidence_reference ~ '^(ticket|synthetic):[A-Za-z0-9_-]{8,80}$'),
 evidence_at timestamptz NOT NULL,operator_id uuid NOT NULL REFERENCES platform.users(id),
 decision text NOT NULL CHECK(decision IN ('INSUFFICIENT','CONFIRMED_SENT','CONFIRMED_NOT_SENT')),
 charge bigint NOT NULL CHECK(charge>=0),request_digest bytea NOT NULL CHECK(octet_length(request_digest)=32),
 decision_at timestamptz NOT NULL DEFAULT now(),outcome text NOT NULL CHECK(outcome IN ('INSUFFICIENT','CONFLICT','COMPLETED','REJECTED')),
 UNIQUE(organization_id,id),
 FOREIGN KEY(organization_id,reservation_id) REFERENCES platform.wa_reservations(organization_id,id),
 FOREIGN KEY(organization_id,integration_id) REFERENCES platform.channel_integrations(organization_id,id),
 FOREIGN KEY(organization_id,outbox_id) REFERENCES platform.channel_outbox(organization_id,id));
CREATE INDEX wa_reconciliation_history ON platform.wa_reconciliation_evidence(organization_id,reservation_id,decision_at);
ALTER TABLE platform.wa_financial_events ADD provider_record_id uuid, ADD evidence_id uuid,ADD provider_version bigint,ADD provider_digest bytea CHECK(provider_digest IS NULL OR octet_length(provider_digest)=32);
ALTER TABLE platform.wa_financial_events ADD FOREIGN KEY(organization_id,provider_record_id) REFERENCES platform.wa_provider_records(organization_id,id), ADD FOREIGN KEY(organization_id,evidence_id) REFERENCES platform.wa_reconciliation_evidence(organization_id,id);
DO $$DECLARE t text;BEGIN FOREACH t IN ARRAY ARRAY['wa_operator_grants','wa_provider_records','wa_reconciliation_evidence'] LOOP
 EXECUTE format('ALTER TABLE platform.%I ENABLE ROW LEVEL SECURITY',t);EXECUTE format('ALTER TABLE platform.%I FORCE ROW LEVEL SECURITY',t);
 EXECUTE format('CREATE POLICY migration_owner ON platform.%I TO aba_migrator USING(true) WITH CHECK(true)',t);
 EXECUTE format('REVOKE ALL ON platform.%I FROM PUBLIC,aba_app',t);EXECUTE format('GRANT SELECT ON platform.%I TO aba_app',t);
 END LOOP;END $$;
CREATE POLICY assigned_read ON platform.wa_operator_grants FOR SELECT TO aba_app USING(organization_id::text=current_setting('app.organization_id',true) AND user_id=platform.current_user_id());
CREATE POLICY scoped_read ON platform.wa_provider_records FOR SELECT TO aba_app USING(organization_id::text=current_setting('app.organization_id',true) AND (platform.has_membership(organization_id,true) OR platform.wa_is_operator(organization_id)));
CREATE POLICY scoped_read ON platform.wa_reconciliation_evidence FOR SELECT TO aba_app USING(organization_id::text=current_setting('app.organization_id',true) AND platform.wa_is_operator(organization_id));
CREATE FUNCTION platform.wa_resource_digest(p_kind text,p_id uuid) RETURNS bytea LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT sha256(convert_to(CASE WHEN p_kind='pricing' THEN (SELECT to_jsonb(p)::text FROM platform.wa_pricing p WHERE id=p_id) WHEN p_kind='template' THEN (SELECT to_jsonb(t)::text FROM platform.wa_templates t WHERE id=p_id) END,'UTF8')) $$;
CREATE FUNCTION platform.wa_provenance_valid(p_kind text,p_id uuid,p_synthetic boolean) RETURNS boolean LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT EXISTS(SELECT 1 FROM platform.wa_provider_records r WHERE (CASE WHEN p_kind='pricing' THEN r.pricing_id ELSE r.template_id END)=p_id AND r.approval_status='VERIFIED' AND r.revoked_at IS NULL AND r.verified_at<=clock_timestamp() AND r.expires_at>clock_timestamp()+interval '6 seconds' AND r.integrity_digest=platform.wa_resource_digest(p_kind,p_id) AND p_synthetic AND r.source_type='SYNTHETIC_TEST' AND r.verification_method='OFFLINE_DIGEST') $$;
-- Preserve predecessor definitions for a guarded, exact rollback.
INSERT INTO platform.wa_function_backup SELECT '9c:'||s,pg_get_functiondef(s::regprocedure) FROM unnest(ARRAY['platform.channel_finish_send(uuid,text,integer,text)','platform.channel_resolve(uuid,uuid,bigint,text)','platform.wa_reserve(bytea,uuid,uuid,uuid,uuid,text,text,text,text,text,jsonb,text,text,jsonb,boolean)','platform.wa_template_check(uuid,uuid,uuid,text,text,jsonb,boolean)','platform.wa_reconcile(uuid,uuid,bigint,bigint,text)','platform.wa_verify_template(bytea,uuid,uuid,uuid,text,text,text,text,jsonb,jsonb,timestamptz)'])s;
DO $$DECLARE d text;BEGIN
 SELECT definition INTO d FROM platform.wa_function_backup WHERE signature LIKE '9c:platform.wa_reserve(%';
 IF position('AND max_charge IS NOT NULL' IN d)=0 THEN RAISE EXCEPTION 'unexpected admission predecessor';END IF;
 d=replace(d,'AND max_charge IS NOT NULL','AND platform.wa_provenance_valid(''pricing'',id,p_synthetic) AND max_charge IS NOT NULL');
 -- Overlapping active policies are ambiguous, even when each individual row looks valid.
 d=replace(d,'(status<>''released'' OR attempt>=q.attempts)','(status<>''released'' OR attempt>=q.attempts OR EXISTS(SELECT 1 FROM platform.wa_reconciliation_evidence ev WHERE ev.reservation_id=platform.wa_reservations.id))');
 d=replace(d,'IF r.id IS NULL OR','IF (SELECT count(*) FROM platform.wa_pricing z WHERE z.organization_id=q.organization_id AND z.integration_id=i.id AND z.region=p.recipient_region AND z.category=cat AND z.message_type=kind AND z.effective_at<=clock_timestamp() AND platform.wa_provenance_valid(''pricing'',z.id,p_synthetic))<>1 OR r.id IS NULL OR');EXECUTE d;
 SELECT definition INTO d FROM platform.wa_function_backup WHERE signature LIKE '9c:platform.wa_template_check(%';
 d=replace(d,'IF t.id IS NULL OR','IF NOT platform.wa_provenance_valid(''template'',p_id,p_synthetic) OR t.id IS NULL OR');EXECUTE d;
 END $$;
DO $$DECLARE d text;BEGIN SELECT definition INTO d FROM platform.wa_function_backup WHERE signature='9c:platform.channel_resolve(uuid,uuid,bigint,text)';
 d=replace(d,'AS $function$ BEGIN','AS $function$ BEGIN');
 d=replace(d,'IF p_org::text IS DISTINCT', 'IF EXISTS(SELECT 1 FROM platform.channel_outbox WHERE organization_id=p_org AND id=p_id) THEN RAISE EXCEPTION ''use evidence workflow'' USING ERRCODE=''42501'';END IF; IF p_org::text IS DISTINCT');EXECUTE d;END $$;
DO $$DECLARE d text;BEGIN SELECT definition INTO d FROM platform.wa_function_backup WHERE signature='9c:platform.channel_finish_send(uuid,text,integer,text)';
 IF position('provider_id=p_provider' IN d)=0 THEN RAISE EXCEPTION 'unexpected finish predecessor';END IF;
 EXECUTE replace(d,'provider_id=p_provider','provider_id=nullif(p_provider,'''')');END $$;
-- Legacy owner-only financial mutation is closed, including direct SQL callers.
CREATE OR REPLACE FUNCTION platform.wa_reconcile(p_org uuid,p_id uuid,p_version bigint,p_charge bigint,p_evidence text) RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$BEGIN RAISE EXCEPTION 'use operator evidence workflow' USING ERRCODE='42501';END $$;
-- Backend proof attests only to synthetic mock data, never official provider approval.
CREATE FUNCTION platform.wa_provision(p_proof bytea,p_org uuid,p_integration uuid,p_kind text,p_id uuid,p_expected bigint,p_source text,p_identity text,p_method text,p_policy text,p_until timestamptz,p_raw text,p_digest bytea) RETURNS jsonb
 LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 DECLARE i platform.channel_integrations;r platform.wa_provider_records;v jsonb;rid uuid;BEGIN
 SELECT * INTO i FROM platform.channel_integrations WHERE id=p_integration AND organization_id=p_org AND worker_hash=sha256(p_proof);
 IF i.id IS NULL THEN RETURN jsonb_build_object('code','FORBIDDEN');END IF;
 PERFORM pg_advisory_xact_lock(hashtextextended(i.id::text,8210));
 SELECT * INTO r FROM platform.wa_provider_records WHERE organization_id=p_org AND (CASE WHEN p_kind='pricing' THEN pricing_id ELSE template_id END)=p_id FOR UPDATE;
 IF p_kind NOT IN ('pricing','template') OR p_source NOT IN ('SYNTHETIC_TEST','MANUAL_UNVERIFIED','VERIFIED_OPERATOR') OR p_digest IS DISTINCT FROM sha256(convert_to(p_raw,'UTF8')) OR p_until<=clock_timestamp() OR p_until>clock_timestamp()+interval '7 days' OR NOT EXISTS(SELECT 1 FROM platform.organizations WHERE id=p_org AND is_test) OR (p_source='SYNTHETIC_TEST' AND (p_method<>'OFFLINE_DIGEST' OR p_identity NOT LIKE 'synthetic:%')) THEN
 INSERT INTO platform.wa_financial_events(organization_id,kind) VALUES(p_org,'PROVISIONING_REJECTED');RETURN jsonb_build_object('code','INVALID_PROVIDER_EVIDENCE');END IF;
 IF r.id IS NOT NULL AND r.payload_digest=p_digest AND r.source_type=p_source AND r.source_identity=p_identity AND r.policy_version=p_policy AND r.verification_method=p_method AND r.expires_at=p_until AND r.revoked_at IS NULL AND r.integrity_digest=platform.wa_resource_digest(p_kind,p_id) THEN RETURN jsonb_build_object('code','OK','version',r.version);END IF;
 IF (r.id IS NULL AND p_expected<>0) OR (r.id IS NOT NULL AND r.version<>p_expected) OR p_expected IS NULL THEN
 INSERT INTO platform.wa_financial_events(organization_id,kind) VALUES(p_org,'PROVISIONING_VERSION_CONFLICT');RETURN jsonb_build_object('code','VERSION_CONFLICT');END IF;
 v=p_raw::jsonb;
 IF p_kind='pricing' THEN
 INSERT INTO platform.wa_pricing(id,organization_id,integration_id,policy_version,region,category,message_type,billing_basis,currency,max_charge,verified_free,source,verified_at,verified_until,effective_at,verified,synthetic)
 VALUES(p_id,p_org,p_integration,p_policy,v->>'region',v->>'category',v->>'message_type',v->>'billing_basis',v->>'currency',(v->>'max_charge')::bigint,coalesce((v->>'verified_free')::boolean,false),'SYNTHETIC_FIXTURE',clock_timestamp(),p_until,(v->>'effective_at')::timestamptz,p_source='SYNTHETIC_TEST',true)
 ON CONFLICT(id) DO UPDATE SET policy_version=excluded.policy_version,region=excluded.region,category=excluded.category,message_type=excluded.message_type,billing_basis=excluded.billing_basis,currency=excluded.currency,max_charge=excluded.max_charge,verified_free=excluded.verified_free,source=excluded.source,verified_at=excluded.verified_at,verified_until=excluded.verified_until,effective_at=excluded.effective_at,verified=excluded.verified,synthetic=true WHERE platform.wa_pricing.organization_id=p_org AND platform.wa_pricing.integration_id=p_integration;
 ELSE
 IF v->>'waba_id' IS DISTINCT FROM i.business_id OR NOT platform.wa_components_valid(v->'components',v->'parameter_schema') THEN
 INSERT INTO platform.wa_financial_events(organization_id,kind) VALUES(p_org,'PROVISIONING_REJECTED');RETURN jsonb_build_object('code','INVALID_PROVIDER_EVIDENCE');END IF;
 INSERT INTO platform.wa_templates(id,organization_id,integration_id,waba_id,name,language,category,status,components,parameter_schema,provider_id,verified_at,verified_until,source,policy_version,synthetic)
 VALUES(p_id,p_org,p_integration,i.business_id,v->>'name',v->>'language',v->>'category',v->>'status',v->'components',v->'parameter_schema',v->>'provider_id',clock_timestamp(),p_until,'SYNTHETIC_MOCK',p_policy,true)
 ON CONFLICT(id) DO UPDATE SET status=excluded.status,category=excluded.category,components=excluded.components,parameter_schema=excluded.parameter_schema,provider_id=excluded.provider_id,verified_at=excluded.verified_at,verified_until=excluded.verified_until,source=excluded.source,policy_version=excluded.policy_version,synthetic=true WHERE platform.wa_templates.organization_id=p_org AND platform.wa_templates.integration_id=p_integration AND platform.wa_templates.waba_id=excluded.waba_id AND platform.wa_templates.name=excluded.name AND platform.wa_templates.language=excluded.language;
 END IF;
 IF NOT FOUND THEN INSERT INTO platform.wa_financial_events(organization_id,kind) VALUES(p_org,'PROVISIONING_REJECTED');RETURN jsonb_build_object('code','FORBIDDEN');END IF;
 rid=coalesce(r.id,gen_random_uuid());
 INSERT INTO platform.wa_provider_records(id,organization_id,integration_id,pricing_id,template_id,source_type,source_identity,verification_method,verified_at,expires_at,policy_version,integrity_digest,payload_digest,approval_status)
 VALUES(rid,p_org,p_integration,CASE WHEN p_kind='pricing' THEN p_id END,CASE WHEN p_kind='template' THEN p_id END,p_source,p_identity,p_method,clock_timestamp(),p_until,p_policy,platform.wa_resource_digest(p_kind,p_id),p_digest,CASE WHEN p_source='SYNTHETIC_TEST' THEN 'VERIFIED' ELSE 'UNVERIFIED' END)
 ON CONFLICT(id) DO UPDATE SET source_type=excluded.source_type,source_identity=excluded.source_identity,verification_method=excluded.verification_method,verified_at=excluded.verified_at,expires_at=excluded.expires_at,policy_version=excluded.policy_version,integrity_digest=excluded.integrity_digest,payload_digest=excluded.payload_digest,approval_status=excluded.approval_status,revoked_at=NULL,version=platform.wa_provider_records.version+1;
 INSERT INTO platform.wa_financial_events(organization_id,provider_record_id,provider_version,provider_digest,kind) VALUES(p_org,rid,coalesce(r.version,0)+1,p_digest,CASE WHEN p_kind='pricing' AND r.id IS NULL THEN 'PRICING_POLICY_CREATED' WHEN p_kind='pricing' THEN 'PRICING_POLICY_VERIFIED' ELSE 'TEMPLATE_STATUS_UPDATED' END);
 IF p_kind='pricing' AND p_source='SYNTHETIC_TEST' THEN INSERT INTO platform.wa_financial_events(organization_id,provider_record_id,provider_version,provider_digest,kind) VALUES(p_org,rid,coalesce(r.version,0)+1,p_digest,'PRICING_POLICY_VERIFIED');END IF;
 RETURN jsonb_build_object('code','OK','version',coalesce(r.version,0)+1);
 EXCEPTION WHEN check_violation OR not_null_violation OR invalid_text_representation OR unique_violation OR foreign_key_violation OR invalid_datetime_format OR datetime_field_overflow THEN
 INSERT INTO platform.wa_financial_events(organization_id,kind) SELECT p_org,'PROVISIONING_REJECTED' WHERE EXISTS(SELECT 1 FROM platform.channel_integrations WHERE id=p_integration AND organization_id=p_org AND worker_hash=sha256(p_proof));
 RETURN jsonb_build_object('code','INVALID_PROVIDER_EVIDENCE');END $$;
CREATE FUNCTION platform.wa_revoke_provider(p_org uuid,p_id uuid,p_expected bigint) RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 DECLARE r platform.wa_provider_records;BEGIN
 IF NOT coalesce(platform.wa_is_operator(p_org),false) THEN INSERT INTO platform.wa_financial_events(organization_id,kind,actor_id) SELECT p_org,'UNAUTHORIZED_ATTEMPT',platform.current_user_id() WHERE platform.has_membership(p_org,false);RETURN jsonb_build_object('code','FORBIDDEN');END IF;
 SELECT * INTO r FROM platform.wa_provider_records WHERE organization_id=p_org AND id=p_id;
 IF r.id IS NULL THEN RETURN jsonb_build_object('code','NOT_FOUND');END IF;
 PERFORM pg_advisory_xact_lock(hashtextextended(r.integration_id::text,8210));
 SELECT * INTO r FROM platform.wa_provider_records WHERE id=p_id FOR UPDATE;
 IF NOT platform.wa_lock_operator(p_org) THEN INSERT INTO platform.wa_financial_events(organization_id,kind,actor_id) VALUES(p_org,'UNAUTHORIZED_ATTEMPT',platform.current_user_id());RETURN jsonb_build_object('code','FORBIDDEN');END IF;
 IF r.revoked_at IS NOT NULL THEN RETURN jsonb_build_object('code','OK');END IF;
 IF r.version IS DISTINCT FROM p_expected THEN RETURN jsonb_build_object('code','VERSION_CONFLICT');END IF;
 UPDATE platform.wa_provider_records SET revoked_at=clock_timestamp(),version=version+1 WHERE id=r.id;
 INSERT INTO platform.wa_financial_events(organization_id,provider_record_id,kind,actor_id) VALUES(p_org,r.id,CASE WHEN r.pricing_id IS NOT NULL THEN 'PRICING_POLICY_REVOKED' ELSE 'TEMPLATE_VERIFICATION_REVOKED' END,platform.current_user_id());RETURN jsonb_build_object('code','OK');END $$;
-- Record expiry once; validation itself always checks the clock, without relying on this sweep.
CREATE FUNCTION platform.wa_expire_provider(p_proof bytea) RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$DECLARE r platform.wa_provider_records;BEGIN
 FOR r IN SELECT x.* FROM platform.wa_provider_records x JOIN platform.channel_integrations i ON i.id=x.integration_id WHERE i.worker_hash=sha256(p_proof) AND x.expires_at<=clock_timestamp() AND NOT EXISTS(SELECT 1 FROM platform.wa_financial_events e WHERE e.provider_record_id=x.id AND e.kind=CASE WHEN x.template_id IS NOT NULL THEN 'TEMPLATE_VERIFICATION_EXPIRED' ELSE 'PRICING_POLICY_EXPIRED' END AND e.created_at>=x.verified_at) ORDER BY x.id LIMIT 100 FOR UPDATE OF x SKIP LOCKED LOOP
 INSERT INTO platform.wa_financial_events(organization_id,provider_record_id,kind) VALUES(r.organization_id,r.id,CASE WHEN r.template_id IS NOT NULL THEN 'TEMPLATE_VERIFICATION_EXPIRED' ELSE 'PRICING_POLICY_EXPIRED' END);END LOOP;END $$;
-- Apply evidence under integration, platform wallet, tenant wallet, reservation locks.
-- It changes financial assessment only; delivery_unknown remains terminal for dispatch.
CREATE FUNCTION platform.wa_reconcile_evidence(p_org uuid,p_res uuid,p_event uuid,p_version bigint,p_decision text,p_charge bigint,p_type text,p_ref text,p_at timestamptz,p_provider text) RETURNS jsonb
 LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 DECLARE r platform.wa_reservations;e platform.wa_reconciliation_evidence;dg bytea;outcome text;BEGIN
 IF NOT coalesce(platform.wa_is_operator(p_org),false) THEN
 INSERT INTO platform.wa_financial_events(organization_id,kind,actor_id) SELECT p_org,'UNAUTHORIZED_ATTEMPT',platform.current_user_id() WHERE platform.has_membership(p_org,false);RETURN jsonb_build_object('code','FORBIDDEN');END IF;
 SELECT * INTO r FROM platform.wa_reservations WHERE organization_id=p_org AND id=p_res;
 IF r.id IS NULL THEN RETURN jsonb_build_object('code','NOT_FOUND');END IF;
 PERFORM pg_advisory_xact_lock(hashtextextended(r.integration_id::text,8210));
 PERFORM 1 FROM platform.wa_platform_budget WHERE singleton FOR UPDATE;PERFORM 1 FROM platform.wa_budgets WHERE organization_id=p_org FOR UPDATE;
 SELECT * INTO r FROM platform.wa_reservations WHERE id=p_res FOR UPDATE;
 PERFORM 1 FROM platform.channel_outbox WHERE id=r.outbox_id FOR UPDATE;
 IF NOT platform.wa_lock_operator(p_org) THEN INSERT INTO platform.wa_financial_events(organization_id,kind,actor_id) VALUES(p_org,'UNAUTHORIZED_ATTEMPT',platform.current_user_id());RETURN jsonb_build_object('code','FORBIDDEN');END IF;
 dg=sha256(convert_to(jsonb_build_array(p_res,p_version,p_decision,p_charge,p_type,p_ref,p_at,p_provider)::text,'UTF8'));
 SELECT * INTO e FROM platform.wa_reconciliation_evidence WHERE id=p_event;
 IF e.id IS NOT NULL THEN
 IF e.organization_id=p_org AND e.reservation_id=p_res AND e.request_digest=dg THEN RETURN jsonb_build_object('code','OK','outcome',e.outcome);END IF;
 INSERT INTO platform.wa_financial_events(organization_id,reservation_id,kind,actor_id) VALUES(p_org,r.id,'RECONCILIATION_REJECTED',platform.current_user_id());RETURN jsonb_build_object('code','VERSION_CONFLICT');END IF;
 IF p_event IS NULL OR p_decision IS NULL OR p_type IS NULL OR p_ref IS NULL OR p_at IS NULL OR p_version IS NULL OR p_charge IS NULL OR p_decision NOT IN ('INSUFFICIENT','CONFIRMED_SENT','CONFIRMED_NOT_SENT') OR p_ref !~ '^(ticket|synthetic):[A-Za-z0-9_-]{8,80}$' OR p_at>clock_timestamp() OR p_at<r.created_at-interval '1 day' OR p_charge<0 OR p_charge>r.amount OR (p_decision='CONFIRMED_NOT_SENT' AND p_charge<>0) OR (p_decision='CONFIRMED_SENT' AND p_charge=0 AND r.amount>0) OR (p_decision='INSUFFICIENT' AND p_charge<>0) OR (p_type NOT IN ('SYNTHETIC_TEST','OPERATOR_REVIEW')) OR (NOT r.synthetic AND (p_type='SYNTHETIC_TEST' OR p_ref LIKE 'synthetic:%')) OR (p_provider IS NOT NULL AND p_provider !~ '^[A-Za-z0-9._:-]{1,200}$') THEN
 INSERT INTO platform.wa_financial_events(organization_id,reservation_id,kind,actor_id) VALUES(p_org,r.id,'RECONCILIATION_REJECTED',platform.current_user_id());RETURN jsonb_build_object('code','INVALID_EVIDENCE');END IF;
 IF r.version<>p_version OR r.status<>'unknown' THEN
 INSERT INTO platform.wa_financial_events(organization_id,reservation_id,kind,actor_id) VALUES(p_org,r.id,'RECONCILIATION_REJECTED',platform.current_user_id());RETURN jsonb_build_object('code','VERSION_CONFLICT');END IF;
 INSERT INTO platform.wa_financial_events(organization_id,reservation_id,kind,actor_id) VALUES(p_org,r.id,'RECONCILIATION_STARTED',platform.current_user_id());
 outcome=CASE WHEN p_decision='INSUFFICIENT' THEN 'INSUFFICIENT' WHEN EXISTS(SELECT 1 FROM platform.wa_reconciliation_evidence WHERE reservation_id=r.id AND decision<>'INSUFFICIENT' AND decision<>p_decision) OR (nullif(r.provider_id,'') IS NOT NULL AND p_provider IS NOT NULL AND r.provider_id<>p_provider) OR (p_decision='CONFIRMED_NOT_SENT' AND (nullif(r.provider_id,'') IS NOT NULL OR EXISTS(SELECT 1 FROM platform.channel_outbox WHERE id=r.outbox_id AND (nullif(provider_id,'') IS NOT NULL OR status IN ('sent','delivered','read'))))) THEN 'CONFLICT' ELSE 'COMPLETED' END;
 INSERT INTO platform.wa_reconciliation_evidence(id,organization_id,reservation_id,integration_id,outbox_id,provider_id,evidence_type,evidence_reference,evidence_at,operator_id,decision,charge,request_digest,outcome)
 VALUES(p_event,p_org,r.id,r.integration_id,r.outbox_id,p_provider,p_type,p_ref,p_at,platform.current_user_id(),p_decision,p_charge,dg,outcome);
 INSERT INTO platform.wa_financial_events(organization_id,reservation_id,evidence_id,kind,actor_id) VALUES(p_org,r.id,p_event,'RECONCILIATION_EVIDENCE_ATTACHED',platform.current_user_id());
 IF outcome='COMPLETED' THEN
 UPDATE platform.wa_budgets SET reserved=reserved-r.amount,spent=spent+p_charge WHERE organization_id=p_org;
 UPDATE platform.wa_platform_budget SET reserved=reserved-r.amount,spent=spent+p_charge WHERE singleton;
 UPDATE platform.wa_reservations SET status=CASE WHEN p_charge=0 THEN 'released' ELSE 'charged' END,charged=p_charge,version=version+1,updated_at=clock_timestamp() WHERE id=r.id;
 ELSE UPDATE platform.wa_reservations SET version=version+1,updated_at=clock_timestamp() WHERE id=r.id;END IF;
 INSERT INTO platform.wa_financial_events(organization_id,reservation_id,evidence_id,kind,actor_id) VALUES(p_org,r.id,p_event,CASE WHEN outcome='COMPLETED' THEN 'RECONCILIATION_COMPLETED' ELSE 'RECONCILIATION_REJECTED' END,platform.current_user_id());
 RETURN jsonb_build_object('code','OK','outcome',outcome);
 END $$;
CREATE FUNCTION platform.wa_operator_state(p_org uuid) RETURNS jsonb LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$BEGIN
 IF NOT coalesce(platform.wa_is_operator(p_org),false) THEN RETURN jsonb_build_object('code','FORBIDDEN');END IF;
 RETURN jsonb_build_object('code','OK','organization_id',p_org,'store',(SELECT name FROM platform.organizations WHERE id=p_org),'channel','whatsapp','items',coalesce((SELECT jsonb_agg(jsonb_build_object('id',r.id,'outbox_id',r.outbox_id,'integration_id',r.integration_id,'created_at',r.created_at,'amount',r.amount,'status',r.status,'version',r.version,'delivery_state',q.status,'synthetic',r.synthetic,'history',coalesce((SELECT jsonb_agg(to_jsonb(e)-'request_digest' ORDER BY e.decision_at) FROM platform.wa_reconciliation_evidence e WHERE e.reservation_id=r.id),'[]'::jsonb))) FROM (SELECT * FROM platform.wa_reservations WHERE organization_id=p_org AND (status='unknown' OR EXISTS(SELECT 1 FROM platform.wa_reconciliation_evidence e WHERE reservation_id=platform.wa_reservations.id)) ORDER BY created_at DESC LIMIT 100)r JOIN platform.channel_outbox q ON q.id=r.outbox_id),'[]'::jsonb));END $$;
-- A delayed receipt never settles money or retries. Contradictory late evidence is auditable.
CREATE FUNCTION platform.wa_late_evidence_audit() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$DECLARE r platform.wa_reservations;BEGIN
 IF (NEW.status,NEW.provider_id) IS DISTINCT FROM (OLD.status,OLD.provider_id) AND NEW.status IN ('sent','delivered','read') THEN
 FOR r IN SELECT x.* FROM platform.wa_reservations x WHERE x.outbox_id=NEW.id AND EXISTS(SELECT 1 FROM platform.wa_reconciliation_evidence e WHERE e.reservation_id=x.id AND e.decision='CONFIRMED_NOT_SENT' AND e.outcome='COMPLETED') LOOP
 INSERT INTO platform.wa_financial_events(organization_id,reservation_id,kind) VALUES(r.organization_id,r.id,'LATE_PROVIDER_EVIDENCE_CONFLICT');END LOOP;END IF;RETURN NEW;END $$;
CREATE TRIGGER wa_late_evidence_audit AFTER UPDATE OF status,provider_id ON platform.channel_outbox FOR EACH ROW EXECUTE FUNCTION platform.wa_late_evidence_audit();
REVOKE ALL ON FUNCTION platform.wa_late_evidence_audit() FROM PUBLIC;
CREATE FUNCTION platform.wa_operator_grant_audit() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$BEGIN
 INSERT INTO platform.wa_financial_events(organization_id,kind,evidence_hash) VALUES(NEW.organization_id,'OPERATOR_ASSIGNMENT_CHANGED',sha256(convert_to(NEW.assigned_by,'UTF8')));RETURN NEW;END $$;
CREATE TRIGGER wa_operator_grant_audit AFTER INSERT OR UPDATE ON platform.wa_operator_grants FOR EACH ROW EXECUTE FUNCTION platform.wa_operator_grant_audit();
REVOKE ALL ON FUNCTION platform.wa_operator_grant_audit() FROM PUBLIC;
CREATE FUNCTION platform.wa_resource_status(p_org uuid,p_kind text,p_id uuid) RETURNS text LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$DECLARE r platform.wa_provider_records;BEGIN
 IF p_org::text IS DISTINCT FROM current_setting('app.organization_id',true) OR NOT (platform.has_membership(p_org,true) OR platform.wa_is_operator(p_org)) THEN RETURN 'UNAVAILABLE';END IF;
 SELECT * INTO r FROM platform.wa_provider_records WHERE organization_id=p_org AND (CASE WHEN p_kind='pricing' THEN pricing_id ELSE template_id END)=p_id;
 IF r.id IS NULL THEN RETURN 'UNVERIFIED';ELSIF r.revoked_at IS NOT NULL THEN RETURN 'REVOKED';ELSIF r.expires_at<=clock_timestamp() THEN RETURN 'EXPIRED';ELSIF r.integrity_digest IS DISTINCT FROM platform.wa_resource_digest(p_kind,p_id) THEN RETURN 'INTEGRITY_MISMATCH';ELSIF r.source_type='SYNTHETIC_TEST' THEN RETURN 'SYNTHETIC_NOT_LIVE';ELSE RETURN 'NO_OFFICIAL_PROVIDER_INSTALLED';END IF;END $$;
CREATE FUNCTION platform.wa_provider_version(p_proof bytea,p_integration uuid,p_template uuid) RETURNS bigint LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT coalesce((SELECT r.version FROM platform.wa_provider_records r JOIN platform.channel_integrations i ON i.id=r.integration_id WHERE i.id=p_integration AND i.worker_hash=sha256(p_proof) AND r.template_id=p_template),0) $$;
DO $$DECLARE f record;BEGIN FOR f IN SELECT p.oid::regprocedure sig,p.proname FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='platform' AND p.proname IN ('wa_resource_status','wa_provider_version','wa_is_operator','wa_resource_digest','wa_provenance_valid','wa_provision','wa_revoke_provider','wa_expire_provider','wa_reconcile_evidence','wa_operator_state') LOOP
 EXECUTE format('REVOKE ALL ON FUNCTION %s FROM PUBLIC',f.sig);
 IF f.proname NOT IN ('wa_resource_digest','wa_provenance_valid') THEN EXECUTE format('GRANT EXECUTE ON FUNCTION %s TO aba_app',f.sig);END IF;END LOOP;END $$;
UPDATE platform.service_metadata SET version=12 WHERE singleton;
-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DO $$BEGIN IF EXISTS(SELECT 1 FROM platform.channel_principals WHERE recipient_region IS NOT NULL) OR EXISTS(SELECT 1 FROM platform.wa_reservations) OR EXISTS(SELECT 1 FROM platform.wa_budgets) OR EXISTS(SELECT 1 FROM platform.wa_operator_grants) OR EXISTS(SELECT 1 FROM platform.wa_provider_records) OR EXISTS(SELECT 1 FROM platform.wa_reconciliation_evidence) THEN RAISE EXCEPTION 'rollback refused: preserve provider/operator/evidence data';END IF;END $$;
DO $$DECLARE r record;BEGIN FOR r IN SELECT definition FROM platform.wa_function_backup WHERE signature LIKE '9c:%' LOOP EXECUTE r.definition;END LOOP;DELETE FROM platform.wa_function_backup WHERE signature LIKE '9c:%';END $$;
DROP TRIGGER wa_late_evidence_audit ON platform.channel_outbox;
DROP FUNCTION platform.wa_late_evidence_audit();
ALTER TABLE platform.wa_financial_events DROP COLUMN provider_record_id,DROP COLUMN evidence_id,DROP COLUMN provider_version,DROP COLUMN provider_digest;
DROP FUNCTION platform.wa_resource_status(uuid,text,uuid),platform.wa_provider_version(bytea,uuid,uuid),platform.wa_operator_state(uuid),platform.wa_reconcile_evidence(uuid,uuid,uuid,bigint,text,bigint,text,text,timestamptz,text),platform.wa_expire_provider(bytea),platform.wa_revoke_provider(uuid,uuid,bigint),platform.wa_provision(bytea,uuid,uuid,text,uuid,bigint,text,text,text,text,timestamptz,text,bytea),platform.wa_provenance_valid(text,uuid,boolean),platform.wa_resource_digest(text,uuid);
DROP TABLE platform.wa_reconciliation_evidence,platform.wa_provider_records;
DROP POLICY assigned_read ON platform.wa_operator_grants;
DROP FUNCTION platform.wa_lock_operator(uuid),platform.wa_is_operator(uuid);
DROP TRIGGER wa_operator_grant_audit ON platform.wa_operator_grants;
DROP FUNCTION platform.wa_operator_grant_audit();
DROP TABLE platform.wa_operator_grants;
UPDATE platform.service_metadata SET version=11 WHERE singleton;
-- +goose StatementEnd
