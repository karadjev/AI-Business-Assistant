# AI Development System — install/quality plan

STATUS: NOT EXECUTED. Approval gate after PHASE A–I. Consent must name a scope below; optional sections are not implicitly included. No commit, push, workflow publication, model request, paid subscription or live application integration is included.

## Wave1: recommended minimal scope

1. Recheck versions/config hashes/symlink targets and the351-file source fingerprint. Stop on drift.
2. Create timestamped private backups as in ROLLBACK_PLAN; no files backed up yet.
3. Create only approved global ~/.codex/AGENTS.md and project AGENTS.md from ARCHITECTURE drafts. No existing instruction is overwritten.
4. Install official VS Code extension openai.chatgpt, candidate version26.51021.22027 (observed public Marketplace metadata). Confirm version, publisher, Intel/darwin-x64 compatibility and signature before executing. If the exact candidate is unavailable/incompatible, stop for a new plan.
5. Verify IDE sidebar and shared configuration locally. Authentication requiring a secret is a separate user-controlled step; no model/API smoke prompt without explicit usage authorization.
6. Leave model=gpt-6.1-sol, effort=low, approvals_reviewer=user, project trust and existing config entries unchanged. No new MCP or hooks in Wave1.

Future command, NOT RUN:

~~~sh
code --install-extension openai.chatgpt@26.51021.22027
code --list-extensions --show-versions
codex --version
codex mcp list --json
~~~

