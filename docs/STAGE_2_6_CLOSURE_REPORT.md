# Stage 2.6 Closure

Stage 2.6 остаётся открытым. Исходная матрица и исторические PASS и FAIL сохранены. Базовые резервирование, восстановление worker, миграции 00021/00022 и 19 браузерных сценариев взяты из предыдущего этапа.

[Исходный отчёт](STAGE_2_6_OPERATIONAL_COMPLETION_REPORT.md) · [Исторические проверки](../.local/stage-2-6/INDEX.md)

## E01 — BLOCKED

Проверен архив `.local/build-env/web/.next/standalone`. Исходный SHA-256 proof — `13b9728f814667403270b3a07e2b2e73b980895a627237dc3d2f2fa15fdbe6b4`. Ожидаемый digest дерева — `3c46c76c160881930262971af5eb971a158a768a09417ee108c427c12b939e13`, фактический — `d08ec358f11d17cdfecd389742957c4b85f5a525b104943d074968e5b2d361c1`. Диагностический пересчёт в памяти с пропуском ровно 15 известных пустых каталогов совпал с ожидаемым digest. Состав имён и сохранённые метаданные подтверждены. Артефакт и proof сохранены.

Затронуты дерево Next.js standalone и проверка его пригодности для запуска. Архив 28 октября содержит наблюдения появления путей и снимки процессов с открытыми дескрипторами. Они не содержат записи успешного mkdir, rename или copy, которую можно связать с PID и PPID. Запрос к macOS Unified Log за 10:53:30–11:52:00 UTC вернул только заголовок. Журнал сообщил о корректировке часов. Эксперименты с чистой и повторной сборкой не воспроизвели дефект. В исторических манифестах Next наблюдавшиеся имена отсутствуют. Достоверно восстановить writer или его родителя из этих данных нельзя.

Причина и направленное исправление не установлены. Регрессия guard и offline preflight прошла 6/6, архив отклонён с exit 42. Файлы integrity guard и исторический proof сохранили SHA-256.

[Исторические build и trace evidence](../.local/stage-2-6/e01/INDEX.md) · [Повторный снимок](../.local/stage-2-6-closure/e01-tree.json) · [Исторический журнал](../.local/stage-2-6-closure/e01-october-retry.log) · [Тесты guard](../.local/stage-2-6-closure/e01-tests.log)

## Сценарии B–E

| Gate | Статус | Подтверждённая граница |
| --- | --- | --- |
| B, автозапчасти | NOT RUN | Существует staff workflow с typed search, fitment и quote. Полная durable цепочка Customer → Sales → Product & Inventory → Compatibility → Sales отсутствует. |
| C, стройматериалы | NOT RUN | Существуют staff workflow и детерминированный расчёт. Полная durable цепочка до Sales и Business Manager отсутствует. |
| D, автосервис | NOT RUN | Атомарный резерв и staff запись уже проверены. Нет durable цепочки, которая сочетает подбор услуги и запчасти с действующим подтверждением заказа. |
| E, недвижимость | NOT RUN | Поиск и intake через staff API подтверждены. Нет durable делегирования поиска и передачи результата на Human Approval. |

Строго типизированные staff previews используют разрешения текущей сессии. Worker работает с lease и ограниченными проекциями. Для полноценных цепочек нужны явно разрешённые lease-проекции для каждого шага, durable admission и проверка согласия или полномочия на запись.

## Marketing и approval

Миграция 00023 сохраняет контекстный черновик в той же транзакции, в которой существующий worker завершает execution. Источником контекста служит связанный execution. Хранятся цель, аудитория, канал, содержание, tenant, время, версия и состояние. PostgreSQL FORCE RLS и owner scope ограничивают чтение. Действия владельца выполняются через узкую SQL-функцию и фиксируются в audit.

Генерация и решение по материалу имеют разные жизненные циклы. Отказ до генерации оставляет задачу отменённой. Отказ после генерации сохраняет текст и опциональную причину. Редактирование отклонённого материала создаёт следующую версию, доступную для отдельного решения. Повтор решения и устаревшая версия отклоняются. Операции утверждения ограничены состоянием записи о согласовании.

Полная генерация остаётся BLOCKED. Текущий путь использует существующий синтетический provider с ограниченными фактами Growth. Единственный другой существующий provider требует live-конфигурацию и выделенный бюджет. Утверждённые цели, сегменты, политики и многоканальный контекст ещё не подключены. Черновики текущей итерации относятся к контекстному Growth.

## Проверки и приёмка

| Критерий | Статус | Результат |
| --- | --- | --- |
| E01 root cause и исправление | BLOCKED | Нет process lineage для архивного события, контрольная сборка его не воспроизвела. |
| E01 guard | PASS | Exit 42 на архивном артефакте, 6/6 regression tests. |
| B–E, durable через worker | NOT RUN | Реализации агентных цепочек остаются открытыми. |
| Marketing, полная персонализированная генерация | BLOCKED | Выполняется синтетический Growth draft. Разрешённый генеративный режим существующего engine и полная проекция контекста отсутствуют. |
| Marketing, durable draft persistence | PASS | Реальный worker сохраняет версию с tenant, контекстной ссылкой и содержанием. |
| Approval | PASS | Отказ до генерации, отказ с причиной после генерации, сохранение исходного текста, повторное согласование новой версии, fencing повторов и audit. |
| Migration 00023 и tenant isolation | PASS | Up/down/up на пустой схеме, отказ отката со строками, 61 таблица с FORCE RLS и изоляция черновиков. SHA 00001–00022 совпадают с baseline. |
| Go и frontend | PASS | Go test, race, vet, сборки API/worker, lint, typecheck и свежий Next.js build. |
| PostgreSQL regression | PASS | Финальная полная suite завершилась с exit 0. Оба промежуточных FAIL сохранены. |
| Chromium regression | PASS | 19/19 через Go API, worker и изолированную PostgreSQL. Два отдельных теста после перезапуска worker. |
| CODE COMPLETE | BLOCKED | Открыты E01, клиентские цепочки B–E и полноценный Marketing. |
| RUNTIME VERIFIED | PASS | Только для реализованных операций и регрессионного набора. Полный closure runtime ещё не подтверждён. |

[Финальная PostgreSQL](../.local/stage-2-6-closure/postgres-run.0ZWzlwgS/integration.log) · [Chromium](../.local/stage-2-6-closure/e2e/run.mJVuLPsO/browser.log) · [Worker restart](../.local/stage-2-6-closure/e2e/run.mJVuLPsO/browser-restart.log) · [Hybrid restart](../.local/stage-2-6-closure/e2e/run.mJVuLPsO/browser-hybrid-restart.log)

## Изменённые файлы

- `apps/api/migrations/00023_marketing_drafts.sql`, `apps/api/internal/platform/db/db.go`
- `apps/api/internal/agentruntime/marketing_drafts.go`, `apps/api/internal/http/agents.go`, Next.js BFF `apps/web/src/app/api/[...path]/route.ts`
- `apps/api/tests/integration/{audit,database,instagram_schema_stage22,stage26_marketing}_test.go`, `apps/web/tests/stage26-marketing-real-api.spec.ts`

## Следующие приоритеты

1. Для E01 сопоставить успешную операцию записи с процессом и родителем при воспроизведении на отдельной копии.
2. Подключить durable цепочки B–E с ограниченными проекциями и существующими правилами подтверждения.
3. Подключить утверждённые источники маркетингового контекста к генерации через существующий engine и подтвердить её в разрешённой конфигурации.

[Индекс closure](../.local/stage-2-6-closure/INDEX.md)
