# Stage12C.3 — D2 execution plan & release recovery

Фактическая проверка: **2026-11-16**. Имя evidence-каталога `stage12c3-20261113` сохранено по запросу; оно не является датой выполнения.

**План подготовлен. Stage12C.2 остаётся SECURITY BLOCKED. PRE-VPS NOT READY; PRIVATE STAGING NO-GO; CUSTOMER PILOT NO-GO.** Нового release нет. Запрос на планирование D2 не является разрешением скачать D2. Никаких внешних запросов, загрузок, sudo, запуска Docker Desktop, SSH, commit/push или deployment на этом этапе не выполнялось.

## Проверенная база

Прочитаны D2_RELEASE_INPUTS_APPROVAL.md, STAGE_12C2_SECURITY_REMEDIATION.md, TRUSTED_RELEASE_PROVENANCE.md и Stage12C2 INDEX. Свежие локальные результаты находятся в [INDEX](../.local/stage12c3-20261113/INDEX.md). Прежние FAIL и OCI archives не менялись.

- Go1.26.11 linux/amd64 уже есть локально. Архив 66999944 bytes повторно проверен: SHA256 `9849f4e04ee35303714e34fbc3ac8ab44ea3e448a95aa182f91e3b9b79c710ec`, совпадает с D1. Проверка SHA не заменяет независимую подпись.
- Все **24 payload D2 отсутствуют в ожидаемом quarantine**, d2-owner-approval.json отсутствует. Сумма 254808036 bytes подтверждена по proposal.
- На диске около65.2GB свободно на момент проверки. Это не оценка полного размера rebuild: распаковка, caches, OCI exports и backup QA потребуют дополнительного места.
- Host Go1.26.2 darwin/amd64 не подходит под producer policy. Не понижать go.mod и не включать автоматическую загрузку toolchain.

## A. Уже доступно локально

|Компонент / источник|Назначение / локальная проверка|Команда следующего шага|Permissions / результат / откат|
|---|---|---|---|
|Go1.26.11, официальный D1 `https://dl.google.com/go/go1.26.11.linux-amd64.tar.gz`|66999944 bytes; SHA256 выше; Linux ELF, не host executable|`python3 .local/stage12c3-20261113/run_go_checks.py` после восстановления Docker|Обычный пользователь, workspace writes; ожидаются version/tests/race/vet/build logs. Ничего глобально не устанавливается; при FAIL не продвигать artifacts|
|Trivy0.76.2 / aquasecurity/trivy|Linux executable в Stage12C1/admitted-tools; binary hash сохранён в local-inputs.json; исходный архив52714130 bytes|Использовать прежнюю sandbox recipe `stage12c2-20261112/patched/scan_runtime.py` как основу нового isolated runner, с новыми inputs/output и свежей DB; старую recipe без review не запускать|Нужен Docker; нет socket в scanner container, RO inputs, no network. Результат — JSON findings, не автоматический security PASS; rollback — не принимать candidate|
|Syft1.57.1 / anchore/syft|Linux executable там же; hash сохранён; исходный архив31432061 bytes|Новая sandbox recipe по прежнему scanner-runner.json; `syft dir:/runtime -o syft-json=/output/syft.json -o cyclonedx-json=/output/runtime.cdx.json` внутри sandbox|Нужен локальный Docker, новый output; SBOM из фактического final rootfs; старый release не менять|
|Go module cache, go.mod/go.sum; npm lock|Cache существует; полнота и происхождение всех inputs не доказаны|`GOPROXY=off GOTOOLCHAIN=local go mod verify` внутри admitted Go runner, затем offline install после D2|Не разрешает missing module download. При несовпадении STOP; сохранить locks/snapshot|
|Cached Linux Go/race/Playwright carriers|IDs сохранены в patched/go-runner.json и cached-builders.json|После старта daemon — `docker --context desktop-linux image inspect IMAGE_ID` для exact IDs из этих файлов|Наличие в остановленном daemon сейчас **не проверено**. Cached origin не VERIFIED; missing image → отдельный acquisition proposal, не pull|
|Docker CLI29.9.2; установленный Docker Desktop|CLI работает; Desktop не запущен|См. Docker ниже|Без sudo/global config; запуск только после отдельного решения владельца|
|Python3, локальные release-control scripts|Подготовка manifest и аудит index|`python3 .local/stage12c3-20261113/audit_index.py`|Никакой сети; результат проверки blobs. Откат — сохранить evidence, не менять runtime|

