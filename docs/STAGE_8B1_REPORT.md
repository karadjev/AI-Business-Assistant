# Этап8Б.1 — Telegram Reliability & Pre-Live Hardening

Дата:18.10.2026. Проверка фактических исходников, SQL и синтетических процессов; не доверие к отчёту8А.

- **SECURITY STATUS: CONDITIONAL.**
- **TELEGRAM LIVE READINESS: NOT READY.**
- **CRITICAL/HIGH FINDINGS:** CRITICAL0; HIGH3 из8А, все перепроверены FIXED; открытых HIGH0.
- **FIXED/OPEN FINDINGS:** совокупно11 FIXED,0 OPEN известных findings. В этом этапе закрыты A09/A10 (MEDIUM), дополнительно найден/исправлен A11 (MEDIUM). NOT RUN проверки не маскируются этим счётчиком.
- **DELIVERY UNKNOWN STRATEGY:** остановка автоматических повторов; staff delivered/abandon, версии, idempotency, actor audit и fencing. Exactly-once не гарантируется.
- **BUDGET RECOVERY STRATEGY:** pending старше5min → UNKNOWN с полным conservative charge, освобождается только active slot; спорные деньги не возвращаются в доступный бюджет. Повтор terminal completion — no-op.
- **LLM BUDGET:0 RUB**, provider NOT SELECTED, mock active, live disabled.
- **EXTERNAL CALLS:NONE** в командах/тестах этапа. Не утверждается отсутствие любых фоновых соединений всего хоста.
- **NEW MIGRATIONS:** `00009_reliability.sql`; существующие00001–00008 побайтно сохранены.

## Исходное состояние и план

Прочитаны STAGE_8A_REPORT, SECURITY_AUDIT_REPORT, readiness checklist и test bot setup, существующий Go/SQL/worker/BFF/TSX код и offline Compose. Git по-прежнему весь untracked (`??`); сделан отдельный SHA-256 снимок до правок `.local/audit8b1/baseline.json`, пользовательские изменения не объявлялись «чистыми» и не удалялись. Рабочий development stack не мигрировался/не останавливался. Ключей/токена не запрашивали, `.env` не меняли.

Порядок: повтор HIGH baseline → воспроизведение отсутствующего recovery → миграция9/backend → текущие UI/BFF/OpenAPI → crash/concurrency/negative tests → offline regressions → документы. Новых зависимостей, сервисов, Redis или отдельного Agent не добавлено.

## HIGH: фактический статус

| Finding | Статус | Проверка18.10 |
|---|---|---|
| A01 Argon2 на публичном webhook | FIXED | `TestAuditWebhookRejectsMalformedBeforeDatabase`, current handler без auth.New на запрос; unit PASS |
| A02 stale outbox send после disconnect/handoff | FIXED | PG handoff/disconnect/admission tests с реальным pg_locks waiter + повтор в финальном suite |
| A04 partial usage как известный0 | FIXED | `TestAuditIncompleteUsageIsUnknown`, zero-budget actual Compatible transport spy и ledger integration |

Начальный baseline до реализации: unit http/agent PASS; полный PG `TestDatabaseAndMigrations` **10.945s PASS**. Незакрытых HIGH, блокирующих продолжение, не обнаружено. Это не гарантия отсутствия иных невыявленных уязвимостей.

## Исправления и схема

1. A09: outbox version, staff-only local resolution с session/CSRF/version/key; решения delivered/abandon, без resend. Private integration fence сериализует resolution/send/disconnect. Stale capability finish не перезаписывает результат. Новый журнал `telegram_delivery_events` хранит попытку/transition/time/operator, не token/payload/capability.
2. A10: private `llm_settle_core`, совместимый авторизованный settle; fixed-age global maintenance и owner manual recover. Все операции блокируют platform→reservation→usage, repeat settle/recover не списывает/не освобождает повторно. Новый `llm_budget_events` сохраняет reserve/terminal accounting+actor; worker tick выполняет recovery независимо от live enabled, без HTTP.
3. A11: HTTP500 с error_code429 ранее становился retry_after (воспроизведено новым unit: RED). Исправлен приоритет: HTTP5xx всегда uncertain для sendMessage; retry только HTTP429+code429. Финальные unit/PG GREEN.

