# Stage 2.7 — Operational Readiness

**Дата:** 2026-11-22 UTC · **Оценка:** staging operating controls verified; production readiness NO-GO.

## Inventory

- VPS `2.26.1.63`, Ubuntu 24.04.4 LTS, 4 vCPU, 7.8 GiB RAM, 148 GiB root filesystem.
- Docker Engine 29.10.0; Compose 5.8.0; Nginx and UFW active.
- Public names: `ai-business-store.com`, `api.ai-business-store.com`; authoritative A checks resolve to `2.26.1.63`.
- Project `aba-private-staging`: proxy, web, API, PostgreSQL, worker, Mailpit. Six containers all healthy after cold VPS reboot. PostgreSQL 18.7; schema 29; app database remains internal-only.
- All six production-like staging containers use `unless-stopped`; the reboot test observed autonomous recovery with no manual `compose up`.
- Post-reboot observation: 9% disk used, approximately 782 MiB RAM used, all containers healthy. This does not establish peak capacity or concurrency limits.

## Operations evidence

| Check | Result |
|---|---|
| DNS / HTTPS | **PASS**; both public hosts TLS-verified from an external client; no Basic Auth gives 401. |
| HTTP/UI/API | **PASS, scoped**; Chromium checked login, private routes, reload, organization, task/result, approval, audit, CSRF, logout and 390 px viewport. |
| Worker restart | **PASS**; seven durable tasks were queued with worker stopped and completed after worker restart. Process-recovery integration tests passed in an isolated QA database. |
| Full server reboot | **PASS**; boot ID changed; six containers healthy without manual start; persistent database volume creation identity and counts (9 orgs, 20 tasks, 104 audit entries, 4 approvals) were preserved. |
| Backup schedule | **PASS configured**; systemd timer active, `OnCalendar=*-*-* 02:30:00 UTC`, next run `2026-11-23 02:30 UTC`. Manual service invocation completed exit 0. The next natural scheduled invocation has not yet occurred during this evidence window. |
| Restore | **PASS local**; new restore project/volume, schema/RLS/role/data assertions and fingerprints matched. No existing volume was removed. |
| Monitoring | **PASS configured**; `aba-staging-monitor.timer` active and latest service result exit 0. Queue, resource and container monitoring are lightweight host scripts; no external alert delivery was tested. |
| Log rotation / capacity | **PARTIAL**; Docker logging policy was present in deployment diagnosis. Current point-in-time disk/RAM are healthy. Long-term log growth and high-load behavior **NOT RUN**. |
| Offsite backup | **BLOCKED**; no independent remote destination/custody is configured. |

## Recovery procedure verified

1. Keep Compose source, protected environment, DB CA and backup key on the VPS with existing owner-only permissions.
2. Select a verified archive from `/srv/aba/staging/backups`; do not modify the active project volume.
3. Restore using the repository `scripts/deployment/backup.py restore-rehearsal` flow into a new, uniquely named Compose project with its own private subnet and empty database volume.
4. Compare schema version, FORCE RLS/policies, app-role restrictions, organization count, task/audit/approval fingerprints and anonymous-RLS probe.
5. Preserve rehearsal evidence and leave the active restore volume intact for review. Do not delete backup or volume as part of this procedure.

The Stage 2.7 rehearsal used an isolated project `aba-private-restore-stage27`; restore evidence reported schema 29, 62 FORCE RLS tables, restricted `aba_app`, 9 organizations, 19 completed tasks, 7 completed agent IDs, 104 audit entries and 3 executed approvals. Restore fingerprints matched the pre-restore snapshot. Main staging then rebooted and preserved its independent active volume.

## Readiness states

- **CODE COMPLETE:** isolated Stage 2.7 UI/BFF and test-driver remediations complete and validated.
- **DEPLOYED:** only the previously approved manifest; remediation not deployed.
- **RUNTIME VERIFIED:** existing staging stack and boot recovery; candidate runtime verified only in isolated test stack.
- **BACKUP VERIFIED:** local backup and isolated restore; offsite copy blocked.
- **LIVE INTEGRATIONS VERIFIED:** no. All provider tests were offline/synthetic.
- **PRODUCTION READY:** no. Business Manager aggregate evidence and public staging chains remain incomplete; E01 is open; external backup custody and production-specific security/load review remain.
