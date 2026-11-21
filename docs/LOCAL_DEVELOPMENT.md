# Локальная разработка

## Версии и реальные границы

Go 1.26.2; Node.js 24.15.0 (npm 11.12.1 в проверенном окружении); PostgreSQL 18.3; Next.js 16.4.0; React 19.3.0; TypeScript 5.9.3; Tailwind 4.3.3; ESLint 10.12.0; pgx 5.9.2; goose 3.27.1; Playwright 1.64.0. Go dependencies и npm dependencies закреплены go.mod/go.sum и package.json/package-lock.json. Dockerfile использует фиксированные image tags; обновления — отдельным проверяемым изменением. Требуется Docker Compose v2 с support service_completed_successfully и --wait. Redis не нужен.

Реализованы health/status, auth, server sessions, организации и FORCE RLS. Worker обслуживает dev email outbox. Каталога, заказов, Telegram и LLM нет. Не используйте реальные клиентские данные. Dashboard защищён проверкой реальной сессии; показатели отсутствуют. Актуальные auth/cookies/RLS: AUTHENTICATION.md и TENANT_ISOLATION.md.

## Полностью контейнерный запуск

Из корня репозитория:

```sh
cp .env.example .env
# Измените локальные пароли в .env, оставляя их URL-safe (например, hex).
docker compose config --quiet
docker compose up --build -d
docker compose ps
```

В .env только локальные настройки. Не записывать туда токены Telegram/LLM на этом этапе; не добавлять файл в Git. Если .env уже существует, сохраните его настройки — не выполняйте cp повторно. Значения .env.example являются явными локальными placeholders, не production credentials. Пароли администратора, мигратора и приложения должны различаться.

Порядок: postgres healthy → одноразовый migrate up → api и worker; web стартует вместе с процессом API и способен показать ошибку даже при неготовом backend. Migration job защищён advisory lock; несколько jobs сериализуются. API не выполняет SQL-миграции. Worker обрабатывает зашифрованный dev email outbox и завершает работу по SIGINT/SIGTERM. Его healthcheck проверяет наличие процесса, не работу бизнес-очереди.

Адреса по умолчанию:

| Адрес | Что доступно |
|---|---|
| http://127.0.0.1:3000 | Главная страница |
| http://127.0.0.1:3000/dashboard | Защищённый кабинет и реальные собственные организации |
| http://127.0.0.1:3000/status | Реальная проверка Go status через Next.js server |
| http://127.0.0.1:3000/api/backend-status | Ограниченный server-side status adapter |
| http://127.0.0.1:8080/health/live | 200, без обращения к БД |
| http://127.0.0.1:8080/health/ready | 200 при безопасной runtime-роли и установленной schema version=2; иначе 503 |
| http://127.0.0.1:8080/api/v1/status | service/status/api_version; только состояние процесса |

API и web опубликованы только на loopback. PostgreSQL по умолчанию не публикует порт. Изменение API_PORT/WEB_PORT меняет только host port, не адреса внутри контейнеров. API_BASE_URL=http://api:8080 задаётся для web container, не передаётся браузеру. Browser → same-origin Next.js endpoint → Go; CORS не включён, никаких wildcard. Endpoint adapter имеет только GET и фиксированный upstream, не принимает произвольный URL.

```sh
docker compose logs --tail=100 api worker web migrate
docker compose stop
docker compose start postgres api worker web
docker compose down
```

`down` сохраняет volume с БД. Не используйте `down --volumes` для разработки, если хотите сохранить данные. PostgreSQL init выполняется только на пустом volume: изменение паролей в .env не меняет уже созданные роли. Менять роли следует отдельной административной процедурой, не удалением пользовательских данных.

## Роли PostgreSQL и миграции

Init script создаёт aba_migrator и aba_app без SUPERUSER, CREATEDB, CREATEROLE, REPLICATION, BYPASSRLS. Migrator владеет schema platform; приложение имеет CONNECT, USAGE и только SELECT service_metadata. Доступ PUBLIC к DB/public schema отозван. Пароль bootstrap superuser передаётся только PostgreSQL; ни API, ни worker, ни migrate его не получают.

Первая миграция создаёт singleton service_metadata, version=1, timestamp. Она позволяет проверить реальный цикл миграций и readiness. Goose хранит версии отдельно в platform.goose_db_version; app не может писать в них. Identity/org tables созданы migration 00002, schema version=2; RLS/tenant tests описаны в TENANT_ISOLATION. Другие бизнес-таблицы позже.

