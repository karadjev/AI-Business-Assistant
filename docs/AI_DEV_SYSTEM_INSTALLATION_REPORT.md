# AI Development System — installation report

## Latest continuation — renewed full-setup request, 2026-10-29

Overall: PARTIAL / BLOCKED. The attachment was read in full. Its baseline predates the existing installation; absent-extension/AGENTS statements are historical. Existing components are preserved, not reinstalled. Prior explicit Audio verification and separate CLI-install approval requirements remain unresolved; broad setup approval is not treated as permission to bypass them.

Current checks performed in this continuation:

| Check | Result |
|---|---|
| Backup archive SHA-256 reread, 5,288,253,440 bytes | PASS: matches `7564d0836380d72a1ed0072476b49b1210e214fb77f021a72bd35f78f869947b`; elapsed 14.52 seconds |
| Backup manifest, operations, verification and configs availability | PASS: present; full restore and current-source equivalence NOT RUN |
| Project/global instructions | PASS: files present, 2017/1052 bytes; hashes `e0a44e88f6b9811e76a753177005752ca3ef7566701f3b10489df9751cd4d1a1` / `e3c7965a61f99269730007d6390ef0f09d9951099fe49bbd8dd37b12c91162b8`; fresh IDE semantic loading NOT RUN |
| Audio persisted disablement | PASS: Default profile disabled-extension state contains openai.codex-audio |
| Audio absence and core operation after restart | BLOCKED: latest available host still pid5716, start23:37:24.673, with earlier audio activation23:37:35.270; running VS Code5702 / core Codex5846. No independent evidence of a later restart |
| Superpowers inventory | PASS: one native cached plugin5.1.3 with14 skills; behavioral scenarios NOT RUN |
| GitHub inventory | PASS: one native plugin0.1.6 with4 skills; remote endpoint api.githubcopilot.com/mcp. Actual credential permissions and authenticated read-only smoke NOT RUN; no duplicate installed |
| MCP config inventory | PASS: no standalone mcp_servers entries; plugin MCP is a separate mechanism |
| Official npm CLI metadata | PASS: registry metadata for @openai/codex0.140.0 points to github.com/openai/codex; optional darwin-x64 alias resolves to0.140.0-darwin-x64, os=darwin/cpu=x64; integrity and signature fields present. Cryptographic signature verification NOT RUN |
| Serena prerequisites | BLOCKED: uv, uvx, python3.13 and gopls absent from PATH; absence from PATH does not prove absent from the entire disk |
| RTK | NOT RUN: no rtk on PATH, no download/install/interception |
| Context7 / Serena pilot / GitHub authenticated operation | NOT RUN: Audio gate unresolved; Context7 also requires user-controlled OAuth, Serena additional dependencies require separate approval |
| Offline quality gates / comparative benchmark | NOT RUN under the existing stop gate; no historical result relabeled PASS |

Backup path remains `/Users/ilezkaradjev/.local/share/ai-dev-backups/20261029T202435Z`, outside the repository. Hash verification confirms archive integrity against the recorded digest, not that subsequent project edits are backed up. Before further installation, capture fresh deltas/config backups and verify current source state; do not overwrite the baseline archive.

CLI repair commands remain proposals only, detailed in AI_DEV_SYSTEM_CLI_AUDIO_FOLLOWUP.md. Preserve existing installations. Candidate pinned repair in a new absent prefix, after separate approval and package review: `npm install --global --prefix "$HOME/.local/share/ai-dev-tools/codex-0.140.0-repair" --include=optional --ignore-scripts @openai/codex@0.140.0`. This version was chosen to isolate repair of the installed version, not asserted to be current/recommended. No global install, package replacement, symlink or PATH edit was performed. Do not run the proposed command until the approval and Audio gates are resolved.

Only this report was changed in this continuation; npm metadata requests may update npm's own cache/logs. No downloaded package or script was executed, no credentials accessed, no microphone command, no application/CI/model/reasoning/sandbox edit, no commit/push/deploy/live API call. E01 remains OPEN. Runtime rollback is unnecessary for this continuation; retain historical rollback below for the prior installation. Incremental productivity benefit, token savings and post-install resource impact remain UNKNOWN.

