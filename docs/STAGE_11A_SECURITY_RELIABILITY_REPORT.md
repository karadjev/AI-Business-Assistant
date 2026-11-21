# 11А — Security & Reliability Hardening

Последний результат **10 ноября 2026, fresh VPS package rehearsal**: Docker доступен; новые Go/race/PG/frontend/build/Browser9/9/Catalog2/2/strict75 HTTP/private install/restart/restore/rollback gates PASS. **11А OPEN; E01 OPEN; Security CONDITIONAL; PACKAGE READY, DEPLOYMENT BLOCKED; CUSTOMER PILOT NO-GO.** Подробности и пределы: `VPS_DEPLOYMENT_READINESS.md`, evidence `.local/vps-readiness-20261110/`. Ранние записи о недоступном Docker ниже относятся к предыдущей фазе этого дня.

Актуальная перепроверка: **10 ноября 2026**, раздел «Перепроверка 10 ноября» в конце. **11А OPEN; E01 OPEN; Security CONDITIONAL; DEPLOYMENT PACKAGE READY, STAGING BLOCKED.** Приведённые ниже runtime PASS от 3 ноября — исторические результаты, не новые прогоны.

Обновление: **3 ноября2026, Recovery11А +12А**. **11А НЕ ЗАКРЫТ; Security CONDITIONAL; E01 OPEN / ROOT CAUSE NOT PROVEN.** Aggregate1s: **ROOT CAUSE PROVEN для нового воспроизведения JIT emission при1.5vCPU**, scoped fix проверен; точная атрибуция первых150 отмен остаётся NOT PROVEN. Отдельные2 отмены last-import metadata — **ROOT CAUSE NOT PROVEN / OPEN**. Deployment verdict: **DEPLOYMENT PACKAGE READY, STAGING BLOCKED**; никакого server/deploy GO.

Все разделы до «Recovery11А +12А» ниже сохраняют историю предыдущих запусков, включая FAIL, прежние гипотезы и PASS только их конкретных прогонов. Более поздний результат не переписывает их. Текущее разрешение **не включает sudo/fs_usage, административный пароль или deployment**. Предложения host audit — только отдельно согласуемый план. Новые результаты и границы causal proof приведены в последнем разделе.


## Основание и10E

Владелец сообщил о ручной проверке интерфейса и работе основных функций: **USER-REPORTED PASS**, запись добавлена в STAGE_10E_READINESS.md. Полный checklist обоих каналов, времена, помощь оператора, mobile/keyboard и protocol evidence не представлены. Формальная HUMAN REHEARSAL не подтверждена;10E не закрыт. Разрешённый11А совместим с readiness: техническое расследование не включает live rollout или изменение критериев10E.

Изучены действующие глобальный/проектный AGENTS,10D/10E/10F, build-integrity/recovery/environment отчёты и исторические E01/SQL материалы. Индексы путей и SHA256: `.local/hardening11a/historical-reports.json`, `historical-evidence-index.json`, `e01-early-evidence-index.json`, `sql-historical-evidence-index.json`. Исходные evidence не перезаписываются. Raw QA traces/session data не предназначены для публикации.

## E01: установленные факты и пределы

Историческое загрязнённое дерево `.local/build-env/web/.next/standalone` сохранено. Два пустых пути `.next 2` и `node_modules 2` существуют до11А. Оба inode имеют birthtime1793201227, более ранний, чем последний build; поэтому birthtime/mtime не являются доказанным временем нового создания. Original proof SHA256 `13b9728f814667403270b3a07e2b2e73b980895a627237dc3d2f2fa15fdbe6b4`. Пути не удаляются, proof не переиздаётся.

В production.sh стандартная упаковка создаёт `.next/standalone/.next`, копирует static/public. artifacts.cjs включает runtime paths/directories в digest и отвергает оба точных E01 имени. Browser wrapper проверяет artifact и served marker до тестов. В проверенных scripts нет операции создания suffixed имён. Проверен и установленный Next16.4.0 `dist/build/utils.js:copyTracedFiles`: удаление outputPath перед свежим build, mkdir/copyFile/symlink по NFT destination; операции добавления суффикса не обнаружено. По62 NFT manifests в original/Desktop/cache, E01 destinations0 (`next-traces.json`). Внутренний copy semaphore10 не доказывает гонку; активные builds разных workspace не разделяли writable output. Исторический filesystem writer этим не установлен. Исторические read handles Virtualization не доказывают writer; ни Docker, ни Finder/FileProvider, ни VS Code не объявляются причиной по одним именам или handles.

Созданы два новых эксклюзивных workspace: Desktop `.local/hardening11a/web-desktop` и cache `~/Library/Caches/AI-Business-Assistant/stage11a-web-independent`. В них клонированы только уже установленные Linux node_modules, без установки/скачивания. Каждый получает source через неизменённый copy/build/record/verify runner. Действующий operator artifact не пересобирается. Локальные overlays меняют только workspace mount, не source/config безопасности и не CI.

Observer с UTC/monotonic timestamp и lstat каждые100мс следит за двумя именами в обоих новых workspace и оригинале. Это точность наблюдения наличия, **не syscall audit/PID attribution**; короткое создание/удаление между опросами может быть пропущено. Process snapshot сохраняет comm/PID/PPID без argv/env. Для доказательства host writer при новом событии необходим отдельно согласованный аудит файловых операций с правами администратора; он не запускался.

Две fresh build PASS: Desktop BUILD_ID `2jfN4cb9UpznOPwdX0vdw`, cache BUILD_ID `G0zyRt7spFEEa5mE-LL4k`; source SHA256 в обеих `8785bb4abeef20f5e0f022e2dcdb9d0c52526516d8c6d771d264cd3081b0b727`. Различные build IDs нормальны; побайтовая reproducibility разных Next builds не заявляется. Cache proof SHA256 `4471eaa7d2ead42af7e9a4fb065e9302ee596ff78bcd94917905665bb5eefc77` сохранён до E2E. Исходный polluted artifact в отдельном RO mount дал ожидаемый FAIL42, reason `standalone changed: E01 directory present`; каталоги/proof не изменены. Это подтверждение refusal, не PASS исходного артефакта.

Результат расследования: **причина E01 не доказана, OPEN**. Повторные успешные сборки и изоляция не считаются причинным исправлением. Guard, baseline и исключения не ослаблены.

## SQL: эксперимент и результаты

Исторический10F фиксирует statement timeouts в накопленной QA-БД (сообщённые12184 variants/476 imports), API503 и запросы ControlPage с `WITH r AS`. Полный исходный PostgreSQL log/lock snapshot непосредственно в момент тех timeout не сохранён; API runtime log сам по себе не содержит полного SQL/плана. Следовательно ретроспективно установить единственную причину нельзя.

Сохранённый volume `aba-ux10f-browser_audit_smoke_data` смонтирован **только RO** для копирования в новый `aba-stage11a-sql-clone`. Запущен отдельный PostgreSQL project `aba-stage11a-sql`, internal network, без host ports. Source volumes и operator DB не изменяются. На момент копии12187 variants,476 receipts; отличие от прежнего12184 отражает более позднее состояние сохранённого тома. Создание probe sessions и временные seed/изменения статистики выполняются только на клоне внутри rollback transactions; session hashes не печатаются.

Использованы точные aggregate/page SQL из control.go под `SET LOCAL ROLE aba_app`, с действующей synthetic session/tenant и FORCE RLS. Statement timeout **2s**, как runtime default, без увеличения. EXPLAIN ANALYZE/BUFFERS JSON, prepared repeats и force_generic_plan сохранены локально. Изменение plan_cache_mode касается только диагностической транзакции, не приложения.

| Эксперимент до изменений runtime | Execution time, мс | Результат |
|---|---:|---|
| Aggregate,8 последовательных чтений |62.169–113.625|0 timeout |
| Page,8 последовательных чтений |52.428–111.861|0 timeout |
| Aggregate force_generic_plan,8 чтений |109.926–234.423|0 timeout |
| Отсутствие pg_statistic для3 catalog tables, rollback-only clone |87.404|0 timeout; модель отсутствующей статистики, не точное воспроизведение исторического stale plan |
| Новый bulk6001 в малом tenant до auto-analyze, rollback-only clone |122.263–185.643|0 timeout |
|1/2/4/8/16 параллельных SQL-сессий,8 чтений на сессию |max127.234/134.326/148.295/387.874/1027.095|0 timeout при2s |

Конкурентный SQL probe — не HTTP SLA;16 сессий превышают default API pool5, запросы alternating aggregate/page. План обычного aggregate:6008 фактических строк, nested loops, indexed products/variants/inventory; inventory index6008 lookups. Planning repeat4.5–10.5мс. Метаданные показывают auto-analyze variants/inventory/imports около12:28:57 UTC после исторических timeout около12:28:49–51. Это зацепка, **не доказательство причины**: отсутствие статистики и новый bulk сами по себе timeout не воспроизвели. Снимок idle clone не обнаружил незахваченных locks; отсутствие исторических блокировок этим не доказано.

Эти первые отрицательные пробы охватывали aggregate/page, **не весь ControlPage**. Новый полный regression затем воспроизвёл24 statement timeout:13 в итоговом SELECT metadata и11 при ожидании tenant advisory lock10130. `integration-final` FAIL сохранён. Monitor зафиксировал21 blocked sample с wait_type=Lock/wait=advisory и blocking PID. Это воспроизведение на свежей isolated БД, без параллельного build gate и без повышения SQL deadline.

### Доказанный bottleneck и минимальное исправление

Итоговый metadata SELECT содержал `(SELECT max(manual_updated_at) FROM platform.inventory)` без явного tenant predicate. RLS корректно фильтровал строки, но planner оценивал scan всей накопленной inventory с RLS subplans в656717.95 cost units, выше порогов дорогого JIT. В подзапросе1935.828мс execution, **1926.602мс JIT** (141 functions; optimization989.014, emission794.304). Весь metadata SELECT отменялся по неизменённому2s deadline даже при одиночной диагностике. Tenant read ControlPage требует owner и использует существующий write advisory lock; медленный запрос удерживал его и вызывал последующие lock timeouts. Blocking evidence и PG error statements сохранены.

Причинный A/B на том же fixture/aba_app/RLS: **только session-local `jit=off`** даёт9.134мс для того же inventory MAX. Это диагностика стоимости JIT; **в приложении/БД JIT не отключается**. После добавления `WHERE organization_id=$3` используется тот же tenant, который уже проверен в begin и RLS; cost328168.92, inventory query170.865мс, JIT161.464мс без дорогой optimization/inlining; весь metadata SELECT357.264мс. В этой первой пробе таймауты, FORCE RLS, grants и advisory locks неизменны. Это устраняет зависимость scan от всех tenants и подтверждённый переход через дорогой JIT threshold в данном сценарии. Никаких RLS bypass или удаления чужих данных.

