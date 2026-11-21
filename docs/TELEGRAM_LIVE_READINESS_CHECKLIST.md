> Актуализация 8Д, фаза 2 (2026-10-21): установка 13 согласованных APK в отдельный Linux/amd64 образ выполнена. Go race, PostgreSQL integration с race, FORCE RLS на 32 таблицах, свежая Next.js production build и production browser E2E Telegram/WhatsApp — PASS. Подробности: [итог фазы 2](BUILD_ENVIRONMENT_PHASE_2_REPORT.md), [race](GO_RACE_FINAL_REPORT.md), [build](NEXTJS_PRODUCTION_BUILD_REPORT.md), [browser E2E](MULTICHANNEL_BROWSER_E2E_FINAL_REPORT.md). Исторические результаты ниже сохранены. Security: CONDITIONAL; открыто наблюдение E01 LOW (неустановленное происхождение пустых каталогов в build workspace). Оба live-канала NOT READY, бюджеты LLM/WhatsApp 0 RUB. Реальные API не вызывались; последующие установки и live-действия автоматически не разрешены.

# Актуализация 8Г — 21.10.2026

**CONDITIONAL / NOT READY.** Независимо повторены Go tests/vet/build, PostgreSQL integration (**24.514s**),32-table FORCE RLS, tenant/channel isolation, order/reserve/recovery/budget regression. Реальных API не было, оба бюджета0 RUB, live off.

Go race **BLOCKED**: в isolated runner нет C compiler; host clang17 есть, но дополнительный host network sandbox не разрешён (`sandbox_apply: Operation not permitted`). Никакого обхода/повышения прав. Next production build **BLOCKED**: недоступны Linux GNU SWC16.4.0/Oxide4.3.3/lightningcss1.32.0. Теперь prebuild завершает процесс42 при missing/mismatch/unloadable native modules; тесты gate PASS5. Это не успешная Next build.

Текущие production browser E2E **Telegram BLOCKED / WhatsApp BLOCKED**. Cached stage7 browser и current TSX/BFF PASS имеют ограниченную область и их не заменяют. Для WA после fresh build нужен отдельный полный browser spec. Подробные актуальные доказательства и команды: [STAGE_8G_REPORT](STAGE_8G_REPORT.md), [GO_RACE_READINESS](GO_RACE_READINESS.md), [NEXTJS_BUILD_RECOVERY](NEXTJS_BUILD_RECOVERY.md), [FULL_BROWSER_E2E_REPORT](FULL_BROWSER_E2E_REPORT.md).

Ниже сохранена история предыдущего этапа; её результаты не выдаются за повторные результаты8Г.

---

# Актуализация 8В — 20.10.2026

**NOT READY.** Подтверждены offline backend E2E обоих каналов и FORCE RLS **32** таблиц, включая новую матрицу трёх магазинов. Go tests/vet/build, lint/typecheck/OpenAPI, текущие TSX+BFF contract tests и ограниченный cached-web browser regression PASS. **Go race BLOCKED; свежий Next production build BLOCKED; полный current SSR browser E2E NOT RUN.** Эти gates не закрываются прежними отчётами или cached image.

См. [STAGE_8V_REPORT](STAGE_8V_REPORT.md), [BUILD_READINESS_REPORT](BUILD_READINESS_REPORT.md), [MULTICHANNEL_E2E_TEST_REPORT](MULTICHANNEL_E2E_TEST_REPORT.md). Оба sending live off; LLM0 RUB; WhatsApp0 RUB. Реальная доставка, getMe/setWebhook/Meta operations, HTTPS и актуальная provider policy остаются NOT RUN и требуют отдельного разрешения. Для WhatsApp дополнительно отсутствует live transport; mock adapter не выдаётся за него. Никакое разрешение сейчас не запрашивается.

