#!/usr/bin/env python3
"""Create new offline, integrity-checked linux/amd64 images. Never deploy/pull."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import struct
import subprocess

REPO = Path(__file__).resolve().parents[2]
FILES = REPO / 'infrastructure/deployment'
NODE = 'mcr.microsoft.com/playwright:v1.64.0-noble@sha256:06a9939e57531807f8d5fd76ce44b53165ffb7d7501d87ab10e285c20b1e971f'
BASES = {
    'postgres': 'postgres:18.7-alpine@sha256:0115267ab5a509022df9319c77be365cef335adb71556c75f60f5360ee247e71',
    'proxy': 'nginx:1.30.7-alpine@sha256:caf4e1e032a20f9c3c5e28fc7f25ffd768ad8677b4975c54193075997b3a97e3',
    'mailpit': 'axllent/mailpit:v1.34.0@sha256:23a2ef85bc50853ab3093645d1d64679afd8887e89da5e48a88940c172591a4e',
}
REQUIRED_GO_VERSION = 'go1.26.11'
# The pinned official image also used by the source Dockerfile reads build metadata.
GO_INSPECTOR = 'sha256:23f198931d4bd83b4f5a613bd75656257ae8bf832dcbc2db80e1050b5219ca2a'
GO_PARTS = ('api', 'worker', 'migrate', 'healthcheck', 'runtime-entrypoint')

def run(args, **kwargs):
    return subprocess.run(args, check=True, **kwargs)

def image(ref):
    value = json.loads(subprocess.check_output(['docker', 'image', 'inspect', ref]))[0]
    if value['Os'] != 'linux' or value['Architecture'] != 'amd64':
        raise ValueError('only previously cached linux/amd64 images are supported')
    return value['Id']

def static_binary(path):
    if path.is_symlink() or not path.is_file():
        raise ValueError('binary must be a regular file')
    if path.stat().st_mode & 0o001 == 0:
        raise ValueError('binary must be executable by the non-root runtime user')
    b = path.read_bytes()
    if len(b) < 64 or b[:6] != b'\x7fELF\x02\x01' or struct.unpack_from('<H', b, 18)[0] != 62:
        raise ValueError('expected linux/amd64 ELF64 binary')
    off, size, count = struct.unpack_from('<Q', b, 32)[0], struct.unpack_from('<H', b, 54)[0], struct.unpack_from('<H', b, 56)[0]
    if size < 4 or off + size * count > len(b) or any(struct.unpack_from('<I', b, off + i * size)[0] == 3 for i in range(count)):
        raise ValueError('Go deployment binaries must be static (CGO_ENABLED=0)')

def parse_go_versions(output):
    versions = {}
    for line in output.splitlines():
        if not line.startswith('/input/'):
            continue
        name, separator, version = line.partition(': ')
        part = name.removeprefix('/input/')
        if not separator or part not in GO_PARTS or part in versions or version != REQUIRED_GO_VERSION:
            raise ValueError('release binaries require the approved patched Go producer')
        versions[part] = version
    if set(versions) != set(GO_PARTS):
        raise ValueError('missing Go compiler metadata for release binaries')
    return versions

def verify_go_versions(artifacts):
    inspector = image(GO_INSPECTOR)
    command = ['docker', 'run', '--rm', '--pull=never', '--network', 'none', '--read-only',
               '--cap-drop', 'ALL', '--security-opt', 'no-new-privileges:true', '--user', '10001:10001',
               '--memory', '256m', '--pids-limit', '64', '-e', 'GOTOOLCHAIN=local', '-e', 'GOTELEMETRY=off',
               '-v', str(artifacts.resolve()) + ':/input:ro', '--entrypoint', 'go', inspector,
               'version', '-m', *['/input/' + part for part in GO_PARTS]]
    output = subprocess.check_output(command, text=True, stderr=subprocess.STDOUT)
    return {'required_go': REQUIRED_GO_VERSION, 'versions': parse_go_versions(output),
            'inspector_image': inspector, 'inspection': 'static metadata only; not independent producer attestation',
            'binary_sha256': {part: hashlib.sha256((artifacts / part).read_bytes()).hexdigest() for part in GO_PARTS}}

def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--release', required=True)
    p.add_argument('--web-volume', required=True)
    p.add_argument('--artifacts', required=True, type=Path)
    p.add_argument('--output', required=True, type=Path)
    p.add_argument('--export-images', action='store_true')
    a = p.parse_args()
    if not re.fullmatch(r'[a-z0-9][a-z0-9-]{0,47}', a.release) or not re.fullmatch(r'aba-[a-z0-9-]+', a.web_volume):
        p.error('invalid release or dedicated ABA build volume')
    # Preflight refuses missing caches; no install, package registry or image pull.
    node = image(NODE)
    support = {k: image(v) for k, v in BASES.items()}
    run(['docker', 'volume', 'inspect', a.web_volume], stdout=subprocess.DEVNULL)
    for part in ('api', 'worker', 'migrate', 'healthcheck', 'runtime-entrypoint'):
        static_binary(a.artifacts / part)
    compiler = verify_go_versions(a.artifacts)
    a.output.mkdir(mode=0o700, parents=True, exist_ok=False)
    context = a.output / 'context'
    (context / 'go').mkdir(parents=True)
    (context / 'web' / 'web').mkdir(parents=True)
    common = ['docker', 'run', '--rm', '--pull=never', '--network', 'none', '--cap-drop', 'ALL', '--security-opt', 'no-new-privileges:true']
    guard = common + ['-v', a.web_volume + ':/apps/web:ro', '-v', str(REPO / 'apps/web') + ':/web-source:ro', '-v', str(REPO / 'scripts') + ':/scripts:ro', '--entrypoint', 'node', node, '/scripts/build-env/artifacts.cjs', 'verify']
    with (a.output / 'guard-before.log').open('w') as log:
        run(guard, stdout=log, stderr=subprocess.STDOUT)
    # Export only source and required verified runtime paths, never env/cache/test data.
    exporter = """const fs=require('fs'),p=require('path');
