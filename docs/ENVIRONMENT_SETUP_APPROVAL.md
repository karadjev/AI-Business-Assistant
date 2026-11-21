> Актуализация 8Д, фаза 2 (2026-10-21): установка 13 согласованных APK в отдельный Linux/amd64 образ выполнена. Go race, PostgreSQL integration с race, FORCE RLS на 32 таблицах, свежая Next.js production build и production browser E2E Telegram/WhatsApp — PASS. Подробности: [итог фазы 2](BUILD_ENVIRONMENT_PHASE_2_REPORT.md), [race](GO_RACE_FINAL_REPORT.md), [build](NEXTJS_PRODUCTION_BUILD_REPORT.md), [browser E2E](MULTICHANNEL_BROWSER_E2E_FINAL_REPORT.md). Исторические результаты ниже сохранены. Security: CONDITIONAL; открыто наблюдение E01 LOW (неустановленное происхождение пустых каталогов в build workspace). Оба live-канала NOT READY, бюджеты LLM/WhatsApp 0 RUB. Реальные API не вызывались; последующие установки и live-действия автоматически не разрешены.

# План подготовки, требующий разрешения — 8Д

**Фаза1 разрешена пользователем и выполнена; фаза2 НЕ разрешена/НЕ выполнена.** Подробно: [BUILD_ENVIRONMENT_PHASE_1_REPORT](BUILD_ENVIRONMENT_PHASE_1_REPORT.md). Основание согласования — разделы3 и11 ТЗ8Д: внешние запросы/загрузка/установка без отдельного разрешения запрещены. Это не разрешение Telegram/Meta/LLM/платежей/CI/deploy/commit/push.

## Рекомендуемый объём: только Docker, без host installs

| Компонент | Точная цель |
|---|---|
| Go |1.26.2, Linux amd64, существующий golang:1.26.2-alpine3.23 (image ID в DIAGNOSTICS) |
| C runtime |Существующий musl1.2.5/Alpine3.23.4; нужны совместимые gcc и musl-dev |
| gcc/musl-dev APK revisions |**Определены в фазе1:** gcc15.2.0-r2, musl-dev1.2.5-r23, x86_64, signed Alpine3.23 main. Compiler/APK archives не устанавливались/не скачивались; загрузка и установка всё ещё требуют разрешения2 |
| Frontend runner |Существующий Playwright1.64.0-noble, Linux amd64/glibc2.39, Node24.21.0/npm11.19.0 (engines проекта допускает24.x) |
| Linux native npm |SWC16.4.0, Oxide4.3.3, lightningcss1.32.0 GNU x64; остальные dependencies строго package-lock, Next16.4.0/React19.3.0 |

## Разрешение1 — конкретные запросы и Node подготовка

Просим разрешить только:

1. Получить APK metadata из `https://dl-cdn.alpinelinux.org/alpine/v3.23/main` и `/community`, выполнить **в одноразовом контейнере** apk update/policy gcc musl-dev. Это не установка host/system packages. После результата записать versions; compiler download/install требует разрешения2 ниже.
2. Выполнить npm ci по текущему неизменённому lockfile в **отдельной Linux workspace** `.local/build-env/web`, cache `.local/build-env/npm-cache`. Единственный resolved host lockfile — `registry.npmjs.org`; lifecycle scripts/audit/fund/update notifier выключены. Все dependencies exact/integrity из lockfile, не npm install latest. Никакие реальные secrets не монтируются.

Команды **только после разрешения1**, из корня repo:

```sh
# Read signed repository metadata, no compiler installation.
docker run --rm --pull never --network bridge --cap-drop ALL --entrypoint sh golang:1.26.2-alpine3.23 -ec 'apk update; apk policy gcc musl-dev'

# Workspace source copy already created offline by web-build gate.
# Reconfirm package-lock SHA matches apps/web/package-lock.json before install.
docker run --rm --pull never --network bridge --cap-drop ALL --security-opt no-new-privileges:true \
  -v "$PWD/.local/build-env/web:/apps/web" \
  -v "$PWD/.local/build-env/npm-cache:/npm-cache" \
  -w /apps/web -e NEXT_TELEMETRY_DISABLED=1 -e npm_config_update_notifier=false \
  --entrypoint npm mcr.microsoft.com/playwright:v1.64.0-noble \
  ci --ignore-scripts --include=optional --no-audit --no-fund --cache=/npm-cache --registry=https://registry.npmjs.org
```

