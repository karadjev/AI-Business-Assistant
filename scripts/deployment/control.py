#!/usr/bin/env python3
"""Manual private staging only. Validation never starts or pulls containers."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import stat
import subprocess
import sys

sys.dont_write_bytecode = True
from common import validate

def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('action', choices=['validate', 'start', 'stop', 'rollback'])
    p.add_argument('--release', type=Path, required=True)
    p.add_argument('--env', type=Path, required=True)
    p.add_argument('--project', required=True)
    p.add_argument('--go-file', type=Path)
    p.add_argument('--local-qa', action='store_true', help='explicit isolated QA scope; never a server GO')
    a = p.parse_args()
    base, manifest = validate(a.release, a.env, a.project)
    if a.action == 'validate':
        print('PASS: local package validation only; no deployment GO')
        return
    if a.go_file is None:
        p.error('state changes require a separately approved owner GO file')
    try:
        info = a.go_file.lstat()
        if not stat.S_ISREG(info.st_mode) or info.st_mode & 0o077 or not 0 < info.st_size <= 65536:
            p.error('owner GO requires a private regular file')
        go = json.loads(a.go_file.read_text())
    except (OSError, ValueError):
        p.error('owner GO unavailable or invalid')
    digest = hashlib.sha256((a.release / 'manifest.json').read_bytes()).hexdigest()
    if go.get('manifest_sha256') != digest or go.get('project') != a.project or go.get('action') != a.action or go.get('approved_by_owner') is not True:
        p.error('owner GO does not match this exact manifest/project/action')
    if (go.get('scope') != ('isolated-local-qa' if a.local_qa else 'private-staging')
            or go.get('database_schema') != manifest['database_schema']
            or a.local_qa and not re.fullmatch(r'aba-private-qa-[a-z0-9-]+', a.project)):
        p.error('owner GO does not match this scope/schema')
    if a.action == 'start' and go.get('migrations_approved') is not True:
        p.error('start requires explicit migration approval')
    if a.action == 'stop':
        # Graceful only; preserve volumes and evidence. Never implicit KILL.
        subprocess.run(base + ['stop', '--timeout', '-1'], check=True)
    elif a.action == 'rollback':
        if go.get('schema_compatible') is not True:
            p.error('rollback requires separately verified database/schema compatibility')
        # Does not run migrations down or restore/delete a database.
        subprocess.run(base + ['up', '-d', '--no-deps', '--pull', 'never', 'api', 'worker', 'web', 'proxy'], check=True)
    else:
        subprocess.run(base + ['up', '-d', '--pull', 'never', '--wait', '--wait-timeout', '120', 'postgres', 'mailpit'], check=True)
        subprocess.run(base + ['run', '--rm', '--no-deps', 'migrate'], check=True)
        subprocess.run(base + ['up', '-d', '--pull', 'never', 'api', 'worker', 'web', 'proxy'], check=True)
    print('Private action completed; verify health and evidence before further GO')

if __name__ == '__main__':
    main()
