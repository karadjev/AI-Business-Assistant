# AI Business Assistant — runbook staging

SSH и host prep прошли. Запуск основной staging базы ожидает GO по кандидату. Источник приложения `a216246e5840eb9c7c49945bfa5220f13d9cba38`. Рабочие изменения deploy tooling находятся в `work/staging-deployment`. Сохраняйте SHA исходников приложения, digest образов и SHA manifest в журнале до старта.

## VPS preflight

После установки публичного ключа владельцем через консоль провайдера проверьте независимый вход с `BatchMode=yes` и `StrictHostKeyChecking=yes`. В новой сессии сначала соберите инвентарь.

```sh
cat /etc/os-release
uname -m
lscpu
free -h
swapon --show
lsblk -o NAME,SIZE,TYPE,FSTYPE,MOUNTPOINTS,ROTA
df -hT
ip -br address
ip route
timedatectl
systemctl --type=service --state=running --no-pager
systemctl list-timers --all --no-pager
getent passwd | cut -d: -f1,3,6,7
getent group sudo
ss -lntup
ufw status verbose
sshd -t
sshd -T | awk '$1 ~ /^(port|permitrootlogin|passwordauthentication|pubkeyauthentication|kbdinteractiveauthentication|authorizedkeysfile)$/'
dpkg-query -W 'docker*' 'containerd*' 'postgresql*' 'nginx*' 'ufw' 2>/dev/null
ls -ld /srv /opt /var/lib/docker /var/lib/postgresql /etc/nginx 2>/dev/null
```

Если Docker уже есть, получите `docker ps -a`, `docker volume ls` и `docker network ls`. Если имеются неизвестные базы, слушатели или volumes, установите владельца и назначение до изменений. В журнале сохраняйте инвентарь и коды завершения. Строки конфигурации с секретами оставляйте на VPS.

## DNS и входной Nginx

Оба авторитетных сервера `ns1.mitelis.cc` и `ns2.mitelis.cc` публикуют A `2.26.1.63` для `@` и `api`. Ubuntu Nginx включает конфигурацию `staging-edge.nginx.conf`, Basic Auth, HTTPS redirect и HTTP-01 webroot. Публичный сертификат включает оба имени. Certbot timer и reload hook активны, renewal dry run прошёл.

Хостовый Nginx направляет оба имени на статический адрес внутреннего proxy `10.254.250.10` с проверкой upstream TLS/SNI. Frontend идёт на 8443 и сохраняет same-origin BFF, API идёт на 8444. Docker Compose сохраняет отсутствие published ports. Все внешние webhooks закрыты. После install проверьте оба HTTPS route с авторизацией, app login и Go readiness. Учётные данные edge оператора находятся в local netrc с правами 0600 вне репозитория. Mailpit остаётся доступен только с VPS для синтетического email flow.

## Подготовка релиза и запуск

Используйте текущие deployment Dockerfiles, `scripts/deployment/{package,deploy,control}.py` и неизменяемый web image. Для Linux build проверьте подготовленный офлайн runner, версию Go 1.26.11 и неизменённый Next proof. Проверьте Linux-артефакт и image digest до запуска. Ссылки на фазы и процедуры есть в [действующем VPS runbook](VPS_DEPLOYMENT_RUNBOOK.md). Для Docker Engine и Compose используйте официальный APT-репозиторий Docker после preflight Ubuntu. Публичными остаются 22, 80, 443. Разрешите SSH до активации UFW и проверьте порты с внешнего клиента, поскольку правила публикации Docker могут обходить фильтрацию UFW. [Docker Engine для Ubuntu](https://docs.docker.com/engine/install/ubuntu/) · [Docker и UFW](https://docs.docker.com/engine/network/packet-filtering-firewalls/)

Кандидат использует `APP_ENV=staging`, обязательные HTTPS/DB TLS и отключённые live flags. Синтетические Telegram/WhatsApp идут через существующие workers и очереди. Instagram mock тестируется отдельно, подтверждая, что обычная сборка соблюдает send gate. Для install требуется подтверждение владельца по manifest, проекту и миграциям, затем штатный `deploy.py`.

При готовом релизе штатный координатор использует `bootstrap-check`, `preflight`, `install`, затем `health` и `diagnose`. Согласованный release manifest нужен для существующего GO-механизма. Перед `install` результат preflight должен показать новый проект и пустой volume. Для существующего проекта `update` сначала проверяет архив. Зафиксируйте versions мигратора и PostgreSQL, `goose_db_version`, schema 29, 62 FORCE RLS, результат запроса без tenant-контекста и реальные cross-tenant тесты двух синтетических организаций.

## Приёмка и мониторинг

Соберите `docker compose ps --all` и снимки `docker stats --no-stream`, перезапуски контейнеров, свободное место, memory/CPU и число соединений. Проверьте `/health/live`, `/health/ready`, `/login`, выдачу и удаление сессии, tenant isolation и защиту CSRF. Worker healthcheck показывает наличие процесса. Движение очереди проверяйте по задачам, возрасту backlog, просроченным leases и ошибкам в Agent Runtime, Telegram, общих channel inbox/outbox и mail outbox. Рассмотрите краткий период наблюдения с synthetic load, чтобы увидеть расход ресурсов в том числе во время backup/restore.

Выполните `TestDatabaseAndMigrations`, `TestConversationalIsolated` и `TestInstagramOfflineIsolated` штатным integration runner с отдельной тестовой PostgreSQL и safety marker. В staging повторите smoke со специально созданными синтетическими org, пользователями и задачами. Для recovery зафиксируйте состояние задачи до перезапуска и результат после него, номер попытки, audit event и отсутствие дубля. Повторите для Human Approval, запрета действия чужого сотрудника и ожидаемой блокировки обычного Instagram worker. Перед E2E проверьте свежий build proof в read-only runtime. При drift E01 остановите запуск соответствующего артефакта и сохраните доказательства. [Маршрутизация проверок](AI_DEV_SYSTEM_INSTALL_PLAN.md#unified-quality-workflow--future-execution-only)
