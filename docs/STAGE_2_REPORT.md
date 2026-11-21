# Отчёт по ТЗ № 2 — рабочий каркас

Дата проверки: 12.10.2026. Код приложения создан и проверен локально и в Docker. Бизнес-функциональность не реализована; реальных клиентских данных/Telegram/LLM/платежей/production deployment нет. Commit/push не выполнялись.

## Что изучено и согласовано

Прочитаны README и SYSTEM_ARCHITECTURE, DATABASE_DESIGN, API_DESIGN, SECURITY, MVP_SCOPE, DEVELOPMENT_ROADMAP, TECHNICAL_DECISIONS. Утверждённые решения пользователя сохранены: модульный монолит Go, отдельный worker, PostgreSQL, Next.js; Telegram первым; сменяемый LLM; заявка ИИ с ручным подтверждением и резервом после него; один магазин электроники.

Минимальные уточнения документов: первоначальный пилот 3–5 магазинов заменён одним; тесты двух синтетических tenants остаются будущей обязательной проверкой изоляции. Архитектурные API/таблицы обозначены как будущие, а не готовые. Compose находится в корне для штатной команды запуска. sqlc, RLS и доменные модули отложены до фактической реализации, поскольку сейчас бизнес-запросов нет. API готовность требует установленной metadata migration, а не только TCP соединения. Worker не открывает pool до появления работы. Webpack выбран для production build после запрета служебного порта Turbopack в ограниченном окружении. Подробности — TECHNICAL_DECISIONS.

## Реализовано

Go: config validation, net/http API, request ID, JSON slog, safe headers/errors/recovery, body size limit, request/server/DB timeouts, pgx pool, live/ready/status, graceful shutdown. Мигратор goose отдельной командой с advisory lock. Bootstrap PostgreSQL создаёт раздельные непривилегированные роли и schema platform; app имеет только SELECT metadata. Единственная migration — metadata, бизнес-таблиц нет. Runtime readiness отклоняет superuser/BYPASSRLS/CREATEDB/CREATEROLE и роль с CREATE в platform.

Worker: config, старт/остановка, честный idle mode без очереди. Frontend: главная, публичный dashboard-макет без показателей, status, loading/error/unavailable, responsive layout и OS light/dark theme. Внутренний API URL остаётся на Next.js server; браузер использует same-origin adapter. Нет фиктивной авторизации или открытого бизнес-CRUD.

Docker: multi-target Go Dockerfile (api/worker/migrate), Next.js Dockerfile, healthchecks, persistent PostgreSQL volume, отдельный одноразовый migrate service, опубликованы только loopback API/web. Tests/CI: unit, integration, browser smoke и GitHub Actions. Документация запуска обновлена.

## Версии

| Инструмент | Версия |
|---|---|
| Go | 1.26.2 |
| Node.js / npm в локальной проверке | 24.15.0 / 11.12.1 |
| PostgreSQL image | 18.3-alpine3.23 |
| Docker Engine / Compose CLI окружения | Engine 29.4.0 / Compose 5.1.2 |
| pgx / goose | 5.9.2 / 3.27.1 |
| Next.js / React | 16.4.0 / 19.3.0 |
| TypeScript / Tailwind CSS | 5.9.3 / 4.3.3 |
| ESLint / typescript-eslint | 10.12.0 / 8.71.1 |
| Playwright | 1.64.0; официальный image v1.64.0-noble |
| actionlint | 1.7.7 |

Зависимости и lockfiles фиксированы; installation в Docker прошла без несовместимых peer dependency overrides после изменения lint config. Docker image tags фиксированы, не latest для базовых образов. Версии нужно обновлять отдельным проверяемым изменением.

## Фактическая структура

```text
apps/api/
  cmd/{api,worker,migrate,healthcheck}/
  internal/config/
  internal/http/
  internal/platform/{db,logging,migrate,run}/
  migrations/
  tests/integration/
  go.mod, go.sum, Dockerfile
apps/web/
  src/app/{dashboard,status,api/backend-status}/
  src/components/{Header,Sidebar,StatusIndicator}.tsx
  src/lib/{config,api}.ts
  tests/, public/, Dockerfile, package.json, package-lock.json
packages/contracts/openapi.yaml
infrastructure/{postgres/init.sh,compose.test.yml,compose.host.yml}
scripts/{check,migrate,integration,smoke,smoke-compose,dev-api,dev-worker,dev-web}.sh
scripts/{go-env.sh,check-openapi.mjs}
.github/workflows/ci.yml
docs/
docker-compose.yml, .env.example, .gitignore, .nvmrc, .node-version, README.md
```

## Результаты реально выполненных проверок

