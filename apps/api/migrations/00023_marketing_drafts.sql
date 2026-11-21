-- +goose Up
-- +goose StatementBegin
CREATE TABLE platform.marketing_drafts (
 id uuid NOT NULL,
 organization_id uuid NOT NULL,
 source_execution_id uuid NOT NULL,
 version integer NOT NULL CHECK(version>0),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 goal text NOT NULL CHECK(length(goal) BETWEEN 1 AND 500),
 audience text NOT NULL CHECK(length(audience) BETWEEN 1 AND 500),
 channel text NOT NULL CHECK(channel IN ('internal_review','instagram','telegram','whatsapp')),
 content text NOT NULL CHECK(length(content) BETWEEN 1 AND 8000),
 status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','approved','rejected')),
 reason text CHECK(length(reason) BETWEEN 1 AND 500),
 decided_by uuid REFERENCES platform.users(id),
 decided_at timestamptz,
 PRIMARY KEY(organization_id,id,version),
 FOREIGN KEY(organization_id,id,source_execution_id) REFERENCES platform.agent_executions(organization_id,task_id,id),
 CHECK((status='pending')=(decided_at IS NULL)),
 CHECK((status='pending')=(decided_by IS NULL)),
 CHECK(reason IS NULL OR status='rejected')
);
ALTER TABLE platform.marketing_drafts ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.marketing_drafts FORCE ROW LEVEL SECURITY;
CREATE POLICY migration_owner ON platform.marketing_drafts TO aba_migrator USING(true) WITH CHECK(true);
CREATE POLICY owner_read ON platform.marketing_drafts FOR SELECT TO aba_app USING(platform.agent_owner(organization_id));
GRANT SELECT ON platform.marketing_drafts TO aba_app;

CREATE FUNCTION platform.capture_marketing_draft() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE draft jsonb;
BEGIN
 IF NEW.status='completed' AND OLD.status='running' THEN
  draft:=NEW.result->0;
  IF draft->>'generation'='synthetic_contextual' AND draft->>'external_communication'='false'
     AND EXISTS(SELECT 1 FROM platform.agent_tasks WHERE organization_id=NEW.organization_id AND id=NEW.task_id AND agent_id='marketing' AND action='draft')
     AND draft ?& ARRAY['goal','audience','channel','text'] THEN
   INSERT INTO platform.marketing_drafts(id,organization_id,source_execution_id,version,goal,audience,channel,content)
   VALUES(NEW.task_id,NEW.organization_id,NEW.id,1,draft->>'goal',draft->>'audience',draft->>'channel',draft->>'text');
   INSERT INTO platform.agent_audit(organization_id,task_id,execution_id,agent_id,action,result)
   VALUES(NEW.organization_id,NEW.task_id,NEW.id,'marketing','draft.persist','pending');
  END IF;
 END IF;
 RETURN NEW;
END $$;
REVOKE ALL ON FUNCTION platform.capture_marketing_draft() FROM PUBLIC;
CREATE TRIGGER capture_marketing_draft AFTER UPDATE OF status ON platform.agent_executions
 FOR EACH ROW EXECUTE FUNCTION platform.capture_marketing_draft();

CREATE FUNCTION platform.marketing_draft_action(p_org uuid,p_id uuid,p_version integer,p_action text,p_content text,p_reason text)
RETURNS boolean LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE d platform.marketing_drafts; uid uuid:=platform.current_user_id();
BEGIN
 IF NOT platform.agent_owner(p_org) OR uid IS NULL THEN RETURN false; END IF;
 PERFORM pg_advisory_xact_lock(hashtextextended(p_org::text||p_id::text,30823));
 SELECT * INTO d FROM platform.marketing_drafts WHERE organization_id=p_org AND id=p_id ORDER BY version DESC LIMIT 1 FOR UPDATE;
 IF NOT FOUND OR d.version<>p_version THEN RETURN false; END IF;
 IF p_action IN ('approved','rejected') AND d.status='pending' AND p_content IS NULL
    AND (p_reason IS NULL OR (p_action='rejected' AND length(btrim(p_reason)) BETWEEN 1 AND 500)) THEN
  UPDATE platform.marketing_drafts SET status=p_action,reason=p_reason,decided_by=uid,decided_at=clock_timestamp()
  WHERE organization_id=p_org AND id=p_id AND version=p_version;
 ELSIF p_action='revise' AND d.status='rejected' AND p_reason IS NULL AND length(btrim(p_content)) BETWEEN 1 AND 8000 THEN
  INSERT INTO platform.marketing_drafts(id,organization_id,source_execution_id,version,goal,audience,channel,content)
  VALUES(d.id,d.organization_id,d.source_execution_id,d.version+1,d.goal,d.audience,d.channel,p_content);
 ELSE RETURN false;
 END IF;
 INSERT INTO platform.agent_audit(organization_id,task_id,execution_id,actor_id,agent_id,action,result)
 VALUES(p_org,p_id,d.source_execution_id,uid,'marketing','draft.review',p_action);
 RETURN true;
END $$;
REVOKE ALL ON FUNCTION platform.marketing_draft_action(uuid,uuid,integer,text,text,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.marketing_draft_action(uuid,uuid,integer,text,text,text) TO aba_app;
UPDATE platform.service_metadata SET version=23 WHERE singleton;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM platform.marketing_drafts) THEN
  RAISE EXCEPTION 'rollback refused: preserve marketing drafts';
 END IF;
END $$;
DROP TRIGGER capture_marketing_draft ON platform.agent_executions;
DROP FUNCTION platform.capture_marketing_draft(),platform.marketing_draft_action(uuid,uuid,integer,text,text,text);
DROP TABLE platform.marketing_drafts;
UPDATE platform.service_metadata SET version=22 WHERE singleton;
-- +goose StatementEnd
