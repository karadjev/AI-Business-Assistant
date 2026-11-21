import copy
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
from common import command, validate

class PrivateAdmissionTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.image = 'sha256:' + 'a' * 64
        self.roles = {'api':'api','worker':'api','migrate':'api','web':'web','postgres':'postgres','proxy':'proxy','mailpit':'mailpit'}
        self.manifest = {'schema':1,'platform':'linux/amd64','runtime_security_admission_version':1,'files':{},'images':{v:self.image for v in self.roles.values()}}
        (self.root/'manifest.json').write_text(json.dumps(self.manifest))
        self.cfg = {'networks':{'private':{'internal':True}},'services':{k:{'image':self.image,'pull_policy':'never'} for k in self.roles}}
        for service in ('api','worker'):
            self.cfg['services'][service]['command']=[service]
        self.cfg['services']['migrate']['command']=['migrate','up']
        for service in ('api','worker'):
            self.cfg['services'][service]['environment'] = {
                'APP_ENV':'production','PUBLIC_ORIGIN':'https://staging.invalid:8443',
                'LLM_PROVIDER':'mock','TELEGRAM_TRANSPORT':'mock',
                'LLM_ENABLE_LIVE':'false','LLM_EXTERNAL_REQUESTS_AUTHORIZED':'false',
                'LLM_KILL_SWITCH':'true','LLM_PLATFORM_BUDGET_MICRORUB':'0',
                'TELEGRAM_EXTERNAL_REQUESTS_AUTHORIZED':'false','TELEGRAM_SENDING_ENABLED':'false',
                'TELEGRAM_SENDING_KILL_SWITCH':'true','WHATSAPP_LIVE_ENABLED':'false',
                'WHATSAPP_EXTERNAL_REQUESTS_AUTHORIZED':'false',
                'WHATSAPP_SENDING_KILL_SWITCH':'true','WHATSAPP_MONETARY_BUDGET_RUB':'0',
            }
    def check(self, cfg, docker_env=None, endpoint='unix:///synthetic/local.sock'):
        def reply(args):
            if args[1:3]==['context','inspect']:return json.dumps(endpoint).encode()
            if args[1:3]==['image','inspect']:return json.dumps([{'Id':self.image,'Os':'linux','Architecture':'amd64'}]).encode()
            return json.dumps(cfg).encode()
        with (patch.dict('os.environ',docker_env or {'DOCKER_HOST':'','DOCKER_CONTEXT':''}),
              patch('common.subprocess.check_output', side_effect=reply)):
            return validate(self.root,self.root/'env','aba-private-test')
    def test_private_valid(self):
        self.check(self.cfg)
    def test_legacy_or_invalid_runtime_admission_rejected(self):
        for version in (None,0,'1',True,2):
            with self.subTest(version=version):
                self.manifest['runtime_security_admission_version']=version
                (self.root/'manifest.json').write_text(json.dumps(self.manifest))
                with self.assertRaises(ValueError):self.check(self.cfg)
    def test_generated_bytecode_rejected_even_if_hashed(self):
        import hashlib
        path=self.root/'scripts/deployment/__pycache__/common.pyc'
        path.parent.mkdir(parents=True);path.write_bytes(b'synthetic bytecode fixture')
        self.manifest['files']={str(path.relative_to(self.root)):hashlib.sha256(path.read_bytes()).hexdigest()}
        (self.root/'manifest.json').write_text(json.dumps(self.manifest))
        with self.assertRaisesRegex(ValueError,'generated Python bytecode'):self.check(self.cfg)
    def test_missing_live_and_budget_flags_fail_closed(self):
        required = (
            'LLM_ENABLE_LIVE','LLM_EXTERNAL_REQUESTS_AUTHORIZED',
            'LLM_KILL_SWITCH','LLM_PLATFORM_BUDGET_MICRORUB',
            'TELEGRAM_EXTERNAL_REQUESTS_AUTHORIZED','TELEGRAM_SENDING_ENABLED',
            'TELEGRAM_SENDING_KILL_SWITCH','WHATSAPP_LIVE_ENABLED',
            'WHATSAPP_EXTERNAL_REQUESTS_AUTHORIZED',
            'WHATSAPP_SENDING_KILL_SWITCH','WHATSAPP_MONETARY_BUDGET_RUB',
        )
        for service in ('api','worker'):
            for key in required:
                with self.subTest(service=service, missing=key):
                    cfg=copy.deepcopy(self.cfg)
                    del cfg['services'][service]['environment'][key]
                    with self.assertRaises(ValueError):self.check(cfg)
    def test_public_and_live_mutations_fail_closed(self):
        changes = [
            ('ports',[{'target':8443,'published':'8443','host_ip':'0.0.0.0'}]),
            ('privileged',True),('network_mode','host'),('image','mutable:latest'),
            ('volumes',[{'source':'/var/run/docker.sock','target':'/var/run/docker.sock'}]),
            ('tmpfs',['/tmp:size=32m','mode=1777']),
        ]
        for key,value in changes:
            with self.subTest(key=key):
                cfg=copy.deepcopy(self.cfg);cfg['services']['api'][key]=value
                with self.assertRaises(ValueError):self.check(cfg)
        for key,value in [('LLM_ENABLE_LIVE','true'),('WHATSAPP_MONETARY_BUDGET_RUB','1'),('TELEGRAM_SENDING_KILL_SWITCH','false'),('DATABASE_URL','synthetic-inline'),('APP_ENV','development'),('PUBLIC_ORIGIN','http://staging.invalid')]:
            with self.subTest(flag=key):
                cfg=copy.deepcopy(self.cfg);cfg['services']['api']['environment'][key]=value
                with self.assertRaises(ValueError):self.check(cfg)
    def test_external_network_and_working_project_rejected(self):
        cfg=copy.deepcopy(self.cfg);cfg['networks']['private']={'internal':False}
        with self.assertRaises(ValueError):self.check(cfg)
        with self.assertRaises(ValueError):command(self.root,self.root/'env','ai-business-assistant')
    def test_manifest_tamper_rejected(self):
        (self.root/'unsafe').write_text('changed')
        self.manifest['files']={'unsafe':'0'*64}
        (self.root/'manifest.json').write_text(json.dumps(self.manifest))
        with self.assertRaises(ValueError):self.check(self.cfg)

    def test_alternative_execution_paths_rejected(self):
        changes = [('api','entrypoint',['/app/api']),
                   ('worker','entrypoint',['/app/worker']),
                   ('api','entrypoint',[]),
                   ('worker','command',['api']),
                   ('migrate','command',['api']),
                   ('web','entrypoint',['/usr/bin/node','.next/standalone/server.js'])]
        for service,key,value in changes:
            with self.subTest(service=service,key=key,value=value):
                cfg=copy.deepcopy(self.cfg);cfg['services'][service][key]=value
                with self.assertRaises(ValueError):self.check(cfg)

    def test_every_cli_refuses_remote_docker_before_compose(self):
        for env,endpoint in (({'DOCKER_HOST':'ssh://forbidden.invalid','DOCKER_CONTEXT':''},'unix:///synthetic/local.sock'),
                             ({'DOCKER_HOST':'unix:///synthetic/local.sock','DOCKER_CONTEXT':'remote-test'},'ssh://forbidden.invalid')):
            with self.subTest(env=env):
                with self.assertRaisesRegex(ValueError,'local Unix'):
                    self.check(self.cfg,docker_env=env,endpoint=endpoint)

    def test_effective_local_endpoint_pinned_for_child_commands(self):
        import os
        from common import pin_local_docker
        with (patch.dict('os.environ',{'DOCKER_HOST':'ssh://ignored.invalid','DOCKER_CONTEXT':'local-test'}),
              patch('common.subprocess.check_output',return_value=b'"unix:///synthetic/effective.sock"') as run):
            pin_local_docker()
            self.assertEqual(os.environ['DOCKER_HOST'],'unix:///synthetic/effective.sock')
            self.assertNotIn('DOCKER_CONTEXT',os.environ)
            run.assert_called_once_with(['docker','context','inspect','--format','{{json .Endpoints.docker.Host}}'])

if __name__ == '__main__':unittest.main()
