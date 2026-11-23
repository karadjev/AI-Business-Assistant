#!/usr/bin/env python3
"""Private deployment coordinator. No downloads, public ports or implicit cleanup."""
import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import shutil
import stat
import subprocess
import sys
import tempfile
from urllib.parse import parse_qs, unquote, urlparse

sys.dont_write_bytecode = True
from common import command, validate

SERVICES = ('postgres', 'mailpit', 'api', 'worker', 'web', 'proxy')
EXPECTED_FORCED_RLS_TABLES = {16: 45, 29: 62, 30: 62, 31: 62}
HERE = Path(__file__).resolve().parent

def digest(path):
    with path.open('rb') as f:
        return hashlib.file_digest(f, 'sha256').hexdigest()

def invoke(args, *, data=None, timeout=60):
    result = subprocess.run(args, input=data, capture_output=True, text=True, timeout=timeout,
                            env={**os.environ, 'PYTHONDONTWRITEBYTECODE':'1'})
    if result.returncode:
        # Never echo commands, stderr, DSNs, SQL errors or provider responses.
        raise ValueError('local command failed; state and evidence retained')
    return result.stdout

def private_file(path, maximum=65536):
    s = path.lstat()
    if not stat.S_ISREG(s.st_mode) or s.st_mode & 0o077 or not 0 < s.st_size <= maximum:
        raise ValueError('private regular file required')
    return path.read_bytes()

def private_directory(path):
    if path.is_symlink() or path.resolve() != path.absolute() or not path.is_dir() or path.stat().st_mode & 0o077:
        raise ValueError('existing private directory required')
    with tempfile.TemporaryFile(dir=path) as f:
        f.write(b'aba storage probe\n'); f.flush(); os.fsync(f.fileno())

def local_engine():
    override = os.environ.get('DOCKER_HOST', '')
    # Ask the installed CLI to resolve an explicitly configured context rather
    # than guessing precedence between context and host across CLI versions.
    host = (json.loads(invoke(['docker', 'context', 'inspect', '--format', '{{json .Endpoints.docker.Host}}']))
            if os.environ.get('DOCKER_CONTEXT') or not override else override)
    if not host.startswith('unix:///'):
        raise ValueError('only a local Unix Docker socket is allowed')
    # Pin the validated endpoint for this process and its children only.
    # This does not change Docker contexts or any host/global configuration.
    os.environ['DOCKER_HOST'] = host
    os.environ.pop('DOCKER_CONTEXT', None)
    invoke(['docker', 'compose', 'version', '--short'])
    info = json.loads(invoke(['docker', 'info', '--format', '{{json .}}']))
    if info.get('OSType') != 'linux' or info.get('Architecture') not in ('x86_64', 'amd64'):
        raise ValueError('linux/amd64 Docker Engine required')
    if info.get('NCPU', 0) < 4 or info.get('MemTotal', 0) < 7 * 1024**3:
        raise ValueError('at least four CPUs and seven GiB available Engine RAM required')
    return info

def owner_go(a, manifest):
    if a.go_file is None:
        raise ValueError('separate owner GO required')
    go = json.loads(private_file(a.go_file))
    if (go.get('approved_by_owner') is not True or go.get('action') != a.action
            or go.get('project') != a.project
            or go.get('manifest_sha256') != digest(a.release / 'manifest.json')
            or go.get('database_schema') != manifest['database_schema']
            or go.get('migrations_approved') is not True
            or go.get('scope') != ('isolated-local-qa' if a.local_qa else 'private-staging')):
        raise ValueError('GO must bind exact operation, manifest, project, schema and scope')

