"""Apply the prepared unified candidate on its original VPS after an exact release decision."""
import argparse
import fcntl
import hashlib
import json
import os
import secrets
from pathlib import Path
import stat
import time
import uuid

import rollout as r
from pilot_flags import statement

TRUSTED_D2_KEY = Path("/etc/aba/release-reviewers/d2.pem")


def private_bytes(path):
    info = path.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_uid != 0 or info.st_mode & 0o077 or not 0 < info.st_size < 1024 * 1024:
        raise ValueError("private root-owned decision file required")
    return path.read_bytes()


def release_decision(manifest_sha):
    path = r.RC / "admission.json"
    decision = json.loads(private_bytes(path))
    if (decision.get("manifest_sha256") != manifest_sha or decision.get("scope") != "one-synthetic-organization"
            or decision.get("database_schema") != 38 or decision.get("approved") is not True
            or not time.time() < decision.get("expires_at", 0) <= time.time() + 7 * 86400):
        raise ValueError("release decision does not match this candidate")
    if decision["kind"] == "independent_d2":
        private_bytes(r.RC / "admission.sig")
        info = TRUSTED_D2_KEY.lstat()
        if not stat.S_ISREG(info.st_mode) or info.st_uid != 0 or info.st_mode & 0o022:
            raise ValueError("trusted D2 public key required")
        r.command(["openssl", "dgst", "-sha256", "-verify", str(TRUSTED_D2_KEY),
                   "-signature", str(r.RC / "admission.sig"), str(path)])
    elif decision["kind"] == "owner_risk_acceptance":
        evidence = private_bytes(r.RC / "owner-decision.txt")
        if (decision.get("d2") != "NOT PERFORMED"
                or hashlib.sha256(evidence).hexdigest() != decision.get("evidence_sha256")):
            raise ValueError("owner decision evidence required")
    else:
        raise ValueError("unsupported decision type")
    return decision


def switch(release):
    record, compose, _, manifest = release
    for service in ("api", "worker", "web"):
        r.command(compose + ["up", "-d", "--no-deps", "--no-build", "--pull", "never", service])
    r.command(compose + ["restart", "--no-deps", "proxy"])
    for _ in range(55):
        try:
            r.gate.check_live(record, manifest)
            return
        except ValueError:
            time.sleep(2)
    raise RuntimeError("6/6 healthy and image check failed")


def edge(path):
    r.swap(path, r.EDGE)
    r.command(["nginx", "-t"])
    r.command(["systemctl", "reload", "nginx"])


def verify_package(release):
    record, _, path, manifest = release
    assert r.sha(path / "manifest.json") == record["manifest_sha256"]
    for name, digest in manifest["files"].items():
        file = path / name
        assert file.resolve().is_relative_to(path) and not file.is_symlink() and r.sha(file) == digest
    assert {k: r.digest_image(v) for k, v in manifest["images"].items()} == manifest["images"]


def postgres(release):
    _, compose, _, _ = release
    pg, = r.gate.run(compose + ["ps", "-q", "postgres"]).split()
    return pg


def down_empty(pg, current, intermediate, new):
    if current == "38":
        assert r.sql(pg, "SELECT count(*) FROM platform.commercial_subscriptions; SELECT count(*) FROM platform.commercial_admin_grants;") == "0\n0"
        r.command(new[1] + ["run", "--rm", "--no-deps", "migrate", "migrate", "down"])
        r.schema(pg, 37, 70)
    if current in ("37", "38"):
        assert r.sql(pg, "SELECT count(*) FROM platform.team_audit; SELECT count(*) FROM platform.inbox_reply_drafts;") == "0\n0"
        r.command(intermediate[1] + ["run", "--rm", "--no-deps", "migrate", "migrate", "down"])
    r.schema(pg, 36, 67)


def apply():
    data, work, old, new, recovery = r.loaded()
    decision = release_decision(new[0]["manifest_sha256"])
    active, _, _, manifest = r.gate.load()
    assert active["manifest_sha256"] == r.BASELINE and r.sha(r.EDGE) == old[0]["nginx_sha256"]
    r.gate.check_live(active, manifest)
    intermediate = r.read_record(Path(data["intermediate_record"]))
    for prepared in (new, recovery, intermediate):
        verify_package(prepared)
        assert r.sha(Path(prepared[1][7])) == r.sha(Path(old[1][7]))
    assert intermediate[0]["manifest_sha256"] == new[3]["migration_steps"][0]["manifest_sha256"]
    assert recovery[3]["images"] == new[3]["recovery_images"]
    proof = json.loads((work / "backup.json").read_text())
    assert 0 <= time.time() - proof["verified_at"] < 3600 and r.sha(proof["path"]) == proof["sha256"]
    pg = postgres(old)
    r.schema(pg, 36, 67)
    try:
        r.command(old[1] + ["stop", "worker", "api"])
        before = r.sql(pg, r.fingerprints())
        r.command(intermediate[1] + ["run", "--rm", "--no-deps", "migrate"])
        r.schema(pg, 37, 70)
        assert r.sql(pg, r.fingerprints()) == before
        r.command(new[1] + ["run", "--rm", "--no-deps", "migrate"])
        r.schema(pg, 38, 74)
        assert r.sql(pg, r.fingerprints()) == before
        switch(new)
        edge(work / "new-edge.conf")
        final_record = dict(new[0], authorization="Exact candidate decision: " + decision["kind"],
                            owner_risk_acceptance=decision["kind"] == "owner_risk_acceptance")
        r.write(work / "issued-record.json", final_record)
        r.swap(work / "issued-record.json", r.gate.RECORD)
        print("PASS schema 38, 6/6 healthy, existing data and volume preserved")
    except BaseException:
        edge(work / "old-edge.conf")
        r.command(new[1] + ["stop", "worker", "api"])
        version = r.sql(pg, "SELECT version FROM platform.service_metadata;")
        if version not in ("36", "37", "38"):
            raise RuntimeError("unexpected schema; refusing rollback")
        down_empty(pg, version, intermediate, new)
        switch(old)
        r.swap(work / "old-record.json", r.gate.RECORD)
        raise


