# Stage 2.7.1 — Business Manager Acceptance

**Outcome: PARTIAL PASS; Stage remains OPEN.** All data used below was synthetic and created in public staging.

## Public API execution

A real staging owner session created a synthetic test organization and catalog item, published a synthetic variant, set synthetic inventory, and opened a conversation. Public API tasks were submitted through `/api/v1/organizations/{id}/agent-tasks` and processed by the deployed SQL queue/worker.

| Agent | Public task result |
|---|---|
| Customer | **PASS — completed** |
| Sales | **PASS — completed** |
| Product & Inventory | **PASS — completed** |
| Marketing | **PASS — completed after approval** |
| Finance | **PASS — completed** |
| Analytics | **PASS — completed** |
| Business Manager | **PASS — completed** |

For the Business Manager task, persisted result included `Plan` and `Children`; child outcomes were `analytics: completed` and `marketing: completed`. Public task history showed 9 tasks across the test organization (including deliberate idempotent duplicate submissions) and all 9 completed. Audit events existed for each of the seven targeted agent tasks. Re-submitting an identical task request with the same idempotency key returned the same task ID.

A separate real Chromium flow created a marketing draft from the AI Team UI. The task remained approval-gated until an owner decision. Missing CSRF was rejected with HTTP 403, the scoped approval returned HTTP 204, and replay returned HTTP 409. The resulting completed task had `external_communication=false` and its approval action was in Audit Log.

## Limits / failures retained

- **NOT RUN:** deliberate specialist failure with the other delegated specialist succeeding; partial-result aggregation/error explanation is therefore not accepted.
- **NOT RUN:** proving that the Business Manager refuses a fabricated price, stock count, compatibility or other unverified business fact under a controlled changed-source/error scenario.
- **NOT RUN:** full A–D industry chains and multi-organization parallel tasks on this exact public candidate.
- **FAIL, harness only:** an earlier runner stopped after a JSON field casing mismatch; corrected inspection used the actual `Plan`/`Children` public response and proved the two child statuses above.
- **FAIL, harness only:** a later bulk API runner encountered transient TLS disconnects and a stale Mailpit command; it is not counted as product acceptance. Its synthetic tasks were reconciled through public API and DB status; the worker remained healthy.

Business Manager has a verified real public happy path coordinating two specialists. It is not accepted as a general-purpose coordinator until partial failure and stale/unknown source behavior are demonstrated. No external delivery was enabled.
