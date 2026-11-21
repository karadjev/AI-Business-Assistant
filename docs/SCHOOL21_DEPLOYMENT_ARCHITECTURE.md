# School21 — deployment architecture

Stage12E.3, 2026-11-16. **PREPARED FOR SERVER HARDENING** означает готовность документации. Серверная проверка и deployment **BLOCKED**; PRIVATE STAGING/CUSTOMER PILOT NO-GO. Все команды ниже — будущие действия после соответствующего разрешения, NOT RUN. Подключений к серверу нет.

## Основание и неизменённые контракты

Изучены действующий корневой AGENTS.md (других применимых AGENTS.md не найдено), Stage12B,12C,12C.1,12C.2, TRUSTED_RELEASE_PROVENANCE, VPS installation/runbook/security/operations/backup/launch gate, Stage12E.2 и реальные Compose/nginx/pg_hba/deployment validators. Исторические QA PASS не повторялись и не считаются Ubuntu server PASS. Главный baseline — [compose.yml](../infrastructure/deployment/compose.yml), [nginx.conf](../infrastructure/deployment/nginx.conf), [common.py](../scripts/deployment/common.py), [deploy.py](../scripts/deployment/deploy.py). Изменения Dockerfiles/Compose/scripts/migrations не требуются для подготовки плана и не выполнены.

Источник server sizing — сообщение владельца: Ubuntu24.04.5 amd64,4vCPU,7.8GiB,120GB; это не fresh host measurement. School21 Zabbix/management и существующие SSH constraints требуют отдельного audit.

## Поток запросов и внутренние имена

Оператор → SSH22 School21 host → private tunnel → proxy:8443/TLS → web:3000/Next BFF → api:8080 → postgres:5432/TLS. Worker читает очередь в PostgreSQL; SMTP идёт только в Mailpit. Go API/worker/migrate используют один API image; всего **пять** runtime image identities, семь Compose services с учётом one-shot migrate.

|Service / Docker DNS|Назначение, внутренний порт|Persistence / security|
|---|---|---|
|proxy|nginx HTTPS8443 → `web:3000`|UID101,RO root,keys bind secrets,config mount RO,tmpfs64MiB; healthcheck nginx syntax, не serving proof|
|web|Next.js frontend+BFF3000 → `api:8080`|UID10001,RO,no host ports,tmpfs64MiB; Node/native/runtime provenance обязательны|
|api|Go HTTP8080 → `postgres:5432`, `mailpit:1025`|UID10001,RO,tmpfs32MiB,aba_app restricted role,readiness не доказывает tenant isolation|
|worker|Нет публичного listener; DB queue и mock providers|UID10001,RO,tmpfs32MiB,worker health=liveness,не успешная доставка|
|postgres|PostgreSQL5432,DB `aba_stage`|Named volume `database` → `/var/lib/postgresql`; TLS verify-full SAN `postgres`,FORCE RLS,отдельный migrator|
|migrate|One-shot `migrate up`,нет listener|API image,aba_migrator credentials; только approved migrations перед API; restart=no|
|mailpit|Локальный synthetic SMTP1025,UI8025|UID10001,RO,DB в tmpfs32MiB; письма НЕ persistent,UI не публикуется|
|Zabbix agent (host)|Обычно10050 passive / исходящий10051 active,проверить actual config|School21-owned service вне Compose; не отключать,не монтировать Docker socket|

Service DNS names стабильны внутри Compose; network/volume actual names имеют project prefix `aba-private-staging`. Единственная сеть `private`, `internal:true`, explicit `ABA_PRIVATE_SUBNET`; network name обычно `aba-private-staging_private`. Subnet — только после сверки School21 VPN/LAN/Docker routes; template10.254.250.0/28 не объявляется свободным. Нет external networks,host networking,build/pull или Docker socket mounts. Общая internal network не является per-service network segmentation; изменение topology требует отдельного reviewed release, здесь не предлагается.

