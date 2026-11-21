# Центр управления каталогом · 10Б

`/dashboard/catalog?org=<existing tenant>`: owner выбирает свой магазин, видит состояние ассортимента и безопасные следующие действия. Прежние products/categories/inventory/onboarding остаются тем же каталогом и API. Раздел добавлен в Sidebar, используется existing dashboard typography/colors/tokens, CSS без новых зависимостей.

Backend GET catalog-control возвращает snapshot time, store name, nonarchived variant rows, metrics, health, policy, страницы import receipts по100 и последние100 source operations. Scope owner-only; staff пользуется существующими catalog/order правами. Агрегаты считаются по всему каталогу без прежнего порога5000; variant rows в новом постраничном режиме выдаются страницами по50. Совместимый запрос без query сохраняет прежний лимит5000 и CATALOG_LIMIT; интерфейс явно запрашивает page=1. Products — уникальные неархивные карточки, active — активные из них; available — активные карточки хотя бы с одним FRESH available>0; review — карточки с хотя бы одним не-FRESH вариантом. Low stock — FRESH варианты available<=настройки, счётчик уникальных карточек. Source/freshness distribution — по вариантам, подписи это обозначают. Last stock check и successful import — разные timestamps; NULL отображается «Нет данных».

Health рассчитывается catalog.Service (см. CATALOG_HEALTH_SCORE). Action Center использует backend counts: stale/unknown, zero-price, incomplete metadata, last-preview conflicts/errors. Кнопки переключают реальный фильтр или историю; ONE_C явно disabled. Product filters/search/pagination50 работают по полному bounded backend result, без выдуманных метрик. Цвет дополняется текстом и dashed status border. Таблицы scroll внутри панели, не растягивают mobile viewport; skeleton route, busy/disabled, alert/status, доступные labels/keyboard focus, source comparison/confirmation.

История показывает filename/actor/дату/status/row summary/errors; details показывают изменения цен/stock и immutable catalog_events. Hold и настройка retention имеют backend versions; очистка не удаляет receipts. Source history показывает прежние группы/версии, target/actor/reason/operation/time. Передача не меняет значения.

Backend через существующий session/CSRF/tenant-scoped BFF:

- GET /api/v1/organizations/{id}/catalog-control
- PUT …/catalog-control/policy
- POST …/catalog-control/source
- POST …/catalog-imports/{resource}/hold
- Прежние upload/mapping/confirm/cancel/result APIs сохранены; result дополнен безопасными audit/retention metadata.

Новые tenant tables catalog_policies/catalog_source_changes, миграция00014; всего45 FORCE RLS. Нет нового tenant/user/catalog/provider, public mutation, зависимости, реальной1С или live кнопки.


## Изменения 10В

Порог5000 вариантов для центра снят: SQL считает metrics/health по всему tenant catalog, HTTP с явным page передаёт до50 строк на страницу. `page` и `import_page` — 1-based, максимум10000; поиск `q` (название/SKU, literal substring) и фильтр `filter` выполняются на сервере. UI применяет поиск/фильтры явной кнопкой. Metadata rows_total/page/page_size и imports_total/import_page/import_page_size показывает полноту страниц. Нет усечённой оценки. Индекс import history из00013 переиспользуется; миграция00015 сохраняет FORCE RLS и семантику tenant read, вычисляя membership один раз на statement. В channel read policies добавлена только эквивалентная ранняя проверка отсутствующего capability; lease/scope/integration predicate при capability сохранён. Общая функция freshness вынесена в inventory_freshness_at; прежний facade для customer/reserve/dispatch сохранён. Source history recent100 не расширялась этим этапом.

История импортов доступна дальше recent100; каждая страница содержит до100 receipts, не raw XLSX. При конкурентных вставках offset pagination не является frozen export: записи могут сдвинуться между страницами, требуется refresh. Один HTTP ответ не является долгоживущим снимком для последующего подтверждения: mutations по-прежнему проверяют expected_version.

Owner workflow capability rotation: RETENTION_CAPABILITY_ROTATION.md. Proposal audit/receipts отдельно: AUDIT_RECEIPTS_RETENTION_PROPOSAL.md; ни один новый срок удаления не установлен.
