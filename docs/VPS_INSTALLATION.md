# VPS installation — private staging only

Stage12C, финализация12 ноября: текущий candidate/вердикт — [pre-VPS report](STAGE_12C_PRE_VPS_SECURITY_REPORT.md). Обязательны исправленные host entrypoint/GO controls; origin/advisory review остаётся OPEN. Перед любыми server changes выполнить [first-boot verification](VPS_FIRST_BOOT_VERIFICATION.md) по отдельному разрешению. Новые QA GO не являются server GO.

Stage12B,11 ноября: использовать только candidate из `STAGE_12B_FINAL_DEPLOYMENT_VERIFICATION.md`. Требуется `runtime_security_admission_version:1`,30 проверенных startup refusals и отсутствие Python bytecode в release. Старые integrity-valid releases не удовлетворяют новому startup gate; добавление поля в старый manifest без проверенного image запрещено.

## Offline bootstrap contract для нового Ubuntu

NOT RUN на реальном VPS. После отдельного bootstrap GO владелец готовит проверенный offline bundle Docker Engine/CLI/containerd/Compose и всех зависимостей для Ubuntu24.04 amd64. Версии, signatures/advisories и доверенный SHA256 bundle должны быть согласованы отдельно; локально проверены Engine29.9.2 и Compose5.6.1. Bundle не скачивался в этой работе. Установка — действие оператора в уже разрешённой root console нового сервера, без автоматического sudo/download:

```sh
cat /etc/os-release
uname -m
getconf _NPROCESSORS_ONLN
free -h
df -h /srv /var/lib
cd /srv/aba/bootstrap
sha256sum -c SHA256SUMS
dpkg --dry-run --install ./debs/*.deb
dpkg --install ./debs/*.deb
systemctl enable --now docker
docker version
docker compose version
docker context inspect --format '{{json .Endpoints.docker.Host}}'
```

Неполный/unreviewed bundle — STOP; не применять apt-fix с загрузками автоматически. Native Engine/image store обязан сохранять проверенные OCI manifest/index IDs после load; это ещё не подтверждено на Ubuntu. При несовпадении `control.py validate` должен отказать, manifest IDs ради PASS не менять. SSH/firewall сначала согласовать и проверить вторую SSH-сессию; приведённые команды не меняют эти правила. Для agent текущего задания bootstrap/install на VPS остаются вне разрешённой области.

10 ноября 2026. Server GO отсутствует. Ни одна серверная команда здесь не выполнялась. Public production и CUSTOMER PILOT — NO-GO. Существующий package используется без смены архитектуры; новые Go/Next dependencies не нужны.

База: Ubuntu 24.04 LTS amd64, 4 vCPU, 8 GB RAM, 80 GB SSD/NVMe. Docker Engine, Compose v2 с `up --wait`, Python 3.12 и доверенный оператор должны быть установлены владельцем отдельно. Автоматической установки пакетов, sudo, изменения Docker/SSH/firewall здесь нет. Для закрытого staging используется существующий `infrastructure/deployment/compose.yml`: APP_ENV production, internal network, 0 host ports, mock providers, бюджеты 0. Будущему production нужны отдельные project/secrets/volume/origin и отдельное разрешение; public overlay намеренно не активирован.

После отдельного bootstrap approval подготовить приватные каталоги на Docker data filesystem. Следующие команды не меняют firewall/SSH и не создают/перезаписывают секреты:

```sh
install -d -m 0700 /srv/aba/staging /srv/aba/staging/backups /srv/aba/staging/reports
```

Если у оператора нет прав — STOP, не добавлять sudo автоматически. Разместить проверенный release в `/srv/aba/releases/<release>`; `/srv/aba/current` в примерах означает выбранный оператором release. Не переписывать исторический manifest или Next proof. Проверить доверенный manifest SHA256 отдельно; затем после GO:

```sh
cd /srv/aba/current
sha256sum -c images.tar.sha256
docker image load -i images.tar
python3 scripts/deployment/control.py validate --release /srv/aba/current --env /srv/aba/staging/environment --project aba-private-staging
```

`environment` mode0600 содержит только image IDs из `images.env`, ABA_SECRETS_DIR и HTTPS origin. Секреты — вне release, directory0700; точные имена/UID/modes/TLS SAN см. [существующий runbook](VPS_DEPLOYMENT_RUNBOOK.md). Нужны три разные random48hex DB passwords, независимые32-byte auth/retention keys, DSN ролей aba_app/aba_migrator с verify-full и CA; никаких default credentials. Создание реальных credentials — операторская первоначальная настройка, текущие QA используют только synthetic значения.

Также задать `ABA_PRIVATE_SUBNET` из `environment.example`: отдельный reviewed non-overlapping private CIDR для staging и каждого restore project. Compose задаёт IPAM явно, не меняя глобальные Docker address pools. Проверить конфликты до GO; пример CIDR не является свободным адресом на любом сервере.

Проверка без запуска сервисов:

```sh
python3 scripts/deployment/deploy.py bootstrap-check --release /srv/aba/current --env /srv/aba/staging/environment --project aba-private-staging --storage-root /srv/aba/staging --backup-dir /srv/aba/staging/backups --tls-ca /srv/aba/staging/proxy-ca.pem --report /srv/aba/staging/reports/bootstrap-001.json
```

Она проверяет локальный unix Engine, Ubuntu/архитектуру, доступные CPU/RAM, native Docker filesystem, минимум20GiB free/70GiB total, private env/secrets и отдельный persistent volume. Probe пишет только временный файл в разрешённых private directories. На macOS `--local-qa` с project `aba-private-qa-*` явно снимает только native Ubuntu/Engine-disk certification, не image/live/GO gates. Storage QA host path не доказывает свободный диск Linux VM.

Основная команда первого развёртывания **только после отдельного GO**:

```sh
python3 scripts/deployment/deploy.py install --release /srv/aba/current --env /srv/aba/staging/environment --project aba-private-staging --storage-root /srv/aba/staging --backup-dir /srv/aba/staging/backups --tls-ca /srv/aba/staging/proxy-ca.pem --go-file /srv/aba/staging/install-go.json --report /srv/aba/staging/reports/install-001.json
```

GO format: [VPS_LAUNCH_GATE.md](VPS_LAUNCH_GATE.md). Install отказывается при существующем project/DB volume; повтор для существующей установки — `update`, который сначала делает backup. После миграции и readiness выполняются schema/FORCE RLS/role/anonymous isolation и CA-verified HTTPS/login/401/CSRF checks. Ошибка прекращает последовательность; автоматического down/delete/rollback нет. JSON reports mode0600, существующий report не перезаписывается. PASS этой команды — технические проверки, не Security PASS или customer GO.
