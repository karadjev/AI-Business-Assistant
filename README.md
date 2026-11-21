# AI Business Assistant

**Этап8Б.1:** миграция **00009**, DELIVERY_UNKNOWN без автоматического повтора, staff resolution и аудит, консервативное восстановление зависшего LLM budget. [Отчёт](docs/STAGE_8B1_REPORT.md), [доставка](docs/TELEGRAM_DELIVERY_RECOVERY.md), [бюджет](docs/LLM_BUDGET_RECOVERY.md). Все3 HIGH перепроверены как FIXED; внешние вызовы и live по-прежнему запрещены, 0 RUB. Readiness требует явной миграции9. Offline harness: `infrastructure/compose.reliability.yml` (+ `compose.reliability-smoke.yml`), только cached images и локальные зависимости.

**Этап 8А — offline security audit:** 8 исправлений, новая миграция **00008**, проверки всех 25 tenant-таблиц и локальный Telegram HTTP emulator. **CONDITIONAL / Telegram NOT READY**, LLM 0 RUB, external/live выключены. [Результаты и offline-команды](docs/STAGE_8A_REPORT.md), [находки](docs/SECURITY_AUDIT_REPORT.md), [инструкция тестового бота](docs/TELEGRAM_TEST_BOT_SETUP.md), [readiness checklist](docs/TELEGRAM_LIVE_READINESS_CHECKLIST.md). Перед обновлённым API явно применить миграцию 8; прошлые отчёты описывают состояние своих этапов. Обычные install/build/CI команды ниже не являются строго offline — для режима 8А использовать только audit harness с уже имеющимися кэшами.

ИИ-продавец для малого бизнеса: проверяемый каталог, общение с покупателями и заявки с подтверждением сотрудником. Первый пилот — один магазин электроники в Ингушетии, первый канал — Telegram. Продукт не позиционируется как замена 1С.

Репозиторий: https://github.com/karadjev/AI-Business-Assistant.

**Этап № 7 — подготовка Mode A:** сохранены mock LLM/Telegram, добавлены gated compatible LLM adapter, нулевой денежный budget/ledger, allowlist Telegram и kill switches, AI settings и evaluation suite. Provider NOT SELECTED, live/external requests запрещены. Миграция 7 и PostgreSQL/RLS проверены на изолированной БД; результаты Docker/browser smoke и подробности [STAGE_7_REPORT](docs/STAGE_7_REPORT.md).

## Быстрый запуск

Требуются Docker Desktop/Engine и Docker Compose v2. Для native разработки/тестов: Go **1.26.2**, Node.js **24.15.0**, npm (проверено **11.12.1**). Полная инструкция: [LOCAL_DEVELOPMENT](docs/LOCAL_DEVELOPMENT.md).

```sh
test -f .env || cp .env.example .env
# Измените локальные URL-safe пароли; не перезаписывайте уже существующий .env.
docker compose config --quiet
docker compose up --build -d
docker compose ps
```

Compose сначала поднимает PostgreSQL, затем выполняет контролируемый migration job, затем API/worker/web. Миграции не запускаются внутри API. Новый AUTH_EMAIL_KEY в .env —случайные 32 байта hex для email outbox; текущий локальный .env уже дополнен. PUBLIC_ORIGIN должен совпадать с URL браузера. `.env` ignored; ключи Telegram/LLM не нужны. PostgreSQL не публикуется наружу, API/web привязаны к 127.0.0.1.

- Frontend: http://127.0.0.1:3000
- Личный кабинет: http://127.0.0.1:3000/dashboard
- Локальная почта: http://127.0.0.1:8025
- Регистрация: http://127.0.0.1:3000/register
- Состояние backend: http://127.0.0.1:3000/status
- Go API: http://127.0.0.1:8080/api/v1/status
- Liveness/readiness: http://127.0.0.1:8080/health/live и http://127.0.0.1:8080/health/ready

