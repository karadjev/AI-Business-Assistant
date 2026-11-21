# Этап 8А — фактический отчёт

Дата: 16.10.2026. SECURITY STATUS: **CONDITIONAL**. TELEGRAM LIVE READINESS: **NOT READY**. LLM BUDGET: **0 RUB**. EXTERNAL CALLS: **NONE для команд и изолированных тестов этого аудита**; глобальное отсутствие сетевой активности всего хоста не доказано и не заявляется.

CRITICAL FINDINGS: 0. HIGH FINDINGS: 3. FIXED FINDINGS: 8. OPEN FINDINGS: 2 (MEDIUM). Подробности, RED/GREEN и пределы: [SECURITY_AUDIT_REPORT.md](SECURITY_AUDIT_REPORT.md). Реальных Telegram API и LLM вызовов не было. Commit, push, deployment, dependency downloads, эскалация прав и туннели не выполнялись.

## Инвентаризация и источник доказательств

Существовали README, отчёты 2–7 и документы архитектуры/схемы/Agent/Telegram/cost/RLS. Все обязательные документы найдены; рабочие модули не создавались заново. Существующий development Compose project не останавливался и не мигрировался. Исходники — Go 1.26.2, pgx/goose, Next.js App Router/TypeScript/Tailwind; точные зависимости зафиксированы в `apps/api/go.mod/go.sum` и `apps/web/package-lock.json`. Использованы локально закэшированные Go/PostgreSQL18.3/Mailpit/Playwright images, без registry requests.

Карта потоков, реально работающие механизмы против запланированных, все 25 tenant-таблиц и глобальные исключения приведены в audit report. Дополнительный полный SQL inventory без данных — [SECURITY_RLS_INVENTORY.json](SECURITY_RLS_INVENTORY.json), получен из pg_catalog после миграции 8. В Git весь проект был untracked; утверждать «чистое дерево» нельзя. Для этого этапа использован предварительный локальный SHA-256 inventory `.local/audit8a/baseline.json`. Миграции 00001–00007 побайтно сохранены.

## Исправления

- A01 HIGH: webhook больше не вычисляет Argon2 dummy hash на каждый запрос; дешёвые отказы до DB limiter.
- A02 HIGH: повторная проверка отправки, session advisory fence, отмена sending/pending после disconnect/handoff, сериализация enqueue; сеть вне SQL-транзакции.
- A03 MEDIUM: crash после сохранения customer не выдаётся за завершённый ответ; lease conflict или один safe handoff без replay tools.
- A04 HIGH: неполный usage не считается известным нулём; zero-budget transport spy.
- A05 MEDIUM: sendMessage 5xx — uncertain без небезопасного retry.
- A06 LOW: JSON null не вызывает worker panic.
- A07 MEDIUM: readiness/AfterConnect проверяют ENABLE/FORCE flags, role/ownership и schema8.
- A08 MEDIUM: immutable message attribution и erasure-only channel UPDATE.

Открыто: A09 — операторская сверка uncertain delivery; A10 — восстановление pending budget после аварии до settle. Консервативная остановка сохраняет безопасность, но не является полноценным восстановлением. Устранение A09 нужно до Telegram live; A10 — до live LLM. Дополнительно не проверены real TLS/provider NLP/legal/backup restore/production monitoring.

## Миграция и совместимость

Новая `apps/api/migrations/00008_security_audit.sql`. Бизнес-таблицы не удаляются. Дополняет send-admission/control fence, disconnect/outbox functions и message update grants/policy. Readiness требует schema8; перед запуском обновлённого API нужна **явная миграция**, а не автоматическое изменение при старте.

Clean up/down/up и повторный up выполнены на отдельной `aba_test` с safety marker. Down8 с существующей organization намеренно отклонён: security downgrade данных не является безопасным штатным rollback. Не выполнять down на пользовательской БД. Публичные API routes/JSON schemas не добавлены; существующие 409/503 и Telegram processing semantics уточнены, OpenAPI validator пройден.

