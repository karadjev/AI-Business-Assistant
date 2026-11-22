import fcntl
import hashlib
import os
from pathlib import Path
import re
import shutil
import stat
import subprocess
import sys
import time

root = Path("/srv/aba/staging")
release = Path("/srv/aba/verify-20261122/release-clean")
backups = root / "backups"
approved = "f5aa3539842be7b991d13e0e7bd6363a9685541ca2dec91c98679274435ad038"
os.umask(0o077)
with (backups / ".schedule.lock").open("a") as lock:
    fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
    if hashlib.sha256((release / "manifest.json").read_bytes()).hexdigest() != approved:
        raise SystemExit("FAIL: release manifest drift")
    if shutil.disk_usage(backups).free < 20 * 1024**3:
        raise SystemExit("FAIL: backup requires 20 GiB free")
    subprocess.run(
        [sys.executable, str(release / "scripts/deployment/backup_job.py"), "--release", str(release),
         "--env", str(root / "environment"), "--project", "aba-private-staging", "--backup-dir", str(backups)],
        check=True, timeout=920,
    )
    managed = []
    for item in backups.iterdir():
        if not re.fullmatch(r"[0-9]{8}T[0-9]{6}-[0-9a-f]{32}\.dump", item.name):
            continue
        checksum = item.with_suffix(".dump.sha256")
        if not checksum.exists():
            continue
        archive_stat, digest_stat = item.lstat(), checksum.lstat()
        if all(stat.S_ISREG(s.st_mode) and s.st_uid == 0 and not s.st_mode & 0o077 for s in (archive_stat, digest_stat)):
            managed.append((archive_stat.st_mtime, item, checksum))
    managed.sort(reverse=True)
    for timestamp, archive, checksum in managed[2:]:
        if timestamp < time.time() - 30 * 86400:
            archive.unlink()
            checksum.unlink()
    print("PASS: scheduled backup with disk floor, 30-day retention and two-copy minimum", flush=True)
