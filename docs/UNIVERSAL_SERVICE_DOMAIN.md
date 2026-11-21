# Universal Service Domain

Статус: **DESIGN ONLY**. Services не реализованы текущими products. Новые domain packages работают в существующих API/worker/PostgreSQL и используют общий order/conversation engine.

## 1. Модель

| Proposed entity | Назначение и constraints |
| --- | --- |
| `services` | org/id, name/description/category, draft/active/archived, version; самостоятельный service catalog |
| `service_offerings` | Вариант услуги: pricing model, currency, duration range, preparation/buffer, booking requirements, version |
| `service_resources` | Staff/equipment/room/capacity; staff membership binding `(org,user_id)` при наличии login, не обязательная новая auth identity |
| `service_resource_assignments` | Какие resources/skills могут выполнить offering; valid dates, required combinations |
| `service_offering_locations` | Join к canonical business_locations без копии адреса/timezone; offering доступен в location или remote |
| `resource_availability_rules/exceptions` | Local weekly intervals, holidays, leave/maintenance, valid period и IANA timezone |
| `booking_holds` + resource allocations | TTL hold ресурсов, order/conversation/principal binding; ограничения capacity и состояния |
| `service_bookings` | Ссылка на общий order и offering snapshot, start/end UTC, location, resource set, state/version |

Все строки tenant-owned; `UNIQUE(org,id)`, composite FKs к offering/resource/location/order/conversation. Tenant-owned внешнего подрядчика нельзя привязать к чужому staff user без membership/assignment. Полные schedules и private notes не доступны customer tool.

Service status и booking status различны. Draft offering не попадает в discovery, archived не удаляет прежние booking snapshots.

## 2. Pricing models

| Модель | Что AI может сообщить | Что допускает подтверждение |
| --- | --- | --- |
| fixed | Точную подтверждённую цену из действующей версии offering | Snapshot цены/условий + customer consent + staff approval |
| hourly | Rate, unit и согласованный объём/лимит | Quote задаёт длительность/cap; превышение требует нового согласия |
| on_request | «Стоимость по запросу» | Только non-binding request; unknown amount не равен нулю |
| custom_estimate | Подтверждённую сотрудником оценку/range с условиями | Обязательство только в границах явно согласованного quote/process |

`price_state=known/unknown/estimated/not_applicable` отделён от amount. Нельзя присвоить unknown service цене 0 или 1 копейку ради прохождения legacy constraints. Дополнительные работы и фактические часы не увеличивают согласованную сумму автоматически. Payment/deposit integrations не включаются этой схемой.

Duration задаётся service data, при диагностике допускается unknown/range. Если достоверной длительности нет, создать request к сотруднику, не обещать точный slot/срок.

## 3. Availability и scheduling

Business opening hours, offering rules, resource shifts, booked intervals, buffers и location exceptions должны одновременно разрешать slot. Отсутствующее расписание → availability unknown, а не «всегда свободно». Public API возвращает slots/capacity projection, не персональные calendars.

Ввод хранит timezone/offset, UTC start/end и локальный смысл appointment. Неоднозначные/несуществующие local times при DST отклоняются либо требуют явного выбора offset. Overnight interval и переезд между locations учитывают buffers; изменение timezone не двигает существующие UTC bookings молча.

MVP concurrency: transaction-scoped locks на tenant/resource и затронутые даты в стабильном порядке → повторный overlap/capacity check → hold/booking commit. UTC day partition для locks не заменяет location timezone для UI. Все даты пересекающего границу интервала входят в lock set. Для capacity N проверяется сумма allocations в пересечениях, а не только число rows.

Возможный range/exclusion design рассматривается отдельно; новые PostgreSQL extensions не устанавливаются автоматически. Первый implementation должен выбрать и нагрузочно доказать один алгоритм, а не смешивать два lock protocol.

## 4. Lifecycle и recovery

1. Customer ищет услугу; backend проверяет active offering и tenant.
2. Формируется non-binding request/quote; booking requirements (location, vehicle/problem/details) проверяются typed schema.
3. После выбора slot backend может создать временный hold: это **не** confirmed booking и не финансовое согласие.
4. Customer accepts конкретный quote/slot; staff confirms в общем order workflow.
5. В одной транзакции проверяются expiry, revisions, consent, resource/stock capacity; hold превращается в booking allocation.
6. Completed/cancelled/no_show/rescheduled — отдельные service transitions; cancellation освобождает ресурс идемпотентно, не возвращает деньги автоматически.

Expired hold никогда не оживляется повторным webhook. Reschedule сохраняет исходный booking audit, повторно проверяет capacity и требует нужного согласия. Expiry проверяется по DB time даже без worker. Cleanup/notification jobs scoped к tenant/resource и version, используют существующий worker и общие lease/retry/DLQ conventions; non-channel jobs требуют обоснованного M-JOBS, не фиктивной integration в channel_inbox. Provider delivery_unknown не вызывает повтор booking command.

## 5. APIs и права

Предлагаемый v2 tenant root: `/services`, `/services/{service}/offerings`, `/service-resources`, `/availability-rules`, `/availability-exceptions`, `/service-slots`, `/booking-holds`, `/bookings/{booking}/reschedule`.

Owner/delegated service catalog rights управляют offerings; schedule rights управляют resource availability; staff order permissions подтверждают commitments. `can_manage_orders` не должен автоматически давать private calendar access. Customer/AI principal: read public services/slots, create scoped requests/holds; не назначать себе staff и не менять booking state напрямую.

Calendar export/sync — отдельная будущая интеграция. До неё scheduling canonical в PostgreSQL, без ложных обещаний «синхронизировано с внешним календарём».

## 6. Acceptance

Два конкурентных подтверждения одного capacity=1 slot → один success; capacity N и multi-resource case также проверяются. Нужны DST/overnight/buffer/timezone tests, expired hold/retry/crash/late finalizer, owner/member/customer ACL, A/B/C read/write isolation и pool reuse. Hybrid confirmation должен атомарно удержать service resources и product stock либо не изменить ничего. [Order design](UNIVERSAL_ORDER_DOMAIN.md) определяет согласие и snapshots.
