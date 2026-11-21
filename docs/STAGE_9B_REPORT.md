# Этап9Б — Financial Safety & Approved Templates

## Статус

- WhatsApp financial admission: **IMPLEMENTED offline**.
- Reservation/settlement/DELIVERY_UNKNOWN: **PASS**, actual PostgreSQL и локальные HTTP emulators.
- Registry/template validation/pricing policy: **IMPLEMENTED / PASS offline**.
- Telegram regression/WhatsApp offline/Go tests/race/vet/build/PG migrations/RLS/frontend lint/typecheck/OpenAPI/production build/browser: **PASS** (команды и пределы ниже).
- FORCE RLS **37 tenant-owned таблиц**, включая5 новых.
- SECURITY **CONDITIONAL**. CRITICAL0/HIGH0/LOW OPEN1: E01. Новых CRITICAL/HIGH в выполненном scope не обнаружено; это не исчерпывающий аудит/production-ready сертификат.
- EXTERNAL CALLS **NONE**. LLM/WhatsApp budgets **0 RUB**. Live integrations **DISABLED**.

## Что реализовано

Миграция00011: wa_budgets, wa_pricing, wa_templates, wa_reservations, wa_financial_events с FORCE RLS, owner-read/deny ordinary writes, tenant-safe FK/unique/index. Global wa_platform_budget без runtime SELECT/DML и закрытая wa_function_backup для точного восстановления функций. Nullable trusted recipient_region, исходно неизвестен. Старые00001–00010 не менялись; рабочая БД не мигрировалась.

Существующие Channel Layer/Agent/catalog/orders/Telegram не дублировались. FinancialAdmission интегрирован в прежний WhatsApp outbox под pinned integration lock, с транзакционными wallet locks, proof/tenant/phone/WABA/version/recipient/body/lease/attempt binding. Unknown cost/expired pricing/unknown region/currency/kill/zero paid budget блокируют отправку. Дубликаты/параллельный допуск не резервируют повторно. Crash и timeout не считаются бесплатной операцией; unknown удерживает сумму, recovery пакетами100 с SKIP LOCKED. Owner reconciliation требует версии/подтверждения/evidence hash и не отправляет сообщение.

Реальный Cloud constructor допускает только durable PostgreSQL FinancialAdmission, отвергает synthetic permit. Offline constructor ограничен literal loopback и не может случайно использовать Graph через DefaultTransport. HTTP-транспорт не выполняет автоматический retry неизвестной отправки. Provider reject/429 без подтверждения отсутствия расхода также оставляет деньги unknown; delivery и финансовый результат разделены. Charged при success — conservative verified maximum **оценка**, не утверждение о фактическом счёте Meta. Релиз допускается по backend-only NoChargeConfirmed, не из UI/LLM/webhook.

TemplateVerifier + MockTemplateVerifier сохраняют source=SYNTHETIC_MOCK, не реальные Meta approvals. Owner не может произвольно присвоить APPROVED. BODY/text parameter schema, WABA/name/language/freshness/components/status/ценовая политика повторно проверяются **перед** send. Typed QueueTemplate использует прежний channel_outbox; вне free-text window только template path с отдельным TemplateSender и financial admission, без fallback в text.

Pricing policy configurable/versioned/source-verified, actual реальные тарифы не внесены. Синтетические положительные units существуют только в isolated integration fixtures, realm не допускается real constructor. Даже verified-free0 требует отдельного подтверждённого правила и явной классификации; отсутствие цены не равно0. Runtime и browser wallets0, real pricing rows0, flags mock/false/killtrue подтверждены.

## API и UI

Owner/test-org/session/CSRF boundary сохранён, BFF тот же. Новые endpoint:

- GET `/api/v1/organizations/{id}/integrations/whatsapp/financial` — budget/available/reserved/conservative spent/blocked/reasons/reservations.
- GET `…/templates` — readonly verified metadata/status/source/synthetic/freshness/unavailable reason.
- POST `…/financial/check` — диагностика; в текущем режиме всегда403 LIVE_DISABLED, не reservation/send.
- POST `…/financial/{resource}/reconcile` — owner-only unknown reconciliation, expected_version/amount/evidence/explicit confirmation.

Нет endpoint повышения бюджета или approval override; QueueTemplate internal-only. OpenAPI0.9.2 проверен, будущие Meta API не выдаются за реализованные публичные routes. UI показывает live disabled,0 RUB/reserved/available/blocked reasons, readonly templates с датой/категорией/языком/явным SYNTHETIC. Никаких secrets/live activation controls.

## Проверки и результаты

