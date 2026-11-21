# Этап 10В — retention capability и масштабирование каталога/истории

Дата: 2026-10-30, Europe/Moscow. Scope — первоначальные STAGE_10C_READINESS и STAGE_10B_REPORT, по явному решению пользователя. Demo Experience & Sales Journey перенесён в отдельный10Г. Security CONDITIONAL; E01 OPEN.

## Реализация

- Owner operator workflow: отдельный POST retention-binding в существующих API/BFF и подтверждение в «Правила и хранение». Tenant выбирается аутентифицированным owner; proof/hash не принимаются HTTP и не возвращаются. Обязательны confirm и expected_version. Tenant advisory lock + policy row lock + version fence; binding/version/audit атомарны. Concurrent/stale confirmation получает409; ошибки audit откатывают binding. Нет global rebinding или GET side effects. Upload/save совместимы с прежним поведением.
- Необязательный CATALOG_RETENTION_KEY отделяет ротацию очистки от email encryption key. Без него прежний domain-separated derivation сохранён; некорректный заданный ключ блокирует старт. API и worker используют один выбор ключа. Никакие рабочие ключи/настройки не менялись. Procedure и in-flight/mixed-version ограничения: RETENTION_CAPABILITY_ROTATION.md.
- Центр считает metrics/health SQL-агрегатами по всему неархивному tenant catalog. Явные page/q/filter/import_page включают серверные страницы:50 вариантов,100 receipts, totals и metadata. Поиск literal substring по name/SKU и прежние freshness/price/incomplete/low-stock filters выполняются на сервере. UI явно запрашивает page1; фильтры применяются кнопкой. Введённые и применённые критерии разделены: переход страницы/refresh не применяет незавершённый поиск. Browser regression проверяет эту последовательность на6008 вариантах.
- Совместимость: запрос catalog-control без query сохраняет прежние строки до5000 и CATALOG_LIMIT выше5000. Масштабируемый UI использует новый явный постраничный режим; существующие поля ответа и source history recent100 сохранены. Нет молчаливо усечённого health score.
- История импортов доступна за recent100, с created_at DESC/id DESC ordering, деталями и существующим hold. Raw XLSX не хранится. Summary/receipts/audit не удаляются.
- AUDIT_RECEIPTS_RETENTION_PROPOSAL.md содержит только неутверждённые варианты и вопросы. Новые сроки/TTL/purge для receipts/source/security/financial audit НЕ установлены.

## Миграция и безопасность

Добавлена00015_catalog_read_scope.sql после фактически последней00014. Старые миграции не менялись. Схема15 обязательна для новых API/worker; рабочего rollout не было.

Необходимость подтверждена нагрузкой: первоначальный агрегат не укладывался в неизменённый SQL timeout. В миграции membership в tenant SELECT policies вычисляется один раз на statement; row tenant equality и проверка действующей сессии сохраняются. В telegram_read/whatsapp_read каталоговых policies добавлена ранняя проверка отсутствующего capability. При наличии capability исходный scope predicate, lease, active integration, test tenant и product checks выполняются как прежде; expiry не кэшируется отдельно. Общая freshness calculation вынесена в inventory_freshness_at; прежний facade inventory_freshness и customer/reserve/dispatch contract сохранены.

Нет новых таблиц, SECURITY DEFINER grants, BYPASSRLS, global admin scope, DELETE или ослабления write policies.45 tenant tables сохраняют ENABLE/FORCE RLS. Изолированная финальная БД: schema15,45 FORCE RLS tables;7 synthetic организаций с non-test flag созданы отрицательными тестами admission и не являются рабочими данными. Down на пустой БД восстанавливает прежние policies/functions, populated catalog/provider state сохраняет fail-closed rollback refusal.

## Проверки и нагрузка

Результаты ниже относятся к текущему коду и отдельным synthetic окружениям; исторические результаты10Б не используются как PASS.

