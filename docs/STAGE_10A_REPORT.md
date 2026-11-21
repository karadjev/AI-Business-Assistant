# Этап10А — Store Onboarding & Excel Catalog Import

STORE ONBOARDING / EXCEL IMPORT / CATALOG SOURCE POLICY: **IMPLEMENTED offline**. Column mapping/preview/idempotency/tenant isolation/RLS PASS. Telegram/WhatsApp mock regression PASS. Go tests/race/vet/build PASS. Frontend lint/typecheck/OpenAPI/fresh production build/browser PASS. Security CONDITIONAL; CRITICAL/HIGH/LOW OPEN0/0/1 (E01), в проверенном scope.

## Аудит и архитектура

Изучены9В/readiness10А/9Б, PRODUCT_CATALOG, existing API/auth/tenant permissions/dashboard/migrations. Исходный каталог уже имел tenant SKU, integer копейки, versions, warehouse/reservations, customer read facade и owner-only mutations. Сохранены users/organizations/memberships, единые products/variants/inventory, existing order/agent/channel/outbox/financial layers. Новых tenant identities, второго каталога или очереди нет.

Миграция00013 добавляет supplementary store_profiles (тот же organization_id), durable catalog_imports, append-only runtime catalog_events —3 FORCE RLS таблицы, всего43. Tenant-safe FK, unique import request keys, actor constraints, индексы, row versions/TTL и guarded rollback. В старые catalog rows добавлены source tags MANUAL/EXCEL/ONE_C и source/audit triggers. Старые00001–00012/go.mod/go.sum/package-lock/embedded migration source сохранены (16 protected files).

Мастер5 шагов сохраняет все поля профиля и название существующего магазина, поддерживает6 статусов и явный ACTIVE после review/mock configuration. ACTIVE не меняет live flags/budgets. Ручное управление MANUAL товарами сохранено. ONE_C — только будущий CatalogFeed contract и disabled UI option, без подключения. Информационная business category дополняет owner profile; прежняя техническая electronics specialization не подменяется свободным текстом.

XLSX parser — Go standard library, без dependency downloads. Bounded ZIP/XML, без extraction/network/evaluation. Mapping/normalization повторно используют existing catalog validators; SKU tenant-scoped, RUB→копейки без float; все категории/товары/варианты/stock применяются одной transaction только после отдельного подтверждения. Ошибки/conflicts/duplicates блокируют весь batch. До confirmation товаров не меняет ни upload, ни mapping. Snapshot/locks защищают от stale preview, concurrent manual edits/stock reservations/imports. Applied confirmation replay не создаёт повторного эффекта. Recent import history восстанавливает preview/result после reload.

Source groups: product metadata, variant price/attributes, inventory on-hand. Обычный manual endpoint не может перезаписать EXCEL/ONE_C; source не принимается из файла/UI. Reserve/release/consume используют прежний trusted order path, не меняют source. Publish/archive остаются явными owner действиями. Отсутствующие в файле товары не удаляются; новые товары draft.

Backend endpoints защищены прежними session/Origin/CSRF/tenant BFF. Только raw upload получает1MiB body limit, JSON endpoints остаются64KiB. Нет публичного catalogue mutation endpoint, force/source/budget override. UI показывает источники, readable errors, metadata details и цену RUB до/после; изменение mapping требует нового preview. При смене tenant client state сбрасывается.

## Проверки

