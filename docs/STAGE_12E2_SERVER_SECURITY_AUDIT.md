# Stage12E.2 — School21 server security audit

Дата документального аудита: **2026-11-16**. Итог: **BLOCKED**.

Цель — безопасная инфраструктура будущего private staging. Проведены поиск проектных evidence, проверка доступности параметров подключения и сверка плана с официальной документацией. **Аудит самого сервера не выполнен:** host/IP, SSH user, доверенный host-key fingerprint/настроенный доступ и серверные logs/config exports не предоставлены. Локального `~/.ssh/config` нет; подходящие серверные evidence в проекте не найдены. Это не означает, что сервер или иные способы доступа отсутствуют. Доступ запрошен у владельца без запроса пароля/приватного ключа.

Серверные команды ниже — план для согласованного read-only исполнения, **NOT RUN**. SSH-соединения, активное сетевое сканирование, изменение SSH/firewall/Zabbix, создание пользователей, установка Docker, apt update/upgrade, reboot и deployment не выполнялись. Пароли, ключи и токены не читались и не выводились. Документ не даёт staging GO и не закрывает release/provenance blockers Stage12C.

## 1. Факты, источник и риски

Все характеристики в таблице предоставлены владельцем; независимого server evidence пока нет. Подтверждённость относится к исходному сообщению, а не к выполненной проверке.

|Данные владельца|Оценка риска / что ещё проверить|
|---|---|
|Ubuntu24.04.5 LTS, amd64,4vCPU,7.8GiB RAM,120GB disk|Заявленная конфигурация; patch level, kernel, filesystem, available space и virtualisation не проверены. Название point release не доказывает установленные security updates|
|SSH22; root SSH login и password authentication enabled|Высокий приоритет: прямой парольный доступ к привилегированной учётной записи на фоне атак. Эффективные настройки и сетевые ограничения требуют проверки|
|Многочисленные failed SSH login attempts|Подтверждено сообщение об атаках на аутентификацию. Количество/период/IP, успех атак и связь событий неизвестны; failures сами по себе не доказательство компрометации|
|UFW inactive|Нет активной защиты через UFW по исходным данным. Нельзя заключить, что отсутствуют nftables/iptables или внешний firewall|
|Zabbix Agent active,0.0.0.0:10050|Listener на всех IPv4 interfaces увеличивает поверхность доступа. Public reachability, IPv6, Server allowlist, TLS и remote-command policy неизвестны. Listener не доказывает доступность агентских данных любому клиенту|
|Docker отсутствует|Не security defect сам по себе. Установка может изменить networking и privileges; отложить до согласований и firewall review|

**Подтверждённых этим аудитом успешных SSH-входов, неуспешных событий или отсутствия постороннего доступа нет.** Высокие риски root/password и широкого listener следуют из предоставленного baseline. Не установлены: отсутствие внешней защиты, уязвимая версия Zabbix, отключённые security updates, факт взлома. Не присваивать CVE без versions и advisory applicability.

## 2. SSH-журналы и признаки компрометации

|Показатель|Результат|
|---|---|
|Период и полнота доступных журналов, rotation/retention/timezone|UNKNOWN / BLOCKED|
|Accepted password, включая root|UNKNOWN, не0|
|Accepted publickey / другие successful methods|UNKNOWN, не0|
|Failed password / Invalid user / PAM failures|По владельцу — многочисленные; точные counts UNKNOWN|
|Подтверждённые входы School21 и владельца|UNKNOWN; нужна сверка с административным roster|
|Несогласованные успешные входы / persistent access|НЕ УСТАНОВЛЕНО; NOT RUN|

После получения доступа проверить UTC-время/синхронизацию, earliest/latest retained records, текущий и предыдущие boots, журнал SSH и auth.log rotations. Первое окно — последние7дней, затем доступный период, покрывающий provisioning и атаки. Не смешивать journal и auth.log без дедупликации: они могут описывать одни события. Отдельно считать SSH records, а не объявлять каждый PAM/Invalid user record отдельной попыткой.

