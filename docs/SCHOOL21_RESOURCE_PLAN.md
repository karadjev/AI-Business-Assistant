# School21 — preliminary resource and storage plan

Stage12E.3,2026-11-16. **PREPARED FOR SERVER HARDENING**, capacity NOT VERIFIED,server measurements NOT RUN. Расчёты — по заявленным4vCPU/7.8GiB/120GB и текущему Compose. Это resource ceilings,не production throughput/число магазинов/SLA. Никаких limit/env/config changes не внесено.

## Memory/CPU/PIDs — сохраняемый baseline

|Service|CPU ceiling|Memory ceiling MiB|PIDs|Примечание|
|---|---:|---:|---:|---|
|PostgreSQL|1.50|2048|128|shared_buffers512MB,max_connections30,shm256MiB|
|web/Next.js|1.00|1024|128|RO production runtime;fresh build отдельно,не на сервере staging|
|api|0.75|512|128|DB_MAX_CONNECTIONS5,tmpfs32MiB|
|worker|0.75|512|128|DB_MAX_CONNECTIONS5,нет live/paid processing,tmpfs32MiB|
|proxy|0.25|128|64|tmpfs64MiB,nginx workers auto ограничены container resources|
|mailpit|0.25|128|64|tmpfs32MiB,synthetic mail непостоянен|
|migrate,one-shot|0.75|512|128|Отдельный запуск,не steady workload;API ждёт успешного migrate|
|Steady containers total|4.50|4352 (4.25GiB)|640|Ceilings,не reservation|
|При одновременном migrate upper bound|5.25|4864 (4.75GiB)|768|Теоретическая сумма;initial install dependency порядок снижает concurrency|

Memory assumptions:7.8GiB≈7987.2MiB. Оставить **2048MiB planning reserve** для kernel/Engine/Zabbix/SSH/School21 services;это не измеренный upper bound этих служб. После steady limits остаётся1587.2MiB (≈1.55GiB),с migrate1075.2MiB (≈1.05GiB). Actual MemTotal/School21 footprint могут уменьшить запас. Container tmpfs/shm/file cache входят в cgroup usage,не прибавлять их целиком повторно как независимый physical reserve;host accounting и Docker/kernel overhead измеряются отдельно.

CPU ceiling sum4.5>4: одновременно выдать все квоты невозможно,контейнеры делят реальные4CPU. Это допускаемая oversubscription ceilings,не гарантированный reserve host CPU. Initial install pg+migrate+mailpit ceiling2.5;при update возможна конкуренция старого runtime и migrator. Не запускать build,scanners,restore rehearsal и migration одновременно на данном host. Если School21 требует гарантированный CPU reserve или measurements покажут contention — отдельный reviewed resource candidate/QA,**не** тихое редактирование protected Compose. Изменение limits меняет release config hash и требует новой integrity/GO.

PostgreSQL512MB buffers не являются total memory:connections/work_mem/sorts/maintenance/cache могут заполнить2GiB. Проверить actual work_mem/maintenance_work_mem/max_connections/autovacuum/locks,не умножать pool5 на unlimited worker replicas. API+worker pools ориентировочно10 connections плюс migration/backup/admin;all consumers и actual role limits проверить. Не повышать timeouts/max_connections ради исчезновения SQL cancellations. Не вводить auto retry write/DELIVERY_UNKNOWN.

Next.js1024MiB и worker512MiB дают принудительные cgroup bounds,но OOM возможен. V8 old-space/GOMEMLIMIT — потенциальные настройки только после profiling,не необходимая часть текущего docs change. Не лечить OOM бесконечным restart/retry или снятием mem_limit. Worker liveness не измеряет queue lag. Swap не считать RAM/доказательством SLA;не включать swap и не менять swappiness/sysctl здесь.

## Disk budget — гипотеза для согласования

120 decimal GB≈111.76GiB до partition/filesystem overhead;реальное usable неизвестно. Storage minimum coordinator70GiB total и20GiB free сохраняется. Предлагаемые planning envelopes,не настроенные quotas:

