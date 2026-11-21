# Stage 2.6.2 — Instagram Delivery Safety Gate

## Решение

| Критерий | Статус | Основание |
|---|---|---|
| Safety Block | PASS | Три синтетических подключения в состоянии `connected` сохраняют запрет отправки. Outbox и аудит фиксируют `cancelled` для обычных ответов. Подтверждение и отмена Auto Service фиксируют `instagram_send_disabled` без создания исходящих заданий. |
| Offline Delivery | BLOCKED | Изолированный `OfflineTransport` позволяет проверить HTTP-контракт и классификацию ответа провайдера. Между очередью и этим транспортом отсутствует рабочий маршрут с доверенным получателем, согласием клиента и актуальной версией записи. |
| Live Delivery | BLOCKED | Действует локальный запрет. Внешние условия Meta и разрешение на живую проверку остаются неподтверждёнными. |

Baseline зафиксирован в [Final Delivery](STAGE_2_6_2_FINAL_DELIVERY_REPORT.md) и [его журнале](../.local/stage-2-6-2-final-delivery/INDEX.md).

## Причина и классификация

| Причина | Классификация | Evidence |
|---|---|---|
| Feature flag и конфигурация | Ожидаемый защитный запрет | `INSTAGRAM_ENABLED` разрешает синтетический runtime вне production. `LoadInstagram` отклоняет отключение `INSTAGRAM_SENDING_KILL_SWITCH`, разрешение внешних запросов, live-режим, ненулевой бюджет и токен из env. [Конфигурация](../apps/api/internal/config/instagram.go) |
| Успешное подключение | Ожидаемый защитный запрет | Синтетический OAuth переводит подключение в активное при наличии scopes. SQL оставляет `sending_enabled=false`, а API возвращает этот флаг даже для `connected`. [Миграция 00017](../apps/api/migrations/00017_instagram_durable.sql), [Instagram Service](../apps/api/internal/instagram/service.go) |
| Durable очередь и маршрутизация | Ожидаемый защитный запрет и незавершённая реализация | Общий worker вызывает `Instagram.SendOne`, который возвращает `false, nil`. Обычный Instagram outbox получает финальный `cancelled`; `channel_claim_send`, `channel_admit_send` и credentials для исходящих закреплены за WhatsApp. Для квитанции Auto Service миграция 00027 сохраняет `instagram_send_disabled` до создания исходящего сообщения. [Worker](../apps/api/cmd/worker/main.go), [Instagram worker](../apps/api/internal/instagram/worker.go), [миграция 00017](../apps/api/migrations/00017_instagram_durable.sql), [миграция 00027](../apps/api/migrations/00027_booking_notifications.sql) |
| Исходящий адаптер, получатель и согласие | Незавершённая реализация | `OfflineTransport` принимает только `mock:` и вызывает внедрённую функцию. `Policy.Check` проверяет состояние подключения, permissions, webhook, срок токена, доступность получателя, неизвестный результат и локальное окно 24 часа. Переданные в тест поля подтверждают контракт; для durable отправки нужен доверенный источник этих полей, tenant-привязка получателя и проверка согласия на уведомления о записи. [Адаптер](../apps/api/internal/instagram/transport.go), [политики](../apps/api/internal/instagram/policy.go), [проверка согласия](../apps/api/migrations/00027_booking_notifications.sql) |
| Credentials и подключение Meta | Внешняя зависимость | Callback принимает синтетический код и хранит зашифрованный синтетический токен. Реальному подключению понадобятся Instagram User access token и учетная запись Professional. Фактическое состояние чужого Meta Dashboard неизвестно. [OAuth в репозитории](../apps/api/internal/instagram/service.go), [Meta Business Login](https://developers.facebook.com/documentation/instagram-platform/instagram-api-with-instagram-login/business-login) |
| Permissions и доступ приложения | Внешняя зависимость | Тестовые scopes `instagram_business_basic` и `instagram_business_manage_messages` проверяются backend. Выдача scopes Meta и уровень доступа приложения проверяются отдельно. Для сторонних Professional accounts Meta указывает Advanced Access; для собственных или добавленных к приложению аккаунтов применим Standard Access с оговорками по возможностям. [Meta Send Messages](https://developers.facebook.com/documentation/instagram-platform/instagram-api-with-instagram-login/messaging-api) |
| Политика переписки | Локальный запрет и внешнее требование | В репозитории применяется окно 24 часа от предыдущего входящего сообщения, исключения выключены. Для живой проверки нужно подтвердить действующие условия Meta для конкретной точки входа в диалог. [Политики приложения](../apps/api/internal/instagram/policy.go), [Meta Send Messages](https://developers.facebook.com/documentation/instagram-platform/instagram-api-with-instagram-login/messaging-api), [Meta Messaging Changelog](https://developers.facebook.com/documentation/business-messaging/instagram-messaging/changelog/) |

Непосредственная причина отказа Auto Service воспроизводится локально до обращения к провайдеру. Отдельной ошибки выбора канала не выявлено. Контракт transport-функции и отсутствие durable dispatch дают разные результаты проверки.

## Изменения и проверки

Добавлены проверки в `apps/api/internal/config/instagram_test.go`, `apps/api/internal/instagram/instagram_test.go`, `apps/api/tests/integration/instagram_test.go` и `apps/api/tests/integration/booking_notification_test.go`. Изменения касаются тестовых проверок и отчёта. Live-флаги, защита отправки, согласие, миграции и конфигурация production сохранены.

| Проверка | Статус | Граница результата |
|---|---|---|
| Offline HTTP-контракт | PASS | Подтверждение с правильным получателем, ответ с неправильным получателем, временный отказ `429`, отказ при отсутствии `Retry-After`, неопределённые ответы и таймаут. |
| Отказ транспорта до вызова fixture | PASS | 11 условий, включая неподходящие permissions, отключённый канал, просроченный токен, закрытое окно, чужое подключение, неверный чат и `DELIVERY_UNKNOWN`. |
| Конфигурация | PASS | Синтетическое подключение принимается в тестовом режиме. Попытка отключить защиту отправки с валидной mock-конфигурацией отклоняется. Go test, race, vet и сборка worker завершились с exit 0. |
| Durable запрет и tenant isolation | PASS | В PostgreSQL проверены связь строки outbox с tenant, подключением, получателем и диалогом, отсутствие попыток и provider ID, наличие `cancelled` в аудите и невидимость чужих строк через RLS. |
| Auto Service, PostgreSQL suite | PASS | Подтверждение и последующая отмена записали два результата `instagram_send_disabled`, исходящих заданий в диалоге — ноль. Полная backend-регрессия завершилась с exit 0. |
| Реальный outbound retry, lease recovery, opt-in и отсечение устаревшей записи перед вызовом транспорта | BLOCKED | У Instagram отсутствует рабочий durable send path. Поведение входящей очереди и классификация тестового ответа проверяются отдельно. |
| Повтор Chromium | NOT RUN | Изменения ограничены backend-тестами и доказательствами. |

## Условия последующей live-проверки

1. Завершить durable dispatch в существующей очереди. Получатель должен происходить из проверенного входящего события и сохранять связь с tenant, аккаунтом, подключением и диалогом. Проверить opt-in и его отзыв, окно переписки и версию заказа непосредственно перед отправкой. Подтвердить идемпотентность, retry после подтверждённого отказа, fencing для `DELIVERY_UNKNOWN`, отмену, восстановление lease и аудит в изолированной среде.
2. Подготовить Meta App с Instagram Login, согласованный Professional Business/Creator test account и тестовые роли, получить необходимые scopes и подтвердить уровень доступа. Проверить необходимость App Review и Business Verification для выбранного приложения и сценария. [Meta Send Messages](https://developers.facebook.com/documentation/instagram-platform/instagram-api-with-instagram-login/messaging-api), [локальный чеклист](INSTAGRAM_META_APP_REVIEW_CHECKLIST.md)
3. Зарегистрировать точный HTTPS OAuth redirect и webhook, подтвердить проверку подлинности, нужные подписки и реальное входящее сообщение от тестового получателя. Проверить Instagram-scoped ID, действительность и отзыв токена, серверное хранение credentials и правила окна для выбранной точки входа. [Meta Business Login](https://developers.facebook.com/documentation/instagram-platform/instagram-api-with-instagram-login/business-login), [Meta Send Messages](https://developers.facebook.com/documentation/instagram-platform/instagram-api-with-instagram-login/messaging-api)
4. Получить отдельное разрешение на ограниченную проверку в тестовом аккаунте Meta и задокументировать результат. Реальный OAuth, отправка, изменения Dashboard, credentials и deployment в этом прогоне имеют статус **NOT RUN**.

## Evidence

[Команды, результаты и журналы](../.local/stage-2-6-2-instagram-gate/INDEX.md).
