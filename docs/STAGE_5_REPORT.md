# Отчёт по ТЗ № 5

Дата: 13.10.2026. **Использован только mock. Реальный LLM/платные API не вызывались.** Adapter проверялся только локальным HTTP test server. Telegram/WhatsApp/1С/платежи/production не подключались. Commit/push не выполнялись, предыдущие документы и migrations00001–00003 сохранены.

## Изучено и реализовано

Прочитаны README, STAGE_4_REPORT, SYSTEM_ARCHITECTURE, DATABASE_DESIGN, AI_AGENT_ARCHITECTURE, API_DESIGN, SECURITY, TENANT_ISOLATION, PRODUCT_CATALOG, INVENTORY_MANAGEMENT и код Go/auth/organizations/catalog/RLS/migrations, Next/BFF, CI/integration/browser scripts. Проверены git status, миграции и версии библиотек. Checkout по-прежнему без commits: git status показывает исходные и новые файлы untracked, существующие данные не удалялись.

- Отдельные test organizations с пустым собственным каталогом/складом; прежние организации остаются обычными. Ни chat, ни тестовые orders не допускаются к обычному складу на API/RLS уровнях. Test flag runtime UPDATE запрещён.
- Server quote5min, active catalogue/price/availability rechecks, explicit consent точного quote/chat/customer, immutable price/name/SKU/quantity snapshots. Creation не резервирует.
- Orders pending_confirmation/confirmed/rejected/cancelled/completed. Atomic multi-item reserve/release/consume, order events и inventory audit, version/idempotency, запрет overselling/invalid transitions/negative stock. Новая цена требует нового согласия, не staff override.
- Manual_updated_at отдельно от обычного stock updated_at; configurable freshness24h. Stale/unknown AI available=null, staff confirm требует ручной сверки.
- Member opt-in staff can_manage_orders через owner-only existing-member endpoint; без доступа к catalogue/settings changes. Без приглашений/новой role hierarchy.
- Conversations/messages/events и bounded sync AI cycle вне DB transaction; handoff/resume, version/run/lease fence против позднего ответа, explicit erasure/lazy expiry7d для synthetic messages.
- Сменяемый Provider, disclosed deterministic Mock, Chat Completions compatible adapter без обязательного ключа; timeouts/iterations/tools/input/output/day quotas. Семь typed strict tools; backend authorization и active-only facts, no SQL/URL/files/model-supplied tenant. Free model content не отображается как товарные факты.
- Protected playground/orders/detail через прежние sessions/CSRF/BFF/SSR; карточки, quote, explicit checkbox/button, snapshot/events/actions, errors, mock/human indicators, safe tenant switching.
- Migration00004, OpenAPI0.5.0, CI/test scripts и документация. Новые внешние зависимости/сервисы отсутствуют; worker продолжает только dev email.

## Файлы

Новые:

- `apps/api/migrations/00004_orders_chat.sql`.
- `apps/api/internal/tenancy/transaction.go`.
- `apps/api/internal/orders/service.go`.
- `apps/api/internal/catalog/stock.go`.
- `apps/api/internal/conversations/service.go`.
- `apps/api/internal/agent/{provider.go,tools.go,orchestrator.go,provider_test.go}`.
- `apps/api/internal/config/{agent.go,agent_test.go}`.
- `apps/api/internal/http/stage5.go`.
- `apps/api/tests/integration/stage5_test.go`.
- `apps/web/src/lib/workflows.ts`.
- `apps/web/src/components/{WorkflowPage.tsx,Workflows.tsx}`.
- `apps/web/src/app/dashboard/ai-playground/page.tsx`, `orders/page.tsx`, `orders/[id]/page.tsx`.
- `docs/{ORDER_MANAGEMENT.md,AI_AGENT_IMPLEMENTATION.md,CONVERSATION_LIFECYCLE.md,STAGE_5_REPORT.md}`.

Изменены:

