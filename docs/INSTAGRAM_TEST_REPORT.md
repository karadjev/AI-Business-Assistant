# Instagram — отчёт проверок

Дата: 2026-11-18. Только результаты этой сессии; исторические PASS не переносились.

| Проверка | Результат | Доказательство / ограничение |
| --- | --- | --- |
| gofmt новых/затронутых Go-файлов | PASS | `.local/instagram-20261118/format.log` |
| Frontend lint, весь проект | PASS | `npm run lint`, exit 0; `frontend-lint.log` |
| Frontend typecheck | PASS | `tsc --noEmit --incremental false`, exit 0; `frontend-typecheck.log` |
| Go unit: Instagram/channel/config/Telegram/WhatsApp | BLOCKED | Go module требует 1.26.11; host 1.26.2. Exit 1 до запуска тестов; `go-unit.log` |
| Prepared isolated runner | BLOCKED | Docker daemon недоступен; `prepared-runner.log` |
| Go race/vet/build | BLOCKED | Изолированный runner недоступен; команды не запускались |
| Migration/RLS/three-tenant DB/concurrency | NOT RUN | Новая миграция отсутствует; runner недоступен |
| Shared AI/catalog/order integration | NOT RUN | Instagram queue/capability wiring отсутствует |
| Telegram/WhatsApp regressions | BLOCKED | Unit command остановлен toolchain gate; интеграционные тесты не запускались |
| Fresh Next production build / current-build E2E | BLOCKED | Prepared runner недоступен; build/proof guards не обходились |
| Instagram desktop/mobile E2E | NOT RUN | Написан `instagram-production.spec.ts`, нужен guarded build + isolated fixture |
| Live OAuth/webhook/message sending | NOT RUN | Не авторизовано; runtime выключен |

Полные доступные логи и команды: `.local/instagram-20261118/checks.json`, `frontend-checks.json`; файлы ignored, не публиковались. Lint/typecheck выполнялись локально установленными инструментами offline, не в подготовленном Linux runner; это не заменяет production build gate.

## Написанные Go tests (не считать PASS)

HMAC и challenge, size/JSON validation, mixed-event handling, batch duplicates, replay age, unknown/foreign account; три tenant binding fixtures; credential isolation/tamper; OAuth CSRF state/session/user/tenant/expiry/replay/cancel; offline code exchange/long token/refresh/permissions; sending policy/24h boundary/token expiry; 1000-byte limit; provider confirmation, 429/backoff bounds, timeout/5xx/unknown delivery. Config tests проверяют запрет live включения и credentials.

Tests используют synthetic fixtures и injected functions без реальных HTTP отправок. Memory StateStore используется только в unit tests. Нет DB-тестов Instagram conversation/token/account ownership, durable duplicate handling, DLQ, leases или повторных заказов; они обязательны для завершения.

## Итоговые release labels

- **CODE COMPLETE: NO** — offline foundation, не полноценный третий канал.
- **META APP REVIEW PENDING**.
- **LIVE CONNECTION NOT TESTED**.
- **PRODUCTION READY: NO — NOT READY**.
- **E01: OPEN** — causal proof не получен, guards не изменены.

## Изменения и следующий gate

Изменены `.env.example`, `internal/channel/channel.go`, `internal/config/config.go`, integrations listing. Добавлены `internal/config/instagram{,_test}.go`, `internal/instagram/{policy,webhook,handler,oauth,token,transport,instagram_test,token_test}.go`, frontend Instagram page и production E2E, пять `docs/INSTAGRAM_*.md`.

Миграций и зарегистрированных backend endpoints: **0**. Frontend route: `/dashboard/integrations/instagram`. Это явное ограничение реализованного объёма, не ожидание Meta credentials как единственный blocker.

Для первого магазина остаются: shared SQL/RLS/queue migration и tests; durable OAuth/status/subscription/disconnect; worker + общий AI/inbox/orders; attachments/profile/history; все connection/error states и inbox filters; полный regression/build/browser gate; Meta checklist; отдельное разрешение live. Нужен доступный подготовленный offline runner с Go1.26.11; скачивания, запуск Docker Desktop или изменение глобальных настроек не выполнялись.
