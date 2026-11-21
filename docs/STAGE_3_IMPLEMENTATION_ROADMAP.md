# Stage 3 — Implementation Roadmap

**Результат текущей задачи: technical blueprint, не реализованная универсальная платформа.** Изменяются только десять новых design documents. Application code, SQL migrations, business logic, branding, production и live integrations не меняются.

## 1. Стартовое решение

Сохранить Go API/worker/PostgreSQL/Next BFF и existing domain services. Обобщение по capabilities и versioned profiles; услуги — самостоятельный домен, orders/identity/catalog не дублируются. Industry — изменяемая конфигурация, не `business_type=electronics` навсегда. Food retail/wholesale исключены из target scope.

Stage 2.1 status по сохранённым report/receipts: **STAGE 2 PARTIALLY COMPLETE**. Go tests/race/vet/build ранее PASS, DB/RLS/queue/channel workflows BLOCKED. Это историческая evidence, не новое подтверждение текущего runtime. Пересекающиеся компоненты и migrations нельзя менять в этой задаче.

Дополнительный code-review blocker: `catalog.Service.beginAccess` обрабатывает shared channel scope только для WhatsApp, тогда как Instagram tenant проходит `tenancy.Begin`. Нужен реальный Instagram catalog/price/stock test и отдельное устранение fail-closed routing gap в Stage 2.1. Не выдавать unit compile за доказательство end-to-end AI.

## 2. Навигация по blueprint

| Документ | Решение |
| --- | --- |
| [Architecture](UNIVERSAL_BUSINESS_ARCHITECTURE.md) | Audit, Business Profile, domain boundaries и единый security contract |
| [Catalog](UNIVERSAL_CATALOG_DESIGN.md) | Stable fields + typed extensions, units/media/wholesale |
| [Services](UNIVERSAL_SERVICE_DOMAIN.md) | Catalog/pricing/resources/availability/booking concurrency |
| [Orders](UNIVERSAL_ORDER_DOMAIN.md) | Existing aggregate, typed lines, consent/approval и compatibility |
| [Industry profiles](INDUSTRY_PROFILES_DESIGN.md) | Declarative versioned registry, overrides, switch/mapping |
| [AI context](AI_CONTEXT_ENGINE_DESIGN.md) | Tenant/evidence-bound context composition и allowed operations |
| [Onboarding / UX](UNIVERSAL_ONBOARDING_DESIGN.md) | Eight-step wizard и capability-driven existing design shell |
| [Migration plan](UNIVERSAL_MIGRATION_PLAN.md) | Expand/backfill/validate, symbolic packages, rollback/guards |
| [Test matrix](UNIVERSAL_BUSINESS_TEST_MATRIX.md) | Восемь обязательных отраслевых сценариев + manufacturing/wholesale/negative cases |
| Этот roadmap | Приоритеты, зависимости, proposed API и acceptance |

## 3. Work packages и приоритеты

