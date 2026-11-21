# Universal Business Test Matrix

Статус: **PLANNED / NOT RUN**. Документ определяет будущие acceptance tests; не содержит новых runtime PASS. Предыдущий Stage 2.1 остаётся partially complete, PostgreSQL gates BLOCKED.

## 1. Business scenarios

Каждый сценарий запускается как минимум в трёх synthetic tenants A/B/C с одинаковыми внешними user IDs/SKU там, где это возможно, но разными данными. Все external integrations — mocks/fixtures, credentials synthetic; live/budgets не включаются.

| ID / бизнес | Catalog/search fixture | AI/evidence expectation | Order/request и обязательный отказ |
| --- | --- | --- | --- |
| B01 Electronics | Variant memory/color, known price, fresh/stale stock | Правильный SKU; гарантия только из source | Existing quote→consent→staff reserve; stale stock deny |
| B02 Auto parts | OEM list, make/model/year/engine + verified/unknown fitment | Уточнить vehicle fields; OEM match не гарантирует fitment | Product request only for validated selection; unknown compatibility handoff |
| B03 Building materials | Package unit, dimensions/weight/volume, exact conversions | Отличать упаковку от kg/m³ | Whole-pack order; fractional request disabled до M-UNIT |
| B04 Furniture | Dimensions/material, assembly, standard/custom | Не выдумывать сроки сборки/изготовления | Product + assembly hybrid или custom RFQ; changed spec invalidates consent |
| B05 Vehicle service | Repair offerings, diagnostic unknown pricing, mechanics/location | Сначала проблема/авто, затем assessment; no invented diagnosis | Non-binding repair request; booking требует capacity, unknown price не 0 |
| B06 Service salon | Fixed/hourly offerings, staff/room/buffers | Public slots only; не раскрывать private appointments | Consent+staff booking, overlap/DST/expired hold deny |
| B07 Real estate agency | Published/withdrawn listings, area/location/status evidence | Не обещать доступность без source; no title guarantee | Viewing/inquiry, не автоматическая продажа объекта |
| B08 Hybrid company | Product + service в одном tenant | Обе domain tool groups только при effective capabilities | Atomic stock+slot confirm; один недоступный ресурс → no partial commit |
| B09 Manufacturing | Standard goods и custom drawing/spec/version | Различать stock и ещё не утверждённую мощность/lead time | Engineering RFQ/estimate approval; changed tolerances require re-consent |
| B10 Non-food wholesale | Tier prices, eligibility, volume, pack constraints | Не выдавать закрытый прайс чужому customer | V2 limits/checked arithmetic; oversized v1 cart остаётся rejected |
| B11 Clothing | Size system/material/color/season | Не переводить неизвестную размерную сетку догадкой | Variant request; mixed size schema mismatch deny |
| B12 Unsupported food trade | Declared food retail/wholesale, включая custom profile label | Explain unsupported segment; не активировать товарный workflow | Eligibility/publish/admission deny; «other industry» не bypass |

B01–B08 — обязательный минимум; B09 проверяет отдельный целевой manufacturing segment. Данные только тестовые, без реальных VIN/телефонов/адресов клиентов/владельческих документов.

## 2. Сквозные dimensions — обязательны для каждого B01–B11

| Dimension | Positive control | Negative / invariant |
| --- | --- | --- |
| Catalog | Owner пишет собственные typed records, customer видит published projection | Чужие definition/value/media IDs, wrong type/unit/reference → deny; no persistent changes |
| Search | Точный filter/range и tenant-specific ranking | No cross-tenant fallback, unauthorized wholesale tier/private service notes |
| AI context | Manifest фиксирует profile/policy/evidence revisions | Tenant B facts отсутствуют в A prompt; document/profile injection не меняет tools |
| Orders / requests | Correct type, exact quote snapshot and consent | No auto financial approval; unknown amount ≠ free; wrong quote/customer/conversation reject |
| Isolation | Own visible rows/updates и DB snapshots как positive controls | A/B/C SELECT/INSERT/UPDATE/DELETE, FK mismatch, empty context, missing role, pool reuse |
| Channels | TG/WA/Instagram normalization→lease→common engine при supported capabilities | Account/channel/tenant spoof, signature bypass, closed window, disabled sending |
| UI | Capability-specific fields/navigation, mobile/keyboard/errors | Hidden nav не открывает API по прямому URL; нет false connected/confirmed |
| Migration | Lossless v1 electronics, industry switch preview | IDs/history/stock/audit не исчезают, invalid coercion не публикуется |