The remaining sections below describe the earlier installation segment, not fresh results from this continuation.

Date: 2026-10-29T20:32:58.321950+00:00

**Overall: PARTIAL — PAUSED AT EXPLICIT ADDITIONAL-PACKAGE APPROVAL GATE.** This is not a full successful installation. User authorized full setup, but the earlier Phase J requirement to stop for an additional package still applies. VS Code installed an unlisted extension-pack member automatically; no remaining installation or activation will continue until its disposition is approved.

## Environment revalidation

Codex CLI0.165.0, VS Code1.143.1/x64 and the existing model gpt-6.1-sol/reasoning low confirmed unchanged. Existing diagnostics record macOS15.7.9, Intel i7-8750H,16GiB, Go1.26.2, Node24.15.0. Those hardware/runtime versions were not all rerun during this installation segment. Git main still has no initial commit; existing files remain untracked. No commit/push/deployment performed.

## Backup — PASS with stated restore limits

Private root: `/Users/ilezkaradjev/.local/share/ai-dev-backups/20261029T202435Z` (outside Git, mode0700).

- Full `project.tar`: project.tar / 5288253440 bytes, 86529 objects. Includes ignored local data and dependencies; never publish it.
- Archive SHA-256: `7564d0836380d72a1ed0072476b49b1210e214fb77f021a72bd35f78f869947b`.
- Every archived regular-file body was re-read and checked against its SHA-256; symbolic links retained without following their targets.
- `manifest.json`: initial paths/types/modes/timestamps/file digests and ABSENT config entries; no secret values printed.
- `configs/`: original global TOML, skills, VS Code settings/profile metadata and extension registry. Auth stores/Keychain/SSH keys were not exported.
- `operations.json`: exact owned instruction files, plugin, extension manifests and config delta.
- `verification.json`:399 existing source/docs files unchanged;52 copied directory-config files matched originals. A README.md restore into a disposable temporary directory passed bytes/mode checks.
- Full project extraction/restore: NOT RUN. ACL/xattr fidelity: NOT VERIFIED. Live VS Code state databases are not transaction-consistent snapshots.
- `.local/ai-dev-install/backup-location` is an ignored pointer inside the workspace, not the backup itself.

## Installed components

| Component | Version / result |
|---|---|
| Official Codex IDE extension |openai.chatgpt26.51021.22027, darwin-x64; installation PASS |
| Automatic extra extension-pack member |openai.codex-audio26.1021.22027; installed by VS Code, NOT separately approved; decision pending |
| Codex CLI |existing0.165.0 retained; version command PASS |
| Superpowers |one native plugin superpowers@openai-curated, package5.1.3, cache revision d6169bef; installation/manifest PASS |
| Global AGENTS.md |created from approved draft,1052 bytes |
| Project AGENTS.md |created from approved draft,2017 bytes |
| Context7 |NOT CONFIGURED / NOT AUTHENTICATED; paused before this step |
| Serena |NOT INSTALLED; uv/uvx/Python3.13 absent from PATH; extra system dependencies require separate approval |
| GitHub |existing github@openai-curated0.1.6 retained; no second GitHub MCP installed |
| RTK |NOT INSTALLED / NOT ENABLED; pilot not reached |
| ECC / GSD |NOT INSTALLED, as requested |

## Codex IDE verification

Official publisher `openai`, extension ID `openai.chatgpt`, version26.51021.22027 reconfirmed from Marketplace and installed manifest. Engine requirement `^1.96.2` is compatible with installed VS Code1.143.1. No other installed copy found in inspected profiles before installation. VS Code CLI reported successful signature verification for both installed OpenAI extensions, then successful installation.

The Codex manifest declares `extensionPack = ["openai.codex-audio"]`, **not** `extensionDependencies`. Audio declares activation on `_codex.microphone.available`. Codex declares onStartupFinished/onUri. These declarations do not prove runtime activation. No microphone permission was requested by this agent. No user credentials, account password or access token were read.

