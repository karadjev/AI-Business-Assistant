# Отчёт по ТЗ № 4

Дата: 13.10.2026. Каталог, категории, варианты, цены и склад одного магазина. Telegram/WhatsApp/LLM/платежи/1С/production не подключались; commit/push не выполнялись. Тестовые пользователи и магазины синтетические; dev volume не удалялся. Архитектурные документы и миграции 00001/00002 сохранены.

## Изучено и реализовано

Изучены README, STAGE_3_REPORT, SYSTEM_ARCHITECTURE, DATABASE_DESIGN, API_DESIGN, SECURITY, TENANT_ISOLATION, TECHNICAL_DECISIONS, текущие Go/Next/BFF/auth/organization/migration/integration/smoke/CI файлы. Проверены git status и закреплённые зависимости. В этом checkout ещё нет commits: status перечисляет исходные и новые файлы как untracked, это не удаление существующего проекта.

- Tenant-owned flat категории: создать/список/переименовать; удаления нет.
- Draft product создаётся атомарно с default variant и нулевым inventory; несколько вариантов, характеристики, ручные цены RUB в копейках, SKU normalized/unique внутри org включая архив.
- List/detail/edit, publish/draft/archive; name/brand/SKU substring search, category/status/stock filters, allowlist сортировки, bounded pagination.
- Owner mutations, member read-only; существующие session cookies/CSRF/SSR и BFF. Foreign/absent одинаковый 404; session401, own forbidden403, version/state/duplicate409, validation422.
- Целые остатки, derived available, version, row locks, атомарные immutable movements с actor/reason/before/after/delta. Две вкладки не перезаписывают друг друга. Публичного резервирования нет.
- FORCE RLS на всех пяти таблицах; составные FK; role ownership startup checks; SET LOCAL session+org; runtime без DELETE и audit UPDATE.
- CustomerCatalog read-only facade: search/details/price/inventory active-only, без публичных инструментов или LLM.
- Пять защищённых frontend страниц, реальные формы/состояния, switching организаций, XSS escaping, мобильная ширина. Данные при ошибке не подменяются нулевыми результатами.

## Файлы

Новые:

- `apps/api/migrations/00003_catalog.sql`.
- `apps/api/internal/catalog/service.go`, `service_test.go`.
- `apps/api/internal/http/catalog.go`.
- `apps/api/tests/integration/catalog_test.go`.
- `apps/web/src/lib/catalog.ts`.
- `apps/web/src/components/CatalogPage.tsx`, `CatalogWorkspace.tsx`.
- `apps/web/src/app/dashboard/products/page.tsx`, `products/new/page.tsx`, `products/[id]/page.tsx`, `categories/page.tsx`, `inventory/page.tsx`.
- `docs/PRODUCT_CATALOG.md`, `INVENTORY_MANAGEMENT.md`, `STAGE_4_REPORT.md`.

Изменены:

- Go `internal/http/identity.go` (регистрация защищённых catalog routes), `internal/platform/db/db.go` (readiness3 и ownership checks), `tests/integration/database_test.go` (catalog suite, миграции 3/2 roundtrip).
- Frontend BFF `app/api/[...path]/route.ts`, `lib/session.ts`, `components/Sidebar.tsx`, `Header.tsx`, `app/page.tsx`, `dashboard/page.tsx`, `globals.css`, browser `tests/auth.spec.ts`, `smoke.spec.ts`.
- `infrastructure/compose.test.yml` —gcc/musl-dev в disposable container и настоящий integration race detector.
- `packages/contracts/openapi.yaml`, `scripts/check-openapi.mjs`.
- README, LOCAL_DEVELOPMENT, TECHNICAL_DECISIONS, TENANT_ISOLATION, API_DESIGN, DATABASE_DESIGN, SYSTEM_ARCHITECTURE.

CI workflow сохранён: существующая команда запускает весь каталог tests/integration с race, браузерный scenario расширен автоматически. Проверен actionlint. Tool/dependency versions не менялись: Go1.26.2, pgx5.9.2, goose3.27.1, PostgreSQL18.3; Node24.15.0, Next16.4.0, React19.3.0, TS5.9.3, Tailwind4.3.3, ESLint10.12.0, Playwright1.64.0. Test GCC15.2.0-r2/musl-dev1.2.5-r23 в Alpine3.23.3; application images прежние.

## Миграция и API

00003 additive: categories/products/variants/inventory/movements, CHECK/unique/compound FK/indexes и FORCE RLS policies. Все id backend-generated UUID; организация не принимается в body. Metadata2→3. Down отказывается при непустом products/categories; tested только isolated database. Runtime/migrator separation сохранён, API не применяет миграции при старте.

O=`/api/v1/organizations/{id}`. Реализовано 15 operations:

| Путь | Методы |
|---|---|
| O/categories | GET, POST |
| O/categories/{resource} | PATCH |
| O/products | GET, POST |
| O/products/{resource} | GET, PATCH |
| O/products/{resource}/publish | POST |
| O/products/{resource}/archive | POST |
| O/products/{resource}/variants | GET, POST |
| O/variants/{resource} | PATCH |
| O/inventory | GET |
| O/inventory/adjustments | POST |
| O/inventory/movements | GET |

OpenAPI0.4.0 содержит только реализованные paths, schemas и ошибки; старые auth/org/health contracts сохранены. Детали —PRODUCT_CATALOG/INVENTORY_MANAGEMENT.

## Результаты проверок

