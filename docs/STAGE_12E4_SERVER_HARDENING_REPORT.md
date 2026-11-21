# Stage12E.4 — School21 server hardening report

2026-11-16. Target176.57.214.93:22,user aba-admin. **Финальный статус: BLOCKED.**

SSH hardening не применялся. Серверный preflight не выполнен: независимое подтверждение ED25519 host fingerprint и сохранённого recovery-доступа ожидается от владельца. Ранее заданный обязательный trust gate остаётся незакрытым;PASS key login/independent session из нового baseline не является явным подтверждением источника host key. Разрешение выполнить hardening есть;повторное общее разрешение не требуется.

## Выполнено на workstation

- Прочитан актуальный AGENTS.md,существующие execution/hardening procedures;новых применимых инструкций не найдено.
- Найдены public key `~/.ssh/aba_school21.pub` и соответствующий private-key path `~/.ssh/aba_school21`,mode0600. Private key contents не читались/не выводились;публичный key fingerprint:`SHA256:HMWo5ogyiREezdPuJCWYxAOrqUn3DAh/mBJubj/sn0k`. Это public identity observation,не fresh server authentication.
- `ssh-add -l`:exit1,нет identities в agent. Наличие key file не доказывает,что BatchMode сможет использовать его без локального unlock. Не запрашивался password/passphrase в чат.
- `lsof` для established TCP target22:exit1,0видимых local matching rows. Это не доказывает отсутствие пользовательской recovery session в другом приложении/устройстве и не является server session inventory.
- Подготовлен `preflight.py`:read-only collection с per-command stdout/stderr/exit;привилегированные команды только через `sudo -n`,без prompt/password. Private-key/PSK files не читаются,raw authentication logs не экспортируются;journal агрегируется максимум по последним20000records за7дней. Временной предел отдельной команды25с. Ограниченная выборка не доказывает полноту incident review.
- Подготовлен точный requested SSH candidate,без применения к host files.
- Сохранены local baseline,command logs,exits и hashes. Candidate/collector — preparatory artifacts,не backup server configuration.

Ни SSH connection,ни ssh-keyscan,ни server command не выполнялись. Никакие local known_hosts,agent identities,SSH configuration или global settings не менялись.

## Факты владельца и свежая проверка

|Состояние|Источник|Fresh server evidence|
|---|---|---|
|Ubuntu24.04.5,aba-admin,key login PASS,independent session PASS|Текущий запрос|NOT RUN|
|sudo PASS,password required;sshd -t PASS|Текущий запрос|NOT RUN;в новой agent session sudo credentials могут отсутствовать|
|Root password changed|Текущий запрос,принято как предоставленный факт|Секрет не запрашивался и не проверялся через authentication|
|PermitRootLogin yes,PasswordAuthentication yes,PubkeyAuthentication yes|Текущий запрос|NOT RUN|
|UFW inactive|Текущий запрос|Actual UFW/nft/iptables/external rules UNKNOWN|
|Zabbix active,0.0.0.0:10050|Текущий запрос|Actual config/listener/server-side monitoring UNKNOWN|
|Zabbix Server92.53.116.12,92.53.116.111,92.53.116.119|Текущий запрос|Не подтверждены fresh config,actual polling sources/NAT,ServerActive или внешние checks|

По прежнему local known_hosts наблюдается ED25519 fingerprint `SHA256:AmEMDMCgk3sPDzEDPIQLLYtD9xTyT6OqBvzUMo2V6sM`;он **не объявлен доверенным независимо**. Запрошено сравнение с trusted School21 console и сохранность recovery session. SSH known_hosts mismatch нельзя обходить удалением записи или StrictHostKeyChecking=no.

## Preflight до любых изменений

Подготовленный collector собирает:

- Effective sshd configuration,syntax,status ssh.service/ssh.socket;selected security directives и Include graph в `/etc/ssh`;разрешённые Match contexts нужно проверить дополнительно по фактическому source/hostname.
- Current sessions/listeners и management connections;interfaces/routes IPv4/IPv6.
- UFW,nftables,iptables/ip6tables;failures доступа сохраняются,не интерпретируются как отсутствие правил.
- Selected Zabbix Server/ServerActive/ListenIP/port/TLS/remote-command controls и Include graph;без PSK/private keys/UserParameter bodies.
- Package versions,cached update policy,timers/last run,NTP,journal usage/persistence-directory marker,ограниченные SSH event counts,free RAM/disk/swap/limits. Наличие `/var/log/journal` не доказывает effective persistence;origin/timer/cached packages не доказывают актуальные security patches.

**Execution NOT RUN.** Сборщик не делает original config backup и не меняет сервер. До мутации необходимы full root-only snapshots фактически затрагиваемых config files сUID/GID/mode,symlink/Include mapping и hashes. Если Include выходит за service root или недоступен,collector помечает gap;не считать partial view достаточным.

После independent host trust можно single-attempt key-only read-only SSH с pinned target-only host file и `~/.ssh/aba_school21`. Connection template лежит в evidence;paths привязать к фактически проверенному key/pinfile. При denied key,locked local key или sudo-n failure остановиться;старый root password,sshpass,передача sudo password через stdin/chat запрещены.

Текущий sudo=password-required baseline означает,что автоматическое privileged execution не гарантировано. Для privileged части потребуется участие владельца в сохранённой интерактивной session либо другой уже утверждённый privilege mechanism. Не создавать NOPASSWD:ALL и не добавлять Docker privileges,чтобы обойти этот blocker.

## SSH candidate и порядок применения

Requested local candidate:

```text
PermitRootLogin no
PasswordAuthentication no
KbdInteractiveAuthentication no
PubkeyAuthentication yes
MaxAuthTries 3
AllowUsers aba-admin
```

**Не применён,Ubuntu sshd -t на нём NOT RUN.** AllowUsers aba-admin явно сужает доступ до одного user;перед применением fresh inventory должен подтвердить,что School21 automation/administration и recovery не требуют других SSH identities и PAM MFA. Zabbix service сам по себе не подтверждает отсутствие таких зависимостей. Существующий private-tunnel forwarding не отключать случайно.

Следующий порядок после preflight:

1. Подтвердить независимый key-only вход aba-admin,доступность authorized key,его permissions и нужные sudo operations в отдельной session. Сохранить открытую recovery session и реально доступную console;не использовать старый раскрытый root password.
2. Сохранить **полные** затрагиваемые SSH config/Include files в уникальном root-only backup,сохранить effective config и hashes. Не включать private host keys в public evidence;не менять existing key material. Зафиксировать created-vs-existing status каждого будущего файла.
3. Подготовить exact config delta по реальному Include precedence;не предполагать,что новый99snippet переопределяет cloud-init/main/Match. Не перезаписывать чужие config files. Backup должен существовать до записи.
4. Выполнить `sshd -t` и `sshd -T -C ...` для нужных access contexts **до reload**. При несовпадении requested effective values или отсутствии reload support —STOP.
5. Только `systemctl reload ssh.service`,не restart. Не закрывать исходную/recovery session.
6. Новая независимая key-only session,проверка требуемого sudo без password logging. Проверить отказ root public-key login с тем же заведомо работающим transport/key;root negative результат без этой positive control неоднозначен. Проверить effective PasswordAuthentication=no/KbdInteractiveAuthentication=no и отсутствие password auth среди offered methods без ввода пароля. Network error не считать expected refusal.
7. Проверить School21 monitoring continuity и no unintended listeners;только после всех PASS закрывать recovery session по решению владельца.

## Rollback

**Документирован,но не выполнен и не tested.** Это отдельный незакрытый acceptance criterion.

