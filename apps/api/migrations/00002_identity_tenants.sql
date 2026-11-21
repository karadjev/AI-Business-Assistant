-- +goose Up
CREATE TABLE platform.users(id uuid PRIMARY KEY,email text NOT NULL UNIQUE CHECK(email=lower(email)),password_hash text NOT NULL,display_name text NOT NULL CHECK(length(display_name) BETWEEN 1 AND 100),email_verified_at timestamptz,created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE platform.sessions(id uuid PRIMARY KEY,user_id uuid NOT NULL REFERENCES platform.users(id) ON DELETE CASCADE,token_hash bytea NOT NULL UNIQUE CHECK(octet_length(token_hash)=32),csrf_hash bytea NOT NULL CHECK(octet_length(csrf_hash)=32),expires_at timestamptz NOT NULL,idle_expires_at timestamptz NOT NULL,revoked_at timestamptz,created_at timestamptz NOT NULL DEFAULT now());
CREATE INDEX sessions_user ON platform.sessions(user_id,expires_at);
CREATE TABLE platform.email_verification_tokens(id uuid PRIMARY KEY,user_id uuid NOT NULL REFERENCES platform.users(id) ON DELETE CASCADE,token_hash bytea NOT NULL UNIQUE CHECK(octet_length(token_hash)=32),expires_at timestamptz NOT NULL,consumed_at timestamptz,created_at timestamptz NOT NULL DEFAULT now());
CREATE INDEX verification_user ON platform.email_verification_tokens(user_id,expires_at);
CREATE TABLE platform.auth_rate_limits(key_hash bytea NOT NULL,bucket timestamptz NOT NULL,attempts integer NOT NULL CHECK(attempts>0),PRIMARY KEY(key_hash,bucket));
CREATE TABLE platform.email_outbox(id uuid PRIMARY KEY,payload bytea NOT NULL,status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','sent','failed')),attempts integer NOT NULL DEFAULT 0,available_at timestamptz NOT NULL DEFAULT now(),lease_until timestamptz,created_at timestamptz NOT NULL DEFAULT now());
CREATE INDEX email_pending ON platform.email_outbox(available_at) WHERE status='pending';
CREATE TABLE platform.organizations(id uuid PRIMARY KEY,name text NOT NULL CHECK(length(name) BETWEEN 1 AND 120),business_type text NOT NULL DEFAULT 'electronics' CHECK(business_type='electronics'),timezone text NOT NULL DEFAULT 'Europe/Moscow',version bigint NOT NULL DEFAULT 1 CHECK(version>0),created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE platform.organization_members(organization_id uuid NOT NULL REFERENCES platform.organizations(id) ON DELETE RESTRICT,user_id uuid NOT NULL REFERENCES platform.users(id) ON DELETE RESTRICT,role text NOT NULL CHECK(role IN ('owner','member')),PRIMARY KEY(organization_id,user_id));
CREATE INDEX members_user ON platform.organization_members(user_id,organization_id);
GRANT SELECT,INSERT,UPDATE ON platform.users,platform.email_verification_tokens,platform.auth_rate_limits,platform.email_outbox TO aba_app;
GRANT INSERT ON platform.sessions TO aba_app;
GRANT SELECT,UPDATE ON platform.organizations TO aba_app;
GRANT SELECT ON platform.organization_members TO aba_app;
ALTER TABLE platform.organizations ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.organizations FORCE ROW LEVEL SECURITY;
ALTER TABLE platform.organization_members ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.organization_members FORCE ROW LEVEL SECURITY;
-- Definer functions execute as the non-superuser migration owner. FORCE RLS still applies;
-- the dedicated owner policy is explicitly granted only to that role, never aba_app.
CREATE POLICY migration_owner ON platform.organizations TO aba_migrator USING(true) WITH CHECK(true);
CREATE POLICY migration_owner ON platform.organization_members TO aba_migrator USING(true) WITH CHECK(true);
-- +goose StatementBegin
CREATE FUNCTION platform.session_user(p_hash bytea) RETURNS TABLE(id uuid,email text,display_name text,csrf_hash bytea) LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT u.id,u.email,u.display_name,s.csrf_hash FROM platform.sessions s JOIN platform.users u ON u.id=s.user_id WHERE s.token_hash=p_hash AND s.revoked_at IS NULL AND s.expires_at>now() AND s.idle_expires_at>now() AND u.email_verified_at IS NOT NULL
$$;
CREATE FUNCTION platform.touch_session(p_hash bytea,p_idle interval) RETURNS void LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 UPDATE platform.sessions SET idle_expires_at=least(expires_at,now()+least(p_idle,interval '24 hours')) WHERE token_hash=p_hash AND revoked_at IS NULL AND expires_at>now() AND idle_expires_at>now()
$$;
CREATE FUNCTION platform.revoke_session(p_hash bytea) RETURNS void LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$ UPDATE platform.sessions SET revoked_at=now() WHERE token_hash=p_hash $$;
CREATE FUNCTION platform.current_user_id() RETURNS uuid LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT id FROM platform.session_user(CASE WHEN current_setting('app.session_hash',true) ~ '^[0-9a-f]{64}$' THEN decode(current_setting('app.session_hash',true),'hex') ELSE NULL END)
$$;
CREATE FUNCTION platform.has_membership(p_org uuid,p_owner boolean DEFAULT false) RETURNS boolean LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT EXISTS(SELECT 1 FROM platform.organization_members WHERE organization_id=p_org AND user_id=platform.current_user_id() AND (NOT p_owner OR role='owner'))
$$;
CREATE FUNCTION platform.list_organizations() RETURNS TABLE(id uuid,name text,business_type text,timezone text,version bigint,role text) LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT o.id,o.name,o.business_type,o.timezone,o.version,m.role FROM platform.organizations o JOIN platform.organization_members m ON m.organization_id=o.id WHERE m.user_id=platform.current_user_id() ORDER BY o.created_at,o.id
$$;
CREATE FUNCTION platform.create_organization(p_id uuid,p_name text,p_timezone text) RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE uid uuid;
BEGIN
 uid:=platform.current_user_id();
 IF uid IS NULL THEN RAISE EXCEPTION 'unauthorized' USING ERRCODE='42501'; END IF;
 INSERT INTO platform.organizations(id,name,timezone) VALUES(p_id,p_name,p_timezone);
 INSERT INTO platform.organization_members(organization_id,user_id,role) VALUES(p_id,uid,'owner');
END $$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION platform.session_user(bytea),platform.touch_session(bytea,interval),platform.revoke_session(bytea),platform.current_user_id(),platform.has_membership(uuid,boolean),platform.list_organizations(),platform.create_organization(uuid,text,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.session_user(bytea),platform.touch_session(bytea,interval),platform.revoke_session(bytea),platform.current_user_id(),platform.has_membership(uuid,boolean),platform.list_organizations(),platform.create_organization(uuid,text,text) TO aba_app;
CREATE POLICY tenant_read ON platform.organizations FOR SELECT TO aba_app USING(id::text=current_setting('app.organization_id',true) AND platform.has_membership(id,false));
CREATE POLICY tenant_update ON platform.organizations FOR UPDATE TO aba_app USING(id::text=current_setting('app.organization_id',true) AND platform.has_membership(id,true)) WITH CHECK(id::text=current_setting('app.organization_id',true) AND platform.has_membership(id,true));
CREATE POLICY membership_read ON platform.organization_members FOR SELECT TO aba_app USING(organization_id::text=current_setting('app.organization_id',true) AND user_id=platform.current_user_id());
UPDATE platform.service_metadata SET version=2 WHERE singleton=true;

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM platform.users) OR EXISTS(SELECT 1 FROM platform.organizations) THEN
  RAISE EXCEPTION 'refusing rollback of nonempty identity tables';
 END IF;
END $$;
-- +goose StatementEnd
UPDATE platform.service_metadata SET version=1 WHERE singleton=true;
DROP FUNCTION platform.create_organization(uuid,text,text),platform.list_organizations();
DROP POLICY tenant_read ON platform.organizations;
DROP POLICY tenant_update ON platform.organizations;
DROP POLICY membership_read ON platform.organization_members;
DROP FUNCTION platform.has_membership(uuid,boolean),platform.current_user_id(),platform.touch_session(bytea,interval),platform.revoke_session(bytea),platform.session_user(bytea);
DROP TABLE platform.organization_members,platform.organizations,platform.email_outbox,platform.auth_rate_limits,platform.email_verification_tokens,platform.sessions,platform.users;