Tenant predicate отдельно оказался недостаточным под concurrent one-second gate: `integration-scoped` FAIL,8 timeout (2 metadata,6 advisory waits), сохранён полностью. Поэтому он не объявляется окончательным исправлением.

Вторая причинная проба: partial ordered index `(organization_id, manual_updated_at DESC) WHERE manual_updated_at IS NOT NULL` на том же failed fixture, внутри rollback-only diagnostic transaction. Полный metadata SELECT **75.601мс**, cost1110.96, **JIT отсутствует**. Inventory index scan возвращает одну строку за0.252мс. Это подтверждает необходимость индекса сверх одного predicate.

Окончательное изменение: новая миграция00016 создаёт этот индекс; service_metadata/readiness16, empty down/up loop обновлён. Наследуется populated rollback refusal, старые миграции не изменяются. Index build только на isolated QA; на рабочей/demo БД миграция не запускалась. Будущее применение потребует обычного согласованного migration rollout и окна для index creation, без обхода мигратора.

После одного индекса полный concurrent suite ещё FAIL:4 advisory-lock timeout и1 timeout в отрицательной RLS UPDATE inventory. Это не зачтено как PASS. Metadata timeout в этом прогоне отсутствует;121-page load уже улучшился до p50218.236/p95339.456/max474.141мс против394.225/565.874/847.122мс в предыдущем independent unmodified run. Это разные isolated runs на том же хосте, не строгий production benchmark.

Дополнительно подтверждены и исправлены:

- Owner ControlPage использовал эксклюзивный writer advisory lock и сериализовал чтения. `beginOwnerRead` сохраняет **те же owner/channel checks**, но берёт shared transaction lock с тем же ключом10130. Остальные writers продолжают брать exclusive lock, несовместимый с каждым reader; writer fencing не убирается. Новый integration case удерживает relation lock, наблюдает два реальных owner-read transaction одновременно, проверяет, что Category writer ждёт advisory lock, а после освобождения оба чтения и запись успешно завершаются. Это проверка поведения сервиса, не только наличия SQL строки.
- Inventory UPDATE для outsider повторял owner/staff permission functions для тысяч строк. Original и даже диагностический jit=off не уложились в2s: этот путь **не сводится к JIT**. EXPLAIN показал per-row `has_membership OR (is_test_org AND can_operate_orders)`. Equivalent InitPlan для `tenant_update` и `staff_stock` дал349.810мс при том же unchanged data/aba_app; affected rows0. Миграция16 сохраняет row tenant equality и исходные owner/staff predicates в обоих USING/WITH CHECK; UUID берётся только из валидного current tenant, как в15-й SELECT оптимизации. Session/membership/permission вычисляются заново каждый statement, не кешируются между statements. Нет изменения FORCE RLS, ролей, grants, session lifetime или разрешений. Down возвращает прежние два predicate, сохраняя populated rollback refusal. Старые миграции не редактируются.

**Точная первопричина воспроизведённого пути установлена: несуженный tenant inventory MAX → дорогой JIT RLS-плана → длительный owner read lock → каскад timeout.** Это не доказывает, что каждый исторический10F timeout имел только эту причину: incident-time полных планов/ожиданий не сохранилось, исторически сообщались также aggregate statements. Большие tenant каталоги сверх проверенного диапазона/другая конкуренция могут иметь другие планы. Финальная regression/measurement после изменения фиксируется ниже.

## Изменения

- `apps/api/internal/catalog/control.go`: явный tenant predicate для last_stock_check и owner read transaction.
- `apps/api/internal/catalog/service.go`: shared owner-read lock при неизменной авторизации; остальные begin/writer locks сохранены.
- `apps/api/tests/integration/catalog_scale_test.go`: новый11А scenario — второй независимый synthetic tenant6001, дополнительно к прежнему6001; малый tenant;24 чтения/4 workers через настоящий catalog.Service и aba_app; immediate bulk без ожидания auto-analyze; one-second statement deadline;237 receipts/3pages без потерь/дубликатов; foreign staff denial и отсутствие чужих rows. DSN берётся из существующего isolated pool в памяти, без вывода.
- `docs/STAGE_10E_READINESS.md`: USER-REPORTED PASS и пределы10E.
- Этот отчёт и `docs/STAGE_11B_PILOT_READINESS.md`.
- `.local/hardening11a/`: локальные runners/overlays, исходные планы, полные gate logs/commands/exit, source baseline, observer и evidence индексы. Новых зависимостей нет. Новая миграция: `apps/api/migrations/00016_inventory_last_stock_check.sql`; readiness ожидает16 (`internal/platform/db/db.go`), empty rollback loop (`tests/integration/database_test.go`) включает16. FORCE RLS/grants/роли не меняются; два inventory UPDATE predicate эквивалентно вычисляют permission один раз на statement, как описано выше.

## Текущие проверки

Все окончательные gates после последнего source изменения выполняются последовательно; полные команды/logs/exit в `.local/hardening11a`. Ранний diagnostic integration-before частично пересёкся с первой Desktop build и не принят как финальный gate/benchmark. NOT RUN не считается PASS.

| Gate | Текущий фактический результат |
|---|---|
|Go tests/vet/format/build|PASS — go-verified, финальный source |
|Go race|PASS — race-verified, финальный source |
|PostgreSQL integration/race/migrations/RLS|**PASS93.100s**, integration-verified; default acceptance без tracer, schema16,45 FORCE RLS tables;0 statement timeout |
|Telegram/WhatsApp security/financial/consent/freshness/retention/idempotency|PASS в полном PG suite и Browser E2E; только mock |
|Frontend lint/TypeScript/OpenAPI/native/artifact unit|PASS — web-check-final;6 Node tests |
|Fresh production build|PASS — build-desktop и build-cache, независимые workspace |
|Browser current production|PASS9/9,0 skipped/flaky/unexpected;911.754s — browser-final |
|Browser accumulated repeat|**FAIL**,1 unexpected/0 expected,83.158s;2 PostgreSQL page timeout. Это блокирует reliability readiness |
|Final artifact/served integrity|PASS — integrity-final/served-final после accumulated FAIL и network-none RO verify после cleanup; polluted artifact по-прежнему FAIL42 |
|Formal10E rehearsal|Не подтверждён; USER-REPORTED PASS отдельно, без автоматического закрытия10E |
|Host syscall audit/PID E01|NOT RUN; нужен отдельно согласованный privileged scope |

### Последовательность SQL evidence

| Прогон | Изменение | Statement timeout | Статус |
|---|---|---:|---|
|integration-final|новый accumulated regression, original runtime|24 (13 metadata/11 advisory)|FAIL |
|integration-scoped|только tenant predicate|8 (2 metadata/6 advisory)|FAIL |
|integration-indexed|predicate + ordered index|5 (4 advisory/1 foreign UPDATE)|FAIL |
|integration-concurrent|index/predicate + shared owner reads + equivalent inventory UPDATE InitPlans|0|PASS107.766s |

Финальный121-page6001/236 sweep: elapsed41.536s, p50305.853мс, p95451.242мс, max506.202мс, max response73366bytes. Accumulated24 reads/4workers +237 receipts/3pages + fencing: elapsed3.154s; p50354.050мс, p95490.254мс, max553.228мс для24 read calls. Statement deadline1s, runtime default2s неизменны. RLS_inventory PASS0.15s;45 FORCE RLS tables; parent crash/recovery/finance/TG/WA suites PASS. Три subprocess-only helper SKIP по дизайну не подменяют реальные parent crash tests.

## Безопасность и оставшиеся риски

FORCE RLS/tenant isolation, роли, consent/staff approval, freshness, capability rotation, audit/receipts, idempotency, оба канала и financial gates не ослаблялись. Сроки audit/receipts не устанавливались. Изоляция/права runtime/сети/budgets/live flags не ослаблены. Нет внешних API, установок, commit/push/deploy, CI/AGENTS/PATH/global изменений или удаления рабочих данных. Operator demo и relay не останавливаются.

Открыты: причинное установление E01; исторический SQL-timeout без incident-time plan/wait evidence; ограниченный диапазон synthetic нагрузок; неполный evidence10E; отдельные approvals для retention/live/уязвимостей с внешними advisory DB. Проект автоматически не готов к live-пилоту.

## Дополнительное накопительное Browser воспроизведение (расследование продолжается)

После9/9 повтор того же Catalog Control Center на той же QA-БД **FAIL**,0 expected/1 unexpected,83.158s. PostgreSQL сохранил2 timeout11:25:54.125/11:25:56.780 UTC, конкретный statement — **page/matched**, не aggregate: `WITH r AS (...), matched AS (...) SELECT jsonb_build_object(...)`. Полный9/9 недостаточен для закрытия SQL риска. Сохранены `browser-accumulated-repeat.*`, JSON `stage11a-accumulated.json`,0600 `postgres-browser-repeat-fail.log` и monitor. Это новый подтверждённый remaining path; финальный статус не переименовывается в PASS по предыдущему чистому прогону.

Дополнительная локализация page path: после auto-analyze original query уже не воспроизводит2s; custom-plan repeats248–333мс переходят к generic44–58мс. Поэтому incident-time план и единственная причина **именно двух page timeout** не доказаны. Сохранены original/custom/generic/JIT-off EXPLAIN и bulk-shaped probes, включая неудачные диагностические seed attempts без изменения данных (rollback). Не приписываем им causal proof, которого нет.

Целевая OLTP оптимизация: `SET LOCAL jit=off` внутри beginOwnerRead, **после прежней owner/channel authorization**; применяется только к ControlPage до commit/rollback. Scope не распространяется на pool, других writers/финансовые запросы или сервер. Причина — измеренная высокая стоимость JIT для этих RLS expression trees и непостоянство cost-плана при bulk/stats; никакие timeout/limit/guard не меняются. Это не считается самостоятельным доказательством исторической причины. Этот эксперимент затем отменён после normal FAIL; окончательный production source возвращён к byte-identical backend9/9 и accumulated FAIL, что подтверждено hash comparison ниже.

Incident monitor page timeout: PID645/130, fingerprint0616c67174606b2ca00a903e3dedf644; active до1575/1720мс, wait_event_type=NULL и blocked_by=[] во всех7 наблюдаемых samples. Это локализует отменённый page path к CPU/execution/planning, не к наблюдаемой lock queue или I/O wait. Sampling не доказывает отсутствие кратких ожиданий между polls и не заменяет compiler trace. Evidence: page-timeout-incident-samples.json.

## Финальное решение по экспериментам и пределам

Экспериментальный transaction-local JIT-off **удалён из production source**: normal integration-oltp FAIL70.566s на первой page6001 даже с JIT-off (новый accumulated test при этом PASS, p95660мс). Нельзя выдавать это за причинное исправление page-path. Оставлены только подтверждённые index/tenant MAX, shared-owner read fencing и эквивалентная inventory UPDATE permission оптимизация. JIT/plan_cache_mode/timeouts приложения и сервера не меняются.

