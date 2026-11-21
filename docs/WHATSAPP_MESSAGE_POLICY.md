> 9А: добавлен Cloud API HTTP wire transport с отдельным template serializer и fail-closed SendAdmission. Runtime остаётся mock-only, бюджеты0; live financial admission/approved registry ещё не подключены. Подробности: [WHATSAPP_CLOUD_API_TRANSPORT](WHATSAPP_CLOUD_API_TRANSPORT.md), [9Б readiness](STAGE_9B_READINESS.md). Исторические ограничения ниже сохранены.

# Политика сообщений WhatsApp

Этот этап не проверяет актуальные правила Meta по сети. Все утверждения о live window/templates/categories/pricing должны быть заново сверены с официальной документацией перед live. Пока действует локальная консервативная policy.

| Условие | Решение сервера |
|---|---|
| Не test org/нет active integration/нет allowlist | отказ |
| Live или неизвестная стоимость/нет разрешения | отказ; live transport отсутствует |
| Mock, sending выключен | сообщения не отправляются |
| Внутри configured service window | разрешена только бесплатная mock-операция |
| Окно закрыто | NEEDS_APPROVED_TEMPLATE; свободный текст не отправляется |
| Нужен approved template | блокировка; template sender отсутствует |
| Handoff | автоматические ответы прекращаются; staff только по policy |
| Неизвестная доставка | ручное разрешение; не retry |

Окно считается от timestamp входящего сообщения, ограниченного received_at (future timestamp не продлевает окно). Дефолт24h, owner может выбрать более короткое окно при connect. Повтор/status event не продлевает window. Для late/out-of-order inbound используется max старого окна и безопасного timestamp; давний replay не делает окно свежим.

Свободный staff ответ вне окна получает409 NEEDS_APPROVED_TEMPLATE. SQL trigger независимо блокирует такой INSERT. Outbox admission повторяет проверку непосредственно перед mock send. Автоматическое предложение заменяющего платного шаблона отсутствует.

`sent → delivered → read` не регрессирует от позднего sent/failed. Signed receipts сопоставляются только по integration+recipient+provider message ID; сообщения других организаций не обновляются. Для неизвестного provider ID событие сохраняется/завершается без угадывания, какое исходящее сообщение ему соответствует.

WhatsApp monetary budget0 RUB отделён от LLM0 RUB. В БД CHECK budget=0; API не позволяет его повысить. Mock не считается расходом и не даёт разрешения live. Будущие категории шаблонов, opt-in, биллинг, тарифы, platform/org monetary reservations требуют отдельного задания и актуальной официальной проверки. Не использовать этот документ как подтверждение бесплатности реальных сообщений.