Новые audit таблицы FORCE RLS с organization_id и composite FK на parent `(organization_id,id)`, без runtime DML grants; scoped staff read. Реальная runtime aba_app проверена на27 tenant tables и подмене tenant, без использования миграционной роли как единственного доказательства. Inventory: [SECURITY_RLS_INVENTORY_8B1.json](SECURITY_RLS_INVENTORY_8B1.json). Старые proof/session boundaries и encrypted secrets сохранены.

Миграция9 не удаляет бизнес-строки, не включает внешние сервисы и не поднимает бюджет. Сохраняет private v8 settle definition только для чистого guarded down. Полный clean up/down до1/up, repeated и concurrent up пройдены в aba_test с safety marker; down9 с организациями намеренно запрещён. Runtime readiness требует9. **Явно применить миграцию до обновления рабочего бинарника**; сам рабочий стек этим этапом не обновлялся.

## API и UI

Все endpoints с существующей session/auth/CSRF/rate limit/error model и same-origin BFF:

| Endpoint | Права / результат |
|---|---|
| GET `/api/v1/organizations/{id}/integrations/telegram/deliveries` | Staff, первые50 uncertain/failed, с приоритетом actionable UNKNOWN с version/events |
| POST `/api/v1/organizations/{id}/integrations/telegram/deliveries/{resource}/resolve` | Staff, explicit confirm/version/UUID key; delivered/abandon;0 network calls |
| GET `/api/v1/organizations/{id}/llm-reservations` | Staff, последние50 операций и audit |
| POST `/api/v1/organizations/{id}/llm-reservations/{resource}/recover` | Owner, confirm conservative charge; active409, terminal idempotent200 |

Нет изменения существующих маршрутов или значений старых статусов. `uncertain` отображается как UNKNOWN. 401/403/404/409/422 и повтор request key проверены. OpenAPI0.8.1 и validator path set обновлены.

На Telegram странице — текущий DeliveryRecovery, явный dialog, no resend, loading/error/empty, история попыток. В ai-settings — BudgetRecovery, owner-only manual action, pending/UNKNOWN/amount/audit. Никаких фиктивных переписок, расходов или успешных операций. Backend повторно проверяет org/role, скрытие кнопки не является авторизацией. Документы восстановления: [TELEGRAM_DELIVERY_RECOVERY.md](TELEGRAM_DELIVERY_RECOVERY.md), [LLM_BUDGET_RECOVERY.md](LLM_BUDGET_RECOVERY.md).

## Failures: доказательства

- **Accepted Telegram/no receipt:** actual local HTTP получает JSON и закрывает socket без ответа; transport unknown, ровно1 запрос, без retry.
- **Process crash после отправки:** дочерний процесс реально `os.Exit(86)` после receipt и до DB finish, без defer cleanup. После expiry новый отдельный процесс worker logic переводит sending в uncertain и не делает send. Это не только функция, возвращающая error.
- **Два worker:** первый actual HTTP request удерживается emulator; второй claim того же задания невозможен. Один sending/sent audit transition.
- **Manual recovery:** два одинаковых конкурентных решения → один operator event; changed decision/key409, anonymous401, CSRF403, чужие данные denied, stale worker finish no-op.
- **429/5xx/timeout:** existing actual HTTP/PG tests плюс новый contradictory5xx и socket drop, bounded retries; exactly-once не обещается.
- **Budget crash:** отдельные процессы `os.Exit(87)` после reserve и после реального ответа loopback Compatible mock, до settle. Два **новых процесса** конкурентно исполняют recover_once;40 synthetic microRUB учтены ровно1 раз, reserved уменьшен на40, active_calls на2, audit2 events на операцию. Late known0 settle не возвращает UNKNOWN.
- **Known usage:** параллельные settle одной reservation дают charged11 и один terminal event; повтор recover не меняет сумму. Owner/member/foreign/private-core ограничения проверены.
- **LLM timeout:** настоящий локальный HTTP задерживает ответ; Metered записывает UNKNOWN/full1100 synthetic microRUB, не refund. Это только изолированный fake ledger, не денежное разрешение пользователя.
- **Disconnect с очередью:** новые и claimed pending/sending не уходят; существующая регрессия8А подтверждена.

