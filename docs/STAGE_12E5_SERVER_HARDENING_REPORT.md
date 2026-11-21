# Stage12E.5 — privileged preflight: текущее продолжение

**17 ноября 2026, MSK. PRIVILEGED PREFLIGHT / SAFE RESTORE REHEARSAL: PASS. HARDENING: NOT RUN по STOP CONDITION.** Firewall readiness остаётся BLOCKED по неизвестной внешней policy и непроверенным стабильным admin ranges. Это продолжение существующего Stage12E.5; новый этап не создавался.

Предыдущая блокировка sudo действительно снята **интерактивной аутентификацией в отдельном Terminal**, а не переносом timestamp из другого SSH-сеанса. Пароль вводился только в sudo TTY: скрипты не получают его как аргумент/переменную, агент не извлекал и не передавал пароль из переписки, инструменты/helper не сохраняли TTY ввод в evidence. Первоначальный новый `sudo -n sshd -T` exit1 сохранён. Разрешение на preflight не расширялось до применения hardening.

## Что реально выполнено

|Проверка|Фактический результат|
|---|---|
|Привилегированный preflight|PASS: код реально исполнялся root через интерактивный sudo; защищённые config/firewall/log queries завершены|
|Include graph|Main `/etc/ssh/sshd_config`, Include на строке12 `/etc/ssh/sshd_config.d/*.conf`; единственный matching файл `50-cloud-init.conf` содержит `PasswordAuthentication yes`; активных Match blocks не найдено|
|Исходная конфигурация|Полные оригиналы сохранены в root-only backup; в локальный audit экспортированы metadata, SHA256 и разрешённые directives. Комментарии/произвольные аргументы скрыты; host private keys не копировались|
|Effective settings|RootLogin yes; PasswordAuthentication yes; PubkeyAuthentication yes; KbdInteractiveAuthentication no; MaxAuthTries6; Port22. Final global и контексты aba-admin/root для текущего peer проверены|
|Backup/restore|PASS: GNU tar с numeric owners, permissions, ACL/xattrs; независимая extraction; mismatch UID/GID/mode/kind/hash=0. В actual source set symlinks нет: их наличие не выдумано|
|Изолированная syntax validation|PASS: restored-tree Include paths перепривязаны к отдельной syntax-copy; `sshd -t -f` exit0. Pristine restored files не переписывались|
|VPS watchdog|PASS: реальный root-owned transient systemd service, ready PID7003; только его fixture process принудительно завершён SIGKILL; service автоматически перезапустился сPID7016,NRestarts1; fixed monotonic deadline восстановил fixture, `sshd -t` восстановленного fixture exit0|
|Рабочие SSH файлы|Не менялись. Финальная root-проверка подтвердила совпадение всех исходных metadata/hashes с backup|
|Reload неизменённой конфигурации|Выполнен **один** `systemctl reload ssh.service`, exit0; restart не выполнялся. MainPID1162 сохранился. Текущий SSH helper продолжил работу после reload; новая независимая key-only SSH connection PASS|
|Zabbix|active/running до и после, User=zabbix; не останавливался, не перезапускался, config/ListenIP не менялись|
|UFW/nft/iptables|Fresh privileged UFW:inactive. nft/iptables/ip6tables snapshots пусты, exit0. Это не доказательство отсутствия perimeter ACL/NAT вне VM|
|Authorized keys|aba-admin .ssh UID/GID1000,0700; authorized_keys UID/GID1000,0600; key fingerprint совпадает с local authorized identity|
|Updates/NTP|APT periodic security policy/timers проверены, NTP synchronized. Пакеты/indices не обновлялись; актуальность всех security patches не объявлена|

## Сохранённый FAIL и адресная проверка

Первый root-helper завершился exit1 после immediate state check: `ActiveState=reloading`, `SubState=reload-notify`. Успешный return reload ещё не означал завершение notify-перехода. Исходный результат `result-poll-1.json`, ошибка и Terminal exit1 **сохранены**, не переписаны как PASS.

Выполнены отдельные, не повторяющие reload проверки: fresh key reconnect, `active/running`, тот же MainPID; затем отдельный read-only privileged finalizer с bounded readiness wait, `sshd -t`, global/context effective settings, archive/source hash comparisons и Zabbix/listeners. Finalizer exit0, все сохранённые settings совпали. Исходные файлы не «исправлялись» ради PASS. Рабочий SSH transport не был принудительно закрыт; завершались только собственные тестовые SSH-команды. Чужие root/aba-admin sessions не завершались.

