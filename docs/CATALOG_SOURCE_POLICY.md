> Этап10Б: источник можно передать через отдельный owner preview/confirm с версиями и историей; обычный CRUD по-прежнему не меняет владельца полей. См. [Source Switching](CATALOG_SOURCE_SWITCHING.md).

# Catalog source policy — 10А

Каталог один. Источник отражается в existing rows, не отдельном Excel/1C каталоге:

- products.catalog_source: MANUAL/EXCEL/ONE_C для имени/описания/бренда/category binding;
- product_variants.catalog_source: SKU/variant name/attributes/price/currency;
- inventory.catalog_source: quantity_on_hand. quantity_reserved остаётся собственностью существующего order workflow.

Старые и новые ручные товары получают MANUAL. Подтверждённые новые Excel rows получают EXCEL. ONE_C — зарезервированный контракт CatalogFeed; реального адаптера или подключения нет. Значение источника не принимается из upload/mapping/API Input. Передача владения между источниками не предоставлена скрытым CRUD.

DB BEFORE triggers запрещают изменение owned fields другим writer и произвольную замену source tag. Trusted backend устанавливает transaction-local EXCEL writer только внутри owner confirmation после проверки preview. Обычные manual API используют MANUAL и получают409 SOURCE_CONFLICT при попытке перезаписи EXCEL/ONE_C. Полностью unchanged MANUAL строки допустимы как no-op без смены источника. Смена product metadata у multi-variant product в импорте блокируется, чтобы не изменять соседние варианты неявно. Категории остаются общим прежним tenant справочником.

UI карточки/склада показывает источник и скрывает обычные формы редактирования external-owned данных. Ручное создание MANUAL товара/варианта сохранено. Publish/archive — отдельное явное действие владельца, не передача цен/остатков. Изменения отсутствующих в файле строк не выполняются.

Reserve/release/consume остаются существующими orders.Service + catalog.MoveOrderStock. Для разрешённого order movement backend устанавливает краткий transaction-local stock flag; source tag не меняется, финансовые/order gates не обходятся. Import не меняет quantity_reserved и не уменьшает on-hand ниже резерва. Проверены imported catalog price grounding, explicit consent, staff confirm/reserve и consume; EXCEL source сохраняется.

Это контроль REST/BFF/LLM boundary и trusted backend SQL, не защита от привилегированного PostgreSQL/control-plane администратора. GUC не является пользовательской настройкой; публичного endpoint для его установки нет. Runtime остаётся aba_app без BYPASSRLS/owner; новые tenant tables FORCE RLS. Audit append-only для runtime, actor берётся из проверенной session; source mutations и import actions журналируются.

Следующие отдельные задачи: управляемая явная передача источника, periodic freshness/reconciliation одинакового stock, retention и trusted ONE_C mapping. В10А нет real sync и нет API перезаписи внешней цены «force=true».