## Offline-среда

Docker endpoint проверен по локальным metadata: UNIX socket desktop-linux, без DOCKER_HOST/CONTEXT override. Использованы уже закэшированные images; `pull_policy:never`, `--pull never --no-build`, internal network, DNS127.0.0.1, proxy env пустые, no host ports, no host network/socket mount. В smoke фактически `Internal=true` и нет default route в `/proc/net/route`; telemetry выключена. Не делался «пробный» запрос в интернет. Go GOPROXY/GOSUMDB/GOVCS off, GOTOOLCHAIN local, GOTELEMETRY off; cache read-only. Node tools — installed JS напрямую без npm/npx install.

Сеть изолирована для audit-контейнеров, не всего хоста. Правами sandbox не управляли и не повышали. Обычные install/CI/smoke scripts не запускались как offline setup: они могут скачивать зависимости. Docker CLI обращался только к local daemon; registry pulls/внешних providers не было.

## Результаты проверок

| Проверка | Статус | Фактический результат |
|---|---|---|
| Go formatting | PASS | gofmt list пусто |
| Go tests | PASS | Все unit packages, в том числе новый socket-drop/contradictory429 |
| Go vet | PASS | exit0 |
| Go build API/worker/migrate/healthcheck | PASS | Текущие binaries, exit0 |
| Go race detector | NOT RUN | Cached Go1.26.2 Alpine и Playwright images не содержат cc/gcc/clang; CGO=0, race требует CGO compiler. Ничего не установлено; обычные concurrency tests не объявляются заменой race |
| PostgreSQL integration | PASS | Финальный проект aba-audit8b1-complete,10.435s, exit0 |
| FORCE RLS/tenant isolation | PASS |27 tables, direct aba_app SQL A/B SELECT/INSERT/UPDATE/DELETE/WITH CHECK |
| Миграции | PASS | Clean up/down/up, repeat/concurrent up и nonempty down guard, внутри полного PG suite |
| Frontend lint | PASS | Installed ESLint в internal-only container |
| TypeScript typecheck | PASS | tsc --noEmit --incremental false |
| Frontend production build | NOT RUN | На host node_modules только `@next/swc-darwin-x64`, `@tailwindcss/oxide-darwin-x64`; в доступном cached standalone web нет Linux SWC/oxide. Native Linux dependencies не скачивались. Host build без доказанного egress isolation не запускался |
| OpenAPI validation | PASS | Local swagger-parser + exact implemented paths |
| Новые текущие UI components | PASS | Реальный Chromium + installed pure-JS TS/webpack, loading/403/409/explicit action/UNKNOWN/tenant switch/owner UI |
| Текущий BFF route contract | PASS | Actual TS source transpiled, restricted path/method/cookie/CSRF forwarding; fetch mocked локально |
| Docker Compose config/start | PASS | New Go9 binaries + PG/Mailpit/current worker, cached web7 runtime |
| Browser regression | PASS |5 passed46.2s,1 intentional backend-down skip; отдельный down тест ниже |
| SIGTERM | PASS | API/worker9 exit0, рабочий стек не затронут |
| Реальный backend-down UI | PASS | Отдельный unavailable browser test с остановленным API, не browser fake response |
| Actual next full8Б.1 SSR/BFF build | NOT RUN | Cached web7 не выдаётся за новую Next сборку; отдельные component/BFF checks не заменяют её |
| Real Telegram/LLM/HTTPS/production | NOT RUN | Запрещены, намеренно не выполнялись |

