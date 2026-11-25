-- +goose Up
ALTER TABLE platform.organizations ADD COLUMN inbox_review_enabled boolean NOT NULL DEFAULT false CHECK(NOT inbox_review_enabled OR is_test);

CREATE TABLE platform.inbox_reply_drafts (
 organization_id uuid NOT NULL,
 conversation_id uuid NOT NULL,
 task_id uuid NOT NULL,
 body text NOT NULL CHECK(octet_length(body) BETWEEN 1 AND 1000),
 expires_at timestamptz NOT NULL,
 status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','approved','rejected')),
 decided_by uuid REFERENCES platform.users(id),
 decided_at timestamptz,
 PRIMARY KEY(organization_id,task_id),
 FOREIGN KEY(organization_id,task_id) REFERENCES platform.agent_tasks(organization_id,id),
 FOREIGN KEY(organization_id,conversation_id) REFERENCES platform.conversations(organization_id,id)
);
CREATE INDEX inbox_reply_drafts_conversation ON platform.inbox_reply_drafts(organization_id,conversation_id,expires_at DESC);
ALTER TABLE platform.inbox_reply_drafts ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.inbox_reply_drafts FORCE ROW LEVEL SECURITY;
CREATE POLICY migration_owner ON platform.inbox_reply_drafts TO aba_migrator USING(true) WITH CHECK(true);
CREATE POLICY staff_read ON platform.inbox_reply_drafts FOR SELECT TO aba_app USING(
 organization_id::text=current_setting('app.organization_id',true)
 AND platform.is_test_org(organization_id) AND platform.can_operate_orders(organization_id));
GRANT SELECT ON platform.inbox_reply_drafts TO aba_app;

-- +goose StatementBegin
CREATE FUNCTION platform.set_inbox_review(p_org uuid) RETURNS void
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
BEGIN
 IF NOT platform.agent_owner(p_org) OR NOT platform.is_test_org(p_org) THEN
  RAISE EXCEPTION 'owner of test organization required' USING ERRCODE='42501';
 END IF;
 UPDATE platform.organizations SET inbox_review_enabled=true WHERE id=p_org AND NOT inbox_review_enabled;
 IF FOUND THEN
  INSERT INTO platform.team_audit(organization_id,actor_id,action) VALUES(p_org,platform.current_user_id(),'inbox.enabled');
 END IF;
END $$;

ALTER FUNCTION platform.conversation_publish(platform.agent_tasks,uuid,text) RENAME TO publish_conversation_reply_previous;
CREATE FUNCTION platform.conversation_publish(p_task platform.agent_tasks,p_execution uuid,p_body text) RETURNS void
LANGUAGE plpgsql SET search_path=pg_catalog,platform,pg_temp AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM platform.organizations WHERE id=p_task.organization_id AND inbox_review_enabled) THEN
  PERFORM platform.publish_conversation_reply_previous(p_task,p_execution,p_body);
  RETURN;
 END IF;
 IF octet_length(p_body) BETWEEN 1 AND 1000 AND
    platform.conversation_current_reply(p_task.organization_id,p_task.conversation_id,p_task.id) THEN
  INSERT INTO platform.inbox_reply_drafts(organization_id,conversation_id,task_id,body,expires_at)
  VALUES(p_task.organization_id,p_task.conversation_id,p_task.id,p_body,p_task.deadline)
  ON CONFLICT DO NOTHING;
  IF FOUND THEN
   INSERT INTO platform.agent_audit(organization_id,task_id,execution_id,agent_id,action,result)
   VALUES(p_task.organization_id,p_task.id,p_execution,p_task.agent_id,'inbox.reply.review','pending');
  END IF;
 END IF;
END $$;