|Host exposure|Policy|
|---|---|
|TCP22|Только согласованные owner/School21 admin VPN/CIDR,IPv4/IPv6|
|TCP10050|Только School21 monitoring server/proxy если нужны passive checks|
|80/443/8443/3000/8080/5432/1025/8025/2375/2376|Не публиковать на host/public interface|

Private tunnel сохраняет существующий контракт. После отдельного deployment GO оператор получает IP **своего** proxy, не фиксирует контейнерный IP как постоянный DNS. Пример future workstation command:

```sh
ssh -N -o ExitOnForwardFailure=yes -L 127.0.0.1:18443:PROXY_PRIVATE_IP:8443 ADMIN@APPROVED_SCHOOL21_HOST
curl --cacert APPROVED_CA.pem --resolve staging.example.test:18443:127.0.0.1 https://staging.example.test:18443/login
```

`staging.example.test` — placeholder: заменить согласованным private hostname. PUBLIC_ORIGIN должен точно совпадать с HTTPS host:18443; сертификат SAN и client DNS должны поддерживать этот hostname. `--resolve` решает DNS только для curl; браузеру нужен согласованный local DNS/hosts mapping и CA trust. Не использовать insecure TLS flags. SSH PermitOpen/forwarding policy согласовать с School21. Доступ host→internal bridge и tunnel на данном Ubuntu ещё NOT RUN. Откат tunnel — завершить только его SSH process; не менять Compose/host firewall ради обхода ошибки.

## Storage: реальные каталоги и владельцы

```text
/srv/ai-business-assistant/             root:root 0700, real directory
  releases/                            root:root 0700
    RELEASE_ID/                        immutable admitted release
  current -> releases/RELEASE_ID        convenience pointer, меняется после успешного rollout
  shared/                              root:root 0700, real storage-root
    environment                        root:root 0600, non-secret paths/IDs/origin
    secrets/                           root:root 0700
    backups/                           root:root 0700, archives0600
    uploads/                           root:root 0700, reserved, пока не mounted
    logs/                              root:root 0700, redacted exports only
    reports/                           root:root 0700, reports0600
    proxy-ca.pem                       approved public CA, controlled permissions
```

Команды выполняет отдельно уполномоченный host operator; root ownership не означает общедоступный Docker access. `current` — symlink удобства: coordinator получает resolved `releases/RELEASE_ID` как `--release`; `--storage-root` — **реальный** `shared`, не symlink. Validator запрещает symlink storage-root и group/other permissions. Shared и DockerRootDir должны иметь одинаковый st_dev; не менять daemon data-root ради PASS. Если layout файловых систем не соответствует — STOP и согласовать storage plan. `/var/lib/docker` (или фактический approved DockerRootDir) содержит named DB volume и container logs; он не является автоматически backup.

`uploads/` зарезервирован без нового volume: текущий Excel import сохраняет grid/preview в PostgreSQL, а не использует host uploads directory. `logs/` также не добавляется как runtime mount: Docker logging уже ограничен json-file10MiB×3/container, proxy access_log off. Изменение mount контрактов без необходимости запрещено.

Secret **файлы**, в отличие от каталога, принадлежат runtime UID, чтобы bind-mounted secret читался контейнером: database_url/migration_database_url/auth_email_key/retention_key UID10001,0400; admin_password/app_password/migrator_password/db_key UID70,0400; proxy_key UID101,0400. Public certificates/CA допустимы0444, private keys никогда. Проверить UID фактического admitted vendor image и compatibility с текущим checker; не подменять владельца ради нового несовместимого image. Корневой daemon может bind-mount файл из0700 directory; не делать каталог world-readable. Secret-file uid/mode remap средствами обычного Compose не предполагать. Значения/key material не выводить.

## Действия оператора: команда, результат, проверка, rollback

Все мутации ниже требуют будущего server-hardening/storage approval; в текущем этапе не исполнялись. RELEASE_ID должен быть уникален, approved и без path separators. Не перезаписывать существующие каталоги/credentials.

