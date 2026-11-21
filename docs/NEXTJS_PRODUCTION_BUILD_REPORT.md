# Next.js production build final — 8Д, фаза2

**PASS — свежая production build реально создана и запущена.** Linux/amd64 glibc2.39, Node24.21.0/npm11.19.0 из существующего Playwright1.64.0-noble. Next16.4.0, React19.3.0. SWC16.4.0/Oxide4.3.3/lightningcss1.32.0 loadable из Linux workspace фазы1.

Новых npm installs/downloads/lifecycle install scripts в фазе2 не было. Выполнен явно разрешённый project build/prebuild script (`npm run build`), а не package installation hooks. Источники копируются в отдельную `.local/build-env/web`, env/host node_modules/.next не копируются. Mac node_modules/lockfile сохранены.

## Команда и artifact

```sh
docker compose --env-file .local/audit8d/compose.env -p aba-phase2-build -f infrastructure/compose.environment.yml -f infrastructure/compose.environment-smoke.yml -f infrastructure/compose.prepared-build.yml run --rm web-build
```

`production.sh`: строгий native preflight → `next build --webpack` → packaged standalone/static/public → proof record+verify. Оба fresh build прогона exit0: actual webpack compilation, TypeScript,22 static-page generation, route manifests/standalone. Новых ресурсов/fonts/packages не загружалось: контейнер internal/no external route, npm offline и telemetry off.

Последняя build:

- BUILD_ID: **WU4apC5uxd4CeLEneFQxC**.
- Created UTC: 2026-10-21T18:00:34.152Z.
- App source SHA256: `0259af2e7fa1d1ca57a30cfd3c00e8edc7d51ebdafcc45024111435f21202535`.
- Immutable proof: `.local/build-env/phase2/build-proof.json`.

`artifacts.cjs` проверяет app inputs/lock/config hash, BUILD_ID, required-server-files, server.js, весь standalone tree (включая runtime node_modules и скопированные static/public), served buildManifest bytes. Root tests/playwright config исключены из app digest: правки тестовых ожиданий не меняют приложение и не требуют искусственной пересборки. Это уточнение проверено regression fixture; hashes самого browser spec сохраняются отдельно. Изменения настоящих app inputs или artifacts по-прежнему блокируют launcher. Это защита от случайной cached build, не от локального разработчика, способного подделать proof.

## Выполненные проверки

- ESLint/typecheck/OpenAPI:PASS; начальный lint error в новом test-only variable assignment исправлен, финальный запуск PASS.
- Native/provenance script tests:6 PASS, включая missing/stale/tampered proof.
- Current standalone web запущен `node .next/standalone/server.js` из собственных artifacts; cached web image не использовался. Local served build gate PASS перед каждым browser run.
- Fresh client `.next/static`:48 файлов,0 exact host-env secret/token/private-key matches. Evidence `.local/build-env/phase2/client-secret-scan.json`; limited scan, не абсолютная гарантия отсутствия любой чувствительной строки. Mock-only synthetic credentials в UI не являются настоящими secrets.
- Backend API URL/session/config server-only boundary и BFF проверяются current browser/regression, а не typecheck вместо build.

API/worker — свежие Go binaries в existing runtime base image; frontend — текущий standalone в Node/browser base image. Повторное использование cached **base runtime** не означает старое cached приложение. No host ports, реальные данные/ключи/интеграции отсутствуют, budgets0.

Состояние browser отдельно: MULTICHANNEL_BROWSER_E2E_FINAL_REPORT. Build PASS не равен разрешению live/production.
