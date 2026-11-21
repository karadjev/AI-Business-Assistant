> Актуализация 8Д, фаза 2 (2026-10-21): установка 13 согласованных APK в отдельный Linux/amd64 образ выполнена. Go race, PostgreSQL integration с race, FORCE RLS на 32 таблицах, свежая Next.js production build и production browser E2E Telegram/WhatsApp — PASS. Подробности: [итог фазы 2](BUILD_ENVIRONMENT_PHASE_2_REPORT.md), [race](GO_RACE_FINAL_REPORT.md), [build](NEXTJS_PRODUCTION_BUILD_REPORT.md), [browser E2E](MULTICHANNEL_BROWSER_E2E_FINAL_REPORT.md). Исторические результаты ниже сохранены. Security: CONDITIONAL; открыто наблюдение E01 LOW (неустановленное происхождение пустых каталогов в build workspace). Оба live-канала NOT READY, бюджеты LLM/WhatsApp 0 RUB. Реальные API не вызывались; последующие установки и live-действия автоматически не разрешены.

# Воспроизводимая build-среда — 8Д

## Выбор

| Вариант | Безопасность | Воспроизводимость | Сеть/установка | Совместимость/поддержка |
|---|---|---|---|---|
| A existing runner/host |Runner isolated, host isolation не доказана|Runner повторяем, но неполон|Нужны compiler и Linux native modules; host tools проблему isolation не решают|Сохраняем для дешёвых unit/lint/PG checks |
| B локальный Docker — **рекомендуется** |Подготовка и тесты разделены, no secrets; offline internal network|Сохраняемые APK archives/hash, lockfile, own Linux node_modules и image ID|Одна отдельно разрешённая preparation; после неё tests без сети|Без host installs, совместим с Go/Next/Postgres/browser, простой повтор |
| C отдельный CI |Зависит от runner/network policy|Хорошая при pinned tools/cache/artifacts|Checkout/actions/package downloads и отправка кода требуют отдельного approval|Существующий CI как будущий вариант, не запускался |

## Две фазы

**PREPARE REQUIRES APPROVAL:** получить compiler archives из signed Alpine3.23 repositories и npm dependencies по существующему lockfile в собственной Linux workspace. Не монтировать .env/реальные credentials. Записать точные APK versions/checksums, native versions, image IDs. Никаких Telegram/Meta/LLM, платных сервисов/host installs.

**OFFLINE VERIFY:** pull_policy never, internal network/no published ports/no usable default routes/no Docker socket, proxies unset, package manager offline, Go proxy/sumdb/toolchain download off. Запустить compiler/native preflight, race, fresh production build, artifact verification и browser E2E именно свежей сборки. Separate Node workspace исправляет первопричину, host node_modules больше не используется для production build.

## Созданные исполняемые инструменты

Запуск через `sh`, chmod не требуется. Exit0 = выполненная операция; preflight0 только PREREQUISITES_ONLY. Exit42 = BLOCKED; failures tests/build сохраняют ненулевой exit. Никаких автоматических installs/downloads.

| Скрипт | Назначение |
|---|---|
| scripts/build-env/common.sh |Host refusal, route/socket checks, offline env. Defense in depth, запуск только в проверенном Compose |
| check-go.sh |Go1.26.2/Linux amd64, compiler + libc headers/linker, реальная compile/run C probe |
| race.sh |check-go, затем CGO1 go test-race, без отключения instrumentation |
| check-web.sh |Существующий строгий native/version/load preflight |
| production.sh |Source-only copy, native preflight, npm build, static/public packaging и проверка proof. Отдельная запись provenance содержит команды, время, PID/PPID и хеши |
| artifacts.cjs |Связь source hash/BUILD_ID/runtime tree/manifest; проверка current artifact и served local build |
| browser.sh |Только после current artifact и served marker match запускает Playwright |
| artifacts.test.cjs |Синтетический proof fixture: missing/stale/tampered rejected, matching accepted. Это не настоящая Next build |

`common.sh` разрешает точный Linux reject-default IPv6 entry00200200 на lo — он unreachable, не маршрут наружу; usable default entries отвергаются. При первом тесте guard ошибочно считал этот reject usable; поправлено, затем safe scripts дошли до фактических compiler/dependency gates. Script не является заменой Docker internal-network configuration.

## Конфигурации