| Gate 8В | Статус | Доказательство / следующий ответственный |
|---|---|---|
| Webhook/principal/allowlist/tenant | PASS offline | prelive_matrix + audit/multichannel tests; QA |
| Inbox/outbox/UNKNOWN/fencing | PASS в сценариях | Реальный process crash, concurrent send, manual recovery; новый WA retry exhaustion; Backend/QA |
| Catalog/consent/staff reserve | PASS offline | 3 shops×2 channels, exact snapshots, last-unit concurrency; Backend/QA |
| FORCE RLS | PASS32 | SECURITY_RLS_INVENTORY_8V.json + aba_app negative SQL; DB engineer |
| Budgets0 / no live | PASS | Actual LLM transport calls0, WA mock-only constraints; сохранить ограничения |
| Текущий production frontend | BLOCKED | Подготовить offline native packages и собрать; разработчик |
| Race | BLOCKED | Подготовить isolated C toolchain; разработчик |
| Полный current browser E2E | NOT RUN | После свежей сборки; QA |
| Реальные API/оператор/политика данных | NOT RUN | Только новое явное разрешение; владелец/оператор |

Ниже сохранена история предыдущих этапов; её PASS не является новым доказательством 8В.

---

# Telegram readiness — 8Б.1, 18.10.2026

**NOT READY для настоящего Telegram.** Все известные HIGH закрыты; A09/A10 закрыты локальными механизмами и тестами. Это не разрешение live. LLM NOT SELECTED/mock/0 RUB; Telegram external=false, sending=false, kill=true; real customer data запрещены.

| Критерий | Статус | Проверка и доказательство | Ответственный / дальше |
|---|---|---|---|
| HIGH A01/A02/A04 | PASS | Unit + PG baseline10.945s и финальная регрессия10.435s | Backend/security; статусы FIXED |
| Webhook auth и tenant routing | PASS | integration UUID+secret, typed16KiB JSON, two-org webhook tests | Backend/QA |
| Customer principal/allowlist | PASS | Bot/org/user/chat binding, no username authority, pre-conversation allowlist | Backend/QA |
| Sending kill switch | PASS | Guarded transport/config denial, mock sending и disconnect fence | Оператор; live не разрешён |
| Inbox dedup | PASS | Concurrent duplicate updates/quote callbacks, одна заявка | Backend/QA |
| Outbox crash/recovery | PASS | Process exit после receipt, новый process без resend, uncertain retained | Backend/QA |
| Manual DELIVERY_UNKNOWN | PASS | Staff+CSRF, version/key, concurrent identical resolution, stale cap rejected | Staff; delivered либо abandon, без resend |
| Delivery attempt audit | PASS | Новый telegram_delivery_events, фактический HTTP request count и state events | Backend/DB |
| Handoff/staff replies | PASS | Existing stages5/6 + browser regression и control fence | Backend/QA |
| Quote/order/staff reserve | PASS | Consent/idempotency/no premature reserve/last-unit concurrency | Backend/QA |
| FORCE RLS27 таблиц | PASS | Runtime aba_app A/B SELECT/INSERT/UPDATE/DELETE/WITH CHECK; actual catalog | DB engineer |
| Encryption/log redaction | PASS в проверенной области | AES-GCM/SQL role gates, прежние tests и redacted scan | Security; не гарантия полного отсутствия утечек |
| Token rotation procedure | PASS документ | TELEGRAM_TEST_BOT_SETUP.md; revoke/deleteWebhook требуют нового approval | Владелец/оператор |
| Настоящая token rotation | NOT RUN | Нет токена/разрешения/внешней сети | Владелец отдельно разрешает |
| Budget0 / live gates | PASS | Actual Compatible transport calls0; member/owner/platform limits | Backend |
| Budget recovery UNKNOWN | PASS | Fixed5min, full conservative charge, новые процессы concurrent recovery, no double settle/refund | Backend/QA |
| Monitoring/audit | PASS локально | Queue states, event history, correlation IDs, budget_recovered_unknown count | Оператор; production alerts drill NOT RUN |
| Rollback | PASS isolated | Up/down/up/repeat/concurrent up, nonempty downgrade guard, миграции1–8 unchanged | DB engineer; пользовательскую БД не меняли |
| Retention | PASS механизм | Test7days/purge regression, delivery audit cascades, financial audit retained | Владелец; утвердить политику до реальных данных |
| Текущие recovery UI/BFF | PASS отдельные уровни | TypeScript/lint, actual TSX Chromium + transpiled BFF fetch contract | QA; это не Next production build |
| Docker/browser regression | PASS | Go9 binaries/PG/Mailpit + cached web7, 5 browser tests + backend-down separately | QA; cached web не доказывает новую Next сборку |
| Go tests/vet/build/OpenAPI | PASS | Offline complete checks | QA |
| Go race | NOT RUN | В доступных Go/Playwright cached images нет cc/gcc/clang; CGO=0 | QA; отдельно подготовить разрешённый offline toolchain |
| Свежий Next production build | NOT RUN | Только Darwin SWC/oxide; Linux пакеты отсутствуют, cached standalone не содержит их | QA; подготовить offline packages по отдельному заданию |
| Real getMe/setWebhook/getWebhookInfo/deleteWebhook/sendMessage | NOT RUN | Запрещены, ни один не выполнен | Только отдельное разрешение владельца |
| Public HTTPS/TLS/endpoint | NOT RUN | Туннели/публикация запрещены | Оператор после разрешения |
| Политика ПДн, backup restore, операторский live incident drill | NOT RUN | Нет реальных данных/выбранного поставщика/согласования | Владелец + ответственные специалисты |
| Operator approval | NOT RUN | 8Б.1 не разрешает live | Владелец решает в8Б.2 |

