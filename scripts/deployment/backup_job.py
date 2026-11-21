#!/usr/bin/env python3
"""Scheduled backup entrypoint. No retention/deletion and no credential output."""
import argparse
import datetime
from pathlib import Path
import subprocess
import sys
import uuid

sys.dont_write_bytecode = True
from deploy import private_directory

def main():
    p=argparse.ArgumentParser(description=__doc__)
    for key in ('release','env','backup-dir'):
        p.add_argument('--'+key,type=Path,required=True)
    p.add_argument('--project',required=True)
    a=p.parse_args()
    try:
        private_directory(a.backup_dir)
        name=datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%S')+'-'+uuid.uuid4().hex+'.dump'
        result=subprocess.run([sys.executable,str(Path(__file__).with_name('backup.py')),'backup',
            '--release',str(a.release),'--env',str(a.env),'--project',a.project,
            '--archive',str(a.backup_dir/name)],capture_output=True,timeout=900)
        if result.returncode:
            raise ValueError('backup failed')
    except (ValueError,OSError,subprocess.SubprocessError):
        print('FAIL: scheduled backup; preserve partial evidence and investigate',file=sys.stderr)
        return 1
    print('PASS: backup archive/checksum/TOC created; restore rehearsal remains separate')
    return 0

if __name__=='__main__':sys.exit(main())