- Go `cmd/api/main.go`, `internal/http/{router.go,identity.go}`, `internal/platform/db/db.go`, `internal/organizations/service.go`, `internal/catalog/service.go`, `tests/integration/database_test.go`.
- Web `app/api/[...path]/route.ts`, `lib/{session.ts,catalog.ts}`, `components/{Sidebar.tsx,Header.tsx,CatalogWorkspace.tsx}`, `app/{globals.css,page.tsx,dashboard/page.tsx}`, `tests/{auth.spec.ts,smoke.spec.ts}`.
- `packages/contracts/openapi.yaml`, `scripts/check-openapi.mjs`, `scripts/{smoke.sh,smoke-compose.sh}`, `.github/workflows/ci.yml`, `docker-compose.yml`, `.env.example`.
- README, SECURITY, TECHNICAL_DECISIONS, TENANT_ISOLATION, API_DESIGN, DATABASE_DESIGN, AI_AGENT_ARCHITECTURE, SYSTEM_ARCHITECTURE, LOCAL_DEVELOPMENT; stage4 docs с уточнениями stage5.

Версии сохранены: Go1.26.2/pgx5.9.2/goose3.27.1/PostgreSQL18.3; Node24.15.0/Next16.4.0/React19.3.0/TS5.9.3/Tailwind4.3.3/ESLint10.12.0/Playwright1.64.0. CI forces mock/live=false; smoke scripts очищают LLM key environment, даже если host настроен иначе. Container browser outputs находятся в ignored `.local/smoke/<unique-project>`; test stacks удаляются своими cleanup, user dev volume не очищается.

## Схема, API и права

00004: organizations.is_test defaultfalse, members.can_manage_orders defaultfalse; inventory.manual_updated_at backfill только по прежним manual movements. Новые orders/items/events/quotes/conversations/messages/events/ai_usage FORCE RLS, compound FK, checks/indexes и immutable item/event privileges. Inventory movements kind/reserved before/after/delta/order FK и CHECK соответствия типу движения. Narrow definer helpers create_test_organization/list_organization_access/set_order_access/lock_order_products/is_test_org/can_operate_orders с fixed search_path, PUBLIC EXECUTE revoked и scoped verified identity. Ownership/runtime safety проверяется в AfterConnect; metadata4 readiness. Up additive, down guard не допускает удаления stage5 данных/test org/grants.

O=`/api/v1/organizations/{id}`:

| Путь | Методы/назначение |
|---|---|
| /api/v1/organizations/test | POST отдельного test магазина |
| O/orders | GET, status/page |
| O/orders/{resource} | GET snapshot/details/events |
| O/orders/{resource}/confirm, reject, cancel, complete | POST protected version/idempotent transition |
| O/conversations | GET/POST |
| O/conversations/{resource} | GET history/state |
| O/conversations/{resource}/messages | POST text/role/idempotency |
| O/conversations/{resource}/handoff, resume, delete | POST expected_version |
| O/conversations/{resource}/quotes | POST items, server quote |
| O/conversations/{resource}/confirm-request | POST quote_id/accepted/key |
| O/agent/status | GET mode/model/limits без ключа |
| O/members/{resource}/order-access | PATCH allowed, owner grant/revoke existing member |

Все session-auth; мутации Origin + CSRF. Owner/staff test scope, foreign/absent404; default member403, real org403. Request schemas strict; деньги целочисленные копейки RUB; version/key в JSON сохраняют стиль этапа4. Семь internal tools не опубликованы как generic execution endpoint.

## Фактические проверки

