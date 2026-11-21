# DE-3: закрытый staging — подготовка

2026-11-17. **BLOCKED: сервер не аудирован, trusted release не собран.** Это будущая процедура, не разрешение на deployment. Старый School 21 VPS исключён. Все выполненные проверки относятся к локальным синтетическим окружениям.

## Предварительные условия

От владельца нужны IP нового DE-3, фактическая ОС, SSH user/key path без содержимого ключа, независимо подтверждённый host fingerprint, recovery console и разрешённый scope доступа. Вопрос отправлен; ответ не получен. Пароли в чат не запрашивать. Объявленные 4 vCPU / 8 GB / 150 GB не являются измерениями хоста.

После подтверждения identity и read-only доступа: снять CPU/RAM/disk/inodes, ОС/ядро, время/NTP, SSH effective config, listeners, firewall/forwarding IPv4/IPv6, Docker version/socket, текущие workloads, security updates и recovery. Изменения SSH/firewall требуют отдельного разрешения. Не переносить School 21 monitoring allowlist на новый хост.

## Используемая архитектура

Сохранить `infrastructure/deployment/compose.yml`: семь services, пять image identities. `api`, `worker`, `migrate` используют один Go image; остальные images — `web`, `postgres`, `proxy`, `mailpit`. Redis в этом проекте не требуется.

SSH tunnel → private `proxy:8443` (TLS) → `web:3000` → `api:8080` → `postgres:5432` (TLS). Worker обрабатывает PostgreSQL queues, email направлен в private Mailpit `1025`; UI Mailpit `8025` не публикуется. Внутренние DNS names совпадают с именами services. Network `private` остаётся `internal: true`; Compose не публикует ни одного host port. Подсеть выбирается после проверки маршрутов нового хоста.

После отдельного staging GO на Linux-хосте определить адрес **своего** proxy; не использовать старый container IP. Пример будущего tunnel, не выполнен:

```sh
ssh -N -o StrictHostKeyChecking=yes -o ExitOnForwardFailure=yes -o ForwardAgent=no \
  -L 127.0.0.1:18443:APPROVED_PRIVATE_PROXY_IP:8443 ADMIN@DE3_IP
curl --cacert APPROVED_CA.pem --resolve PRIVATE_STAGING_NAME:18443:127.0.0.1 \
  https://PRIVATE_STAGING_NAME:18443/login
```

Browser DNS mapping, certificate SAN/CA trust и `PUBLIC_ORIGIN=https://PRIVATE_STAGING_NAME:18443` должны совпасть. Не обходить TLS errors. Host→bridge reachability и SSH forwarding пока NOT RUN. Rollback tunnel: завершить только этот SSH process. Не добавлять published ports: текущий release validator справедливо отвергает такой override.

## Ресурсы: исходный бюджет, не capacity

|Service|Memory ceiling MiB|CPU ceiling|Проверка после разрешённого запуска|
|---|---:|---:|---|
|PostgreSQL|2048|1.50|buffers 512 MB, max_connections 30, memory/locks/temp/WAL|
|Next.js|1024|1.00|RSS, OOM, restart count, SSR latency|
|API|512|0.75|pool max 5, latency/errors, GC/RSS|
|Worker|512|0.75|pool max 5, lag/failed jobs, budget controls|
|Proxy|128|0.25|HTTPS serving check; `nginx -t` alone недостаточен|
|Mailpit|128|0.25|synthetic-only, capped temp storage|
|Migrate (one-shot)|512|0.75|отдельное окно; не запускать совместно с нагрузкой/restore/build|

Steady sum 4352 MiB, с migrator 4864 MiB. При **реальных 8 GiB** и planning reserve 2048 MiB остаётся 1792 / 1280 MiB соответственно; если 8 GB означают decimal GB, запас меньше — считать по MemTotal. Tmpfs/cache учитываются внутри cgroup usage. CPU ceilings sum 4.5 > 4: это oversubscription, не гарантированный резерв ОС. Проверить под нагрузкой, затем согласовать снижение квот при необходимости. Не повышать limits/timeouts ради PASS. Сборки и сканеры выполнять локально, не на staging VPS.

## Storage и восстановление

Будущая структура: `/srv/ai-business-assistant/releases/`, `current` (контролируемый pointer), `shared/{secrets,backups,uploads,logs,reports}`. Сейчас каталоги на VPS не создавались. Release immutable; не хранить данные внутри release. Секреты/backup directories 0700, backup/env files 0600; mount secret ownership должен соответствовать runtime UID и требованиям PostgreSQL, не назначать всем файлам одного владельца вслепую. У приложения нет Docker socket и privileged DB credentials.

150 GB NVMe — заявленный размер, usable/free неизвестны. Сохранять минимум 20 GiB free planning reserve, измерять размер следующего dump, WAL/temp growth и место для двух релизов. Никакого automatic prune/удаления backup. Daily + pre-update backup, отдельная encrypted off-host copy с независимым key custody — дизайн, не установленная система. Предлагаемые RPO 24 h / RTO 60 min требуют согласования и проверки полного восстановления, не только SQL restore.

Локально выполнен restore в отдельную БД: `database-optimized/status.json` в evidence; это не испытание production backup package, off-host recovery или secret recovery. Для будущего release использовать существующие `scripts/deployment/backup_job.py` / `backup.py`, сохранить их empty-target и GO guards.

## Обязательные release/host gates

1. Закрыть SQL cancellation/lock flakiness причинным evidence; текущий serial PASS не закрывает прежний FAIL и E01.
2. Разрешить точные inputs после свежей проверки manifests. Старый D2 на Next 16.4.1 устарел; теперь нужен 16.4.5. Node 24.15.0 ещё используется в локальном runner; требуется проверенный patched runtime, совместимый со standalone/native modules.
3. Не запускать `package.py` с legacy `BASES`/mutable Node default. Сначала обновить producer recipe под admitted inputs, получить пять новых subjects, SBOM, свежий Trivy, unsigned factual provenance, integrity manifest и полный release regression. Старые архивы не перезаписывать.
4. govulncheck, новые runtime refusals на фактических images, HTTPS smoke, isolated release backup/restore и rollback/roll-forward обязательны; current local source tests их не заменяют.
5. После аудита нового хоста подготовить reviewed secret files/volumes, recovery и scoped owner GO. На этом этапе transfer/start/deployment NOT RUN.

## Минимальный monitoring acceptance

Проверять API readiness, HTTP latency/error rate из JSON logs с request ID, PG health/connections/locks, worker lag и queue depth/failed jobs, order transition failures, integration failures, CPU/RAM/OOM/restarts, free disk/inodes и backup freshness. Существующие Docker log bounds: 3 × 10 MB на service; worker liveness не доказывает queue progress.

До пилота нужны работающие alerts: API down >2 min; DB unavailable; disk <20 GiB или <15%; backup старше согласованного RPO/неуспешный job; устойчивый рост failed jobs/worker lag; повторные ошибки подтверждения заказа. Доставка alerts, адресат и тестовое срабатывание NOT RUN. Не записывать query arguments, customer messages, токены или пароли; не добавлять tenant ID в публичные/high-cardinality metrics.

Итог: **DE-3 STAGING BLOCKED / PILOT NO-GO**. Локальная демонстрация на synthetic data возможна; production/security GO не выдан.