[Official IDE instructions](https://developers.openai.com/codex/ide) link the [OpenAI extension](https://marketplace.visualstudio.com/items?itemName=openai.chatgpt). Marketplace metadata is not a successful install test. Review per-extension auto-update policy; do not disable all security updates globally or silently edit VS Code settings. No automatic reinstall/upgrade of other extensions.

Optional durable CLI entrypoint if terminal usability outside the app is required: npm global prefix is already user-local. After separate approval and inspection of existing bin/symlinks, candidate command is:

~~~sh
npm install -g @openai/codex@0.165.0
~~~

This downloads dependencies and changes user-global bins. It is NOT needed just because current CLI is in an npm cache. No sudo, Homebrew replacement, shell-profile/PATH editing or CLI update is part of Wave1 by default.

## Wave2: optional MCP, one at a time

All examples are inactive drafts in this document, not configuration installed on disk. [Codex reference](https://developers.openai.com/codex/config-reference) supports enabled, enabled_tools, bearer_token_env_var and HTTP header settings. Three draft TOML blocks parse successfully; a disabled ephemeral entry is accepted by the installed CLI without persistence. However --strict-config is unsupported for codex mcp in0.165.0: do not present it as a valid schema-check command. Review effective parsed settings and actual tool discovery after each approved merge; no connection was tested now. Merge only owned tables. No wholesale config replacement.

### Context7 hosted — preferred over local package

[Vendor setup](https://context7.com/docs/resources/all-clients) documents standard endpoint /mcp and OAuth /mcp/oauth. Use OAuth first:

~~~toml
[mcp_servers.context7]
enabled = false
url = "https://mcp.context7.com/mcp/oauth"
enabled_tools = ["resolve-library-id", "query-docs"]
default_tools_approval_mode = "prompt"
startup_timeout_sec = 20
tool_timeout_sec = 30
~~~

After explicit approval, merge this one disabled table, check effective config, then enable and authenticate with:

~~~sh
codex mcp login context7
~~~

Do not put credentials in TOML, .env, argv, repository or chat. OAuth credentials stay in the client's supported credential store. Validate resulting storage/permissions; do not assume authentication succeeded.

Alternative API-key transport, only if separately chosen: URL https://mcp.context7.com/mcp and bearer_token_env_var="CONTEXT7_API_KEY". The variable is supplied to the approved process by the user's secret mechanism, never written in repo files. Current vendor docs recommend a key for rate limits; anonymous availability is not guaranteed. OAuth avoids manually persisting the key. Do not invoke npx ctx7 setup automatically: it authenticates and writes configuration/skills.

Only public library IDs, pinned library version and minimal public API questions may leave the machine. No source files, SQL data, tenant IDs, logs or customer text. Hosted is chosen to avoid another Node dependency/startup; local @upstash/context7-mcp still calls the service. Hosted server version cannot be pinned. Free quota/terms and retention must be checked before enabling; no paid plan is authorized. Disable with enabled=false; remove only that table or codex mcp remove context7 after approval.

### GitHub: preserve one connection, enforce read-only

Existing github@openai-curated plugin is enabled; do not add a second connection over it. First inspect its effective tool names/auth permissions without exposing tokens. Preferred no-change option: use ordinary git; gh installation is optional. If read-only MCP is genuinely needed, separately approve disabling the old plugin MCP entry and adding exactly one read-only connection.

Possible safe disable patch for the existing server, subject to effective config validation:

~~~toml
[plugins."github@openai-curated".mcp_servers.github]
enabled = false
~~~

Candidate replacement, disabled until dedicated credentials are approved:

~~~toml
[mcp_servers.github_readonly]
enabled = false
url = "https://api.githubcopilot.com/mcp/readonly"
bearer_token_env_var = "GITHUB_MCP_READ_TOKEN"
http_headers = { "X-MCP-Readonly" = "true", "X-MCP-Toolsets" = "repos,pull_requests,issues", "X-MCP-Lockdown" = "true" }
enabled_tools = ["get_file_contents", "get_commit", "list_commits", "pull_request_read", "issue_read"]
default_tools_approval_mode = "prompt"
startup_timeout_sec = 20
tool_timeout_sec = 30
~~~

[Official remote parameters](https://github.com/github/github-mcp-server/blob/main/docs/remote-server.md) and [Codex setup](https://github.com/github/github-mcp-server/blob/main/docs/installation-guides/install-codex.md). Restrict a fine-grained token to the single selected repository: contents:read, pull requests:read, issues:read, automatic metadata:read; short expiry. Actions read is an extra opt-in only if CI inspection is needed. Never actions:write, contents:write, administration, workflow dispatch, merge or push. Server read-only + token restrictions + local allowlist are independent controls. Check actual tool discovery because hosted schemas can change; unknown tools stay unavailable.

Do not copy upstream's optional .env token examples. No new token is created in this phase; existing plugin credentials are neither read nor reused automatically. GitHub read requests and provider connection are not part of Wave1. Hosted version is mutable; local v2.0.3 container would add image/runtime overhead and requires a verified digest, so is not recommended initially.

### Serena / Superpowers / ECC / GSD

Serena config is intentionally NOT prepared: uv and isolated Python3.13 prerequisite are missing. gopls0.23 exists but is not in PATH. Need separate runtime/LSP dependency, GPL, filesystem isolation and resource benchmark approvals. No serena init/setup, hooks or auto-indexing now.

Superpowers optional official route is Codex /plugins → superpowers → inspect exact version/ref and hook trust → Install Plugin, or App Plugins/Coding. Candidate upstream7.0.1; marketplace pin behavior must be validated before approval to install. Never duplicate with legacy ~/.agents/skills symlinks/clones. No undocumented --version flag is invented.

ECC full plugin/config sync and archived GSD install are excluded. Selected reference ideas do not authorize running their installers.

## Wave3: RTK OSS pilot only

Candidate verified asset name:
https://github.com/rtk-ai/rtk/releases/download/v1.1.0/rtk-oss-x86_64-apple-darwin.tar.gz

Checksum manifest:
https://github.com/rtk-ai/rtk/releases/download/v1.1.0/checksums.txt

Only after explicit download approval: obtain both as data (no pipe to shell), inspect archive paths, verify exact asset checksum/provenance, extract into a dedicated private staging directory and place the verified binary in a versioned user-owned tool directory. Validate actual executable name/architecture/signature metadata before use. Same-origin checksum alone is not independent provenance; missing checksum/signature requires review, not a guessed hash. No cargo/toolchain installation.

Manual prefixes first; do NOT run init automatically. Possible later non-writing preview/removal commands, after that binary's help confirms flags:

~~~sh
rtk init --codex --dry-run
rtk init --codex --uninstall --dry-run
~~~

No global init, proxy daemon, API traffic interception or hooks in the approved manual pilot. A future project hook requires a second approval, dry-run diff, symlink inspection and benchmark success. Rollback commands are in ROLLBACK_PLAN.

## Unified quality workflow — future execution only

Existing runners, verified paths:

| Gate | Existing route | Important boundary |
|---|---|---|
| Go format/tests/vet/build |prepared Compose service go |Offline module cache, GOPROXY off, source RO |
| Go race |service race / scripts/build-env/race.sh |Existing cached C-toolchain image |
| DB/migrations/RLS/tenant/finance/catalog/concurrency |service integration, Go -race -tags=integration ./tests/integration |Fresh dedicated aba_test DB; never working DSN |
| Frontend lint/typecheck/OpenAPI/native guards |service web-check |Existing scripts, no package download |
| Fresh Next production |service web-build / production.sh |Exclusive writable build workspace; stop existing test web before rebuilding |
| Browser current production |service browser / browser.sh |Original proof+served marker, three synthetic production specs |
| Artifact safety |scripts/build-env/artifacts.cjs verify; browser served check |E01 must fail closed; do not rerecord proof to bless drift |
| Authorization/financial/upload regressions |existing Go integration + mock E2E |Both channels; live off; budgets0 |
| Secret scanning |existing ignore rules and historical client scan only |Reusable repo-wide automated scanner missing: BLOCKED as a new automated gate |
| Dependency advisories / deeper SAST |npm audit available but needs approved network; govulncheck/staticcheck etc absent |NOT RUN; no audit-fix, new tool or external DB fetch without approval |

sh scripts/check.sh and sh scripts/integration.sh exist, but may write cache/build artifacts, build images or fetch dependencies. They were not run read-only and are not silently treated as strict-offline runners. scripts/smoke.sh owns local test processes but must receive isolated test DSNs. Prefer the already prepared containers.

Future invocation skeleton from repo root; use one unique RUN stem, distinct build/DB/browser projects. Run sequentially because evidence/build paths are shared. The env file is referenced, never printed.

~~~sh
AI_DEV_RUN="aba-ai-dev-$(date -u +%Y%m%dT%H%M%SZ)"
~~~

For every docker compose command use:
--env-file .local/audit8d/compose.env
-f infrastructure/compose.environment.yml
-f infrastructure/compose.environment-smoke.yml
-f infrastructure/compose.prepared-build.yml

Then, in order:

~~~text
-p "$AI_DEV_RUN-build" run --rm go
-p "$AI_DEV_RUN-build" run --rm race
-p "$AI_DEV_RUN-db" run --rm integration
-p "$AI_DEV_RUN-build" run --rm web-check
-p "$AI_DEV_RUN-build" run --rm web-build
-p "$AI_DEV_RUN-build" run --rm go go build -o /artifacts/browserfixture ./tests/browserfixture
-p "$AI_DEV_RUN-browser" up -d api worker web
-p "$AI_DEV_RUN-browser" run --rm -e PLAYWRIGHT_JSON_OUTPUT_NAME=/results/ai-dev.json browser --grep "current production" --output=/results/ai-dev --reporter=json
~~~

The text lines are argument tails, not standalone shell commands. Before execution verify cached images/modules/Linux native bindings and env-file existence; missing assets → BLOCKED, not automatic pull/install. Keep integration fixtures and browser fixtures in separate DB projects. Freeze app source after build. Capture complete failure logs outside Git, track exact exit code (pipefail when using tee), report NOT RUN separately, and clean only these named test projects with down -v --remove-orphans. Do not prune global Docker resources.

## CI proposed delta — not applied/published

Local workflow already has least-privilege contents:read and no deploy. Keep its structure, no new workflow_dispatch or pull_request_target. Replace existing action tags everywhere with these public upstream tag-resolved commit SHA (verified through GitHub refs API):

~~~diff
- uses: actions/checkout@v4.2.2
+ uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4.2.2
- uses: actions/setup-go@v5.5.0
+ uses: actions/setup-go@d35c59abb061a4a6fb18e82ac0862c26744d6ab5 # v5.5.0
- uses: actions/setup-node@v4.4.0
+ uses: actions/setup-node@49933ea5288caeca8642d1e84afbd3f7d6820020 # v4.4.0
~~~

For checkout add persist-credentials:false. Make all three channels/providers' existing kill/budget flags explicit, including WHATSAPP_LIVE_ENABLED=false, WHATSAPP_EXTERNAL_REQUESTS_AUTHORIZED=false, WHATSAPP_SENDING_KILL_SWITCH=true, WHATSAPP_MONETARY_BUDGET_RUB=0. Preserve current Telegram and LLM mock/kill flags.

Further separate review: image digests, complete current-production E2E/build-proof routing rather than only old smoke scripts, retention-safe synthetic artifacts, and an approved pinned secret/vulnerability scanner. No production secrets or paid CodeQL/hosted service assumed available. Expensive DB/race/browser checks may be separate jobs but are not silently skipped for auth/financial/RLS changes. Existing tag pins are not immutable security proof.

Git currently has no tracked files/HEAD. These deltas are a reviewable plan only; activating CI needs separate authorization for repository versioning/publication. Approval of tool setup does not authorize commit/push.

## Network and permissions matrix

| Scope | External destinations | Writes/permissions |
|---|---|---|
| Current A–I |public GitHub/official docs/Marketplace metadata only |Seven docs; read-only diagnostics |
| Wave1 |marketplace.visualstudio.com and its official asset/CDN endpoints |~/.vscode/extensions, extension registry; two new AGENTS files. No repo push |
| Optional CLI |registry.npmjs.org/package assets |User-local npm prefix/bin; no sudo/PATH edits without another consent |
| Context7 |mcp.context7.com, Context7 OAuth issuer discovered on login |One approved TOML table; user credential store; minimal public queries |
| GitHub readonly |api.githubcopilot.com and GitHub auth as selected |One scoped token/env reference, one enabled connection, no writes |
| RTK pilot |GitHub release assets/CDN |One versioned OSS binary + private benchmark directory; no hooks |
| Future CI |GitHub Actions/approved npm/modules/container registries/advisory DBs |Separate approved workflow diff; never production secrets/deployment |

Zero project LLM/WhatsApp budgets remain enforced. Model-based verification consumes agent entitlement/usage and is NOT authorized as a free API call by this plan. No benefit or dollar/token saving is promised before measurements.
