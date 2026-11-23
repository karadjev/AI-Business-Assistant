"""Pin host operations to the approved release and the currently mounted database."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import subprocess
import sys
import time

sys.dont_write_bytecode = True
ROOT = Path("/srv/aba/staging")
RECORD = ROOT / "active-release.json"
PROJECT = "aba-private-staging"
ROLES = {role: role for role in ("api", "web", "proxy", "postgres", "mailpit")}
ROLES.update(worker="api", migrate="api")
RESTARTABLE = ("worker", "web", "api", "proxy")


def run(args, *, data=None):
    env = dict(os.environ)
    for key in list(env):
        if key.startswith(("DOCKER_", "COMPOSE_", "ABA_")):
            del env[key]
    env["DOCKER_HOST"] = "unix:///var/run/docker.sock"
    return subprocess.run(args, input=data, stdin=subprocess.DEVNULL if data is None else None,
                          capture_output=True, check=True,
                          text=True, timeout=60, env=env).stdout


def read_private(path):
    info = path.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_uid != os.geteuid() or info.st_mode & 0o077:
        raise ValueError("release record must be a private regular file")
    return json.loads(path.read_bytes())


def load():
    record = read_private(RECORD)
    base = record["compose_args"]
    if (len(base) != 10 or base[:3] != ["docker", "compose", "--env-file"]
            or base[4] != "-f" or base[6] != "-f" or base[8:] != ["-p", PROJECT]):
        raise ValueError("explicit Compose files and active-volume override required")
    env, compose, override = map(Path, (base[3], base[5], base[7]))
    if (not env.is_relative_to(ROOT / "reports") or not override.is_relative_to(ROOT / "reports")
            or override.name != "active-volume.yml" or env.parent != override.parent
            or compose.parts[-3:] != ("infrastructure", "deployment", "compose.yml")
            or not compose.is_relative_to(Path("/srv/aba"))):
        raise ValueError("unexpected release or override location")
    release = compose.parents[2]
    raw = (release / "manifest.json").read_bytes()
    if hashlib.sha256(raw).hexdigest() != record["manifest_sha256"]:
        raise ValueError("release manifest mismatch")
    manifest = json.loads(raw)
    if manifest["database_schema"] != record["database_schema"]:
        raise ValueError("schema admission mismatch")
    volume = record["postgres_active_volume"]
    if not re.fullmatch(r"aba-private-[a-z0-9-]+_database", volume):
        raise ValueError("invalid active volume")
    cfg = json.loads(run(base + ["config", "--format", "json"]))
    db = cfg["volumes"]["database"]
    if db.get("name") != volume or db.get("external") is not True:
        raise ValueError("resolved Compose does not mount the active volume")
    if not any(v.get("type") == "volume" and v.get("source") == "database"
               and v.get("target") == "/var/lib/postgresql" and not v.get("read_only")
               for v in cfg["services"]["postgres"]["volumes"]):
        raise ValueError("resolved PostgreSQL mount missing")
    if set(cfg["services"]) != set(ROLES) or any(
        not n.get("internal") or n.get("external") for n in cfg["networks"].values()
    ):
        raise ValueError("unexpected services or network exposure")
    for role, service in cfg["services"].items():
        if (service.get("ports") or service.get("privileged") or service.get("build")
                or service["image"] != manifest["images"][ROLES[role]]
                or service.get("pull_policy") != "never"):
            raise ValueError("unexpected service image or exposure")
    return record, base, release, manifest


def check_live(record, manifest, services=None):
    roles = services if services is not None else [role for role in ROLES if role != "migrate"]
    names = [PROJECT + "-" + role + "-1" for role in roles]
    rows = json.loads(run(["docker", "inspect", "--type", "container", *names]))
    for row, name, role in zip(rows, names, roles, strict=True):
        if (row["Name"] != "/" + name or row["Image"] != manifest["images"][ROLES[role]]
                or row["State"].get("Health", {}).get("Status") != "healthy"
                or row["HostConfig"].get("PortBindings")):
            raise ValueError("running service identity, health or exposure mismatch")
        if role == "postgres":
            actual = [m.get("Name") for m in row["Mounts"]
                      if m.get("Destination") == "/var/lib/postgresql" and m.get("Type") == "volume"]
            if actual != [record["postgres_active_volume"]]:
                raise ValueError("running PostgreSQL volume does not match the release record")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("check", "restart"))
    parser.add_argument("service", nargs="?", choices=RESTARTABLE)
    arg = parser.parse_args()
    if (arg.action == "restart") != bool(arg.service):
        parser.error("restart requires one stateless service")
    try:
        record, base, _, manifest = load()
        check_live(record, manifest)
        if arg.service:
            run(base + ["restart", "--no-deps", arg.service])
            deadline = time.monotonic() + 70
            while True:
                try:
                    check_live(record, manifest)
                    break
                except ValueError:
                    if time.monotonic() >= deadline:
                        raise
                    time.sleep(2)
        print("PASS: six healthy images, private ports and pinned database volume")
        return 0
    except (OSError, ValueError, KeyError, TypeError, subprocess.SubprocessError):
        print("FAIL: active release or service validation refused", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
