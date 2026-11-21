# Безопасность Telegram

## Границы этапа

Только mock transport, mock AI, отдельные test shops, synthetic messages. LoadTelegram отвергает live transport; production enabled запрещён до согласования privacy policy. Реальные токены/сообщения/Telegram HTTP не используются. Это намеренная граница безопасности, не фиктивная рабочая интеграция с внешней сетью.

AES-256-GCM: случайный nonce на каждое шифрование, organization UUID как authenticated associated data. Серверный TELEGRAM_ENCRYPTION_KEY —32 байта hex, не БД и не NEXT_PUBLIC; отдельный от email key, только ignored env. Runtime не может SELECT token_cipher/webhook_hash столбцы. Узкие SECURITY DEFINER операции владельца/worker возвращают cipher только при owner check либо на claimed send. Все search_path fixed, PUBLIC EXECUTE revoked, no dynamic SQL. Runtime aba_app без superuser/BYPASSRLS/schema CREATE/table ownership. Компрометация runtime credentials + server encryption key вне гарантии tenant context; требуется отдельный production secrets manager/rotation plan до live.

В браузере credential password field, autocomplete off, очищается сразу после submit; не localStorage. Только synthetic mock credentials разрешены на локальном HTTP. Реальный token сможет передаваться только HTTPS после безопасного включения канала. API не возвращает токен, cipher, webhook secret/hash, job capability. Логи содержат только безопасные коды/состояния, не transport URL (в Telegram URL находится token), тело, заголовки или customer context. HTTP adapter подавляет Telegram descriptions и URL-bearing errors, запрещает redirects, uses fixed HTTPS base URL, bounded body/timeouts; локальные tests используют отдельный test client.

## Principal и RLS

Telegram external user/chat ID не даёт account session, membership или owner role. Username не хранится как identity. Private chat требует positive from ID, nonbot, chat.id=from.id, chat.type=private. DB UNIQUE(integration,user,chat) и compound organization FK. Bot принадлежит только одной организации; повторная установка того же бота в чужом магазине отклоняется без выдачи tenant metadata. Настройки serialized advisory lock, неизменяемый bot ID для существующей org binding.

Worker сначала предъявляет HMAC-SHA256 queue proof, производный от server encryption key с отдельным purpose. БД хранит только SHA256 proof; runtime не может SELECT этот column. Claim/send/purge проверяют proof, один runtime SQL login без server secret не получает channel scope или ciphertext. Proof не API/logs. Затем Worker получает ephemeral random capability конкретного inbox job. platform.telegram_scope сверяет capability/status/lease/active integration/test organization/transaction-local selected org и, для customer data, точный conversation. Go Tenant.ChannelCapability устанавливается только worker; отсутствует во входных DTO/tools. SET LOCAL через set_config(...true), транзакции всегда закрываются; customer job не копирует session owner. Конец lease, finish, отключение интеграции лишают capability силы. Read catalogue active-only; customer policies не разрешают UPDATE inventory или products. Internal order Change дополнительно запрещает channel principal, DB trigger запрещает channel UPDATE orders и ставит source telegram при INSERT. Старые staff policies сохраняются.

Сотрудник не может POST customer message, quote/confirm-request от имени Telegram-покупателя. Он может handoff, human reply, явный resume и order decisions. Staff outbox/все FK привязаны к тому же principal/chat. Customer cannot resume/delete; agent tool can only handoff. Inline confirm с random quote UUID связан с conversation/customer, серверными snapshots/expiry; order unique quote/idempotency. Свободное «да» в сообщении и tool text не являются согласием. Стоимость и наличие повторно проверяются существующим Orders service, товар не резервируется при customer consent.

## Ingress и transport

Webhook endpoint UUID —routing, не credential. Проверяется secret header constant-time SHA256 comparison в Go + повторный DB binding. Только normalizer-approved private updates, 16KiB, bounded middleware timeouts. Global/per-integration quota, durable dedup. Origin/CSRF на webhook отсутствуют по природе server-to-server; session-auth изменения через BFF сохраняют оба. Synthetic route требует owner/CSRF/test scope; production configuration не допускает enabled/debug route.

Данные каталога, callback data, user text и tool results недоверенные. Telegram plain text, no parse_mode, no link previews; React escaping. Agent schemas и bounded loops/quotas/redaction из этапа5 не ослаблены. Callback args не несут price/tenant и не исполняют URL/SQL. Tool/error tests —контрольные сценарии, не полный penetration/red-team audit.

## Retention

TELEGRAM_RETENTION_DAYS=7, диапазон1–30, применяется при создании conversation; изменения env не переписывают срок уже созданных. Worker background purge закрывает expired Telegram chats и очищает messages content/data/request_hash, inbox/outbox payload. Explicit existing delete очищает те же channel payloads. Заявки, audit, delivery metadata, principal IDs и routing остаются: это temporary synthetic retention, **не полное production обезличивание/удаление всех ПДн**. Worker outage задерживает purge до восстановления; payload в backups отдельно не стирается. До реальных покупателей необходимы согласованный срок хранения всех metadata/backups, права удаления, основания/согласия и производственный privacy review. До этого приложение не включает production processing.

Отключение останавливает новые claims и pending sends, удаляет cipher; уже начатый внешний запрос нельзя гарантированно отменить. В mock внешнего эффекта нет. Потерянный ответ отправки —uncertain; автоматическая повторная отправка блокируется, см. MESSAGE_DELIVERY.

## Уточнение этапа 8А

Исходники содержат gated live transport подготовки этапа7, однако Telegram external=false, sending=false, kill=true. Это уточняет прежние описания отсутствия live-кода: live **запрещён конфигурацией**, но адаптер существует. В 8А выполнены только mock/loopback HTTP проверки. Миграция8 проверяет integration active/mode/allowlist/human state повторно непосредственно перед send, сериализует send/disconnect/control transitions. Уже допущенная отправка может завершиться до возврата disconnect; точно отозвать внешний запрос невозможно. Полные доказательства и setup: SECURITY_AUDIT_REPORT.md, TELEGRAM_TEST_BOT_SETUP.md.
