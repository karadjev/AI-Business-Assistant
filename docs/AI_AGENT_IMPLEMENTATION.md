# Реализация агента — этап 5

Использован только **mock**. Реальный LLM и платный API не вызывались. `internal/agent.Provider` имеет Generate(ctx, messages/tools/limits) и Mode; Mock —детерминированный сценарий поиска SKU, не настоящий интеллект. Compatible —адаптер Chat Completions function tools. Он тестируется на локальном httptest HTTP-сервере; real compatibility выбранной модели/аккаунта/доступности не подтверждена.

Реальный endpoint фиксирован в trusted constructor: https://api.openai.com/v1/chat/completions, без redirect и URL от пользователя/LLM. ENV не включает пользовательский endpoint. Модель live по умолчанию не выбрана; требуется явно совместимая с Chat Completions tool calling модель. Формат сверён через навык OpenAI Docs с [официальным function calling guide](https://developers.openai.com/api/docs/guides/function-calling) и [Create chat completion reference](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create). Это узкий адаптер, не обещание поддержки всех OpenAI моделей: для моделей Responses-only потребуется другой adapter. Реальный запуск остаётся запрещён без отдельного согласования.

## Конфигурация и лимиты

| ENV | Default | Граница |
|---|---|---|
| LLM_PROVIDER | mock | mock / openai_compatible |
| LLM_ENABLE_LIVE | false | Explicit opt-in, не разрешение оплат из этого задания |
| LLM_MODEL | empty для live; deterministic-test для mock | Для live обязательно |
| LLM_API_KEY | empty | Серверный secret, никогда NEXT_PUBLIC/API/log |
| LLM_TIMEOUT | 2s | 1ms–3s, общий модельный цикл |
| LLM_MAX_TOOL_CALLS | 4 | 1–8; весь цикл |
| LLM_INPUT_TOKENS | 16000 | 1–32000; input UTF-8 bytes ≤cap как консервативный верхний token bound |
| LLM_OUTPUT_TOKENS | 512 | 1–2048, max_completion_tokens |
| LLM_DAILY_REQUESTS | 40 | 1–1000, org/day |
| LLM_DAILY_TOKENS | 2000000 | 1–10000000, org/day |
| INVENTORY_FRESHNESS | 24h | 1h–168h |

Максимум 2 Generate за cycle, input schema/history/output bounded, history≤10 messages, message≤2000 chars, tool args≤4096bytes, result≤32768bytes, provider body≤65536bytes. Parallel tools disabled; операции выполняются последовательным allowlist runner. Stage5 HTTP quota120/min/user, chat≤100 inbound messages, ≤20 open chats/org. Provider errors, invalid tools/args, loop/timeouts/limits →human safe fallback. Нет автоматического повторного paid запроса или записи.

`ai_usage` —tenant FORCE RLS aggregate. До network атомарно резервируется 2*(input+output) tokens и request slot. Успешный known usage заменяет reserve фактическим token usage; unknown/timeout/error сохраняет reserve консервативно, не считает неизвестный расход нулём. Day определяется записью reservation, не временем завершения. Mock usage синтетический и явно тестовый. Это token/request budget, не валютный счёт/проверенная стоимость: прайс, monetary budget, дополнительные provider charges и reconciliation должны быть согласованы перед paid/live. Unknown reservations автоматически не освобождаются в течение дня; fail-closed, не скрытый retry.

## Инструменты

Все JSON schemas type object, additionalProperties=false, required properties; typed Go validation повторяет ограничения. Backend Tenant содержит только проверенную session и выбранный org; model-visible arguments не содержат organization/user/URL/SQL. Runner повторно проверяет staff/test/membership, catalog reads —через CustomerCatalog и RLS, active-only.

| Tool | Вход | Выход/доступ |
|---|---|---|
| search_products | query≤200, limit1–3 | ≤10 verified variant cards; только active выбранного магазина |
| get_product_details | product_id UUID | bounded cards: name/SKU/attributes/price/freshness |
| check_inventory | variant_id UUID, quantity1–10 | fresh flag, manual_updated_at, available/requested_available; stale→null |
| get_product_price | variant_id UUID | price_amount RUB, version, updated_at |
| create_order_request | quote_id UUID | Заявка; только trusted backend Consent exact quote/chat, никогда из LLM args |
| get_order_status | order_id UUID | Только order связанного chat/customer: id/status/test/reserved, без staff notes |
| handoff_to_human | reason enum customer_request/unknown_data/provider_error | human mode, автоответы прекращаются |

Quote подготовка —controlled authenticated endpoint, не привилегия модели. Consent endpoint вызывает тот же create_order_request runner с trusted proof после explicit accepted; без proof инструмент отклоняется. Ни поиск, ни «хочу купить», ни обычное «да» не создают заявок. В UI нужны выбор варианта/количества, новая server quote, checkbox и подтверждение.

## Prompt injection и ответы

Модель не является authority. Инструкции платформы неизменяемы для пользовательского текста/каталога. SQL/файлов/HTTP tools нет; schema запрещает попытки добавить organization_id/url. Не передаются secrets, owner email, session hashes или staff internal notes. Email/phone redaction перед моделью —дополнительная эвристика, не гарантия обезличивания произвольного текста. В тестовом контуре реальные ПД запрещены.

Свободный provider message content вообще не попадает в UI. Model предлагает tools; backend строит безопасный текст и карточки из успешных controlled reads. Имена/attributes считаются данными, React экранирует HTML. Никакого dangerouslySetInnerHTML/исполнения инструкции из product name. Наличие fresh —«по данным магазина», stale/unknown —«требует подтверждения». Резерв обещается только по confirmed order state. Mock маркируется в state и каждом assistant reply.

Model generation работает вне DB transaction. Conversation сохраняет version + run UUID + lease; при finalization проверяются version, mode, status и active run. Handoff/resume/delete меняет version; поздний reply старого run отбрасывается. При отмене HTTP запроса bounded independent finalization позволяет safe handoff, но невозможность записать после DB outage не скрывается: сервер возвращает ошибку, lease истекает, сотрудник может handoff вручную. Нет durable AI queue/system principal пока; worker продолжает только dev email, не копирует owner sessions в задания.

Проверки: mock/protocol/local HTTP errors/timeouts, schema spoof, excess calls/loop, forbidden consent, real RLS, token/request guards и racing handoff. Это контрольные случаи, не полный red-team/audit и не оценка качества live модели. Дальше требуются выбранная модель, privacy/retention договоры, monetary caps, live evals и отдельный job principal перед Telegram worker.

## Этап6

Existing Service.Send/Agent.Run вызывается Telegram inbox worker от leased customer capability (не employee session). Mock provider обязателен; tools сохраняют typed authorization/active-only facts/limits. Ответ в Telegram plain text строится из trusted cards, new engine/free-text claims не добавлены. Callback quote/consent использует прежний Orders service; staff не подтверждает от имени customer. Human mode suppress auto replies. TELEGRAM_INTEGRATION и MESSAGE_DELIVERY описывают durable channel queue; worker больше не только dev email.

## Дополнение этапа7

electronics-v1, compatible guarded adapter и Metered ledger; активен mock, live не согласован, provider NOT SELECTED и monetary0. LLM_MAX_TOOL_CALLS теперь1–3 (default3); старое значение4 заменить на3. Диалог20k input/5k output/30attempts, bounded two iterations. Owner auto off аннулирует in-flight version; resume human→agent только явно. Budget/SSRF gates и 30scenarios: LIVE_LLM_INTEGRATION, LLM_COST_CONTROL, AI_EVALUATION. Новые PG проверки PASS, live NLP NOT RUN.
