# Отчёт ТЗ № 7 — безопасная подготовка live LLM и Telegram

Дата: 2026-10-14. Режим A. **Provider NOT SELECTED; бюджет 0 RUB; активные LLM и Telegram mock; live/external requests запрещены. Production и настоящие покупатели не включались. Commit/push не выполнялись.**

## Что изучено и сохранено

README, отчёт этапа 6, AI_AGENT_IMPLEMENTATION, TELEGRAM_INTEGRATION/SECURITY, MESSAGE_DELIVERY, SECURITY, TENANT_ISOLATION, TECHNICAL_DECISIONS, миграции 1–6, API/worker/Next BFF и тесты. Git status: существующее дерево целиком untracked; удаление пользовательских файлов и переинициализация не выполнялись. Существующий dev volume/стек не использованы для destructive tests. Модульный Go монолит, provider interface, PostgreSQL queue и FORCE RLS сохранены, новые зависимости не добавлены.

## Версии

Сохранены существующие pinned Go1.26.2, PostgreSQL18.3-alpine3.23, Node24.15.0, Next16.4.0/React19.3.0, TypeScript5.9.3, Tailwind4.3.3, pgx5.9.2/goose3.27.1, Playwright1.64.0. Локальный Docker29.4.0/Compose5.1.2; actionlint1.7.7. Поставщик/модель LLM не выбраны; цены поставщика не запрашивались.

## Реализация и схема

Compatible Chat Completions adapter, строгие tools, timeout/context, capped 429 retry, redacted errors, exact HTTPS allowlist + DNS/IP protection. Metered reserve до HTTP и reconcile после ответа. Эвристическое маскирование email/phone, bounded history, electronics-v1. Ответы содержат только проверяемые backend cards/template, свободный текст модели не источник цены. Mock всегда обозначен как тестовый.

Миграция **00007_live_preparation.sql**: глобальная `llm_platform_budget` без runtime grants; tenant FORCE RLS `ai_settings`, `llm_usage`, `llm_reservations`, `agent_metrics`, `ai_settings_events`. Платформенный row lock сериализует короткие reserve/settle транзакции, HTTP вне транзакции; целые microRUB, request/dialog/day/month/platform limits, parallel gate и conservative unknown usage. Схема/readiness=7; 1–6 сохранены. Down на пустой test DB проверен; при сохранённых событиях/расходах/настройках guard запрещает потерю данных.

Telegram guarded transport, обязательный allowlist live пользователей, SQL mode/user filter до создания principal, sending env+org gates, manual operations и encrypted webhook candidate. Startup/migrations/dashboard не выполняют внешние проверки. Изолированный owner purge удаляет синтетический conversation graph и освобождает подтверждённый резерв, сохраняя обезличенные денежные агрегаты.

Защищённая `/dashboard/ai-settings`: mode/model, NOT SELECTED/NOT RUN, лимиты, реальный budget ledger, отдельные mock/live metrics, inbox/outbox lag/retry/errors, webhook из БД, owner budget и отключение auto. Выключение auto также аннулирует in-flight conversation run. Budget форма не активирует live.

## Новые API

Все пути относительно `/api/v1/organizations/{id}`, require session + test organization; staff read, owner write, CSRF и rate limit.

| Метод | Путь | Назначение |
|---|---|---|
| GET | /ai-settings | Состояние конфигурации, бюджета и Telegram из БД |
| PATCH | /ai-settings | Owner settings, expected_version, целые microRUB |
| GET | /agent/metrics | Mode-разделённые метрики и cost state |
| POST | /integrations/telegram/live-operations | verify_token/set_webhook/webhook_info/delete_webhook, сейчас403 без сети |
| POST | /conversations/{resource}/purge-test-data | Owner explicit synthetic deletion с version и отдельным confirm |

OpenAPI 0.7.0 описывает реализованные маршруты и ошибки. Новые manual routes не означают выполненные live E2E.

## Созданные файлы

- `apps/api/migrations/00007_live_preparation.sql`.
- `apps/api/internal/config/live.go`, `live_test.go`.
- `apps/api/internal/agent/metered.go`, `network.go`, `live_test.go`, `evaluation_test.go`, `testdata/evaluation_v1.json`.
- `apps/api/internal/costcontrol/service.go`, `service_test.go`.
- `apps/api/internal/platform/network/client.go`, `client_test.go`.
- `apps/api/internal/http/stage7.go`.
- `apps/api/internal/telegram/live.go`, `live_test.go`, `operations.go`.
- `apps/api/tests/integration/stage7_test.go`.
- `apps/web/src/app/dashboard/ai-settings/page.tsx`, `apps/web/src/components/AISettings.tsx`.
- `infrastructure/compose.smoke.yml`: явный offline overlay для изолированной контейнерной браузерной проверки.
- `docs/LIVE_LLM_INTEGRATION.md`, `LLM_COST_CONTROL.md`, `AI_EVALUATION.md`, `PILOT_READINESS.md`, `DATA_RETENTION_POLICY_DRAFT.md`, `STAGE_7_REPORT.md`.

