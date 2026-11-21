# Надёжность inbox/outbox

Никаких Redis/Kafka. Inbox unique(integration_id,update_id) —durable dedup. Webhook нормализует payload и коммитит запись перед ACK; duplicate ACK200. Synthetic ingress использует те же таблицы/processor, owner session никогда не помещается в job.

## Inbox

pending →processing →done; при ошибке →pending с exponential backoff2^attempt секунд (до300), максимум5 попыток, затем failed. Claim требует secret server worker proof (HMAC purpose, hash-only registry), затем SELECT FOR UPDATE SKIP LOCKED; random capability +30s lease. Самое раннее сохранённое незавершённое событие chat блокирует последующие этого chat, разные chats независимы. Порядок —**порядок durable arrival (created_at,id)**, не обещание восстановить Telegram occurrence order: IDs общие для бота, отсутствующие IDs не ожидаются. Late/ambiguous callback не обходит quote/customer verification. При crash lease переиздаётся; последняя exhausted lease становится failed. Перед claim выполняется bounded синтетический retention purge.

Существующий Conversations.Send сохраняет customer message по inbox job UUID как client_key. Повтор после commit без inbox finish получает ранее сохранённый результат, не добавляет message/assistant второй раз. AI calls выполняются вне DB transaction; финальный ответ fenced существующими conversation version/run/lease. Handoff прекращает ответы и отбрасывает поздний run. Worker uses mock Provider и existing CustomerCatalog; нового AI engine нет.

Offer callback создаёт server quote5min; confirm callback создаёт pending request с idempotency=quote UUID. Crash может оставить лишнюю неиспользованную quote, но customer request уникален по quote/key. Quote и confirmation проверяют тот же conversation/customer; changed/expired/stock conflicts безопасно просят новое предложение. Системная ошибка не выдаётся за успешную заявку. Резерв только employee transition через прежнюю транзакцию.

## Outbox

Message INSERT assistant/staff запускает узкий DB trigger и **атомарно** пишет outbox. Catalogue cards преобразуются в plain facts/inline offer buttons; цена целочисленная, freshness/manual timestamp, без promises reserve. Quote/confirmation replies имеют deterministic message UUID из inbox job UUID; INSERT message + outbox trigger в одном commit, ON CONFLICT DO NOTHING. Нажатия customer buttons тоже сохраняются в history; order reply содержит безопасный linked order ID для кабинета. Никакой внешний HTTP внутри PostgreSQL transaction.

pending →sending →sent; receipt записывается по claim capability. Lease15s. Retryable explicit rejection/429 →pending, delay=max(retry_after,2^attempt), capped3600s, максимум5. Nonretryable error →failed. Transport timeout/неопределённый ответ →uncertain; expired sending lease также становится uncertain. HTTP send timeout5s/body64KiB/redirects disabled. Worker defaulttick1s, один inbox/send за tick; production throughput/per-bot shaping не измерены и до live требуют проверки. Общий processing context8s; cancellation сохраняет bounded independent completion1s и завершает process gracefully.

Oldest pending/sending/uncertain send блокирует последующие этого chat, чтобы не менять состояние сообщения незаметным reorder. Uncertain не отправляется автоматически: безопаснее остановить и показать ошибку. В stage6 нет UI ручного resend/resolve; перед реальным каналом нужен проверяемый owner/support reconciliation с предупреждением о возможном дубле. Disconnect отменяет pending; начавшаяся отправка может завершиться. Sent в mock означает receipt mock transport, не доставку настоящему покупателю.

У внешней отправки **нет строгого exactly-once**. Telegram может принять sendMessage, а приложение потерять receipt/умереть до его commit. Unique logical key защищает DB enqueue, но не отменяет внешний эффект. Автоматический retry неопределённого send запрещён; при ручном повторе возможен дубль. Даже успешный provider receipt не доказывает чтение сообщения. Правила Telegram: [Bot API](https://core.telegram.org/bots/api#sendmessage), [webhook](https://core.telegram.org/bots/api#setwebhook), [429 response parameters](https://core.telegram.org/bots/api#responseparameters).

## Диагностика

В integration page: pending_inbox, failed_outbox, safe last_error и последние10 delivery status/payload. В conversations —история/control; orders —source/channel/customer/request. Worker logs telegram_processing_unavailable/telegram_delivery_unavailable, без body/token. Dead-letter после5 attempts и uncertain видны, но не автоматически скрываются фиктивным успехом. Нет внешнего мониторинга/alerts/production SLO на этом этапе.

```sh
sh scripts/integration.sh
sh scripts/smoke-compose.sh
```

Integration запускает отдельную aba_test с safety marker/tmpfs, snapshot source в container перед Go compile. Это устраняет неоднозначность при продолжающихся правках checkout; рабочая БД не очищается. Проверяются concurrent claim, own/foreign principal, committed-message replay, lease recovery, order consent/dedup, RLS/pool, retries, handoff/staff и erasure. Реальные provider outages/rate limits не проверены, только mock/local protocol fixtures. Фактические результаты —STAGE_6_REPORT.

## Изменения безопасности 8А

Миграция8: новый telegram_prepare_send и session advisory send fence. Соединение закреплено во время HTTP, **SQL-транзакция не открыта**; finish на том же соединении, unlock или destruction при ошибке. Disconnect отменяет и pending, и sending; handoff отменяет недоставленные assistant (кроме серверного handoff notice). Enqueue использует тот же fence, исключая late commit после disconnect. sendMessage5xx, timeout и потеря подтверждения — uncertain, без автоматического повтора. 429 остаётся bounded retry. Последующие сообщения за uncertain блокируются: operator workflow A09 остаётся OPEN. Порядок — durable ingestion, не ожидание любых будущих пропущенных update IDs.

## Актуализация8Б.1

A09 FIXED: `telegram_delivery_events`, outbox version и staff resolution с idempotency+CSRF. Допустимы delivered (проверено сотрудником) и abandon; resend не предусмотрен, exactly-once не обещается. Любой HTTP5xx имеет приоритет над error_code429 в теле и не запускает send retry. Реальные process crash/restart и потеря HTTP receipt проверены offline; [TELEGRAM_DELIVERY_RECOVERY.md](TELEGRAM_DELIVERY_RECOVERY.md).
