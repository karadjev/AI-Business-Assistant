# Stage12B — Final offline deployment verification

11 ноября 2026. **PACKAGE VERIFIED, STAGING BLOCKED. CUSTOMER PILOT NO-GO.** Локальная QA не является проверкой реального Ubuntu VPS. Evidence: `.local/stage12b-20261111/INDEX.md`; все raw exits/failure logs хранятся отдельно от transfer candidate.

## Docker и входные артефакты

До запуска установленного Desktop daemon socket отсутствовал; CLI29.9.2/Compose5.6.1/context desktop-linux были доступны. Sandbox не позволил status plugin записать host log. Разрешённый штатный `/usr/bin/open -a Docker` выполнен без sudo/переустановки/settings edits; затем daemon29.9.2 доступен через local Unix socket. Причина предшествующего останова Desktop не устанавливалась. Engine linux/amd64,12 CPUs,8,321,814,528 bytes allocated RAM. Snapshot VM MemAvailable5,565,996KiB; VM filesystem available164,661,640KiB, Mac host имел около64GiB free. Guest sparse-disk availability не заменяет host free-space monitoring. Это снимки, не target4CPU capacity.

Проверены оба фактически существующих input:

- Запрошенный `.local/recovery12a-20261110/release/`: manifest SHA256 `3d782c9fd9ffd0ec91c1e5b851b0055c424a343d1847d983c465546730554901`,2380 files,OCI graph/архив PASS.
- Последний `.local/vps-readiness-20261110/release/`: SHA256 `b2a0387dbdc8febbfc0f81af57b84d4dd39d682335ef3c9e5cb0a1b5b93b2af1`,2395 files,OCI graph/архив PASS. Старые PASS от10 ноября не переобъявлены новыми.

Integrity-valid входы не прошли новое требование startup: существующие API/worker реально продолжали работу без `LLM_KILL_SWITCH` и readiness exit0. Это отсутствие явного admission, не факт включения live: остальные запреты/defaults/budgets сохранились. Оба legacy targets отклоняются current validator без admission v1.

## Минимальные причинные исправления

1. Только deployment `runtime-entrypoint` проверяет14 exact control values и HTTPS PUBLIC_ORIGIN до чтения secrets/exec API/worker. Все15 omissions ×2 дали30 реальных exit1/refusal. Migrate/health probes сохраняют свои контракты. Business API/worker/migrate/healthcheck binaries byte-identical свежей Go-сборке; HTTP/business logic, migrations/RLS/timeout не менялись.
2. Подтверждена запись `common.cpython-314.pyc` дочерним Python: родительский `-B` не наследовался. Hash изменился, guard остановил roll-forward **до state change**. First-party CLI теперь отключают bytecode до local imports, child получает PYTHONDONTWRITEBYTECODE. Validator явно отклоняет cache даже при совпадающем hash. Default `python3 --help` regression воспроизвела запись до fix и неизменность после. Загрязнённая QA copy целиком сохранена в `preserved-bytecode-contaminated-release/`; новый clean derived candidate использует те же runtime assets, без guard exclusions или rebaseline polluted tree.

С нуля package не строился: переиспользованы существующие configs/static assets и4 из5 OCI image IDs, заменён один API/worker image только новым launcher. Дополнены необходимые host scripts/minimum policy и report. Исходные packages/Next proof не изменены.

## Проверяемые image digests

|Component|Eligible linux/amd64 image ID|
|---|---|
|API/worker/migrator|`sha256:7d76a4d9bc7a51b7cc71f76bde13389fb525a6503a8881480e5d3140c24cadb2`|
|Next/BFF|`sha256:0a15ca0de37a5a0cb14225dd081226022d06cbd01721c7e1745ef43a63b43509`|
|PostgreSQL|`sha256:54451ecb8ab38c24c3ec123f2fd501303a3a1856a5c66e98cecf2460d5e1e9d7`|
|HTTPS proxy|`sha256:814a8e88df978ade80e584cc5b333144b9372a8e3c98872d07137dbf3b44d0e4`|
|Local Mailpit|`sha256:81370195cd4a0eab9604d17c2617a7525b0486f9365555253b6c5376c6350f1a`|

Transfer candidate: `.local/stage12b-20261111/verified-release/`. Финальный manifest SHA256 и archive checksum — в INDEX/final verification JSON. QA secrets/dumps/GO records/contaminated/rollback copies переносить нельзя. IDs — OCI index/manifest IDs текущего Engine; native Ubuntu load/inspect compatibility ещё NOT RUN.

## Реально выполненные gates

