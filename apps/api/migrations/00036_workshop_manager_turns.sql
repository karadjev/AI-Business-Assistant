-- +goose Up
CREATE TABLE platform.workshop_manager_turns (
 organization_id uuid NOT NULL REFERENCES platform.organizations(id),
 actor_id uuid NOT NULL REFERENCES platform.users(id),
 id uuid NOT NULL,
 body text NOT NULL CHECK(length(body) BETWEEN 1 AND 2000),
 reply text NOT NULL CHECK(length(reply) BETWEEN 1 AND 2000),
 request_hint uuid,
 context jsonb CHECK(octet_length(context::text)<=4096),
 task_id uuid,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(organization_id,actor_id,id),
 FOREIGN KEY(organization_id,task_id) REFERENCES platform.agent_tasks(organization_id,id)
);
CREATE INDEX workshop_manager_timeline ON platform.workshop_manager_turns(organization_id,actor_id,created_at DESC,id DESC);
ALTER TABLE platform.workshop_manager_turns ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.workshop_manager_turns FORCE ROW LEVEL SECURITY;
CREATE POLICY migration_owner ON platform.workshop_manager_turns TO aba_migrator USING(true) WITH CHECK(true);
CREATE POLICY owner_read ON platform.workshop_manager_turns FOR SELECT TO aba_app
 USING(platform.workshop_owner(organization_id) AND platform.current_user_id()=actor_id);
CREATE POLICY owner_insert ON platform.workshop_manager_turns FOR INSERT TO aba_app
 WITH CHECK(platform.workshop_owner(organization_id) AND platform.current_user_id()=actor_id);
GRANT SELECT,INSERT ON platform.workshop_manager_turns TO aba_app;
-- +goose StatementBegin
-- Keep the original request, executions and attempt limits. Only analysis can
-- be restarted and only before it has ever produced a reserve approval.
CREATE FUNCTION platform.workshop_retry_analysis(p_org uuid,p_request uuid) RETURNS uuid
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE r platform.workshop_requests; root platform.agent_tasks; child platform.agent_tasks;
 actor uuid:=platform.current_user_id(); due timestamptz:=clock_timestamp()+interval '15 minutes';
BEGIN
 IF NOT platform.workshop_owner(p_org) THEN RETURN NULL; END IF;
 SELECT * INTO r FROM platform.workshop_requests WHERE organization_id=p_org AND id=p_request FOR UPDATE;
 IF NOT FOUND OR r.reserve_task_id IS NOT NULL OR r.reserved_at IS NOT NULL OR r.closed_at IS NOT NULL THEN RETURN NULL; END IF;
 SELECT * INTO root FROM platform.agent_tasks WHERE organization_id=p_org AND id=p_request AND
  action='delegate_workshop' AND status='failed' AND initiator=actor FOR UPDATE;
 IF NOT FOUND THEN RETURN NULL; END IF;
 PERFORM 1 FROM platform.channel_inbox WHERE organization_id=p_org AND agent_task_id=root.id AND status='failed' AND attempts<3 FOR UPDATE;
 IF NOT FOUND THEN RETURN NULL; END IF;
 IF EXISTS(SELECT 1 FROM platform.agent_tasks x WHERE x.organization_id=p_org AND x.parent_id=root.id
  AND (x.delegation_step IS NULL OR x.status IN ('pending','queued','running','waiting_approval'))) THEN RETURN NULL; END IF;
 SELECT * INTO child FROM platform.agent_tasks WHERE organization_id=p_org AND parent_id=root.id AND delegation_step IS NOT NULL
  AND status<>'completed' ORDER BY delegation_step LIMIT 1 FOR UPDATE;
 IF NOT FOUND OR child.status NOT IN ('failed','cancelled') OR EXISTS(
  SELECT 1 FROM platform.channel_inbox q JOIN platform.agent_tasks x ON x.organization_id=q.organization_id AND x.id=q.agent_task_id
  WHERE x.organization_id=p_org AND x.parent_id=root.id AND x.delegation_step>child.delegation_step) THEN RETURN NULL; END IF;
 IF EXISTS(SELECT 1 FROM platform.channel_inbox WHERE organization_id=p_org AND agent_task_id=child.id AND (status<>'failed' OR attempts>=3))
 THEN RETURN NULL; END IF;
 UPDATE platform.agent_tasks SET status='queued',error_code=NULL,completed_at=NULL,deadline=due WHERE organization_id=p_org AND id=root.id;
 UPDATE platform.agent_tasks SET status=CASE WHEN id=child.id THEN 'queued' ELSE 'pending' END,error_code=NULL,completed_at=NULL,deadline=due
  WHERE organization_id=p_org AND parent_id=root.id AND delegation_step>=child.delegation_step AND status<>'completed';
 UPDATE platform.channel_inbox SET status='pending',available_at=clock_timestamp(),agent_lease=NULL,lease_until=NULL
  WHERE organization_id=p_org AND agent_task_id=root.id;
 INSERT INTO platform.channel_inbox(id,organization_id,external_id,external_user_id,payload,channel,agent_task_id)
  VALUES(child.id,p_org,child.id::text,'','{}','agent',child.id)
 ON CONFLICT(id) DO UPDATE SET status='pending',available_at=clock_timestamp(),agent_lease=NULL,lease_until=NULL;
 INSERT INTO platform.agent_audit(organization_id,task_id,actor_id,agent_id,action,result)
 VALUES(p_org,root.id,actor,'business_manager','task.retry','queued');
 RETURN root.id;
END $$;
REVOKE ALL ON FUNCTION platform.workshop_retry_analysis(uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.workshop_retry_analysis(uuid,uuid) TO aba_app;
-- +goose StatementEnd
UPDATE platform.service_metadata SET version=36 WHERE singleton;

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM platform.workshop_manager_turns) THEN
  RAISE EXCEPTION 'rollback refused: preserve manager conversation history';
 END IF;
END $$;
-- +goose StatementEnd
DROP FUNCTION platform.workshop_retry_analysis(uuid,uuid);
DROP TABLE platform.workshop_manager_turns;
UPDATE platform.service_metadata SET version=35 WHERE singleton;