Диагностические варианты не являются quality acceptance: autovacuum-off isolated integration-freeze-diagnostic PASS96.854s; восемь rollback-only комбинаций частичного ANALYZE не воспроизвели page-timeout; tracer-before EXPLAIN PASS86.116s, но меняет prepared counters; error-only tracer PASS85.237s, ошибки не случилось, полноценный incident-time ANALYZE plan не получен. Происхождение page-timeout по-прежнему не доказано до единственной операции; CPU/query-path локализация и оставшийся FAIL сохранены. Не переносить эти diagnostic PASS в итоговые gates.

Добавлен только test-tag helper `apps/api/tests/integration/catalog_plan_trace_test.go`, включаемый явным ABA_CAPTURE_CATALOG_PLAN_DIR внутри isolated /artifacts. Аргументы, session hash/credentials не пишутся; передаётся тот же AfterConnect guard из db.Open. Default acceptance не включает tracer. После исходного57014 optional helper может выполнить отдельный EXPLAIN ANALYZE с diagnostic deadline10s, **возвращая первоначальный FAIL без изменения acceptance1s**. Этот error-capture branch не был достигнут в проверенном прогоне: NOT RUN; не является доказательством полного slow plan. В production/API нет диагностического endpoint или этого env-пути.

Три известных SQL механизма доказаны и исправлены, но **SQL reliability не закрыта** из-за подтверждённого второго accumulated Browser FAIL. Привилегированный host audit E01 не запускался; отдельно подготовлен `.local/hardening11a/E01_PRIVILEGED_AUDIT_PROPOSAL.md` и whitelist-filter. Global AGENTS.md: “Request approval for destructive, privileged, costly, or broad actions outside the authorized scope.” Host audit требует administrator прав, поэтому нужен отдельный owner approval. Это ограничение для E01, не доказательство причины SQL.11Б остаётся только предложением; решение о live readiness не принято.

## Финальное состояние, provenance и cleanup

Финальные API/worker/migrate/healthcheck/browserfixture binaries **побайтово совпали** с записью перед Browser9/9; /proc/1/exe API/worker QA также совпали. Browser9/9 и accumulated FAIL относятся к **окончательному production коду** после удаления JIT-off, а не к другому backend. Frontend source не менялся после двух fresh builds; повторять build ради изменённого test-tag helper не требовалось. Original и cache proof byte-identical после runtime/cleanup; guards source неизменен. Evidence: binary-comparison-final.json, running-binaries.json, proof-final.json.

Последний обычный PG gate:6001/236 receipts/121pages elapsed30.724s, p50217.916мс, p95341.953мс, max757.878мс, max response73316bytes. Accumulated24 reads/4workers: p50328.865мс, p951059.859мс, max1366.560мс; полный elapsed4.245s включает проверки237 receipts/3pages и fencing. **Service-call latency включает несколько SQL statements и lock wait;1s — лимит каждого statement, не HTTP SLA.** RLS_inventory PASS0.05s; полные45 tables, owner/staff/outsider, оба канала и financial gates PASS. При этом accumulated Browser FAIL остаётся отдельным blocker, не перекрывается этим PASS.

Observer10:34:32.257–2026-10-31T11:59:59.307117+00:00 (UTC, filesystem-events.jsonl) не зарегистрировал новых E01 имён в двух fresh workspace; исходные два каталога сохранились. Polling не является PID audit. Operator demo/relay и посторонние проекты не остановлены; рабочая/demo БД не мигрировалась. На согласованную будущую миграцию16 потребуется отдельный rollout; текущий работающий operator API не перезапускался.

13 собственных QA projects остановлены graceful `docker stop -t -1` (без SIGKILL escalation); containers/networks удалены. Все QA volumes, исходный failed10F volume, клонированная БД, оба new build workspace и full evidence сохранены; volume deletion0. Нет процессов, принудительно завершённых этим этапом; Audio не обслуживался. Cleanup-final/stopped-states сохраняют точные project names/exit states. Проверенные QA networks internal, host publications0; live/kill/budget flags сохранены (runtime-security.json).

Финальный source manifest:5 modified production/test files +2 added (миграция16 и test-only plan tracer), deleted0; apps/web/scripts/infrastructure/packages и прежние15 migrations совпали с исходным source baseline. Отдельно3 docs:10E readiness append,11А report и11Б readiness. Полный список: source-changes.json; raw trace/plan/runtime logs не публиковать.

**Результат:11А не закрыт.** Доказанные SQL узкие места исправлены и их регрессии прошли; intermittent accumulated page-timeout и E01 остаются OPEN.11Б/live NOT READY, Security CONDITIONAL. Для следующего E01 шага нужно отдельно разрешить600-second privileged fs_usage с whitelist-filter (proposal в.local/hardening11a), без установки/сети/изменения системных настроек. Причинное SQL расследование требуется продолжить отдельно от host writer attribution; одного разрешения fs_usage для SQL closure недостаточно.


## Продолжение 1 ноября — непривилегированная диагностика

Все новые evidence находятся в `.local/hardening11a-continued-20261101/` (каталог0700). Пароли, session hashes, cookies, OAuth данные и SQL bind arguments в новые timing/trace сводки не экспортируются. План API сохраняет только разрешённые структурные поля, стоимости, строки, имена индексов и времена; Filters/Index Cond/Output с возможными значениями исключены. SQL experiments содержат только synthetic fixtures в отдельной QA-БД. Raw Browser traces остаются приватными и не предназначены для публикации.

### E01: ROOT CAUSE NOT PROVEN; OPEN

Созданы два новых workspace из существующего cached Linux node_modules, без установки зависимостей: `.local/hardening11a-continued-20261101/web-desktop` и `~/Library/Caches/AI-Business-Assistant/stage11a-continued-control`. Сборки последовательные; writable outputs не разделяются. Вторая сборка — контроль без Node preload. Проверены действующие production/copy/package scripts, установленный Next16.4.0, NFT paths, whitelist build environment и конкурирующие процессы. Доказательства операции, добавляющей суффикс ` 2`, не получены.

Наблюдение 13:49:55–13:53:37 UTC: host lstat каждые20мс по двум точным именам в новых workspace и original; в первой сборке дополнительный Node preload наблюдает fs mkdir/cp/rename/symlink/removal, включая promises и sync API. Пишутся только фиксированные target IDs, operation/errno, Node PID/PPID и фазы. Это **не macOS syscall audit**: native/external host writer может не попасть в Node wrapper, а краткое событие между polls может быть пропущено. Нельзя установить writer или стадию исторического загрязнения по отсутствию нового события.

Обе сборки exit0: COPY → PREFLIGHT → NEXT_BUILD → PACKAGE → RECORD → VERIFY. E01 operations0; новых E01 paths0. Build IDs: Desktop `as_Uz0OKV2fDnoMzLUo97`, control `EX1lI4kamR8pRNvszfBli`; source SHA256 обеих `8785bb4abeef20f5e0f022e2dcdb9d0c52526516d8c6d771d264cd3081b0b727`. Original `.next 2`/`node_modules 2` сохранены пустыми с теми же inode18875546/18875558, birthtime/mtime; original proof по-прежнему SHA256 `13b9728f814667403270b3a07e2b2e73b980895a627237dc3d2f2fa15fdbe6b4`. Baseline/guard/exclusions не менялись. Evidence: `e01-before.json`, `e01-after-builds.json`, `e01-timeline.jsonl`, `node-fs-events.jsonl`, `build-instrumented.*`, `build-control.*`. sudo/fs_usage и попытки обхода macOS ограничений в этом продолжении NOT RUN по прямому запрету владельца.

### SQL page-path: ROOT CAUSE PROVEN для воспроизведённого механизма

Точный route: `GET /api/v1/organizations/{id}/catalog-control?page=…`; UI `/dashboard/catalog`, переход «Далее» после bulk6001. Отменяется page statement `WITH r AS (...), matched AS (...) SELECT jsonb_build_object(...)`, не aggregate, не last_stock MAX, не advisory-lock acquisition.

Сравнение сохранённых двух PostgreSQL statements из накопительного FAIL31 октября: оба после нормализации whitespace **точно совпадают** с rollback reproducer; normalized SHA256 `14dc5aab326dea360f3f24d830ca205af889e3d561b4b5c463cf98b7d2c6f371`. Evidence: `historical-page-template-comparison.json`. Исторические incident-time планы этих двух backend не сохранились, поэтому не утверждается ретроспективный захват каждого их плана.

1 ноября новый original Catalog Browser E2E вновь FAIL на page2. Private diagnostic API зафиксировал request27, backend PID24815, generic plan cost163.98, SQLSTATE57014 после2001.676мс, HTTP503 за2460.807мс. План выбран до исполнения именно существующего pgx prepared statement; observer фиксирует выбор EXPLAIN для следующего actual call, чтобы лишний prepared counter не заменил плохой план хорошим. Это диагностическое вмешательство, **не acceptance gate**. Поля request IDs/route template не содержат tenant IDs или аргументов. CPU delta216 ticks при CLK_TCK100 за2.588s подтверждает существенную CPU работу; два active samples на том же PID/интервале не показывают wait_event/blockers. Prefix-class monitor неоднозначен из-за усечения pg_stat_activity.query, поэтому SQL stage определяется API trace + PID/time, а не ошибочным ярлыком AGGREGATE в старом sampler. Evidence: `failed-page-plan.json`, `failed-page-cpu-summary.json`, `api-trace/events-before-pointlookup.jsonl`, `browser-diag-3.*`.

Детерминированное воспроизведение: приложение прогревает generic plan на малом каталоге **до** вставки6001 строк. Статистика удерживается от auto-analyze отдельным QA lock SHARE UPDATE EXCLUSIVE; обычные read/write разрешены. После bulk сохранённый estimate1 row приводит к nested loop: inventory PK используется только по organization_id, variant_id проверяется join filter. Для каждого варианта заново перебираются остатки магазина — квадратичный объём работы. Это не отсутствие индекса: PK `(organization_id,variant_id)` уже есть. Cost163.98 ниже JIT threshold; выключение JIT не является исправлением этой причины.

| Точная rollback probe, deadline2s | До: обычный inventory JOIN | После: параметризованный lookup |
|---|---:|---:|
|План прогрет до bulk;6002 variants|57014, отмена2s|64.562мс|
|Тот же cached plan;101 variants|69.399мс; inventory среднее51 row ×101 loops|5.569мс;1 row ×101 loops|
|Inventory key при6002|только organization_id|organization_id + variant_id;1 row ×6002 loops|

`controlRows` использует JOIN LATERAL с точным lookup по двум PK полям и LIMIT1. Ограничение1 следует из существующего PK, **не ограничивает число товаров**; отсутствие inventory по-прежнему исключает вариант как inner JOIN. Сохраняются все колонки, агрегаты, freshness, сортировка, пагинация и legacy отказ при>5000. FORCE RLS и прежняя авторизация исполняются. Не меняются runtime JIT, plan_cache_mode, SQL deadlines, pool limits или server settings. Evidence: `cached-before-bulk.sql/.log`, `cached-before-bulk-lateral.sql/.log`, `cached-plan-before-after-summary.json`. Все изменения fixture/statistics в этих пробах откатываются; административная подготовка fixture не является измеренным app SQL.

