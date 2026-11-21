# Private Linux VPS runbook — package only

Stage12C: актуальный review и candidate — [STAGE_12C_PRE_VPS_SECURITY_REPORT.md](STAGE_12C_PRE_VPS_SECURITY_REPORT.md). Для первоначального install/update использовать coordinator из VPS_INSTALLATION.md. Старые host tools не использовать для обхода scope/entrypoint admission. Origin/advisory/security GO не заменяются локальным PASS. Historical records ниже сохранены.

Stage12B,11 ноября: current eligible candidate `.local/stage12b-20261111/verified-release/`, точный verdict/риски в `STAGE_12B_FINAL_DEPLOYMENT_VERIFICATION.md`. Admission v1/15 runtime controls и no-bytecode immutability обязательны. Старые packages ниже — historical, не rollback targets для hardened API.

Актуальная automation10 ноября: **PACKAGE READY, DEPLOYMENT BLOCKED; CUSTOMER PILOT NO-GO.** Новый package `.local/vps-readiness-20261110/release/`, одна основная install/update command описана в `VPS_INSTALLATION.md`; health/diagnose/rollback — `VPS_OPERATIONS.md`, scheduled backup/restore — `VPS_BACKUP_RECOVERY.md`, exact GO — `VPS_LAUNCH_GATE.md`. Fresh Docker QA install/restart/restore/rollback/roll-forward PASS; target Ubuntu/VPS NOT RUN. Ниже сохранён исходный manual runbook и история прежних packages; для нового review использовать новый candidate и обязательный ABA_PRIVATE_SUBNET из его template.

3 ноября2026. **STAGING BLOCKED. Ни одна серверная команда ниже в этом задании не выполнялась.** Не подключаться к VPS и не запускать start без нового owner GO. E01 OPEN, Security CONDITIONAL; live/paid OFF. Подробные результаты: STAGE_12A_DEPLOYMENT_READINESS.md и11А.

Перепроверка **10 ноября 2026**: current candidate `.local/recovery12a-20261110/release/` содержит исправленный admission validator, требующий все live/budget/kill flags API/worker. Runtime images и Next proof прежние; новый manifest содержит parent SHA256. Старый release остаётся историческим. Docker daemon недоступен: новый full image validation, build/startup/DB/Browser/backup rehearsal **NOT RUN (BLOCKED)**. Offline integrity/Compose/unit checks не заменяют их. До server GO восстановить локальный Docker и пройти prepared isolated gates; не изменять глобальную конфигурацию из runbook автоматически.

## Цель и prerequisites

Ubuntu24.04 LTS **x86_64**,4vCPU/8GB/80GB SSD/NVMe, установленный и отдельно согласованный Docker Engine + Compose и Python3.12. Проверить фактические версии, свободный диск/RAM, clock/NTP, доступность локальных image IDs и непересекающихся Docker networks. Никаких автоматических install/pull/update в scripts нет. На ARM64 остановиться: package рассчитан на linux/amd64.

Stage12A проверял Linux containers на Docker Desktop, не VPS. Перед GO повторить целевые smoke/capacity/backup/RLS checks именно на Ubuntu. Не совмещать staging DB/project/volumes/secrets с рабочим demo или другим приложением. Docker group/socket даёт административные полномочия: доступ только согласованным операторам, без socket в приложениях.

## Сеть, SSH и TLS — до GO

- Default deny inbound; SSH22 только с согласованных operator IP/VPN. Key authentication, запрет root/password login, отдельная operator учётная запись и ограниченный доступ к Docker. Проверять новую SSH-сессию **до** изменения действующих правил; scripts не меняют sshd/UFW/iptables.
- Compose **не публикует порты вообще**. Не добавлять0.0.0.0/host network. PostgreSQL5432, SMTP1025, Mailpit8025, Docker2375/2376 не публиковать. Учесть Docker firewall chains и DOCKER-USER; UFW сам по себе не доказывает отсутствие Docker exposure.
- HTTPS nginx слушает8443 только внутри private network. TLS certificate/key предоставляет оператор; автоматического ACME, public DNS или paid сервиса нет. PUBLIC_ORIGIN должен точно совпадать с согласованным HTTPS origin. Нельзя отключать certificate verification.
- Private operator access после отдельного GO: SSH forwarding к IP **своего** proxy container на Ubuntu, bind tunnel строго127.0.0.1. Например `ssh -N -L 127.0.0.1:18443:<PRIVATE_PROXY_CONTAINER_IP>:8443 <operator>@<vps>`. Сам SSH host port у proxy не появляется. Browser origin/hostname/certificate trust должны быть заранее согласованы; не обходить ошибки сертификата. CLI probe возможен с `curl --cacert <CA.pem> --resolve <HOST>:18443:127.0.0.1 https://<HOST>:18443/login`, при совпадающем PUBLIC_ORIGIN. На Docker Desktop host routing отличается; этот способ там не выдаётся за проверенный.
- Открытие80/443 или дополнительного loopback publication — отдельное решение с проверкой exposure; текущие validators запрещают любые published ports.

