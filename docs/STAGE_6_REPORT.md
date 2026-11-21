# Отчёт по ТЗ №6 — Telegram Integration

13.10.2026. **Только mock Telegram transport и mock AI.** HTTP `getMe`/`sendMessage` adapter проверен только local HTTP fixture, реальный Telegram API не вызывался. Настоящие сообщения, платные LLM, real PII, production, WhatsApp/1С/платежи отсутствуют. Commit/push не выполнялись.

## Изучение и архитектура

Изучены README, STAGE_5_REPORT, SYSTEM_ARCHITECTURE, AI_AGENT_IMPLEMENTATION, CONVERSATION_LIFECYCLE, ORDER_MANAGEMENT, SECURITY, TENANT_ISOLATION, INTEGRATIONS, текущие Go/auth/catalog/orders/agent/conversations/config/worker/RLS/migrations, Next BFF/UI и CI/tests. Проверен git status: checkout без commits, файлы по-прежнему untracked; это не новые удаления. Существующие документы и migrations00001–00004 сохранены, пользовательский volume не очищался. Официальный Telegram Bot API просмотрен как первичный источник, никаких bot HTTP requests при этом не выполнялось.

Основное противоречие: stage5 RLS/actor требовали employee session, а канал должен быть customer/system. Минимальное решение —ephemeral inbox capability в trusted Go Tenant, transaction-local org/cap scope с exact conversation; без owner session в queue, без нового AI engine, без RLS bypass. Customer audit actor nullable, customer ID отдельный; employee decisions прежние. Stage5 test-only stock boundary сохранена: real pilot data не участвует до privacy approval.

Компоненты: Integration Service/Configuration + encrypted credentials; webhook/normalizer/principal; durable inbox/leased processor; existing Conversations/Agent/Orders; message outbox trigger; sender/worker; same-origin protected Next кабинет. Архитектура/инструкция —[TELEGRAM_INTEGRATION](TELEGRAM_INTEGRATION.md), безопасность —[TELEGRAM_SECURITY](TELEGRAM_SECURITY.md), delivery —[MESSAGE_DELIVERY](MESSAGE_DELIVERY.md).

## Файлы

Созданы:

- `apps/api/migrations/00005_telegram.sql`, `00006_telegram_worker_gate.sql`.
- `apps/api/internal/telegram/{transport.go,http_transport.go,service.go,worker.go,transport_test.go,http_transport_test.go}`.
- `apps/api/internal/config/{telegram.go,telegram_test.go}`.
- `apps/api/internal/http/stage6.go`.
- `apps/api/tests/integration/stage6_test.go`.
- `apps/web/src/components/TelegramIntegration.tsx`.
- `apps/web/src/app/dashboard/integrations/telegram/page.tsx`, `dashboard/conversations/page.tsx`.
- `docs/{TELEGRAM_INTEGRATION.md,TELEGRAM_SECURITY.md,MESSAGE_DELIVERY.md,STAGE_6_REPORT.md}`.

Изменены:

- API `cmd/{api,worker}/main.go`, `internal/{http/router.go,http/identity.go,conversations/service.go,orders/service.go,catalog/service.go,tenancy/transaction.go,platform/db/db.go}`, `tests/integration/database_test.go`.
- Web `components/{Sidebar,Header,Workflows}.tsx`, `lib/workflows.ts`, `app/{page.tsx,dashboard/page.tsx,api/[...path]/route.ts}`, `tests/auth.spec.ts`.
- `docker-compose.yml`, `.env.example`, `infrastructure/compose.test.yml`, `scripts/{smoke.sh,smoke-compose.sh,check-openapi.mjs}`, `.github/workflows/ci.yml`, `packages/contracts/openapi.yaml`.
- README, TECHNICAL_DECISIONS, TENANT_ISOLATION, SECURITY, API_DESIGN, SYSTEM_ARCHITECTURE, DATABASE_DESIGN, AI_AGENT_IMPLEMENTATION, CONVERSATION_LIFECYCLE, LOCAL_DEVELOPMENT.

Версии инструментов прежние: Go1.26.2/pgx5.9.2/goose3.27.1/PostgreSQL18.3; Node24.15.0/Next16.4.0/React19.3.0/TS5.9.3/Tailwind4.3.3/ESLint10.12.0/Playwright1.64.0. Новых npm/Go dependency нет. OpenAPI0.6.0.

## Схема/API/правила

00005: telegram_integrations/principals/inbox/outbox/events; compoundFK, global bot uniqueness, inbox integration/update uniqueness, outbox logical key uniqueness, leased states/attempts/backoff/receipts, FORCE RLS/runtime ownership guards. Existing nullable customer/system audit, internal_chat/telegram source, trusted source/blocked channel UPDATE order trigger; shared product/inventory locks без предоставления channel UPDATE. Secret column privileges исключают SELECT cipher/hash runtime. Additive00006 hash-only HMAC worker gate закрывает прежние claim/send/purge signatures; applied00005 не переписывается, legacy owner reconnect. Controlled migration, metadata6; clean down roundtrip, nonempty guard.

