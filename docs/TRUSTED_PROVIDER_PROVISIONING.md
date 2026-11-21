# Trusted provider provisioning — 9В

Строго offline. OFFICIAL_PROVIDER не установлен; ни одного реального подтверждения Meta нет. Бюджеты0 RUB, live disabled.

Карта существующих компонентов:

| Компонент | До 9В | Расширение |
|---|---|---|
| `internal/whatsapp/cloud.go` | HTTP + обязательный SendAdmission | сохранён, synthetic запрещён real constructor |
| `financial.go`, migration00011 | durable reserve/finish/recover, owner reconciliation | provenance в admission; owner mutation закрыта |
| `worker.go`, channel_outbox | lease/capability/attempt/integration lock | прежняя очередь; expiry audit при recovery |
| wa_pricing / wa_templates | verified/source/synthetic/freshness | связанные wa_provider_records, integrity/version/revocation |
| auth/session, tenancy, BFF | tenant owner/staff | отдельные expiring tenant operator assignments на тех же сессиях |
| wa_financial_events | ledger audit | provisioning, grants, evidence, denial и late receipt audit |

`PricingPolicySource` и `TemplateStatusSource` — разные backend-интерфейсы. Тарифы не считаются частью template API. Установлены только `MockPricingSource`/`MockTemplateSource`: они принудительно перезаписывают provenance как SYNTHETIC_TEST / synthetic:offline-fixture / OFFLINE_DIGEST и вычисляют SHA256 payload. Ложная метка OFFICIAL_PROVIDER в mock input не сохраняется.

`Service.ProvisionPricing/ProvisionTemplate` проверяет offline source, границы JSON/ID, backend proof и tenant/integration binding. `platform.wa_provision` сериализует обновления integration lock8210, проверяет SHA256 точных входных bytes, optimistic expected_version, business keys/WABA/components/schema и срок максимум7days. Дубликат одинакового payload/provenance/expiry идемпотентен; конкурентный другой payload на той же версии получает конфликт. Мутации данных и audit транзакционны; ожидаемые отказы возвращают безопасный code, чтобы audit не откатывался вместе с API error. Нет публичного import endpoint.

Модель доверия:

| Source | Значение | Live право |
|---|---|---|
| SYNTHETIC_TEST | Проверенная целостность synthetic fixture | никогда |
| MANUAL_UNVERIFIED | Неподтверждённая запись | никогда |
| VERIFIED_OPERATOR | Проверка оператором, не подтверждение Meta | никогда |
| OFFICIAL_PROVIDER | Зарезервированная модель проверяемого происхождения | в9В import запрещён SQL CHECK и backend boundary |

wa_provider_records хранит org/integration/resource, source identity/type, verification method/time, expiration, policy version, payload/integrity digests, approval_status, revoked_at, optimistic version. Целостность проверяется по текущей строке ресурса, не по переданному UI approved flag. Изменение resource row без нового provisioning инвалидирует допуск. Digest доказывает целостность локального состояния, **не** подпись Meta. Runtime не имеет прямого DML к records/ресурсам; привилегированный migration/control-plane администратор остаётся отдельной доверенной границей, а не защитой от владельца PostgreSQL.

`wa_provenance_valid` вызывается существующими SQL admission/template validation непосредственно перед dispatch. Оно требует matching digest, VERIFIED, неотозванный и действующий ещё6s synthetic record и explicit simulation. Для live всегда false до отдельной реализации проверяемого official source. Legacy raw verified/source= META_VERIFIED недостаточно. Тарифы и шаблоны не разблокируются простой записью в UI/обычной runtime SQL.

Новые tenant-owned tables00012: wa_operator_grants, wa_provider_records, wa_reconciliation_evidence; FORCE RLS, tenant-safe FK, unique/index, fixed search_path у SECURITY DEFINER. Всего40 tenant tables. Новых global data tables нет; используется прежняя закрытая wa_function_backup для exact rollback. Down отказывается терять финансовые/operator/provider/evidence данные. Старые00001–00011 сохранены.

Оператор назначается только control-plane/migration role: tenant+user, expiry, revocation, opaque `control:<change_id>`. Нет API самоназначения/повышения бюджета/включения live. Изменение grant аудируется. Store owner и staff не становятся оператором вследствие membership. Назначенный оператор может работать без store membership, но только через явный tenant context, grant и узкие функции; RLS не отключается.
