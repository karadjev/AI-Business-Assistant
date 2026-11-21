# VPS deployment readiness

Stage12C, финализация12 ноября: актуальный review — [STAGE_12C_PRE_VPS_SECURITY_REPORT.md](STAGE_12C_PRE_VPS_SECURITY_REPORT.md), PRE-VPS: SECURITY / SUPPLY CHAIN BLOCKED, CUSTOMER PILOT NO-GO. Current candidate содержит исправленные host admission/GO scripts и прежние verified runtime IDs. Historical gates ниже сохранены; authenticated provenance/current advisory review и native Ubuntu остаются незавершёнными.

Актуальный Stage12B review11 ноября: `STAGE_12B_FINAL_DEPLOYMENT_VERIFICATION.md`. Выявлены и исправлены два конкретных deployment defects: отсутствовал runtime отказ при missing security flags и Python child загрязнял release bytecode. Исторический package от10 ноября сохранён, но не удовлетворяет новым обязательным admission/immutability gates. Текущий eligible candidate и окончательные статусы приведены только в Stage12B report; Security CONDITIONAL, CUSTOMER PILOT NO-GO.

10 ноября 2026. **PACKAGE READY, DEPLOYMENT BLOCKED. CUSTOMER PILOT NO-GO.** Текущий evidence run: `.local/vps-readiness-20261110/`. Это локальная подготовка и изолированная QA, не Ubuntu/VPS certification. Финальные gate exits и provenance — в INDEX.md этого run.

## Фактическая новая репетиция

Docker стал доступен после запуска владельцем. Все builds/checks использовали cached images/modules, без pull/install/download. Новый Linux build volume `aba-vps12b-web-20261110`, отдельные DB/browser/private/restore projects, internal networks и собственные artifacts/results. Рабочие demo/другие проекты не останавливались и не использовались. Current transfer candidate: `.local/vps-readiness-20261110/release/`; QA secrets/dumps/reports/rollback-only candidate переносить нельзя.

|Проверка|Результат / evidence stem|
|---|---|
|Go tests/gofmt/vet/build|PASS `go`|
|Go race|PASS `race`|
|Fresh migrations/integration/race/RLS/tenant/finance/consent/staff/TG-WA|PASS88.883s `integration`; три subprocess-only helper SKIP по дизайну, parent cases выполнены|
|Frontend lint/TypeScript/OpenAPI|PASS `web-check`|
|Fresh isolated Linux Next build + record/verify|PASS `web-build`; BUILD_ID55_-2Dh8LU1Js_xkUr8Dh|
|Current Browser E2E|PASS9/9,825.463s,0 skipped/unexpected/flaky `browser-full`|
|Accumulated Catalog Browser|PASS2/2,40.138s/46.048s `catalog-accum-1/2`|
|Strict accumulated HTTP on normal PG|PASS75/75,4workers,pool5,SQL1s,PG1.5CPU/2GiB,0 errors/canceled acquires; `accumulated-sql-75`|
|Host deployment admission/ordering/failure tests|PASS21 Python tests, в том числе final isolated Linux nonroot runner `deployment-python-linux-final` (первый run20 сохранён)|
|Smoke/preflight/artifact Node regression|PASS9/9 в Linux nonroot runner `deployment-node-linux`|
|API/worker + minimal Node/native image build/export|PASS `package-build`; API/worker используют существующий общий static artifact image|
|Archive reload on current local Engine / immutable IDs|PASS5 IDs `local-image-archive-load`; не clean Ubuntu import certification|
|One-command install, empty private QA volume|PASS `deployment-install-final`, schema16/45 FORCE RLS, restricted role, six healthy services, verified HTTPS/auth smoke|
|Production-image auth/owner/shop smoke|PASS `production-auth-owner-smoke`: local mail verification, Secure/HttpOnly cookie,CSRF403,catalog200,logout revocation|
|Full graceful stop/restart and data persistence|PASS `restart-persistence.json`: counts/digests пяти таблиц совпадают;6001 variants/inventory,237 imports,1 user,2 organizations|
|Backup job / real isolated restore|PASS `backup-job`, `restore`; dump TOC/checksum, owners/effective ACL/policies/definers/FORCE RLS/data digests match; `restore-final-security-equivalence.json`|
|Restored tenant fencing / runtime|PASS foreign0/anonymous0/own6001/imports237, restored HTTPS/readiness; `restored-tenant-isolation`, `restored-https-smoke`|
|Restore into nonempty DB|Ожидаемый отказ exit2 `nonempty-restore-refusal`; данные не перезаписаны|
|Same-version update with pre-update backup|PASS `deployment-update`|
|Previous-image rollback / current-image roll-forward, schema16|PASS `artifact-version-rollback/rollforward`, health/served proof; old/new BUILD_ID различаются, migration down не выполнялся|
|Original polluted artifact / native Sharp|Ожидаемый image-start refusal42, original proof/inodes/timestamps unchanged; PNG PASS `native-sharp`|
|Archive integrity / known synthetic secrets|PASS5 OCI graphs/configs/layers and file hashes;0 matches9 actual QA secret values,0 unexpected .env/run-secrets files; **не универсальный secret scanner**|
|Native Ubuntu/VPS/SSH/firewall/production capacity/advisories|NOT RUN; server/download approval отсутствует|
|Systemd timer installation/scheduling|NOT RUN; example units подготовлены, backup job проверен вручную|

