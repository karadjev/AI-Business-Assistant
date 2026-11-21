# Instagram — аудит и архитектура

Дата: 2026-11-18. Статус: частичная реализация offline-контрактов; runtime не подключён.

## Изучено

- Telegram: `internal/telegram/{channel,service,worker,http_transport,send_guard}.go`, существующая схема `telegram_*` и маршрутизация HTTP.
- WhatsApp: `internal/whatsapp/{adapter,service,worker}.go`, financial admission, provisioning; `internal/http/whatsapp.go`.
- Общие границы: `internal/channel/{channel,engine}.go`, `conversations.Service`, `agent.Runner`, `tenancy.Begin`; общий worker вызывает `channel.Queue`.
- Схема: migrations 00010–00012; последняя migration на момент аудита — 00016. Очередь и capability SQL по-прежнему WhatsApp-specific.
- Frontend: BFF/session, канал settings, `ConversationList`, страницы integrations/conversations.
- Проверки: multichannel tenant/capability tests, frontend production fixtures, маршруты quality gates из `AI_DEV_SYSTEM_INSTALL_PLAN.md`.

## Краткий план

1. Отдельно проверить формат Meta, OAuth state, token lifecycle, подпись, нормализацию и policy на synthetic fixtures.
2. Расширить существующие channel repository/queue/capabilities следующей миграцией; сохранить FORCE RLS, составные FK и отправочные fences.
3. Подключить Instagram к существующему `channel.Engine` → conversations → agent/catalog/orders; не создавать второй AI или каталог.
4. Подключить OAuth/status/disconnect и единый inbox; подтвердить gates на трёх магазинах в изолированной БД.
5. Только после подтверждения кода и отдельного разрешения переходить к Meta/live.

## Что реализовано

`internal/instagram` содержит HMAC/challenge, нормализатор, HTTP handler contract, OAuth state interface, offline exchange/refresh, AES-256-GCM envelope, policy и mock Send API transport. Используются существующие `channel.Message`, attachments, delivery states и tenant tuple. Общий enum расширен значением `instagram`.

Нормализация сохраняет external mid, sender/chat ID, timestamps, reply mid и типы вложений; UUID correlation стабилен для tenant+integration+mid. Медиа считается требующим сотрудника. URLs не сохраняются и не загружаются. Неизвестные события игнорируются; повреждённые отдельные messaging entries учитываются как rejected, корректные соседи продолжают обработку. Ошибка общей структуры или чужой account отклоняет batch.

Webhook handler подтверждает только после успешного Enqueue; Enqueue является интерфейсом, а не реализованной очередью. Внутри нет AI. Дедупликация в одном batch реализована; durable дедупликация пока отсутствует. Replay age ограничен 24 часами с допуском будущего времени 5 минут; старые сообщения после длительного сбоя не восстанавливаются автоматически.

## Обязательное продолжение

Текущие `channel_integrations` и `channel_principals` имеют CHECK WhatsApp. `channel_enqueue`, `channel_claim`, order source, capability routing и финансовые triggers требуют адресного изменения. Нельзя просто разрешить Instagram в CHECK: это может выдать WhatsApp capabilities и отправочный путь другому провайдеру.

Следующая миграция должна обеспечить уникальный account mapping, encrypted credentials, permissions/subscription/observed-event state, атомарное OAuth state consumption, account ownership, provider-aware claim/outbox/admission, сохранение metadata и processing state. Все изменения SECURITY DEFINER требуют тестов RLS/role/lease/concurrency и rollback preservation. Эта миграция **не создана**, миграции 00001–00016 не изменены.

`channel.Engine` и `tenancy.Begin` намеренно ещё не разрешают Instagram. Нет зарегистрированных Instagram backend endpoints, фоновых задач, inbox filter или создания заказов из Instagram. Предлагаемые маршруты: tenant-scoped `integrations/instagram` status/start/disconnect, HTTPS OAuth callback и отдельный webhook; URI нужно зафиксировать при реализации, а не настраивать сейчас.

Frontend содержит карточку и страницу `/dashboard/integrations/instagram` с явным недоступным подключением. Статус «Подключён» нигде не имитируется.

## Официальные источники

Проверены непосредственно перед кодом 2026-11-18:

- [Instagram Login](https://developers.facebook.com/documentation/instagram-platform/instagram-api-with-instagram-login/business-login): модель OAuth, endpoints и refresh.
- [Instagram Login Messaging API](https://developers.facebook.com/documentation/instagram-platform/instagram-api-with-instagram-login/messaging-api): Instagram token/host, permissions, ограничения сообщений.
- [Instagram webhook reference](https://developers.facebook.com/documentation/business-messaging/instagram-messaging/webhooks): webhook reference; login-specific subscription lifecycle ещё нужно проверить перед runtime.

Некоторые первоначальные URL возвращали 429; login и messaging страницы удалось получить по актуальным canonical URLs. Конфигурация конкретного приложения Meta не проверялась. Facebook Login/Page tokens не реализованы и не смешиваются с Instagram Login.