def resource_storage(a, info):
    if not a.local_qa:
        release = platform.freedesktop_os_release() if platform.system() == 'Linux' else {}
        if release.get('ID') != 'ubuntu' or release.get('VERSION_ID') != '24.04':
            raise ValueError('native Ubuntu 24.04 required; local QA is a separate scope')
        engine_root = Path(info['DockerRootDir'])
        if engine_root.stat().st_dev != a.storage_root.stat().st_dev:
            raise ValueError('storage root must be on the Docker data filesystem')
    elif not re.fullmatch(r'aba-private-qa-[a-z0-9-]+', a.project):
        raise ValueError('local QA requires its dedicated project prefix')
    private_directory(a.storage_root)
    private_directory(a.backup_dir)
    usage = shutil.disk_usage(a.storage_root)
    if usage.total < 70 * 1024**3 or usage.free < 20 * 1024**3:
        raise ValueError('storage requires seventy GiB total and twenty GiB free')
    return {'engine_cpus': info['NCPU'], 'engine_ram_bytes': info['MemTotal'],
            'storage_total_bytes': usage.total, 'storage_free_bytes': usage.free,
            'storage_scope': 'QA host paths; Engine disk NOT VERIFIED' if a.local_qa else 'native Docker filesystem',
            'observed_application_usage': 'NOT MEASURED'}

def check_secrets(cfg, local_qa):
    private_keys = {'database_url':10001, 'migration_database_url':10001,
                    'auth_email_key':10001, 'retention_key':10001,
                    'telegram_key':10001, 'whatsapp_key':10001,
                    'admin_password':70, 'app_password':70, 'migrator_password':70,
                    'db_key':70, 'proxy_key':101}
    expected = set(private_keys) | {'db_ca','db_certificate','proxy_certificate'}
    if set(cfg.get('secrets', {})) != expected:
        raise ValueError('unexpected or missing secret definitions')
    values = {}
    for name, definition in cfg['secrets'].items():
        path = Path(definition['file'])
        s = path.lstat()
        if not stat.S_ISREG(s.st_mode) or s.st_size == 0 or s.st_size > 65536:
            raise ValueError('secret file unavailable or invalid')
        if name in private_keys:
            values[name] = private_file(path)
            if not local_qa and s.st_uid != private_keys[name]:
                raise ValueError('secret owner does not match container UID')
    for name in ('admin_password','app_password','migrator_password'):
        if not re.fullmatch(rb'[0-9a-f]{48}\n?', values[name]):
            raise ValueError('database passwords require independent random hex values')
    if len({values[name].strip() for name in ('admin_password','app_password','migrator_password')}) != 3:
        raise ValueError('database role credentials must differ')
    encryption_keys = ('auth_email_key', 'retention_key', 'telegram_key', 'whatsapp_key')
    for name in encryption_keys:
        if not re.fullmatch(rb'[0-9a-fA-F]{64}\n?', values[name]):
            raise ValueError('encryption key format invalid')
    if len({bytes.fromhex(values[name].decode().strip()) for name in encryption_keys}) != len(encryption_keys):
        raise ValueError('encryption keys must differ')
    for name, role in (('database_url','aba_app'),('migration_database_url','aba_migrator')):
        u = urlparse(values[name].decode().strip())
        options = parse_qs(u.query)
        password = values['app_password' if role == 'aba_app' else 'migrator_password'].decode().strip()
        if (u.scheme not in ('postgres','postgresql') or u.hostname != 'postgres'
                or u.username != role or u.path != '/aba_stage'
                or u.password is None or unquote(u.password) != password
                or options.get('sslmode') != ['verify-full']
                or options.get('sslrootcert') != ['/run/secrets/db_ca']):
            raise ValueError('database role, database or TLS configuration invalid')

def volume_name(cfg, project):
    value = cfg.get('volumes', {}).get('database', {})
    if value.get('external') or value.get('driver', 'local') != 'local' or value.get('driver_opts') or value.get('name') != project + '_database':
        raise ValueError('persistent volume must belong exclusively to this project')
    mounts = cfg['services']['postgres'].get('volumes', [])
    if not any(m.get('type') == 'volume' and m.get('source') == 'database'
               and m.get('target') == '/var/lib/postgresql' and not m.get('read_only') for m in mounts):
        raise ValueError('persistent PostgreSQL mount missing')
    return value['name']

def existing_volume(name):
    return invoke(['docker','volume','ls','--filter','name=^'+re.escape(name)+'$','--format','{{.Name}}']).split()