IDE sidebar activation, interactive sign-in, project interaction and CLI/IDE end-to-end operation: **BLOCKED pending the extra-package decision and later UI smoke**. No paid model request was made. Successful installation is not reported as successful IDE operation.

## Instructions and skills

Both AGENTS.md were extracted from docs/AI_DEV_SYSTEM_ARCHITECTURE.md; no existing instruction file was replaced. Global instructions appeared as loaded AGENTS instructions in the active conversation after creation. Project file content/path verified. Independent fresh CLI/IDE model-session instruction-discovery smoke: NOT RUN; global host loading does not prove every client loaded project instructions.

Native Superpowers install succeeded through `codex plugin add superpowers@openai-curated --json`. Required skill files exist: writing-plans, systematic-debugging, test-driven-development, requesting-code-review, verification-before-completion. Fourteen skill directories installed, one plugin copy. No legacy skills symlink or second framework installed.

Upstream obra/superpowers latest release rechecked:7.0.1,2026-10-21. Reviewed source archive SHA2562295dd270d929ddbe3f82e43ab5cc6e3dfb4afdbfb56225347394491d34d747d. Current public openai/plugins manifest says6.3.0, while local built-in official marketplace exposes5.1.3. `marketplace upgrade openai-curated` failed because this built-in source is not configured as a Git marketplace. No force replacement or second marketplace was created. Do not call installed5.1.3 latest.

Installed official skills and script inventory reviewed. Script helpers include optional brainstorm server, polluter test runner and graph rendering; none executed. No global hook file or standalone MCP was introduced. Plugin script availability does not authorize execution. Skills suggesting frequent commits, parallel agents or branch/worktree actions cannot override user restrictions; no such action performed. Functional model-driven planning/debugging/TDD/review scenarios: NOT RUN. No claim of semantic skill effectiveness.

## MCP and security

Global TOML parsed after native installation. Exact semantic delta from backup is only `[plugins."superpowers@openai-curated"] enabled=true`; all model/reasoning/approval/project-trust settings retained. No sandbox weakening. `codex mcp list --json` still returns[]. No Context7 token or secret entered TOML/repository.

Existing GitHub plugin includes GitHub access and skills github/gh-fix-ci/gh-address-comments/yeet. Its manifest uses a bearer environment variable reference and has no read-only restriction. Actual granted token permissions UNKNOWN; no credential inspection or authenticated operation. Read-only smoke and least-privilege enforcement remain BLOCKED. This is an existing risk, not evidence of safe read-only access. Do not invoke write-capable skills automatically.

Context7 planned next: single hosted OAuth MCP at https://mcp.context7.com/mcp/oauth, allow only resolve-library-id/query-docs, public documentation queries only. No Context7 configuration/login/connectivity check yet. If login requires manual browser OAuth, user must complete it locally; stop at that step per user instruction. Do not ask for credentials in chat.

Security: CONDITIONAL / installation incomplete. E01 remains OPEN with no new causal investigation. No app business logic, migrations, CI or working database changed. No real Telegram/Meta/LLM/1C API calls. Public docs/metadata and Marketplace/Codex plugin installation are the only intentional external operations. Application LLM/WhatsApp budgets remain0 RUB. No new background server intentionally started by this agent; IDE runtime resource impact UNKNOWN.

## Verification and benchmark

| Check | Result |
|---|---|
| Full backup archive body/hash verification |PASS |
| Representative file restore bytes/mode |PASS |
| Existing source/doc integrity399 files |PASS |
| Original directory configuration copies52 files |PASS |
| Codex CLI version |PASS |
| IDE install/signatures/engine compatibility |PASS |
| IDE runtime/login/CLI interaction |BLOCKED |
| Global instructions loaded in active host conversation |PASS |
| Fresh project/global discovery across CLI and IDE |BLOCKED |
| Superpowers native install/required files |PASS |
| Superpowers task behavior |NOT RUN |
| Context7 documentation query |BLOCKED |
| Serena semantic search/resource pilot |BLOCKED |
| GitHub authenticated read-only query |BLOCKED |
| RTK benchmark |NOT RUN |
| Go tests/race/vet/build |NOT RUN — installation paused |
| Frontend lint/typecheck/build/OpenAPI/E2E |NOT RUN — installation paused |
| PostgreSQL/RLS/tenant/security regression |NOT RUN — installation paused |

