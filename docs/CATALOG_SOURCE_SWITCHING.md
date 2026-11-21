# Controlled catalog source handoff · 10Б

Сохранён единый catalog_source guard13. Новый owner-only POST catalog-control/source: operation_id(UUID), product_id, target MANUAL/EXCEL, reason(1..500), confirm. ONE_C не активируется. Preview передаёт всю карточку с группами product metadata, variants price/attributes и inventory on-hand; несколько вариантов входят одной транзакцией. Существующие source tags не подменяются из обычного CRUD/XLSX.

Первый запрос confirm=false сохраняет 15-minute snapshot всех полей/версий/источников и actor; значения каталога не меняет. До100 неистёкших source previews на tenant. UI показывает текущую/предлагаемую цену для сравнения и последствия передачи. Предлагаемая цена вообще не отправляется в mutation API: это сравнение, не override.

Отдельный confirm=true с тем же operation ID/body: existing owner/session/Origin/CSRF/BFF, tenant RLS → общий catalog advisory lock → product/inventory locks в прежнем порядке → preview row lock → JSON snapshot equality → source-only guarded updates → versions+1 → APPLIED/applied_at → audit SOURCE_CHANGED с resource_id=operation_id → COMMIT. Цены/stock/timestamps не переписываются. Старые import/source previews становятся stale. Дубли APPLIED возвращают существующий результат; другой body того же ID/expired/stale/foreign/staff/ONE_C блокируются. Два разных preview одного товара не могут оба примениться к одной версии.

`catalog_source_changes` FORCE RLS, compound product FK, owner policies, actor=current_user_id insert, append-only applied history trigger, нет runtime DELETE. Снимок показывает прежние источники/версии каждой группы, target и детерминированные новые версии (+1), actor/time/reason/op ID. История не очищается Excel retention worker.

Safe actions: оставить текущее, отменить, запросить передачу. После передачи в MANUAL цену меняют через прежнюю versioned карточку товара. Прямая ручная перезапись EXCEL/ONE_C остаётся409 SOURCE_CONFLICT. Source handoff не означает проверку фактического остатка и не включает live.