| Проверка | Результат | Подтверждение |
|---|---|---|
| Go unit/race | PASS | Auth/config/HTTP/shutdown regression; mock/protocol/local HTTP timeout/error/malformed, tools/limits/redaction |
| Go vet/gofmt/build | PASS | API/worker/migrate/healthcheck, writable GOCACHE |
| Real PG integration/race | PASS | Последний полный suite: 10.150s; identity/catalog/stage5, two tenants, real RLS, consent/idempotency/price/stock/staff |
| Migration clean up/down/repeat/concurrent up | PASS | Isolated aba_test marker/tmpfs, metadata4→3→2→1→4, nonempty guard |
| Reserve concurrency/partial rollback | PASS | Two orders/last unit: один reserve, один stock conflict; multi-item refusal откатывает всё |
| Cancel/complete once | PASS | Повтор успешного key не двигает stock повторно, snapshot immutable |
| Quote/prices/freshness | PASS | No consent, changed price before consent and staff, expired quote, stale/sold stock; manual freshness not reserve time |
| Staff/tenant/RLS | PASS | Foreign API404, tables FORCE/owner, direct SQL hidden/no context, tenant UPDATE spoof; staff API/SQL cannot change catalogue/settings |
| Tools/injection/limits | PASS в контрольных случаях | Org/URL/unknown tool rejected, model cannot forge consent, loop/excess calls, daily token/request quota, no arbitrary SQL/HTTP |
| Provider error/timeout/handoff race | PASS | Local fake HTTP/unit + real DB mock failure; human stops replies; racing run reply discarded |
| Frontend lint/typecheck/build | PASS | Native/Webpack and container production build |
| Browser functional/regression | PASS | 5 live scenarios (32.1s), full register/Mailpit/auth/org/catalog + sandbox/chat/consent/reserve/cancel/complete/handoff |
| Frontend price conflict | PASS | Другой tab изменяет цену, old quote409, orders остаётся0; новая quote показывает цену и требует нового checkbox |
| Actual outages/shutdown | PASS | PG stop ready503/live200; API/worker exit0; 1 real-offline browser scenario (7.3s) |
| Compose config/up/build/wait | PASS | Full synthetic stack healthy, controlled migration job |
| OpenAPI validation | PASS | Swagger parser + implemented path-set, новые схемы/ошибки |
| CI static | PASS | actionlint, minimal permissions, explicit mock, те же integration/race/browser commands |
| Remote GitHub Actions | NOT RUN | Нет разрешённого push |
| Native whole smoke / monolithic check.sh | NOT RUN целиком | Их отдельные проверки выполнены; выбран полный container smoke из-за окружения localhost/browser/socket |
| Live LLM/paid/model quality/monetary billing | NOT RUN | Не авторизовано, выбран только mock/local adapter fixture |
| Production/load/legal/penetration audit | NOT RUN | Не входит в этап; контрольные security cases не выдаются за полный audit |

Выявленные ошибки не скрывались: пустой required[] в OpenAPI исправлен; hanging timeout test fixture заменён bounded local server; container HTTP context не поддерживал crypto.randomUUID, frontend использует cryptographic getRandomValues UUIDv4 (без Math.random). Runtime network/LLM не подменялись фиктивным успехом. Один Docker registry TLS timeout устранился повторным build. Ранние browser FAIL после исправлений повторно проверены полным smoke, не исключены из suite.

## Критерии A–T

| Критерий | Статус | Проверка |
|---|---|---|
| A explicit chat consent/request | PASS | Browser checkbox/button + API/DB consent negative |
| B кабинет заявки | PASS | Browser detail/list snapshot |
| C confirm/reject | PASS | Browser confirm, integration reject/staff roles |
| D резерв после сотрудника | PASS | Browser inventory0 до, reserved1 после confirm |
| E нет overselling | PASS | Real PG goroutines last unit + multi-item rollback |
| F cancel release | PASS | DB repeat-key and browser stock0 |
| G complete consume | PASS | DB repeat-key/browser on_hand-1/reserved0 |
| H идемпотентность | PASS | Unique quote/key, same response repeat, no duplicate stock events |
| I price/stock conflicts | PASS | Changed/expired/stale/sold + UI re-consent |
| J scoped tools | PASS | Active catalogue, foreign IDs/schema injection failures, no tenant arg |
| K provider replaceable | PASS | Interface/Mock/Compatible local transport test; live separately NOT RUN |
| L mock без расходов | PASS | Mode/prefix, no key needed; CI/smoke forcedmock |
| M human handoff | PASS | Human no auto, racing reply fenced, explicit resume |
| N tenant/RLS | PASS | Two independent synthetic owners/orgs, SQL/API checks, scoped helper, pool reset |
| O chat/orders UI | PASS | Real BFF/SSR forms/actions/states/tenant sandbox separation |
| P regression | PASS | Previous identity/catalog/health/worker tests + browser |
| Q доступные checks | PASS в выполненном объёме | Component Go/web/PG/OpenAPI/CI static + full Docker smoke; NOT RUN отдельно |
| R Docker | PASS | Full build/up healthy + real outage/shutdown |
| S документация | PASS | Models/rules/config/limits/retention/commands/OpenAPI описаны |
| T secrets/real data | PASS в проверенном объёме | Publishable source и последние300 API/worker log lines без локальных DB/email/LLM ключей; .env/artifacts ignored, fixtures synthetic |

