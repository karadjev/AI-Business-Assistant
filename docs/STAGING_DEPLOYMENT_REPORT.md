# AI Business Assistant — staging deployment

## Статус на 2026-11-22

| Критерий | Результат |
| --- | --- |
| CODE COMPLETE | PASS для Linux release tooling, `APP_ENV=staging`, тестовых сценариев и неизменяемого пакета. |
| DEPLOYED | PASS. Проект `aba-private-staging` работает на VPS. Все шесть постоянных контейнеров healthy после общего перезапуска. |
| RUNTIME VERIFIED | PASS. Реальный HTTPS-вход, изоляция двух синтетических организаций, обработка задач всеми семью агентами, Human Approval, аудит и восстановление worker проверены в staging. |
| SECURITY VERIFIED | PASS в границах staging. Проверены SSH, firewall, private network, PostgreSQL role/TLS/FORCE RLS, web session, Basic Auth, CSRF и запрет публичных webhooks. |
| BACKUP VERIFIED | PASS. Созданы архивы до и после миграций, выполнен ручной прогон daily backup и восстановлен snapshot с данными staging в отдельный volume. Внешнее хранилище BLOCKED. |
| LIVE INTEGRATIONS VERIFIED | NOT RUN. Все внешние доставки запрещены, бюджеты 0 ₽. |
| PRODUCTION READY | NO-GO. E01 остаётся OPEN. |

## Источник и VPS

GitHub подтвердил `a216246e5840eb9c7c49945bfa5220f13d9cba38` на `work/stage-2-6-4-conversational-orchestration` и его происхождение от checkpoint `43d1aa888f956c3475fc35487b540f0d5462d965`. Источник развёрнутого пакета — `ab84b42e8939d52b55cdafe8ef8a8638e834a292` в `work/staging-deployment`. Содержимое release inputs сверено с этим опубликованным коммитом по SHA-256.

На `2.26.1.63` инвентаризация подтвердила Ubuntu 24.04.4 LTS, Linux 6.8.0-149, 4 vCPU Ryzen 9 5950X, 7.8 GiB RAM, swap 511 MiB, диск 150 GB и 137 GiB свободного места перед установкой. VPS видит виртуальный диск `vda`; физический тип накопителя из VPS не подтверждён. Системное время синхронизировано. До установки слушали только OpenSSH снаружи и systemd-resolved на loopback; каталогов Docker/PostgreSQL и следов прикладных сервисов не обнаружено.

