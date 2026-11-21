# Технические решения и вопросы согласования

Решения ADR-01, ADR-07 (принцип ручного подтверждения), ADR-08 и ADR-10 утверждены пользователем в ТЗ № 2. Остальные решения — проектные, если не указано иначе. Требование пользователя: backend преимущественно Golang; в этом проекте весь прикладной backend, AI orchestration и adapters предлагаются на Go. Frontend — Next.js/TypeScript.

| ID | Решение | Обоснование и последствия | Когда пересмотреть |
|---|---|---|---|
| ADR-01 | Модульный монолит, API+worker из одной Go codebase | Простые транзакции/эксплуатация для одного разработчика; требуется дисциплина модульных границ | Измеренный независимый bottleneck или разные команды |
| ADR-02 | Shared PostgreSQL schema + organization_id, RLS и составные FK | Низкая стоимость tenant, двойной контроль утечек; bootstrap/worker roles требуют тестов | Изолированный enterprise tenant, регуляторные/нагрузочные требования |
| ADR-03 | PostgreSQL inbox/jobs/outbox | Атомарность событий с доменными изменениями; нет отдельного брокера | Queue latency/DB contention доказаны нагрузкой |
| ADR-04 | Redis только вспомогательный | Не усложняет durable delivery и stock correctness; optional для пилота | Несколько API instances и нагрузка limiter/cache |
| ADR-05 | REST/OpenAPI, единый origin, opaque session cookie | Простой кабинет и контроль отзывов; нужен CSRF | Мобильные приложения/service-to-service auth |
| ADR-06 | Один склад, целые единицы, деньги bigint RUB | Подходит электронике; не подходит весовым товарам/нескольким валютам | Новые отрасли/склады |
| ADR-07 | requested не резервирует; confirmed резервирует | Не блокирует товар ложными намерениями; сотрудник обеспечивает факт наличия | Надёжная ERP и согласованная автономная политика |
| ADR-08 | Telegram MVP, WhatsApp/webchat позже | Доступный официальный bot API и простой пилот; покупатель должен начать чат | Канал не востребован магазинами/покупателями |
| ADR-09 | Tool calling через Go, факты рендерит backend | Prompt сам не гарантирует точности; шаблоны ограничивают гибкость, повышают проверяемость | Более сложные вопросы и доказанная валидируемая генерация |
| ADR-10 | Provider abstraction, один адаптер сначала | Смена модели без переделки домена; все capability/privacy различия явны | Реальный failover/ценовое сравнение |
| ADR-11 | FTS/SKU/model search без RAG/vector | Проверяемая актуальная цена и короткий каталог; семантика limited | Низкий recall на реальном каталоге после нормализации |
| ADR-12 | Версии инструкций, quote и price, budget reserve | Воспроизводимость/подтверждения/ограничение параллельных расходов | Новые требования и provider billing |
| ADR-13 | Next.js, TypeScript, Tailwind, polling MVP | Удобный responsive кабинет, стандартный стек; нет нужды в realtime broker | Inbox latency потребует SSE |
| ADR-14 | Primary/backup ПД в РФ; внешний LLM только после оценки | Предварительный путь к соблюдению требований; договор/согласие не заменяют правовую проверку | Выбранные провайдеры и актуальное регулирование |
| ADR-15 | Ручная подписка пилота, hard AI limits | Контролирует расходы без эквайринга; тарифная экономика ещё не проверена | Платный коммерческий запуск |

## Требуют явного согласования перед реализацией зависимой части

1. Telegram утверждён как первый канал; спрос и удобство проверить в пилоте. Веб-чат отложен.
2. Ручное подтверждение и резерв только после него утверждены. Параметры свежести (24 ч) и quote TTL (10 мин) ещё предложены.
3. LLM-провайдер/модель, бюджет пилота, максимально допустимая стоимость диалога, правила передачи переписок; сейчас конкретный провайдер не выбран.
4. Юридическое лицо/договоры, роли обработки ПД, хостинг/backup в РФ, окончательные retention и уведомления; до real traffic.
5. Первый пилот — один магазин электроники, утверждено. Owner-only UI, без оплат и ERP; продукт — ИИ-продавец, не замена 1С.
6. Тарифы, бесплатный trial и контроль превышения; пока ручной план + hard cap.
7. Ресурс одного разработчика и реалистичный диапазон 10–19 полных рабочих недель с резервом; approvals и part-time отдельно.
8. Выбор email delivery, error monitoring и test/prod hosting на следующих заданиях, без создания инфраструктуры сейчас.

