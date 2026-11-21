# Instagram — предварительный security review

Дата: 2026-11-18. Решение: NOT READY. Это review offline-контрактов, не подтверждение production isolation.

## Реализованные ограничения

- Runtime выключен; попытка включения или подачи credentials отклоняется config validation.
- Подпись HMAC-SHA256 raw body проверяется до JSON processing/Enqueue; payload ≤64 KiB. Duplicate signature headers отклоняются.
- Account/recipient проверяются относительно server-owned binding; tenant из webhook не читается. Неизвестный account отклоняется.
- Действующий tenant+integration+account задаёт AAD AES-256-GCM. Ciphertext другого binding не расшифровывается; ключ исключён из JSON.
- OAuth state: random 256 bits, hash-only storage interface, tenant/user/session/redirect binding, expiry, атомарное одноразовое потребление как обязательство repository.
- Policy: подтверждения access/webhook, expiry, recipient state, 24h window, kill/sending state, запрет DELIVERY_UNKNOWN resend.
- Mock sending принимает только synthetic token, ограничивает текст 1000 байт, использует фиксированный Instagram host. Только API success с message_id и правильным recipient даёт `sent`; это не доказательство доставки.
- Timeout, malformed success, 5xx → `delivery_unknown`. Подтверждённый 429 с ограниченным Retry-After допускает ограниченный backoff; автоматический scheduler пока отсутствует.
- Media URLs не используются для запросов. Нет scraping, password collection, browser login, unofficial API или live network client.

## Непройденные gates

1. StateStore и Enqueue — интерфейсы; тестовые реализации не обеспечивают production durability. Нет account ownership repository и уникального account mapping.
2. Трёхмагазинные unit fixtures проверяют binding, credential AAD, OAuth state и sender tenant; они не заменяют PostgreSQL FORCE RLS и cross-conversation tests.
3. Не реализованы Instagram capabilities, tenant-scoped background lease, shared inbox metadata/attachments, conversation authorization, AI context и order idempotency через очередь.
4. Не выполнены migration/RLS/concurrency, Telegram/WhatsApp regression, race/vet/build и browser gates текущего изменения.
5. Требуются токен revoke/delete, rotation, privacy/deletion endpoint, sanitization на reverse proxy и в OAuth logging, subscription identity verification и status reconciliation.
6. Нет поддержки reply sending, story-specific context, profile lookup, provider handover и официальных исключений messaging window. Они должны оставаться отключены до проверки.

Существующие FORCE RLS, SQL policies, financial admission, catalog freshness и order consent не изменены. Их регрессионная работоспособность в этой сессии не подтверждена. E01 остаётся OPEN; artifact guards не изменялись.
