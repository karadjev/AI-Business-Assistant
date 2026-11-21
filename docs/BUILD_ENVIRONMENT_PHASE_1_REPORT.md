# Build environment — фаза1

**Результат: PASS.** Выполнена только явно разрешённая фаза1. Compiler/system packages не устанавливались; phase2, production build, race и browser E2E не запускались. Реальные Telegram/Meta/LLM не вызывались, оба бюджета0 RUB. Commit/push/deployment/host settings не выполнялись.

## Область разрешения и сеть

Внешние запросы выполнялись **только** для официальных Alpine3.23 metadata и npm ci из registry.npmjs.org. Поэтому EXTERNAL CALLS: **AUTHORIZED — Alpine metadata + npm registry**, а не NONE. Проверки signatures/native load выполнялись затем с `--network none`. Image pulls запрещены `--pull never`; использованы уже локальные Linux/amd64 images, без Docker socket и host home/secrets.

## Проверка до npm

- `apps/web/package.json` и package-lock идентичны собственной копии `.local/build-env/web` по SHA256.
- Проверены272 lock entries, dependency specs/resolved URLs и integrity: сторонних registry, git/file/link/workspace или произвольных URLs не найдено; все resolved hosts — registry.npmjs.org.
- Проектные .npmrc отсутствуют; actual container npm config указывает `https://registry.npmjs.org/`, unsafe registry/proxy/auth config keys не обнаружены.
- Для install дополнительно использованы раздельные пустые user/global config, явный registry, `--ignore-scripts --include=optional --no-audit --no-fund`, update notifier off, fetch retries0. Host npm config/.env не монтировались.

## Alpine metadata

Использован cached `golang:1.26.2-alpine3.23`, Linux x86_64; apk-tools3.0.6-r0. До запроса /etc/apk/repositories сравнен с точными официальными main/community URLs.

`apk update` прошёл без `--allow-untrusted`, с штатной проверкой доверенных подписей. Затем **без сети** `apk verify` обоих сохранённых APKINDEX завершился OK. Public trusted-key hashes и index SHA256 сохранены. До/после `apk info -v` полностью совпадает: никаких системных пакетов не установлено.

| Пакет | Точная версия | Архитектура | Репозиторий |
|---|---|---|---|
| gcc | **15.2.0-r2** | x86_64 | Alpine3.23 main |
| musl-dev | **1.2.5-r23** | x86_64 | Alpine3.23 main |

Это snapshot опубликованного индекса, не установленный compiler и не гарантия неизменности будущего repository. APK archives/dependency closure не скачивались. Начальные/конечные предупреждения apk info о пустом default cache не относятся к signature failures; update и отдельный verify завершились0/OK.

Индексы:

- APKINDEX.3ebb674c.tar.gz: `51929d5b9c8ee64140dcafc8a3758ace4827e453252fddcd32ac5a0adea62618`.
- APKINDEX.ab19bef5.tar.gz: `d39019b70de5973e693f2613260d4951f2f2517ee076afe5c43ac06b5038ac6f`.

## Linux npm workspace

Cached `mcr.microsoft.com/playwright:v1.64.0-noble`, Linux/amd64 glibc2.39, Node24.21.0/npm11.19.0. Установка выполнена **только** в `.local/build-env/web/node_modules`, npm cache `.local/build-env/npm-cache`. macOS node_modules не монтировался в install container и не изменялся.

Команда: `npm ci --ignore-scripts --include=optional --no-audit --no-fund --cache=/npm-cache --registry=https://registry.npmjs.org --fetch-retries=0`. **PASS, exit0, added214 packages in54s.** npm проверяет tarball integrity по lockfile; integrity errors не было. Никаких audit/fund/update запросов. В debug log observed URL hosts только registry.npmjs.org; lifecycle `info run` records0.

Первый старт прекратился до разрешения config из-за одинакового `/dev/null` для global/user npm config (double-loading). Исправлено раздельным пустым global config; scope разрешения не расширялся, install scripts не включались.

## Offline native verification

В новом контейнере с `--network none` и read-only mounts запущен существующий строгий `scripts/check-next-offline.cjs`. Все3 packages имеют ожидаемую версию и реально загружаются:

| Пакет | Версия | Статус |
|---|---|---|
| @next/swc-linux-x64-gnu |16.4.0|LOADABLE|
| @tailwindcss/oxide-linux-x64-gnu |4.3.3|LOADABLE|
| lightningcss-linux-x64-gnu |1.32.0|LOADABLE|

Итог CLI: **PREREQUISITES_ONLY**, exit0. Это закрывает отсутствие native packages, но **не является production build PASS**.

## Сохранность и evidence

Исходный и workspace lockfile остались byte-identical, SHA256 `7662043316c773b89100b22b07a56a31ec70d7c3ded2995030ed31323382787d`. package.json и4 host native .node hashes совпадают с pre-operation snapshot. Полная hash-инвентаризация всего macOS node_modules не выполнялась; install его вообще не монтировал. Миграции/бизнес-код/версии зависимостей не менялись. Server env/бюджеты/live gates не затрагивались.

Локальные ignored evidence: `.local/build-env/phase1/` — baseline.json, results.json, apk-update.log, apk-policy.log, apk-index-sha256.txt, apk-trusted-key-sha256.txt, installed-before/after, сами signed indexes, npm-ci.log. Native verification stdout зафиксирован в results.json как итог. npm debug logs остаются в собственном npm-cache; реальные secrets не использовались/не печатались.

## Следующий gate

gcc15.2.0-r2/musl-dev1.2.5-r23 теперь известны. Для загрузки APK/dependency closure, compiler image installation и последующих build/race/E2E нужно **новое отдельное согласование**. Ничего из этого автоматически не выполнено. Система не объявляется live-ready. Работа остановлена после фазы1.
