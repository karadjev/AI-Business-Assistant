# Отчёт8Б.2 — многоканальная платформа, offline/mock

Дата:18.10.2026. **SECURITY STATUS: CONDITIONAL. TELEGRAM LIVE READINESS: NOT READY. WHATSAPP LIVE READINESS: NOT READY.** LLM provider NOT SELECTED, active mock, LLM budget0 RUB; WhatsApp live disabled, отдельный budget0 RUB. Реальные покупатели/production запрещены.

**EXTERNAL CALLS: NONE** для выполненных действий и изолированных проверок этого этапа. Настоящие Telegram/Meta/LLM не вызывались. Commit/push/deployment/повышение прав/загрузка зависимостей/туннели не выполнялись. Сеть хоста глобально заблокированной не объявляется.

## Что проверено до изменений

Прочитаны README, отчёты7/8А/8Б.1, security/delivery/budget/tenant/agent/conversation/order документы и соответствующий код/миграции/маршруты/worker/UI/CI. В начале находились migrations1–9, нет channel/whatsapp пакетов. Git status показывал полностью untracked проект: baseline commit отсутствует. Поэтому перечень изменений получен SHA-256 снимком файлов, без git add/commit. Старые документы не удалялись.

Сведения8Б.1 подтверждены кодом и повторными тестами: три прежние HIGH и A09/A10 остаются FIXED в рамках их regression scenarios. Перед изменениями локальный Go suite прошёл. Отчёты не использовались вместо повторных тестов. Число tenant tables расширено с27 до32. Прежние проблемы отсутствующих gcc/Linux frontend native packages воспроизведены, не объявлены исправленными.

## Архитектура до и после

До: Telegram-specific webhook/principal/inbox/outbox → Conversation/AI/Catalog/Orders.

После: provider-specific Telegram/WhatsApp auth+normalization → channel.Message/Principal → общий worker contract channel.Queue → channel.Engine.Receive/Action → **те же** Conversation, AI Agent, Catalog, Orders. Outbound идёт через channel.Adapter/guarded provider send и соответствующий repository. Agent/quote/consent/reserve не дублируются.

Telegram physical tables и API сохранены. WhatsApp получает channel_* repository; не выполнялась рискованная миграция TG rows в общую таблицу. Это единые бизнес-логика и worker contract при двух storage implementations. Telegram Send требует приватный bound guard; прямой adapter вызов его не обходит. Общий Adapter поддерживает normalization/capabilities/status/disable/send/policy, Queue — durable processing.

Подробная карта: MULTICHANNEL_ARCHITECTURE. Отдельные provider retry/window/signature policies намеренно сохранены.

## Схема и миграции

Новая **00010_multichannel.sql**:

- channel_integrations: mock-only, отдельный encrypted credential envelope, hashed worker proof, unique Phone ID/org, sending defaultfalse, monetary CHECK0, window/retention.
- channel_principals: org/integration/channel/user namespace, conversation и window; tenant-safe compound uniqueness/FK.
- channel_inbox: dedup event key, normalized payload, bounded attempts, availability/lease/capability.
- channel_outbox: UUID, payload/role, lease/fencing/attempts/provider ID/version, pending/sending/sent/delivered/read/failed/UNKNOWN/NEEDS_TEMPLATE/cancelled.
- channel_events: append-only attempts/status/control audit.

Все5 таблиц ENABLE/FORCE RLS; runtime role не владелец/не BYPASS, секретные колонки и прямые writes недоступны. Shared policies допускают только leased WA scope; он не авторизует TG storage. Общие authorization helpers расширены на оба scope. Глобальная migration-only channel_function_backup содержит SQL definitions, без tenant-data и runtime grants.

Миграции1–9 SHA-256 неизменны. Тесты реально применили up/down/repeated/concurrent up на чистой marked aba_test, затем 10→9→10 с уже существующими synthetic TG integrations/principals/orders: snapshot не изменился. Rollback с WA данными отказал вместо удаления. Старый API с readiness9 не считается пригодным к бесшовному rolling rollout schema10; API/worker обновляются вместе.

## API и UI

Новые реализованные пути:

| Метод | Путь |
|---|---|
| GET | /api/v1/organizations/{id}/integrations/whatsapp |
| POST | тот же/connect |
| POST | тот же/control |
| POST | тот же/synthetic-updates |
| POST | тот же/deliveries/{resource}/resolve |
| GET/POST | /api/v1/webhooks/whatsapp/{integration} |

GET conversations дополнен фильтром channel. Детали conversation содержат WA window и delivery statuses. Расширены OpenAPI channel/source enums (ранее ошибочно только internal_chat), typed WhatsApp webhook/request/status/error schemas и BFF allowlist. Все старые endpoints сохранены.

Новые страницы /dashboard/integrations и /dashboard/integrations/whatsapp; общие conversations/orders/ai-settings показывают каналы, статусы, warnings и ссылки. Настройки owner-only; staff может читать/отвечать/handoff и вручную разрешать UNKNOWN. Реальные сохранённые credentials UI не получает. Mock явно обозначен, фиктивных рабочих товаров/переписок нет.

**Ограничение UI verification:** новые current-source TSX компоненты/BFF проверены Chromium, lint/typecheck проходят. Свежая Next production build и реальные новые SSR страницы в собранном Next **NOT RUN**. Docker browser regression использовала cached stage7 web image; это проверка совместимости старого UI с текущим API, не доказательство новой production сборки.

## Реализованная безопасность и правила

HMAC по raw body, constant-time compare, отдельный challenge verify token, configured WABA/Phone mapping, allowlist до principal, body/batch/text bounds и durable dedup. Несколько org/каналов не объединяются по внешнему ID. Tenant context SET LOCAL не протекает в pool. Owner/member/CSRF проверяются backend и SQL, а не только UI.

Message+outbox commit атомарен. Внешней отправки в SQL transaction нет. Pinned session lock сериализует send с disconnect/handoff; admission повторяет active/window/control/expiry/lease. 429 — bounded Retry-After/backoff;5xx/timeout/неопределённый success — UNKNOWN без auto retry. Recovery после process death не отправляет сообщение снова. Staff resolve sent/cancelled с version, повтор stale version409, без resend. Exactly-once delivery не обещается.

Window default24h configurable60–86400s, retention default7 дней configurable1–30 при connect. Окно закрыто → NEEDS_APPROVED_TEMPLATE, staff получает409, никаких платных templates. Timestamp ограничивается receive time, status/replay не продлевают окно. Поздние receipts не понижают read/delivered. Worker стирает истёкшие payload; metadata/orders/audit сохраняются — это не полное удаление ПДн.

Quote/price/stock/explicit confirm/idempotency/reserve используют существующие сервисы. WA и TG подтверждение относится к конкретному quote/customer/conversation. До staff decision reserve0. LLM/WA budget0; live WA transport отсутствует и положительные live/budget env отклоняются.

## Проверки и результаты

Все перечисленные запуски выполнены локально. Временная БД исключительно aba_test с safety marker; рабочий стек/данные не мигрировались и не останавливались.

