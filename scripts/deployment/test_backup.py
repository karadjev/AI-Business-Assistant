import contextlib
import io
from pathlib import Path
from types import SimpleNamespace
import tempfile
import unittest
from unittest.mock import patch
import backup

class BackupValidationTest(unittest.TestCase):
    def run_backup(self, toc_exit):
        temp=tempfile.TemporaryDirectory();self.addCleanup(temp.cleanup)
        root=Path(temp.name);archive=root/'synthetic.dump'
        calls=[]
        def run(args,**kwargs):
            calls.append(args)
            if 'pg_dump' in args:kwargs['stdout'].write(b'synthetic archive');return SimpleNamespace(returncode=0)
            return SimpleNamespace(returncode=toc_exit)
        argv=['backup.py','backup','--release',str(root),'--env',str(root/'env'),
              '--project','aba-private-qa-backup','--archive',str(archive)]
        output=io.StringIO()
        with (patch('sys.argv',argv),patch('backup.validate',return_value=([],{})),
              patch('backup.postgres',return_value='qa-container'),patch('backup.subprocess.run',side_effect=run),
              contextlib.redirect_stdout(output)):
            if toc_exit:
                with self.assertRaises(RuntimeError):backup.main()
                self.assertFalse(archive.with_suffix('.dump.sha256').exists())
                self.assertNotIn('PASS',output.getvalue())
            else:
                backup.main()
                self.assertEqual(archive.with_suffix('.dump.sha256').read_text().strip(),backup.digest(archive))
                self.assertIn('restore rehearsal still required',output.getvalue())
        self.assertEqual(archive.stat().st_mode&0o777,0o600)
        self.assertTrue(any('pg_restore' in c for c in calls))
    def test_bad_archive_is_retained_without_success_checksum(self):self.run_backup(1)
    def test_toc_verified_before_success_checksum(self):self.run_backup(0)

if __name__=='__main__':unittest.main()