| Проверка | Результат |
|---|---|
| Go unit + race | PASS |
| Go vet / formatting / builds | PASS |
| Real PostgreSQL integration + race | PASS: два owner/tenant, member, SQL RLS INSERT/SELECT/UPDATE/DELETE, compound FK, две physical connections, concurrency/audit, active-only reads, identity regression |
| Migrations up/down/repeat/concurrent up | PASS: clean isolated aba_test, metadata3→2→1→3, nonempty down refuses |
| Frontend lint / typecheck | PASS |
| Frontend production build | PASS |
| OpenAPI validation | PASS |
| Compose config | PASS |
| Browser catalog + auth regression | PASS: 5 live tests, включая реальный каталог/две вкладки/два магазина |
| Actual API/DB outages, worker/API shutdown | PASS: ready503/live200 при остановке БД, offline UI при остановке API, API/worker exit0 |
| CI static actionlint | PASS |
| Remote GitHub Actions | NOT RUN: push не разрешён |
| Native smoke целиком / production / load & penetration audit | NOT RUN: контейнерный smoke выбран для окружения; остальные не входят в этап |

Первые браузерные прогоны выявили accessible-name select и неоднозначные test locators цены в двух формах/alert Next route announcer; исправлены aria-label и scope нужного SKU. Один повтор build оборвался на Docker Hub TLS handshake timeout, последующий build прошёл. Не использовались отключение тестов/моки RLS/фиктивные success для получения PASS. Итоговый smoke успешно завершён: 5 live + 1 real-offline tests, actual DB outage, API/worker graceful shutdown.

## Критерии A–T

| Критерий | Статус | Проверка |
|---|---|---|
| A категория | PASS | Go API integration |
| B товар/default variant/цена | PASS | DB transaction + integer price |
| C свой список | PASS | API fixtures |
| D поиск/фильтры | PASS | SKU/name/brand, category/status/stock, pagination |
| E вариант/цена | PASS | PATCH/version/attributes/price |
| F остаток | PASS | initial and concurrent target adjustments |
| G история | PASS | two committed changes, actor/delta, immutable grants |
| H отрицательный остаток | PASS | API422 и SQL CHECK |
| I нет lost updates | PASS | concurrent goroutines, 1 success/1 conflict |
| J только active customer reads | PASS | read-only facade negative draft/archive |
| K API tenant isolation | PASS | foreign product/org404, body spoof rejected |
| L RLS | PASS | all five tables, WITH CHECK, member, pool reuse |
| M member read-only | PASS | API403 + SQL0 writes |
| N frontend | PASS | 5 live tests, реальные формы, switching, SSR redirect, XSS, mobile width |
| O OpenAPI | PASS | validator + endpoint-set check |
| P все проверки | PASS в доступном окружении | Go/race/vet, web/lint/types/build, PG/race, OpenAPI, Docker smoke |
| Q Compose | PASS | config/up/build/wait, 5 healthy runtime services |
| R auth/org/health/worker regression | PASS | identity integration + browser/Mailpit + real outages/shutdown |
| S документация | PASS | models/API/concurrency/rules/commands описаны |
| T данные/секреты | PASS в проверенном объёме | Только synthetic fixtures; .env/build artifacts ignored; локальные ключи отсутствуют в publishable source и проверенных API/worker logs |

## Архитектурные отличия и ограничения

Решения в TECHNICAL_DECISIONS: price_amount/currency_code/quantity_on_hand/quantity_reserved по ТЗ № 4; общая version варианта; JSON expected_version; PATCH полного редактируемого набора с optional defaults; page/page_size вместо cursor; substring вместо полнотекстовой relevance выдачи; UUID route; no images metadata до загрузки; терминальный архив. Это новый domain API, прежний контракт auth/org не изменён. FORCE RLS session-bound preserved.

Нет восстановления архивов, удаления категорий/вариантов, CSV/images, fuzzy search, нескольких складов, fractional stock, public reserve/заказов, внешних каналов или AI. Substring query может сканировать каталог; FTS GIN пока подготовлен, большой объём не измерялся. Coarse product lock сериализует stock/variant/status; измерить contention перед масштабом. updated_at не подтверждает физическое наличие. Отложенная политика свежести, quote TTL, retention и system/job principal требуют решения перед заказами/AI, но не блокируют текущий каталог. Гарантии RLS не включают полный взлом Go process, DBA или migration credentials.

## Запуск и следующее задание

```sh
docker compose config --quiet
docker compose up --build -d --wait
sh scripts/migrate.sh status
sh scripts/check.sh
sh scripts/integration.sh
sh scripts/smoke-compose.sh
```

Frontend http://127.0.0.1:3000, Mailpit http://127.0.0.1:8025, API http://127.0.0.1:8080. Существующий .env не перезаписывать, volume не удалять. Выберите org в кабинете, создайте category/draft product, установите stock с причиной, publish. При 409 обновите данные; не retry старого target автоматически.

Для ТЗ № 5 рекомендованы заявки requested с server-side price snapshots, staff-confirmed atomic reserve, idempotency и reserve/release/consume movements. Сначала определить stock freshness и quote TTL; затем реальные concurrency tests последней единицы и согласованный worker principal. LLM/Telegram подключать только отдельным заданием. По завершении остановиться без commit/push и ожидать ТЗ № 5.

Финальный источник результатов: PostgreSQL integration/race 8.771 s, browser live 5 passed (15.6 s), real-offline 1 passed (1.7 s), Docker smoke exit0. Ранние FAIL отражены выше; итоговые PASS не заменяют remote CI, production или нагрузочные проверки.

Локальный dev stack обновлён `docker compose up --build -d --wait` с сохранением существующего PostgreSQL volume/.env: API/web/worker/postgres/mailpit healthy. Migration status: 1,2,3 applied. Тестовые stacks удалены своими cleanup scripts; dev оставлен запущенным. Проверка локальных DB/email secret values выполнена против publishable source и последних 300 строк API/worker logs без вывода значений. Это проверка конкретных источников, не всеобъемлющий audit.