| Проверка | Результат | Фактическое подтверждение |
|---|---|---|
| Compose config | PASS | `docker compose config --quiet`, также вариант с .env.example |
| Полная сборка и запуск | PASS | `docker compose up --build -d --wait --wait-timeout 180`; 4 runtime services healthy, migration exited 0 |
| Go formatting | PASS | gofmt применён, проверка отсутствия неформатированных файлов |
| Go unit + race | PASS | `go test -race -count=1 ./...`; config, endpoints, recovery/errors/timeouts, pool config, draining shutdown и worker cancel |
| Go vet | PASS | `GOCACHE=<writable cache> go vet ./...` |
| Go build | PASS | API, worker, migrate, healthcheck; также CGO_ENABLED=0 Docker builds |
| PostgreSQL integration | PASS | Две независимые свежие aba_test на tmpfs; up/repeat/concurrent up, rollback/reapply, readiness, runtime SELECT-only, unreachable DB |
| Migration status | PASS | version=1, state=applied через отдельную команду migrate status |
| Frontend lint | PASS | ESLint 10, без warnings после исправления конфигурации |
| Frontend TypeScript | PASS | `npm run typecheck` и проверка внутри production build |
| Frontend build | PASS | Webpack build на хосте и в Docker; static pages + dynamic status adapter |
| OpenAPI | PASS | swagger-parser validation и проверка реализованного набора endpoints |
| Browser smoke | PASS | 4 сценария при работающем backend + 1 отдельный сценарий после настоящей остановки backend |
| Реальная остановка PostgreSQL | PASS | ready=503, live=200; после восстановления база вновь доступна |
| SIGTERM API/worker | PASS | Сценарий smoke-compose, обе exit codes=0 |
| SIGINT API/worker | PASS | Контролируемый Docker SIGINT, обе exit codes=0, после проверки перезапущены |
| Layout | PASS | Desktop/mobile screenshot inspection; мобильный dashboard без горизонтального переполнения |
| Секреты | PASS в проверенном объёме | .env ignored, локальные passwords/DSNs отсутствуют в фактических service logs; panic/DB error redaction unit tests |
| Shell/Markdown | PASS | sh -n для 10 scripts, локальные ссылки и закрытые fences |
| CI static validation | PASS | actionlint 1.7.7, минимальные permissions, закреплённые инструменты |
| GitHub Actions на GitHub | NOT RUN | Нет разрешения на commit/push; локальная проверка не заменяет remote run |
| Native smoke.sh целиком | NOT RUN | Полный эквивалентный контейнерный smoke-compose выполнен; native ports/browser ограничены окружением |
| Автоматический security audit / penetration test | NOT RUN | Полный аудит не входит в проверку каркаса; не заявляется |

Контейнерный smoke-compose.sh выполнен целиком через Docker CLI test driver: собственный синтетический stack, реальные сбои БД/API и 5 браузерных сценариев; после теста его containers/network/volume удалены. Интеграционные stacks также удалены. Основной development stack оставлен запущенным.

Первичные неуспешные запуски не скрываются: ограниченный запуск Go socket test получил operation not permitted (повтор через разрешённую go test прошёл); Turbopack build не мог открыть служебный порт (production build переведён на поддерживаемый Webpack и прошёл); host browser отсутствовал/системный Chrome не запускался (полный browser test перенесён в официальный контейнер и прошёл). Первоначальные peer warnings линтера устранены совместимым набором plugins. Это не заглушки и не исключение проверок из тестового набора.

## Приёмка A–O

| Критерий | Статус | Обоснование |
|---|---|---|
| A | PASS | Compose config валиден |
| B | PASS | Полный build/start, migrations и 4 services проверены |
| C | PASS | Реальный live=200, даже при остановленной БД |
| D | PASS | Ready=200 на доступной БД и 503 на остановленной/неподготовленной |
| E | PASS | Фактический JSON service/status/api_version проверен |
| F | PASS | Главная открыта реальным Chromium |
| G | PASS | Dashboard помечен макетом, нет выдуманных KPI/auth |
| H | PASS | Реальный offline backend →503 adapter и видимое unavailable |
| I | PASS | Clean DB + repeat/concurrent up + down/re-up |
| J | PASS | Go tests/vet/build, frontend lint/typecheck/build |
| K | PASS | SIGTERM и SIGINT завершают API/worker с кодом 0 |
| L | PASS в пределах локального окружения | Workflow создан, actionlint и соответствующие проверки выполнены; remote run NOT RUN |
| M | PASS | README/LOCAL_DEVELOPMENT описывают фактический запуск, роли, порты и тесты |
| N | PASS в проверенном объёме | Нет реальных provider secrets/PII; .env ignored, секреты отсутствуют в проверенных logs/ответах |
| O | PASS | Все 11 первоначальных документов сохранены, изменения ограничены уточнениями |

## Запуск и текущее состояние

Локальный ignored .env уже подготовлен с разными случайными URL-safe паролями только для development PostgreSQL; значения не выводились. Не перезаписывайте его при повторном запуске.

```sh
docker compose up --build -d
docker compose ps
sh scripts/migrate.sh up
sh scripts/migrate.sh status
```