Status=running означает только работающий API. Ready проверяет БД, безопасную runtime роль и установленную схему. API_BASE_URL — только серверная настройка Next.js, не NEXT_PUBLIC.

```sh
sh scripts/migrate.sh up
sh scripts/migrate.sh status
# Явный откат: только на пустом каталоге / identity tables; непустые аккаунты/магазины защищены guard:
sh scripts/migrate.sh down
sh scripts/migrate.sh up
docker compose logs --tail=100 api worker web migrate
docker compose down
```

`down` сохраняет БД. Не удаляйте volumes с нужными данными. Изменение паролей в .env не обновляет роли существующего PostgreSQL volume.

## Проверки

```sh
cd apps/web
npm ci --no-fund --no-audit
cd ../..
sh scripts/check.sh
sh scripts/integration.sh
sh scripts/smoke-compose.sh
```

`check.sh`: gofmt/vet/race tests/build, frontend lint/typecheck/build и OpenAPI. Integration использует отдельную aba_test на tmpfs с safety marker, без удаления пользовательской БД. Browser smoke создаёт свой синтетический Compose stack и проверяет реальные outages и завершение процессов; нужны npm dependencies и официальный Playwright image. Native smoke и отдельные команды описаны в [локальной инструкции](docs/LOCAL_DEVELOPMENT.md). GitHub Actions workflow создан; запуск на GitHub требует последующего разрешённого push.

## Структура

```text
apps/api/{cmd/{api,worker,migrate,healthcheck},internal/{config,http,platform},migrations,tests}
apps/web/{src/{app,components,lib},tests,public}
packages/contracts/openapi.yaml
infrastructure/{postgres,compose.test.yml,compose.host.yml}
scripts/
.github/workflows/ci.yml
docs/
docker-compose.yml
```

Необходимые доменные модули добавляются после аутентификации и изоляции. Redis и пустые бизнес-абстракции отсутствуют.

## Диагностика

- Ready=503: проверьте postgres/migrate logs и права aba_app; не используйте superuser для API.
- Frontend сообщает unavailable: проверьте API_BASE_URL и запущенный Go API.
- Порт занят: измените API_PORT/WEB_PORT в .env.
- Docker недоступен: запустите Docker Desktop; без daemon контейнерные тесты не выполняются.
- Go cache/служебный порт запрещён: используйте writable GOCACHE; production frontend собирается через Webpack.

Подробности и ограничения: [LOCAL_DEVELOPMENT](docs/LOCAL_DEVELOPMENT.md).

Фактические результаты проверок и полный список файлов: [отчёт по ТЗ № 5](docs/STAGE_5_REPORT.md).

## Архитектурная документация

- [Видение продукта](docs/PRODUCT_VISION.md)
- [Бизнес-требования](docs/BUSINESS_REQUIREMENTS.md)
- [Границы MVP](docs/MVP_SCOPE.md)
- [Архитектура](docs/SYSTEM_ARCHITECTURE.md)
- [База данных](docs/DATABASE_DESIGN.md)
- [ИИ-агент](docs/AI_AGENT_ARCHITECTURE.md)
- [Проектный API и страницы](docs/API_DESIGN.md) — проектные и реализованные бизнес-endpoints, точный контракт в OpenAPI
- [Безопасность](docs/SECURITY.md)
- [Интеграции](docs/INTEGRATIONS.md)
- [План разработки](docs/DEVELOPMENT_ROADMAP.md)
- [Принятые решения и уточнения](docs/TECHNICAL_DECISIONS.md)

Коммиты/push, платные API и production deployment требуют отдельного задания/разрешения.

Аутентификация/ручной сценарий: [AUTHENTICATION](docs/AUTHENTICATION.md). Гарантии и пределы RLS: [TENANT_ISOLATION](docs/TENANT_ISOLATION.md). SMTP Mailpit без внешнего relay; тестировать только синтетические адреса example.test.