def containers(base, expected_images=None):
    raw = invoke(base + ['ps','--all','--format','json']).strip()
    rows = json.loads(raw) if raw.startswith('[') else [json.loads(line) for line in raw.splitlines()]
    found = {}
    for row in rows:
        service = row['Service']
        if service == 'migrate' and row.get('State') == 'exited' and row.get('ExitCode') == 0:
            continue
        published = any(p.get('PublishedPort') not in (None,0) for p in (row.get('Publishers') or []))
        if service not in SERVICES or service in found or row.get('State') != 'running' or published:
            raise ValueError('unexpected, stopped or exposed runtime container')
        if row.get('Health') != 'healthy' and (service != 'mailpit' or row.get('Health')):
            raise ValueError('runtime container is not healthy')
        found[service] = {'state':'running','health':row.get('Health') or 'not configured'}
        if expected_images is not None:
            image = invoke(['docker', 'inspect', '--type', 'container', '--format', '{{.Image}}', row['ID']]).strip()
            expected = expected_images['api' if service == 'worker' else service]
            if image != expected:
                raise ValueError(f'{service} image does not match the release manifest')
            found[service]['image'] = image
    if set(found) != set(SERVICES):
        raise ValueError('runtime services incomplete')
    return found

def database(base, schema):
    expected_forced = EXPECTED_FORCED_RLS_TABLES.get(schema)
    if expected_forced is None:
        raise ValueError('database schema has no reviewed FORCE RLS count')
    sql = """SELECT json_build_object(
 'schema',(SELECT version FROM platform.service_metadata WHERE singleton),
 'forced',(SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='platform' AND c.relrowsecurity AND c.relforcerowsecurity),
 'unforced',(SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='platform' AND c.relrowsecurity AND NOT c.relforcerowsecurity),
 'restricted',(SELECT NOT (rolsuper OR rolbypassrls OR rolcreatedb OR rolcreaterole OR rolreplication) AND NOT has_schema_privilege('aba_app','platform','CREATE') AND NOT pg_has_role('aba_app','aba_migrator','MEMBER') FROM pg_roles WHERE rolname='aba_app'),
 'lock_timeout',current_setting('lock_timeout'),
 'transaction_timeout',current_setting('transaction_timeout'),
 'idle_in_transaction_session_timeout',current_setting('idle_in_transaction_session_timeout'));
BEGIN READ ONLY; SET LOCAL ROLE aba_app;
SELECT json_build_object('anonymous_organizations',count(*)) FROM platform.organizations;
ROLLBACK;"""
    raw = invoke(base + ['exec','-T','postgres','psql','-X','-qAt','-U','postgres','-d','aba_stage','-v','ON_ERROR_STOP=1'], data=sql)
    rows = [json.loads(line) for line in raw.splitlines() if line.startswith('{')]
    if (len(rows) != 2 or rows[0].get('schema') != schema or rows[0].get('forced') != expected_forced
            or rows[0].get('unforced') != 0 or rows[0].get('restricted') is not True
            or rows[1].get('anonymous_organizations') != 0):
        raise ValueError('database schema, FORCE RLS or runtime role check failed')
    return rows

def smoke(base, origin, ca):
    raw = invoke(base + ['exec','-T','web','/usr/bin/node','-e',(HERE/'smoke.cjs').read_text()],
                 data=json.dumps({'origin':origin,'ca':ca.read_text()}), timeout=45)
    if json.loads(raw) != {'api_ready':200,'tls_login':200,'unauthenticated':401,'csrf':403}:
        raise ValueError('private HTTPS/readiness/auth smoke failed')
    return json.loads(raw)

