# School21 — infrastructure and release checklist

Stage12E.3,2026-11-16. **BLOCKED — SSH trust prerequisites incomplete.** Владелец разрешил системную подготовку личного VPS176.57.214.93; отсутствуют независимо подтверждённый fingerprint, verified key login/recovery и network inventory. Подготовленные документы не являются выполненным hardening. PRIVATE STAGING NO-GO;CUSTOMER PILOT NO-GO. [Execution report](STAGE_12E3_EXECUTION_REPORT.md) содержит фактически выполненные локальные проверки и evidence hashes. SSH,package install,sudo,GitHub/deployment/live/budget changes не выполнялись.

## Подготовленные документы

- [Architecture/storage/ports/DNS](SCHOOL21_DEPLOYMENT_ARCHITECTURE.md).
- [Hardening/School21 access/rollback](SCHOOL21_SERVER_HARDENING.md).
- [Official Docker APT installation](SCHOOL21_DOCKER_INSTALLATION.md).
- [Preliminary resource limits](SCHOOL21_RESOURCE_PLAN.md).

Внутри таблиц каждой процедуры есть команда,expected result,verification и rollback. Placeholders — deliberate STOP conditions,не executable defaults. Все server commands NOT RUN. Текущий task разрешает системные изменения только после их prerequisites; отсутствие verified access/trust не обходить через опубликованный root-пароль или автоматическое принятие host key.

Текущая последовательность: A local preparation DONE → B independent SSH trust BLOCKED → C hardening NOT RUN → D Docker NOT RUN → E storage NOT RUN → F release transfer BLOCKED отдельно по12C.2/12C.3. Root password compromise требует emergency rotation через доверенную console и incident review; возврат к старому секрету запрещён. Firewall allowlists пока не определены;не брать один текущий IP за полный School21 administrative range. HTTPS/HTTP host ports остаются закрытыми до отдельного обоснования,существующий private Compose без published ports сохраняется.

## Gate sequence

|Gate / действие|Команда или review artifact|Acceptance|Rollback / текущий статус|
|---|---|---|---|
|G0 server audit/access/incident decision|Read-only collection из [12E.2](STAGE_12E2_SERVER_SECURITY_AUDIT.md)|SSH successes/failures рассмотрены,School21 identities/firewall/Zabbix inventory,console verified|Evidence сохранить;при неизвестном privileged success — incident response;BLOCKED|
|G1 School21 change approval|Reviewed hardening plan,owner+School21 admin CIDR/MFA/monitoring/recovery|Конкретные approved accounts/addresses/config candidates,window и rollback owner|Нет approval → не менять;BLOCKED|
|G2 hardening|Команды SCHOOL21_SERVER_HARDENING.md|Owner и School21 second-session login,SSH effective policy,firewall/monitoring continuity,updates/audit policy PASS|Вернуть только собственные changes через console по snapshot;NOT RUN|
|G3 Docker|SCHOOL21_DOCKER_INSTALLATION.md|Official APT key/origin/versions/size/hash admission,local Unix Engine,plugins/firewall/storage checks|Reviewed package/config rollback,без prune/purge;NOT RUN|
|G4 storage/secrets/TLS|SCHOOL21_DEPLOYMENT_ARCHITECTURE.md|Real shared path,same st_dev as DockerRootDir,private permissions,UID-specific secrets,CA/SAN/origin verified|Сохранить candidate,не менять active secrets/volume;NOT RUN|
|G5 trusted release|Новый approved revision+5OCI subjects+SBOM+fresh Trivy+provenance review|Нет unexplained blocking HIGH/CRITICAL,scope/coverage/freshness известны;independent attestation не подменена self declaration|Старые candidate archives не устанавливать;BLOCKED|
|G6 native preflight и owner GO|Команды ниже;exact new manifest/project/action/schema/migrations approval|Все gates PASS,accepted E01/SQL scope decision и operational owner;не fake GO|No GO → no install;BLOCKED|
|G7 deployment/recovery rehearsal|Existing coordinator,isolated restore,health/Browser/tenant/backup|Только отдельно разрешённое synthetic private staging;no public/live|При FAIL STOP,capture,no implicit destructive rollback;NOT RUN|

## Stage12B–12C.2: актуальные ограничения