## Offline-изоляция

`infrastructure/compose.audit.yml` и `compose.audit-smoke.yml`: internal-only network, no default route (проверен `/proc/net/route`), DNS loopback, no ports, empty proxy env, cached images/pull never/no build, telemetry disabled. Docker endpoint проверен как локальный UNIX socket. Go modules — read-only cache; GOPROXY/GOSUMDB off, GOVCS off, GOTOOLCHAIN local, CGO=0. JS tools — установленные scripts напрямую без npm/npx install. Стенд не монтирует Docker socket и не использует host network.

macOS sandbox-exec не смог применить профиль: Operation not permitted. Это не было обойдено повышением прав. Поэтому host Go/npm runtime checks, для которых не доказана изоляция, не запускались. Контейнерный harness обеспечивает отдельное доказательство; не утверждается, что вся внешняя сеть хоста выключена.

Первая тестовая конфигурация tmpfs/noexec не позволяла исполнять Go test binaries: harness исправлен до тестов, без изменения приложения. Для destructive integration использован tmpfs на новой БД. Для restart smoke нужен отдельный named test volume: попытка с tmpfs показала ожидаемую утрату disposable схемы, после чего стенд заменён на named volume и проверка повторена. Пользовательская БД не затрагивалась. Первая усиленная pg_stat_activity проверка не имела видимости чужой роли; заменена отдельным runtime observer и затем прошла с реальным pg_locks wait.

## Команды и результаты

Команды ниже не скачивают зависимости. Нужны **уже имеющиеся** images/cache; при отсутствии остановиться, а не pull/install. Указать реальный локальный cache каталог в `ABA_OFFLINE_GOMODCACHE`. На этой машине: `/Users/ilezkaradjev/go/pkg/mod`. В `.local/audit8a/offline.env` хранится только этот путь, без секретов. `.local/audit8a/{go-cache,artifacts,browser-results}` — disposable результаты. Имена проектов должны начинаться `aba-audit8a-` и не совпадать с рабочим стеком.

```sh
mkdir -p .local/audit8a/go-cache .local/audit8a/artifacts .local/audit8a/browser-results
# Предварительно убедиться в локальном UNIX Docker endpoint и наличии cached images.
# Создать .local/audit8a/offline.env с ABA_OFFLINE_GOMODCACHE=<локальный cache>.
docker compose --env-file .local/audit8a/offline.env -p aba-audit8a-build-final -f infrastructure/compose.audit.yml up --pull never --no-build --abort-on-container-exit --exit-code-from go go
docker compose --env-file .local/audit8a/offline.env -p aba-audit8a-final -f infrastructure/compose.audit.yml up --pull never --no-build --abort-on-container-exit --exit-code-from integration integration
docker compose --env-file .local/audit8a/offline.env -p aba-audit8a-web-final -f infrastructure/compose.audit.yml up --pull never --no-build --abort-on-container-exit --exit-code-from web-check web-check
```

Integration требует **новый чистый** test project/БД. Повтор с уже заполненной БД намеренно откажет; удалить только свой audit project, либо выбрать новый. Не использовать общие команды очистки Docker. `scripts/integration.sh`, `smoke.sh`, CI не запускались как offline harness: в обычном окружении они могут устанавливать пакеты/зависимости.

