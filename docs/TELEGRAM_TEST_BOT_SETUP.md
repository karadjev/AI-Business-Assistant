# Тестовый Telegram-бот: подготовка и отдельная активация

Дата: 16.10.2026. **Сейчас только PREPARED, mock. Настоящий токен не предоставлен. Внешние запросы не разрешены.** Не выполнять нижеследующие внешние действия автоматически.

## PREPARED — без внешней сети

1. Работать в отдельной test organization (`is_test=true`), с синтетическим каталогом/остатками. Использовать отдельный Compose project и БД; не копировать реальных клиентов. Mock credential вида `mock:10001` не является Telegram-токеном.
2. Сохранить режимы: `LLM_PROVIDER=mock`, `LLM_ENABLE_LIVE=false`, `LLM_EXTERNAL_REQUESTS_AUTHORIZED=false`, `LLM_KILL_SWITCH=true`, `LLM_PLATFORM_BUDGET_MICRORUB=0`; `TELEGRAM_TRANSPORT=mock`, `TELEGRAM_EXTERNAL_REQUESTS_AUTHORIZED=false`, `TELEGRAM_SENDING_ENABLED=false`, `TELEGRAM_SENDING_KILL_SWITCH=true`. Production не включать.
3. Ключ `TELEGRAM_ENCRYPTION_KEY` — 32 случайных байта в hex, только server-side. Генерировать локально, хранить в `.env`/secret-file с mode 0600, который игнорируется Git. Не отправлять ключ в чат. Публичные синтетические ключи audit harness предназначены только для уничтожаемого offline-контейнера, не для будущего live.
4. `TELEGRAM_TEST_USER_IDS` — список положительных numeric user ID через запятую. Username ненадёжен. Подготовить ID только согласившихся тестировщиков; получение ID через сторонний сервис/бот также внешнее действие и сейчас не разрешено. Для private chat user/chat должны соответствовать проверкам adapter. Live без allowlist не стартует; при заданном allowlist mock также проверяет его.
5. Локальная проверка формата настроек, ролей и mock подключения не проверяет настоящий токен. `/dashboard/integrations/telegram` в mock принимает только mock credential. Статус «mock» нельзя трактовать как успешный getMe.
6. Offline harness: `infrastructure/compose.audit.yml`, merged `compose.audit-smoke.yml`; точные команды и preconditions в STAGE_8A_REPORT. Никаких автоматических туннелей, публичных портов или setWebhook.

## REQUIRES APPROVAL — ручное создание тестового бота

Эти шаги описаны для будущего оператора и **в этапе 8А не выполнялись**.