O=/api/v1/organizations/{id}: integrations/telegram GET; connect/check/disconnect/synthetic-updates POST. Owner configuration/synthetic, staff sanitized status. Protected sessions/BFF/CSRF/SSR прежние. Public integration-bound `/api/v1/webhooks/telegram/{integration}` POST header secret, private validation/16KiB/rate/durable ACK. Existing conversations/messages/handoff/resume/delete and orders/actions reused. Staff customer impersonation/quote-consent Telegram запрещены. Реальный webhook не зарегистрирован: synthetic_only, application live/production gate.

Scenario: synthetic private text → inbox commit → worker channel principal → existing mock agent / active catalogue → trusted facts + inline offer → server quote5min → explicit bound confirm callback →pending request/snapshots, reserve0 →employee confirm reserve. Changed/expired price/availability/freshness errors сохраняют безопасную семантику stage5. Handoff stops automation; staff reply message+outbox atomic. Button actions/quote/confirmed request history сохраняются; linked order/source в UI. Retention default7d/1–30days: worker erases expired message/inbox/outbox payload, existing explicit erase extends to channel payload. Routing/order/audit metadata сохраняются до дальнейшей privacy policy.

## Проверки

Все результаты ниже относятся к финальному source после worker proof/purge hardening. Изолированные test stacks удалены собственными cleanup; dev volume сохранён.

| Проверка | Результат | Подтверждение |
|---|---|---|
| Go unit/race | PASS | Последний go test -race ./..., auth/config/http/agent/catalog + Telegram encryption/AAD/tamper/private IDs/local transport |
| Go vet/build/gofmt | PASS | API/worker/migrate/healthcheck, formatting без изменений |
| PostgreSQL integration/race | PASS | Последний полный suite18.064s, stage3/4/5 regression + stage6, настоящая isolated PostgreSQL18.3 |
| Migrations up/down/repeat/concurrent | PASS | Clean6→5→4→3→2→1→6, repeatup, concurrentup, nonempty rollback refusal |
| Telegram connect/ownership/secrets | PASS | Validmock, invalidtoken, samebotforeignreject, cipher not plaintext, runtime cannot SELECT cipher, API logs no secret/token/message |
| Webhook/dedup | PASS | Correct/incorrectsecret, malformedJSON/missingID/group/oversize rejection, duplicate update single inbox |
| Principal/RLS/worker proof | PASS | Two owners/bots with same external sender independent conversations, foreign API/SQL blocked, all5tables FORCE; invalid worker proof cannot claim inbox or ciphertext; transaction-local capability no pool leak |
| Queue/concurrency/restart | PASS | Concurrent claim, same-chat next job blocked, expired lease recovery, committed-message replay produces one row, retry_after scheduled/retried, lost outbox receipt→uncertain blocks automatic resend |
| Consent/orders/inventory | PASS | No order before callback, bound quote explicitconfirm, repeatquote/update one request, Telegram pending source, reserved0; stage5 staff reserve/last-unit concurrency/cancel/complete unchanged |
| Handoff/staff/retention | PASS | Human no auto, staff message/outbox, staff cannot impersonatecustomer/configurebot, payload erasure expiry; background purge requires server proof |
| Frontend lint/typecheck/build | PASS | Native/Webpack and Docker production build |
| Full browser smoke | PASS | 5live47.5s: register/Mailpit/session/org/catalog + sandbox/orders + mock Telegram connect/update/offer/confirm/history/staff send/disconnect |
| Actual outages/shutdown | PASS | PGstop ready503/live200; API/worker exit0, 1offline browser7.5s, no fabricated success |
| Compose config/up/build | PASS | Isolated full smoke + updated existing dev stack all5healthy |
| OpenAPI validation | PASS | Swagger parser + implemented endpoint set0.6.0 |
| CI static | PASS | Docker actionlint1.7.7, minimum contents read, mock forced/no keys required; native tool download DNS unavailable, Docker fallback passed |
| Source/log secret scan | PASS в проверенном объёме | 170files + latest300 API/worker loglines checked against actual ignored local keys/passwords; no values output; .env/.local/.gocache/.next ignored |
| Documentation links | PASS | Local links resolved, previous documents/migrations preserved |
| Remote GitHub Actions | NOT RUN | Commit/push не разрешены |
| Native whole smoke/check.sh | NOT RUN целиком | Их component checks выполнены; selected full container smoke |
| Real Telegram / HTTPS webhook lifecycle | NOT RUN | No real API/token/send; HTTP adapter tested local fixture only, live application disabled |
| Real LLM / real PII / production / load/security audit | NOT RUN | Не входит в разрешённый контур; controlled security tests не полный audit |

Ранние FAIL не скрывались: connection column privileges потребовали узкого owner definer вместо открытия cipher SELECT; handoff SSR refresh race исправлена без ослабления теста; после сохранения quote/reply history locator должен проверять3assistant messages, strict single-element assertion исправлен; SQL recovery alias collision в00006 исправлена, затем полный PG+Docker smoke повторно PASS. Source snapshot в integration container устраняет compile/vet inconsistency при правках working checkout. Ни один сценарий не исключался ради зелёного результата.

## Критерии A–T

