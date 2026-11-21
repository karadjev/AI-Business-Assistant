# Этап9В — Trusted Provider Provisioning & Financial Reconciliation

Реализована offline основа доверенных данных и операторской сверки в существующей архитектуре9Б. Реальные provider integrations остаются NOT READY и выключены, оба бюджета0 RUB. Реальные APIs не вызывались.

## Аудит и архитектура

Изучены отчёты9А/9Б/readiness и финансовая/template/transport/security/build документация. Повторно использованы FinancialAdmission, wa_pricing/wa_templates/wa_reservations/wa_financial_events, Channel Layer/outbox/lease/fencing/integration lock, tenant auth/session/CSRF/BFF. Параллельного агента, ledger, очереди или отдельной WhatsApp бизнес-логики нет.

Недостающими механизмами были проверяемое provenance и revocation, отдельное назначение platform operator и evidence-based workflow вместо owner-only settlement. Карта компонентов и границы — TRUSTED_PROVIDER_PROVISIONING.md.

Миграция `00012_whatsapp_trusted_reconciliation.sql` добавляет3 tenant-owned FORCE RLS таблицы: wa_operator_grants, wa_provider_records, wa_reconciliation_evidence. Всего40 tenant-owned tables. Tenant-safe FK/unique/index, транзакции, immutable runtime evidence, operator scope и guarded down. Старые00001–00011 не редактировались. Прежняя закрытая function backup хранит точные определения predecessor functions; новых global data tables нет.

PricingPolicySource и TemplateStatusSource разделены. Только mock implementations, принудительно SYNTHETIC_TEST/OFFLINE_DIGEST, no live. MANUAL_UNVERIFIED/VERIFIED_OPERATOR не Meta approval. OFFICIAL_PROVIDER import намеренно отвергается до реальной проверяемой реализации. Простой verified=true/source label либо resource-row write не создаёт provenance. Reserve/template-check повторно проверяют digest, source, expiry/revocation; overlapping valid pricing policies блокируются. Никаких актуальных реальных цен/Graph version не вымышлено.

Owner mutation9Б закрыта. Назначенный platform operator использует ту же session/tenant boundary, но отдельный expiring grant; staff/store owner не могут самоназначиться. Grants не выдаются через UI/API. После ожидания integration/wallet locks полномочия проверяются повторно и удерживаются SHARE lock до commit. Evidence и optimistic version/event digest обеспечивают single financial effect, crash/rollback safety и сохранение отказов в audit. DELIVERY_UNKNOWN не становится delivered и не уходит на resend. Позднее противоречащее provider событие аудируется отдельно; автоматического финансового исправления нет. Пустой provider ID нормализуется вNULL, не конкурирует с настоящим unique provider identifier.

Новый UI `/dashboard/operations/whatsapp?org=<assigned tenant>` показывает store/channel/unknown/reserve/timestamp/evidence/history и требует основание+подтверждение. Owner UI показывает readonly provider readiness/source/freshness и причины блокировки; кнопок live/self-approval/budget override нет. API contracts/BFF обновлены до0.9.3. Реальный operator provisioning и evidence/invoice authentication требуют отдельного control-plane/operator review перед live.

## Проверки

Go tests/race/vet/build PASS. Финальный PostgreSQL integration/race PASS37.996s: clean/repeated/concurrent migration up, empty down/up,12→11→10→9→12 с сохранением синтетических Telegram/orders, populated down refusal, runtime RLS40, финансовая конкурентность, recovery/crash, trusted provider/templates/evidence и Telegram/WhatsApp regression. Служебные subprocess helper SKIP не являются пропущенными parent crash scenarios.

Frontend lint/typecheck/OpenAPI и6 native/provenance guard tests PASS. Fresh Next production build PASS: `OetOwOfFcsPr2uyFJKXch`, created `2026-10-28T10:52:22.135Z`, app source SHA256 `11d97f0bd23118cfad5f251eb8d8c0b2283bc0c9dc191aa37676ffa5c556307a`. Browser final/evidence/integrity см. финальное дополнение ниже; interrupted attempt не засчитан PASS.

Trusted provisioning18 требований покрыты `whatsapp_provisioning_test.go`: synthetic/verified + tamper; MANUAL_UNVERIFIED/VERIFIED_OPERATOR/OFFICIAL_PROVIDER denial; expired/revoked; currency/region/category; conflicting versions; identical import; owner/raw RLS update denial; synthetic template verified; template revoke/expiry/WABA; tenant/integration mismatch; concurrent PAUSED/REJECTED; invalid digest. Unit mock test доказывает принудительную synthetic классификацию. Existing real constructor tests запрещают synthetic live.