CREATE FUNCTION platform.decide_inbox_reply(p_org uuid,p_conv uuid,p_task uuid,p_decision text) RETURNS text
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE draft platform.inbox_reply_drafts; anchor timestamptz; source_message uuid;
BEGIN
 IF NOT platform.agent_owner(p_org) OR NOT platform.is_test_org(p_org) THEN
  RAISE EXCEPTION 'owner of test organization required' USING ERRCODE='42501';
 END IF;
 IF p_decision NOT IN ('approved','rejected') OR p_decision IS NULL THEN RETURN 'invalid'; END IF;
 PERFORM 1 FROM platform.conversations WHERE (organization_id,id)=(p_org,p_conv) FOR UPDATE;
 SELECT * INTO draft FROM platform.inbox_reply_drafts
 WHERE (organization_id,conversation_id,task_id)=(p_org,p_conv,p_task) FOR UPDATE;
 IF NOT FOUND THEN RETURN 'missing'; END IF;
 IF draft.status=p_decision THEN RETURN draft.status; END IF;
 IF draft.status<>'pending' THEN RETURN 'conflict'; END IF;
 IF p_decision='approved' THEN
  SELECT m.created_at,m.id INTO anchor,source_message FROM platform.agent_tasks t
  JOIN platform.agent_tasks root ON (root.organization_id,root.id)=(t.organization_id,CASE WHEN t.action='delegate_conversation' THEN t.id ELSE t.idempotency_key END)
  JOIN platform.messages m ON (m.organization_id,m.conversation_id,m.client_key)=(root.organization_id,root.conversation_id,root.idempotency_key)
  WHERE (t.organization_id,t.id)=(p_org,p_task) AND m.role='customer';
  IF anchor IS NULL OR draft.expires_at<=clock_timestamp()
   OR EXISTS(SELECT 1 FROM platform.messages m WHERE (m.organization_id,m.conversation_id)=(p_org,p_conv)
      AND m.role='customer' AND m.id<>source_message AND m.created_at>=anchor)
   OR NOT platform.conversation_current_reply(p_org,p_conv,p_task) THEN RETURN 'stale'; END IF;
  INSERT INTO platform.messages(id,organization_id,conversation_id,role,content,data,client_key)
  VALUES(p_task,p_org,p_conv,'assistant',draft.body,'{"conversation_route":true,"reviewed":true}',p_task);
 END IF;
 UPDATE platform.inbox_reply_drafts SET status=p_decision,decided_by=platform.current_user_id(),decided_at=clock_timestamp()
 WHERE (organization_id,task_id)=(p_org,p_task);
 INSERT INTO platform.agent_audit(organization_id,task_id,actor_id,agent_id,action,result)
 VALUES(p_org,p_task,platform.current_user_id(),'business_manager','inbox.reply.review',p_decision);
 RETURN p_decision;