Каталог: [PRODUCT_CATALOG](docs/PRODUCT_CATALOG.md), склад: [INVENTORY_MANAGEMENT](docs/INVENTORY_MANAGEMENT.md). В кабинете выберите магазин и откройте `/dashboard/products?org=<UUID>`, категории и склад доступны через навигацию. Создание draft →корректировка начального остатка →публикация; подтверждённые резервы появятся на этапе заявок. При конфликте версии обновите данные и повторно оцените изменение. Новые env-переменные и внешние ключи не нужны; миграция 00003 применяется controlled migration job.

## Тестовые заявки и чат

[ORDER_MANAGEMENT](docs/ORDER_MANAGEMENT.md), [AI_AGENT_IMPLEMENTATION](docs/AI_AGENT_IMPLEMENTATION.md), [CONVERSATION_LIFECYCLE](docs/CONVERSATION_LIFECYCLE.md). `/dashboard/ai-playground` в обычном магазине предлагает создать отдельную тестовую организацию с пустым каталогом. Добавьте синтетический active товар и ручной stock в её каталоге →новый chat →спросите SKU →выберите позиции →server quote5min →checkbox + explicit confirm →`/dashboard/orders`. Только employee confirm резервирует, cancel освобождает, complete списывает. Test org warehouse изолирован от обычного магазина.

LLM_PROVIDER=mock, LLM_ENABLE_LIVE=false по умолчанию. Real adapter есть, но платный API не вызывался; не включать live без отдельного согласования. Все ключи только серверные. Порог свежести INVENTORY_FRESHNESS=24h; manual_updated_at не меняется при резерве. Member получает staff право только от owner через documented order-access API, каталог/настройки остаются owner-only.

Migration00004 выполняется controlled job, readiness требует metadata4. Старый .env не перезаписывать: новые настройки используют безопасные default без ключа. Тесты по прежним командам включают новые scenarios автоматически; отдельная БД/tmpfs и synthetic browser stack.

## Telegram · этап6 (mock)

[TELEGRAM_INTEGRATION](docs/TELEGRAM_INTEGRATION.md), [TELEGRAM_SECURITY](docs/TELEGRAM_SECURITY.md), [MESSAGE_DELIVERY](docs/MESSAGE_DELIVERY.md), [STAGE_6_REPORT](docs/STAGE_6_REPORT.md).

Добавьте в existing ignored `.env` **отдельный** случайный32-byte hex `TELEGRAM_ENCRYPTION_KEY` (`openssl rand -hex 32`, не публикуйте вывод), TELEGRAM_TRANSPORT=mock, TELEGRAM_RETENTION_DAYS=7. Без ключа прежнее приложение запускается, Telegram disabled. `docker compose up --build -d --wait` применяет controlled00005–00007; readiness metadata7. Сохраните volumes, не используйте down --volumes для рабочего стека.

В отдельном test shop добавьте synthetic active SKU и вручную сверенный stock. `/dashboard/integrations/telegram?org=<UUID>` →`mock:10001` →synthetic private update →worker →mock reply/inline offer →отдельный confirm callback →pending Telegram request. `/dashboard/conversations` —history/handoff/staff reply; `/dashboard/orders` —employee reserve/reject/cancel/complete. Повторные события/действия идемпотентны. Credentials/keys не NEXT_PUBLIC/localStorage; runtime no BYPASSRLS. Mock sent не означает реальную Telegram-доставку.

**Настоящие Telegram-запросы/сообщения/HTTPS webhook registration не включены.** Только mock transport/local protocol tests; production и реальные ПДн требуют отдельного согласования. Не вводите настоящий BotFather token. Tests по прежним командам включают stage6, Docker/native smoke генерирует собственный ignored ephemeral test key. Полный отчёт содержит PASS/FAIL/NOT RUN; commit/push не выполняются.

## Подготовка live и текущие ограничения

[LLM adapter](docs/LIVE_LLM_INTEGRATION.md), [расходы](docs/LLM_COST_CONTROL.md), [evaluation](docs/AI_EVALUATION.md), [ручной пилот](docs/PILOT_READINESS.md), [проект политики данных](docs/DATA_RETENTION_POLICY_DRAFT.md).