| Проверка | Результат | Доказательство / границы |
|---|---|---|
| Go formatting | PASS | gofmt check внутри offline go service |
| Go tests | PASS | go test -count=1 ./..., включая WA raw/wire/timeout/policy/config/cipher tests |
| Go vet | PASS | go vet ./... |
| Go build API/worker/migrate/healthcheck | PASS | текущие binaries в ignored artifacts |
| Go race | NOT RUN | попытка CGO_ENABLED1 go test -race завершилась до тестов: `C compiler gcc not found`; command exit1, race не выполнен |
| PostgreSQL integration | PASS | итоговый TestDatabaseAndMigrations; integration.log содержит полный stdout |
| FORCE RLS | PASS |32 actual-row negative suites под aba_app; inventory JSON |
| Migration compatibility/up/down | PASS | clean/marked DB, repeat/concurrent, existing TG/order snapshot unchanged, refusal of destructive down |
| Offline Telegram wire regression | PASS | реальный адаптер перенаправлен только в loopback emulator; не настоящий Telegram |
| WhatsApp wire/security/recovery | PASS | local HTTP mock, signed fixtures, two workers, actual subprocess exit87 после receipt, restart no resend |
| Frontend lint | PASS | installed eslint, без install |
| TypeScript | PASS | installed tsc --noEmit --incremental false |
| OpenAPI | PASS | swagger-parser + exact implemented path set; schemas обеих channels |
| Current WhatsApp component Chromium | PASS | owner forms,409/CSRF/tenant selection/UNKNOWN/XSS, mock badge |
| Current recovery components/BFF Chromium/JS | PASS |403/409/version/consent; actual BFF allowlist в том числе WA routes, restricted cookies |
| Fresh Next production build | NOT RUN | нет @next/swc-linux-x64-gnu и @tailwindcss/oxide-linux-x64-gnu; preflight отказался запускать Next с возможным download |
| Docker Compose config | PASS | основной compose и isolated multichannel+smoke config --quiet |
| Docker smoke | PASS в указанном scope | current Go binaries + cached web image, PostgreSQL/Mailpit, healthchecks, явный migrate job, без build/pull |
| Browser existing regression | PASS |5 Playwright tests: auth/Mailpit/session/org/XSS/logout/health/error; cached web |
| Actual backend unavailable UI | PASS | отдельно остановлены только own smoke API/worker; unavailable.spec:1 passed |
| New Next SSR/multichannel full-browser E2E | NOT RUN | требует fresh build; component harness не подменяет эту проверку |
| Real Telegram/Meta/LLM | NOT RUN | запрещено/credentials не предоставлены |

Обычный запуск integration helper tests помечает SKIP только для standalone crash-helper entrypoints; родительский сценарий действительно запускает их дочерними процессами и проверяет exit status. SKIP не означает пропуск crash scenario.

##30 обязательных offline сценариев

PASS означает конкретную автоматическую проверку, не качество настоящего провайдера и не новый Next SSR E2E.

| № | Сценарий | Статус | Доказательство |
|---|---|---|---|
|1|Telegram inbound|PASS|stage6/telegram_http_audit + общий Engine|
|2|WhatsApp inbound|PASS|multichannel_test signed ingress→worker→messages|
|3|WA verification|PASS|Challenge unit+HTTP handler|
|4|Invalid signature|PASS|adapter_test + integration rejection|
|5|Wrong Phone ID|PASS|adapter unit + чужая integration fixture|
|6|Duplicate WA|PASS|durable unique event; inserted1→0|
|7|Duplicate TG|PASS|stage6/concurrent dedup regression|
|8|Cross-tenant|PASS|две WA org, API/service +32-table runtime RLS|
|9|Cross-channel principal collision|PASS|PG same700 TG/WA, разные conversations; tuple unit|
|10|Concurrent worker|PASS|local wire held open; second worker не claim-ит same job, call count1|
|11|Delivery unknown|PASS|500/timeout wire tests + expired sending→UNKNOWN|
|12|Crash recovery|PASS|реальный child exit87 после mock receipt; restart без resend; TG crash regression|
|13|Delivery status|PASS|signed delivered/read receipts|
|14|Out-of-order status|PASS|read сохраняется после поздних sent/failed|
|15|Closed window|PASS|staff reply blocked, pending sender не вызывает transport|
|16|No approved template|PASS|NEEDS_APPROVED_TEMPLATE, нет fallback template sender|
|17|Disabled integration|PASS|ingress rejects, control cancels, admission active check|
|18|WA budget0|PASS|env validation/DB CHECK/live transport absent; mock не расход|
|19|LLM budget0|PASS|existing zero gates/actual Compatible transport count0 regression|
|20|Catalog search|PASS|WA asks real synthetic active SKU через общий agent/catalog|
|21|Explicit consent|PASS|offer+confirm quote, повтор→одна pending заявка; до consent count0|
|22|No early reserve|PASS|quantity_reserved0 до employee decision; shared transactional orders regression|
|23|Handoff|PASS|WA text/media после human не добавляют assistant; TG regression|
|24|Staff reply|PASS|WA human staff message→actual loopback send; TG regression|
|25|Unsupported media|PASS|normalization + PG handoff, no worker panic/download|
|26|Secret leakage|PASS в пределах скана|cipher column denied, Status/log/JSON negatives; redacted signature/exact-env scan|
|27|Frontend permissions|PASS в component/API scope|owner/member Chromium; actual member/CSRF API denials; new SSR E2E NOT RUN|
|28|Migration compatibility|PASS|old migration hashes и existing TG/order upgrade snapshot|
|29|TG regression|PASS|stage6/8A/8B1 + same external ID separate TG conversation|
|30|No external calls|PASS в test-contour scope|internal network/no default route/proxies off, only loopback/internal services, cached deps|

