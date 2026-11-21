# VPS launch gate

Stage12C,11 ноября: текущие host controls и ограничения — в [pre-VPS report](STAGE_12C_PRE_VPS_SECURITY_REPORT.md). Alternative entrypoint/command и remote Docker endpoint должны отклоняться до Compose. Все state-changing вызовы control.py требуют private GO с exact manifest/project/action, scope и database_schema; start дополнительно migrations_approved:true, rollback — schema_compatible:true. Local QA требует явного --local-qa и project aba-private-qa-*. Это не server GO. Supply chain/advisory gates остаются незавершёнными.

Stage12B,11 ноября: обязательны admission v1 и реальный negative startup matrix API/worker (15 параметров ×2), а также immutable plain-Python operation без bytecode в release. Legacy manifests без `runtime_security_admission_version:1` неприемлемы для install/update/rollback в current tools. Тег — заявление builder, не замена matrix/источникам/owner review. Исходные releases и polluted QA copy не переписываются.

10 ноября 2026. **CUSTOMER PILOT NO-GO.** Private staging и public production — разные решения. Текущая разработка/QA не даёт server GO, live approval или Security PASS.

До server GO нужны: актуальные PASS prepared Go/race/PG/RLS/frontend/build/Browser/accumulated gates; image build/load/inspect/readiness/smoke/restart persistence/restore evidence; сохранённый guard refusal polluted artifact; image provenance/advisory/privacy review; решение по E01/остаточным SQL рискам; Ubuntu/SSH/firewall/TLS/backup custody/capacity verification. Более поздний PASS не стирает ранний FAIL.

Новый deploy.py требует отдельный private mode0600 GO JSON, привязанный к reviewed manifest:

```json
{
  "approved_by_owner": false,
  "action": "install",
  "scope": "private-staging",
  "project": "aba-private-staging",
  "manifest_sha256": "EXACT_REVIEWED_MANIFEST_SHA256",
  "database_schema": 16,
  "migrations_approved": false
}
```

Placeholder/false должны отклоняться. Только владелец разрешает true/exact hash; агент не выдаёт server GO автоматически. Для update — action update и новая reviewed версия, backup/restore approval и миграционный план. Migration approval требуется даже при первом install; неизвестная/необратимая новая миграция требует отдельного review. Запись GO — operational evidence, не криптографическая подпись.

Изолированная локальная QA имеет scope `isolated-local-qa`, project `aba-private-qa-*`, явный `--local-qa`, synthetic-only credentials/data и разрешение текущего запроса на QA. Такой протокол не переносится на VPS и не является private-staging GO. control.py stop/rollback используют action stop/rollback с обязательными scope/database_schema и private GO file; rollback дополнительно schema_compatible:true. Не подменять один action/scope другим.

`deploy.py` выводит технический PASS только для выполненных этапов и всегда `customer_pilot:NO-GO`. Для запуска первого магазина дополнительно требуются формальная10E human rehearsal обоих каналов, consent/staff/order/financial controls, privacy/retention/operator ownership, подтверждённые provider/templates/ownership/pricing и отдельные live/budget approvals. Текущие budgets остаются0 RUB.
