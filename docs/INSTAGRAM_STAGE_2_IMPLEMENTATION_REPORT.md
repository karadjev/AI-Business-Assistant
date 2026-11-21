# Instagram Stage 2 — durable backend foundation

Статус: **STAGE 2 PARTIALLY COMPLETE**.

Реализация подготовлена в коде, но Go compilation, migration up/down, PostgreSQL isolation и регрессии не подтверждены. Принимать Stage 2 как COMPLETE или применять миграцию к production нельзя на основании этого отчёта.

## Аудит и границы

Проверены Git/status и незакоммиченные изменения, все Stage 1 Instagram Go-файлы/тесты и отчёты, migrations 00010–00016, Go API/worker lifecycle, transaction-local tenancy, capabilities, RLS, webhook/auth middleware, shared conversation/AI/order path, BFF и frontend settings/inbox.

Рабочее дерево изначально содержало многочисленные staged/unstaged/untracked изменения. Они сохранены; полный исходный status — `.local/instagram-stage2/git-status-before.txt`. Предыдущие пять Instagram reports не изменены; hash-проверка сохранена отдельно. Старые migrations не редактировались. Commit/push/deploy и глобальные настройки не выполнялись. Новых зависимостей и скачиваний нет.

## Что реализовано

### Persistence и migration 00017

Добавлена `apps/api/migrations/00017_instagram_durable.sql`, Goose Up/Down в транзакции:

- `instagram_connections`: organization/integration/account, username, connection status, token/OAuth expiry, authorization epoch, permissions, subscription, observed event timestamp, creation/update/revocation metadata.
- `instagram_oauth_states`: только hash state, organization/integration/user/session hash/redirect, expiry и consumption timestamp; PK и индексы. Runtime не имеет прямого SELECT/INSERT к state store.
- Обе tenant-таблицы имеют ENABLE/FORCE RLS. Shared connection secret остаётся в `channel_integrations.secret_cipher`, без runtime SELECT privilege.
- AES-256-GCM в приложении связывает ciphertext с tenant/integration/account/Instagram Login. Токены, state, коды и provider bodies не записываются в audit/logs.
- `(organization_id,channel)` заменяет старое предположение «одна channel integration на магазин». Account сохраняет глобальную уникальность маршрутизации; составные FK связывают tenant/integration/channel/account. Чужой account нельзя перепривязать.
- Goose повторный Up управляется version table/session advisory lock, а не `IF NOT EXISTS`, скрывающим schema drift. Existing migration integration runner дополнен для roundtrip 17→16→…→1→17 и повторного/concurrent Up.
- Down восстанавливает сохранённые определения predecessor-функций. При provider/state/conversation/order/catalog данных rollback **отказывается** работать вместо удаления данных. Up/Down не запускались в этой сессии.
- Добавлены ограниченные lock/statement timeouts. Применение требует согласованного обновления API/worker до schema 17; rolling compatibility не доказана.

### Общая очередь и worker

Instagram использует существующие `channel_inbox`, `channel_outbox`, `channel_principals`, `channel_events`; отдельной Instagram queue нет.

Inbox job имеет tenant/integration/channel/external account, event ID (`external_id`), idempotency/correlation key (`id`), conversation после claim, status/attempts/available_at/lease/capability. FK и binding trigger проверяют account, channel и tenant. Claim переопределяет tenant/integration/channel/user/chat/conversation/correlation из SQL-authoritative rows, а не payload.

Идемпотентность: существующий unique `(integration_id,external_id)`, стабильный UUID correlation и existing conversation message idempotency. Webhook acknowledgement возвращается после commit ingress+event observation. Ошибка БД даёт retryable HTTP failure, а не ложное принятие. Повреждённые отдельные события не мешают корректным соседям нормализованного batch; отказ общей структуры/account отклоняет запрос.

Добавлен `channel_finish_classified`: bounded exponential backoff, максимум 5 attempts; permanent/exhausted jobs переходят в `failed` (существующее durable dead-letter состояние). Expired/revoked Instagram connection отменяет pending/processing jobs и capability. Payloads очищаются существующим retention purge, terminal identifiers сохраняются; automatic DLQ replay не добавлен.

`instagram.Service` реализует `channel.Queue` и зарегистрирован в существующем worker рядом с Telegram/WhatsApp. Получение вызывает общий `channel.Engine` → conversations → существующий AI/catalog/orders. Нет нового AI/catalog/identity pipeline. Graceful cancellation, bounded independent finalization и structured correlation logs используют текущие соглашения.

`SendOne` намеренно ничего не отправляет: kill switch обязателен, Instagram outbox records создаются как `cancelled`. DELIVERY_UNKNOWN не переотправляется, API delivery success не имитируется, токен worker для отправки не читает. Поддержка полноценного Instagram outbound — следующий этап.