Принятие документа не разрешает автоматически подключать платные API, deploy или commit/push. ТЗ № 1 завершено проектированием; ТЗ № 2 разрешает реализацию каркаса, сохраняя отдельное разрешение на commit/push.

## Открытые риски

Качество каталога/сверок владельца; физические продажи мимо платформы; privacy/availability LLM; Telegram adoption; WhatsApp approvals и доступность; uncertain outbound delivery; неверное толкование «да»; утечки через RLS bootstrap/cache/history; race handoff; перерасход при unknown usage; сроки восстановления; ограниченное время единственного разработчика. Mitigations распределены в DATABASE_DESIGN, AI_AGENT_ARCHITECTURE, SECURITY, INTEGRATIONS и DEVELOPMENT_ROADMAP.

## Источники и пределы проверки

Проверены официальные Telegram Bot API/FAQ, PostgreSQL RLS/locking, политика WhatsApp Business и официальный код Meta signature validation. Meta overview/webhooks pages не удалось прочитать; onboarding/permissions/региональная доступность не подтверждены. Правовые источники и область будущей юридической проверки указаны в SECURITY. Версии библиотек/прайсы/production SLA не зафиксированы, результаты нагрузочных и программных тестов не заявлены.

## Реализация каркаса — ТЗ № 2

- Go API и idle worker, Next.js App Router, PostgreSQL. Бизнес-модули/tenant tables/RLS пока не реализованы: для них сначала аутентификация и отдельные тесты.
- Конфигурация размещена в internal/config, HTTP в internal/http; platform содержит только применяемые DB/logging/run/migrate. sqlc отложен до доменных запросов.
- PostgreSQL 18.3; goose 3.27.1 с session advisory lock и отдельным cmd/migrate. Первая миграция — platform.service_metadata, чтобы readiness проверял не только соединение, но и установленную схему. Runtime aba_app имеет только CONNECT, USAGE и SELECT metadata; aba_migrator владеет platform, но не superuser/CREATEDB/CREATEROLE/BYPASSRLS.
- Worker пока не открывает пул: очереди и выполняемой работы нет. Healthcheck worker проверяет только процесс, не обработку очереди.
- API может стартовать без доступной БД; live/status отвечают, ready возвращает 503. Status `running` означает только процесс API, не весь продукт.
- Frontend обращается к Go через ограниченный server-side endpoint /api/backend-status. Внутренний API_BASE_URL не NEXT_PUBLIC, URL фиксируется конфигурацией, не данными запроса. CORS не включён. Перед auth потребуется отдельная согласованная стратегия общего origin для бизнес-API.
- Next.js production build использует поддерживаемый Webpack (--webpack): Turbopack в ограниченном окружении требовал запрещённый служебный порт. Это полноценная сборка, не заглушка. ESLint 10 с совместимыми TypeScript/Next/React Hooks plugins вместо устаревших peer-зависимостей eslint-config-next.
- Светлая/тёмная тема следует настройке ОС; нет отдельного state/theme-пакета. Dashboard публичный макет без данных и авторизации.
- Единственный первичный пилот — один магазин. Две синтетические организации остаются обязательными будущими тестами изоляции, а не планом двух реальных магазинов. Реальные клиентские данные в тестах не используются.
- Бюджет LLM, retention, правовой контур и подключение реальных каналов не утверждены и на этом этапе не нужны. Нет платных API, deployment, commit/push.

Локальный Compose располагается в корневом docker-compose.yml (вместо первоначально предложенного infrastructure/compose.yaml), чтобы команда `docker compose up --build` работала из корня без дополнительных флагов. PostgreSQL init и тестовые overrides остаются в infrastructure.

## ТЗ № 3 — фактические решения