Для8Б.2: сначала обеспечить пропущенные offline race/Next build и проверить новую полную сборку, затем актуализировать policy/operator scope. Подготовленная recovery не является обещанием exactly-once и не разрешает live/платные вызовы. За новым бюджетом LLM автоматически не обращаться.

---

## Исторический checklist8А (не актуальные статусы A09/A10)

# Telegram live readiness — этап 8А

16.10.2026. **NOT READY.** PASS ниже относится к offline-доказательствам, а не к настоящему Telegram API. Бюджет 0 RUB, LLM mock; Telegram external=false, sending=false, kill=true. Operator approval отсутствует.

| Область / критерий | Статус | Способ и доказательство | Ответственный / следующее действие |
|---|---|---|---|
| Security: default-deny live gates | PASS | `config/*`, `agent/live_test`, `telegram/live_test`, stage7 PG tests | Backend; сохранить запреты |
| Security: отсутствие известных критических находок в проверенной области | PASS | A01–A08 исправлены, regression GREEN; не гарантия отсутствия иных ошибок | Архитектор; review перед live |
| Tenant isolation: 25 таблиц ENABLE/FORCE и runtime role | PASS | `testAuditRLS`, `SECURITY_RLS_INVENTORY.json`, direct aba_app negative SQL | DB engineer; повторять после схемных изменений |
| Tenant isolation: API чужих организаций | PASS | stages3–7 integration, 404/denied; pool context cleanup | Backend/QA |
| Webhook: secret, integration binding, body/JSON validation | PASS | `TestAuditWebhookRejectsMalformedBeforeDatabase`, stage6 + wire fixture | Backend/QA |
| Webhook: настоящий getMe | NOT RUN | Нет разрешения/токена | Владелец отдельно разрешает |
| Webhook: публичный HTTPS, TLS, routing, perimeter limit | NOT RUN | Интернет/туннели запрещены; endpoint не публиковался | Оператор после согласования домена |
| Webhook: setWebhook/getWebhookInfo/deleteWebhook на Telegram | NOT RUN | Нет внешних запросов; локальный transport не доказывает Telegram state | Владелец разрешает каждую операцию |
| Principal mapping: bot/org/user/chat, no username authority | PASS | stage6, `testTelegramHTTPAudit`, allowlist до создания conversation | Backend/QA |
| Principal mapping: реальные согласованные Telegram ID | NOT RUN | Только синтетические ID | Оператор/тестировщики |
| Inbox/outbox: dedup, leases, bounded retry, 429 | PASS | stage6/stage7 PG, loopback actual HTTP adapter | Backend/QA |
| Inbox/outbox: ручная сверка uncertain delivery | FAIL | A09 OPEN; безопасная остановка есть, workflow восстановления отсутствует | Backend/оператор: закрыть A09 до live |
| Worker: конкуренция, stale claim, disconnect fence | PASS | `send_admission_serializes_disconnect`, actual pg_locks; leases recovery | Backend/DB engineer |
| Worker: сигналы/остановка и PG outage | PASS | Изолированный Docker smoke; live200/ready503; clean SIGTERM | DevOps/QA |
| Message sending: реальный Telegram send/429/timeout | NOT RUN | Только HTTP emulator, не api.telegram.org | Владелец разрешает после остальных gates |
| Message sending: allowlist/kill switch/disconnect | PASS | Config/Guarded tests, claimed send cancellation + browser mock | Backend/оператор |
| Human handoff: прекращение автоответов, staff, resume | PASS | stages5–6, новый queued-message test, browser flow | Backend/QA |
| Order creation: inline quote consent/idempotency | PASS | Actual webhook→worker→quote→confirm fixture, PG duplicate/order tests | Backend/QA |
| Order creation: reserve только staff/no overselling | PASS | Stage5 concurrent PG tests + browser reserve/cancel/complete | Backend/QA |
| Budget: live запрещён при 0 RUB и member denied | PASS | `TestAuditZeroBudgetStopsActualAdapterTransport`, PG zero/concurrent budget | Backend; денежное разрешение отсутствует |
| Budget: reconciliation после crash до settle | FAIL | A10 OPEN; резерв не освобождается без доказательств | Backend/DB engineer: закрыть до live LLM |
| Secrets: encryption/no response/log/bundle leakage по выполненным проверкам | PASS | 249-file redacted scan, mode0600, stage6/router/provider tests | Security; не публиковать секреты |
| Secrets: реальный token lifecycle/ротация | NOT RUN | Токен отсутствует, BotFather не использовался | Владелец/оператор |
| Monitoring: request/correlation IDs, агрегаты и queue states | PASS | `telegram/worker.go`, `costcontrol.State`, stage7 settings/metrics tests | Backend; локальные механизмы |
| Monitoring: алерты и отработанная операторская реакция | NOT RUN | Инцидентный drill с live не проводился | DevOps/оператор |
| Rollback: clean migration up/down и nonempty downgrade guard | PASS | `database_test.go`, миграция8 отказывается при organization data | DB engineer |
| Rollback: backup restore реальных данных | NOT RUN | Реальные данные запрещены, рабочие volumes не трогались | DevOps; отдельно synthetic backup drill |
| Data retention: 7-day configurable test lifecycle/purge | PASS | Config retention, PG stage7 purge; public retention policy draft | Backend/QA |
| Data retention: утверждённая правовая политика/локация поставщиков | NOT RUN | Провайдер не выбран, реальная обработка запрещена | Владелец + специалист по ПДн |
| CI/build: Go unit/vet/build, lint/typecheck/OpenAPI/actionlint | PASS | Offline команды в STAGE_8A_REPORT | QA |
| CI/build: race и свежий frontend production build | NOT RUN | Нет offline C compiler/Linux native dependencies | QA; подготовить разрешённый cache без автоматических скачиваний |
| Operator approval: бот, endpoint, тестовые участники, scope и stop plan | NOT RUN | Отдельного разрешения нет | Только владелец |

Порядок перехода в 8Б: закрыть A09, подготовить отсутствующие offline-инструменты и повторить проверки, выполнить synthetic failure/recovery drill, согласовать тестовую политику и оператора; затем запросить отдельное разрешение на ограниченный Telegram test. A10 блокирует live LLM; **разрешение Telegram не включает LLM**. Ни один статус здесь не разрешает production или реальных покупателей.

## Уточнение после8Б.2

Многоканальный слой сохраняет Telegram SQL repository и guarded send. Offline Telegram regression, общий quote/consent и32-table RLS повторены вместе с WhatsApp mock; фактические команды — STAGE_8B2_REPORT. Это не live Telegram delivery. Readiness остаётся NOT READY: настоящий Telegram не вызывался, race/fresh Next build имеют отдельные NOT RUN, operator approval и privacy prerequisites не сняты. WhatsApp readiness оценивается отдельным checklist и не наследует Telegram approval.