Для каждого локального binary фактические размер/SHA256: `local-inputs.json`. Исторический bootstrap trust P2/D1 не расширен до independent publisher authentication. Ни один scanner сейчас не исполнялся; Linux tools на macOS напрямую не запускать.

## B. Требующие внешней загрузки — только после разрешения

Полные **24 строки с официальным URL, exact size/hash, permissions, command, expected result и rollback**: [d2-actions.json](../.local/stage12c3-20261113/d2-actions.json). Они ссылаются на исходные exact descriptors, не на mutable tags. Краткая группировка:

|Компонент / версия|Источник|Bytes|Проверка / назначение|
|---|---|---:|---|
|Next16.4.1|registry.npmjs.org/next/-/next-16.4.1.tgz|43677546|SHA512 SRI + ранее проверенная registry signature metadata; Next patch|
|@next/env16.4.1|registry.npmjs.org/@next/env/-/env-16.4.1.tgz|4833|SHA512 SRI; согласованная Next dependency|
|@next/swc-linux-x64-gnu16.4.1|registry.npmjs.org/@next/swc-linux-x64-gnu/-/swc-linux-x64-gnu-16.4.1.tgz|35231459|SHA512 SRI; amd64 native build dependency|
|Node24.22.3 linux-x64|nodejs.org/dist/v24.22.3/node-v24.22.3-linux-x64.tar.xz|32064292|SHA256 `4f2b175a6d53a5c617a80dba2676238a0688cef28c6da4eba35c7d844e66fcfb`; patched runtime|
|x/text v0.41.0|proxy.golang.org/golang.org/x/text/@v/v0.41.0.zip|7336337|Canonical H1 и go.mod H1 в D2; dependency patch|
|x/sync v0.22.0|proxy.golang.org/golang.org/x/sync/@v/v0.22.0.zip|25790|Canonical H1 и go.mod H1; заявленный module graph|
|PostgreSQL18.6/alpine3.23, nginx1.30.6 slim, Mailpit1.33.1 —18 уникальных layers|registry-1.docker.io, library/postgres, library/nginx, axllent/mailpit|136467779 вместе|Каждый layer SHA256+size; root/platform bindings из D2; runtime images|

Версии/official metadata здесь — сохранённые наблюдения12 ноября, не свежая проверка доступности или рекомендация о latest. Не делать fallback на другой tag/version при404.

Общая команда acquisition (НЕ выполнялась):

```sh
python3 .local/stage12c2-20261112/acquire_d2.py
```

Prerequisites: реальное явное разрешение владельца на точный D2 и trust должно быть записано в d2-owner-approval.json; не создавать фиктивное `approved:true`. Helper проверяет наличие записи до сетевых действий. Бюджет280MiB body, metadata<=8MiB, <=140 requests, только host allowlist из D2. Registry bearer tokens не сохранять; signed redirect query не логировать. Команда исполняет весь24-payload proposal. После size/hash PASS — карантин и admission record, затем безопасная распаковка и offline locks/install. Для npm lifecycle scripts выключены; дополнительные implicit tarballs/ZIP/OCI layers запрещены. Не применять общий npm/go update.

Откат для всей группы: не продвигать quarantine в release, оставить старые immutable archives; сохранить ledger и FAIL. Перед lock/recipe edits сохранить hash-bound snapshot. Вернуть только собственные targeted edits при отмене, не использовать broad git reset/clean и не удалять evidence. Нельзя откатывать security policy на старый Go ради сборки.

Дополнительно к D2:

|Компонент|Что неизвестно / verification|Permissions, command, expected result, rollback|
|---|---|---|
|Fresh Trivy DB schema2|Version/digest/size нового snapshot **UNKNOWN**; старая NextUpdate2026-11-13 истекла|Сначала отдельное разрешение metadata discovery, затем exact payload proposal. Не выполнять `trivy image` с implicit DB download. Admission нового DB через separate cache и SHA256 manifest/layer; stale DB сохранить, не перезаписывать|
|Go advisory DB для govulncheck|Актуальный snapshot, размер и exact hash **UNKNOWN**; Trivy DB его не заменяет|Отдельное разрешение metadata+payload, namespace vuln.go.dev; сохранить совместимый file-DB snapshot с digest. Без него scan NOT RUN|
|govulncheck v1.9.0 (`golang.org/x/vuln`)|Историческая версия; source ZIP/deps отсутствуют; размер и H1 до approved metadata **UNKNOWN**|Отдельный acquisition/trust approval; no `go install ...@latest`. После exact admission: offline `go build -mod=readonly -o /output/govulncheck ./cmd/govulncheck` в отдельном tool module, не application module. Результат — hashed tool binary и build log; rollback — не использовать tool, evidence оставить|