До reload сохранить точные original files и metadata,их пути и new-file list. Recovery operator должен уметь вернуть **только изменённые** config files;новый snippet вывести из Include-пути в root-only evidence,не удалять unrelated files. Затем `sshd -t`,проверка restored effective configuration,approved SSH reload и reconnect. Если baseline был PermitRootLogin/password=yes,реальное восстановление временно возвращает небезопасные значения;такую репетицию согласовать с владельцем,минимизировать окно и повторно применить hardened candidate после проверки доступа. Offline copy/hash rehearsal не выдавать за tested console/network rollback. Не reboot,не закрывать последнюю working session и не восстанавливать старый раскрытый пароль.

## Firewall и Zabbix

**UFW не включался;rules не добавлялись.** Нельзя включать до подтверждения external SSH path,School21 policy,admin source stability,recovery,проверенного monitoring source list и Docker forwarding policy.

Prepared policy:default deny incoming,allow outgoing;22только owner/School21 verified ranges либо явно обоснованный временный broader scope сexpiry/review;10050только verified monitoring sources;PostgreSQL5432,Docker2375/2376,внутренние API/web/worker/SMTP/UI не публиковать. Default outgoing allow не означает доказанную безопасность egress. HTTP/HTTPS сейчас не открывать;proxy/TLS необходимости в этом этапе нет.

Три предоставленных Zabbix Server IP — candidate allowlist,не автоматическое разрешение менять listener/firewall. Проверить actual agent allowlist,current connections,периодическое polling (отсутствие established connection в одном snapshot не опровергает monitoring),внешний latest-data/health со стороны School21 и маршрутизацию/NAT/IPv6. ListenIP менять только после подтверждения совместимости. Не stop/restart/remove Zabbix в этом этапе без отдельной доказанной необходимости;monitoring continuity пока NOT VERIFIED.

Docker **не устанавливать** в Stage12E.4. Будущая установка может добавить DNAT/forwarding paths,обходящие обычный UFW INPUT;требуется проверить выбранный backend/appropriate policy перед дальнейшими сетевыми изменениями. Отсутствие published ports нельзя доказать только UFW status.

## Acceptance

|Критерий|Фактический результат|
|---|---|
|Root SSH disabled|NOT RUN;baseline yes supplied|
|Password/KbdInteractive SSH disabled|NOT RUN;password yes supplied|
|aba-admin key login|PASS supplied,не fresh verified|
|sudo|PASS supplied,password required;fresh privileged probe NOT RUN|
|Independent reconnect|PASS supplied,не проверен после изменений (изменений не было)|
|Original SSH config backup|NOT RUN:нет server access;local candidate не заменяет backup|
|Rollback documented and tested|Documented;test NOT RUN|
|Zabbix operational|Active supplied;fresh external continuity NOT RUN|
|No unintended exposed ports|NOT RUN;свежий listeners/firewall preflight отсутствует|
|Effective firewall documented|Prepared policy;actual rules UNKNOWN,UFW inactive supplied|
|Logging/security updates/NTP|NOT RUN;collector подготовлен|

**Ни один mandatory server criterion не объявлен fresh PASS.** Local compile/document checks — только подготовка.

## Deliverables и следующий шаг

Созданы этот report и [.local/stage12e4/INDEX.md](../.local/stage12e4/INDEX.md),unique run directory сbaseline/commands/exits/logs,preflight collector иcandidate. Existing application,Compose,Dockerfiles,migrations/deployment scripts не менялись. Нет установки Docker,deployment,live,commit/push или изменений данных других пользователей.

Следующая точная команда — **в уже доверенной console School21**,не через ещё не подтверждённый SSH:

```sh
ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub -E sha256
```

Владелец подтверждает совпадение/источник и сохранённую recovery session. После этого можно выполнить fresh preflight,сохраняя per-command results. Отчёт обновляется по фактическим результатам;до них статус остаётся **BLOCKED**,а не hardening PASS.