| Проверка | Результат | Доказательство / ограничение |
|---|---|---|
| Go formatting | PASS | `gofmt -l cmd internal tests` пусто |
| Go unit tests | PASS | `go test -count=1 ./...` в offline container, все пакеты |
| Go vet | PASS | `go vet ./...`, exit0 |
| Go build API/worker/migrate/healthcheck | PASS | Локальные artifacts из текущего исходника, exit0 |
| Go race | NOT RUN | Нет C compiler в доступных offline Go/Playwright images; CGO race недоступен, ничего не скачано |
| PostgreSQL integration/RLS/migrations/regression3–7 | PASS | Финальный `TestDatabaseAndMigrations`: 9.074s, exit0; не тесты на mock БД |
| Все 25 tenant tables A/B SQL operations | PASS | ENABLE/FORCE, SELECT/INSERT/UPDATE/DELETE, WITH CHECK и runtime owner flags |
| Новые crash/handoff/disconnect/budget/channel-negative cases | PASS | Все subtests audit_test.go в финальном PG прогоне |
| Actual Telegram HTTP contract | PASS | Loopback emulator, реальные POST/JSON adapter calls; не внешний Telegram |
| Frontend lint | PASS | Установленный eslint JS в internal-network container |
| TypeScript | PASS | tsc --noEmit --incremental false |
| OpenAPI | PASS | `scripts/check-openapi.mjs`, локальные зависимости |
| Свежая frontend production build | NOT RUN | Linux SWC/Tailwind native packages отсутствуют; установлены Darwin-варианты; host build без enforced egress не запускался |
| CI syntax | PASS | Cached actionlint1.7.7, `--network=none`, shellcheck/pyflakes disabled |
| GitHub CI execution | NOT RUN | Внешний GitHub/скачивание actions запрещены |
| Compose validation | PASS | `config --quiet`, без вывода секретов |
| Docker startup | PASS | Отдельные API8/worker8/migrate8 binaries + PG/Mailpit и cached web7 image, healthchecks |
| Browser regression | PASS | 5 прошли; один backend-down test запускается отдельно; исходники frontend8А не менялись |
| Реальный остановленный backend → UI unavailable | PASS | Отдельный Playwright unavailable.spec.ts, 1 passed, без подмены ответа BFF |
| Token/log/bundle scan | PASS (ограниченный) | 249 файлов, 0 redacted signature/exact-env matches; logs/API redaction tests |
| External vulnerability feeds / package freshness | NOT RUN | Нужна внешняя сеть; аудита всех CVE зависимостей не заявляем |
| Реальные Telegram APIs / LLM / paid E2E | NOT RUN | Нет разрешения, ключей/бота/бюджета; намеренно не запускались |

Smoke использует **cached web7** для регрессии, это не свежая сборка и не доказательство воспроизводимости production build. Никаких новых frontend runtime изменений в этапе 8А нет. Go artifacts текущие, содержат миграцию8. Полный CI/race и production deploy не объявляются PASS.

### Docker smoke и восстановление

Команды (проект только `aba-audit8a-smoke`, никаких host ports):

```sh
docker compose --env-file .local/audit8a/offline.env -p aba-audit8a-smoke -f infrastructure/compose.audit.yml -f infrastructure/compose.audit-smoke.yml config --quiet
docker compose --env-file .local/audit8a/offline.env -p aba-audit8a-smoke -f infrastructure/compose.audit.yml -f infrastructure/compose.audit-smoke.yml up --pull never --no-build -d --wait postgres mailpit migrate api worker web
docker compose --env-file .local/audit8a/offline.env -p aba-audit8a-smoke -f infrastructure/compose.audit.yml -f infrastructure/compose.audit-smoke.yml run --no-deps --rm --pull never browser
```

Повторный браузерный прогон на named test volume: 5 passed (50.0s), 1 intentional skip для отдельно запускаемого backend-down. При остановке **тестового** PostgreSQL live=200, ready=503; после start ready=200 и `users count>0`, схема/синтетические записи сохранились. API и worker остановлены SIGTERM, exit0 у обоих. Отдельный backend-down browser test: 1 passed (13.9s), UI показывает unavailable при действительно остановленном API. Для очистки после теста — `down --volumes` с **этим же точным audit project**, никогда без `-p` в рабочей директории.

Полный kill worker в каждой точке внешнего HTTP не симулировался: leases/committed-message crash окна воспроизводятся SQL fixture и повторным Service/worker вызовом; uncertainty recovery доказан тестами, но не полноценным production chaos drill. Backup restore и alert response NOT RUN.

## Offline Telegram сценарии

