#!/usr/bin/env python3
"""Backup/restore and three-tenant checks ONLY on an isolated synthetic audit DB.

Not a replacement for release-package backup.py. No working or remote DB accepted.
Archive and disposable restored database are retained, including on failure.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import time


def require(condition, message):
    # Safety checks must remain active with PYTHONOPTIMIZE / python -O.
    if not condition:
        raise RuntimeError(message)


def run(args, data=None):
    result = subprocess.run(args, input=data, capture_output=True, timeout=180)
    if result.returncode:
        # Never include SQL, session hashes, stderr or credentials in diagnostics.
        raise RuntimeError("isolated database command failed (exit %d)" % result.returncode)
    return result.stdout


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--project", required=True)
    parser.add_argument("--evidence", type=Path, required=True)
    parser.add_argument("--restore-database", default="aba_readiness_restore")
    args = parser.parse_args()
    if not re.fullmatch(r"aba-readiness-integration-[a-z0-9-]+", args.project):
        parser.error("only a dedicated readiness integration project is accepted")
    endpoint = os.environ.get("DOCKER_HOST")
    if not endpoint:
        endpoint = json.loads(run(["docker", "context", "inspect", "--format", "{{json .Endpoints.docker.Host}}"] ))
    if not endpoint.startswith("unix:///"):
        parser.error("only local Unix Docker is accepted")
    os.environ["DOCKER_HOST"] = endpoint
    os.environ.pop("DOCKER_CONTEXT", None)
    ids = run(["docker", "ps", "-q", "--filter", "label=com.docker.compose.project=" + args.project,
               "--filter", "label=com.docker.compose.service=postgres"]).decode().split()
    if len(ids) != 1:
        parser.error("exactly one isolated PostgreSQL is required")
    container = ids[0]
    meta = json.loads(run(["docker", "inspect", container]))[0]
    if meta["HostConfig"].get("PortBindings") or meta["Mounts"]:
        # The prepared recipe has only its read-only bootstrap script bind.
        mounts = meta["Mounts"]
        if meta["HostConfig"].get("PortBindings") or any(
            m.get("RW") or m["Destination"] != "/docker-entrypoint-initdb.d/10-roles.sh" for m in mounts
        ):
            parser.error("published ports or writable persistent mounts refused")
    for network in meta["NetworkSettings"]["Networks"]:
        if not json.loads(run(["docker", "network", "inspect", network]))[0]["Internal"]:
            parser.error("test database must use internal-only networks")

    def sql(statement, database="aba_test", runtime=False):
        if runtime:
            cmd = ["docker", "exec", "-i", container, "sh", "-ec",
                   'export PGPASSWORD="$APP_DB_PASSWORD"; exec psql -X -h 127.0.0.1 -U aba_app -d "$1" -At -v ON_ERROR_STOP=1', "sh", database]
        else:
            cmd = ["docker", "exec", "-i", container, "psql", "-X", "-U", "postgres", "-d", database, "-At", "-v", "ON_ERROR_STOP=1"]
        return run(cmd, statement.encode()).decode().strip()

    require(sql("SELECT marker FROM platform.test_safety WHERE singleton") == "aba-synthetic-isolated", "synthetic safety marker missing")
    target = args.restore_database
    if not re.fullmatch(r"aba_readiness_restore(?:_[a-z0-9]+)?", target):
        parser.error("only a dedicated disposable restore database is accepted")
    require(sql("SELECT count(*) FROM pg_database WHERE datname='" + target + "'") == "0", "restore target exists; refusing overwrite")
    out = args.evidence.resolve()
    out.mkdir(mode=0o700, parents=True, exist_ok=False)
    os.chmod(out, 0o700)
    report = {"scope": "synthetic local aba_test only; release backup tool NOT exercised", "project": args.project,
              "rehearsal_source_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest()}
    (out / "status.json").write_text(json.dumps({"status": "INCOMPLETE", **report}, indent=2))
    archive = out / "platform.dump"
    start = time.monotonic()
    fd = os.open(archive, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
    with os.fdopen(fd, "wb") as stream:
        result = subprocess.run(["docker", "exec", container, "pg_dump", "-U", "postgres", "-d", "aba_test", "-Fc", "--schema=platform"], stdout=stream, stderr=subprocess.PIPE, timeout=180)
    require(result.returncode == 0, "backup failed; archive retained")
    blob = archive.read_bytes()
    report["backup_seconds"] = time.monotonic() - start
    report["archive_bytes"] = len(blob)
    report["sha256"] = hashlib.sha256(blob).hexdigest()
    (out / "platform.dump.sha256").write_text(report["sha256"] + "\n")
    run(["docker", "exec", "-i", container, "pg_restore", "--list"], blob)
    tables = sql("SELECT tablename FROM pg_tables WHERE schemaname='platform' ORDER BY tablename").splitlines()
    require(tables and all(re.fullmatch(r"[a-z_]+", table) for table in tables), "unexpected schema identifiers")

    def fingerprint(database):
        values = {}
        for table in tables:
            values[table] = sql("SELECT count(*)||':'||md5(coalesce(string_agg(to_jsonb(t)::text,'|' ORDER BY to_jsonb(t)::text),'')) FROM platform." + table + " t", database)
        # pg_dump may represent an explicit owner-only ACL as NULL (the same
        # PostgreSQL default). Compare effective privileges, never raw storage.
        values["__rls_acl_owner"] = sql("SELECT coalesce(jsonb_agg(to_jsonb(t) ORDER BY relname)::text,'[]') FROM (SELECT c.relname,c.relrowsecurity,c.relforcerowsecurity,pg_get_userbyid(c.relowner) AS owner,(SELECT jsonb_agg(to_jsonb(a) ORDER BY grantor,grantee,privilege_type,is_grantable) FROM aclexplode(coalesce(c.relacl,acldefault('r',c.relowner))) a) AS acl FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='platform' AND c.relkind IN ('r','p')) t", database)
        values["__policies"] = sql("SELECT coalesce(jsonb_agg(to_jsonb(p) ORDER BY tablename,policyname)::text,'[]') FROM pg_policies p WHERE schemaname='platform'", database)
        return values

    before = fingerprint("aba_test")
    start = time.monotonic()
    sql("CREATE DATABASE " + target)
    run(["docker", "exec", "-i", container, "pg_restore", "-U", "postgres", "-d", target, "--single-transaction", "--exit-on-error"], blob)
    after = fingerprint(target)
    (out / "fingerprints.json").write_text(json.dumps({"before": before, "after": after}, indent=2))
    require(before == after, "restored rows, RLS, ACL, ownership or policies differ")
    report["restore_and_verify_seconds"] = time.monotonic() - start
    report["tables_verified"] = len(tables)
    report["row_policy_acl_fingerprints_equal"] = True
    report["snapshot_consistency"] = "source quiescent after integration suite; no concurrent writer"
    # Select three distinct synthetic owners with one organization each. Tokens/hashes stay in memory.
    tenants = sql("SELECT DISTINCT ON(m.user_id) m.organization_id||'|'||encode(s.token_hash,'hex') FROM platform.organization_members m JOIN platform.sessions s ON s.user_id=m.user_id WHERE m.role='owner' AND s.expires_at>now() AND s.idle_expires_at>now() AND (SELECT count(*) FROM platform.organization_members other WHERE other.user_id=m.user_id)=1 AND EXISTS(SELECT 1 FROM platform.products p WHERE p.organization_id=m.organization_id) AND EXISTS(SELECT 1 FROM platform.orders o WHERE o.organization_id=m.organization_id) AND EXISTS(SELECT 1 FROM platform.conversations c WHERE c.organization_id=m.organization_id) ORDER BY m.user_id,s.expires_at DESC LIMIT 3", target).splitlines()
    require(len(tenants) == 3, "three populated owner fixtures are required")
    checks = 0
    scoped_tables = sql("SELECT table_name FROM information_schema.columns WHERE table_schema='platform' AND column_name='organization_id' AND table_name IN ('products','orders','conversations') ORDER BY table_name", target).splitlines()
    require(len(scoped_tables) == 3, "required tenant tables are missing")
    for tenant in tenants:
        own, session_hash = tenant.split("|")
        require(re.fullmatch(r"[a-f0-9-]{36}", own) and re.fullmatch(r"[a-f0-9]{64}", session_hash), "invalid synthetic identity metadata")
        prefix = "BEGIN; SELECT set_config('app.organization_id','" + own + "',true); SELECT set_config('app.session_hash','" + session_hash + "',true); "
        require(sql(prefix + "SELECT count(*) FROM platform.organizations WHERE id='" + own + "'; ROLLBACK;", target, True).splitlines()[-2] == "1", "owner positive control failed")
        for table in scoped_tables:
            require(int(sql(prefix + "SELECT count(*) FROM platform." + table + " WHERE organization_id='" + own + "'; ROLLBACK;", target, True).splitlines()[-2]) > 0, "positive tenant fixture is empty or inaccessible")
        for alien in tenants:
            foreign = alien.split("|")[0]
            if own == foreign:
                continue
            for table in scoped_tables:
                # Runtime policy failures must be checked alongside actual foreign row state.
                for operation in ["SELECT", "UPDATE", "DELETE"]:
                    if operation == "SELECT":
                        statement = "SELECT count(*) FROM platform." + table + " WHERE organization_id='" + foreign + "'"
                    else:
                        mutation = ("UPDATE platform." + table + " SET organization_id=organization_id" if operation == "UPDATE" else "DELETE FROM platform." + table) + " WHERE organization_id='" + foreign + "' RETURNING 1"
                        statement = "WITH changed AS (" + mutation + ") SELECT count(*) FROM changed"
                    # Permission denial is a valid fail-closed mutation result. SELECT must succeed with zero.
                    try:
                        result = sql(prefix + statement + "; ROLLBACK;", target, True)
                        require(result.splitlines()[-2] == "0", "cross-tenant operation affected rows")
                    except RuntimeError:
                        if operation == "SELECT":
                            raise
                        # Independently require missing table privilege, rather than accepting arbitrary errors.
                        allowed = sql("SELECT has_table_privilege('aba_app','platform." + table + "','" + operation + "')", target)
                        require(allowed == "f", "mutation failed for unknown reason")
                    checks += 1
        for table in scoped_tables:
            require(sql("SELECT count(*) FROM platform." + table, target, True) == "0", "pool/context-free read leaked data")
    require(fingerprint(target) == after, "negative tenant checks changed stored data")
    report.update({"three_tenant_negative_checks": checks, "context_free_reads": 9, "status": "PASS",
                   "RPO": "point-in-time dump; operational schedule/off-host recovery NOT RUN"})
    (out / "status.json").write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps(report, indent=2))


if __name__ == "__main__":
    main()
