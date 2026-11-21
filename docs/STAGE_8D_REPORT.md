# Этап8Д — восстановление build-среды

Дата21.10.2026. **SECURITY STATUS: CONDITIONAL.** Это инфраструктурная подготовка, не новая бизнес-функциональность и не разрешение live.

| Итог | Результат |
|---|---|
| Environment |Host macOS/Darwin x86_64; Docker Desktop Linux/amd64, isolated Go Alpine / Node GNU runners |
| Root cause Go race |Минимальный Go Alpine image без gcc/libc headers; host clang не решает Linux/isolation |
| Root cause Next |Darwin node_modules bind-mounted в Linux/glibc; lockfile Linux entries присутствуют |
| Recommended strategy |**B: отдельная подготовка локальных Docker tools/deps, затем offline verify** |
| Go race |BLOCKED до compiler preparation; preflight42 вместо бесполезной компиляции |
| Next production build |BLOCKED до locked Linux installation в own workspace |
| Telegram/WhatsApp current production browser E2E |BLOCKED; cached image в8Д не использовался |
| LLM / WhatsApp budgets |0 RUB /0 RUB, live off |
| External calls |NONE для выполненных действий8Д |
| Migrations |NONE;00001–00010 сохранены |
| Нужное разрешение |Фаза1 из ENVIRONMENT_SETUP_APPROVAL: metadata-only Alpine и locked Linux npm preparation; compiler install только после review revisions |

## Что изучено и установлено

Все6 запрошенных отчётов найдены по точным именам и изучены вместе с фактическими scripts/Compose/lockfile/Go module. Повторный Git status показывает весь проект untracked; commit baseline нет. Для изменений создан локальный SHA snapshot, без git add/commit/push.

Фактический host Go1.26.2/Node24.15.0/npm11.12.1, CGO1, cc/c++; Docker client/server29.4.0 через local Unix socket, context desktop-linux. Go runner Alpine3.23.4/musl1.2.5/Go1.26.2, CGO default0, gcc/g++ отсутствуют. Node/browser runner Playwright1.64.0-noble, Node24.21.0/npm11.19.0/glibc2.39. Проверены image IDs и отсутствие compiler в Go/Playwright/Postgres16. Host commands не являются Linux runner, Host Darwin modules не являются GNU/musl bindings. Нельзя лечить проблему отключением CGO или удалением lockfile.

Точные Linux GNU versions уже есть в lockfile с integrity: SWC16.4.0, Oxide4.3.3, lightningcss1.32.0; Next16.4.0/React19.3.0. Только registry.npmjs.org в resolved hosts. Локальных подходящих native npm cache records0, APK compiler archives в Go image не найдены. Compiler package revisions невозможно достоверно назвать без signed repository metadata: они не выдуманы, metadata требует отдельного approval.

Подробно: [ENVIRONMENT_DIAGNOSTICS](ENVIRONMENT_DIAGNOSTICS.md). A/B/C варианты сравнены в [BUILD_ENVIRONMENT_PLAN](BUILD_ENVIRONMENT_PLAN.md); выбран Docker, host/CI не запрашиваются.

## Конкретное решение вместо повторения BLOCKED

Создан самостоятельный workflow:

1. Отдельно согласованная подготовка exact local APK archive set/compiler image и **собственных Linux node_modules** по lockfile; исходный host node_modules не изменяется.
2. Полностью offline verification через internal network/pull never/no host ports, Go/npm download gates, no secrets. Проверяются compiler+headers/linker, native modules/versions, затем actual race/build.
3. Fresh source/build proof и проверка served build до browser; старый cached web не может случайно стать доказательством current production PASS.

Созданы runnable scripts, prepared Compose overlay и Dockerfile, устанавливающий только local APK с checksum validation. Они не скачивают инструменты. Подготовка остановлена до внешних шагов, как требует раздел11 ТЗ. В [ENVIRONMENT_SETUP_APPROVAL](ENVIRONMENT_SETUP_APPROVAL.md) точные команды, источники, версии/неизвестные revisions, изменения, риски, откат и явное разделение host/Docker/Node/CI.

## Доступные проверки — фактически выполнены

| Проверка | Результат8Д | Доказательство/предел |
|---|---|---|
| Local diagnostics/image/config |PASS|versions, image inspect, lock OS/CPU/integrity, cache check |
| Compose base/prepared overlay validation |PASS|config --quiet; не pull/build/install |
| Go tests/vet/gofmt/build |PASS|run --rm go, свежие4 binaries |
| PostgreSQL integration |PASS **26.804s**|.local/audit8d/artifacts/integration.log |
| FORCE RLS |PASS32|runtime aba_app negative tests, tenant-safe FK/policies/context cleanup |
| Migrations |PASS|clean/repeated/concurrent up/down,9↔10 compatibility/data preservation, destructive populated down guard |
| Telegram/WA backend E2E |PASS|3shops×2channels, actual local HTTP mock, own catalog/price, quote consent/order/reserve/tenant isolation |
| Recovery/budgets |PASS в сценариях|real child crash/restart, UNKNOWN/no resend/manual resolve, lease/fencing/retry, conservative ledger/idempotency, zero gates |
| Frontend lint/typecheck/OpenAPI |PASS|web-check exit0 |
| Node script tests |PASS6|5 native preflight tests +1 artifact provenance test с несколькими negative assertions |
| Shell syntax |PASS|sh -n всех6 shell files |
| Host refusal |PASS|check-go на macOS42 до workload |
| Compiler preflight |PASS отказ|race wrapper42, gcc missing; actual race НЕ выполнен |
| Source-only production workspace |PASS copy/refusal|0 env files, no copied node_modules; preflight42 без install |
| Artifact/browser guard |PASS отказ|missing/stale proof42 до HTTP/Playwright; synthetic positive proof только unit fixture |
| Actual race |BLOCKED/NOT RUN|не повторялась бесполезная Go compilation без gcc |
| Actual Next build |BLOCKED/NOT RUN|нет Linux dependencies, compiler Next не стартовал |
| Current production browser TG/WA |BLOCKED/NOT RUN|нет current artifact, cached frontend не запускался |
| External preparation/live/CI |NOT RUN|нет разрешения, никаких запросов/установок |

