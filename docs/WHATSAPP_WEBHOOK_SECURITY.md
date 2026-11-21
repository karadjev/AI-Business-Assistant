# WhatsApp webhook security — 9А

Существующие пути не менялись: GET/POST `/api/v1/webhooks/whatsapp/{integration}`. OpenAPI публичный контракт соответствует прежним handlers, новых endpoint нет.

GET проверяет exact hub.mode=subscribe, единственность аргументов, constant-time verify token, numeric challenge. Challenge не даёт права на POST. POST требует JSON,64KiB cap, корректный header X-Hub-Signature-256, global rate limit, HMAC-SHA256 **сырых bytes** с tenant App Secret. Некорректные signature/integration не создают principal/диалог. Ошибки не возвращают секреты.

В `Adapter.Normalize` дополнительно проверяются integration ID, entry WABA ID, metadata Phone Number ID, event timestamp, allowlisted user, batch/field limits и отсутствие trailing JSON. Одной подписи недостаточно. OrganizationID берётся из server decrypted integration config, не из webhook. SQL ingress уникально связывает Phone Number ID с организацией, principal с org+integration+channel+externalID; tenant-safe FK и FORCE RLS остаются.

Inbound text и interactive actions нормализуются существующей моделью. Неизвестный media type становится Unsupported; malformed/неподдерживаемые служебные поля/status kinds отвергаются, не исполняются как команда. Delivery sent/delivered/read/failed сопоставляются по integration+recipient+providerID; поздние события не регрессируют delivered/read. Duplicate event не продлевает окно/не создаёт повторную заявку. Window основан на безопасном customer timestamp, не статусе доставки; worker повторно проверяет окно непосредственно перед отправкой. Closed/unknown window -> NEEDS_APPROVED_TEMPLATE, mock не отправляет свободный текст.

Защита доказана unit `TestWebhookContract`, PostgreSQL `runMultichannelTests`, `auditTenantTables`, recovery/multichannel stabilization tests. Точные результаты запуска см. STAGE_9A_REPORT и WHATSAPP_OFFLINE_CONTRACT_TESTS. Реальные подписки/HTTPS/TLS/Meta verification NOT RUN. В runtime допускаются исключительно synthetic test org/credentials; будущая real credential ingress требует отдельного разрешения и изменения live configuration gate. Никакой публичный webhook не регистрировался.
