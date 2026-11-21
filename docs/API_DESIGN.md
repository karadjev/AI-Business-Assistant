# REST API и личный кабинет

> ТЗ № 4: реализованы auth, organizations, каталог и склад; Ниже — целевая архитектура, а не перечень готовых бизнес-функций. Фактический запуск и проверки: [LOCAL_DEVELOPMENT](LOCAL_DEVELOPMENT.md).
Проектный контракт, не работающие endpoints. Префикс `/api/v1`, JSON UTF-8, UUID в строках, даты ISO 8601 UTC, суммы `{amount_minor,currency}`. OpenAPI реализованной платформы создан в packages/contracts/openapi.yaml и содержит health/status, auth и organizations. Таблица ниже определяет будущие бизнес-ресурсы и команды.

## Общие правила

Cookie session Secure/HttpOnly/SameSite=Lax на общем origin; unsafe methods требуют CSRF token и проверки Origin. Membership/RBAC проверяются на каждом запросе. org_id из URL проверяется, не является полномочием. Чужой ресурс — 404, доступ к запрещённой функции внутри своего tenant — 403. Cursor pagination limit 1–100 (default 20), сортировка timestamp+id; opaque cursor привязан к tenant, фильтру и ресурсу. Ответ списка `{items,next_cursor}`.

Ошибки: `{error:{code,message,request_id,fields?}}`. 400 синтаксис, 401 сессия, 403 право, 404 ресурс, 409 версия/состояние/идемпотентность, 422 валидация, 429 лимит (Retry-After), 503 временная недоступность. Ни SQL, ни stack trace, ни данные провайдера клиенту. Сервер логирует очищенный error_code.

POST создания заявки/перехода требует Idempotency-Key. Запись `(tenant,key,request_hash,result)` в Orders либо command-record создаётся транзакционно; одинаковый key+body возвращает результат, иной body —409. Для transitions отдельная служебная command-record таблица при реализации, уникальная `(org,order,key)`, request_hash, result, created_at; либо эквивалентная уникальная запись movement+audit без потери результата. Version передаётся через If-Match; изменяемые ресурсы возвращают ETag. Нет тихого overwrite. Финансовые и stock mutations не принимают неопределённые partial payloads.

## Ресурсы и роли

O = `/organizations/{org_id}`. Owner обладает всеми tenant-правами; A=admin, Op=operator, V=viewer. Для MVP доступ owner, другие роли проектные.

| Метод и путь | Тело / результат | Доступ |
|---|---|---|
| POST `/auth/register` | email,password,display_name →202; neutral result | public, rate limit |
| POST `/auth/verify-email` | одноразовый token →204 | public |
| POST `/auth/login` | email,password → session cookie + me | public, generic failure |
| POST `/auth/logout` | revoke session →204 | session+CSRF |
| POST `/auth/password-reset` | email →202 независимо от наличия | public |
| POST `/auth/password-reset/confirm` | token,new_password →204; revoke sessions | public |
| GET `/me` | профиль, memberships | self |
| POST `/organizations` | name,business_type,timezone →organization+owner | verified user; пилотный лимит |
| GET/PATCH `O` | profile/settings, ETag | read members; write owner/A |
| GET/POST/PATCH `O/members[/{id}]` | список, приглашение, role/status | owner; UI после MVP |
| GET/POST `O/products` | filters/q/cursor; name,brand,model,attributes,default_variant →product | read owner/A/Op/V; write owner/A |
| GET/PATCH `O/products/{id}` | карточка, version; editable fields | те же |
| POST `O/products/{id}/variants` | sku,name,attributes,price_minor →variant | owner/A |
| PATCH `O/variants/{id}` | attributes/status с If-Match | owner/A |
| PATCH `O/variants/{id}/price` | amount_minor,currency,expected_price_version | owner/A |
| GET `O/inventory` | variant filter; on_hand,reserved,available,verified_at | owner/A/Op/V |
| POST `O/inventory/{variant_id}/adjustments` | target_on_hand,expected_version,reason,verify bool | owner/A; журнал |
| GET/POST `O/agents` | конфигурация; создание draft | owner/A; Op видит статус |
| PATCH `O/agents/{id}` | draft instructions/policy, version | owner/A |
| POST `O/agents/{id}/publish` | draft_version →active immutable version | owner/A; аудит |
| POST `O/agents/{id}/pause` | reason →paused | owner/A |
| GET `O/conversations` | status/control/cursor → summaries | owner/A/Op |
| GET `O/conversations/{id}/messages` | cursor → ordered history и delivery | owner/A/Op |
| POST `O/conversations/{id}/messages` | text,client_message_id →pending message | owner/A/Op; только human mode |
| POST `O/conversations/{id}/handoff` | reason,expected_revision →human | owner/A/Op |
| POST `O/conversations/{id}/resume-ai` | expected_revision →ai | owner/A/Op; явное действие |
| GET/POST `O/orders` | filters; customer_id,items,confirmation context →requested | owner/A/Op; ручная заявка с подтверждением сотрудника |
| GET `O/orders/{id}` | items/snapshot/status/version | owner/A/Op |
| POST `O/orders/{id}/transitions` | target_status,expected_version,reason | owner/A/Op; stock transaction |
| GET/POST `O/integrations` | masked config; channel+credentials →pending | owner/A |
| POST `O/integrations/{id}/verify` | provider check →connection status | owner/A |
| POST `O/integrations/{id}/disable` | stop reception/automation →status | owner/A |
| GET `O/notifications` | unread,cursor →inbox | own recipient |
| POST `O/notifications/{id}/read` | →204 | own recipient |
| GET `O/analytics/summary` | from,to → counts/rates/cost,coverage | owner/A/V; Op ограниченный dashboard |
| GET `O/subscription`, `O/usage` | plan,limits,measured/estimated spend | owner; A usage |
| POST `/webhooks/telegram/{public_id}` | Telegram Update →200 только после inbox commit | secret header, не cookie |
| GET/POST `/webhooks/whatsapp/{public_id}` | verify challenge / events | verify token / HMAC; позже |