- `infrastructure/compose.environment.yml`: отдельный project aba-audit8d, существующие offline unit/PG/web checks.
- `compose.environment-smoke.yml`: runtime overlay; cached web только историческая диагностика, **в8Д не запускался**.
- `compose.prepared-build.yml`: заменяет web на own fresh artifact и race на подготовленный local compiler image; web-build/browser guards. Не использовать как cached current build.
- `infrastructure/build/GoRace.Dockerfile`: установка **только local APK archives** после checksum verification, apk --no-network; archives ещё не предоставлены, image не создан.

Рабочие каталоги `.local/build-env/web-verified`, `.local/build-env/provenance`, `go-context/apks` и `npm-cache`; всё ignored. Source копируется без node_modules/.next/.env/tsbuildinfo/next-env. Generated next-env не включён в source hash; standalone runtime вместе с node_modules/static хешируется полностью. Старая proof/изменённый source/artifact блокируют browser. Local served marker проверяется на точный BUILD_ID/hash с redirect:error; разрешены только local web/127.0.0.1/localhost. Это защита от случайного использования cached artifact, не криптографическая защита от локального разработчика, способного менять manifests.

## После одобренной подготовки

```sh
docker compose --env-file .local/audit8d/compose.env -f infrastructure/compose.environment.yml -f infrastructure/compose.environment-smoke.yml -f infrastructure/compose.prepared-build.yml config --quiet
# Compiler image must already exist; no pull allowed.
docker compose --env-file .local/audit8d/compose.env -f infrastructure/compose.environment.yml -f infrastructure/compose.environment-smoke.yml -f infrastructure/compose.prepared-build.yml run --rm race
docker compose --env-file .local/audit8d/compose.env -f infrastructure/compose.environment.yml -f infrastructure/compose.environment-smoke.yml -f infrastructure/compose.prepared-build.yml run --rm web-build
```

Для race PostgreSQL suite после подготовки compiler использовать отдельный project `aba-audit8d-integration-race`, те же3files и `run --rm integration`: prepared overlay включает CGO1 и actual `go test -race -tags=integration`. Его БД должна быть чистой, собственный project очищается после теста.

Для runtime использовать **отдельный** project `aba-audit8d-current`, те же3files и `-p aba-audit8d-current`, `up -d postgres mailpit migrate api worker web`, затем `run --rm browser`. Перед этим source/build gates должны реально пройти и свежие Go binaries находиться в audit8d/artifacts. Не смешивать populated integration tmpfs и runtime DB. Никаких host ports. Browser wrapper сначала отказывает при missing proof, до HTTP/browser.

Existing tests покрывают TG, но не полный current-production WA scenario: после разблокировки добавить/запустить WA browser spec (signed webhook fixture, channel filter/status/window/template/handoff/order/staff/reserve,3 tenant BFF isolation). Исполняемый launcher не означает готового полного WA coverage. Браузерный PASS разрешён только после этих tests и actual fresh artifact, не по unit fixture.

## Условия PASS и откат

Race: actual instrumented tests0 без data race. Build: actual Next command0, fresh artifacts и current proof verification0. Browser: verified served build + оба полных channel scenarios0. До этого статусы BLOCKED. После approval успешные пути новых production/browser scripts ещё нужно проверить на реальных build artifacts; сейчас проверены лишь safe refusals и unit provenance logic.

Откат — остановить только own project, удалить только own compiler tag/копию Linux node_modules/cache после проверки отсутствия использующих процессов. Не применять system prune, не менять host packages/lockfile/work volumes. Разрешение на preparation не является разрешением live API.


### Изоляция подготовленной сборки

Архив E01 в `.local/build-env/web` предназначен для расследования. Подготовленный Compose использует `.local/build-env/web-verified` и проверяет proof до старта web. Linux `node_modules` подготавливаются в новом рабочем каталоге из уже проверенного локального набора зависимостей. Каталог `.local/build-env/provenance` монтируется отдельно от исходников и артефакта.

Если Git доступен из исходного дерева, provenance записывает HEAD. При отдельном bind mount подкаталога `apps/web` можно передать `ABA_BUILD_SOURCE_COMMIT` из `git rev-parse --verify HEAD` запускающего checkout. Запись помечает источник как `checkout`, `caller` или `unavailable`. Digest фактических исходников рассчитывается независимо.