Список10 ранее выявленных govulncheck dependencies и source URL находится в Stage12C1/govulncheck-acquisition-requirements.json; по каждой из11 source/dependency позиций подготовлены источник, команда, permissions, verification и rollback в [govulncheck-actions.json](../.local/stage12c3-20261113/govulncheck-actions.json). Это candidate graph, а не разрешение загрузки или доказательство полного build-only closure. Сначала проверить фактическую closure и module H1/размеры. Не скачивать test-only modules без необходимости. Полная подпись/Fulcio/Rekor chain всё ещё требует решения о доверии/verifier.

## C. Системные права

**Необходимость sudo не установлена; сейчас таких действий нет.** Docker Desktop установлен; каталог socket принадлежит пользователю и доступен ему на запись. chmod/chown socket, добавление в группы, установка другого runtime, изменение Docker global settings, запуск privileged service и обход endpoint guards не нужны и не предлагаются.

Если обычный запуск Desktop покажет конкретную OS permission/error, сохранить его и подготовить отдельный запрос. Не трактовать membership в admin group как разрешение sudo.

## Docker: причина и восстановление

Свежие факты: CLI29.9.2 отвечает; context desktop-linux указывает на `unix:///Users/ilezkaradjev/.docker/run/docker.sock`; socket отсутствует. `/var/run/docker.sock` — symlink на тот же отсутствующий endpoint. В списке процессов найден только helper `com.docker.vmnetd`; Docker Desktop/backend/daemon процессов не найдено. Каталог socket имеет owner ilezkaradjev, mode0755, writable для текущего пользователя. Ошибка — ENOENT, не permission denied. **Непосредственная причина: локальный backend не работает и не создал socket. Почему Desktop остановлен, не установлено; crash не доказан.**

colima, limactl, podman, orb не найдены; дополнительные contexts кроме default/desktop-linux отсутствуют. Подтверждённой альтернативной разрешённой build environment нет. Remote Docker/School21 не использовать как fallback.

После отдельного решения владельца запустить установленный Desktop обычным пользователем (не выполнено):

```sh
open -a Docker
```

Проверить без изменения context/settings:

```sh
docker --version
docker context inspect desktop-linux
test -S "$HOME/.docker/run/docker.sock"
docker --context desktop-linux info --format '{{.ServerVersion}}'
```

Не выдавать успех `open` за daemon readiness. Проверить сохранность существующих containers/volumes до QA; не запускать старые app stacks. Запуск Desktop может возобновить containers по restart policy: владелец должен учесть это перед запуском. Откат — остановить только созданные этим этапом isolated QA resources; не останавливать чужие containers и не выполнять Docker reset/prune. Desktop shutdown — отдельное решение владельца.

## Security и точные команды следующего шага

Свежий source review: `Email` в security.go отклоняет пустой/нормализованный >254-byte input и non-ASCII **до** ParseAddress. Единственный direct net/mail parser в исходниках приложения. CVE-2026-39820 / GO-2026-4986 и CVE-2026-42499 / GO-2026-4977 по сохранённым официальным advisories исправлены с1.26.3/1.25.10; выбран1.26.11. go.mod, check-go.sh и package.py требуют1.26.11. Это source/producer review, **не свежая runtime verification**. Короткий ASCII всё ещё достигает parser; admission gate не заменяет patched toolchain.

После восстановления локального Docker:

```sh
python3 .local/stage12c3-20261113/run_go_checks.py
```

Подготовленный runner проверяет exact local endpoint, отвергает DOCKER_HOST/DOCKER_CONTEXT overrides, требует exact compiler version, использует `--pull=never --network none --read-only`, readonly source/module cache, capabilities dropped, новый output/cache на каждый запуск. Использует прежние exact carrier IDs. Сам runner сейчас только syntax-checked; Docker execution NOT RUN. Его команды внутри Linux runner:

```sh
go version
go test -count=1 ./...
go test -count=1 -run TestEmail ./internal/auth ./internal/http
go vet ./...
# Для каждого part: api worker migrate healthcheck runtime-entrypoint:
go build -trimpath -o /artifacts/api ./cmd/api
# Отдельный C-toolchain carrier, CGO_ENABLED=1:
sh /scripts/build-env/race.sh
```

