# План разработки

Оценки для одного разработчика с Codex, предполагают опыт Go/SQL/React и уже согласованные требования. День = примерно 6 часов сосредоточенной работы; Codex ускоряет черновики, но не отменяет ревью, отладку и проверку. Диапазоны — человеко-дни, не обещанные календарные сроки. Каждая стадия заканчивается review и доказательством критериев готовности. Этап 0 завершён; этап 1 реализован по ТЗ № 2, auth/организации/изоляция —по согласованному scope ТЗ № 3 (восстановление пароля остаётся отдельной задачей). Фактические проверки фиксируются в отдельном отчёте, не выводятся из этого плана.

## Этап 0. Архитектура и согласование — ТЗ № 1, завершён

Цель: зафиксировать границы и технические решения. Функции: документация, схема и контракты, риски. Зависимости: исходное ТЗ и пустой репозиторий. Файлы: docs/*.md, README, .gitignore, пустые каталоги. Готовность: все пункты ТЗ покрыты, открытые решения перечислены. Проверка: комплектность, локальные ссылки, согласованность терминов; программных тестов нет. Риск: молчаливое принятие допущений. Оценка последующего review/согласования: 1–3 дня, текущая подготовка документов отдельно не включена в прогноз реализации.

## Этап 1. Каркас, контракты и окружение — 3–5 дней

Цель: воспроизводимая разработка. Функции: Go api/worker entrypoints, конфигурация, health/readiness, Next.js skeleton, Compose PostgreSQL, миграции, OpenAPI, CI. Зависимости: согласованный архитектурный вариант, версии инструментов. Файлы: apps/api/cmd/*, internal/platform/*, migrations, apps/web/src/app, packages/contracts/openapi.yaml, infrastructure/compose.yaml, .github/workflows/ci.yaml, scripts/check. Готовность: новый checkout запускается по README, CI строит оба приложения; никаких production secrets. Тесты: config validation, миграции с нуля на PostgreSQL, API smoke/health, typecheck/build. Риск: version mismatch и сложный CI.

## Этап 2. Authentication, Organizations и изоляция — 6–10 дней

Цель: безопасный tenant foundation. Функции: email verification/reset через mock/local mailbox, sessions/CSRF, создание org+owner, RLS, RBAC interfaces, аудит. Зависимости: 1. Файлы: internal/{auth,organizations,members}, migrations identity/tenants, web auth/onboarding/settings. Готовность: пользователи двух организаций не видят чужие записи; account reset отзывает sessions. Тесты: реальные DB RLS/FK/pool, отказ без контекста, bootstrap membership, login/reset enumeration, CSRF, последний owner. Риск: опасный bootstrap privilege/RLS recursion. Публичный email transport подключить позднее после выбора сервиса.

## Этап 3. Каталог и остатки — 5–8 дней

Цель: проверяемый источник фактов. Функции: товары/варианты, SKU, price version, stock adjustment, freshness, поиск; кабинет Products. Зависимости: 2. Файлы: internal/{products,inventory}, queries, migrations catalog, web products. Готовность: CRUD с конфликтами версий; невозможен отрицательный stock и чужая variant reference. Тесты: денежные значения, archived visibility, concurrent adjustments, freshness/unknown, поиск моделей/памяти/цвета. Риск: грязный каталог и неактуальные данные магазина.

## Этап 4. Заявки и резерв — 5–8 дней

Цель: транзакционно корректные продажи. Функции: requested/confirmed/completed/cancelled, snapshots, confirmation/idempotency, stock movements, notifications inbox, Orders. Зависимости: 2–3; customer/conversation минимальный foundation создаётся здесь для FK, полноценный канал на этапе 5. Файлы: internal/{orders,notifications,conversations}, migrations orders/confirmation/customer, web orders. Готовность: одна последняя единица не резервируется двумя заказами, повтор не списывает дважды. Тесты: многопозиционный rollback, все переходы, повтор key с другим body, цена изменилась, чужое согласие/заказ, cancelled/retry. Риск: неверная семантика заявки и резерва.

## Этап 5. Диалоги, jobs и Telegram — 5–9 дней

Цель: надёжный один канал без ИИ. Функции: inbox/outbox, leased worker, Telegram adapter, identity binding, callback, ручные ответы, handoff, кабинет Conversations/Integrations. Зависимости: 2,4; HTTPS test endpoint и явная авторизация подключения реального бота на соответствующем задании. Файлы: internal/integrations/telegram, internal/platform/jobs, migrations jobs/inbox/messages, web conversations/integrations. Готовность: webhook ACK после commit; дубль не создаёт вторую заявку; manual mode защищён. Тесты: fake provider, spoof/replay, worker crash/lease, 429, uncertain send, out-of-order, handoff race; затем честно отмеченный sandbox check. Риск: невозможно гарантировать exactly-once внешней отправки; отсутствие Telegram у целевых покупателей.

## Этап 6. Агент, tools и расходы — 7–12 дней

Цель: полезные контролируемые ответы. Функции: provider interface+mock, один согласованный LLM adapter, версии агента, 7 tools, факт-шаблоны, quote, budget reserve/reconcile, timeouts/handoff. Зависимости: 3–5, выбор провайдера и правовые условия. Файлы: internal/agents/{orchestrator,tools,providers,policy}, migrations agent_versions/ai_runs/budget, web agent, tests/evals. Готовность: end-to-end каталог → согласие → requested; отсутствие выдуманных товарных фактов в согласованном наборе; budget race закрыт. Тесты: evals AI_AGENT_ARCHITECTURE, инструменты независимо от LLM, prompt injection, concurrent budget, pricing changes, unknown usage. Платные проверки только в пределах отдельно утверждённого бюджета. Риск: модель может не поддержать строгий контракт или окажется экономически невыгодной.

## Этап 7. Кабинет, агрегаты и эксплуатация — 4–7 дней

Цель: рабочий продукт для владельца. Функции: Dashboard/Analytics, usage/plan, notifications, responsive polish, redacted logs/metrics, backup/restore scripts и runbook. Зависимости: 2–6. Файлы: internal/{analytics,billing}, web dashboard/analytics/settings, infrastructure/{observability,backup}, scripts/{backup,restore}, docs operational runbooks. Готовность: метрики совпадают с исходными событиями, ошибки понятны, секреты скрыты, restore drill подтверждён. Тесты: aggregate fixtures/timezones/zero denominators, права viewer, mobile/keyboard smoke, backup restoration. Риск: ошибочная интерпретация конверсии и недостаточные ресурсы эксплуатации.

## Этап 8. Стабилизация и ограниченный пилот — 6–10 дней

Цель: проверить продукт на сквозных сценариях и реальном каталоге. Функции: исправления, onboarding одного магазина электроники, нагрузочный/tenant/security audit, инструкции владельцам; публикация только отдельным заданием после подготовки проверяемого результата. Зависимости: 1–7, юридический и эксплуатационный go/no-go. Файлы: tests/e2e, tests/load, docs/pilot-runbook и измеренные отчёты; изменения по найденным дефектам. Готовность: MVP acceptance выполнен, риски и известные ограничения записаны, назначен ответственный за инциденты. Тесты: полный сценарий двух tenants, последние единицы, восстановление, недоступность провайдера/БД, rate limits, billing cap. Риск: реальные каналы/продажи обнаружат неполноту правил; время ожидания approvals не входит в человеко-дни.

## Общая оценка и порядок

Этапы 1–8: 41–69 человеко-дней (246–414 часов), плюс резерв 25–35% на интеграционные проблемы и изменения: примерно 51–93 дня. При 5 полных рабочих днях в неделю ориентир 10–19 недель; при частичной занятости дольше. Провайдерские согласования, юридическая работа, поиск магазинов и ожидание внешних сервисов отдельно и могут увеличить календарный срок. Пересчитать после этапа 3 по реальной скорости, не планировать коммерческий запуск по одной этой оценке.

При нехватке времени сократить polish/аналитику/расширенные настройки, но не изоляцию, подтверждения, stock correctness и recovery. Самый ранний проверяемый вертикальный срез — один магазин и синтетический чат на mock LLM после этапов 2–4; это ещё не готовый MVP.

Следующее рекомендуемое задание: согласовать решения TECHNICAL_DECISIONS, затем этап 1 — Go/Next skeleton, локальная БД, OpenAPI и CI без платных API и deployment. Не начинать его автоматически в рамках текущего ТЗ.