| Проверка | Результат и границы |
|---|---|
| Go test -count=1 ./..., race, vet, gofmt, API/worker/migrate/healthcheck build | PASS, final parser snapshot |
| Actual PostgreSQL integration/race | PASS41.489s; не mock DB |
| Migrations | PASS clean/repeated/concurrent up, empty13 down/up; populated rollback refusal сохраняет source/import audit и existing catalog |
| FORCE RLS / tenant / owner-staff | PASS43; actual outsider SELECT/INSERT/UPDATE/DELETE, re-parent denial, pool context isolation |
| Upload/mapping/preview | PASS no catalog writes, proper schema and invalid mapping denial |
| Transaction safety | PASS concurrent imports, duplicate confirmation, stale manual edit, injected transaction failure/rollback, replay |
| Source policy | PASS EXCEL/ONE_C manual overwrite blocked, invalid mixed batch rejected, reserved stock protected |
| Imported catalog regression | PASS current price grounding, consent required, staff confirm/reserve/consume, source EXCEL retained |
| Telegram/WhatsApp/financial regression | PASS existing actual process crash/unknown/outbox/tenant/security suites |
| XLSX security | PASS valid/empty/corrupted/oversized, formula/macro/external/path/DTD/malformed XML/bomb/row/column bounds; shared-string expanded text cap; invalid prices/currency/stock/attributes/null/model/duplicate SKU |
| Frontend lint/typecheck/OpenAPI0.10.0 | PASS |
| Native/artifact guard tests | PASS6; E01 now blocks record as well as verify |
| Fresh Next production build | PASS `Qz7bLOo_mrjL562aXZegn` |
| Browser current production | PASS2 expanded tests,0 skipped/unexpected/flaky |
| Client secret scan | PASS limited50 static files,0 findings |
| Original proof after exact E01 repair | PASS strict verify/served marker/content baseline; metadata observation OPEN |
| Real API/1C/LLM/messages/TLS/paid/deploy | NOT RUN — запрещены |

Import выполняется синхронно в API, отдельного import worker нет. Новая failure injection проверяет mid-transaction rollback (модель crash/connection loss), не выдаётся за отдельный OS-kill нового import worker. Existing channel worker process-kill cases реально выполнены. Повтор неизвестного commit разрешается через durable APPLIED result/history/version. Служебные subprocess helper SKIP не являются пропущенными parent crash tests.

Final Next build created `2026-10-28T13:00:04.564Z`, source SHA256 `560eb4d39fae90f9c25b6d122e7d7b1f3abd12bf2b2359144bec73e9665ed746`. Browser start `2026-10-28T13:03:06.003Z`, duration 468.878s:3shops×2channels + operator regression + full synthetic onboarding/Excel flow. First complete browser pass тоже2tests PASS; финальные UX/RUB/tenant-state улучшения подтверждены новым fresh build/full browser run. Нет cached build, выданной за свежую.

Остальные limitations supported format/workflow — EXCEL_CATALOG_IMPORT/CATALOG_SOURCE_POLICY: один tabular worksheet, RUB only, no image/media import,100 job history bound, controlled retention/source handoff/periodic freshness и real1C остаются следующим этапом. Owner AI instructions сохраняются; новый live prompt/store-policy tool не подключается. Это не незавершённый Excel apply и не разрешение live.

## Безопасность / E01

New tenant tables ENABLE/FORCE, runtime aba_app не owner/BYPASSRLS. Исходники FILE credentials не попадали в upload logs/audit. Цена/остаток не берутся из AI и не обходят tenant/source constraints. Public download — только синтетический demo workbook, не mutation API. Secrets/migrations/lockfiles не менялись. Working DB/stack не мигрировались/не перезапускались.

E01 **OPEN**: post-browser снова обнаружены пустые `standalone/.next 2` и `standalone/node_modules 2`. Content/path/type diff — ровно2 empty directories;2650 existing records изменили только metadata. Original proof неизменён. Strict verify отказал42; сохранены before/after manifests/timestamps/empty children/lsof. Только после capture удалены ровно эти empty paths через rmdir; original proof/served marker и точное content baseline снова PASS. Runtime web bind mount RO; macOS Virtualization PID68998 имеет read-handles, включая оба suffixed paths. Это **не доказанная causal attribution**. Guard не ослаблялся: новый record тоже отвергает эти известные paths, regression проверяет refusal без перезаписи proof. E01 не закрыт и не скрыт.

Development evidence: первый PG прогон обнаружил ошибку изменения тестового inventory count: строковый replace40→43 задел expected HTTP403, превратив его в433. Исправлено обратно403; фактический backend правильно отказал FORBIDDEN. Сохранён failed log, не засчитан PASS. Parser дополнительно получил safe handling attributes=null, expansion budget sharedStrings и ранний column bound; final Go/race и PG suite PASS. Старые миграции не изменялись, защитные policies не ослаблялись ради теста.

