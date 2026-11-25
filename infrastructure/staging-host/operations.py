"""Read-only operations view for the authenticated server administrator."""
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
from datetime import datetime, timezone


def last_backup(directory):
    eligible = []
    for archive in directory.iterdir():
        if not re.fullmatch(r"\d{8}T\d{6}-[a-f0-9]{32}\.dump", archive.name):
            continue
        checksum = archive.with_suffix(".dump.sha256")
        if not checksum.exists():
            continue
        if any(not stat.S_ISREG(item.lstat().st_mode) or item.lstat().st_uid != 0 or item.lstat().st_mode & 0o077
               for item in (archive, checksum)):
            continue
        eligible.append((archive.stat().st_mtime, archive, checksum))
    if not eligible:
        return {"verified": False, "status": "missing"}
    timestamp, archive, checksum = max(eligible)
    digest = checksum.read_text().strip()
    with archive.open("rb") as stream:
        expected = hashlib.file_digest(stream, "sha256").hexdigest()
    verified = digest == expected
    age_hours = round((time.time()-timestamp)/3600, 1)
    return {"verified": verified, "age_hours": age_hours, "created_at": datetime.fromtimestamp(timestamp, timezone.utc).isoformat(),
            "status": "current" if verified and age_hours <= 30 else "stale" if verified else "checksum_mismatch"}


def main():
    if os.geteuid() != 0:
        raise SystemExit("BLOCKED: authenticated server administrator required")
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--host-dir", type=Path, default=Path(__file__).resolve().parent)
    root = parser.parse_args().host_dir.resolve(strict=True)
    result = subprocess.run([sys.executable, str(root/"monitor.py")], capture_output=True, text=True, timeout=45)
    status = json.loads(result.stdout) if result.stdout else {}
    db = status.get("database", {})
    print("Сервисы:", ", ".join(status.get("services", [])) or "нет диагностического снимка")
    print("Worker, очереди, каналы:", json.dumps({key:value for key,value in db.items() if key.startswith(("agent_","channel_","telegram_"))}, ensure_ascii=False))
    print("Локальный backup:", json.dumps(last_backup(root/"backups"), ensure_ascii=False))

    sys.path.insert(0, str(root))
    from active_release import load, run
    _, base, _, _ = load()
    pg, = run(base+["ps", "-q", "postgres"]).split()
    costs = run(["docker","exec","-i",pg,"psql","-XqAt","-U","postgres","-d","aba_stage","-v","ON_ERROR_STOP=1"], data="""
      BEGIN READ ONLY;
      SELECT json_build_object('monthly_exposure_microrub',coalesce(sum(CASE WHEN status='settled' THEN coalesce(charged,amount) ELSE amount END),0),
       'reserved_microrub',coalesce(sum(amount) FILTER(WHERE status='pending'),0),
       'unknown_cost_entries',count(*) FILTER(WHERE status='uncertain'))
      FROM platform.llm_reservations WHERE month_key='month:'||to_char(now() AT TIME ZONE 'UTC','YYYY-MM');
      ROLLBACK;
    """)
    print("Расходы AI:", costs.strip())
    if result.returncode:
        raise SystemExit("FAIL: эксплуатационная проверка сервисов")


if __name__ == "__main__":
    main()
