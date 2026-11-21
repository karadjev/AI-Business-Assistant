# AI Development System — диагностика

Дата: 2026-10-29. Выполнены только PHASE A–I. Это read-only обследование плюс семь согласованных документов, не отчёт об установке. Команды с модификацией настроек, сборки, тесты приложения, миграции, provider API и package installation в этой фазе не запускались. Публичное web/GitHub исследование явно разрешено текущим ТЗ и не включает runtime Telegram/Meta/LLM.

## Подтверждённое окружение

| Компонент | Наблюдение |
|---|---|
| OS / architecture | macOS15.7.9, build24G830; x86_64/Intel |
| Hardware | Intel Core i7-8750H @2.20GHz;12 logical CPUs;16 GiB RAM |
| VS Code |1.143.1; commit e646868227c3e3a9e3f24277c8c7220be42c0be7; x64 |
| Codex CLI |0.165.0; @openai/codex; доступный entrypoint находится в ~/.npm/_npx/.../node_modules/.bin/codex |
| Codex Desktop |26.422.21637, build2056, bundle com.openai.codex |
| Codex в VS Code |NOT INSTALLED в проверенном профиле: openai.chatgpt отсутствует и в CLI inventory, и в ~/.vscode/extensions |
| Git |2.50.1, Apple Git-155 |
| Python |3.14.5; python3.13 не найден в PATH |
| Node / npm |24.15.0 /11.12.1; user-local Node prefix |
| Go |1.26.2 darwin/amd64 |
| gopls |0.23.0 в ~/go/bin/gopls, не в PATH этой сессии |
| Docker CLI / daemon |29.9.2 /29.9.2; daemon version получен read-only после разрешения socket access |
| Docker Compose |5.6.1 |
| PostgreSQL CLI |psql/pg_dump NOT FOUND в PATH; проверенные libpq/PostgreSQL18 Homebrew locations отсутствуют. Image pin18.3 — не host tooling |
| Homebrew |6.0.12 |
| uv / uvx / gh / RTK / Cargo |NOT FOUND в PATH; ничего не устанавливалось |
| ripgrep |15.2.0, daemon-provided codex-path |
| govulncheck / gitleaks / semgrep / staticcheck / trivy |NOT FOUND в PATH |

PATH lookup не доказывает отсутствие программы на всём диске. История способа установки Desktop/CLI UNKNOWN: путь npm cache подтверждён, факт постоянной глобальной npm-установки не установлен. Не заменять рабочий CLI только из-за этого.

## VS Code extensions

26 установленных extensions; никаких изменений:

- bradlc.vscode-tailwindcss0.16.0; christian-kohler.path-intellisense2.10.0.
- dbaeumer.vscode-eslint3.0.34; editorconfig.editorconfig0.18.2; esbenp.prettier-vscode12.4.0.
- expo.vscode-expo-tools1.6.3; github.vscode-github-actions0.32.3.
- golang.go0.58.0; honnamkuan.golang-snippets1.2.0; neonxp.gotools0.1.5; zsh.go-snippets0.0.4.
- graphql.vscode-graphql0.13.4; graphql.vscode-graphql-syntax1.3.13.
- humao.rest-client0.25.1.
- ms-azuretools.vscode-containers2.5.3; ms-azuretools.vscode-docker2.0.0.
- ms-ceintl.vscode-language-pack-ru1.131.2026090407.
- ms-python.debugpy2026.6.0; ms-python.python2026.10.0; ms-python.vscode-pylance2026.4.1; ms-python.vscode-python-envs1.40.0.
- ms-vscode-remote.remote-containers0.469.0.
- ms-vscode.vscode-typescript-next6.0.20260416; typescriptteam.native-preview1.0.1.
- msjsdiag.vscode-react-native1.13.0; prisma.prisma31.13.0; redhat.vscode-yaml2.0.0.

Go snippets overlap; two experimental TypeScript extensions can disagree with project TypeScript5.9.3. This is a potential overlap, not a reproduced fault. Do not uninstall them globally: they may serve other projects. Prefer workspace TypeScript after an approved settings review. Presence of an extension is not a functional test of its language server.

## Instructions, config, MCP, skills

