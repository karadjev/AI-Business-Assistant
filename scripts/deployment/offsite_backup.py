#!/usr/bin/env python3
"""Encrypt a verified local PostgreSQL dump for an offline recovery recipient."""
import argparse
import hashlib
import hmac
import os
from pathlib import Path
import re
import stat
import subprocess
import sys
import tempfile


def sha256(path):
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def private_regular(path):
    info = path.lstat()
    return stat.S_ISREG(info.st_mode) and not info.st_mode & 0o077


def run(argv):
    subprocess.run(argv, check=True, timeout=900, stdin=subprocess.DEVNULL,
                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)


def verify_archive(archive, checksum):
    if not private_regular(archive) or not private_regular(checksum):
        raise ValueError("private regular dump and checksum required")
    fields = checksum.read_text().strip().split(maxsplit=1)
    if not fields or not re.fullmatch(r"[a-f0-9]{64}", fields[0]):
        raise ValueError("invalid local checksum")
    if not hmac.compare_digest(fields[0], sha256(archive)):
        raise ValueError("local backup checksum mismatch")
    return fields[0]


def encrypt(archive, recipient, output):
    run(["openssl", "cms", "-encrypt", "-binary", "-aes-256-gcm", "-outform", "DER",
         "-in", str(archive), "-out", str(output), str(recipient)])


def destination():
    bucket = os.environ.get("ABA_OFFSITE_BUCKET", "")
    prefix = os.environ.get("ABA_OFFSITE_PREFIX", "")
    if not re.fullmatch(r"[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]", bucket) or not re.fullmatch(r"[A-Za-z0-9_-]+(?:/[A-Za-z0-9_-]+)*", prefix):
        raise ValueError("private S3 destination configuration required")
    return f"s3://{bucket}/{prefix}"


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--archive", type=Path, required=True)
    parser.add_argument("--checksum", type=Path, required=True)
    parser.add_argument("--recipient-certificate", type=Path, required=True)
    parser.add_argument("--recipient-fingerprint", required=True, help="Pinned certificate SHA-256 of the provisioned PEM bytes")
    args = parser.parse_args()
    local_sha = verify_archive(args.archive, args.checksum)
    if (not re.fullmatch(r"[a-f0-9]{64}", args.recipient_fingerprint)
            or not hmac.compare_digest(args.recipient_fingerprint, sha256(args.recipient_certificate))):
        raise ValueError("recovery certificate fingerprint mismatch")
    target = destination()
    os.umask(0o077)
    with tempfile.TemporaryDirectory(prefix="aba-offsite-") as directory:
        envelope = Path(directory) / "backup.cms"
        returned = Path(directory) / "verify.cms"
        encrypt(args.archive, args.recipient_certificate, envelope)
        cipher_sha = sha256(envelope)
        # Recheck the immutable source before sending the ciphertext.
        if not hmac.compare_digest(local_sha, sha256(args.archive)):
            raise ValueError("backup changed during encryption")
        uri = f"{target}/{local_sha}/{cipher_sha}.cms"
        run(["aws", "s3", "cp", str(envelope), uri, "--only-show-errors"])
        run(["aws", "s3", "cp", uri, str(returned), "--only-show-errors"])
        if not hmac.compare_digest(cipher_sha, sha256(returned)):
            raise ValueError("offsite ciphertext checksum mismatch")
    print(f"PASS encrypted upload and readback ciphertext SHA-256 {cipher_sha}")


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, subprocess.SubprocessError) as error:
        print(f"BLOCKED offsite backup: {type(error).__name__}", file=sys.stderr)
        raise SystemExit(1)
