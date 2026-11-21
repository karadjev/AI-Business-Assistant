> Этап10Б: tenant-scoped Catalog Control Center, common freshness gates, source handoff и retention реализованы offline; FORCE RLS45. Security CONDITIONAL, E01 OPEN. Live Telegram/WhatsApp NOT READY, оба бюджета0 RUB. Итоговые проверки: [STAGE_10B_REPORT](STAGE_10B_REPORT.md).

> Актуализация10А: owner onboarding/XLSX mapping/preview/atomic import/source policy IMPLEMENTED offline. Go/tests/race/vet/build, PostgreSQL/race41.489s, FORCE RLS43, frontend lint/typecheck/OpenAPI/6 guard tests PASS. Fresh build Qz7bLOo_mrjL562aXZegn; browser2 expanded tests PASS468.878s/0 skipped/errors/flaky. E01 OPEN: два empty paths повторились, strict guard отказал; forensic capture/exact rmdir и unchanged original proof/served marker после этого PASS. Security CONDITIONAL0/0/1. Live disabled, budgets0, external calls NONE. [STAGE_10A_REPORT](STAGE_10A_REPORT.md). Ниже — история.

> Актуализация9В (2026-10-28): trusted provisioning и operator-only evidence reconciliation IMPLEMENTED offline. Go/tests/race/vet/build, PostgreSQL/race37.996s, FORCE RLS40, frontend lint/typecheck/OpenAPI/6 guard tests PASS. Fresh Next build `X5yxJt--Ha4O28_xPF-Wq` PASS; browser3shops×2channels +3 operator flows PASS1 expanded test/359.131s/0 skips/errors/flaky. E01 OPEN: два empty suffixed paths и metadata2642 повторились; strict guard отказал, forensic capture и exact rmdir выполнены, original proof/served marker после ремонта PASS. SECURITY CONDITIONAL0/0/1. Оба бюджета0 RUB, live disabled, external calls NONE. Подробности [STAGE_9C_REPORT](STAGE_9C_REPORT.md); старые результаты ниже — история.

> Актуализация9Б (2026-10-26): durable WhatsApp ledger/approved-template registry и миграция00011 добавлены; FORCE RLS37. Go/race/PG integration и fresh Next build PASS, multichannel browser1 test/6 flows + financial UI PASS. Live disabled, LLM/WhatsApp budgets0. Security CONDITIONAL, E01 LOW OPEN (пустой .next 2 повторился, доказательства собраны, источник не установлен). Отчёт [STAGE_9B_REPORT](STAGE_9B_REPORT.md); ниже сохранены исторические результаты.

# Актуализация 8Г — 21.10.2026

**CONDITIONAL / NOT READY.** Независимо повторены Go tests/vet/build, PostgreSQL integration (**24.514s**),32-table FORCE RLS, tenant/channel isolation, order/reserve/recovery/budget regression. Реальных API не было, оба бюджета0 RUB, live off.

Go race **BLOCKED**: в isolated runner нет C compiler; host clang17 есть, но дополнительный host network sandbox не разрешён (`sandbox_apply: Operation not permitted`). Никакого обхода/повышения прав. Next production build **BLOCKED**: недоступны Linux GNU SWC16.4.0/Oxide4.3.3/lightningcss1.32.0. Теперь prebuild завершает процесс42 при missing/mismatch/unloadable native modules; тесты gate PASS5. Это не успешная Next build.

Текущие production browser E2E **Telegram BLOCKED / WhatsApp BLOCKED**. Cached stage7 browser и current TSX/BFF PASS имеют ограниченную область и их не заменяют. Для WA после fresh build нужен отдельный полный browser spec. Подробные актуальные доказательства и команды: [STAGE_8G_REPORT](STAGE_8G_REPORT.md), [GO_RACE_READINESS](GO_RACE_READINESS.md), [NEXTJS_BUILD_RECOVERY](NEXTJS_BUILD_RECOVERY.md), [FULL_BROWSER_E2E_REPORT](FULL_BROWSER_E2E_REPORT.md).

Ниже сохранена история предыдущего этапа; её результаты не выдаются за повторные результаты8Г.

---

# Pre-live security regression — 8В

20.10.2026. **SECURITY STATUS: CONDITIONAL.** Новых воспроизведённых CRITICAL0/HIGH0, новых открытых уязвимостей0 в проверенной области. Три исторические HIGH A01/A02/A04 остаются FIXED по повторным регрессиям. Счётчик0 не означает доказательства отсутствия неизвестных уязвимостей. Build/race/current SSR/реальные provider проверки — открытые gates, не скрытые PASS.

## Доказательства

