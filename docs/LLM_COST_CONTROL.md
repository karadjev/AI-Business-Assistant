# Контроль расходов LLM

Текущий утверждённый денежный лимит **0 RUB**. Тарифы **0**, provider NOT SELECTED. Любой live reserve при текущей конфигурации отклоняется до обращения к БД и провайдеру. Mock-циклы считаются отдельно и не являются расходами.

Все суммы целочисленные **microRUB**, 1 RUB = 1 000 000 microRUB. Тарифы `LLM_INPUT_MICRORUB_PER_MILLION` / `LLM_OUTPUT_MICRORUB_PER_MILLION` — microRUB за миллион соответствующих токенов. Оценка: `ceil((input * input_rate + output * output_rate) / 1_000_000)`. Float для ledger не используется. Реальный тариф и валютный пересчёт пока не утверждены, стоимость оценочная, а не счёт поставщика.

## Модель и конкурентность

Миграция `00007_live_preparation.sql`:

| Таблица | Область | Назначение |
|---|---|---|
| llm_platform_budget | глобальная, без runtime grants | общая сумма, kill switch, reserved, spent, active calls |
| ai_settings | tenant + FORCE RLS | owner настройки, optimistic version, auto/live/sending flags |
| llm_usage | tenant + FORCE RLS | dialog, UTC day, UTC month buckets |
| llm_reservations | tenant + FORCE RLS | снимок тарифов, caps, модель, reservation и settlement |
| agent_metrics | tenant + FORCE RLS | отдельные mock/live агрегаты |
| ai_settings_events | tenant + FORCE RLS | actor и аудит настроек/удалений |

Reserve блокирует единственную platform row, затем настройки организации и usage windows. Это намеренно простой общий mutex для MVP; межорганизационные конкурентные запросы сериализованы на короткой DB-транзакции. HTTP выполняется после commit. Проверяются platform env+DB caps, parallelism, request/dialog/day/month money и dialog tokens/requests. Прямые runtime записи в денежные таблицы запрещены; доступны узкие SECURITY DEFINER функции с фиксированным search_path, авторизацией и проверкой рассчитанной суммы. Org context transaction-local; worker использует проверенный Telegram lease capability.

Перед каждым HTTP резервируется худшая стоимость input/output caps × максимальное число попыток. При достоверном usage разница возвращается, requests учитываются консервативно; после retries или ошибки/отсутствия usage списывается весь резерв. Повторное settle идемпотентно. При сбое reconcile резерв остаётся занят; после падения процесса pending также остаётся занят и блокирует дальнейшие вызовы. Автоматического возврата неизвестных расходов нет: оператор сверяет их с поставщиком и согласует восстановление. Это ограничение доступности, предотвращающее скрытый перерасход.

День/месяц — UTC; settle использует окна, зафиксированные при reserve, включая переход полуночи. Общий platform budget — суммарный лимит, не сбрасывается ежемесячно. Org settings cannot exceed approved platform cap. Включение auto не возвращает ранее переданный сотруднику диалог агенту автоматически.

## Технические лимиты

- Диалог: 20 000 input, 5 000 output, 30 provider attempts.
- До 3 tool calls за цикл и до 2 итераций провайдера.
- Parallel=2 по умолчанию, конфигурируемо 1–16.
- Input cap=16 000, output cap=512 на запрос; сериализованный размер ограничен консервативно байтами, а не обещанием точной токенизации.
- Timeout 2 s по умолчанию, 1 ms–3 s; подходит текущему lease fencing, для медленной модели потребуется пересмотреть leases до расширения.
- Денежные request/dialog/day/month/platform defaults=0.

`PATCH /api/v1/organizations/{id}/ai-settings` доступен только owner test organization с CSRF и expected_version. Он не утверждает platform budget и не выбирает provider. Runtime не может менять platform approval; после отдельного разрешения оператор миграционной роли может установить согласованную сумму/kill_switch, сохранив spent/reserved. Такой SQL сейчас не выполнялся.

Нативные tests доказали нулевой бюджет, fixed-point расчёт и отсутствие вызова провайдера. Настоящие PostgreSQL concurrency/RLS/settlement tests выполнены: PASS. Проверены диалоговый и общий межорганизационный лимиты, повторное settle, conservative unknown usage и изоляция. Это подготовка; live финансовое E2E и сверка с поставщиком NOT RUN.

## Уточнения аудита 8А

Неполный/null usage не считается нулевой известной стоимостью. Compatible требует достоверные input/output поля; иначе ledger консервативно учитывает резерв. Тест TestAuditZeroBudgetStopsActualAdapterTransport доказывает отсутствие вызова реального адаптерного транспорта при0 RUB, даже если другие тестовые gates разрешены. После падения процесса до settle pending/active_calls могут сохраниться: автоматическая сверка A10 не реализована; нельзя без доказательств освобождать неизвестную стоимость. Реальный LLM по-прежнему запрещён.

## Актуализация8Б.1

A10 FIXED: миграция9, private settlement core, fixed5min conservative recovery старых pending, immutable financial events и owner-only manual endpoint. UNKNOWN расходится по полной сумме и не возвращается в доступный бюджет; после crash recovery идемпотентно. Подробный lifecycle, ограничения и доказательства: [LLM_BUDGET_RECOVERY.md](LLM_BUDGET_RECOVERY.md). Live/mock defaults и0 RUB сохранены; прежний раздел8А описывает исторический пробел.
