# Stage 2.7.2 — Final Staging Acceptance

**Assessment:** 2026-11-22 UTC
**Decision:** **OPEN — staging is running; mandatory workflow/reliability criteria remain unverified.**
**Scope:** public staging only, VPS `2.26.1.63`; no production systems or live provider delivery.

## Release and current state

| Item | Result |
|---|---|
| Working branch | `work/stage-2-7-2-final-acceptance` |
| Branch base / current HEAD during work | `d732174bd80f7add61673815b2ffe9ad13b526f5` |
| Public application source | `89af012b3d38720796dc8e5feeadb6ab37f468a9` |
| Public manifest | `/srv/aba/stage-2-7-1/candidate-v5-package-v3/manifest.json` |
| Manifest SHA-256, rechecked | `06905a2917650b8c4de2fb754430d871623a0feaa5aec1ba41ca27a5814765e6` |
| Staging DB volume | `aba-private-staging_database`, retained |
| Schema / FORCE RLS / orgs / tasks / audit / approvals | `29 / 62 / 19 / 39 / 200 / 9` at latest captured matrix run |
| Public Compose services | `api`, `mailpit`, `postgres`, `proxy`, `web`, `worker` — all healthy on final inspection |

The named historical commits were present locally after `git fetch origin`; the acceptance branch is based on the Stage 2.7.1 report commit. No application code or public deployment manifest was changed in this stage. The pre-existing untracked `docs/GIT_BASELINE_RECOVERY_REPORT.md` was not modified or staged.

## Evidence and status

| Criterion | Status | Evidence / limitation |
|---|---|---|
| Manifest integrity and current public services | **PASS** | VPS SHA-256 exactly matches the approved value; six `aba-private-staging` services healthy. |
| Public HTTPS / Basic Auth / frontend / API | **PASS, inherited + current container check** | Stage 2.7.1 authenticated Chromium and HTTPS evidence; current public services healthy. Authenticated URLs previously returned 200: `https://ai-business-store.com/login`, `https://api.ai-business-store.com/health/ready`. Basic Auth remains in place. |
| Real public Chromium flows / BFF / industry profile / mobile 390 px | **PASS, Stage 2.7.1** | Details in `STAGE_2_7_1_CANDIDATE_PROMOTION_REPORT.md`; not repeated without code/config change. |
| Three-tenant API/data isolation | **PASS, scoped** | Stage 2.7.2 public synthetic matrix; cross-tenant product, order, tasks, audit, profile writes and team delegation denied; three independent test organizations. |
| Seven public agent tasks and durable completion | **PASS, scoped** | All seven task types completed through public API and worker, with durable audit. Worker was stopped while the tasks were queued and restarted; tasks completed afterward. This does not exercise stale-lease expiry. |
| Business Manager public success aggregation | **PASS, inherited/scoped** | Prior public run aggregated completed Analytics and Marketing children. |
| Business Manager public partial state | **PASS, narrowly scoped** | Analytics child completed and Marketing child was explicitly rejected by owner, producing a partial aggregate. This is an approval-rejection path, not specialist error or timeout. |
| Business Manager specialist failure / timeout / retry behavior | **NOT RUN publicly** | No controlled public specialist error or timeout was induced. |
| Public full industry workflows A–D | **NOT RUN** | Verified fitment, service availability/slot and current property fixtures require trusted setup not exposed by safe public test API. Do not represent mock/isolated fixtures as public execution. |
| Public lease expiry, fencing, duplicate business action check | **NOT RUN** | Public worker stop/recovery driver previously timed out before establishing an evidence-backed lease transition. |
| Isolated lease/delegation integration | **FAIL — test harness setup** | `TestDelegationIsolated` could not open fixture; restored DB HBA permits only `aba_stage`, not test DB `aba_test`. A test-only HBA override failed PostgreSQL file permissions and was rolled back; no integration assertion ran. |
| Backup and local restore | **PASS, local-only** | New archive verified and restored to separate PostgreSQL project/volume; selected database fingerprints matched exactly (details below). |
| External backup custody | **BLOCKED** | No external account/credential/backend configured. |
| E01 causal root cause | **BLOCKED / OPEN** | Historical build workspace writer remains unattributed; prior evidence and guard preserved. |
| Production readiness | **NO** | Live providers, external backup, A–D, BM failure/timeout and public lease expiry remain unaccepted. |

