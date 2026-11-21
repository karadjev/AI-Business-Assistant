# Stage 2.6 — Final Integration

Stage 2.6 остаётся открытым. Исторический baseline включает миграции 00021–00023, атомарный резерв, восстановление worker, хранение и согласование версий черновиков, FORCE RLS на 61 таблице и Chromium 19/19. Исходные PASS и FAIL сохранены.

[Closure report](STAGE_2_6_CLOSURE_REPORT.md) · [Operational completion](STAGE_2_6_OPERATIONAL_COMPLETION_REPORT.md) · [Исторические проверки](../.local/stage-2-6-closure/INDEX.md)

## Сценарии B–E

| Gate | Статус | Граница |
| --- | --- | --- |
| B, автозапчасти | NOT RUN | Typed search, fitment, quote и последующий consent проверены в baseline режима сотрудника. Цепочка Sales → Product & Inventory → Compatibility → Sales → Business Manager через worker отсутствует. |
| C, стройматериалы | NOT RUN | Расчёт и упаковка подтверждены для staff preview. Durable цепочка расчёта, предложения и агрегации отсутствует. |
| D, автосервис | NOT RUN | Существующий атомарный резерв товара и слота подтверждён. Durable цепочка подбора услуги, запчасти и согласованного бронирования отсутствует. |
| E, недвижимость | NOT RUN | Фильтры, проверка актуальности и staff intake подтверждены. Durable поиск, согласование и результат Business Manager отсутствуют. |
| Agent-to-Agent, Orchestrator и worker | NOT RUN | Для B–E текущий Orchestrator регистрирует один read-only инструмент Customer Agent. Очередь и fencing есть у других задач, включая Growth, и не заменяют выполнение цепочек B–E. |
| Security и recovery B–E | NOT RUN | Для новых цепочек не проверены tenant scope, retry, correlation, parent/child persistence и защита от повторной записи. |

Staff adapter требует текущую сессию сотрудника и отклоняет agent lease. Существующий worker использует отдельный lease с привязкой к задаче. Для B–E нужно добавить узкие lease-проекции каталога и расписания, привязать запись к конкретному решению и передавать готовые результаты через действующие execution и queue tables. Staff session не переносилась в очередь.

## Marketing

| Gate | Статус | Основание |
| --- | --- | --- |
| Generative runtime | BLOCKED | Подготовленный runner использует `LLM_PROVIDER=mock`, `LLM_ENABLE_LIVE=false` и бюджет 0. Для `openai_compatible` существующий engine требует отдельное разрешение, готовую конфигурацию, модель, credential и зарезервированный бюджет. |
| Полная проекция бизнес-контекста | NOT RUN | Текущий Growth использует узкую tenant/lease-проекцию отрасли, наличия и спроса. Цели, сегменты, утверждённая аналитика, политики и разрешённые каналы ещё не подключены. |
| Draft persistence и Human Approval | PASS, baseline | Синтетические черновики, версии, отказ, повторное согласование и tenant scope подтверждены Closure. |
| Черновик полноценной генерации после restart | NOT RUN | Ожидает разрешённую генерацию и проверку полного runtime. |

Текущий Marketing использует собственный синтетический provider через контракт `agent.Provider`. Существующий генеративный provider расположен в `internal/agent/provider.go`. Он требует budget permit перед вызовом. Генеративная проверка не выполнялась. Тесты синтетического режима остались отдельно от этого gate.

## E01 и containment

**Root cause — UNDETERMINED. E01 — BLOCKED.** Историческая перепроверка хешей, метаданных и журнала остаётся точкой отсчёта. Новый read-only снимок архива совпал по исходному proof и digest. Ожидаемый digest standalone `3c46c76c160881930262971af5eb971a158a768a09417ee108c427c12b939e13`. Фактический digest `d08ec358f11d17cdfecd389742957c4b85f5a525b104943d074968e5b2d361c1`. Диагностическое исключение ровно 15 пустых каталогов в памяти воспроизвело ожидаемый digest. Исторические наблюдения не связывают изменение с PID или PPID. Guard и доказательства сохранены.

**Containment — PASS.** Подготовленный Compose отделяет новый рабочий каталог `.local/build-env/web-verified` от архива `.local/build-env/web`. Запуск web предварительно проверяет proof. Команда сборки также отказывает до любых записей в архив при обнаружении E01. Проверены раздельные mounts и два запуска проверки на архиве с exit 42, включая отказ до команды старта. Мера применяется к подготовленному Compose и связанным build scripts.

Запись provenance создаётся в отдельном каталоге. Она содержит UTC, последовательность argv и exit, PID/PPID, идентификатор commit при наличии, digest исходников, digest standalone и SHA-256 proof. Значение commit из окружения помечается как указанное вызывающей стороной. Git checkout без HEAD отражается как `null`. Запись не включает секреты и содержимое окружения.

## Проверки и уровни приёмки

| Проверка | Статус | Результат |
| --- | --- | --- |
| E01 guard/preflight | PASS | 6/6, архив отклонён exit 42. Proof и guard сохранили исходный SHA. |
| Первый fresh build | FAIL | Next worker завершился SIGABRT. Полный журнал и неуспешный provenance сохранены. |
| Офлайн-повтор, provenance и served proof | PASS | Выход сборки 0, receipt совпал с proof, web отдал тот же build marker из read-only workspace. |
| Go targeted | PASS | Unit tests текущих workflow и Marketing. |
| Полный Go API, SQL queue и worker B–E | NOT RUN | Durable цепочки отсутствуют. |
| Генерация Marketing и persistence после restart | NOT RUN | Генеративный gate блокируется конфигурацией подготовленного стенда. |
| CODE COMPLETE | BLOCKED | B–E, полноценная генерация и E01 root cause остаются открытыми. |
| RUNTIME VERIFIED | PASS | Для containment и provenance. Полный бизнес-runtime Final Integration остаётся непроверенным. |

Chromium 19/19 сохранён как исторический baseline. Повтор полного набора и новые integration tests через worker не выполнялись. Сценарии и миграции 00021–00023 в этой итерации сохранили своё текущее состояние.

## Изменённые файлы

- `scripts/build-env/{production.sh,build-provenance.cjs}`
- `infrastructure/compose.prepared-build.yml`
- `docs/BUILD_ENVIRONMENT_PLAN.md`

## Следующие приоритеты

1. Подключить B–E по одному через scoped lease, уже существующие таблицы задач и SQL-очередь, затем проверить полные цепочки в изолированной PostgreSQL.
2. Подключить утверждённый контекст Marketing и выполнить генеративный gate при разрешённом provider и бюджете.
3. Для E01 продолжить причинную атрибуцию при появлении события, которое связывает фактическую запись с процессом и родителем.

[Индекс доказательств](../.local/stage-2-6-final/INDEX.md)