|Check|Current result|Local evidence stem|
|---|---|---|
|Go tests/vet/gofmt/build|PASS|`go`|
|Go race|PASS|`race`|
|Fresh PG integration/race/migrations/RLS|PASS73.000s;3 subprocess-only helpers SKIP, parent cases executed|`integration`|
|Frontend lint/TS/OpenAPI|PASS|`web-check`|
|Fresh exclusive Linux Next build/guard|PASS; package Next image reused separately|`web-build`|
|Fresh Browser|PASS9/9,829.751s,0 skipped/unexpected/flaky|`browser-full`|
|Accumulated Catalog|PASS2/2,28.258s/54.753s|`catalog-accum-1/2`|
|Strict SQL75 calls,4workers,pool5,1s SQL deadline,PG1.5CPU/2GiB|PASS75/75,0 errors/canceled acquires,JIT restored on|`sql-regression-final`|
|Deployment Python/Node|PASS24 Linux nonroot Python tests /9 Node tests|`deployment-immutability-linux`, `deployment-node-linux`|
|Runtime security omissions|PASS30/30 actual refused starts; old two-case FAIL preserved|`runtime-omissions-after.json`, `runtime-omissions-before.json`|
|Existing image load/platform/native Sharp|PASS5 OCI IDs/graphs/layers,PNG|`artifacts.log`, `artifact-verification.json`|
|Startup/migrations/health/private TLS/auth|PASS six services,schema16,45 FORCE RLS,restricted aba_app,anonymous0|`patched-update`, `reports/patched-update.json`|
|Exact production-image tenant/profile/XLSX|PASS local email verify,Secure/HttpOnly,CSRF,preview without mutation,unconfirmed rejection,confirm/replay6 products,logout revocation|`production-auth-owner-smoke`|
|Restart/persistence|PASS count/digest equality5 tables;6007 variants/inventory,238 imports,1 user,2 organizations|`restart-persistence.json`|
|Backup/job/real separate restore|PASS schema/owners/effective ACL/policies/definers/45 FORCE RLS/data equality|`backup-job`, `restore-final`, `restore-final-security-equivalence.json`, `restore-persistence.json`|
|Restored tenant isolation/HTTPS|PASS foreign0/anonymous0/own6001/imports237; restored serving|`restored-tenant-isolation`, `restored-https-smoke`|
|Nonempty restore|Expected refusal exit2,target retained|`nonempty-restore-refusal`|
|Plain Python actual update after immutability fix|PASS backup/migrate/readiness/HTTPS,no cache/all hashes unchanged|`plain-python-immutable-update`, `release-immutability-after-update.json`|
|Hardened rollback/roll-forward|PASS; API/worker7d76… retained,old/new frontend served proofs match,no cache/hash drift|`immutable-*`, `rollback-immutable-summary.json`|
|Original E01 tree|Expected startup refusal42; original proof/inode/timestamps unchanged|`original-tainted-image-refusal`, `artifact-verification.json`|
|Known synthetic-secret scan|PASS0 matches9 actual values,0 unexpected .env/run-secrets files; not universal scanner|`artifact-verification.json`|
|Native VPS/SSH/firewall/advisory downloads/install scheduler|NOT RUN|scope/authorization below|

Telegram/WhatsApp mocks, consent, order/staff approval and inventory concurrency прошли prepared integration/Browser stack. Его API/worker bytes равны deployed business binaries (`binary-equivalence.json`). Exact production-mode image отдельно проверен TLS/auth/onboarding/XLSX/persistence/restore. Production channel processing остаётся намеренно выключенным privacy/config gate; этот report не объявляет его включённым ради mock PASS и не разрешает реальные webhooks.

Новый SQL run без tracer/server hook/plan override: p951073.061ms/max1138.562ms — весь HTTP operation из нескольких SQL,не длительность одного statement. Причины150 исторических отмен и2 metadata cancellations не переатрибутированы.

QA restore сначала был отклонён до pg_restore из-за старого API digest в restore env. После исправления только отдельного QA env реальное восстановление PASS. Harness path/log/count errors и first FAIL сохранены; они не считаются application defects и не маскируются retry. Историческая metadata SQL cancellation не объясняется успешной поздней проверкой.

## Остаточные риски и условия

