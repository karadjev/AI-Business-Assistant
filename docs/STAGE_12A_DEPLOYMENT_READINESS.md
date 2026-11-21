# 12А — Linux deployment package

Последнее обновление10 ноября: **PACKAGE READY, DEPLOYMENT BLOCKED; CUSTOMER PILOT NO-GO.** После запуска Docker владельцем выполнена новая полная isolated rehearsal: Go/race/PG/frontend/fresh Linux build, Browser9/9, Catalog2/2, strict75 HTTP, current image build/load, one-command install, restart persistence, backup/restore и compatible artifact rollback/roll-forward PASS. Current package `.local/vps-readiness-20261110/release/`; current source of truth — `VPS_DEPLOYMENT_READINESS.md` и новый evidence INDEX. Ранний Docker BLOCKED ниже сохранён как история, не текущая недоступность.

3 ноября2026. **DEPLOYMENT PACKAGE READY, STAGING BLOCKED.** Это подготовка и локальная Docker-репетиция, не размещение на VPS и не production-ready.

## Перепроверка 10 ноября 2026

**DEPLOYMENT PACKAGE READY, STAGING BLOCKED.** Текущий review candidate: `.local/recovery12a-20261110/release/`. Это отдельная offline переупаковка прежних неизменённых images/Next proof с актуальными scripts/docs, новым manifest и parent-manifest SHA256. Старый `.local/recovery12a-20261103/release-final/` сохранён; не подменять в нём scripts/manifest и не использовать его старый validator для следующего GO review.

Доказан и исправлен admission-пробел: отсутствие обязательного live/budget/kill flag API/worker раньше принималось. Теперь все 11 controls обязательны для каждого процесса и должны иметь прежние безопасные значения. Regression **FAIL 22 cases до fix**, **PASS 5 tests после**; это не факт включения live, текущий Compose оставался безопасным. Дополнительно **PASS** Node guard tests 6/6, Compose config, сохранённый clean artifact verify и ожидаемый polluted refusal42. Исторические 325 evidence hashes, 2380 release files, archive checksum/OCI graph проверены; proof/inode/timestamps E01 сохранены. Полные logs/exits: `.local/recovery12a-20261110/`.

**Docker daemon сейчас недоступен, socket отсутствует.** Новые Go tests/race/vet/build, PostgreSQL/migration/RLS/tenant/concurrency/consent/staff/channel checks, frontend lint/typecheck/fresh build, Browser/accumulated SQL, image builds/startup/HTTPS smoke и backup/restore — **NOT RUN (BLOCKED Docker)**. Все соответствующие PASS в исторической таблице ниже относятся к 3 ноября. Изменений API/SQL/migrations/frontend/runtime images нет; обязательные prepared runners не обходились host-запуском. Полное `control.py validate`, включая inspect локальных image IDs, тоже BLOCKED до восстановления Engine; offline manifest/Compose verification его не заменяет.

Archive содержит OCI index/manifest IDs, проверенные по descriptor graph и linux/amd64 configs/layers. Реальный `image load`/inspect/контейнерный запуск на целевом Ubuntu Engine ещё NOT RUN; совместимость image store нельзя считать доказанной одной проверкой tar. До переноса сверить новый manifest по доверенному каналу, после разрешённой загрузки обязательно подтвердить ровно эти IDs; при несовпадении STOP. Новая упаковка не является image rebuild или server GO.

E01 и две metadata отмены OPEN; Security CONDITIONAL. Следующий шаг: доступный локальный Docker → повтор prepared isolated checks/new package validation → 12Б review blockers и отдельный owner GO. Базовая согласованная конфигурация остаётся Ubuntu 24.04 x86_64, 4 vCPU/8 GB/80 GB SSD; меньший VPS не проверялся. Никаких downloads/global settings/server/live действий не выполнено.

Остальные разделы сохраняют историю подготовки и проверок **3 ноября 2026**.

11А остаётся OPEN, E01 OPEN, Security CONDITIONAL.10E USER-REPORTED PASS не превращается в формальную HUMAN REHEARSAL PASS. Telegram/WhatsApp/LLM live OFF; внешний бюджет0 RUB. Сроки audit/receipts не менялись.

## Разрешённый результат

Подготовлен самостоятельный private-staging package для **Ubuntu24.04 LTS, linux/amd64,4 vCPU,8 GB RAM,80 GB SSD/NVMe**, Docker Engine/Compose. Это заданная владельцем конфигурация, а не доказанный минимальный production sizing. Docker Desktop на macOS не заменяет проверку реального VPS. ARM64 не проверялся и отвергается packager.

Используется существующая архитектура: Go API/worker/migrator, PostgreSQL18, Next.js/BFF, HTTPS nginx, локальный Mailpit. Нового сервиса идентификации/каталога нет. Production auth требует HTTPS и PG `sslmode=verify-full`; эти проверки сохранены. Существующий production-запрет обработки Telegram/WhatsApp с ключами также сохранён. Канальные regressions выполняются штатными mock runners, не включением processing/live в production.

