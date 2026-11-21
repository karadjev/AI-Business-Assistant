# Universal Catalog Design

Статус: **PROPOSED / NOT IMPLEMENTED**. Основа — действующие products/variants/categories/inventory, не каталог на каждую отрасль. Prerequisite: G0 из [roadmap](STAGE_3_IMPLEMENTATION_ROADMAP.md).

## 1. Stable core и ограничения baseline

| Business concept | Current canonical field | Решение |
| --- | --- | --- |
| ID/name/description/category/brand | `products`, `product_categories` | ID сохраняются; отрасль не меняет product identity |
| SKU/variant/price/currency | `product_variants` | SKU уникален внутри tenant; price находится на sellable variant |
| Custom attributes | `product_variants.attributes` JSONB; Go `map[string]string` | Legacy map сохраняется, typed v2 не меняет тип v1 property |
| Stock/reservations | `inventory`, `inventory_movements` | Единственный источник stock, existing source/freshness/locking gates |
| Images | Нет dedicated product media entity в текущей схеме | Tenant media metadata + product links, не произвольная URL fetch |
| Availability | Customer facade вычисляет available/null по freshness | Не создавать вручную редактируемый дубликат availability |

Сегодня RUB-only, single-warehouse integer inventory, cart 1–10 unique variants и quantities 1–10; search — name/brand/SKU ILIKE. Эти ограничения не исчезают от добавления профиля.

## 2. Typed hybrid model

Предпочтение: общие relational поля + versioned tenant attribute definitions + bounded JSONB typed values. Полный EAV для всех свойств не выбран: сложнее constraints/чтение; исключительно свободный JSON не выбран: нет надёжных типов и range search.

Proposed entities:

- `catalog_attribute_definitions(org,id,key,scope,type,unit,enum_values,min,max,required,schema_revision,visibility)`;
- `product_attribute_sets(org,product_id,schema_revision,values,source_revision,validation_state,version)`;
- `variant_attribute_sets(org,variant_id,schema_revision,values,source_revision,validation_state,version)`.

Две value tables обеспечивают обычные composite FKs вместо незащищённого `(entity_type, entity_id)`. Это разделение уровней product/variant, а не отраслевые таблицы. Definitions создаются из approved profile и/или owner custom draft. Required fields проверяются перед v2 activation, не при каждом сохранении draft. Уже active legacy товар не архивируется автоматически: typed projection отмечается legacy_unvalidated и не подтверждает отсутствующие schema facts. Source owner/revision и verification timestamp задаёт backend; sidecar не обходит текущие MANUAL/EXCEL/ONE_C ownership fences.

Типы: bounded string, integer, decimal string, boolean, enum, date/year, bounded list/object. Decimal без binary float; unit только из контролируемого dictionary. Reserved keys `price/stock/organization_id/permissions` не подменяют core fields. Product defaults наследуются variant; конфликт или override restricted field требует validation, не выбора LLM.

Начальный лимит — 32 keys/4 KiB на set, как консервативная база текущего API. Изменение лимитов — отдельная нагрузочная проверка; большой fitment matrix не запихивается в одно JSON поле.

Пример значения:

```json
{"schema_revision": 2, "values": {"oem": ["SYNTH-123"], "weight": {"value": "12.500", "unit": "kg"}, "warranty_months": 12}}
```

Это иллюстрация типов, не payload для нынешнего v1 endpoint.

## 3. Начальные отраслевые schemas

| Профиль | Поля и типы | Safety rule |
| --- | --- | --- |
| Автозапчасти | OEM list, manufacturer string, make/model enum/reference, year_from/year_to integer, bounded fitment assertions | OEM equality не доказывает совместимость; требуется verified applicability/source |
| Стройматериалы | Weight/volume/length/width decimal+unit, selling unit, package quantity | Package и физическая мера различаются; нельзя продавать 0.5 через integer ledger |
| Электроника | Brand/model, memory integer+unit, color enum, warranty_months integer | Не придумывать гарантию/характеристики по похожему SKU |
| Одежда | Size value+size-system, color, material composition, season list | Размер EU нельзя автоматически считать RU; неизвестный состав остаётся unknown |
| Мебель | Dimensions+unit, material, color, assembly option, made_to_order flag | Срок изготовления не равен stock availability |

Дополнительные validators: year_from ≤ year_to; dimensions и package quantity положительны; warranty_months неотрицателен; enum codes имеют versioned dictionary; material composition проверяется на допустимый диапазон, а полнота состава не угадывается. List/object depth и размер bounded. Conversion требует явного unit/source, несовместимые единицы отклоняются.

Large compatibility graphs позднее могут потребовать generic typed catalog relations с composite FKs и evidence, но не `auto_parts_products` как второй master catalog. Предложение такой relation требует отдельного объёма и индексов.

## 4. Search, availability, prices

Search v2 принимает только schema-declared filters с finite operators (eq/in/range), типизированными значениями и bounded pagination. Backend строит разрешённый SQL; profile не содержит SQL/JS. Product search остаётся catalog facade, услуги и listings имеют свои query handlers и tagged discovery projection.

Индексы выбирать по подтверждённым запросам: tenant/category/status btree, отдельные numeric projections для популярных range fields; общий GIN не добавлять всем JSONB без plan/load evidence. Raw/derived search projections имеют tenant key, RLS и invalidation по schema/value revision.

`availability`: available/out_of_stock/unknown/not_applicable вычисляется доменом; stale или missing stock → unknown. При reservation используются прежние freshness и lock/recheck, не search cache.

Wholesale — tenant price lists/tier rules/customer eligibility, а не discount в attributes. Price resolver выдаёт amount_minor+currency+source/revision/validity; выбранная модель не даёт клиенту доступ к закрытому прайсу. Первый increment остаётся RUB и whole-unit продажей упаковок. Fractional stock и иные currencies требуют отдельных end-to-end migrations ledger/quote/order/UI; metadata currency в профиле сама по себе их не включает.

## 5. Media и security

`business_media` — tenant-owned metadata: storage key, checksum, MIME/size, processing/publication state, actor/source. `product_media_links(org,product_id,media_id,sort_order,alt_text)` с composite FKs; при необходимости variant links отдельно. Storage URL/key — не доказательство ACL. Signed URL выдаётся только после tenant/publication check, tenant при этом не выбирается по URL.

Не загружать URL из webhook/catalog произвольным HTTP клиентом. Image upload/decoder/object storage выбираются отдельно с разрешением на новые зависимости. Не читать private service/customer attachments через public product media API.

Owner/source-controlled writes; customer tools — active/public projection. Archive/schema switch не удаляет legacy values, product IDs, snapshots и source history. Массовый import проходит dry-run → explicit mapping → validation/conflict report → atomic confirm; строки legacy не переводятся в typed numbers догадками.

## 6. API и приёмка

Общий root — `/api/v2/organizations/{id}`. Предлагаемые relative paths: `/catalog/attribute-definitions`, `/products/{product}/attribute-set`, `/variants/{variant}/attribute-set`, `/catalog/search`, `/products/{product}/media`. CRUD/schema publish — owner; customer search — только server-bound principal. Все writes: expected_version, audit, соответствующий idempotency contract. v1 `attributes: object<string,string>` не меняется.

Acceptance: lossless legacy read/import, type/unit/unknown-field rejection, tenant FK/RLS отрицательные tests, duplicate SKU, numeric filtering plans, stale availability, unverified compatibility → question/handoff, private media denial, current reserve/release/consume regression. Сценарии — [test matrix](UNIVERSAL_BUSINESS_TEST_MATRIX.md).
