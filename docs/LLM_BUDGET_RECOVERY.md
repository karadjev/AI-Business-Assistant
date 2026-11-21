# Восстановление бюджета LLM — 8Б.1

18.10.2026. Provider NOT SELECTED, active mock, **LLM BUDGET0 RUB**, live disabled. Recovery — локальный финансовый учёт, не разрешение вызвать провайдера.

## Инварианты

`llm_reserve` атомарно проверяет platform/organization/dialog/day/month limits и параллельность, резервирует максимальную оценку с учётом разрешённых retries. `Metered.Generate` получает разрешение на HTTP только после commit reservation. Нулевой денежный бюджет/kill switch не позволяет дойти до транспорта; owner не может поднять platform limit, member не управляет денежными лимитами.

Для одной операции:

- pending: сумма находится в reserved; слот active_calls занят.
- settled: достоверный bounded usage, сумма charged перенесена в spent, остаток освобождён, слот закрыт.
- uncertain = **UNKNOWN**: нет достоверного полного usage, сумма **целиком** перенесена из reserved в spent, slot закрыт. Это консервативная оценка, не утверждение точного счёта провайдера.

Для всех transitions: `spent + reserved` не уменьшается за счёт неизвестного исхода; одна операция не списывается/не освобождается дважды. Израсходованная UNKNOWN сумма не превращается обратно в доступный бюджет. При known usage освобождается только разница между верхней оценкой и подтверждённой стоимостью.

## Crash и срок

Миграция9 добавляет private `llm_settle_core` (runtime не может вызвать напрямую), авторизованный совместимый `llm_settle` и `llm_recover_stale`. Lock order везде один: platform budget → reservation → usage buckets. Повтор завершения terminal операции — no-op. Параллельные recover/settle сериализуются, не нарушают финансовую сумму и active_calls.

Если worker умер после reserve либо после provider response до settle, запись остаётся pending. После **5 минут** от server created_at fixed maintenance выбирает до100 старых pending, блокирует ledger и завершает каждую как UNKNOWN по полной сумме. Порог нельзя уменьшить браузером/LLM. Он превышает обычный ограниченный цикл провайдера (env timeout максимум3s, bounded requests/retries). Возврата денег при timeout нет.

`cmd/worker` вызывает `costcontrol.RecoverStale` на каждом tick, в том числе после перезапуска, даже если live выключен: старые обязательства требуют учёта без нового provider request. При recover записывается безопасный лог count; не тексты/ключи. Сбой транзакции откатывает и counters, и audit. Следующий tick продолжит. Если сам процесс suspended дольше срока и позднее принесёт usage, уже закрытый UNKNOWN **не переоценивается автоматически**; максимум уже учтён, repeat settle не даёт refund.

Runtime maintenance function намеренно имеет ограниченные глобальные полномочия: фиксированный возраст, bounded batch, только консервативное завершение. Она не читает пользователю строки, не меняет limits/kill switch, не освобождает деньги и не делает HTTP. PUBLIC EXECUTE отозван. Это доверенный backend maintenance capability, а не пользовательский межтенантный API.

## Аудит и ручная проверка

`platform.llm_budget_events`: reservation ID, organization ID, status, amount, charged, actor_id (NULL у maintenance/channel) и время. Trigger атомарно фиксирует reserve и terminal settlement/recovery. Прямых runtime INSERT/UPDATE/DELETE grants нет, FORCE RLS и tenant FK включены. События не содержат prompt, provider key или клиентский текст. Миграция не придумывает исторические события для операций до её применения.

`GET /api/v1/organizations/{id}/llm-reservations` — staff, последние50 операций с audit.

`POST /api/v1/organizations/{id}/llm-reservations/{resource}/recover`, body `{"confirm_conservative_charge":true}` — только owner, session+CSRF. Активный pending моложе5 минут →409. Старый pending → UNKNOWN/full charge; terminal → idempotent200; foreign/absent404; member403. Никакого ввода «фактической стоимости», нулевого refund или параметра обхода возраста.

UI `/dashboard/ai-settings`: «Восстановление бюджета», pending/UNKNOWN, сумма в microRUB и audit. Кнопка консервативного завершения только owner; backend повторно проверяет права. Эта форма не включает live и не устанавливает бюджет.

При неизвестном provider outcome не переиспользовать reservation, не обнулять counters SQL-командой и не пытаться автоматически повторить запрос. Если провайдер не поддерживает достоверную последующую сверку, UNKNOWN остаётся полной оценкой. Позднейшее уменьшение начисления по документированному счёту требует отдельного audited adjustment workflow, который **не реализован** и не нужен для безопасного консервативного восстановления этого этапа.

## Retention

Synthetic purge удаляет сообщения/связи покупателя, но не финансовую память лимитов. Reservation conversation_id может стать NULL; минимальные financial aggregates/audit сохраняются. Actor_id — идентификатор сотрудника, не покупателя. Это не обещание полной анонимности всего аудита. До реальных данных нужны утверждённые сроки/правовая политика; сейчас только синтетика.

## Проверки

`tests/integration/reliability_test.go` реально завершает дочерние процессы после reserve и после ответа loopback Compatible provider. Новые отдельные процессы конкурентно исполняют recovery. Проверены полное UNKNOWN списание, один terminal event, повторный settle/recover без изменения суммы, запрет преждевременного/чужого/member recovery, private core denied и timeout без refund. Existing stage7 проверяет конкурентный monetary reserve и platform cap; agent audit тест — actual Compatible transport count0 при0 RUB.

Установленный positive simulated ledger существует только внутри уничтожаемой aba_test fixture. Приложение/рабочая БД не получили бюджет, provider key или разрешение внешнего доступа. Реальный счёт LLM/последующая provider сверка: NOT RUN.
