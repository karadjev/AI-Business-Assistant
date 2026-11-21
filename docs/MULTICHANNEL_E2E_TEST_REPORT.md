# Многоканальные offline E2E — этап 8В

20.10.2026. **Telegram backend offline E2E: PASS. WhatsApp backend offline E2E: PASS. Полный current Next.js SSR browser E2E: NOT RUN.** Слово backend существенно: последний шаг отображения новых страниц текущей сборкой ещё не доказан.

## Стенд и доказательства

Новый `apps/api/tests/integration/prelive_matrix_test.go`, `testPreliveMatrix`, вызывается из `database_test.go` вместе с прежними integration suites. Последний полный прогон: **PASS 17.538s**. Полный stdout `.local/audit8v/artifacts/integration.log`; метаданные фактических policies/FK/runtime grants — `docs/SECURITY_RLS_INVENTORY_8V.json`.

Три независимых synthetic Shop A/B/C, по владельцу и отдельному staff member с can_manage_orders, собственным одинаково названным PHONE8V, собственным variant, цене 110099/220099/330099 копеек и on_hand=1, двум интеграциям. Одинаковые внешние IDs 101001/101002 в разных org/channel не объединяются. Отдельная таблица customers в текущей схеме не добавлялась: customer_id и channel principals — фактическая существующая модель. Реальные credentials не применялись.

Настоящие HTTP requests backend и wire adapter перенаправляются строго в httptest loopback server; PostgreSQL выполняет фактические SQL, транзакции и RLS. Не имитируется результат DB или заявки. Mock LLM остаётся активным. Telegram HTTP transport проверяет getMe/sendMessage wire **только локально**, это не обращение к Telegram. WA HTTPMock проверяет официальный message shape, локальный endpoint, recipient и provider response. За прогон новой матрицы wire send counters: TG37, WA37.

| Сценарий | Telegram | WhatsApp | Доказательство / предел |
|---|---|---|---|
| webhook → durable inbox → principal → общий agent/catalog → outbox → HTTP mock → sent | PASS | PASS | Новая матрица 3×2, настоящий HTTP webhook и authenticated conversation list API |
| Авторитетный variant/цена своего магазина | PASS | PASS | Cards parsed, exact variant/price assertions всех трёх магазинов |
| Отсутствующий товар | PASS | PASS | Отсутствуют catalog cards; товар не выдуман |
| Неверный/отсутствующий secret или HMAC | PASS | PASS | Новые HTTP негативные сценарии; WA изменённое тело со старой подписью403 |
| Чужая/неизвестная integration / Phone ID | PASS | PASS | Unknown integration403; WA wrong Phone ID403 |
| Подмена organization_id в JSON | PASS | PASS | Тело не меняет routing; foreign inbox count неизменен |
| Allowlist до нового диалога | PASS | PASS | Неизвестный user403; ID не даёт staff-полномочий |
| Payload >limit | PASS | PASS | HTTP413; validation tests malformed JSON/unsupported media в regression suites |
| Повтор webhook/кнопки | PASS | PASS | Повтор одного update и новое событие той же confirm-кнопки: одна заявка |
| До explicit quote consent заявки нет | PASS | PASS | SQL order count0, затем quote-bound callback |
| Snapshot и quantity1 | PASS | PASS | Exact price/qty проверены; external cart остаётся single variant×1 |
| До staff reserve0; reject без резерва | PASS | PASS | Actual inventory assertions всех шести сценариев |
| Две заявки / последняя единица | PASS | PASS | Два concurrent staff confirm: один успех, reserved1, затем idempotent cancel reserved0 |
| Price conflict / sold out перед staff confirm | PASS | PASS | Для каждого из6 channel-created orders authoritative fixture price+1 → PRICE_CHANGED, on_hand0 → INSUFFICIENT_STOCK, reserve остаётся0 |
| Completion exactly once | PASS shared service | PASS shared service | stage5 regression; отдельный completion через каждый webhook/browser не дублировался |
| Межорганизационные API/service/SQL | PASS | PASS | Цикл A→B→C: foreign conversation404, staff reply/stock/quote denied; scoped SQL0 |
| Channel principal collision | PASS | PASS | TG/WA customer одного external ID имеют разные conversations |
| Handoff / staff reply / закрытое окно | PASS | PASS | stage6/multichannel suites; новый matrix не дублирует все control paths |
| 429, timeout, 5xx | PASS | PASS | Telegram audit/reliability; WA TestHTTPWireContract/TestTimeoutAndCancelledContextAreUnknown через loopback HTTP |
| Worker после provider receipt умер | PASS | PASS | Реальные child process exit, lease expiry, recovery без resend; reliability/whatsapp_crash tests |
| Два worker, manual UNKNOWN и stale version | PASS | PASS | Existing reliability/multichannel, actual HTTP count1 |
| Retry exhaustion/backoff/stale capability audit | Existing bounded retry regression | PASS | Новый WA subtest: 5 claims, backoff, failed, old capability ignored, 5 audit attempts, no resend |
| Delivery sent/delivered/read/late failed | Не применимо | PASS | multichannel_test: read не понижается поздним sent/failed; unit transition tests |
| Закрытое service window / нет template | Не применимо | PASS | Staff отказ, queued needs_approved_template, без платного send |
| Отключение | PASS | PASS | Новые webhook403 и отсутствие нового send; queued/in-flight guards прежних suites |
| Retention / payload purge | PASS | PASS | Regression synthetic purge tests; политика реальных данных не утверждена |
| Dashboard channel filter API | PASS | PASS | Session-auth GET содержит ожидаемый conversation; это не browser assertion |
| Свежие SSR pages + обе channel cards в полном приложении | NOT RUN | NOT RUN | Fresh Next production build BLOCKED |