## Файлы

- `apps/api/internal/catalog/control.go`: transaction-local JIT tuning только ControlPage, см.11А.
- `apps/api/tests/integration/catalog_scale_test.go`: server-side JIT scope, commit/rollback и reuse одного connection между tenants.
- `apps/api/cmd/runtime-entrypoint/main.go`, `main_test.go`: whitelist `_FILE`, приватные regular files, запрет symlinks/конфликтов/oversize/multiline; exec с сохранением PID/signals. Секреты не печатаются.
- `infrastructure/deployment/api.Dockerfile`: static Go artifact image, scratch, UID10001.
- `infrastructure/deployment/web.Dockerfile`, `web-entrypoint.cjs`: glibc-compatible Node/native runtime из уже cached компонентов; guard при каждом старте до Next. Браузеры/build tools в final image не копируются.
- `infrastructure/deployment/compose.yml`: production environment, internal network,0 published ports, раздельные runtime/migrator secrets, health/startup ordering, caps/memory/pids/read-only application roots, log rotation.
- `infrastructure/deployment/nginx.conf`, `pg_hba.conf`: HTTPS8443 внутри сети; PG hostssl/SCRAM только ролям приложения/мигратора в `aba_stage`.
- `infrastructure/deployment/init-roles.sql`: те же restricted roles/schema grants, что проектный bootstrap; новый volume, без передачи passwords в argv.
- `infrastructure/deployment/environment.example`: paths/origin/image placeholders, без credentials.
- `scripts/deployment/package.py`: offline preflight, static linux/amd64 binaries, unchanged build guard до/после export, новые image tags, immutable image IDs, manifest, image archive/checksum. Никакого push/deploy/pull.
- `scripts/deployment/common.py`, `control.py`: manifest/config/image validation; запрет host/public networking, inline credentials, live/budget bypass; отдельный owner GO для start/stop/rollback.
- `scripts/deployment/backup.py`: private dump, checksum, восстановление только в пустую isolated rehearsal DB; сохранение owners/ACL/FORCE RLS, без global role/password dump.
- `scripts/deployment/test_private_controls.py`: fail-closed regression для exposure/live/config tampering.
- Этот документ, `VPS_DEPLOYMENT_RUNBOOK.md`, обновления11А/11Б.

Обычные `apps/*/Dockerfile` остаются прежним source-install маршрутом; для этого offline package используются **только** artifact Dockerfiles выше. Никаких npm/apk/go-module installs, новых dependencies, CI/AGENTS/PATH/global changes нет.

## Package и evidence

Private evidence: `.local/recovery12a-20261103/`. Финальный release: `.local/recovery12a-20261103/release-final/`. Перед переносом использовать его `manifest.json`, `images.tar.sha256`, `images.env`, вложенные scripts/config. **Не переносить всю `.local`, QA credentials, старые volumes или raw диагностические данные.** Manifest status PACKAGE_ONLY_NOT_DEPLOYMENT_GO.

Build input/source монтируются read-only, Next output находится в отдельном Linux named volume `aba-recovery12a-web-20261103`. Сначала проверяется весь runtime tree прежним guard, потом экспорт, ещё одна проверка, image packaging и startup guard. При отказе публикация пакета/выдача manifest не выполняется; runtime завершается42 до Next. Docker push вообще отсутствует. Исходный загрязнённый artifact всё ещё отклоняется42; proof/baseline не переиздавался. Clean builds не доказывают причину E01 или byte-for-byte reproducibility разных Next BUILD_ID.

Cached support images — кандидаты для QA, не подтверждение актуальности/advisory safety. Перед реальным GO нужны provenance/advisory review и согласование base images; сеть/advisory fetch в этом задании не разрешались. Release фиксирует реально использованные local image IDs и linux/amd64, а не изменяемые tags.

## Проверки

Точные текущие gate exits и финальные результаты приведены в11А и `INDEX.md` evidence; строки ниже не заменяют исходные FAIL.

