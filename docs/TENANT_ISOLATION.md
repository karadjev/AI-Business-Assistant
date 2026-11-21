# Изоляция организаций — ТЗ № 3

## Границы данных

Глобальные таблицы: users, sessions, email_verification_tokens, auth_rate_limits, email_outbox, service_metadata, goose_db_version. Это account/auth/infrastructure state, не каталог магазина. Runtime имеет auth repository privileges на users/tokens/rate/outbox; password hash доступен только backend login, никогда API/frontend. Sessions нельзя перечислять напрямую SELECT. Полная компрометация runtime DB credentials/Go process выходит за гарантию tenant context: backend —доверенная граница, не публичный SQL gateway.

Tenant-owned: organizations (tenant root: id) и organization_members (organization_id,user_id composite PK, role owner/member). Конфигурация DATABASE_URL разрешает только aba_app. Каждый новый pool connection проверяет роль/привилегии/ownership в AfterConnect до любой auth/org операции; одной readiness недостаточно для защиты от ошибочной привилегированной конфигурации. Runtime aba_app —не table owner, не superuser, без BYPASSRLS/CREATEDB/CREATEROLE; schema CREATE также отсутствует. Table owner aba_migrator, отдельная migration connection. На обеих таблицах ENABLE + FORCE RLS. Прямого app INSERT организаций/memberships нет; нет API добавления member, передачи владения или удаления аккаунта/организации. Member имеет только read своей организации, owner —read/update name/timezone с optimistic version.

## Контекст и RLS

В каждой org transaction backend выполняет параметризованный `set_config('app.session_hash', hash, true)` и `set_config('app.organization_id', id, true)`. `true` соответствует SET LOCAL. Не используются connection/session-level SET и app.user_id из клиента. Hash получен только после проверки случайного session cookie; expired/revoked/unverified session не даёт пользователя. Функция current_user_id повторно проверяет этот hash в sessions, malformed/отсутствующий context →NULL.

Read organizations: id=context.organization_id AND действительная membership current_user_id. Update: те же условия плюс role=owner, USING и WITH CHECK. Members read: только своя membership выбранного tenant. Подмена organization_id на чужой, пустой/невалидный hash и попытка задать произвольный app.user_id не дают доступа. UUID —валидируемый идентификатор, не право.

Каждая транзакция закрывается commit/rollback; Get использует rollback после read, List commits read transaction. Pool не содержит tenant context вне transaction; проверены commit и rollback reuse. Ограничение transaction/timeouts сохраняется. RLS —дополнительный барьер, backend тоже проверяет роль и возвращает безопасные 404/403. Forbidden owner operation на своей организации —403, чужая/отсутствующая —одинаковый 404.

## Bootstrap без BYPASSRLS

Узкие SECURITY DEFINER functions с search_path=pg_catalog,platform,pg_temp, квалифицированными table names и без dynamic SQL:

- session_user(hash): verified current user и CSRF hash по одному непредсказуемому session hash, с проверкой TTL/revocation.
- touch_session / revoke_session: одна указанная session, без выдачи списка hashes.
- current_user_id: пользователь session context.
- has_membership(org, owner): boolean, предотвращает рекурсивные RLS policy queries.
- list_organizations(): только memberships текущего session user, список без произвольного user_id параметра.
- create_organization(id,name,timezone): получает user из session context; организация+owner одной DB transaction, откат обоих при любой ошибке.