|Область|Envelope GiB|Правило|
|---|---:|---|
|Ubuntu/School21/Zabbix/Engine metadata|20|Измерить actual occupancy,не удалять school files|
|PostgreSQL volume (data+WAL+indexes/temp)|25|Следить за WAL/long transactions/bloat;лимит не физическая quota|
|OCI images/build metadata + releases|15|Только admitted releases;не выполнять automatic prune|
|Local backup staging|15|Daily+pre-update могут превысить budget;archive size сначала измерить|
|Container/system/audit reports/logs|3|Compose log limits есть;system audit retention отдельно согласовать|
|Uploads directory|0 initially|Reserved/unmounted;Excel data в DB|
|Оставшийся planning reserve|≈33.76|До filesystem overhead;free guard20GiB обязателен|

Не гарантия вместимости: retained backup count,archive compression,DB/WAL growth и two-release image sharing неизвестны. Proposed daily/off-host policy не даёт права удаления. При backup capacity forecast ниже одного очередного dump+роста/20GiB reserve остановить update/новую нагрузку,согласовать расширение/off-host custody;не удалять evidence/backups ради PASS.

## Измерение, alert thresholds и реакция

Все команды будущие,после отдельного server/read-only или runtime GO. No secrets/full inspect/log payload export.

|Действие / команда|Ожидаемый результат и проверка|Rollback/реакция|
|---|---|---|
|Host baseline: `free -b; getconf _NPROCESSORS_ONLN; df -B1 /srv /var/lib; df -i /srv /var/lib`|Actual available RAM/CPU/disk/inodes;сравнить с declared sizing|Read-only;если<4CPU/<7GiB Engine RAM/<70GiB filesystem STOP,не ослаблять guard|
|`docker stats --no-stream --format '{{.Name}} {{.CPUPerc}} {{.MemUsage}} {{.PIDs}}' APPROVED_PROJECT_CONTAINER_IDS`|Только свой project;series по workload,не единичный capacity proof|Read-only;нагрузку остановить при threshold,не трогать School21 services|
|Для каждого своего ID: `docker inspect --format '{{.State.OOMKilled}} {{.RestartCount}} {{.HostConfig.Memory}} {{.HostConfig.NanoCpus}} {{.HostConfig.PidsLimit}}' CONTAINER_ID`|Actual effective limits и OOM/restarts,без Env|OOM/restart рост → STOP/capture;разбор memory/bug,без снятия limit|
|`docker compose --env-file PRIVATE_ENV -f RELEASE/infrastructure/deployment/compose.yml -p aba-private-staging ps`|Readiness/health своих services|Health API не равно worker queue progress,проверять synthetic queue separately|
|Approved aggregate DB metric SQL через существующий operator workflow|Connection/lock/cancellation/queue metrics без query text/tenant payload|Не менять timeout/RLS/query plan ради показателя|
|Synthetic load/recovery gates через prepared offline runners|Fresh Browser9/9,Catalog2/2,SQL75 calls и isolated restore с latency/OOM/disk evidence|При FAIL остановить только собственный QA,сохранить volumes/logs;не выдавать production capacity|

Предлагаемые alerts для согласования,не включены:memory usage>80% container limit sustained5мин warning;>90%,OOMKilled или unexpected restart critical;host MemAvailable<1GiB sustained warning,<512MiB critical;disk free<25GiB warning,<20GiB stop new deployment/ingest;inode free<10% warning;<5% critical. CPU saturation/PSI/SQL p95/queue age thresholds нужно калибровать по synthetic baseline,а не придумывать SLA. Backup age>26ч warning при approved daily schedule,nonzero backup/restore status critical. Alert backend — существующий Zabbix по School21 approval,без socket/group privileges.

Rollback resource tuning (если позже понадобится):restore previously admitted compatible config/image release через отдельный exact-manifest GO,без DB downgrade. Текущий этап не создал tuning candidate. Native4CPU load/backup/restore ещё BLOCKED;12CPU Docker Desktop historical PASS не переносится на этот host.
