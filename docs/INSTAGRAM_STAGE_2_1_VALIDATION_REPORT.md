# Instagram Stage 2.1 — runtime validation and remediation

**STAGE 2 PARTIALLY COMPLETE**

Go compilation/tests/race/vet и API/worker builds подтверждены текущими запусками. Исправлены два воспроизведённых дефекта. Полностью закрыть Stage 2 нельзя: PostgreSQL migrations, RLS и межtenantные queue/order regressions не выполнялись из-за отсутствия disposable PostgreSQL.

## Environment preflight

- Host: macOS/Darwin amd64. Обычный `go` в PATH — **1.26.2**; default `GOTOOLCHAIN=auto`, GOPATH `/Users/ilezkaradjev/go`, GOMODCACHE `/Users/ilezkaradjev/go/pkg/mod`.
- Найден **Go1.26.11 darwin/amd64** в существующем cache `~/Library/Caches/AI-Business-Assistant/readiness-module-cache-residual-20261117/golang.org/toolchain@v0.0.1-go1.26.11.darwin-amd64`.
- Содержимое установленного toolchain сверено с существующим zip: **11 524 файла, 0 расхождений**; вычисленный module h1 совпал с ziphash и кешированной записью официальной Go checksum database. Это локальная проверка сохранённой provenance, не новая online-аттестация.
- Проверки использовали этот toolchain явно через argv/GOROOT и `GOTOOLCHAIN=local`, `GOPROXY=off`, `GOSUMDB=off`, `GOVCS=*:off`, `GOFLAGS=-mod=readonly`. Глобальные PATH/settings не менялись; ничего не скачивалось.
- Первый выбор residual module cache не содержал текущий x/crypto: `modules-1` и `unit-1` завершились FAIL/setup error. Выбран уже существующий полный `/Users/ilezkaradjev/go/pkg/mod`; `modules-2` подтвердил `all modules verified`. Версии зависимостей и `go.mod`/`go.sum` не менялись.
- Docker CLI доступен, daemon недоступен на **обоих** настроенных endpoints: `/var/run/docker.sock` и `~/.docker/run/docker.sock`.
- Native `psql/postgres/pg_ctl/initdb` не найдены в PATH, проверенных Homebrew/Postgres.app/Library/MacPorts installations и project/Go caches. Common embedded-PostgreSQL caches отсутствуют; loopback 5432/5433 не слушают. Рабочие БД не подключались.
- Clang и macOS SDK доступны; race выполнялся с `CGO_ENABLED=1`. Обычные builds/tests — pure Go с `CGO_ENABLED=0`.
- Полный начальный Git status сохранён. Чужие staged/unstaged/untracked изменения не сбрасывались. Исторические evidence, migrations 00001–00017 и предыдущие Instagram reports сохранены.

## Фактически выполненные Go gates

| Проверка | Результат | Финальная receipt / exit |
| --- | --- | --- |
| Go1.26.11 executable + cache integrity | PASS | `cached-go-version.log`, `toolchain-verification.json` |
| Module integrity | PASS | `modules-2.log`, exit 0 |
| gofmt verification | PASS | `format.command.json`, exit 0, пустой список |
| `go test -count=1 ./...` | PASS | `unit-4.log`, exit 0 |
| `go test -race -count=1 ./...` | PASS | `race-2.log`, exit 0 |
| `go vet ./...` | PASS | `vet-3.log`, exit 0 |
| Full API build, darwin/amd64 | PASS | `api-build-3.log`, exit 0 |
| Full worker build, darwin/amd64 | PASS | `worker-build-3.log`, exit 0 |
| API cross-build, linux/amd64 | PASS | `linux-api-build-1.log`, exit 0 |
| Worker cross-build, linux/amd64 | PASS | `linux-worker-build-1.log`, exit 0 |
| Integration harness compilation (`-tags=integration -c`) | PASS | `integration-compile-3.log`, exit 0 |
| Targeted Instagram/HTTP/config/Telegram/WhatsApp unit suites | PASS | `targeted-2.log`, exit 0 |

Все test runs использовали `-count=1`; результаты не взяты из старого test-result cache. Полные argv, cwd, toolchain/cache env, время и exit codes сохранены в `*.command.json`. Linux binaries собраны, но Linux runtime/race execution без Docker не проверялись. Компиляция integration harness **не означает** выполнение PostgreSQL tests.

## Исправленные дефекты

### R1 — несовпадение normalizer identity limit и общей SQL-модели

Общая SQL-модель ограничивает numeric external user ID двадцатью цифрами. Instagram normalizer принимал до 32. Подписанный batch с 21-значным sender мог пройти нормализацию и затем сорвать atomic enqueue корректных соседей на SQL constraint.