## Commands and results

Commands with direct outcomes in this stage:

- `ssh -S none -o BatchMode=yes -o ConnectTimeout=8 root@2.26.1.63 '...docker ps...'` — **exit 0**; SSH worked; six public services and separate restore/test containers enumerated.
- `sha256sum /srv/aba/stage-2-7-1/candidate-v5-package-v3/manifest.json` on VPS — **exit 0**, digest matched exactly.
- Private public matrix runner: `python3 .local/stage-2-7-1/public-agent-acceptance.py` — **exit 0**. Sanitized transcript: `.local/stage-2-7-2/public-acceptance.log` (mode restricted, private).
- `npm run lint && npm run typecheck && npm run check:openapi` in `apps/web` — **exit 0**.
- `GOPROXY=off GOTOOLCHAIN=auto go test ./...` in `apps/api` — **exit 0**.
- `GOPROXY=off GOTOOLCHAIN=auto go test -race ./...` in `apps/api` — **exit 0**.
- `GOPROXY=off GOTOOLCHAIN=auto go vet ./... && GOPROXY=off GOTOOLCHAIN=auto go build ./...` in `apps/api` — **exit 0**.
- Focused restored-DB integration attempt, `TestDelegationIsolated` — **FAIL, exit non-zero before fixture**, PostgreSQL rejected connection (SQLSTATE `28000`); no test assertions executed.
- Isolated restore Postgres recovery after failed HBA override — base Compose `stop` then `up -d --no-deps --wait postgres` — **exit 0**, healthy; `pg_isready` accepted connections. Staging project was not restarted.

The web/browser flow was not re-run in this stage because no frontend source or configuration changed after the candidate's public Chromium PASS. Current runtime availability is corroborated by the six healthy public containers and prior authenticated endpoint evidence. This is not a new Chromium execution claim.

## Backup/restore evidence

Current-stage backup `/srv/aba/staging/backups/stage272-20261122T175412Z.dump`, size `890683` bytes, mode `0600`, SHA-256 `8713742c1b71434f24ab4619ec7096b38d8ff288519aa990e31b1079dbe55ec9`. Candidate backup utility verified archive and table of contents. Restore rehearsal completed into the separate project `aba-private-restore-stage272` and distinct volume `aba-private-restore-stage272_database`; source and restore fingerprints matched exactly: schema `29` / 30 migration records, 62 FORCE RLS relations, 19 organizations, 39 tasks, 200 audit rows and 9 approvals, including matching digests. No existing volume was removed. An initial backup invocation with the wrong environment failed closed before archive creation; the exact candidate environment retry passed. An initial restore networking overlap was corrected using a separate non-overlapping network. The temporary HBA test override produced a permission failure and was removed; original restore container health was recovered.

`aba-backup.timer` is enabled; daily 02:30 UTC schedule is inherited from the verified Stage 2.7.1 timer inspection. No timer was forced to run in this stage; the explicit snapshot above was run and restored. External backup remains blocked.

## Remaining mandatory work

1. Run public A–D workflows with approved synthetic trusted fixtures and capture parent/child task, business outputs, approvals and audit evidence.
2. Publicly induce BM child execution failure, timeout and retry without disabling safety gates; verify partial result semantics and no duplicate side effects.
3. Establish public lease expiry/recovery and idempotency evidence using a safe test hook or isolated worker-control procedure.
4. Repair the isolated integration fixture's test-database authentication setup without changing staging HBA; rerun only against the restored test DB.
5. Configure external backup custody through the provider's protected credential path and prove restore into a distinct volume.
6. Continue E01 host-level causal investigation; do not change its OPEN status absent writer attribution and reproducible regression evidence.

**Stage 2.7.2 disposition: OPEN.** No production deployment, live messaging, public webhook enablement, DNS change, volume deletion or application promotion was performed.