### Дополнительные доказанные blockers исходного Catalog Browser сценария

До достижения bulk первоначальный сценарий обнаружил ещё два реальных timeout2s на накопленной QA-БД: source preview вызывает unscoped inventory SELECT FOR UPDATE в lockCatalog; Excel mapping вызывает unscoped catalog fingerprint. RLS фильтрует правильный tenant, но отсутствие явного UUID tenant predicate завышает estimated cost и включает дорогую JIT компиляцию. Проба исходного inventory lock с диагностическим JIT-off —7.721мс; с исходными настройками — timeout2s. Production JIT не выключается.

Минимальное исправление: явный `organization_id=current_setting('app.organization_id')::uuid` в двух lockCatalog SELECT и четырёх fingerprint aggregates. Порядок/набор tenant locks, fingerprint ordering/inputs, idempotency и preview validation сохраняются. Проверка эквивалентности старого и scoped fingerprint дала boolean true без вывода digest. Inventory lock после —7.644мс; fingerprint после —3.100мс. Evidence: `lock-original.*`, `lock-original-nojit.*`, `lock-scoped.*`, `fingerprint-original.*`, `fingerprint-scoped.*`, `fingerprint-equivalence.*`; Browser FAIL `browser-diag-1/2.*` сохранены.

### Подтверждённый накопительный timeout списка товаров

Первый **обычный** full Browser gate на клоне с24115 variants/951 receipts/32 tenant catalogs — **FAIL0/9,219.587s**. Это полноценный сохранённый FAIL, а не diagnostic PASS. В его точном временном окне PostgreSQL зафиксировал11 отмен, все — `Products` (`WITH matched AS`), page/matched отмен0. HTTP trace подтверждает `GET /api/v1/organizations/{id}/products` →503; в mock walkthrough также затронут customer Products через существующие инструменты обоих каналов. До конца этого gate дополнительные SQL probes не выполнялись. Evidence: `browser-normal-full.*`, `normal-browser-full-timeouts.json`, `normal-browser-full-http-errors.json`, Browser JSON `stage11a-cont-normal-full.json`.

Первопричина этого blocker **PROVEN**: RLS обеспечивает доступ к своему tenant, но список products и три коррелированные выборки variants не задавали явные UUID tenant-ключи. Estimated cost owner711051.59/customer711340.76 включает дорогую JIT оптимизацию, хотя реальный ответ малого каталога мал. Под тем же2s deadline owner original и customer generic original с реальным status=active отменяются. Контроль только JIT-off даёт6.431/6.547мс; это диагностическое доказательство, не production флаг. Customer custom active выполняется228.193мс при cost461077.65: поэтому важен также generic вариант, а не только один custom EXPLAIN.

Добавление только outer tenant predicate снижает cost до13175–13181; дополнительно составные `(organization_id,product_id)` условия в коррелированных variants используют уже существующий `variants_product` и снижают cost до457. Owner scoped5.309мс, customer scoped active/generic6.861мс. Две boolean проверки полного JSON old/new далиtrue, включая active-only и freshness/null семантику customer. Исправление ограничено SQL в Products, фильтры/сортировка/суммы/цены/availability/авторизация и RLS остаются прежними. Новых индексов, миграций или глобальных planner flags нет. Evidence: `products-*.sql/.log`, `products-plan-summary.json`; специальные active/generic и equivalence probes сохранены отдельно.

Регрессия добавляет200 synthetic foreign products к двум6001-variant foreign catalogs и вызывает настоящие owner/customer Products малого tenant под test-only generic plan, deadline1s. Проверяются точные total/IDs собственных products, отсутствие foreign IDs и полнота результата. Подготовка выполняется после немедленных concurrent reads, сохраняя прежнюю cold-bulk нагрузку. После этого узкого исправления требуется новая normal acceptance, прежние gates не переносятся на новый backend.

### Изменения и новые регрессии продолжения

Изменены только production `apps/api/internal/catalog/control.go`, `apps/api/internal/catalog/import.go`, `apps/api/internal/catalog/service.go`, test `apps/api/tests/integration/catalog_scale_test.go` и этот отчёт. Новых migrations, production diagnostic endpoints/env flags, frontend/CI/global configuration изменений нет. Private Compose overlays/trace copies/logs находятся в отдельном `.local` evidence, не являются runtime поставкой.

Новые integration regressions: source preview малого tenant рядом с двумя большими foreign catalogs; XLSX mapping/fingerprint в той же ситуации; generic plan на одном соединении, прогретый на1 варианте до вставки6001, затем total6002/first page50/last page2/search1 и legacy refusal. Test-only plan_cache_mode и stats-freeze lock не переносятся в runtime; original AfterConnect role/security guard и deadline1s сохраняются.

Первый обычный integration gate **FAIL99.659s**: 121-page sweep PASS; source preview31.164мс PASS; import preview92.007мс PASS; новая cached-before-bulk регрессия PASS, first page211.900мс. Однако один concurrent-read отменён в **AGGREGATE** statement при1s. Это отдельный незакрытый риск, не возврат доказанного page-plan и не основание поднять лимит. `integration-final-timeouts.json` сохраняет безопасный SQL template. Дополнительные диагностические повторы не заменяют этот FAIL: один inactive-trace run PASS; before-EXPLAIN run PASS83.355s с эффектом prepared counters; отдельный error-only observer сохраняет исходную ошибку и не повышает deadline. Причина единичного aggregate timeout пока NOT PROVEN.

### Итоговые offline gates продолжения

|Проверка1 ноября|Фактический результат|
|---|---|
|Go tests/vet/gofmt/build|PASS `go-products-final` после последнего production исправления; первый `go-final` не запустил тесты из-за duplicate key в private overlay, FAIL сохранён|
|Go race|PASS `race-products-final` после последнего production исправления|
|Первый ordinary PG/integration/RLS|FAIL99.659s, один AGGREGATE statement timeout1s; не скрыт повтором|
|Ordinary PG repeat|PASS80.849s `integration-normal-verify`,0 statement timeouts|
|Cold-bulk PG integration/race/RLS до Products fix|PASS74.405s `integration-cold-final`,0 statement timeouts; preview проверки перенесены после concurrency, чтобы чтения начинались непосредственно после bulk|
|Diagnostic error-only PG|PASS77.111s; error-capture branch NOT RUN, поскольку ошибки не возникли; не acceptance|
|Frontend lint/TypeScript/OpenAPI/native/artifact unit checks|PASS `web-check-final`,6 Node tests|
|Fresh production builds|PASS2/2; инструментированная Desktop и обычная cache, разные workspace/build IDs|
|Diagnostic original Catalog Browser|PASS1/1,64.297s; SQL57014=0, HTTP503=0, explicit_read_retry=false; не acceptance|
|Normal production Browser full before Products fix|FAIL0/9,219.587s;11 Products SQL timeouts,0 page timeouts; подтверждённый blocker исправлен; новая acceptance приведена ниже|
|Normal accumulated Catalog repeats|PASS2/2,45.838s и36.908s; explicit_read_retry=false/automatic_write_retry=false в обоих; финальный clone42271 variants/1665 receipts/70 catalogs/7 больших|
|Final PG integration/race/RLS после Products fix|PASS91.072s `integration-products-final`; полный набор включая новый owner/customer products regression;0 statement timeouts (`integration-products-timeout-count.json`)|
|Normal accumulated full после Products fix|FAIL8/9,845.592s;0 SQL timeouts; единственный FAIL — onboarding fixture collision (409, telegram_integrations_bot_id_key)|
|Accumulated final artifact/served integrity|PASS `integrity-accumulated`/`served-accumulated`; fresh final verify/served PASS также, см. ниже|

Cold-bulk load до Products fix:6001 variants/236 receipts/121 pages —23.515s, p50171.856мс, p95240.055мс, max358.855мс, max response73366bytes. Накопительные24 read calls/4workers плюс237 receipts/3pages/fencing —3.116s, p50214.493мс, p95935.770мс, max1.079s. Это service-call latencies с несколькими statements, а1s — deadline **каждого SQL statement**, не SLA HTTP. Новые source/import preview26.316/74.244мс; warmed generic first page6002 —121.424мс. FORCE RLS inventory и все45 tables, tenant/owner/staff/outsider, retention, consent, idempotency, Telegram/WhatsApp и financial crash/recovery suites PASS в полном final gate. Три subprocess-only helper SKIP по дизайну; parent crash scenarios реально выполнялись.

Первый AGGREGATE FAIL не удалось воспроизвести в последующих diagnostic и двух ordinary gates. JIT-eligible планы до163896 cost встречались в диагностике, но это не incident-time план первого FAIL и не доказательство его причины. Во время первого FAIL дополнительно работал Docker-exec CPU sampler; его overhead и конкуренция ресурсов могут влиять на1s результат, однако контрольного причинного доказательства нет. Для ordinary final gates sampler остановлен. Никаких production изменений под эту гипотезу нет; оставшийся риск явно сохраняется.


После последнего Products исправления: Go tests/vet/gofmt/build и race повторно PASS (`go-products-final`, `race-products-final`). Полный final PG gate включает6001/236/121 sweep25.801s (p50182.236мс, p95293.568мс, max395.176мс);24 concurrent reads p50249.869мс/p95971.261мс/max1.077s service time; owner/customer Products91.815/19.660мс; cached-before-bulk6002 first page151.453мс. Deadline1s остаётся на каждый statement, не на сумму всех statements сервиса. Финальные source/binary/proof hashes закреплены в `provenance-final.json`, `source-changes-final.json` и `normal-runtime-products-start.json`.


Накопительный full после Products fix:8 scenarios PASS, onboarding FAIL на подключении Telegram mock. HTTP409 `IDEMPOTENCY_CONFLICT`, PG constraint `telegram_integrations_bot_id_key`; тест повторяет фиксированный synthetic bot ID, уже присутствующий в клонированной QA-БД после предыдущего полного прогона. Это доказанный конфликт fixture, не SQL timeout и не повод ослаблять уникальность/владение. Полное окно845.592s:0 statement timeouts. Исходный FAIL сохраняется (`browser-products-full.*`, `browser-products-full-safe-summary.json`, `onboarding-conflict-code.json`); тесты и авторизация под него не переписываются. Полный normal acceptance выполняется отдельно на новом Browser volume, а Catalog repeats — на том же накопленном clone. Результаты этих следующих gates записываются только после завершения.


