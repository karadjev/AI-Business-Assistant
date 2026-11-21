# AI Context Engine Design

Статус: **PROPOSAL**, расширение существующего `agent.Run`/Runner, а не отдельный отраслевой AI. Источники baseline: `agent/orchestrator.go`, `agent/tools.go`, `conversations/service.go`, `catalog/service.go`, `tenancy/transaction.go`.

## 1. Аудит baseline

`InstructionsVersion="electronics-v1"`; system instructions ориентированы на электронику, `Schemas()` содержит product search/details/price/inventory/order status/request/handoff. Runner получает trusted Tenant/ConversationID, customer consent не задаётся model tool arguments. History ограничена, backend формирует безопасный текст/cards, provider budget admission уже существует.

`store_profiles.ai_instructions` сохраняется, но в проверенных `agent`/`conversations` нет чтения этого поля при сборке контекста. Само наличие поля в onboarding не доказывает влияние на AI. Pricing/stock tools возвращают структурированные данные, но текущий `Redact` применяется также к сериализованным tool results: перед поддержкой OEM/SKU нужна field-aware redaction, чтобы длинный артикул не маскировался как телефон.

G0-CATALOG: Instagram branch в catalog transaction helper требует отдельной Stage 2.1 проверки/исправления; [architecture](UNIVERSAL_BUSINESS_ARCHITECTURE.md). Здесь код не меняется.

## 2. ContextComposer contract

Вход только server-derived: tenant, authenticated/channel principal, conversation, control mode, request/correlation ID. Ни tenant ID, ни роль не выбираются из LLM arguments или импортированного документа.

Выход: typed ContextBundle + manifest, а не конкатенация всего каталога:

| Компонент | Источник | Доверие и scope |
| --- | --- | --- |
| Business Profile | Approved aggregate revision | Public factual projection; free text — untrusted data |
| Industry Profile | Pinned approved bindings/schema revisions | Только декларативные validated rules |
| Catalog | Existing product facade + optional implemented service/listing facades | Tenant/current customer price scope, active/public records |
| Business Policies | Published policy revision | Core authorization/financial rules всегда выше preferences |
| Conversation | Current tenant+conversation, bounded history | Customer text не создаёт permissions/consent |
| Available Operations | Compiled operation registry ∩ effective capability ∩ principal/policy gates | Schema и executor используют один allowlist |
| Relevant Knowledge | Published tenant+ACL filtered evidence | Document/chunk/source revision, freshness, no global tenant fallback |

Manifest содержит org/conversation/principal scope identifiers, profile/policy/schema/tool versions, evidence IDs/versions/observed_at/valid_until, model/instructions version, budget decision и correlation. Secrets/tokens/raw private documents в manifest/log не входят. Customer request text не логируется как diagnostic metadata по умолчанию.

## 3. Pipeline одного хода

1. Worker lease или authenticated session определяют tenant/conversation scope; validate org/account/channel binding.
2. Проверить human control/closed/expired conversation, usage/cost gates. Нет оплаты или unknown cost → не считать запрос бесплатным.
3. Загрузить approved profile/bindings/policies и вычислить effective operations. Missing configuration/module → bounded clarification либо human handoff.
4. Retrieval только релевантных authorized catalog/knowledge records. Не загружать весь tenant CRM/calendar/catalog в prompt.
5. Нормализовать evidence: known/unknown/stale/conflicting, amount+currency, typed units, source/validity. Price/stock/slot факты приходят из domain tools, не из profile prose.
6. Сформировать bounded messages с чёткой границей instructions/data. Schema versions и размеры проверяются до LLM call. Оставить текущие iteration/tool/token limits до отдельного evaluation.
7. Tool executor повторно проверяет operation grant и tenant/resource access, даже если tool не предлагался модели или manifest устарел.
8. Backend renderer создаёт product/service/listing cards и non-binding request/quote summary из typed results. Unsupported factual output не становится обязательством.
9. Перед mutation проверить актуальные versions, consent, freshness/capacity/admission в доменной транзакции. При изменении условий — новый quote/согласие; не использовать cache для confirmation.
10. Сохранить outcome/context manifest без secrets. Ошибка/нехватка evidence → handoff, не invented answer или бесконечный retry.

Нельзя держать SQL transaction открытой во время LLM/network call. Context snapshot служит audit, но не замораживает permissions: revocation и более строгие policies действуют при каждом tool/action.

## 4. Operation registry

Сохранить product tools как compatibility facade. Добавления после соответствующих modules: `search_services`, `get_service_details`, `get_available_slots`, `prepare_service_quote`, `create_service_request`, `search_listings`, `get_listing_details`, `create_viewing_request`, `create_custom_quote_request`.

Типизированные аргументы не содержат organization_id, произвольных SQL/URL/path или raw staff assignment. `create_*_request` не означает `confirm`. Staff approval/stock consumption/payment/production launch не выдаются customer LLM. Existing consent envelope, idempotency и state machine остаются в backend.

Domain routing определяется intent + effective operations + validated evidence. Отраслевая подсказка «рекомендовать аналог» не разрешает выдавать unverified compatibility за match.

## 5. Factual evidence policy

| Факт | Обязательное evidence | При недостатке |
| --- | --- | --- |
| Цена / скидка / wholesale tier | Current price source, customer eligibility, currency, revision, validity | Price unknown / запрос сотруднику |
| Наличие | Inventory availability + existing freshness decision | Не обещать stock/резерв |
| Совместимость | Verified applicability assertion, нужные vehicle/spec fields | Уточняющий вопрос; handoff |
| Service slot | Current rules/resources/capacity check | Нет подтверждённого slot |
| Lead time / custom work | Approved estimate/capacity evidence с conditions | Non-binding inquiry |
| Условия | Published applicable policy revision | Не обобщать из другой отрасли/магазина |
| Listing status | Tenant-owned current listing source/verification date | Не обещать доступность/право собственности |

## 6. Knowledge и cache isolation

Будущие `knowledge_documents/chunks` tenant-owned, draft/approved/published/expired, source provenance, ACL и retention. Jobs ingestion несут trusted tenant/document revision; URL из документа не становится HTTP destination. Customer uploads/private staff notes не публикуются автоматически.

Первый increment может использовать bounded PostgreSQL retrieval; отдельный vector store не нужен для объявления architecture. Если он потребуется, отдельное обоснование/dependency approval, tenant+ACL partition, deletion/invalidation и isolation tests обязательны.

Cache keys включают tenant, authorization/customer price scope, relevant revisions, locale/timezone и query; conversation-private content дополнительно conversation ID. Нельзя cache по одному тексту вопроса или SKU. Shared public template cache отделён от tenant evidence. Tightened ACL/archival/profile switch инвалидируют projections; mutation всегда перечитывает источники.

Field-aware PII minimization сохраняет разрешённые SKU/OEM/цены и маскирует персональные поля. Prompt injection tests охватывают description, attributes, profile overrides, documents, tool results и сообщения. Конфиденциальные записи другого tenant не попадают в prompt даже если модель их просит.

## 7. Acceptance

Все scenario fixtures из [matrix](UNIVERSAL_BUSINESS_TEST_MATRIX.md): правильные tools/questions, source-bound facts, zero unsupported factual assertions в deterministic fixtures, отсутствие cross-tenant evidence, prompt injection refusal, no automatic approval, version race/handoff и budgets0. Live LLM evaluation отдельно требует разрешения; mock PASS не доказывает качество реальной модели.
