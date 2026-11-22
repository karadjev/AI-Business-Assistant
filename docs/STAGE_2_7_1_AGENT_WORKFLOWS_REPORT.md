# Stage 2.7.1 — Agent Workflows Report

**Scope:** deployed public candidate `89af012b3d38720796dc8e5feeadb6ab37f468a9`; synthetic-only execution.
**Outcome:** partial acceptance; A–D remain open.

## Evidence on public staging

- **PASS:** Agent Registry returned seven enabled agents with permissions to the authenticated owner.
- **PASS:** durable SQL tasks were created for Customer, Sales, Product & Inventory, Marketing, Finance, Analytics and Business Manager; worker completed each and audit records were queryable.
- **PASS:** task creation with the same idempotency key returned the same ID.
- **PASS:** Business Manager result persisted a plan and two child outcomes: Analytics and Marketing, both `completed`.
- **PASS:** marketing draft was blocked on Human Approval until owner action; CSRF omission was rejected and approval replay returned conflict.
- **PASS:** a real synthetic SKU/variant, inventory adjustment, publish action and conversation were created through the public staging API for domain-task context.

## Industry scenarios

| Scenario | Status | Evidence |
|---|---|---|
| A — Auto Parts: fitment, current price/stock, clarification, proposal | **NOT RUN** | No complete public worker delegation chain executed on this candidate. |
| B — Construction Materials: quantity, package rounding, stock/price, offer | **NOT RUN** | No complete public worker delegation chain executed on this candidate. |
| C — Auto Service: part, slot, staff approval, reservation, response | **NOT RUN** | No complete public worker delegation chain executed on this candidate. |
| D — Real Estate: typed search, freshness, offer, viewing request/confirmation | **NOT RUN** | No complete public worker delegation chain executed on this candidate. |
| E — Business Manager: multi-agent task aggregation | **PASS, scoped** | Real public task completed with Analytics and Marketing child tasks; both child statuses persisted as completed. |
| E — Business Manager partial/error result | **NOT RUN** | No controlled failing specialist was injected in this public test. |

Previously recorded isolated VPS integration suites are historical evidence only; they are not substituted for public staging A–D acceptance. Channel adapters remain offline/mock; no real send or open webhook was used.

## Remaining workflow checks

- Run A–D with synthetic typed catalog/service/property records through public API, persisted SQL delegation and deployed worker.
- Exercise a controlled specialist error and verify Business Manager labels unavailable facts rather than inventing them.
- Confirm no duplicate reservations/orders under repeated events and concurrent synthetic tenants.
- Re-run worker lease/termination recovery as a separately observed public test. A worker control attempt timed out; no PASS is claimed for that attempt. The subsequent successful all-container restart proves cold service restart only, not lease expiry while a worker is down.
