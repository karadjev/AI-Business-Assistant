# WhatsApp template test report — 9Б

Финальный PostgreSQL/race suite PASS44.742s, local HTTP only. Проверены как registry functions/real constraints, так и worker в прежнем outbox. Ни одного настоящего approval Meta нет.

|№|Сценарий|Результат/доказательство|
|---|---|---|
|1|APPROVED|PASS — WA_approved_template_HTTP_admission + queue positive outside window|
|2|PENDING|PASS — WA_template_PENDING|
|3|REJECTED|PASS — WA_template_REJECTED|
|4|PAUSED|PASS — WA_template_PAUSED|
|5|DISABLED|PASS — WA_template_DISABLED|
|6|UNKNOWN / missing|PASS — WA_template_UNKNOWN/not_found|
|7|Expired verification|PASS — WA_template_expired|
|8|Wrong language|PASS — WA_template_language|
|9|Wrong parameters|PASS — WA_template_params + WA_template_financial_parameter_type|
|10|Wrong WABA|PASS — WA_template_waba|
|11|Cross-tenant|PASS — WA_template_cross_tenant + RLS_wa_templates|
|12|Duplicate template|PASS — VerifyTemplate replay + business key uniqueness|
|13|Invalid components|PASS — WA_template_components + invalid verifier components/placeholders|
|14|Approval revocation|PASS — trusted MockVerifier PAUSED after enqueue, worker denied|
|15|Worker delay|PASS — expiry checked at reserve, not enqueue/UI|
|16|Changed status before dispatch|PASS — WA_template_queue_revoked_before_dispatch|
|17|No financial admission|PASS — WA_template_financial_no_pricing/no_budget|
|18|Mock approval not real|PASS — WA_template_live_mock + real constructor synthetic refusal|

Дополнительные проверки: BODY placeholder schema, максимум20 text arguments,512-char allowed name и отдельный PostgreSQL length bound; source/provenance принудительно synthetic; idempotent QueueTemplate сохраняет ровно один финансовый charge; generic text sender не используется вместо template sender. Реальный Cloud serializer+DB admission отправляет template в local loopback server, создаёт ProviderID и один conservative settlement.

Browser evidence и точная актуальная build приведены STAGE_9B_REPORT. Browser пытается POST APPROVED через existing BFF; корректный безопасный отказ404 и неизменённый PENDING проверяются отдельно. Неподтверждённые статусы никогда не отображаются как real Meta approval. Browser fixtures — synthetic rows под привилегированным **test-only** helper в собственной БД, не публичный обход verifier.

NOT RUN: real Meta sync/approval/status notification, реальные template messages/categories/pricing, публичные webhook/TLS. Новые финансовые/registry таблицы FORCE RLS; ordinary owner/runtime не имеет прямого DML, staff не может выполнять financial reconciliation. Разрешение live необходимо отдельно; offline PASS не заменяет его.


## Повторная проверка 2026-10-28

Повторно выполнены offline Go tests/race/vet/build и PostgreSQL integration/race (PASS59.480s, FORCE RLS37), financial/template и Telegram/WhatsApp regression. Frontend lint/typecheck/OpenAPI и6 guard tests PASS. Fresh production build `oBqtgGCX-OU0t3jYnOX8V` PASS; browser3shops×2mock channels PASS (1 expanded test,345.065s,0 skips/errors/flaky). Evidence `.local/audit9b-recheck/`; точные границы и команды — STAGE_9B_REPORT, раздел повторной проверки.

E01 LOW OPEN: пустой `standalone/node_modules 2` повторился после browser; strict guard правильно отказал. Сохранены forensic snapshot и metadata diff; удалён только пустой generated path через rmdir, original proof неизменён. После этого strict verify/served marker/content baseline PASS. Метаданные2634 существующих записей менялись; read-handles macOS Virtualization PID68998 не доказывают источник записи. Security CONDITIONAL, новых CRITICAL/HIGH не выявлено в scope. Реальные API NOT RUN/запрещены; external calls NONE; оба бюджета0 RUB, live disabled. Новых продуктовых изменений/миграций в повторном проходе нет.