|Действие / команда|Ожидаемый результат / проверка|Rollback|
|---|---|---|
|Проверить пути: `findmnt -T /srv; df -h /srv; stat -c '%d %a %U:%G %n' /srv`|Layout inventory; после Docker сравнить с фактическим DockerRootDir|Нет мутации|
|Создать структуру только если base отсутствует: `test ! -e /srv/ai-business-assistant && install -d -o root -g root -m 0700 /srv/ai-business-assistant /srv/ai-business-assistant/releases /srv/ai-business-assistant/shared /srv/ai-business-assistant/shared/secrets /srv/ai-business-assistant/shared/backups /srv/ai-business-assistant/shared/uploads /srv/ai-business-assistant/shared/logs /srv/ai-business-assistant/shared/reports`|`stat` всех путей:0700,root:root; base существует → STOP/review,не recursive chmod|Оставить private пустые каталоги; удаление только отдельным решением и только пустых своих путей|
|Разместить verified release в новом `releases/RELEASE_ID` по утверждённому archive/extraction manifest|`sha256sum -c images.tar.sha256` и source/file manifest bindings; не извлекать untrusted tar с overwrite|Не активировать candidate,сохранить archive/FAIL; старый release не трогать|
|Подготовить `shared/environment` вне checkout; `stat -c '%a %U:%G' /srv/ai-business-assistant/shared/environment`|0600,immutable image IDs,absolute secret path,HTTPS origin,non-overlap subnet; не `cat` файла в report|Вернуть собственную сохранённую env version,не менять active secrets|
|Выдать file ownership для secret только по таблице: например `chown 10001:10001 /srv/ai-business-assistant/shared/secrets/database_url; chmod 0400 /srv/ai-business-assistant/shared/secrets/database_url`|`stat` без contents; затем loader/UID probe внутри approved image,без secret output|Вернуть сохранённые owner/modes; не ротировать ключи и не заменять содержимое|
|После успешного отдельно разрешённого rollout: `test ! -e /srv/ai-business-assistant/current.next && ln -s releases/RELEASE_ID /srv/ai-business-assistant/current.next` затем `mv -T /srv/ai-business-assistant/current.next /srv/ai-business-assistant/current`|Pointer ссылается на проверенный release; проверить `readlink -f` перед/после; old link записать заранее|Вернуть prior pointer атомарно; это НЕ откат DB/контейнеров,для них отдельный GO|

## Backup и monitoring

Стартовая **предлагаемая**, не включённая policy: daily backup02:30 UTC + перед update, отдельное off-host encrypted copy; целевые RPO≤24ч,RTO≤4ч пока NOT VERIFIED. Retention и удаление требуют отдельного согласования; не включать автоматическую очистку. Для реальных данных нужны encryption/key custody/access/privacy approval. Synthetic archive0600 не заменяет шифрование. Подробная procedure в [checklist](SCHOOL21_DEPLOYMENT_CHECKLIST.md); существующие backup scripts сохраняются.

Хранить отдельно DB dump+checksum, release/manifest/schema metadata и необходимые ключи в независимом secret recovery custody. Secret recovery bundle не класть в общий dump/release. Docker DB volume snapshot не равен согласованному logical backup. Restore только в новый `aba-private-restore-*`,новый volume/subnet/env; PostgreSQL сначала,без API/worker,проверка FORCE RLS/ACL/tenants и data digests. Нельзя down -v/prune/truncate/restore поверх staging.

Monitoring использует установленный School21 Zabbix; new agent/exporter не устанавливать. Нужны host RAM/disk/inodes/clock/SSH, container memory/OOM/restarts/health, DB connections/locks, queue age, backup age/status и HTTPS reachability из approved private scope. Не давать Zabbix agent доступ к Docker socket; custom root metric scripts — отдельный security review. Не включать SQL parameters/body/cookies/DSN в logs; Nginx error context тоже под privacy review. Security audit retention отдельно от Docker log rotation.