Страница `/dashboard/ai-settings` работает через защищённый same-origin BFF для test организации. Бюджет по умолчанию 0 RUB; ключи и provider не выбраны. Открытие страницы, startup и миграции не вызывают Telegram/LLM. Нужна миграция `00007`; readiness теперь требует schema version 7. Не запускать source 7 поверх schema 6 без явного migration job.

Go/frontend/OpenAPI/static Compose checks выполнены нативно; PostgreSQL и контейнерные проверки запускаются разрешёнными прямыми Docker Compose командами. Shell wrapper может получать socket permission denied в sandbox. Для повторной проверки после восстановления доступа: `sh scripts/integration.sh`, затем `sh scripts/smoke-compose.sh`. Эти scripts принудительно используют mock и отключают external gates. Live getMe/setWebhook/сообщения и платные LLM требуют нового разрешения, не входят в автоматический check.

## Этап 8Б.2 — Telegram + WhatsApp, только offline/mock

Добавлены общий `channel.Engine`/`channel.Queue`, WhatsApp raw-body HMAC webhook, отдельные tenant-safe inbox/outbox/principals и единый кабинет диалогов. Telegram сохраняет старые данные и API. Миграция00010 выполняется явно; API/worker требуют schema10.

Страницы: `/dashboard/integrations`, `/dashboard/integrations/whatsapp`, `/dashboard/conversations?channel=whatsapp`, существующие orders/ai-settings. Только отдельная test organization. WhatsApp live отсутствует, budgets WhatsApp/LLM0 RUB, mock sending по умолчанию выключен. Реальные токены backend не принимает.

Для local mock задайте **отдельный**32-byte hex `WHATSAPP_ENCRYPTION_KEY` в ignored .env одинаково для API/worker. Не печатайте ключ и не публикуйте его в чат/Git. Без ключа статус доступен, но подключение/webhook не работают. Не запускайте обычный Docker build, если требуется строгий offline: пользуйтесь стендом ниже, который не скачивает образы или зависимости.

Существующие команды `go test ./...` и integration suite автоматически включают новый пакет/сценарии в CI; lint/typecheck/OpenAPI проверяют новые UI/контракты. Для воспроизводимых локальных **offline** проверок (зависимости и указанные образы уже должны существовать):

```sh
# В ignored .local/audit8b2/compose.env:
# ABA_OFFLINE_GOMODCACHE=/absolute/path/to/already-cached/go/pkg/mod

docker compose --env-file .local/audit8b2/compose.env -f infrastructure/compose.multichannel.yml -p aba-audit8b2 config --quiet
docker compose --env-file .local/audit8b2/compose.env -f infrastructure/compose.multichannel.yml -p aba-audit8b2 run --rm --no-deps --pull never go
docker compose --env-file .local/audit8b2/compose.env -f infrastructure/compose.multichannel.yml -p aba-audit8b2 run --rm --pull never integration
docker compose --env-file .local/audit8b2/compose.env -f infrastructure/compose.multichannel.yml -p aba-audit8b2 run --rm --no-deps --pull never web-check
docker compose --env-file .local/audit8b2/compose.env -f infrastructure/compose.multichannel.yml -p aba-audit8b2 run --rm --no-deps --pull never web-check node /scripts/test-multichannel-components.cjs
```

Integration suite требует чистую `aba_test` с safety marker. Для повторного полного прогона удаляйте **только собственный** aba-audit8b2 project (`down`): его БД tmpfs. Не используйте рабочую БД/проект. Нужны локальные каталоги `.local/audit8b2/go-cache` и `artifacts`. Web scripts не заменяют Next production build; результат build/race и ограничения — [отчёт](docs/STAGE_8B2_REPORT.md).

