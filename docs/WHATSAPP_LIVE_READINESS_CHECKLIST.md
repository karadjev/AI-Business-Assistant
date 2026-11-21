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

# WhatsApp live readiness — 8Б.2

**WHATSAPP LIVE READINESS: NOT READY.** Только offline/mock. LLM/WhatsApp budgets0 RUB, live disabled, настоящие credentials отсутствуют. PASS ниже означает конкретную локальную проверку, не реальную доставку Meta.

| Область | Статус | Проверка и доказательство | Следующее действие / ответственный |
|---|---|---|---|
| Raw HMAC, invalid signature | PASS | whatsapp/adapter_test.go; multichannel_test.go, signed fixtures/real HTTP handlers | Разработчик сохраняет regression |
| Verify challenge | PASS | adapter Challenge + handler tests, wrong token rejection | Оператор отдельно разрешает подписку Meta |
| WABA/Phone/tenant routing | PASS | wrong Phone/integration tests; server-assigned org | Разработчик; до live подтвердить владение номером |
| Allowlist/principal namespace | PASS | две org, один wa_id; отдельный TG customer700; no staff elevation | Оператор утверждает тестовые аккаунты |
| FORCE RLS | PASS |32-table audit под aba_app, negative SELECT/INSERT/UPDATE/DELETE, no BYPASS | Разработчик сохраняет inventory |
| Pool tenant context | PASS | leased WA cap нельзя применить к TG/foreign org, rollback без утечки | Разработчик |
| Inbox dedup/order/lease | PASS | integration key, SKIP LOCKED, old TG regression и WA duplicate | Разработчик; расширять нагрузочные тесты |
| Outbox concurrency/recovery | PASS | два worker, один реальный loopback send; subprocess exit87 после receipt, UNKNOWN/no retry | Разработчик |
| Delivery receipts | PASS | signed delivered/read/sent/failed, read не регрессирует | Разработчик; live receipts NOT RUN |
| Handoff/staff | PASS | auto replies прекращаются; staff в human; closed window409 | Разработчик/оператор |
| Order consent/reserve | PASS | общий quote/confirm, repeated quote ID→одна pending заявка, reserve0 до staff | Разработчик |
| Budget / kill switches | PASS | CHECK budget0, env live rejected; explicit mock sending off/on/disconnect | Оператор не включает реальные отправки |
| Secrets | PASS | ciphertext column denied, AES-GCM AAD, public status/log negative tests и redacted scan | Оператор хранит будущие реальные ключи отдельно |
| Rotation real credentials | NOT RUN | настоящие credentials отсутствуют; audited live rotation API не реализован | Разработчик + оператор до live |
| Monitoring | PASS | persisted attempts/status/events, inbox/outbox/UNKNOWN/retry counters, safe generic logs | Оператор задаёт alerting для live отдельно |
| Retention | PASS | worker purge истёкших payload; config1–30 дней; source implementation | Оператор согласует metadata/backups; полная анонимизация не реализована |
| Migration/rollback | PASS | up/down/repeat, upgrade с существующими TG/order rows без изменения snapshot; down с WA данными запрещён | Оператор делает backup и не форсирует DROP |
| Frontend current components/BFF | PASS | Chromium current TSX, owner UI/CSRF/XSS/409, restricted BFF routes | Разработчик |
| Fresh Next production build / new SSR pages E2E | NOT RUN | отсутствуют cached Linux SWC/oxide; старый web image не доказывает новую сборку | Разработчик в среде с заранее доступными пакетами |
| Go race detector | NOT RUN | попытка CGO_ENABLED1 прервана: gcc отсутствует | Разработчик в подготовленной offline среде |
| Actual Meta API/delivery | NOT RUN | не разрешено, live transport отсутствует | Отдельный этап после разрешения |
| Current official policy/version/pricing | NOT RUN | внешняя документация в этом offline этапе не запрашивалась | Оператор/разработчик после разрешения |
| Privacy/legal/production | NOT RUN | реальные клиенты запрещены; draft не утверждён | Владелец/ответственный за данные |
| Operator live approval | NOT RUN | отсутствует; текущий этап не является разрешением | Владелец отдельно утверждает каждое внешнее действие |

До live: устранить инструментальные NOT RUN, реализовать/проверить безопасный live transport и rotation, утвердить HTTPS/тестовый номер/allowlist/стоимость/policy и внешние операции. Разрешение одного провайдера не включает другой. Не выполнять live activation автоматически.