| Проверка | Результат |
|---|---|
| Go tests / format / vet / build | PASS, go-current.exit0 |
| Go race | PASS, race-current.exit0 |
| PostgreSQL integration/race, migrations/RLS | PASS82.613s, integration-final.exit0;45 FORCE RLS tables |
| Telegram / WhatsApp regression | PASS в полном PostgreSQL suite; обе adapter/consent/stock/outbox/financial/tenant ветки |
| Frontend lint / TypeScript / contract / native guards | PASS, web-check-final.exit0;6 native/artifact tests |
| Fresh production build | PASS, web-build-final.exit0; Next16.4.0 webpack |
| Current production Browser E2E / mobile390×844 | PASS3;0 skipped/unexpected/flaky;533.201s, browser-final.exit0; desktop/mobile screenshots и overflow assertions |
| Build integrity / served marker | PASS: post-build, final served/current artifact, post-cleanup verify; original final proof unchanged |

Новый PostgreSQL сценарий:6001 варианта, полный обход121 страницы,236 receipts (235 новых плюс ранее минимизированный receipt), проверка уникальности/полноты, aggregate totals/health, поиск за первой страницей, tenant isolation, совместимый legacy limit. Финальный замер: elapsed44.085s, p50 336.509ms, p95 431.905ms, max690.363ms, max response73314 bytes. Полный suite82.613s. Все6001 variant IDs и236 receipt IDs получены без пропусков/дубликатов. Три subprocess helper tests отмечены SKIP по дизайну; соответствующие parent crash tests реально выполнены. Это synthetic sequential load, не production capacity/SLA и не тест миллионов строк. Ротация проверяется с разными synthetic proofs, staff/outsider denial, explicit consent, stale/concurrent confirmation, injected audit failure и rollback, отзывом старого policy scope и отсутствием global rebinding.

Общий integration harness deadline увеличен120→300 секунд для121 запросов; SQL statement timeout тестового pool остаётся1 секунда, runtime default2 секунды и request timeout не менялись. Heavy gates запускаются последовательно, с cached images/modules/native bindings, pull_policy never и internal networks. Отдельные Compose projects:aba-stage10c-build, aba-stage10c-db, aba-stage10c-browser. Только synthetic aba_test; рабочие DSN/containers не используются. Browser runtime отдельно проверяет dedicated synthetic retention key API/worker.

Исторические PASS10Б не подменяют текущие результаты. Промежуточные FAIL сохранены в.local/audit10c: обязательное поле нагрузочной фикстуры; transient compile error при ограничении нового request field; соответствие rollback guard новой latest migration; SQL timeout на нагрузке. Промежуточный полный тест завершился PASS, но изменённая во время работы локальная оболочка сохранения exit status дала ошибку; она исправлена, окончательный gate запускается заново. Полные логи не сокращены до terminal tail.

Первый current-production Browser E2E:3 PASS,0 skipped/unexpected/flaky,546.389s; его post-run integrity/served PASS. После исправления draft/applied filter semantics frontend lint/typecheck и fresh build повторены; финальный browser gate завершён на новом proof:3 PASS,0 skipped/unexpected/flaky,533.201s; start2026-10-29T22:04:19.017Z. Это обоснованный повтор после изменения кода, первый результат не выдаётся за финальный.

Предварительные EXPLAIN выполнялись только в изолированной БД под aba_app; локальные SQL-перестановки для исследования плана не являются миграцией рабочего окружения. Окончательная миграция проверяется заново через goose на чистой БД. Неудачная перестановка имён policies не вошла в миграцию.

## E01 и границы

E01 OPEN: первопричина не установлена. До build неожиданные .next 2/node_modules 2 отсутствовали в выделенном workspace. Guard не изменён, baseline не переписывается для принятия drift, неожиданные paths не удаляются. Fresh build proof: BUILD_ID 5slS7mCoUYNuFjHTRi-Jw, created2026-10-29T22:02:54.732Z, source SHA256 2993c8f4ad447105229b8170615055c2ffedfae5f0eeb23f31ac649750ece730. Непосредственный post-build verify PASS; served/final integrity PASS после E2E; после cleanup повторный verify PASS, bytes исходного final proof неизменны. E01 не наблюдался на обоих build/browser прогонах при интервале observer500ms; это не causal proof отсутствия первопричины.

Offset pagination не является frozen export: concurrent inserts могут сдвинуть страницы каталога и истории; нужен refresh. Bounds:page/import_page≤10000, по50/100 строк; larger-than-tested sizes, long-period export и source history beyond100 — отдельный scope. Source/category freshness overrides не расширялись без реальной потребности.

