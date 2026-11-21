> Актуализация9В: owner-only settlement ниже — историческая модель9Б. В00012 legacy mutation закрыта; только назначенный platform operator через evidence workflow. Admission требует provider provenance. См. WHATSAPP_FINANCIAL_RECONCILIATION.md и TRUSTED_PROVIDER_PROVISIONING.md.

# WhatsApp financial admission — 9Б

## Trust boundary и область

`FinancialAdmission` реализует существующий `SendAdmission`, не создаёт новый агент/заказы/канал. Реальный `NewCloudTransport` принимает только durable `*FinancialAdmission`; произвольное in-memory разрешение не допускается к Graph. Default flags/budgets выключены/нулевые. Production factory также отвергает Simulation. `NewOfflineCloudTransport` принимает только literal loopback HTTP URL, без credentials/query/path/fragment; сам переписывает fixed Graph request на локальный emulator без proxy/DNS. Инъекция DefaultTransport не может случайно отправить synthetic операцию в Meta.

Worker использует прежний pinned connection и integration advisory lock8210. Admission выполняется на **том же соединении** непосредственно перед HTTP; иначе второй connection ждал бы собственный session lock. Внешнего HTTP внутри SQL-транзакции нет. Проверка/резерв и finish — отдельные атомарные PostgreSQL функции. Control/handoff/verification используют тот же integration lock. Templates не могут деградировать до generic free-text sender: нужен отдельный TemplateSender; неподдерживаемый sender не отправляет сообщение.

## Резервирование

`platform.wa_reserve` проверяет worker proof hash, current outbox capability/attempt/lease, organization/integration/recipient и точное содержимое text/actions. Phone Number ID, WABA и Graph version связаны с записью интеграции. Затем — active/sending flags, account enabled/kill, synthetic realm, open conversation/handoff, окно или approved template. Для text необходимо ещё6s окна; template verification/pricing также должны оставаться действительными6s, worker HTTP deadline5s. Pending job после ожидания не использует старое UI-решение.

Блокировки: integration → platform wallet → organization wallet → outbox. Валидная tariff policy задаёт известный verified conservative maximum в microRUB (1 RUB=1_000_000); неизвестный region/currency/cost/источник/срок/тип биллинга блокирует admission. Max операции1e12, wallet1e15; никакого float/FX. Без явно проверенного free rule amount0 не разрешён. Verified-free amount0 может пройти zero money limit только в явно разрешённой классификации CloudOptions.AllowVerifiedFree и при всех остальных доказательствах. В текущем runtime live всё равно запрещён.

Уникальность outbox+attempt предотвращает двойной резерв; reserved/unknown/charged предыдущего attempt запрещают повторный допуск. Только released известной неотправленной попытки может уступить место новому fenced attempt. Отрицательные балансы, превышение лимита и несогласованность tenant FK запрещены constraints. Platform lock обеспечивает суммарный cap между организациями; его состояние не раскрывается одному tenant.

## Settlement и unknown

| Событие | Финансовое состояние |
|---|---|
| 200/providerID, стоимость ограничена проверенным maximum | charged — **консервативная оценка**, не подтверждённый счёт Meta |
| Доказано, что запрос не отправлялся (`NoChargeConfirmed`, backend-only) | released один раз |
| Timeout/reset/5xx/ошибка settlement/неопределённый расход | unknown; сумма остаётся reserved |
| HTTP reject/429 без подтверждения нулевого расхода | unknown финансово, даже если доставка отклонена |
| Crash до/после HTTP, lease expired | recover→unknown; не refund |

`SendResult.NoChargeConfirmed` не берётся из webhook/LLM/UI. Cloud выставляет его только при отмене/ошибке формирования **до HTTP**. Неуспешная доставка сама по себе не является доказательством бесплатности. Delivery retry429 дополнительно ограничен финансовым gate: спорная стоимость не позволяет повторно платить; до сверки такие повторы блокируются. Повтор finish не меняет уже финальное/unknown состояние.

`wa_recover` обрабатывает максимум100 expired/inconsistent reservations за tick, SKIP LOCKED, удерживает деньги и пишет аудит. Повтор/concurrent recovery не создают второй финансовый эффект. Оператор не должен перезапускать отправку unknown job с новым id ради обхода блокировки.

## Ручная сверка

Только owner через authenticated CSRF-protected API, expected_version, explicit confirmation, charge≤amount и evidence reference10–1000 символов. `wa_reconcile` под тем же порядком wallet locks переводит unknown→charged/released и учитывает сумму один раз. Совпадающий evidence hash+charge допускает идемпотентный повтор; другое решение/версия конфликтует. Сохраняются actor_id и SHA256 ссылки на доказательство, не полный чувствительный текст. Сверка **не** считает сообщение доставленным и не запускает resend; delivery resolution остаётся отдельным действием.

Пример структуры запроса без credentials: POST `…/financial/{reservation}/reconcile` с expected_version, charge_microrub, evidence_reference (код локального ticket, без ПД), confirm_reconciliation=true. Не публиковать cookies/CSRF/ключи в отчёте. API доступен в текущем test-org контуре; backend не даёт staff менять деньги.

## Схема и доступ

00011 добавляет FORCE RLS на wa_budgets/wa_pricing/wa_templates/wa_reservations/wa_financial_events — всего37 tenant-owned таблиц. Runtime имеет owner-scoped SELECT; DML только через строго проверенные SECURITY DEFINER функции с fixed search_path. Migration owner имеет отдельную policy, runtime не owner/BYPASSRLS. wa_platform_budget — global singleton без runtime SELECT/DML; wa_function_backup хранит DDL для rollback, не tenant/customer данные, тоже недоступен runtime.

Default wallet limits0, enabledfalse, killtrue; нет API повышения бюджета. Синтетические положительные fixtures возможны только в isolated aba_test и не допускаются production transport. Ledger snapshots/FK сохраняют финансовый аудит; down отказывается удалять populated финансовые таблицы и подтверждённый recipient_region даже до создания ledger. Старые00001–00010 не редактируются. Рабочие данные не мигрировались этим аудитом.