## Изменённые файлы

- Go: `cmd/api/main.go`, `cmd/worker/main.go`; `internal/config/{agent,telegram}.go`; `internal/agent/{provider,orchestrator}.go`; `internal/conversations/service.go`; `internal/telegram/{service,worker}.go`; `internal/http/{router,identity}.go`; `internal/platform/db/db.go`.
- Existing tests: config Telegram; PostgreSQL `tests/integration/{database,stage5,stage6,identity}_test.go` (schema7/новые function signatures; resend limiter перестал зависеть от более раннего запроса в той же минуте).
- Web: same-origin `src/app/api/[...path]/route.ts`, `src/components/Sidebar.tsx`, `tests/auth.spec.ts`.
- `.env.example`, `docker-compose.yml`, `.github/workflows/ci.yml`, `scripts/check-openapi.mjs`, `scripts/{smoke,smoke-compose}.sh`, `packages/contracts/openapi.yaml`.
- README и docs API_DESIGN, DATABASE_DESIGN, AI_AGENT_IMPLEMENTATION, TELEGRAM_INTEGRATION, LOCAL_DEVELOPMENT, TECHNICAL_DECISIONS, SECURITY, TENANT_ISOLATION. Старые отчёты не заменены.
- Игнорируемый локальный `.env`: принудительные Mode A значения, сохранены DB/auth/encryption credentials; реальные LLM/bot credentials не добавлялись.

## Проверки

| Проверка | Итог | Доказательство / границы |
|---|---|---|
| Go unit + race | PASS | `go test -race -count=1 ./...`; adapter fixtures только localhost |
| Go format / vet / builds | PASS | gofmt без изменений; vet; api/worker/migrate/healthcheck builds |
| PostgreSQL integration + race | PASS | Изолированная aba_test, полный TestDatabaseAndMigrations этапов3–7; финальный прогон18.15s тест /19.266s пакет |
| Миграция7 up/repeat/concurrent/down | PASS | Чистая test DB, roundtrip до1 и повторный up, data-loss guards |
| Budget concurrency / RLS | PASS | 8 reserve одной организации + конкуренция двух организаций на общий cap; foreign SQL, FORCE, unknown/idempotent settle |
| Synthetic purge | PASS | Financial link anonymized; реальные test order/items/events/quotes удалены, employee reservation освобождён, on_hand сохранён |
| External gates / token allowlist / kill | PASS | Spy=0 при текущих defaults; SQL не создаёт principal для прежнего queued sender вне allowlist |
| 30 evaluation common guards | PASS | Все30 messages: schemas, no implicit mutation, unapproved provider denied |
| Качество30 live диалогов | NOT RUN | Настоящая модель не выбрана; common guards не заменяют NLP/business evaluation |
| Frontend lint/typecheck/build | PASS | Native lint/TS; production Docker build после исправления SSR чтения JSON |
| Browser smoke | PASS | 5 Chromium tests,43.6s; полный auth/catalog/orders/mock Telegram/AI settings workflow и error409 |
| OpenAPI | PASS | Swagger Parser + точный набор реализованных путей |
| Compose config / build / healthy | PASS | Прямые команды Compose; API/worker/web/PG/Mailpit healthy, migrate exit0 |
| Backend/database outages + SIGTERM | PASS | PG stop→ready503/live200; настоящий остановленный backend→UI unavailable (1 test,1.9s); API/worker exit0 |
| CI static | PASS | actionlint1.7.7; workflow не запускался на GitHub |
| Secret scan | PASS | 184 publishable файлов сверены с локальными secret values без вывода значений; frontend не имеет key/token env;352 app log lines без checked sensitive values/полной тестовой переписки |
| GitHub CI execution | NOT RUN | Commit/push запрещены |
| Real getMe/setWebhook/info/messages | NOT RUN | Telegram external NOT AUTHORIZED |
| Live LLM / paid requests | NOT RUN | Provider NOT SELECTED, budget0, live disabled |
| Правовая/поставщик quality проверка | NOT RUN | Draft policy, настоящий provider/infra ещё не выбраны |

Промежуточные проблемы не скрывались: первая auth rate-limit проверка зависела от границы минуты; исправлена проверка, не отключён limiter. В новом smoke overlay YAML преобразовал незаключённый в кавычки синтетический hex key в число; key теперь строка. Browser обнаружил двойное чтение Response в SSR; исправлено единственным чтением. Native wrapper с socket bind/Docker доступом ограничен sandbox; стандартные разрешённые Go/Compose команды позволили выполнить проверки.

## Критерии A–T