## Backup и восстановление

Private root-owned каталог0700:

```text
/var/backups/ai-business-assistant/aba-stage12e5-preflight-20261116T211623Z
```

Архив `ssh-config.tar` создан с umask0077; SHA256:

```text
defe7d734a8ac0826912dcc2c0a3e261c9351c29f174afd960fc55a75ae58421
```

В source manifest входят `/etc/ssh` и `sshd_config.d` с0755, main file0644 и `50-cloud-init.conf`0600; все UID/GID0. Точные file hashes и original metadata сохранены в `original-manifest.json` и private local result. Owner/mode/hash после extraction и после unchanged reload совпали; archive digest финально проверен повторно. Raw configuration backup остался на VPS вне публичных каталогов; passwords/private host keys не экспортировались. Root-only `sanitized-result.json` сохраняет первый FAIL, `finalize-result.json` — успешную последующую проверку.

Watchdog fixture и syntax trees находятся только внутри этого backup job. Рабочий `/etc/ssh/sshd_config` не был rollback target в данном тесте. Production rollback после применения нового hardening **не заявляется выполненным**. Доказаны безопасные части: root backup/extraction, реальные supervision/restart/deadline, восстановление файла, restored syntax, unchanged live reload и independent reconnect. Reboot/host failure/out-of-band console этим тестом не покрываются. Собственный transient fixture unit остановлен после проверки; persistent service/timer не устанавливались.

## Authentication log evidence и residual risk

Root journal query:6324 records в доступной выборке за7дней, фактические первые/последние records **2026-11-16 18:10:50–21:28:39UTC**. Найдены1428 `Failed password`,936 `Invalid user`,20 `Accepted publickey`,9 `Accepted password`; все9 password-success records относятся кroot. Это event counts, категории могут пересекаться; их нельзя суммировать как уникальные попытки. Ограниченная выборка не доказывает полноту retention.

Raw SSH journal MESSAGE и парольные значения не записывались в local evidence. **Принадлежность9 успешных root-входов владельцу пока не установлена.** Это не автоматическое доказательство взлома и не основание объявлять хост чистым после смены пароля. Перед более широким использованием нужен приватный incident/owner review этих событий и recovery decision.

Host trust остаётся continuity ранее закреплённого ED25519 key; independent identity attestation отсутствует. Использование интерактивного sudo не устраняет residual initial-enrollment/host-compromise risk. Ключи/пользователи/пароли не менялись.

## Firewall: точные доступные сведения и неизвестные ограничения

Fresh Zabbix config подтверждает только application Server allowlist:92.53.116.12,92.53.116.111,92.53.116.119; listener0.0.0.0:10050 и DenyKey=system.run[*]. Это не равно host firewall allowlist или подтверждению поступления метрик на внешний server. Actual external polling/School21 acceptance остаются NOT VERIFIED. Kernel route lookup для всех3адресов: via176.57.214.1,dev eth0,src176.57.214.93;пакеты к monitoring hosts не отправлялись. Это исходящий route,не proof inbound reachability/NAT. Перевод listener наprivate eth1 без дополнительной проверки совместимости не обоснован.

Host UFW inactive; nft/iptables snapshots пусты. Есть public eth0 и private eth1, routes сохранены. School21/provider ingress ACL, NAT/VPN restrictions и стабильные административные ranges из этих данных **не выводятся**. Наблюдаемые peer IP не объявлены allowlist. Поэтому UFW/ListenIP не менялись. PostgreSQL/Docker API/internal ports не открывались; Docker не устанавливался. Docker DNAT/forwarding compatibility — отдельный будущий gate, сейчас native Docker отсутствует по baseline и не тестировался.

## PASS / FAIL / BLOCKED / NOT RUN

