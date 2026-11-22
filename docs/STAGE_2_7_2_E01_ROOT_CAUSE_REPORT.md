# Stage 2.7.2 — E01 Root Cause Investigation

**Status: OPEN / BLOCKED — exact causal writer is not established.**

## Historical observed failure

Existing evidence in `STAGE_9C_SECURITY_REPORT.md`, `STAGE_10B_REPORT.md`, `STAGE_10D_REPORT.md`, and `.local/audit9c/` records a strict artifact-integrity guard refusal (exit 42). The full path/type/content diff was limited to `.next` and `node_modules` directory entries, observed empty in the captured state; broad metadata drift affected 2,642 entries. Original FAIL records and forensic snapshots remain preserved. Guard logic was not weakened and historical artifacts were not deleted as part of this stage.

The forensic capture found read handles attributed to macOS process `com.apple.Virtualization.VirtualMachine` (PID 68998 in that capture). A read handle proves access, not a write. There is no recorded write event, process lineage, or causally linked modification event identifying that process as the writer. Therefore this observation cannot establish root cause.

## Stage 2.7.2 investigation

- Re-read historical reports and retained `.local/audit9c` evidence references.
- Checked deployed manifest identity and public service state; these concern the VPS candidate and do not explain the historical local build-workspace mutation.
- No new host-level writer attribution evidence was available from the public VPS or current repository workspace.
- No source or integrity-guard change was made. No regression test was added because a causal mechanism to target has not been proven; inventing one would encode an assumption.

## Assessment

**Root cause: NOT PROVEN.** The affected surface is the development build workspace/artifact integrity capture, with observed `.next` / `node_modules` path metadata and broad metadata drift. Related artifact provenance remains under investigation. The available evidence does not establish whether a VM shared-folder process, another watcher, or a separate build tool caused the event.

## Required evidence to resolve

Capture a reproducible integrity-guard event in an isolated build workspace with host filesystem event tracing that records operation, path, timestamp and process identity/lineage; correlate it with build and VM lifecycle; reproduce the same mutation; then implement a narrowly scoped fix and regression test. Preserve the original failed capture and demonstrate the unchanged strict guard rejects the injected mutation and accepts a clean controlled build.

**Disposition:** E01 remains **OPEN**. No status closure, broad exclusion, guard weakening, or unsupported causal claim is made. This issue does not currently block the already running public staging services, but the unresolved artifact provenance issue remains a release-process blocker until proven and corrected.
