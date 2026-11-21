# Store onboarding E2E — 10А

PASS: fresh production build `Qz7bLOo_mrjL562aXZegn`, source SHA256 `560eb4d39fae90f9c25b6d122e7d7b1f3abd12bf2b2359144bec73e9665ed746`, created `2026-10-28T13:00:04.564Z`. Browser suite start `2026-10-28T13:03:06.003Z`, duration 468.878s,2 expanded tests,0 skips/errors/flaky. Guard verified source/artifact/served marker before launch. Raw result `.local/audit10a/browser-final.json`, screenshots `.local/audit8d/browser-results/stage10a-verified/`.

`onboarding-production.spec.ts` проверяет владельца существующего synthetic shop (implicit tenant не создаётся), все поля профиля, upload demo .xlsx, auto mapping с доступными select controls, preview6new, отсутствие writes до confirmation, disabled apply без checkbox, reload/history/resume, atomic apply, repeat6unchanged, cancel, EXCEL source price409 через BFF, настройку прежнего Telegram mock, review/ACTIVE с live=false/budgets0, foreign tenant404 и staff403/no owner UI. New template/download содержит6 synthetic electronics товаров, явно тестовые цены и остатки.

`multichannel-production.spec.ts` в той же final build проверяет3shops×Telegram/WhatsApp, product grounding/current prices, explicit consent, staff order approval/reserve, handoff/window/receipts, tenant isolation, WhatsApp financial/template diagnostics и operator reconciliation. Все transports mock, реальные API не использовались.

Post-browser integrity: strict guard корректно отказал из-за двух пустых E01 paths; snapshot подтвердил неизменность content/types/other paths и original proof, metadata2650 записей менялись. После exact capture/rmdir strict verify и served marker PASS, baseline content совпал. E01 OPEN, не замаскирован. Это CONDITIONAL build integrity, а не заявление об отсутствии исторического E01.

Первый полный development browser pass тоже2tests PASS; после окончательной UX/RUB/tenant-key правки выполнена новая production build и полный повтор. Final build не подменяется первым прогоном. PostgreSQL transaction-fault tests и source/reserve/consume regression описаны в STAGE_10A_REPORT. Real1C/real channels/real LLM/customer data/paid calls NOT RUN.
