# Восстановление доставки Telegram — 8Б.1

18.10.2026. Только offline/mock. Настоящие Telegram API запрещены. Этот документ не разрешает отправку сообщений или live-тест.

## Модель

`platform.telegram_outbox.id` — уникальный внутренний UUID сообщения; `(integration_id,message_key)` предотвращает повторное enqueue. UUID не передаёт Telegram универсальный idempotency key: **exactly-once доставка не гарантируется**.

`pending → sending → sent / pending / failed / uncertain`. `uncertain` соответствует **DELIVERY_UNKNOWN**: сообщение могло дойти, но достоверного receipt нет. Это терминальная остановка автоматической отправки этого задания; последующие сообщения того же chat блокируются до решения сотрудника. Новые updates сохраняются идемпотентно, но не превращают unknown в разрешение на повтор.

- Claim: SQL `telegram_claim_send`, `FOR UPDATE SKIP LOCKED`, capability UUID, lease15s, максимум5 попыток. Порядок — durable arrival, не будущая числовая сортировка update_id.
- Admission: `telegram_prepare_send` проверяет актуальные active integration, mode, test org, principal/allowlist, conversation/handoff и sending gate. `send_guard.go` удерживает session advisory lock на закреплённом соединении; SQL-транзакция во время HTTP не открыта. Finish выполняется на том же соединении, lock освобождается или соединение уничтожается.
- Restart: истёкший sending переводится в uncertain, а не pending. Capability отзывается. Даже если Telegram успел доставить сообщение, новый worker не делает blind resend.
- 429: ограниченный backoff с retry_after (1…3600s), attempts≤5. Для sendMessage retry признаётся только при HTTP429 и error_code429. Противоречивый HTTP500/error_code429 — uncertain.
- HTTP5xx, timeout, EOF/разрыв, неверный/отсутствующий receipt: uncertain без автоматического повтора. Timeout не доказывает отсутствие доставки.
- Постоянный отказ — failed; disconnect — cancelled для pending/sending. Handoff отменяет queued assistant, исключая доверенное server notice. Уже допущенная отправка может завершиться до возврата disconnect; отправленное отозвать нельзя.

Миграция `00009_reliability.sql` добавляет `outbox.version` и `telegram_delivery_events`: внутренний message ID, номер попытки, старый/новый статус, версия, время, а для ручного решения — actor_id, decision и request_key. Начало попытки фиксируется **до** внешнего HTTP. Audit не содержит токена, ciphertext, capability или полного текста переписки. В runtime нет INSERT/UPDATE/DELETE grants на журнал; writes выполняет trigger/проверенная definer function.

## Ручное разрешение

Разрешено owner или member с явным `can_manage_orders=true`, в test organization, через действующую сессию и CSRF. Обычный member не получает этих прав. Browser org UUID не источник авторизации.

`GET /api/v1/organizations/{id}/integrations/telegram/deliveries` — первые50 uncertain/failed, с приоритетом actionable UNKNOWN, их версии и история попыток. Это очередь инцидентов, не весь архив всех sent сообщений.

`POST /api/v1/organizations/{id}/integrations/telegram/deliveries/{resource}/resolve`:

```json
{"expected_version":7,"decision":"abandon","idempotency_key":"11111111-1111-4111-8111-111111111111","confirm_resolution":true}
```

Возможны только:

- `delivered`: сотрудник **независимо проверил** доставку и делает локальную отметку sent. Приложение не обращается к Telegram для такой проверки и не выдумывает telegram_message_id. Отметка не является техническим receipt.
- `abandon`: отказаться от дальнейшей доставки, cancelled. Покупатель мог уже видеть сообщение; никакого повторного сообщения не создаётся.

**Resend не реализован намеренно.** Если оператор не знает исход, безопасный вариант — не делать повтор, передать разговор человеку и выбрать abandon после оценки контекста. Любая будущая возможность resend требует отдельного дизайна и явного согласия на риск дубля.

SQL `telegram_resolve_delivery` берёт тот же integration fence, блокирует строку, проверяет state/version и записывает результат+actor audit атомарно. Повтор того же key/decision возвращает текущий результат; смена смысла key либо устаревшая версия —409. Два конкурентных одинаковых подтверждения дают одно действие/один operator event. Старый worker с отозванным capability не может перезаписать решение. Чужой/отсутствующий message —404; отсутствие сессии401; staff/CSRF отказ403; неверное действие/подтверждение422. Действие локальное, не выполняет getMe/sendMessage/deleteWebhook.

UI: `/dashboard/integrations/telegram`, блок «DELIVERY UNKNOWN · восстановление». Загрузка инициируется кнопкой, отображаются empty/error/loading, версия и audit. После решения инцидент исчезает из списка unresolved; metadata audit остаётся в БД до согласованного удаления test data. При409 обновить список; при timeout повторить с тем же API idempotency key либо проверить состояние, а не отправлять сообщение заново.

## Восстановление и остановка

1. Не сбрасывать uncertain вручную в pending. Проверить integration status, kill switch, очередь и actor permissions.
2. При инциденте отключить sending/autoanswers, дождаться завершения admitted send/timeout. Внешний kill не обещает отзыв уже принятого сообщения.
3. Staff просматривает переписку, независимо устанавливает известный исход или выбирает abandon; фиксирует решение через endpoint/UI.
4. Проверить, что один operator event записан и очередь разблокирована. Возврат от handoff к agent — отдельное явное действие.
5. Не включать live в текущем контуре. Удалять только synthetic test data через existing purge. Delivery events каскадно удаляются с их outbox при test erasure, поэтому это operational audit, а не бесконечный архив.

## Доказательства

- `telegram/recovery_test.go`: реальный локальный HTTP получает payload и закрывает соединение без receipt; transport uncertain и ровно1 запрос. HTTP500 с ложным429 body — RED до исправления, GREEN после. Timeout/5xx/429 также проверены existing HTTP tests.
- `integration/reliability_test.go`: дочерний процесс реально завершается после receipt до DB finish; новый процесс worker logic не повторяет отправку; concurrent workers дают один actual HTTP request; manual auth/CSRF/version/idempotency и stale worker fence.
- Integration stages5–8А: повторные updates/quote callbacks не создают duplicate order; inventory reserve только staff; handoff и disconnect.
- `scripts/test-recovery-components.cjs`: текущие TSX компоненты в Chromium, 403/409 и подтверждение; отдельно текущий BFF module с локальным fetch mock. Это не Next production build.

Реальная Telegram доставка/ошибки API/HTTPS/token rotation: NOT RUN. Проверенные локальные механизмы не доказывают внешнее exactly-once или production readiness.
