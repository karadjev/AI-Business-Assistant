# Stage 3.0 — следующий релиз AI Business Assistant

Исходная ветка сверена с GitHub на `5bc1033`. Работа выполнена в отдельном worktree `work/stage-3-core-product`. Публичный релиз `7a84b6a` и его манифест `949977f4997aab73e3e4b18e6a801e31510cab035d032d6613cdb9ba56a171d7` остаются действующими. Изменений на VPS и миграций в Stage 3.0 нет.

## Объём кандидата

- Business Manager возвращает различимые итоги по состоянию каждого специалиста. Финансовый и аналитический worker отклоняет неполную проекцию вместо подстановки нулей.
- В AI Team обновляются существующие и дочерние задачи. Пользователь видит прогресс, статус, частичный результат и решения по согласованию. При потере связи есть ограниченное ожидание и ручное обновление.
- Отраслевые формы используют авторизованные BFF и существующие сервисы. Автосервис доходит до согласования заказа и резерва. Для стройматериалов, запчастей и объектов недвижимости формы показывают сохранённый результат worker. [Матрица возможностей и ограничения](STAGE_3_CAPABILITY_MATRIX.md)

## Допуск к следующей публикации

Перед отдельным решением о выпуске собрать новый неизменяемый манифест со свежими API/worker и web, согласовать актуальные материалы и проверить provenance и advisories. Одноразовое решение владельца для `7a84b6a` относится к прежнему выпуску. Node.js в проверочной Linux-сборке поступает из подготовленного offline cache.

Новым формам требуется проверка edge-конфигурации в отдельном production-like контуре. На публичном frontend-host должны маршрутизироваться `GET /api/v1/organizations/{id}/conversations`, `POST /api/v1/organizations/{id}/operations/services/search` и существующие маршруты делегирования, истории и согласования. Проверить отказ без сессии, с отсутствующим CSRF и для чужой организации. Прямой API, финансовые допуски, live webhooks, правила отправки и ограничения email сохраняют текущий режим.

Кандидат совместим со схемой 31. Настоящий server promotion требует заново сверить прежние образы, backup и обязательный override активного PostgreSQL volume по текущему runbook. [Процедура выката и отката](VPS_DEPLOYMENT_RUNBOOK.md)

## Проверки Stage 3.0

- PASS: Go unit, race, vet и build; deployment unit tests, OpenAPI validation и artifact guard.
- PASS: свежая guarded Linux production build Web, проверка served marker и lint/typecheck.
- PASS: Chromium на этой свежей сборке с изолированным PostgreSQL: 4/4 отраслевых сценария (автосервис с заказом, одобрением и резервом; стройматериалы; автозапчасти; недвижимость), 2/2 сценария AI Team (CSRF, роль, tenant isolation, approval, audit, дочерний результат) и отдельный worker restart сценарий.
- PASS: полная изолированная PostgreSQL regression suite с FORCE RLS, всеми Stage 25 operations, новой проверкой отказа всех специалистов Business Manager и гонками Stage 26. Чистый повтор завершился PASS; более ранний отдельный запуск дал единичный `SERVICE_UNAVAILABLE` в тесте гонки резервирования, а чистый повтор прошёл.
- NOT RUN: Chromium против публичного домена и production-like edge-маршрутизации; production не менялся.
- NOT RUN: production manifest, image provenance/advisory review и отдельное решение о выпуске. Stage 3.0 не является опубликованным релизом.

Локальные журналы и точные артефакты проверок сохранены вне git в `.local/stage-3-core-product/evidence`, включая `integration-rerun2.log`, `industry-final-browser.log`, `agent-team-final-browser.log` и `agent-team-restart-final.log`. Первый browser locator-прогон также сохранён отдельно; итоговый прогон новой guarded сборки прошёл.