- В официальном Telegram открыть проверенный [@BotFather](https://t.me/BotFather). Проверить точное имя, не использовать посредников. Вручную создать нового бота командой `/newbot` после согласования внешнего теста. Название/username должны явно содержать TEST/тест; отдельный бот от магазина, без реальных клиентов. Отключить добавление в группы; MVP поддерживает private chat.
- Токен не публиковать в чате с Codex, GitHub, тикетах, скриншотах и terminal command arguments/history. В будущем передать его через password-input существующего owner-only UI **только по одобренному HTTPS**, либо через локальный секретный файл/ввод без echo в проверенный операторский инструмент. Не использовать `curl ...bot<TOKEN>`: URL попадёт в history/process list.
- Хранение интеграции шифрует token в БД; encryption key расположен вне БД. Не хранить токен в NEXT_PUBLIC/localStorage. Наличие синтаксически корректного токена не даёт права на вызов getMe.

## REQUIRES APPROVAL — endpoint и конкретные операции

До каждого шага согласовать scope: бот/organization, тестовые ID, срок, endpoint, допустимые сообщения, ответственное лицо и остановку. Разрешение Telegram не разрешает LLM; LLM остаётся mock/0 RUB.

1. **getMe**: отдельное разрешение на этот read запрос. Только после него серверный external gate/live mode может использоваться для проверки/подключения токена. Проверить возвращённые bot ID/name/username; один bot ID не должен принадлежать двум организациям. `connect` в live вызывает getMe и потому сейчас запрещён.
2. **HTTPS**: оператор выбирает принадлежащий ему тестовый домен, сертификат и TLS termination. Проверка DNS/certificate/public availability — отдельные будущие внешние действия, сейчас NOT RUN. Base URL должен совпасть с `TELEGRAM_APPROVED_WEBHOOK_BASE_URL`, быть public HTTPS origin без userinfo/query/fragment/IP; backend не должен быть опубликован целиком. Разрешить только нужный ingress route и установить лимиты. Туннель не запускать автоматически.
3. **setWebhook**: отдельное разрешение после устранения readiness blockers. Existing owner-only `/api/v1/organizations/{id}/integrations/telegram/live-operations`, CSRF, `operation=set_webhook`, `confirm_external_request=true`; backend сам строит endpoint с integration UUID и секретом. Не конструировать secret/token в браузерном URL. Не сбрасывать pending updates без отдельного решения.
4. **getWebhookInfo**: отдельно разрешённая операция `webhook_info`. Она обращается к Telegram, dashboard её автоматически не вызывает. Состояние БД не доказывает фактическую установку webhook.
5. **Сообщения**: отдельно разрешить test sending, проверить allowlist и организационный sending gate. Только затем `TELEGRAM_SENDING_ENABLED=true`, `TELEGRAM_SENDING_KILL_SWITCH=false` и организационное разрешение. Согласовать небольшой набор синтетических сообщений. Никаких настоящих покупок/клиентов. Handoff, inline consent, stock conflict и retry проверять только в test store.

Приём webhook требует заголовок `X-Telegram-Bot-Api-Secret-Token` и верную integration. В offline contract tests проверены неверный secret, allowlist, повторные события. Реальная проверка заголовка от Telegram NOT RUN. Не выводить secret для диагностики: сравнивать только безопасные статусы/хеши на сервере.

## Остановка и отзыв

- Немедленно отключить отправку: `TELEGRAM_SENDING_KILL_SWITCH=true`, `TELEGRAM_SENDING_ENABLED=false`, выключить организационную отправку/автоответы и перезапустить соответствующие процессы для применения env. Уже выполняющийся HTTP может завершиться; дождаться timeout/остановки worker. Платформенный LLM kill/budget остаются true/0.
- Owner `disconnect` — локальная транзакционная деактивация, очистка cipher и отмена queued/claimed работ. Она **не вызывает deleteWebhook**. Уже отправленное не отзывается.
- **deleteWebhook** требует отдельного разрешения, пока token ещё доступен; existing manual operation `delete_webhook` также требует CSRF/confirmation. По возможности выполнить согласованное удаление webhook перед локальным disconnect; при аварии сначала kill switch. Локальный статус disabled не является доказательством удаления webhook у Telegram.
- **Отозвать токен** вручную через официальный BotFather (`/revoke`) — внешнее действие, требующее отдельного разрешения. Не записывать новое значение в отчёт; после отзыва убрать локальные копии и закрыть доступ к key storage. Ротация ключа шифрования — отдельная операция с миграцией ciphertext, не простая замена env.
- Удалять только synthetic test conversations через owner-only `purge-test-data` с current version; pending операции блокируют purge. Связанные test orders/quotes/messages/principals/inbox/outbox удаляются согласованно, резерв освобождается; финансовые агрегаты/минимальный аудит сохраняются по описанной политике. Stale pending budget сначала требует A10 reconciliation. Не выполнять SQL DELETE произвольной базы.
- Изолированный audit Compose project можно удалить `down --volumes` только с точным заранее выбранным audit project name. Рабочий стек, volumes и данные пользователя не трогать.

Переход к тесту разрешён только после новой оценки [TELEGRAM_LIVE_READINESS_CHECKLIST.md](TELEGRAM_LIVE_READINESS_CHECKLIST.md), закрытия блокеров и явного operator approval. Подготовленная инструкция не является разрешением выполнять её live-шаги.