«Negative passed» требует фактической DB проверки counts/fingerprints после операции, а не только HTTP 403/404. Unexpected SQL syntax/connection error не принимается за authorization proof.

## 3. Channel matrix

| Канал | Required regression | Runtime policy |
| --- | --- | --- |
| Telegram | Existing fenced repository/handlers; product и implemented service actions; identity tuple | Legacy repository сохраняется; mock transport, live off |
| WhatsApp | Shared queue, mapping, statuses, retries, approved-template/provider/financial gates | Budget0; no unapproved initiation/template; DELIVERY_UNKNOWN не resend |
| Instagram | Signature before enqueue, durable state, account binding, current-epoch webhook proof, product lookup (G0-CATALOG), worker | Mock-only baseline; SendOne disabled/cancelled не считается успешной delivery |

Для каждого implemented domain run выполняется во всех трёх каналах. Если adapter не умеет действие/вложение/consent UI, ожидается explicit unsupported/handoff, а не фиктивное прохождение. Отсутствующий critical path остаётся BLOCKED до реализации/проверки; реальные сообщения не нужны для mock acceptance.

## 4. Concurrency / reliability pack

- Два staff approvals последнего stock item; reserve/release/consume ровно один раз.
- Две booking confirmations одного slot; multi-resource и capacity N; cancellation/reschedule races.
- Hybrid confirm при changed price/stock/slot/spec revision: transaction оставляет ноль частичных effects.
- Duplicate webhook, duplicate order command, interrupted worker after message commit/before ack, expired/reclaimed lease, late finalizer.
- Bounded exponential backoff, retry classification, terminal DLQ, queue metadata tenant/account/channel cannot be replaced by payload.
- Repeat publish/profile switch; immutable past quote snapshot, stricter authorization действует сразу.
- Price/stock/knowledge freshness, expired evidence, schema invalidation/cache partitioning.
- Decimal/currency scale/overflow/rounding; fractional mode и unsupported currency запрещены до readiness.

## 5. Isolation entities / principals

Проверить profile/bindings/attribute definitions+values/media/customer extensions/identity links/services/resources/calendars/holds/bookings/quotes/orders/listings/specifications/knowledge/context manifests/jobs.

Principals: owner A, authorized staff A, ordinary member A, customer A через каждый канал, owner B/C, no session, no tenant, expired capability, wrong-channel capability. Приватные resource calendar/customer notes не доступны даже public service reader. Tenant admin не публикует template override глобально.

## 6. Evidence и gates

Каждый run сохраняет source/schema/profile versions, exact commands/env without secrets, exit codes, test IDs, full initial FAIL, repair diff, rerun result, DB safety marker, SQL result assertions и plan/latency evidence для новых search/locks. Browser checks запускаются только на свежем guarded production build; proof не переписывается для сокрытия drift.

Gate G0 Stage 2.1 → domain unit/type validation → isolated migration/RLS/concurrency → full Go test/race/vet/build + existing TG/WA regressions → frontend lint/typecheck/fresh build/E2E → mock channel/domain acceptance. NOT RUN, BLOCKED, FAIL и PASS не взаимозаменяемы.

Перед запуском numeric performance targets фиксируются на существующем synthetic baseline и fixture размерах; здесь latency/throughput PASS не выдумываются. Live LLM/provider/business rollout — отдельная авторизация вне этого blueprint.