## Ограничения и согласования

- Только test org/orders/chat, не реальные продажи и не публичный канал. Ordinary store stock не участвует. Реальный pilot/Telegram source/customer principal должны быть реализованы в ТЗ6 отдельными тестами и политиками.
- Mock deterministic, реальный интеллект не заявлен. Live adapter не проверен на выбранной модели/аккаунте/доступности; Responses-only модели требуют другого adapter. Нет автоматического provider failover.
- Token/request caps не monetary budget. До paid live нужны модель, прайс/валюта/лимит денег, privacy/обработчик и unknown-usage reconciliation. Error/timeouts консервативно сохраняют reserved tokens.
- 7 дней temporary synthetic chat expiry; explicit erase/lazy purge payload/hash. Нет гарантированного cron удаления без обращений к tenant, физического удаления audit/order metadata и production backup/retention. До real PII —отдельное согласование.
- Worker пока dev email; synchronous AI bounded≤3s. Durable queue/system principal/idempotent outbound delivery —следующий этап. Owner/staff session не копируется в job.
- При price change —new quote/new consent/new request, старую pending отклонить/отменить; автоматической repricing нет. No payments/shipping/ERP/multiwarehouse.
- Оптимистичные версии + sorted product/inventory locks; coarse locking допустим для пилота, нагрузка не измерялась. Catalog updated_at/ручная сверка не доказывает физическое наличие.
- Нет staff invitation UI: owner grant только уже существующему member, settings rights не расширены. Authentication retention/MFA/recovery из прежних этапов остаются вне этого ТЗ.

## Запуск и ТЗ № 6

```sh
docker compose config --quiet
docker compose up --build -d --wait
sh scripts/migrate.sh status
sh scripts/check.sh
sh scripts/integration.sh
sh scripts/smoke-compose.sh
```

Сохранять существующий .env/volume. Default mock/live=false, новых ключей не нужно. Frontend http://127.0.0.1:3000, API8080, Mailpit8025. В playground создать отдельный test shop, вручную добавить синтетические active товары и stock, спросить SKU, quote→explicit consent→orders→employee confirm. Обычный магазин не переводится в test.

ТЗ6: Telegram ingress с проверкой webhook/dedup/inbox/outbox/PG queue; отдельный доверенный channel customer/system principal и tenant RLS; реальный consent callback связывается с Telegram sender/chat, не user staff session. До реальных данных утвердить privacy/retention, до live LLM модель/monetary caps/обработчика. Повтор delivery/last-unit race/handoff/freshness/price должны покрываться integration/e2e. По завершении остановиться и ждать ТЗ6 без commit/push.

Локальный dev stack обновлён с принудительным LLM_PROVIDER=mock/LLM_ENABLE_LIVE=false/empty LLM key и сохранением старого .env/volume. PostgreSQL/API/worker/web/Mailpit healthy; controlled migration00004 применена (status: 1–4 applied). Dev stack оставлен работающим, isolated test stacks удалены. Последний full PG suite10.150s, browser live5 passed32.1s/real-offline1 passed7.3s. Source/log secret scan выполнен без вывода значений; ссылки/fences документации проверены. Это проверка конкретных источников, не полный security audit.