Эти commands имеют сетевой доступ намеренно и **не входят** в offline test suite. Исполнять только точные одобренные commands; не устанавливать host packages, не запускать package lifecycle scripts, не передавать host home/.env/docker socket контейнеру. Registry availability/integrity текущих версий онлайн ещё не проверены. При404/integrity error остановиться, не обновлять lockfile/versions автоматически.

## Разрешение2 — compiler archives и local image

После metadata1 предъявить фактические **точные** gcc/musl-dev revisions, затем отдельно согласовать загрузку их dependency closure. Этот этап не запускается автоматически по разрешению1. Пример с обязательными значениями:

```sh
# Fill only reviewed exact versions obtained in phase1. Empty variables are an error.
: "${GCC_VERSION:?Reviewed exact APK version required}"
: "${MUSL_DEV_VERSION:?Reviewed exact APK version required}"
docker run --rm --pull never --network bridge --cap-drop ALL --entrypoint sh \
  -e GCC_VERSION -e MUSL_DEV_VERSION \
  -v "$PWD/.local/build-env/go-context/apks:/out" golang:1.26.2-alpine3.23 -ec \
  'apk update; apk fetch --recursive --output /out "gcc=$GCC_VERSION" "musl-dev=$MUSL_DEV_VERSION"; cd /out; sha256sum *.apk > SHA256SUMS'
cp infrastructure/build/GoRace.Dockerfile .local/build-env/go-context/Dockerfile
docker build --pull=false --network=none -t aba-go-race:go1.26.2-alpine3.23-local .local/build-env/go-context
```

Dockerfile устанавливает local archives с checksum verification и apk --no-network; repository signatures проверяет apk. Установка происходит только в новом локальном image. Docker builder может разрешать base metadata через Docker Hub даже при --network=none (это ограничивает RUN, не весь daemon); если потребуется registry metadata, такое обращение также отдельно согласовать в фазе2, не утверждать network isolation build daemon. Base image ID сверить с DIAGNOSTICS; archives/versions/checksums/image ID сохранить. Host Go/compiler/libc не меняются.

## После подготовки — всё offline

Проверить `check-go.sh`: Go1.26.2, C compile/link/run probe; `check-web.sh`: GNU x64 exact native versions/load. Затем race, web-build, artifact verify, full current browser E2E через internal Compose из BUILD_ENVIRONMENT_PLAN. Полный WA browser spec ещё требуется, существующий TG/component suite не заменяет его. LLM/WhatsApp бюджеты0, real API off.

## Риски и откат

Supply-chain packages/metadata, platform mismatch, возможные404 текущих версий, несколько GB локального disk space и build resources. Снижение: signed repositories, exact lockfile+integrity, ignore-scripts install, собственные directories, pinned collected versions/checksums/image ID, отсутствие secrets при preparation; offline тестовая фаза полностью отделена.

Откат: остановить только own test project, удалить только собственный `aba-go-race:...-local` image и `.local/build-env` после проверки, что они не используются. Не делать docker system prune, не трогать чужие volumes/images, host node_modules, lockfile или рабочие данные. Source/config scripts остаются reviewable; их удаление не требуется для отката инструментов.

## Host и CI — не запрашиваются

Host install: **не рекомендуется и не разрешается данным планом**; clang уже есть, но host network isolation не решена. Никаких brew/xcode installs/global settings.

CI: **не выбран и не запускается**. Потребовались бы отдельные разрешения на отправку кода/checkout/actions/tool downloads/runner resources. Локальная подготовка не разрешает CI или облачные ресурсы.

Разрешение1 получено и исполнено: signed metadata и locked Linux npm workspace подготовлены, native3 packages LOADABLE. Разрешение2 не получено, APK download/install и новые build/race/E2E не начаты. Точные версии для review: gcc15.2.0-r2, musl-dev1.2.5-r23. Работа остановлена, дальнейшее согласование автоматически не запрашивается.
