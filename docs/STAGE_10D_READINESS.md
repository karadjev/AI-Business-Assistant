# Этап 10Г — Demo Experience & Sales Journey

Будущий отдельный этап. В10В демонстрационные функции не реализуются. Этот документ не разрешает live, платные API, CI/deploy, изменение глобальных настроек или рабочих данных.

## Объём перенесённого задания

Сначала сверить STAGE_10C_REPORT.md и текущие gates/E01. Изучить существующие synthetic seed/browserfixture/mock adapters; расширять только существующие каталог, identity, channels, agent/orders/outbox. Сохранить согласованный визуальный стиль.

Создать локальный повторяемый изолированный demo store: реалистичная synthetic электроника, категории/характеристики/цены RUB, доступные/отсутствующие товары, устаревшие остатки, проблемы каталога, тестовые диалоги и заявки разных статусов. Явная маркировка симуляции; не копировать рабочие tenant/PII.

Путь5–10 минут: знакомство → подключение магазина → каталог/health → synthetic покупатель TG/WA → grounded mock AI/Inventory Safety → явный consent → заявка → staff confirmation до reserve / handoff → synthetic owner summary. Telegram и WhatsApp равноправны на каждом шаге. Повтор сообщений не создаёт повторную заявку; DELIVERY_UNKNOWN не пересылается автоматически.

Sales journey объясняет целевого владельца, оба канала, качество каталога, контроль заявок, prerequisites и порядок пилота. Не придумывать продажи/конверсию/экономию и не выдавать pricing hypotheses за коммерческие условия. Все demo metrics помечать synthetic.

## Приёмка и проверки

- Owner/staff/tenant scope, FORCE RLS, freshness/source ownership, consent/idempotency, injection protection, financial gates неизменны; demo не обходит auth.
- Desktop и390×844: навигация, keyboard/focus, empty/error/loading, карточки/таблицы, отсутствие page horizontal overflow.
- Последовательные offline Go/tests/race/vet/build; isolated PostgreSQL/migrations/RLS; frontend lint/typecheck/fresh production; browser E2E обоих mock channels; строгая build integrity.
- Никаких live Telegram/Meta/1С/LLM вызовов, budgets0 RUB. Новые зависимости/скачивания отдельно согласовываются.
- Отдельный отчёт docs/STAGE_10D_REPORT.md: implementation, changed files/migrations, точные PASS/FAIL/BLOCKED/NOT RUN, UX/screenshots только synthetic, security/channel parity, E01 evidence, external calls/costs и следующие blockers.

## Открытые ограничения

E01 остаётся OPEN, Security CONDITIONAL до causal proof и проверенного исправления. Не удалять неожиданные build paths для сокрытия проблемы и не переписывать proof для принятия drift. Audit/receipts proposal10В требует отдельного решения;10Г не разрешает их удаление. Source history recent100 и snapshot/export большого периода — отдельный scope. Live readiness blockers из9В сохраняются.


## Проверенный вход после10В

10В реализован по первоначальному readiness. Go/tests/race/vet/build, PostgreSQL integration/race и45-table FORCE RLS, frontend lint/typecheck, fresh Next16.4.0 production, current-production Browser E2E3/0 skipped/flaky/unexpected и final integrity/served PASS. Подтверждены6001 variants/236 receipts в SQL sweep и6008 variants/237 imports в browser UI; owner capability rebind и независимый retention key реализованы. Подробности/границы и evidence: STAGE_10C_REPORT.md.

Offline10Г может начинаться отдельным заданием при сохранении строгих guards. E01 OPEN и Security CONDITIONAL не переименовываются в PASS; live готовность отсутствует. Audit/receipts proposal не утверждён и не является разрешением удаления. Схема15 и согласованная конфигурация retention key API/worker — обязательные prerequisites для окружения с новым backend; рабочий rollout10В не выполнялся.