Оценка **для разрешённого synthetic/mock контура**. External live readiness отдельно NOT RUN/disabled.

| Критерий | Статус | Доказательство |
|---|---|---|
| A owner test integration | PASS | Browser mock:61001 connect + own/foreign service tests |
| B secure webhook | PASS | Secret/binding/body/private/rate/durable inbox tests |
| C duplicate update no duplicate order | PASS | Webhook dedup + repeat inline quote + browser repeatedupdate |
| D correct principal | PASS | Private sender/chat match, bot scoped integration, distinct customer UUID |
| E stores isolated | PASS | Two bots/sameexternalID separated at API/PG |
| F worker messages | PASS | Real worker in Docker browser chain + leased PG processor |
| G outbox retries | PASS | Mock429 retry/backoff/receipt, expired sending→uncertain blocks chat/no resend; ambiguous HTTP local protocol classified |
| H agent only own products | PASS | Scoped active catalogue; foreign quote/customer denied, existing catalog RLS regression |
| I correct catalogue reply | PASS | Outbox actual price/availability/manual freshness, browser1100.05RUB, disclosedmock |
| J order only confirmed consent | PASS | No order before button, own quote callback, duplicate unique order |
| K reserve employee only | PASS | Pending0reserved; channel SQL UPDATE0/Go status denied, existing staff reserve tests |
| L handoff | PASS | No assistant afterhuman, explicit resume guard, existing racingrun fence regression |
| M staff reply | PASS | Browser human form →message+outbox →mock sent, staff API check |
| N integration UI | PASS | SSR/session/BFF forms/status/deliveries/empty/errors/disconnect, channel/source/linkedorder |
| O protected tokens | PASS | AESGCM/orgAAD/tamper, secret columns noSELECT, noAPI/key/localStorage/log disclosure in checkedcases |
| P FORCE RLS | PASS | Five new tables forced, foreign rows hidden, worker proof negative/capability scope/pool reset |
| Q old functions | PASS | Identity/catalog/orders/agent/health/worker suites and browser regression |
| R Docker | PASS | Full newbuild/up/health/outage/shutdown; dev stack preservedupdated |
| S available tests | PASS | Go/race/vet/build, PG, web, OpenAPI, CI static, Docker; NOTRUN items above |
| T docs | PASS | Four new docs, README/contracts/decisions/security/schema/commands updated |

## Локальный запуск / подключение тестового бота

```sh
docker compose config --quiet
docker compose up --build -d --wait
sh scripts/migrate.sh status
sh scripts/check.sh
sh scripts/integration.sh
sh scripts/smoke-compose.sh
```

Current ignored .env дополнен отдельным random TELEGRAM_ENCRYPTION_KEY без вывода/перезаписи остальных значений; mode mock/retention7d. Controlled00005+00006 применены, metadata6. API8080/web3000/Mailpit8025 на127.0.0.1, DBprivate; dev stack healthy и оставлен работающим. No user data deletion/realcustomers/realbotnetwork. В test shop создать synthetic active SKU/stock, integration page подключить mock:<ID>, подать message→offer callback→confirm callback. История/заявки/staff в кабинете; details TELEGRAM_INTEGRATION. Actual BotFather bot пока не подключать.

## Ограничения и следующий этап

- Внешний Telegram getMe/send adapter подготовлен/локально тестируется, но activation/registration/getWebhookInfo/deleteWebhook/answerCallbackQuery в приложении не включены. Mock sent не значит actual delivery. До реального тест-бота нужно отдельное разрешение на Telegram HTTP/send и HTTPS sandbox setup; до real buyers —privacy/consents/retention/backups.
- Один bot на org, bot ID замена запрещена, reconnect same bot разрешён; simultaneous global ownership защищено unique + configuration lock.
- Inline mock flow один variant ×1; multi-item Orders остаётся в internal playground, Telegram cart/media/groups/business/channel support отложены.
- Uncertain блокирует следующую доставку этого chat; UI показывает ошибку, ручного reconciliation/resend endpoint нет. Перед live нужен owner/support безопасный resolve, duplicate warning, rate shaping, webhook reconciliation, мониторинг/SLO/load tests. Exactly-once внешней отправки не обещается.
- Arrival-order serialization, поздний Telegram occurrence order не восстанавливается; callbacks проверяют точный quote/chat/customer. Возможна unused quote после crash; request/order/outbox uniqueness не допускает повторной покупки из того же quote.
- Temporary payload retention не полное anonymization/удаление metadata и backups. Worker outage задерживает purge. Runtime/secret-key compromise за trusted backend boundary.
- Нет автоматического live LLM/provider failover/платежей/deploy/1С/WhatsApp. Старые бизнес-модули сохранены.

ТЗ7 рекомендуется посвятить согласованному тестовому Telegram sandbox: approval на real API/send, HTTPS registration lifecycle + provider fixtures, callback acknowledgements, privacy/metadata/backups policy, uncertain reconciliation, stable delivery/rate monitoring и live-only negative tests. Real LLM и реальные покупателей включать отдельным этапом после согласования расходов/ПДн. По завершении остановиться без commit/push.