| Сценарий | Результат | Где фактически проверен |
|---|---|---|
| Успешный update→catalog→outbox | PASS | `testTelegramHTTPAudit`: реальный adapter POST/JSON на loopback |
| Неверный secret / чужой integration | PASS | stage6 webhook API + локальный wire fixture, no principal leak |
| Неавторизованный user / allowlist | PASS | stage7 + wire fixture, блок до conversation |
| Повтор update / callback | PASS | Unique inbox, concurrent duplicate webhook, одна pending request |
| Два одновременных update | PASS | stage6 concurrent dedup/claims и real HTTP fixture |
| Не по порядку numeric ID | PASS для arrival semantics | Новый PG test: 200 затем100, durable order/single lease; numeric reordering не обещается |
| Ошибка отправки /429 | PASS | Real HTTP wire500/429, bounded retry/uncertain |
| Timeout | PASS | `TestAuditHTTPTimeoutIsUncertain`: локальный server задерживает реальный запрос |
| Worker restart/потеря receipt | PASS для моделируемого окна | stage6 stale lease/re-claim и HTTP uncertain/restarted Service |
| Disconnect с claimed job | PASS | Новый negative PG test + fence/lock waiter; wire ingress после disconnect403 |
| Handoff / staff reply | PASS | Stage5/6, wire fixture и браузер; queued auto отменяется |
| Создание заявки после consent | PASS | Inline quote callback; no reserve до staff |
| Изменение цены / отсутствие товара | PASS | Реальные PG order/catalog tests и browser stale-price conflict; не каждый путь повторён через wire fixture |
| Ошибка БД | PASS для readiness/API outage | Изолированная остановка PG →503, восстановление→200; не полный webhook fault E2E |

Таким образом не заявляется, что все перечисленные сбои покрыты одним сквозным HTTP сценарием. Политики безопасности проверены разными независимыми уровнями. 30+ evaluation fixtures проверяют mock/schema/no mutation; настоящая семантика ответов модели NOT RUN.

## Критерии A–T

| Критерий | Статус | Основание |
|---|---|---|
| A Инвентаризация | PASS | Компоненты, потоки, код/SQL, dependency/CI review |
| B Webhook offline | PASS | Negative handler и PG/wire tests |
| C Principal | PASS | IDs/org/capability/allowlist и две организации |
| D Повторная доставка | PASS | Unique key + duplicate/update consent tests |
| E Конкуренция | PASS | PG inventory/queues/ledger/fence tests; race detector отдельно NOT RUN |
| F Отключение отправки | PASS | Claimed send revalidation и disconnect/control cancellation |
| G Все tenant FORCE RLS | PASS | 25 tables actual catalog и runtime SQL matrix |
| H Межорганизационный доступ | PASS | API/RLS negative tests; trusted backend boundary описана |
| I Без согласия заявок нет | PASS | Quote-specific consent / tool rejection |
| J Нет overselling | PASS | Две concurrent confirmations, last-unit test |
| K Zero-budget transport | PASS | Actual Compatible RoundTripper count0 |
| L Секреты API/log | PASS в проверенной области | Redacted scan + regression tests; не абсолютная гарантия |
| M Frontend/BFF | PASS в development | Lint/typecheck/browser cached runtime; fresh build/HTTPS NOT RUN |
| N Offline HTTP contract | PASS | Loopback server, no external route |
| O Критические исправлены/переход заблокирован | PASS | Critical0, fixed8, live NOT READY, A09/A10 OPEN |
| P Регрессия | PASS доступных проверок | Go/PG/browser; race и fresh build NOT RUN |
| Q Test bot setup | PASS | PREPARED/REQUIRES APPROVAL разделены |
| R Readiness checklist | PASS | Категории, метод, evidence и ответственные |
| S Audit report evidence | PASS | Код/SQL/test names, RED/GREEN, scope limitations |
| T Ограничения этапа | PASS для выполненных действий | No external/install/commit/push/deploy; host-global egress не заявлен |

