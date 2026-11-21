# Этап 10Г — Demo Experience & Sales Journey

Дата проверки: 30 октября 2026. Объём: STAGE_10D_READINESS.md и принятое задание. Security CONDITIONAL; E01 OPEN. Только offline synthetic/mock. Рабочий rollout не выполнялся.

## Реализация

Добавлен защищённый маршрут `/dashboard/demo?org=…`: девять шагов от знакомства и настройки магазина до фактического результата диалога. Telegram и WhatsApp используют один интерфейс и равноправный сценарий. Сохранены существующие стиль, меню, BFF и API. Landing объясняет оба канала и предлагает demo; нет выдуманных продаж, конверсии, экономии или коммерческих тарифов.

Шаги: знакомство → test store/mock channel → каталог и health → synthetic покупатель → фактический mock AI ответ → отдельное согласие → заявка без резерва → подтверждение сотрудником/резерв и handoff → фактическая сводка. Действия используют прежние ingress/callback/orders/integration endpoints, agent, repositories и queues. Ответы читаются из реальных mock deliveries; успешные ответы API не подменяются. Новый привилегированный HTTP seed не добавлен.

UI показывает loading/error/empty, время последнего чтения и фактические состояния. Ошибка snapshot не превращается в утверждение об отсутствии диалога. Чтения соблюдают Retry-After; POST автоматически не повторяются. Повтор события отправляет тот же идентификатор для проверки idempotency. URL сохраняет шаг/канал; заголовок получает keyboard focus. Длинный технический журнал раскрывается отдельно. Макет проверяется на desktop и390×844.

## Демонстрационные данные

CLI `/artifacts/browserfixture demo UUID` требует изолированную aba_test с safety marker. Детерминированные IDs, session advisory lock и completion audit receipt обеспечивают повторяемость и безопасное продолжение незавершённого seed. Повтор не сбрасывает остатки, заявки, каналы или историю.

Создаются synthetic owner/staff/read-only, test tenant, категории и7 товаров электроники с RUB ценами/характеристиками: свежие доступные, отсутствующие, устаревшие и неизвестной свежести, неполная карточка. Учебные заявки pending/rejected/confirmed/completed создаются через действующий orders service, а не SQL-подстановкой статусов. Они относятся к DEMO-CHARGER и отделены от результата текущего диалога DEMO-S25. Контакты synthetic@example.test; credentials не публикуются в отчёте. Примеры не являются реальными клиентами или рыночными котировками.

Инструкция подготовки и сценарий: [DEMO_WALKTHROUGH.md](DEMO_WALKTHROUGH.md). Новый optional Compose overlay публикует только loopback-порт отдельного demo project и согласованный PUBLIC_ORIGIN. Базовые конфигурации окружения не изменены. Между операторскими показами сохраняется volume; новый UUID создаёт новый показ без скрытого удаления старой истории.

## Изменённые файлы и миграции

Изменены:

- `apps/api/tests/browserfixture/main.go` — CLI dispatch.
- `apps/web/src/app/page.tsx`, `dashboard/page.tsx`, `dashboard/integrations/telegram/page.tsx` — demo entry и точные mock/channel тексты.
- `apps/web/src/app/globals.css`, `components/Header.tsx`, `components/Sidebar.tsx` — согласованный responsive UI и навигация.
- `apps/web/src/lib/workflows.ts` — существующий backend source whatsapp добавлен в TS union.

Добавлены:

- `apps/api/tests/browserfixture/demo.go`, `demo_test.go`.
- `apps/web/src/app/dashboard/demo/page.tsx`, `loading.tsx`.
- `apps/web/src/components/DemoJourney.tsx`.
- `apps/web/tests/demo-production.spec.ts`.
- `infrastructure/compose.demo-offline.yml`.
- `docs/DEMO_WALKTHROUGH.md`, этот отчёт, `docs/STAGE_10E_READINESS.md`.

Миграций и зависимостей нет; схема15 сохранена. Production Go services/domain не переписаны. `costcontrol/recovery.go`, существующие migrations, module/lock manifests и artifact guards защищены сравнением SHA256. AGENTS.md, Superpowers, CI, PATH, глобальные настройки не изменялись.

