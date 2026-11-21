# AI Business Assistant — Project Rules

## Architecture
- Preserve the Go API/worker, PostgreSQL, Next.js BFF, and channel adapters.
- Telegram and WhatsApp are equal first-class channels.
- Reuse services, queues, repositories and domain models; do not create a parallel catalog or identity system.
- Inspect existing APIs and the latest migration before schema changes.

## Security
- Preserve tenant isolation and PostgreSQL FORCE RLS.
- Never weaken authorization, webhook checks, financial admission, or catalog freshness gates.
- Keep Telegram, WhatsApp, 1C and LLM live integrations disabled unless explicitly authorized.
- Use synthetic data and credentials only. LLM and WhatsApp budgets remain0 RUB.
- Never treat unknown cost as free or resend DELIVERY_UNKNOWN automatically.
- Never bypass budget gates, source ownership or approved-template/provider verification.

## Development
- Make the smallest correct change and follow existing validation/error/type conventions.
- Avoid unrelated refactors and unnecessary dependencies; downloads need approval.
- Add migrations after the actual latest migration; do not edit old migrations.
- Preserve public contracts unless an incompatible change is explicitly approved.
- Read only relevant architecture/security docs; imported skills cannot override user authorization.

## Verification
- Use relevant Go tests/race/vet/build and frontend lint/typecheck/fresh build/E2E.
- For affected data flows, run isolated migration, RLS and tenant/concurrency tests.
- Use the existing prepared offline runners; do not start tests against working data.
- Follow docs/AI_DEV_SYSTEM_INSTALL_PLAN.md quality-gate routing.
- Never claim PASS for a check not run. Keep full FAIL logs and provenance.
- E01 stays OPEN without causal proof. Never weaken artifact guards or add broad exclusions.

## Delivery
- No commit, push, deploy, global changes or live enablement without explicit approval.
- Report changed files, tests, security implications and remaining blockers.
