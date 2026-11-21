# Stage 2.1 + Stage 3 + Multi-Agent Foundation

Дата: 2026-11-18. **Stage 2.1: BLOCKED.** Универсальное ядро и агентная основа реализованы как изолированные Go-модули; они не подключены к production/runtime. Полная приёмка объединённого плана не достигнута. Документация, компиляция и unit tests не заменяют PostgreSQL isolation evidence.

## 1. Baseline и Stage 2.1

В репозитории отсутствует первый commit: `git rev-parse HEAD` возвращает exit 128. Сохранены полный исходный Git status и content hashes; существующие staged/unstaged/untracked изменения не сбрасывались. Прочитаны полный Stage 2.1 report, десять Stage 3 blueprint документов, existing Agent Architecture, quality-gate routing и соответствующий код API/worker/catalog/agent/трёх каналов. Последняя migration — 00017.

Интеграционный исполнитель владел Stage 2 remediation и итоговой проверкой. Параллельные исполнители работали только в непересекающихся новых пакетах `businessfoundation`, `agentfoundation`, `specialists`. Общие migrations/queue/tenant middleware не редактировались параллельно. После handoff интеграционный исполнитель исправил finding в регистрации специалистов и повторил общую проверку.

PATH Go — 1.26.2; фактический default GOTOOLCHAIN — auto. Использован существующий официальный cached Go **1.26.11 darwin/amd64**, с явным GOROOT/GOTOOLCHAIN=local и полным существующим module cache. Повторно сверены 11 524 toolchain files с cached archive: 0 mismatches, h1 совпадает с сохранённой checksum database записью. Это локальная provenance-проверка, не новая online-аттестация. Downloads, dependency/toolchain requirement changes и global settings не выполнялись.

Docker CLI 29.9.2 доступен; оба configured daemon endpoints недоступны. Native PostgreSQL binaries не найдены в PATH и проверенных установках; 5432/5433 не слушают. Disposable DB отсутствует. Рабочие DSN не использовались. Harness, запущенный с явно отсутствующими DSN, остановился на safety guard до SQL connection. Три PostgreSQL tenants в этом запуске **не создавались**.

| Stage 2.1 acceptance gate | Результат |
| --- | --- |
| Go format/tests/race/vet/API build/worker build | PASS |
| Migration 00017 Up/schema/FK/index/Down/reapply | BLOCKED |
| PostgreSQL FORCE RLS, A/B/C I/U/D, pooled connections | BLOCKED |
| Durable OAuth state/credentials/attachments isolation | BLOCKED |
| Queue enqueue/claim/ack/retry/backoff/DLQ/recovery | BLOCKED |
| Реальное успешное worker execution с PostgreSQL | BLOCKED |
| Telegram full DB regression | BLOCKED |
| WhatsApp full DB regression | BLOCKED |
| Instagram → tenant catalog full route | BLOCKED |

Финальные Go receipts: unit-3/race-3/vet-3/api-build-3/worker-build-3, все exit 0 на одном стабильном source digest. Format-2 и integration-compile-4 также PASS. Полные команды и статусы — в локальном INDEX.

## 2. Universal Business Architecture

`internal/businessfoundation` содержит работающие pure-Go validators и projections:

- Business Model отделён от versioned industry bindings, configured capabilities и configuration references. Model/profile codes расширяемы; effective capabilities — пересечение implemented/enabled/authorized/policy facts. Food trade явно отклоняется; отсутствие декларации для product sales не считается разрешением.
- Typed catalog sidecar ссылается на existing product/variant IDs. Поддержаны string/integer/exact decimal/boolean/enum/date/string-list, controlled units, required fields, bounds, 32 keys/4 KiB, reserved-key denial и cross-field ranges. Цены/stock не переносятся в attributes. Arbitrary nested objects/fitment graphs пока не реализованы.
- Самостоятельные service/offering/provider/resource/availability и proposal-only booking descriptors; fixed/hourly/on_request/custom_estimate pricing. Unknown amount отличается от zero. Проверяется snapshot availability, buffers и capacity. Это не transactional reservation и не подтверждение записи.
- Product/service/repair/quotation/custom/property/hybrid intents ссылаются на существующий order backbone и отвергают financial confirmation. Нет второго order engine, нового product master или tenant identity.
- Readiness projection привязана к organization/profile revision; channel configuration, offline validation и live authorization различаются. Это архитектура backend-мастера, не реализованный UI wizard.

Постоянное хранение Business Profile, industry switching/history, typed attribute sidecars, service scheduling, generalized order dispatch и соответствующие APIs пока отсутствуют. Existing RUB/integer ledger, consent/staff confirmation и legacy DTO не менялись. Structural Reference checks не доказывают наличие/ownership строк: это обязанность будущих repositories с RLS/composite FK.

