#!/usr/bin/env python3
"""Offline backup, or restore ONLY into an empty isolated rehearsal project."""
import argparse
import hashlib
import os
from pathlib import Path
import re
import subprocess
import sys

sys.dont_write_bytecode = True
from common import validate, postgres

def digest(path):
    with path.open('rb') as archive:
        return hashlib.file_digest(archive, 'sha256').hexdigest()

def query(container, sql):
    return subprocess.check_output(['docker', 'exec', '-i', container, 'psql', '-X', '-U', 'postgres', '-d', 'aba_stage', '-At', '-v', 'ON_ERROR_STOP=1'], input=sql.encode()).decode().strip()

def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('action', choices=['backup', 'restore-rehearsal'])
    p.add_argument('--release', type=Path, required=True)
    p.add_argument('--env', type=Path, required=True)
    p.add_argument('--project', required=True)
    p.add_argument('--archive', type=Path, required=True)
    a = p.parse_args()
    base, _ = validate(a.release, a.env, a.project)
    container = postgres(base)
    if a.action == 'backup':
        if a.archive.parent.stat().st_mode & 0o077:
            p.error('backup directory must have mode0700')
        fd = os.open(a.archive, os.O_CREAT | os.O_EXCL | os.O_WRONLY | os.O_NOFOLLOW, 0o600)
        with os.fdopen(fd, 'wb') as f:
            # Preserve ACLs, policies, functions and schema. Do not dump global roles/passwords.
            result = subprocess.run(['docker', 'exec', container, 'pg_dump', '-U', 'postgres', '-d', 'aba_stage', '-Fc', '--schema=platform'], stdout=f, stderr=subprocess.PIPE)
        if result.returncode:
            raise RuntimeError('backup failed; incomplete archive retained, no values printed')
        with a.archive.open('rb') as f:
            result = subprocess.run(['docker', 'exec', '-i', container, 'pg_restore', '--list'], stdin=f, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE)
        if result.returncode:
            raise RuntimeError('backup archive validation failed; private archive retained')
        checksum = digest(a.archive)
        with (a.archive.with_suffix(a.archive.suffix + '.sha256')).open('x') as f:
            f.write(checksum + '\n')
        print('PASS: private archive and table of contents verified; restore rehearsal still required')
        return
    if not re.fullmatch(r'aba-private-restore-[a-z0-9-]+', a.project):
        p.error('restore is limited to a separately provisioned isolated rehearsal project')
    if a.archive.is_symlink() or not a.archive.is_file() or a.archive.stat().st_mode & 0o077:
        p.error('archive must be a private regular file')
    if digest(a.archive) != a.archive.with_suffix(a.archive.suffix + '.sha256').read_text().strip():
        p.error('archive checksum mismatch')
    if query(container, "SELECT count(*) FROM pg_tables WHERE schemaname='platform'") != '0':
        p.error('restore refused: target platform schema is not empty')
    active = subprocess.check_output(base + ['ps', '--services', '--status', 'running'], text=True).split()
    if active != ['postgres']:
        p.error('restore requires only the isolated target PostgreSQL to be running')
    temp = subprocess.check_output(['docker', 'exec', container, 'mktemp', '-d', '/tmp/aba-restore.XXXXXX'], text=True).strip()
    # Temporary copies remain on this new rehearsal container if restore fails.
    subprocess.run(['docker', 'cp', str(a.archive), container + ':' + temp + '/archive'], check=True)
    toc = subprocess.check_output(['docker', 'exec', container, 'pg_restore', '-l', temp + '/archive'], text=True)
    toc = '\n'.join(line for line in toc.splitlines() if not re.match(r'^\d+; \d+ \d+ SCHEMA - platform ', line)) + '\n'
    subprocess.run(['docker', 'exec', '-i', container, 'sh', '-ec', 'umask 077; cat > "$1/list"', 'sh', temp], input=toc.encode(), check=True)
    # The local admin restores data through FORCE RLS and restores the archived
    # aba_migrator ownership/ACLs. Runtime never receives this administrative role.
    result = subprocess.run(['docker', 'exec', container, 'pg_restore', '-U', 'postgres', '-d', 'aba_stage', '--single-transaction', '--exit-on-error', '--use-list=' + temp + '/list', temp + '/archive'], capture_output=True)
    if result.returncode:
        raise RuntimeError('restore failed; private target/archive retained, no data printed')
    print('PASS: isolated restore completed; RLS/ACL/data validation still required')

if __name__ == '__main__':
    main()
