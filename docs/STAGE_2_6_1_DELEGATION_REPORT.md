# Stage 2.6.1 — Durable multi-agent delegation

[Final Integration baseline](STAGE_2_6_FINAL_INTEGRATION_REPORT.md) · [Closure baseline](STAGE_2_6_CLOSURE_REPORT.md) · [Baseline evidence](../.local/stage-2-6-final/INDEX.md)

## Architecture

Migration 00024 adds a bounded input projection on the parent, a unique ordinal for each child, and `updated_at` to the existing task table. Parent, children, correlation, priority, lease, attempts, execution results and audit use the existing schema and SQL queue. No tenant table was added. Previous migrations and all historical evidence are retained.

`POST /api/v1/organizations/{id}/agent-delegations/auto-parts` accepts `conversation_id`, `idempotency_key` and typed `requirements`. Admission is restricted to owners of isolated test organizations with an Auto Parts trade/hybrid profile and an active conversation. Identical replay returns the same task ID. A different payload with the same key returns 409. The listing API exposes parent and correlation IDs, priority, timestamps, error information and execution results.

The allowlisted plan selects agent and tool pairs, creates the child records in one transaction, and queues the first child. The existing worker claims each child with an independently fenced lease. Each step runs through the existing Agent Registry, Orchestrator, permission check and typed read-only Tool Registry. A successful completion queues the next child in the same transaction. A terminal child failure cancels remaining dependencies. The parent aggregates the persisted child outcomes, preserving completed results if the parent fails. The existing SQL queue records terminal failures and recovers expired leases with bounded retries.

## Auto Parts and permissions

The steps are Customer requirements, Sales search plan, Product & Inventory lookup, fitment verification, Sales proposal, and Business Manager aggregation. The proof takes an exact OEM reference and vehicle fields. Missing information returns explicit clarification fields. The parts projection is bound to the exact tenant, test business profile, current conversation, leased agent/action and recorded inputs. It returns active variants, stored prices, and stock only when the existing freshness function permits it. Fitment respects the existing positive and negative evidence rules, validity window and revocation. Conflicting evidence yields unknown compatibility.

The proposal is nonbinding. The existing quote, customer consent and order confirmation paths govern any consequential action. The worker has no order or reservation write tool. The new endpoint cannot accept extra agent or tool instructions. The architecture supports adding other allowlisted plans, with Auto Parts the only registered delegated plan in this iteration.

## Files

- `apps/api/migrations/00024_agent_delegation.sql`
- `apps/api/internal/agentruntime/{delegation,service,worker}.go`, `apps/api/internal/http/agents.go`, `apps/api/internal/platform/db/db.go`, and Next.js BFF `route.ts`
- `apps/api/tests/integration/{database,instagram_schema_stage22,stage261_delegation}_test.go` and `apps/web/tests/stage261-delegation-real-api.spec.ts`

## Acceptance

| Gate | Status | Evidence |
| --- | --- | --- |
| Parent/child persistence and correlation | PASS | Five PostgreSQL child tasks are visible while the worker is stopped. Replay returns the same parent and conflicts reject a changed payload. |
| Registry, Orchestrator and worker | PASS | Each claimed task executes its own registered tool and records its execution. Chromium receives the final parent aggregate after a real restart. |
| Retry, terminal failure and partial results | PASS | Focused PostgreSQL proves one successful execution after lease reclaim. Exhausted child retries fail the parent, cancel remaining dependencies, and preserve completed results. |
| Tenant isolation and permissions | PASS | Wrong lease, foreign conversation, missing CSRF, and extra agent instructions are denied. |
| Audit consistency | PASS | Delegation, dispatch, execution and finish events are persisted. Chromium verifies successful finish on a delegated child. |
| Auto Parts | PASS | Three business outcomes through Go API and worker. Verified fitment produces price and stock from the synthetic catalog. Missing information requests clarification. Unknown fitment produces no proposal. |
| Human Approval boundary | PASS | The proposal creates no order or reservation. Creating a separate quote through the existing API still leaves orders empty. Existing approval tests passed. |
| Full PostgreSQL regression | PASS | Serial rerun exited 0 with the same binary and SQL limit. Migration 00024 rollback/reapply and FORCE RLS on 61 tables passed. |
| Browser regression | PASS | 19 existing Chromium cases and the new Auto Parts scenario. |
| Go and frontend checks | PASS | Go test, race, vet, API and worker builds. Next.js production build, lint and typecheck. |
| E01 root cause | BLOCKED, baseline | UNDETERMINED. Containment PASS remains the prior result. |
| Other delegated workflows and Marketing generation | NOT RUN | Outside Stage 2.6.1. |

Intermediate failures are retained. A strict JSON validation failure initially discarded the partial aggregate. The fix omits missing child results and unknown stock values. A parallel full PostgreSQL rerun then passed delegation and hit the legacy catalog preview limit with SQLSTATE 57014. The limit stayed at one second. The serial rerun passed. A causal link between parallel execution and the timeout was not established.

## Reproduction

```sh
python3 .local/stage-2-6-1/run_go.py unit race vet api-build worker-build linux-worker-build linux-integration-compile
sh .local/stage-2-6-1/focused/run_integration.sh
sh .local/stage-2-6-1/run_integration.sh
sh .local/stage-2-6-1/e2e/run.sh
```

The PostgreSQL and Chromium runners use disposable databases and synthetic fixtures. The browser runner proves the previous 19-case suite separately from the new delegated scenario. The new scenario checks the persisted five-child plan while the worker is stopped, then requires a real restart and a final proposal. It also checks missing requirements and unknown fitment.

## Boundaries

Stage 2.6 as a whole remains open. E01 containment stays PASS, root cause UNDETERMINED, and E01 BLOCKED. Investigation and Marketing generation were outside this iteration. The existing draft storage and approval flows are included in regression.

[Stage 2.6.1 evidence and commands](../.local/stage-2-6-1/INDEX.md)
