# Заявки и резерв — этап 5

Только изолированный тестовый контур: `organizations.is_test=true` создаётся отдельным POST `/api/v1/organizations/test`. Обычная организация автоматически тестовой не становится. Каталог/склад тестового магазина заполняются отдельно синтетическими товарами. Любая заявка этапа 5 имеет `source=internal_chat`, `is_test=true`; API и FORCE RLS запрещают её создание в обычном магазине. Telegram и реальные клиенты появятся только после отдельного задания.

## Цена, согласие и идентификаторы

PostgreSQL — источник истины. Backend создаёт order_quote на 5 минут с позициями, UUID вариантов, quantity 1–10, именем/SKU, unit_price_amount в копейках RUB, variant version, available и временем ручной сверки. Максимум 10 уникальных позиций; цена и сумма не принимаются от браузера/LLM. Перед предложением и созданием заявки повторно проверяются active status, цена/version и доступное количество. TTL не фиксирует цену и не обещает наличие.

Оформление — отдельный защищённый POST confirm-request: quote_id, accepted=true, idempotency_key UUID. UI показывает позиции/сумму/срок, требует checkbox и кнопку подтверждения. Текст «да», аргументы модели и кнопка подготовки quote не являются согласием. Backend связывает consent с точным tenant/chat/customer, записывает agreed_at/agreed_by. Customer UUID в conversations/orders отдельный от user UUID оператора; для симуляции не собираются имя/телефон покупателя. confirmed_by и actor_id событий — сотрудник из сессии.

При истечении/изменении цены, version характеристики или уменьшении доступного количества —409 QUOTE_EXPIRED/PRICE_CHANGED/INSUFFICIENT_STOCK. Заявки не создаётся. Пользователь нажимает «Обновить предложение цены», видит новые данные и повторно соглашается. Проверка version консервативна: изменение характеристик также требует нового согласия. Согласованный snapshot orders/items неизменяем; employee не может подставить новую цену. При PRICE_CHANGED во время employee confirm нужно подготовить новое предложение и получить новую явную заявку; старую pending можно отклонить/отменить, не переписывать snapshot.

## Схема

Migration00004 добавляет `order_quotes`, `orders`, `order_items`, `order_events`, conversations/messages/events/ai_usage, is_test и can_manage_orders. Везде organization_id, UUID и составные FK. Quote items — ограниченный backend JSON snapshot; order items нормализованы. Orders unique(org,idempotency_key), unique(org,quote_id); items unique(org,order,variant); events unique(org,order,idempotency_key). Есть created/updated/confirmed timestamps и confirmed_by. Total BIGINT, без floating point. RLS INSERT/SELECT/UPDATE selected org + verified session + staff + test org. Items/events append-only для runtime. Down00004 отказывается при stage5 данных/test organizations/staff grants; миграции 1–3 сохраняются.

## Переходы и транзакции

| Откуда | Команда | Куда | Склад |
|---|---|---|---|
| pending_confirmation | confirm | confirmed | reserved += qty |
| pending_confirmation | reject | rejected | Без изменений |
| pending_confirmation | cancel | cancelled | Без изменений |
| confirmed | cancel | cancelled | reserved -= qty |
| confirmed | complete | completed | on_hand -= qty, reserved -= qty |

Другие переходы запрещены. Заявка при создании не резервирует. Confirm/reject/cancel/complete требует expected_version и UUID idempotency_key. Повтор успешной команды с тем же key+body возвращает записанный результат без движения склада; другой body с тем же key —409. Новая команда со старой version —409. Failed commands не считаются успешными и не сохраняют результата, повтор заново проверяет состояние. Создание сериализуется lock conversation и transaction advisory lock org/key; повтор того же quote/key возвращает единственную заявку, другой quote/key конфликтует.

Транзакция: membership/RLS →lock order →lock products sorted UUID →lock inventories sorted variant UUID →проверка всех позиций →MoveOrderStock →orders status/version →order_event →COMMIT. Любая ошибка откатывает весь резерв, включая ранее обработанные позиции. Узкая definer `lock_order_products(uuid[])` проверяет selected test org/staff и только блокирует строки этого org; не выдаёт member право менять каталог. Склад пишет общий internal catalog.MoveOrderStock в переданной транзакции, отдельного public reserve endpoint нет.

При confirm цена должна совпадать со snapshot, товар active, available≥qty, последняя ручная сверка свежая. Cancel/complete confirmed разрешены даже после архива товара, чтобы существующий резерв можно было корректно закрыть. DB CHECK сохраняет 0≤reserved≤on_hand; version увеличивается при каждом движении. Immutable inventory_movements добавляют kind reserve/release/consume, before/after и delta reserved, order_id, actor. Manual adjust также сохраняет reserved snapshot.

`inventory.manual_updated_at` — последняя ручная корректировка/сверка; reserve/release/consume не освежают её. `updated_at` продолжает отражать любую запись. Backfill берёт последнюю manual movement прежнего этапа; если сверки не было —NULL. INVENTORY_FRESHNESS default24h. Stale/unknown наличие не гарантируется агентом; staff confirm получает STALE_INVENTORY до ручной сверки владельцем.

## Права и API

| Право | Owner | Member без staff | Member с can_manage_orders |
|---|---|---|---|
| Каталог read | Да | Да | Да |
| Каталог/настройки write | Да | Нет | Нет |
| Test orders/chat read/write, handoff | Да | Нет | Да |
| Выдать/отозвать staff существующему member | Да | Нет | Нет |

`PATCH O/members/{user_id}/order-access {allowed:boolean}` не добавляет участника и не меняет role. Безопасные приглашения остаются вне этапа. Права проверяются заново при каждом API/service запросе; cookie не фиксирует роль.

O=`/api/v1/organizations/{id}`. GET O/orders?status&page, GET O/orders/{resource}; POST O/orders/{resource}/confirm|reject|cancel|complete. Quote POST O/conversations/{resource}/quotes и consent POST .../confirm-request. Требуются session, Origin + CSRF для мутаций; foreign/absent404, default member403, real org403, validation422, price/stock/version/state409. JSON expected_version/idempotency_key сохраняет стиль этапа4 вместо If-Match/Idempotency-Key header. Точные schemas —[OpenAPI](../packages/contracts/openapi.yaml).

UI `/dashboard/orders`, `/dashboard/orders/[id]`: status filter, snapshot позиции/сумма, события, подтверждающий сотрудник в API/events, confirm/reject/cancel/complete с явным диалогом подтверждения и обработкой conflict. Список ограничен 20/page; safe tenant switch сбрасывает чужой detail. Тестовые заявки явно обозначены.

Проверки: Go integration/race реальный PostgreSQL —consent/idempotence, changed price, stale/sold stock, две pending на последнюю единицу, multi-item rollback, cancellation/completion once, permissions/RLS. Browser —chat→quote→explicit consent→pending→reserve→cancel и отдельная complete. Фактический результат —STAGE_5_REPORT. Никакого production/manual external sale учёта, оплаты или доставки пока нет.
