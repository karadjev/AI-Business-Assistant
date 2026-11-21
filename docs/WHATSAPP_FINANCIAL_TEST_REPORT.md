# WhatsApp financial test report — 9Б

Automated PostgreSQL + race suite PASS44.742s (`.local/audit9b/integration-final.log`). Тестовые budgets/costs положительны только в **SYNTHETIC isolated aba_test**; runtime/browser wallets и оба configuration budgets0 RUB. Никаких настоящих тарифов, сообщений или денег.

|№|Сценарий|Результат/доказательство|
|---|---|---|
|1|Budget0|PASS — WA_financial_budget_zero|
|2|Достаточно бюджета|PASS — positive reserve + WA_approved_template_HTTP_admission|
|3|Недостаточно бюджета|PASS — WA_financial_budget_insufficient + WA_concurrent_budget|
|4|Unknown pricing|PASS — WA_financial_pricing_unknown|
|5|Expired pricing|PASS — WA_financial_pricing_expired|
|6|Unsupported currency|PASS — WA_financial_currency|
|7|Unknown region|PASS — WA_financial_region|
|8|Duplicate reservation|PASS — positive reserve replay + WA_concurrent_duplicate_workers|
|9|Duplicate settlement|PASS — positive finish replay + ledger conservation|
|10|Concurrent workers|PASS — WA_concurrent_duplicate_workers + WA_concurrent_budget|
|11|Crash before send|PASS — WA_crash_before_send, actual subprocess exit77|
|12|Crash after send|PASS — WA_crash_after_send, actual local messages POST receipt then exit77|
|13|Timeout|PASS — WA_financial_HTTP_timeout, durable unknown|
|14|Connection reset|PASS — WA_financial_HTTP_reset, durable unknown|
|15|DELIVERY_UNKNOWN|PASS — WA_unknown_manual_reconcile + HTTP/crash cases|
|16|Manual reconciliation|PASS — WA_unknown_manual_reconcile; owner/evidence/version/idempotency; staff refusal|
|17|Cross-tenant budget|PASS — RLS_wa_budgets/RLS_wa_reservations/events + FinancialState outsider|
|18|Disabled integration|PASS — WA_financial_disabled|
|19|Kill switch|PASS — WA_financial_kill|
|20|Synthetic pricing in live|PASS — WA_financial_synthetic_in_live + NewCloud rejects offline permit|

Дополнительно: verified-free0 проходит только явную подтверждённую synthetic rule/AllowVerifiedFree; max_charge0 без free verification отвергается. Отмена до HTTP освобождает резерв один раз. Финансово неизвестный reject остаётся held, даже если delivery Failed. Concurrent recovery даёт ровно один RECOVERED_UNKNOWN event; баланс равен сумме reserved/unknown и charged ledger rows. Wrong Phone/WABA/version tuple не может использовать чужие credentials. 37 реальных tenant tables проверены на ENABLE/FORCE, runtime role non-owner/NOBYPASS, outsider SELECT/INSERT/UPDATE/DELETE и owner tenant substitution. Global wallet SELECT под aba_app отдельно получил permission denied.

Crash subprocess использует ту же FinancialAdmission/Cloud сериализацию, локальный HTTP endpoint и намеренный os.Exit77 до send либо в Finish после receipt. Parent проверяет0/1 request, истечение lease, concurrent recovery и удержание суммы. Два старых и новый helper top-level SKIP — только subprocess entrypoints; parent tests реально PASS. Это не mock PostgreSQL.

Ошибки разработки сохранены: worker helper is_test_org ошибочно требовал session — заменён proof-bound проверкой actual test organization; negative cases дополнены проверкой **точной причины** отказа. Postgres regexp quantifier512 превышал ARE limit — заменён alphabet regex+length bound. Затем добавленный unknown-cost case изменил накопленный held balance, старое абсолютное ожидание10 исправлено на проверку дельты+10 (не на ослабление ledger). Неудачные прогоны не засчитаны PASS; финальный clean-DB прогон прошёл. Диагностический temporary test удалён, reproducer/evidence только в ignored `.local/audit9b/`.

Реальная тарификация/Meta invoice/FX/production/live E2E: NOT RUN. Автоматическое использование verified real policy и positive limits не включено. Частные HTTP unit tests дополнительно проверяют typed errors/rate/size/redirect/zero gates и production admission type boundary; отдельный live провайдер не вызывался.


## Повторная проверка 2026-10-28

Повторно выполнены offline Go tests/race/vet/build и PostgreSQL integration/race (PASS59.480s, FORCE RLS37), financial/template и Telegram/WhatsApp regression. Frontend lint/typecheck/OpenAPI и6 guard tests PASS. Fresh production build `oBqtgGCX-OU0t3jYnOX8V` PASS; browser3shops×2mock channels PASS (1 expanded test,345.065s,0 skips/errors/flaky). Evidence `.local/audit9b-recheck/`; точные границы и команды — STAGE_9B_REPORT, раздел повторной проверки.

E01 LOW OPEN: пустой `standalone/node_modules 2` повторился после browser; strict guard правильно отказал. Сохранены forensic snapshot и metadata diff; удалён только пустой generated path через rmdir, original proof неизменён. После этого strict verify/served marker/content baseline PASS. Метаданные2634 существующих записей менялись; read-handles macOS Virtualization PID68998 не доказывают источник записи. Security CONDITIONAL, новых CRITICAL/HIGH не выявлено в scope. Реальные API NOT RUN/запрещены; external calls NONE; оба бюджета0 RUB, live disabled. Новых продуктовых изменений/миграций в повторном проходе нет.
