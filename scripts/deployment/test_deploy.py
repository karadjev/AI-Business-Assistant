import contextlib
import io
import json
from pathlib import Path
from types import SimpleNamespace
import tempfile
import unittest
from unittest.mock import patch
import deploy

class DeploymentTest(unittest.TestCase):
    def setUp(self):
        temp=tempfile.TemporaryDirectory(); self.addCleanup(temp.cleanup)
        self.root=Path(temp.name).resolve()
        self.manifest={'schema':1,'database_schema':16}
        (self.root/'manifest.json').write_text(json.dumps(self.manifest))
        self.a=SimpleNamespace(action='install',release=self.root,env=self.root/'env',
            storage_root=self.root,backup_dir=self.root,tls_ca=self.root/'ca',
            report=self.root/'report.json',project='aba-private-qa-test',local_qa=True,
            go_file=self.root/'go.json')
        self.a.env.write_text('no_credentials=here\n'); self.a.env.chmod(0o600)
        self.a.tls_ca.write_text('synthetic public CA')
        self.write_go()
        self.cfg={'volumes':{'database':{'name':self.a.project+'_database'}},
            'services':{'postgres':{'volumes':[{'type':'volume','source':'database','target':'/var/lib/postgresql'}]},
                        'api':{'environment':{'PUBLIC_ORIGIN':'https://staging.invalid:8443'}}}}
        self.calls=[]
    def write_go(self, **changes):
        value={'approved_by_owner':True,'action':self.a.action,'project':self.a.project,
            'database_schema':16,'migrations_approved':True,'scope':'isolated-local-qa',
            'manifest_sha256':deploy.digest(self.root/'manifest.json')}
        value.update(changes)
        self.a.go_file.write_text(json.dumps(value));self.a.go_file.chmod(0o600)
    def invoke(self, args, **kwargs):
        self.calls.append(args)
        if 'config' in args:return json.dumps(self.cfg)
        if args[:3]==['docker','volume','ls']:
            return self.a.project+'_database\n' if self.a.action=='update' else ''
        if args[0] == deploy.sys.executable:
            archive=Path(args[args.index('--archive')+1]);archive.write_bytes(b'synthetic dump')
            archive.with_suffix('.dump.sha256').write_text(deploy.digest(archive))
        return ''
    def execute(self, invoke=None):
        report={'stages':[],'state_change_started':False}
        with contextlib.ExitStack() as stack:
            stack.enter_context(patch('deploy.invoke',side_effect=invoke or self.invoke))
            stack.enter_context(patch('deploy.local_engine',return_value={}))
            stack.enter_context(patch('deploy.resource_storage',return_value={}))
            stack.enter_context(patch('deploy.validate',return_value=(['compose'],self.manifest)))
            stack.enter_context(patch('deploy.check_secrets'))
            stack.enter_context(patch('deploy.containers',return_value={}))
            stack.enter_context(patch('deploy.database',return_value={}))
            smoke=stack.enter_context(patch('deploy.smoke',return_value={}))
            deploy.execute(self.a,report)
            return report,smoke.call_count
    def test_install_orders_migration_before_wait_and_smoke(self):
        report,smokes=self.execute()
        migration=next(i for i,c in enumerate(self.calls) if 'migrate' in c)
        runtime=next(i for i,c in enumerate(self.calls) if 'api' in c)
        self.assertLess(migration,runtime)
        self.assertIn('--wait',self.calls[runtime]);self.assertIn('--no-deps',self.calls[runtime])
        self.assertTrue(report['state_change_started']);self.assertEqual(smokes,1)
    def test_migration_failure_stops_before_runtime(self):
        def invoke(args,**kwargs):
            if 'migrate' in args:raise ValueError('synthetic migration failure')
            return self.invoke(args,**kwargs)
        with self.assertRaises(ValueError):self.execute(invoke)
        self.assertFalse(any('api' in c for c in self.calls))
    def test_existing_install_volume_refused(self):
        def invoke(args,**kwargs):
            if args[:3]==['docker','volume','ls']:return self.a.project+'_database\n'
            return self.invoke(args,**kwargs)
        with self.assertRaises(ValueError):self.execute(invoke)
        self.assertFalse(any('up' in c for c in self.calls))
    def test_update_requires_verified_backup_before_mutation(self):
        self.a.action='update';self.write_go()
        self.execute()
        backup=next(i for i,c in enumerate(self.calls) if '--archive' in c)
        start=next(i for i,c in enumerate(self.calls) if 'up' in c)
        self.assertLess(backup,start)
    def test_backup_failure_does_not_start_migration_or_runtime(self):
        self.a.action='update';self.write_go()
        def invoke(args,**kwargs):
            if '--archive' in args:raise ValueError('synthetic dump failure')
            return self.invoke(args,**kwargs)
        with self.assertRaises(ValueError):self.execute(invoke)
        self.assertFalse(any('up' in c or 'migrate' in c for c in self.calls))
    def test_go_binds_operation_scope_migration_and_manifest(self):
        for change in ({'action':'update'},{'scope':'private-staging'}, {'migrations_approved':False},
                       {'approved_by_owner':False},{'manifest_sha256':'0'*64},{'database_schema':17}):
            with self.subTest(change=list(change)):
                self.write_go(**change)
                with self.assertRaises(ValueError):self.execute()
        self.assertEqual(self.calls,[])
    def test_remote_docker_refused_before_info(self):
        with patch.dict('os.environ',{'DOCKER_HOST':'ssh://forbidden.invalid'}),patch('deploy.invoke') as run:
            with self.assertRaises(ValueError):deploy.local_engine()
            run.assert_not_called()
    def test_effective_remote_context_refused_with_local_host_env(self):
        def invoke(args,**kwargs):
            if args[1:3]==['context','inspect']:return json.dumps('ssh://forbidden.invalid')
            if args[1:3]==['compose','version']:return '2.40.0'
            return json.dumps({'OSType':'linux','Architecture':'amd64','NCPU':4,'MemTotal':8*1024**3})
        with (patch.dict('os.environ',{'DOCKER_CONTEXT':'remote-test','DOCKER_HOST':'unix:///local/socket'}),
              patch('deploy.invoke',side_effect=invoke) as run):
            with self.assertRaises(ValueError):deploy.local_engine()
            run.assert_called_once_with(['docker','context','inspect','--format','{{json .Endpoints.docker.Host}}'])
    def test_insufficient_engine_resources_refused(self):
        for cpu,ram in ((3,8*1024**3),(4,6*1024**3)):
            replies=['2.40.0',json.dumps({'OSType':'linux','Architecture':'amd64','NCPU':cpu,'MemTotal':ram})]
            with patch.dict('os.environ',{'DOCKER_HOST':'unix:///local/socket'}),patch('deploy.invoke',side_effect=replies):
                with self.assertRaises(ValueError):deploy.local_engine()
    def test_public_volume_or_unhealthy_container_refused(self):
        self.cfg['volumes']['database']['external']=True
        with self.assertRaises(ValueError):deploy.volume_name(self.cfg,self.a.project)
        rows=[{'Service':s,'State':'running','Health':'healthy','Publishers':[]} for s in deploy.SERVICES]
        for field,value in (('Health','unhealthy'),('State','exited'),('Publishers',[{'PublishedPort':5432}])):
            bad=json.loads(json.dumps(rows));bad[0][field]=value
            with patch('deploy.invoke',return_value=json.dumps(bad)):
                with self.assertRaises(ValueError):deploy.containers(['compose'])
    def test_database_rls_and_runtime_role_fail_closed(self):
        valid={'schema':16,'forced':45,'unforced':0,'restricted':True}
        for change in ({'schema':15},{'forced':44},{'unforced':1},{'restricted':False}):
            row={**valid,**change}
            with patch('deploy.invoke',return_value=json.dumps(row)+'\n'+json.dumps({'anonymous_organizations':0})):
                with self.assertRaises(ValueError):deploy.database(['compose'],16)
    def test_mailpit_unhealthy_refused_and_internal_expose_allowed(self):
        rows=[{'Service':s,'State':'running','Health':'healthy','Publishers':[]} for s in deploy.SERVICES]
        rows[0]['Publishers']=[{'TargetPort':5432,'PublishedPort':0}]
        with patch('deploy.invoke',return_value=json.dumps(rows)):
            self.assertEqual(set(deploy.containers(['compose'])),set(deploy.SERVICES))
        rows[1]['Health']='unhealthy'
        with patch('deploy.invoke',return_value=json.dumps(rows)):
            with self.assertRaises(ValueError):deploy.containers(['compose'])
    def test_private_file_symlink_and_public_permissions_refused(self):
        link=self.root/'link';link.symlink_to(self.a.env)
        with self.assertRaises(ValueError):deploy.private_file(link)
        self.a.env.chmod(0o644)
        with self.assertRaises(ValueError):deploy.private_file(self.a.env)
    def test_report_redacts_failure_and_refuses_overwrite(self):
        argv=['deploy.py','preflight','--release',str(self.root),'--env',str(self.a.env),
            '--storage-root',str(self.root),'--backup-dir',str(self.root),'--tls-ca',str(self.a.tls_ca),
            '--report',str(self.a.report),'--project',self.a.project,'--local-qa']
        output=io.StringIO()
        with (patch('sys.argv',argv),patch('deploy.execute',side_effect=ValueError('DO_NOT_LOG_SYNTHETIC_SECRET')),
              contextlib.redirect_stdout(output),contextlib.redirect_stderr(output)):
            self.assertEqual(deploy.main(),42)
            original=self.a.report.read_bytes()
            self.assertEqual(deploy.main(),42)
            self.assertEqual(self.a.report.read_bytes(),original)
        self.assertNotIn('DO_NOT_LOG',output.getvalue()+original.decode())
        self.assertEqual(self.a.report.stat().st_mode&0o777,0o600)

if __name__=='__main__':unittest.main()