| Область | Статус | Код и реально выполненная проверка |
|---|---|---|
| Telegram auth/DoS/route | VERIFIED | `internal/http` webhook handler, `TestAuditWebhookRejectsMalformedBeforeDatabase`, новый `telegram_negative_webhook_worker_and_kill`; неверный secret/unknown integration/oversize rejected |
| WA HMAC/raw bytes/Phone ID | VERIFIED | `internal/whatsapp/adapter.go:Normalize`, `TestWebhookContract`, новый WA negative matrix; constant-time HMAC, изменённое тело403 |
| Tenant/channel principal | VERIFIED | `prelive_matrix_test.go:testPreliveMatrix`, `multichannel_test.go` capability scope; A/B/C isolation, no employee identity from customer |
| FORCE RLS32 | VERIFIED | `audit_test.go:testAuditRLS`, runtime aba_app, реальные чужие rows, SELECT/INSERT/UPDATE/DELETE и reparent; `SECURITY_RLS_INVENTORY_8V.json` |
| Tenant context/pool | VERIFIED в сценариях | `internal/tenancy/transaction.go`, stage3/6/multichannel tests; transaction-local context |
| Consent/orders/reserve | VERIFIED | `internal/orders/service.go`, stage5 и новая 3×2 matrix; quote-bound action, no pre-staff reserve, last-unit concurrent confirm |
| Lease/fencing/UNKNOWN | VERIFIED в сценариях | SQL9/10, `reliability_test.go`, `whatsapp_crash_test.go`, новый `whatsapp_retry_exhaustion_and_stale_fencing`; no uncertain resend |
| Integration disable/handoff | VERIFIED в сценариях | send admission guard + advisory lock, regression audit tests и новый matrix negative test |
| LLM budget0 | VERIFIED | `agent/audit_test.go:TestAuditZeroBudgetStopsActualAdapterTransport`, stage7/reliability PG ledger; provider calls0 при запрете |
| LLM tool args/injection/SSRF | VERIFIED контрольные случаи | agent/provider/live tests, fixed allowlist/typed tool args, trusted backend tenant; это не гарантия любого естественного текста будущей модели |
| WA policy/budget0 | VERIFIED | `Adapter.Send/CanSend`, `TestNoCredentialJSONAndNoLiveOrDisabledSend`, DB CHECK0/window tests; live transport не реализован |
| Auth/CSRF/roles | VERIFIED в сценариях | identity integration, session server checks, `http/identity.go` origin+CSRF, owner/member tests; cookie HttpOnly/SameSite/Secure config |
| Current BFF/UI | PARTIALLY VERIFIED | Actual TSX Chromium и transpiled current BFF contract PASS; fresh Next SSR NOT RUN |
| Secrets | PARTIALLY VERIFIED | Credential structs json:-, encryption/role tests, captured log checks и ограниченный redacted scan; полный exhaustive analysis не заявляется |
| Race | NOT VERIFIED | CGO toolchain отсутствует в isolated runner; BLOCKED |
| Реальные provider / TLS / policy/pricing | NOT VERIFIED | Запрещены внешние запросы; актуальность официальной WA policy не проверялась |

## Надёжность и остаточные риски

1. Telegram/WA не дают универсальной exactly-once send гарантии. После неопределённого результата сохраняется UNKNOWN; automatic resend запрещён. Manual resolution не доказывает фактическую доставку. Это documented provider limitation, не новая HIGH.
2. Поздние WA delivery receipts обрабатываются идемпотентно, read не понижается. Live semantics/version/pricing требуют отдельной официальной проверки после разрешения. Нет production-capable WA live transport; контролируемый Meta-тест сейчас также NOT READY.
3. Shared order service проверен на price/stock conflicts; не все комбинации ошибок прогнаны как отдельные полные webhook→browser сценарии обоих каналов.
4. Race и fresh frontend build/current SSR browser coverage не завершены. До закрытия нельзя заявлять полный pre-live PASS.
5. Backup restore полного пользовательского стека, внешний TLS, privacy/legal approval, реальный provider token rotation/операторская авария не выполнялись. Runtime пользователя не трогали.

## Сеть и секреты

Все исполняемые Go/Node/browser тесты — internal Docker network/no published ports/no default IPv4 route, DNS loopback, proxies empty, images pull never, caches read-only. HTTP provider destinations в тестах принудительно loopback; неожиданные hosts отклоняются. Глобальная внешняя сеть хоста не объявляется отключённой. Пакеты/образы не скачивались, настоящие API/токены не использовались; commit/push/deploy/повышение прав отсутствовали.

Подробные результаты scan и неизменности migrations фиксируются в `STAGE_8V_REPORT.md`. Значения локальных env secrets никогда не печатались. Все credentials в новом стенде синтетические; в обычном приложении live gates и бюджеты не менялись.

## Решение

**Telegram live: NOT READY. WhatsApp live: NOT READY.** Открытых известных CRITICAL/HIGH в проверенной области нет, но verification blockers остаются. Нужны заранее подготовленные offline toolchains, свежая сборка и полный current browser regression; затем отдельное точечное разрешение на live, без настоящих покупателей.


## Повторная проверка 2026-10-28

Повторно выполнены offline Go tests/race/vet/build и PostgreSQL integration/race (PASS59.480s, FORCE RLS37), financial/template и Telegram/WhatsApp regression. Frontend lint/typecheck/OpenAPI и6 guard tests PASS. Fresh production build `oBqtgGCX-OU0t3jYnOX8V` PASS; browser3shops×2mock channels PASS (1 expanded test,345.065s,0 skips/errors/flaky). Evidence `.local/audit9b-recheck/`; точные границы и команды — STAGE_9B_REPORT, раздел повторной проверки.

E01 LOW OPEN: пустой `standalone/node_modules 2` повторился после browser; strict guard правильно отказал. Сохранены forensic snapshot и metadata diff; удалён только пустой generated path через rmdir, original proof неизменён. После этого strict verify/served marker/content baseline PASS. Метаданные2634 существующих записей менялись; read-handles macOS Virtualization PID68998 не доказывают источник записи. Security CONDITIONAL, новых CRITICAL/HIGH не выявлено в scope. Реальные API NOT RUN/запрещены; external calls NONE; оба бюджета0 RUB, live disabled. Новых продуктовых изменений/миграций в повторном проходе нет.
