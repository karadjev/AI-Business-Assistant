# Этап 8Г — Build Recovery, Race Detection & Full Multichannel Browser E2E

21.10.2026. Работа в существующем репозитории, строго offline/mock; новые бизнес-функции не добавлялись.

| Итог | Статус |
|---|---|
| STAGE | **8Г** |
| SECURITY STATUS | **CONDITIONAL** |
| TELEGRAM BACKEND E2E | **PASS** |
| WHATSAPP BACKEND E2E | **PASS** |
| TELEGRAM BROWSER E2E — current production | **BLOCKED** |
| WHATSAPP BROWSER E2E — current production | **BLOCKED** |
| GO RACE DETECTOR | **BLOCKED** |
| NEXT.JS PRODUCTION BUILD | **BLOCKED** |
| FORCE RLS | **PASS32 tenant-owned tables** |
| CRITICAL / HIGH / OPEN findings | **0 / 0 / 0 новых в проверенной области** |
| Прежние3 HIGH | **FIXED**, повторные regressions PASS |
| LLM BUDGET / WHATSAPP BUDGET | **0 RUB / 0 RUB** |
| EXTERNAL CALLS | **NONE для выполненных действий этапа** |
| LIVE READINESS | **Telegram NOT READY; WhatsApp NOT READY** |
| MIGRATIONS | **NONE**,00001–00010 сохранены |

BLOCKED не равнозначен PASS. Полная стабилизация/pre-live приёмка не достигнута, система не объявляется production-ready или готовой к реальным покупателям.

## Подготовка и фактическое состояние

Все7 запрошенных документов существуют под точными именами: STAGE_8V_REPORT, BUILD_READINESS_REPORT, MULTICHANNEL_E2E_TEST_REPORT, PRE_LIVE_SECURITY_REPORT, PILOT_TEST_PLAN, STAGE_8B2_REPORT, MULTICHANNEL_ARCHITECTURE. Проверены README, module/dependencies, npm lockfile/scripts, Docker/CI, API/worker, migrations, browser и PG test infrastructure. Предыдущие отчёты использованы как карта, не доказательство текущего PASS.

Один Go module `apps/api`, Go1.26.2; npm/Next16.4.0, React19.3.0, Tailwind/Oxide4.3.3, lightningcss1.32.0. Host Darwin x86_64, Node24.15.0, CGO1 и Apple clang17. Isolated runner Linux x86_64, Go1.26.2 Alpine3.23, default CGO0; browser runner Playwright1.64.0-noble с Node**24.21.0**, не host24.15.0. Existing native packages Darwin-only. Последняя migration10,32 tenant tables; source/channel/business architecture сохранена.

Git status на старте — весь проект untracked, baseline commit нет. Изменения сравниваются с SHA256 snapshot `.local/audit8g/baseline.json`; git add/commit/push не выполнялись. Рабочий `ai-business-assistant-*` stack не останавливался, его DSN не использовалась.

План: исследовать безопасные toolchains → fail-closed native gate → повторить offline backend/regression → запустить isolated production-like backend + явно cached frontend → зафиксировать реальные и отсутствующие evidence → описать точные шаги разблокировки. Архитектура и публичные API не менялись.

## Что изменено

1. `scripts/check-next-offline.cjs`: вместо MISSING+exit0 — **exit42**, проверка ОС/arch/libc, трёх bindings, точных установленных версий и фактической загрузки. Metadata доступна даже при закрытых package.json exports. Успех называется PREREQUISITES_ONLY, не build PASS.
2. `scripts/check-next-offline.test.cjs`:5 tests PASS для missing/mismatch/loading/musl/unsupported runner.
3. `apps/web/package.json`: `prebuild` блокирует Next до потенциального download missing SWC; `test:build-preflight` запускает tests. Lockfile и зависимости не менялись.
4. `infrastructure/compose.build-recovery.yml` + smoke overlay: отдельные aba-audit8g/aba-audit8g-smoke, cached images pull never, isolated PG/tmpfs или own synthetic volume, свежие Go artifacts, no host ports, no Docker socket, internal network/dns loopback/proxies empty. Проверено internal=true и IPv6=false. Go/npm download/telemetry отключены.
5. Четыре новых отчёта и пять актуализаций документов. Бизнес-логика не переписана, новые migrations не нужны.

### Находка G01 — LOW, FIXED

Компонент: прежний offline native preflight. В8В команда печатала MISSING, но exit0 мог быть принят automation за успех; проверялись только2 package names. В8В отчёт явно отделял preflight от build, поэтому прежний production PASS не заявлялся. Исправлено fail-closed gate и prebuild, regression tests PASS. Residual: successful prerequisite check не доказывает успешную сборку, этот предел явно напечатан. Новых CRITICAL/HIGH/OPEN0; это не гарантия отсутствия неизвестных уязвимостей.

