# Universal Order Domain

Статус: **PROPOSED**. Существующие `orders`, `order_quotes`, `order_items`, `order_events` остаются единственным order backbone. Не вводить параллельный «универсальный заказ» с другими ID.

## 1. Baseline и неизменяемые гарантии

`orders.Service.PrepareQuote/CreateRequest/Change` уже реализует server snapshots, короткий срок quote, привязку customer/conversation, idempotency, optimistic version и staff inventory reserve/release/consume. `Change` запрещён channel capability. Эти правила сохраняются.

Текущие ограничения: product/variant FK обязательны, money RUB, quantity целое 1–10, до 10 cart lines, order total/line price положительны. Legacy confirm всегда вызывает stock path. Поэтому добавление `type='service'` без нового dispatch и constraints недопустимо.

## 2. Типы и единый контракт

Order type — versioned server-supported code, первоначально product_sale, service_booking, repair_request, quotation_request, custom_manufacturing, hybrid, property_inquiry. Отраслевой profile выбирает разрешённый implemented type, но не задаёт исполняемый transition код.

Order aggregate получает `contract_version`, `order_type_code`, workflow revision, pricing/fulfillment summaries, policy/profile snapshot references. Existing ID, source channel, customer/conversation, audit, quote and confirmation attribution сохраняются.

| Item kind | Ссылки и поведение |
| --- | --- |
| product | Существующие product/variant, price/stock versions; inventory service |
| service | Service offering snapshot; optional booking/allocation; без product SKU |
| custom_work | Tenant specification revision/approved estimate; engineering/staff workflow |
| listing_reference | Property listing inquiry/viewing reference; не продажа объекта через товарный stock |

Предпочтение — поэтапно расширить `order_items` discriminator/typed references, а не создавать вторую monetary line system. Product columns становятся nullable **только** для разрешённых v2 kinds; CHECK требует ровно допустимые references, все FK включают organization_id. Добавляются stable line ID/position и tenant-scoped unique constraints. Generic `(type,id)` без FK не допускается.

Legacy rows маркируются contract_version=1/product; их старые ограничения сохраняются условными constraints и v1 validation. SQL/runtime изменение nullable колонок и generalized totals допускается только вместе с проверенным dispatch; не отдельным «быстрым» schema patch.

## 3. Quote, money, quantity

Quote snapshots остаются immutable, versioned и server-owned. Для v2 в `order_quotes` добавляется format/workflow metadata; JSON snapshot имеет объявленный формат. Relational `quote_line_references` связывает line IDs с tenant-owned product/service/specification/listing для composite FK ownership; не дублирует цены как второй источник истины. Persist helper проверяет соответствие reference set snapshot.

Money: amount_minor + currency + price_state + price source/revision/validity. Mixed currencies в одном quote запрещены; unknown total — null/unknown, не сумма только известных lines. Known zero отличим от unknown и требует отдельного разрешённого v2 workflow. RUB remains initial implemented currency; валютное расширение не активируется одним profile flag.

Количество v1 не меняется. Wholesale integer quantities выше 10 и fractional quantities требуют отдельных v2 bounds/overflow/reconciliation tests. Fractional value вводится exact decimal+unit, только после согласованной миграции inventory movements/reservations/quotes/order math. Расчёт стоимости — checked arithmetic; binary float запрещён.

Изменение цены/объёма/даты/срока/условий после согласия создаёт новую revision предложения и новое согласие. Исторические snapshots не пересчитываются по новому profile.

## 4. Workflow и финансовая граница

| Workflow | Допустимый смысл |
| --- | --- |
| product v1 | Existing quote → customer consent → pending_confirmation → staff confirm/reserve → complete/consume или cancel/release |
| service | Request/quote → optional temporary hold → customer consent → pending_confirmation → staff confirm/allocation → fulfillment |
| repair/custom | Intake → needs_assessment → staff estimate → customer consent → pending_confirmation → staff acceptance |
| quotation request | Non-binding inquiry без обещанного total/срока; approval не превращает её молча в обязательство |
| hybrid | Общий quote и согласие; financial approval и resource/stock admission атомарны |
| property inquiry | Lead/viewing request; title/lease/sale completion вне автоматического AI confirmation |

Финансовое approval и operational fulfillment имеют разные states. Draft quote или hold не подтверждает оплату, договор, производство или резерв товара. Model text «да», tool invocation, message delivery receipt не являются consent. Consent хранит actor/principal, quote hash/revision, scope, timestamp и authorized channel action.

Staff confirm выполняет свежую authorization, state/version/idempotency checks, повторную проверку catalog/source/stock либо schedule/specification, затем запись movements/allocations/events в одной DB transaction. Hybrid lock order стабилен: order → resource IDs → существующий catalog product/variant/inventory protocol, без инверсии внутренних product locks. Финальный порядок требуется проверить на shared paths до кода; network calls внутри транзакции не выполняются.

При внешнем эффекте в будущем — durable operation ledger и outbox, неизвестный результат не replay автоматически. Пока external payments/calendars/production control не включаются.

## 5. API compatibility

Existing v1 product endpoints/payloads/errors сохраняются. V2 orders обслуживает тот же repository с type-aware handlers. V1 list/detail/commands не сериализуют service rows в старый product schema: documented legacy product projection либо явный `ORDER_TYPE_UNSUPPORTED` для ID v2, согласованный до rollout. Нельзя молча скрыть исторические product orders или изменить старый response type.

Предлагается `/api/v2/organizations/{id}/quotes`, `/order-requests`, `/orders`, `/orders/{order}`, `/orders/{order}/commands`. Command — registered operation, expected_version, scoped idempotency key, reason; approve/reschedule/complete имеют отдельную authorization. Первый v2 increment сохраняет conversation-bound requests. Customer actions всегда связаны с конкретным conversation; employee-created case требует explicit actor/source, не fabricated channel customer. Standalone ERP orders без диалога — отдельный scope, а не молчаливое ослабление существующих quote/conversation FKs.

## 6. Acceptance

Все v1 product tests остаются обязательными. Новые: each type state transitions, nullable/reference CHECK/FK, cross-tenant line injection, unpriced request, changed quote/consent, no auto-confirm, hybrid partial failure rollback, oversell/overbook race, double webhook/replayed command/worker interruption, cancellation compensation exactly once, wrong currency/unit/overflow, archived profile replay and immutable historical snapshots.
