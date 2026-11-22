# Stage 2.7 — E2E Staging Acceptance

**Дата проверки:** 2026-11-22 UTC
**Объект:** только staging `2.26.1.63` (`ai-business-store.com`, `api.ai-business-store.com`)
**Результат этапа:** **OPEN — частичная приёмка; production readiness не подтверждена.**

## Release identity and scope

- Git working branch: `work/stage-2-7-e2e-acceptance`, based on `70b2162029ee21edea1ac9275281e9eeab255b1d` (`work/staging-deployment`).
- Deployed application source: `ab84b42e8939d52b55cdafe8ef8a8638e834a292`.
- Deployed manifest SHA-256: `f5aa3539842be7b991d13e0e7bd6363a9685541ca2dec91c98679274435ad038` (rechecked against `/srv/aba/verify-20261122/release-clean/manifest.json`).
- No production database, live messaging credentials, external webhook, DNS record, or real customer data was used or changed. Public webhooks remain closed. Basic Auth was retained.
- Remediations in this branch were tested in a fresh isolated QA stack on the VPS; they were **not deployed** to the approved public staging release during this acceptance.

## Acceptance summary

| Gate | Result | Evidence / limit |
|---|---|---|
| G0 infrastructure, HTTPS, reboot | **PASS** | Ubuntu 24.04.4; Docker/Compose 29.10.0/5.8.0; Nginx; UFW 22/80/443; DNS; valid TLS; unauthenticated public requests receive Basic Auth 401. Whole VPS reboot returned all six containers healthy without manual Compose start. Persistent volume and row counts matched. |
| G1 browser/UI | **PASS, scoped** | Chromium against public staging: login, private-route redirect, secure session cookie, page reload, organization creation, seven-agent registry, real SQL-backed task/result, approval/replay, audit, CSRF denial, logout and 390 px viewport. Separate isolated candidate build: 5/5 Chromium smoke/auth tests. |
| G2 seven agents | **PASS for task execution; partial coordination evidence** | Public staging harness queued seven durable tasks with worker stopped; all seven (`customer`, `sales`, `product_inventory`, `marketing`, `finance`, `analytics`, `business_manager`) completed after worker restart, each with audit. Separate QA proves delegation. The staging harness stopped on a JSON field casing mismatch while reading Business Manager child outcomes; full aggregate contents on the public deployment are therefore **NOT VERIFIED**. |
| G3 chains A–E | **PARTIAL / NOT COMPLETE on public deployment** | Isolated VPS Go integration QA passed conversational/channel orchestration, auto parts, construction, service closure, real-estate, delegation and worker recovery tests. Do not interpret these test fixtures as completed end-to-end industry chains on the running public deployment. |
| G4 industries | **PARTIAL PASS** | Public Go API accepted all nine configured profiles and capability mappings; rejected `grocery`; cross-tenant profile updates were denied. Typed search, inventory, and complete industry chains were not run for every profile on public staging. Isolated focused construction/service profile integration initially failed (recorded) and passed after the test driver was made state-aware. |
| G5 channels | **PASS for offline/synthetic paths; live delivery NOT RUN** | Isolated VPS signed-event/offline integration tests passed for Telegram, WhatsApp and Instagram paths; Instagram `sending_enabled=false` remained enforced through the normal worker. Public webhook delivery remains blocked. Live provider credentials/delivery are not configured or tested. |
| G6 queue/recovery | **PASS for exercised paths** | Seven staging tasks persisted while worker was stopped and completed after restart; isolated Go tests passed retry/lease, idempotency, delegation and worker-process recovery. Cold VPS reboot also restored worker. High-volume concurrency/load was **NOT RUN**. |
| G7 backup/restore | **PASS locally; offsite BLOCKED** | Active daily timer is set for 02:30 UTC. `aba-backup.service` completed with exit 0; archive mode 0600, 870,836 bytes, SHA-256 recorded in private evidence. Restore to a distinct PostgreSQL volume passed and matched data fingerprints, schema, 62 FORCE RLS tables, seven agents, tasks, audit, tenant rows and approvals. No independent external backup storage exists. |
| G8 security | **PASS for exercised assertions; residual review open** | Dedicated integration DB: 29 migrations, 62 FORCE RLS tables/267 policies, restricted app role and cross-scope probes. Public API harness checked three synthetic tenants, product/order/task/audit/profile isolation, CSRF, cookies, approvals/replay and invalid profile. Full adversarial security assessment was **NOT RUN**. |
| G9 E01 | **OPEN** | Artifact manifest and runtime binary hashes verified; four existing integrity/smoke guard tests pass. Historical origin of unexpected host-backed artifact mutation remains unknown; E01 is not closed. |