До исправления regression test дал **FAIL**: `accepted=2 rejected=0`. Лог: `identity-boundary-1.log`.

В `internal/instagram/webhook.go` добавлена граница 20 цифр для account binding/sender, соответствующая текущему repository contract. Несовместимое событие отклоняется индивидуально до enqueue; корректный sibling сохраняется в нормализованном batch. Boundary 20 остаётся разрешённой.

После исправления: `identity-boundary-2.log` PASS, далее общий unit и race PASS. Это проверка Go/SQL contract boundary; само enqueue в PostgreSQL пока BLOCKED. Ограничение 20 — текущий storage contract проекта, не заявление об универсальном лимите Meta.

### R2 — DB failure ошибочно выдавался за rate limit

Нативная проверка запущенного API с недоступным **собственным синтетическим DB endpoint** выявила: GET verification и POST events возвращали **429 RATE_LIMITED**, хотя `identity.Limit` возвращал ошибку недоступности БД.

Исходный FAIL: `runtime-1.json` — оба ответа actual 429 при expected 503. Оригинальные API/worker logs сохранены.

В `internal/http/instagram.go` добавлено корректное различение:

- подтверждённый rate limit → 429 + `Retry-After: 60`;
- недоступность storage/неожиданная ошибка → generic 503 без раскрытия DB details.

Добавлен regression test для typed throttling, database unavailability и ошибки с synthetic sensitive URI. После исправления: unit/race/vet/build PASS; повторный запуск реальных процессов — **PASS** в `runtime-2.json`, оба endpoint вернули 503.

Telegram/WhatsApp бизнес-логика и frontend-дизайн не изменялись.

## Реальные процессы: startup, HTTP failure paths, shutdown

`runtime_probe.py` запускает **собранные API и worker binaries** с минимальным synthetic environment. DB URL указывает на loopback port, эксклюзивно занятый неслушающим socket этого probe; он не может быть рабочей БД. Все внешние providers/live flags выключены, Instagram — только mock.

В финальном `runtime-2.json` подтверждены:

- API startup; liveness 200; readiness 503 при отсутствии БД;
- connection status и OAuth start без сессии → 401;
- unsigned webhook → 403 до DB rate limiter/account lookup;
- DB failure на verification/events → 503;
- worker startup и вызов зарегистрированного Instagram handler: структурированный error-path log с `channel=instagram`;
- API и worker завершаются по SIGTERM с exit 0;
- synthetic key/token/DB URL values не попадают в process logs.

Это **не** проверка успешного OAuth callback, получения challenge после успешного SQL limiter, enqueue/claim/AI processing или durable worker recovery. Соответствующие DB gates остаются BLOCKED.

## PostgreSQL, migration и tenant isolation

| Gate | Результат |
| --- | --- |
| 00017 Up, tables/constraints/FK/indexes | BLOCKED |
| FORCE RLS и tenant context в реальных connections | BLOCKED |
| Down, повторный Up и состояние после reapply | BLOCKED |
| Three-tenant SELECT/INSERT/UPDATE/DELETE isolation | BLOCKED |
| OAuth state/credentials/conversation/message/attachments isolation | BLOCKED |
| Queue claim/process/ack/idempotency/backoff/DLQ | BLOCKED |
| Pooled connection tenant/capability isolation | BLOCKED |
| Worker interruption/recovery и отсутствие повторных заказов | BLOCKED |

Причина одинакова: disposable PostgreSQL недоступен; ни Docker, ни установленного native server нет. Новый PostgreSQL не скачивался и не устанавливался.

Дополнительно запущен скомпилированный `TestDatabaseAndMigrations` **без ambient DSNs**: safety gate отказал до подключения (`ABA_TEST_DATABASE_URL must point to isolated aba_test with role aba_app`, exit 1). Receipt: `postgres-safety-guard.*`. Это инфраструктурный **BLOCKED**, не PostgreSQL test PASS и не доказательство исправности migration.

`postgres-results.json` явно фиксирует `tenants_created=false` и `sql_connections_made=false`. PostgreSQL fingerprints/row snapshots не выдаются за полученные: их runtime evidence пока нет. Cross-tenant leakage не оценена на PostgreSQL; критический isolation gate остаётся открытым.

### Дополненные интеграционные проверки — скомпилированы, не выполнены

В existing safety-checked harness добавлены:

- foreign INSERT/UPDATE/DELETE для messages/connection/queue с проверкой 42501 либо zero affected rows;
- migrator-side fingerprints до/после отрицательных операций: тип ошибки сам по себе не считается доказательством неизменности данных;
- DLQ positive control владельца и отрицательные проверки чужих tenants;
- reuse одного физического pooled connection для A/B/C/A, проверка pid, сброса transaction-local session/tenant/capability и zero visibility вне контекста;
- worker interruption после commit customer message, но до inbox ack; reclaim после истечения lease, неизменное количество сообщений/заказов и отказ late finalizer регрессировать результат.