## 3. Multi-Agent Foundation

`internal/agentfoundation` реализует immutable tenant-bound registry, capability selection, bounded orchestrator, typed tools, least-data context admission и approval/audit/idempotency storage contracts. Никакого второго LLM, произвольного SQL/shell/filesystem/HTTP tool или отдельного процесса не создано. Пакет вызывается в тестах и может быть подключён только после реализации проверенных durable adapters.

## 4. Специализированные агенты

`internal/specialists.Registrations` создаёт семь compiled definitions для trusted business model и отдельных industry profile codes. Это регистрация в библиотеке, не регистрация нового production worker handler.

| Agent | Реализованный режим / граница |
| --- | --- |
| Customer | Existing Runner tools: product reads, own order status, human handoff; нового LLM нет |
| Sales | Тот же проверяемый Runner facade; создание заказа/quote в новом агентном интерфейсе пока отсутствует |
| Product & Inventory | Existing customer catalog projection, строго без stock mutations |
| Marketing | Детерминированный review-required draft; не анализ кампаний и не публикация |
| Finance | Read-only injected tenant-scoped projection с revision/freshness; без reader возвращает unavailable |
| Analytics | Такой же проверяемый read projection; реальные аналитические repositories не подключены |
| Business Manager | Детерминированный план read-only направлений; coordination/delegation отключены до общего budget/cycle protocol |

Legacy catalog adapters допускаются только для явно заданного electronics profile в совместимой trade/hybrid модели. Unknown/mixed industry и service-only не получают electronics workflow автоматически. Общие read/draft/plan adapters допускают новые валидные model/profile codes без изменения фиксированного списка; их исполнение всё равно требует tenant configuration и permissions. Реальная универсальная AI-консультация этим не объявляется готовой.

## 5. Agent Registry

Descriptors включают ID/name/description/version/capabilities/permissions/tools/supported models/profiles/limits/status/configuration. Registry deep-copies metadata и не имеет cross-tenant fallback. Tenant configuration должна явно enable агента и может только сузить compiled tool/context allowlists. Disabled/unknown/ambiguous routing отклоняется. Descriptive business metadata не является authorization; будущий adapter обязан получать модель/профили из canonical approved Business Profile.

## 6. Orchestrator

Capability-based selection проверяет tenant и текущую configuration. `Run` ограничивает steps, context/results bytes, deadline и abstract work units; неизвестная стоимость отклоняется. Agent limits могут только ужесточить global limits, включая явный zero budget. Повтор canonical action определяется как cycle; task claims не освобождаются автоматически после fail/unknown. Context/configuration/permission перечитываются перед effects. Детачированная finalization ограничена timeout.

Это не RUB admission и не замена существующего monetary budget ledger. Межагентных вызовов сейчас нет: поэтому nested orchestration не может сбрасывать родительский budget, но полноценная multi-agent decomposition ещё не реализована.

## 7. Permissions

Каждый tool call повторно проверяется backend PermissionChecker и tenant configuration. Grants не берутся из planner/context/tool JSON. Action kinds разделяют read/create/update/execute/external communication/financial/critical settings/irreversible effects. Четыре последних high-effect типа требуют approval уже при регистрации tool. Domain consent, role, source ownership и финансовые правила остаются дополнительной обязательной проверкой executor transaction.

## 8. Tool Registry

`NewJSONTool[I,O]` использует concrete struct DTO, strict JSON, input/output validators, canonical parameters и bounded output. Отклоняются unknown/duplicate/case-confused fields, null, trailing data и неправильный scope. Typed output проходит дополнительную проверку перед выдачей. Existing Runner bridge проверяет tenant/conversation/session principal и применяет existing tool schemas. Channel principal ownership требует DB-backed resolver; сейчас такой resolver не установлен.

## 9. Shared Context

Scope содержит tenant/principal/conversation, приходит только от authenticated backend. ContextProvider получает только разрешённые keys и byte budget. Bundle должен точно соответствовать scope; лишние keys/oversize/invalid JSON отклоняются, planner получает копии. Real Business Profile/catalog/services/customer/history/policy/analytics source adapters и provenance manifests в runtime пока не подключены. Fixture scope equality не заменяет реальную PostgreSQL tenant isolation.

## 10. Queue integration

**BLOCKED.** SQL queue и worker registration не расширялись для agent jobs до закрытия G0. Существующая shared channel queue требует integration/account/conversation; fake Instagram/WhatsApp integration для agent task не создавалась. Telegram остаётся на прежнем repository в общем worker lifecycle. Retry/backoff/DLQ текущих каналов не переписывались.