## Безопасность offline окружения

Docker endpoint проверен как локальный UNIX socket desktop-linux. Использованы только уже существующие образы с pull_policy:never и `--pull never --no-build`. Test Compose networks internal=true, отсутствует default route и опубликованные порты, DNS127.0.0.1, proxy env пусты, host Docker socket не монтируется. Проверены actual route table и container network/ports/DNS, а не только настройки приложения. GOPROXY/GOSUMDB/GOVCS/GOTELEMETRY отключены, GOTOOLCHAIN local, Next telemetry disabled. Host firewall недоступен/не менялся; глобальная изоляция хоста не заявляется.

Проверки использовали Go1.26.2-alpine3.23, PostgreSQL18.3-alpine3.23, cached Playwright1.64.0-noble, Mailpit1.21. Go/npm dependencies/lockfiles не менялись и не скачивались. Native race/build prerequisites не устанавливались.

## Критерии A–T

| Критерий | Статус | Основание |
|---|---|---|
|A Общая Channel Adapter архитектура|PASS|types/interfaces, общий Engine/Queue, provider guards|
|B Telegram совместим|PASS|полная доступная TG regression, старые rows/API сохранены|
|C WA offline adapter|PASS|normalization/wire fixtures/mock sender; live отсутствует|
|D HMAC|PASS|raw bytes/constant-time negative tests|
|E Tenant isolation|PASS|две org + actual runtime RLS|
|F Principal isolation|PASS|одинаковый external ID не объединяется|
|G Inbox/outbox оба канала|PASS|общий worker contract, separate durable repositories|
|H UNKNOWN|PASS|no auto retry, explicit versioned resolution|
|I Crash recovery|PASS|actual child process termination, persisted state/restart|
|J Общий AI|PASS|одни Conversation/Agent/Catalog/Orders services|
|K Consent|PASS|конкретный quote/callback, idempotency|
|L Staff reserve|PASS|reserve0 до staff; прежние concurrent invariants|
|M Handoff|PASS|WA/TG suppression and staff replies|
|N Frontend оба канала|NOT RUN целиком|код/components/BFF готовы и проверены; fresh Next SSR production E2E недоступен|
|O WA policy|PASS|server window/active/budget checks, template blocked|
|P Budgets0|PASS|active config/mock/defaults/constraints; положительные amounts только isolated synthetic ledger tests|
|Q RLS/migrations|PASS|32 tables, up/down/upgrade compatibility|
|R Regression|PASS в выполненном scope|Go/PG/TG и cached-web browser; race/new Next build отдельно NOT RUN|
|S Документация|PASS|8 запрошенных документов, актуальные API/security/setup/report|
|T Нет внешних/money/git/deploy|PASS|выполненные локальные действия; ограниченная область network guarantee описана выше|

Этап не объявляется полностью принятым по всем A–T: N и инструментальные race/build prerequisites остаются открытыми. Production-ready не заявляется.

## Найденные и исправленные проблемы

До итогового прогона исправлены: nil-error ошибочно превращался в Unavailable в новом Status; PL/pgSQL row alias ambiguity; политика могла сохранять TG scope из-за search_path при deparse; metadata enums OpenAPI не учитывали даже Telegram; DB unavailable webhook классифицирован как503 без ложного ack; добавлены tenant-safe principal/outbox FK, backend closed-window409 и проверка owner/member/CSRF. Ошибки тестовых fixtures (email case) также исправлены. Ранние FAIL не скрываются; итоговые PASS относятся к исправленной версии.

## Открытые риски и ограничения