```sh
sh scripts/migrate.sh up
sh scripts/migrate.sh status
# Только явный откат последней миграции, на текущем этапе удаляет metadata:
sh scripts/migrate.sh down
sh scripts/migrate.sh up
```

Status выводит version/state миграции. Down migration 00002 откажется при непустых identity tables. На пустой тестовой БД после down readiness=503 до up. Down не вызывается автоматически. Up повторяем, DB advisory lock предотвращает одновременное изменение migration bookkeeping.

## Native Go/Next.js с контейнерной БД

```sh
docker compose -f docker-compose.yml -f infrastructure/compose.host.yml up -d postgres
docker compose -f docker-compose.yml -f infrastructure/compose.host.yml run --rm migrate up
cd apps/web
npm ci --no-fund --no-audit
cd ../..
sh scripts/dev-api.sh
# В другом терминале:
sh scripts/dev-worker.sh
# В третьем:
sh scripts/dev-web.sh
```

Override публикует БД только 127.0.0.1:5432. Скрипты читают локальный .env; DATABASE_URL и MIGRATION_DATABASE_URL для native команд используют localhost. API_BASE_URL для native web также localhost. Если контейнерный API/web уже занял 8080/3000, остановите их перед native запуском. Production-настройка DATABASE_URL требует sslmode=verify-full; локальный Compose использует development+disable только во внутренней сети.

Конфигурация: APP_ENV, HTTP_ADDR, DATABASE_URL, DB_TIMEOUT=2s, REQUEST_TIMEOUT=5s, SHUTDOWN_TIMEOUT=10s, MAX_BODY_BYTES=1048576, DB_MAX_CONNECTIONS=5, LOG_LEVEL=info. DB_TIMEOUT обязан быть меньше REQUEST_TIMEOUT. Ошибки конфигурации называют параметр, но не его секретное значение. Request ID генерируется backend; входящий заголовок не считается доверенным ID. Текст panic/DB error и URL с паролями не логируются.

## Проверки без внешних сервисов

```sh
sh scripts/check.sh
```

Команды по отдельности:

```sh
cd apps/api
export GOCACHE="$PWD/../../.gocache"
test -z "$(gofmt -l .)"
go vet ./...
go test -race -count=1 ./...
go build ./cmd/api ./cmd/worker ./cmd/migrate ./cmd/healthcheck
cd ../web
npm ci --no-fund --no-audit
npm run lint
npm run typecheck
npm run check:openapi
npm run build
```

Unit-тесты используют test lookup для конфигурации и fake readiness, без БД. Graceful shutdown unit-тест открывает loopback ephemeral port: окружение должно разрешать локальные sockets. Production build uses Webpack, одинаково локально и в Docker. Next.js не обращается к API при build — статус загружается в runtime.

OpenAPI 3.0.3: packages/contracts/openapi.yaml (JSON-compatible YAML). Проверка swagger-parser плюс контроль набора реализованных GET endpoints. Бизнес-endpoints из архитектурного API_DESIGN — будущие, отсутствуют в исполняемом контракте.

## Изолированные PostgreSQL integration tests

```sh
sh scripts/integration.sh
```

Отдельный Compose project с уникальным именем, aba_test на tmpfs, без host DB port. После завершения удаляются только его тестовые контейнеры/volumes. Test runner получает read-only исходники. Тест отказывается запускать миграции, если имя БД/роль не совпадают, отсутствует marker aba-synthetic-isolated или platform schema содержит что-либо кроме safety marker. Пользовательская БД не удаляется. Проверяются connect, отсутствие схемы, up, repeat/concurrent up, runtime privileges, down/re-up и unreachable DB →503.

В CI отдельная service database; init.sh создаёт safety marker только при POSTGRES_DB=aba_test. Ручной запуск tagged tests требует именно новой изолированной БД и ABA_TEST_DATABASE_URL/ABA_TEST_MIGRATION_DATABASE_URL; не подставляйте обычный DATABASE_URL.

## Browser smoke

После npm ci и установки Playwright Chromium:

```sh
cd apps/web
npx playwright install chromium
cd ../..
# При доступной мигрированной локальной БД:
set -a
. ./.env
set +a
export SMOKE_REQUIRE_READY=1
sh scripts/smoke.sh
```