## Изменённые файлы

Созданы:

- `apps/api/migrations/00008_security_audit.sql`
- `apps/api/internal/telegram/send_guard.go`
- `apps/api/internal/{agent,http,telegram}/audit_test.go` (три файла)
- `apps/api/tests/integration/audit_test.go`
- `apps/api/tests/integration/telegram_http_audit_test.go`
- `infrastructure/compose.audit.yml`
- `infrastructure/compose.audit-smoke.yml`
- `docs/SECURITY_AUDIT_REPORT.md`
- `docs/SECURITY_RLS_INVENTORY.json`
- `docs/TELEGRAM_TEST_BOT_SETUP.md`
- `docs/TELEGRAM_LIVE_READINESS_CHECKLIST.md`
- `docs/STAGE_8A_REPORT.md`

Изменены:

- `apps/api/internal/agent/provider.go`
- `apps/api/internal/conversations/service.go`
- `apps/api/internal/http/stage6.go`
- `apps/api/internal/platform/db/db.go`
- `apps/api/internal/telegram/{http_transport,service,worker}.go` (три файла)
- `apps/api/tests/integration/database_test.go`
- `apps/api/tests/integration/stage6_test.go` (добавлено второе настоящее staff message для независимых retry/lost-receipt сценариев после исправленного handoff, проверки не удалены)
- `README.md`
- `docs/{LLM_COST_CONTROL,MESSAGE_DELIVERY,SECURITY,TECHNICAL_DECISIONS,TELEGRAM_SECURITY,TENANT_ISOLATION}.md` (шесть файлов)

Lockfiles/зависимости/публичные routes/frontend runtime/CI workflow не менялись; CI syntax и контракт проверены. Локальные `.local/audit8a` binaries/cache/logs/screenshots не являются публикуемыми файлами и игнорируются Git. Ошибочно считать исходные hidden файлы новыми из-за неполного первоначального hash inventory — они не создавались в этом этапе.

## Следующий этап 8Б и отдельные разрешения

Рекомендуется сначала закрыть A09 и добавить crash-reconciliation A10, обеспечить offline C compiler/Linux frontend dependencies и выполнить пропущенные race/build; подготовить операторский recovery drill. После повторного checklist можно рассматривать **контролируемый Telegram test с mock LLM**, не настоящий пилот с покупателями.

Новое разрешение потребуется для создания/использования настоящего тестового бота, получения тестовых ID через внешние сервисы, каждого getMe/getWebhookInfo/setWebhook/deleteWebhook, отправки/подтверждения callback через настоящий Telegram, проверки/публикации HTTPS endpoint, токен-ротации BotFather, любых downloads/CVE feeds, live LLM/API key/положительного денежного бюджета, реальных персональных данных, commit/push/deployment. Ни одно из этих действий не выполнялось. Утверждение privacy policy и независимая проверка применимых требований — отдельные gates; документация 8А не является правовым заключением.

Работа этапа остановлена после локальных проверок. Реальная интеграция не активирована.

Финальный повтор после добавления arrival-order case: проект `aba-audit8a-order-final`, TestDatabaseAndMigrations **9.074s, PASS**. Дополнительный timeout unit: telegram package **0.117s, PASS**. Test logs (включая RED до исправлений) сохранены локально в ignored `.local/audit8a/results/`; основной evidence — воспроизводимые исходники тестов и inventory, а не доверие к этим логам.

Завершение: создано14 и изменено16 публикуемых файлов (всего30; список выше). Все созданные audit-контейнеры удалены; отдельный synthetic smoke volume удалён. Проверка `docker ps` после очистки: исходные ai-business-assistant API/worker/web/PostgreSQL/Mailpit остаются healthy с прежним uptime; audit8А контейнеров нет. Рабочие миграции и данные не изменялись. Readiness этого старого development стека не доказывает применение security patch: обновление его бинарников/схемы в этом аудите не выполнялось.
