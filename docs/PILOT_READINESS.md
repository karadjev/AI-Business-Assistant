# Готовность тестового пилота

**Реальный пилот не разрешён.** Provider NOT SELECTED; 0 RUB; external Telegram NOT AUTHORIZED; live LLM и sending выключены. Только синтетические товары, переписки и заявки отдельной test organization. Production запрещён.

## Подготовленный Telegram transport

Сохранён mock transport. Реальный transport фиксирован на HTTPS `api.telegram.org`, с безопасным HTTP client, без redirects и раскрытия токена в ошибке. Методы getMe, setWebhook, getWebhookInfo, deleteWebhook и answerCallbackQuery проверены на локальном HTTP fixture. Это не тест настоящего Telegram.

Защищённый owner endpoint: `POST /api/v1/organizations/{id}/integrations/telegram/live-operations`. Поля: `operation`, `confirm_external_request:true`, `token` только для `verify_token`. Сейчас любой вызов возвращает 403 без внешней сети. Открытие dashboard лишь читает БД и не проверяет соединение через Telegram.

Операции независимы: `verify_token`, `set_webhook`, `webhook_info`, `delete_webhook`. CSRF + owner + test org + live transport + external authorization + HTTPS dashboard обязательны. setWebhook дополнительно требует точного совпадения TELEGRAM_WEBHOOK_BASE_URL и TELEGRAM_APPROVED_WEBHOOK_BASE_URL; приложение не создаёт endpoint, DNS, сертификат или туннель самостоятельно.

## Что нужно согласовать заново

1. Provider/model, точный base URL allowlist, ключ, тарифы и положительный денежный лимит с периодом.
2. Отдельное разрешение на LLM внешние запросы; org limits и platform DB approval.
3. Тестовый Telegram-бот, токен, участники и allowlist числовых Telegram user ID с их согласием.
4. Отдельное разрешение на getMe, webhook info/setup/delete и тестовую отправку сообщений.
5. HTTPS origin кабинета и публичный webhook origin, способ размещения; никаких автоматических туннелей.
6. Политика данных, поставщики и инфраструктурные регионы; правовая проверка до реальных клиентов.
7. Проверка отчёта локальных PostgreSQL/RLS/миграционных и Docker/browser smoke tests, затем отдельное разрешение на live E2E.

Наличие конфигурации не является разрешением. Нельзя передавать ключи в чат, OpenAPI, frontend environment или обычные логи. Использовать защищённое локальное хранилище backend/секреты deployment только после согласования.

## Будущий ручной запуск после согласования

Локальные integration и Chromium smoke уже выполнены: PASS; перед разрешённым live запуском повторить на подготовленном тестовом окружении, проверить миграцию7, нулевые расходы и отсутствие живых клиентов. Создать test organization через кабинет и наполнить вымышленным каталогом; stock этой организации изолирован от магазина.

После разрешения изменить только игнорируемую server config: TELEGRAM_TRANSPORT=live, authorization=true, nonempty TELEGRAM_TEST_USER_IDS и 32-byte encryption key. Sending оставить false и kill switch true. Перезапустить API/worker. Вручную выполнить owner verify_token через защищённый BFF; токен не помещать в историю shell. Проверить bot metadata. Затем отдельно подтверждённый set_webhook и webhook_info. Secret генерирует backend, сохраняет encrypted + hash и не возвращает frontend.

Для отправки нужны одновременно разрешение, live mode, allowlist, TELEGRAM_SENDING_ENABLED=true, TELEGRAM_SENDING_KILL_SWITCH=false и owner org telegram_sending_enabled=true. Пользователь проверяется до сохранения ingress, при SQL claim до создания principal/conversation и перед отправкой. В mock с пустым allowlist допускаются синтетические IDs; это не разрешение живых пользователей.

LLM отдельно активируется только после согласования всех денежных gates; нельзя включать его для проверки Telegram автоматически. Лимиты и ручные операции сейчас заблокированы.

## Остановка и отзыв

Owner выключает auto replies в AI settings: in-flight run аннулируется, новые сообщения передаются сотруднику. Для аварийной остановки Telegram sending установить kill=true и sending=false, перезапустить worker/API; отключить org sending. Глобальный LLM DB kill switch можно включить оператором, затем отключить live env и перезапустить. Начатый HTTP невозможно гарантированно отозвать; дождаться таймаута, сверить outbox/unknown budget. Авто-retry uncertain delivery отсутствует.

Отключить интеграцию в кабинете. Удаление webhook требует отдельно разрешённой внешней операции delete_webhook; локальное disconnect само не вызывает Telegram. Токен отозвать вручную у BotFather после согласования. Отзыв токена и Telegram удаление сообщений приложение не имитирует.

## Не закрытые риски

PG migration 7, RLS и concurrency проверены на временной aba_test: PASS. Docker доступен через разрешённые команды Compose; запуск через shell wrapper ограничен sandbox. Natural-language live quality NOT RUN. setWebhook не атомарен с БД: candidate secret сохраняется заранее, при ошибке webhook disabled; нужна ручная сверка и повторная установка после разрешения. Потерянное подтверждение отправки может привести к неопределённой доставке; exactly-once Telegram не обещается. answerCallbackQuery подготовлен как transport method, но отдельное надёжное callback acknowledgement в worker пока не подключено — при live кнопке возможен spinner до timeout. Pending cost reservations требуют ручной сверки после падения процесса.

Интернет API справочник: [Telegram Bot API](https://core.telegram.org/bots/api#setwebhook).