Два normal Catalog repeats на том же clone после full8/9 — PASS45.838s и36.908s. Каждый снова создаёт bulk6001, проверяет6008 rows/поиск/пагинацию,237 receipts/3pages, source/freshness/retention и mobile; оба cold_read attachments: explicit_read_retry=false, automatic_write_retry=false. Финальный clone42271 variants/1665 receipts/70 tenant catalogs,7 catalogs>5000;0 statement timeouts с запуска final production API. Artifact verify и served marker после повторов PASS. Evidence: `accumulated-browser-final-summary.json`, `accumulated-final-counts.json`, `browser-accumulated-1/2.*`, `integrity-accumulated.*`, `served-accumulated.*`.

Клон штатно остановлен без SIGKILL, volume/containers/failed evidence сохранены. Fresh full Browser acceptance использует отдельный новый volume `aba-stage11a-cont-browserfresh_audit_smoke_data`, обычные final API/worker binaries (SHA256 match в `fresh-browser-runtime-start.json`), тот же fresh control frontend artifact, internal network и0 published ports. Это исключает повторное использование фиксированного onboarding mock ID без изменения теста, данных исходного clone или защиты владения. Gate завершён **PASS9/9, 828.068s**,0 unexpected/0 flaky/0 skipped и0 SQL timeouts (`browser-fresh-full.*`, `fresh-browser-final-summary.json`). Cold-read retry=false, write retry=false. В том числе прошли onboarding и оба mock-канала; фиксированные fixture IDs не менялись.


### Окончательные результаты продолжения и оставшиеся риски

|Проверка final backend / frontend|Результат|
|---|---|
|Go tests/vet/gofmt/build; race|PASS `go-products-final`, `race-products-final`|
|PostgreSQL integration -race, migrations, FORCE RLS/tenant/security|PASS91.072s `integration-products-final`;45 FORCE RLS tables;0 SQL timeouts|
|Frontend lint/TypeScript/OpenAPI и6 native/artifact checks|PASS `web-check-final`; frontend source неизменен после двух fresh builds|
|Fresh Next production builds|PASS2/2; source/proof hashes не менялись после Browser|
|Fresh production Browser E2E|PASS9/9,828.068s; Telegram/WhatsApp, onboarding, desktop1440×1000/tablet768×1024/mobile390×844 и375×812;0 SQL timeouts,0 retries cold/write|
|Accumulated original Catalog E2E|PASS2/2,45.838s/36.908s; clone42271 variants/1665 receipts;0 SQL timeouts,0 cold-read/write retries|
|Artifact/served integrity after accumulated and fresh E2E|PASS `integrity-accumulated`, `served-accumulated`, `integrity-fresh-final`, `served-fresh-final`; independent Desktop verify PASS|
|Original tainted artifact|**FAIL42**, ожидаемый отказ guard; original directories/proof неизменны, не исправлены удалением или rebaseline|
|Privileged fs_usage/sudo|NOT RUN в этом продолжении по запрету владельца|

**E01: ROOT CAUSE NOT PROVEN, OPEN. SQL page-path: ROOT CAUSE PROVEN, исправление подтверждено для воспроизведённого cached-plan механизма.** Отдельно доказаны и исправлены source-lock/fingerprint/Products cost/JIT blockers. Incident-time планы каждого из двух исторических page failures не были сохранены; точное совпадение обоих SQL templates и причинный rollback reproducer не выдаются за ретроспективный syscall/plan capture.

**AGGREGATE1s: ROOT CAUSE NOT PROVEN.** Первый ordinary PG FAIL99.659s сохранён. Более поздние diagnostic и ordinary runs, два cold-bulk full PG gates, свежий full Browser и accumulated repeats не повторили его. Считаем риск незакрытым, не отключаем JIT и не повышаем таймауты. Полный accumulated Browser также сохраняет FAIL8/9 из-за доказанного fixture bot-ID collision; fresh9/9 исправляет изоляцию acceptance, а не переименовывает этот FAIL. Ограничения нагрузки: один tenant проверен на6001/6002/6008 variants и236/237 receipts;42271/1665 — суммарно по QA-БД, не по одному магазину. Это не production SLA.

Security **CONDITIONAL**,11А **НЕ ЗАКРЫТ**, live-пилот **NOT READY**.10E остаётся USER-REPORTED PASS, формальная репетиция не закрыта автоматически. Сроки audit/receipts не устанавливались. FORCE RLS, авторизация/роли, consent/staff approval, freshness, retention capability, idempotency, финансовые gates и channel parity сохраняются. Production planner/cache/timeouts, CI, AGENTS.md, глобальные настройки и PATH не менялись; новых migrations/dependencies в продолжении нет. Три production файла, один integration test и этот отчёт — полный список изменений source/docs относительно начала1 ноября.

Новых внешних вызовов, downloads/установок, платных API, commit/push/deploy или live enablement не выполнялось. QA сети internal, опубликованных портов0; mock/live/budget flags сохранены (`runtime-security-final.json`). Исходные failed volumes/containers/evidence, все build workspaces и contaminated artifact сохранены. Cleanup только собственных QA: штатный `docker stop -t -1`, без SIGKILL escalation и удаления volumes. Работающий operator demo не перезапускался и не мигрировался; посторонние проекты/Audio процессы не изменялись. Итоги остановки и состояние operator containers: `cleanup-final.json`.


## Продолжение3 ноября — Final Root Cause Investigation

### Условия и неизменность исходных evidence

Прочитаны актуальные global/project AGENTS,11А/11Б, build-integrity history и сохранённые E01/SQL материалы. Evidence этого запуска: `.local/hardening11a-final-20261103/`, parent0700. Исходный polluted standalone и proof, старые failed QA volumes и логи не удаляются и не перезаписываются. Состояние начала: `e01-before.json`; source baseline: `source-before.json`.

В начале Docker daemon не был запущен, socket отсутствовал. Запущен уже установленный Docker Desktop обычным непривилегированным `open -g -a Docker`, без установки/изменения настроек. Его существующие restart policies подняли ранее настроенные сервисы; они не использовались как QA, не останавливались и не мигрировались. Все дальнейшие DB/Browser процессы имеют отдельные QA project/volume/network names. Рабочие DSN и рабочие данные в экспериментах не используются.

### E01 — ROOT CAUSE NOT PROVEN; OPEN

Четыре независимых fresh builds, каждый с отдельным writable output, без installs/downloads и без удаления предыдущего дерева:

|Эксперимент|Storage/наблюдение|Результат / BUILD_ID|
|---|---|---|
|DESKTOP_A|Новый host-backed workspace, Node wrapper + libc observer|PASS `loAUThgz4XGOe0T_y4Hm8`|
|LINUX_A|Новый Docker-managed Linux volume, те же observers|PASS `zpIs8Wdf6md0XMUipxlUA`|
|DESKTOP_B|Отдельный host workspace, без preload|PASS `kJn2gw3z8aNMaiNjlNHEr`|
|LINUX_B|Второй новый Linux volume, без preload|PASS `myekM5A1-t0VYJcyYlOfS`|

Каждый прошёл COPY/PREFLIGHT/NEXT_BUILD/PACKAGE/RECORD/VERIFY. Linux dependencies клонированы из уже подготовленного cached node_modules, не устанавливались. Source SHA256 и отдельные proof hashes: `e01-build-proofs.json`; все4 source digests совпадают. Ни в одном build не зафиксировано наличие E01 paths или успешной native операции к ним.

Новый observer — приватная libc interposition library, собранная существующим cached GCC без новых runtime dependencies. Она передаёт операции настоящей libc, сохраняет errno и регистрирует только enum target/operation/process, PID/PPID и clock; не пишет пути, argv/env или содержимое файлов. Calibrated Node mkdir создал оба **намеренных** каталога в отдельном CALIBRATION root, зафиксирован PID1 соответствующего контейнера. Это не воспроизведение E01. В настоящих builds observer увидел13/18 обычных событий к стандартным standalone каталогам (Node и mkdir). Новых target1/2 events0; calibration targets11/12 учтены отдельно. Node callback/promise instrumentation и polling20мс дополняют native observation; неинструментированные B controls проверяют влияние preload.

Проверены build/package/copy scripts, Next standalone paths и проектные dev hooks: буквальные suffixed имена встречаются в guard/его тестах, не как рабочая команда создания. Project `.vscode` не найден. Phase snapshots содержат только host PID/PPID и фиксированные категории Docker/IDE/Node/Finder/FileProvider, без argv или env. Нет основания объявлять любой из этих процессов виновным по наличию в snapshot. Native PID относится к Linux container namespace, не к macOS PID.

Наблюдение не является полным syscall audit: interposition не видит прямые syscalls/native hidden calls и host writers; polling может пропустить краткое событие. Linux volumes исключают обычный прямой доступ host IDE/Finder к output, но отсутствие события в четырёх опытах не доказывает невозможность такого writer. Штатный unprivileged unified-log query по точным именам и historical ctime window остановлен SIGINT после превышения времени наблюдения; raw messages не сохранены, полнота окна НЕ подтверждена (`unified-log-query.json`, complete=false). Не считать его отрицательным causal proof.

Итог: нового E01 события нет, поэтому точное время создания исторических путей, host writer PID и механизм не установлены. Старые inode/birthtime/mtime/ctime и original proof сохраняются. Guard/baseline/exclusions не менялись. Привилегированная ветка остановлена; конкретный отдельно согласуемый план подготовлен ниже.

### Timeout агрегата1s — ROOT CAUSE NOT PROVEN

Точный statement — aggregate `WITH r AS (...), a AS (...) SELECT total,complete,priced,known,fresh,jsonb_build_object(...)`, SHA256 `a9ced8f813692041fd36cb3fd5bc63b47401551047b19c91c46f4fc34ec189d1`. Он byte-identical в текущем production и diagnostic source и совпадает с историческим SQL после нормализации log whitespace. Это не page/matched, не last_stock MAX и не advisory acquisition.