|Risk|Classification|Evidence/operating condition|
|---|---|---|
|Legacy runtime omission / Python cache pollution|BLOCKS PRIVATE STAGING для старых/загрязнённых targets; исправленные targets требуют всех новых gates|Два real accepted starts и hash mismatch доказаны;30 refusals и plain-Python immutable update проверяют fix. Не downgrade к legacy API|
|E01 writer unknown,OPEN|MONITORING REQUIRED для approved synthetic-only private scope; BLOCKS CUSTOMER PILOT до отдельного risk decision|Clean Linux input/immutable image,RO runtime/startup guard,old polluted refusal42. При drift STOP/capture,no rebaseline; причина не доказана|
|Historical150 SQL attribution +2 metadata cancels,OPEN|MONITORING REQUIRED в bounded private workload; BLOCKS CUSTOMER PILOT до review/SLA/causal resolution|Сохранён historical JIT proof; новый regression не заменяет incident-time plan. SQL timeout не повышать,auto write retry запрещён|
|Image provenance/advisory approval отсутствует|BLOCKS PRIVATE STAGING по действующему launch gate|Cached digests проверены,но внешние advisory/signature review NOT RUN. Не объявлять отсутствие CRITICAL/HIGH без evidence|
|Native Ubuntu/bootstrap/image-store/UID/SSH/firewall/TLS/private access verification|BLOCKS PRIVATE STAGING|Docker Desktop не доказывает VPS controls. Нужны конкретный сервер,approved offline bundle,локальный socket и native load/health/ownership checks|
|Owner exact-manifest server GO/risk acceptance отсутствует|BLOCKS PRIVATE STAGING|Созданы только records текущего разрешения на isolated local QA; они не authorise VPS/private-staging scope|
|Plaintext QA backup,real-data encryption/custody undecided|BLOCKS CUSTOMER PILOT; MONITORING REQUIRED для synthetic-only private|Archives0600/directories0700,real restore PASS; encryption NOT RUN,никакого real data/automatic retention deletion|
|Target capacity/DB lock-transaction tuning/worker liveness/backup scheduling|MONITORING REQUIRED private; BLOCKS CUSTOMER PILOT до operational evidence|Engine12CPU,PG1.5CPU/2GiB; native4CPU/peak/SLA NOT RUN. Server lock/txn/idle settings0 наблюдались; app SQL2s/HTTP5s/pool5 unchanged. Worker health=liveness|
|Formal10E/privacy/providers/templates/ownership/budgets|BLOCKS CUSTOMER PILOT|Автоматические mock tests не являются human rehearsal/provider approval;live/paid OFF,LLM/WA0 RUB|

Классификация не понижает severity подтверждённых CRITICAL/HIGH. Security verdict: **CONDITIONAL**, не Security PASS. Отдельные platform/owner условия обязательны даже при исправленных локальных defects; потому итогB, а неA. Любой новый confirmed CRITICAL/HIGH или artifact/tenant/financial guard FAIL блокирует private staging.

## VPS и действия владельца

Целевая база: Ubuntu24.04 LTS amd64,4vCPU/8GB/80GB SSD,local Docker Engine/Compose,Python3.12. Доказанный меньший минимум и production capacity не установлены. Начальный CPU/RAM/disk проверены локально; native firewall/SSH/UID и sparse-disk/backup storage ещё требуют проверки.

1. Предоставить IP/домен и закрытый access plan,operator/SSH allowlist,private TLS trust; реальные keys/passwords не передавать в чат.
2. Согласовать provenance/advisories и offline Engine/Compose bundle; команды bootstrap в `VPS_INSTALLATION.md` подготовлены,но не выполнялись на сервере. Не менять текущий Desktop/global settings.
3. Провести owner review E01/SQL/privacy/backup custody и подписать operational exact-manifest GO только после prerequisites; QA GO records не использовать на VPS.
4. Перед первым магазином — native private rehearsal,formal10E,Security/provider/ownership/template/pricing/live/budget approvals. Бюджеты не повышались,retention audit/receipts не менялся.

Следующая **read-only** команда после покупки/первоначальной настройки VPS,доставки доверенного release,offline image load/inspect и provisioning env/secrets (выполняет владелец после отдельного server authorization):

```sh
python3 /srv/aba/current/scripts/deployment/deploy.py bootstrap-check \
  --release /srv/aba/current --env /srv/aba/staging/environment \
  --project aba-private-staging --storage-root /srv/aba/staging \
  --backup-dir /srv/aba/staging/backups --tls-ca /srv/aba/staging/proxy-ca.pem \
  --report /srv/aba/staging/reports/stage12b-bootstrap-001.json
```

Затем `control.py validate`,а install — отдельная команда с private-staging GO,как в installation/runbook. Bootstrap-check не стартует сервисы; пишет только private report/temporary storage probe. При несовпадении IDs или любом guard FAIL — STOP,не править baseline. Подключения к VPS,public deployment,sudo,commit/push,external downloads/live API и удаления evidence не было.