|Проверка|Результат и предел|
|---|---|
|Go tests, vet, formatting, build API/worker/migrate/healthcheck|PASS; локальные caches, CGO0 binaries|
|Go race detector|PASS actual -race -count=1 ./..., GCC15.2/Go1.26.2 linux/amd64|
|PostgreSQL integration с race|PASS, финальный лог integration-final.log; не mock DB|
|Migrations|PASS clean/repeat/concurrent up, empty down/up,11→10→9→11 с existing synthetic Telegram/orders, populated down refusal; verified recipient metadata rollback refusal|
|Financial concurrency, unknown, crash/recovery|PASS —20-case matrix + дополнительные free/credential/unknown-reject/cancel/conservation assertions|
|Template contracts|PASS —18-case matrix + actual Cloud serializer/DB/local HTTP + worker queue/revocation|
|Runtime role RLS/tenant isolation|PASS37, outsider SELECT/INSERT/UPDATE/DELETE, WITH CHECK/privilege denial, no bypass/owner/pool leak|
|Global wallet confidentiality|PASS actual aba_app SELECT got permission denied|
|Telegram/WhatsApp mock regression|PASS existing Go/PG3shops×2channels и current browser|
|Frontend lint/typecheck|PASS|
|OpenAPI validation|PASS; implemented endpoint set matched|
|Native/provenance script tests|PASS6, missing/stale/tampered proof still blocks|
|Fresh Next production build|PASS, standalone/static packaged, source+served-marker gate|
|Browser|PASS1 expanded test,6 store/channel flows plus9Б finance/templates/auth checks,0 skips/errors/flaky|
|Client secret scan|PASS limited48 static files,0 exact local-env secret/private-key/token pattern matches|
|Post-browser integrity|PASS paths/kinds/contents/proof,2633 records,0 metadata differences after captured E01 repair|
|Real API/prices/TLS/Meta approval/delivery|NOT RUN — запрещены|
|CI external run/production deployment|NOT RUN — не запускались|

Build `hlcpXOCjKuefEXqXOc2kM`, created `2026-10-26T09:51:05.695Z`, app source SHA256 `5eeea8c0cd77ba09034729643a6efd64fa97ff662409a7e90a52e1fefd66f6ed`. Browser start `2026-10-26T10:07:30.278Z`, duration 379.681s. Настоящая новая production build, не cached frontend. Тесты/root Playwright config не являются app inputs; исправления тестового ожидания не перезаписывали app proof, source/runtime изменения по-прежнему блокируются.

Три независимых test shops A/B/C; owner видит0 RUB и SYNTHETIC APPROVED/PENDING/PAUSED, может только проверить ограничения (403). POST APPROVED через BFF получает404, последующий GET доказывает PENDING неизменным. Foreign budget/registry404, member financial403. Старые catalog/consent/order/staff reserve/handoff/window/receipts/channel isolation прошли в том же успешном прогоне. Привилегированный fixture helper строго проверяет aba_test/postgres/test-org/name prefix; не входит в API/worker, не заменяет runtime-role RLS tests.

## E01 / исправления при разработке

E01 **OPEN**: до browser повторно появился пустой `.next/standalone/.next 2`, node_modules2 отсутствовал. Сохранены stat/birth/ctime/mtime/zero children, полный manifest и diagnostic hash; без ровно этого empty path hash совпал с оригинальным proof. Birthtime09:24:47Z старше proof09:51:05.695Z, поэтому не выдаётся за точное время/процесс появления после build. `ps` host sandbox запретил, lsof не дал открытых handles; привилегии не повышались, источник NOT VERIFIED. После forensic snapshot удалён **только пустой generated path через rmdir**, original proof не менялся, строгий verify PASS. Включён локальный polling observer `.local/audit9b/fs-events.jsonl`, фиксирующий metadata/phase; он не позволяет достоверно приписать host PID. После successful browser paths/files/metadata/proof неизменны. Никаких широких исключений; no CRITICAL/HIGH signs от этого наблюдения.

Сохранены неудачные development прогоны: wrong session-dependent helper в worker gate (fixed proof-bound organization check + exact denial reasons); PostgreSQL ARE regex bound512 (fixed alphabet+length и512-name regression); OpenAPI empty required array (fixed contract); cross-case held total10 vs20 (fixed delta assertion, не финансовое освобождение); browser expected405 vs actual secure BFF404 (fixed expectation и доказательство неизменённого PENDING). Failure evidence не засчитан PASS. Temporary diagnostic reproducer удалён из product tests, archived local text only.

## Изоляция и сохранность

Все dependency downloads запрещены/не выполнялись. Cached images pull never; internal Docker networks IPv6false/no external default route/no host ports/docker socket. Proxy/toolchain/package downloads/telemetry выключены; routing проверяется build scripts, Go deps offline. Реальные credentials/customer data не использовались; raw traces только synthetic sessions и не предназначены для публикации. Host node_modules/toolchain не переустанавливались, lockfile/Go modules/старые миграции сохранены. Рабочие ai-business-assistant-* containers/volumes не менялись. Только own stage9b synthetic resources удаляются после final checks; image/workspace/evidence сохраняются.