Следующее изменение после G0 должно расширить **существующую durable queue**, а не создавать agent outbox: явный job-kind discriminator; channel-event defaults/constraints сохраняются; agent tasks имеют tenant/agent/conversation/correlation/idempotency/deadline/version metadata и typed tenant FKs. Provider columns разрешено ослабить только условным CHECK для agent kind после schema review. Existing channel claim/scope обязаны исключать agent kind; agent claim получает отдельный узкий authority без channel capabilities. Одна таблица/lease/retry/DLQ инфраструктура, тот же worker, authoritative tenant из row/FKs, payload не источник tenant. Migration и adapters требуют real PostgreSQL adversarial/concurrency tests.

Это supersedes optional separate domain-outbox proposal старого blueprint для agent задач; исторические документы не переписывались. Ни DDL, ни работающая queue integration этим планом не заявляются.

## 11. Audit Log

Исполняемый orchestrator формирует события с hash tenant/principal/conversation/task/agent/action, canonical parameter hash, step/result status и approval status none/required/consumed. Raw prompts, credentials, parameters, results, PII и SQL errors в events не входят. Ошибка intent audit блокирует effect; claims сохраняются. Persistent audit sink и transactional linkage к domain mutations пока отсутствуют.

## 12. Human Approval

Approval binding включает tenant/principal/conversation, task, agent, action, canonical parameters hash, tool/agent/configuration versions и expiry. Изменённые parameters/versions, чужой scope, consumed/revoked/expired approval должны отклоняться atomic `Consume`. Planner не выпускает approvals и не восстанавливает использованное согласие. Unit fixtures проверяют drift/replay/revocation; durable issuance, awaiting-approval/resume/revocation flow, owner API/UI и DB-atomic domain admission **не реализованы**. Такой контракт не объявляется готовым durable workflow.

## 13. Изменённые файлы

Existing application files:

- `apps/api/internal/catalog/service.go`: Instagram использует shared channel scope; `app.channel_type` устанавливается transaction-local вместе с capability. WhatsApp выбирает прежний shared scope, Telegram — прежний fenced scope.
- `apps/api/tests/integration/instagram_test.go`: signed registered webhook → durable enqueue → common worker/AI → actual tenant product cards; до worker messages отсутствуют.

Новые files:

- `apps/api/tests/integration/instagram_catalog_test.go`: actual A/B/C products/prices/stock, lease/channel/tenant rejection, read-only mutation rejection и DB fingerprints.
- `apps/api/internal/businessfoundation/{profile,attributes,service,order,onboarding,foundation_test,decimal_regression_test}.go`.
- `apps/api/internal/agentfoundation/{contracts,registry,json_tool,orchestrator,foundation_test}.go`.
- `apps/api/internal/specialists/{specialists,bridge,specialists_test,business_model_regression_test}.go`.
- Этот report и `.local/stage-2-1-3/` evidence/index/runners.

Точный список, hashes и сохранность старых файлов — `source-comparison-final.json`. Новых dependencies нет. Frontend/branding, existing AI engine и channel adapters не переписаны.

## 14. Migrations

Новых migrations нет; 00001–00017 сохранены побайтно. Migration 00017 не применялась и не откатывалась в этом запуске. Следующие номера не резервировались. После G0 нужны последовательные additive packages M-BP/IP/ATTR/SVC/ORD из blueprint, затем обоснованное shared queue/agent configuration/approval/audit расширение с FORCE RLS и composite FKs. Старые таблицы/IDs/history/financial snapshots нельзя удалять или заменять.

## 15. API endpoints

Новых routes для Stage 3/agents нет. Existing registered Instagram routes сохранены:

- `GET /api/v1/organizations/{id}/integrations/instagram`
- `POST .../oauth/start`; `GET .../oauth/callback`
- `POST .../disconnect`; `GET .../webhook-status`
- `GET/POST /api/v1/webhooks/instagram/{integration}`

Auth/signature/error paths проверяются mock suites и real process probe. Successful durable OAuth/webhook/ownership flows остаются BLOCKED. Proposed v2 Business/agent/approval APIs не зарегистрированы.

## 16. Тесты и evidence

Окончательные команды, exits, source stability и статусы: [.local INDEX](../.local/stage-2-1-3/INDEX.md). Полные logs сохранены, включая исходные FAIL. После последнего code fix выполнены новые whole-tree unit/race/vet/build checks; промежуточные PASS не подменяют финальные receipts. Runtime-3 подтвердил native process probe. PostgreSQL safety-guard-2 завершился exit 1 до соединения и обозначен BLOCKED, не PASS.

Domain fixtures: electronics, auto parts, building materials, furniture, vehicle service, service company, real estate, hybrid; manufacturing дополнительный. Проверены typed values/ranges/units/decimal precision, unknown pricing, nonbinding order/booking validation, capacity snapshot, tenant reference mismatches, readiness и food exclusion. Это не DB/platform acceptance этих отраслей.

