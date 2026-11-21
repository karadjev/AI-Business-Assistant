# Этап10Б — Smart Catalog Control Center

Реализован единый «Центр управления каталогом» в существующем Dashboard: backend metrics/health, common inventory safety, source handoff/history, import history и scoped retention worker. Live не включается; бюджеты0 RUB. Все обязательные offline gates PASS; Security CONDITIONAL из-за E01.

## Аудит и архитектура

Проверены10А/readiness10Б/onboarding/Excel/source/9В/final offline gate/catalog/inventory, фактические API/auth/RLS/migrations/tests. Исходно43 FORCE RLS tables, migration13. Не обнаружено открытых релевантных CRITICAL/HIGH; E01 LOW остаётся OPEN.

Сохранены organizations/memberships, один products/variants/inventory каталог, целочисленные копейки, reservation workflow, customer facade, common agent/order/channel/outbox, durable import jobs и append-only catalog_events. Новых пользователей/магазинов/каталогов/очередей нет. Dependencies/lockfiles не меняются.

Migration00014: tenant policies и source operations/history (+2 FORCE RLS,45 total), price timestamp, import receipt/retention metadata/FAILED/EXPIRED, scoped worker capability, source-only handoff и queued stock recheck triggers. Старые00001–00013 не менялись. Guarded down сохраняет populated catalog history. Runtime aba_app не owner/BYPASSRLS.

## Поведение

- Backend рассчитывает реальные counters и catalog-health-v1; пустой каталог→NULL score. Metric semantics/weights/rounding описаны в CATALOG_HEALTH_SCORE. No data не маскируется fake values.
- Freshness отличается от product/price/import timestamps. Не-FRESH available=NULL в customer search/details/inventory; stock в учёте сохраняется. Общий Runner, orders staff confirmation и stock reserve fail closed; оба outbox повторно проверяют подготовленные факты при dispatch. Quote/request требуют прежнего consent, неизвестный delivery не повторяется.
- Source change — owner preview/explicit confirm, stable snapshot/versions/locks/idempotency/audit. Все группы одного товара передаются вместе, значения не меняются. ONE_C не активируется. Прямой manual overwrite EXCEL/ONE_C остаётся409.
- Import history/result/details/audit/hold расширяют существующие jobs. Повреждённый XLSX оставляет safe FAILED receipt и422. Repeated import остаётся идемпотентным; подтверждённый авторитетный EXCEL snapshot обновляет дату проверки даже при неизменённом/нулевом количестве.
- Cleanup отделяет grid, diagnostics, applied lines и минимальную историю, не удаляет audit/financial/source history. Owner upload/save привязывает hash внутренней domain-separated worker capability; каналы для cleanup не нужны. SET LOCAL scope/RLS, batch20/SKIP LOCKED/transactional audit/hold/replay. Raw XLSX никогда не сохраняется, общего filesystem cleanup нет.
- UI: реальные cards/Action Center/таблица/filters/search/pagination/history/source comparison/confirmation/settings, skeleton/empty/error/status/keyboard focus; desktop/mobile. Общий token muted усилен для контраста, logout не сжимается на mobile. Новых библиотек/дизайн-языка нет.

## Качество и границы

Go unit/race/vet/build PASS. Actual PostgreSQL integration with race PASS71.086s,45-table FORCE RLS inventory, outsider CRUD/reparent/pool isolation, migrations up/repeat/concurrent/empty down+up/populated refusal. New tests: health/policy, stale masking/fresh verification/future UNKNOWN/AGING reserve denial, owner/tenant/source concurrency/replay/stale snapshot/rollback, cleanup proof/no-channel/hold/concurrency/active import/in-use row lock/audit rollback, delayed stock recheck TG/WA. Existing upload/consent/catalog/reservation/provider/financial/unknown crash suites PASS. Subprocess helper SKIP — служебные helpers, parent process-kill tests реально выполнены.

