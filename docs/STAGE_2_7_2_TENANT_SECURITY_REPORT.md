# Stage 2.7.2 — Tenant Security Matrix

**Result: PASS for tested public API matrix; not a complete security certification.**
**Environment:** public staging, synthetic tenants only; current public manifest SHA-256 `06905a2917650b8c4de2fb754430d871623a0feaa5aec1ba41ca27a5814765e6`.

## Matrix results

The public runner registered three synthetic owner accounts through the protected site/API, used Mailpit for verification, created three separate organizations, and then attempted cross-tenant reads and writes using real authenticated sessions.

| Boundary | Result |
|---|---|
| Tenant profile update / industry profile | **PASS** — foreign tenant update rejected; invalid `grocery` profile rejected. |
| Product / inventory | **PASS** — foreign tenant product access denied. |
| Orders | **PASS** — foreign tenant order access denied. |
| Agent tasks | **PASS** — foreign tenant task read denied. |
| Agent-team delegation | **PASS** — cross-tenant delegation rejected. |
| Audit log | **PASS** — foreign tenant audit access denied. |
| Human Approval ownership | **PASS, scoped** — owner approval action constrained; replay/invalid scope/hash cases rejected; explicit owner rejection recorded. |
| CSRF | **PASS** — missing/invalid CSRF denied in public runner; previous Chromium test observed HTTP 403. |
| Session expiry | **PASS, scoped** — one synthetic session expired server-side and subsequent use rejected. |
| Idempotency | **PASS, generic task** — duplicate task submission with identical key did not create a second task; no domain reservation/booking side effect was tested. |
| RLS catalog state | **PASS, inherited/captured** — deployed schema 29 with 62 FORCE RLS relations; current staging DB volume retained. |
| Three-way concurrent load / interleavings | **NOT RUN** — matrix was sequential; no stress/concurrency proof. |
| Full direct DB role/permission probe | **NOT RUN in this stage** — prior security report evidence remains available, but no new SQL role-bypass probe was executed here. |

Public runner command: `python3 .local/stage-2-7-1/public-agent-acceptance.py`, **exit 0**. Sanitized transcript is `.local/stage-2-7-2/public-acceptance.log` with restrictive local permissions. It reported successful denial across products, orders, tasks, audit and delegation, then seven task completions after worker restart. No credentials or real customer data are included in the report.

## Runtime controls preserved

- Basic Auth remains enabled; the public edge challenge was verified in the runner and authenticated HTTPS success was recorded during Stage 2.7.1.
- TLS, CSRF, PostgreSQL RLS and tenant policy were not disabled or weakened.
- Telegram, WhatsApp and Instagram remain offline/mock only; no live sends or public webhook opening occurred.
- No database volume was deleted, no migration was run, and no public deployment was performed in this stage.
- The failed isolated test-HBA override was applied only to the temporary restore project, failed closed due to file permission denial, and was removed. The restore PostgreSQL service returned healthy. It did not touch public staging.

## Security disposition

No cross-tenant data exposure was observed in the tested API matrix. This is a scoped PASS, not a guarantee against untested routes, direct database access, concurrent race conditions or multi-agent context edge cases. Keep Stage 2.7.2 OPEN pending independent direct-RLS assertions and domain-workflow permission checks.