Новые scripts: common/check-go/race/check-web/production/artifacts/browser и artifact unit test. Успешные real production/race/browser paths ещё требуют исполнения после подготовки; они **не объявлены проверенными** на основании fixture или shell syntax. Browser launcher запускает existing tests; полный WA current-production spec всё ещё нужно добавить/выполнить после разблокировки, как уже зафиксировано8Г.

В первом guard-run обнаружена false-positive проверка IPv6: kernel reject-default на lo принимался за usable route. Поправлено разрешение только точного reject entry00200200/lo, после чего оба wrappers дошли до настоящих dependency gates. Это не отключение network checks и не новое разрешение сети. Полноценная isolation обеспечивается supplied internal Compose, guards — defense in depth.

## Артефакты/безопасность

Own workspace `.local/build-env/web` содержит source-only copy для будущей Linux preparation, без env/native deps; не рабочие данные. Backend logs/RLS metadata в `.local/audit8d/artifacts`. Existing host node_modules, .env, lockfile, Go dependencies, business code, migrations не менялись. Свежий frontend bundle отсутствует, его scan/startup NOT RUN; source/старые доступные bundles проверены ограниченным redacted scan (результат ниже). Секреты не печатались. Все test data synthetic; budgets0 и live/kill switches сохранены.

Сеть host не объявляется глобально отключённой. Любые Go/Node test workloads — internal containers, DNS loopback, proxies off, no Docker socket/host ports, package managers offline. Без проверки возможностей среды не запускались host race/Next. Никаких real tokens/API/messages, платных операций, privilege escalation, system install/settings, CI, deployment, commit/push. Own test containers очищены после проверок; рабочий ai-business-assistant stack не останавливался/не мигрировался.

## Файлы

Созданы16 файлов:

- `scripts/build-env/common.sh`, `check-go.sh`, `race.sh`, `check-web.sh`, `production.sh`, `browser.sh`, `artifacts.cjs`, `artifacts.test.cjs`.
- `infrastructure/compose.environment.yml`, `compose.environment-smoke.yml`, `compose.prepared-build.yml`, `infrastructure/build/GoRace.Dockerfile`.
- `docs/STAGE_8D_REPORT.md`, `ENVIRONMENT_DIAGNOSTICS.md`, `BUILD_ENVIRONMENT_PLAN.md`, `ENVIRONMENT_SETUP_APPROVAL.md`.

Обновлены2: `README.md`, `docs/BUILD_READINESS_REPORT.md`. Итого18. Миграций нет. Изменения сопоставлены с SHA snapshot; deleted0. Нативный preflight8Г с missing→42 сохранён.

## A–T

| Критерий | Статус |
|---|---|
| A фактическая среда |PASS|
| B Go root cause |PASS|
| C Next root cause |PASS|
| D версии/tools |PASS для известных; exact APK revisions требуют metadata approval, не выдуманы|
| E local alternatives |PASS ограниченный inventory, не все disk layers|
| F recommended strategy |PASS Docker|
| G reproducible scripts |PASS подготовлены/negative gates+unit tests; real success paths NOT RUN|
| H false PASS |PASS отсутствуют|
| I race или конкретный план |PASS план; actual race BLOCKED|
| J build или конкретный план |PASS план; actual build BLOCKED|
| K browser только fresh production |BLOCKED, cached не использован|
| L existing channels |PASS backend regression|
| M PG/RLS |PASS32|
| N budgets0 |PASS|
| O real API не вызваны |PASS|
| P downloads без approval нет |PASS|
| Q рабочие данные |PASS сохранены|
| R commit/push/deploy нет |PASS|
| S docs |PASS|
| T next step |PASS конкретный approval1|

## Следующий шаг и согласование

Запросить только **разрешение1**: получить signed Alpine3.23 metadata для точных gcc/musl-dev versions и выполнить locked npm ci --ignore-scripts в отдельном Linux Docker workspace. Это конкретные внешние действия, запрещённые без согласования разделами3/11 ТЗ. Host install/CI/live не входят. Затем предъявить exact APK revisions для разрешения2 compiler archives/image; не устанавливать compiler автоматически по первому разрешению.

До согласования зависимые race/Next/current-browser работы остановлены, остальная offline подготовка завершена. Не обещается полная готовность к пилоту: оба live readiness NOT READY, WA live transport отсутствует, реальные клиенты запрещены.

## Финальная фиксация evidence

Redacted scan720 source/доступных старых bundle files:0 signatures/exact long local-env secret matches; excluded dependency/cache directories и файлы>8MB. Это ограниченный scan, не проверка отсутствующей current production build. SHA migrations1–10, lockfile и Go module files совпадает с baseline; changes2/created16/deleted0. Собственные test containers/network удалены; Linux source-only workspace сохранена для согласованной подготовки, рабочего user data в ней нет.