- Нет fresh Next build/new SSR E2E и race detector; не заменены cached image или обычным go test.
- Настоящие WhatsApp/Telegram/LLM и актуальные официальные Meta policy/version/pricing не проверялись.
- WA live transport/credential rotation/billing/templates/multi-number не реализованы; current mock credentials не доказывают владение реальным номером.
- Общий queue contract не означает одну physical table; repositories намеренно два. Нужна отдельная security review новых definer functions/policy generation до live.
- Metadata/principals/order snapshots/audit после payload expiry сохраняются. Полное удаление/анонимизация и backup policy требуют отдельного согласования.
- UNKNOWN нельзя безопасно resend автоматически; человеческая ошибочная отметка sent остаётся операционным риском. Exactly-once не гарантируется.
- UI full production test нового multichannel hub не выполнен, несмотря на typecheck/components PASS.

## Воспроизводимые команды и CI

Команды offline go/integration/web/component — README, `infrastructure/compose.multichannel.yml`; локальный env файл содержит только абсолютный путь к уже существующему Go module cache. Race: тот же `run ... race` (реально попробован, gcc missing). Next preflight: `run ... web-check node /scripts/check-next-offline.cjs` (missing Linux native modules). Recovery/BFF: `run ... web-check node /scripts/test-recovery-components.cjs`.

Smoke: добавить `-f infrastructure/compose.multichannel-smoke.yml -p aba-audit8b2-smoke`, затем `up -d --pull never --no-build postgres mailpit migrate api worker web` и `run --rm --no-deps --pull never browser`. Это использует текущие Go artifacts и **cached** web. Для backend-down отдельно stop api worker только этого smoke project и run browser с SMOKE_BACKEND_DOWN1 на unavailable.spec. Не запускать обычные scripts, которые могут скачать deps.

Существующий CI автоматически включает новые Go пакеты/integration tests, web lint/typecheck/build и OpenAPI. CI workflow не запускался через GitHub, внешние Actions не вызывались. Локальные команды являются источником PASS; положительный будущий CI не предполагается.

## Что требует нового разрешения / следующий этап

1. Предварительно подготовить offline toolchain с gcc и Linux SWC/oxide, выполнить race и новую production build/SSR E2E обоих каналов. В этом этапе установка по сети не выполнялась.
2. Независимо проверить migration10 и outbound gates; устранить оставшиеся тестовые пробелы перед live.
3. Для Telegram отдельно: настоящий test bot/token/allowlist, HTTPS endpoint и явные разрешения getMe/getWebhookInfo/setWebhook/deleteWebhook/sendMessage. Ничего из этого не разрешено текущим ТЗ.
4. Для WhatsApp отдельно: Meta App/WABA/test number/credentials, актуальные official policies/pricing/Graph version, audited rotation/live transport, отдельный денежный лимит, HTTPS/webhook subscription и тестовые сообщения/receipts/templates. Не выполнять автоматически.
5. Реальные покупатели, privacy/legal policy, production, LLM provider и любой платный бюджет — отдельные решения. Telegram approval не даёт WhatsApp/LLM approval.

Работа останавливается после локальных проверок; никакой автоматической активации live.

## FILES CHANGED

Полный перечень относительно SHA-256 снимка начала этапа приведён ниже. Git commit отсутствует, поэтому git diff не отражает эти изменения. Ignored .local artifacts/cache/logs в перечень не включены.

Создано29, изменено33, удалено0.

### Созданы

- `apps/api/internal/channel/channel.go`
- `apps/api/internal/channel/engine.go`
- `apps/api/internal/config/whatsapp.go`
- `apps/api/internal/config/whatsapp_test.go`
- `apps/api/internal/http/whatsapp.go`
- `apps/api/internal/telegram/channel.go`
- `apps/api/internal/whatsapp/adapter.go`
- `apps/api/internal/whatsapp/adapter_test.go`
- `apps/api/internal/whatsapp/service.go`
- `apps/api/internal/whatsapp/worker.go`
- `apps/api/migrations/00010_multichannel.sql`
- `apps/api/tests/integration/multichannel_test.go`
- `apps/api/tests/integration/whatsapp_crash_test.go`
- `apps/web/src/app/dashboard/integrations/page.tsx`
- `apps/web/src/app/dashboard/integrations/whatsapp/page.tsx`
- `apps/web/src/components/WhatsAppIntegration.tsx`
- `docs/MULTICHANNEL_ARCHITECTURE.md`
- `docs/MULTICHANNEL_DELIVERY.md`
- `docs/SECURITY_RLS_INVENTORY_8B2.json`
- `docs/STAGE_8B2_REPORT.md`
- `docs/WHATSAPP_INTEGRATION.md`
- `docs/WHATSAPP_LIVE_READINESS_CHECKLIST.md`
- `docs/WHATSAPP_MESSAGE_POLICY.md`
- `docs/WHATSAPP_SECURITY.md`
- `docs/WHATSAPP_TEST_SETUP.md`
- `infrastructure/compose.multichannel-smoke.yml`
- `infrastructure/compose.multichannel.yml`
- `scripts/check-next-offline.cjs`
- `scripts/test-multichannel-components.cjs`

