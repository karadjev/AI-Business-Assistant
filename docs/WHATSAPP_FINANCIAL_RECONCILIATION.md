# WhatsApp financial reconciliation — 9В

Сохраняется прежняя reservation/settlement/DELIVERY_UNKNOWN модель. Unknown не является доставкой или бесплатностью. Нет автоматических retry/refund. Owner-only mutation9Б закрыта и на HTTP, и в SQL; legacy Service.Reconcile/wa_reconcile отказывают. Прежний channel_resolve для WhatsApp также не позволяет staff/owner принудительно установить delivery state. Telegram logic сохранена.

Полномочия:

| Участник | Возможности |
|---|---|
| Owner | Видит свои budgets/templates/provider readiness; не сверяет деньги и не подтверждает provider status |
| Staff | Прежние разрешённые store operations; financial/operator mutation запрещена |
| Platform operator | Только явно назначенные tenant scopes; expiring/revocable grant; evidence workflow и provider revocation |

Новые endpoint под существующими auth/CSRF/BFF:

- GET `…/integrations/whatsapp/operator`: per-tenant unknown/history list, store, channel, reserve, timestamp, version, evidence/decision history.
- POST `…/financial/{reservation}/reconcile`: operator-only evidence workflow.
- GET `…/provider`: readonly readiness владельца.
- POST `…/provider/{record}/revoke`: operator-only, expected_version и explicit confirmation.

UI: `/dashboard/operations/whatsapp?org=<assigned_tenant_id>`. Это per-tenant операторский deep link; глобальный обход RLS/список чужих магазинов отсутствует. Назначение grant выполняется отдельным control-plane процессом, не обычным UI. В offline E2E используется test-only helper на собственной aba_test БД.

Для решения нужны event_id UUID, expected_version, confirm_reconciliation=true, decision, charge_microrub, evidence_type, evidence_reference и evidence_at; provider_message_id опционален. Reference — только opaque `ticket:CASE_0001` или `synthetic:CASE_0001`; URL/access tokens/свободный текст/ПД не принимаются. Хранятся reference, timestamp, operator identity, tenant/integration/outbox/reservation/provider IDs, decision/outcome/time и digest запроса. История append-only для runtime. SHA256 не выдаётся за provider signature.

| Workflow | Финансовый результат |
|---|---|
| Unknown без evidence | AWAITING_RECONCILIATION: резерв удерживается |
| INSUFFICIENT | evidence сохраняется, version растёт, резерв удерживается |
| CONFLICT | противоречие записано, резерв удерживается, автоматического завершения нет |
| CONFIRMED_SENT + подтверждённая допустимая сумма | COMPLETED, один settlement; nonzero reserve нельзя освободить простым sent+charge0 |
| CONFIRMED_NOT_SENT + отсутствие платы + charge0 | COMPLETED, release один раз, при отсутствии противоречащего provider evidence |
| Новый несовместимый запрос после завершения | VERSION_CONFLICT, audit, без второго изменения денег |

Это ручное утверждение авторизованного оператора по внешнему ticket, а не автоматически проверенное подтверждение Meta. Синтетическое evidence нельзя применить к real reservation. Настоящий invoice/operator evidence workflow требует отдельной организационной проверки перед live.

Lock order: integration8210 → platform wallet → tenant wallet → reservation → outbox → grant SHARE. Grant повторно проверяется после ожидания; изменение назначения блокируется до commit. Старый expected_version и конкурирующий оператор не могут списать/освободить деньги дважды. event_id+request digest обеспечивает replay одинакового запроса, отличающийся payload конфликтует. Дубликат требует действующего operator grant.

API коммитит возвращённые SQL denial/conflict codes перед формированием HTTP error, поэтому audit разрешённых к обработке аутентифицированных попыток не исчезает при отказе. Неожиданная SQL ошибка откатывает всю финансовую транзакцию. Direct SQL transaction rollback также откатывает данные и audit как единое целое; отдельного скрытого autonomous transaction нет.

Сверка не переводит outbox в delivered/sent/pending и не выполняет HTTP. Reconciled attempts дополнительно блокируют повторное financial admission. Поздний receipt не пересчитывает ledger; противоречащий completed NOT_SENT receipt создаёт LATE_PROVIDER_EVIDENCE_CONFLICT. Он требует новой operator investigation; автоматического исправления invoice/повторного платежа нет. Повтор одного unchanged receipt не создаёт дополнительного финансового эффекта.
