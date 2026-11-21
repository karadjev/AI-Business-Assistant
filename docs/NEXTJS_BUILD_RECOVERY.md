> Актуализация 8Д, фаза 2 (2026-10-21): установка 13 согласованных APK в отдельный Linux/amd64 образ выполнена. Go race, PostgreSQL integration с race, FORCE RLS на 32 таблицах, свежая Next.js production build и production browser E2E Telegram/WhatsApp — PASS. Подробности: [итог фазы 2](BUILD_ENVIRONMENT_PHASE_2_REPORT.md), [race](GO_RACE_FINAL_REPORT.md), [build](NEXTJS_PRODUCTION_BUILD_REPORT.md), [browser E2E](MULTICHANNEL_BROWSER_E2E_FINAL_REPORT.md). Исторические результаты ниже сохранены. Security: CONDITIONAL; открыто наблюдение E01 LOW (неустановленное происхождение пустых каталогов в build workspace). Оба live-канала NOT READY, бюджеты LLM/WhatsApp 0 RUB. Реальные API не вызывались; последующие установки и live-действия автоматически не разрешены.

# Next.js build recovery — 8Г

21.10.2026. **NEXT.JS PRODUCTION BUILD: BLOCKED.** Новая production build не создана; текущие production browser E2E обоих каналов BLOCKED.

## Фактические зависимости

Host Node24.15.0, Darwin x64; isolated cached Playwright1.64.0-noble содержит **Node24.21.0**, Linux x64 glibc. Оба удовлетворяют engines >=24.15 <25, но версии различаются и не скрываются. Package manager npm, `apps/web/package-lock.json` сохранён. Next16.4.0, Tailwind/Oxide4.3.3, lightningcss1.32.0.

На хосте имеются Darwin x64 native packages; безопасный host runner без внешней сети недоступен. Linux glibc runner не имеет:

- `@next/swc-linux-x64-gnu@16.4.0`;
- `@tailwindcss/oxide-linux-x64-gnu@4.3.3`;
- `lightningcss-linux-x64-gnu@1.32.0`.

Повторно прочитан локальный npm cache index: подходящих native tarball entries0. Docker image list проверен локально; существующие runtime web images не дают доказательства наличия полного build toolchain. Не выполнялись image pulls, Docker build с потенциальным registry metadata lookup, installs или обновление lockfile. Полный поиск всех BuildKit cache layers не заявляется.

Существующий host `.next/BUILD_ID` датирован14.10; он не является свежей сборкой текущих исходников. Cached `aba-stage7-smoke-web:latest` также не используется как доказательство новой сборки.

## Исправление проверки

Ранее `scripts/check-next-offline.cjs` печатал MISSING, но завершался exit0, проверяя только два Linux package names. Это могло вводить автоматизацию в заблуждение. Теперь проверяются ОС/arch/libc, **три** native packages, точная установленная версия и успешная загрузка каждого binding. Учитываются закрытые package.json exports: metadata читается локально через resolved package path. Ни одного fetch/install нет.

- BLOCKED → **exit42**.
- Успех означает только **PREREQUISITES_ONLY**, не готовую сборку.
- В `apps/web/package.json` добавлен `prebuild`, вызывающий этот gate до Next, и `test:build-preflight`.
- Пять локальных node:test cases PASS: missing, load/versions, mismatch, musl namespace, unsupported runner.

Реальный `npm run build` в изолированном runner завершился42 на **prebuild**. Сам Next compiler не запускался и artifact не создан. Это честная инструментальная блокировка, не FAIL бизнес-функции и не PASS build.

## План разрешённой среды

Предоставить заранее целостные node_modules/native packages и toolchain для выбранной ОС/arch/libc по сохранённому lockfile. Не копировать Darwin .node в Linux. В Alpine x64 потребуются **musl**, а не gnu bindings; текущий browser runner — GNU. Убедиться в наличии всех plugins, не только этих трёх.

Внутри изолированного runner с read-write **собственной копией** apps/web (не cached image и не user build directory):

```sh
node --version
npm run test:build-preflight
node ../../scripts/check-next-offline.cjs
NEXT_TELEMETRY_DISABLED=1 npm_config_offline=true npm_config_audit=false npm_config_fund=false npm run build
```

Эти команды выполнять только после проверки network isolation: internal/no external default route, no host ports, DNS loopback, empty proxy, no Docker socket. При отсутствии cache прекращать, не запускать install с network fallback. Текущий web-check монтирует source read-only: для настоящей build нужен отдельный writable checkout/artifact volume; не снимать ограничения рабочего проекта произвольно.

**Build PASS:** команда exit0, свежие `.next/BUILD_ID`, manifests и standalone artifacts от текущего source snapshot, запуск этих artifacts без startup errors. Зафиксировать SHA256 source snapshot/build artifacts и дату. Затем bundle scan без печати секретов; запустить **именно эти** artifacts с isolated API/worker/PG и полный browser E2E Telegram/WhatsApp. Не подменять старт старым cached web image. Локальные переменные API key/backend-only, server-only imports и BFF forwarding проверять отдельно.

В8Г current bundle security scan и current production startup NOT RUN из-за отсутствия нового artifact. Source/BFF/component checks PASS в своей области. Внешняя документация и native packages не запрашивались.
