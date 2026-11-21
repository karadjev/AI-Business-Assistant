# Universal Migration Plan

Статус: **ПЛАН**, SQL files не созданы и не применялись. Фактическая последняя migration при аудите — **00017**. Приведённые ниже M-* — dependency identifiers, **не зарезервированные Goose номера**: Stage 2 remediation может добавить новые migrations раньше.

## 1. Gate и правила перехода

G0: завершить Stage 2.1 PostgreSQL Up/Down/RLS/queue/channel regressions и проверить G0-CATALOG routing finding. До этого не менять пересекающиеся catalog/orders/agent/channel/tenancy/BFF components и не писать конфликтующий DDL. Документация безопасно готовится независимо.

После G0 каждая миграция получает фактический следующий номер, Up/Down, runtime schema-readiness update и isolated safety-marked roundtrip. Existing migrations/исторические snapshots не редактируются. Нет production execution или разрешения на downloads этим документом.

Expand → lossless backfill → validate → enable small synthetic cohort → wider opt-in. Contract/column removal не входит в первый Stage 3 rollout. Любой destructive Down с данными должен отказываться, а не стирать их ради PASS.

## 2. Proposed migration packages

| ID | Proposed data change | Зависимости / scope |
| --- | --- | --- |
| M-BP | Additive business_model/modes/profile revision в existing store profile, capability config, business_locations/hours/exceptions, versioned business_policies | G0; organization ID/name/timezone и legacy business_type сохраняются |
| M-IP | Industry bindings/overrides/version history, tenant attribute definitions | M-BP; approved public registry отдельно от tenant rows |
| M-ATTR | Product/variant typed attribute sidecars, media/link metadata; supported filter indexes | M-IP; products/variants/categories/inventory master IDs неизменны |
| M-CUST | Minimal customer extension по existing `(org,customer_id)`, verified identity links/aliases | G0; dry-run IDs, atomic anchor creation во всех channel/conversation writers до FK validation, без auto-merge |
| M-SVC | Services/offerings/resources/assignments/location links/availability rules | M-BP + M-IP; read-only catalog разрешён раньше booking writes |
| M-ORD | Versioned order/quote metadata, item kinds/typed references/constraints, quote_line_references | M-CUST + M-SVC для service refs; существующий order aggregate и v1 semantics сохраняются |
| M-SCHED | Holds/allocations/bookings и conflict/expiry/command idempotency indexes | M-ORD + M-SVC; stable resource locks/confirmation protocol |
| M-JOBS | При доказанной необходимости один typed tenant domain outbox + lease/claim helpers | После соответствующих domain entities; без fake channel integration или нового процесса |
| M-KNOW | Tenant knowledge source/publication/ACL/chunk metadata и context manifests | M-BP/M-IP; no vector-store dependency by default |
| M-LIST | Optional property listings/evidence/viewing references | M-BP + M-ORD; не отраслевые копии product tables |
| M-CUSTOM | Custom specification/estimate revisions и workflow metadata | M-ORD; manufacturing/repair use общие requests/quotes |
| M-PRICE | Wholesale price lists/tiers/customer eligibility | M-ORD + M-ATTR; no arbitrary discount attribute |
| M-UNIT / M-MONEY | Exact fractional ledger/quotes/orders и currency exponent contracts | Отдельный высокий риск; до end-to-end proofs feature flags off |
| M-ONBOARD | Versioned onboarding steps и legacy summary projection | M-BP/IP; ветки только реализованных modules |

Это пакеты изменений, а не требование выполнить весь объём за один release. Внутри пакета допустимо несколько small migrations после schema/locking review. Новые extensions/dependencies требуют отдельного обоснования и approval.

## 3. Backfill и source of truth