def execute(a, report):
    def step(name, fn):
        report['last_stage'] = name
        value = fn()
        report['stages'].append({'name':name,'status':'PASS'})
        return value
    manifest = json.loads((a.release/'manifest.json').read_text())
    if a.action in ('install','update'):
        step('owner_go',lambda:owner_go(a,manifest))
    step('private_environment_file',lambda:private_file(a.env))
    info = step('local_engine',local_engine)
    report['resources'] = step('resources_storage',lambda:resource_storage(a,info))
    base, manifest = step('package_admission',lambda:validate(a.release,a.env,a.project))
    cfg = step('resolved_config',lambda:json.loads(invoke(base+['config','--format','json'])))
    name = step('persistent_volume',lambda:volume_name(cfg,a.project))
    step('secret_files',lambda:check_secrets(cfg,a.local_qa))
    step('tls_ca_file',lambda:private_or_public_ca(a.tls_ca))
    if a.action in ('bootstrap-check','preflight'):
        return
    if a.action == 'install':
        def fresh_install():
            if existing_volume(name) or invoke(base+['ps','--all','-q']).strip():
                raise ValueError('install requires a new isolated project and volume; use update for an existing install')
        step('fresh_install',fresh_install)
    if a.action == 'update':
        step('existing_health',lambda:containers(base))
        def owned_volume():
            if existing_volume(name) != [name]:
                raise ValueError('update requires its existing persistent database volume')
        step('existing_volume',owned_volume)
        archive=a.backup_dir/('before-update-'+datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%S%f')+'.dump')
        step('backup_before_update',lambda:invoke([sys.executable,str(HERE/'backup.py'),'backup','--release',str(a.release),'--env',str(a.env),'--project',a.project,'--archive',str(archive)],timeout=300))
        def backup_checksum():
            if digest(archive) != archive.with_suffix('.dump.sha256').read_text().strip():
                raise ValueError('pre-update backup checksum failed')
        step('backup_checksum',backup_checksum)
        report['backup_archive_name']=archive.name
    if a.action in ('install','update'):
        report['state_change_started']=True
        step('postgres_start',lambda:invoke(base+['up','-d','--pull','never','--wait','--wait-timeout','120','postgres','mailpit'],timeout=150))
        step('migration',lambda:invoke(base+['run','--rm','--no-deps','migrate'],timeout=300))
        step('runtime_start',lambda:invoke(base+['up','-d','--no-deps','--pull','never','--wait','--wait-timeout','180','api','worker','web','proxy'],timeout=210))
    report['containers']=step('runtime_health',lambda:containers(base, manifest['images']))
    report['database']=step('database_safety',lambda:database(base,manifest['database_schema']))
    report['smoke']=step('https_auth_smoke',lambda:smoke(base,cfg['services']['api']['environment']['PUBLIC_ORIGIN'],a.tls_ca))
    # Project-scoped metrics only; absence does not become a zero-usage claim.
    if a.action == 'diagnose':
        ids=invoke(base+['ps','-q']).split()
        report['usage']=step('resource_snapshot',lambda:invoke(['docker','stats','--no-stream','--format','{{json .}}',*ids]))

def private_or_public_ca(path):
    s=path.lstat()
    if not stat.S_ISREG(s.st_mode) or not 0 < s.st_size <= 65536 or b'PRIVATE KEY' in path.read_bytes():
        raise ValueError('public CA certificate file required')

def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('action',choices=['bootstrap-check','preflight','install','update','health','diagnose'])
    for key in ('release','env','storage-root','backup-dir','tls-ca','report'):
        p.add_argument('--'+key,type=Path,required=True)
    p.add_argument('--project',required=True)
    p.add_argument('--go-file',type=Path)
    p.add_argument('--local-qa',action='store_true',help='explicit macOS/local QA scope; never certifies Ubuntu or Engine disk')
    a=p.parse_args()
    report={'schema':1,'action':a.action,'scope':'isolated-local-qa' if a.local_qa else 'private-staging',
            'at_utc':datetime.datetime.now(datetime.timezone.utc).isoformat(),
            'status':'BLOCKED','customer_pilot':'NO-GO','state_change_started':False,'stages':[]}
    try:
        command(a.release,a.env,a.project)
        private_directory(a.report.parent)
        fd=os.open(a.report,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600)
    except (ValueError,OSError):
        print('BLOCKED: invalid project or private report destination; nothing started',file=sys.stderr)
        return 42
    code=0
    with os.fdopen(fd,'w') as out:
        try:
            execute(a,report)
            report['status']='PASS'
        except (ValueError,OSError,KeyError,TypeError,subprocess.SubprocessError):
            report['status']='FAIL' if report['state_change_started'] else 'BLOCKED'
            report['stages'].append({'name':report.get('last_stage','input'),'status':report['status']})
            code=1 if report['state_change_started'] else 42
        finally:
            json.dump(report,out,indent=2);out.write('\n');out.flush();os.fsync(out.fileno())
    print(report['status']+': '+report.get('last_stage','input')+'; CUSTOMER PILOT NO-GO; see private report')
    return code

if __name__=='__main__':
    sys.exit(main())