### Изменены

- `.env.example`
- `README.md`
- `apps/api/cmd/api/main.go`
- `apps/api/cmd/worker/main.go`
- `apps/api/internal/catalog/service.go`
- `apps/api/internal/conversations/service.go`
- `apps/api/internal/http/identity.go`
- `apps/api/internal/http/router.go`
- `apps/api/internal/http/stage5.go`
- `apps/api/internal/orders/service.go`
- `apps/api/internal/platform/db/db.go`
- `apps/api/internal/telegram/worker.go`
- `apps/api/internal/tenancy/transaction.go`
- `apps/api/tests/integration/audit_test.go`
- `apps/api/tests/integration/database_test.go`
- `apps/web/src/app/api/[...path]/route.ts`
- `apps/web/src/components/AISettings.tsx`
- `apps/web/src/components/Sidebar.tsx`
- `apps/web/src/components/WorkflowPage.tsx`
- `apps/web/src/components/Workflows.tsx`
- `apps/web/src/lib/workflows.ts`
- `docker-compose.yml`
- `docs/API_DESIGN.md`
- `docs/LOCAL_DEVELOPMENT.md`
- `docs/SECURITY.md`
- `docs/SECURITY_AUDIT_REPORT.md`
- `docs/TECHNICAL_DECISIONS.md`
- `docs/TELEGRAM_INTEGRATION.md`
- `docs/TELEGRAM_LIVE_READINESS_CHECKLIST.md`
- `docs/TENANT_ISOLATION.md`
- `packages/contracts/openapi.yaml`
- `scripts/check-openapi.mjs`
- `scripts/test-recovery-components.cjs`

Redacted scan688 файлов (source + доступные локальные bundles, исключая dependency/cache directories и файлы>8MB):0 найденных token/private-key signatures/exact long local-env secret matches. Это ограниченный скан, не абсолютное доказательство отсутствия любого секрета. Значения не выводились. SHA-256 migrations1–9 совпадают с начальным снимком; пользовательские файлы не удалялись.

## Финальная фиксация результатов

Последний полный PostgreSQL suite: **PASS23.884s**, включая новые owner/member/CSRF HTTP сценарии, stale window→NEEDS_TEMPLATE, реальный child process crash и payload retention. Итоговые Go tests/vet/build/format и web lint/typecheck/OpenAPI прошли. После уточнения OpenAPI enums/typed webhook схем validation повторён:PASS. Current WhatsApp Chromium component test также повторён:PASS.

Docker smoke API/worker были корректно остановлены с exit0 и вновь запущены с текущими binaries; API/Postgres/web/Mailpit healthy, worker running. Затем удалены только собственные aba-audit8b2 и aba-audit8b2-smoke контейнеры/сеть и созданный синтетический smoke volume. Исходные ai-business-assistant API/worker/web/Postgres/Mailpit остались healthy с прежним uptime4–5 дней. Их БД не мигрировалась. Restricted mode flags локального .env соответствуют утверждённым mock/false/0 defaults; значения секретов не выводились.

Дополнительное функциональное ограничение: external offer-кнопка на этом этапе подготавливает один variant ×1; общий Orders Service поддерживает несколько позиций, но полноценная внешняя channel-cart UI не добавлена. Это не меняет общие price/consent/reserve инварианты.

Работа остановлена. Telegram/Meta/LLM live, платные операции и следующие разрешения автоматически не запрашивались.