|Проверка|Результат|
|---|---|
|Go tests/vet/gofmt/build, race после JIT fix|PASS `go-final`, `race-final`|
|Fresh PostgreSQL integration/race/RLS после fix|PASS164.010s `integration-final`; новая JIT scope regression включена|
|Frontend lint/TypeScript/OpenAPI/6 native/artifact unit checks|PASS `web-check-current`; frontend source не менялся|
|Fresh isolated Linux Next production build|PASS `web-build-linux`, guard record/verify; предыдущий polluted output не использован|
|Pre-fix fresh Browser full|PASS9/9 `browser-fresh-full`; не подмена post-fix результата|
|Post-fix full Browser / accumulated Catalog|PASS9/9,899.713s; Catalog2/2,66.795s/53.130s; cold-read/write retry=false; normal PG0 timeouts|
|Accumulated strict HTTP,75 calls/4workers/1s per SQL|PASS `accum-sql-final`, normal PG без hook/client tracer, default JIT restored|
|Artifact Docker builds/export|PASS `package-build-final`; scratch Go и совместимый glibc Node/native images|
|Native Sharp runtime|Первый candidate FAIL из-за отсутствующего cached `libresolv`; исправлено копированием существующей библиотеки, v2 PASS1×1 PNG, без install|
|Compose validation / TLS private startup|PASS final package; production API/worker/web/PG/proxy healthy, migrate exit0|
|HTTPS/auth/CSRF local smoke|PASS final: CA verified, login200, unauth401, local email verification, Secure/HttpOnly cookie, missing CSRF403, catalog200, logout revocation|
|Backup/restore rehearsal|PASS final6001 variants/237 imports в отдельную новую DB; ownership/effective ACL/policies/FORCE RLS match; restored runtime ready200; чужой/анонимный контекст0 строк|
|Nonempty restore / tainted artifact|Ожидаемые отказы: restore exit2; runtime exit42. Данные не перезаписывались|
|Deployment admission unit tests|PASS4 tests, включая группы unsafe mutations|
|Real Ubuntu/VPS/SSH/firewall/rollout/rollback|NOT RUN; нет подключения к серверу или внешнего размещения|
|Dependency advisory scan / production customer load|NOT RUN; не заявляются проверенными|

Первоначальные smoke FAIL сохранены: неверно разобранный YAML tmpfs flow list; `.sh` bootstrap не исполнился и оставил частично initialized volume; nginx пытался создать default fastcgi temp на read-only root. Исправлены quoted mounts, официальный `.sql` bootstrap и все temp paths в `/tmp`. Failed DB сохранена, следующий smoke использовал новый project/volume; ручного исправления roles в повреждённом startup volume не было. Ни read-only root, ни no-new-privileges ради PASS не отключались.

## Security boundaries и ограничения

API/worker — UID10001, proxy — UID101; PostgreSQL official bootstrap временно использует root для своего volume, сервер работает своей штатной ролью/UID. Runtime/migrator не получают superuser DSN. Docker socket не монтируется. Приложения имеют cap_drop ALL/no-new-privileges; PG требует обычного volume bootstrap и не объявляется полностью rootless. Все сети QA/internal; никаких ports на host, включая PG/SMTP/Mailpit/proxy.

Resource limits — caps, не reservations: PG1.5 CPU/2GiB, API и worker по0.75 CPU/512MiB, web1 CPU/1GiB, proxy/Mailpit по0.25 CPU/128MiB; transient migrator0.75 CPU/512MiB. Сумма работающих caps4.5 CPU допускает sharing на4vCPU; полноценный capacity/latency test именно VPS ещё нужен. Worker healthcheck подтверждает **liveness**, не успешную обработку queue. Nginx `-t` проверяет config, отдельный HTTPS smoke проверяет serving.

Application logs ограничены10MiB×3 на container; это ротация process logs, **не retention audit/receipts в PostgreSQL**. Proxy access logging выключен; nginx error context всё ещё может содержать URI, поэтому отсутствие любых query strings в будущих error logs не гарантируется. До real-data использования нужен отдельный logging/privacy review; текущие проверки только synthetic. Backup сейчас plaintext custom dump с0700 directory/0600 file. Для реальных данных до GO нужны отдельно согласованные encryption/key custody/storage/access решения; автоматического удаления/retention нет. Секретные файлы/ключи восстанавливаются отдельно и согласованно с dump; иначе невозможно расшифровать provider/email/retention state.

## Что блокирует следующий шаг

1. E01 writer/mechanism неизвестны. Linux volume + immutable image + fail-closed guard ограничивают артефакт, но не закрывают E01. fs_usage требует нового отдельного разрешения, сейчас NOT RUN.
2. SQL JIT mechanism доказан в новой reproduction с captured actual plans; первые150 cancels не имеют incident plan. В diagnostic oracle также остались2 last-import metadata cancels, причина NOT PROVEN; не маскируются поздним PASS и не объявляются исправленными по одному улучшенному EXPLAIN.
3. Security CONDITIONAL, advisory/provenance review, реальные VPS controls/capacity и owner GO отсутствуют.
4. Private origin/cert trust, SSH access, encrypted backup custody, storage monitoring и rollback compatibility должны быть согласованы до сервера. Реальные магазины/email/live каналы этим package не разрешены.

Следующий этап: **12Б — private staging GO review и ограниченная Ubuntu rehearsal после устранения blockers либо отдельного явно документированного решения владельца об остаточном риске**. Такое решение не является автоматическим закрытием E01/11А или разрешением live-пилота.