## Получение offline release

Текущий candidate: `.local/recovery12a-20261110/release/`. Переносить **только этот каталог** после GO, не `.local` целиком. Он содержит images archive/checksum, manifest, `images.env`, config и scripts. QA secret files в него не копируются. Это offline переупаковка сохранённых images, а не новый build/smoke; первоначальный release от 3 ноября сохранён. Перед переносом/загрузкой сверить новый manifest SHA256 по доверенному отдельному каналу; checksum рядом с архивом сам по себе не доказывает происхождение.

Будущий локальный rebuild, без установки dependencies:

```sh
python3 scripts/deployment/package.py \
  --release <NEW_UNIQUE_RELEASE> \
  --web-volume <DEDICATED_VERIFIED_ABA_LINUX_BUILD_VOLUME> \
  --artifacts <FROZEN_STATIC_GO_ARTIFACTS_DIRECTORY> \
  --output <NEW_PRIVATE_RELEASE_DIRECTORY> --export-images
```

Builder ожидает уже подготовленные offline caches: Go1.26.2, Playwright1.64.0/Node24 glibc, PostgreSQL18.3, nginx1.27.3, Mailpit1.21. Это QA candidates; перед VPS требуется separate advisory/provenance review. Нет доступа к registry/новых tools — NOT RUN/BLOCKED, не автоматическая загрузка. Existing source-install Dockerfiles не используются для этого маршрута. Новый Next build делается штатным production.sh в **новом** Linux output без env/host native modules. Старый polluted tree не удалять/не исправлять baseline.

После переноса на одобренный сервер, только в своём release directory:

```sh
sha256sum -c images.tar.sha256
docker image load -i images.tar
```

Это будущие действия;12А их на сервере не выполнял. IDs из manifest должны сохраниться после load. `control.py validate` отвергает mutable image references, несовместимую платформу, missing image и изменения hashed config/scripts/context. Не переносить секреты в image build context.

Сохранённый archive использует OCI index/manifest digests как IDs. Загрузку и сохранение этих IDs целевым Ubuntu Engine/image store ещё не проверяли. После разрешённого `load` подтвердить все пять IDs через `image inspect` и `control.py validate`; при несовпадении STOP, не заменять manifest IDs ради прохождения gate. Runtime config/logs/secrets при проверке не печатать.

## Secrets и environment

Создать отдельный root-controlled secrets directory0700 вне checkout/release; секреты не вводить в командную строку, shell history, чат или report. Env file0600 содержит **только** immutable image IDs, absolute secrets path и HTTPS origin. Взять IDs из release `images.env`; `environment.example` объясняет значения, а не является готовой авторизацией старта.

|Файл в ABA_SECRETS_DIR|Доступ в контейнере / содержание|
|---|---|
|database_url|UID10001,0400; `aba_app`, database `aba_stage`, host `postgres`, `sslmode=verify-full&sslrootcert=/run/secrets/db_ca`|
|migration_database_url|UID10001,0400; отдельная `aba_migrator` DSN с теми же TLS/database требованиями|
|auth_email_key, retention_key|UID10001,0400; независимые32bytes hex; не ротировать ключ существующих данных без согласованного workflow|
|admin_password, app_password, migrator_password|PG UID70,0400; разные randomly generated48 lowercase hex characters; bootstrap проверяет app/migrator формат|
|db_certificate, db_key|PG UID70; private key0400, certificate допустимо0444; SAN DNS:postgres, serverAuth, действующий срок|
|db_ca|UID10001 readable; публичный CA certificate,0400 или0444; CA private key хранится отдельно|
|proxy_certificate, proxy_key|nginx UID101; key0400, certificate readable; SAN совпадает с согласованным private origin|

File-backed Compose secrets не гарантируют remap UID/GID/mode: реальный bind file должен уже иметь правильные права. Проверить их **из контейнера нужным UID**, не делая secrets world-readable ради PASS. `_FILE` loader отвергает symlinks, пустые/многострочные/oversize files и одновременный inline value. Секреты попадают только в process environment дочернего Go binary, не в Compose environment/manifest/logs. Доступ operator/root к process memory/env остаётся доверенной границей.

Нельзя добавлять Telegram/WhatsApp encryption keys в production: действующий код запрещает production processing до отдельного privacy/live approval. Включён только локальный email sink. Реальные письма не отправляются; Mailpit UI не публикуется. Доступ к synthetic verification письму — только приватный операторский workflow, не public verification bypass.

## Validate, migration и start — только после GO

```sh
python3 scripts/deployment/control.py validate \
  --release <RELEASE_DIRECTORY> --env <PRIVATE_ENV_FILE> \
  --project aba-private-staging
```

