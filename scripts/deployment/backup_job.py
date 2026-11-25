#!/usr/bin/env python3
"""Scheduled backup entrypoint. No retention/deletion and no credential output."""
import argparse
import datetime
import os
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
    p.add_argument('--compose-override',type=Path)
    p.add_argument('--expected-volume')
    a=p.parse_args()
    try:
        private_directory(a.backup_dir)
        name=datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%S')+'-'+uuid.uuid4().hex+'.dump'
        volume_args=[]
        if a.compose_override is not None or a.expected_volume is not None:
            if a.compose_override is None or a.expected_volume is None:
                raise ValueError('incomplete active volume arguments')
            volume_args=['--compose-override',str(a.compose_override),'--expected-volume',a.expected_volume]
        result=subprocess.run([sys.executable,str(Path(__file__).with_name('backup.py')),'backup',
            '--release',str(a.release),'--env',str(a.env),'--project',a.project,
            '--archive',str(a.backup_dir/name),*volume_args],capture_output=True,timeout=900)
        if result.returncode:
            raise ValueError('backup failed')
    except (ValueError,OSError,subprocess.SubprocessError):
        print('FAIL: scheduled backup; preserve partial evidence and investigate',file=sys.stderr)
        return 1
    print('PASS: backup archive/checksum/TOC created; restore rehearsal remains separate', flush=True)
    offsite = {
        'ABA_OFFSITE_BUCKET', 'ABA_OFFSITE_PREFIX', 'ABA_OFFSITE_CERTIFICATE', 'ABA_OFFSITE_CERT_SHA256',
    }
    if any(key in os.environ for key in offsite):
        if not all(os.environ.get(key) for key in offsite):
            print('BLOCKED: incomplete external backup configuration; local backup retained', file=sys.stderr)
            return 1
        archive = a.backup_dir / name
        try:
            subprocess.run([sys.executable, str(Path(__file__).with_name('offsite_backup.py')),
                '--archive', str(archive), '--checksum', str(archive.with_suffix('.dump.sha256')),
                '--recipient-certificate', os.environ['ABA_OFFSITE_CERTIFICATE'],
                '--recipient-fingerprint', os.environ['ABA_OFFSITE_CERT_SHA256']],
                check=True, timeout=1850)
        except (OSError, subprocess.SubprocessError):
            print('BLOCKED: external backup; local backup retained', file=sys.stderr)
            return 1
    return 0

if __name__=='__main__':sys.exit(main())
