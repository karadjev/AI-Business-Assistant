# Universal Onboarding & Workspace UX

Статус: **UX/CONTRACT DESIGN ONLY**. Branding AI Business Assistant, существующие tokens/components, responsive layouts и accessibility patterns сохраняются. В этом этапе frontend не изменяется.

## 1. Что есть сейчас

`StoreOnboarding.tsx` показывает шесть store-oriented шагов: информация, каталог, preview, AI, каналы, готовность. Profile validation требует address/delivery/payment/returns даже без product sales. Import предполагает RUB и integer stock. `Sidebar.tsx` всегда показывает «Товары», заказы и магазинные разделы. Это хороший reusable shell, но не service/manufacturing-ready UX.

## 2. Восемь будущих шагов

| Шаг | Данные / действия | Gate продолжения |
| --- | --- | --- |
| 1. Бизнес-модель | Trade/service/agency/manufacturing/hybrid или supported custom code; retail/wholesale modes | Effective capabilities и non-food trade eligibility |
| 2. Отрасль | Несколько industry/sub-industry bindings или generic specialization | Approved compatible profile versions; не закрытый dropdown навсегда |
| 3. Специализация | Description, locations, timezone/currency; typed required questions | Unknown факты допускаются как unknown, не auto-filled обещания |
| 4. Каталог / услуги | Product import/manual OR service offerings OR listings/custom intake | Capability-specific validation; нет fake product/stock для услуг |
| 5. Правила работы | Hours/exceptions, delivery или booking/cancellation/estimation policies | Required только применимые поля; explicit policy revision |
| 6. AI | Approved public context, clarification/handoff, preview available operations | Core guardrails неизменяемы; unsupported tools disabled |
| 7. Каналы | Telegram/WhatsApp/Instagram readiness из backend | Mock и live статусы разделены; credentials никогда в AI/profile |
| 8. Тестирование | Synthetic сценарий с quote/request/handoff/approval | Сохранённый validation result; config complete ≠ production enabled |

Service-only бизнес не обязан загружать товары, задавать возврат товара или выдумывать stock. Hybrid видит обе ветки. Manufacturing custom-only начинает с specification/RFQ, без фиктивного готового склада.

## 3. Сохранение и изменение профиля

Draft сохраняется server-side по tenant и expected_version. Новый wizard использует `business_onboarding_steps` с profile/capability revision; существующий `store_profiles.status` остаётся legacy summary projection, а не второй источник независимой готовности. Back/refresh/mobile не теряют уже сохранённые шаги; несохранённый upload явно отмечен.

Переход industry/model после регистрации: preview impact → explicit mapping → owner confirmation → publish new revision → повторная проверка затронутых шагов. Не обнулять канал, историю, products/services/orders. Disabled capability скрывает новые действия, но не историю прежних заказов/bookings.

При concurrent edit вернуть version conflict и показать изменение. Retry создающего действия использует idempotency key; UI не создаёт второй заказ/booking при повторном нажатии.

## 4. Универсальная панель

Общий shell: обзор, обращения/диалоги, заказы/заявки, AI, каналы, настройки. Внутри existing request list — type filter, status и responsible employee; отдельный inbox по отрасли не нужен.

| Effective capability | Дополнительные разделы |
| --- | --- |
| product_sales | Existing товары/категории/остатки/качество каталога |
| wholesale_pricing | Прайсы и quantity tiers только уполномоченным сотрудникам |
| service_requests | Услуги, intake/assessment queue |
| scheduling | Календарь/записи/resources по отдельным permissions |
| property_listings | Объекты/статусы публикаций/запросы на просмотр |
| custom_quotations / production_requests | Технические задания/оценки/engineering review |

Не показывать все отраслевые поля одновременно. Reuse existing form/table/status/card components; definitions задают labels/input type/help, но не произвольный HTML/JS. На mobile — progressive disclosure и доступные labels/focus/error summaries. Date/time всегда показывает location timezone, unknown price/availability — явный badge.

Capability visibility приходит из server effective-capability projection. Скрытая вкладка не заменяет API authorization; прямой URL/API к выключенному или чужому модулю отклоняется. Старые URLs/закладки сохраняются через compatible route/view, без изменения бренда или глобального visual redesign.

## 5. Readiness и каналы

Отдельные states: configuration draft, configuration validated, offline scenario passed, channel mock configured, live access authorized, provider proof pending/error. Нельзя одним общим зелёным «Готово» скрыть subscription/permission/budget failure.

Channel readiness агрегирует current provider status/permissions/webhook observation/kill switch, а не просто `integration.active`. Instagram Stage 2 остаётся mock-only и имеет незакрытый PostgreSQL gate. Telegram не переводится на другую очередь ради UX. WhatsApp templates/pricing/provider/budget gates показывают фактический отказ; wizard их не обходит.

Текущие LLM/WhatsApp budgets0 и live-disabled не меняются. Подтверждение настройки не инициирует реальные сообщения и не является deployment approval.

## 6. Proposed API и acceptance

Tenant root v2: `GET/PATCH /business-profile`, `GET /business-capabilities`, `GET/POST /business-onboarding`, `POST /business-onboarding/validate`, industry preview/publish endpoints из [profile design](INDUSTRY_PROFILES_DESIGN.md). Validation возвращает per-capability readiness/blockers и profile revision, не secrets.

Acceptance: восемь веток matrix без нерелевантных mandatory fields, возвращение после refresh, version conflict, keyboard/mobile flows, private resource denial по прямому URL, profile switch сохраняет историю, unknown/error не отображается как connected, сохранение существующего electronics workflow и branding.