Validate не стартует containers и не скачивает images. Проверяет integrity/config/image IDs, internal network/no ports/no socket, mock/kill/budget flags. Не проверяет реальный firewall, владельца решения или отсутствие CVEs.

Owner GO — отдельный reviewable JSON, не создаётся агентом автоматически:

```json
{
  "approved_by_owner": false,
  "action": "start",
  "scope": "private-staging",
  "database_schema": 16,
  "migrations_approved": false,
  "project": "aba-private-staging",
  "manifest_sha256": "<EXACT_REVIEWED_MANIFEST_SHA256>"
}
```

`false`/placeholder должны приводить к отказу. Только после отдельного решения владельца подготовить действительное согласование этой версии/project/action. GO file — операционная запись, не криптографическая подпись/замена разрешения владельца. Текущий stage12А такого GO не выдаёт.

```sh
python3 scripts/deployment/control.py start \
  --release <RELEASE_DIRECTORY> --env <PRIVATE_ENV_FILE> \
  --project aba-private-staging --go-file <OWNER_APPROVAL_FILE>
```

Postgres bootstrap создаёт только restricted roles, schema и grants на новом volume. Migrator использует отдельный secret, goose lock и versioned migrations до16; API стартует только после successful migration и PG health. `up` идемпотентен; никакого `migrate down`, автоматического reinitialization или удаления volumes. Если bootstrap оборвался и data directory уже существует — STOP, сохранить volume/logs и расследовать; не чинить рабочие roles вручную ради запуска. Новый пустой QA project допустим только как отдельная репетиция.

После start проверить image IDs, UID, health, CA-verified HTTPS `/login`, unauthenticated401, обычную авторизацию/CSRF/tenant checks, no published ports и live OFF. Worker health — лишь liveness; queue progress требует отдельной synthetic regression. Источник E01 остаётся OPEN даже при чистом image guard.

## Backup и изолированное восстановление

Backups directory0700, archive0600. Не давать backup/runtime контейнерам Docker socket; backup script выполняется доверенным локальным оператором через Engine. `pg_dump` идёт по local socket **своего** PG container; passwords/global roles не экспортируются. Все owners/ACL/policies сохраняются. Сроки хранения/удаления не устанавливаются. Synthetic dump текущей репетиции не зашифрован; реальные данные запрещены до отдельного encryption/key custody/storage approval.

```sh
python3 scripts/deployment/backup.py backup \
  --release <RELEASE_DIRECTORY> --env <PRIVATE_ENV_FILE> \
  --project aba-private-staging --archive <PRIVATE_BACKUP_DIRECTORY>/before-change.dump
```

Для restore создать **новый** проект `aba-private-restore-<UNIQUE>` с новым volume и отдельной internal subnet при необходимости. Запустить только PostgreSQL bootstrap; не API/worker. Новый target schema platform должен быть пустым. Helper сверяет checksum, project prefix и отсутствие других running services; непустая DB отвергается. Не использовать `--clean`, truncate или исходный volume.

```sh
python3 scripts/deployment/backup.py restore-rehearsal \
  --release <RELEASE_DIRECTORY> --env <RESTORE_ENV_FILE> \
  --project aba-private-restore-<UNIQUE> --archive <PRIVATE_BACKUP_DIRECTORY>/before-change.dump
```

Restore использует local admin для COPY через FORCE RLS и восстанавливает archived aba_migrator ownership/ACL. Runtime admin secret не получает. Проверить schema16, все45 FORCE RLS tables, effective grants/owners/SECURITY DEFINER search_path, policies, synthetic counts и runtime `/health/ready`. NULL/default owner ACL может отличаться побайтово от explicit owner ACL — сравнивать effective privileges, не менять DB ради вида ACL. Ключи auth/provider/retention и роль credentials предоставляются отдельно согласованно с DB. До real-data эксплуатации проверить recovery на representative backup.

## Stop и rollback

Остановить только свой project, с отдельной записью GO action `stop` для control.py; `stop --timeout -1` ждёт штатного завершения, не escalates KILL, volumes сохраняются. Не применять `down -v`, prune, killall/pkill и не трогать demo/Audio/другие проекты.

Rollback — возврат ранее проверенных immutable API/worker/web/proxy images **только при доказанной schema compatibility**. Сначала backup и review; GO action `rollback` должен содержать `schema_compatible:true`, project и SHA256 именно старого manifest. `control.py rollback` не запускает migrations down/restore и не меняет PG volume. Если schema несовместима — STOP, отдельный recovery plan, не автоматический rollback. Реальная rollback rehearsal в12А NOT RUN.

До первого магазина остаются blockers из12А: E01/остаточные SQL evidence, Security review, private staging GO, Ubuntu verification и operational ownership. Этот runbook не разрешает public/live pilot.