## Проверки и evidence

Полные локальные логи, exit codes и команды: `.local/audit10d/`. Это отдельные QA projects с одноразовыми synthetic volumes; рабочая БД не используется. Проверки запускаются последовательно через существующие cached runners, без downloads.

| Проверка | Результат |
|---|---|
| Go format/tests/vet/build | PASS — `go-final` |
| Go race | PASS — `race-final` |
| PostgreSQL integration/race/RLS | PASS — `integration`,81.249s;45 FORCE RLS tables, tenant/role/concurrency gates |
| Fixture unit/CLI, повтор и конкурентный seed | PASS; повтор после резерва не сбрасывает состояние |
| Next lint, TypeScript, contracts/native/artifact unit | PASS — `web-check-release` |
| Fresh Next16.4.0 production build | PASS — `web-build-release` |
| Final production Browser E2E | PASS — `browser-final`,5 expected/0 unexpected/0 flaky/0 skipped,24.1min |
| Final artifact/served integrity | PASS — `integrity-final`: current artifact и current artifact served by local web |
| Изолированный loopback smoke | FAIL readiness дважды; ручной localhost показ BLOCKED, см. ниже |
| Реальное human usability study | NOT RUN |
| Live/provider/paid проверки | NOT RUN — вне разрешённого scope |

Три subprocess helper SKIP в Go integration являются дочерними точками входа; соответствующие родительские проверки выполняются. Масштабирование не менялось: существующий SQL integration sweep сохраняет проверку6001 variants; результаты6008 variants/237 imports предыдущего10В — исторические, не выдаются за новый browser load test10Г.

Новые E2E проверяют оба канала: реальный ответ по свежему товару, stale/out-of-stock и injection, отсутствие заявки/резерва до согласия, повтор входящего события/consent без дубликатов, staff session confirmation, handoff без последующего автоответа, foreign tenant404/read-only403, reload, keyboard/focus, все9 шагов390×844 без горизонтального page overflow. Для проверки retry искусственно возвращается один отрицательный GET429; успешные business данные и mutations остаются фактическими. На mobile обязательны загруженные данные и отсутствие app alerts.

## Длительность и UX

План рассказчика6:15:45/45/60/30/45/45/30/45/30 секунд. E2E presentation mode отдельно сохраняет core elapsed,375s narration и interaction, исключая seed/login; критерий300–600s проверяется assertion. Это измерение автоматизированного показа с заданными паузами, не время реального клиента. Итог: Telegram430.381s (7:10.381), из них narration375s и interaction55.381s; WhatsApp431.231s (7:11.231), из них narration375s и interaction56.231s. Дополнительные проверки после core walkthrough заняли время сверх него; verification_total437.682/438.406s. JSON attachments и screenshots сохранены в `.local/audit8d/browser-results/stage10d-final*`, сводка `.local/audit10d/final-timings.json`. Human rehearsal остаётся отдельной приёмкой.

## Безопасность

Demo разрешён только test tenant и пользователю с правами управления заявками; каждое действие сохраняет server authorization, tenant scope и CSRF/Origin. Owner-only catalog health не расширяется для staff. FORCE RLS, freshness/source ownership, consent, version fence, financial admission, provider/template/window/signature gates и DELIVERY_UNKNOWN policy не ослаблены. Auto resend неизвестной доставки не добавлен. Mock mode проверяется перед действиями; Telegram/WhatsApp/1С/LLM live не включались, budgets0 RUB.

Результат текущего диалога отделён от начальных учебных заявок. При достижении первой страницы20 orders UI не выдаёт усечённый список за полный итог; inventory view ограничен100 записями для небольшого demo. Эти пределы не являются новым общим каталогом/историей и не отменяют пагинацию существующих экранов.

## Ошибки промежуточных прогонов и E01

Сохранены полные FAIL logs: начальный lint выявил refs-in-render/Date.now, исправлено без исключений; промежуточный E2E — неоднозначный role=alert из-за Next route announcer, selector ограничен main. Ранний debug E2E технически PASS, но просмотр screenshot выявил429 и пустые mobile данные: добавлены backoff и строгая проверка реальных данных, старый результат не принят как UX acceptance.