|Статус|Содержание|
|---|---|
|PASS|Fresh privileged audit, Includes/effective config, backup/metadata/hash integrity, isolated restore syntax, real VPS watchdog restart/deadline test, unchanged reload с последующим readiness/reconnect, Zabbix local state, source preservation|
|FAIL — сохранён|Первый noninteractive sudo probe exit1; initial root-helper immediate reload-notify check exit1. Поздние результаты не удаляют эти записи|
|BLOCKED|External School21 network policy/stable admin ranges, внешнее monitoring подтверждение, attribution root password successes; применение hardening требует следующей подготовленной candidate validation и настоящего production guard/commit protocol|
|NOT RUN по scope|Отключение root/password SSH, установка hardening candidate, UFW enable, Zabbix changes, Docker install, package/security upgrades, deployment, live, Git push|

## Точная следующая команда — приватный review root logins

До применения hardening владелец должен сопоставить9 успешных root/password events со своими сеансами. Команда только читает журнал; actual passwords в этих accepted-event records не содержатся. Вывод держать приватным:

```sh
sudo journalctl -u ssh.service --since '2026-11-16 18:10:00 UTC' --until '2026-11-16 21:29:00 UTC' --no-pager --grep='Accepted password for root '
```

Если источник/время хотя бы одного входа не объяснены, сначала incident review, а не объявление сервера чистым после hardening.

### После review — подготовка candidate без применения

Создан и загружен SHA256-verified helper, который проверяет source metadata/hash и неизменность Include inventory, создаёт **новый private candidate** рядом с backup и выполняет его syntax/effective validation. Он не пишет в `/etc/ssh`, не вызывает reload и не включает UFW. Сам helper пока **NOT RUN**:

```sh
sudo /usr/bin/python3 /home/aba-admin/aba-stage12e5-preflight-20261116T211623Z/prepare_candidate.py
```

Пароль вводится только в доверенном терминале. После candidate validation остаются review diff и expected contexts, привязка production guard к **одному** изменяемому main file, проверка живого guard/срока/commit и план последующего key/sudo/root/password-negative checks. Применение SSH hardening требует отдельного продолжения: текущая STOP CONDITION его запрещает. Простая смена заголовка отчёта не заменяет эти проверки.

Root preflight потребовал создания только private helper directory в home aba-admin, root backup/fixture directories и transient **тестового** unit; бизнес-приложение, Compose, Dockerfiles, migrations, project data не менялись. Все hashes текущих evidence — в [Stage12E.5 INDEX](../.local/stage12e5/INDEX.md). Старый запуск и прежние FAIL сохранены ниже как история.

---

## История исходного Stage12E.5 до данного продолжения

Ниже дословно сохранён прежний отчёт: его sudo BLOCKED отражает **тот запуск**, а не текущую доступность интерактивного sudo. Устаревшая «следующая команда» исторического текста не является текущей инструкцией.

# Stage12E.5 — autonomous SSH recovery & hardening

Завершение локальной работы: **17 ноября 2026,MSK**. Fresh server evidence:16 ноября 2026,20:58–21:06UTC. Target176.57.214.93:22,user aba-admin,key `~/.ssh/aba_school21`.

**Финальный статус: BLOCKED.** Key login и independent reconnect — свежие PASS. Полный SERVER AUDIT PASS и HARDENING PASS не выданы:password-required sudo недоступен неинтерактивному процессу,часть конфигурации/firewall/logs недоступна. SSH/firewall/Zabbix/system settings не изменялись.

## Phase1 — реально выполненный remote read-only audit

Созданы три последовательных независимых SSH process:initial identity probe,основной collector,дополнительный audit/reconnect. Все exit0;ControlMaster reuse не применялся. Использованы `-F /dev/null`,BatchMode,publickey-only,IdentitiesOnly,StrictHostKeyChecking=yes,ED25519-only,отдельный target-only known_hosts,UpdateHostKeys=no,ForwardAgent=no. Парольная аутентификация не использовалась;локальный ключ не копировался на сервер. Пароли/private keys/PSK не запрашивались и не выводились.