END $$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION platform.conversation_publish(platform.agent_tasks,uuid,text),
 platform.set_inbox_review(uuid),platform.decide_inbox_reply(uuid,uuid,uuid,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.set_inbox_review(uuid),platform.decide_inbox_reply(uuid,uuid,uuid,text) TO aba_app;
ALTER TABLE platform.conversations ADD COLUMN assignee_id uuid REFERENCES platform.users(id);
ALTER TABLE platform.organizations ADD COLUMN staff_limit integer CHECK(staff_limit BETWEEN 1 AND 1000);
CREATE TABLE platform.team_invites (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 organization_id uuid NOT NULL REFERENCES platform.organizations(id),
 email text NOT NULL CHECK(email=lower(email) AND length(email) BETWEEN 3 AND 254),
 token_hash bytea NOT NULL UNIQUE CHECK(octet_length(token_hash)=32),
 can_manage_orders boolean NOT NULL,
 created_by uuid NOT NULL REFERENCES platform.users(id),
 expires_at timestamptz NOT NULL DEFAULT clock_timestamp()+interval '24 hours',
 consumed_at timestamptz,
 revoked_at timestamptz
);
CREATE INDEX team_invites_org ON platform.team_invites(organization_id,email);
CREATE TABLE platform.team_audit (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 organization_id uuid NOT NULL REFERENCES platform.organizations(id),
 actor_id uuid NOT NULL REFERENCES platform.users(id),
 target_id uuid,
 action text NOT NULL CHECK(action IN ('invite.created','invite.accepted','invite.revoked','member.role','member.revoked','inbox.enabled','conversation.assigned')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX team_audit_org ON platform.team_audit(organization_id,created_at DESC);
ALTER TABLE platform.team_invites ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.team_invites FORCE ROW LEVEL SECURITY;
CREATE POLICY migration_owner ON platform.team_invites TO aba_migrator USING(true) WITH CHECK(true);
ALTER TABLE platform.team_audit ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.team_audit FORCE ROW LEVEL SECURITY;
CREATE POLICY migration_owner ON platform.team_audit TO aba_migrator USING(true) WITH CHECK(true);

-- +goose StatementBegin
CREATE FUNCTION platform.team_state(p_org uuid) RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
BEGIN
 IF p_org::text IS DISTINCT FROM current_setting('app.organization_id',true) OR NOT platform.has_membership(p_org,true) THEN RAISE EXCEPTION 'owner required' USING ERRCODE='42501'; END IF;
 RETURN jsonb_build_object(
  'members',coalesce((SELECT jsonb_agg(to_jsonb(m) ORDER BY m.role DESC,m.email) FROM (
   SELECT u.id,u.display_name,u.email,x.role,(x.role='owner' OR x.can_manage_orders) AS operator
   FROM platform.organization_members x JOIN platform.users u ON u.id=x.user_id WHERE x.organization_id=p_org)m),'[]'),
  'invites',coalesce((SELECT jsonb_agg(to_jsonb(i) ORDER BY i.expires_at DESC) FROM (
   SELECT id,email,can_manage_orders AS operator,expires_at FROM platform.team_invites WHERE organization_id=p_org
   AND expires_at>clock_timestamp() AND consumed_at IS NULL AND revoked_at IS NULL ORDER BY expires_at DESC LIMIT 100)i),'[]'),
  'audit',coalesce((SELECT jsonb_agg(to_jsonb(a) ORDER BY a.created_at DESC) FROM (
   SELECT id,action,actor_id,target_id,created_at FROM platform.team_audit WHERE organization_id=p_org ORDER BY created_at DESC,id LIMIT 50)a),'[]'));
END $$;

CREATE FUNCTION platform.team_invite(p_org uuid,p_email text,p_hash bytea,p_operator boolean) RETURNS void
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE uid uuid:=platform.current_user_id(); new_id uuid;
BEGIN
 IF p_org::text IS DISTINCT FROM current_setting('app.organization_id',true) OR NOT platform.has_membership(p_org,true) THEN RAISE EXCEPTION 'owner required' USING ERRCODE='42501'; END IF;
 PERFORM 1 FROM platform.organizations WHERE id=p_org FOR UPDATE;
 UPDATE platform.team_invites SET revoked_at=clock_timestamp() WHERE organization_id=p_org AND email=p_email
   AND consumed_at IS NULL AND revoked_at IS NULL;
 INSERT INTO platform.team_invites(organization_id,email,token_hash,can_manage_orders,created_by)
 VALUES(p_org,p_email,p_hash,p_operator,uid) RETURNING id INTO new_id;
 INSERT INTO platform.team_audit(organization_id,actor_id,target_id,action) VALUES(p_org,uid,new_id,'invite.created');
END $$;

CREATE FUNCTION platform.team_accept(p_org uuid,p_hash bytea) RETURNS text
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE uid uuid:=platform.current_user_id(); invitation platform.team_invites; cap integer;
BEGIN
 IF uid IS NULL OR p_org::text IS DISTINCT FROM current_setting('app.organization_id',true) THEN RETURN 'invalid'; END IF;
 SELECT staff_limit INTO cap FROM platform.organizations WHERE id=p_org FOR UPDATE;
 SELECT i.* INTO invitation FROM platform.team_invites i JOIN platform.users u ON u.id=uid AND u.email=i.email
 WHERE i.organization_id=p_org AND i.token_hash=p_hash AND i.consumed_at IS NULL AND i.revoked_at IS NULL
 AND i.expires_at>clock_timestamp() AND u.email_verified_at IS NOT NULL FOR UPDATE OF i;
 IF NOT FOUND OR NOT EXISTS(SELECT 1 FROM platform.organization_members
 WHERE organization_id=p_org AND user_id=invitation.created_by AND role='owner') THEN RETURN 'invalid'; END IF;
 IF EXISTS(SELECT 1 FROM platform.organization_members WHERE organization_id=p_org AND user_id=uid) THEN RETURN 'member'; END IF;
 IF cap IS NOT NULL AND (SELECT count(*) FROM platform.organization_members WHERE organization_id=p_org AND role='member')>=cap THEN RETURN 'limit'; END IF;
 INSERT INTO platform.organization_members(organization_id,user_id,role,can_manage_orders)
 VALUES(p_org,uid,'member',invitation.can_manage_orders);
 UPDATE platform.team_invites SET consumed_at=clock_timestamp() WHERE id=invitation.id;
 INSERT INTO platform.team_audit(organization_id,actor_id,target_id,action) VALUES(p_org,uid,invitation.id,'invite.accepted');
 RETURN 'accepted';
END $$;

CREATE FUNCTION platform.team_change(p_org uuid,p_id uuid,p_action text,p_operator boolean) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
BEGIN
 IF p_org::text IS DISTINCT FROM current_setting('app.organization_id',true) OR NOT platform.has_membership(p_org,true) THEN RAISE EXCEPTION 'owner required' USING ERRCODE='42501'; END IF;
 PERFORM 1 FROM platform.organizations WHERE id=p_org FOR UPDATE;
 CASE p_action
 WHEN 'member.role' THEN UPDATE platform.organization_members SET can_manage_orders=p_operator
 WHERE organization_id=p_org AND user_id=p_id AND role='member';
 WHEN 'member.revoked' THEN DELETE FROM platform.organization_members WHERE organization_id=p_org AND user_id=p_id AND role='member';
 WHEN 'invite.revoked' THEN UPDATE platform.team_invites SET revoked_at=clock_timestamp()
 WHERE organization_id=p_org AND id=p_id AND consumed_at IS NULL AND revoked_at IS NULL;
 ELSE RETURN false;
 END CASE;
 IF NOT FOUND THEN RETURN false; END IF;
 IF p_action='member.revoked' OR (p_action='member.role' AND NOT p_operator) THEN
  UPDATE platform.conversations SET assignee_id=NULL,version=version+1,updated_at=clock_timestamp()
  WHERE organization_id=p_org AND assignee_id=p_id;
 END IF;
 INSERT INTO platform.team_audit(organization_id,actor_id,target_id,action) VALUES(p_org,platform.current_user_id(),p_id,p_action);
 RETURN true;
END $$;

CREATE FUNCTION platform.inbox_assign(p_org uuid,p_conv uuid,p_user uuid,p_version bigint) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
BEGIN
 IF NOT platform.agent_owner(p_org) THEN RAISE EXCEPTION 'owner of test organization required' USING ERRCODE='42501'; END IF;
 PERFORM 1 FROM platform.organizations WHERE id=p_org FOR UPDATE;
 IF p_user IS NOT NULL AND NOT EXISTS(SELECT 1 FROM platform.organization_members
 WHERE organization_id=p_org AND user_id=p_user AND (role='owner' OR can_manage_orders)) THEN RETURN false; END IF;
 UPDATE platform.conversations SET assignee_id=p_user,control_mode='human',in_flight=NULL,lease_until=NULL,
  version=version+1,updated_at=clock_timestamp()
 WHERE (organization_id,id,version)=(p_org,p_conv,p_version) AND status='open' AND expires_at>clock_timestamp()
 AND channel IN ('telegram','whatsapp','instagram');
 IF NOT FOUND THEN RETURN false; END IF;
 INSERT INTO platform.team_audit(organization_id,actor_id,target_id,action)
 VALUES(p_org,platform.current_user_id(),p_conv,'conversation.assigned');
 RETURN true;
END $$;

CREATE FUNCTION platform.workspace_usage(p_org uuid) RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
BEGIN
 IF p_org::text IS DISTINCT FROM current_setting('app.organization_id',true) OR NOT platform.has_membership(p_org,true) THEN RAISE EXCEPTION 'owner required' USING ERRCODE='42501'; END IF;
 RETURN jsonb_build_object(
  'staff_limit',(SELECT staff_limit FROM platform.organizations WHERE id=p_org),
  'staff',(SELECT count(*) FROM platform.organization_members WHERE organization_id=p_org AND role='member'),
  'active_channels',(SELECT count(*) FROM platform.telegram_integrations WHERE organization_id=p_org AND active)
                    +(SELECT count(*) FROM platform.channel_integrations WHERE organization_id=p_org AND active),
  'tasks_30d',(SELECT count(*) FROM platform.agent_tasks WHERE organization_id=p_org AND created_at>=clock_timestamp()-interval '30 days'),
  'tasks_failed',(SELECT count(*) FROM platform.agent_tasks WHERE organization_id=p_org AND created_at>=clock_timestamp()-interval '30 days' AND status IN ('failed','expired')),
  'tasks_active',(SELECT count(*) FROM platform.agent_tasks WHERE organization_id=p_org AND status IN ('queued','running')),
  'incoming_failed',(SELECT count(*) FROM platform.telegram_inbox WHERE organization_id=p_org AND status='failed')
    +(SELECT count(*) FROM platform.channel_inbox WHERE organization_id=p_org AND agent_task_id IS NULL AND status='failed'),
  'input_tokens',(SELECT coalesce(sum(input_tokens),0) FROM platform.llm_usage WHERE organization_id=p_org),
  'output_tokens',(SELECT coalesce(sum(output_tokens),0) FROM platform.llm_usage WHERE organization_id=p_org),
  'estimated_cost_microrub',(SELECT sum(charged) FILTER(WHERE status='settled') FROM platform.llm_reservations WHERE organization_id=p_org),
  'uncertain_cost_entries',(SELECT count(*) FROM platform.llm_reservations WHERE organization_id=p_org AND status='uncertain'),
  'reserved_cost_microrub',(SELECT coalesce(sum(amount),0) FROM platform.llm_reservations WHERE organization_id=p_org AND status='pending'));
END $$;

CREATE FUNCTION platform.inbox_erase_drafts() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
BEGIN
 DELETE FROM platform.inbox_reply_drafts WHERE (organization_id,conversation_id)=(NEW.organization_id,NEW.id);
 RETURN NEW;
END $$;
CREATE TRIGGER inbox_erase_drafts AFTER UPDATE OF status ON platform.conversations
 FOR EACH ROW WHEN(NEW.status='closed' AND OLD.status IS DISTINCT FROM NEW.status) EXECUTE FUNCTION platform.inbox_erase_drafts();

CREATE FUNCTION platform.inbox_hold_automatic_outbound() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
BEGIN
 IF EXISTS(SELECT 1 FROM platform.organizations WHERE id=NEW.organization_id AND inbox_review_enabled)
 AND EXISTS(SELECT 1 FROM platform.messages m WHERE (m.organization_id,m.conversation_id,m.id)=(NEW.organization_id,NEW.conversation_id,NEW.id)
 AND m.role='assistant' AND NOT (m.data @> '{"reviewed":true,"conversation_route":true}')) THEN
  RETURN NULL;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER inbox_hold_telegram BEFORE INSERT ON platform.telegram_outbox FOR EACH ROW EXECUTE FUNCTION platform.inbox_hold_automatic_outbound();
CREATE TRIGGER inbox_hold_channel BEFORE INSERT ON platform.channel_outbox FOR EACH ROW EXECUTE FUNCTION platform.inbox_hold_automatic_outbound();
-- +goose StatementEnd

REVOKE ALL ON FUNCTION platform.team_state(uuid),platform.team_invite(uuid,text,bytea,boolean),
 platform.team_accept(uuid,bytea),platform.team_change(uuid,uuid,text,boolean),platform.inbox_assign(uuid,uuid,uuid,bigint),platform.workspace_usage(uuid),
 platform.inbox_erase_drafts(),platform.inbox_hold_automatic_outbound() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.team_state(uuid),platform.team_invite(uuid,text,bytea,boolean),
 platform.team_accept(uuid,bytea),platform.team_change(uuid,uuid,text,boolean),platform.inbox_assign(uuid,uuid,uuid,bigint),platform.workspace_usage(uuid) TO aba_app;

UPDATE platform.service_metadata SET version=37 WHERE singleton;

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM platform.inbox_reply_drafts) OR EXISTS(SELECT 1 FROM platform.team_audit)
 OR EXISTS(SELECT 1 FROM platform.organizations WHERE staff_limit IS NOT NULL) OR EXISTS(SELECT 1 FROM platform.organizations WHERE inbox_review_enabled) THEN
  RAISE EXCEPTION 'rollback refused: preserve inbox approvals and opted-in organizations';
 END IF;
END $$;
-- +goose StatementEnd
DROP TRIGGER inbox_erase_drafts ON platform.conversations;
DROP TRIGGER inbox_hold_telegram ON platform.telegram_outbox;
DROP TRIGGER inbox_hold_channel ON platform.channel_outbox;
DROP FUNCTION platform.team_state(uuid),platform.team_invite(uuid,text,bytea,boolean),
 platform.team_accept(uuid,bytea),platform.team_change(uuid,uuid,text,boolean),platform.inbox_assign(uuid,uuid,uuid,bigint),platform.workspace_usage(uuid),
 platform.inbox_erase_drafts(),platform.inbox_hold_automatic_outbound();
DROP FUNCTION platform.set_inbox_review(uuid),platform.decide_inbox_reply(uuid,uuid,uuid,text),
 platform.conversation_publish(platform.agent_tasks,uuid,text);
ALTER FUNCTION platform.publish_conversation_reply_previous(platform.agent_tasks,uuid,text) RENAME TO conversation_publish;
DROP TABLE platform.inbox_reply_drafts,platform.team_invites,platform.team_audit;
ALTER TABLE platform.conversations DROP COLUMN assignee_id;
ALTER TABLE platform.organizations DROP COLUMN staff_limit;
ALTER TABLE platform.organizations DROP COLUMN inbox_review_enabled;
UPDATE platform.service_metadata SET version=36 WHERE singleton;
