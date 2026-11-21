# Stage 2.2 — PostgreSQL runtime и агентная основа

**PostgreSQL и регрессия каналов — PASS.** Финальный изолированный прогон завершился с exit 0. Stage 2.2 закрыт частично, поскольку durable agent queue, persistent audit и Human Approval ещё требуют реализации.

## Окружение

Git-репозиторий ещё без первого commit. Исходное состояние индекса и рабочего дерева сохранено в локальных evidence. Сборки использовали кешированный Go 1.26.11 и существующий module cache. Результат сверки кешированного дистрибутива и команды проверок находятся в локальном индексе.

При первичной проверке оба Docker socket были недоступны. Локальные PostgreSQL client/server, слушающий PostgreSQL и выделенный удалённый runner не были обнаружены. В рабочей папке обнаружен OCI-архив PostgreSQL для Linux. Во время этапа Docker Desktop стал доступен, и тесты использовали уже загруженные PostgreSQL 18.3 и Mailpit. Linux harness собран Go 1.26.11 на хосте. Контейнер запускает готовый бинарник на внутренней сети без маршрута по умолчанию и опубликованных портов. Тестовый проект использует tmpfs, отдельную БД `aba_test`, роли `aba_app` и `aba_migrator`, safety marker и проверку чистой схемы. Каждый прогон создаёт чистую БД и удаляет ресурсы своего Compose-проекта.

Первая попытка создания сети завершилась FAIL из-за исчерпания стандартных пулов Docker. Для повторных прогонов использован свободный локальный subnet `10.253.242.0/28` в Compose override. Системная конфигурация Docker не менялась.

## PostgreSQL, очередь и каналы

Migration 00017 проверяется в существующем harness после применения, отката до 16 и повторного применения. В каждом checkpoint assertions проверяют ожидаемую версию схемы. Покрыты 11 индексов, четыре валидированных составных FK, ENABLE/FORCE RLS и возврат ограничения уникальности версии 16. Также выполняются повторный и конкурентный Up, откат пустой базы и отказ отката с защищаемыми данными. Новых migration файлов, изменения схемы рабочей БД и операций над историческими SQL migrations в этом этапе нет.

Три синтетических магазина проверяют SELECT/INSERT/UPDATE/DELETE на границе приложения и PostgreSQL, восстановление контекста при переиспользовании pooled connection, чужие credentials/состояния OAuth, диалоги, сообщения, вложения, очередь и DLQ. В тестах очереди проверяются claim leases, ограниченные повторы, backoff, DLQ, повторный claim после прерывания и дубликаты. Telegram сохраняет свой repository, WhatsApp и Instagram используют общую SQL очередь.

Telegram использует синтетический update, нормализацию, mock AI, каталог, подготовку ответа, согласие на заявку, обработку повторов и сценарии сбоя дочернего процесса. WhatsApp проверяется через подпись webhook, выбор аккаунта и tenant, общую очередь, каталог и согласие на заказ, outbox и ограничения доставки. Instagram проверяет зарегистрированные routes, OAuth/state, подпись, durable enqueue, смешанные аккаунты и tenants, точные цены/остатки из каталога и подготовку ответа. Вложения Instagram проверяются как метаданные и передача сотруднику. Реальные провайдеры и live delivery вне этих проверок.

На первом SQL прогоне обнаружен дефект assertion Instagram. Поле `attachments` в текстовом сообщении может содержать JSON `null`. PostgreSQL вернул SQLSTATE 22023 при вызове `jsonb_array_length`. Assertion теперь ищет конкретное ожидаемое image-вложение через JSONB containment и требует ровно одну строку. Сценарий сохранил проверки вложений и общей маршрутизации. Изменение относится к тесту. Следующий чистый прогон дошёл до общего RLS-аудита и обнаружил устаревший guard на 45 таблиц. Он учитывает обе новые Instagram-таблицы по именам и выполняет прежние отрицательные операции для всех 47 таблиц. Другие таблицы из аудита не исключены.

## Agent Registry, Orchestrator и permissions

Продолжены существующие `agentfoundation` и `specialists`. Registry использует tenant, версию, статус, capabilities и индивидуальные разрешения. Конфигурация tenant может сузить allowlist. Новый `RunCapability` выбирает агента по явно заданной backend capability и передаёт задачу в тот же `Run` с общими проверками разрешений, deadline, бюджета, дубликатов и повторных действий. Автоматическая классификация текста, многоагентное делегирование с общим бюджетом и агентные повторы на durable queue остаются открыты.

У Customer, Sales и Product & Inventory отдельный permission на каждое действие existing Runner. Проверки цены, запасов, поиска и карточек классифицируются как чтение. Просмотр статуса заказа доступен Customer и Sales. Передача сотруднику имеет собственный permission и эффект update. Product & Inventory использует исключительно чтение. Legacy широкий permission `existing_engine` больше не предоставляет полномочий.

