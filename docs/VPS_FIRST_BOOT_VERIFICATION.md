# VPS first-boot verification

Финализация 12 ноября 2026; локальные проверки 11 ноября. **NOT RUN на VPS.** VPS ещё не куплен. Все команды ниже предназначены владельцу после покупки и отдельного разрешения на read-only inspection; они не устанавливают пакеты, не стартуют контейнеры и не меняют firewall/SSH. CUSTOMER PILOT NO-GO.

## 1. Identity, resources and storage

На проверенной SSH/console session, сверив host key независимым каналом:

```sh
cat /etc/os-release
uname -m
getconf _NPROCESSORS_ONLN
lscpu
free -h
lsblk -o NAME,TYPE,SIZE,FSTYPE,MOUNTPOINTS
df -h / /srv /var/lib
df -i / /srv /var/lib
findmnt -T /srv
findmnt -T /var/lib
systemd-detect-virt
timedatectl status
```

Ожидаются Ubuntu 24.04 LTS, x86_64, ≥4 logical CPUs, заказанные 8 GB RAM (Engine gate ≥7 GiB), 80 GB class SSD/NVMe с ≥70 GiB доступной filesystem ёмкости и ≥20 GiB free. Сверить ограничения провайдера, swap, CPU steal и persistent storage. `df`/разовый CPU snapshot не доказывают peak performance. Отсутствующая `/srv` или недостаточные права — BLOCKED, не добавлять sudo/install автоматически.

## 2. Access, network and permissions

```sh
id
ip -br address
ip route
ss -lntup
stat -c '%a %U:%G %n' /srv /var/lib
```

Проверить вторую key-only operator session и независимую recovery console до будущего SSH hardening. Provider firewall проверяется отдельно в выбранном account; `ss` не доказывает внешнюю недоступность. Read-only `sshd -T`, `nft list ruleset`, `ufw status verbose` требуют уже имеющихся прав; при отказе сохранить BLOCKED и поручить владельцу console review. Здесь не даётся разрешение на privilege escalation.

Ожидаются только согласованный SSH management listener/allowlist, закрытые web/DB ports по IPv4 и IPv6 и отсутствие публичного Docker API. Выбранная схема доступа: [private security plan](VPS_PRIVATE_STAGING_SECURITY_PLAN.md). Host/Docker firewall forwarding и provider ACL требуют двух независимых проверок; UFW сам по себе не доказательство Docker isolation.

## 3. Docker — только если уже установлен владельцем

```sh
docker version
docker compose version
docker context show
docker context inspect --format '{{json .Endpoints.docker.Host}}'
docker info --format '{{json .ServerVersion}} {{json .OSType}} {{json .Architecture}} {{json .NCPU}} {{json .MemTotal}} {{json .DockerRootDir}}'
docker info --format '{{json .DriverStatus}}'
stat -c '%a %U:%G %n' /var/run/docker.sock
```

Требуются local Unix socket, linux/amd64, поддержка Compose `config --format json`, `up --wait`, `pull_policy: never`, secrets/tmpfs/resource limits. Локально проверены Engine 29.9.2 / Compose 5.6.1 с containerd image store. Это tested tuple, не сертификация всех версий или classic image store. Socket доступен только доверенным администраторам; docker group даёт root-equivalent полномочия. Если Engine отсутствует — NOT RUN, bootstrap только по отдельно reviewed offline bundle из [installation](VPS_INSTALLATION.md).

## 4. Release и prerequisites — после отдельного transfer/load authorization

Это read-only проверки уже доставленного release; сам transfer и `docker image load` являются отдельными действиями и здесь не выполнялись:

```sh
cd /srv/aba/current
sha256sum manifest.json
sha256sum -c images.tar.sha256
python3 scripts/deployment/control.py validate --release /srv/aba/current --env /srv/aba/staging/environment --project aba-private-staging
stat -c '%a %U:%G %n' /srv/aba/staging /srv/aba/staging/environment /srv/aba/staging/backups /srv/aba/staging/reports
```

Manifest hash сверить с отдельно доверенным review record, не только соседним checksum. Все пять IDs обязаны сохраниться после load; validator проверяет local image architecture/ID. При несовпадении — STOP, не переписывать manifest. Не печатать env, raw inspect, private keys, DSN или GO contents в публичный лог. `PUBLIC_ORIGIN`/cert SAN/private route/CA trust и native UID/modes секретов должны быть согласованы.

## 5. Отдельный следующий gate

После completed provenance/advisory review, provisioning, разрешения на private storage probe и записи private report:

```sh
python3 /srv/aba/current/scripts/deployment/deploy.py bootstrap-check --release /srv/aba/current --env /srv/aba/staging/environment --project aba-private-staging --storage-root /srv/aba/staging --backup-dir /srv/aba/staging/backups --tls-ca /srv/aba/staging/proxy-ca.pem --report /srv/aba/staging/reports/first-bootstrap-001.json
```

Этот gate не стартует сервисы, но пишет report и temporary storage probe; он отделён от строго read-only команд выше. Не применять `--local-qa` на VPS. Install требует нового exact-manifest/private-staging GO с migration approval, затем readiness/RLS/TLS/private access и отдельный restore test. Серверный PASS не выдается заранее.

Evidence текущего local review: `.local/stage12c-20261111/baseline.json`, `docker-baseline.json`, `image-runtime-inspect.json`, `resource-measurement-report.json`; [итоговый report](STAGE_12C_PRE_VPS_SECURITY_REPORT.md).
