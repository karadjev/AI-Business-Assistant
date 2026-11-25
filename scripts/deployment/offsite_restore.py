#!/usr/bin/env python3
"""Verify and decrypt a downloaded backup for an isolated PostgreSQL rehearsal."""
import argparse
import hmac
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile

from offsite_backup import private_regular, sha256


def recover(envelope, ciphertext_sha, archive_sha, certificate, key, output):
    if not all(re.fullmatch(r"[a-f0-9]{64}", value) for value in (ciphertext_sha, archive_sha)):
        raise ValueError("both expected SHA-256 checksums are required")
    if any(not private_regular(path) for path in (key, envelope)):
        raise ValueError("private recovery key and ciphertext required")
    if not hmac.compare_digest(ciphertext_sha, sha256(envelope)):
        raise ValueError("ciphertext checksum mismatch")
    if not output.parent.is_dir() or output.parent.stat().st_mode & 0o077:
        raise ValueError("private rehearsal output directory required")
    with tempfile.TemporaryDirectory(prefix="aba-offsite-restore-", dir=output.parent) as directory:
        dump = Path(directory) / "recovered.dump"
        subprocess.run(["openssl", "cms", "-decrypt", "-binary", "-inform", "DER",
                        "-in", str(envelope), "-recip", str(certificate), "-inkey", str(key), "-out", str(dump)],
                       check=True, timeout=900, stdout=subprocess.DEVNULL)
        if not hmac.compare_digest(archive_sha, sha256(dump)):
            raise ValueError("decrypted artifact checksum mismatch")
        dump.chmod(0o600)
        os.link(dump, output)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for path in ("envelope", "certificate", "key", "output"):
        parser.add_argument("--" + path, type=Path, required=True)
    parser.add_argument("--ciphertext-sha256", required=True)
    parser.add_argument("--archive-sha256", required=True)
    args = parser.parse_args()
    recover(args.envelope, args.ciphertext_sha256, args.archive_sha256,
            args.certificate, args.key, args.output)
    print("PASS encrypted archive authentication and original SHA-256. Restore only on an isolated database.")


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, subprocess.SubprocessError) as error:
        print(f"BLOCKED offsite recovery: {type(error).__name__}", file=sys.stderr)
        raise SystemExit(1)
