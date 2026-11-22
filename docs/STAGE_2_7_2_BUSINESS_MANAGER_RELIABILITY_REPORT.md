# Stage 2.7.2 — Business Manager Reliability

**Result: OPEN — public happy path passes; error and timeout acceptance is incomplete.**

## Public runtime

On the current public manifest (`06905a2917650b8c4de2fb754430d871623a0feaa5aec1ba41ca27a5814765e6`, application source `89af012b3d38720796dc8e5feeadb6ab37f468a9`), the synthetic public API runner submitted durable tasks while worker was stopped. Following worker restart, all queued tasks completed and audit records remained. The seven enabled agent roles were exercised. A Business Manager parent produced durable child records and an aggregate result.

Prior Stage 2.7.1 public happy-path evidence independently showed Analytics and Marketing children both completed. In the current three-tenant matrix, the Marketing child was explicitly rejected by the organization owner while Analytics completed; the parent persisted a partial aggregate and Business Manager task completed. This verifies approval rejection/partial state, not worker error handling.

## Failure matrix

| Case | Result | Interpretation |
|---|---|---|
| Multiple children complete | **PASS, scoped** | Public Stage 2.7.1 parent had Analytics and Marketing completed. |
| Child rejected through Human Approval | **PASS, scoped** | Public Stage 2.7.2 Analytics completed; Marketing rejected/cancelled; BM aggregate retained partial outcome. |
| Child specialist execution error while sibling succeeds | **NOT RUN publicly** | No controlled public specialist failure was induced. |
| Child timeout while sibling succeeds | **NOT RUN publicly** | No timeout was injected or observed in the public path. |
| Retry after child error/timeout | **NOT RUN publicly** | No bounded retry/retry exhaustion assertion. |
| No duplicate business side effect after retry | **NOT RUN** | Generic task idempotency was checked; this does not prove domain action idempotency. |
| Parent recovery after worker restart | **PASS, queued restart only** | Parent/children persisted; queued tasks completed after restart. Lease-expired running parent was not demonstrated publicly. |
| Partial result clearly labelled incomplete | **PASS, scoped** | Owner rejection yielded partial aggregate; not sufficient for unverified facts/error/timeout behavior. |

## Isolated integration attempt

An isolated test runner used source mounted read-only from the deployed candidate and a separate restored database volume, never the public staging database. Focused test `apps/api/tests/integration/stage261_delegation_test.go::TestDelegationIsolated` was intended to cover lease expiry/fencing plus delegation partial failure. The runner failed before test assertions because PostgreSQL rejected the `aba_test` fixture connection (SQLSTATE `28000`); restored HBA is scoped to `aba_stage`. A test-only HBA overlay was attempted, but PostgreSQL could not read the mounted config (`Permission denied`). The overlay was removed, isolated PostgreSQL returned to `healthy`, and the temporary Go test runner was stopped. Result: **FAIL (harness), no integration PASS**. The test's expected behavior must not be inferred from its source.

## Required follow-up

1. Provide a secure, test-only fixture database rule in the isolated restore environment with correct container UID/GID and file mode, validated before restart. Do not change public staging HBA.
2. Rerun the focused integration test and retain complete sanitized output and exit code.
3. Add/run public synthetic fault injection only through an existing supported test seam; do not alter permissions, approval gates or public delivery controls.
4. Demonstrate child failure, timeout, bounded retry, partial aggregate, audit, and no duplicate action on the public staging runtime.

Business Manager is not accepted as resilient to child execution failure or timeout. No real messages or production services were used.
