> Актуализация 8Д, фаза 2 (2026-10-21): установка 13 согласованных APK в отдельный Linux/amd64 образ выполнена. Go race, PostgreSQL integration с race, FORCE RLS на 32 таблицах, свежая Next.js production build и production browser E2E Telegram/WhatsApp — PASS. Подробности: [итог фазы 2](BUILD_ENVIRONMENT_PHASE_2_REPORT.md), [race](GO_RACE_FINAL_REPORT.md), [build](NEXTJS_PRODUCTION_BUILD_REPORT.md), [browser E2E](MULTICHANNEL_BROWSER_E2E_FINAL_REPORT.md). Исторические результаты ниже сохранены. Security: CONDITIONAL; открыто наблюдение E01 LOW (неустановленное происхождение пустых каталогов в build workspace). Оба live-канала NOT READY, бюджеты LLM/WhatsApp 0 RUB. Реальные API не вызывались; последующие установки и live-действия автоматически не разрешены.

# Full multichannel browser E2E — 8Г

21.10.2026. **Telegram CURRENT PRODUCTION BROWSER E2E: BLOCKED. WhatsApp CURRENT PRODUCTION BROWSER E2E: BLOCKED.** Причина: fresh Next production build BLOCKED; хостовый старый .next и cached stage7 web не соответствуют требованию текущей сборки.

## Разделение доказательств

| Уровень | Сборка / стенд | Результат8Г | Что доказывает |
|---|---|---|---|
| Backend Telegram/WhatsApp | Текущий Go source, свежие binaries, real isolated PostgreSQL | PASS | full webhook/inbox/principal/mock-agent/catalog/outbox/order/reserve,3 stores |
| Current UI components | Текущие TSX через локальный webpack harness + Chromium | PASS | WA mock/owner UI/409/CSRF/XSS/tenant/UNKNOWN, recovery UI |
| Current BFF contract | Текущий transpiled route, controlled fetch | PASS | restricted route allowlist, только session/CSRF forwarding, no arbitrary destination |
| Cached browser regression | aba-stage7-smoke-web:latest, image1302bbd09cb6 + текущие Go binaries | PASS5, skipped1;36.288s | Старые пользовательские маршруты, не новая production build |
| Development frontend | Не запускался | NOT RUN | Не требуется вместо production |
| Current production Telegram browser | Artifact отсутствует | BLOCKED | Не выполнено |
| Current production WhatsApp browser | Artifact отсутствует | BLOCKED | Не выполнено |

## Выполненные backend сценарии

`TestDatabaseAndMigrations` PASS24.514s, включая `testPreliveMatrix`: три independent магазина, оба канала, own exact variant/price/stock, no missing-product invention, duplicate webhooks/callbacks, quote-bound consent, no early reserve, reject, concurrent last-unit confirm/cancel, price change→PRICE_CHANGED и stock0→INSUFFICIENT_STOCK. A→B→C API404/RLS0, foreign integration/service denial, external ID namespaces.

Повторены Telegram secret и WA HMAC/raw body/Phone ID tests, malformed/oversize/unsupported media, allowlist, handoff/staff/window, UNKNOWN/no unsafe resend, process crash/restart, concurrent worker, lease/fencing/retry exhaustion, budget recovery/double settlement/zero transport counter. Общие server tests не объявляются отдельными browser assertions.

## Evidence

Дата21.10.2026. `.local/audit8g/artifacts/integration.log` — полный integration stdout; `.local/audit8g/artifacts/build-manifest.json` — SHA свежих Go binaries и явная отметка current frontend BLOCKED. `.local/audit8g/artifacts/web-check.log` — lint/typecheck/OpenAPI/preflight tests. Browser results — `.local/audit8g/browser-results/cached-browser.json`; лог/trace не публикуются в Git и не содержат реальных пользовательских данных. Cached browser:5 passed,1 intentional backend-down skip,0 errors, duration36.288s; start2026-10-21T10:21:10.486Z. Backend-down запускается отдельно и учитывается отдельно.

Ключевые current component tests: `scripts/test-multichannel-components.cjs` PASS; `scripts/test-recovery-components.cjs` PASS. Они используют real Chromium, synthetic controlled API responses и текущий TSX, но не Next SSR. Никаких current-production screenshots не создано: отсутствующий artifact не выдаётся за работающую сборку.

## После снятия блокировки

1. QA связывает fresh source hash→production artifact hash→запущенный web process/image; старые cached images исключаются.
2. Сначала существующие `apps/web/tests/auth.spec.ts`, smoke/unavailable — регистрация/Mailpit/сессия/catalog/inventory/orders/TG/AI settings/BFF/redirect/ошибки.
3. Расширить и выполнить полноценный current-production WA browser spec: mock connect/allowlist/enable, signed webhook fixture, exact catalog reply, conversation channel filter/status, offer+quote confirm, staff approve/reject/reserve, handoff/staff reply, window expiry→NEEDS_APPROVED_TEMPLATE/no send, disable, errors. Existing component test не заменяет этот новый полный spec.
4. Три synthetic owners/staff проверяют foreign BFF requests404/403, own organization switching, channel principals и budgets; owner/member controls проверяются backend, не только hidden buttons.
5. Ключевые сценарии записывают test name/result/date/build ID/logs, failure screenshot только synthetic data; после fix повторяются на той же свежей сборке. Backend-down тест запускать отдельно после остановки только тестового API.

Команды в подготовленном стенде: `node node_modules/@playwright/test/cli.js test --output=<own-results> --reporter=json`; они недостаточны для WA coverage без полноценного WA spec выше. Текущий smoke overlay явно использует cached web и **не должен** применяться для фиксации current-production PASS. Нужен override image/entrypoint/mounts на проверенный свежий artifact, без внешней сети/host ports. Подготовка compiler/native dependencies не разрешает live.

Отдельный controlled backend-down: 1 passed, 0 failed, duration7.865s; фактический API был остановлен. Лог `.local/audit8g/browser-results/backend-down.json`.
