> Этап10Б: добавлены FAILED/EXPIRED history, filename/applied_at, hold и безопасная [Retention Policy](IMPORT_RETENTION_POLICY.md). Подтверждённый EXCEL snapshot теперь сверяет и неизменённый/нулевой EXCEL stock. Raw XLSX не сохраняется.

# Excel catalogue import — 10А

Используется единый catalog.Service. Новые XLSX библиотеки/зависимости не устанавливались: archive/zip, encoding/xml/json из Go standard library. Импорт синхронный, отдельной очереди/worker нет.

Workflow: raw binary upload → durable job → owner mapping → нормализация/validation → durable preview → отдельный confirm → одна SQL transaction. Upload/mapping/preview не меняют товары, категории или остатки. История в мастере позволяет восстановить preview/result после reload/потери ответа. Cancel удаляет сохранённый grid и не меняет каталог. Applied тоже очищает grid, сохраняя preview/report/audit.

API под `O=/api/v1/organizations/{id}`:

| Метод | Путь | Действие |
|---|---|---|
| POST | O/catalog-imports | Raw .xlsx bytes; X-Idempotency-Key UUID |
| GET | O/catalog-imports/{resource} | Job, headers, mapping, preview/result, version |
| POST | O/catalog-imports/{resource}/mapping | mapping field→zero-based column index + expected_version |
| POST | O/catalog-imports/{resource}/confirm | expected_version + confirm=true |
| POST | O/catalog-imports/{resource}/cancel | expected_version |

Recent100 IDs/statuses доступны через owner GET onboarding. Все пути защищены прежними session/CSRF/tenant BFF. Только upload имеет body limit1MiB; остальные JSON endpoints сохраняют64KiB. MIME клиента игнорируется при определении формата; multipart не используется. Реальный OOXML workbook проверяется backend.

Обязательные mapping поля: sku/name/price/quantity. Необязательные: category/brand/model/description/currency/availability/attributes. Одна колонка не может быть назначена двум полям. Mapping неизвестного поля, выход за границы, отсутствующие обязательные поля отвергаются. Заголовки не дают прав на импорт tenant ID/source/secret/channel/budget полей.

SKU нормализуется по существующему правилу trim+uppercase ASCII1–64. Совпадение только по `(organization_id,sku)`, не по названию. Price — десятичные RUB, максимум2 дробных знака, без float; хранение — целые копейки0…9000000000000. RUB — единственная поддерживаемая валюта; при отсутствии currency mapping применяется прежний default RUB. Quantity — целое0…1000000000, не ниже действующего резерва. Availability, если задано, должно соответствовать quantity>0. Model хранится в attributes.model, attributes — объект строковых значений, прежние32 keys/4096bytes/ограничения длин сохранены. Existing catalog validators повторно используются.

Новые строки создают draft product + default variant + inventory; импорт не публикует автоматически. Active SKU с нулевой ценой и archived SKU блокируются. Не найденные в файле товары не удаляются. Неявной передачи источника MANUAL/ONE_C→EXCEL нет. Unchanged строки не получают новые товары/варианты; repeated file с новым upload key строит новый preview относительно текущего каталога. Тот же upload key и digest возвращают прежний job; другой digest под тем же key конфликтует.

Preview показывает totals/new/updated/unchanged/errors/conflicts/duplicates, каждую строку, причину, новые поля, цену и остаток до/после. Дубликат SKU отмечается у всех повторных строк. Любая ошибка/конфликт/дубликат блокирует **весь** batch; скрытого partial apply нет. Изменение mapping выключает apply до повторного preview.

Concurrency: existing manual catalog mutations и imports берут tenant advisory transaction lock10130. Preview/confirm блокируют products/inventory в стабильном порядке; это совместимо с существующим order product→inventory locking. Snapshot включает products/variants/inventory/categories. Confirm заново сравнивает его под locks; изменения цены, резерва, категории или параллельное manual creation требуют нового preview. Jobs имеют optimistic version. Два confirm одного preview имеют один effect; конкурентный другой import со старым snapshot получает409. Applied job с matching original confirmation version возвращает сохранённый result. При crash/connection loss до commit PostgreSQL откатывает весь batch; после commit job=APPLIED позволяет сверку/безопасный replay. Exact-once HTTP response не обещается.

Security bounds: upload1MiB; archive64 entries; total uncompressed8MiB, each part4MiB, ratio≤100; one visible worksheet;500 data rows+header,24 columns; cell16KiB; **суммарный expanded text1MiB** (включая повторные sharedStrings); persisted grid≤2MiB, preview≤4MiB. Есть отдельный shared-string expansion regression. XML/ZIP не извлекается в filesystem. Reject malformed/CRC/path traversal/symlink/encrypted/non-XML parts, macro-enabled types/VBA/ActiveX/embeddings/external links, external relationships, DTD/entities, formula cells/cached formula results, merged/hidden structures. Ни формулы, ни ссылки, ни macros не выполняются. Image-rich/multisheet workbooks не входят в supported tabular format. Пропуски строк сохраняются как пустые строки для validation; повреждённые/неполные headers отвергаются.

До20 незавершённых jobs и100 total jobs на tenant; expiry24h запрещает stale apply. Это fail-closed storage bound, не обещание автоматического удаления просроченных jobs: controlled retention/cleanup — следующий этап. Raw file на диск не сохраняется. Audit хранит IDs/actor/kind, без полного тела файла, контактов или секретов. Owner-only grid временно содержит загруженные ячейки; загрузка данных клиентов не использовалась в тестах.

Синтетический шаблон: `apps/web/public/fixtures/synthetic-electronics.xlsx`, идентичная testdata в API. Generator `scripts/create-demo-catalog.py` использует только Python standard library и детерминированный ZIP. iPhone/Samsung/Xiaomi/наушники/чехлы/зарядка — все имена/описания и цены/остатки явно SYNTHETIC.