|Проверка|Fresh result / ограничения|
|---|---|
|hostnamectl/id|Ubuntu24.04.5 LTS,x86-64,KVM,kernel6.8.0-149-generic;UID1000 aba-admin,группа sudo|
|SSH service|active/running,CanReload=yes;ssh.socket inactive(exit3),не daemon failure|
|SSH listeners|0.0.0.0:22 и[::]:22;видны listener10050 и local resolver/DHCP sockets;5432/2375/2376/3000/8080 не обнаружены в snapshot host namespace|
|Sudo|`sudo -n true` и`sudo -n -l`:exit1,`sudo: a password is required`. Группа sudo не заменяет authentication|
|Effective sshd|Unprivileged `sshd -T`:exit1,`/etc/ssh/sshd_config.d/50-cloud-init.conf: Permission denied`;sudo variants также denied|
|SSH files/Includes|Доступный main file:root:root0644,Include `/etc/ssh/sshd_config.d/*.conf`,PermitRootLogin yes,KbdInteractiveAuthentication no,UsePAM yes. Cloud-init Include прочитать нельзя;полная effective policy UNKNOWN|
|Authorized key|Home .ssh UID/GID1000,0700;authorized_keys UID/GID1000,0600. Fingerprint совпал с local key:`SHA256:HMWo5ogyiREezdPuJCWYxAOrqUn3DAh/mBJubj/sn0k`. Fresh key authentication PASS|
|Независимый reconnect|PASS во втором отдельном audit SSH process,UID/group и public key повторно проверены;это не console recovery/привилегированный login|
|Сессии|who показывает 11TTY records:7root,4aba-admin;TCP snapshot содержит 13established SSH connections. TCP ESTABLISHED не равно authenticated session. Принадлежность старых root sessions не подтверждена;не завершались/не использовались агентом|
|UFW/nft/iptables/ip6tables|Privileged queries exit1,password required. UFW inactive — только supplied baseline;actual effective firewall UNKNOWN,правила не менялись|
|Zabbix service/listener|active/running,User=zabbix,MainPID813 в snapshot;listener0.0.0.0:10050. Не останавливался/не перезапускался|
|Zabbix config|Fresh root-owned0644config:Server92.53.116.12,92.53.116.111,92.53.116.119;StartAgents3;DenyKey=system.run[*]. External monitoring continuity,TLS actual defaults/version и совместимость ListenIP change не доказаны|
|Zabbix package inventory|dpkg-query не подтвердил установленный zabbix-agent/agent2 package;service существует. Это packaging/origin gap,не доказательство остановленного monitoring|
|Interfaces/routes|eth0 public176.57.214.93/24,eth1 private10.18.4.5/24,IPv6 link-local. Default route через 176.57.214.1;public/private routes сохранены в private evidence. Это не подтверждает стабильный admin CIDR|
|CPU/RAM/disk|4CPU;RAM8325492736bytes(≈7.75GiB),available7845195776bytes;swap0. `/srv` и`/var/lib` на/dev/sda1,total123690532864,available121529217024bytes(≈113.18GiB)|
|unattended-upgrades|Package2.9.1+nmu4ubuntu1 установлен;APT periodic update/unattended1;security origins перечислены;оба apt timers есть. apt-daily-upgrade Result=success,но ExecMainExitTimestamp пуст —не доказательство recent successful patch run|
|Security package cache|OpenSSH1:9.6p1-3ubuntu13.20;cached candidate совпадает. `apt list --upgradable` exit0,пакетов не перечислено. Indices не обновлялись;нет current security/advisory clearance|
|NTP|NTP=yes,NTPSynchronized=yes —fresh PASS snapshot|
|Journald/auth logs|/var/log/journal существует;selected merged config ForwardToSyslog=yes. Journal visibility ограничена текущим user;privileged SSH journal denied. Полнота persistence/retention и success/failure counts не доказаны|

Полные команды/stdout/stderr/exits сохранены приватно. `dpkg-query` exit1 из-за missing package entries не отменяет факт установленных OpenSSH/unattended-upgrades;`ssh.socket` inactive не считается stopped SSH. JSON collector exit0 означает завершённый сбор,**не PASS всех вложенных проверок**.

Никаких package downloads/updates,network scans,правок пользователей/ключей/паролей или application data не выполнялось. SSH/sudo audit logging мог добавить штатные auth records;это не config mutation. Root password changed — предоставленный факт,повторная password authentication/rotation не выполнялась.

## Phase2 — recovery design и локальные реальные тесты

Подготовлены local `rollback_guard.py`,синтетические subprocess tests иточный SSH candidate. На VPS скрипты не устанавливались,original SSH files **не backup-ились**:нет privileged access иполного Include inventory.

