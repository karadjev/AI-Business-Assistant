# Private staging security plan

Финализация 12 ноября 2026; локальные проверки 11 ноября. **PROPOSED / NOT APPLIED на VPS. Security CONDITIONAL; CUSTOMER PILOT NO-GO.** План предназначен только для синтетических данных, live OFF и AI/WhatsApp budgets 0 RUB. Решения владельца ниже ещё не утверждены.

## Access path и trust boundaries

Согласованный оператор → SSH по ключу и allowlist → локальный SSH tunnel → private proxy:8443 → Next/BFF:3000 → Go API:8080 → PostgreSQL:5432. Worker использует ту же isolated DB/queues; Mailpit — локальный synthetic email sink. На host у Compose **0 published ports**; все семь service definitions (migrate одноразовый) используют internal network. Не добавлять public overlay для удобства.

На Linux host SSH forwarding может достигать IP только своего Docker proxy. Предлагаемый browser origin: `https://STAGING_HOST:8443`; имя должно локально разрешаться в 127.0.0.1 у оператора и совпадать с certificate SAN/PUBLIC_ORIGIN. Для примера туннеля placeholders заменяет владелец после разрешения:

```text
ssh -N -o ExitOnForwardFailure=yes -o StrictHostKeyChecking=yes \
  -L 127.0.0.1:8443:PRIVATE_PROXY_IP:8443 OPERATOR@VPS_IP
```

Это не выполнялось. Host key сверяется независимым каналом, agent forwarding выключен. DNS/hosts mapping, tunnel port и certificate trust выбирает владелец; нельзя обходить TLS errors или использовать другой browser origin. Пример меняет только локальный tunnel после запуска владельцем, не публикует Docker port. Provider VPN/private network — отдельный optional design, не установленная зависимость.

## SSH, firewall и Docker administration

Отдельный operator account с именованным ключом. До hardening проверить вторую SSH-сессию и независимую recovery console, не блокировать единственный путь восстановления. Пример **целевого состояния**, только placeholders; это не готовый файл для установки и не разрешение менять sshd:

```text
PubkeyAuthentication yes
PasswordAuthentication no
KbdInteractiveAuthentication no
PermitRootLogin no
AllowUsers OPERATOR
AllowAgentForwarding no
X11Forwarding no
Match User OPERATOR
    AllowTcpForwarding local
    PermitOpen PRIVATE_PROXY_IP:8443
```

PermitOpen привязан к выбранному proxy IP; при recreate IP может измениться — нужен согласованный restricted update или устойчивый private route. Не расширять PermitOpen до wildcard автоматически. `sshd -t`, проверка второй сессии и rollback конфигурации — будущие owner-controlled действия.

Provider firewall и host firewall: default deny inbound; SSH только approved operator IP/CIDR; web/3000/8080/8443, PG/5432, Mailpit/1025/8025 и Docker/2375/2376 закрыты по IPv4/IPv6. Проверять Docker forwarding отдельно: UFW INPUT не доказывает isolation опубликованных контейнерных портов. Отдельного agent-managed firewall изменения нет. Updates/approved transfer выполняет оператор; runtime не получает egress/live provider credentials.

Docker socket только локальный Unix, без mount в containers и без world read/write. Docker group/root — доверенная административная граница: такой оператор может заменить entrypoint, manifest, GO и image. Общий validator перед Compose проверяет effective local Unix endpoint и закрепляет его для CLI/children; remote host/context отклоняется. Launcher/validator не защищают от злонамеренного root. Приложение не имеет socket, host mounts с исходниками или возможности менять собственный image. Разрешённый pipeline отвергает alternative entrypoint/role-command; запуск `/app/api` напрямую через административный Docker CLI остаётся обходом всего pipeline, а не поддерживаемым deployment path.

## Runtime permissions и resource policy

|Component|Verified local settings / требования|
|---|---|
|API/worker|UID/GID 10001, RO rootfs, cap_drop ALL, no-new-privileges, /tmp tmpfs 32 MiB, pids128, только mounted secrets, 15 exact startup parameters|
|Next/BFF|UID10001, RO rootfs, cap_drop ALL, NNP, /tmp64 MiB, pids128, artifact guard перед Next|
|Proxy|UID101, RO rootfs, cap_drop ALL, NNP, /tmp64 MiB, pids64, server cert/key read-only|
|Mailpit|UID10001, RO rootfs, cap_drop ALL, NNP, /tmp32 MiB; synthetic-only volatile mailbox|
|PostgreSQL|Official-image root initialization, затем фактически UID/GID70; writable database volume/rootfs, default capabilities сохранены для bootstrap. Это явное исключение, не fully-rootless stack. NNP, limits, private network, SCRAM/TLS; API роль aba_app без superuser/BYPASSRLS/DDL|
|Migrations|Отдельный aba_migrator; только после PG healthy, до API readiness; schema16 и FORCE RLS gates обязательны|

Шесть steady services имеют в canonical Compose CPU ceilings суммарно4.5 cores и memory limits4.25 GiB; это ceilings, не reserved capacity. Stage12C bounded QA снизила API/worker до0.5 каждый, сумма4 cores; overlay QA-only и не входит в approved runtime baseline. Target остаётся4 vCPU/8 GB/80 GB, native operational capacity NOT RUN. Worker health=liveness, proxy health=config test; реальное HTTPS serving и queue progress проверяются отдельно. Restart unless-stopped не отменяет admission и не разрешает бесконечные автоматические recovery writes.

## Secrets и TLS lifecycle