Документы: [архитектура](docs/MULTICHANNEL_ARCHITECTURE.md), [WhatsApp](docs/WHATSAPP_INTEGRATION.md), [security](docs/WHATSAPP_SECURITY.md), [policy](docs/WHATSAPP_MESSAGE_POLICY.md), [setup](docs/WHATSAPP_TEST_SETUP.md), [delivery](docs/MULTICHANNEL_DELIVERY.md), [readiness](docs/WHATSAPP_LIVE_READINESS_CHECKLIST.md).


## Стабилизация 8В: offline результаты

20.10.2026: backend E2E Telegram/WhatsApp на трёх synthetic организациях,32 FORCE RLS tables, recovery и regression PASS. Общая pre-live готовность **CONDITIONAL / NOT READY**: race и свежая Next production build BLOCKED, полный current SSR browser E2E NOT RUN. Cached web regression не заменяет новую сборку. Live обоих каналов выключен, LLM и WhatsApp бюджеты0 RUB.

Отчёты: [итог8В](docs/STAGE_8V_REPORT.md), [E2E](docs/MULTICHANNEL_E2E_TEST_REPORT.md), [безопасность](docs/PRE_LIVE_SECURITY_REPORT.md), [сборки и точные команды](docs/BUILD_READINESS_REPORT.md), [будущий пилот — только план](docs/PILOT_TEST_PLAN.md). Изолированные Compose: `infrastructure/compose.stabilization.yml` и `compose.stabilization-smoke.yml`; никогда не подставлять рабочую БД. Они не скачивают images/dependencies. Для повтора нужен заранее заполненный local Go cache; fresh frontend build требует заранее доступных native packages. Реальные API/HTTPS/токены/расходы требуют нового отдельного разрешения.


## Среда сборки8Д

Причины и воспроизводимое решение: [диагностика](docs/ENVIRONMENT_DIAGNOSTICS.md), [Docker build plan](docs/BUILD_ENVIRONMENT_PLAN.md), [требуемое согласование](docs/ENVIRONMENT_SETUP_APPROVAL.md), [отчёт8Д](docs/STAGE_8D_REPORT.md). Скрипты `scripts/build-env/` не скачивают dependencies и отказываются от host execution. Prepared Compose использует отдельные Linux node_modules и current-artifact proof; cached web не засчитывается как production E2E. Подготовка toolchain требует отдельного разрешения, которое ещё не дано. Оба budgets0 RUB, live off.

<!-- phase2-final -->

Фаза 2 этапа 8Д: локальные Go race, свежая production-сборка и browser E2E обоих mock-каналов прошли. Live-интеграции выключены, оба бюджета 0 RUB. Отчёт и ограничения: [BUILD_ENVIRONMENT_PHASE_2_REPORT](docs/BUILD_ENVIRONMENT_PHASE_2_REPORT.md).

Этап 9А: [WhatsApp Cloud API transport](docs/WHATSAPP_CLOUD_API_TRANSPORT.md), [offline contracts](docs/WHATSAPP_OFFLINE_CONTRACT_TESTS.md), [ограничения перед 9Б](docs/STAGE_9B_READINESS.md). Runtime остаётся mock-only; автоматической live-активации нет.

Этап9Б: [financial admission](docs/WHATSAPP_FINANCIAL_ADMISSION.md), [pricing](docs/WHATSAPP_PRICING_POLICY.md), [registry](docs/WHATSAPP_TEMPLATE_REGISTRY.md), [отчёт](docs/STAGE_9B_REPORT.md). Schema00011, RLS37. Настройки live и оба бюджета остаются0/disabled; текущий рабочий стек не обновлялся автоматически.

Offline9В: [trusted provider provisioning и операторская сверка](docs/STAGE_9C_REPORT.md); [готовность10А](docs/STAGE_10A_READINESS.md). Live выключен, оба бюджета0 RUB.

Этап10А offline: [мастер магазина и безопасный Excel import](docs/STAGE_10A_REPORT.md), [синтетический XLSX](apps/web/public/fixtures/synthetic-electronics.xlsx), [готовность10Б](docs/STAGE_10B_READINESS.md). Live выключен, оба бюджета0 RUB.