Reconciliation18 требований: INSUFFICIENT/CONFIRMED_SENT/CONFIRMED_NOT_SENT/unknown; same event replay/different payload conflict; two independent operators; wrong tenant including separately assigned foreign tenant; staff/owner denial; missing evidence/incorrect zero release; conflicting provider ID; existing worker crashes; transaction rollback; duplicate settlement; late provider contradiction audit; immutable runtime evidence/RLS/audit integrity; grant expiry while waiting for worker lock. Parent ledger conservation asserts aggregate reserved/spent match all reservation states.

Финальный SQL включает нормализацию пустого provider ID, grant-lock fencing и late evidence trigger. Последующие Go tests/vet/build/race выполнены после этих изменений. Browser повторно запускается на новой test DB со свежими final API/worker binaries; frontend build не менялся. Исправления browser ожиданий в tests не меняют app build proof.

## E01 и development evidence

E01 LOW OPEN. После build до browser воспроизвелись два пустых каталога `.next 2`/`node_modules 2`, metadata changes2642. Strict guard отказал42; full snapshot показал только2 новых empty path, содержимое остальных records неизменно. Capture/ctime/mtime/birthtime/time sequence/process handles/mounts сохранены. Birthtimes старше build, поэтому точный causal creation moment не заявляется. PID68998 macOS Virtualization держит read-handles; read-only web bind mount не является доказательством causal writer. Scripts не создают suffixed paths. Удалены только доказанно пустые generated paths через rmdir, original proof не менялся, whitelist не расширялся.

Сохранены неудачные development runs: initial rollback guard пропускал откат при recipient metadata — усилен; empty provider string ошибочно считался подтверждением — nullif guard; privileged synthetic fixture с несколькими empty unique provider IDs — нормализация finish и точечный fixture; interrupted browser точный label selector включал option text — заменён combobox role, добавлено ожидание обновлённой истории. Первый исправляющий локальный Python snippet завершился SyntaxError до записи, повтор выполнен корректно. Неудачные/прерванные runs не выданы за PASS.

Evidence `.local/audit9c/`: baseline, integration attempts/final, RLS inventory, build proof, manifests/metadata observer/E01 captures, interrupted/final browser summaries, preservation и final file changes. Browser screenshots/traces — `.local/audit8d/browser-results/stage9c-*`, только синтетические данные; raw sessions/traces не публиковать как production artifacts.

Команды — existing prepared Compose overlays `compose.environment.yml`, `compose.environment-smoke.yml`, `compose.prepared-build.yml`, env `.local/audit8d/compose.env`, cached images/pull never. Isolated projects prefix `aba-stage9c`, final PG `aba-stage9c-check6`, final browser `aba-stage9c-current2`. Working compose project не использовался. Real Meta/Telegram/LLM/TLS/live pricing/operator external sources NOT RUN — запрещены. Downloads/commit/push/deployment не выполнялись.

## Remaining blockers / следующий этап

Telegram live NOT READY, WhatsApp live NOT READY. Нужны real verified source adapters/contracts, актуальные official docs/version/pricing, operator/control-plane/privacy/invoice procedures, HTTPS/allowlist и отдельное разрешение на внешние тесты/денежный scope. E01 OPEN без доказанного causal writer. Следующий шаг — отдельное offline ТЗ10А, см. STAGE_10A_READINESS.md. После отчёта остановиться.


## Финальный результат9В

TRUSTED PROVIDER PROVISIONING IMPLEMENTED offline; TRUSTED PRICING POLICY PASS; TRUSTED TEMPLATE STATUS PASS; REVOCATION/EXPIRATION PASS; FINANCIAL RECONCILIATION IMPLEMENTED; DELIVERY_UNKNOWN SAFETY/OPERATOR AUTHORIZATION/TENANT ISOLATION PASS. Telegram/WhatsApp regression PASS. Go tests/race/vet/build PASS; PostgreSQL integration/race PASS37.996s; RLS PASS40. Frontend lint/typecheck/OpenAPI/6 guard tests PASS.

**Окончательная свежая Next build:** `X5yxJt--Ha4O28_xPF-Wq`, created `2026-10-28T11:45:52.890Z`, source SHA256 `493133235608cc75a7315f3f0e5ffeb1dbe167ece059279312efbc9398e8a19c`. Эта build заменила первоначальную build выше после перевода operator UUID на crypto.getRandomValues для isolated HTTP. Dependency downloads не выполнялись. Финальный spec синхронизирован в build workspace; проверены combobox role и bounded action/navigation timeouts. App proof исключает tests/config по прежнему правилу, не был переписан ради E2E.