- Настоящие credentials создаёт владелец **вне репозитория/release/chat**, после provisioning approval. В release/build args/logs их быть не должно. Изолированные local tests используют только ранее подготовленные synthetic secrets.
- Host secret directory0700 с native контейнерными UID/modes; private regular files0400/0600 по validator/runbook. Passwords трёх DB roles независимы; 32-byte auth/retention material независим. Environment0600 содержит только refs/paths/origin и не inline secrets. См. [installation](VPS_INSTALLATION.md).
- HostUID ownership проверяется на Ubuntu: Docker Desktop local-qa не сертифицирует bind-mount UID semantics. DB private key UID70, proxy key UID101, API keys UID10001; public CA/certs не содержат private key. Exact permissions — `deploy.py check_secrets`.
- Private CA trust утверждает владелец; CA signing key хранить off-host. Для VPS передаётся только scoped server key/cert с SAN/expiry, DB verify-full CA и private proxy trust. Предлагаемые server certificate срок30 дней / renewal за7 дней требуют согласования и рабочей renewal/expiry alert procedure; автоматизация ещё NOT IMPLEMENTED.
- Rotation: одобренное окно, backup и dependency-impact review; DB password change согласовать с DSN, затем restart/readiness/TLS/tenant tests. Auth key может инвалидировать sessions. Retention/encryption key нельзя просто заменять: нужен versioned compatibility/migration plan, сохранение audit/receipts и проверка восстановления. При suspected leak закрыть доступ/остановить свой stack, сохранить evidence, затем approved rotation; не логировать значения.

Пример только с placeholders:

```text
ABA_API_IMAGE=EXACT_REVIEWED_SHA256_ID
ABA_WEB_IMAGE=EXACT_REVIEWED_SHA256_ID
ABA_POSTGRES_IMAGE=EXACT_REVIEWED_SHA256_ID
ABA_PROXY_IMAGE=EXACT_REVIEWED_SHA256_ID
ABA_MAILPIT_IMAGE=EXACT_REVIEWED_SHA256_ID
ABA_SECRETS_DIR=/srv/aba/staging/secrets
ABA_PUBLIC_ORIGIN=https://STAGING_HOST:8443
ABA_PRIVATE_SUBNET=REVIEWED_NON_OVERLAPPING_CIDR
```

Этот пример не проходит admission до корректного owner provisioning. Live/kill/budget flags берутся из immutable Compose: live false, kill switches true, mock providers, budgets0. Реальные provider keys не добавлять.

## Backup, restore и retention

Текущая автоматизация создаёт plaintext pg_dump archives0600 в directory0700, TOC/checksum проверены, реальные separate restore rehearsal PASS. Это допускается только для нынешних synthetic QA. Для реальных данных **BLOCKS CUSTOMER PILOT**: выбрать и проверить authenticated client-side encryption, off-host custody/recovery и cipher-text restore. `age`/GPG сейчас не установлены; они не устанавливались и не заменялись самодельной криптографией/OpenSSL enc. Выбор инструмента, binaries, trust source и downloads — отдельное разрешение.

Предлагаемые owner decisions: RPO24h/RTO4h, ежедневный logical backup; 7 daily/4 weekly/3 monthly encrypted off-host копии плюс один проверенный pre-update backup. Это предложения, не согласованные SLA/retention и не реализованная scheduled job. Ключ восстановления хранится отдельно от VPS/provider account, доступ проверяется в аварийном сценарии. Retention относится к backup objects, не изменяет business audit/receipts. Никаких автоматических deletion/retention changes до утверждения.

Restore — только новый `aba-private-restore-*`, свой пустой DB volume, только PG running, matching image IDs/checksum, schema/owners/effective ACL/policies/FORCE RLS/tenant/data verification и HTTPS smoke. Не восстанавливать dump поверх working/demo/staging DB. Ciphertext path должен быть испытан до real data. Не отдавать Mailpit реальные emails/PII; snapshots не заменяют backup и не гарантируют DB consistency.

## Operations и incidents

Application/container logging сейчас json-file10 MiB×3 per container; это не audit/receipts retention. Нужны утверждённые host/journal retention и private access; исключать payloads/cookies/DSNs/secrets, хранить failures с hashes/provenance. Предложение host logs256 MiB/7 days требует review и пока не применено.

Наблюдать health/restarts/OOM, real queue progress, SQL57014/lock waits/503, oldest transaction, disk/inodes, cert expiry, backup age and restore freshness. Предлагаемые disk thresholds: warning70%, block new imports/updates при85% или free<20 GiB, emergency review90%; они не реализованы как agent daemon. Не удалять receipts/evidence или повышать SQL deadline ради освобождения/успешного gate. Server lock/txn/idle settings0 из12B — operational risk, tuning требует отдельного causal review.

При drift/E01/unexpected live/tenant/financial failure: закрыть private access, остановить **свой** project с approved emergency GO, сохранить исходные bytes/proof/UTC/container IDs/редактированные logs/планы/locks. Не rebaseline artifact. Kill switches уже true; после incident не включать live и не увеличивать бюджеты. Unknown cost не считать free; DELIVERY_UNKNOWN автоматически не повторять.

Emergency rollback требует current hardened launcher/host controls, exact reviewed previous frontend/images и доказанную schema compatibility; no migration down/DB overwrite. Backup до update, separate owner GO, затем readiness/HTTPS/RLS. Если schema несовместима — STOP и отдельный recovery plan. См. [operations](VPS_OPERATIONS.md) и [launch gate](VPS_LAUNCH_GATE.md).

Evidence: `.local/stage12c-20261111/runtime-hardening-inspect.json`, `verified-actual-process-*.log`, `runtime-negative-matrix.json`, `verified-actual-control-immutability.json`, `capacity-outcome.json`, `resource-measurement-report.json`; историческая ACL/RLS equivalence — `.local/stage12b-20261111/restore-final-security-equivalence.json`. Реальный VPS, encryption automation, scheduler и owner choices остаются NOT RUN/OPEN.
