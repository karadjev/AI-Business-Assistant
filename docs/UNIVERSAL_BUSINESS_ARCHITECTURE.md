# Universal Business Architecture — blueprint

Статус: **DESIGN ONLY**. Универсальная поддержка не реализована. Этот документ — точка входа в [roadmap](STAGE_3_IMPLEMENTATION_ROADMAP.md); остальные девять документов уточняют контракты. Изменения приложения разрешаются только отдельными задачами после gates.

## 1. Проверенная исходная архитектура

Go API + существующий Go worker, PostgreSQL, Next.js BFF, channel adapters и общий `channel.Engine` сохраняются. Новые домены — пакеты существующего приложения, а не новые процессы или отраслевые копии AI/catalog/identity.

| Источник | Факт из кода | Ограничение |
| --- | --- | --- |
| `migrations/00002_identity_tenants.sql`, `internal/organizations/service.go` | Tenant — `organizations.id`; `business_type='electronics'` в CHECK и create response | Тип бизнеса смешан с отраслью и неизменяем |
| `00013_store_onboarding_catalog_import.sql`, `catalog/onboarding.go` | `store_profiles`: description/address/opening_hours/условия/AI instructions строками | Один адрес, неструктурированные правила и часы |
| `00003_catalog.sql`, `catalog/service.go` | products → variants → integer inventory; JSONB attributes, Go `map[string]string`, RUB | Нет typed units, services, schedules, images domain, дробного ledger |
| `00004_orders_chat.sql`, `orders/service.go` | quotes, consent, pending_confirmation, staff reserve/release/consume | Только variant-based product items, RUB, ограниченные целые количества |
| `00004`, `00010`, `00017`, `conversations/service.go` | `customer_id` UUID, conversations/messages и channel principals | Полноценной customer CRM entity в migrations нет; identity tuple уже существует |
| `agent/orchestrator.go`, `agent/tools.go` | `electronics-v1`, static product tool schemas | Профиль бизнеса/услуги не составляют динамический контекст |
| `Sidebar.tsx`, `StoreOnboarding.tsx`, `AttributeFields.tsx` | Магазинная навигация, шесть шагов, string attribute editor | Нет capability-driven workspace |

Отчёт [Stage 2.1](INSTAGRAM_STAGE_2_1_VALIDATION_REPORT.md) и `.local/instagram-stage2-1/acceptance-gates.json` фиксируют **STAGE 2 PARTIALLY COMPLETE**. Прошлые Go PASS — исторические результаты; PostgreSQL Up/Down/RLS/queue/channel regression BLOCKED. В этой задаче они не перезапускались.

**Дополнительный статический finding G0-CATALOG:** `catalog.Service.beginAccess` в `internal/catalog/service.go` выбирает shared channel scope только для WhatsApp; Instagram попадает в Telegram branch. `tenancy.Begin` уже различает WhatsApp/Instagram. Product lookup из Instagram runner поэтому имеет вероятный функциональный fail-closed разрыв. Это вывод по коду, не подтверждённый runtime incident или leakage. Нужен отдельный Stage 2.1 regression с реальным товаром и channel capability; здесь исправлений нет.

## 2. Business Profile без второго tenant/store

Tenant остаётся `organization_id`. Business Profile — versioned aggregate поверх **существующих** `organizations`, `store_profiles` и новых subordinate records. Не создавать параллельную `businesses` или вторую таблицу профилей с копиями имени/часового пояса.

| Поле aggregate | Предлагаемый источник / семантика |
| --- | --- |
| ID/name/timezone | Существующая organization; IANA timezone, отдельные location overrides |
| business_model | Новый code registry: trade/service/agency/manufacturing/hybrid — стартовые варианты, не закрытый SQL enum |
| selling_modes | retail/wholesale и будущие modes; независимо от industry |
| industry/sub-industry | Несколько versioned bindings; primary designation, specialization text |
| description | Существующий `store_profiles.description`, недоверенные factual data |
| configured_capabilities | Product sales, services, scheduling, listings, custom requests и др.; отдельно от effective permissions |
| currency | Business default; на первом этапе только реально поддерживаемый RUB, расширение требует money gate |
| operating hours | Structured weekly intervals/exceptions по locations; legacy opening_hours сохраняется |
| locations | Tenant-owned адреса/remote locations, timezone, contact/public visibility |
| business policies | Versioned структурированные delivery/cancellation/booking/payment rules с источником и effective dates |
| AI configuration | Existing `ai_settings` плюс ограниченные profile preferences; budgets/kill switches не становятся profile fields |

`organizations.business_type` пока сохраняется как legacy industry marker для v1 clients. Новый `business_model` не записывает `service` поверх `electronics` в старое поле. Legacy marker не используется для authorization или выбора AI specialization; новые clients читают v2 profile/capabilities. Legacy tenants получают совместимые defaults, а старые endpoints не должны обходить explicit capability disable у нового service-only бизнеса. Один tenant может менять specialization и иметь несколько отраслей; branches — locations, а не автоматически новые tenants.

Пример будущего aggregate, **не существующий API payload**:

```json
{
  "organization_id": "<existing-org-uuid>",
  "profile_revision": 3,
  "business_model": "hybrid",
  "selling_modes": ["retail", "wholesale"],
  "industry_bindings": [{"code": "auto_parts", "version": 1}, {"code": "vehicle_service", "version": 1}],
  "configured_capabilities": ["product_sales", "service_requests"],
  "default_currency": "RUB",
  "timezone": "Europe/Moscow"
}
```