Окружение: GOTOOLCHAIN=local, GOPROXY=off, GOSUMDB=off, GOVCS=*:off, GOFLAGS=-mod=readonly; GOROOT=/approved-go. Повторно сверить admitted toolchain/material hashes перед execution. Любой missing input — STOP, не download.

govulncheck — команда внутри отдельного offline Linux sandbox **только после admission tool+совместимого snapshot**:

```sh
GOVULNDB=file:///approved-advisory-db /approved-tools/govulncheck -json ./...
```

Mount application source RO в `/src`, module cache RO, approved tools/DB RO; network none, GOPROXY=off, GOTOOLCHAIN=local. Сохранить stdout/stderr/exit и coverage/tool versions. Сейчас обе prerequisites отсутствуют; команда не заявляется выполненной. Успешные unit tests не доказывают отсутствие CVE.

## GitHub: проверка и exact commit scope

Git публикация на этом этапе **запрещена текущим запросом**, прежнее разрешение на push не используется. Remote не опрашивался. Index исходно содержал459 файлов, включая3 `.pyc`: `infrastructure/__pycache__/demo-localhost.cpython-314.pyc`, `scripts/deployment/__pycache__/backup.cpython-314.pyc`, `scripts/deployment/__pycache__/test_backup.cpython-314.pyc`. Все три убраны **только из index**, файлы сохранены на диске. `.gitignore` дополнен `__pycache__/` и `*.py[cod]`; эта правка ещё не staged.

Свежий scan оставшихся456 staged blobs: zero checked secret-pattern matches, zero запрещённых artifact/env paths, zero files>10MiB. `.env.example` оставлен как synthetic template. Два `.xlsx` разобраны как ZIP/XML: DEMO/SYNTHETIC fixtures, не клиентский export. SQL-файлы — migrations/init scripts. Локальные dumps/backups, `.local`, `.env`, private keys, certificate serial, node_modules/builds не включены. Heuristic scan не равен полноценному secret scanner; отсутствие любых реальных клиентских данных по одному regex доказать нельзя. Финальный human data/privacy review остаётся необходимым перед будущей публикацией. В логах нет найденных credential values.

Exact текущий index: `commit-files-index.txt`, content hashes/size — `github-index-audit.json`. **Предлагаемый commit scope:457 файлов** (456 оставшихся путей с актуальными working-tree bytes, включая обновлённый .gitignore, плюс этот новый план). Полный перечень — `commit-files-proposed.txt` и content hashes — `commit-files-proposed.json`. Приватный INDEX/evidence не публикуются. Проверить повторно при любых изменениях; manifest не является commit/attestation.

Будущая подготовка index, только в отдельно разрешённом publication этапе:

```sh
git add --pathspec-from-file=.local/stage12c3-20261113/commit-files-proposed.nul --pathspec-file-nul
git diff --cached --check
python3 .local/stage12c3-20261113/audit_index.py
```

Сейчас эти staging-команды не выполнены; commit/push не выполнены. Имя/email автора не настроены по прежнему review и не выдумываются; решение владельца потребуется только при разрешённой публикации. CI остаётся manual workflow_dispatch; сеть/Actions не запускались.

## School21 — отдельный будущий read-only checklist

SSH не выполнялся. Перед будущим доступом получить разрешённые host/user/port, fingerprint host key из независимого канала, перечень разрешённых путей и read-only audit scope. Не принимать неизвестный host key автоматически; не отправлять credentials в prompts/logs. Не устанавливать tools, не читать .env/private keys/backup payloads и не запускать deployment.

После разрешённого входа обычным пользователем:

1. `id`, `uname -srmo`, `cat /etc/os-release`, `date -u`, `uptime`: OS/arch/time/identity; версия kernel не доказывает applied security updates.
2. `df -h`, `free -m`, `findmnt -o TARGET,SOURCE,FSTYPE,OPTIONS`: ёмкость/память/mounts; записывать только допустимые metadata.
3. `systemctl is-active docker`, `docker --version`, `stat /var/run/docker.sock`: daemon/tool/socket. Если нет прав — BLOCKED, без sudo/group changes. Docker socket даёт широкие возможности: не выдавать read-only SSH account за безопасный Docker API доступ.
4. `ss -lnt`, `systemctl is-active ssh`, `systemctl is-active ufw`: слушающие порты/статус, без changes. Firewall rules/sshd effective policy, недоступные обычному пользователю, запросить у администратора как redacted export; не запускать sudo.
5. По approved application path — `stat` директорий, `sha256sum` manifest/OCI archive, labels/image IDs через явно разрешённые formatted inspect fields. Никогда не выводить полный docker inspect/compose config/env. Проверить architecture, runtime flags, nonroot/read-only policy по redacted metadata, без старта сервиса.
6. По approved backup path — только ownership/mode/mtime/size и approved checksum manifest. Проверить custody/retention/ACL и существование rehearsal evidence; не читать dump, не restore и не test-write на сервере.
7. TLS/domain/renewal, patch policy, SSH access/MFA, firewall, monitoring и restore ownership сверить с администратором по redacted evidence. Не запускать внешние network probes без отдельного scope.
8. Итог — gaps/permissions и hashes отчёта; любой deployment, migrations, restart, backup/restore rehearsal — отдельный этап. Без недоступных evidence не заявлять server READY.

## D. Решения владельца и минимальный critical path

1. **Docker:** разрешить обычный запуск существующего Desktop с учётом restart policies; без переустановки/settings/remote fallback.
2. **D2:** явно принять exact proposal и HTTPS+hash/SRI/H1 bootstrap,254808036 bytes, существующие ceilings/hosts. Новые версии/hosts вне proposal — STOP.
3. **Metadata и scanners:** разрешить bounded discovery fresh Trivy/Go advisory DB и govulncheck metadata, затем отдельный exact payload approval после размеров/hash. Suggested discovery budget: <=30 HTTPS GET/HEAD, суммарно<=4MiB, official ghcr.io/api.github.com/raw.githubusercontent.com (aquasecurity namespaces), vuln.go.dev, proxy.golang.org, sum.golang.org; только named advisories/tool module/declared dependency closure. Registry auth token endpoint — отдельное явное включение при необходимости. Никаких binary/layer/module ZIP payload на metadata-only approval.
4. **Provenance:** решить, достаточен ли approved source snapshot или нужен immutable Git revision и independent signer/reviewer. По прежнему trusted plan нужен approved Git commit; текущий запрет commit оставляет этот gate открытым. Unsigned local statement не является independent attestation.
5. **GitHub и School21:** отдельное последующее разрешение; сейчас не являются prerequisites локального source remediation review. Не объединять их с download approval.

После решений1–3: fresh Go checks и exact input admission → targeted locks/recipes → isolated fresh build и SBOM/Trivy/govulncheck → review findings/material trust → отдельный новый five-image release manifest/provenance → обязательная regression matrix. API/web build, а vendor images acquisition/derivation или source rebuild должны быть честно различены: перенос pinned upstream bytes не объявлять собственным rebuild из upstream source. Проверить все пять digest subjects и locks. Не использовать legacy mutable BASES из package.py без review.

Затем30 startup refusals+all flags, release integrity/GO/remote rejection, isolated migrations/FORCE RLS/tenant/concurrency/Excel, frontend lint/typecheck/fresh build, Browser9/9, Catalog2/2, SQL, backup/restore, rollback/roll-forward, HTTPS smoke. Сейчас все post-rebuild gates **NOT RUN**, E01 и SQL causal attribution OPEN. Security findings/provenance gaps могут потребовать дополнительного patch cycle; D2 сам по себе не обещает clean release.

Оценка как план, не SLA: Docker diagnosis после запуска15–45мин; metadata/admission1–3ч плюс ожидание approvals; offline lock/recipe/build3–6ч; scans/triage2–6ч; full regression/recovery4–8ч; provenance/review1–3ч. Итого порядка12–27 инженерных часов после разрешений, при достаточных caches и отсутствии новых security FAIL; независимый review и новые acquisition/patch циклы оцениваются отдельно.

## Validation этого этапа

PASS: локальная Docker диагностика; повторный D1 archive hash; сумма/отсутствие D2 payloads; staged blob audit после удаления3 `.pyc`; синтаксис подготовленного Python runner; проверка whitespace index/working diff. FAIL первого inline audit сохранён в audit-initial-error.log: index менялся во время чтения; стабильный повтор завершён отдельно. Никакие результаты приложения из этого не выводятся.

BLOCKED: Docker daemon; свежая runtime проверка net/mail; trusted rebuild. NOT RUN: Go tests/race/vet/build, govulncheck, fresh Trivy/Syft execution, server audit, commit/push/deployment. Исторические PASS Stage12C2 здесь не переименованы в свежие.