Новые проверки используют реальные pgx repositories/connections при доступной disposable `aba_test`; SQL не заменён in-memory/mock database. Полнота/корректность этих сценариев должна быть подтверждена запуском, а не только compilation.

## Security review

**PASS в выполненном scope:** HMAC/signature/payload/duplicate-header/size validation; sender/account boundary; OAuth state CSRF/expiry/replay/session/tenant unit tests; encrypted credential AAD/tamper/wrong-key checks; JSON secret suppression; unknown delivery/no-send guards; authentic HTTP auth rejection и log redaction; race detector.

Новые HTTP tests используют nil DB dependency как sentinel: missing/forged/duplicate signature и oversized payload должны быть отклонены **до** обращения к DB limiter/mapping/queue.

**Key rotation:** другой encryption key не открывает старый ciphertext — fail closed, подтверждено новым unit test. Runtime worker proof также выводится из key; прозрачной rotation/rewrap модели и key-version metadata нет. Для эксплуатации потребуется контролируемый rewrap/reconnect plan; этот результат не подтверждает hot rotation.

**BLOCKED:** PostgreSQL IDOR/BOLA/write isolation, lease/concurrent job fencing, durable state replay, DB-backed duplicate/order recovery и actual tenant data leakage checks. Unit binding checks не заменяют эти проверки.

## Telegram и WhatsApp regression

Existing backend mock suites обоих каналов выполнены в общем `go test ./...`, targeted verbose run и общем race run — **PASS**. Включены existing transport/signature/serialization/policy/provider mock tests.

Existing PostgreSQL regressions (conversation mapping, shared catalog/AI/order flow, queue/retries/delivery/financial semantics) **BLOCKED** вместе с integration harness. Поэтому полные Telegram/WhatsApp Stage 2 acceptance gates не помечены PASS. Telegram остаётся на прежнем repository в общем worker lifecycle; его архитектура не переписывалась.

## Acceptance summary

| Stage 2 criterion | Итог |
| --- | --- |
| Go compilation/tests/race/vet | PASS |
| API build / worker build | PASS — native и Linux cross-build |
| Migration Up/Down | BLOCKED |
| PostgreSQL RLS / three-tenant isolation | BLOCKED |
| Queue reliability | BLOCKED |
| Worker execution | BLOCKED — startup/error-routing/SIGTERM отдельно PASS |
| OAuth tests | BLOCKED для durable path; unit tests PASS |
| Webhook tests | BLOCKED для durable path; crypto/HTTP rejection tests PASS |
| Telegram regression | BLOCKED для DB flows; mock unit/race PASS |
| WhatsApp regression | BLOCKED для DB flows; mock unit/race PASS |

Машиночитаемый итог — `acceptance-gates.json`. Ни один BLOCKED не преобразован в PASS.

## Файлы и сохранность

Изменены:

- `apps/api/internal/instagram/webhook.go` — storage-compatible identity boundary;
- `apps/api/internal/http/instagram.go` — правильная классификация limiter failures;
- `apps/api/internal/http/instagram_test.go` — pre-DB rejection и limiter regression tests;
- `apps/api/tests/integration/instagram_test.go` — подключение новых DB/recovery сценариев.

Добавлены:

- `apps/api/internal/instagram/boundary_test.go`;
- `apps/api/internal/instagram/credential_security_test.go`;
- `apps/api/tests/integration/instagram_validation_test.go`;
- этот report и `.local/instagram-stage2-1/INDEX.md` с command/log/hash receipts.

Migration 00017 и все predecessor migrations, `go.mod`/`go.sum`, предыдущие Instagram reports не изменены. Native/Linux artifacts находятся только в task-local evidence directory. Global settings, рабочая БД, frontend-дизайн и live providers не менялись. Deployment/push/release tag не выполнялись.

## Что осталось

Нужен уже подготовленный Docker runner либо безопасно предоставленный native disposable PostgreSQL с test roles/safety marker/isolated `aba_test`. После восстановления выполнить существующий PostgreSQL harness, сохранить исходные SQL FAIL при их наличии, устранить их без ослабления RLS/guards и подтвердить весь acceptance matrix. Текущий отчёт **не** является разрешением на загрузки, изменение рабочей БД или production rollout.

Stage 3 не начат. E01 остаётся OPEN; artifact guards и историческая evidence не изменялись.

**STAGE 2 PARTIALLY COMPLETE**