## Воспроизведение/evidence

Compose files: infrastructure/compose.environment.yml + compose.environment-smoke.yml + compose.prepared-build.yml; ignored env `.local/audit8d/compose.env`. Projects aba-stage9b, aba-stage9b-check (fresh clean DB), aba-stage9b-build, aba-stage9b-current.

```sh
docker compose --env-file .local/audit8d/compose.env -p aba-stage9b -f infrastructure/compose.environment.yml -f infrastructure/compose.environment-smoke.yml -f infrastructure/compose.prepared-build.yml run --rm race
docker compose --env-file .local/audit8d/compose.env -p aba-stage9b-check -f infrastructure/compose.environment.yml -f infrastructure/compose.environment-smoke.yml -f infrastructure/compose.prepared-build.yml run --rm integration
docker compose --env-file .local/audit8d/compose.env -p aba-stage9b-build -f infrastructure/compose.environment.yml -f infrastructure/compose.environment-smoke.yml -f infrastructure/compose.prepared-build.yml run --rm web-build
```

Browser prepared service with `--grep 'current production'`, fixture env and current artifacts; report `.local/audit9b/browser-final.json`, screenshots/traces `.local/audit8d/browser-results/stage9b-multichannel-final/`. Extra /packages RO mount required for OpenAPI web-check command. Compiled synthetic browserfixture helper fresh; no live key. Complete raw logs/proofs/RLS inventory, forensic manifests, secret scan, source-preservation/diff `.local/audit9b/`. Helper top-level SKIPs — subprocess entrypoints, parent crash tests actually PASS; not missing assertions marked successful.

## Критерии A–T

A–J: PASS offline (durable ledger/settlement/unknown/registry/fresh validation/unapproved+unverified+synthetic live denial/tenant/RLS). K–N: PASS (Telegram/WhatsApp tests, fresh build, current browser). O: новых CRITICAL/HIGH не обнаружено в scope; security CONDITIONAL/E01 OPEN. P–S: PASS (live disabled, runtime budgets0, working data preserved, no commit/push/deployment). T: PASS — STAGE_9C_READINESS создан. Это не live acceptance.

## Следующий шаг

Telegram/WhatsApp live **NOT READY**. Нужны отдельное разрешение, реальные verified pricing/free-case/region и template provider, текущая официальная версия/политика, data policy/operator/HTTPS/live tests. Новая financial foundation готова к их **отдельной** локальной подготовке; actual pricing/Meta approval не вымышлены. E01 требует дальнейшего filesystem/process audit без расширения прав автоматически. См. STAGE_9C_READINESS. После отчёта остановиться, реальных подключений не выполнять.

## Финальная сверка файлов

Изменено 19, создано 11, удалено 0; полный список:

- `README.md`
- `docs/WHATSAPP_CLOUD_API_TRANSPORT.md`
- `docs/PRE_LIVE_SECURITY_REPORT.md`
- `docs/BUILD_READINESS_REPORT.md`
- `scripts/check-openapi.mjs`
- `packages/contracts/openapi.yaml`
- `apps/web/tests/multichannel-production.spec.ts`
- `apps/web/src/app/dashboard/integrations/whatsapp/page.tsx`
- `apps/web/src/app/api/[...path]/route.ts`
- `apps/api/tests/browserfixture/main.go`
- `apps/api/tests/integration/audit_test.go`
- `apps/api/tests/integration/database_test.go`
- `apps/api/internal/platform/db/db.go`
- `apps/api/internal/channel/channel.go`
- `apps/api/internal/http/whatsapp.go`
- `apps/api/internal/whatsapp/cloud.go`
- `apps/api/internal/whatsapp/worker.go`
- `apps/api/internal/whatsapp/adapter.go`
- `apps/api/internal/whatsapp/cloud_test.go`
- `docs/WHATSAPP_FINANCIAL_ADMISSION.md`
- `docs/WHATSAPP_PRICING_POLICY.md`
- `docs/WHATSAPP_TEMPLATE_REGISTRY.md`
- `docs/STAGE_9B_REPORT.md`
- `docs/WHATSAPP_TEMPLATE_TEST_REPORT.md`
- `docs/WHATSAPP_FINANCIAL_TEST_REPORT.md`
- `docs/STAGE_9C_READINESS.md`
- `apps/web/src/components/WhatsAppSafety.tsx`
- `apps/api/migrations/00011_whatsapp_financial_templates.sql`
- `apps/api/tests/integration/whatsapp_financial_test.go`
- `apps/api/internal/whatsapp/financial.go`

