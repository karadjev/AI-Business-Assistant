# Готовность к 9Б

Telegram live: **NOT READY**. WhatsApp live: **NOT READY**. Offline wire transport не означает одобренную реальную интеграцию.

Перед отдельным контролируемым live этапом нужны:

1. Отдельное разрешение пользователя на точный API scope, test accounts, HTTPS endpoint и synthetic participant allowlist. Разрешения на вызовы в 9А нет.
2. Проверка текущих официальных Meta docs: поддерживаемая Graph version, messages/templates/error contract, service window, approval/opt-in, categories и тарификация. Версия сейчас не выбрана; тестовый v99.0 не применять.
3. Реальный server-side durable WhatsApp SendAdmission: атомарные org/platform reservations, known cost, idempotency, unknown outcome, recovery/audit; положительные лимиты только после отдельного согласования. Пока бюджеты0 и runtime mock-only. Interface/positive synthetic unit fixtures не заменяют ledger.
4. Реестр подтверждённых approved templates/языков/параметров и безопасная template outbox модель, если шаблоны потребуются. Сейчас есть wire serializer, но нет публичной отправки шаблонов или их live approval.
5. Owner-only real credential provisioning/rotation, live factory wiring и проверка ingress configuration gate. Существующий Connect специально запрещает real credentials, API/worker используют Mock. Не включать CloudTransport простым изменением environment.
6. Политика персональных данных, актуальная правовая/географическая проверка, retention/deletion и operator rollback/monitoring procedures.
7. Разобраться с E01 LOW из 8Е либо отдельно явно оценить остаточный риск. Невоспроизведение не равно CLOSED.
8. Только после указанных проверок — отдельные разрешённые get/webhook/send тесты. Никакие credentials не публиковать в чат/лог/Git; production и реальные клиенты не входят в текущий scope.

Автоматическое включение live, покупка шаблонов, Meta App/WABA регистрация, токены, публичные webhook, отправка и загрузка зависимостей в 9А не выполняются.
