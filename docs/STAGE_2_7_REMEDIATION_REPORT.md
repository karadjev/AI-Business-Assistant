# Stage 2.7 — Findings and Remediation

**Дата:** 2026-11-22 UTC · Changes prepared on `work/stage-2-7-e2e-acceptance`; no production or approved public staging artifact was modified.

## Findings

### R1 — Frontend could not update its business profile

- **Reproduction:** authenticated `PATCH /api/v1/organizations/{id}/agent-business-profile` through the Next.js BFF returned 404, while the Go API route was reachable and accepted valid profile revisions.
- **Impact:** users could not choose/update the industry from the AI Team page; configuration was only possible through the direct API.
- **Fix:** add only `PATCH` for the exact existing profile path to the BFF method allowlist. Add an owner-only selector for supported business profiles, preserving CSRF and optimistic revision checks; display API errors and refresh on conflict.
- **Regression:** existing browser auth test now verifies non-owner/CSRF rejection, successful profile update, reload persistence, and stale-revision 409. Fresh isolated production build and Chromium stack on the VPS passed (5/5 browser tests). `npm run lint` and `npm run typecheck` passed.
- **Deployment:** not deployed to the public staging release. It requires a new immutable candidate and release approval.

### R2 — Focused industry test depended on queue call count

- **Reproduction:** `TestStage262Focused/service_only` failed with no parent execution result after six `ProcessOne` calls. The construction subtest passed. Service booking/cancellation could enqueue additional legitimate work, so a fixed call count did not establish completion of the target parent.
- **Impact:** the test was timing/queue-order sensitive and yielded a false negative; it did not show a product runtime defect.
- **Fix:** poll the exact parent task and its latest execution result with a bounded 20-call limit. Fail on worker error, terminal failure, or noncompletion; assert the exact expected result state. No queue behavior, permissions, retry policy or production code was changed.
- **Regression:** the original failing log is retained (`g-industries.log`, exit 1). The patched test passed in fresh isolated PostgreSQL with construction and service-only subtests, exit 0.

### R3 — Acceptance harness mismatches (not application changes)

- Initial API harness sent invalid delegation JSON and received 400 `INVALID_JSON`; corrected synthetic request passed tenant-denial validation.
- The API serializes Business Manager result fields with uppercase `Children`; harness expected lowercase `children` and raised `KeyError` after seven actual staging agent tasks completed. The isolated candidate harness accepts either wire representation, but final public aggregate child assertion remains incomplete.
- A later public account harness retry had intermittent TLS handshake and Mailpit-output errors. It is recorded as **FAIL/NOT EVIDENCE** and is not counted as a product pass. Chromium against the real public staging account flow and isolated fresh-stack tests supply the separate browser evidence.

## Validation evidence

- `npm run lint` — PASS, exit 0.
- `npm run typecheck` — PASS, exit 0.
- Fresh isolated VPS candidate build: production build, lint, typecheck, OpenAPI and unchanged artifact guard — PASS, exit 0.
- Fresh isolated VPS Chromium auth/smoke — 5/5 PASS.
- Patched Go integration subtests for construction/service-only — PASS, exit 0; baseline FAIL retained.
- Artifact/smoke guard Node tests — 4/4 PASS.
- `git diff --check` — PASS.

## Change boundaries

No schema migration, permission expansion, mock bypass, message delivery, provider gate, secret, production configuration, existing volume, historical evidence or integrity guard was changed. No public deployment was performed. E01 remains OPEN.