| Критерий | Результат | Граница |
|---|---|---|
| A real adapter | PASS | Протокол на local fixture, real API NOT RUN |
| B mock preserved | PASS | Unit/PG и старые mock workflows |
| C live off | PASS | Закрытые defaults и factory |
| D monetary limits | PASS | Нулевой budget + PG ledger gates |
| E parallel budget | PASS | Общая блокировка и two-org конкурентный тест |
| F tools authorization/schema | PASS | Strict validation + existing scoped backend |
| G facts | PASS | Проверенные mock/tool данные; live NLP quality NOT RUN |
| H explicit consent | PASS | Existing UI/inline quote flow; no free-text consent |
| I staff reserve | PASS | PG regression; pending без резервирования |
| J Telegram prepared | PASS | Manual gated methods; реальные операции NOT RUN |
| K webhook | PASS | Existing secret/binding/dedup tests |
| L inbox/outbox | PASS | PG stage6 regression с новыми signatures |
| M handoff | PASS | PG stop/resume и auto-off |
| N tenant/RLS | PASS | Two tenants, forced policies, runtime isolation |
| O AI dashboard | PASS | Настоящий authenticated BFF, Mode A, auto on/off и error409; positive budget/live UI не активированы |
| P monitoring | PASS | SQL aggregates, mode separation, structured IDs; real-provider metrics пока отсутствуют |
| Q available checks | PASS | Go/web/PG/OpenAPI/Compose/Chromium/actionlint; live/legal checks отдельно NOT RUN |
| R old functions | PASS | Полная PG regression3–6; browser regression также PASS |
| S docs | PASS | Шесть документов, contracts/config/commands |
| T no unauthorized live | PASS | Реальных Telegram/LLM вызовов не выполнялось |

## Ограничения и дальнейшие разрешения

Результаты30 кейсов и критерии качества: [AI_EVALUATION](AI_EVALUATION.md). Только ограниченные common guards PASS для всех30; реальные диалоги/quality NOT RUN. Совместимый adapter не гарантирует качество/совместимость любого поставщика. Unknown usage/pending reserve остаётся conservative; crashed pending требует ручной финансовой сверки. Platform row mutex ограничивает throughput, намеренно достаточен MVP. Financial estimate не является точным provider bill.

Webhook setup не атомарен с БД, при ошибке candidate сохраняется как disabled; требуется ручная reconciliation. Exactly-once внешней отправки невозможно гарантировать. answerCallbackQuery transport подготовлен, отдельный надёжный callback ACK ещё не связан с worker. Retention7д очищает payload автоматически; полное удаление графа — отдельная owner операция. Отзыв токена/внешнее удаление Telegram сообщений не имитируются.

Для следующего действия с внешними API требуется **новое отдельное разрешение**: выбор provider/model/allowlist URL; key и тарифы; положительный денежный budget и DB/org approval; LLM external requests; тестовый бот и согласованные Telegram user IDs; getMe; set/get/deleteWebhook; отправка тестовых сообщений; HTTPS origin/dashboard/webhook и способ публикации; live E2E. Для настоящих покупателей — отдельная privacy/legal policy и infrastructure/provider регион. Production/commit/push также не разрешены.

Рекомендуемый следующий этап: утвердить поставщика/лимит/тестовый HTTPS и участников, затем короткий отдельно разрешённый live E2E с синтетикой по [PILOT_READINESS](PILOT_READINESS.md). Для предпринимателя сначала review draft данных и оценка ответов; готовность Mode A не означает готовность production.

## Воспроизводимые команды

```sh
# Native checks, no real keys:
sh scripts/check.sh
# If sandbox rejects socket access through a shell wrapper, use direct approved commands:
docker compose --env-file .env.example config --quiet
docker compose -p aba-stage7-integration -f infrastructure/compose.test.yml up --abort-on-container-exit --exit-code-from integration integration
# Delete ONLY this isolated project after completion:
docker compose -p aba-stage7-integration -f infrastructure/compose.test.yml down --volumes

# Offline browser overlay. Always a separate -p project, never the dev project:
docker compose --env-file .env.example -p aba-stage7-smoke -f docker-compose.yml -f infrastructure/compose.smoke.yml up --build -d --wait api worker web mailpit
docker compose --env-file .env.example -p aba-stage7-smoke -f docker-compose.yml -f infrastructure/compose.smoke.yml run --rm --no-deps browser
docker compose --env-file .env.example -p aba-stage7-smoke -f docker-compose.yml -f infrastructure/compose.smoke.yml down --volumes
```

Не повторять test integration на непустой/пользовательской БД. Safety marker/имя aba_test и роли проверяются. Smoke overlay требует Compose с `!reset` support (2.24.4+), synthetic server key в нём не является Telegram credential. Новые тестовые стеки будут удалены; исходный dev стек/volume сохранены, без автоматической миграции его существующих данных.

Ранее автоматический контроль отклонил повышение sandbox-доступа для Docker: категория sandbox_approval запрещена профилем. Проверки удалось выполнить обычными разрешёнными Go/Docker Compose командами без повышения.
