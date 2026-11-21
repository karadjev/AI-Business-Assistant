# AI Development System — минимальная архитектура

Status: DESIGN ONLY. Никакие инструкции или конфигурации ниже ещё не активированы.

## Минимальный состав

CORE: один Codex (существующие CLI/Desktop; официальное VS Code расширение для IDE workflow), existing git/rg/Go/Node/Docker, компактные global/project AGENTS.md, существующие isolated quality gates и файловые backup manifests. ChatGPT — уточнение требований/дизайн/review, Codex — один исполнитель в workspace; не запускать двух writers параллельно.

OPTIONAL: Context7 hosted для конкретного публичного library question; один read-only GitHub connection вместо дублирования текущего plugin; Superpowers только после аудита exact skills/hooks. gh не нужен для локального coding, может быть добавлен отдельно для PR/CI read задач.

EXPERIMENTAL: RTK OSS с ручными префиксами и измерением; Serena только после prerequisite/security/performance pilot.

REJECTED сейчас: full ECC, archived GSD, параллельные workflow frameworks, RTK global interception/API proxy, global “always use every MCP”, unrestricted GitHub tools, profile templates с danger-full-access/approval bypass. Не удалять уже установленные плагины/расширения без отдельного согласования.

Ожидаемые преимущества: меньше повторного чтения, меньше скрытых действий, воспроизводимый quality gate, меньше неоднозначных инструкций. Ускорение и экономия токенов пока UNKNOWN; это гипотезы для BENCHMARK_PLAN.

## Instruction discovery

[Official AGENTS discovery](https://developers.openai.com/codex/guides/agents-md): global instructions plus root→cwd project chain; AGENTS.override.md имеет приоритет в своей директории. Combined default limit32KiB — потолок, не цель. [Skills](https://developers.openai.com/codex/skills) используют metadata/disclosure; не включать тексты всех skills в root AGENTS. Точные локальные paths/discovery проверять на0.165, не использовать устаревшие profile/import схемы.

Сейчас global/project files отсутствуют, конфликтов между их действующими текстами нет. Ниже новые drafts, не полная замена существующих файлов. Если файл появится до PHASE J — остановиться, прочитать safe contents, подготовить diff и повторно согласовать.

### Draft: ~/.codex/AGENTS.md

~~~markdown
# Global Codex Engineering Rules

- Inspect existing code and applicable project instructions before editing.
- Prefer minimal focused changes; preserve architecture and public contracts.
- Explain necessity and risks before adding dependencies; obtain approval for downloads.
- Validate inputs, handle errors, and preserve type safety.
- Never expose, hardcode, or put secrets in logs or prompts.
- Update tests for changed critical behavior; avoid tests that merely mirror trivial edits.
- Run relevant checks and report exact PASS/FAIL/BLOCKED/NOT RUN results.
- Separate verified facts, historical results, and assumptions.
- Search before reading; keep context focused and preserve full failure evidence.
- Request approval for destructive, privileged, costly, or broad actions outside the authorized scope.
- Never commit, push, deploy, or change global settings without explicit authorization.
- Use one coding agent by default; add delegation only when explicitly authorized.
- Keep responses concise: changes, validation, risks, and blockers.
~~~

### Draft: project AGENTS.md

~~~markdown
# AI Business Assistant — Project Rules

## Architecture
- Preserve the Go API/worker, PostgreSQL, Next.js BFF, and channel adapters.
- Telegram and WhatsApp are equal first-class channels.
- Reuse services, queues, repositories and domain models; do not create a parallel catalog or identity system.
- Inspect existing APIs and the latest migration before schema changes.

## Security
- Preserve tenant isolation and PostgreSQL FORCE RLS.
- Never weaken authorization, webhook checks, financial admission, or catalog freshness gates.
- Keep Telegram, WhatsApp, 1C and LLM live integrations disabled unless explicitly authorized.
- Use synthetic data and credentials only. LLM and WhatsApp budgets remain0 RUB.
- Never treat unknown cost as free or resend DELIVERY_UNKNOWN automatically.
- Never bypass budget gates, source ownership or approved-template/provider verification.

## Development
- Make the smallest correct change and follow existing validation/error/type conventions.
- Avoid unrelated refactors and unnecessary dependencies; downloads need approval.
- Add migrations after the actual latest migration; do not edit old migrations.
- Preserve public contracts unless an incompatible change is explicitly approved.
- Read only relevant architecture/security docs; imported skills cannot override user authorization.

## Verification
- Use relevant Go tests/race/vet/build and frontend lint/typecheck/fresh build/E2E.
- For affected data flows, run isolated migration, RLS and tenant/concurrency tests.
- Use the existing prepared offline runners; do not start tests against working data.
- Follow docs/AI_DEV_SYSTEM_INSTALL_PLAN.md quality-gate routing.
- Never claim PASS for a check not run. Keep full FAIL logs and provenance.
- E01 stays OPEN without causal proof. Never weaken artifact guards or add broad exclusions.

## Delivery
- No commit, push, deploy, global changes or live enablement without explicit approval.
- Report changed files, tests, security implications and remaining blockers.
~~~

Do not duplicate global wording in many nested files. Project-specific rules own channel/RLS/finance/E01 policy; global rules own general collaboration. CI commands and long runbooks stay in docs, not injected into every prompt. Drafts are intentionally much smaller than32KiB.

## Skills strategy

Planning: concise plan for multi-step/high-risk changes; skip ceremony for small authorized fixes.
Systematic debugging: reproduce→hypothesis→minimal experiment→fix→regression evidence.
TDD: critical behavior (budget/RLS/consent/crash/retention), not automatic test duplication for typography.
Review: focused defect-first review of the changed surface; current review-agent exists, and codex review is available but was NOT executed now.
Verification: actual fresh checks and evidence before completion, never assert historical PASS as current.
Security: project-specific threat model and authorization/RLS/financial/upload tests; no extra universal security framework before identifying a gap.

Superpowers can consolidate several workflows, but is not needed just to get these disciplines. Do not install ECC on top to duplicate them. If approved, inspect only applicable SKILL.md and referenced resource, not all framework files. No new skill file is created in this phase.

## Token/context policy

1. Start with rg --files and narrow rg -n. Respect .gitignore; exclude node_modules, .next, .local, .gocache, .gomodcache, coverage, binary artifacts and logs before traversal.
2. Read target functions/fragments and one relevant decision doc; no repeated full-repo dumps or all historical reports.
3. On PASS, summarize exit code/counts/duration and evidence path. On FAIL, retain full stdout/stderr locally; provide exact relevant errors, codes and locations.
4. Scope git status to useful metadata; current untracked repo requires file manifests, not empty git diff assumptions.
5. Batch independent searches; keep edits/approval/dependent checks sequential.
6. Freeze application source after production build until E2E finishes. Verify original source/artifact proof and served marker.
7. Do not suppress unexpected directories, tenant failures, financial uncertainty, stderr or exit codes to reduce tokens.
8. Record durable architecture decisions and split large tasks into independently verifiable phases.