Subprocess helper зарегистрирован как отдельный Go test и **SKIP** без специального env; в основном PG suite он реально выполнялся дочерними процессами (ожидаемые exit86/87 и новые normal0). Этот технический skip не означает, что crash scenarios не запускались.

Изначальный RED recovery contract на схеме8: функции отсутствуют,6.377s FAIL (ожидаемо). Первый green-кандидат выявил ошибку теста на identity audit IDs: PostgreSQL428C9 срабатывал до проверки привилегий. Вставка с OVERRIDING SYSTEM VALUE исправила именно тест, чтобы требовать настоящий42501; финальный весь matrix PASS. Browser harness первоначально отдавал JS без UTF-8 charset, из-за чего locator не находил кириллицу; исправлены headers стенда, текущие компоненты прошли без заглушки результата. Эти промежуточные FAIL не скрыты.

Дополнительная ручная SQL-подготовка старого reservation в smoke не была выполнена: sandbox policy отклонила ввод/exec SQL, эскалация не запрашивалась. Не используется как PASS. Нужные crash/restart/recover случаи вместо этого доказаны разрешённым изолированным integration harness с реальными дочерними процессами. Данные рабочего контура не использовались.


## Команды воспроизведения без сети

Нужны заранее имеющиеся images `golang:1.26.2-alpine3.23`, `postgres:18.3-alpine3.23`, `axllent/mailpit:v1.21`, `mcr.microsoft.com/playwright:v1.64.0-noble`; smoke дополнительно использует cached `aba-stage7-smoke-{api,worker,migrate,web}:latest`. При отсутствии не pull/install, а NOT RUN. Создать `.local/audit8b1/{go-cache,artifacts,browser-results}` и `offline.env` только с `ABA_OFFLINE_GOMODCACHE=<уже имеющийся каталог>`. Здесь cache `/Users/ilezkaradjev/go/pkg/mod`.

```sh
docker compose --env-file .local/audit8b1/offline.env -p aba-audit8b1-build-complete -f infrastructure/compose.reliability.yml up --pull never --no-build --abort-on-container-exit --exit-code-from go go
docker compose --env-file .local/audit8b1/offline.env -p aba-audit8b1-complete -f infrastructure/compose.reliability.yml up --pull never --no-build --abort-on-container-exit --exit-code-from integration integration
docker compose --env-file .local/audit8b1/offline.env -p aba-audit8b1-types-final -f infrastructure/compose.reliability.yml up --pull never --no-build --abort-on-container-exit --exit-code-from web-check web-check
docker compose --env-file .local/audit8b1/offline.env -p aba-audit8b1-ui-verified -f infrastructure/compose.reliability.yml run --no-deps --rm --pull never web-check node /scripts/test-recovery-components.cjs
docker compose --env-file .local/audit8b1/offline.env -p aba-audit8b1-smoke -f infrastructure/compose.reliability.yml -f infrastructure/compose.reliability-smoke.yml up --pull never --no-build -d --wait postgres mailpit migrate api worker web
docker compose --env-file .local/audit8b1/offline.env -p aba-audit8b1-smoke -f infrastructure/compose.reliability.yml -f infrastructure/compose.reliability-smoke.yml run --no-deps --rm --pull never browser
```

Integration требует свежую пустую aba_test с marker и aba_app/aba_migrator role separation. Повторный запуск на заполненной БД намеренно откажет — использовать новый **свой** audit project. Нельзя подставлять рабочую БД/делать глобальную очистку Docker. Smoke binaries собраны из текущих Go исходников; web7 cached служит только regression, не fresh build. Для backend-down отдельно остановлены только свои api/worker; их exit0, BFF/UI unavailable проверены отдельным тестом13.9s. Финальные API9 после пересоздания вернули health/live, health/ready и status200.

`test-recovery-components.cjs` создаёт только временный JS bundle/loopback server внутри изолированного контейнера и убирает его; React-компоненты берёт из текущих TSX, API ответ mock принадлежит этому тесту. BFF test исполняет текущий route после TypeScript transpile, перехватывая fetch без сети. Это уровневое доказательство UI/contract, не full Next SSR pipeline. Production build и race по-прежнему NOT RUN.