SQL p951697.607ms/max2198.423ms — **весь HTTP operation из нескольких SQL**, не превышение одного statement deadline1000ms. Использован private probe с byte-identical текущими internal Go files, без client tracer/server hook/JIT или plan overrides. Global connection JIT после транзакций restored on. Это acceptance regression, не новая causal attribution исторических отмен.

Первый новый install **FAIL** на `https_auth_smoke`: helper при `node -e` молча не исполнял main. Regression воспроизвела проблему, исправление `module.parent` прошло Node test и новый install на другом пустом volume. Первый volume/report сохранён. Дополнительно negative test обнаружил, что coordinator принимал unhealthy Mailpit; admission исправлен, фактические rollback/roll-forward/health PASS. Final21 Python tests дополнительно проверяют разрешение mixed Docker context/host: coordinator закрепляет проверенный Unix endpoint только в своём процессе/children. На локальном CLI probe HOST имел приоритет; фактического remote обращения/инцидента не было. Новая проверка — защита от неоднозначности CLI, а не объявленный доказанным production incident. Initial helper/syntax FAIL logs сохранены отдельно. Backup дополнен TOC validation, без заявления о доказанной порче старых backup.

## Измеренные ресурсы

Один local post-restart/health snapshot шести production-image services: **274.093MiB RAM**, суммарно **10.98% одного CPU ≈0.1098 core**. Это не peak/длительный average и не first-shop capacity. Database17,831,615 bytes, весь PG volume `du`75,788,288 bytes; image archive244,912,640 bytes. Сумма logical image sizes874,733,623 bytes включает shared layers и не является unique Engine disk usage. Exact per-service stats: `resource-measurements.json`.

Тестовый Engine имел12 CPUs/8,321,810,432 bytes RAM; строгий PG ограничен1.5CPU/2GiB. Native VPS4CPU не проверялся. Минимальная доказанная VPS-конфигурация **не установлена**; целевая/рекомендуемая база private rehearsal остаётся4vCPU/8GB/80GB, уменьшать её по одному idle snapshot нельзя.

## Состав и оставшиеся границы

До начала уже были artifact Dockerfiles, private production-mode Compose, TLS proxy/PG HBA, restricted bootstrap/migrations, runtime secret loader, Next integrity guards, backup/restore helpers, пакет с пятью immutable images и исторические runtime gates от3 ноября. Их не заменяют новыми параллельными сервисами/каталогом/identity.

Добавлены coordinator `scripts/deployment/deploy.py`, private HTTPS smoke, backup job entrypoint и неактивные systemd examples. Одна install/update command проверяет Engine/resources/secrets/storage/config/images/GO, выполняет migration/start/wait, проверяет schema/FORCE RLS/runtime role/anonymous isolation и HTTPS/login/auth401/CSRF403, пишет private JSON без values. Update предваряет миграцию проверенным backup. Первый install отказывается использовать существующий volume; failure не удаляет данные и не делает автоматический rollback. Backup дополнен TOC validation и streaming checksum.

Основные команды и границы: [installation](VPS_INSTALLATION.md), [operations](VPS_OPERATIONS.md), [security](VPS_SECURITY.md), [backup/recovery](VPS_BACKUP_RECOVERY.md), [launch gate](VPS_LAUNCH_GATE.md). Packager включает эти документы/scripts в новый hashed release. Обычные source-install Dockerfiles сохранены; offline маршрут использует существующие минимальные artifact images, без установки dependencies.

Точные текущие PASS/FAIL/BLOCKED/NOT RUN не выводить из намерений scripts. Проверки выполняются в новых QA projects/volumes с cached dependencies и internal networks. Runtime tests обоих каналов выполняются prepared mock runner; production package по-прежнему запрещает channel processing keys/live. Полный security/application verdict не следует из технического smoke.

E01 **OPEN / ROOT CAUSE NOT PROVEN**. Исторический aggregate JIT механизм доказан только в controlled reproduction; первые150 отмен не имеют полной incident attribution, две last-import metadata cancellation остаются **OPEN**. Ни новые успешные повторения, ни Linux images не закрывают эти границы. Timeout/guard/RLS не ослабляются.

VPS baseline/recommended для private rehearsal: Ubuntu24.04 amd64,4vCPU/8GB/80GB SSD. Меньший минимум не доказан. Config caps — PG1.5CPU/2GiB, API/worker по0.75CPU/512MiB, web1CPU/1GiB, proxy/mailpit по0.25CPU/128MiB; сумма caps4.5CPU/4.25GiB, transient migrator ещё0.75CPU/512MiB. Caps не являются фактическим usage. Реальные CPU/RAM/I/O/диск сообщаются только по project-scoped snapshot, размеру QA и archive; target-VPS capacity пока NOT RUN.

Connection pool5 на API/worker, runtime DB statement timeout2s, HTTP context5s; накопительный strict SQL gate1s. Server transaction/lock/idle-in-transaction settings coordinator наблюдает, не повышает/меняет. Отдельного application transaction/lock budget сверх context/statement текущий code не задаёт; это явно не считается подтверждённой production tuning. Зафиксировать фактические server values в QA report и до VPS review.

Остаются server-only работы: согласованные OS/Engine installation, image advisories/provenance, SSH/firewall/private domain/TLS trust, encrypted backup custody, target load/rollback verification, E01/SQL residual-risk decision и owner GO. Public ports/live/paid providers/real shops в эту подготовку не входят.