1. Обычный fresh PG integration/race/RLS **PASS74.572s**, SQL deadline1s, без tracer.6001 variants/236 receipts/121 pages:23.125s, p50168.541мс/p95229.754мс/max335.682мс.24 reads/4workers: p50203.030мс/p951.035s/max1.210s **service time**, не длительность одного SQL. Ни один timeout не повышался.
2. Preserved failed volume `aba-stage11a-cont-final_audit_smoke_data` был остановлен и скопирован с RO source в новый `aba-stage11a-final-aggregate-clone-20261103`. В нём после исторического сбоя уже был добавлен третий cached-plan fixture на6002 variants. A/B/A на одном clone со stats-freeze lock: auto и forced-generic,6 profiles ×75 настоящих ControlPage calls,4workers, deadline1s — все diagnostic runs без отмен. В B воспроизведён прежний Docker-exec sampler. Нет причинного доказательства, что именно sampler вызвал historical FAIL.
3. Отдельная logical incident model удаляет **только постинцидентный6002-variant synthetic fixture из новой копии**, не из source. Две исходные большие организации по6001 остаются;12027 variants суммарно. Это не точный physical snapshot момента historical failure: удаление создаёт dead tuples. После ANALYZE в этой модели диагностические A/B/A дали по50 AGGREGATE SQLSTATE57014 из75 calls; суммарно150 отмен. Лимит по trace —1s; фактическая отмена наблюдалась через~1.6–4.3s. Это факт поведения PostgreSQL/interrupt delivery, не увеличение statement_timeout. Все FAIL сохранены (`matrix-INCIDENT_AUTO.json`, `agg-INCIDENT_*`, `events-INCIDENT_*`).
4. Лёгкий persistent collector внутри **этого** PostgreSQL container читает только numeric /proc/cgroup и PID/state/wait/blockers из pg_stat_activity, не query text/arguments.50мс CPU samples и100мс activity отделены от старого Docker-exec sampling. У отмен доминировало guest backend CPU время: например2302мс wall, CPU bracket2290мс, scheduler-wait bracket28.8мс. Brackets включают соседние statements в пределах polling interval; host/VM scheduling полностью этим не исключено. Ресурсный sampler не является production acceptance.
5. Post-error pgx helper **не сохранил план150 отмен**: следующая обычная pgx команда очищала invalidated prepared statement; EXPLAIN получил26000. Не выдавать error_plan record без поля plan за сохранённый plan. Диагностический helper изменён на direct PgConn capture до pgx cleanup; новая error branch ещё не достигнута при последующих успешных runs. Это исправление наблюдателя, не приложения.
6. После autovacuum12:42:53 UTC в той же модели (видимость/оценки строк изменились) trace v1/v2 и control **без client tracer** перестали воспроизводить отмены. Повторный ANALYZE восстановил estimated12027 rows вместо приблизительных7967/8021, но failure не вернулся. Отдельная pre-vacuum copy с autovacuum=off **только для диагностического QA server** также не повторила fault. Следовательно корреляция с vacuum не является доказанным единственным механизмом.
7. Exact SQL probes при1s: на устойчивой модели cost80507; pre-vacuum cost121471, JIT basic228 functions, но single execution~294мс и без отмен; diagnostic JIT-off~53–77мс. Эти результаты не объясняют доказательно150 CPU-heavy cancels; выключать JIT в приложении под эту гипотезу нельзя. Probes rollback-only, без результата с customer data в логах (`incident-plan-*`, `prevac-original-*`, `incident-current-statistics.json`, `incident-reanalyze.json`).
8. Для actual-plan capture подготовлен приватный PostgreSQL executor hook из **уже имеющихся** headers и GCC, без скачивания, CREATE EXTENSION или изменений production image/DB. Он загружается только новым isolated QA PostgreSQL process; логирует structural/numeric plan и START/RUN CPU/wall/JIT timing, сохраняет исходный ERROR. SQL text, expressions/filters, bind arguments и session data не экспортируются. Calibration pg_sleep при1s сохранил SQLSTATE57014 и~999мс RUN, подтверждая error capture. Initial helper compile FAIL (missing header) сохранён, исправленный compile PASS.
9. Fresh server-hook run без client tracer **PASS100.242s**.162 aggregate executions: max actual plan cost574, JIT flags0, max RUN289.808мс/CPU286.374мс; actual canceled aggregate не возник. Это реальный другой plan state и ограничение экстраполяции: поздний успешный план не считается планом предыдущих отмен. Shared preload/track_functions относятся только к этому диагностическому QA server, не к приложению и не к глобальным настройкам.

Причинная развилка остаётся открытой: отмена того же SQL воспроизведена в transient diagnostic model, но его actual failing plan/состав CPU работы не получен. Observer effect, PostgreSQL план/статистика и host resource state не разделены до единственной доказанной первопричины. Не приписываем ошибку JIT/vacuum/sampler без такого доказательства. **Новых SQL/RLS/schema исправлений приложения нет.** Не менялись planner/cache/timeout defaults production, FORCE RLS и авторизация.

### Регрессии и готовность текущего запуска

Текущие результаты3 ноября, по сохранённым exit codes, без подстановки historical PASS:

|Проверка|Фактический результат / evidence в `.local/hardening11a-final-20261103/`|
|---|---|
|Go tests/vet/gofmt/build|PASS `go-baseline`, exit0|
|Go race|PASS `race-baseline`, exit0|
|Fresh PostgreSQL integration/race, migrations, FORCE RLS и tenant/security regressions|PASS74.572s `integration-baseline`; включая Telegram/WhatsApp, consent, approval, inventory, retention, idempotency и financial recovery|
|Frontend lint/TypeScript/OpenAPI/native/artifact checks|PASS `web-check`;6 Node tests,0 skipped|
|Fresh Next production builds|PASS4/4 DESKTOP_A/B, LINUX_A/B; outputs независимы, source digest совпадает|
|Fresh normal production Browser E2E|PASS9/9,815.106s `browser-fresh-full`;0 skipped/unexpected/flaky; оба mock-канала и существующие desktop/mobile сценарии|
|Accumulated normal Catalog Browser E2E|PASS2/2,55.505s/85.711s `accum-browser-1/2`|
|Strict1s accumulated ControlPage probe|PASS75 calls/4workers,7.753s `accum-strict-control`; обычный PG без executor hook, без client SQL tracer; каждое SQL deadline1000мс|
|Artifact + served-build после свежего и накопительного Browser|PASS `integrity-fresh-final`, `served-fresh-final`, `integrity-accum-final`, `served-accum-final`|
|Независимые остальные artifacts после всех runs|PASS `integrity-DESKTOP_A-final`, `integrity-LINUX_A-final`, `integrity-LINUX_B-final`; четыре proof hashes неизменны|
|Исходный загрязнённый artifact|**FAIL42 — ожидаемый fail-closed отказ**, `integrity-original-final`; Browser для него не запускался|
|Transient SQL incident-model A/B/A|**FAIL50/75 в каждом из3 runs**,150 AGGREGATE57014 подтверждены PostgreSQL; failing actual plan не сохранён, причина NOT PROVEN|
|Diagnostic server-hook fresh run|PASS100.242s; это наблюдатель/диагностика, не замена ordinary acceptance|
|sudo policy, privileged fs_usage, cross-user PID mapping/stop escalation|**NOT RUN**; подготовлены только план и10 непривилегированных selftests|

Fresh9/9 и оба accumulated Catalog reports содержат `explicit_read_retry=false`, `automatic_write_retry=false`. Логи каждого соответствующего normal PostgreSQL container от запуска до финальной проверки:0 statement timeouts,0 lock timeouts. Scope не включает намеренную calibration отмену и150 diagnostic FAIL в других DB проектах. Summaries: `browser-fresh-final-summary.json`, `browser-accum-1/2-final-summary.json`; исходные Playwright JSON и screenshots сохраняются в `.local/audit8d/browser-results/stage11a-final-*` с SHA256 отчётов в summaries. Три subprocess-only integration helpers SKIP по дизайну; их parent crash tests запускались. Не выдаём SKIP за отдельно выполненный helper gate.

Accumulated source volume42271 variants/1665 imports скопирован read-only в новый QA clone; исходный volume не менялся. После двух повторов clone: **54291 variants/2139 imports/76 tenant catalogs,9 catalogs>5000**. Это сумма QA tenants, не размер одного магазина. Сценарий одного магазина проверяет6008 variants и237 imports с пагинацией дальше recent100. Strict1s probe на исходном accumulated clone: p50316.129мс/p951087.472мс/max1415.185мс **полного service call**, failures0, canceled acquires0; service содержит несколько SQL, поэтому service>1s не означает превышение лимита одного SQL. `accumulated-final-counts.json` также подтверждает45 FORCE RLS tables,0 RLS-enabled tables без FORCE. Новые индексы, миграции и увеличение timeout не вводились.

Runtime SHA256 `/proc/1/exe` обычных API/worker в обоих Browser проектах совпадает с `artifacts-normal`. Все новые QA networks internal, host port publications0; normal LLM mock, channel live/external authorization выключены, sending kill switches включены, WhatsApp budget0 (`runtime-security-final.json`). Приватные diagnostic binaries/PG hook не подменяли обычные acceptance binaries. Production roles/RLS/security gates и audit/receipts сроки не менялись.

Original proof SHA256 и inode/birthtime/mtime/ctime обоих старых загрязнённых каталогов **точно совпадают** с началом (`e01-final.json`); старые чистые control proofs/paths также неизменны. Четыре новых proof hashes совпадают с сохранёнными до E2E (`e01-proof-final.json`). Первый дополнительный RO proof-reader получил BLOCKED125 из-за отсутствующего cached image; pull был запрещён, download не выполнялся. Чтение повторено уже проверенным cached Playwright image и прошло; этот helper blocker не скрыт (`proof-recheck-initial.json`). Исходные contaminated evidence и guard/baseline не исправлялись.

Cleanup: только явно перечисленные собственные QA containers штатно остановлены `docker stop -t -1`, без force kill, удаления containers/networks/volumes или исходных FAIL. Другие работающие проекты сохранены (`cleanup-final.json`). Demo/Audio/IDE процессы не были целями сигналов; working demo не пересобирался и не мигрировался. Доступность browser demo не является результатом этих QA gates.

**Изменения этого продолжения:** только `docs/STAGE_11A_SECURITY_RELIABILITY_REPORT.md`, `docs/STAGE_11B_PILOT_READINESS.md` и приватные диагностические/evidence файлы под `.local/hardening11a-final-20261103/`. Production Go/frontend source, контракты, migrations, AGENTS, CI и глобальные настройки не менялись. Source hashes401 ранее зафиксированных файлов сверены; итоговый manifest и индекс: `source-changes-final.json`, `INDEX.md`. Новых dependencies/install/downloads, внешних API, commit/push/deploy не было.

**Нерешённое:** E01 не воспроизведён в новых outputs; writer/mechanism неизвестны. SQL aggregate был повторён с настоящими отменами, но transient failing plan/CPU attribution не получены. Более поздние PASS не являются причинным устранением. Следующий SQL шаг — захват actual canceled plan в том же transient state, с контрольным run без observer и сохранённым pre-run snapshot; поздние быстрые планы не подставлять вместо него. Исправлять приложение до этого под JIT/vacuum/sampler гипотезу нельзя. Состояние CPU/VM остаётся отдельным ограничением измерений.

### Отдельно согласуемый следующий E01 шаг

Полный проверяемый план: `.local/hardening11a-final-20261103/E01_HOST_AUDIT_PLAN.md`. Подготовлены root-wrapper, exact paths manifest/hash, calibration и отдельный новый workspace; выполнены **только10 непривилегированных selftests** parser/privacy/native TID64 mapping. Ни sudo policy, ни privileged run, ни фактическая stop escalation не запускались. Старый широкий pipeline не использовать: новый wrapper держит raw fs_usage в RAM/pipe, выводит только sanitized fields и ограничивает600с/1MiB/1000 events. Проверка применимого sudo/централизованного I/O logging остаётся предварительным blocker — безопасность неизвестной политики не объявляется подтверждённой.

