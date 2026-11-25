-- +goose Up
CREATE TABLE platform.commercial_plans (
 code text PRIMARY KEY,
 name text NOT NULL
);
INSERT INTO platform.commercial_plans VALUES ('pilot','Пилот'),('standard','Стандартный'),('custom','Индивидуальный');

CREATE TABLE platform.commercial_admin_grants (
 organization_id uuid NOT NULL REFERENCES platform.organizations(id),
 user_id uuid NOT NULL REFERENCES platform.users(id),
 PRIMARY KEY(organization_id,user_id)
);
CREATE TABLE platform.commercial_subscriptions (
 organization_id uuid PRIMARY KEY REFERENCES platform.organizations(id),
 plan_code text NOT NULL REFERENCES platform.commercial_plans(code),
 status text NOT NULL CHECK(status IN ('active','paused')),
 staff_limit integer CHECK(staff_limit BETWEEN 0 AND 1000),
 agent_limit integer CHECK(agent_limit BETWEEN 0 AND 7),
 channel_limit integer CHECK(channel_limit BETWEEN 0 AND 3),
 task_limit integer CHECK(task_limit BETWEEN 0 AND 1000000),
 ai_monthly_cap bigint CHECK(ai_monthly_cap BETWEEN 0 AND 1000000000000),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 fence bigint NOT NULL DEFAULT 0
);
CREATE TABLE platform.commercial_audit (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 organization_id uuid NOT NULL REFERENCES platform.organizations(id),
 actor_id uuid NOT NULL REFERENCES platform.users(id),
 old_settings jsonb,
 new_settings jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX commercial_tasks_usage ON platform.agent_tasks(organization_id,created_at,agent_id);
CREATE INDEX commercial_model_usage ON platform.llm_reservations(organization_id,month_key);

-- +goose StatementBegin
DO $$ DECLARE name text; BEGIN
 FOREACH name IN ARRAY ARRAY['commercial_plans','commercial_admin_grants','commercial_subscriptions','commercial_audit'] LOOP
  EXECUTE format('ALTER TABLE platform.%I ENABLE ROW LEVEL SECURITY',name);
  EXECUTE format('ALTER TABLE platform.%I FORCE ROW LEVEL SECURITY',name);
  EXECUTE format('CREATE POLICY migration_owner ON platform.%I TO aba_migrator USING(true) WITH CHECK(true)',name);
 END LOOP;
END $$;

CREATE FUNCTION platform.commercial_admin(p_org uuid) RETURNS boolean
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT coalesce(p_org::text=current_setting('app.organization_id',true) AND platform.has_membership(p_org,true)
 AND EXISTS(SELECT 1 FROM platform.commercial_admin_grants WHERE organization_id=p_org AND user_id=platform.current_user_id()),false)
$$;

CREATE FUNCTION platform.commercial_state(p_org uuid) RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE period timestamptz:=date_trunc('month',clock_timestamp(),'UTC');
 cycle_key text:='month:'||to_char(period AT TIME ZONE 'UTC','YYYY-MM');
 sub jsonb;
BEGIN
 IF p_org::text IS DISTINCT FROM current_setting('app.organization_id',true) OR NOT platform.has_membership(p_org,true) THEN
  RAISE EXCEPTION 'owner required' USING ERRCODE='42501';
 END IF;
 SELECT to_jsonb(s)-'fence'-'organization_id' INTO sub FROM platform.commercial_subscriptions s WHERE organization_id=p_org;
 RETURN jsonb_build_object('subscription',coalesce(sub,'{"plan_code":"pilot","status":"active","version":0}'::jsonb),
  'can_edit',platform.commercial_admin(p_org),
  'plans',(SELECT jsonb_agg(to_jsonb(p) ORDER BY code) FROM platform.commercial_plans p),
  'cycle_start',period,
  'usage',jsonb_build_object(
   'staff',(SELECT count(*) FROM platform.organization_members WHERE organization_id=p_org AND role='member'),
   'channels',(SELECT count(*) FROM platform.telegram_integrations WHERE organization_id=p_org AND active)+
    (SELECT count(*) FROM platform.channel_integrations WHERE organization_id=p_org AND active),
   'tasks',(SELECT count(*) FROM platform.agent_tasks WHERE organization_id=p_org AND created_at>=period),
   'agents',(SELECT count(DISTINCT agent_id) FROM platform.agent_tasks WHERE organization_id=p_org AND created_at>=period),
   'ai_exposure',(SELECT coalesce(sum(CASE WHEN status='settled' THEN coalesce(charged,amount) ELSE amount END),0)
       FROM platform.llm_reservations l WHERE organization_id=p_org AND l.month_key=cycle_key)));
END $$;

CREATE FUNCTION platform.commercial_configure(p_org uuid,p_expected bigint,p_plan text,p_status text,
 p_staff integer,p_agents integer,p_channels integer,p_tasks integer,p_ai bigint) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE before_value jsonb; after_value jsonb; current_version bigint;
BEGIN
 IF NOT platform.commercial_admin(p_org) THEN RAISE EXCEPTION 'commercial admin required' USING ERRCODE='42501'; END IF;
 PERFORM pg_advisory_xact_lock(hashtextextended(p_org::text,8381));
 SELECT to_jsonb(s)-'fence'-'organization_id',version INTO before_value,current_version
 FROM platform.commercial_subscriptions s WHERE organization_id=p_org FOR UPDATE;
 IF coalesce(current_version,0)<>p_expected THEN RETURN false; END IF;
 INSERT INTO platform.commercial_subscriptions AS s(organization_id,plan_code,status,staff_limit,agent_limit,channel_limit,task_limit,ai_monthly_cap)
 VALUES(p_org,p_plan,p_status,p_staff,p_agents,p_channels,p_tasks,p_ai)
 ON CONFLICT (organization_id) DO UPDATE SET plan_code=excluded.plan_code,status=excluded.status,
   staff_limit=excluded.staff_limit,agent_limit=excluded.agent_limit,channel_limit=excluded.channel_limit,
   task_limit=excluded.task_limit,ai_monthly_cap=excluded.ai_monthly_cap,version=s.version+1
 RETURNING to_jsonb(s)-'fence'-'organization_id' INTO after_value;
 INSERT INTO platform.commercial_audit(organization_id,actor_id,old_settings,new_settings)
 VALUES(p_org,platform.current_user_id(),before_value,after_value);
 RETURN true;
END $$;

CREATE FUNCTION platform.enforce_commercial_admission() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE p_org uuid:=NEW.organization_id; sub platform.commercial_subscriptions;
 period timestamptz:=date_trunc('month',clock_timestamp(),'UTC');
 denied text;
BEGIN
 IF TG_ARGV[0]='channel' THEN
  IF NOT NEW.active THEN RETURN NEW; END IF;
  IF TG_OP='UPDATE' AND OLD.active THEN RETURN NEW; END IF;
 ELSIF TG_ARGV[0]='staff' THEN
  IF NEW.role<>'member' THEN RETURN NEW; END IF;
 END IF;
 PERFORM pg_advisory_xact_lock(hashtextextended(p_org::text,8381));
 -- The row write fences concurrent snapshots even at stricter isolation levels.
 UPDATE platform.commercial_subscriptions SET fence=fence+1 WHERE organization_id=p_org RETURNING * INTO sub;
 IF NOT FOUND THEN RETURN NEW; END IF;
 IF sub.status='paused' THEN denied:='paused';
 ELSIF TG_ARGV[0]='staff' AND sub.staff_limit IS NOT NULL AND
  (SELECT count(*) FROM platform.organization_members WHERE organization_id=p_org AND role='member')>sub.staff_limit THEN denied:='staff';
 ELSIF TG_ARGV[0]='channel' AND sub.channel_limit IS NOT NULL AND
  (SELECT count(*) FROM platform.telegram_integrations WHERE organization_id=p_org AND active)+
  (SELECT count(*) FROM platform.channel_integrations WHERE organization_id=p_org AND active)>sub.channel_limit THEN denied:='channels';
 ELSIF TG_ARGV[0]='task' THEN
  IF sub.task_limit IS NOT NULL AND
   (SELECT count(*) FROM platform.agent_tasks WHERE organization_id=p_org AND created_at>=period)>sub.task_limit THEN denied:='tasks';
  ELSIF sub.agent_limit IS NOT NULL AND
   (SELECT count(DISTINCT agent_id) FROM platform.agent_tasks WHERE organization_id=p_org AND created_at>=period)>sub.agent_limit THEN denied:='agents';
  END IF;
 ELSIF TG_ARGV[0]='model' THEN
  IF sub.ai_monthly_cap IS NOT NULL AND
   (SELECT coalesce(sum(CASE WHEN status='settled' THEN coalesce(charged,amount) ELSE amount END),0)
    FROM platform.llm_reservations l WHERE organization_id=p_org AND l.month_key=NEW.month_key)>sub.ai_monthly_cap THEN denied:='ai';
  END IF;
 END IF;
 IF denied IS NOT NULL THEN RAISE EXCEPTION 'commercial allowance exhausted' USING ERRCODE='P4L01', DETAIL=denied; END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION platform.commercial_admin(uuid),platform.enforce_commercial_admission(),
 platform.commercial_state(uuid),platform.commercial_configure(uuid,bigint,text,text,integer,integer,integer,integer,bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.commercial_state(uuid),
 platform.commercial_configure(uuid,bigint,text,text,integer,integer,integer,integer,bigint) TO aba_app;

CREATE TRIGGER commercial_staff_limit AFTER INSERT ON platform.organization_members
 FOR EACH ROW EXECUTE FUNCTION platform.enforce_commercial_admission('staff');
CREATE TRIGGER commercial_task_limit AFTER INSERT ON platform.agent_tasks
 FOR EACH ROW EXECUTE FUNCTION platform.enforce_commercial_admission('task');
CREATE TRIGGER commercial_telegram_limit AFTER INSERT OR UPDATE OF active ON platform.telegram_integrations
 FOR EACH ROW EXECUTE FUNCTION platform.enforce_commercial_admission('channel');
CREATE TRIGGER commercial_channel_limit AFTER INSERT OR UPDATE OF active ON platform.channel_integrations
 FOR EACH ROW EXECUTE FUNCTION platform.enforce_commercial_admission('channel');
CREATE TRIGGER commercial_model_limit AFTER INSERT ON platform.llm_reservations
 FOR EACH ROW EXECUTE FUNCTION platform.enforce_commercial_admission('model');
UPDATE platform.service_metadata SET version=38 WHERE singleton;

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM platform.commercial_subscriptions) OR EXISTS(SELECT 1 FROM platform.commercial_admin_grants) THEN
  RAISE EXCEPTION 'rollback refused: preserve subscriptions and grants';
 END IF;
END $$;
-- +goose StatementEnd
DROP TRIGGER commercial_staff_limit ON platform.organization_members;
DROP TRIGGER commercial_task_limit ON platform.agent_tasks;
DROP TRIGGER commercial_telegram_limit ON platform.telegram_integrations;
DROP TRIGGER commercial_channel_limit ON platform.channel_integrations;
DROP TRIGGER commercial_model_limit ON platform.llm_reservations;
DROP FUNCTION platform.commercial_state(uuid),platform.enforce_commercial_admission(),
 platform.commercial_configure(uuid,bigint,text,text,integer,integer,integer,integer,bigint),platform.commercial_admin(uuid);
DROP INDEX platform.commercial_tasks_usage,platform.commercial_model_usage;
DROP TABLE platform.commercial_audit,platform.commercial_subscriptions,platform.commercial_admin_grants,platform.commercial_plans;
UPDATE platform.service_metadata SET version=37 WHERE singleton;