## Go race: точная причина BLOCKED

`run --rm race` реально выполнял `CGO_ENABLED=1 go test -race -count=1 ./...`: exit1, `C compiler gcc not found`, до выполнения tests. Проверены compiler paths в cached Go/Playwright/Postgres16/ai-operator images, gcc/clang/cc не найдены. Не делается вывод обо всех существующих image layers.

Host compiler существует. Дополнительный deny-network sandbox для безопасного host запуска проверен **только на /usr/bin/true**, без внешнего probe; sandbox_apply отказал exit71. Никакого повышения прав/обхода. Поэтому host Go tests/build не выполнялись вне гарантированной изоляции. Обычный CGO0 suite не подменяет race. Нужен заранее подготовленный offline compiler runner; точные команды/условия PASS — [GO_RACE_READINESS](GO_RACE_READINESS.md).

## Next production build: точная причина BLOCKED

Реальный `npm run build` в internal runner завершился42 в prebuild: missing/unloadable `@next/swc-linux-x64-gnu@16.4.0`, `@tailwindcss/oxide-linux-x64-gnu@4.3.3`, `lightningcss-linux-x64-gnu@1.32.0`. Native cache tarball entries0 в просмотренном npm cache index. Existing `.next/BUILD_ID` от14.10 не считается новой сборкой. Next compiler не запускался; fresh artifact отсутствует. Host Darwin packages нельзя переносить в Linux, а safe host runner недоступен.

Никаких install/pull/registry metadata lookup/изменений global settings/lockfile. Нужны заранее предоставленные native dependencies и writable **собственная копия** web для build; текущий web-check source read-only. Команды и необходимые версии — [NEXTJS_BUILD_RECOVERY](NEXTJS_BUILD_RECOVERY.md). Current frontend bundle/startup/security и оба production browser E2E **BLOCKED/NOT RUN**, без подмены cached image.

## Результаты выполненных проверок

Prefix для base checks: `docker compose --env-file .local/audit8g/compose.env -f infrastructure/compose.build-recovery.yml`; env содержит только путь к заполненному local Go cache. Smoke prefix дополнительно `-p aba-audit8g-smoke -f infrastructure/compose.build-recovery-smoke.yml`. Рабочий docker-compose.yml не запускался.

| Проверка / команда | Результат | Доказательство |
|---|---|---|
| `config --quiet` base+smoke | PASS | valid compose, no secrets output |
| `run --rm go` | PASS | Go tests/vet/gofmt/build4 binaries; текущий source, no downloads |
| `run --rm integration` | PASS **24.514s** | `.local/audit8g/artifacts/integration.log` |
| RLS32/tenant-safe schema | PASS | `testAuditRLS` real runtime aba_app, foreign SELECT/INSERT/UPDATE/DELETE/WITH CHECK, no BYPASS/ownership, scoped transaction cleanup |
| Migrations1–10 | PASS | clean/repeated/concurrent up/down +9↔10 synthetic preservation; destructive populated down refused; original hashes unchanged |
| Telegram/WA backend E2E | PASS |3 shops×2 channels real local HTTP ingress/wire, shared catalog/agent/orders; own prices/principals/inventory |
| Confirmation/reserve/concurrency | PASS | No order pre-consent, quote/callback dedup, no early reserve, staff reject/confirm, last-unit concurrency, cancel once, price/stock conflict |
| Recovery/security regression | PASS в сценариях | duplicate/invalid HMAC/secret/foreign tenants/malformed/media; real child-process crash/UNKNOWN/no resend, two workers, lease/fencing, retry exhaustion/manual resolution |
| Budgets | PASS | Actual LLM transport counter0; platform cap/member checks; concurrent ledger, crash/unknown/double settlement; WA mock-only CHECK0/policy/window |
| `run --rm race` | BLOCKED | exit1 missing compiler; race tests NOT RUN |
| `run --rm web-check` | PASS | ESLint, tsc noEmit, OpenAPI,5 native preflight tests |
| `run --rm web-check npm run build` | BLOCKED | prebuild42; no new Next artifacts |
| Current TSX/BFF scripts | PASS ограниченный | multichannel/recovery scripts в real local Chromium + current BFF fetch contract |
| Cached browser regression | PASS ограниченный |5 passed36.288s,1 intentional backend-down skip; current Go + cached stage7 web |
| Реальный backend-down browser | PASS1, 7.865s | Отдельный test после stop только тестовых API/worker; не mock response |
| Graceful shutdown/restart | PASS | API/worker exit0, повторный запуск; healthy/readiness/status |
| Полный current production browser TG/WA | BLOCKED | Нет fresh frontend artifact; complete WA browser spec ещё требуется |
| Real Telegram/Meta/LLM, public HTTPS/pricing | NOT RUN | Запрещено; реальные токены/клиенты не использовались |