**BROWSER E2E PASS:** старт `2026-10-28T11:46:40.437Z`, duration 359.131s,1 expanded test,0 skipped/unexpected/flaky. Три магазина×2 mock канала +3 operator flows: owner видит live disabled/0RUB/provider blocked; arbitrary approval отвергнут; tenant isolation; existing consent/staff reserve/handoff/window/receipts; owner operator403; assigned operator без store membership видит только назначенный scope; evidence confirmation обязательна; INSUFFICIENT сохраняет резерв/историю; CONFIRMED_NOT_SENT завершает один settlement; outbox остаётся delivery_unknown. Screenshots9 (finance/budgets/operator для каждого магазина).

Второй interrupted browser повторно использовал старую копию spec; после этого выполнены новая build и явная проверка копии spec. Во втором interrupted run teardown сообщил повреждённый trace/file stream; этот trace не считается полным evidence. Оба interrupted прогона не PASS. Финальный browser использует отдельный project `aba-stage9c-current3` с актуальными schema12 и fresh API/worker/migrate binaries. Проверено, что migrate binary содержит final grant fencing/provider-null normalization.

**BUILD INTEGRITY CONDITIONAL / E01 OPEN:** после successful browser оба пустых suffixed каталога повторились. Diff:2642 existing records изменились только metadata, добавлены ровно2 empty directories; file bytes/types/paths иных записей и original proof неизменны. Strict guard правильно отказал42. Сохранены final-integrity/e01-final-capture и timestamps. После удаления **только** двух пустых paths через rmdir строгие verify+served marker PASS; post-repair content/path/type точно совпал с pre-browser baseline. Исключений нет; E01 не закрыт. Process attribution по-прежнему NOT VERIFIED.

Actual final synthetic runtime: schema12; platform WA limit0; nonzero tenant WA wallets0; real pricing0; real templates0; official provenance records0; unsafe tenant RLS tables0. Worker: LLM/Telegram/WhatsApp mock/live false/kill true/external authorization false, оба monetary budgets0. Limited final scan49 static client files:0 findings. Protected14 files (00001–00011/go.mod/go.sum/frontend lockfile):0 изменений. Рабочие данные и рабочий compose stack не затронуты.

SECURITY CONDITIONAL; CRITICAL/HIGH/LOW OPEN0/0/1 (E01) в проверенном scope. EXTERNAL CALLS NONE. Real providers/TLS/pricing/version/operator external evidence NOT RUN — запрещены. LIVE READINESS Telegram NOT READY, WhatsApp NOT READY. NEXT STEP: отдельное offline ТЗ10А из STAGE_10A_READINESS. Реальные API/бюджеты не включать.


## Итоговый список source files

Изменено 21, создано 13, удалено 0. Generated build/cache/evidence в ignored .local не включены.

- `README.md`
- `apps/api/internal/http/whatsapp.go`
- `apps/api/internal/platform/db/db.go`
- `apps/api/internal/whatsapp/financial.go`
- `apps/api/tests/browserfixture/main.go`
- `apps/api/tests/integration/audit_test.go`
- `apps/api/tests/integration/database_test.go`
- `apps/api/tests/integration/multichannel_test.go`
- `apps/api/tests/integration/whatsapp_financial_test.go`
- `apps/web/src/app/api/[...path]/route.ts`
- `apps/web/src/components/WhatsAppSafety.tsx`
- `apps/web/tests/multichannel-production.spec.ts`
- `docs/BUILD_READINESS_REPORT.md`
- `docs/PRE_LIVE_SECURITY_REPORT.md`
- `docs/STAGE_9C_READINESS.md`
- `docs/WHATSAPP_CLOUD_API_TRANSPORT.md`
- `docs/WHATSAPP_FINANCIAL_ADMISSION.md`
- `docs/WHATSAPP_PRICING_POLICY.md`
- `docs/WHATSAPP_TEMPLATE_REGISTRY.md`
- `packages/contracts/openapi.yaml`
- `scripts/check-openapi.mjs`
- `apps/api/internal/whatsapp/provisioning.go`
- `apps/api/internal/whatsapp/provisioning_test.go`
- `apps/api/migrations/00012_whatsapp_trusted_reconciliation.sql`
- `apps/api/tests/integration/whatsapp_provisioning_test.go`
- `apps/web/src/app/dashboard/operations/whatsapp/page.tsx`
- `apps/web/src/components/WhatsAppOperator.tsx`
- `docs/STAGE_10A_READINESS.md`
- `docs/STAGE_9C_REPORT.md`
- `docs/STAGE_9C_SECURITY_REPORT.md`
- `docs/TRUSTED_PRICING_POLICY.md`
- `docs/TRUSTED_PROVIDER_PROVISIONING.md`
- `docs/TRUSTED_TEMPLATE_STATUS.md`
- `docs/WHATSAPP_FINANCIAL_RECONCILIATION.md`

Cleanup: собственные synthetic stage9c containers/networks/volumes удалены; observer завершён; build workspace и evidence сохранены. Рабочий стек не изменялся. После финальной сверки14 protected files неизменны.
