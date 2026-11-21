# Trusted pricing policy — 9В

Реальных тарифов нет. Не выполнялись запросы к тарифным страницам или Meta API. Все fixtures SYNTHETIC, денежные значения в tests — условные microRUB, runtime budgets0.

Существующая wa_pricing сохраняет version/effective dates/region/category/message type/billing basis/currency/conservative maximum/free classification/source/freshness. wa_provider_records добавляет происхождение, verification method, expiration, integrity digest и отзыв. Проверка цены находится в прежнем `wa_reserve`, а не в UI.

Допуск требует одновременно:

- Разрешённые tenant/integration/lease/outbox/kill/window либо approved template.
- Одна применимая policy, совпадающие trusted recipient region/category/message type и RUB.
- Известный conservative maximum, проверенный срок и source; для zero amount — отдельный verified_free.
- Действующее provenance, точный digest, неотозванный record, правильный realm.
- Атомарные platform/tenant wallet reservations.

Больше одной одновременно подходящей valid policy блокирует операцию; сортировка по версии не разрешает противоречие автоматически. Старую policy нужно явно отозвать/вывести из действия. Версии, synthetic units и regionZZ не являются текущими правилами Meta. Billing basis конфигурируется; нельзя считать модель гарантированно message-based или conversation-based. FX отсутствует, неподдержанная валюта блокируется.

MANUAL_UNVERIFIED/VERIFIED_OPERATOR сохраняются с UNVERIFIED и не дают финансовый admission. Mock принудительно SYNTHETIC_TEST. Даже валидная synthetic policy не может разрешить live: SQL provenance predicate и Cloud constructor отвергают такой режим. OFFICIAL_PROVIDER import в этой версии закрыт полностью. Реальная интеграция потребует отдельного проверяющего адаптера и подтверждения происхождения/актуальности источника.

Owner не может менять policy или verified статус. Backend proof нужен даже для offline provisioning. Runtime DML запрещён. Operator revocation проверяет tenant grant, expected_version и integration lock. Pricing created/verified/revoked/expired и rejected/conflict imports пишутся в wa_financial_events; импорт сохраняет version/digest в audit. Expiry проверяется по часам в admission независимо от sweep; sweep нужен только для наблюдаемости.

Прошлые резерв/списание не пересчитываются от обновления policy: reservation сохраняет amount и pricing_id. Charge остаётся conservative estimate, не утверждение о фактическом invoice Meta. Никакого повышения лимитов или автоматической покупки тарифов нет.