## Критерии A–T

| Критерий | Результат | Доказательство |
|---|---|---|
| A HIGH статусы | PASS | A01/A02/A04 FIXED, baseline и финальная регрессия |
| B Нет open HIGH | PASS в проверенной области |0 open HIGH, без заявления об отсутствии всех неизвестных ошибок |
| C Unknown без опасных повторов | PASS | Socket drop, crash after receipt, restart no resend, manual delivered/abandon |
| D Update не дублирует заявки | PASS | Existing PG/wire/браузер duplicate callback/order |
| E Два worker | PASS | Одновременный claim, один удержанный actual HTTP send |
| F Restart outbox | PASS | Новый отдельный процесс с новой DB connection, uncertain после expiry |
| G Budget idempotency | PASS | Concurrent settle/recover, один terminal audit, counters |
| H Спорный резерв не переиспользуется | PASS | Full UNKNOWN charged, late0 settle не refund |
| I Budget0 блокирует live | PASS | Actual adapter transport count0 и config tests |
| J RLS/tenant | PASS |27 tables, runtime two-tenant SQL, API/service denial |
| K Offline fault tolerance | PASS | Local HTTP + реальные subprocess crash/restart |
| L Race выполнен либо честный NOT RUN | PASS требования отчётности | Сам race NOT RUN, нет C compiler/CGO |
| M Production build либо честный NOT RUN | PASS требования отчётности | Сам Next build NOT RUN, нет Linux SWC/oxide |
| N Доступная регрессия | PASS | Go/PG/старый browser regression/new components, без новых блокирующих FAIL |
| O Checklist | PASS | Актуализирован18.10, настоящий Telegram NOT RUN |
| P Инструкции recovery | PASS | Два новых документа, статусы/оператор/финансы/rollback |
| Q Существующие данные | PASS в выполненных действиях | Только synthetic audit projects; рабочая БД не менялась; migrations1–8 hashes unchanged |
| R Реальные API не вызывались | PASS | Mock/loopback only, enforced internal network для тестов |
| S Commit/push/deploy/escalation | PASS | Не выполнялись |
| T Отчёт/ограничения/8Б.2 | PASS | Этот документ и обновлённый audit report |

## REMAINING RISKS

- Принципиально отсутствует универсальный Telegram exactly-once; delivered — решение сотрудника, не проверка Telegram. Abandon может означать отказ повторять уже доставленное сообщение. Resend не предоставляется.
- UNKNOWN LLM может завышать расход. Поздний actual usage автоматически не уменьшает его; post-billing adjustment workflow не реализован. Это остаточное ограничение выбранной консервативной стратегии, не возможность повторного расхода.
- Не выполнены race, fresh Next production/full SSR, реальные Telegram/TLS/rate limits/provider billing, production alerts, правовая проверка и реальный pilot. Показанные PASS не заменяют эти gates.
- Локальная auth/browser проверка работает в development HTTP; настоящий Secure cookie/TLS posture не проверен.
- Delivery audit удаляется вместе с test outbox при retention purge; financial audit сохраняет минимальную связь с организацией/сотрудником. Сроки реальных данных не утверждены.
- Работающий старый development stack не обновлён на schema9/новые бинарники. Его healthy не доказательство применения patch; рабочий запуск обновления — отдельный согласованный scope.

## APPROVALS REQUIRED FOR8Б.2

Подготовить offline C toolchain и точные Linux native frontend dependencies; **любая загрузка из интернета требует отдельного разрешения**, текущего задания недостаточно. После этого повторить race/полную новую Next build и UI/BFF regression. Актуализировать operator/retention checklist, выполнить recovery drill из полной сборки.