## Infrastructure and reboot evidence

- Read-only preflight reported Ubuntu 24.04.4 LTS, 4 vCPU, 7.8 GiB RAM, 148 GiB filesystem with 128 GiB free after reboot (9% used), Docker daemon and Compose plugin active.
- Host UFW is active, default inbound deny, permits only 22/tcp, 80/tcp and 443/tcp (v4/v6). Nginx configuration test passed and service is active.
- DNS authoritative checks returned `2.26.1.63` for both domain names. The first preflight had a timeout querying one nameserver; the retry against both authoritative servers passed. Preserve this transient FAIL in `.local` evidence.
- Before/after reboot, `aba-private-staging_database` retained creation timestamp `2026-11-22T11:33:51Z`; counts matched: 9 organizations, 20 tasks, 104 audit entries, 4 approvals. All six project containers reached `healthy` under their existing `unless-stopped` restart policy without `compose up`.
- From an external client after reboot, both HTTPS hosts returned 401 without Basic Auth and certificate verification result 0. API/PostgreSQL are not publicly exposed; Compose reports PostgreSQL 5432 only inside the private network.
- Current host resource observation after reboot: ~782 MiB RAM used of 7.8 GiB; filesystem 9% used. This is a point-in-time reading, not load/capacity testing.

## Browser and public API evidence

Chromium exercised the actual staging frontend/API with HTTPS and Basic Auth. The UI showed data created and returned by the deployed backend, not frontend fixtures. Session refresh and logout worked. A synthetic agent task was persisted, completed, displayed, approved and audited; CSRF without token was rejected and a replayed approval returned conflict. Mobile viewport was 390 px.

The actual staging API harness verified three synthetic tenants, seven enabled registry entries, product/order/task/audit isolation, all nine profile/model configurations and invalid-profile rejection. It queued seven specialist tasks with the real worker stopped and observed them complete after worker recovery. Its first attempt had an invalid delegation payload (400 `INVALID_JSON`); the retry corrected the payload and passed through task completion. It then stopped at decoding Business Manager children as lowercase `children`; the Go result field serializes as `Children`. The isolated candidate/browser suite passed after the harness and code fixes, but the full public Business Manager child aggregation assertion remains open.

An authenticated public BFF `PATCH /agent-business-profile` returned 404 while the same Go API route worked. A narrow BFF method allowlist fix and an owner-only industry selector with optimistic revision/CSRF handling were implemented and tested in the isolated candidate build. Public staging still runs the approved pre-fix artifact until a new candidate is independently approved and deployed.

## Commands and exit codes

Commands were run from clean committed source on the VPS or from the repository worktree. Private raw logs and helper scripts are indexed in `.local/stage-2-7/INDEX.md`.