EXECUTE отозван у PUBLIC, выдан aba_app; функции принадлежат непривилегированному migrator. FORCE RLS продолжает действовать для owner, поэтому есть отдельная policy TO aba_migrator USING true WITH CHECK true. Она служит миграциям/узким definer functions, а не runtime. aba_app не член aba_migrator, не может SET ROLE в него. Нет общей bypass-функции, SQL strings/table/user parameters не принимаются. [Правила безопасности SECURITY DEFINER PostgreSQL](https://www.postgresql.org/docs/current/sql-createfunction.html).

Это осознанное минимальное уточнение архитектурного app.organization_id/app.user_id подхода: дополнительно привязываем identity к session hash. Нельзя считать свободно выставленный user UUID доказательством входа. Для будущих tenant таблиц понадобятся organization_id, составные FK (org,target_id) и аналогичные USING/WITH CHECK policies; текущий этап их заранее не создаёт.

## Миграции и сохранение данных

00001 сохранена. 00002 добавляет identity/auth/org tables/functions/policies и schema metadata version=2, не удаляя старую metadata. Runtime readiness требует 2 и безопасную роль. Up повторяем, goose advisory lock сериализует jobs, API не запускает DDL.

Down 00002 разрешён только если users и organizations пусты; иначе транзакционно отказывается, сохраняя данные и version=2. Это предотвращает случайное уничтожение аккаунтов/магазинов. Roundtrip down/up проверяется на новой пустой aba_test, не dev volume. Не обходить guard ради зелёного теста. Test marker + exact aba_test name + чистая platform schema обязательны до любых тестовых миграций. Удаление только собственных ephemeral test resources; никаких DROP DATABASE пользовательского окружения.

## Доказательства тестами

Go integration создаёт пользователей A/B и организации A/B, дополнительную A2; подтверждение идёт через настоящий dev sink. Проверяет:

- list содержит только memberships, A читает A/A2, не B; GET/PATCH B →404;
- owner редактирует A, stale version →409; B как member A читает, PATCH →403;
- прямой SQL runtime с A session + B tenant: SELECT=0, UPDATE=0;
- B member session + A tenant: прямой SQL UPDATE=0;
- после commit/rollback pool query без context не видит tenant rows;
- защищённые таблицы owner=migrator, ENABLE/FORCE RLS; sessions enumeration запрещена;
- session expiry/revocation, CSRF, account rate limiting;
- blank DB up/repeat/concurrent up/down/re-up; nonempty down guard.

Browser тест проверяет SSR redirect без session, Mailpit verification, login/persist on reload, создание/редактирование, XSS text output и logout redirect. Никакой frontend фильтр не заменяет backend/RLS. Тестовые users/messages строго синтетические. Фактические результаты —STAGE_3_REPORT; сами инструкции не являются доказательством PASS.

Будущие AI/background tenant jobs не должны копировать owner session в очередь или получать миграционные привилегии. Для них потребуется отдельный проверяемый system principal/job context и отдельные RLS-тесты; текущий email worker использует только глобальную auth очередь и tenant context ему не нужен.

## Расширение ТЗ № 4

Tenant-owned также product_categories, products, product_variants, inventory, inventory_movements. Все organization_id обязательны, связанные tenant FK составные. FORCE RLS: SELECT membership; INSERT/UPDATE owner, USING/WITH CHECK selected org + проверенная session. Runtime не имеет DELETE и не может UPDATE movements. Movement INSERT проверяет actor_id=current_user_id(). Owner policy migrator остаётся только для миграций. AfterConnect ownership check расширен на все защищённые таблицы. Category/product/variant/stock методы повторно проверяют роль в своей SET LOCAL transaction. CustomerCatalog не ослабляет RLS и исключает draft/archived. Подробности PRODUCT_CATALOG и INVENTORY_MANAGEMENT; реальные negative tests —catalog_test.go и STAGE_4_REPORT.

## Расширение ТЗ № 5

orders/order_items/order_events/order_quotes/conversations/messages/conversation_events/ai_usage tenant-owned, compound FK, ENABLE/FORCE RLS, owner=migrator. Policies selected org/session + can_operate_orders + is_test_org. Member defaultdeny, explicit staff от owner. Организации прежнего этапа остаются is_testfalse; runtime нельзя UPDATE is_test. Узкие definer helpers перечислены SECURITY/TECHNICAL_DECISIONS. Direct arbitrary SQL endpoint отсутствует. Catalog owner-only UPDATE policy сохраняется; staff stock updates только test org, actual API через order transaction. AfterConnect проверяет ownership всех новых protected tables; metadata4. Services SET LOCAL+commit/rollback, no transaction held during provider network.

## Этап6: channel capability

Telegram integrations/principals/inbox/outbox/events tenant-owned, FORCE RLS; migration00005. Existing catalog read policies дополнены только active read в выбранном tenant с valid inbox capability. Conversations/quotes/orders scoped к exact leased conversation. Channel не имеет account session/membership и не меняет inventory/price/order status; staff права прежние. Узкие lock_inventory/product helpers нужны для serializable price/availability snapshot без выдачи channel UPDATE privilege. Go guards + DB order source/UPDATE trigger предотвращают employee operation от channel. API customer impersonation/quote-consent сотрудником для telegram запрещены. SET LOCAL scope cleared commit/rollback, реальные PG integration проверяют capability expiry/recovery/pool/foreign rows. AfterConnect расширен на5новых tenant tables, readiness metadata6. Подробнее TELEGRAM_SECURITY.

## Дополнение этапа 7

ai_settings, llm_usage, llm_reservations, agent_metrics, ai_settings_events — tenant-owned, ENABLE + FORCE RLS, scoped read и составной FK reservations→conversation. Global llm_platform_budget не tenant-owned и вообще недоступен прямому runtime SELECT/UPDATE. Narrow reserve/settle functions проверяют selected org и authenticated staff либо Telegram capability. Runtime денежные записи только через функции; owner approval settings отдельно. Новые интеграционные тесты подготовлены в stage7_test.go; RLS stage7 выполнен на изолированной PostgreSQL: PASS. Не переносить исторический PASS этапа6 на новую миграцию.

## Проверка 8А

Полный фактический каталог25 tenant-таблиц, policies/USING/WITH CHECK/grants/FK: [SECURITY_RLS_INVENTORY.json](SECURITY_RLS_INVENTORY.json), объяснения глобальных сущностей — SECURITY_AUDIT_REPORT.md. testAuditRLS проверяет каждую реальной runtime ролью aba_app на чужих строках и подмене tenant. Миграция8 ограничивает message UPDATE по колонкам и Telegram policy до retention erasure. readiness требует schema8 и ENABLE/FORCE для известных таблиц. Новую tenant-таблицу нужно добавить в список readiness и аудит, а не считать защищённой автоматически.

## Актуализация8Б.1

Readiness требует schema9 и ENABLE/FORCE уже27 tenant-owned таблиц. Новые `telegram_delivery_events`, `llm_budget_events` имеют organization_id, FORCE RLS, migration-owner policy, scoped SELECT и составные FK на соответствующую очередь/reservation. Runtime не пишет/не удаляет audit напрямую. `llm_settle_core`/сохранённый rollback helper недоступны runtime; recover_stale — узкая maintenance function без возврата строк/повышения лимитов/refund/HTTP. Фактический каталог и новые runtime negative tests: [SECURITY_RLS_INVENTORY_8B1.json](SECURITY_RLS_INVENTORY_8B1.json), `testAuditRLS`. Старые контекстные/session boundaries не менялись.

## 8Б.2 — расширение inventory до32 tenant-owned таблиц

В00010 добавлены channel_integrations/principals/inbox/outbox/events с ENABLE/FORCE, org FK и tenant-safe связями. Runtime не имеет прямых write grants в queue/security/audit; select секретного ciphertext запрещён. Общие catalog/conversation/order policies отдельно допускают leased WhatsApp scope; Telegram policies не расширены на WA. `app.channel_cap` только SET LOCAL, проверяется org/active/test/lease/conversation; TG и WA capabilities взаимно непригодны. Runtime role validation/readiness проверяет новые таблицы и schema10. Глобальная channel_function_backup содержит только SQL definitions, недоступна runtime, не требует tenant RLS. Тесты: multichannel_test.go и расширенный32-table audit_test.go (конкретный результат в STAGE_8B2_REPORT).
