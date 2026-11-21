# Этап 9А — WhatsApp Cloud API Transport

## Итог

WHATSAPP CLOUD API TRANSPORT: **IMPLEMENTED (HTTP wire layer)**. Runtime **mock-only**, live activation **BLOCKED**. Это не утверждение о завершённом live admission/billing/template provisioning или production readiness.

WHATSAPP WEBHOOK SECURITY: **PASS offline**. WHATSAPP OFFLINE CONTRACT TESTS: **PASS**. TELEGRAM REGRESSION: **PASS**. NEXT.JS PRODUCTION BUILD: **PASS**. BROWSER E2E: **PASS**. RLS: **PASS32**. SECURITY STATUS: **CONDITIONAL**. CRITICAL/HIGH/LOW OPEN в проверенном scope: **0/0/1 (E01)**. Новых CRITICAL/HIGH не обнаружено; полный аудит всей системы не заявляется.

EXTERNAL CALLS: **NONE**. Бюджеты LLM/WhatsApp **0 RUB**. Реальные API, клиенты/credentials, платные операции, скачивания зависимостей, deployment, commit и push не использовались. Рабочий ai-business-assistant stack не перезапускался и не мигрировался.

## Архитектура и изменения

Сохранены общий Channel Layer, Agent/catalog/orders, SQL tenant routing, inbox/outbox/lease/fencing/recovery/handoff. Новый CloudTransport реализует существующий Transport.Send/SendResult, отдельный typed SendTemplate и interactive reply buttons. Официальный fixed HTTPS messages endpoint/Bearer JSON, no redirects/proxies/user URLs, deadlines/timeouts,64KiB cap, sanitized typed errors, bounded429 Retry-After, ambiguous outcomes UNKNOWN/no auto retry.

Runtime mock config validator остаётся mock-only. Cloud-specific validator не даёт владельцу подключить настоящий access token через текущий Connect. Валидация header injection/oversized tokens добавлена, template input bounded. Loader/Compose/example добавляют live sending kill-switch locked true. UI явно показывает mock/live disabled/disconnected, ошибки и отсутствие реальной проверки; кнопки live activation нет.

Graph version **NOT SELECTED**; обязательный server reviewed allowlist без default. В tests используется вымышленная v99.0. Актуальные официальные версии/тарифы/правила не проверялись по сети, перед live требуется отдельная проверка.

Финансовая граница `SendAdmission` обязательна и trusted backend-only. При budget0/flags off/missing admission транспорт вообще не вызывает HTTP. Reserve/Finish задают контракт будущего atomic/idempotent billing и conservative UNKNOWN. **Live WhatsApp PostgreSQL ledger/admission, approved-template registry и публичная template queue не реализованы/не включены этим этапом**; существующий LLM ledger не выдаётся за WhatsApp. Без них stage9Б live остаётся blocked, даже при наличии HTTP serializer. Текущий worker/API явно создаёт Mock{}, поэтому никакого автоматического выбора Cloud по environment/dashboard нет. Это сохранённый safety gate, не имитация реальной интеграции.

API endpoint/OpenAPI схемы и миграции не менялись. Новые library operations не документируются как публичные API. Секреты не возвращаются frontend; текущие tenant-scoped AES-GCM credentials, owner access и RLS сохранены.

## Результаты проверок

|Проверка|Результат|
|---|---|
|Go tests/vet/format/build API+worker+migrate+healthcheck|PASS; финально повторены после последнего token bound fix|
|Go race ./...|PASS; финально повторён|
|Cloud local HTTP contract tests|PASS; text/template/interactive,4xx/429/5xx/timeout/reset/oversize/bad JSON/no ID/redirect/zero budget/disabled/config/tenant/cancel/settlement|
|PG integration -race|PASS43.427s, основной test42.34s|
|FORCE RLS32 runtime aba_app|PASS; foreign SELECT/INSERT/UPDATE/DELETE, no bypass/context leak|
|Tenant/channel principal isolation|PASS3shops×2channels SQL/HTTP/worker + browser negative BFF|
|Telegram inbound/outbound/catalog/consent/staff/reserve/handoff|PASS integration и current browser|
|WhatsApp HMAC/challenge/WABA/Phone ID/dedup/receipts/window|PASS unit/actual local HTTP/integration/browser|
|Crash recovery/UNKNOWN/fencing/last-unit concurrent reserve|PASS PostgreSQL suite|
|Budget0/current live denial|PASS unit zero HTTP calls + integration/config/runtime flags|
|Frontend lint/typecheck/OpenAPI|PASS|
|Native/provenance script tests|PASS6|
|Fresh Next production build|PASS|
|Browser current production|PASS1 expanded test,6 store/channel flows,0 skips/errors/flaky|
|Client secret scan|PASS limited,48 static files,0 pattern/exact-local-env matches|
|Build proof before/after browser|PASS paths/types/bytes/proof; metadata observation below|
|Actual Meta/Telegram/LLM API, TLS/real delivery/template approval/pricing|NOT RUN; запрещены|
|Live WhatsApp budget reservation/settlement|BLOCKED: no live admission installed; future contract/mock tests не заменяют ledger|

Top-level PG crash helper SKIPs — subprocess entrypoints; родительские crash tests реально выполнены. Браузерный PASS доказывает **mock runtime**, новый Cloud wire отдельно доказан локальным HTTP server, а не реальной отправкой из worker.

