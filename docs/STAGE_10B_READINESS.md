# Готовность к10Б

Следующий этап рекомендуется посвятить offline пилоту реальных форматов прайс-листов на синтетических данных: расширению supported XLSX cases, управляемой передаче источников и stock freshness/retention. Реальная1С пока не подключена; CatalogFeed — только контракт.

Нужны отдельные решения:

- Owner-visible source handoff с preview/audit/версией, без скрытого price override.
- Политика обновления stock freshness при неизменившемся остатке и сверке внешнего источника.
- Controlled retention/cleanup просроченных import jobs; текущие fail-closed bounds20 active/100 total и24h apply TTL не заменяют удаление данных.
- После отдельного решения — расширение1С adapter/мульт листов/картинок и поддерживаемых форматов. Никаких автоматических dependency downloads или сетевых подключений.
- Review store-policy tool/owner AI instructions в общей agent архитектуре: профиль сохранён, но live prompt не подключается и не получает прав обходить существующие gates.
- E01 LOW остаётся OPEN, пока causal writer не доказан. Исключения для suffixed generated paths запрещены; record/verify guard теперь явно отвергает оба известных имени.

Telegram/WhatsApp live NOT READY. Бюджеты LLM/WhatsApp0 RUB. Блокеры9В (официальные тарифы/templates/version, operator/privacy/invoice/HTTPS procedures и точное разрешение на внешние вызовы) не устраняются onboarding ACTIVE. До них реальных интеграций/клиентов/paid operations быть не должно.