Read-only команды для оператора с существующими правами (без sudo):

```sh
date -u
id
uptime
journalctl --list-boots --no-pager
systemctl is-active ssh.service ssh.socket
last -Fai
lastb -Fai
```

`last/lastb` и журнал содержат identities/IP; не публиковать raw output. При отсутствии файла/прав результат BLOCKED, не «0 входов». Для aggregate event counts допустим следующий bounded collector, выполняемый только на сервере после подтверждения доступа:

```sh
set -o pipefail
journalctl -u ssh.service --since '7 days ago' --until now --no-pager -o json |
python3 -c 'import sys,json,collections
counts=collections.Counter(); first=None; last=None; records=0
for line in sys.stdin:
    item=json.loads(line); msg=item.get("MESSAGE", ""); stamp=item.get("__REALTIME_TIMESTAMP")
    if stamp: first=stamp if first is None else min(first,stamp); last=stamp if last is None else max(last,stamp)
    records+=1
    for tag,needle in [("accepted_password","Accepted password for "),("accepted_publickey","Accepted publickey for "),("failed_password","Failed password for "),("invalid_user","Invalid user ")]:
        if needle in msg: counts[tag]+=1
print(json.dumps({"records":records,"first_unix_microseconds":first,"last_unix_microseconds":last,"event_counts":dict(counts)}))'
```

Collector не выводит MESSAGE, имена/IP/ключи. Он не покрывает автоматически другие auth methods, все syslog identifiers или auth.log rotations; не является полноценным incident review. Выходной status pipeline обязателен. Пустой недоступный/неполный журнал не даёт clean result. Время из retention metadata нужно сверить отдельно: первая запись окна не доказывает время provisioning.

В приватном разборе сверить successful logins с разрешёнными пользователями, source IP/VPN, timestamp и key fingerprint. Проанализировать Accepted после серий failures, неизвестные users/keys, sessions в необычное время, sudo/su, изменения пользователей/authorized_keys/sudoers, неожиданные systemd services/timers/cron, исполняемые файлы и исходящие соединения. Не читать private keys, shell history целиком, environment или `/etc/shadow`; не запускать неизвестные scripts. В отчёт передавать redacted event table с pseudonymous user/source и UTC, без credential values.

Если выявлен несогласованный successful privileged login: приостановить hardening-as-usual, сохранить evidence и обратиться к School21 за incident response. Не чистить логи, не удалять подозрительные файлы и не менять firewall самостоятельно. Решение об изоляции/ротации credentials/reprovision принимается совместно, до staging.

## 3. Существующие административные ограничения

UNKNOWN: owner/server tenancy, допустимые sudo operations, School21 admin users/groups/keys, PAM/SSSD/SSH certificates/MFA, configuration management, jump hosts/VPN, emergency console, recovery SLA, внешний firewall и правила мониторинга. Известно только исходное сообщение о включённом root/password доступе. Отсутствие SSH export не позволяет перечислить действующие AllowUsers/AllowGroups/DenyUsers/Match restrictions.

Запросить существующие права и redacted administrative inventory. Не выполнять sudo даже для чтения на данном этапе без отдельного согласования. Если `/usr/sbin/sshd -T`, journal, firewall rules или Zabbix config недоступны текущему пользователю — получить read-only export от School21, не менять mode/owner/groups.