`integrity-before-rebuild` exit42 произошёл после согласованного изменения source до fresh rebuild; mismatch ожидаем и не переименован в PASS. Старый proof не переписан для принятия drift. После изменений выполнена fresh production build. Release proof: BUILD_ID `MLNNjKQo6bmpPspBrTmCO`, source SHA256 `02dc4cba382fd9d7039f286e89a7e770d85523c6c8331fa005b4468408b70fe9`.

Observer следит за неожиданными `.next 2`/`node_modules 2`; причинного доказательства устранения E01 нет. E01 остаётся OPEN независимо от успешных проверок этого этапа. Security остаётся CONDITIONAL.

## Внешние действия, audit/receipts и границы

В рамках10Г выполнялись только локальные процессы, Docker internal/loopback HTTP и подготовленные offline mock adapters. Запросов к Context7, Telegram/Meta/LLM/1С, платных API и установок не выполнялось. OAuth-пилот не продолжался. Фактические расходы API0 RUB. Это описание выполненных команд/изолированного окружения, не утверждение о packet capture всей машины.

Сроки хранения audit/receipts не установлены, удаление не разрешено; предложение10В остаётся на отдельном согласовании. Commit/push/deploy не выполнялись. Неизвестная первопричина E01, отсутствие human rehearsal и live readiness остаются открытыми. Следующий этап описан отдельно в STAGE_10E_READINESS.md.

Audio: отдельная ранее заявленная блокировка не объявлена устранённой; в10Г не выполнялись audio remediation или принудительное завершение процессов.

## Финальный результат и локальная блокировка

Все5 production E2E PASS: Catalog Control Center/large history, Telegram demo, WhatsApp demo, три магазина/multichannel regression, onboarding/import replay. Skipped/flaky/unexpected0. Screenshots текущей сборки проверены; mobile содержит реальные loaded counters, без app error. `integrity-final` подтвердил совпадение current artifact и served local web.

Дополнительный host-loopback smoke FAIL дважды до login/seed: Connection refused после30/60 readiness попыток. Next внутри контейнера сообщает Ready; resolved Compose содержит правильный127.0.0.1 published port, но `docker compose port web 3000` возвращает `:0`. Docker network остаётся internal. Это локализация проблемы публикации, не доказательство её первопричины. Credentials/CSRF/Origin сценарий этого smoke **NOT RUN**, не PASS. Файлы `localhost-smoke*.log/.exit` и `localhost-runtime.log` сохранены. Optional overlay синтаксически принят Compose, но ручной host доступ в данном Docker окружении **BLOCKED**; не считать его рабочим runbook без устранения блокировки. Сеть с внешним доступом не добавлялась. Автоматический browser внутри изолированной сети успешно работает с обоими каналами; это не подтверждает host-loopback.

Функциональность10Г и offline regression реализованы, но полная ручная demo readiness требует отдельного решения localhost publication blocker и human rehearsal. Следующий этап не разрешает автоматически ослаблять network isolation.

Финальная проверка после cleanup: `integrity-after-cleanup` PASS, release proof byte-identical. Observer08:23:14–09:23:45 UTC не зафиксировал подозрительных путей; остановлен marker без force kill. `final-cleanup.json` подтверждает отсутствие containers/networks/volumes четырёх собственных QA projects (db/browser/local/build). Рабочие проекты не обслуживались. 23 защищённых файла повторно совпали с baseline; итоговый manifest `.local/audit10d/changed-files.json` содержит18 файлов:8 modified/10 added, deletions0. Полные intermediate failure evidence сохранены.

## Дополнение: ограниченный этап localhost

Последующая авторизованная диагностика устранила технический blocker через loopback-only Docker-stdio relay, без расширения сетей. Host login/demo/auth/CSRF/Origin и integrity/served PASS. Исходные FAIL выше сохранены как история10Г. Подробности и пределы: [DEMO_LOCALHOST_REPORT.md](DEMO_LOCALHOST_REPORT.md), актуальный запуск: [DEMO_WALKTHROUGH.md](DEMO_WALKTHROUGH.md). HUMAN REHEARSAL NOT RUN, E01 OPEN, Security CONDITIONAL.
