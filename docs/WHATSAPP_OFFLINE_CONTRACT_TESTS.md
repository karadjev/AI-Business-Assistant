# WhatsApp offline contract tests — 9А

Все внешние API запрещены. New Cloud wire tests используют `httptest.NewServer`, HTTP serializer/parser настоящего транспорта и private test-only RoundTripper, который проверяет fixed official origin и переписывает адрес только на локальный сервер. В production endpoint override отсутствует. Конструктор не делает сетевых вызовов. Config version v99.0 и credentials mock:synthetic вымышленные. Значение1 в test-only budget fixture — условный разрешающий unit token, не настройка runtime и не реальная денежная операция; оба app budgets0.

| № | Сценарий | Результат / доказательство |
|---|---|---|
|1|Успешный text|PASS — TestCloudWireContracts/text|
|2|Approved template|PASS — TestCloudTemplateAndInteractive (synthetic trusted admission)|
|3|Неверный токен|PASS — TestCloudWireContracts/bad_token|
|4|Истёкший токен|PASS — TestCloudWireContracts/expired_token (provider code190/subcode463 fixture)|
|5|HTTP400|PASS — TestCloudWireContracts/400|
|6|HTTP401|PASS — TestCloudWireContracts/bad_token|
|7|HTTP403|PASS — TestCloudWireContracts/403|
|8|HTTP429|PASS — TestCloudWireContracts/429, Retry-After2s|
|9|HTTP500|PASS — TestCloudWireContracts/500, UNKNOWN|
|10|Timeout|PASS — TestCloudTimeoutResetAndSettlement/timeout|
|11|Connection reset|PASS — TestCloudTimeoutResetAndSettlement/reset, server hijack+close|
|12|Malformed response JSON|PASS — TestCloudWireContracts/malformed|
|13|Нет provider message ID|PASS — TestCloudWireContracts/missing_id|
|14|Duplicate webhook|PASS — PostgreSQL multichannel_test +3shops×2channels replay|
|15|Invalid HMAC|PASS — TestWebhookContract/wrong_signature + actual HTTP integration|
|16|Unknown Phone ID|PASS — TestWebhookContract/wrong_phone + SQL integration|
|17|Cross-tenant routing|PASS — TestWebhookContract/wrong_account/wrong_integration + runtime SQL/RLS32|
|18|Expired window|PASS — TestCloudTemplateAndInteractive denies text; worker SQL window scenarios|
|19|Worker restart|PASS — actual process crash/recovery integration|
|20|DELIVERY_UNKNOWN|PASS — Cloud timeout/reset/5xx + existing queue manual recovery tests|
|21|Budget0|PASS — TestCloudFailClosedNoHTTP/zero_org_budget,zero_platform_budget + config integration|
|22|Disabled live|PASS — TestCloudFailClosedNoHTTP/disabled,kill,no_authorization,no_admission|
|23|Secret redaction|PASS — Cloud typed sanitized error checks + existing JSON credentials tests|
|24|Unsupported event|PASS — TestWebhookContract/media + malformed/trailing JSON + worker unsupported scenarios|

Дополнительно: response>64KiB; redirect не следует Location; context cancelled before HTTP; unsupported version; header injection; wrong tenant/integration/user; missing admission; settlement error->UNKNOWN; unapproved template отказ; реальные credentials не принимаются runtime mock Connect. Проверяются actual request method/path/Bearer/content-type/product/to/type/text/template language/components; provider message ID сохраняется в SendResult. Existing queue tests проверяют его связь с outbox, receipt routing и async status monotonicity.

Cloud authorization tests доказывают ноль вызовов HTTP при запрещающих условиях. Settlement mock проверяет вызов/состояние и ошибку, **не доказывает live финансовую транзакционность PostgreSQL WhatsApp**: такой backend пока не активирован. Existing PG ledger crash tests относятся к LLM ledger, а current WhatsApp SQL gate запрещает live при budget0. Отдельной реальной template queue/approved registry нет; local approved flag — имитация доверенного admission, не UI-флаг.

Команды: prepared Compose project aba-stage9a/race (`go test -race -count=1 ./...`); aba-stage9a/go (tests/vet/format/build); aba-stage9a-check/integration (`go test -race -tags=integration -count=1 -v ./tests/integration`). Все images cached, internal networks/no external route, dependencies offline. Логи `.local/audit9a/go.log`, `race.log`, `integration-race.log`. Go integration PASS43.427s; top-level subprocess helper SKIP не выдаются за отдельные пройденные tests — их parent crash scenarios реально выполнялись.

Initial development failures не скрыты: первый local timeout handler ожидал cancellation бесконечно при unread request body — собственный test container остановлен, fixture заменён bounded60ms response для реального client20ms timeout. Следующий прогон обнаружил нарушение старого Config.Validate mock-only contract; разделены transport ValidateConfig и mock Validate, runtime Connect дополнительно требует mock access prefix. Полные тесты после исправления PASS. Первый OpenAPI запуск в web-build не имел /packages mount; повтор с read-only packages mount PASS. Это исправления test orchestration/границ нового кода, не основание засчитать неудачные прогоны PASS.

Реальные Meta error semantics/version/pricing/TLS/token validity NOT RUN. Успешные mock tests не подтверждают актуальные правила Meta или доставку WhatsApp. Browser regression текущей production build приведена отдельно в STAGE_9A_REPORT.