|Тема|Подтверждённая история|Условие нового release|
|---|---|---|
|net/mail|CVE-2026-39820/GO-2026-4986 и CVE-2026-42499/GO-2026-4977;source теперь ограничивает length/non-ASCII до parser;go.mod/producer policy требуют1.26.11|Fresh Go tests/race/vet/build/targeted regressions +actual compiler metadata all5Go binaries+advisory/reachability scan;source fix не обновил старый OCI image|
|OCI provenance|Пять старых API,web,Postgres,proxy,Mailpit identities остаются unverified;нового trusted release нет|Approved immutable source revision,exact base/locks/tool materials,executed build logs/platform/config/index digests и SBOM связаны с новым manifest|
|Trivy findings|Stage12C.1 historical791 occurrences,339IDs,283holds;не exploit counts. Stage12C.2 patched binary scan ещё имел dependency findings|Fresh admitted DB/scans actual5images/source;Nov12 DB с NextUpdateNov13 stale,не считать clean|
|SBOM/native coverage|Есть historical Syft inventories,не new-release SBOM;runtime native coverage ограничено|Inventory из final filesystem,Go/npm/OS/native scope/limitations;govulncheck не заменён unit tests|
|Runtime admission|Исторически30actual startup refusals;15required values×API/worker,admission v1|Повторить matrix на новых images,required flags/entrypoint/GO/remote endpoint negatives,release integrity/no bytecode|
|E01|Writer cause OPEN;guard должен отказывать polluted artifact|No rebaseline/exclusions;проверить clean build/served proof,preserved original refusal,owner risk decision для private scope;customer gate остаётся|
|SQL|Historical150cancellations+2metadata attribution OPEN;поздний75/75PASS не causal proof|Сохранить SQL deadline/isolation,retest bounded synthetic workload,разбор/отдельное решение по remaining risk;без auto retry writes|

Приоритет источников: [12C.2](STAGE_12C2_SECURITY_REMEDIATION.md),[provenance](TRUSTED_RELEASE_PROVENANCE.md),[12C.1](STAGE_12C1_SUPPLY_CHAIN_VERIFICATION.md),затем исторические[12C](STAGE_12C_PRE_VPS_SECURITY_REPORT.md)/[12B](STAGE_12B_FINAL_DEPLOYMENT_VERIFICATION.md). Старый integrity-valid package НЕ eligible только из-за historical QA PASS. D2/fresh DB/govulncheck/download approvals не получены этим документом. Release rebuild — отдельная локальная работа,не установка уязвимого candidate на сервер ради теста.

## Точные будущие проверки и approved rollout interface

Ниже не authorization deployment. Только после G0–G5,доставки нового admitted release,approved images load и создания собственных private env/secrets/TLS. Выбрать **конкретный** `RELEASE_ID`,не mutable pointer для проверки. Пример env variable является локальным удобством shell,не global setting:

```sh
ABA_SCHOOL_RELEASE=/srv/ai-business-assistant/releases/RELEASE_ID
ABA_SCHOOL_SHARED=/srv/ai-business-assistant/shared
cd "$ABA_SCHOOL_RELEASE"
sha256sum -c images.tar.sha256
python3 "$ABA_SCHOOL_RELEASE/scripts/deployment/control.py" validate \
  --release "$ABA_SCHOOL_RELEASE" --env "$ABA_SCHOOL_SHARED/environment" --project aba-private-staging
python3 "$ABA_SCHOOL_RELEASE/scripts/deployment/deploy.py" bootstrap-check \
  --release "$ABA_SCHOOL_RELEASE" --env "$ABA_SCHOOL_SHARED/environment" --project aba-private-staging \
  --storage-root "$ABA_SCHOOL_SHARED" --backup-dir "$ABA_SCHOOL_SHARED/backups" \
  --tls-ca "$ABA_SCHOOL_SHARED/proxy-ca.pem" --report "$ABA_SCHOOL_SHARED/reports/bootstrap-UNIQUE.json"
```

Expected:manifest/archive/IDs/control contracts and native Ubuntu storage/UID/TLS prerequisites PASS. `validate` требует images уже loaded;load требует отдельного admission action (`docker image load -i images.tar` из verified release),не network pull. `bootstrap-check` пишет temp storage probe/private unique report — это не строго read-only server audit;нужен соответствующий future scope. При ошибке STOP/retain evidence,не `--local-qa` на Ubuntu и не правка manifest IDs.

После **отдельного** owner GO на exact manifest/action install,scope private-staging,project,schema,migrations_approved:

```sh
python3 "$ABA_SCHOOL_RELEASE/scripts/deployment/deploy.py" install \
  --release "$ABA_SCHOOL_RELEASE" --env "$ABA_SCHOOL_SHARED/environment" --project aba-private-staging \
  --storage-root "$ABA_SCHOOL_SHARED" --backup-dir "$ABA_SCHOOL_SHARED/backups" \
  --tls-ca "$ABA_SCHOOL_SHARED/proxy-ca.pem" --go-file "$ABA_SCHOOL_SHARED/install-go.json" \
  --report "$ABA_SCHOOL_SHARED/reports/install-UNIQUE.json"
```