**Граница унификации:** один worker/`channel.Queue` lifecycle обслуживает три канала, общая SQL queue — WhatsApp и Instagram. Telegram сохраняет прежний `telegram_*` repository и fenced sender. Физическая миграция Telegram в `channel_*` не выполнялась: это потребовало бы отдельного изменения его security/lease/delivery semantics. Если acceptance подразумевает одну физическую таблицу для всех трёх каналов, этот пункт ещё не выполнен.

### Совместимость WhatsApp

WhatsApp status/injection/control/credentials/send/financial helpers явно ограничены своим каналом; они больше не выбирают произвольную integration магазина. Финансовые gates, budgets и template/provider verification не отключены. Копирование predecessor definitions для Down сохраняет изменения 00011/00012. Добавлен coexistence test WhatsApp+Instagram в одном tenant. Это подготовленный код и тест, не PASS регрессии.

### Зарегистрированные endpoints

При обычной регистрации authenticated API доступны:

| Метод | Путь |
| --- | --- |
| GET | `/api/v1/organizations/{id}/integrations/instagram` |
| POST | `/api/v1/organizations/{id}/integrations/instagram/oauth/start` |
| GET | `/api/v1/organizations/{id}/integrations/instagram/oauth/callback` |
| POST | `/api/v1/organizations/{id}/integrations/instagram/disconnect` |
| GET | `/api/v1/organizations/{id}/integrations/instagram/webhook-status` |
| GET, POST | `/api/v1/webhooks/instagram/{integration}` |

Protected routes используют existing session/Origin/CSRF wrapper; connection operations дополнительно owner-only и test-tenant-only. Callback требует authenticated инициатора и одноразовый tenant/user/session/redirect-bound state. Состояние consumed сохраняется и при корректном state с отказом пользователя. Неизвестный/чужой state не меняет connection.

Webhook GET проверяет raw text challenge, но не подтверждает получение события. Middleware сохраняет explicit text/plain для challenge; JSON default и timeout error contract сохранены. POST проверяет HMAC/headers/size до account lookup и enqueue; AI внутри HTTP не выполняется. Account берётся из защищённого mapping, неизвестный/revoked/expired account отклоняется.

Runtime **только MOCK**. Start возвращает локальный synthetic callback URL; Meta OAuth/HTTP не вызываются. Synthetic provider подтверждает simulated permissions/subscription; `connected · MOCK` появляется только после подписанного тестового inbound event текущего authorization epoch; повтор старого persisted event не подтверждает новое подключение. Это не подтверждение реального Meta subscription. Stage 1 official wire fixtures сохранены. Подключение живого provider требует отдельного этапа и разрешения.

### Frontend

BFF allowlist включает новые protected routes. Карточка и settings читают backend state, доступны владельцу test store; поддержаны not connected/connecting/connected MOCK/reauthorization/webhook error/permission error. Неизвестный status/недоступная БД не превращаются в успех. Mock-only ограничения видимы пользователю.

В общий inbox добавлены Instagram filter/label/type и отображение attachment type metadata без загрузки URL. Ответы сохраняются локально, не доставляются; UI это явно сообщает. Добавлять отдельный Instagram inbox не потребовалось.

## Конфигурация

По умолчанию `INSTAGRAM_ENABLED=false`. Для изолированной synthetic проверки разрешён `INSTAGRAM_ENABLED=true` только вне production, с 32-byte hex `INSTAGRAM_ENCRYPTION_KEY` и `mock:`-prefixed synthetic APP_SECRET/VERIFY_TOKEN. Пример не содержит реальных credentials.

Live/external flags остаются false, kill switch true, budget 0. `INSTAGRAM_ACCESS_TOKEN` в environment запрещён; токен попадает только в encrypted persistence через mock callback. Production Instagram runtime отвергается конфигурацией. Общие Telegram/WhatsApp/LLM gates сохранены.

## Проверки текущей сессии

| Gate | Результат | Evidence |
| --- | --- | --- |
| gofmt изменённых Go-файлов | PASS | `format.log`, пустой список |
| Frontend ESLint | PASS | `frontend-lint.log`, exit 0 |
| Frontend TypeScript | PASS | `frontend-typecheck.log`, exit 0 |
| OpenAPI schema + exact registered path set | PASS | `openapi.log`, exit 0 |
| Первый ESLint запуск | FAIL, исправлен | `frontend-lint-attempt1.log`: empty catch; original `frontend-checks.json` сохранён |
| Go unit | BLOCKED | active host Go 1.26.2, module требует ≥1.26.11; exit 1 до тестов |
| Go race/vet/build | BLOCKED | тот же toolchain gate, отдельные полные логи |
| PostgreSQL integration | BLOCKED | command остановлен до тестов; Docker daemon/socket также недоступен |
| Migration up/down/repeat/concurrent | BLOCKED | тестовый runner не запущен; SQL syntax/runtime не подтверждены PostgreSQL |
| Три tenant RLS/repository/queue/worker/HTTP | BLOCKED | тесты написаны, PostgreSQL выполнение отсутствует |
| Telegram regression | BLOCKED | тесты не запустились; не считать исторические PASS доказательством |
| WhatsApp regression/coexistence/finance | BLOCKED | тесты не запустились |
| Fresh production build и guarded browser/mobile E2E | NOT RUN | подготовленный Docker runner недоступен; artifact guards не обходились |
| Real Meta login/webhook/messages | NOT RUN | запрещено scope; live выключен |

