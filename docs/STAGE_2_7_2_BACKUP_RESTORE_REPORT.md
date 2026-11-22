# Stage 2.7.2 — Backup and Restore

**Local backup/restore: PASS, scoped. External custody: BLOCKED.**

## Backup

- Backup implementation: candidate's existing `backup.py`; no parallel backup system added.
- New snapshot: `/srv/aba/staging/backups/stage272-20261122T175412Z.dump`
- Size/mode: `890683` bytes / `0600`
- SHA-256: `8713742c1b71434f24ab4619ec7096b38d8ff288519aa990e31b1079dbe55ec9`
- Candidate tool reported: `PASS: private archive and table of contents verified`.
- `aba-backup.timer`: enabled, existing daily 02:30 UTC schedule. The timer was inspected, not manually triggered; snapshot was generated directly for this acceptance.
- Existing backups were retained. No pruning/retention job was run.

An initial invocation with an incorrect shared environment failed closed with a release-image admission error before producing a backup. Retrying with the candidate-specific environment succeeded. This initial FAIL is retained.

## Restore rehearsal

The archive was restored with the existing rehearsal command into a separate Compose project and volume `aba-private-restore-stage272_database`; the public volume `aba-private-staging_database` was not used as the restore target. A first isolated network allocation overlapped with another network and failed before the database service started; a new unused private subnet resolved the collision. The restore command returned exit 0 and reported isolated restore complete, with RLS/ACL/data validation still required.

Post-restore source and target fingerprints were compared and matched exactly:

| Fingerprint | Public source | Isolated restore |
|---|---:|---:|
| Schema version / migration rows | `29 / 30` | `29 / 30` |
| FORCE RLS relations | `62` | `62` |
| Organizations | `19`, digest `ca1c0b9d8eb3bf03f8c008b50e6c8a7d` | exact match |
| Tasks | `39`, digest `5e6ef91e9fb8c9b4504629c71768675b` | exact match |
| Agent audit rows | `200`, digest `6ae09a0c12075bcaa77380d95609ca7b` | exact match |
| Human approvals | `9`, digest `f868e2b554248c131097068920fa3e5f` | exact match |

Fingerprint comparison: **PASS**. The restore volume was retained.

## Test harness incident and recovery

To run an isolated delegation integration test, a separate `aba_test` database was prepared inside the restored PostgreSQL service. The restored `pg_hba.conf` intentionally only allowed the application database, so fixture connection failed with SQLSTATE `28000` before assertions. A test-only HBA override was composed and validated, but PostgreSQL then failed closed because the mounted file was not readable (`Permission denied`). The override was immediately removed and the PostgreSQL service recreated from the original Compose configuration; `up --wait` returned **exit 0**, service health was **healthy**, and `pg_isready` reported accepting connections. The public staging database/service was not restarted or modified. The temporary runner container was stopped and retained (not deleted).

The failed integration setup is not a restore failure; it is a harness failure. It also means lease-expiry integration was **NOT verified**.

## External storage

**BLOCKED:** VPS inspection found no configured `rclone`, `restic`, AWS CLI or `s5cmd`, and no external backup backend/credentials. Backups currently live on the same VPS/storage failure domain. To unblock: owner supplies an external storage account/bucket through the approved secret provisioning channel, including endpoint/region and access policy; then implement encrypted upload with least-privilege credentials, verify remote checksum, and rehearse restore from the remote object into a new isolated PostgreSQL volume. No secret should be sent in chat or committed.

**Production DR conclusion:** local restore is verified, but independent custody is absent; production disaster recovery is not ready.