Expected:new own volume/schema16 (текущая latest migration00016,не менять старые migrations),restricted aba_app,no BYPASSRLS,45FORCE RLS по текущему schema baseline,migrate отдельной ролью/readiness/TLS/CSRF checks. Future schema changes требуют нового review и count inventory,не хардкодить старое число в обход проверок. Повторный install при существующем project/volume должен отказать;update через существующий coordinator с backup и отдельным GO. Rollback — только compatible hardened approved release+schema_compatible:true,никогда migrate down/restore поверх current DB автоматически.

## Backup/restore action matrix

Все команды ниже также только будущий approved synthetic runtime. Current release backup scripts не устанавливают retention/deletion policy.

|Действие / команда|Ожидаемое и verification|Rollback|
|---|---|---|
|`python3 "$ABA_SCHOOL_RELEASE/scripts/deployment/backup_job.py" --release "$ABA_SCHOOL_RELEASE" --env "$ABA_SCHOOL_SHARED/environment" --project aba-private-staging --backup-dir "$ABA_SCHOOL_SHARED/backups"`|Unique archive0600,TOC+checksum PASS,exit0;без globals/password dump|При FAIL сохранить partial/evidence,не update;source DB не меняется|
|Review existing backup.service/timer examples и подготовка approved host units с новыми absolute paths|Предлагаемый daily02:30UTC,operator/no secret logging,timer failure alert;все `/srv/aba` paths заменить в **новом host candidate**,не внутри hashed release|Unit installation/enable отдельное разрешение;вернуть previous unit state,не удалять backups|
|Off-host encrypted copy через separately approved storage tool/custody workflow|Independent failure domain,key recovery/access иchecksum verification;команда transfer выбирается после storage approval,сейчас STOP|Не удалять local copy до verify;unknown cost не считать free|
|`python3 "$ABA_SCHOOL_RELEASE/scripts/deployment/backup.py" restore-rehearsal --release "$ABA_SCHOOL_RELEASE" --env "$ABA_SCHOOL_SHARED/restore-UNIQUE/environment" --project aba-private-restore-UNIQUE --archive "$ABA_SCHOOL_SHARED/backups/APPROVED.dump"`|До команды создать отдельный approved env/secrets/subnet/volume и стартовать только его PG;target platform schema пустая;helper отказывает непустому target|При FAIL retain archive/isolated volume/report;не переключать DSN и не удалять volumes|
|Restore security/data+runtime tests по VPS_BACKUP_RECOVERY|Schema/owners/effective ACL/definers/FORCE RLS/tenant isolation/counts/digests,затем separately approved readiness/HTTPS/Browser|Не править ACL ради byte equality,не открывать доступ к real data|

Последняя restore команда сама не создаёт правильный target/env и не разрешает запуск PG без approval. Отдельная orchestration review обязательна;не использовать working volume. Ключи auth/retention должны соответствовать data recovery;реальные ключи отдельно от DB dump. RPO/RTO из architecture — предлагаемые targets,не проверены.

## Regression и финальный go/no-go

Обязательны на **новом** candidate:Go unit/race/vet/build/govulncheck;source/image Trivy и Syft;30startup refusals/all flags;integrity/GO/remote endpoint rejection;isolated migrations/RLS/tenant/concurrency/Excel;frontend lint/typecheck/fresh build+Browser9/9+Catalog2/2;SQL regression;restart/persistence;backup/restore;compatible rollback/roll-forward;CA-verified HTTPS smoke. Routing — [AI_DEV_SYSTEM_INSTALL_PLAN](AI_DEV_SYSTEM_INSTALL_PLAN.md). Рабочая DB не test target,tools/network acquisition отдельно согласованы.

Финальные success criteria инфраструктуры:School21 access+monitoring continuity,host hardening/Docker/storage checks PASS,resource telemetry/alerts иrestore rehearsal PASS,все security/supply-chain holds разрешены,exact new manifest owner GO. Formal privacy/human rehearsal/provider/template/source ownership/financial gates сохраняются;Telegram/WhatsApp/1C live OFF,LLM mock,LLM/WA budgets0RUB. Unknown cost≠free;DELIVERY_UNKNOWN не resend автоматически.

## Выполненная в Stage12E.3 проверка документации

Только five new docs;existing Dockerfiles/Compose/deployment scripts/migrations не изменялись. Статические проверки arithmetic/links/Markdown/whitespace/protected hashes — см. локальную evidence `.local/stage12e3-20261116/validation.json`. Application tests,server audit/hardening/install/deployment — NOT RUN по scope. Нет commit/push/GitHub изменений. Этот результат готовит следующий change review,не утверждает,что сервер hardened или release пригоден к запуску.
