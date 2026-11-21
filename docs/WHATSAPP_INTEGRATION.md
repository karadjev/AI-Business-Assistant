> 9А: добавлен Cloud API HTTP wire transport с отдельным template serializer и fail-closed SendAdmission. Runtime остаётся mock-only, бюджеты0; live financial admission/approved registry ещё не подключены. Подробности: [WHATSAPP_CLOUD_API_TRANSPORT](WHATSAPP_CLOUD_API_TRANSPORT.md), [9Б readiness](STAGE_9B_READINESS.md). Исторические ограничения ниже сохранены.

# WhatsApp Cloud API — offline адаптер

Используется wire-формат официальной WhatsApp Business Platform. WhatsApp Web, личные аккаунты и браузерная автоматизация не используются. Связь с Meta не проверялась.

## Реализовано

- Owner создаёт mock integration в отдельном test org. Поля App ID, Business Account ID, Phone Number ID, Graph API version, synthetic access/app/verify credentials, allowlist, retention/window policy.
- AES-256-GCM шифрует credentials+allowlist, AAD=`organization/whatsapp/v1`. Отдельный32-byte серверный key не хранится в БД. API возвращает только публичную конфигурацию и фактические локальные queue statuses.
- GET challenge проверяет verify token constant-time, subscribe и цифровой challenge. POST отдельно проверяет HMAC исходных bytes и configured WABA/Phone ID; challenge не авторизует POST.
- Inbound text/interactive/unsupported и delivery receipts, bounded batches, allowlist до создания principal. Надёжная запись inbox до HTTP200; повтор события возвращает200 без повторной записи.
- Общие mock agent, catalog tools, quotes и orders. offer/confirm привязаны к variant/quote и conversation. Staff подтверждает заявку и резервирует существующим сервисом.
- Outbox с leases/fencing/попытками/audit/UNKNOWN. Локальный transport и wire-level HTTPMock для тестов; нет production Meta HTTP transport.
- Единый кабинет диалогов, фильтр channel, staff replies, handoff/resume. Закрытое окно блокирует свободный ответ, автоматические templates отсутствуют.

## API

O=`/api/v1/organizations/{id}/integrations/whatsapp`:

| Метод | Путь | Доступ |
|---|---|---|
| GET | O | staff/owner в test org |
| POST | O/connect | owner, mock-only |
| POST | O/control | owner; active, sending_enabled |
| POST | O/synthetic-updates | owner; подписанный fixture создаётся backend |
| POST | O/deliveries/{resource}/resolve | staff; version, sent/cancelled, явное подтверждение |
| GET | /api/v1/webhooks/whatsapp/{integration} | verify token; только challenge |
| POST | тот же webhook | raw HMAC, integration/WABA/Phone mapping, allowlist |

Изменяющие session endpoints используют существующие Origin/CSRF/session middleware, BFF allowlist и request rate limits. Общие conversations/orders endpoints сохранены; GET conversations принимает `channel=telegram|whatsapp|internal_chat`. Чужие организации/диалоги не раскрываются. Invalid signature/routing403, oversize413, content type415, quota429; безопасные backend failures503. Схемы — OpenAPI.

## Конфигурация

`WHATSAPP_ENCRYPTION_KEY` пуст по умолчанию (канал не настроен); `WHATSAPP_LIVE_ENABLED=false`, `WHATSAPP_EXTERNAL_REQUESTS_AUTHORIZED=false`, `WHATSAPP_MONETARY_BUDGET_RUB=0`. Попытка положительного бюджета или live flags отклоняет запуск. Production с ключом отклоняется. API/worker должны использовать одинаковый отдельный локальный ключ.

Connect принимает только credentials с `mock:`. Дефолт окна86400s (разрешено60–86400), retention7 дней (1–30). Это консервативные настройки теста, не подтверждение актуальной политики Meta. Версия `v0.0` в UI/fixtures намеренно синтетическая, не рекомендованная live Graph version.

Mock-отправка отключена при создании и включается отдельным owner control. Отключение отменяет ожидающие задания и запрещает новые ingress/send. Pending UNKNOWN никогда не переотправляется автоматически.

## Ограничения

Одна WA integration/Phone Number ID на организацию, phone_id глобально уникален. Повторный connect не заменяет credentials и не переносит номер между org. Изменение/ротация credentials потребует отдельной audited операции до live; нельзя обходить это ручным изменением owner-данных. Реальная активация, тарификация и регистрация webhook в Meta не реализованы и не разрешены.
