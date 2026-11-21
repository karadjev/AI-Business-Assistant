# Отчёт по ТЗ № 3

Дата: 12.10.2026. Реализованы аутентификация, организации и изоляция; реальные клиентские данные/внешняя почта/платные API/deploy отсутствуют. Commit/push не выполнялись. До работы прочитаны README, STAGE_2_REPORT, SYSTEM_ARCHITECTURE, DATABASE_DESIGN, API_DESIGN, SECURITY, TECHNICAL_DECISIONS и текущий код; git status и миграции проверены. Предыдущие файлы/документы и migration 00001 сохранены.

## Что реализовано

- Go modules auth/organizations, Argon2id 64 MiB/2/1, ограничения/нормализация email/password/name.
- Одноразовое email verification с hash-only token, expiry, resend и PostgreSQL rate limits; одинаковые ответы для duplicate/unknown/unverified там, где иначе раскрывается аккаунт.
- Atomic user+verification+AES-GCM email outbox; настоящий worker отправляет только в локальный Mailpit, очищает payload после ACK, retry/failed states.
- Opaque server sessions: SHA-256 только случайных ID/CSRF, HttpOnly cookies, Secure/__Host- в production, абсолютный и idle expiry, ротация login и отзыв logout. Session hashes нельзя перечислить app-role SELECT.
- Origin + session-bound CSRF header; JSON validation и body limits. Никакого localStorage auth.
- Организации и owner/member; несколько организаций; атомарное создание с owner, GET/list/PATCH, version conflict, никаких публичных member creation endpoints.
- Два tenant-owned tables с ENABLE/FORCE RLS; проверенная session hash + org ID в SET LOCAL transaction. Runtime aba_app не owner/superuser/BYPASSRLS. Конфигурация и AfterConnect блокируют ошибочно привилегированную runtime connection до auth/org запросов.
- Next.js allowlist BFF, SSR session checks, register/login/verify/dashboard/organizations/new/settings, понятные loading/error/empty states, logout, активная организация и XSS-safe текст.
- Migration 00002, OpenAPI 0.2.0, Docker Mailpit, CI и документация. Up additive; down отказывается на непустых identity tables.

## Архитектурные уточнения

Зафиксированы в TECHNICAL_DECISIONS: owner/member вместо более широкой проектной роли; auth/me вместо /me; membership composite PK; session-bound RLS вместо доверия app.user_id; узкие definer functions для bootstrap без BYPASSRLS; expected_version в PATCH вместо будущего If-Match/ETag; worker теперь dev-email processor. Первый пилот один магазин, данные тестов синтетические. Другие бизнес-модули не добавлены.

## Фактические проверки

| Проверка | Статус | Подтверждение |
|---|---|---|
| Go unit/race | PASS | `go test -race -count=1 ./...` после реализации; auth/config/HTTP/shutdown regression |
| Go formatting/vet/build | PASS | gofmt, `go vet ./...`, отдельные API/worker/migrate/healthcheck builds с writable GOCACHE |
| PostgreSQL integration | PASS | Несколько свежих aba_test с marker, без dev DB; реальный Mailpit; последний запуск включает две занятые физические pool connections |
| Up/down/repeat/concurrent up | PASS | Blank database roundtrip; metadata 1→2→1→2; advisory lock; nonempty down refuses |
| Auth/token/session | PASS | Validation/duplicate, unverified/wrong password, token expiry/reuse/resend, fixation rotation, logout, idle/absolute expiry, production cookie flags |
| Role/API isolation | PASS | A/A2 vs B; GET/PATCH foreign=404; member PATCH=403; own owner update и version conflict |
| RLS/pool isolation | PASS | SQL foreign SELECT/UPDATE=0, member UPDATE=0, no-context=0; commit/rollback reset; два одновременно занятых соединения с разными tenants |
| CSRF/rate limits | PASS | Missing Origin/header rejection, login/resend quota, 429; session-bound CSRF |
| Browser full scenario | PASS | Register→Mailpit→verify→login→refresh→create→edit→XSS safe→logout→protected redirect |
| Browser regression/outages | PASS | 5 сценариев при live backend + 1 после настоящей остановки API; PostgreSQL outage ready=503/live=200; worker/API SIGTERM exit=0 |
| Frontend lint/typecheck/build | PASS | ESLint без ошибок, tsc, полноценный Next.js Docker/Webpack build |
| OpenAPI validation | PASS | Swagger parser и проверка реально зарегистрированных endpoints |
| Compose | PASS | config, полный up/build/wait, postgres/api/worker/web/mailpit healthy |
| CI static | PASS | actionlint 1.7.7; реальные команды воспроизводимы, Mailpit service и integration/race configured |
| GitHub Actions remote | NOT RUN | Нет разрешённого push; static/local проверки не выдаются за GitHub run |
| Native smoke.sh целиком | NOT RUN | Выполнен полный контейнерный smoke-compose; native browser/socket ограничения окружения |
| Production HTTPS/SMTP/security penetration audit | NOT RUN | Production deployment и внешняя почта не входят в задание |

Первый lint обнаружил set-state-in-effect в verify form; исправлено на ref/DOM synchronization без отключения правила. Один повторный Docker build получил registry TLS handshake timeout; повторный полный build/up успешно завершён. Ошибки не скрывались заглушками, тесты не исключались ради PASS.

## Критерии A–P

