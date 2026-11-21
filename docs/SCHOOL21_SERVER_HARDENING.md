# School21 — server hardening procedure

Stage12E.3,2026-11-16. **BLOCKED BEFORE SSH CONNECT**. В новом запросе владелец подтвердил личный сервер176.57.214.93 и разрешил системные изменения при выполнении предварительных условий. Прежний запрет системных изменений не является текущим blocker; отсутствуют независимо подтверждённый ED25519 fingerprint, проверенный key login/recovery и management/monitoring inventory. Ни одна серверная команда не выполнялась. [Execution report](STAGE_12E3_EXECUTION_REPORT.md) отделяет текущие факты от подготовленных процедур; [Stage12E.2](STAGE_12E2_SERVER_SECURITY_AUDIT.md) сохраняет исторический audit scope.

## Текущий обязательный trust/recovery gate

Старый root-пароль опубликован и считается скомпрометированным. Его нельзя получать из истории, использовать, хешировать или сохранять в evidence. Через уже доверенную provider/School21 console владельцу необходимо немедленно сменить его интерактивно (`passwd root`, без передачи значения в чат/command arguments) либо согласовать эквивалентное emergency containment. Это команда для владельца в доверенной консоли, здесь NOT RUN. Ротация не доказывает отсутствие компрометации; successful-login/incident review остаётся обязательным.

Для независимой сверки в той же доверенной console: `ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub -E sha256`. Локальный known_hosts/ssh-keyscan не являются независимым источником. До подтверждения endpoint/key привязки запрещены SSH connect и серверные мутации. Не отключать root login или recovery до проверки второго key-only входа aba-admin и его разрешённых административных операций. Не менять host keys/known_hosts для подавления mismatch. Смена скомпрометированного секрета не откатывается к старому паролю.

Все команды мутаций выполняет в будущем отдельно уполномоченный администратор через существующий approved privilege mechanism. Placeholder `CHANGE_ID`,approved public-key path,CIDR и rollback artifacts необходимо заранее заполнить и проверить. Нельзя копировать таблицу как безусловный shell script.

## До первого изменения

Согласовать School21 admin roster/keys/CA/MFA/SSSD, owner VPN/CIDR IPv4+IPv6, automation/SSH tunnels, Zabbix endpoints, configuration-management ownership, incident decision и maintenance window. Нужна работающая out-of-band console и ответственный за rollback,а не только текущая SSH session. Root/password включены по прежнему сообщению; многочисленные failures не доказывают отсутствие/наличие successful intrusion. При неизвестном успешном privileged login — сначала School21 incident response,evidence preservation и решение о доверенной переустановке. Hardening не является очисткой скомпрометированной системы.

## Порядок admin/key/SSH

|Действие / будущая команда|Ожидаемый результат|Проверка|Rollback|
|---|---|---|---|
|Inventory: `id; getent passwd aba-admin; getent group sudo`|Определить свободное имя/центральные учётные записи,без изменений|UID/name collision или SSSD policy → STOP|Не нужен|
|Подготовить защищённый snapshot SSH в private CHANGE_ID каталоге: `cp -a /etc/ssh APPROVED_CHANGE_DIR/ssh-before`|Есть точная предшествующая конфигурация и modes|Root-only change dir,checksum manifest,не выводить private host keys|Snapshot сохранить;не публиковать в Git|
|После проверки отсутствия account: `adduser --disabled-password --gecos '' aba-admin`|Персональный non-root account;парольный вход не включён|`id aba-admin; getent passwd aba-admin`|Не удалять account/home автоматически;School21 отдельно блокирует/отменяет новую identity после evidence review|
|На workstation: `ssh-keygen -t ed25519 -a 64 -f ~/.ssh/aba-school21`|Passphrase-protected private key остаётся локально;при policy использовать hardware-backed key|Fingerprint public key сверить out-of-band;не выводить private key|Снять только новую public authorization по согласованию;private key custody сохраняется|
|На сервере после передачи только public key: `install -d -o aba-admin -g aba-admin -m 0700 /home/aba-admin/.ssh` и `install -o aba-admin -g aba-admin -m 0600 APPROVED_PUBLIC_KEY_FILE /home/aba-admin/.ssh/authorized_keys`|Key-only login нового user;не overwrite если account уже управляется School21|`stat` owner/mode и `ssh-keygen -lf ...` с redacted fingerprint output|Восстановить previous authorized_keys если существовал;не менять School21 keys|
|Подготовить только School21-approved privilege rule: `visudo -cf APPROVED_SUDOERS_CANDIDATE`|Синтаксис least-privilege policy PASS;роль определена заранее|Штатный admin deployment policy и тест требуемых операций во второй session|Не вводить NOPASSWD:ALL;вернуть прежний sudoers artifact и повторить validator|
|Workstation key-only second session: `ssh -o PasswordAuthentication=no -o KbdInteractiveAuthentication=no -o IdentitiesOnly=yes -i ~/.ssh/aba-school21 aba-admin@APPROVED_HOST`|Вход работает без password fallback|Новая session+approved privilege operation;School21 отдельно проверяет свой вход|Старую session/console держать открытой;при FAIL SSH policy не менять|

