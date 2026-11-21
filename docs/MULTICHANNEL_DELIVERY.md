# Inbox/outbox двух каналов

Общий worker использует channel.Queue: ProcessOne/SendOne. Telegram сохраняет проверенный repository8Б.1; WhatsApp использует новые channel_* таблицы. Идемпотентность бизнес-действий остаётся в общих Conversation/Orders services.

## Inbound

Webhook только проверяет аутентичность, нормализует, валидирует и надёжно сохраняет bounded batch. Уникальные integration+external event keys отбрасывают replay. Не делает LLM/каталог/внешнюю отправку. Claim SKIP LOCKED выдаёт одну capability на30s и блокирует более поздние события того же integration/user до завершения предыдущего. Между независимыми диалогами параллелизм разрешён. Timestamp клиента не задаёт порядок очереди.

Ошибка возвращает pending с bounded exponential backoff, максимум5 попыток; expired processing lease восстанавливается. Исчерпание → failed (dead-letter). При повторе сообщения Conversation client_key не повторяет agent, confirm quote ID не создаёт вторую заявку. Неизвестные media безопасно передаются staff, не скачиваются.

## Outbound

Message+outbox сохраняются в одной SQL-транзакции через trigger. HTTP не выполняется внутри транзакции. Claim устанавливает sending, attempts, lease15s и capability. Worker удерживает pinned **session advisory lock** integration на время bounded5s send. Disconnect/handoff используют тот же lock. Перед send admission заново проверяет active, mock, sending, conversation status/control/expiry, window и lease. Перед сетевой фазой config расшифрована; во время удержания lock worker не ждёт второй pool connection.

Защита: конкурентный worker не claim-ит sending; lease expiry переводит sending в DELIVERY_UNKNOWN, а не pending. Старый capability не завершает новую операцию. Pending FIFO не обходит предыдущее UNKNOWN того же клиента.

| Результат WhatsApp mock wire | Состояние |
|---|---|
| HTTP200, bounded JSON с одним message ID | sent |
| Явный429 | pending, Retry-After/backoff до3600s; максимум5 попыток |
| 5xx | delivery_unknown — провайдер мог принять сообщение |
| Timeout/connection loss/неполный или невалидный success | delivery_unknown |
| Достоверный4xx отказ | failed |
| Закрытое окно | needs_approved_template, без отправки |
| Disabled/handoff/stale admission | cancelled / не отправляется |

Любой outgoing имеет внутренний UUID, attempt counter/capability и append-only channel_events при смене статуса/попытки. Отправка exactly-once не обещается ни одному каналу. Telegram8Б.1 recovery не изменён.

## Recovery

После аварии новый worker видит старые persisted leases. Expired inbox можно retry с прежним message idempotency key. Expired sending становится UNKNOWN без HTTP. Staff в test org явно фиксирует sent/cancelled по expected_version. Устаревшая/повторная версия409; ни одно решение не вызывает resend. Не создавать новое outgoing вручную под видом «безопасного retry» без проверки сотрудником.

WhatsApp delivered/read receipts могут прийти позже/не по порядку. Сопоставление только integration+external user+provider ID. Read не понижается до sent/failed, delivered не понижается до sent/failed. Если provider ID потерян при аварии, событие не угадывает соответствие: остаётся ручное разрешение UNKNOWN.

Retention7 дней по умолчанию; worker стирает истёкшие payload, metadata/audit/order snapshots сохраняются. Для полного реального удаления потребуется отдельная утверждённая политика. Наблюдаемость: status endpoint counters, delivery statuses/attempts и channel_events; generic worker error logs без payload/secret. Нет внешней телеметрии.