Ротация требует одинакового ключа всех API/worker, штатного завершения старых cleanup transactions и owner confirmation для каждого tenant. Прежний integration-proof fallback не отзывается автоматически. Recovery/rollback ключа требует отдельного operator решения; настоящий secret rotation не выполнялся.

## Внешние вызовы и следующий этап

В этом этапе внешние API/downloads не выполнялись; всё тестирование локальное/offline/mock. Telegram/WhatsApp/LLM/1С live не включались; бюджеты LLM/WhatsApp0 RUB. Context7 не использовался. Новые зависимости/установки не добавлялись. Commit/push/deploy, CI/AGENTS/PATH/global Codex settings не менялись. Чужие процессы/контейнеры не останавливались.

Audit/receipts proposal ожидает отдельного согласования владельца продукта.10Г определён в STAGE_10D_READINESS.md; Offline readiness к10Г подтверждён текущими gates, с Security CONDITIONAL и E01 OPEN; реализация10Г — отдельное будущее задание. Существующие live blockers не сняты. Live Telegram/WhatsApp NOT READY.

## Файлы и evidence

Изменено18 существующих файлов, добавлено6, удалено0. 21 защищённый исходный файл (старые миграции/embed, dependency manifests/locks и artifact guards) проверен неизменным. Список сверен с исходным SHA256 baseline, поскольку Git HEAD отсутствует.

- `apps/api/cmd/worker/main.go`
- `apps/api/internal/catalog/control.go`
- `apps/api/internal/catalog/control_test.go`
- `apps/api/internal/config/auth.go`
- `apps/api/internal/config/auth_test.go`
- `apps/api/internal/http/onboarding.go`
- `apps/api/internal/platform/db/db.go`
- `apps/api/migrations/00015_catalog_read_scope.sql`
- `apps/api/tests/browserfixture/main.go`
- `apps/api/tests/integration/catalog_scale_test.go`
- `apps/api/tests/integration/database_test.go`
- `apps/api/tests/integration/store_import_test.go`
- `apps/web/src/app/api/[...path]/route.ts`
- `apps/web/src/app/dashboard/catalog/page.tsx`
- `apps/web/src/components/CatalogControlCenter.tsx`
- `apps/web/tests/catalog-control-production.spec.ts`
- `docs/AUDIT_RECEIPTS_RETENTION_PROPOSAL.md`
- `docs/CATALOG_CONTROL_CENTER.md`
- `docs/IMPORT_RETENTION_POLICY.md`
- `docs/RETENTION_CAPABILITY_ROTATION.md`
- `docs/STAGE_10C_REPORT.md`
- `docs/STAGE_10D_READINESS.md`
- `packages/contracts/openapi.yaml`
- `scripts/check-openapi.mjs`
 Ignored evidence:.local/audit10c (source baseline, full logs/exit/provenance, EXPLAIN, protected hashes, E01 observations); synthetic browser screenshots/traces:.local/audit8d/browser-results/stage10c-final; первоначальный прогон сохранён отдельно вstage10c. Публичных публикаций evidence нет.


## Итог и очистка

Согласованные задачи10В IMPLEMENTED: operator workflow, server pagination/whole-catalog aggregate, import history beyond100, отдельное неутверждённое retention proposal. Обязательных offline blockers10В не осталось. Security CONDITIONAL; E01 OPEN; live TG/WA NOT READY.

Все окончательные gates имеют exit0 в.local/audit10c/final-gates.json. Отдельные тестовые containers/networks/volumes проектовaba-stage10c-build/db/browser удалены и отсутствие проверено по точным project labels. Observer штатно завершён marker-файлом; неожиданные E01 paths не удалялись. Evidence, cached build workspace и неизменённый final proof сохранены. Рабочие data/services не менялись, чужие процессы не завершались.

Открытые вопросы: срок и архивирование минимальных import receipts; горизонт idempotency; source/security/financial audit; hold/backup/privacy/operator obligations. До отдельного утверждения остаётся прежнее сохранение receipts/audit. Ни предложение, ни готовность10Г не разрешают их удаление или live.
