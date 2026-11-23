import json
from pathlib import Path
import shutil
import sys

sys.dont_write_bytecode = True
from active_release import check_live, load, run

record, base, _, manifest = load()
check_live(record, manifest)
raw = run(base + ["ps", "--all", "--format", "json"]).strip()
rows = json.loads(raw) if raw.startswith("[") else [json.loads(line) for line in raw.splitlines()]
core = {"api", "mailpit", "postgres", "proxy", "web", "worker"}
healthy = {r["Service"] for r in rows if r.get("State") == "running" and r.get("Health") == "healthy"}
if healthy != core or any(p.get("PublishedPort") not in (None, 0) for r in rows for p in (r.get("Publishers") or [])):
    raise SystemExit("FAIL: unhealthy staging stack or published container port")
run(base + ["exec", "-T", "api", "/app/healthcheck"])
run(base + ["exec", "-T", "web", "/usr/bin/node", "-e",
    'fetch("http://127.0.0.1:3000/login").then(r=>process.exit(r.status===200?0:1)).catch(()=>process.exit(1))'])
run(base + ["exec", "-T", "worker", "/app/runtime-entrypoint", "worker-live"])
db = json.loads(run(base + ["exec", "-T", "postgres", "psql", "-X", "-qAt", "-U", "postgres", "-d", "aba_stage",
    "-v", "ON_ERROR_STOP=1"], data="""
BEGIN READ ONLY;
SELECT json_build_object(
 'schema',(SELECT version FROM platform.service_metadata WHERE singleton),
 'db_connections',(SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND backend_type='client backend'),
 'db_connections_limit',current_setting('max_connections')::int,
 'agent_backlog',(SELECT count(*) FROM platform.agent_tasks WHERE status IN ('queued','pending','running')),
 'agent_failed',(SELECT count(*) FROM platform.agent_tasks WHERE status='failed'),
 'agent_waiting_approval',(SELECT count(*) FROM platform.agent_tasks WHERE status='waiting_approval'),
 'channel_inbox_backlog',(SELECT count(*) FROM platform.channel_inbox WHERE status IN ('pending','processing')),
 'channel_outbox_backlog',(SELECT count(*) FROM platform.channel_outbox WHERE status IN ('pending','sending')),
 'channel_delivery_unknown',(SELECT count(*) FROM platform.channel_outbox WHERE status='delivery_unknown'),
 'channel_failed',(SELECT count(*) FROM platform.channel_inbox WHERE status='failed')+
                  (SELECT count(*) FROM platform.channel_outbox WHERE status='failed'),
 'telegram_inbox_backlog',(SELECT count(*) FROM platform.telegram_inbox WHERE status IN ('pending','processing')),
 'telegram_outbox_backlog',(SELECT count(*) FROM platform.telegram_outbox WHERE status IN ('pending','sending')),
 'telegram_failed',(SELECT count(*) FROM platform.telegram_inbox WHERE status='failed')+
                   (SELECT count(*) FROM platform.telegram_outbox WHERE status IN ('failed','uncertain')),
 'email_backlog',(SELECT count(*) FROM platform.email_outbox WHERE status='pending'),
 'email_failed',(SELECT count(*) FROM platform.email_outbox WHERE status='failed'));
ROLLBACK;
""").strip())
if db["schema"] != record["database_schema"]:
    raise SystemExit("FAIL: unexpected schema")
ids = [r["ID"] for r in rows if r["Service"] in core]
stats = [json.loads(l) for l in run(["docker", "stats", "--no-stream", "--format", "{{json .}}", *ids]).splitlines()]
restart_lines = run(["docker", "inspect", "--format", "{{.Name}} {{.RestartCount}}", *ids]).splitlines()
disk = shutil.disk_usage("/srv/aba/staging")
memory = {name: int(value.split()[0]) for name, value in (line.split(":", 1) for line in Path("/proc/meminfo").read_text().splitlines())
          if name in ("MemTotal", "MemAvailable")}
print(json.dumps({"services": sorted(healthy), "database": db, "disk_free_bytes": disk.free, "memory_kib": memory,
                  "load": Path("/proc/loadavg").read_text().split()[:3],
                  "restarts": dict(entry.lstrip("/").rsplit(" ", 1) for entry in restart_lines),
                  "docker": [{"service": row["Name"], "cpu": row["CPUPerc"], "ram": row["MemUsage"]} for row in stats]}))
if disk.free < 20 * 1024**3 or memory["MemAvailable"] < 512 * 1024:
    raise SystemExit("FAIL: staging host resource threshold")