Реализованы auth/email verification/server sessions, организации и roles owner/member. Вместо архитектурного `/me` — `/api/v1/auth/me` согласно ТЗ № 3. Cookies/BFF common origin; безопасная server-side проверка dashboard. Users.email нормализован, UNIQUE; membership composite PK, без публичного добавления участников. Org root id —tenant, две таблицы FORCE RLS. Context: app.session_hash + app.organization_id; identity определяется проверенной session, не произвольным app.user_id. Bootstrap/list/membership —узкие definer functions с owner-specific policy и PUBLIC EXECUTE revoked, без BYPASSRLS. Подробности AUTHENTICATION/TENANT_ISOLATION.

Worker перестал быть idle: обрабатывает только зашифрованный dev-email outbox через loopback Mailpit, без реальной рассылки/бизнес-очереди. Регистрация и token/outbox атомарны. Argon2id 64 MiB/2/1, максимум два password calculations одновременно на API; SHA-256 только для случайных session/verification/CSRF tokens. Metadata schema version=2; nonempty identity rollback запрещён. Password reset/MFA/auth retention cleanup/production SMTP остаются вне этапа. Existing .env дополнен отдельным локальным email encryption key без вывода значения.

Для реализованного PATCH organizations optimistic concurrency передаётся JSON expected_version (min 1), а не первоначально предложенным If-Match/ETag. На этапе 2 изменяющих endpoints не было, поэтому существующая API совместимость сохранена. Окончательные request/response schemas —OpenAPI; добавление ETag к следующим ресурсам отдельным решением.

## ТЗ № 4 — каталог и склад

- Products/variants/categories/inventory/movements реализованы одним компактным domain module internal/catalog: одна transactional boundary для publish/archive/price/stock, без новых внешних технологий. CustomerCatalog — отдельный read-only facade с четырьмя методами, не публичный endpoint. sqlc пока не добавлен; SQL параметризован, сорта allowlist.
- Categories tenant-owned flat; default variant + нулевой stock атомарны. UUID route вместо slug; SKU uppercase уникален внутри организации, включая архив. Архив терминальный и не изменяется. product_images/загрузка отложены, чтобы не добавлять неиспользуемую таблицу/небезопасные remote URL.
- ТЗ № 4 field names price_amount/currency_code/quantity_on_hand/quantity_reserved вместо предварительных price_minor/currency/on_hand/reserved. Это новый бизнес-API, прежние auth/org contracts не меняются. variant version общая для цены и характеристик; updated_at вместо отдельного price_updated_at.
- PATCH передаёт редактируемый набор полей и expected_version, как уже реализованные org settings; не JSON Merge Patch. Optional отсутствующие значения сбрасываются к default. Нет второго price endpoint; единый variant PATCH. Draft через product PATCH, publish/archive команды. Inventory adjustments collection с variant_id body, по ТЗ № 4.
- Pagination page/page_size+total для products вместо предложенного cursor; bounded offset для малого пилота, стабильный id tie-breaker. Literal case-insensitive substring search, без fuzziness/RAG; FTS GIN подготовлен, массовые нагрузки ещё не измерены.
- FOR UPDATE product → inventory + expected_version + conditional UPDATE + движение в той же transaction. coarse product lock упрощает гонки archive/publish/price, допустим для пилота. Резерв public API отсутствует. updated_at не означает физическую сверку; freshness/quote TTL перед заказами требуют решения.
- Пять новых FORCE RLS tables, compound FK, owner writes/member reads; movement INSERT actor=current session user; runtime без DELETE, audit UPDATE. Metadata3 readiness. Up additive, down refuses nonempty catalog.
- Existing BFF allowlist расширен точными catalog routes, URL queries передаются только к фиксированному backend; JSON body limit 64 KiB для описания и attrs. Auth cookies/CSRF/SSR сохраняются.
- Integration container ставит gcc/musl-dev только в ephemeral test environment для реального Go race detector; application images остаются прежними. CI уже запускает все tests/integration, новые тесты входят автоматически.

Согласование зависимой части: политика свежести физического остатка, quote TTL и будущий worker system principal до заказов/AI. Текущее ТЗ № 4 не требует внешних подключений или нового разрешения.

## ТЗ № 5 — фактические решения