Parent integration tests реально запускают crash helper subprocesses; их top-level SKIP не означает пропуск родительских crash assertions. Существование теста не приравнивалось к успешному запуску. Race/build toolchain failures не скрыты и не засчитаны PASS.

Полные browser scopes/date/build/results: [FULL_BROWSER_E2E_REPORT](FULL_BROWSER_E2E_REPORT.md). Existing browser tests не дают полного WA current-production coverage: после build требуется отдельный полный WA spec с signed fixtures, window/template/status/handoff/owner/member/three-tenant BFF assertions.

## Секреты, ограничения и доказательства

Runtime mock/kill switches/budgets0 сохранены; tests работают только с synthetic organizations/integrations/credentials. Положительные synthetic ledger fixtures не включают live и не меняют runtime platform budget. Server-only imports API/session/config и restricted BFF forwarding сохранены. Source/имеющиеся старые bundles просканированы без печати значений секретов; **704 файла,0 совпадений** по token/private-key patterns и exact long local-env secrets; исключены deps/cache и файлы>8MB. Это ограниченный scan, не абсолютная гарантия. Current production bundle проверить невозможно до новой build — NOT RUN, не PASS.

`.local/audit8g/artifacts/build-manifest.json` — SHA256 fresh Go artifacts и явный frontend BLOCKED; `integration.log`, `web-check.log`, runtime evidence и browser JSON сохраняются локально. Сеть хоста глобально заблокированной не объявляется. Все исполняемые тесты — isolated containers; локальные provider adapters принудительно loopback. Внешних запросов/платных операций/real tokens/production/commit/push/privilege escalation нет.

После проверок очищены только собственные aba-audit8g ресурсы/тестовый volume; ignored evidence остаётся. Рабочие контейнеры и данные пользователя не затронуты.

## A–T

| Критерий | Результат |
|---|---|
| A состояние репозитория | PASS |
| B Go race | BLOCKED, честно зафиксирован |
| C Next production build | BLOCKED, честно зафиксирован |
| D Go tests/vet/build | PASS |
| E PG integration | PASS |
| F FORCE RLS | PASS32 |
| G migrations | PASS |
| H Telegram backend | PASS |
| I WhatsApp backend | PASS |
| J current Telegram browser | BLOCKED |
| K current WhatsApp browser | BLOCKED |
| L tenant isolation | PASS backend, current browser scope BLOCKED |
| M channel isolation | PASS backend |
| N order confirmation | PASS |
| O reservation | PASS |
| P recovery | PASS в выполненных сценариях |
| Q budgets | PASS |
| R новых open CRITICAL/HIGH нет | PASS в проверенной области |
| S documentation/evidence | PASS |
| T нет внешних/платных/commit/push/deploy | PASS |

## Файлы и неизменность

14 файлов:7 изменены,7 созданы относительно SHA snapshot. Изменены `apps/web/package.json`, `scripts/check-next-offline.cjs`, `docs/BUILD_READINESS_REPORT.md`, `docs/PRE_LIVE_SECURITY_REPORT.md`, оба channel LIVE_READINESS_CHECKLIST, `docs/PILOT_TEST_PLAN.md`. Созданы `scripts/check-next-offline.test.cjs`, `infrastructure/compose.build-recovery.yml`, `infrastructure/compose.build-recovery-smoke.yml`, `docs/STAGE_8G_REPORT.md`, `docs/GO_RACE_READINESS.md`, `docs/NEXTJS_BUILD_RECOVERY.md`, `docs/FULL_BROWSER_E2E_REPORT.md`.

Migrations00001–00010, lockfile, Go module dependencies, business code и OpenAPI не изменялись. Данные пользователя не удалялись. Новый migration11 не нужен.

## Следующий шаг

Сначала предоставить offline toolchains и закрыть race, fresh frontend build, полный current-production browser suite для обоих каналов/трёх магазинов. Для WA также по-прежнему отсутствует live transport; его подготовка и актуальная policy/version/pricing проверка — отдельный будущий этап, не этот offline PASS.

Затем отдельно утверждать synthetic participants/data policy/allowlist/HTTPS/точные provider operations. Ни одного нового разрешения автоматически не запрашивается. Реальные Telegram/Meta/LLM, платные отправки и реальные покупатели остаются запрещены. Работа8Г остановлена после локальных проверок и отчётов; полная pre-live готовность не заявляется.
