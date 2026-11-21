# Inventory freshness · 10Б

Один общий механизм `platform.inventory_freshness(org,manual_updated_at)`, tenant-scoped `catalog_policies`. Default fresh_hours24, stale_hours72, low_stock3; owner меняет их versioned API. Настройки применяются одинаково к MANUAL/EXCEL; category/source overrides намеренно не вводятся. Часы UTC, `now()` стабилен в транзакции. Threshold bounds: fresh1..720, stale>=fresh..2160.

- UNKNOWN: timestamp NULL либо из будущего.
- FRESH: age<=fresh_hours.
- AGING: fresh_hours<age<=stale_hours.
- STALE: age>stale_hours.

`inventory.manual_updated_at` — подтверждение физического остатка источником, а не product.updated_at. Reserve/release/consume не освежают его. Owner stock adjustment, даже тем же количеством, фиксирует явную ручную сверку; source guard запрещает менять EXCEL/ONE_C количество, но no-op сверка тем же значением не передаёт владение источником и остаётся в movements/audit; успешный явно подтверждённый Excel snapshot подтверждает все его EXCEL stock rows, включая неизменённые/нулевые. Upload/mapping/preview/cancel не подтверждают остаток. MANUAL-строки при no-op Excel не освежаются. Source handoff сохраняет timestamp. `price_updated_at` отдельно меняется только при новой цене/валюте; historical variant.updated_at не считается доказательством старой цены. applied_at берётся из фактического IMPORT_APPLIED audit для старых jobs.

Customer search/detail/inventory facade возвращает available=NULL для не-FRESH; raw on-hand/reserved удалены из customer inventory. Учётные остатки владельца не обнуляются. Shared agent Runner дополнительно применяет прежний environment StockFreshness cap; AI/order считают данные свежими только при прохождении обеих проверок. Порог среды может быть строже tenant policy. Никакого prompt-only разрешения.

Оба канала используют те же cards и SQL outbox renderers: неактуальное наличие требует подтверждения магазина. Общие orders quote/request сохраняют право создать заявку только с явным consent; staff confirmation повторно проверяет свежесть и доступность. `MoveOrderStock` сам запрещает reserve вне FRESH; release/consume существующего резерва сохраняют корректную семантику. Live/financial gates независимы.

Очередь тоже защищена: before status=sending на обоих outbox `catalog_dispatch_freshness` сравнивает actual inventory, timestamp и available с сохранёнными cards. Устаревшее/изменённое обещание заменяется безопасным текстом без старых offer buttons; пишется CATALOG_DISPATCH_RECHECK. SECURITY DEFINER имеет фиксированный search_path, использует только NEW.organization_id/NEW.id, существующие FORCE RLS migration-owner policies; runtime не получает прямой доступ к чужим rows и RLS не выключается. DELIVERY_UNKNOWN retry semantics не меняются.