- Предложение5min и stock freshness24h по новому утверждённому ТЗ вместо предварительного quote10min; INVENTORY_FRESHNESS configurable. Separate manual_updated_at не освежается reserve/release/consume, updated_at остаётся общим временем записи.
- pending_confirmation/confirmed/rejected/cancelled/completed по ТЗ вместо проектного requested. Snapshot immutable, price changed →new quote/consent/request, no staff price override. Order command keys UUID в JSON + expected_version сохраняют стиль этапа4.
- Заказы/chat этапа5 только в отдельном test org; is_test defaultfalse для прежних организаций. Create test org atomic narrow definer, no auto-copy каталога/склада. Direct runtime UPDATE is_test запрещён. Все orders source internal_chat/is_testtrue; Telegram/real order routing ещё не реализованы.
- Member opt-in can_manage_orders (owner existing-member grant API), не новая hierarchy/admin роль и не доступ к catalog/settings. Узкая lock_order_products функция разрешает staff брать нужные row locks, не расширяя UPDATE policy каталога.
- Синхронный bounded agent cycle вне DB transaction с conversation revision/run lease. Worker email не получает user sessions и не становится AI queue. Для Telegram потребуется отдельный system principal/durable queue, не reuse cookie.
- Provider interface + deterministic disclosed Mock + fixed-URL Chat Completions adapter, live disabled. Free model content ignored; structured facts render backend. Ключ/модель не обязателен для mock. Token/request quota вместо неподтверждённого monetary pricing; валютный лимит/terms до paid live.
- Temporary synthetic retention7d; explicit erase и lazy purge payload/hash, audit/order metadata не удаляются. Это не production/legal retention и не background гарантированный cleanup.

Новые схемы и API —OpenAPI0.5.0; документы ORDER_MANAGEMENT/AI_AGENT_IMPLEMENTATION/CONVERSATION_LIFECYCLE. Нет новых внешних библиотек/сервисов, Redis/LLM SDK/микросервисов; existing stack preserved.

## Этап6: Telegram customer principal и безопасный mock контур

- Прежний RLS использовал только owner/staff session. Для worker добавлен random leased inbox capability, server-created SET LOCAL scope с точным tenant/conversation. Owner sessions не сериализуются. Narrow definer claim/finish/ingress/outbox/lock helpers с fixed search_path, PUBLIC revoked; runtime без bypass/ownership. Active catalogue read-only; channel inventory/catalog/order transition UPDATE запрещён.
- Telegram customer audit не получает user staff ID: nullable actor для channel/system и customer UUID через conversation/principal; human/order decisions сохраняют employee ID. Старые записи не переписаны. Источник заявки определяется DB trigger по capability, не входом LLM.
- Существующий Agent/Conversations/Orders reuse вместо второго engine. Telegram staff не может имитировать customer или подтверждать quote через playground endpoint. Consent через bound quote callback; reserve only employee.
- Специализированные telegram_inbox/outbox служат jobs, PostgreSQL SKIP LOCKED/leases/order + существующая message/order idempotency. Новые инфраструктурные зависимости не нужны. Message/outbox atomic trigger, outbound вне DB txn. Uncertain delivery fail-closed/manual reconciliation вместо обещания exactly-once.
- Этап6 runtime намеренно mock/test-only; bot getMe/send protocol adapter проверяется локальным HTTP server, но application config не выбирает live. Real HTTPS registration/getWebhookInfo/answerCallbackQuery/provider send и privacy approval отложены, status synthetic_only. Это отличие от целевой архитектуры, необходимое для запрета реальных запросов/ПДн текущего задания; не выдаётся за установленный webhook.
- Temporary synthetic retention1–30days, default7, background worker purge payload + existing explicit erase. Principal/order/audit metadata сохраняются; production anonymization/backups/legal policy ещё требуют согласования. Raw metadata Telegram нормализуются, payload тестовых сообщений plaintext в local PG; production encrypted storage/retention не заявлены.
- Config AES key отдельный серверный, optional для совместимости старого .env: без него Telegram disabled, прежние модули доступны. 00005 additive, FORCE RLS/compoundFK; controlled migration/readiness6. Rollback nonempty integration отказ, только isolated clean roundtrip.

### Additive migration00006: worker proof

00005 уже была применена к локальному dev volume, поэтому hardening не меняет применённую миграцию. 00006 добавляет hash-only worker gate и закрывает ungated claim/send/purge function signatures. HMAC purpose derives отдельный queue proof из server key, DB хранит hash и проверяет предъявленный proof; один raw runtime SQL access без server secret не становится customer principal. Legacy integrations worker_hashNULL безопасно не обрабатываются до owner reconnect того же bot, данные сохранены. Новый readiness metadata6, clean6→5→4→3→2→1 tests; nonempty down refused.

