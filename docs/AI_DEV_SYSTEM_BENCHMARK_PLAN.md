# AI Development System — benchmark plan

STATUS: PLAN ONLY. RTK/Serena/Context7 not installed or activated. Baseline timing, task token usage, output compression and savings: UNKNOWN. No percentage from upstream marketing is accepted as a measurement.

## Baseline and controlled task

1. Freeze source commit if one later exists; currently use the private snapshot manifest because HEAD is absent. Fix OS/architecture, Codex0.165, task prompt, model/effort, repo paths, Go/Node/dependencies and cached test images.
2. Navigation task: locate common channel ingress→catalog freshness→quote→staff reserve→dispatch recheck. Deliver a checked path/symbol list without editing. Same success rubric before/after; do not send private data to a hosted service.
3. Output task: explicit Go test output and repository search. No RTK hook in baseline. Use identical synthetic fixture and caches, alternate A/B order, at least3 runs each. Count repeated commands/tools and investigation errors.
4. Add only one optional component per experiment: compact AGENTS, then optional skills, then MCP, then manual RTK. Do not attribute a combined change to one tool.

Synthetic fixture directory after approval: /private/tmp/ai-dev-benchmark-<nonce>. Use local stdlib Go tests with passing table cases and intentional failure/panic/context errors. Do not edit app tests just to induce a failure; no real app/provider calls. Set GOPROXY=off/GOSUMDB=off/GOTOOLCHAIN=local and caches to private approved paths.

## RTK procedure

- Inspect verified OSS v1.1.0 binary help first. Confirm supported direct command syntax; proposed pair is go test -count=1 -v ./... versus rtk go test -count=1 -v ./... in the same synthetic module.
- Preserve stdout and stderr plus exact exit status for both. Use direct subprocess execution, not a pipe that returns tee's exit code. Never discard raw failure logs.
- For real repo navigation, compare narrow rg searches and git status --short --untracked-files=all. Current git diff is empty because files are untracked: it is not a representative diff benchmark.
- Record UTF-8 bytes, line counts, wall-time median, exit code, diagnosis correctness, missing error locations/test names and round trips. Measure cold/warm process cost separately.
- Verify intentional child nonzero remains nonzero; Go stack/message, SQLSTATE, authorization/tenant/budget failures and DELIVERY_UNKNOWN must remain diagnosable.
- Confirm actual Codex0.165 tool path: hosted functions.exec/exec_command is not proven equivalent to vendor Bash matcher. Keep automatic interception disabled regardless of a manual-prefix win until a separate hook test/approval.

| Metric | Baseline | Candidate |
|---|---|---|
| Task correctness / actionable mistakes |UNKNOWN|UNKNOWN|
| Time / tool calls / retries |UNKNOWN|UNKNOWN|
| Output bytes / lines |UNKNOWN|UNKNOWN|
| Actual model token usage |UNKNOWN|UNKNOWN|
| Backup/rollback restoration |NOT RUN|NOT RUN|

Output reduction =1 - candidate_bytes/baseline_bytes, only when baseline_bytes>0. It is an output-size proxy, not tokens or billing. RTK gain is an estimate and cannot replace agent usage telemetry. No inference API request is authorized to manufacture a token count.

Proposed acceptance, not measured benefit: retain100% required failure facts and exact exit status; no secret redaction regression; correct result; meaningful byte reduction on at least one representative noisy task; median overhead <=10% or <=100ms for short commands. Reject RTK if debugging quality falls, even if output gets shorter. Do not force optimization on already tiny successful output.

## MCP/skills and resource experiments

Context7: a single public library/version question with citations and a control using existing official web docs. Measure answer correctness, requests, latency and context volume, not vendor claims. No source upload and no paid plan.

Serena: only after prerequisites and GPL/containment review. Compare symbol/reference task against existing rg+gopls; measure cold start, index disk size, peak/steady RSS/CPU and warm lookups. startup_timeout_sec from upstream examples is a configuration timeout, not a measured startup duration. No arbitrary memory/time estimate is asserted now.

Superpowers: one selected workflow on the same task, no concurrent delegation, compare duplicated planning/context/tool overhead. Do not preload all skills. Keep built-in review separate from a second overlapping framework.

Rollback each candidate before the next experiment. A failed experiment is a legitimate REJECTED result, not permission to hide logs or relax project security.