Docker Engine 29.10.0 из официального репозитория, Compose 5.8.0, Ubuntu Nginx `1.24.0-2ubuntu7.21`, Certbot 2.9.0. Из Docker Registry получены и закреплены по immutable IDs Linux/amd64 Go 1.26.11, PostgreSQL 18.7, внутренний Nginx 1.30.7, Mailpit 1.34.0 и Node 24.21.0 из Playwright 1.64.0. Next.js 16.4.5 собран из lockfile. Пакеты контейнеров обновлены после проверки исправлений поставщиков. [PostgreSQL 18.7](https://www.postgresql.org/docs/18/release-18-7.html) · [Nginx advisories](https://nginx.org/en/security_advisories.html) · [Mailpit releases](https://github.com/axllent/mailpit/releases) · [Ubuntu Nginx backport](https://ubuntu.com/security/CVE-2026-118899)

Независимые сессии `aba-admin` и `root` по ключу прошли после reload. `aba-admin` имеет только ограниченные команды STATUS через sudo. Root key доступ сохранён для сопровождения. PasswordAuthentication и keyboard-interactive выключены. UFW разрешает публично только 22, 80 и 443, политика deny incoming/routed. У проекта отдельные файловые credentials и TLS CA. Контейнерные логи ограничены 10 MiB × 3 на сервис.

## DNS и публичные точки

Оба авторитетных сервера Mitelis вернули A `2.26.1.63` для `ai-business-store.com` и `api.ai-business-store.com`. AAAA и CAA не опубликованы. Let’s Encrypt выдал сертификат с обоими SAN до 2027-02-20 10:10:09 UTC. `certbot renew --dry-run` PASS, таймер активен. Оба HTTPS endpoint подтвердили доверенную цепочку и вернули 401 без Basic Auth. HTTP отдаёт 308, webhooks закрыты на ingress и вернули 404. На работающей системе клиент с Basic Auth получил 200 для frontend `/login` и API `/health/ready`. В основной сессии проверены `__Host-aba_session` с Secure, HttpOnly и SameSite=Lax и CSRF-cookie с Secure и SameSite=Strict.

## Граф зависимостей

```mermaid
flowchart TD
    DNS[DNS и TCP 80/443] --> EDGE[Входной Nginx и TLS, план]
    EDGE --> WEB[Next.js standalone и BFF]
    EDGE --> API[Go API, отдельный hostname, план]
    WEB --> API
    PG[(PostgreSQL persistent volume)] --> MIG[Go migration job]
    MIG --> API
    MIG --> W[Go worker]
    API --> PG
    W --> PG
    W --> MAIL[Внутренний Mailpit]
    W --> Q[SQL очереди и Agent Runtime]
    Q --> PG
    Q --> CHANNELS[Офлайн адаптеры каналов]
```

## Контур и данные

В Compose находятся PostgreSQL, мигратор, Go API, worker, Next.js, внутренний Nginx и Mailpit. Сохраняются PostgreSQL volume, внутренний DNS, внутренние порты 8443/8444, TLS до proxy и verify-full до PostgreSQL. PostgreSQL использует `aba_app` для runtime и `aba_migrator` для выполнения миграций. Лимит пула Go 5 на процесс, PostgreSQL 30 соединений и 512 MB shared buffers. На сервере Compose публикует ноль контейнерных портов. Все живые каналы остаются выключены.

`APP_ENV=staging` требует безопасные cookies, HTTPS origin, проверенный DB TLS и явные нулевые бюджеты и kill switches. Worker включает conversational orchestration через синтетический mock и имеющиеся SQL gate и approval. Production отклоняет mock LLM и Telegram и по умолчанию отключает соответствующие transports. Telegram/WhatsApp staging используют отдельные ключи шифрования. Instagram fixture ограничен отдельной тестовой БД и специальной сборкой. API edge закрывает публичный ingress webhooks.

Последняя миграция `00029_conversational_orchestration.sql`, ожидаемая схема 29 и 62 таблицы FORCE RLS. Registry содержит `customer`, `sales`, `product_inventory`, `marketing`, `finance`, `analytics`, `business_manager`. Все семь вернули доступные permissions через Agent Registry и завершили по одной синтетической задаче после перезапуска worker. Проверено наличие audit events для каждого агента.

## Проверки и evidence

| Проверка | Статус |
| --- | --- |
| GitHub, VPS preflight, authoritative DNS, UFW и sudo | PASS. `vps-preflight.log`, `vps-firewall.log`, `vps-admin-sudo.log`. |
| SSH hardening | Первый FAIL из-за отображения OpenSSH `without-password` вместо ожидаемого `prohibit-password`. Проверка обоих вариантов, reload и независимый повторный вход PASS. |
| Nginx edge, ACME, TLS и renewal | PASS. `vps-edge-install.log`, `vps-public-edge.log`, `vps-certbot.log`, `vps-cert-renewal.log`. |
| Go unit/race/vet/build | PASS на Go 1.26.11 локально. Linux unit/vet/build также PASS. |
| Linux frontend, lint, typecheck, OpenAPI, Next proof и npm audit | PASS на чистом snapshot. Первый lint FAIL из-за AppleDouble `._*` при переносе с macOS. Сборка повторена в отдельном каталоге. Production dependency audit нашёл 0 уязвимостей. Общий audit прошёл порог high, зарегистрировал 11 moderate в dev-зависимостях. |
| Deployment Python и artifact guards | PASS. 35 тестов deployment, отдельные проверки readiness и artifact proof. |
| PostgreSQL и маршрутизация | PASS в выделенных `aba_test`: `TestDatabaseAndMigrations` повторён на 18.7, `TestConversationalIsolated` прошёл на 18.3 для трёх каналов. Изолированные пакеты проверяют tenant isolation, lease recovery и Human Approval. |
| Instagram | Первый отдельный запуск FAIL из-за пропуска пути к обычному worker. Повтор на 18.7 с двумя worker-бинарниками PASS. Обычная сборка сохранила запрет отправки. |
| Chromium | PASS, 5/5, включая регистрацию с Mailpit, сессию, каталог, заказы, mock Telegram, настройки AI и logout. Первые запуски блокировали права bind mounts и sandbox. Официальный Playwright seccomp profile использован только для непривилегированного тестового контейнера. Селекторы обновлены под текущие подписи и мобильный список. |
| Native package preflight | PASS. Кандидат `f5aa3539842be7b991d13e0e7bd6363a9685541ca2dec91c98679274435ad038`, report `/srv/aba/staging/reports/preflight-20261122.json`. |
| Immutable image smoke | PASS. Next image guard и `/login`, отказ API/worker при `APP_ENV=test` и `production`, `nginx -t` и Mailpit readiness под непривилегированными UID. |
| Backup/restore в отдельных проектах | PASS. Архив 843322 байта, TOC и SHA-256 проверены. На восстановленной PostgreSQL подтверждены схема 29, 62 FORCE RLS, 267 policies, ограниченная роль и отсутствие строк без tenant-контекста. |
| Приложение, worker, runtime monitoring и backup основного staging | PASS. Фактические результаты и команды ниже. |
| Внешнее backup-хранилище | BLOCKED. Место назначения отсутствует. |

Команды, результаты и журналы находятся в [локальном индексе](../.local/staging-deployment/INDEX.md). Штатный `deploy.py install` проверяет owner GO, привязанный к точному manifest, проекту и миграциям. Владелец подтвердил точный SHA и миграции 00001–00029 только в новом PostgreSQL volume. Интеграции и клиентский пилот остаются выключены.

## Техническая приёмка запуска

| Проверка | Статус и результат |
| --- | --- |
| Manifest и свежий volume | PASS. Повторно сверён SHA-256 `f5aa3539842be7b991d13e0e7bd6363a9685541ca2dec91c98679274435ad038`. До запуска целевого проекта не существовало. После bootstrap и до миграций пользовательских отношений 0. |
| До миграций | PASS. `before-migration-f5aa3539.dump`, 1 817 байт, TOC и checksum проверены. SHA-256 `e951ec04f1329585101daed61dd7226c8e1738f9c1925dfbee692d14415095a8`. |
| Миграции | PASS. `deploy.py install` с GO и штатной проверкой допустимости. Операторская обвязка выполнила pre-backup, `docker compose run --rm --no-deps migrate` и проверку истории Goose. Применены ровно 00001–00029. |
| После миграций | PASS. `after-migration-f5aa3539.dump`, 843 321 байт, TOC и checksum проверены. SHA-256 `bc8b0a83d837dfe22d90247bf053f21166cec700d1304039b76b5ec25febb5f8`. |
| API, frontend, вход | PASS. `curl` через внешний edge подтвердил 200 с TLS верификацией и Basic Auth. Проверены Mailpit-верификация, аутентифицированная tenant-сессия, cookie flags, CSRF 403 и чужая организация отклонена. |
| Worker и агенты | PASS. Worker остановлен, семь задач сохранены в SQL Queue; повторные запросы вернули исходные task IDs. Проверены `waiting_approval`, чужой отказ, неправильный hash, одобрение и отказ replay. После старта worker все задачи completed, у каждого агента сохранён аудит. В отдельном orchestration suite ранее проверяли истёкшие leases. |
| Каналы | PASS. Реальный staging API принял синтетические события Telegram/WhatsApp, повтор не создал дублей, worker обработал обе очереди и создал синтетические диалоги. Instagram runtime и исходящая отправка выключены. Instagram offline PASS только в отдельном fixture. Публичные webhooks всех трёх каналов вернули 404 на обоих HTTPS-именах после restart. |
| PostgreSQL | PASS. PostgreSQL 18.7, schema 29, 62 FORCE RLS, 0 unforced, 267 политик, ограниченная `aba_app`, 0 организаций без tenant-контекста. Найдено 2 runtime-соединения, оба TLS. Docker network internal; PostgreSQL не публикует порт. |
| Backup timer | PASS. `aba-backup.timer` enabled/active, каждый день 02:30 UTC, `Persistent=true`. Ручной запуск `systemctl start aba-backup.service` exit 0. Настроены 20 GiB floor, 30 дней retention и минимум 2 плановые копии. Защищены отдельные launch snapshots. Плановый запуск по календарю ещё не наступил. |
| Restore с данными | PASS. Архив 851 647 байт восстановлен в проект `aba-private-restore-launch-f5aa3539` с отдельной сетью и volume. Совпали fingerprints задач и аудита, 2 организации, 7 завершённых задач 7 агентов, 36 audit events и 1 исполненное approval. Восстановленный PostgreSQL после проверки остановлен. |
| Общий перезапуск | PASS. Все шесть контейнеров перезапущены и вернулись healthy. PostgreSQL volume identity и fingerprints задач/аудита совпали до и после. `deploy.py diagnose` PASS. |
| Мониторинг | PASS. Host timer `aba-staging-monitor.timer` каждые пять минут проверяет health, место, RAM, CPU/load, рестарты, backlog, ошибки и DB connections. После channel smoke осталось 0 failed и backlog, 3 клиентских соединения включая монитор, около 128.3 GiB свободного диска и 6.7 GiB доступной RAM. |
| Критические логи | PASS в проверяемом окне. Счётчик структурированных ERROR, PANIC, emerg и segfault по шести сервисам показал 0. |
| Сбой в ранней приёмке | FAIL первой внешней Python-проверки из-за TLS EOF и отдельной попытки SSH banner. Другая попытка ожидала dev-cookie `aba_session`, в staging применяется `__Host-aba_session`. Проверка каналов сначала ожидала variants в списке товаров и затем использовала штатный endpoint детали. Повторы PASS. Исходные журналы сохранены. |

Server evidence находится в `/srv/aba/staging/reports/`, локальные журналы перечислены в [индексе](../.local/staging-deployment/INDEX.md). Реальные внешние отправки и production deployment отсутствуют. Внешнее backup-хранилище, естественный календарный запуск таймера и полная production-приёмка остаются открытыми.