AI-tools — внутренние Go-команды, не публичный API для выполнения произвольных tools из браузера. Endpoint регистрации webhook не принимает произвольный URL: callback строит сервер. Веб-чат позже требует отдельную scoped session; tenant slug не даёт доступ к истории.

Пример мутации остатков: `{target_on_hand:5,expected_version:7,reason:"Сверка магазина",verify:true}`. Ответ `{on_hand:5,reserved:1,available:4,version:8,verified_at:"…"}`; при конфликте 409, кабинет предлагает перезагрузить данные. Цена заявки рассчитывается сервером; поля total/unit_price из клиента не принимаются.

## Страницы

| Страница | Функциональность и компоненты | API |
|---|---|---|
| Dashboard | Карточки обращений/заявок/расходов, новые заявки, список активных диалогов, статус интеграции, warning freshness | analytics/summary, orders, conversations, notifications, integrations |
| Products | Таблица/мобильные карточки, поиск, фильтры, drawer редактирования, варианты, цена, остатки, дата сверки, conflict banner | products, variants/price, inventory/adjustments |
| AI Agent | Форма тона/графика, безопасные инструкции, allowlist действий, статус версии, publish/pause, предупреждение бюджета | agents, publish/pause, usage |
| Conversations | Список и чат, search/filter, сообщения, delivery indicators, контекст заявки, переключатель human, composer | conversations, messages, handoff/resume-ai |
| Orders | Таблица/карточки, фильтр статусов, item snapshots, timeline, confirm/cancel/complete; сообщение о недостатке товара | orders, detail, transitions |
| Integrations | Карточка Telegram, пошаговое подключение бота, secret input без повторного показа, health, disable; остальные каналы «планируются» | integrations, verify/disable |
| Analytics | Период, counts/rates, токены/оценка расходов, наличие неполных данных; без выдуманной выручки | analytics/summary, usage |
| Settings | Компания, часовой пояс, контакты, график, пароль/сессия; сотрудники/роли позже, безопасность и политика данных | O, me, auth, members позже |

Дополнительно: Login/Register/Reset и onboarding wizard. Минимализм: один основной action на экран, понятные русские названия, доступные контраст/labels/focus, поддержка клавиатуры. На мобильном списки как карточки, чат отдельным экраном, формы одной колонкой. Каждая страница имеет loading/empty/error/permission/conflict states. Секреты, trace и provider internals не показываются в рабочем сценарии; пользователю нужны состояние подключения и понятное действие.

Обновление inbox сначала polling 5–10 с, с паузой в фоне; SSE позже при потребности. API и polling rate ограничены. Не требуется WebSocket для MVP.

## Определения базовых метрик

