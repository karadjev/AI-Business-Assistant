> Этап10Б: [Центр управления каталогом](CATALOG_CONTROL_CENTER.md), backend health/freshness, управляемая передача источников и retention импортов. FORCE RLS45; прежние CRUD/API сохранены.

> Актуализация10А: общий каталог теперь поддерживает owner onboarding и подтверждаемый XLSX import. Source MANUAL/EXCEL/ONE_C, atomic preview/confirm, source conflict и43-table RLS — [STAGE_10A_REPORT](STAGE_10A_REPORT.md), [EXCEL_CATALOG_IMPORT](EXCEL_CATALOG_IMPORT.md), [CATALOG_SOURCE_POLICY](CATALOG_SOURCE_POLICY.md). Ниже сохранена история.

# Каталог — этап 4

Каталог реализован в Go `internal/catalog`, PostgreSQL migration 00003 и защищённых Next.js страницах. Прежние auth/organizations и same-origin BFF сохраняются. Telegram, LLM, заказы, платежи и загрузка изображений отсутствуют.

## Модель и правила

`product_categories`: UUID id, organization_id FK, name 1–120, version, created_at/updated_at. Уникальное имя без учёта регистра внутри магазина; плоский справочник. Создание/список/переименование; удаления нет. Составной FK товара RESTRICT не допускает случайного удаления используемой категории.

`products`: UUID id, organization_id, nullable category_id, name 1–200, description до 10000, brand до 100, status draft/active/archived, version, timestamps. UUID в пути — стабильный идентификатор, slug не нужен. Составной FK `(organization_id, category_id)` исключает чужую категорию.

`product_variants`: UUID id, organization_id, product_id, sku, variant_name, attributes JSONB, price_amount BIGINT, currency_code RUB, version, timestamps. Составной FK product; уникальные `(organization_id,id)` и `(organization_id,sku)`. SKU trim + uppercase ASCII, 1–64 символа, `[A-Z0-9][A-Z0-9._-]*`; остаётся занят после архива. Варианты не удаляются. Имя по умолчанию «Основной». Attributes — объект максимум 32 строковых пар, ключи 1–64, значения до 256 символов, JSON до 4096 байт. DB независимо проверяет object/размер; backend проверяет строки и длины.

Цена — целые копейки от 0 до 9 000 000 000 000. Нулевая цена допустима в draft, публикация требует положительную цену всех вариантов и наличие inventory rows. UI принимает десятичные рубли до двух знаков; разбирает цифры в копейки без умножения floating point суммы. Одна функция money форматирует RUB. Общая version варианта защищает одновременно характеристики и цену; updated_at — время последнего изменения, не гарантия физического наличия.

Создание товара атомарно создаёт draft + один default variant + нулевой inventory. Несколько вариантов добавляются отдельно. Публикация, изменение вариантов, склад и архив блокируют product row в одном порядке; статус active не остаётся с нулевой ценой после разрешённой операции. Archived нельзя редактировать, публиковать или корректировать; восстановления и удаления в MVP нет. Изменение active→draft выполняется PATCH.

Все PATCH передают полный редактируемый набор полей, а не JSON Merge Patch; отсутствующие optional поля сбрасываются к documented default. BFF ограничивает JSON body 64 KiB. Обязательны name/expected_version для товара/категории, sku/price_amount/expected_version для варианта. Формы отправляют полный набор. `expected_version` защищает от тихого overwrite: 409 требует обновить данные. POST publish/archive принимает только expected_version. Organization_id из тела запрещён, определяется проверенным URL и session context.

## API

O = `/api/v1/organizations/{id}`. Каждый endpoint требует сессию. Owner пишет, member читает. Мутации требуют Origin и session-bound X-CSRF-Token. Чужой/отсутствующий ресурс одинаково 404, forbidden write в своём магазине 403, version/duplicate/state 409, validation 422. SQL/credentials никогда не возвращаются. Точные schemas — [OpenAPI](../packages/contracts/openapi.yaml).

| Метод | Путь | Назначение |
|---|---|---|
| GET / POST | O/categories | Список / создание |
| PATCH | O/categories/{resource} | Переименование с версией |
| GET / POST | O/products | Список / draft с default variant |
| GET / PATCH | O/products/{resource} | Карточка / изменение |
| POST | O/products/{resource}/publish | Проверенная публикация |
| POST | O/products/{resource}/archive | Архив без удаления |
| GET / POST | O/products/{resource}/variants | Варианты / добавить |
| PATCH | O/variants/{resource} | SKU, имя, attributes, цена с версией |

Products list: q — буквальный case-insensitive substring названия, бренда или SKU; символы `%`, `_`, `\` экранируются. category_id/status/stock(in_stock,out_of_stock), sort(newest,name,price_asc,price_desc). page 1–10000, page_size 1–100, default 20. `{items,total,page,page_size}`; tie-breaker id. Stock/price summary суммирует доступное количество и берёт минимальную цену вариантов. Отдельные страницы данных могут смещаться при новых вставках (offset pagination для малого пилота). Даты UTC ISO 8601. У inventory/history тоже page/page_size.

Индексы: org/status/created/id, category FK, variants org/product и unique SKU, categories normalized name; GIN tsvector подготовлен, но текущий substring search использует ILIKE и может сканировать tenant catalogue. До больших каталогов нужны EXPLAIN и нагрузочные проверки, а не обещание быстрого поиска по произвольной подстроке.

## Внутренние операции для будущего AI

Read-only facade `Service.Customer()` предоставляет только SearchProducts, GetProductDetails, CheckInventory, GetProductPrice. Они принимают доверенный Tenant с проверенной Session + OrganizationID. Каждая операция повторно проверяет membership и RLS, читает только active. Возвращает структурированный JSON с актуальными DB price/version/updated_at/available; не вызывает LLM и не имеет публичного tool endpoint. Draft/archived детали/цена/остаток дают NOT_FOUND; search их исключает. Worker без пользовательской сессии пока не получает этот доступ: будущий system principal потребует отдельного задания.

UI: `/dashboard/products`, `/new`, `/[id]`, `/dashboard/categories`; SSR session/membership, owner формы, member read-only, безопасное переключение org из списка memberships, XSS-safe текст без HTML rendering. История и склад — [INVENTORY_MANAGEMENT](INVENTORY_MANAGEMENT.md). Изображения, импорт CSV, восстановление архивов, массовое редактирование, fuzzy/semantic search, multi-currency и bulk price lists отложены.

## Расширение этапа5

CustomerCatalogue используется семью scoped agent tools для test chat. Цена/stock перечитываются перед quote/consent/employee confirm. Цена в заявке snapshot immutable, изменённая требует нового согласия. Inventory включает manual_updated_at; freshthreshold24h configurable; draft/archive по-прежнему исключаются. Member staff grant не даёт catalog/settings write. Отдельный test shop имеет собственные товары/warehouse, не копирует ordinary shop.