- ~/.codex/config.toml exists, regular file, mode0600,1168 bytes.
- Safe keys: model=gpt-6.1-sol, model_reasoning_effort=low, approvals_reviewer=user; current project trust_level=trusted.
- Other top-level parameter names: projects, plugins, tui, notice. Their full values are not reproduced.
- CODEX_HOME override unset. Global AGENTS.md / AGENTS.override.md absent.
- Project AGENTS.md / AGENTS.override.md, .codex/, .agents/ absent; relevant nested AGENTS.md not found by repository search. Ancestors /Users, home and Desktop checked.
- ~/.agents/skills/ absent. Five system skills found: review-agent, skill-creator, skill-installer, openai-docs, imagegen. Only openai-docs applied to this diagnostic task.
- Native github@openai-curated plugin enabled; cached plugin0.1.6, cache revision d6169bef, four skills: github, gh-fix-ci, gh-address-comments, yeet.
- Its MCP manifest points to api.githubcopilot.com, HTTP with bearer-token environment reference; no read-only header was configured in that manifest. Token value, auth stores and Keychain were not read. Authentication/actual granted permissions UNKNOWN.
- codex mcp list --json returned[]: zero standalone configured MCP servers. This does not mean zero plugin-provided or host-provided tools.
- Host session exposes Figma/Sites/Data/plugin-management and related tool/skill metadata. No GitHub/Context7/Serena tool names were exposed in the inspected host tool catalog.
-67 cached plugin SKILL.md files found across native/remote cache; cache presence is not enablement. No Superpowers/ECC/Serena/RTK installation found in inspected configuration. Avoid loading whole cache.
- ~/.codex/hooks.json, project .vscode/settings.json absent. No inline hooks key in inspected global TOML.
- Available CLI diagnostic commands confirmed with help: doctor (including redacted --json/--summary), debug, mcp list/get, plugin list. doctor runtime/auth probes were NOT RUN; no account login or model request was made.

Дополнительная проверка drafts:3 TOML blocks parse PASS; combined global/project AGENTS drafts3067 UTF-8 bytes. Disabled ephemeral CLI override parsed with enabled=false without writing a server entry or connecting. codex --strict-config is not supported for codex mcp in0.165.0; this is NOT a successful strict schema/connection test. Sandbox also refused CLI PATH-alias creation warnings; no permission was granted to create aliases.

CLI/IDE share Codex configuration, but host app tools and native plugin inventories differ. Configuration syntax is checked against [official reference](https://developers.openai.com/codex/config-reference); instruction discovery against [AGENTS.md docs](https://developers.openai.com/codex/guides/agents-md).

## Git and CI

Branch main, origin uses HTTPS/github.com with no embedded HTTP credentials or query. HEAD does not exist; git ls-files reports0. Application/config/docs are untracked. This is a real rollback/CI reproducibility gap, not permission to commit. Do not use git reset/clean/restore as backup. Remote repository contents and deployed GitHub Actions status UNKNOWN; no authenticated GitHub operation was attempted.

Local .github/workflows/ci.yml exists with contents:read, concurrency and Go/web/integration-smoke jobs; actions currently pinned by version tags, not immutable commit SHA. It uses npm ci and Playwright browser downloads: it is not a strict-offline runner. No deployment job or pull_request_target seen. Details and proposed patch are in INSTALL_PLAN.

## Project component map

| Existing component | Evidence path |
|---|---|
| API / worker / migrations |apps/api/cmd/{api,worker,migrate,healthcheck}; apps/api/migrations |
| PostgreSQL runtime/RLS |apps/api/internal/platform/db; tests/integration/audit_test.go |
| Next.js / session BFF |apps/web/src/app; src/app/api/[...path]/route.ts |
| Equal channel adapters / common engine |internal/telegram, internal/whatsapp, internal/channel |
| Inbox/outbox / DELIVERY_UNKNOWN |channel and WhatsApp migrations/services; reliability/crash tests |
| Financial admission / trusted reconciliation |internal/whatsapp/{financial,provisioning}.go and integration tests |
| Catalog / Excel / onboarding |internal/catalog/{service,xlsx,import,onboarding}.go |
| Freshness / source/retention |internal/catalog/control.go; migration00014_catalog_control_center.sql |
| Browser E2E |apps/web/tests/{catalog-control,multichannel,onboarding}-production.spec.ts |
| Build integrity |scripts/build-env/artifacts.cjs; production.sh; browser.sh |
| Security history |docs/STAGE_10B_REPORT.md; FINAL_OFFLINE_SECURITY_GATE.md |

Last migration14; prior report records45 FORCE RLS tables and offline PASS, security CONDITIONAL,0/0/1 open findings, E01 OPEN. These are historical results from10Б, NOT rerun results of this task. Runtime flags/config untouched; LLM/WhatsApp budgets remain0 RUB by existing policy.

Current application tests/build/migrations/RLS/E2E: NOT RUN in this read-only phase. MCP connectivity: NOT TESTED. RTK benefit/token usage: UNKNOWN.

## Change boundary and integrity

Only the seven AI_DEV_SYSTEM_*.md documents are authorized writes now. Global TOML/settings/extensions/workflow/AGENTS/skills/application files are not changed. A351-file application/existing-doc/config source fingerprint was collected excluding these seven future reports: SHA25674b14af9e932359d0ee0b5ec229486991eeeb3df038af5c3b379933782ddf8a4. Diagnostics deliberately exclude .env files, auth stores, keychains and private keys. No dependency downloads, app API calls, git commit/push or deployment.

Final protected-file check: global TOML, VS Code settings/extensions registry and existing workflow hashes are unchanged. Agent instruction files/directories remain absent; only the seven report documents are created.