Frontend lint/typecheck/OpenAPI0.10.1/native/artifact tests6 PASS. Fresh Next production build PASS: `Hf5QQTkeHMFwonl2YwQ3G`, created `2026-10-28T15:27:12.048Z`, source SHA256 `4d57a84261fd6222df69f663e66bf357f6bb028ed64b365e38d818c4c2264e8f`. Browser current production PASS3,0 skipped/unexpected/flaky, 557.675s; start `2026-10-28T15:31:06.747Z`. Desktop/mobile390×844 и все12 acceptance scenarios PASS. До финальных CSS-правок отдельный полный production run тоже3 PASS554.357s; он не подменяет окончательный прогон.

Новый cleanup crash case — injected PostgreSQL failure после UPDATE до audit и проверка rollback; не выдаётся за отдельный OS-kill нового cleanup процесса. Existing channel/budget OS-crash tests остаются выполненными. Platform/privacy/live/real1C/official Meta proofs запрещены и NOT RUN.

Known bounds: overview≤5000 variants (явный отказ, не усечённая оценка), history recent100, одна tenant freshness policy плюс прежний stricter environment cap. Source handoff product-wide. Старые tenant jobs получают новую cleanup binding после следующего owner upload/save; rotation требует такой перепривязки, global обхода нет. Минимальные receipts/source/security/financial audits не удаляются до отдельного policy решения. Это границы пилота, не разрешение live.

## Security / E01

SECURITY CONDITIONAL. CRITICAL/HIGH/LOW OPEN0/0/1 в проверенном scope; E01 OPEN. Source/financial/upload isolation не ослаблялись. Финальная synthetic DB: schema14, real pricing0, non-test stores0, live rows TG/WA/AI0, WA nonzero wallets0, LLM limit/spent0 и kill=true. Static client secret scan52 files/0 findings. Protected16 files (13 old migrations+embed+go.mod/go.sum) unchanged; package-lock совпадает с ранее проверенным неизменённым baseline. No external APIs/downloads/real credentials/customer data/paid operations/commit/push/deployment. Только собственные synthetic test DBs, рабочий стек не менялся.

E01 воспроизведён: ровно2 новых empty paths standalone/.next 2 и node_modules 2,0 content changes,2657 existing records metadata changed. Два production-прогона повторили E01. Strict guard отказал42; manifests/timestamps/lsof сохранены. После capture удалены только эти пустые paths через rmdir, original proof не переписан; verify/served перед следующим browser PASS; после финального E2E повторно42 → exact empty rmdir → неизменённый original proof/served marker PASS. Content baseline снова совпадает. Read handles у macOS Virtualization PID68998, causal writer не доказан. Production scripts делают обычный mkdir/cp, runtime mounts RO. Исключений/подавления проверки нет.

Development failures не скрыты: cleanup INSERT RETURNING потребовал запрещённое audit SELECT (запрос исправлен без выдачи прав); form serializer послал extra version (исправлен UI); test selectors/rate pacing/timeout и повторный synthetic bot fixture исправлены. Один race runner не стартовал из-за исчерпания Docker address pool; удалены только собственные завершённые stage10b сети, повтор race PASS.

## Evidence / следующий этап

Ignored `.local/audit10b`: source/protected baseline, PG failed/final logs,45-table inventory, build proofs, metadata observer/manifests/E01 repair/read handles, limited client-secret scan/results/source changes. Browser screenshots/traces только synthetic, в `.local/audit8d/browser-results/`; sessions не публикуются.

Telegram live NOT READY; WhatsApp live NOT READY. Оба выключены, kill switches true, budgets0. Следующий шаг — отдельный offline10В: operator key rotation procedure, larger-catalog queries/history, согласованная audit retention; реальные1С/Meta/LLM требуют отдельного разрешения и live blockers из9В.

## Итоговые статусы

