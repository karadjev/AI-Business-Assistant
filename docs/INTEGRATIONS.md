# Интеграции и доставка сообщений

Проверка открытых первичных источников: 12.10.2026. Возможности провайдеров меняются; перед реализацией повторно проверить доступность из РФ, права приложения, актуальную версию API и тарифы. Ни одна интеграция сейчас не подключена.

## Общий интерфейс на Go

Adapter: VerifyWebhook(rawBody,headers), Normalize(event), SendMessage(destination,content), ValidateCredentials, Capabilities. Нормализованный envelope: provider, integration_id, provider_event_id, external_chat_id, external_user_id, message_id, occurred_at, received_at, kind, text/callback. Organization определяется сервером по интеграции и verified provider account; tenant из payload не принимается. Customer identity относится к integration и tenant; не доверять display_name как идентификатору.

Ingress ограничивает body size/timeout и проверяет подлинность до JSON parsing/персистирования пользовательских данных. Транзакция сохраняет inbound_event и job с UNIQUE key; повтор возвращает 200 без второй job. ACK только после COMMIT, при недоступной БД —5xx. Не ждать LLM в webhook. Retention сырого payload короткий и encrypted; для отладки очищенные metadata.

Очередь PostgreSQL, не Redis Pub/Sub: leased jobs, exponential backoff+jitter, отдельный failed state после предложенных 5 попыток. 429 учитывает Retry-After, auth failure приостанавливает канал и уведомляет owner, invalid recipient — без бесконечного retry. После восстановления оператор может повторить failed task с тем же logical key. Таймаут запроса к провайдеру может означать, что сообщение уже отправлено: delivery=uncertain; не обещать exactly-once и не отправлять автоматически бесконечные копии. У Telegram нет общего гарантированного idempotency key для sendMessage; uncertain требует осторожного ручного решения.

Inbox, создание заявки и notification защищены DB uniqueness. Outbound фиксируется до отправки; ответ provider_message_id сохраняется после неё. Crash между отправкой и сохранением создаёт неопределённость, полностью исключить дубли внешней доставки нельзя.

Порядок: один run на conversation; локальный sequence выделяется под lock. Provider occurred_at не гарантирует порядок доставки. Worker обрабатывает сохранённые события последовательно, краткое окно сборки предложено 1 с для burst; update_id Telegram помогает отсечь дубли, но не ждать отсутствующие IDs (они общие для бота). Запоздалое сообщение сохраняется, при неоднозначном согласии новая quote/уточнение. Callback проверяется независимо, по binding и сроку.

## Telegram — рекомендуемый MVP

Собственный бот каждого магазина, созданный владельцем через BotFather; credentials проверяются getMe, provider_account_id=bot ID. Пользователь должен первым начать диалог с ботом, произвольным пользователям бот написать не может. Только приватные текстовые сообщения и callback buttons; groups, business-account delegation, медиа и темы вне MVP.

HTTPS webhook с отдельным случайным secret_token; сравнение X-Telegram-Bot-Api-Secret-Token constant-time. Это общий секрет в заголовке, не подпись тела. Случайный public_id в URL — только routing, не защита. Список allowed_updates ограничен message/callback_query; токен бота хранится encrypted и не логируется, включая URL Telegram API. Webhook и getUpdates нельзя использовать одновременно. При настройке не использовать drop_pending_updates без отдельного решения, чтобы не потерять обращения.

[Telegram Bot API](https://core.telegram.org/bots/api) и [официальное руководство webhook](https://core.telegram.org/bots/webhooks) описывают настройку и секрет. Дополнительные ограничения отправки и обязательность начала диалога: [FAQ Telegram](https://core.telegram.org/bots/faq). Точные rate limits не зашиваются навечно: глобальный и per-chat limiter плюс реакция на 429/retry_after. Длинный ответ разделяется по актуальному лимиту API; MVP шаблоны короткие.

Дедупликация update_id на интеграцию, callback дополнительно по callback_query.id/confirmation. Проверяется callback.from.id, chat и binding server-side. Callback data — короткий opaque token; цена/tenant/согласие не доверяются строке клиента. AnswerCallbackQuery отправляется быстро, независимо от оформления заявки, но «заявка создана» показывается только после commit.

Handoff: статус conversation human, уведомление в кабинете. Оператор отправляет сообщение через тот же бот, а не через свой личный аккаунт. Для первой версии уведомления внутри кабинета, внешний Telegram alert владельцу — позже с явной настройкой адресата.

## WhatsApp Business Platform — следующий этап

Только официальная Cloud API или разрешённый BSP, не автоматизация личного WhatsApp/QR-сессии. Требуются бизнес-аккаунт/номер, токены и разрешения; onboarding чужих бизнесов через SaaS может потребовать App Review, business verification и подходящие права. Доступность продукта, платежей и номера для российского бизнеса отдельно проверяется до включения в roadmap; успешный sandbox не доказывает production approval.

GET verification: проверить verify_token и вернуть hub.challenge. POST: HMAC-SHA256 исходных raw bytes с app secret, constant-time compare X-Hub-Signature-256; проверить business/phone account mapping. Официальный пример Meta: [проверка подписи](https://github.com/fbsamples/whatsapp-api-examples/blob/main/signature-validation-with-webhooks-payloads/app.py). Verify token не заменяет проверку POST подписи.

[Политика WhatsApp Business](https://business.whatsapp.com/policy) регулирует ответы: свободные ответы внутри 24-часового окна после последнего сообщения пользователя; вне окна — разрешённые templates с учётом правил согласия/opt-in. Автоматизация должна предоставлять понятный путь к человеку. Ограничения действуют и для сотрудника. Хранить last_customer_message_at, проверять окно перед каждой отправкой; approved template IDs связаны с аккаунтом. Не считать все шаблоны, категории или сценарии автоматически одобренными; цены и quality/messaging limits зависят от правил провайдера.

Дедупликация входящих по message ID; статусы sent/delivered/read/failed — самостоятельные события (message ID+status+provider timestamp), не новые сообщения покупателя. Порядок статусов может нарушаться; не понижать delivered до sent. Официальные страницы [overview](https://developers.facebook.com/documentation/business-messaging/whatsapp/overview) и [webhooks](https://developers.facebook.com/documentation/business-messaging/whatsapp/webhooks/overview) при текущей проверке не удалось прочитать инструментом; полная проверка текущего onboarding и прав остаётся задачей следующего этапа, не предполагается выполненной.

## Внутренний веб-чат — позже

Виджет получает короткоживущую подписанную session, связанную с tenant/customer/conversation, не общий service token. Проверка origin allowlist магазина, CSRF/session binding, rate limit и антиспам. CORS не является авторизацией. Анонимный посетитель видит только собственный чат; потеря session не даёт восстановить историю по одному conversation ID. Начать с HTTP POST + polling, SSE после необходимости. Ручной перехват использует общий Conversations service. При интеграции на внешний сайт учитывать cookies/third-party ограничения; не рассчитывать на доступность third-party cookies.