Отдельные сценарии crash **до** отправки/во всех возможных instruction boundaries, новая полная browser-проверка трёх организаций и всех WhatsApp screens не заявляются проверенными только на основании соседних тестов. Все shared service PASS относятся к выполненным общим тестам, а не к отдельному полному wire сценарию каждого канала.

## Бюджеты и безопасность

PASS: zero LLM budget не вызывает actual Compatible transport, владельцы не обходят platform cap, staff не меняет budget; PG concurrent reserve/settle/recover UNKNOWN idempotent. Положительные synthetic ledger fixtures не включают live и не меняют runtime budget0. WA config/policy/DB CHECK запрещают положительный денежный бюджет/live; mock wire работает без денег. Это не реализация/проверка будущего платного WA ledger.

32 tenant tables реально проверены под aba_app: чужие SELECT/INSERT/UPDATE/DELETE, reparent/WITH CHECK, FORCE, migrator owner, отсутствие tenant context leak. Полный список policies/FK сохранён отдельно. Global identity/session/platform budget проверяются отдельными API/SQL правами и не объявляются tenant таблицами.

## Browser regression

В изолированном smoke stack: **5 passed (48.7s)** — регистрация/email/session/organizations/catalog/orders/mock Telegram/AI settings/XSS/logout, главная и auth redirect, backend status, loading/unavailable/invalid contract. Один backend-down test сначала намеренно skipped; затем при реально остановленном тестовом API выполнен отдельно: **1 passed (8.2s)**. Случайный дополнительный запуск без нужного флага также skipped и доказательством не считается.

Web — cached stage7 standalone, backend — свежие binaries8В. Current TSX/browser отдельно: `test-multichannel-components.cjs` PASS (WA mock/owner UI/409/CSRF/XSS/tenant/UNKNOWN), `test-recovery-components.cjs` PASS (recovery + restricted current BFF forwarding). Это не полная свежая Next production/browser E2E; критерий R остаётся NOT RUN в полном требуемом объёме.
