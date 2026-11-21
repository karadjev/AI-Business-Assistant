# AI Development System — upstream review

Snapshot:2026-10-29. Official repository pages, GitHub REST repo/releases/latest/commits and linked vendor documentation were read. No tool was installed or executed to test integration. Listed versions are observed releases, not proof of local compatibility. Dates UTC. pushed_at is repository activity, not a semantic release or proof of quality.

## Verified upstream metadata

| Requested / canonical upstream | Status | Last available GitHub release | Published UTC | License / qualification | Latest default-branch commit |
|---|---|---|---|---|---|
|[obra/superpowers](https://github.com/obra/superpowers)|active, not archived|[v7.0.1](https://github.com/obra/superpowers/releases/tag/v7.0.1)|2026-10-21 21:05:11|MIT|2ce8aa15d05b,2026-10-21 21:00:49|
|[upstash/context7](https://github.com/upstash/context7)|active|[@upstash/context7-sdk@0.6.0](https://github.com/upstash/context7/releases/tag/%40upstash/context7-sdk%400.6.0)|2026-10-23 11:23:07|MIT|eabe36965ec6,2026-10-28 14:44:33|
|[oraios/serena](https://github.com/oraios/serena)|active|[v2.1.0](https://github.com/oraios/serena/releases/tag/v2.1.0)|2026-10-19 17:30:43|Application GPL-3.0-or-later; SolidLSP MIT|ede329aaf0c8,2026-10-28 10:45:54|
|[github/github-mcp-server](https://github.com/github/github-mcp-server)|active|[v2.0.3](https://github.com/github/github-mcp-server/releases/tag/v2.0.3)|2026-10-21 09:52:29|MIT|f2a07cd13243,2026-10-28 14:28:59|
|[rtk-ai/rtk](https://github.com/rtk-ai/rtk)|active|[v1.1.0](https://github.com/rtk-ai/rtk/releases/tag/v1.1.0)|2026-10-25 18:00:11|OSS Apache-2.0; full binary also BUSL-1.1|7e92da00a5cf,2026-10-28 23:54:21|
|everything-claude-code → [affaan-m/ECC](https://github.com/affaan-m/ECC)|renamed, active|[v2.2.3](https://github.com/affaan-m/ECC/releases/tag/v2.2.3)|2026-10-01 23:00:36|MIT|25278e7d7b6c,2026-10-26 04:03:57|
|[gsd-build/get-shit-done](https://github.com/gsd-build/get-shit-done)|ARCHIVED|[v1.42.3](https://github.com/gsd-build/get-shit-done/releases/tag/v1.42.3)|2026-05-16 04:36:09|MIT|bdcaab2c752d,2026-05-31 13:34:35|

Metadata is reproducible through https://api.github.com/repos/OWNER/REPO, /releases/latest and /commits?per_page=1. Archive date is UNKNOWN; last push is not archive date. GSD README redirects to open-gsd/gsd-core; that successor was not silently substituted or approved.

“Last significant update”: Superpowers21Oct release explicitly changes brainstorming/compaction; Context7 head28Oct changes Admin MCP console; GitHub MCP head28Oct fixes merged-state output; Serena latest release19Oct is the last verified release-level milestone (28Oct commit is LS logging); RTK latest release25Oct (28Oct merge significance not established); ECC latest release1Oct (26Oct merge functional significance UNKNOWN); archived GSD latest functional release16May,31May README/link change is not a new functional release. Upstream test pass and security assurance remain UNKNOWN for every project.

Context7 is a monorepo: latest GitHub SDK release is NOT an MCP-server version. [MCP source package](https://github.com/upstash/context7/blob/master/packages/mcp/package.json) currently declares4.4.4 and Node>=20.18.1; npm publication time/version parity was not verified. Hosted service version cannot be pinned by that package tag.

## Decisions, compatibility, permissions and rollback

| Tool | Decision | Fit, integration and prerequisites | Main risks / conflicts | Rollback |
|---|---|---|---|---|
|Superpowers|OPTIONAL|Official Codex plugin marketplace; CLI /plugins or App Coding section. Skills can support planning/debug/TDD/review/verification; IDE requires working Codex integration.|Framework prompts/hooks can compete with project rules, encourage delegation or add context. No broad filesystem access is needed for documentation alone. Inspect exact plugin version/hooks before trust.|Disable/remove only that plugin; restore owned instruction/hook changes from manifest. Never maintain native plugin plus legacy cloned/symlinked copy.|
|Context7|OPTIONAL|Codex HTTP MCP supported; VS Code/Codex shared config. Prefer hosted optional OAuth; local stdio adds Node/npm dependencies despite Node already available.|Query/library/version and connection/auth metadata leave machine; private source/logs forbidden. Local wrapper still contacts service. Endpoint is mutable; offline tasks must keep it disabled.|enabled=false or remove only new server; revoke new OAuth separately.|
|Serena|EXPERIMENTAL, prerequisites BLOCKED|Codex CLI/App and MCP IDE support documented; Go/TS LSP navigation potentially useful. uv and requested isolated Python3.13 are missing; gopls0.23 exists off PATH.|Cold start/index memory/CPU UNKNOWN. Auto-download LSP/runtime deps, filesystem editing/shell tools and v2 REPL expand authority; read-only mode is not OS isolation. GPL application licensing needs review for distribution.|Disable/remove MCP; remove only owned index/config after backup; uninstall isolated tool/runtime only if this plan created them. No config generated now.|
|GitHub MCP|OPTIONAL; no duplicate|Official remote/local Codex setup and VS Code support. Existing native GitHub plugin already present. Plain git is sufficient locally; gh would add convenient CI/PR reads but is absent.|Current plugin read-only/auth scope unverified. Remote issue/PR text is untrusted; no writes/settings/actions dispatch. Dedicated selected-repo read credentials plus tool allowlist required.|Disable new server, restore prior plugin setting, revoke only new token. Hosted version drift; local container digest would need review.|
|RTK OSS|EXPERIMENTAL manual pilot|Vendor documents Codex PreToolUse.updatedInput, dry-run/uninstall; CLI0.165 is installed but actual host exec_command hook coverage NOT TESTED. Use explicit prefixes before hooks.|Filtering may drop failures/RLS evidence or alter exit codes. Codex hook matcher Bash may not cover current hosted orchestration. Full/Proxy add BUSL engine/API interception; no need here.|Manual pilot: stop prefix/use and remove owned binary. Optional future hook removal only after dry-run+manifest; global interception not approved.|
|ECC full bundle|REJECTED for this rollout; selected references optional|Current renamed repo has native Codex plugin and project config; not Claude-only. Native route differs from legacy sync.|Duplicates workflow skills/MCP, owns hooks/memory/config and model policy. Copying sample config or sync can overwrite custom safety. VS Code support through Codex must be tested, not assumed.|If later chosen, one native plugin only, no legacy sync; retain license notices for selected copied material.|
|Archived GSD upstream|REJECTED|No need for another orchestration layer; specified upstream archived.|Stale installer/dependency assumptions; successor requires a separate review.|Do not install; no rollback needed.|

Superpowers official route: [current README Codex section](https://github.com/obra/superpowers#codex-cli). Old .codex/INSTALL.md currently404; do not reuse old clone+symlink tutorials. Marketplace source is [openai/plugins](https://github.com/openai/plugins). Installed plugin version/ref pin must be checked in the marketplace; do not assume CLI plugin add supports --version.

Serena license is [component-specific](https://github.com/oraios/serena/blob/main/LICENSE), not uniformly MIT. [Official install](https://oraios.github.io/serena/02-usage/010_installation.html) uses uv with isolated Python3.13; [Codex setup](https://oraios.github.io/serena/02-usage/030_clients.html#codex-cli-and-app) can write configuration/hooks. No Serena config is proposed until prerequisites and containment are approved.

RTK [editions](https://github.com/rtk-ai/rtk#installation) distinguish rtk-oss from full rtk and rtk-proxy. Only the OSS x86_64-apple-darwin release asset is a pilot candidate. [Codex init source at v1.1.0](https://github.com/rtk-ai/rtk/blob/v1.1.0/src/hooks/init/codex.rs) and [agent documentation](https://www.rtk-ai.app/docs/getting-started/supported-agents/) establish available dry-run/removal mechanisms, not a successful local test. No savings claim is accepted from marketing or stars.

ECC [Codex integration](https://github.com/affaan-m/ECC#codex-app-and-cli) now uses its own marketplace; the old sync/config-copy route is not the recommended route and is excluded here.

All package/plugin code runs with the agent/user's effective access unless isolated. A prompt or skill is not a security boundary. Sources were read as untrusted reference data; no remote install script was executed.
