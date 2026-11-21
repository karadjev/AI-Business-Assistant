# Multichannel browser E2E final — 8Д, фаза2

**Telegram: PASS. WhatsApp: PASS — строго offline/mock на актуальной production build.** Cached frontend не использовался. Реальные provider APIs/delivery не проверялись.

## Artifact и стенд

BUILD_ID `WU4apC5uxd4CeLEneFQxC`, created2026-10-21T18:00:34.152Z, app-source SHA256 `0259af2e7fa1d1ca57a30cfd3c00e8edc7d51ebdafcc45024111435f21202535`. Перед каждым запуском browser wrapper проверял current app inputs/standalone proof и bytes served buildManifest по локальному HTTP. Browser spec hashes отдельно `.local/build-env/phase2/browser-test-sha256.json`. Тестовые ожидания не являются app build inputs, поэтому их корректировка не подменяет artifact старым cached приложением.

Project **aba-phase2-current**, отдельный synthetic PostgreSQL volume, текущие Go binaries API/worker, свежий Next standalone, Mailpit, один общий mock Agent, существующие Telegram/WA mock transports. Internal network, IPv6=false, без external default route/host ports/docker socket; real sending/live LLM запрещены и budgets0. HTTP webhook/wire contracts отдельно прошли Go integration с loopback servers; browser использует actual UI/BFF/DB/worker, runtime mock provider, а не настоящие Telegram/Meta.

## Итоговые запуски

| Запуск | Результат | Evidence |
|---|---|---|
| Новый multichannel production test | **1 PASS,338.772s**,0 skipped/errors/flaky | phase2-multichannel-pass.json |
| Current production registration/catalog/orders/TG/status/error regression | **5 PASS,49.837s**,1 intentional backend-down skip | phase2-regression.json |
| Backend-down после реального stop тестового API | **1 PASS,1.826s**,0 skip/errors | phase2-backend-down.json |

Всего7 реально пройденных Playwright tests; новый большой test включает6 store/channel flows, это не6 отдельных tests. JSON/скриншоты/failed traces находятся `.local/audit8d/browser-results/`. UTC timestamps сохранены в stats каждого JSON. Три screenshots shop-0/1/2-budgets.png относятся к финальному успешному прогону.

## Новый сценарий apps/web/tests/multichannel-production.spec.ts

Три независимых Synthetic8D Shop A/B/C, свои owners/catalog/price/stock и интеграции; отдельные реальные login sessions staff member и read-only member для A. Fixture main — test-only бинарник `apps/api/tests/browserfixture`, не API/worker, без HTTP/debug endpoints. Административная DSN только собственного isolated aba_test; synthetic verified accounts и test clock/window используются для setup. Auth проверяет реальный Argon2 password/session, не fake tokens. Регистрация/email отдельно прошли Mailpit regression.

| Проверка | Telegram | WhatsApp |
|---|---|---|
| Login владельца, own catalog/price/stock |PASS3 shops|PASS3 shops|
| Настройка интеграции и явный mock |PASS|PASS|
| Synthetic inbound → inbox → shared catalog/mock Agent → outbox → mock sent |PASS|PASS|
| Conversation SSR и channel filter |PASS|PASS|
| Exact own variant/price cards |PASS|PASS|
| Нет заявки до quote-bound explicit action |PASS|PASS|
| Повторное confirm не дублирует |PASS|PASS|
| Нет резерва до employee decision |PASS|PASS|
| Подтверждение через UI staff/owner и reserve increment |PASS|PASS|
| Handoff и staff reply/stop auto |PASS|PASS|
| Подписанный webhook + invalid signature403 |Backend race suite|PASS actual local webhook HTTP|
| Async provider read status в UI |Не применимо|PASS|
| Закрытое окно / NEEDS_APPROVED_TEMPLATE / отказ free text |Не применимо|PASS|
| Disable и valid synthetic update404 |PASS|PASS|
| Owner чужой организации через BFF404 |PASS|PASS|
| Read-only member: category/order write403, orders SSR denied |PASS общая RBAC|PASS общая RBAC|
| Budgets0/mock/live-disabled |PASS combined UI/backend|PASS|

Staff A подтверждает две заявки из отдельного login context; для B/C owner является разрешённым сотрудником. Test-only helper воспроизводит expiry атомарно: queued staff message до window expiry, затем worker обязан поставить needs template. Это не публичный обход messaging policy. Отрицательные роли и cross-tenant проверки выполняются backend, не только hidden buttons.

Пост-run SQL по **последним трём** test shops подтвердил: shops3, handoff customer messages6, assistant messages после handoff0, confirmed orders6, reserved units6, active TG0/WA0. Это дополнительное наблюдение реальной DB после browser, не замена RLS/authorization tests. RLS32/channel principal collision/concurrent reserve/signature/dedup/crash/UNKNOWN/budget details подтверждены отдельным race integration suite.

## Ошибки тестов и повтор

1. Первый прогон упёрся в30config requests/min, затем обращался к undefined error response. Поправлены только test pacing/backoff и проверка HTTP status; application limits не повышались.
2. Следующий прогон получил правильный409 «Диалог изменился», пока worker завершал повторный callback. Test теперь ждёт inbox quiescence перед versioned handoff. Backend optimistic locking не ослаблен.
3. Начальный lint no-useless-assignment в новом spec исправлен; lint/typecheck повторены PASS.
4. Перед backend-down guard обнаружил две лишние **пустые** root dirs standalone (`.next 2`, `node_modules 2`). Диагностический hash без них точно совпал с оригинальным proof; runtime files/app inputs не менялись. Удалены только empty generated dirs, proof не регенерировался, strict guard не менялся. После этого current artifact/served gate и backend-down PASS. Источник появления dirs не установлен: открытое LOW наблюдение среды, не кодовая уязвимость и не основание доверять изменённым artifacts.

Failure evidence сохранён отдельно в phase2-multichannel / phase2-multichannel-final; их FAIL не скрываются и не засчитываются PASS. Финальные passed results — отдельные JSON выше. Особенности app-source digest: root tests/playwright config исключены, runtime tree полностью включён; regression доказывает, что app/source/runtime tampering всё ещё блокируются.

## Повтор

Уже подготовленные image/modules используются без downloads:

```sh
docker compose --env-file .local/audit8d/compose.env -p aba-phase2-current -f infrastructure/compose.environment.yml -f infrastructure/compose.environment-smoke.yml -f infrastructure/compose.prepared-build.yml up -d postgres mailpit migrate api worker web
docker compose --env-file .local/audit8d/compose.env -p aba-phase2-current -f infrastructure/compose.environment.yml -f infrastructure/compose.environment-smoke.yml -f infrastructure/compose.prepared-build.yml run --rm -e PLAYWRIGHT_JSON_OUTPUT_NAME=/results/phase2-multichannel-pass.json browser --grep 'current production' --output=/results/phase2-multichannel-pass --reporter=json
```

Перед повтором fresh app artifact/proof должны соответствовать исходникам. Fixture only own isolated database. New spec требует ABA_BROWSER_FIXTURE_BIN/admin synthetic DSN, supplied prepared overlay; без них test намеренно SKIP, который не считается current PASS (например, в неадаптированном старом CI). CI в этом этапе не запускался. До реальных данных/внешних provider нужно отдельное согласование.