| Поле | Итог |
|---|---|
| STAGE |10Б|
| CATALOG CONTROL CENTER |IMPLEMENTED|
| CATALOG HEALTH SCORE |PASS|
| INVENTORY FRESHNESS |PASS|
| AI INVENTORY SAFETY |PASS|
| SOURCE SWITCHING |PASS|
| SOURCE CONFLICT RESOLUTION |PASS|
| IMPORT HISTORY |PASS|
| RETENTION POLICY |PASS|
| CLEANUP WORKER |PASS|
| TELEGRAM REGRESSION |PASS|
| WHATSAPP REGRESSION |PASS|
| GO TESTS/RACE/VET/BUILD |PASS|
| NEXT.JS PRODUCTION BUILD |PASS|
| BROWSER E2E |PASS3;0 skipped/flaky|
| MOBILE UX |PASS390×844; visual+overflow checks|
| RLS |45 tables, PASS|
| SECURITY STATUS |CONDITIONAL|
| CRITICAL/HIGH/LOW OPEN |0/0/1|
| E01 |OPEN; captured recurrence, guard42, exact repair, unchanged proof verified|
| EXTERNAL CALLS |NONE|
| LLM BUDGET |0 RUB|
| WHATSAPP BUDGET |0 RUB|
| MIGRATIONS |00014_catalog_control_center.sql|
| FILES CHANGED |43:27 modified,16new,0removed; list below|
| LIVE READINESS |Telegram NOT READY; WhatsApp NOT READY; disabled|
| REMAINING BLOCKERS |No required offline10Б blockers; E01 root cause, existing9В live blockers remain|
| NEXT STEP |Offline10В: capability rotation procedure, larger catalog/history bounds, separate audit retention policy|

## Source files changed

Изменено 27, создано 16, удалено 0. Всего 43; generated .local cache/build/evidence исключены.

- `apps/api/cmd/worker/main.go`
- `apps/api/internal/agent/tools.go`
- `apps/api/internal/catalog/import.go`
- `apps/api/internal/catalog/service.go`
- `apps/api/internal/catalog/stock.go`
- `apps/api/internal/catalog/testdata/synthetic-electronics.xlsx`
- `apps/api/internal/http/onboarding.go`
- `apps/api/internal/orders/service.go`
- `apps/api/internal/platform/db/db.go`
- `apps/api/tests/browserfixture/main.go`
- `apps/api/tests/integration/audit_test.go`
- `apps/api/tests/integration/store_import_test.go`
- `apps/web/public/fixtures/synthetic-electronics.xlsx`
- `apps/web/src/app/api/[...path]/route.ts`
- `apps/web/src/app/globals.css`
- `apps/web/src/components/Sidebar.tsx`
- `apps/web/src/components/StoreOnboarding.tsx`
- `apps/web/tests/multichannel-production.spec.ts`
- `docs/BUILD_READINESS_REPORT.md`
- `docs/CATALOG_SOURCE_POLICY.md`
- `docs/EXCEL_CATALOG_IMPORT.md`
- `docs/INVENTORY_MANAGEMENT.md`
- `docs/PRE_LIVE_SECURITY_REPORT.md`
- `docs/PRODUCT_CATALOG.md`
- `packages/contracts/openapi.yaml`
- `scripts/check-openapi.mjs`
- `scripts/create-demo-catalog.py`
- `apps/api/internal/catalog/control.go`
- `apps/api/internal/catalog/control_test.go`
- `apps/api/migrations/00014_catalog_control_center.sql`
- `apps/api/tests/integration/catalog_control_test.go`
- `apps/web/src/app/dashboard/catalog/loading.tsx`
- `apps/web/src/app/dashboard/catalog/page.tsx`
- `apps/web/src/components/CatalogControlCenter.tsx`
- `apps/web/tests/catalog-control-production.spec.ts`
- `docs/CATALOG_CONTROL_CENTER.md`
- `docs/CATALOG_CONTROL_CENTER_E2E.md`
- `docs/CATALOG_HEALTH_SCORE.md`
- `docs/CATALOG_SOURCE_SWITCHING.md`
- `docs/IMPORT_RETENTION_POLICY.md`
- `docs/INVENTORY_FRESHNESS_POLICY.md`
- `docs/STAGE_10B_REPORT.md`
- `docs/STAGE_10C_READINESS.md`

Cleanup завершён: собственные stage10b containers/networks/volumes отсутствуют, observer остановлен, evidence/build workspace сохранены. Рабочий стек не изменён. Commit/push/deployment не выполнялись.
