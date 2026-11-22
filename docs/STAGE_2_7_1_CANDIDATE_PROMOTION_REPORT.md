# Stage 2.7.1 — Candidate Promotion Report

**Assessment date:** 2026-11-22 UTC
**Scope:** public staging only (`2.26.1.63`, `ai-business-store.com`, `api.ai-business-store.com`)
**Outcome:** **OPEN — candidate promoted; full acceptance incomplete.**

## Release identity

| Item | Verified value |
|---|---|
| Working branch | `work/stage-2-7-1-candidate-promotion` |
| Candidate source commit | `89af012b3d38720796dc8e5feeadb6ab37f468a9` |
| Candidate manifest | `/srv/aba/stage-2-7-1/candidate-v5-package-v3/manifest.json` |
| Candidate manifest SHA-256 | `06905a2917650b8c4de2fb754430d871623a0feaa5aec1ba41ca27a5814765e6` |
| Previous approved manifest SHA-256 | `f5aa3539842be7b991d13e0e7bd6363a9685541ca2dec91c98679274435ad038` |
| Database volume | Existing `aba-private-staging_database`; retained |
| Schema / safety counts after promotion | `29:62:9:20:104:4` (schema, FORCE RLS tables, organizations, tasks, audit rows, approvals) |

The candidate was built from a clean `git archive` of the exact candidate commit. The web production build, lint, typecheck, OpenAPI checks, package admission and exact image hashes were verified before promotion. Candidate API and web images match the public container image IDs. PostgreSQL was not recreated and no downward migration ran.

## Promotion history and incidents

| Attempt | Result | Evidence / disposition |
|---|---|---|
| First update GO document | **FAIL, exit 1; no state change** | Malformed JSON was rejected at `owner_go`; `state_change_started=false`. |
| Candidate package v4 | **FAIL, exit 1; rollback PASS** | Migration runner could not execute a packaged Go binary because its executable mode was lost. Public update stopped; the previous manifest was restored. Post-rollback diagnosis passed and database volume/schema/data counts were unchanged. |
| Candidate package v5 isolated rehearsal | **PASS, exit 0** | Added packaging guard and regression test requiring executable Go binaries; isolated migration and restored-volume validation passed. |
| Candidate v5 public promotion | **PASS, exit 0** | All release gates through database safety and HTTPS/Auth smoke passed. `update-v5.json` records the run. |
| Post-deploy diagnosis | **PASS, exit 0** | `postdeploy-diagnose.json`; candidate API/web images, existing database and schema safety checks verified. |
| Six-container restart | **PASS** | Compose restart completed; after 19 seconds all six public services reported healthy. Authenticated public HTTPS checks returned 200. |

The initial package-mode defect and rollback are retained as FAIL/rollback evidence; the corrected candidate does not erase them. No existing volume was deleted. The isolated restore rehearsal containers were stopped after the rehearsal; their volumes were retained.

## Public checks

- **PASS:** DNS names resolve to the staging VPS; TLS verification result `0` for both domains.
- **PASS:** unauthenticated Basic Auth challenge is retained; authenticated `https://ai-business-store.com/login` and `https://api.ai-business-store.com/health/ready` returned HTTP 200.
- **PASS:** public deployment diagnostic; six containers (`proxy`, `web`, `api`, `worker`, `postgres`, `mailpit`) healthy after restart.
- **PASS:** public Chromium login, private route, Secure/HttpOnly/Lax session cookie, reload, organization creation, industry profile save through BFF, API persistence, reload, and 390 px overflow check.
- **PASS:** public UI agent task persisted and rendered; CSRF omission rejected (403), approval accepted, duplicate approval rejected (409), audit event present; logout and private-route redirect.
- **PASS:** public API durable tasks for all seven agents and Business Manager task; the Business Manager result contained `analytics: completed` and `marketing: completed`; agent audit was present. See the dedicated report for scope limitations.
- **NOT RUN:** complete public industry chains A–D, public synthetic webhook signing/replay paths, three-tenant product/order/task negative matrix on this exact candidate, and public partial-failure aggregation for Business Manager.

Chromium runs: first attempt **FAIL, exit 1** because the driver created a non-test organization and then expected the test-only AI Team route. The corrected retry used a synthetic test organization and passed the listed UI checks. This was a test-driver error, not evidence of a product failure.

## Rollback and backup

- **PASS:** rollback to the previous approved manifest after the v4 packaging failure; post-rollback database counts and volume identity were unchanged.
- **PASS:** promotion backup `/srv/aba/staging/backups/before-update-20261122T170035622436.dump`, SHA-256 `77d74b8d5003a613f720e0dc2f62c261a699028df7e5de0a6a7d1f4b9e9cebab`; checksum file matched.
- **PASS:** backup restore rehearsal to a separate fresh volume before promotion; database fingerprints matched. No existing staging volume was used as the restore target.
- **PASS:** `aba-backup.timer` enabled and waiting for daily 02:30 UTC; `aba-staging-monitor.timer` enabled at five-minute cadence.
- **BLOCKED:** independent external backup custody is not configured. Do not treat local backup/restore as production-grade disaster recovery.

## Acceptance state

| Criterion | Status |
|---|---|
| Candidate integrity and public promotion | **PASS** |
| Six public containers and HTTPS/Auth after restart | **PASS** |
| BFF industry selection and persistence | **PASS** |
| Seven agents and Business Manager aggregate on public staging | **PASS, scoped** |
| Full A–D public worker chains | **NOT RUN** |
| Full three-tenant security matrix on this manifest | **NOT RUN** |
| Backup and rollback | **PASS, local only** |
| External backup | **BLOCKED** |
| E01 | **OPEN** |
| Stage 2.7.1 closure | **NO — OPEN** |
| Production readiness | **NO** |

Public provider sends, real webhooks, live credentials and production systems remained disabled/out of scope. See `.local/stage-2-7-1/INDEX.md` for private evidence locations and sanitized command transcripts.