| Legacy data | Migration rule | Проверка |
| --- | --- | --- |
| organizations.business_type=electronics | Сохранить v1 значение; seed новый trade + electronics binding с recorded origin | Не выводить отрасль всех будущих tenants из этого CHECK |
| store_profiles | Канонический профиль остаётся здесь; description/conditions сохраняются как legacy revision | Counts, versions, exact text/digests |
| opening_hours/address | Legacy strings сохраняются; proposed structured mapping требует owner review | Не превращать нераспознанные часы в 24/7 |
| Product string attributes | Preserve map; explicit typed mapping preview, unknown/invalid остаются legacy | Roundtrip и conflict report, не silent coercion |
| Products/categories/variants/SKU | Existing IDs и uniqueness/source ownership неизменны | Counts, FK/orphans, prices/stock movements invariant |
| Customer UUIDs | Создать extension anchors из distinct `(org,customer_id)` без смены ID | Composite references, no phone/username merge |
| Orders/quotes/items/events | contract_version=1, product kind; snapshots/consent/audit не переписываются | History digest, totals/currency/reserves, v1 replay |
| Conversations/messages/channel identities | Сохранить IDs/history/leases; source mapping не перекодировать | Per-channel principal checks, retention и bounded reads |
| Telegram/WhatsApp/Instagram credentials/jobs | Не backfill в новый business profile/context | Шифрование, proof scopes, no secrets in manifests |

Для каждого поля зафиксировать единственный writer. V1 profile endpoint и V2 facade используют один write service/transaction; не поддерживать две независимо обновляемые копии. Typed sidecar не становится источником stock/price.

Backfill batches имеют `(tenant,migration_task,source_revision)` idempotency, checkpoints, bounded locks и repeatable dry-run. Unknown result не означает success. System migration worker не получает безусловный tenant-wide application grant: trusted lease/capability, минимальные resource IDs и audit. Goose DDL остаётся у migration role.

## 4. Constraints / RLS rollout

Новые tables сразу ENABLE/FORCE RLS, no PUBLIC grants; runtime role не owner и не BYPASSRLS. `organization_id NOT NULL`, composite unique/FKs для всех parent references. Product/variant pair проверяется совместно, не только отдельными FK. Typed item references — discriminator/exactly-one validation.

Сначала inventory orphan/duplicate check в disposable copies, затем backfill, затем validation/NOT NULL/unique/FK enforcement с оценкой locks. Не менять ownership/BYPASSRLS ради backfill. SECURITY DEFINER функции имеют fixed search_path, minimal EXECUTE ACL и свои tenant/principal/version checks.

Readiness ожидает согласованную schema version; обновляются API/worker/test harness вместе. Новая форма не публикуется до compatible readers/handlers. Нельзя включить service line, пока legacy confirm продолжает без dispatch читать каждый item как product variant.

## 5. Compatibility и rollback

- Existing electronics tenants получают product-sales defaults, поэтому их прежние операции сохраняются. New service-only tenants используют v2 workspace; v1 routes не обходят effective capability admission. Legacy electronics marker не считается фактической специализацией.
- V1 DTO types и limits неизменны. Нельзя заменить `attributes: string map` typed objects в том же endpoint или вернуть service item в product-only client.
- V2 operations используют существующий aggregate с explicit format/type dispatch; immutable quotes сохраняют свою revision.
- Feature disable останавливает новые writes, не прячет/стирает историю. Uncommitted DB changes rollback атомарны; после внешнего или stock/schedule effect нужен audited compensation, не blind Down/retry.
- Snapshot/digest/row-count before/after, catalog ownership, reservation sums, booking allocations и idempotency records сохраняются как evidence.
- Populated rollback refuse ожидаем; после первого v2 order может потребоваться forward repair/compatibility view. Это должно быть описано до активации.
- Multi-currency/fractional rollout не выполняется в фоне «заодно»: отдельно versioned arithmetic, UI/import/tools/contracts/ledger и regression.

## 6. Verification и запуск

На disposable `aba_test` с safety marker: Up, repeated/concurrent Up, empty Down/reapply, populated Down refusal и untouched-data fingerprints. Далее A/B/C I/U/D/RLS/pool reuse, handler/worker/queue concurrency, electronics v1 + восемь profiles и три mock channels.

Никаких tests против working database. Не переносить исторические Go PASS на новую schema. Подготовленные offline runners и quality routing из `AI_DEV_SYSTEM_INSTALL_PLAN.md` сохраняются; отсутствующий runner → BLOCKED. E01 не закрывается новым записанным build proof или broad exclusions.