| Критерий | Статус | Доказательство |
|---|---|---|
| A —регистрация | PASS | Go integration + browser |
| B —подтверждение через dev sink | PASS | Реальная доставка SMTP/Mailpit, browser подтверждение |
| C —login/logout | PASS | Go и browser, серверный revoke |
| D —persist/expiry | PASS | Реальное page reload; DB expiry/revocation tests |
| E —без session доступа нет | PASS | API 401, SSR redirect для dashboard/settings |
| F —создание организации | PASS | Browser/DB transaction с owner |
| G —редактирование owner | PASS | Реальный PATCH и обновление списка |
| H —чужие данные API/DB | PASS | Foreign 404 + прямой SQL RLS=0 |
| I —RLS/tenant proof | PASS | FORCE/ownership checks, commit/rollback + отдельные physical pool connections |
| J —CSRF/rate/session limits | PASS | Negative CSRF, account counters, invalid/reused/expired tokens/sessions, cookie flags |
| K —frontend формы/состояния | PASS | Полный Chromium scenario, XSS escaping, пустые orgs, errors/loading |
| L —health/worker regression | PASS | Live/ready/status, реальные outages, delivery/shutdown |
| M —Compose | PASS | Пять runtime services healthy, controlled migration job |
| N —quality checks | PASS | Unit race/vet/build, frontend checks, OpenAPI; remote CI отдельно NOT RUN |
| O —документация | PASS | README/AUTHENTICATION/TENANT_ISOLATION/LOCAL_DEVELOPMENT/OpenAPI согласованы с реализацией |
| P —нет секретов/real client data в repo | PASS в проверенном объёме | .env/тестовые artifacts ignored, provider secrets отсутствуют; только synthetic fixtures, токены не логируются/не возвращаются API |

## Локальный запуск

```sh
docker compose up --build -d --wait
docker compose ps
sh scripts/migrate.sh status
```

Текущий .env дополнен отдельным случайным AUTH_EMAIL_KEY и сохранён ignored, его значения не печатались. Не перезаписывать .env. PUBLIC_ORIGIN должен совпадать с URL браузера (default http://127.0.0.1:3000). Страницы /register, /login, /verify-email, /dashboard/organizations/new; Mailpit http://127.0.0.1:8025. Регистрация synthetic owner@example.test →последнее письмо→кнопка verify→login→магазин. API/worker/БД/web/mailpit оставлены запущенными; тестовые stacks удаляются отдельно, dev volume не очищался.

Команды тестов: `sh scripts/check.sh`, `sh scripts/integration.sh`, `sh scripts/smoke-compose.sh`; инструкции по native mode/конфигурации —LOCAL_DEVELOPMENT и AUTHENTICATION. API paths/schemas —packages/contracts/openapi.yaml.

## Ограничения

Нет каталога, заказов, Telegram, LLM, платежей, реальной почтовой рассылки или production deployment. Нет password recovery/MFA/invites/account deletion/session-management UI. Нет автоматического retention cleanup для auth state: перед расширением трафика отдельная maintenance задача. Rate limit по API peer IP за BFF общий плюс отдельный email quota; trusted proxy/client-IP topology для production ещё не определена. Email at-least-once может дать повтор письма; verification consumption атомарен. Mailpit dev mailbox не production email provider.

RLS защищает tenant operations при заданной доверенной runtime boundary; не заявляется защита от полного взлома Go process/DB credentials/DBA. Runtime global auth privileges описаны явно, session enumeration закрыта. Для будущих noninteractive AI workers потребуется отдельный system/job principal, не owner session и не migration role. Тесты не равны penetration audit или юридическому заключению.

Дальше: ТЗ № 4 —каталог/варианты/цены/остатки с org-scoped API, составными FK и RLS, owner/member permissions, конкурентными обновлениями и тестами A/B. Не подключать LLM/Telegram до согласованного соответствующего этапа. Этот этап завершить остановкой, без commit/push.

## Файлы реализации

Новые: `apps/api/internal/auth/{security.go,security_test.go,service.go,email.go}`, `internal/config/{auth.go,auth_test.go}`, `internal/http/identity.go`, `internal/organizations/service.go`, `migrations/00002_identity_tenants.sql`, `tests/integration/identity_test.go`; frontend `api/[...path]/route.ts`, `lib/session.ts`, `components/IdentityForm.tsx`, register/login/verify-email, dashboard layout/organizations/new/settings pages, `tests/auth.spec.ts`; документы AUTHENTICATION.md, TENANT_ISOLATION.md, STAGE_3_REPORT.md.

Обновлены: Go API/worker entrypoints, config/validation tests, pool/readiness, HTTP router, migration integration runner, go.mod/go.sum; frontend Header/Sidebar/home/dashboard/styles и smoke regression; Docker Compose/host/test overrides; CI; OpenAPI и validator; smoke/native/integration scripts; README/LOCAL_DEVELOPMENT и актуальные архитектурные примечания SYSTEM_ARCHITECTURE/SECURITY/DATABASE_DESIGN/API_DESIGN/TECHNICAL_DECISIONS/DEVELOPMENT_ROADMAP. `.env` обновлён только локально и excluded.

Заключительная проверка: локальные реальные dev DB/email ключи отсутствуют в publishable source и проверенных application logs; Markdown ссылки/fences и shell syntax валидны. Это проверка конкретных файлов/logs, не всеобъемлющий security audit.
