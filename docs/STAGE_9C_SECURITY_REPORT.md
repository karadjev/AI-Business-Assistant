# Security report — 9В

SECURITY STATUS: CONDITIONAL. CRITICAL0/HIGH0/LOW OPEN1 — E01, в проверенном scope. Это не сертификат production readiness и не исчерпывающий аудит.

| Контроль | Результат |
|---|---|
| Tenant RLS | PASS40 ENABLE/FORCE, runtime aba_app не owner/BYPASSRLS |
| Новые grants/provider/evidence таблицы | PASS foreign SELECT/INSERT/UPDATE/DELETE и owner re-parent/write denial |
| Operator authorization | PASS explicit tenant assignment, expiry/revocation, повторная проверка после ожидания и grant SHARE lock |
| Owner/staff escalation | PASS old owner financial function закрыта, channel_resolve WhatsApp закрыт, новый workflow требует operator |
| Financial conservation | PASS existing ledger/concurrency/unknown/crash/recovery и new evidence settlement/replay/rollback |
| Trust labels | PASS manual/operator не Meta; mock принудительно synthetic; OFFICIAL_PROVIDER import запрещён |
| Pricing/template validation | PASS digest, binding, verification, overlap, revocation, expiration перед dispatch |
| Client/server boundary | PASS authenticated session, origin+CSRF, tenant BFF allowlist; no budget/approval/self-grant endpoint |
| Audit | PASS provisioning version/digest, operator assignments, evidence/decision/denial, late conflicting receipt |
| Credentials | Прежние encrypted config/json redaction сохранены; новые evidence — opaque references, provider IDs, никакого credential provisioning |
| Webhook | Existing signed HMAC/dedup/channel routing tests PASS offline |
| External APIs/pricing/version/TLS/legal | NOT RUN, запрещены |

Новые SECURITY DEFINER функции имеют fixed search_path, PUBLIC execute отозван; private digest/eligibility/grant-lock helpers runtime не вызываются напрямую. Авторизация операторских read/write отдельно ограничена конкретным organization_id и назначенным пользователем. Runtime прямого DML к3 новым таблицам не имеет. RLS не отключается. Platform-wide administrative grant bypass не добавлен.

Synthetic attestation подтверждает только offline fixture consistency. Ни hash, ни VERIFIED_OPERATOR не считаются подлинным доказательством Meta. Production цены/approval не созданы, реального provider adapter нет, source label OFFICIAL_PROVIDER не может пройти SQL CHECK. Live transport/config и бюджеты остаются выключенными/нулевыми.

DELIVERY_UNKNOWN не переотправляется автоматически. Reconciliation не устанавливает фактическую доставку, не создаёт новую outbox job и не вызывает transport. Недостаточные/противоречивые evidence удерживают деньги. Settled evidence replay не меняет суммы; отличающийся payload конфликтует. Позднее противоречие пишется в audit, без автоматического повторного settlement.

E01 OPEN: после свежей production build до browser наблюдались `.next 2` и `node_modules 2`, оба пустые. Full path/type/content diff — только эти2 entries; metadata2642 записей менялись. Strict guard отказал42. Original proof сохранён; forensic snapshots/time sequence `.local/audit9c/`. После capture удалены только пустые paths через rmdir; guard не ослаблялся. macOS Virtualization PID68998 имеет read-handles; runtime web bind mount RO, production script только копирует static/public. Causal writer NOT VERIFIED. Наблюдение handles не доказывает виновника. Post-browser результаты дополняются в STAGE_9C_REPORT.

Offline runners: cached images/pull never, internal=true/IPv6false Docker networks, DNS loopback, no external default route, no published ports/socket, proxies/downloads/telemetry disabled. Использованы только отдельные aba_test synthetic DB projects. Рабочие production данные/контейнеры не мигрировались и не перезапускались. Нет dependency downloads, real credentials/API/сообщений/денежных операций/commit/push/deployment.

Final: browser PASS1 expanded test/359.131s/0 skips/errors/flaky на fresh build `X5yxJt--Ha4O28_xPF-Wq`. E01 два empty paths повторились post-browser; strict guard42, forensic capture/exact rmdir, unchanged original proof/served marker/content baseline после этого PASS. Limited scan49 client files0 findings;14 protected files unchanged. Causal writer NOT VERIFIED.