Финальный PostgreSQL/race прогон после усиления rollback guard: PASS44.742s. Собственные stage9b containers/networks/volumes удалены; observer завершён. Рабочий стек и данные сохранены. E01 OPEN, текущий post-browser proof и2633 entries неизменны.


## Повторная offline-проверка 2026-10-28

По повторному заданию 9Б существующая реализация проверена без дублирования бизнес-логики и без изменений прикладного кода. Исходное дерево уже содержало все 30 файлов реализации, перечисленных выше. Сравнение с сохранённым baseline: 19 изменённых файлов, неожиданных изменений нет; 14 защищённых файлов (миграции 00001–00010, Go modules и frontend lockfile, присутствующие в baseline) неизменны.

Текущие результаты, а не перенос прежних PASS: Go tests/race/vet/build — PASS; PostgreSQL integration с race — PASS 59.480s, включая migrations/rollback, financial concurrency/crash/recovery, template contracts, Telegram/WhatsApp regression и runtime FORCE RLS 37 таблиц. Subprocess helper SKIP остаются служебными entrypoints; родительские crash-сценарии выполнены. Frontend lint/typecheck/OpenAPI — PASS; шесть build guard/native tests — PASS.

Свежая production-сборка PASS: BUILD_ID `oBqtgGCX-OU0t3jYnOX8V`, создана `2026-10-28T09:47:20.829Z`, app source SHA256 `5eeea8c0cd77ba09034729643a6efd64fa97ff662409a7e90a52e1fefd66f6ed`. Browser current-production — PASS: 1 expanded test, 0 skipped/unexpected/flaky, старт `2026-10-28T09:52:37.999Z`, 345.065s. Проверены три синтетических магазина, оба mock-канала, budget0/registry/forbidden approval/403 live check/foreign tenant и существующий consent/reserve/handoff/window flow. Build/served-marker gate перед тестом PASS. Limited secret scan48 client files — 0 findings.

**E01 OPEN / SECURITY CONDITIONAL:** после E2E найден пустой `.next/standalone/node_modules 2`; `.next 2` отсутствовал. Строгий guard правильно завершился42. Сохранены полный manifest, birth/ctime/mtime, empty children и неизменённый original proof. Изменились метаданные2634 существующих записей; иных path/type/content изменений нет. Birthtime пустого каталога старше текущей build: это не доказательство точного времени появления. `lsof` нашёл read-handles у PID68998, `/System/Library/Frameworks/Virtualization.framework/Versions/A/XPCServices/com.apple.Virtualization.VirtualMachine.xpc/Contents/MacOS/com.apple.Virtualization.VirtualMachine`; это наблюдение, **не доказанная причинная атрибуция**. После snapshot удалён только пустой каталог через `rmdir`. Original proof не перезаписан; exact content/path/type baseline, strict verify и served marker повторно PASS. Исключения/guard не ослаблялись. CRITICAL/HIGH новых не выявлено в проверенном scope; LOW OPEN1.

Evidence: `.local/audit9b-recheck/` — results.json, integration-race.log, rls_inventory.json, browser.json, build-proof.json, integrity.json, before/after manifests, metadata-events.jsonl, e01-capture.json, e01-process.json, preservation.json, client-secret-scan.json. Browser screenshots: `.local/audit8d/browser-results/stage9b-recheck/`. Команды те же prepared Compose, отдельные projects `aba-stage9b-recheck`, `aba-stage9b-rebuild`, `aba-stage9b-rebrowser`, cached images/pull never/internal networks/no host ports/no downloads. Проверено: network internal=true/IPv6=false, runtime mock/live=false/kill=true, platform WA limit0, nonzero tenant WA budgets0, real pricing rows0. Внешние API/реальные токены/данные/платные операции/commit/push/deployment не использовались; LLM/WhatsApp0 RUB. Рабочий стек не изменялся.

В этом повторном проходе изменены только6 документов: `docs/STAGE_9B_REPORT.md`, `docs/WHATSAPP_FINANCIAL_TEST_REPORT.md`, `docs/WHATSAPP_TEMPLATE_TEST_REPORT.md`, `docs/PRE_LIVE_SECURITY_REPORT.md`, `docs/BUILD_READINESS_REPORT.md`, `docs/STAGE_9C_READINESS.md`. Новых миграций нет; этап использует существующую00011. Telegram/WhatsApp live NOT READY. Следующий шаг — отдельное offline ТЗ9В на trusted provider/policy provisioning и operator workflow; реальные тарифы/approval/version/legal/live проверки остаются блокерами.

Cleanup повторного аудита: собственные test containers/networks/volumes трёх projects удалены; observer завершён. Evidence/build workspace сохранены.