## Этап 7: закрытая live-подготовка (2026-10-14)

Provider NOT SELECTED, budget 0 RUB, mock активен. Сохраняем интерфейс, добавляем совместимый protocol adapter без выбора поставщика. Денежный ledger использует целые microRUB, общий platform row lock и worst-case reservations; scale tradeoff приемлем для MVP. Неизвестный usage не возвращает бюджет. Global approval отдельно от owner org limits. Defaults закрывают external LLM/Telegram, sending kill switch true, live allowlist обязателен. Manual getMe/webhook отдельно подтверждаются, dashboard читает только DB. FORCE RLS защищает новые tenant таблицы; глобальный budget не имеет runtime grants. Миграция 7 сохраняет 1–6; состояние ledger/метрик делает down небезопасным и он отклоняется. Нативные tests passed; PostgreSQL миграции и регрессия 3–7 с race detector PASS. Контейнерные результаты — в отчёте. Подробности STAGE_7_REPORT.

## ADR-8A — минимальное усиление без смены архитектуры

16.10.2026: сохранены модульный монолит, PostgreSQL inbox/outbox и один Agent. Для send/disconnect race выбран pinned connection + session advisory lock, потому что нельзя держать SQL transaction на внешнем HTTP, а claim сам по себе устаревает. Hand-off/disconnect и enqueue используют общий integration fence. Неопределённый sendMessage5xx не повторяется автоматически (приоритет исключению дубля); operator recovery A09 остаётся обязательным. Недостоверный LLM usage учитывается консервативно. Message actor immutable, channel update только erasure. Новая миграция8, прежние1–7 неизменны; down с организациями запрещён как небезопасное security downgrade. Offline audit harness использует только cached images/internal network; fresh web build/race остаются NOT RUN до наличия offline native tools.

## ADR-8B1 — безопасное разрешение UNKNOWN

18.10.2026. Сохранены статусы uncertain для совместимости API и явная UI-метка DELIVERY_UNKNOWN/UNKNOWN. Telegram operator resolution не делает resend: выбрать delivered/abandon безопаснее, чем разрешать повтор без provider idempotency. Решение сериализуется с send fence и имеет actor/version/request key. Попытки и outcomes фиксируют DB triggers; grant на прямую запись audit не выдаётся.

Для зависшего LLM reservation выбран полный conservative charge через5min, без refund и без provider query. Это устраняет зависшие active slots и сохраняет максимум расходов при неизвестном исходе. Поздний usage не снижает уже учтённый UNKNOWN автоматически. Три финансовых операции используют общий lock order; core private, worker maintenance bounded100 и owner manual recovery проверяет возраст/роль. Точная последующая корректировка счёта — отдельная будущая операция, не текущий финансовый бюджет.

Новая миграция9, предыдущие1–8 не изменены; down с организациями запрещён. Текущие TSX recovery components проверены отдельно через установленный TypeScript/webpack/Chromium; это сознательно не заменяет отсутствующую Linux native Next production build. Live не разрешён.

## ADR — 8Б.2, общий Channel Layer без переписывания Telegram

Выбор: общие типы/Engine/Queue/Adapter, отдельный WhatsApp repository channel_*; сохранён Telegram repository telegram_*. Причина: не переносить активные rows/capabilities и не терять проверенные fencing/recovery8Б.1. Shared AI/catalog/orders/reserve не дублируются. Общий интерфейс не отменяет provider-specific auth и send admission.

WhatsApp работает только с synthetic credentials, без production transport. Отдельный key/budget0, default sending disabled. Window/retention ограничены локальной policy, Graph versionv0.0 в UI — fixture; официальные условия перед live нужно заново проверить с отдельным разрешением. Неизвестный исход HTTP5xx/timeout →UNKNOWN, без auto retry. Реальные templates/биллинг/многономерность/ротация live credentials не включены.

00010 хранит оригинальные определения трёх shared SQL helpers в закрытой глобальной migration-only таблице для точного rollback. Down отказывается удалять WA data; исходные migrations1–9 неизменны. Обновление schema/API/worker контролируемое, бесшовный rolling downgrade не обещается.
