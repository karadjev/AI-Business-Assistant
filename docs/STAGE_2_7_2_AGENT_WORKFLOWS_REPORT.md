# Stage 2.7.2 — Agent Workflows

**Result: PARTIAL PASS; full industry acceptance not achieved.**
**Public source/manifest:** commit `89af012b3d38720796dc8e5feeadb6ab37f468a9`, manifest SHA-256 `06905a2917650b8c4de2fb754430d871623a0feaa5aec1ba41ca27a5814765e6`.

## Public staging evidence

The public synthetic matrix runner used the actual frontend/API edge protection and Go API against staging PostgreSQL/SQL queue/worker. It created three isolated synthetic organizations and exercised nine supported API industry profiles, permission/capability mappings, invalid-profile rejection, cross-tenant profile-write rejection, products, orders, conversations, tasks and audit access.

| Agent / behavior | Result | Evidence |
|---|---|---|
| Customer Agent | **PASS, scoped** | Real public durable task completed and audit persisted. |
| Sales Agent | **PASS, scoped** | Real public durable task completed and audit persisted. |
| Product & Inventory Agent | **PASS, scoped** | Real public durable task completed; synthetic catalog/inventory path also created. |
| Marketing Agent | **PASS, scoped** | Public task and owner-gated behavior exercised; in BM partial case the child was explicitly rejected. |
| Finance Agent | **PASS, scoped** | Real public durable task completed and audit persisted. |
| Analytics Agent | **PASS, scoped** | Real public durable task completed and audit persisted. |
| Business Manager | **PASS, scoped** | Public parent task completed with child results; one run had Analytics completed and Marketing explicitly rejected, persisted as partial. |
| Permissions / cross-tenant delegation | **PASS, scoped** | Cross-tenant team delegation rejected; all three tenants' task and audit surfaces denied foreign access. |
| Industry profiles / capability gates | **PASS, catalog only** | Nine API profiles exercised; invalid `grocery` rejected. Does not prove complete vertical workflows. |

The public runner also queued seven distinct agent tasks while the public worker was stopped, accepted an owner-scoped approval, rejected replay/hash/security violations, then restarted the worker; all persisted tasks completed with audit. The exact private transcript is `.local/stage-2-7-2/public-acceptance.log`; runner command `python3 .local/stage-2-7-1/public-agent-acceptance.py`, exit 0. The run used synthetic data; no live delivery was enabled.

## A–D end-to-end workflow acceptance

| Scenario | Required domain evidence | Status |
|---|---|---|
| A — Auto Parts | Trusted fitment compatibility, stock, current price, clarification and BM proposal | **NOT RUN publicly** |
| B — Construction Materials | Unit conversion, pack rounding, inventory/price and BM proposal | **NOT RUN publicly** |
| C — Auto Service | Service/work selection, compatible part, slot, Human Approval, reservation and BM response | **NOT RUN publicly** |
| D — Real Estate | Typed requirement search, listing freshness, viewing request/approval and BM response | **NOT RUN publicly** |

These are not treated as passed based on generic agent tasks, UI registry entries or isolated integration fixtures. Trusted setup for fitment, service availability and current property inventory was not safely available through the public test surface in this run. No mock result is presented as public execution.

## Tooling and regression tests

- Frontend lint, typecheck and OpenAPI validation: `npm run lint && npm run typecheck && npm run check:openapi` (`apps/web`) — **PASS, exit 0**.
- Backend unit suite: `GOPROXY=off GOTOOLCHAIN=auto go test ./...` (`apps/api`) — **PASS, exit 0**.
- Backend race suite: `GOPROXY=off GOTOOLCHAIN=auto go test -race ./...` — **PASS, exit 0**.
- Backend static/build suite: `GOPROXY=off GOTOOLCHAIN=auto go vet ./... && GOPROXY=off GOTOOLCHAIN=auto go build ./...` — **PASS, exit 0**.
- Focused delegation integration `TestDelegationIsolated` against separate restored PostgreSQL — **FAIL, harness prerequisite**. Fixture connection failed before assertions (SQLSTATE `28000`): restored HBA permits the application DB but not the test DB `aba_test`. Attempted read-only HBA bind override was unreadable to PostgreSQL container (permission denied), then removed; restore container was returned to healthy baseline. No public staging database or HBA was changed.

## Limitations

Agent task completion proves durable execution/audit for the tested generic tasks, not domain correctness, verified data provenance, every tool permission, or recovery after lease expiry. Public child execution error, timeout and retry are **NOT RUN**. See the Business Manager reliability report for the narrowly scoped approval-rejection result. Overall agent/workflow acceptance remains **OPEN** until A–D and failure-path checks pass publicly.