Guard намеренно ограничен восстановлением **одного** `/etc/ssh/sshd_config`. Он не меняет Includes,SSH keys,users,passwords или firewall. Root-only job directory0700 содержит immutable snapshot metadata/hash,private backup,deadline с monotonic clock и boot ID. До записи проверяются ownership/modes/regular files,отсутствие symlinks,backup hash. Восстановление atomic с mode/UID/GID,fsync,затем `/usr/sbin/sshd -t` итолько `systemctl reload ssh.service`,не restart. Commit допускается только с verified_success ивалидным monotonic timestamp до deadline;поздний/невалидный commit не подавляет rollback. Success/failure сохраняются локальными job events. Runtime root snapshot/checksum/reload/systemd acceptance **NOT RUN**.

Синтетические тесты выполнили **реальный отдельный watchdog process на workstation**,а не только проверку текста:

|Scenario v3|Результат|
|---|---|
|Deadline истёк → target восстановлен byte-for-byte|PASS|
|Подтверждённый commit → candidate сохранён|PASS|
|Corrupt backup → fail до ready/target mutation|PASS,ожидаемый child exit1,полный traceback сохранён|
|Late commit →игнорируется,rollback выполнен|PASS|
|Malformed commit →игнорируется,rollback выполнен|PASS|

V1/V2 tests и guard versions сохранены,не перезаписаны. Эти 5PASS **не доказывают** работоспособность remote systemd,reload,console recovery или rollback через network loss.

Перед production use нужны root-only snapshots **всех реально затрагиваемых config/Include files**,original metadata/hash,baseline sshd-t PASS ивыбор минимального delta по полному Include graph. Если изменяются не только main file,этот узкий guard неприменим без расширения/review/повторных тестов. Не предполагать,что 99snippet override-ит cloud-init или Match.

### Независимый от SSH rollback executor — подготовлено,не запущено

На VPS требуется root-owned job под supervision systemd service,а не один timer. Future invocation после утверждённого root job иполучения privileged access:

```sh
systemd-run --unit=aba-ssh-guard-CHANGE_ID \
  --property=User=root --property=UMask=0077 \
  --property=Restart=on-failure --property=RestartSec=1s \
  --property=StartLimitIntervalSec=0 \
  /usr/bin/python3 /root/aba-ssh-CHANGE_ID/rollback_guard.py --job /root/aba-ssh-CHANGE_ID
```

Перед SSH edit необходимо **реально проверить** ActiveState/MainPID,ready event,живой процесс и deadline;на отдельном fixture job на этом же host дождаться expiry/restore,проверить restart supervision ичто exit SSH-клиента не прекращает unit. Только затем arm настоящий guard сдостаточным deadline. После hardened reload,new independent key login,sudo и negative auth tests записать root-only verified commit до deadline;при любом FAIL не commit иинициировать/дождаться restore. Не доверять одному успешному `systemd-run` exit. Reboot/host failure не покрываются transient watchdog —console recovery отдельно;автоматический timer сам по себе не доказательство восстановления.

Сейчас actual remote watchdog startup/rehearsal/backup **BLOCKED**. Ни service,ни timer,ни root job на server не создавались. Текущие SSH sessions не закрывались;бесконтрольного background process не оставлено.

## Phase3 — trust decision

Применён latest owner instruction:read-only SSH допустим систорическим pin иявным residual risk. Локальная запись ED25519 сверена сранее сохранённым fingerprint `SHA256:AmEMDMCgk3sPDzEDPIQLLYtD9xTyT6OqBvzUMo2V6sM`. Три successful strict SSH handshakes подтверждают владение тем же host private key;автоматического принятия/замены known_hosts не было.

**Независимая server identity не доказана.** Остаются риски initial TOFU enrollment,скомпрометированного сервера/host private key и MITM сэтим key. Ранее раскрытый root password повышает важность incident review;его смена не доказывает отсутствие persistent compromise. Изактивных utmp records нельзя заключить,что root sessions принадлежат атакующему иливладельцу. Журналы successful logins пока недоступны;атрибуция OPEN. Recovery console/systemd rollback отдельно не проверены. При host-key mismatch или unexpected successful privileged login —STOP/capture,не отключать host-key checking ине продолжать hardening как доказательство очистки.

## Phase4 — SSH hardening BLOCKED

Точный local candidate:

```text
PermitRootLogin no
PasswordAuthentication no
KbdInteractiveAuthentication no
PubkeyAuthentication yes
MaxAuthTries 3
AllowUsers aba-admin
```

Не применялся. Fresh key и independent reconnect есть;fresh privileged sudo,полный Include/Match review,root-only config backups,server-sideguard/rehearsal и recovery verification отсутствуют. Нельзя безопасно менять main file поверх недоступного 50-cloud-init.conf иливыдавать local guard test зас erver rollback PASS. AllowUsers должен также пройти dependency review перед requested restriction;сам Zabbix agent не подтверждает отсутствие иных maintenance SSH identities.

После unlock privilege:backup →baseline syntax/effective contexts →server guard rehearsal/ready →candidate syntax/effective context checks →reload →new independent key login и sudo →root SSH refusal/отсутствие password/KbdInteractive methods безввода паролей →Zabbix/listeners →commit. Отказ из-за transport failure нельзясчитать успешным negative auth test. Любой FAIL —restore через guard ипроверка доступа;никогда restart SSH,delete keys/users иливозврат старого раскрытого пароля.

## Phase5 — firewall BLOCKED,без изменений

Внешний interface определён:eth0;есть private eth1. Но protected UFW/nft/iptables rules недоступны,stable admin addresses неизвестны. Источник existingSSHconnections не являетсяавтоматическим admin allowlist. Fresh Zabbix Server config совпал спредоставленными 3 адресами,но external polling/monitoring и NAT path не подтверждены. Поэтомуни ufw enable,ни rules change,ни ListenIP change не выполнялись;Zabbix не отключён.

Подготовленная policy: default deny incoming, allow outgoing; SSH только с проверенных административных адресов либо с отдельно обоснованным временным scope; 10050 только с проверенных monitoring sources. PostgreSQL, Docker API и внутренние сервисы не публикуются. До применения нужны backup фактических правил, проверка внешнего SSH/recovery и анализ Docker DNAT. Docker не устанавливался. Без чтения effective rules и проверенного восстановления firewall остаётся без изменений.

## Изменения и acceptance

|Пункт|Фактический статус|
|---|---|
|Fresh key login / independent connection|PASS|
|Read-only unprivileged server collection|PASS execution;partial audit|
|Complete server audit / full effective SSH /firewall/privileged logs|BLOCKED|
|Privileged sudo without secret transfer|BLOCKED,password required|
|Root/passwordSSH disabled|NOT RUN;неизменены|
|Real SSH config backups / remote rollback execution|NOT RUN|
|Local automatic rollback sandbox|5PASS,ограниченный scope|
|External Zabbix continuity|NOT VERIFIED;service/config/listenerfresh observed|
|Changes to SSH/UFW/Zabbix/network/password/users/keys|NONE|
|Docker/install/update/deployment/live/Git push/project data|NOT RUN|

Ожидаемые nonzero exits сохранены с provenance: privilege denials, отсутствующие dpkg packages, неактивный ssh.socket и отказ при повреждённом synthetic backup. Исторические PASS не объявлены свежими. Конфигурации инфраструктуры и бизнес-приложения не изменены. Отчёт и приватные evidence: [INDEX](../.local/stage12e5/INDEX.md).

## Точная граница и следующий шаг

Следующий необходимый шаг — **интерактивная привилегированная операция владельца в уже доверенном терминале**, без обращения к администраторам School21 и без отправки пароля в чат. Первая точная read-only команда:

```sh
sudo /usr/sbin/sshd -T
```

Полный вывод сохранять приватно; передавать только разрешённые параметры. Затем нужны root-only backup конфигурации и полный privileged preflight. Нельзя вводить пароль в чужую TTY, передавать его через stdin или использовать старый root-пароль. `sudo -v` в терминале владельца не обязательно предоставляет timestamp другой SSH-сессии: автоматическое продолжение в другом процессе не обещается. Нельзя добавлять NOPASSWD:ALL или ослаблять sudo для обхода blocker. Для продолжения нужен доступный, утверждённый способ privileged execution.

**Итог: BLOCKED по privileged access и recovery prerequisites.** Совпадение исторического host key проверено; независимое подтверждение server identity отсутствует. Выполнены свежий remote audit в доступном объёме, независимый key reconnect и локальные rollback tests. Сервер не объявлен hardened.
