# Локальный тест WhatsApp — PREPARED

Никакой Meta App/номер/токен не нужен. Не подключайте реальные credentials.

1. Прочитайте LOCAL_DEVELOPMENT и выполните явную миграцию up до00010. Старые миграции не редактируются. API/worker обновляются вместе.
2. Локально создайте отдельный случайный32-byte hex WHATSAPP_ENCRYPTION_KEY в ignored .env с правами0600. Не публикуйте его в чате/Git/логах. Передайте одинаковый ключ API/worker; LLM и WhatsApp budgets оставьте0, live flags false.
3. Используйте отдельный test org и вымышленные товары/остатки. Откройте /dashboard/integrations → WhatsApp.
4. Создайте синтетическую конфигурацию с вымышленными ID и wa_id700. UI использует заведомо недействительные mock credentials и Graph version v0.0. Настоящий Meta токен backend не принимает.
5. Отдельно включите mock-отправку. Подайте synthetic message с уникальным message_id. Backend строит подписанный fixture, записывает inbox; worker вызывает общий mock agent.
6. Найдите предложение в доставках. Подайте interactive `offer:variantUUID`, затем **только после просмотра предложения** `confirm:quoteUUID`. Заявка pending_confirmation появляется в кабинете; резерва ещё нет.
7. Staff подтверждает заявку отдельным действием. Это меняет только synthetic stock выбранного test org. Handoff останавливает automation; ответ сотрудника допускается только внутри window.
8. Выключите sending или отключите integration. UNKNOWN разрешайте только sent/cancelled без повтора. Подробности MULTICHANNEL_DELIVERY.

Offline tests: см. воспроизводимые команды STAGE_8B2_REPORT и infrastructure/compose.multichannel.yml. Они используют локальные образы/cache, internal network, никакого install/pull/build с загрузкой зависимостей.

## REQUIRES APPROVAL — не выполнено

- Создание Meta App и выбор владельца/permissions.
- Подключение WhatsApp Business Account.
- Получение/хранение настоящих access tokens/App Secret/verify token, механизм ротации.
- Выбор актуальной Graph API version.
- Регистрация/верификация тестового номера и test accounts allowlist.
- Выбор/проверка публичного HTTPS endpoint, настройка/подписка webhook.
- Проверка действующих customer service window, opt-in, approved template categories и тарифов по официальным источникам.
- Установка отдельного денежного бюджета WhatsApp, реализация live transport и финансового gate.
- Отправка тестовых сообщений, templates и проверка реальных delivery receipts.

Разрешение WhatsApp не разрешает Telegram, LLM, production или обработку реальных данных. Автоматические туннели и настройки Meta отсутствуют.
