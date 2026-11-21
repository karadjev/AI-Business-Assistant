# Stage12E.3 — server hardening & infrastructure execution report

Фактическая работа: **2026-11-16**. Target **176.57.214.93:22**.

## Вердикт

**BLOCKED BEFORE SSH CONNECT.** Не SERVER HARDENED,не DOCKER READY,не PREPARED FOR TRUSTED RELEASE TRANSFER. Подготовленная документация и локальный option-parsing PASS не являются server hardening PASS.

Владелец подтвердил личное владение VPS и разрешил системные изменения при prerequisites. Повторное общее разрешение не требуется. Блокер — **независимо подтверждённый host fingerprint и доступ/recovery**,а не прежний read-only scope. В запросе прямо предписано остановиться до подключения без доверенного fingerprint. Это условие соблюдено:никаких SSH connect/ssh-keyscan/target probes не выполнялось.

## A–F: выполнено и остановлено

|Этап|Фактическое действие|Статус|
|---|---|---|
|A local repository preparation|Прочитаны действующие AGENTS.md и перечисленные10 документов (с учётом12C.3);SSH_TRUST_READINESS.md отсутствует;исходные3редактируемых документа сохранены до edits|DONE для локальной подготовки|
|B SSH trust verification|Проверены local known_hosts,SSH-agent,видимые local established connections;offline `ssh -G` проверил строгие опции|BLOCKED independent trust;remote auth/recovery NOT RUN|
|C server hardening|aba-admin,ограниченный sudo,ротация root password,SSH/firewall/base system не применялись|NOT RUN:gate B|
|D Docker installation|Сохранён официальный APT plan;не выполнялись metadata/package downloads/install|NOT RUN:SSH hardening prerequisite|
|E storage preparation|Существующий layout/permissions plan сохранён;каталоги на сервере не создавались|NOT RUN:gate B и последовательность C/D|
|F release transfer readiness|Повторно проверены только наличие D2 approval/payload paths,без повторения scans/test suites|BLOCKED отдельно:24missing D2 inputs,нет trusted release|

## Реальные локальные наблюдения

- Применим корневой AGENTS.md;других AGENTS.md в active source scope не найдено. Snapshot-копии не являются инструкциями рабочего дерева.
- В local known_hosts есть3записи target:ED25519,RSA,ECDSA. **Нет evidence независимой проверки их происхождения.** Локальный ED25519 observation:`SHA256:AmEMDMCgk3sPDzEDPIQLLYtD9xTyT6OqBvzUMo2V6sM`. Это публичный fingerprint-наблюдение,**не trust approval и не console result**. known_hosts не менялся.
- `ssh-add -l`:exit1,identity list пуст. Это состояние agent,не доказательство отсутствия key на диске или отсутствия server authorization. Private key files не открывались;ключ не генерировался и в agent не загружался.
- `lsof -nP -iTCP@176.57.214.93:22 -sTCP:ESTABLISHED`:exit1,0видимых текущему macOS user matching rows. Это не проверка всех серверных SSH sessions;пользователь может иметь console/session в другом приложении/host.
- `ssh -G -F /dev/null ...`:exit0,offline option parsing PASS. Проверены BatchMode,PasswordAuthentication=no,KbdInteractiveAuthentication=no,PreferredAuthentications=publickey,StrictHostKeyChecking=yes,HostKeyAlgorithms=ssh-ed25519,IdentitiesOnly=yes,ForwardAgent=no,UpdateHostKeys=no. Pinned host-file и key paths в этом тесте **placeholders**,их существование и работа не подтверждены. Сеть не использовалась.
- Не запрашивались private keys,passwords,tokens. Значение опубликованного root password не извлекалось,не использовалось,не сохранялось и не хешировалось.

Полные local command/log/exit records и hashes:[private INDEX](../.local/stage12e3/INDEX.md). Публичный документ не содержит исходных секретов;локальные known_hosts observations не повышены до независимой attestation.

## Credential exposure и безопасный следующий шаг

По сообщению владельца старый root-пароль раскрыт. Он считается **скомпрометированным**,даже если пока нет доказанных посторонних successful logins. Не ждать hardening SSH через этот пароль. Владелец должен использовать уже доверенную provider/School21 console,немедленно сменить root password интерактивно (`passwd root`),не передавать новый пароль в чат/командные аргументы и проверить emergency access. Это указание владельцу,не выполненное действие агента. Смена секрета не отменяет incident review. Не блокировать root/не отключать recovery до проверенного aba-admin access;никогда не откатываться на раскрытый пароль.

**Следующая точная read-only команда — только в независимо доверенной server console:**

```sh
ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub -E sha256
```

Нужны результат SHA256 и подтверждение источника (trusted console/control panel),working recovery console,bootstrap SSH username и путь к уже разрешённому local key либо согласованный путь его enrollment через console. Пароль/private key не присылать. По SSH не выполнять даже эту read-only команду до trust gate. Если console key не совпадает с local observation — STOP,разобрать provider identity/reinstall/rotation;не `ssh-keygen -R` и не StrictHostKeyChecking=no ради подключения.

После независимой сверки закрепить только проверенный ED25519 public host key в отдельном private known_hosts file для этой работы,сохранив прежние файлы. Затем single key-only probe (НЕ выполнен;paths/user заполняются после trust):

