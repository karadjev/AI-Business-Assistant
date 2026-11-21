# School21 — verified Docker installation plan

Stage12E.3,2026-11-16. **BLOCKED at prerequisite SSH trust gate**. Системные изменения личного VPS176.57.214.93 разрешены владельцем; это разрешение не отменяет обязательный порядок SSH trust → recovery/admin key login → SSH hardening → Docker. Все команды ниже остаются будущими, NOT RUN. Не выполнялись sudo,подключение,package install/update или daemon configuration. Actual Docker state не проверен; «отсутствует» — предоставленный baseline, не результат свежего запроса. [Execution report](STAGE_12E3_EXECUTION_REPORT.md).

## Gate D0: источник и полномочия

Источник **только официальный Docker APT repository** `https://download.docker.com/linux/ubuntu`,suite noble,component stable,architecture amd64. Не convenience installer,не `curl | sh`,не произвольный PPA. [Официальная установка](https://docs.docker.com/engine/install/ubuntu/) подтверждает Ubuntu24.04/amd64 и нужные packages. Exact versions,размеры,dependency closure и approved signing fingerprint здесь **UNKNOWN**,не выдуманы. Наличие repository key на том же HTTPS endpoint — bootstrap trust,не независимое доказательство publisher identity.

Текущая owner authorization позволяет системную подготовку после prerequisites; повторное общее разрешение на hardening не требуется. До Docker нужны verified trust/recovery/SSH hardening, известные School21 platform/network constraints и проверяемый package acquisition manifest. Exact key fingerprint/versions/hashes/sizes не подменяются предположениями; admission неустановленных inputs фиксируется отдельно до установки. D2 application-image approval не является доказательством доверия Docker packages. Ранее VPS_INSTALLATION.md предлагал offline bootstrap bundle;эта процедура готовит такой bundle из **официального APT**,не заменяет старые scripts и не расширяет release/deployment GO.

## Команды и checkpoints

Блоки не исполнять целиком без соответствующего gate. Все evidence сохранять в private approved change directory,без environment/proxy credentials. Исходное состояние host зафиксировать до network/package actions.

|Шаг / команда|Ожидаемый результат / проверка|Rollback|
|---|---|---|
|D1 preflight: `cat /etc/os-release; dpkg --print-architecture; uname -m; systemd-detect-virt; df -h /srv /var/lib`|ID ubuntu,VERSION_ID24.04,noble,amd64,x86_64;диск и VM constraints согласованы|Нет изменений|
|`dpkg-query -W docker-ce docker-ce-cli docker.io containerd containerd.io runc docker-buildx-plugin docker-compose-plugin podman-docker`|Inventory conflict packages/versions;absent package exit не installation FAIL|Не удалять School21 container runtime;при конфликте STOP/review|
|`command -v curl gpg; stat /etc/apt/sources.list.d /etc/apt/keyrings`|Необходимые tools/paths существуют или missing-input proposal|Не устанавливать dependencies автоматически|
|Сохранить repo/keyring/package/service/network inventory в private change record|Понятно,что было до Docker|Evidence не удалять;не копировать secrets в report|

После explicit metadata/key approval,обычным approved admin privilege mechanism (без sudo в текущем этапе):

```sh
# Не перезаписывать существующий key/source file без review.
test ! -e /etc/apt/keyrings/docker.asc
test ! -e /etc/apt/sources.list.d/docker.sources
install -d -m 0755 /etc/apt/keyrings
curl --fail --show-error --silent --proto '=https' --tlsv1.2 \
  https://download.docker.com/linux/ubuntu/gpg -o APPROVED_CHANGE_DIR/docker.asc.candidate
gpg --show-keys --with-colons APPROVED_CHANGE_DIR/docker.asc.candidate
```

Ожидаемый результат: candidate key,без выполнения downloaded code. Проверка: `fpr` сравнить с independently approved Docker fingerprint,сохранить exact key hash/source/time. Совпадение с самостоятельно вычисленным hash не проверка origin. Fingerprint/expiry/revocation policy пока не утверждены → **STOP до Signed-By installation**. Rollback: candidate остаётся quarantine;старый keyring не менялся.

После key admission:

```sh
install -o root -g root -m 0644 APPROVED_CHANGE_DIR/docker.asc.candidate /etc/apt/keyrings/docker.asc
# APPROVED_DOCKER_SOURCES заранее reviewed; точное содержимое ниже.
install -o root -g root -m 0644 APPROVED_DOCKER_SOURCES /etc/apt/sources.list.d/docker.sources
```

Содержимое candidate:

```text
Types: deb
URIs: https://download.docker.com/linux/ubuntu
Suites: noble
Components: stable
Architectures: amd64
Signed-By: /etc/apt/keyrings/docker.asc
```

Ожидается scoped trust без trusted=yes/allow-unauthenticated и без global apt-key. Проверка source/key metadata и APT signature acceptance;rollback — восстановить прежние файлы если были,новые файлы отключить по change record,не трогать другие School21 repositories.

|Шаг / команда после своего approval|Ожидаемый результат / проверка|Rollback|
|---|---|---|
|D2 metadata: `apt-get update`|Signed metadata; network/global APT cache writes требуют approval. Проверить все используемые repositories;никаких insecure fallback|Сохранить ledger/FAIL;source можно отключить,не объявлять cache restore package rollback|
|`apt-cache policy docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin` и `apt-cache madison docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin`|Выбрать стабильные **точные** approved versions для всех5packages;apt policy доказывает origin candidates,не отсутствие CVE|Только inspection,нет package changes|
|D3 freeze: `apt-get -s install docker-ce=ENGINE_VERSION docker-ce-cli=CLI_VERSION containerd.io=CONTAINERD_VERSION docker-buildx-plugin=BUILDX_VERSION docker-compose-plugin=COMPOSE_VERSION`|Exact dependency closure,removal/downgrade proposal;любой неожиданный package/removal → STOP|Нет installation;review proposal заново|
|`apt-cache show PACKAGE=EXACT_VERSION` для каждого closure package|Record version,architecture,size,SHA256,repo filename;связать с validated signed Packages/InRelease,не доверять только локально вычисленному SHA|Нет изменений;pending hashes/sizes остаются UNKNOWN до actual metadata|
|D4 только после exact download approval: `apt-get --download-only install docker-ce=ENGINE_VERSION docker-ce-cli=CLI_VERSION containerd.io=CONTAINERD_VERSION docker-buildx-plugin=BUILDX_VERSION docker-compose-plugin=COMPOSE_VERSION`|APT signature/hash checks,package payloads admitted;ограничить утверждённым closure/byte budget,если solver меняет plan STOP|Не устанавливать quarantine;cache/evidence сохранить|
|`sha256sum -c APPROVED_CHANGE_DIR/approved-debs.sha256`|Все exact package bytes совпадают с **предварительно утверждённым** manifest,не manifest из тех же непроверенных bytes|При FAIL STOP;не чинить hash manifest ради PASS|
|D5 install: `apt-get --no-download install docker-ce=ENGINE_VERSION docker-ce-cli=CLI_VERSION containerd.io=CONTAINERD_VERSION docker-buildx-plugin=BUILDX_VERSION docker-compose-plugin=COMPOSE_VERSION`|Offline installation exact set;maintainer scripts могут запустить daemon/change network — это должно быть approved заранее|Сохранить package/service/network state;School21 выполняет reviewed uninstall/reinstall прежних packages,без purge/prune/volume deletion|

Без CA/curl/gpg или при missing transitive dependencies — новая exact acquisition proposal,не implicit apt fix/скачивание. `ENGINE_VERSION` и другие placeholders не разрешены для реального исполнения;epoch/revision должны быть полными. Package signatures/metadata не заменяют advisory review Docker/containerd/runc/BuildKit/Compose dependency scope.

## Engine acceptance до любого app image load

|Команда|Ожидаемое / проверка|Rollback|
|---|---|---|
|`systemctl is-active docker; docker version; docker buildx version; docker compose version`|Actual client/server/plugins и versions совпадают с admission;не запускать hello-world (это pull)|При FAIL сохранить evidence,не ставить другую версию автоматически|
|`docker context inspect --format '{{json .Endpoints.docker.Host}}'`|Только local Unix endpoint,не tcp/ssh remote daemon|Не менять global contexts;исправление endpoint отдельным решением|
|`docker info --format '{{.OSType}} {{.Architecture}} {{.NCPU}} {{.MemTotal}} {{.DockerRootDir}}'`|linux/amd64,≥4CPU,≥7GiB RAM,local real storage|Если VM/Engine mismatch STOP,не включать --local-qa на сервере|
|`stat -c '%d' ACTUAL_DOCKER_ROOT /srv/ai-business-assistant/shared; df -B1 /srv/ai-business-assistant/shared`|Одинаковый st_dev;≥70GiB filesystem и≥20GiB free;no symlink storage-root|Не менять daemon data-root/guards ради PASS;согласовать storage layout|
|`ss -lnt; stat /var/run/docker.sock`|Нет Docker TCP API2375/2376;socket только доверенным admins|Не создавать remote listener;права не ослаблять|
|`iptables-save; nft list ruleset; ufw status verbose`|Actual firewall backend и coexistence с School21 понятны;не flush Docker-managed chains|Если networking нарушен STOP,School21 возвращает captured state по approved plan|

Default image-store/config semantics выбранной версии должны пройти **реальное** future trusted OCI load/inspect round-trip с exact manifest/index/config IDs. Нельзя отключать containerd store/переписывать release manifest ради совпадения. Такой тест включает image load и выполняется только после release input approval,не сейчас.

[Docker firewall documentation](https://docs.docker.com/engine/network/packet-filtering-firewalls/) различает iptables/nftables backends;не переносить DOCKER-USER assumptions на native nftables. У Compose остаются0 published ports. Не выставлять iptables=false,не открывать Docker socket приложению/Zabbix,не добавлять пользователя в docker group автоматически. Docker operator — привилегированная trust boundary.

Global daemon log-limit/data-root/backend changes требуют review отдельного exact candidate: текущие per-service Compose log limits уже есть. Не перезаписывать daemon.json целиком. Rollback Docker не является database rollback;никаких `rm -rf /var/lib/docker`,`docker system prune`,`down -v`. После установки ещё нужны hardening/server verification и отдельный trusted-release GO.
