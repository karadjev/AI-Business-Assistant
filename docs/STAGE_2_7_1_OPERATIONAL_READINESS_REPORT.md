# Stage 2.7.1 — Operational Readiness Report

**Result: STAGING RUNNING; operational acceptance PARTIAL. Production readiness: NO.**

## Current public deployment

- VPS: Ubuntu 24.04.4, 4 vCPU, 8 GiB RAM, 150 GB NVMe class; currently 6.9 GiB available memory and 126 GiB free disk after public restart (observation time 2026-11-22 UTC).
- Docker Compose project `aba-private-staging`; services `proxy`, `web`, `api`, `worker`, `postgres`, `mailpit` all healthy after a full six-service Compose restart.
- Images: API and worker use candidate image `sha256:c870889f25c69653d1a834c9fd92fc147e1ea7605cb0ac32755e3284e869e1d0`; web uses `sha256:31a68dfd46dac11f0898d83929837162a46768bb8e1e9d670cd9a5386d9dcdb6`. PostgreSQL, proxy and Mailpit remain the existing staging images.
- Existing persistent DB volume and schema 29 retained; 62 FORCE RLS relations reported in pre/post safety evidence. Promotion did not recreate PostgreSQL or run a destructive/down migration.
- Public HTTPS domains: `https://ai-business-store.com` and `https://api.ai-business-store.com`. TLS validation result 0; authenticated GETs returned HTTP 200; anonymous requests remained behind Basic Auth (401).
- UFW, SSH and Nginx configuration were not changed by promotion. External messaging/webhooks remain disabled/offline.

## Recovery and backups

- **PASS:** full Compose restart followed by all six healthy services and authenticated HTTP checks.
- **PASS:** rollback from the failed package v4 update restored the previous manifest; safety counts and existing volume identity were unchanged.
- **PASS:** pre-promotion compressed PostgreSQL archive SHA-256 `77d74b8d5003a613f720e0dc2f62c261a699028df7e5de0a6a7d1f4b9e9cebab`; pre-promotion isolated restore rehearsal passed on a separate volume.
- **PASS:** daily `aba-backup.timer` is enabled for 02:30 UTC; five-minute `aba-staging-monitor.timer` is enabled.
- **BLOCKED:** offsite/independent storage not configured. Backup is local to the VPS; production DR is not ready.
- **NOT RUN:** post-promotion dedicated restore rehearsal and a restore from external custody.
- A worker-only stop/recovery command timed out. The worker was subsequently observed healthy; this attempt is not evidence of lease-expiration recovery.

## Runtime checks

- **PASS:** real public Chromium login, session cookie and refresh, synthetic organization creation, BFF industry profile save/reload, AI Team data from the API, 390 px viewport, task result, Human Approval, CSRF rejection/replay rejection, Audit Log, logout and private-route redirect.
- **PASS:** public durable task execution for all seven agents and a Business Manager aggregate with two completed child tasks.
- **NOT RUN:** complete public A–D workflows; live channels/provider sends; controlled partial-failure aggregation; high-load concurrency and prolonged soak.
- **OPEN:** E01 remains open; no causal writer/root cause established and guards unchanged.

## Readiness

Candidate deployment and basic public operation are verified. Stage 2.7.1 remains **OPEN** until A–D, partial-failure coordination, stronger tenant/concurrency checks, worker lease recovery and independent backup are proven. The current staging must not be described as production-ready.