Скрипт строит Go binaries, запускает API/worker на 18080 и production web на 13000 (frontend build должен уже существовать), ждёт запуска, проверяет страницы, мобильный layout, реальный status, loading/error/unavailable. Затем останавливает настоящий API, проверяет server-side 503 и видимую ошибку в браузере, проверяет чистый SIGTERM API/worker. Работает с выбранным DATABASE_URL, миграции не выполняет. Без SMOKE_REQUIRE_READY=1 readiness=200 не проверяется и нельзя считать полный healthy smoke пройденным.

Для проверки уже поднятого Compose:

```sh
cd apps/web
SMOKE_REQUIRE_READY=1 npm run test:smoke -- --grep-invert 'реальный остановленный'
```

Для системного Chrome можно задать PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH; в ограниченном Codex окружении host browser может быть запрещён. Контейнерный вариант:

```sh
sh scripts/smoke-compose.sh
```

Он создаёт собственный синтетический stack на 28080/23000, использует официальный Playwright 1.64.0 container и локальные node_modules (сначала npm ci). Проверяет реальную остановку PostgreSQL и backend; удаляет только созданный им stack. Дополнительный скачиваемый browser image большой. Тест остановленного backend выполняется отдельно с SMOKE_BACKEND_DOWN=1; в обычном smoke этот сценарий пропущен намеренно, его запуск обязателен в полном smoke script.

## Диагностика

- Docker socket permission denied: проверить `docker version` и доступ Docker Desktop. В ограниченном агентском окружении выполнять Docker отдельной разрешённой командой; без daemon integration/stack smoke нельзя объявлять пройденными.
- Ready=503: postgres health, завершение migrate, роль aba_app и права SELECT platform.service_metadata. Не подключать API через postgres/aba_migrator ради зелёной проверки.
- Ошибка подключения после изменения пароля: init не повторяется для существующего volume. Проверьте актуальные роли и .env без вывода паролей в публичные логи.
- Порт занят: измените API_PORT/WEB_PORT в .env; native и контейнерный запуск не должны конкурировать.
- Frontend unavailable: проверить API_BASE_URL в server environment; localhost внутри web container — сам web, используйте api:8080. Ошибка не заменяется фиктивными данными.
- Browser executable missing: установить matching Chromium либо использовать smoke-compose.
- Turbopack служебный port запрещён: production build уже закреплён на Webpack; для ограниченного dev окружения можно `npm run dev -- --webpack`.
- Go cache permission denied: задайте GOCACHE на writable путь; scripts/check.sh использует .gocache в корне.
- Integration сообщает schema not clean: создайте новый test stack, не очищайте произвольную БД вручную ради теста.

## CI и дальнейшие этапы

.github/workflows/ci.yml: Go formatting/vet/race tests/build; frontend lint/typecheck/build; OpenAPI; Compose config; PostgreSQL integration и browser smoke. permissions contents:read, фиксированные tools/action versions, без платных сервисов/реальных ключей. Локальная static-проверка workflow: `go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.7 .github/workflows/ci.yml`. Запуск GitHub Actions не подтверждается локальным lint.

Следующий этап: Authentication/Organizations/Members, реальные sessions/CSRF, schema bootstrap без BYPASSRLS и тесты двух синтетических tenants. Каталог и ИИ начинать после изоляции; реальные переписки после отдельного решения по LLM, бюджету и ПД.

## ТЗ № 3: дополнительные настройки

Mailpit: http://127.0.0.1:8025, SMTP mailpit:1025 внутри Compose. Для native подключения SMTP только loopback override 1025. AUTH_EMAIL_KEY —64 hex символа; сохранять стабильным, не выводить в логи. PUBLIC_ORIGIN=http://127.0.0.1:3000 (при изменении WEB_PORT также изменить Origin). SESSION_TTL=168h, SESSION_IDLE_TTL=24h, VERIFICATION_TTL=24h. Existing .env уже дополнен, не копировать .env.example поверх него. Для нового окружения заменить dummy email key и все dev passwords.

Go integration теперь включает реальный Mailpit и два синтетических пользователя/tenant, RLS SQL checks, CSRF, cookies, token/session expiry. Full smoke: пять сценариев при доступном backend (включая register→mail→verify→login→org→edit→logout), затем отдельный реальный outage. В тестовом Compose origin=http://web:3000 совпадает с браузером в сети контейнеров; это не разрешение произвольных Origin. Для проверки обычного dev stack с browser container требуется тот же доверенный публичный origin, не подменять заголовок для обхода CSRF.

Предыдущие команды остаются применимы; описания stage 2 в старом отчёте исторические. Текущая документация и пределы: AUTHENTICATION/TENANT_ISOLATION/STAGE_3_REPORT. Следующий этап —каталог после проверки этой основы; password recovery, MFA, SMTP production, retention и deployment согласуются отдельно.