```sh
ssh -F /dev/null -o BatchMode=yes -o StrictHostKeyChecking=yes \
  -o UserKnownHostsFile=VERIFIED_KNOWN_HOSTS_FILE -o GlobalKnownHostsFile=/dev/null \
  -o HostKeyAlgorithms=ssh-ed25519 -o UpdateHostKeys=no \
  -o PasswordAuthentication=no -o KbdInteractiveAuthentication=no \
  -o PreferredAuthentications=publickey -o IdentitiesOnly=yes -o ForwardAgent=no \
  -o ConnectionAttempts=1 -o ConnectTimeout=10 \
  -i APPROVED_PRIVATE_KEY APPROVED_USER@176.57.214.93 'id -u'
```

Это не proof recovery:после bootstrap нужны separate aba-admin session и утверждённая privilege operation,School21 access check и out-of-band rollback. Если required PAM MFA — согласовать поддержанную publickey+MFA chain,не включать старый root-password fallback. Credential path/authorization пока не известны,команду сейчас не выполнять.

## Системные состояния и acceptance

|Пункт final acceptance|Результат и provenance|
|---|---|
|1. Реально выполнено|Local discovery/doc review,pre-change backups3docs,SSH metadata inventory,offline strict-policy parsing,D2 path-presence check,local evidence validation/seal|
|2. Системные настройки|**Не изменены** ни на VPS,ни global local SSH/Docker;только required repo docs и private evidence|
|3. Тесты|PASS только local option parsing,document/link/whitespace checks,evidence hashes и protected-file preservation. Server/application tests NOT RUN|
|4. BLOCKED|Independent fingerprint,actual key login,recovery,server preflight;далее admin/SSH/firewall/Docker/storage. Release — отдельный blocker|
|5. SSH recovery|**NOT VERIFIED**. Provider console и альтернативный admin login не проверены;старый пароль не recovery guarantee|
|6. Docker|Отсутствует **по владельцу**;actual daemon/socket/plugins/firewall behavior НЕ ПРОВЕРЕНЫ. DOCKER READY не выдан|
|7. Firewall|UFW inactive **по владельцу**;host nft/iptables/external School21 firewall неизвестны. Правила не изменены;CIDR не предположены|
|8. Zabbix|Работает **по владельцу**;Server/ServerActive/TLS/listener/network allowlists неизвестны. Не отключён/не перезапущен|
|9. Storage|Actual каталоги/owners/free-space/mounts UNKNOWN;ничего не создано/не удалено;current pointer не установлен без release|
|10. Release compatibility|Проектные no-published-ports/internal networks/resource/UID contracts сохранены;native Engine/load/UID/storage compatibility NOT RUN. Transfer BLOCKED|
|11. Следующая команда|Console `ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub -E sha256`;SSH probe только после independent verification|

## Что изменено в проекте

Только targeted updates:SCHOOL21_SERVER_HARDENING.md фиксирует новое разрешение,root credential exposure,обязательный trust gate,MaxAuthTries3 и включение необходимых School21 identities в user restrictions;SCHOOL21_DOCKER_INSTALLATION.md и SCHOOL21_DEPLOYMENT_CHECKLIST.md различают granted system authorization и невыполненные access prerequisites. Создан этот report и private INDEX/evidence. Предыдущие версии3документов сохранены;старые evidence не перезаписаны. Dockerfiles/Compose/migrations/deployment scripts не изменены. Git commit/push не выполнялись.

## Невыполненные prerequisites следующих фаз

После trust/key/recovery:снять server baseline и private backups SSH/UFW/Zabbix/apt/log config;review successful logins с момента возможного раскрытия секрета и доступный retained interval,incident decision;проверить School21 management/monitoring addresses и console rollback. Только затем aba-admin с limited privilege policy,second session,sshd syntax/effective contexts,SSH reload и new-session tests. Нет готового безопасного AllowUsers/CIDR list —нет автоматического сужения доступа. HTTPS/HTTP не открывать для данного private deployment без отдельной необходимости;Postgres/internal ports всегда закрыты. Zabbix сохраняется.

Base-system update/NTP/journal/free-space/swap/limits audit требует server evidence;Ubuntu release upgrade запрещён. Docker — официальный APT,key/version/size/hash verification,без convenience installer и automatic docker-group membership. Storage создаётся только после baseline/no-overwrite check сprivate modes и same-filesystem guard. Resource baseline остаётся4.25GiB steady container memory (+0.5GiB migrate),2GiB planning OS reserve,CPU ceilings4.5на4vCPU;actual OOM/capacity не проверены.

## Trusted release blocker сохраняется

Stage12C.2/12C.3 требуют нового approved release:net/mail CVE-2026-39820/CVE-2026-42499 source admission+Go1.26.11 не обновляют старые images;fresh Go unit/race/vet/build/govulncheck,targeted regressions,fresh Trivy DB,SBOM/native coverage,provenance/digests/integrity manifest обязательны.24D2payload paths отсутствуют,approval record не найден. Старый Trivy DB NextUpdate2026-11-13 истёк. E01 writer и historical SQL cancellations attribution OPEN;позднийPASS не causal closure. Ограниченные private risk decisions не заменяют устранение confirmed security blockers и проверку origin.

Ни image transfer,ни customer deployment,ни production DB changes,ни live/provider/budget changes не выполнялись. SERVER HARDENED / DOCKER READY / PREPARED FOR TRUSTED RELEASE TRANSFER **не объявлены**. Общий результат **BLOCKED**,локальная подготовка завершена до честной границы доступа.
