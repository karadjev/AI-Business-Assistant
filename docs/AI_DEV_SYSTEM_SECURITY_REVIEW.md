# AI Development System — security review

Status: CONDITIONAL / design review. Application tests and tool post-install tests NOT RUN in this phase. Existing10Б security report0 CRITICAL/0 HIGH/1 LOW is historical, not a fresh certification. E01 remains OPEN.

## Main risks and required controls

| Risk | Observation | Control / remaining limit |
|---|---|---|
| No rollback baseline |Git HEAD absent, tracked files0 |Private filesystem backup+manifest mandatory; no git reset/clean/restore or unauthorized initial commit |
| Instruction/plugin authority |Project trusted, native GitHub plugin enabled; external skills/hooks may change behavior |User task authorization first; no config replacement/auto-delegation/commit hooks; inspect exact code/ref and effective tools |
| GitHub write scope |Existing plugin has no read-only header; token scopes/auth UNKNOWN; no GitHub tools currently exposed |Do not claim connection PASS. Review once, prefer one read-only connection with selected-repo token, server read-only and client allowlist |
| Documentation exfiltration |Context7 receives query/library/version/auth/connection metadata |Only minimal public docs questions. Do not send private source, logs, secrets, tenants or customer records. Vendor retention UNKNOWN pending review |
| Local MCP authority |Serena can edit/execute; REPL can access what its process can |Do not install now. Prerequisites/containment required; API/tool exclusion is not OS sandbox. Startup/resource use UNKNOWN |
| Command rewrite |RTK hook can transform approvals/commands and suppress output |Manual OSS pilot; preserve exit status/full errors, no global hook or API proxy, no estimated-token claims as real usage |
| Licensing/supply chain |Serena v2 GPL; full RTK mixes BUSL; npm/plugins/images can drift |Version+asset/checksum/provenance review; prefer OSS RTK; no remote install scripts. Hosted MCP cannot be version-pinned |
| CI supply chain |Action version tags mutable; service image tags mutable; npm downloads in hosted CI |Proposed immutable SHA, image digest review, read-only token/persist-credentials:false, no PR production secrets or deploy |
| Missing reproducible security automation |No installed dedicated scanner or advisory tool found |Security gate incomplete, not PASS. Approve specific pinned scanner/advisory access separately; no npm audit fix |
| Build integrity |E01 empty dirs/metadata drift documented and reproduced in10Б |Preserve original manifests/proof, exact evidence; do not add wildcard ignores or bless changed artifacts |

[Serena configuration security boundary](https://oraios.github.io/serena/02-usage/050_configuration.html): limiting REPL APIs is steering, not sandboxing. [RTK edition boundary](https://github.com/rtk-ai/rtk#installation): choose OSS candidate only, not default full/proxy payload. Distribution/embedding terms need a separate review if the tool is redistributed.

## Existing project controls to preserve

Go API/worker, Next.js BFF and equal Telegram/WhatsApp channel layer remain unchanged. Preserve session/Origin/CSRF, tenant-safe keys, FORCE RLS45, webhook verification, durable admission/reservation/settlement/unknown outcomes, trusted template/pricing verification, inventory freshness and source guards. No migration against working data, no actual customer data or credentials in tests, no bypass of zero budgets. ACTIVE/onboarding/health/source flags must never enable live.

Current project tooling has pinned package versions, isolated test DB conventions, mock transports, custom auth/financial/concurrency tests, fresh-build and E01 guards. These controls are valuable but do not replace current advisory scanning or a post-install test.

## Secret handling and metadata

.env/.env.local, auth.json, keychains/SSH private keys and secret stores were not inspected. Reports include safe config parameter names and versions, not tokens. No login, token creation, authenticated GitHub request or real Telegram/Meta/LLM call was made. HTTPS origin metadata was parsed without emitting the full remote URL; no embedded HTTP credential/query observed.

Before a future tool install, back up only approved configs with mode0700 directory/0600 sensitive files, store outside Git, preserve symlinks instead of following them, and do not echo config/token values in command output. Never use a PAT as a CLI argument. OAuth login and token revocation require user control; do not copy unrelated auth stores as a convenience backup.

Do not assume Codex approval prompts sandbox a local MCP server's filesystem automatically. Its process permissions, mounting/network and credential exposure must be reviewed independently. Read-only GitHub tools do not neutralize prompt injection in issue/PR text; treat returned content as data, not instructions.

## Reviewed vs not verified

Verified now: local versions/config keys/skills metadata, empty Git tracking, local workflow structure, public upstream release/archive/license metadata, official setup syntax.

NOT RUN: app tests/build/migrations/RLS/E2E, extension installation/sign-in, MCP connection/authorization negative tests, skill discovery after changes, RTK benchmark, backup restore rehearsal, dependency scans, upstream test suites and secret-scanner execution.

No global/project config, hook, workflow, extension or business code is changed. No installation, git commit/push/deployment. PHASE J remains blocked on the explicit approval gate.

Protected-file hash checks confirm no change to global TOML, VS Code settings/extensions registry or existing CI. The read-only disabled configuration probe did not create a server. Strict schema validation via codex mcp was unavailable; live tool permissions remain unverified.