Для SSH проверить `sshd -T` и context-specific `sshd -T -C user=...,addr=...,host=...` для owner и School21 sources. Подставлять только согласованные значения. Проверить main config, Include/drop-ins, Match blocks, PAM/SSSD и socket activation. Не считать один grep основного файла эффективной политикой. Ubuntu использует config snippets и first-value precedence; перед будущим применением проверить syntax через `sshd -t`, затем effective config. [Ubuntu OpenSSH](https://ubuntu.com/server/docs/how-to/security/openssh-server/).

## 4. Внешний и host firewall

Provider/School21 perimeter access не предоставлен; **доступные механизмы внешнего firewall не подтверждены**. Запросить read-only сведения о security groups/cloud firewall, router/VLAN ACL, VPN/bastion, NAT/port forwarding, public/private addressing, IPv6, управляющей панели и владельце изменений. Привязать export правил к конкретному server/interface и дате. Firewall может находиться вне VM и быть невидимым в UFW. Активные internet port scans не разрешены этим read-only scope автоматически.

Host read-only inspection, при существующих правах:

```sh
ss -lntup
ufw status verbose
nft list ruleset
iptables-save
ip6tables-save
ip -brief address
ip route show
ip -6 route show
```

Permission denied сохранить как BLOCKED. Не выполнять `ufw enable`, `nft add/flush`, `iptables -A/-F`, sysctl writes. Не отправлять raw network/process metadata в public GitHub.

**Предлагаемая policy после согласований:**

|Traffic|Будущая policy|
|---|---|
|Established/related и loopback|Сохранить; не обрывать существующие admin sessions|
|SSH TCP22|Разрешить только подтверждённые owner VPN/bastion/admin CIDR **и все School21 management sources**, IPv4/IPv6. Не заменять их одним текущим IP|
|Zabbix TCP10050 passive|Разрешить только согласованные School21 server/proxy sources; остальные запретить|
|Zabbix active checks|Исходящий доступ к согласованным server/proxy endpoints/ports (обычно10051), только если реально используется|
|ICMP/ICMPv6, ND/PMTU и обязательная provider infrastructure|Сохранить необходимые types; не ломать IPv6/addressing|
|Web80/443, DB5432, SMTP/Mailpit, API8080, Docker2375/2376 и прочие app ports|Не открывать публично для этого staging; никаких deployment exceptions|
|Остальной inbound|Default deny после проверки management/monitoring/recovery inventory|
|Outbound|Сначала inventory DNS/NTP/School21 monitoring/repository/proxy paths; ограничение по отдельному решению, не отключать инфраструктуру вслепую|

Нет IP/CIDR — нет готового к применению ruleset. Порядок: emergency console → allowlists → сохранение старых rules → staged changes с out-of-band rollback → новая owner SSH session и School21 validation → default deny. Не менять host и perimeter одновременно. План rollback должен быть согласован заранее; сейчас ничего не применять.

## 5. Zabbix

Проверить package/version и тип agent/agent2, active unit, конфигурационный путь из unit и разрешённые Include. Не выводить полный unit Environment, config или UserParameter command bodies: там могут быть секреты. По allowlist извлечь `Server`, `ServerActive`, `ListenIP`, `ListenPort`, `StartAgents` (если поддерживается этой версией), `TLSAccept`, `TLSConnect`, `AllowKey`, `DenyKey`, `EnableRemoteCommands`/remote-command policy, `UnsafeUserParameters`, Include. Не выводить TLSPSKFile contents, PSK, private keys, credentials; сверить только ownership/mode и факт наличия. Проверить system.run и небезопасные UserParameters статическим review, не выполнять items.

`Server` ограничивает источники passive checks; `ServerActive` задаёт active destinations. Listener binding, source allowlist, firewall и TLS — разные controls. Параметры должны проверяться по **фактически установленной версии**, которая пока UNKNOWN; reference7.4 не доказывает версию сервера. [Zabbix agent configuration](https://www.zabbix.com/documentation/7.4/en/manual/appendix/config/zabbix_agentd).

Ограничение listener рекомендуется рассмотреть, но не применить автоматически: если School21 polling приходит на private management IP — согласовать binding к нему плюс allowlist/firewall. Если management source требует текущий interface — ограничить sources и transport. Loopback-only сломает удалённые passive checks; active-only возможен лишь после подтверждения templates/monitoring архитектуры. Zabbix не отключать. TLS/PSK/cert transition требует согласованной настройки обоих концов и проверки monitoring continuity.

## 6. unattended-upgrades

Состояние **UNKNOWN / NOT RUN**. Read-only проверка:

```sh
dpkg-query -W -f='${Package} ${Version} ${Status}\n' unattended-upgrades apt openssh-server zabbix-agent zabbix-agent2
systemctl list-timers --all --no-pager apt-daily.timer apt-daily-upgrade.timer
systemctl show apt-daily-upgrade.service -p Result -p ExecMainStatus -p ExecMainExitTimestamp
apt-config dump | grep -E '^(APT::Periodic::|Unattended-Upgrade::(Allowed-Origins|Origins-Pattern|Automatic-Reboot|Remove-Unused))'
test -e /var/run/reboot-required
```

Несуществующий package/timer и exit1 последнего `test` документировать отдельно; не трактовать как общий FAIL. Дополнительно проверить последний successful security run, errors/locks и redacted summary `/var/log/unattended-upgrades/`, allowed security origins, package holds/pinning, reboot-needed policy и maintenance ownership. Кэш `apt list --upgradable` без свежих indices не доказывает отсутствие patches. На этом этапе не выполнять apt update, upgrade, install или unattended-upgrade, включая dry-run режимы с потенциальными lock/cache/log writes. Наличие timer не доказывает успешное обновление. [Ubuntu automatic updates](https://ubuntu.com/server/docs/how-to/software/automatic-updates/).

## 7. План admin user, SSH keys и безопасной SSH policy

Только для следующего отдельно разрешённого change window:

1. Согласовать owner account name, School21 accounts/cert authority/MFA, least-privilege sudo scope, recovery console и break-glass procedure. Проверить namespace/UID collision; не создавать параллельную неуправляемую identity, если School21 требует SSSD/центральные учётные записи.
2. Создать персональный non-root admin account по согласованному механизму; не вводить shared password и NOPASSWD:ALL по умолчанию. sudo policy проверить штатным validator. Приватный ключ остаётся на workstation; публичный ключ/fingerprint подтвердить out-of-band.
3. Владелец готовит passphrase-protected Ed25519 key либо hardware-backed key по School21 policy. Установка только public key, корректные ownership/modes .ssh0700/authorized_keys0600, без чтения private key. Не пересылать ключевые материалы через отчёт.
4. Сохранив исходную session и console, открыть **вторую** key-only session нового пользователя; проверить privilege workflow, затем отдельный вход School21 из их network. Password fallback не должен маскировать неработающий key.
5. После обоих подтверждений согласовать SSH candidate ниже, syntax/effective policy и rollback; применять reload отдельно, не reboot. Проверить новые sessions ещё раз до закрытия старой. School21 должен подтвердить monitoring/automation compatibility.

Candidate, **не готовый deployment snippet**, не устанавливался:

```text
Port 22
PubkeyAuthentication yes
PermitRootLogin no
PasswordAuthentication no
PermitEmptyPasswords no
UsePAM yes
X11Forwarding no
PermitUserEnvironment no
```

`KbdInteractiveAuthentication no` допустим только при отсутствии требуемого School21 PAM MFA; иначе сохранить согласованный publickey+MFA chain. `AuthenticationMethods publickey`, AllowUsers/AllowGroups, MaxAuthTries/MaxStartups, forwarding restrictions и timeouts выбирать после inventory. Не отключать TCP forwarding вслепую: проектный private access может зависеть от SSH tunnel; ограничить нужные destinations по отдельной policy. Не менять порт22 ради видимости защиты. Не вводить устаревшие cipher/KEX списки без конкретного требования. Не удалять School21 keys или отключать root, пока их согласованный альтернативный доступ и console recovery не проверены.

## 8. План Docker из официального источника

Сейчас **NOT INSTALLED по владельцу; установка запрещена**. Перед будущим разрешением: подтвердить Noble/amd64, School21 virtualization/cgroups/overlay/network restrictions, ownership Docker admin boundary, disk/retention, conflict packages/container runtimes и maintenance plan. Не удалять containerd/runc/другие пакеты без проверки, что они не обслуживают инфраструктуру School21.

Предпочтительный будущий источник — официальный APT repository `https://download.docker.com/linux/ubuntu`, suite noble, amd64; scoped Signed-By keyring. Отдельно подтвердить signing-key fingerprint по доверенному официальному каналу; точные package versions/dependency closure, размеры и hashes определять после разрешённого metadata review. Не использовать convenience `curl | sh`, latest или новый неподтверждённый fingerprint.

После отдельного разрешения и acquisition manifest установить pinned `docker-ce`, `docker-ce-cli`, `containerd.io`, `docker-buildx-plugin`, `docker-compose-plugin` и необходимые зависимости с APT signature/hash verification. Установка может автоматически запустить daemon/создать network rules; это должно входить в согласованный window. Docker group даёт привилегированный доступ — не добавлять туда пользователей автоматически. Docker API не публиковать; socket не монтировать в приложение. `hello-world` подразумевает image download/run и сейчас также не разрешён.

Обязателен review forwarding/firewall policy **до** установки: published container ports могут обходить обычные UFW rules. Проверить фактический firewall backend выбранной версии и соответствующую policy; не отключать Docker firewall management ради UFW. Ubuntu24.04 amd64 присутствует в официальной supported-platform документации; это не подтверждает поддержку конкретной VM School21. [Docker Ubuntu installation](https://docs.docker.com/engine/install/ubuntu/), [Docker firewall interaction](https://docs.docker.com/engine/network/packet-filtering-firewalls/).

Откат будущей установки заранее согласовать с School21: восстановить затронутые config/network state без удаления school services/volumes, сохранить install logs; никаких prune/purge данных. Docker readiness не означает trusted release или разрешение deployment.

## 9. Необходимые согласования и безопасный порядок

Нужны School21:

- SSH host/user, независимый host fingerprint, разрешённый audit window и read-only access/export для журналов, effective config, firewall и monitoring; не присылать пароли/приватные ключи.
- Административный roster, owner/VPN/bastion CIDR обоих protocol families, automation/MFA/CA dependencies, configuration-management ownership и recovery console.
- Perimeter firewall controls/exports и полномочия изменения host/perimeter policy.
- Zabbix version, server/proxy addresses, passive/active mode, TLS policy, approved listener interface и validation contact.
- Admin user/sudo/key transition, SSH candidate, rollback и проверка двух независимых admin access paths.
- Security update/reboot policy, окна обслуживания и Docker/network/privilege approval. Никаких автоматических изменений из данного аудита.

Порядок: **доступ/evidence → проверка successful logins и incident decision → management/recovery inventory → согласование hardening → новый admin/key и School21 access validation → поэтапные SSH/firewall изменения → Zabbix continuity → updates policy → отдельное Docker решение → отдельный trusted-release/staging gate**. Если обнаружена компрометация, сначала School21 incident response и решение о доверенной переустановке; обычный hardening не доказывает очистку.

## 10. Итог проверок

|Проверка|Статус|
|---|---|
|Поиск проектных server evidence и локального SSH config|PASS как локальный discovery; серверный доступ не найден|
|Официальные Ubuntu/Zabbix/Docker references|PASS: просмотрены; это справочники, не server evidence|
|Server SSH logs, successful/failed login counts, compromise review|BLOCKED: нет доступа/exports|
|Effective SSH/admin restrictions|BLOCKED|
|External/host firewall, Zabbix Server/TLS/listener restrictions|BLOCKED|
|unattended-upgrades/installed patch status|BLOCKED|
|Admin/key/SSH/firewall/Docker change plans|PREPARED, NOT EXECUTED|
|Server changes, installs, reboot, deployment, commit/push|NOT RUN|

**Финальный статус: BLOCKED.** Для READY FOR HARDENING требуются проверенные server evidence, решение по подозрительным успешным входам, согласованные School21 management/recovery constraints и reviewable change/rollback plan. READY FOR HARDENING будет разрешать только планирование/согласованный hardening, а не staging deployment. PRIVATE STAGING и CUSTOMER PILOT остаются NO-GO.
