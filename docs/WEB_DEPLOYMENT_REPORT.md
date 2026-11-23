# Web deployment report

## Current public runtime · 23 November 2026 UTC

- **LIVE, UPDATED.** Commit `7a84b6a10d385edf68366feaab60f8f6a8101f5b`, manifest SHA-256 `949977f4997aab73e3e4b18e6a801e31510cab035d032d6613cdb9ba56a171d7`.
- **PASS.** All six containers match the manifest. Schema 31, 62 FORCE RLS tables and active volume `aba-private-restore-cutover-20261123004447_database`. Zero queued backlog in final monitoring.
- **PASS.** Public Chromium regression and mandatory-part Auto Service acceptance. The application login guards dashboard routes. The direct API retains its edge protection.
- **PASS.** Fresh backup SHA-256 `f1a24e4eb929dd3fd2feed6d3d0a5d82baa60aa9932170dcca32f29eabfa5b43`, isolated restore and prior web startup. Image-only cutover used the active-volume override.
- D2 **NOT PERFORMED**. The owner explicitly approved this manifest and separately accepted the missing independent verification.

[Acceptance, limitations and evidence](POST_LAUNCH_IMPROVEMENTS_REPORT.md)

## Historical runtime · before launch promotion

## Current public runtime

- VPS: `2.26.1.63`, Ubuntu 24.04.4.
- Website/API: `https://ai-business-store.com` / `https://api.ai-business-store.com`.
- Manifest SHA-256: `06905a2917650b8c4de2fb754430d871623a0feaa5aec1ba41ca27a5814765e6`.
- Source: `89af012b3d38720796dc8e5feeadb6ab37f468a9` (`stage271-89af012-v3`).
- Services: `api`, `web`, `worker`, `proxy`, `postgres`, `mailpit`; fresh key-based SSH read-only inventory confirmed all six healthy. Separate `aba-stage273-isolated-test` and `aba-private-restore-stage272` containers were also present and healthy; they were not modified.
- Health: public website and API live/ready/status checks returned 200 when authenticated; Basic Auth challenge returned 401 without credentials.
- `APP_ENV=staging`; LLM provider and Telegram transport are mock; Instagram live delivery false. WhatsApp/Instagram live sends were not enabled.
- PostgreSQL 18.7, schema 29, 62 FORCE RLS tables. Persistent `aba-private-staging_database` remains mounted; no migrations executed.
- Nginx syntax test passed. UFW active with only 22/tcp, 80/tcp and 443/tcp inbound. HTTPS certificate verification passed.
- `aba-backup.timer` enabled and local backup files exist. External backup storage is not configured.

## Deployment action this task

No application deployment, migration, Docker image rebuild, DNS edit, or Basic Auth change was performed. The working tree contains uncommitted Stage 2.7.4 Auto Service changes and the locally tested password-reset migration/API/UI, plus tests/docs; none were deployed. Migrations 00030 and 00031 are not applied to staging. D2 provenance remains blocked because the available D2 proposal does not match current frontend dependency locks and no candidate build attestation exists. No volume or backup was removed.

## Rollback

There is no release to roll back from this task. Runtime remains at the verified manifest above. If a later release passes D2, take a fresh DB backup, retain the current manifest/package as rollback point, verify schema compatibility, deploy by immutable manifest, smoke-test, and revert to the previous manifest if health or user-flow checks fail. Do not apply candidate migration 00030 until the release gate is cleared.

## Public access decision

Basic Auth is intentionally still enabled. Although app-level email/password auth, secure sessions, CSRF, rate limits and tenant RLS exist, public delivery is not ready: external email is absent, password recovery is implemented only in an uncommitted candidate, and D2 does not certify a candidate. Opening the edge now would leave new users unable to verify or recover an account and would expose an unreleased build.
