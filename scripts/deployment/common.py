"""Fail-closed checks shared by the manual private-staging tools."""
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
from urllib.parse import urlparse

def command(release, env, project):
    if not re.fullmatch(r'aba-private-[a-z0-9][a-z0-9-]{0,47}', project):
        raise ValueError('only explicitly named private staging projects are allowed')
    return ['docker', 'compose', '--env-file', str(env), '-f', str(release / 'infrastructure/deployment/compose.yml'), '-p', project]

def pin_local_docker():
    override = os.environ.get('DOCKER_HOST', '')
    host = (json.loads(subprocess.check_output(['docker', 'context', 'inspect', '--format', '{{json .Endpoints.docker.Host}}']))
            if os.environ.get('DOCKER_CONTEXT') or not override else override)
    if not isinstance(host, str) or not host.startswith('unix:///'):
        raise ValueError('only a local Unix Docker socket is allowed')
    # Current process/children only. Never edit the user's Docker contexts.
    os.environ['DOCKER_HOST'] = host
    os.environ.pop('DOCKER_CONTEXT', None)

def validate(release, env, project):
    release = Path(release).resolve()
    manifest = json.loads((release / 'manifest.json').read_text())
    if manifest.get('schema') != 1 or manifest.get('platform') != 'linux/amd64':
        raise ValueError('unsupported release manifest')
    admission = manifest.get('runtime_security_admission_version')
    if type(admission) is not int or admission != 2:
        raise ValueError('release must preserve verified staging runtime security admission v2')
    if any(p.name == '__pycache__' or p.suffix == '.pyc' for p in (release / 'scripts').rglob('*')):
        raise ValueError('generated Python bytecode in immutable release scripts')
    for name, expected in manifest['files'].items():
        file = release / name
        if file.is_symlink() or not file.resolve().is_relative_to(release) or hashlib.sha256(file.read_bytes()).hexdigest() != expected:
            raise ValueError('release file checksum mismatch')
    base = command(release, env, project)
    pin_local_docker()
    cfg = json.loads(subprocess.check_output(base + ['config', '--format', 'json']))
    if any(not value.get('internal') or value.get('external') for value in cfg['networks'].values()):
        raise ValueError('network must remain internal')
    roles = {'api': 'api', 'worker': 'api', 'migrate': 'api', 'web': 'web', 'postgres': 'postgres', 'proxy': 'proxy', 'mailpit': 'mailpit'}
    if set(cfg['services']) != set(roles):
        raise ValueError('unexpected service')
    for service, value in cfg['services'].items():
        if value.get('ports') or value.get('privileged') or value.get('network_mode') or value.get('build'):
            raise ValueError('published ports, privilege, host networking and build are prohibited')
        if value['image'] != manifest['images'][roles[service]] or value.get('pull_policy') != 'never':
            raise ValueError('service image is not the verified release image')
        if service in ('api', 'worker', 'migrate'):
            if value.get('entrypoint') not in (None, ['/app/runtime-entrypoint']):
                raise ValueError('runtime entrypoint must preserve security admission')
            expected = ['migrate', 'up'] if service == 'migrate' else [service]
            if value.get('command') != expected:
                raise ValueError('runtime command must match the service role')
        if service == 'web' and value.get('entrypoint') not in (None, ['/usr/bin/node', '/guard/web-entrypoint.cjs']):
            raise ValueError('web entrypoint must preserve artifact admission')
        for mount in value.get('volumes', []):
            if 'docker.sock' in mount.get('source', '') or 'docker.sock' in mount.get('target', ''):
                raise ValueError('Docker socket mount prohibited')
        if any(not isinstance(mount, str) or not mount.startswith('/') for mount in value.get('tmpfs', [])):
            raise ValueError('tmpfs mounts must use absolute target paths')
        flags = value.get('environment', {})
        if service in ('api', 'worker'):
            origin = urlparse(flags.get('PUBLIC_ORIGIN', ''))
            if flags.get('APP_ENV') != 'staging' or origin.scheme != 'https' or not origin.hostname or origin.username or origin.query or origin.fragment or origin.path:
                raise ValueError('private runtime requires staging mode and an HTTPS origin')
            if flags.get('LLM_PROVIDER') != 'mock' or flags.get('TELEGRAM_TRANSPORT') != 'mock':
                raise ValueError('providers must remain mock')
            # Require explicit admission controls even if today's runtime has
            # safe defaults. An omitted control must not silently pass review.
            required = (
                'LLM_ENABLE_LIVE', 'LLM_EXTERNAL_REQUESTS_AUTHORIZED',
                'LLM_KILL_SWITCH', 'LLM_PLATFORM_BUDGET_MICRORUB',
                'TELEGRAM_EXTERNAL_REQUESTS_AUTHORIZED', 'TELEGRAM_SENDING_ENABLED',
                'TELEGRAM_SENDING_KILL_SWITCH', 'WHATSAPP_LIVE_ENABLED',
                'WHATSAPP_EXTERNAL_REQUESTS_AUTHORIZED',
                'WHATSAPP_SENDING_KILL_SWITCH', 'WHATSAPP_MONETARY_BUDGET_RUB',
                'INSTAGRAM_ENABLED', 'INSTAGRAM_LIVE_ENABLED', 'INSTAGRAM_EXTERNAL_REQUESTS_AUTHORIZED',
                'INSTAGRAM_SENDING_KILL_SWITCH', 'INSTAGRAM_MONETARY_BUDGET_RUB',
            )
            if any(key not in flags for key in required):
                raise ValueError('explicit live, kill-switch and zero-budget controls are required')
        for key in ('DATABASE_URL', 'MIGRATION_DATABASE_URL', 'AUTH_EMAIL_KEY', 'CATALOG_RETENTION_KEY', 'POSTGRES_PASSWORD', 'APP_DB_PASSWORD', 'MIGRATOR_DB_PASSWORD', 'LLM_API_KEY', 'TELEGRAM_ENCRYPTION_KEY', 'WHATSAPP_ENCRYPTION_KEY', 'INSTAGRAM_ACCESS_TOKEN', 'INSTAGRAM_APP_SECRET', 'INSTAGRAM_VERIFY_TOKEN', 'INSTAGRAM_ENCRYPTION_KEY', 'ABA_INSTAGRAM_OFFLINE_PROVIDER', 'LLM_BASE_URL', 'TELEGRAM_WEBHOOK_BASE_URL'):
            if flags.get(key):
                raise ValueError('inline runtime credentials are prohibited')
        for key in ('LLM_ENABLE_LIVE', 'LLM_EXTERNAL_REQUESTS_AUTHORIZED', 'TELEGRAM_EXTERNAL_REQUESTS_AUTHORIZED', 'TELEGRAM_SENDING_ENABLED', 'WHATSAPP_LIVE_ENABLED', 'WHATSAPP_EXTERNAL_REQUESTS_AUTHORIZED', 'INSTAGRAM_ENABLED', 'INSTAGRAM_LIVE_ENABLED', 'INSTAGRAM_EXTERNAL_REQUESTS_AUTHORIZED'):
            if key in flags and flags[key] != 'false':
                raise ValueError('live authorization must remain off')
        for key in ('LLM_KILL_SWITCH', 'TELEGRAM_SENDING_KILL_SWITCH', 'WHATSAPP_SENDING_KILL_SWITCH', 'INSTAGRAM_SENDING_KILL_SWITCH'):
            if key in flags and flags[key] != 'true':
                raise ValueError('sending kill switch must remain on')
        for key in ('LLM_PLATFORM_BUDGET_MICRORUB', 'WHATSAPP_MONETARY_BUDGET_RUB', 'INSTAGRAM_MONETARY_BUDGET_RUB'):
            if key in flags and flags[key] != '0':
                raise ValueError('external budgets must remain zero')
    for ref in manifest['images'].values():
        if not re.fullmatch(r'sha256:[0-9a-f]{64}', ref):
            raise ValueError('release images must be immutable local IDs')
        obj = json.loads(subprocess.check_output(['docker', 'image', 'inspect', ref]))[0]
        if obj['Id'] != ref or obj['Os'] != 'linux' or obj['Architecture'] != 'amd64':
            raise ValueError('missing or incompatible local release image')
    return base, manifest

def postgres(base):
    ids = subprocess.check_output(base + ['ps', '-q', 'postgres'], text=True).split()
    if len(ids) != 1:
        raise ValueError('exactly one private PostgreSQL container must be running')
    return ids[0]
