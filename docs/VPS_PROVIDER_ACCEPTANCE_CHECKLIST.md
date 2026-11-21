# VPS provider acceptance checklist

Финализация 12 ноября 2026, Stage 12C; локальные проверки 11 ноября. **PRE-VPS: SECURITY / SUPPLY CHAIN BLOCKED; CUSTOMER PILOT NO-GO.** Провайдер не выбран, покупка и серверные проверки не выполнялись. Это универсальные критерии, без рекомендации конкретного платного сервиса.

## REQUIRED — до заказа

|Requirement|Что получить от провайдера / критерий приёмки|
|---|---|
|OS/architecture|Чистый Ubuntu Server 24.04 LTS amd64/x86_64; документированная переустановка из доверенного image|
|CPU|4 vCPU; указаны shared/dedicated, throttling, fair-use и steal/oversubscription policy. Не ARM и не marketing burst-only без понятных лимитов|
|RAM|8 GB без зависимости от swap/ballooning для заявленного объёма; swap не считается RAM|
|Disk|80 GB SSD/NVMe persistent block storage; понятны usable size, IOPS/throughput, quotas и стоимость расширения. План должен оставлять ≥20 GiB свободно после provisioning|
|Network|IPv4 либо проверяемая документированная альтернатива; входящий SSH и исходящий approved maintenance path, понятны bandwidth, traffic quota и overage. Web остаётся private|
|Access|SSH keys, отдельный operator account, root/bootstrap или serial/VNC recovery console; MFA у provider account и независимый аварийный доступ|
|Isolation/Docker|Полная VM либо документированная поддержка нужных cgroups/namespaces/overlayfs/bridge/iptables и Docker Engine; ограничения nested/container VPS известны до покупки|
|Firewall|Настраиваемые provider и host firewall; SSH allowlist, закрытые IPv4/IPv6 web/DB ports. Docker forwarding учитывается отдельно от обычного UFW INPUT|
|Recovery|Задокументированы recovery/reinstall и snapshot consistency/restore procedure; независимый off-host backup доступен. Snapshot не заменяет pg_dump/restore|
|Cost/ownership|Прозрачны валюта/налоги, setup/renewal, диск/IPv4/backup/traffic overage, cancellation/export и срок удаления данных. Provider account принадлежит владельцу|

Не считать checkbox пройденным по названию тарифного плана. Сохранить provider offer/terms и дату review отдельно; цены/актуальные предложения здесь не проверялись. Для реальных данных дополнительно нужны решение владельца о jurisdiction/privacy, DPA, сроках хранения и доступе поддержки.

## RECOMMENDED

- Понятные sustained CPU и disk latency, метрики steal/OOM/IOPS и уведомления о quota/renewal.
- Консоль, snapshots и backup export, которые проверяемы без установки provider agent в приложение.
- Отдельное зашифрованное backup storage в другом failure domain, MFA/restricted recovery identities, журнал доступа.
- Возможность увеличить диск/RAM, настроить reverse DNS и ограничить management access; отсутствие привязки данных к одному account/key.
- SLA/status history и способ срочно заблокировать сеть/восстановить SSH, сохранив volumes/evidence.

## OPTIONAL

Private provider network/VPN, reserved dedicated CPU, дополнительный диск для локальной staging restore rehearsal, immutable object storage и billing alerts. Любой VPN/agent/tool требует отдельного выбора и разрешения, здесь не устанавливался.

## После покупки

Владелец передаёт только provider/region, IP/hostname, подтверждённые характеристики, access/recovery plan и разрешённый scope. Keys/passwords/backup recovery keys в чат не передаются. Сначала выполнить [read-only first-boot checks](VPS_FIRST_BOOT_VERIFICATION.md), затем отдельные bootstrap и staging gates. Покупка не разрешает deploy/live.

Основание sizing: `.local/stage12c-20261111/resource-measurement-report.json`, `capacity-limits.json`, `capacity-outcome.json`. Это локальная synthetic rehearsal с CPU quotas, не peak/SLA benchmark. Сохраняются 4 vCPU / 8 GB / 80 GB, меньший минимум не доказан. Полный verdict: [Stage 12C](STAGE_12C_PRE_VPS_SECURITY_REPORT.md).