## 3. Доменные границы

- Products/variants/inventory — существующий catalog; typed attributes и media — его расширения.
- Services/offerings/resources/booking — самостоятельный service domain, без фиктивного SKU/stock.
- Orders/quotes/items/consent/events — существующий order aggregate с format/type dispatch; новые типы не создают второй order engine.
- Customers — при необходимости extension существующих `(organization_id, customer_id)` UUID. Identity remains channel tuple; не staff users и не новая авторизация. Нельзя объединять клиентов по username/телефону автоматически.
- Property listings — отдельный optional asset domain: уникальный объект/публикация, а не variant с quantity=1. Booking просмотра/обращение входят в общий order flow; сделки/право собственности AI не подтверждает.
- Manufacturing — standard goods остаются products; custom specification/RFQ и engineering approval — расширение общего order workflow. MRP/BOM/shop-floor ERP не объявляются готовыми частью первого increment.
- Общий discovery DTO может возвращать tagged results `product_variant/service_offering/property_listing`; он не становится новым master catalog.

### Customer extension без новой identity system

Предлагаемые `customer_records(org,customer_id,status,display_name,version)` используют существующий UUID как ключ. `customer_identity_links` ссылается на existing Telegram principal либо shared channel principal через два nullable composite FK и exactly-one CHECK; external user/account identifiers читаются из principal, а не объявляются новой authority. `(org,principal)` уникален. Private notes/PII отделены от public customer projection.

Optional `customer_aliases(org,original_customer_id,canonical_customer_id)` сохраняют старые IDs вместо массовой перезаписи orders/history. Merge — verified staff action, same-tenant FKs, cycle/concurrency checks и audit. До неё никакого auto-merge по номеру/имени. Добавление customer anchors требует обновления **всех** conversation/claim writers и regression трёх каналов до включения новых обязательных FKs; это работа после G0.

## 4. Capability и security contract

Effective capability = configured capability ∩ реализованный модуль/feature flag ∩ authorization principal ∩ channel/policy/admission constraints. Отрасль или tenant JSON не могут разрешить LIVE, финансовое подтверждение или обход freshness.

Для **каждой** новой tenant-owned entity обязательны `organization_id NOT NULL`, `UNIQUE(organization_id,id)`, composite tenant FKs, ENABLE/FORCE RLS, runtime non-owner/non-BYPASSRLS, одинаковые owner predicates в USING/WITH CHECK. Empty context → deny. Capability lease разрешает конкретные conversation/customer/public projections, не всё содержимое tenant.

| Entities | Write authorization | Channel/AI read surface |
| --- | --- | --- |
| Profile, industry bindings, policies, attribute definitions | Owner; отдельные delegated rights только после явного назначения | Только approved public projection |
| Product values/media | Existing catalog owner/source ownership; media tenant ACL | Active/public media, разрешённые характеристики |
| Services/resources/calendars/locations | Owner/delegated catalog/schedule permissions | Public offerings и свободные slots; не личный staff calendar |
| Customer extension/identity links | Existing trusted principals; merge — отдельный verified staff workflow | Только текущий customer, без поиска по всем клиентам |
| Quotes/orders/booking holds/custom specifications | Current principal + operation permission + state/consent checks | Только текущий conversation/order scope |
| Knowledge items/context manifests/domain jobs | Approved publisher / worker lease с tenant/resource scope | Tenant/ACL-filtered published evidence |

Global industry templates — immutable approved configuration без tenant secrets; tenant overrides всегда tenant-owned. UI visibility не является authorization. Каждый новый API object ID проверяется внутри tenant до чтения или изменения. Queue jobs получают tenant/account/channel из trusted mapping; payload не выбирает их.

### Domain jobs и channel queues

Текущие channel queues требуют integration/account/conversation, поэтому local booking expiry или knowledge indexing нельзя помещать в них с фиктивным аккаунтом. Переиспользуются worker process и lease/idempotency/retry conventions, а не ошибочная модель subject. TTL hold истекает по authoritative DB time при admission, независимо от фоновой очистки.

Если нужны durable non-channel jobs, отдельный обоснованный M-JOBS добавляет **один** tenant-scoped domain outbox для реализованных job kinds, не очередь на отрасль. Booking/order/document references имеют typed columns/composite FKs, lease scope и version; payload не выбирает tenant. Общий worker dispatch использует registered handlers, unknown kind → terminal failure. Уведомления переходят в existing channel outbox только после provider/window/budget admission; job не разрешает самостоятельное подтверждение заказа.

## 5. Совместимость и scope

Исключены продуктовые магазины и продуктовая розничная/оптовая торговля. Custom industry label не обходит eligibility policy. Автоматическая классификация всех товаров по названию не считается надёжным enforcement; спорный ассортимент требует review. Иные сервисные специализации не запрещаются одним только названием отрасли.

Сохраняются branding и существующая дизайн-система. Никаких микросервисов, внешних calendars, нового vector store, dependency downloads или live enablement этим blueprint не предписано. Все proposed APIs/migrations описаны в [migration plan](UNIVERSAL_MIGRATION_PLAN.md) и [roadmap](STAGE_3_IMPLEMENTATION_ROADMAP.md), но не созданы.