Период [from,to) по UTC с отображением в timezone магазина. Обращения = число conversations с хотя бы одним inbound в периоде (не количество LLM-вызовов). Сообщения = принятые уникальные inbound. Заявки = orders.created_at в периоде. Конверсия = conversations периода, давшие requested в том же периоде / conversations с inbound; заявки без исходного conversation отдельно. Автоматизация = conversations с inbound и ответом AI без handoff/human replies в периоде / conversations с inbound; незавершённые и ожидающие отдельно, это показатель наблюдаемого периода, не доказательство полного решения проблемы. Расходы = ai_runs в периоде с разделением exact/estimated/unknown; нулевой denominator → null. Не суммировать валюты напрямую. В MVP SQL-агрегаты tenant-scoped, экспорт переписок отложен.

## Реализовано в ТЗ № 3

Авторитетный исполняемый контракт —packages/contracts/openapi.yaml (0.2.0). `/api/v1/auth/{register,verify-email,resend-verification,login,logout,me}`, `/api/v1/organizations` GET/POST, `/api/v1/organizations/{id}` GET/PATCH. Роли owner/member, email verification через dev sink, CSRF header+Origin для cookie mutations. Остальные приведённые выше бизнес-endpoints остаются будущими. Error codes/status и request schemas описаны в OpenAPI; актуальные security/ownership правила —AUTHENTICATION и TENANT_ISOLATION.

## Фактический контракт ТЗ № 4

OpenAPI 0.4.0 добавляет категории, products/detail/publish/archive/variants, variant PATCH, inventory/adjustments/movements. Полный список —PRODUCT_CATALOG и OpenAPI. Owner writes/member reads, session+CSRF, foreign404. Суммы price_amount BIGINT копеек/currency_code RUB. Page/page_size вместо cursor, expected_version JSON вместо If-Match. PATCH редактируемого набора (optional поля с default), не Merge Patch. Категории и SKU tenant-owned. Проектные variant price и inventory/{id}/adjustments endpoints выше заменены единым PATCH variant и inventory/adjustments collection для реализованной части. Остальные будущие ресурсы не объявлены работающими.

## Фактический контракт ТЗ № 5

OpenAPI0.5.0 добавляет POST organizations/test, org orders GET/detail и confirm/reject/cancel/complete POST, conversations GET/POST/detail/messages/handoff/resume/delete/quotes/confirm-request, agent/status GET, existing member/order-access PATCH. Explicit consent + UUID idempotency/expected_version JSON. Default member403, owner/opt-in staff; только test organization, foreign404. Чаты source internal_chat, не внешний канал. Подробности новых endpoints/models —ORDER_MANAGEMENT и CONVERSATION_LIFECYCLE.

## Реализовано в этапе6

Контракт0.6.0: O/integrations/telegram GET, connect/check/disconnect/synthetic-updates POST; integration-bound `/api/v1/webhooks/telegram/{integration}` POST secret header. Только mock/test scope. Status owner/staff, config/synthetic owner+CSRF; body16KiB webhook, ACK after durable commit, duplicate200, invalid422/400/403, quota429, DB503. Existing conversations/orders API reuse; staff customer impersonation и quote-consent в Telegram запрещены. Sources internal_chat/telegram; customer scope —leased capability, не session. Endpoint/schema/details TELEGRAM_INTEGRATION + actual OpenAPI. Real webhook registration/send NOT RUN/disabled.

## Реализованная подготовка этапа 7

GET/PATCH `/api/v1/organizations/{id}/ai-settings`, GET `.../agent/metrics`, POST `.../integrations/telegram/live-operations`, POST `.../conversations/{resource}/purge-test-data`. Сессия обязательна; test organization, staff read, owner change, CSRF для изменений. Лимиты microRUB, expected_version; race409, validation422, denied403, foreign404. Сейчас live operations всегда403 без внешней сети. Авторитетный контракт — packages/contracts/openapi.yaml version0.7.0, проверка scripts/check-openapi.mjs.

## Этап8Б.2 — WhatsApp mock и фильтр каналов

Реализованы GET integrations/whatsapp; POST connect/control/synthetic-updates/deliveries/{resource}/resolve в organization scope; GET/POST webhooks/whatsapp/{integration}. Настройка owner-only, чтение/staff recovery требуют действующей session и staff в test org. Public webhook проверяет verify token (GET) или raw HMAC+phone/WABA+allowlist (POST); не регистрирует webhook у провайдера. GET conversations поддерживает channel. Актуальные JSON schemas/status codes — packages/contracts/openapi.yaml; роли, ограничения и инструкции — WHATSAPP_INTEGRATION/MESSAGE_POLICY/MULTICHANNEL_DELIVERY. Реальных Meta requests нет.
