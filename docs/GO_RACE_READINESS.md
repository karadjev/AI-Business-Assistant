> Актуализация 8Д, фаза 2 (2026-10-21): установка 13 согласованных APK в отдельный Linux/amd64 образ выполнена. Go race, PostgreSQL integration с race, FORCE RLS на 32 таблицах, свежая Next.js production build и production browser E2E Telegram/WhatsApp — PASS. Подробности: [итог фазы 2](BUILD_ENVIRONMENT_PHASE_2_REPORT.md), [race](GO_RACE_FINAL_REPORT.md), [build](NEXTJS_PRODUCTION_BUILD_REPORT.md), [browser E2E](MULTICHANNEL_BROWSER_E2E_FINAL_REPORT.md). Исторические результаты ниже сохранены. Security: CONDITIONAL; открыто наблюдение E01 LOW (неустановленное происхождение пустых каталогов в build workspace). Оба live-канала NOT READY, бюджеты LLM/WhatsApp 0 RUB. Реальные API не вызывались; последующие установки и live-действия автоматически не разрешены.

# Go race readiness — 8Г

21.10.2026. **BLOCKED**, не PASS. Один релевантный Go module: `apps/api`, Go1.26.2. Обычные tests/vet/build прошли отдельно и не доказывают отсутствие гонок.

## Проверенные факты

- Host Darwin x86_64, Go1.26.2 darwin/amd64, CGO_ENABLED=1, CC=cc; CommandLineTools Apple clang17.0.0 реально доступен. Компилятор на хосте **не отсутствует**.
- Cached isolated runner golang:1.26.2-alpine3.23 — Linux x86_64, default CGO_ENABLED=0, cc/gcc/clang отсутствуют. При явном CGO_ENABLED=1 команда `go test -race -count=1 ./...` завершилась exit1 до выполнения tests: `cgo: C compiler "gcc" not found`.
- В проверенных Playwright1.64.0-noble, postgres:16 и ai-operator-ai-service cc/gcc/clang также не найден. Это ограниченный поиск, не инвентаризация всех слоёв всех Docker images.
- Попытка применить **дополнительное ограничение** host network через `sandbox-exec` только к `/usr/bin/true`, без сетевого probe: exit71, `sandbox_apply: Operation not permitted`. Дочерняя команда не запущена. Повышение прав/обход не выполнялись. Тесты с host compiler не запускались, поскольку их строго offline выполнение не обеспечено.

## Что необходимо предоставить заранее

Локальный trusted image с Go **1.26.2**, совместимым gcc/clang, linker и libc development headers/libraries под Linux amd64. Конкретная версия GCC не фиксируется проектом: необходима поддержка CGO/race Go1.26.2. До проверки записать фактические `go version`, `cc --version`, OS/libc/arch и image digest. Установка/скачивание toolchain в текущем этапе запрещены.

Или заранее подготовленный macOS runner с clang17 и **действующим** запретом внешней сети, допускающим localhost для httptest. Наличие clang само по себе не решает изоляцию. Не повторять неразрешённый sandbox с повышенными правами.

## Точные команды в подготовленной среде

Текущий изолированный Compose: `infrastructure/compose.build-recovery.yml`, project aba-audit8g, pull_policy never, internal network без опубликованных портов/IPv6/default route, Go module cache read-only. Ignored `.local/audit8g/compose.env` содержит только абсолютный путь `ABA_OFFLINE_GOMODCACHE` к существующим модулям.

Создать **собственный** локальный override, указав имя уже предоставленного compiler image:

```yaml
services:
  race:
    image: prepared-go-race:1.26.2
    pull_policy: never
```

```sh
docker image inspect prepared-go-race:1.26.2
docker compose --env-file .local/audit8g/compose.env -f infrastructure/compose.build-recovery.yml -f .local/audit8g/race.override.yml config --quiet
docker compose --env-file .local/audit8g/compose.env -f infrastructure/compose.build-recovery.yml -f .local/audit8g/race.override.yml run --rm race
```

Этот service уже явно устанавливает CGO_ENABLED=1, GOPROXY/GOSUMDB=off, GOTOOLCHAIN=local, GOVCS=*:off, telemetry=off. Не ставить CGO=0 ради PASS. Для интеграционной race-проверки добавить такой же compiler image в **отдельный** integration override, CGO_ENABLED=1 и команду `go test -race -tags=integration -count=1 -v ./tests/integration`; применять только к свежей aba_test этого project. Не повторять integration поверх заполненной тестовой БД: сначала down только собственного tmpfs project. Не использовать рабочую DSN.

**PASS:** реальный race instrumentation запущен, все требуемые тесты завершились exit0 без WARNING DATA RACE. Компиляция без запуска, обычные tests, SKIP или compiler error не равнозначны PASS. После найденной гонки нужен воспроизводящий тест, fix и повтор race+regression. В8Г это не выполнено из-за toolchain/isolation blockers.
