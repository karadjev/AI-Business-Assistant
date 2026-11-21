# Industry Profiles Design

Статус: **CONFIGURATION BLUEPRINT**, не исполняемый plugin system. [Business model](UNIVERSAL_BUSINESS_ARCHITECTURE.md) и industry — разные оси.

## 1. Registry и ownership

Approved global profile registry хранит только public immutable configuration: code/version/label, parent industry, sub-industry, schema references, required implemented capabilities и minimum compatible application/schema version. Его записи не содержат tenant data и не изменяются обычным runtime principal.

Tenant-owned `industry_profile_bindings`, override revisions и local attribute definitions имеют organization_id, composite FKs, FORCE RLS, owner-only draft/publish. Tenant override не меняет общий template для других клиентов. Возможно несколько bindings; primary profile служит UX default, не запрещает hybrid operations.

Custom industry допускает новую specialization и typed fields без core code change. Новый **домен/операция**, например финансовое действие, требует server implementation и review; declarative configuration не заменяет код.

## 2. Формат profile revision

| Раздел | Содержание |
| --- | --- |
| identity | code/version, industry/sub-industry labels, locale, compatibility metadata |
| catalog_schemas | Product/variant attribute definitions, field visibility, validation, units |
| service_schemas | Service/repair requirements, duration/pricing models, resource capabilities |
| matching_rules | Approved finite eq/in/range/required operators, evidence requirements |
| clarification_questions | Bounded question templates и dependencies полей |
| consultation_steps | Declarative intent/question/retrieve/clarify/handoff stages |
| request_rules | Permitted implemented order type, required intake, approval policy reference |
| restrictions | Unsupported segments/actions, unknown-data behavior, escalation conditions |

Не допускаются SQL/JS, shell, arbitrary HTTP endpoints, regex/expressions без ограничения, raw system prompts, secrets или именование несуществующих tools. Конфигурация не задаёт tenant, DB credentials, разрешения, LIVE flag или бюджет.

## 3. Начальные profiles — примеры, не предел системы

| Profile | Подбор / вопросы | Request / ограничение |
| --- | --- | --- |
| electronics | SKU/model, memory/color, warranty source | Product quote; отсутствие характеристики не заменять данными аналога |
| auto_parts | Make/model/year/engine, verified OEM/fitment evidence | Unknown fitment → уточнение/специалист; похожий OEM не sufficient |
| building_materials | Dimensions, selling unit/package, required volume | Exact unit/pack conversion; fractional ledger gate |
| furniture | Dimensions/material, assembly/delivery, standard/custom | Product sale или custom specification; lead time staff-verified |
| vehicle_service | Vehicle/problem, diagnostic need, location/resource | Repair request/booking; diagnosis и cost не угадывать |
| personal_services | Offering/duration/resource/location/preferences | Booking; sensitive details minimization |
| real_estate | Sale/rent intent, listing status, area/location/budget | Listing inquiry/viewing; availability evidence, no title/legal assurance |
| manufacturing | Standard SKU vs custom drawing/spec, quantity, tolerances, target date | RFQ/engineering review; no invented capacity or delivery date |

Для real estate минимальная optional listing schema: tenant/asset ID, public/private address projection, sale/rent, area decimal+m², price_state/amount/currency/period, publication/availability state, source verification time, assigned agent membership, media. Это listing evidence, не подтверждение сделки. Для manufacturing: immutable specification revision, requested quantity/date, verified estimate/lead time и engineer approval; requested_date не является обещанным сроком.

Hybrid — composition совместимых bindings, а не огромный hardcoded отраслевой profile. При конфликте required fields/units/rules publish останавливается с явным conflict report; silently last-wins merge запрещён.

## 4. Lifecycle и переключение отрасли

1. Owner выбирает profiles и версии, видит capability requirements.
2. Создаётся draft binding; dry-run оценивает current products/services/forms/AI tools.
3. Report показывает missing/invalid attributes, incompatible units/workflows, required mapping и disabled capabilities.
4. Explicit mapping/confirmation публикует revision с expected_version; запись history обязательна.
5. Новые dialogs/operations получают новую revision. Active conversations фиксируют context revision на ход; опасное изменение policies/authorization применяется немедленно через server admission, независимо от snapshot.
6. Старые orders/quotes/bookings сохраняют исходные snapshots. Draft/schema mismatch не делает опубликованные товары автоматически достоверными для новых фильтров.

Нельзя удалять legacy attributes при switch. Можно откатить binding к совместимой revision, но нельзя откатить уже исполненные financial/stock/schedule effects. Missing migration mapping → read-only legacy view/handoff, не догадки.

## 5. APIs и validation

Proposed: `GET /api/v2/industry-profiles` для public templates; tenant root `/industry-bindings`, `/industry-bindings/preview`, `/industry-bindings/publish`, `/catalog/attribute-definitions`. Publish — owner, expected_version, idempotency key, audit. Custom definition остаётся private в пределах tenant.

Validation: allowed field types/units/operators, limits on depth/bytes/list size, conflicting keys, required module versions, unsupported food trade, immutable version references. Профиль не может активировать service booking, если scheduling module отсутствует или выключен.

Acceptance: extension новой непродовольственной отрасли конфигурацией без ядрового refactor, A/B override isolation, unknown module fail-closed, malicious instructions/config rejection, safe version switch/backfill preview, existing electronics replay unchanged. Coverage — [test matrix](UNIVERSAL_BUSINESS_TEST_MATRIX.md).