Specialists: 8 × 7 execution cases, включая ожидаемый отказ неподдерживаемым catalog domains; unauthorized tools/scope injection/foreign principal, mutations/admin/send/delegation refusal, stale evidence, unavailable readers, unknown costs, duplicate tasks и cancellation. Foundation security tests проверяют permission escalation, extra context, concurrent duplicate claims, approval drift/replay, cycles/steps/deadline/budget, immutable descriptors и mandatory approval kinds.

Native process probe запускает собранные API/worker с synthetic environment и собственным **неслушающим** loopback DB socket. Проверяет startup, health, auth/signature rejection, DB failure→503, registered Instagram worker error path, redaction и SIGTERM. Это не PostgreSQL worker/job proof и не execution новых foundation modules в production process.

## 17. Security findings и remediation

1. **G0-CATALOG, static finding:** Instagram catalog reads выбирали Telegram scope; исправлены routing и transaction-local provider discriminator. Добавлен реальный PostgreSQL regression с own-positive controls, expected 403/404 вместо принятия любой SQL error и before/after product/inventory fingerprints. PostgreSQL подтверждение исправления пока BLOCKED.
2. **Specialist model/profile conflation, reproduced FAIL:** закрытый список industry names блокировал новый valid business model. `business-model-regression-1.log` сохраняет исходный FAIL. Исправлено: model и profiles независимы, generic registrations расширяемы, legacy catalog остаётся deny-by-default; missing/ambiguous/invalid configuration отклоняется. Новый regression и whole-tree rerun сохранены отдельно.
3. Intermediate `vet-1` FAIL возник во время публикации неполного specialist module (`RunnerResolver` ещё отсутствовал). Также в specialist handoff сохранены два compile FAIL: unused import и неправильно размещённое поле Kind. Эти ошибки устранены; исходные logs не удалены. Финальная проверка выполняется после freeze, с source-before/source-after hashes.

Cross-tenant PostgreSQL leakage не оценена и не объявлена отсутствующей. Key rotation и durable provider/security риски предыдущего Stage 2.1 report остаются открыты. E01 остаётся OPEN; guards/exclusions/proof rerecord не изменялись.

## 18. BLOCKED

- Stage 2.1: PostgreSQL migration/schema/FORCE RLS/A-B-C/pooling, durable OAuth/credential/conversation/message/attachment/queue/DLQ/recovery и полные TG/WA/Instagram regressions.
- G0-CATALOG full route: добавленный integration test только компилируется.
- Business persistence, generalized order dispatch, safe profile switch, actual resource locks/holds и API/AI context integration.
- Agent durable configuration/idempotency/approval/audit storage и shared queue/worker integration.
- Human approval durable lifecycle и shared-budget multi-agent coordination.

Первичный инфраструктурный blocker — отсутствующий изолированный PostgreSQL runner. Остальные пункты также требуют реализации после G0: появление Docker само по себе их не завершит.

## 19. NOT RUN

Frontend lint/typecheck/fresh build/E2E в этой задаче не запускались: frontend не менялся, новый wizard/UI не реализован. Новые domain/agent HTTP end-to-end scenarios не выполнялись, endpoints отсутствуют. Linux runtime verification и live Meta/Telegram/WhatsApp/LLM не выполнялись. Production deployment/readiness, сообщения, финансы и кампании не запускались и не разрешались.

## 20. Следующие приоритеты и приёмка

1. Предоставить существующий isolated Docker/native PostgreSQL runner с `aba_test`, aba_app/aba_migrator и safety marker; выполнить prepared integration harness. Не использовать рабочую БД и не скачивать dependencies без разрешения.
2. Сохранить реальные SQL FAIL, исправить причины без weakening RLS; подтвердить 00017 Up/Down/reapply/FKs, A/B/C writes/reads/pooling, queue/recovery, TG/WA regressions и Instagram product-backed route. Только после этого Stage 2.1 COMPLETE.
3. Реализовать canonical Business Profile/capabilities/profile adapters и additive persistence; далее typed catalog/services/requests с прежними orders/consent. Не включать услугу в legacy product confirmation без dispatch/constraints/concurrency proof.
4. Реализовать durable agent configuration/claim/audit/approval adapters и безопасное расширение существующей queue, затем зарегистрировать handlers в том же worker. Проверить DB ownership каждого context/tool input/output, deadlines/idempotency/retry/DLQ и параметр-bound human approval.
5. Подключить существующий AI engine к approved universal context; отдельно принять service/listing tools и shared-budget manager coordination, затем capability-driven onboarding UI без rebrand. Пройти матрицу восьми бизнесов на трёх tenants и трёх mock channels.

До закрытия этих gates нет утверждения CODE COMPLETE для объединённого плана, RUNTIME VERIFIED для durable foundation или production readiness. Commit/push/tag/deploy, изменение рабочей БД и live enablement не выполнялись.