Существенное уточнение local man: wide fs_usage suffix описан как **thread ID**, не гарантированный PID. Прямое присвоение suffix полю PID запрещено; подготовлен PROC_PIDTHREADID64INFO lookup с проверкой на собственном непривилегированном процессе. PID при невозможности mapping остаётся UNRESOLVED. Реальный calibration root имеет отдельный scope и не считается E01. Точные будущие sudo/helper/build/stop commands, права, ограничения и last-resort сигнал **только собственному observer PID** перечислены в плане. Требуется новое отдельное согласование; пароли не запрашиваются, sudoers/SIP/security settings не меняются.

**Закрытое staging: NOT READY / NOT AUTHORIZED.** E01 writer неизвестен, aggregate1s cause неизвестна; gates подтверждают только проверенные состояния. Security CONDITIONAL и11А OPEN сохраняются; live-интеграции и платные API выключены. Ни текущий отчёт, ни успешный gate не дают разрешения deploy. Формальное10E не закрыто автоматически; audit/receipts сроки не устанавливаются.


## Recovery11А +12А — Critical path to first deployment

Evidence: `.local/recovery12a-20261103/`, mode0700. Прочитаны фактически существующие11А/11Б, E01_HOST_AUDIT_PLAN и global/project AGENTS. Нового общего аудита проекта не проводилось. Рабочие demo/другие DB не использовались; исходные failed volumes/артефакты сохранены. Новых migrations/dependencies/global settings/CI изменений нет, sudo и внешние вызовы не выполнялись.

### Aggregate1s: ROOT CAUSE PROVEN в воспроизведённом сценарии

Точка: `catalog.Service.ControlPage`, первый aggregate `WITH r AS (...), a AS (...)`; HTTP `GET /api/v1/organizations/{id}/catalog-control?page=...&import_page=...`. SQL SHA256 остаётся `a9ced8f813692041fd36cb3fd5bc63b47401551047b19c91c46f4fc34ec189d1`. Параметр aggregate — только tenant organization UUID; поиск/фильтр не влияют на aggregate. В trace UUID/session hash/аргументы не сохраняются, используются SMALL/LARGE_A/LARGE_B и request ordinal.

Две новые копии защищённого historical volume созданы с RO source. Логическая incident model снова убирает только post-incident6002-variant fixture **из новой копии**, оставляя две большие организации по6001; autovacuum=off относится только к этим диагностическим PostgreSQL servers. Это не точный physical snapshot исторического момента. Default-resource реальные HTTP handlers (in-process transport) прошли2×75 calls: один без server hook, второй с ним. Новый QueryTracer не делает дополнительных SQL/SAVEPOINT/EXPLAIN в измеряемой транзакции: только safe operation/fingerprint/PID/request/timestamps/SQLSTATE/cancel classification. Transaction timing восстанавливается по BEGIN/COMMIT/ROLLBACK; межсетевой RTT здесь не измерен.

Обычный psql probe затем повторил4 отмены EXECUTE/EXPLAIN ANALYZE при1s. Первый hook не распознавал prepared-probe source prefix; это ограничение сохранено. Observer расширен **только в private QA** на известные PREPARE/EXECUTE/EXPLAIN measured prefixes, собран cached GCC/headers без downloads. На следующем unthrottled on/off/on все executions уложились в1s: JIT on~0.47–0.64s, off~0.08s. Факт более позднего PASS не объясняет прежние отмены.

Причинный контроль использует **планируемый deployment limit PostgreSQL1.5vCPU/2GiB**, а не произвольное снижение timeout. Same DB/data/query,4workers,75 HTTP calls, statement_timeout1000ms, HTTP timeout5s:

|Серия|AGGREGATE отмены /50 large reads|p50/p95/max большого aggregate, мс|
|---|---|---|
|Default JIT ON1|47|1057.279 /1481.069 /1699.780|
|Diagnostic connection JIT OFF|0|284.449 /499.007 /584.847|
|Default JIT ON2|48|1403.602 /1754.797 /1815.184|
|Scoped transaction-local fix, default connection JIT ON|0|199.746 /528.106 /604.773|

Все95 отмен классифицированы именно **STATEMENT_TIMEOUT**, SQLSTATE57014; это не HTTP5s/context deadline. Клиентский trace связан с PostgreSQL backend PID и captured actual plan/timing. Server executor hook сохранил cost~121471, JIT flags25, около239 generated functions и отмену в RUN с доминирующим LLVM emission: пример wall1187ms/CPU429ms, emission1115ms. Counters generation/emission не следует складывать с RUN как непересекающиеся фазы. JIT задерживает обработку interrupts, поэтому observed wall может превышать1000ms без увеличения лимита. Structural plan bounded200 nodes; top cost/JIT flags и timing/error — actual execution, не поздний EXPLAIN, подставленный вместо него.

Это causal proof дорогой LLVM compilation в коротком interactive path **под воспроизведённым resource contention**. Глубокие RLS subplans повышают estimated cost до JIT threshold; JIT overhead не окупается на6001 rows. Это не доказательство дефектности/лишности RLS. CPU quotas и shared Docker host отличаются от реального4-core VPS; точный вклад всех host/VM задержек не объявляется измеренным.

Неудачные query-only candidates сохранены: single-pass aggregate rewrite не снял threshold и дал ещё больше JIT functions; явные дополнительные tenant predicates также не снизили cost. Они **не внесены** в production. Последние metadata EXPLAIN probes также не превращаются в доказательство причины отмен.

**Минимальное исправление:** после успешной authorization/shared tenant fence в `ControlPage` выполняется `SET LOCAL jit=off`. Только эта interactive transaction; aggregate/page/metadata SQL, параметры, RLS, policies, indexes и timeout не меняются. COMMIT/ROLLBACK восстанавливает прежний connection setting. Production/server-wide JIT default остаётся on. Это измеренное scoped tuning, не отключение security gate и не повышение deadline. Tradeoff: для существенно больших будущих catalogs может понадобиться отдельный benchmark, а не blanket JIT configuration.

На той же модели normal-default connection + local fix:75/75 HTTP calls PASS,0 canceled acquires. Metrics/health digests для всех3 synthetic target classes совпали с успешными reference ответами. Проверен JIT reset всех idle connections. Новая integration regression дополнительно проверяет aggregate/page JIT off **в сервере**, reuse одного backend PID между tenants и JIT on после commit и over-limit legacy rollback. Она реально прошла в full integration/race gate.

Evidence: `sql-quota-http-on1/off/on2.*`, `sql-quota-hook-events.json`, `sql-aggregate-before-after.json`, `sql-local-jit-comparison.json`, `sql-artifacts/http-QUOTA_*.jsonl`, `sql-causal-*.json`, `sql-causal-status.json`. Первые исторические150 отмен по-прежнему не имеют incident-time plan; новое причинное воспроизведение того же SQL **не выдаётся за ретроспективную атрибуцию каждой из них**.

### Отдельные metadata отмены: ROOT CAUSE NOT PROVEN / OPEN

Во второй diagnostic reference серии с connection JIT OFF были2 SQL57014 в `ControlPage` при чтении последнего import preview: `SELECT coalesce(jsonb_agg(x),'[]') FROM (... platform.catalog_imports WHERE minimized_at IS NULL ORDER BY created_at DESC LIMIT1) x`. HTTP73/75 PASS,2×503; этот run — **FAIL**, даже при совпавших metrics успешных ответов. Это не aggregate и не старый исправленный page query. Safe fingerprint/PID/request/duration/source line: `sql-oracle-other-errors.json`.

Single later EXPLAIN ANALYZE/BUFFERS на том же model: original2.838ms/cost269.85; диагностический scoped-org вариант0.260ms/cost90.85. Incident locks/planning/CPU state этих двух отмен не сохранены. Быстрый поздний план не доказывает их причину; scoped variant **не внесён** в source. New local-fix75/75 и normal accumulated75/75 не объявляются причинным устранением metadata риска. Последняя ограниченная серия75 HTTP calls с отдельным LAST_IMPORT actual-plan hook и numeric wait/CPU collector прошла без ошибок; захвачено75 metadata plans, max RUN64.967ms. Actual canceled metadata не возник, поэтому причина остаётся NOT PROVEN; дальнейшие одинаковые повторения остановлены (`metadata-last-summary.json`). SQL reliability остаётся открытой в этой границе, без произвольного нового timeout или маскировки FAIL.

### E01: ROOT CAUSE NOT PROVEN / OPEN; Linux artifact boundary

Использованы предыдущие4 independent build experiments/observer calibration и защищённый исходный contaminated tree. В новом **отдельном Linux named volume** выполнен fresh Next production build неизменённым prepared runner, только cached glibc dependencies; E01 не появился. Source tree и build input RO, output эксклюзивный, served artifact RO/immutable. Ни чужой host watcher, ни другая сборка не имеют общего writable output этого run.

Это дополняет проверенные Linux samples, но **не доказывает**, что E01 возможен только на macOS. Исторический filesystem writer/точное создание по-прежнему неизвестны; нового события для PID attribution нет. Guard unchanged: verify перед export, после export, при каждом image startup и served marker. Исходный polluted output всё ещё даёт42 и сохраняет original proof/inode/timestamps. Никакого удаления суффиксов/rebaseline/exclusions нет. Публикации/размещения вообще не происходило. Hash/BUILD_ID текущего Linux artifact находятся в release manifest; разные Next BUILD_ID не считаются byte reproducibility.

Privileged ветка остановлена; существующий exact host-audit plan остаётся отдельным предложением. sudo policy/fs_usage/admin password NOT RUN. Нового разрешения в12А на них нет.

### Текущие gates после scoped fix