SECURITY CONDITIONAL; новых CRITICAL/HIGH в проверенном scope не найдено, не exhaustive production certificate. Live Telegram/WhatsApp/LLM disabled, kill switches true, external authorization false, оба бюджета0 RUB. Actual final synthetic DB schema13/WA limit0/nonzero WA wallets0/real pricing0/official records0/unsafe tenant RLS0. Никаких external calls/downloads/real credentials/customer data/paid operations/commit/push/deployment.

## Воспроизведение / evidence

Existing prepared Compose overlays environment + environment-smoke + prepared-build, ignored env `.local/audit8d/compose.env`. Cached images/pull never, internal networks/IPv6false/no host ports/no Docker socket in runner/no usable external route; proxies/toolchain/package downloads/telemetry disabled. Только собственные synthetic aba_test DB projects с prefix aba-stage10a. Финальный browser project aba-stage10a-current2; final PG aba-stage10a-check.

Evidence `.local/audit10a/`: baseline/protected preservation, integration-attempt1/final, RLS inventory, first/final build proof and browser JSON, metadata observer/E01 manifests/capture, client secret scan, results/source-changes. Screenshots `.local/audit8d/browser-results/stage10a-verified/`, only synthetic. Raw traces/sessions не публикуются. Source code список приводится ниже.

## Следующий шаг

Обязательных offline blockers10А не осталось. E01 LOW OPEN. Следующее отдельное offline ТЗ10Б: controlled source handoff, retention и stock freshness/reconciliation. Live и реальная1С NOT READY, требуют отдельного решения/разрешения; см. STAGE_10B_READINESS. После отчёта остановиться.


## Source files changed

Изменено 21, создано 19, удалено 0. Generated .local build/cache/evidence не входят в source count.

- `README.md`
- `apps/api/internal/catalog/service.go`
- `apps/api/internal/catalog/stock.go`
- `apps/api/internal/http/identity.go`
- `apps/api/internal/http/middleware.go`
- `apps/api/internal/platform/db/db.go`
- `apps/api/tests/integration/audit_test.go`
- `apps/api/tests/integration/database_test.go`
- `apps/web/src/app/api/[...path]/route.ts`
- `apps/web/src/components/CatalogWorkspace.tsx`
- `apps/web/src/components/Sidebar.tsx`
- `apps/web/src/lib/catalog.ts`
- `docs/BUILD_READINESS_REPORT.md`
- `docs/INVENTORY_MANAGEMENT.md`
- `docs/PRE_LIVE_SECURITY_REPORT.md`
- `docs/PRODUCT_CATALOG.md`
- `docs/STAGE_10A_READINESS.md`
- `packages/contracts/openapi.yaml`
- `scripts/build-env/artifacts.cjs`
- `scripts/build-env/artifacts.test.cjs`
- `scripts/check-openapi.mjs`
- `apps/api/internal/catalog/import.go`
- `apps/api/internal/catalog/onboarding.go`
- `apps/api/internal/catalog/testdata/synthetic-electronics.xlsx`
- `apps/api/internal/catalog/xlsx.go`
- `apps/api/internal/catalog/xlsx_test.go`
- `apps/api/internal/http/onboarding.go`
- `apps/api/migrations/00013_store_onboarding_catalog_import.sql`
- `apps/api/tests/integration/store_import_test.go`
- `apps/web/public/fixtures/synthetic-electronics.xlsx`
- `apps/web/src/app/dashboard/onboarding/page.tsx`
- `apps/web/src/components/StoreOnboarding.tsx`
- `apps/web/tests/onboarding-production.spec.ts`
- `docs/CATALOG_SOURCE_POLICY.md`
- `docs/EXCEL_CATALOG_IMPORT.md`
- `docs/STAGE_10A_REPORT.md`
- `docs/STAGE_10B_READINESS.md`
- `docs/STORE_ONBOARDING.md`
- `docs/STORE_ONBOARDING_E2E_REPORT.md`
- `scripts/create-demo-catalog.py`

Cleanup завершён: собственные stage10a synthetic containers/networks/volumes удалены, observer остановлен. Build workspace и evidence сохранены. Рабочий стек не изменялся. Финальная сверка16 protected files:0 изменений.