- `python3 scripts/deployment/deploy.py diagnose --release /srv/aba/verify-20261122/release-clean --env /srv/aba/staging/environment --project aba-private-staging --storage-root /srv/aba/staging --backup-dir /srv/aba/staging/backups --tls-ca /srv/aba/staging/secrets/db_ca --report /srv/aba/staging/reports/restore-stage27/diagnose.json` — **PASS, exit 0** against approved manifest before reboot.
- DNS authoritative A checks for `ai-business-store.com` and `api.ai-business-store.com` — final retry **PASS, exit 0**; initial nameserver timeout retained.
- Public Chromium acceptance — **PASS, exit 0**; actual HTTPS staging flow.
- Isolated integration runners: DB/migrations, conversational routing, Instagram offline, delegation, service closure, real estate — **PASS, exit 0** each.
- Industry focused suite baseline — **FAIL, exit 1** at `service_only` because a fixed six-call helper stopped before the parent result; construction passed. State-aware bounded loop remediation — **PASS, exit 0** on fresh isolated DB.
- Candidate `/scripts/build-env/production.sh`, `npm run lint`, `npm run typecheck`, `npm run check:openapi`, artifact proof, then `node /scripts/build-env/browser.sh tests/smoke.spec.ts tests/auth.spec.ts --output=/results/run --reporter=list` — **PASS, exit 0**; Chromium 5/5.
- `node --test scripts/build-env/artifacts.test.cjs scripts/deployment/smoke.test.cjs` — **4/4 PASS, exit 0**.
- `systemctl start aba-backup.service` — **PASS, exit 0**; newest archive `20261122T154338-dcd8fb59949d4e8b96d63468d6d28ccf.dump`, 870,836 bytes, SHA-256 `0ae09a6de4781e59f2e083c3eb0dc61728d91119cf7deafa5162ef7264d32957`. `python3 scripts/deployment/backup.py restore-rehearsal --release /srv/aba/verify-20261122/release-clean --env /srv/aba/staging/reports/restore-stage27/restore.env --project aba-private-restore-stage27 --archive /srv/aba/staging/backups/20261122T154338-dcd8fb59949d4e8b96d63468d6d28ccf.dump` — **PASS, exit 0**.
- Full server reboot — **PASS**: scheduled with `systemd-run --unit=aba-stage27-reboot --on-active=3s /usr/bin/systemctl reboot`; boot ID changed, no manual service start, six healthy containers, data counts and persistent-volume identity preserved.
- `bash /srv/aba/stage-2-7/industries-fixed.sh industries` — **PASS, exit 0**; baseline run was **FAIL, exit 1**. `go test -tags=integration -count=1 -v -run '^TestDatabaseAndMigrations$' ./tests/integration`, `... -run '^TestConversationalIsolated$' ...`, and the focused offline channel/delegation/closure/estate tests — each **PASS, exit 0** in the pinned Go container with a fresh temporary PostgreSQL. `npm run lint && npm run typecheck` — **PASS, exit 0**. `node --test scripts/build-env/artifacts.test.cjs scripts/deployment/smoke.test.cjs` — **4/4 PASS, exit 0**. The first public API retry contained wrong JSON (400); subsequent corrected attempt completed all seven agents but did not finish its Business Manager aggregate assertion. A later runner retry encountered intermittent client TLS/mail harness errors and is not counted as acceptance evidence.

## Delivery state

- **CODE COMPLETE:** narrow remediation is implemented locally and candidate build is verified.
- **DEPLOYED:** existing approved release only; Stage 2.7 remediation not deployed.
- **RUNTIME VERIFIED:** existing staging frontend/API/workers and restart behavior; candidate behavior verified in an isolated VPS stack.
- **SECURITY VERIFIED:** stated assertions passed; not a full penetration test.
- **BACKUP VERIFIED:** local restore passed; independent/offsite custody blocked.
- **LIVE INTEGRATIONS VERIFIED:** **NO**; only offline/synthetic adapters were exercised.
- **PRODUCTION READY:** **NO**. Stage 2.7 remains open.

## Remaining blockers

1. Approve a new immutable release manifest before deploying the verified BFF/UI and integration-test fixes to public staging; then rerun Chromium, API and worker acceptance against that exact deployment.
2. Complete Business Manager child-outcome assertion and full public staging inter-agent chains A–E.
3. Run full typed-search and domain workflows per supported industry on public staging.
4. Keep all provider sending disabled; separately approve synthetic external-provider sandbox credentials if required. No live customer messages were sent.
5. Add independent remote backup custody and test remote restore before production.
6. Investigate E01 root cause without weakening guards or changing historical evidence.
7. Run load/concurrency and broader adversarial security review before any production decision.