Frontend http://127.0.0.1:3000, dashboard /dashboard, status /status; API http://127.0.0.1:8080/api/v1/status, health /health/live и /health/ready. БД host port отсутствует. Команды тестирования и native development — [LOCAL_DEVELOPMENT](LOCAL_DEVELOPMENT.md). Остановка `docker compose down` сохраняет development volume.

## Ограничения и последующие решения

Это технический каркас, не готовый MVP и не production SaaS. Нет sessions/auth/RBAC/RLS, tenant данных, резервов/заказов, каталога, durable queue, Telegram, LLM, billing, backup/PITR или error-monitoring service. Нельзя тестировать реальных клиентов и обещать изоляцию ещё не реализованных доменных данных. Статус running означает процесс, не готовность всех будущих модулей; worker health означает только процесс.

Требуют отдельного решения до зависимого этапа: конкретный LLM/модель, бюджет, срок хранения переписок и правовые основания/договоры обработки ПД, email delivery, production hosting и backup. Эти вопросы не блокируют текущий каркас. Коммиты/push и deployment не разрешены текущим заданием.

ТЗ № 3: Authentication + Organizations + Memberships. Email/password с verify/reset, безопасные opaque sessions/CSRF, org+owner transaction, минимальные миграции этих модулей, runtime RLS/bootstrap privileges, негативные тесты двух синтетических организаций и базовый onboarding. Сначала доказать изоляцию; каталог, Telegram и LLM — последующими этапами. ИИ-продавец с каталогом/заявками, не замена 1С.

## Изменённые существующие файлы

- `README.md`
- `.gitignore`
- `docs/PRODUCT_VISION.md`
- `docs/MVP_SCOPE.md`
- `docs/SYSTEM_ARCHITECTURE.md`
- `docs/DATABASE_DESIGN.md`
- `docs/API_DESIGN.md`
- `docs/SECURITY.md`
- `docs/DEVELOPMENT_ROADMAP.md`
- `docs/TECHNICAL_DECISIONS.md`

## Созданные файлы

- `.env.example`
- `.github/workflows/ci.yml`
- `.node-version`
- `.nvmrc`
- `apps/api/.dockerignore`
- `apps/api/Dockerfile`
- `apps/api/cmd/api/main.go`
- `apps/api/cmd/healthcheck/main.go`
- `apps/api/cmd/migrate/main.go`
- `apps/api/cmd/worker/main.go`
- `apps/api/go.mod`
- `apps/api/go.sum`
- `apps/api/internal/config/config.go`
- `apps/api/internal/config/config_test.go`
- `apps/api/internal/http/errors.go`
- `apps/api/internal/http/middleware.go`
- `apps/api/internal/http/router.go`
- `apps/api/internal/http/router_test.go`
- `apps/api/internal/platform/db/db.go`
- `apps/api/internal/platform/db/db_test.go`
- `apps/api/internal/platform/logging/logging.go`
- `apps/api/internal/platform/migrate/migrate.go`
- `apps/api/internal/platform/run/run.go`
- `apps/api/internal/platform/run/run_test.go`
- `apps/api/migrations/00001_service_metadata.sql`
- `apps/api/migrations/embed.go`
- `apps/api/tests/integration/database_test.go`
- `apps/web/.dockerignore`
- `apps/web/Dockerfile`
- `apps/web/eslint.config.mjs`
- `apps/web/next-env.d.ts`
- `apps/web/next.config.ts`
- `apps/web/package-lock.json`
- `apps/web/package.json`
- `apps/web/playwright.config.ts`
- `apps/web/postcss.config.mjs`
- `apps/web/public/.gitkeep`
- `apps/web/src/app/api/backend-status/route.ts`
- `apps/web/src/app/dashboard/page.tsx`
- `apps/web/src/app/error.tsx`
- `apps/web/src/app/globals.css`
- `apps/web/src/app/layout.tsx`
- `apps/web/src/app/loading.tsx`
- `apps/web/src/app/page.tsx`
- `apps/web/src/app/status/page.tsx`
- `apps/web/src/components/Header.tsx`
- `apps/web/src/components/Sidebar.tsx`
- `apps/web/src/components/StatusIndicator.tsx`
- `apps/web/src/lib/api.ts`
- `apps/web/src/lib/config.ts`
- `apps/web/tests/smoke.spec.ts`
- `apps/web/tests/unavailable.spec.ts`
- `apps/web/tsconfig.json`
- `docker-compose.yml`
- `docs/LOCAL_DEVELOPMENT.md`
- `docs/STAGE_2_REPORT.md`
- `infrastructure/compose.host.yml`
- `infrastructure/compose.test.yml`
- `infrastructure/postgres/init.sh`
- `packages/contracts/openapi.yaml`
- `scripts/check-openapi.mjs`
- `scripts/check.sh`
- `scripts/dev-api.sh`
- `scripts/dev-web.sh`
- `scripts/dev-worker.sh`
- `scripts/go-env.sh`
- `scripts/integration.sh`
- `scripts/migrate.sh`
- `scripts/smoke-compose.sh`
- `scripts/smoke.sh`
