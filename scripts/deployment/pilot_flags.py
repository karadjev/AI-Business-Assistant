"""Build scoped SQL for the isolated Inbox pilot. Calling code controls database access."""
import uuid


def statement(organization, actor, enable):
    org = str(uuid.UUID(organization))
    owner = str(uuid.UUID(actor))
    value = "true" if enable else "false"
    return (f"""
BEGIN;
SET LOCAL lock_timeout='5s';
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM platform.organizations o
  JOIN platform.organization_members m ON m.organization_id=o.id
  WHERE o.id='{org}' AND o.is_test AND o.inbox_review_enabled AND m.role='owner' AND m.user_id='{owner}')
 OR EXISTS(SELECT 1 FROM platform.workshop_admissions WHERE organization_id='{org}')
 OR EXISTS(SELECT 1 FROM platform.telegram_integrations WHERE organization_id='{org}' AND mode<>'mock')
 OR EXISTS(SELECT 1 FROM platform.channel_integrations WHERE organization_id='{org}' AND mode<>'mock') THEN
  RAISE EXCEPTION 'synthetic Inbox pilot with review and an assigned owner required';
 END IF;
END $$;
INSERT INTO platform.ai_settings AS current(organization_id,auto_enabled)
 VALUES('{org}',{value})
ON CONFLICT(organization_id) DO UPDATE SET auto_enabled=excluded.auto_enabled,version=current.version+1,updated_at=clock_timestamp()
WHERE current.auto_enabled IS DISTINCT FROM excluded.auto_enabled;
"""
        + ("" if enable else f"""
UPDATE platform.telegram_outbox q SET status='cancelled',capability=NULL,lease_until=NULL
 WHERE q.organization_id='{org}' AND q.status='pending'
 AND EXISTS(SELECT 1 FROM platform.messages m WHERE m.organization_id=q.organization_id AND m.id=q.id AND m.data @> '{{"reviewed":true}}');
UPDATE platform.channel_outbox q SET status='cancelled',capability=NULL,lease_until=NULL
 WHERE q.organization_id='{org}' AND q.status='pending'
 AND EXISTS(SELECT 1 FROM platform.messages m WHERE m.organization_id=q.organization_id AND m.id=q.id AND m.data @> '{{"reviewed":true}}');
UPDATE platform.inbox_reply_drafts SET expires_at=least(expires_at,clock_timestamp())
 WHERE organization_id='{org}' AND status='pending';
""")
        + f"""
INSERT INTO platform.ai_settings_events(id,organization_id,actor_id,kind)
 VALUES(gen_random_uuid(),'{org}','{owner}','settings_changed');
COMMIT;
""")