Для PAM MFA последнюю команду адаптировать к согласованной authentication chain: запрет KbdInteractive здесь проверяет pure key только когда School21 policy это допускает. Disabled-password account не обеспечивает пароль для sudo: privilege enrollment/другой approved mechanism должен быть решён отдельно,до отключения root. Не добавлять пользователя в sudo/docker вслепую.

SSH candidate для будущего review:

```text
Port 22
PubkeyAuthentication yes
PermitRootLogin no
PasswordAuthentication no
MaxAuthTries 3
PermitEmptyPasswords no
UsePAM yes
X11Forwarding no
PermitUserEnvironment no
```

Не отключать required MFA/SSSD и forwarding,нужный private tunnel. `KbdInteractiveAuthentication no` только при отсутствии PAM MFA requirements; AuthenticationMethods/AllowUsers/AllowGroups/PermitOpen/MaxStartups — после inventory. MaxAuthTries3 проверить effective policy; ограничение пользователей должно явно включать aba-admin и все необходимые School21 identities/CA-managed accounts. Статический AllowUsers без School21 identities запрещён. Не менять порт22 ради сокрытия.

|Применение / команда|Результат и verification|Rollback|
|---|---|---|
|Review main Include/Match и `sshd -T -C user=aba-admin,addr=APPROVED_SOURCE_IP,host=APPROVED_HOSTNAME`|Effective policy owner и School21 contexts зафиксирована|Read-only,отката нет|
|После approval: `install -o root -g root -m 0644 APPROVED_SSH_CANDIDATE /etc/ssh/sshd_config.d/00-aba-approved.conf`|Candidate появился;first-value precedence и более ранние snippets проверены|Восстановить snapshot/предыдущий файл;новый snippet вывести из Include без удаления evidence|
|`/usr/sbin/sshd -t` затем `sshd -T -C ...` для всех approved admin contexts|Syntax и intended policy PASS до reload;проверить ssh.socket activation и unit|При FAIL не reload;вернуть own candidate и повторить syntax|
|`systemctl reload ssh.service` только если service active и reload поддержан|Новые sessions получают policy;старую не закрывать|Через console восстановить snapshot,`sshd -t`,approved reload;не reboot|
|Owner+School21 новые sessions и private-forward test|Оба access paths работают,root/password действительно запрещены|При любой потере доступа — согласованный console rollback,не многократные blind SSH retries|