Existing commands remain unchanged: scripts/check.sh; apps/web package scripts lint/typecheck/check:openapi/build/test:smoke; isolated prepared PostgreSQL/browser runners documented in AI_DEV_SYSTEM_INSTALL_PLAN.md. Do not execute scripts/check.sh blindly offline because it can build/download. No CI changes made. A new one-command quality runner was NOT created in this paused segment.

No pre-install model-task baseline was collected. No after-only test is represented as before/after. Token usage, speed improvement, operation-count reduction, RTK compression, Serena RAM/startup and IDE incremental load: UNKNOWN. Confirmed benefits are reproducible backup evidence and compact persistent instructions; development productivity improvement remains unmeasured.

## Exact change inventory

- Created project `AGENTS.md`.
- Created global `~/.codex/AGENTS.md`.
- Created this report `docs/AI_DEV_SYSTEM_INSTALLATION_REPORT.md`.
- Global `~/.codex/config.toml`: native installer added only Superpowers enablement.
- Created `~/.codex/plugins/cache/openai-curated/superpowers/d6169bef/` (native managed plugin copy); CLI may also create its temporary catalog/PATH aliases, not added to shell profile.
- VS Code extension store/registry: added openai.chatgpt and openai.codex-audio; CLI installation logs/cache created.
- Created ignored `.local/ai-dev-install/backup-location` pointer.
- Created private external backup root, manifests and verification evidence.
- Downloaded reviewed public source/metadata into `/private/tmp/ai-dev-upstream-review/`; no downloaded script executed.
- Existing app code, dependencies/lockfiles, CI and prior reports unchanged.

## Rollback procedure — not executed on real installation

1. Close affected Codex/VS Code sessions. Read private operations.json; refuse to overwrite later user edits. Never git clean/reset/restore this untracked repository.
2. If removing this rollout, uninstall only newly installed extensions: `code --uninstall-extension openai.chatgpt`, then `code --uninstall-extension openai.codex-audio`. They were absent from inspected profiles at baseline. Do not uninstall pre-existing extensions or remove their directories manually.
3. Use `codex plugin remove superpowers@openai-curated` for the single owned plugin. Verify its command help/current state first; do not remove all plugins/skills.
4. Remove only newly created AGENTS files whose current SHA256 equals operations.json. Preserve any subsequently edited file for manual merge.
5. Restore global TOML from configs/0 only if no concurrent change occurred; otherwise remove only this rollout's owned Superpowers table. Before whole-file restoration, compare against the expected recorded after-hash. Restore original0600 mode. Never touch model/reasoning or other plugin tables.
6. Verify pre-install extension inventory, original TOML semantics and source-file hashes. Keep backups and evidence outside Git. Revoke OAuth only if one was actually created (none now).
7. For project recovery, select explicit manifest paths and verify archive containment/symlinks before extracting; never unpack wholesale over current user files. The representative restore passed; full restore remains untested.

## Required decision / next step

Approve either keeping `openai.codex-audio` as the official extension-pack member, or uninstalling that optional member while retaining Codex and checking core operation. No microphone usage is authorized by retaining the package. Once decided, resume UI smoke, then Context7 OAuth; pause again if interactive authorization is required. Remaining Serena/RTK/GitHub/testing phases stay pending and must not be marked PASS.

Sources: [OpenAI IDE](https://learn.chatgpt.com/docs/ide), [Codex developer commands](https://learn.chatgpt.com/docs/developer-commands), [Marketplace](https://marketplace.visualstudio.com/items?itemName=openai.chatgpt), [Superpowers upstream](https://github.com/obra/superpowers), [official plugin package](https://github.com/openai/plugins/tree/main/plugins/superpowers), [Context7 clients/OAuth](https://context7.com/docs/resources/all-clients).