Build ID `DHPmIWZuusLfeX7EzULKF`, UTC `2026-10-23T11:23:44.801Z`, app source SHA `de36fdc191c3c7e784de5b079a1f5f78872324175b045e16bc87cf8829f5c927`. Browser start `2026-10-23T11:25:34.373Z`, duration `339.474s`. Обе интеграции/согласие/заявки/решение staff/резерв/handoff/window/status/isolation проверены на актуальном frontend. Финальная правка server-only token length после первоначального Go прогона проверена дополнительным targeted race и затем повторным полным Go/race/build; mock business behavior не менялось.

## LOW и целостность

E01 LOW из8Е не закрыт. `.next 2`/`node_modules 2` не появились. Однако post-browser manifest отличается от pre-browser **mtime430 каталогов**; paths/kinds/file SHA256/proof совпали, strict artifact verify PASS. Источник изменения metadata не установлен, причинная связь с историческим E01 не доказана. Отдельная информационная запись среды, не подтверждённая модификация executable/dependencies/config и не основание ослаблять guard. Raw manifests/diff сохранены. Никаких широких whitelist/exclusions не добавлено. Byte-for-byte reproducibility разных Next builds не заявляется.

## Evidence и воспроизведение

`.local/audit9a/`: go-final.log, race-final.log, integration-race.log, rls_inventory.json, web-check-final.log, build-*.log/build-proof.json, browser-result.json/browser-completion.log, client-secret-scan.json, build-integrity.json/artifact-diff.json/standalone-after-*.json, preservation.json/source-changes.json. Browser screenshots/traces `.local/audit8d/browser-results/stage9a-multichannel/` (синтетические данные; локальные traces могут содержать тестовые cookies, не публиковать сырыми).

Три файла Compose: compose.environment.yml + compose.environment-smoke.yml + compose.prepared-build.yml; env `.local/audit8d/compose.env`. Cached images с pull never. Internal networks internal=true/IPv6false, без host ports/socket/usable external default route; common.sh проверяет routing, отключает proxies/downloads/telemetry. Package managers offline, dependencies не устанавливались. Использованы только свои synthetic aba_test volumes, cleanup только собственных stage9a projects.

```sh
docker compose --env-file .local/audit8d/compose.env -p aba-stage9a -f infrastructure/compose.environment.yml -f infrastructure/compose.environment-smoke.yml -f infrastructure/compose.prepared-build.yml run --rm race
docker compose --env-file .local/audit8d/compose.env -p aba-stage9a-check -f infrastructure/compose.environment.yml -f infrastructure/compose.environment-smoke.yml -f infrastructure/compose.prepared-build.yml run --rm integration
docker compose --env-file .local/audit8d/compose.env -p aba-stage9a-build -f infrastructure/compose.environment.yml -f infrastructure/compose.environment-smoke.yml -f infrastructure/compose.prepared-build.yml run --rm web-build
```

Для OpenAPI в web-build нужен read-only /packages mount; первый запуск без него FAIL ENOENT, повтор с mount PASS. Browser command — prepared `browser --grep 'current production' --reporter=json` с PLAYWRIGHT_JSON_OUTPUT_NAME; fixture environment supplied overlay. Не засчитывать SKIP при отсутствии fixture как E2E PASS. Initial test development failures/fixes описаны в WHATSAPP_OFFLINE_CONTRACT_TESTS, не скрыты.

## Изменённые файлы

Полный SHA-based список `.local/audit9a/source-changes.json` (репозиторий untracked, git diff не подменяет baseline). Основные: cloud.go/cloud_test.go; adapter/service mock boundary; config loader/tests; kill flags Compose/example; WhatsApp UI status; пять новых документов9А и notes в README/архитектуре/политике. MIGRATIONS: **NONE**. Lockfile/go.mod/go.sum/старые миграции не изменены. Host node_modules/toolchain не переустанавливались.

## Remaining blockers / next step

Telegram live **NOT READY**, WhatsApp live **NOT READY**. Нужны отдельное разрешение, реальные test credentials/HTTPS/allowlist/privacy policy; для WhatsApp — актуальные docs/version/pricing, durable authorized cost admission, template registry/queue при необходимости и live config wiring. Непроверенные live возможности не выдаются за готовые. E01 LOW и metadata observation остаются для локальной диагностики. Полный список [STAGE_9B_READINESS](STAGE_9B_READINESS.md). Следующий этап — отдельное offline задание на финансовый/template admission и review текущего API contract до какого-либо разрешённого live теста. Работа после локального отчёта остановлена.

### Финальный список файлов

Изменено 12, создано 7, удалено 0.

- `README.md`
- `docker-compose.yml`
- `.env.example`
- `docs/WHATSAPP_MESSAGE_POLICY.md`
- `docs/WHATSAPP_INTEGRATION.md`
- `docs/MULTICHANNEL_ARCHITECTURE.md`
- `infrastructure/compose.environment-smoke.yml`
- `apps/web/src/components/WhatsAppIntegration.tsx`
- `apps/api/internal/config/whatsapp.go`
- `apps/api/internal/config/whatsapp_test.go`
- `apps/api/internal/whatsapp/service.go`
- `apps/api/internal/whatsapp/adapter.go`
- `docs/WHATSAPP_CLOUD_API_TRANSPORT.md`
- `docs/STAGE_9A_REPORT.md`
- `docs/WHATSAPP_WEBHOOK_SECURITY.md`
- `docs/WHATSAPP_OFFLINE_CONTRACT_TESTS.md`
- `docs/STAGE_9B_READINESS.md`
- `apps/api/internal/whatsapp/cloud.go`
- `apps/api/internal/whatsapp/cloud_test.go`

Собственные проекты aba-stage9a/current/check/build и их синтетические volumes/networks удалены после проверок. Compiler image/Linux workspace/evidence сохранены. Рабочие контейнеры ai-business-assistant-* продолжают работу.
