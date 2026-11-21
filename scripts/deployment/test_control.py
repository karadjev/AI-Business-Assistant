import contextlib
import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import control


class ManualControlTest(unittest.TestCase):
    def setUp(self):
        temp=tempfile.TemporaryDirectory();self.addCleanup(temp.cleanup)
        self.root=Path(temp.name)
        self.manifest={'database_schema':16}
        (self.root/'manifest.json').write_text(json.dumps(self.manifest))
        self.go=self.root/'go.json'
        self.value={'approved_by_owner':True,'action':'start','project':'aba-private-staging',
                    'manifest_sha256':control.hashlib.sha256((self.root/'manifest.json').read_bytes()).hexdigest(),
                    'scope':'private-staging','database_schema':16,'migrations_approved':True}

    def invoke(self, **changes):
        value={**self.value,**changes}
        self.go.write_text(json.dumps(value));self.go.chmod(0o600)
        args=['control.py','start','--release',str(self.root),'--env',str(self.root/'env'),
              '--project','aba-private-staging','--go-file',str(self.go)]
        with (patch('sys.argv',args),patch('control.validate',return_value=(['compose'],self.manifest)),
              patch('control.subprocess.run') as run,contextlib.redirect_stderr(io.StringIO()),
              contextlib.redirect_stdout(io.StringIO())):
            try:
                control.main()
                refused=False
            except SystemExit:
                refused=True
            return refused,run.call_count

    def test_valid_scoped_start(self):
        self.assertEqual(self.invoke(),(False,3))

    def test_scope_schema_and_migration_approval_fail_closed(self):
        for change in ({'scope':'isolated-local-qa'},{'scope':None},
                       {'database_schema':17},{'database_schema':None},{'migrations_approved':False},
                       {'action':'install'},{'approved_by_owner':False},{'manifest_sha256':'0'*64}):
            with self.subTest(change=change):
                self.assertEqual(self.invoke(**change),(True,0))

    def test_public_or_symlink_go_refused_before_mutation(self):
        original=Path.lstat
        for mode in ('public','symlink'):
            def info(path):
                if path == self.go:
                    target=self.root/'other.json'
                    if mode == 'public':path.chmod(0o644)
                    else:
                        target.write_text(path.read_text());path.unlink();path.symlink_to(target)
                return original(path)
            with self.subTest(mode=mode),patch.object(Path,'lstat',info):
                self.assertEqual(self.invoke(),(True,0))
            if self.go.is_symlink():self.go.unlink()

    def test_explicit_local_qa_scope_and_project(self):
        with patch('control.argparse.ArgumentParser.parse_args') as parse:
            from types import SimpleNamespace
            for project,expected in (('aba-private-qa-test',False),('aba-private-staging',True)):
                self.value.update(project=project,scope='isolated-local-qa')
                parse.return_value=SimpleNamespace(action='start',release=self.root,env=self.root/'env',project=project,go_file=self.go,local_qa=True)
                self.assertEqual(self.invoke(),(expected,0 if expected else 3))


if __name__=='__main__':unittest.main()
