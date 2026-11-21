# Production readiness gates — 2026-11-17

Область: текущий source и локальные disposable/synthetic окружения. Evidence root: `.local/production-readiness-20261117/` (далее `E/`). PASS ограничен названным сценарием. Предыдущие FAIL сохранены; наличие успешного exit code у сканера не означает отсутствия findings.

|Раздел|Предыдущий результат|Текущий результат|Evidence|Оставшийся риск / необходимое действие|
|---|---|---|---|---|
|Frontend|Исторический UX PASS; security warning OPEN|PASS regression; security scope отдельно|E/frontend-e2e-retry.json: 9; frontend-updated-go.json: 3; usability-results.json: 94; dark results: 20|VoiceOver/axe NOT RUN; custom automation не полный WCAG audit|
|Backend|Старый runtime 1.26.2; producer mismatch|PASS unit/race/vet/build на новых locks|E/go-final/summary.json; build.log; go-repro/verify.exit|govulncheck BLOCKED; binary tests не OCI release admission|
|PostgreSQL|Исторические миграции/RLS PASS, SQL cancellations OPEN|PASS schema16/migration/refusal/role regressions; FAIL под конкурирующей нагрузкой сохранён|E/integration-final/race-integration.log; integration-serial/race-integration.log|Один serial PASS не закрывает причинно необъяснённый FAIL|
|Multi-tenant|Исторические 3 shops × 2 channels|PASS tested HTTP/SQL/worker scopes; 3 populated tenants restore checks|E/integration-serial/race-integration.log; database-optimized/status.json|Не доказательство всех возможных tenant paths; negative tests обязательны на release|
|Authentication|Исторический PASS|PASS login/logout/session/CSRF/rate-limit tested paths|E/go-final/unit.log; integration-serial/race-integration.log; frontend-updated-go.json|Production TLS/cookie end-to-end NOT RUN на новых images|
|Authorization|Исторический PASS|PASS owner/staff/member/foreign requests и direct SQL|Те же integration log и frontend-updated-go.json|Сохранить runtime role/permission admission; не полагаться на UI|
|Orders|Исторический real PG browser PASS|PASS consent/staff/reserve/idempotency/concurrency/rollback и 3 fresh browser tests|E/integration-serial/race-integration.log; frontend-updated-go.json|Production order data не использовались; release-bound rerun нужен|
|Catalog|Исторические scale/JIT fixes; cancellations OPEN|FAIL stability gate; serial suite PASS|E/integration-final/race-integration.log (page55 unavailable, lock observation); integration-serial/race-integration.log|P1: SQL flakiness и latency; timeout/750ms lock guards не ослаблены|
|AI|Mock safety/cost gates исторически PASS|PASS tested mock grounding/tools/approval/cost/crash gates|E/go-final/unit.log; integration-serial/race-integration.log|Provider/model-specific adversarial eval NOT RUN; LIVE OFF, budget 0|
|Telegram|Mock historical PASS|PASS synthetic webhook/auth/dedup/retry/unknown-delivery/worker tests|E/integration-serial/race-integration.log|Реальная отправка NOT RUN и не разрешена|
|WhatsApp|Mock financial/template gates historical PASS|PASS synthetic signature/routing/financial/concurrency/crash cases|E/integration-serial/race-integration.log|Реальные номера/provider verification NOT RUN; LIVE OFF, budget 0|
|Performance|Без подтверждённой DE-3 capacity|FAIL candidate read p95; 5/10/20/30 mixed profiles NOT RUN|E/integration-serial/race-integration.log: p95 828 ms sweep / 1276 ms concurrent|DE-3 не аудирован; не обещать число магазинов; p99/RPS/resource/queue time series не собраны|
|Backup/restore|Исторический release rehearsal, не новый release|PASS local DB dump/restore, 57 tables + RLS/ACL/policies, 54 cross-tenant negatives + 9 context-free reads|E/database-optimized/status.json; fingerprints.json; archive SHA256|Release backup workflow, secrets/off-host/full DR NOT RUN; RPO/RTO требуется согласовать|
|Monitoring|JSON logs и bounded Docker logs существуют|BLOCKED operational acceptance|Source middleware/logging/worker; DE3_STAGING_PREPARATION.md|Alerts delivery, queue lag, backup freshness, host time series NOT RUN|
|Supply chain|FAIL/UNVERIFIED|FAIL old inventories; partial source/binary verification PASS|E/security-final/; npm-signature-verification.json; legacy-image-reassessment/; trivy-db/|11 npm moderate; Go UNKNOWN OpenPGP finding; patched Node/OCI inputs и carrier origin unresolved|
|Release|Новые пять images NOT BUILT|BLOCKED|E/binary-provenance.json; d2-current-assessment.json; docs/TRUSTED_RELEASE_PROVENANCE.md|Пять бинарников ≠ пять OCI images. Нет нового manifest/digest set/attestation|
|VPS readiness|DE-3 только параметры тарифа|BLOCKED|DE3_STAGING_PREPARATION.md|Нужны новый IP/OS/key/fingerprint/recovery/read-only authorization; старый VPS не используется|
|Staging readiness|NO-GO|BLOCKED / NO-GO|Все release, host, SQL gates выше|Нет trusted release + проверенного нового host + scoped GO|
|Pilot readiness|NO-GO|BLOCKED / NO-GO|Все hard gates выше|Локальное demo не customer pilot; реальные данные/каналы не подключались|

## Checks с точным статусом

- **PASS:** TypeScript, ESLint, fresh Next 16.4.5 production build + artifact/served guard; 9 frontend E2E и ещё 3 после замены synthetic API/worker на новые Go binaries; 94 custom accessibility/responsive/theme/zoom checks и 20 dark route checks; 0 browser page/console errors в usability audit.
- **PASS:** Go 1.26.11 `go mod verify`, `go test -count=1 ./...`, `go test -race -count=1 ./...`, `go vet ./...`, build пяти cmd; API/worker повторная сборка побайтово совпала. `go list -deps` не содержит OpenPGP.
- **PASS последнего отдельного прогона:** Go/PostgreSQL `go test -race -tags=integration -count=1 -v ./tests/integration` на новой БД; 184.425 s, 163 PASS entries. Crash subprocess helper top-level SKIP ожидаемы; их родительские crash cases выполнялись.
- **PASS:** 35 deployment/control unit tests, 3 readiness safety tests, Node artifact/preflight tests и 4 dashboard render tests. CI source расширен; GitHub workflow **NOT RUN**.
- **FAIL сохранены:** integration-final cancellation/lock failure; первые incomplete restore/fingerprint runs (host process exhaustion, затем семантически равные raw ACL); неправильный путь первого presentation test; первый browser invocation exit0 с пустым JSON — **не PASS**, заменён проверенным JSON run; неполный cache у первого Go dependency rerun; первый secret-template extension error. Полные логи остаются в E/.
- **BLOCKED:** govulncheck отсутствует, отдельная установка запрошена, не выполнена; свежие пять release images/OCI provenance/manifest; доступ к новому DE-3.
- **NOT RUN:** mixed load 5/10/20/30 shops, full release image startup/HTTPS/rollback/roll-forward, release-bound restore/off-host DR, full CI execution/nightly, VoiceOver и axe. Старые checks не подменяют их.

Hard NO-GO: untrusted artifact, unresolved exploitable critical vulnerability, cross-tenant/authorization/approval bypass, order corruption, exposed secrets, unverified release restore или unsafe public deployment. Cross-tenant leakage/approval bypass в выполненных сценариях не выявлены; полный production GO из этого не следует. **E01 OPEN; SQL cancellations OPEN.**
