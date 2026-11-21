> Актуализация 8Д, фаза 2 (2026-10-21): установка 13 согласованных APK в отдельный Linux/amd64 образ выполнена. Go race, PostgreSQL integration с race, FORCE RLS на 32 таблицах, свежая Next.js production build и production browser E2E Telegram/WhatsApp — PASS. Подробности: [итог фазы 2](BUILD_ENVIRONMENT_PHASE_2_REPORT.md), [race](GO_RACE_FINAL_REPORT.md), [build](NEXTJS_PRODUCTION_BUILD_REPORT.md), [browser E2E](MULTICHANNEL_BROWSER_E2E_FINAL_REPORT.md). Исторические результаты ниже сохранены. Security: CONDITIONAL; открыто наблюдение E01 LOW (неустановленное происхождение пустых каталогов в build workspace). Оба live-канала NOT READY, бюджеты LLM/WhatsApp 0 RUB. Реальные API не вызывались; последующие установки и live-действия автоматически не разрешены.

# Диагностика среды — 8Д,21.10.2026

## Где выполняются команды

Codex exec работает в macOS/Darwin x86_64, непосредственно с `/Users/ilezkaradjev/Desktop/AI-Business-Assistant.` (точка в конце имени). Это host-процесс с managed filesystem/network sandbox, не Linux Go runner. Docker context `desktop-linux`, endpoint — локальный Unix socket Docker Desktop; daemon Linux/amd64. Исходники остаются на host, тесты запускаются в Linux containers. Внешний CI не использовался.

Права: запись только в разрешённый workspace/temp; повышение прав не разрешено. Network restricted не равнозначно доказанному deny-all для arbitrary host tests. В8Г дополнительный host sandbox_apply отказал; в8Д обход/повтор запуска тестов вне изоляции не выполнялся. На хосте только локальные diagnostics/read/hash/edit и host-refusal script; Go/Node test workloads внутри internal Docker network без host ports. Глобальная блокировка сети компьютера не заявляется.

| Инструмент/параметр | Host | Фактический runner |
|---|---|---|
| OS/arch | Darwin x86_64 | Linux/amd64 Docker Desktop |
| Go |1.26.2 darwin/amd64|1.26.2 linux/amd64, cached Alpine image |
| CGO_ENABLED |1|default0; race service явно1 |
| CC/CXX |cc/c++ (Apple clang)|gcc/g++, но executable отсутствует |
| gcc/clang |/usr/bin/gcc,/usr/bin/clang; Apple clang17 по предыдущей диагностике|Go/Playwright/Postgres16:не найдены |
| libc |Darwin libSystem, не Linux ABI|Go:musl1.2.5, Alpine3.23.4; frontend:glibc2.39 |
| Node |24.15.0|Playwright image:24.21.0 |
| npm |11.12.1|11.19.0 |
| pnpm/yarn |pnpm executable path обнаружен; version не запускалась во избежание shim download; yarn не обнаружен|не нужны, проект использует npm |
| Docker |CLI29.4.0|daemon29.4.0 Linux/amd64 |
| Next/React |16.4.0/19.3.0 по installed/lockfile|те же JS dependencies в bind mount, но неверные native modules |

Локальные проверенные базовые image IDs:

- Go: `sha256:f85330846cde1e57ca9ec309382da3b8e6ae3ab943d2739500e08c86393a21b1` (`golang:1.26.2-alpine3.23`).
- Node/browser: `sha256:06a9939e57531807f8d5fd76ce44b53165ffb7d7501d87ab10e285c20b1e971f` (`mcr.microsoft.com/playwright:v1.64.0-noble`).

Image inspection/run использует pull never, diagnostics — network none. APK cache compiler archives отсутствуют в проверенном Go image; подходящих Linux native npm tarball records в локальном cache0. Это не exhaustive scan всех Docker layers/диска.

## Первопричина Go

Тестовый runner выбран из минимального Go Alpine image, который не содержит C compiler/libc development headers. Host clang не применим внутри Linux container и не может безопасно использоваться вне гарантированной host isolation. CGO=0 достаточен для обычного build, но непригоден для race. Повторять компиляцию без компилятора бессмысленно: новый preflight возвращает42 до `go test -race`.

## Первопричина frontend

`apps/web/node_modules` материализован под Darwin x64. В Linux runner bind mount даёт JS packages, но не требуемые Linux `.node` bindings. Это **ошибка разделения installation workspaces/platforms**, не отсутствие Linux версий в lockfile.

Lockfile реально содержит Linux optional packages и sha512 integrity:

| Пакет GNU Linux x64 | Версия |
|---|---|
| @next/swc-linux-x64-gnu |16.4.0|
| @tailwindcss/oxide-linux-x64-gnu |4.3.3|
| lightningcss-linux-x64-gnu |1.32.0|

Также имеется SWC musl entry; нельзя смешивать musl Go runner и glibc frontend bindings. Все resolved hosts в lockfile — registry.npmjs.org. Lockfile менять не требуется: нужно выполнить locked installation **в отдельной Linux workspace**, сохранив host node_modules. Next preflight из8Г сохранён: missing→42, успешный preflight не равен build PASS.

## Проверки8Д

Go tests/vet/gofmt/build PASS; PG integration PASS26.804s, migrations и FORCE RLS32,3 магазина×2channels, recovery/consent/reserve/budget gates PASS. Frontend lint/typecheck/OpenAPI и6 Node tests PASS. Проверены host refusal, missing compiler refusal, isolated source copy без env/native modules и missing dependencies refusal. Fresh production build/browser E2E и successful race NOT RUN: dependent work остановлена до подготовки tools. Не использовался cached browser для имитации текущего результата.
