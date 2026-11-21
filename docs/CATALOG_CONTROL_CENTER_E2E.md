# Catalog Control Center E2E · 10Б

Strict offline: prepared cached Linux dependencies/images, internal Docker networks, pull never, no host ports/public webhook/real provider. Все магазины/пользователи/цены/остатки — SYNTHETIC. Browser выполняет production standalone, не dev server. Перед запуском проверяются source/artifact proof и served build marker.

Сценарии в `catalog-control-production.spec.ts`, `multichannel-production.spec.ts`, `onboarding-production.spec.ts`:

| Acceptance | Проверяемое действие |
|---|---|
| Overview | Backend products=1 до импорта,7 после; отсутствие mutation до confirmation |
| Health Score | Реальное значение0..100, карточка/формула; не fixture-number в UI |
| Freshness | Fixture переводит физическую дату в STALE/UNKNOWN, backend считает review; фильтр unknown/empty state |
| Excel | Upload→mapping→preview→explicit confirm→реальные показатели; repeat no duplicates в onboarding regression |
| Source conflict | Прямая manual PATCH цены EXCEL получает409; сравнение/оставить текущее |
| Source switching | PREVIEW, disabled confirmation до checkbox, APPLIED/history; backend повтор/parallel/stale fencing в PG |
| Import history | filename/row summary/details/audit, APPLIED и FAILED |
| Retention | Hold защищает APPLIED; отдельный FAILED job состарен helper, реальный worker минимизирует его без канала, receipt остаётся |
| Telegram stale answer | Реальные inbox/worker/shared agent/outbox mock; текст «наличие требует подтверждения»; owner adjustment тем же stock подтверждает сверку |
| WhatsApp stale answer | Тот же путь/правила; HMAC/financial/template/window/unknown regression остаются |
| Tenant isolation | Foreign API404, staff403 для owner control;3shops×2channels проверяют отдельные цены/заявки/резервы |
| Mobile |390×844, no document horizontal overflow, full-page screenshot; таблица scroll внутри панели |

Демонстрационный Excel расширен четырьмя категориями: смартфоны, наушники, аксессуары, зарядные устройства. Test prices/stock явно помечены. SQL helper ограничен isolated postgres/aba_test, is_test и Synthetic8D prefix; не является runtime endpoint. Невалидный workbook создаётся через настоящий API, не выдаётся за provider данные.

Известные development attempts сохранены отдельно: incorrect exact-label/exact-cell selectors; строгий backend выявил лишнее поле version формы (исправлен serializer, backend не ослаблен); повтор статического mock bot ID столкнулся с прежним synthetic integration (убрана ненужная зависимость cleanup-теста от канала). Rate-limited SSR и poll5500ms при default timeout5000ms потребовали корректного pacing/timeout теста. API rate limits не изменялись. E01 один запуск остановил42 до начала browser tests, что не PASS.

Окончательные результаты и build identity фиксируются в STAGE_10B_REPORT. Только завершённый успешный прогон считается PASS; trace/screenshots/failed attempts сохраняются в ignored .local evidence, без публикации session cookies.

Финальный результат: **PASS всех12 строк acceptance**,3 завершённых current-production tests,0 skipped/unexpected/flaky. Build `Hf5QQTkeHMFwonl2YwQ3G`, source `4d57a84261fd6222df69f663e66bf357f6bb028ed64b365e38d818c4c2264e8f`; start `2026-10-28T15:31:06.747Z`, duration 557.675s. Последние screenshots: `.local/audit8d/browser-results/stage10b-display-final/`. Mobile logout/контраст проверены визуально на этой сборке. E01 после browser вновь пойман guard42; после точечного удаления двух captured empty paths original proof/served marker снова PASS, finding остаётся OPEN.