Ubuntu config snippets имеют precedence;`99-*.conf` не гарантирует override. Основание: [Ubuntu OpenSSH](https://ubuntu.com/server/docs/how-to/security/openssh-server/). Port/socket changes здесь не планируются;если unit socket-activated,сначала effective systemd review.

## Firewall с сохранением School21

Ни одного готового CIDR в evidence нет. Поэтому команды ниже — template,**не готовый ruleset**. Не включать UFW до подтверждения всех management/monitoring sources,IPv6 policy,console и предыдущего ruleset. Снять разрешённый private export UFW/nft/iptables;не выполнять reset/flush.

|Команда после approval|Ожидаемый результат / проверка|Rollback|
|---|---|---|
|`ufw status verbose; nft list ruleset; iptables-save; ip6tables-save`|Inventory текущего host firewall;при отсутствии прав получить export School21|Нет мутации|
|Для каждого owner/School21 IPv4/IPv6 CIDR: `ufw allow proto tcp from APPROVED_ADMIN_CIDR to any port 22`|SSH allow rules **до enable/default deny**;проверить `ufw show added`|Удалить только newly added exact rule по change record,не чужие правила|
|Для каждого passive monitoring source: `ufw allow proto tcp from APPROVED_ZABBIX_CIDR to any port 10050`|Zabbix School21 остаётся доступен;active egress согласовать отдельно|Вернуть собственную rule change по snapshot|
|`ufw default deny incoming` и `ufw default allow outgoing`|Предварительная policy;не ограничивать DNS/NTP/School21 egress без inventory|Вернуть прежние defaults/rules через console по change record|
|`ufw enable` только при ready console и reviewed policy|IPv4/IPv6 rules применены;SSH owner+School21/monitoring tests PASS|School21 восстанавливает previous state (исходно inactive по владельцу),сохраняя evidence;не reset rules|
|После Docker: `docker ps --format '{{.Names}} {{.Ports}}'; ss -lnt; iptables-save; nft list ruleset`|Не опубликованы app ports;actual Docker backend/forwarding reviewed|Не flush Docker chains;STOP rollout и согласовать исправление|

Сохранить loopback,established/related и необходимые ICMP/ICMPv6/PMTU/ND/School21 infrastructure flows. Не добавлять public80/443/8443/5432/8080/3000/SMTP/Mailpit или remote Docker API. [Docker firewall documentation](https://docs.docker.com/engine/network/packet-filtering-firewalls/) объясняет обход обычного UFW INPUT при port publishing. Для iptables backend policy анализируется в DOCKER-USER;для native nftables backend нет автоматического предположения о такой chain — нужна поддержанная policy этой версии. Не ставить `iptables:false` и не переключать backend ради обхода проверки. Текущий Compose с0 published ports сохраняется.

## Zabbix, обновления и audit logging

|Действие / команда|Результат / проверка|Rollback|
|---|---|---|
|`dpkg-query -W zabbix-agent zabbix-agent2`;`systemctl is-active zabbix-agent zabbix-agent2`|Установить actual agent/version;несуществующий вариант не считать отказом working agent|Нет мутации|
|Разрешённый selective config export Server/ServerActive/ListenIP/TLSAccept/TLSConnect/AllowKey/DenyKey/Include|Source allowlist и TLS отдельно от listener;не читать PSK/private keys/UserParameter bodies в общий report|Нет мутации|
|После School21 approval установить отдельно reviewed agent candidate через `install ... APPROVED_AGENT_CONF ACTUAL_CONFIG_PATH`|Private management ListenIP только если passive checks сохраняются;Server — реальные School21 proxy/server sources|Вернуть previous config/mode;restart/reload только с School21,не отключать service|
|School21-approved validation и `systemctl restart ACTUAL_ZABBIX_UNIT` в maintenance window|Agent active и server-side fresh checks/no alerts;restart только если нельзя reload|Вернуть approved snapshot и unit state;не disable Zabbix|
|`systemctl list-timers --all apt-daily.timer apt-daily-upgrade.timer`;selective apt-config export|Actual origins/timer/result/last successful run и reboot-needed установлены,не inferred от наличия package|Нет мутации|
|После отдельного update-policy approval: `install -o root -g root -m 0644 APPROVED_APT_POLICY /etc/apt/apt.conf.d/52aba-approved`|Только reviewed security origins/schedule;automatic reboot запрещён до отдельного решения|Вернуть policy snapshot;это не package downgrade,installed updates не откатывать автоматически|
|После отдельного approval: `systemctl enable --now apt-daily.timer apt-daily-upgrade.timer`|Timers активны;может запустить network/update activity — требуется package/update scope|Вернуть прежнее enable/active state через School21;не отменять running dpkg грубо|
|`journalctl --disk-usage; systemctl is-active systemd-journald; test -d /var/log/journal`|Audit persistence/usage inventory;не печатать raw credentials/payloads|Нет мутации|
|После retention/storage approval: `install -o root -g root -m 0644 APPROVED_JOURNAL_DROPIN /etc/systemd/journald.conf.d/aba-approved.conf` и approved journald reload/restart|Persistent journal,agreed size/time retention,ошибки/SSH/sudo logs доступны только admins|Вернуть prior config;не vacuum/удалять старые logs|

Для Zabbix сверить параметры именно installed version:[reference7.4](https://www.zabbix.com/documentation/7.4/en/manual/appendix/config/zabbix_agentd);version сервера UNKNOWN. Loopback-only и active-only не включать без School21 template/architecture approval.

Предлагаемые journald budget256MiB,SystemKeepFree≥2GiB,retention30дней — **гипотеза для согласования**,не действующая policy. Перед применением проверить поддержанные параметры installed systemd и filesystem budget;пакет auditd не устанавливать автоматически. Auditd rules/service возможны отдельным решением;даже journald persistent не доказывает tamper resistance. Запретить auth/SQL parameters/provider payload/secrets в application exports,не отключать security events. [Ubuntu automatic updates](https://ubuntu.com/server/docs/how-to/software/automatic-updates/) описывает реальные update actions;его dry-run не используется как read-only evidence здесь.

## Exit gate

Ready-to-execute требует signed-off change matrix,verified console,оба admin key access paths,эффективные SSH/School21 dependencies,firewall/Zabbix/network policy и rollback rehearsal. Всё это пока NOT RUN. Если любой access/monitoring test FAIL — STOP,сохранить evidence,не переходить к Docker/deployment. План не включает fail2ban/новые зависимости без отдельной необходимости и approval.