|Проверка|Фактический результат|
|---|---|
|Go tests/vet/gofmt/build; Go race|PASS `go-final`, `race-final`; новый runtime secret loader tests включён|
|Fresh PostgreSQL integration/race/RLS|PASS164.010s `integration-final`; новая JIT scope regression0.39s; оба канала, consent/staff approval, inventory/retention/idempotency/financial/tenant suites|
|Frontend lint/TypeScript/OpenAPI/6 native/artifact tests|PASS `web-check-current`; frontend source не менялся после build|
|Fresh Linux Next production|PASS `web-build-linux`, record/verify; source digest8785bb4abeef20f5e0f022e2dcdb9d0c52526516d8c6d771d264cd3081b0b727|
|Fresh full Browser на final API/worker и current standalone|PASS9/9,899.713s,0 skipped/unexpected/flaky `browser-final-full`|
|Accumulated Catalog Browser на final API/worker|PASS2/2,66.795s/53.130s `accum-catalog-1/2`|
|Accumulated strict HTTP on normal PG|PASS75/75,4workers,1s per SQL, default JIT restored; **без client tracer/server hook** `accum-sql-final`|
|Artifact/served integrity|PASS `integrity-final`, `served-final`, `integrity-accum-final`, `served-accum-final`; final image-served check PASS, original polluted image-start отказ42|
|Final deployment Docker images + Compose validation|PASS `release-final`, `package-validation-final`; no pull/network during build|
|Production-image local HTTPS/auth smoke|PASS `deployment-smoke-final`; CA verified, Secure/HttpOnly cookie,401/CSRF403/logout, local mail verification, catalog200|
|Native Sharp final image|PASS1×1 PNG `native-runtime-final`; initial missing-lib FAIL сохранён, dependency не скачивалась|
|Backup/restore|PASS final synthetic6001 variants/237 imports; source/restored effective ACL/owner/policy/FORCE RLS match; separate rollback-only tenant probe foreign0/anonymous0/own6001/imports237; restored API ready200|
|Deployment admission regression|PASS4 Python tests с группами negative cases; live/exposure/tampered manifest отвергаются|
|Image embedded QA-secret scan|PASS0 matches9 фактических synthetic secret values в final archive; **не универсальный secret scanner**|
|Real Ubuntu/VPS/SSH/firewall/deploy/rollback/advisory scan|NOT RUN; не разрешены/не выполнены|

Fresh/accumulated Browser cold-read attachments: explicit_read_retry=false, automatic_write_retry=false. Normal fresh и accumulated PG logs от запуска до последней проверки:0 statement/lock timeouts; diagnostic FAIL в другом project этим не стираются. Полный mock Browser выполнялся штатным prepared runner, **не** production live processing; production package отдельно проверен HTTPS/auth smoke с запрещёнными channel keys/live. Три subprocess-only helpers SKIP по дизайну; parent crash scenarios выполнялись.

Accumulated source54291 variants/2139 imports скопирован RO в новый clone. После2 Catalog repeats:66311 variants/2613 imports/82 tenant catalogs,11>5000;45 FORCE RLS tables,0 RLS-enabled без FORCE. Это суммы QA tenants, не размер одного магазина; один сценарий проверяет6008/237 и историю дальше recent100.75 HTTP calls при PG1.5CPU/2GiB:0 errors; полный service p951629.838ms/max1718.240ms содержит несколько SQL и не является превышением одного statement deadline1000ms. CPU quota — локальный тест, не capacity certificate реального VPS.

### Deployment package и оставшиеся blockers

Подготовлены artifact Dockerfiles, private production Compose, TLS proxy/PG HBA, restricted bootstrap, `_FILE` secrets loader, health/startup/resource/log limits, offline image manifest/archive, manual GO controls и isolated backup/restore scripts. Подробный список source файлов/границ: `STAGE_12A_DEPLOYMENT_READINESS.md`, `VPS_DEPLOYMENT_RUNBOOK.md`; exact file hashes и evidence index — `.local/recovery12a-20261103/`.

Initial deployment FAIL сохранены и исправлены: quoted tmpfs YAML; `.sql` bootstrap вместо не исполнившегося host `.sh`; nginx default temp paths перенесены в `/tmp` с сохранением read-only root; cached glibc resolver library включена в minimal Node image после native Sharp FAIL. Half-initialized failed PG volume сохранён; успешная репетиция использовала новый DB project. Backup сохраняет owners/ACL и local admin COPY через FORCE RLS, без выдачи admin runtime. Default versus explicit owner ACL сравниваются семантически, а не изменением данных ради byte match.

Все изменения — focused JIT transaction setting + regression, optional deployment runtime launcher, новые deployment assets/scripts и4 docs. **Ни одна migration, policy, role predicate, бизнес-логика заказов/каналов или срок audit/receipts не менялись.** Live/payments/budgets OFF/0. Никаких глобальных Docker address-pool изменений: их исчерпание было environmental FAIL, новые QA получили проверенные непересекающиеся internal subnets. Working demo/другие projects не останавливались. Собственные QA останавливаются штатно без SIGKILL и удаления volumes; финальная запись `cleanup-final.json`.

**11А OPEN; Security CONDITIONAL. E01 writer unknown; historical150 precise attribution unavailable;2 metadata cancels remain OPEN.** Linux image controls не закрывают E01, поздние PASS не закрывают metadata риск. Реальный VPS, base-image advisories/provenance, capacity, TLS/private access и encrypted backup custody ещё не проверены/согласованы. Никакого customer/live pilot или server GO.

**Вердикт: DEPLOYMENT PACKAGE READY, STAGING BLOCKED.** Следующий этап12Б — review blockers/residual risk и отдельное owner GO на ограниченную Ubuntu private-staging rehearsal; не автоматическое завершение11А/10E.

## Перепроверка 10 ноября

Evidence: `.local/recovery12a-20261110/`. До изменений все 420 файлов из `source-changes-final.json` совпадали с SHA256 предыдущего результата. Проверены 325 сохранённых evidence hashes, 2380 файлов прежнего release, checksum image archive и OCI descriptor/config/layer graph пяти linux/amd64 images. Это проверка сохранности доказательств, не повторение PostgreSQL/Browser экспериментов.

**Новый доказанный дефект:** private deployment validator проверял unsafe значения live/budget/kill flags только при их наличии. Удаление любого из 11 обязательных controls в API или worker принималось. Новая regression до исправления дала **FAIL, 22 subcases**; полный лог `admission-omissions-before.log` сохранён. Это пробел admission, не доказательство фактического live enablement: текущий Compose явно задаёт безопасные значения, runtime defaults не менялись. Минимальное исправление в `scripts/deployment/common.py` требует присутствия всех controls; прежняя проверка точных значений сохранена. После исправления **PASS, 5 tests**, включая 22 omission cases и прежние unsafe mutations.

|Проверка 10 ноября|Результат / evidence|
|---|---|
|Deployment baseline до новой regression|PASS, 4 tests; `deployment-baseline.*`|
|Regression до исправления / после|FAIL 22 omission cases / PASS 5 tests; `admission-omissions-before.*`, `admission-after.*`|
|Node native preflight/artifact guard unit tests|PASS 6/6; `node-guards.*`|
|Production Compose parse/interpolation|PASS, exit0; `compose-source.*`; daemon не требуется|
|Сохранённый clean Next artifact против текущего source|PASS; `clean-artifact.*`; это verify, не fresh build|
|Оригинальный polluted artifact|Ожидаемый отказ exit42; `original-e01-refusal.*`; proof/inode/birthtime/mtime/ctime неизменны|
|Исторические evidence/release/OCI archive|PASS; `preservation-oci.*`, `preservation.json`|
|Docker prerequisites|BLOCKED; socket отсутствует; `docker-preflight.*`|
|Новые Go tests/race/vet/build; PG integration/migrations/RLS/tenant/concurrency|NOT RUN (BLOCKED Docker); прежние PASS только от 3 ноября|
|Новые frontend lint/typecheck/fresh build, Browser E2E, accumulated SQL|NOT RUN (BLOCKED Docker); prepared runner не обходился host-запуском|
|Новые Docker build/startup/HTTPS smoke и DB backup/restore|NOT RUN (BLOCKED Docker); сохранённый archive проверен offline|
|Новые consent/staff approval и Telegram/WhatsApp mock runtime regressions|NOT RUN (BLOCKED Docker); прежние integration/Browser evidence сохранены|
|VPS/SSH/firewall/rollback/live/advisory downloads|NOT RUN; нет server GO/download approval|

Первый вспомогательный archive verifier завершился FAIL: ошибочно приравнял OCI index/manifest IDs к legacy config digests. Лог `preservation.log` сохранён; helper исправлен на проверку реального OCI graph, следующий `preservation-oci` PASS. Это ошибка новой диагностической проверки, не обнаруженный дефект image archive.

SQL production source и timeout не менялись. Причинный вывод о JIT относится к прежней controlled reproduction; первые 150 отмен и две metadata cancellation по-прежнему не имеют достаточной incident-time атрибуции. Новых SQL повторов без QA Docker нет. E01 writer неизвестен; clean verify/guard refusal не закрывают E01. sudo/fs_usage не запускались.

Отдельный производный package: `.local/recovery12a-20261110/release/`, с исправленным validator и актуальными docs, новым manifest и ссылкой на SHA256 родителя. Runtime images/Next proof и archive перенесены без изменения; исходный release/evidence не перезаписываются. Новых image build/start/load проверок это не заменяет. Manifest остаётся PACKAGE_ONLY_NOT_DEPLOYMENT_GO. Следующий шаг — восстановить доступность локального Docker и повторить prepared isolated gates, затем 12Б GO review; серверные действия требуют отдельного решения.

## Fresh VPS package rehearsal 10 ноября — после запуска Docker владельцем

Это новая фаза, `.local/vps-readiness-20261110/`, не переобъявление старых PASS. Fresh Go/tests/vet/build/race, PG integration88.883s, frontend lint/TS/OpenAPI, Linux Next build, Browser9/9 (825.463s), accumulated Catalog2/2 и strict HTTP75/75 прошли. SQL deadline1s, pool5/4workers, normal PG1.5CPU/2GiB; no client tracer/server hook/plan override, JIT reset true. Полный service p951697.607ms/max2198.423ms не является длительностью одного SQL. Две исторические metadata cancellation не воспроизвелись; причинное закрытие не заявляется. Первые150 отмен по-прежнему без точной incident attribution. **SQL reliability в этих границах OPEN**.

Одна новая exclusive Linux Next build прошла прежний guard; новый BUILD_ID55_-2Dh8LU1Js_xkUr8Dh. Исходный polluted tree по-прежнему даёт image-start exit42, proof/inode/birthtime/mtime/ctime сохранены. E01 writer не установлен, OPEN. Никаких guard exclusions/rebaseline, sudo/fs_usage, изменений SQL/RLS/migrations/timeouts.

Новый deployment coordinator и TOC-verified backup job реально проверены: clean install, private CA-verified HTTPS/auth/owner/shop, graceful restart/data digests, backup/restore6001 variants/237 imports с owners/ACL/policies/45 FORCE RLS и foreign/anonymous0. Rollback к прежним images и roll-forward к новым при schema16/served proof PASS. Это локальная QA, не Ubuntu/VPS GO; live/paid OFF/0, channel mocks отдельно подтверждены prepared integration/Browser.

Авторские defects новой automation (`node -e` no-op и acceptance unhealthy Mailpit) воспроизведены negative tests и исправлены; initial FAIL сохранены.21 Python/9 Node Linux nonroot tests PASS; final context test и preflight закрепляют проверенный Unix endpoint только в дочернем процессе, без global settings. Архив/OCI layers/known9 synthetic secrets проверены,0 совпадений, не универсальный/advisory scan. Native Ubuntu/SSH/firewall/security review/customer live/target capacity NOT RUN. Source package/scripts/docs и exact exits перечислены в новом INDEX; старый evidence не удалён. **PACKAGE READY, DEPLOYMENT BLOCKED; CUSTOMER PILOT NO-GO**.