def active_pilot():
    data, work, _, new, _ = r.loaded()
    live, _, _, manifest = r.gate.load()
    assert live["manifest_sha256"] == new[0]["manifest_sha256"]
    r.gate.check_live(live, manifest)
    return data, work, postgres(new)


def provision():
    data, work, pg = active_pilot()
    org = str(uuid.UUID(data["pilot"]))
    uid = str(uuid.uuid4())
    assert not (work / "pilot-owner.json").exists()
    r.sql(pg, f"""
BEGIN;
INSERT INTO platform.users(id,email,password_hash,display_name,email_verified_at)
 VALUES('{uid}','synthetic-inbox-{uid}@example.test','disabled-synthetic','Synthetic Inbox Pilot',clock_timestamp());
INSERT INTO platform.organizations(id,name,is_test,inbox_review_enabled)
 VALUES('{org}','Synthetic Communications Pilot',true,true);
INSERT INTO platform.organization_members(organization_id,user_id,role) VALUES('{org}','{uid}','owner');
INSERT INTO platform.commercial_admin_grants(organization_id,user_id) VALUES('{org}','{uid}');
INSERT INTO platform.team_audit(organization_id,actor_id,action) VALUES('{org}','{uid}','inbox.enabled');
COMMIT;
""")
    r.write(work / "pilot-owner.json", {"id": uid})
    print("PASS synthetic pilot provisioned; external configuration remains unchanged")


def smoke_session():
    data, work, pg = active_pilot()
    uid = str(uuid.UUID(json.loads((work / "pilot-owner.json").read_text())["id"]))
    sid = str(uuid.uuid4())
    token, csrf = secrets.token_urlsafe(32), secrets.token_urlsafe(32)
    digest = lambda value: hashlib.sha256(value.encode()).hexdigest()
    if (work / "pilot-session.json").exists():
        raise ValueError("finish or revoke the previous smoke session first")
    r.sql(pg, f"""
INSERT INTO platform.sessions(id,user_id,token_hash,csrf_hash,expires_at,idle_expires_at)
 VALUES('{sid}','{uid}',decode('{digest(token)}','hex'),decode('{digest(csrf)}','hex'),
 now()+interval '20 minutes',now()+interval '20 minutes');
""")
    r.write(work / "pilot-session.json", {"id": sid})
    print(json.dumps({"origin": "https://ai-business-store.com", "org": data["pilot"], "token": token, "csrf": csrf}))


def finish_smoke():
    data, work, pg = active_pilot()
    org = str(uuid.UUID(data["pilot"]))
    session = work / "pilot-session.json"
    sid = str(uuid.UUID(json.loads(session.read_text())["id"]))
    r.sql(pg, f"UPDATE platform.sessions SET revoked_at=clock_timestamp() WHERE id='{sid}'")
    session.rename(work / "pilot-session-revoked.json")
    assert r.sql(pg, f"""
SELECT EXISTS(SELECT 1 FROM platform.inbox_reply_drafts WHERE organization_id='{org}' AND status='approved');
SELECT EXISTS(SELECT 1 FROM platform.telegram_outbox o JOIN platform.telegram_integrations i
  ON (i.organization_id,i.id)=(o.organization_id,o.integration_id) WHERE o.organization_id='{org}' AND i.mode='mock' AND o.status='sent');
SELECT EXISTS(SELECT 1 FROM platform.commercial_audit WHERE organization_id='{org}');
SELECT EXISTS(SELECT 1 FROM platform.team_audit WHERE organization_id='{org}' AND action='invite.accepted');
SELECT NOT EXISTS(SELECT 1 FROM platform.telegram_integrations WHERE organization_id='{org}' AND mode<>'mock');
""") == "t\nt\nt\nt\nt"
    print("PASS pilot evidence; smoke session revoked")

def flags(enable):
    data, work, pg = active_pilot()
    owner = json.loads((work / "pilot-owner.json").read_text())["id"]
    r.sql(pg, statement(data["pilot"], owner, enable))
    print("PASS scoped Inbox automation " + ("enabled" if enable else "paused; old drafts expired"))


def recover():
    data, work, old, new, recovery = r.loaded()
    active, _, _, _ = r.gate.load()
    assert active["manifest_sha256"] == new[0]["manifest_sha256"]
    pg = postgres(new)
    r.schema(pg, 38, 74)
    edge(work / "old-edge.conf")
    if (work / "pilot-owner.json").exists():
        owner = json.loads((work / "pilot-owner.json").read_text())["id"]
        r.sql(pg, statement(data["pilot"], owner, False))
    switch(recovery)
    r.swap(work / "recovery-record.json", r.gate.RECORD)
    print("PASS recovery on schema 38; kept all data and the active volume")


if __name__ == "__main__":
    os.umask(0o077)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("apply", "provision", "smoke-session", "finish-smoke", "pause", "resume", "recover"))
    action = parser.parse_args().action
    with (r.RC / ".cutover.lock").open("a") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        {"apply": apply, "provision": provision, "smoke-session": smoke_session, "finish-smoke": finish_smoke,
         "pause": lambda: flags(False), "resume": lambda: flags(True), "recover": recover}[action]()
