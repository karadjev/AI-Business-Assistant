import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

from offsite_backup import encrypt, sha256, verify_archive
from offsite_restore import recover


@unittest.skipUnless(shutil.which("openssl"), "requires the prepared OpenSSL binary")
class OffsiteBackupTest(unittest.TestCase):
    def test_encrypt_restore_and_tamper(self):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            archive, checksum = root / "synthetic.dump", root / "synthetic.dump.sha256"
            archive.write_bytes(b"PGDMP\x00\x99" * 100)
            archive.chmod(0o600)
            checksum.write_text(sha256(archive) + "\n")
            checksum.chmod(0o600)
            verify_archive(archive, checksum)
            cert, key = root / "cert.pem", root / "offline.key"
            subprocess.run(["openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes",
                            "-subj", "/CN=isolated-backup-test", "-days", "1", "-keyout", str(key), "-out", str(cert)],
                           check=True, capture_output=True)
            cipher, restored = root / "cipher.cms", root / "restored.dump"
            encrypt(archive, cert, cipher)
            self.assertNotIn(archive.read_bytes()[:100], cipher.read_bytes())
            key.chmod(0o600)
            cipher.chmod(0o600)
            recover(cipher, sha256(cipher), sha256(archive), cert, key, restored)
            self.assertEqual(sha256(archive), sha256(restored))
            with self.assertRaises(FileExistsError):
                recover(cipher, sha256(cipher), sha256(archive), cert, key, restored)
            with self.assertRaises(ValueError):
                recover(cipher, "0"*64, sha256(archive), cert, key, root/"bad.dump")
            decrypt = ["openssl", "cms", "-decrypt", "-binary", "-inform", "DER",
                       "-in", str(cipher), "-recip", str(cert), "-inkey", str(key), "-out", str(restored)]
            subprocess.run(decrypt, check=True, capture_output=True)
            self.assertEqual(sha256(archive), sha256(restored))
            data = bytearray(cipher.read_bytes())
            data[-6] ^= 1
            cipher.write_bytes(data)
            self.assertNotEqual(subprocess.run(decrypt, capture_output=True).returncode, 0)
            os.chmod(checksum, 0o644)
            with self.assertRaises(ValueError):
                verify_archive(archive, checksum)