const allowed=new Set(['standalone','static','BUILD_ID','required-server-files.json','aba-build-proof.json']);
fs.cpSync('/input','/output/web',{recursive:true,filter:f=>{
 const r=p.relative('/input',f),a=r.split(p.sep);
 if(a.some(x=>x.startsWith('.env')))return false;
 if(['node_modules','tests','.git','test-results','playwright-report'].includes(a[0]))return false;
 if(a[0]==='.next'&&a.length>1&&!allowed.has(a[1]))return false;
 return true;
}});"""
    run(common + ['-v', a.web_volume + ':/input:ro', '-v', str((context / 'web').resolve()) + ':/output', '--entrypoint', 'node', node, '-e', exporter])
    packed_guard = common + ['-v', str((context / 'web' / 'web').resolve()) + ':/apps/web:ro', '-v', str(REPO / 'apps/web') + ':/web-source:ro', '-v', str(REPO / 'scripts') + ':/scripts:ro', '--entrypoint', 'node', node, '/scripts/build-env/artifacts.cjs', 'verify']
    with (a.output / 'guard-exported.log').open('w') as log:
        run(packed_guard, stdout=log, stderr=subprocess.STDOUT)
    for part in ('api', 'worker', 'migrate', 'healthcheck', 'runtime-entrypoint'):
        shutil.copy2(a.artifacts / part, context / 'go' / part)
    (context / 'go' / 'compiler-verification.json').write_text(json.dumps(compiler, indent=2) + '\n')
    with (context / 'go' / 'ca-certificates.crt').open('wb') as f:
        run(common + ['--entrypoint', 'cat', node, '/etc/ssl/certs/ca-certificates.crt'], stdout=f)
    shutil.copy2(REPO / 'scripts/build-env/artifacts.cjs', context / 'web' / 'artifacts.cjs')
    shutil.copy2(FILES / 'web-entrypoint.cjs', context / 'web' / 'web-entrypoint.cjs')
    env = {**os.environ, 'DOCKER_BUILDKIT': '0'}
    tags = {'api': 'aba-private-api:' + a.release, 'web': 'aba-private-web:' + a.release}
    for part in ('api', 'web'):
        if subprocess.run(['docker', 'image', 'inspect', tags[part]], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode == 0:
            raise ValueError('release tag already exists; choose a new release')
        args = ['docker', 'build', '--pull=false', '--network=none', '--label', 'aba.release=' + a.release, '-f', str(FILES / (part + '.Dockerfile')), '-t', tags[part]]
        if part == 'web':
            args += ['--build-arg', 'NODE_IMAGE=' + node]
        with (a.output / ('image-' + part + '.log')).open('w') as log:
            run(args + [str(context / ('go' if part == 'api' else 'web'))], env=env, stdout=log, stderr=subprocess.STDOUT)
    images = {**support, **{k: image(v) for k, v in tags.items()}}
    config = a.output / 'infrastructure'
    shutil.copytree(FILES, config / 'deployment')
    shutil.copytree(REPO / 'scripts/deployment', a.output / 'scripts/deployment', ignore=shutil.ignore_patterns('__pycache__', '*.pyc'))
    (a.output / 'docs').mkdir()
    for name in ('STAGE_12A_DEPLOYMENT_READINESS.md', 'VPS_DEPLOYMENT_RUNBOOK.md',
                 'VPS_DEPLOYMENT_READINESS.md', 'VPS_INSTALLATION.md', 'VPS_OPERATIONS.md',
                 'VPS_SECURITY.md', 'VPS_BACKUP_RECOVERY.md', 'VPS_LAUNCH_GATE.md',
                 'STAGE_12B_FINAL_DEPLOYMENT_VERIFICATION.md',
                 'STAGE_12C_PRE_VPS_SECURITY_REPORT.md', 'RELEASE_SUPPLY_CHAIN_REPORT.md',
                 'VPS_PROVIDER_ACCEPTANCE_CHECKLIST.md', 'VPS_PRIVATE_STAGING_SECURITY_PLAN.md',
                 'VPS_FIRST_BOOT_VERIFICATION.md'):
        shutil.copy2(REPO / 'docs' / name, a.output / 'docs' / name)
    hashes = {}
    for base in (config, a.output / 'context', a.output / 'scripts', a.output / 'docs'):
        for file in sorted(base.rglob('*')):
            if file.is_file() and not file.is_symlink():
                hashes[str(file.relative_to(a.output))] = hashlib.sha256(file.read_bytes()).hexdigest()
    proof = json.loads((context / 'web' / 'web' / '.next' / 'aba-build-proof.json').read_text())
    manifest = {'schema': 1, 'release': a.release, 'platform': 'linux/amd64', 'images': images,
                'web_proof': proof, 'files': hashes, 'database_schema': 31,
                'runtime_security_admission_version': 2,
                'status': 'PACKAGE_ONLY_NOT_DEPLOYMENT_GO', 'security': 'CONDITIONAL', 'e01': 'OPEN'}
    (a.output / 'manifest.json').write_text(json.dumps(manifest, indent=2) + '\n')
    (a.output / 'images.env').write_text('\n'.join('ABA_' + k.upper() + '_IMAGE=' + v for k, v in images.items()) + '\n')
    if a.export_images:
        run(['docker', 'image', 'save', '-o', str(a.output / 'images.tar'), *images.values()])
        (a.output / 'images.tar.sha256').write_text(hashlib.sha256((a.output / 'images.tar').read_bytes()).hexdigest() + '  images.tar\n')
    print('Offline package created; no deployment authorization')

if __name__ == '__main__':
    main()