| Package | Приоритет / prerequisites | Deliverable / exit gate |
| --- | --- | --- |
| G0 — close Stage 2.1 | P0, до пересекающейся реализации Stage 3 | Disposable PostgreSQL Up/Down/RLS/three-tenant/queue/recovery, TG/WA regression, Instagram product-tool path PASS |
| BP — Business Profile + capability registry | P1, G0 | Legacy profile facade + draft/publish/version, model отдельно от industry, no new tenant identity |
| IP — industry binding/schema registry | P1, BP | Immutable approved versions, custom tenant definitions, impact preview/safe switch |
| CAT — typed product extensions | P1, IP | Legacy string map retained, typed validation/search/source ownership, media ACL; electronics regression |
| AI-R — context composer, read-only tools | P1, BP+IP+CAT | Existing Runner integration, evidence manifest, dynamic supported allowlist, prompt/tenant eval; no new mutation rights |
| SVC-R — service catalog | P1, BP+IP | Distinct services/offerings/public availability semantics, no SKU/stock workaround |
| CUST / ORD — customer anchors + generalized requests | P1, G0 + SVC-R для service refs | Same IDs/quotes/orders, explicit format/type dispatch, non-binding estimates/repair/custom intake, v1 contracts preserved |
| SCHED — resources/holds/booking | P2, SVC-R+ORD | Capacity race/expiry/reschedule/fencing PASS; confirmation does not bypass staff |
| JOBS — domain async work | P2, только при необходимости после ORD/SCHED/knowledge entities | Один typed domain outbox, same worker; no fake integration, narrow leases и bounded retry/DLQ |
| HYB — hybrid approval | P2, CAT+ORD+SCHED | Atomic stock + resource admission; no partial commitments |
| EXT — listings/manufacturing/wholesale | P2, BP+IP+ORD и нужные facades | Optional domain modules, verified evidence, no per-industry product clones/ERP claims |
| AI-W — new domain tools | P2, tested corresponding module | Same guardrails/consent/idempotency; only implemented operations exposed |
| UX — universal wizard/workspace | P1 incremental, BP/IP + relevant module | Eight-step branches/capability projection, same branding/design, existing URLs compatible |
| QUANT/MONEY | P3, separate explicit design/approval + ledger tests | Exact quantity/currency end-to-end; remain disabled until accepted |
| VALIDATE — complete scenario pack | Перед acceptance каждого increment | Tests из matrix, real PostgreSQL isolation/concurrency и fresh guarded frontend E2E |

После G0 независимые CAT и SVC-R можно планировать раздельными изменениями, но delegation/new agents этим документом не разрешаются. Feature flag не может скрывать failed security gate. Schedule/currency/AI tooling не показываются как доступные по одной записи registry.

## 4. Proposed migration / API inventory

Migrations — M-BP, M-IP, M-ATTR, M-CUST, M-SVC, M-ORD, M-SCHED, optional M-JOBS, M-KNOW, M-LIST, M-CUSTOM, M-PRICE, M-UNIT/M-MONEY, M-ONBOARD из [migration plan](UNIVERSAL_MIGRATION_PLAN.md). Реальные номера назначать после фактической последней Stage 2 fix migration; **00018 не резервируется**. Ни одного `.sql` этим blueprint не создано.

Ниже paths — **PROPOSED, NOT REGISTERED**. Все новые tenant routes под `/api/v2/organizations/{id}`; existing `/api/v1` contracts остаются.

| Ресурс / методы | Authorization / контракт |
| --- | --- |
| `GET/PATCH business-profile`, `GET business-capabilities` | Owner write; read approved projection по principal; expected_version |
| `GET /api/v2/industry-profiles` | Только approved public templates, без tenant overrides |
| `GET/PUT industry-bindings`, `POST industry-bindings/preview,publish` | Owner; version/idempotency/audit; immutable revisions |
| `catalog/attribute-definitions`, `products/{id}/attribute-set`, `variants/{id}/attribute-set` | Owner/source-aware write; schema-bound public reads |
| `GET catalog/search`, product media/link operations | Tenant/customer price scope; typed filters и media ACL |
| `services`, `services/{id}/offerings`, service resources/availability rules/exceptions | Service catalog/schedule permissions раздельны |
| `GET service-slots`, `POST booking-holds` | Conversation-bound customer operations, no private calendar |
| `POST quotes`, `POST order-requests`, `GET orders/{id}`, `POST orders/{id}/commands` | Same order aggregate; staff-only financial confirm, explicit consent |
| `POST bookings/{id}/reschedule` | Version/consent/permission + conflict check |
| `GET/POST listings`, `PATCH listings/{id}`, `POST listing-inquiries` | Published projection / owner-agent write / current customer inquiry; только implemented module |
| `POST custom-specifications`, `POST custom-specifications/{id}/estimates` | Customer-owned intake и отдельное staff engineering/estimate approval; no AI commitment |
| `GET/POST business-onboarding`, `POST business-onboarding/validate` | Owner + revision-bound readiness; config success ≠ live enabled |