Логи `.local/instagram-stage2/` содержат точные команды, exit codes и source hashes. Active Go проверен фактически. Есть кешированный Linux ELF Go1.26.11, но он не является runnable macOS toolchain; изолированный Linux Docker runner сейчас недоступен. `go.mod`, toolchain directives и зависимости не менялись для обхода gate. Host ESLint/TypeScript/OpenAPI PASS не заменяют production build или PostgreSQL tests.

## Подготовленные security tests

`testInstagramDurable` запускается внутри существующего isolated `aba_test` harness после проверки safety marker/roles/migrations. Создаёт A/B/C и проверяет:

- owner/session/CSRF и чужой tenant в HTTP, включая member с order privileges;
- encrypted token roundtrip, cross-tenant AAD rejection, отсутствие direct runtime token/state SELECT;
- FORCE RLS, unknown/foreign account, uniqueness/FK constraints;
- реальные customer/conversation rows и image metadata A/B; cross-tenant messages/attachments reads и пустой tenant/session;
- durable webhook duplicate, shared engine processing, tenant spoofing в ingress и переопределение подменённого payload при claim;
- cross-tenant/cross-channel capability rejection, bounded exponential retry и durable failed/DLQ;
- actual backend statuses, token expiry fencing, durable state expiry/denial/concurrent consumption;
- disconnect, token erasure, revoked ingress rejection и WhatsApp coexistence.

Добавлены unit tests registered routes/fail-closed/log redaction, config gates и worker retry classification. Existing Telegram/WhatsApp tests сохранены. Ни один Go-тест не помечен PASS без запуска.

## Изменённые файлы

- DB: `apps/api/migrations/00017_instagram_durable.sql`; `internal/platform/db/db.go`; migration harness `tests/integration/database_test.go` и новый `instagram_test.go`.
- Instagram: новые `internal/instagram/{service,ingress,worker,worker_test}.go`; изменения binding tags в `webhook.go`, актуализация package comment `policy.go`.
- Runtime/API: `cmd/{api,worker}/main.go`, `internal/http/{router,identity,middleware,instagram,instagram_test}.go`, `internal/config/instagram{,_test}.go`.
- Shared: `internal/channel/engine.go`, `internal/tenancy/transaction.go`, `internal/conversations/service.go`, адресные channel filters в `internal/whatsapp/service.go` и `internal/catalog/onboarding.go`.
- Web: BFF route, integrations listing/Instagram page, новый `InstagramIntegration.tsx`, новый `lib/instagram.ts`, inbox filter/labels/types/attachment notice в `WorkflowPage.tsx`, `ProductUI.tsx`, `Workflows.tsx`, `lib/workflows.ts`, обновлённый `instagram-production.spec.ts`.
- Contracts/config/evidence: `packages/contracts/openapi.yaml`, `scripts/check-openapi.mjs`, `.env.example`, этот report и `.local/instagram-stage2/INDEX.md`.

Точный перечень и hashes — `source-manifest.json`. Не включать чужие исходные незакоммиченные изменения в будущий commit автоматически.

## Риски и следующий этап

1. Критический blocker — не выполнены compilation/PostgreSQL up/down/RLS/concurrency/regressions. Следующий шаг: восстановить уже подготовленный offline runner с Go1.26.11, выполнить текущий integration harness и устранить обнаруженные ошибки. Не применять миграцию к рабочей БД для получения доказательств.
2. Это durable synthetic runtime, не завершённый официальный Meta OAuth/subscription/revoke provider. Пока отсутствуют live networking, provider status reconciliation, scheduled real token refresh и outbound dispatch. Для будущего real exchange потребуется отдельная recovery/fencing модель между durable state consumption и внешним token exchange.
3. Telegram repository физически не объединён с `channel_*`; это явно ограничивает буквальную трактовку acceptance «одна SQL queue для трёх каналов».
4. `failed` — текущая durable DLQ, без UI replay. State/audit identifiers не удаляются автоматически; retention policy для consumed OAuth hashes требует отдельного решения перед эксплуатацией.
5. Down безопасно отказывается при provider/catalog данных; populated rollback через удаление данных не предлагается. Доказательство безопасного roundtrip пока отсутствует.
6. E01 остаётся OPEN, production readiness не объявляется. Старые evidence/guards сохранены. Meta App Review PENDING, LIVE CONNECTION NOT TESTED.

**STAGE 2 PARTIALLY COMPLETE**
