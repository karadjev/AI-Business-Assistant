# Безопасность WhatsApp — mock

## Trust boundaries

Сервер расшифровывает конфигурацию по integration UUID через узкую proof-gated SQL-функцию. HMAC-SHA256 проверяется по исходному телу, hex подпись сравнивается constant-time. Все entry/changes должны иметь настроенные WABA ID/Phone Number ID; никакой organization_id из webhook не используется. Предел raw body64KiB, не более10 entry/10 changes/50 events в change/100 events в запросе; тексты до2000 символов, JSON single document, валидный UTF-8.

Verify token и App Secret разделены. GET возвращает только цифровой challenge и не принимает сообщения. Некорректные signature/integration/phone/user дают одинаковый отказ. Replay сообщения — уникальность integration+message ID; status event дополнительно содержит тип status в ключе. Порядок обработки — durable arrival, а не управляемый клиентом timestamp.

Principal создаётся только после allowlist. External IDs не дают users/sessions/membership. Scope выдаётся только leased inbox job, не JSON/LLM/browser. Runtime не может напрямую изменять queues, secrets, events; shared writes ограничены RLS и существующими сервисами. FORCE RLS включает5 новых таблиц; общий inventory32 tenant tables проверяется integration suite.

## Секреты и внешняя сеть

Сохраняются только AES-GCM ciphertext и server worker-proof hash; ключ отдельно в environment. Token/App Secret/Verify Token не входят в Status/OpenAPI responses, frontend не получает сохранённые значения. UI использует только явно публичные synthetic fixtures, которые не являются настоящими токенами. Полные тела webhook/prompts/credentials не логируются. HTTPMock требует явного тестового RoundTripper и использует фиктивный адрес, который тест перенаправляет только в loopback. Рабочая конфигурация всегда выбирает Mock без HTTP.

Наличие mock mode само по себе не является firewall. Проверки этапа выполняются в Compose internal network без default route, host ports, Docker socket, proxy env; DNS127.0.0.1. Это ограничение тестовых контейнеров, не утверждение о глобальной блокировке сети хоста. Подтверждение — команды и результаты STAGE_8B2_REPORT.

## Authorization и бизнес-правила

Owner меняет канал/allowlist при connect; staff читает диалоги, отвечает в human mode и разрешённом окне. Member без can_manage_orders не имеет staff-доступа. BFF не является единственной защитой: backend session/CSRF, membership и SQL проверяются повторно. Заказ только через конкретный действующий quote и интерактивное confirm; проверка цены/наличия и резерв — существующие сервисы. Catalogue data и сообщения остаются недоверенными, не изменяют системные правила.

## Остановка и хранение

Owner выключает sending или active. Admission+session advisory lock сериализуют отправку с disconnect/handoff. Lease fencing не разрешает старому worker завершить новое задание. UNKNOWN удерживает порядок до ручного sent/cancelled, без resend. Подробности MULTICHANNEL_DELIVERY.

Default retention7 дней на новые диалоги, настраивается1–30 при connect. Worker закрывает истёкшие WA conversation и стирает message/inbox/outbox payload; IDs/principals/audit и order snapshots сохраняются. Это не полное удаление/анонимизация всех персональных данных. Реальные покупатели запрещены до отдельной политики, срока metadata/backups и юридической проверки.
