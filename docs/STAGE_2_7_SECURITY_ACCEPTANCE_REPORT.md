# Stage 2.7 — Security Acceptance

**Дата:** 2026-11-22 UTC · **Состояние:** PASS для перечисленных проверок; общий security sign-off остаётся OPEN.

## Scope and identity

Проверялся только staging `2.26.1.63`, опубликованный deployment `70b2162029ee21edea1ac9275281e9eeab255b1d`, application source `ab84b42e8939d52b55cdafe8ef8a8638e834a292`, approved manifest SHA-256 `f5aa3539842be7b991d13e0e7bd6363a9685541ca2dec91c98679274435ad038`. Использованы только синтетические tenants и сообщения. Production и реальные provider accounts не затрагивались.

## Results

| Control | Result | Evidence / limits |
|---|---|---|
| Public perimeter | **PASS** | UFW active, inbound default deny, only 22/80/443; Nginx active; TLS certificate verified externally (`ssl_verify_result=0`); both hosts return Basic Auth 401 unauthenticated. |
| PostgreSQL exposure | **PASS** | PostgreSQL is reachable through private Compose network only; container reports internal port 5432 without published host binding. |
| FORCE RLS | **PASS** | Fresh isolated migrated PostgreSQL: schema 29, 62 platform tables with FORCE RLS, 267 policies; readiness regression denies when RLS is disabled. Restored staging backup reproduced 62 FORCE RLS tables. |
| Database role | **PASS** | `aba_app` is non-superuser, no bypass-RLS/DDL/admin flags, no schema CREATE, and no membership in migrator role. |
| Tenant isolation | **PASS for exercised routes** | Three synthetic owners/organizations; cross-tenant product/order/task/audit and profile access/update probes rejected. Public Business Manager delegation attempt across tenant was denied; independent delegation integration test passed. Exhaustive table-by-table authorization review **NOT RUN**. |
| CSRF and cookies | **PASS** | Browser/API rejected mutation without CSRF; session cookie Secure/HttpOnly/SameSite and CSRF cookie Secure/SameSite checked; public session refresh and logout observed. Candidate regression also checks stale optimistic revision conflict. |
| Human Approval | **PASS for exercised cases** | Synthetic approval ownership/hash checks passed; approval replay rejected (HTTP conflict); restored DB contains executed approvals. Broader concurrent approval race test **NOT RUN**. |
| Webhook and replay | **PASS offline / live blocked** | Isolated Telegram/WhatsApp/Instagram signed synthetic paths, consent/recipient checks and idempotency were exercised. Public webhook paths remain inaccessible; no real provider webhook was enabled. |
| Instagram sending gate | **PASS offline** | Offline adapter and normal worker preserved `sending_enabled=false`, with refusal/audit path. No live Instagram send was attempted. |
| Secret handling | **PASS by execution design, limited scan** | Credentials were read from protected local files and not printed. No secret file, backup, runtime dump or `.local` artifact is part of the Git change. Full host filesystem secret scan **NOT RUN**. |
| Artifact integrity | **PASS for current artifact; E01 OPEN** | Rechecked exact approved manifest; API/worker binary hashes match it; running Next runtime/source proof matched the deployed proof. Existing guard tests: 4/4 PASS. Historical unexplained host-backed artifact mutation is unresolved; guards remain unchanged. |

## Remediation finding

The real frontend BFF rejected `PATCH /agent-business-profile` although the authenticated Go API route accepted it. A scoped BFF allowlist fix and owner-only profile selector were implemented and passed isolated Chromium/API tests (CSRF and stale revision included). They are not in the currently approved running artifact. The public staging configuration remains behind the release gate until the new immutable candidate is reviewed and approved.

## Status

- **SECURITY VERIFIED:** only the controls above, on staging and isolated synthetic fixtures.
- **FAIL preserved:** public BFF industry-profile update returned 404 in the original deployment; later candidate regression passes in isolation.
- **BLOCKED:** independent offsite backup storage; full adversarial security review; approved deployment of candidate fix.
- No TLS, CSRF, RLS, tenant, Instagram, Basic Auth or artifact guard was disabled or weakened.