## Контекст, инструменты, аудит и approval

Shared Context ограничен tenant/principal/conversation scope, разрешёнными ключами и размером. Existing Runner проверяет доступ к источнику каталога. Универсальные источники Business Profile, Industry, услуг, клиентов, правил и аналитики ещё требуют runtime adapters. Доступные мосты переиспользуют existing AI engine и проверяют параметры по типизированным схемам. Создание заказов и бронирований, получение клиентов и полноценные аналитические отчёты через новую агентную основу ещё не подключены.

Аудит фиксирует хеши инициатора, tenant, задачи, агента, операции и канонических параметров, состояние approval и tenant-scoped correlation hash. По вызову инструмента добавлены отдельные outcome-события и ограниченные коды ошибок. Тест проверяет, что произвольный backend error и сырые параметры корреляции не попадают в события. Persistent sink и привязка изменённых сущностей к доменной транзакции остаются открытыми.

Approval guard сверяет действие, хеш точных параметров, tenant, инициатора, задачу, агента, версии и expiry, затем вызывает атомарный `Consume` по контракту. Проверки подмены, повторного применения и срока выполнены через тестовый store. Durable issuance, решение уполномоченного владельца, отмена, ожидание/возобновление задачи и сохранение результата исполнения требуют PostgreSQL реализации. Опасные операции отключены.

## Семь специалистов

Customer, Sales и Product & Inventory используют соответствующие действия existing Runner. Marketing формирует черновик для проверки, Finance и Analytics читают только предоставленную tenant projection, Business Manager формирует план. Полномочий на платежи, отправку кампаний, изменение запаса или запуск других агентов у этих адаптеров нет. Тестовая матрица проверяет семь определений в восьми контекстах бизнеса и ожидаемые отказы для неподдерживаемых специализаций. В integration fixture Product & Inventory получает price через Orchestrator и existing Runner под проверенным Instagram lease. Сценарий проверяет точную RUB-цену своего варианта, ответ каталога 404 на чужой вариант, отказ для чтения остатков без отдельного grant и запись аудит-событий в тестовый sink. Resolver и агентные хранилища в этом сценарии тестовые. Новые специалисты не зарегистрированы в пользовательском API или рабочем worker.

## Изменённые файлы

- `apps/api/internal/agentfoundation/{contracts,orchestrator,foundation_test}.go` — выбор и исполнение по capability, correlation ID, tool outcomes и тесты.
- `apps/api/internal/specialists/{bridge,specialists,specialists_test}.go` — права для конкретных действий, явные эффекты, scope binding и тесты отказа.
- `apps/api/tests/integration/{audit_test,database_test,instagram_test,instagram_schema_stage22_test,specialist_catalog_stage22_test}.go` — schema checkpoints, полный охват RLS, regression вложений и оркестрация под PostgreSQL lease.
- Этот отчёт и evidence в `.local/stage-2-2/`.

## Приёмка и следующие приоритеты

Финальный PostgreSQL harness прошёл за 72,18 с. В журнале подтверждены 47 таблиц с ENABLE/FORCE RLS и владелец aba_migrator, отрицательные операции под aba_app и три отдельных проверки Product & Inventory. Go checks и Linux integration compile выполнены на одном digest исходников. [Команды, журналы, исходные FAIL и инвентарь RLS](../.local/stage-2-2/INDEX.md).

| Критерий | Статус |
| --- | --- |
| Migration 00017 up/down/reapply | PASS |
| FORCE RLS audit of 47 tables | PASS |
| Three-tenant isolation and pooled context | PASS |
| Durable channel queue and worker recovery | PASS |
| Telegram PostgreSQL regression | PASS |
| WhatsApp PostgreSQL regression | PASS |
| Instagram PostgreSQL regression and catalog routing | PASS |
| Go format, tests, race, vet, API and worker builds | PASS |
| Compiled registry and tenant configuration guard | PASS |
| Orchestrator capability routing and execution limits | PASS |
| Per-action permissions and typed registered tools | PASS |
| Product & Inventory catalog integration under Instagram lease | PASS |
| Scoped ContextProvider guard and correlated audit outcomes | PASS |
| Full shared business context sources | BLOCKED |
| Agent config/idempotency storage and durable queue wiring | BLOCKED |
| Persistent agent Audit Log | BLOCKED |
| Approval parameter binding and replay guard with test store | PASS |
| Durable Human Approval lifecycle | BLOCKED |
| Runtime registration of seven specialists and manager delegation | BLOCKED |
| Frontend, production and live provider tests | NOT RUN |

Следующий инженерный приоритет — привязать typed task/configuration, audit и approval records к tenant через FORCE RLS и составные FK, реализовать атомарное consume и тесты конкуренции на этом же disposable runner, затем расширить существующую durable queue с сохранением lease/idempotency channel jobs. Production readiness остаётся открытой. Push, deployment и live отправки не выполнялись.