## Каталог и склад — этап 4

После существующего login создайте/выберите организацию. `/dashboard/products?org=<UUID>` →Добавить товар →SKU/цена в рублях/категория →Сохранить draft. В карточке корректируйте остаток с причиной, добавляйте варианты; публикация требует положительной цены. `/dashboard/categories` —плоские категории, `/dashboard/inventory` —остатки/история. `expected_version` обязателен в API; UI получает его из данных. На 409 обновите страницу и повторно проверьте target, автоматического перезаписывания нет. Archived не редактируется. Новые секреты не нужны.

Readiness теперь требует schema metadata3. Compose запускает migration 00003 отдельным job. Для обновления локального каркаса: `docker compose up --build -d --wait`; не удалять volume и не менять .env. Down00003 разрешён guard только при пустом каталоге; рабочие миграции не откатывать ради тестов.

`sh scripts/integration.sh` теперь действительно запускает `go test -race -tags=integration`, устанавливая gcc/musl-dev в одноразовый Go test container; нужны доступ к Alpine registry и Go dependencies. Новый catalog_test входит в существующий CI автоматически. Browser auth scenario расширен реальным каталогом, переключением организаций, историей, конфликтом двух вкладок и mobile width; всё синтетическое в отдельном stack.

## Этап 5

`docker compose up --build -d --wait` применяет00004 и запускает defaultmock без новых ключей. Сохраните старый .env/volume. В обычном магазине `/dashboard/ai-playground` создайте отдельную test organization. Заполните её каталог синтетическими active товарами и ручным stock; mock ищет SKU/модель по сообщению. Выберите карточку/quantity, обновите quote, явно согласитесь. `/dashboard/orders` —подтверждение с резервом, отказ/отмена/завершение. Платный API disabled; конфигурация и лимиты —AI_AGENT_IMPLEMENTATION.

Существующие check/integration/smoke команды включают новый stage5_test и расширенный browser scenario. Integration использует only aba_test marker/tmpfs; smoke only synthetic own project. Remote CI не запускается без push. Не откатывать dev migration00004: guard защищает test org/grants/chats/orders.

## Этап6 mock Telegram

Отдельный optional TELEGRAM_ENCRYPTION_KEY32bytes hex в ignored .env, TELEGRAM_TRANSPORT=mock и RETENTION_DAYS7. Сохранить прежние secrets/volumes. Controlled00005–00007 →readiness7, прежние migrations сохранены. UI `/dashboard/integrations/telegram` подключает только mock:<bot ID>; synthetic updates owner+CSRF/test scope. Реального Telegram token/network/tunnel не нужно. Worker email + inbox + outbox; LLM mock. Инструкция TELEGRAM_INTEGRATION. Native и Compose smoke генерируют свой ephemeral test key без вывода; integration копирует source snapshot в test container перед compilation, пользовательская БД не очищается.

## Этап 7

Схема теперь 7; используйте явный migration job до старта API. Default .env.example остаётся Mode A: budget0, mock, external=false, kills=true. Live API key/бот не нужны. Protected `/dashboard/ai-settings` доступен только test организации. Native команды `sh scripts/check.sh` не выполняют live E2E; PG `sh scripts/integration.sh` и Docker `sh scripts/smoke-compose.sh` изолированы и синтетические. При Docker socket permission denied не обходить sandbox; отметить эти проверки NOT RUN. Ручная будущая активация и остановка — PILOT_READINESS.md.

## Offline многоканальные проверки8Б.2

Стенд `infrastructure/compose.multichannel.yml` использует только cached Go/Postgres/Mailpit/Playwright images, `pull_policy:never`, internal network, DNS127.0.0.1, без host ports и proxy env. GOPROXY/GOSUMDB/GOVCS отключены. Перед запуском должны существовать local module cache, installed frontend dependencies и `.local/audit8b2/{go-cache,artifacts}`. Команды/честные результаты — README/STAGE_8B2_REPORT. Нельзя запускать npm ci/go get/обычный smoke-compose.sh в строгом offline-режиме.

Тестовые credentials/данные этого стенда не подходят production. Для обычного local mock WhatsApp потребуется отдельный32-byte hex WHATSAPP_ENCRYPTION_KEY на API/worker; live/external flags false, budget0. Connect не выполняет запрос в Meta, sending включается отдельно и только mock. Ротация реальных credentials/registration endpoints требуют отдельного задания и разрешения.
