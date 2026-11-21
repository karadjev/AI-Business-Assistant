import hashlib
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

class ReleaseImmutabilityTest(unittest.TestCase):
    def test_plain_python_cli_help_never_writes_into_release(self):
        with tempfile.TemporaryDirectory() as temp:
            tools=Path(temp)/'deployment'
            shutil.copytree(Path(__file__).parent,tools,ignore=shutil.ignore_patterns('__pycache__','*.pyc'))
            def snapshot():
                return {str(p.relative_to(tools)):hashlib.sha256(p.read_bytes()).hexdigest() for p in tools.rglob('*') if p.is_file()}
            before=snapshot()
            env={k:v for k,v in os.environ.items() if k!='PYTHONDONTWRITEBYTECODE'}
            for script in ('control.py','backup.py','deploy.py','backup_job.py'):
                with self.subTest(script=script):
                    result=subprocess.run([sys.executable,str(tools/script),'--help'],env=env,capture_output=True,text=True,timeout=10)
                    self.assertEqual(result.returncode,0)
                    self.assertFalse(list(tools.rglob('__pycache__')),'CLI polluted the immutable release')
                    self.assertEqual(snapshot(),before)

if __name__=='__main__':unittest.main()