Для настоящего Telegram отдельно потребуются тестовый бот, согласившиеся test ID/allowlist, одобренный HTTPS endpoint и явное разрешение на конкретные getMe, getWebhookInfo, setWebhook, deleteWebhook и test sends/реальные callbacks. Токен передавать только защищённо по TELEGRAM_TEST_BOT_SETUP, не в чат/логи/Git. Настоящий LLM/положительный бюджет **не запрашиваются и не включаются автоматически**; разрешение Telegram не разрешает LLM. Production, реальные клиентские данные, commit/push/deploy остаются запрещены.

## FILES CHANGED

Создано15, изменено18 файлов; удалено0. Полный перечень:

### Созданы

- `apps/api/internal/costcontrol/recovery.go`
- `apps/api/internal/http/recovery.go`
- `apps/api/internal/telegram/recovery.go`
- `apps/api/internal/telegram/recovery_test.go`
- `apps/api/migrations/00009_reliability.sql`
- `apps/api/tests/integration/reliability_test.go`
- `apps/web/src/components/BudgetRecovery.tsx`
- `apps/web/src/components/DeliveryRecovery.tsx`
- `docs/LLM_BUDGET_RECOVERY.md`
- `docs/SECURITY_RLS_INVENTORY_8B1.json`
- `docs/STAGE_8B1_REPORT.md`
- `docs/TELEGRAM_DELIVERY_RECOVERY.md`
- `infrastructure/compose.reliability-smoke.yml`
- `infrastructure/compose.reliability.yml`
- `scripts/test-recovery-components.cjs`

### Изменены

- `README.md`
- `apps/api/cmd/worker/main.go`
- `apps/api/internal/http/stage7.go`
- `apps/api/internal/platform/db/db.go`
- `apps/api/internal/telegram/http_transport.go`
- `apps/api/tests/integration/audit_test.go`
- `apps/api/tests/integration/database_test.go`
- `apps/web/src/app/api/[...path]/route.ts`
- `apps/web/src/components/AISettings.tsx`
- `apps/web/src/components/TelegramIntegration.tsx`
- `docs/LLM_COST_CONTROL.md`
- `docs/MESSAGE_DELIVERY.md`
- `docs/SECURITY_AUDIT_REPORT.md`
- `docs/TECHNICAL_DECISIONS.md`
- `docs/TELEGRAM_LIVE_READINESS_CHECKLIST.md`
- `docs/TENANT_ISOLATION.md`
- `packages/contracts/openapi.yaml`
- `scripts/check-openapi.mjs`

Lockfiles/версии зависимостей и исходные миграции1–8 не менялись. Local cache/logs/screenshots/binaries в `.local/audit8b1` игнорируются Git и не входят в эти списки.

## Итоговая проверка перед остановкой

Повторный current-source UI/BFF test PASS, в том числе стабильный idempotency key при повторе решения. Финальный lint/typecheck/OpenAPI PASS. Redacted scan6236 локальных файлов (включая имеющийся Go cache и bundle):0 token/key/private-key signatures или точных длинных local-env secret matches; значения не выводились. Это ограниченная сигнатурная проверка, не абсолютное доказательство отсутствия любого секрета. Проверенные текущие local mode flags: mock/mock, live=false, budget0, Telegram external=false/sending=false/kill=true. Миграции1–8 SHA-256 совпали со снимком начала этапа. Полный журнал команд/test stdout сохранён в ignored `.local/audit8b1`, без публикации секретов.

Дополнительный финальный regression:51 старый failed перед actionable UNKNOWN не скрывает инцидент — uncertain сортируются первыми. Тест включён в основной suite; повторный полный PG **10.435s PASS**, Go unit/vet/build/format PASS. Публичный контракт не менялся. Это уточнение доступности очереди инцидентов, не новая внешняя операция.

Очистка завершена: созданные aba-audit8b1-* контейнеры и отдельный synthetic smoke volume удалены; caches/доказательства оставлены только в ignored .local. Исходные API/worker/web/PostgreSQL/Mailpit ai-business-assistant по-прежнему healthy с прежним uptime. Commit/push/deployment/повышение привилегий не выполнялись. Остановка после локальных проверок, без активации Telegram или запроса платного LLM.