Method/schemas/error codes окончательно фиксируются в OpenAPI до реализации соответствующего package, включая v1 handling v2 order IDs. HTTP writes используют existing session/Origin/CSRF, bounded input, optimistic version и нужную idempotency. External/customer entrypoints сохраняют provider signature/state/principal model; tenant не берётся из payload.

## 5. Compatibility risks и решения

| Риск | Безопасное решение / доказательство |
| --- | --- |
| Новый industry перезаписывает legacy electronics type | Additive model/bindings и v1 projection; no destructive rewrite |
| String attributes превращаются в произвольные objects | Новый versioned DTO/sidecar; v1 response type неизменен |
| Service NULL variant ломает старый confirm/render | Dispatch и conditional constraints до enable v2; explicit incompatible-ID handling |
| Старый stock path запускается на услуге | Type-aware fulfillment; product inventory code reuse только для product lines |
| Slot и stock фиксируются частично | Single DB transaction/consistent lock protocol, races и compensation tests |
| Free-form profile/knowledge даёт AI полномочия | Data/instructions separation + operation allowlist + executor authorization |
| Industry change меняет исторические условия | Immutable quote/order/profile snapshots, explicit new consent |
| Currency/quantity flag обходит текущие constraints | Флаг off до ledger/arithmetic/API/UI gate |
| Universal customer CRM объединяет разные identities | Preserve existing UUID/tuple; explicit verified tenant-local links |
| Stage 2 unverified SQL маскируется новой migration | G0 prerequisite, next-number-at-implementation, no SQL now |

## 6. Приёмка реализации Stage 3

1. G0 закрыт новыми PostgreSQL receipts; статический routing finding проверен отдельно.
2. Business model и industry independent/versioned; custom supported profile без core refactor; смена профиля не теряет историю.
3. Existing products/categories/SKU/order/customer/conversation IDs и v1 behavior сохранены, no duplicate source of truth.
4. Typed product filters/units/media/source ownership корректны; unknown price/stock/compatibility не становятся фактами.
5. Services — отдельные offerings/resources, четыре pricing models, honest unknowns, concurrent booking без overbooking.
6. Product/service/repair/quotation/custom/hybrid request types используют единый approval/idempotency/audit; AI не подтверждает обязательства.
7. Все новые entities и projections проходят PostgreSQL A/B/C read/write/RLS/pool/job tests; credential/context leakage — rejection.
8. AI использует current tenant profile/catalog/policies/conversation/operations/knowledge с evidence и revocation; mock и live validation не смешиваются.
9. Onboarding/UX показывают только applicable implemented capabilities, сохраняют branding/дизайн и mobile/accessibility, readiness не фальсифицируется.
10. B01–B08 плюс manufacturing и критические negative cases из matrix PASS; Telegram/WhatsApp/Instagram mock regressions обязательны.
11. Relevant Go tests/race/vet/build, migration rounds, frontend lint/typecheck/fresh guarded build/E2E подтверждены для каждого изменения. E01 не ослабляется и не закрывается без causal proof.
12. Production/live providers/budgets/releases требуют отдельной авторизации вне этого blueprint.

## 7. Проверка текущих deliverables

В этой задаче допустимы PASS только для полноты десяти документов, внутренних ссылок и сохранности application/migrations относительно baseline. Runtime/domain tests — **NOT RUN (docs-only scope)**. Исторические Stage 2 PostgreSQL gates остаются **BLOCKED**. Проверка полноты design не является acceptance универсальной реализации.

Audit evidence и проверка неизменности сохраняются локально в `.local/stage3-blueprint/`. Оставшиеся продуктовые decisions (права scheduling, список реально поддерживаемых currencies/units, локальные appointment правила, media storage/dependencies) утверждаются до соответствующего package; отсутствие решения не обходится свободным AI текстом.
