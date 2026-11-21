# Production readiness — выполненная работа

2026-11-17. **TRUSTED RELEASE BLOCKED · DE-3 STAGING NO-GO · CUSTOMER PILOT NO-GO.** Локальную демонстрацию на синтетических данных можно показывать. Это не production/security GO и не завершение всех фаз программы.

Evidence: `.local/production-readiness-20261117/INDEX.md`. Подробная матрица: [PRODUCTION_READINESS_GATES.md](PRODUCTION_READINESS_GATES.md). Новый хост: [DE3_STAGING_PREPARATION.md](DE3_STAGING_PREPARATION.md).

## Baseline и сохранность

Прочитаны действующие AGENTS.md, предыдущие UX reports, Stage12E5, Stage12C2/C3, trusted provenance и связанные release/backup/resource документы. Обязательные пути существуют. Source inspection охватил Go API/worker, Next BFF/auth routes, order/catalog contracts, migration16, PostgreSQL pool/RLS admission, offline runners, Compose, release packager и CI. Исторический PASS не объявлялся свежим результатом.

Git HEAD **UNBORN**; до этой работы уже были staged исходники, незакоммиченный UX и другие файлы. Сохранены исходные status/diff/staged-stat и 309-file source snapshot с SHA256 (`baseline.json`, `source-before/`). Ничего не staged/committed/pushed. Исторические OCI archives, evidence, рабочие volumes и чужие изменения сохранены. Изменения сервера, deployment, live/paid calls не выполнялись.

Локальные проверки изолированы: отдельные Compose projects с tmpfs `aba_test`, safety marker, restricted runtime role, internal-only network и без published ports. Исчерпанный default Docker address pool обойдён **для новых test networks** явными непересекающимися /28; существующие сети не удалялись, глобальные pools не менялись. Host имеет широкие корпоративные private routes; тестовые сети internal-only и не использовались для внешней маршрутизации.

## Подтверждённые P0/P1 и исправления

|Приоритет / дефект|Исправление / результат|Evidence|
|---|---|---|
|P0: Next 16.4.0 попадает под опубликованные advisories, включая critical RCE|Next + @next plugin 16.4.5, React/React DOM 19.3.1, согласованный lock. Совместимость Node engine/React peers проверена, API/auth contracts не менялись|npm-audit-before.json; npm-audit-candidate.json; web-build.log; frontend E2E|
|P0: старый Go builder 1.26.2 против требуемого go.mod 1.26.11|`apps/api/Dockerfile` закреплён на официальном Go 1.26.11-alpine3.23 index digest `sha256:23f198931d4bd83b4f5a613bd75656257ae8bf832dcbc2db80e1050b5219ca2a`. Image не загружался и не собирался|golang-base-tag.json; Dockerfile diff|
|P1: свежая DB находит 20 Go module findings|x/crypto 0.50→0.56, x/sys 0.43→0.47, x/text 0.36→0.41, graph-required x/sync 0.20→0.22. Новые modules получены из Go proxy с checksum database; отдельный cache, global cache не изменён|go-dependency-update-retry.*; go.mod/go.sum; security-final/|
|P1: неполное operational evidence восстановления|Добавлен безопасный `scripts/readiness/database_rehearsal.py`: отказывает remote Docker/working projects, проверяет marker/internal network/no published ports, создаёт только новую DB, не перезаписывает archives|database-optimized/; readiness-optimized-safety.log|
|P1: CI не проверял часть готовых локальных guards|Добавлены dashboard/preflight/artifact, dependency high/critical audit, release/control и rehearsal safety unit gates. Остался manual-only запуск|.github/workflows/ci.yml; workflow-parse.json; локальные unit logs. Remote CI NOT RUN|
|P1: нестабильность SQL каталога|Свежий FAIL воспроизведён во время параллельных проверок; отдельный последовательный прогон PASS. Причина не доказана, код и timeouts не менялись|integration-final/ и integration-serial/. **OPEN**|

### Security advisory decision

Минимальная выбранная Next версия — **16.4.5**, поскольку [Nov3 advisory GHSA-mfr8-8qww-v76h](https://github.com/vercel/next.js/security/advisories/GHSA-mfr8-8qww-v76h) затрагивает версии до 16.4.4 включительно. Старый D2 на 16.4.1 уже недостаточен. [Critical GHSA-673q-mjm6-r5xr](https://github.com/vercel/next.js/security/advisories/GHSA-673q-mjm6-r5xr) исправлен с 16.4.1. React pair согласован с [RSC advisory](https://github.com/react/react/security/advisories/GHSA-xqc2-xf4c-x3h3); standalone top-level React update сам по себе не доказывает исправление vendored RSC в Next. Для Next применён patched framework release, ручной подмены vendored packages не было.

Fresh npm audit: **до: critical1 + moderate11; после: critical0/high0/moderate11**. Full audit exit1 сохранён из-за moderate ESLint/TypeScript/yaml toolchain findings, не обозначен полным security PASS. Trivy production lock graph: 0 findings; dev dependencies им по умолчанию не учитываются, поэтому npm audit отдельно охватывает их. Пакеты закреплены, свежий изолированный `npm ci --ignore-scripts` PASS (216 packages, lockfile не изменился), payload SRI проверен; registry ECDSA signatures шести Linux build inputs математически проверены. Это registry trust, не независимая publisher attestation.

Исторический net/mail defect: [`GO-2026-4986 / CVE-2026-39820`](https://pkg.go.dev/vuln/GO-2026-4986), fixed Go 1.26.3 (линия 1.26). Register/login/resend HTTP JSON → `auth.Email` → `net/mail.ParseAddress`; middleware ограничивает body, `Email` проверяет 1..254 ASCII bytes до parser. Других прямых net/mail calls при поиске source не обнаружено. Worker использует net/smtp, а не второй email parser. Существующие regression tests повторно выполнены с Go 1.26.11, включая oversized/Unicode/boundary/repeated requests и отсутствие input в логах. Полное дерево Go toolchain (15048 files) сверено с admitted archive SHA256 без расхождений; `go-toolchain-integrity.json` PASS. Новые пять cmd binaries имеют metadata **go1.26.11 linux/amd64**, stdlib отражён в Syft для всех пяти. Старые OCI images этим не исправлены.

После Go module update остался **один module-level UNKNOWN `GO-2026-5932` (OpenPGP)** в source и binary SBOM scan. [Официальный advisory](https://pkg.go.dev/vuln/GO-2026-5932) предупреждает, что пакет unsafe by design. Никакого OpenPGP в compiled import graph новых cmd нет; API использует x/crypto Argon2. Это ограниченное reachability evidence текущей сборки, не замена отсутствующего govulncheck. Finding не suppressed. Новых HIGH/CRITICAL в проверенном Go graph не осталось.

Node host/build runner остаётся **24.15.0**, что не равно актуальному patched runtime: [Node 24.22.3 — security release](https://nodejs.org/en/blog/release/v24.22.3). Отдельное разрешение на release input acquisition запрошено; замена Node/OCI inputs не выполнена. Production GO на текущем Node не выдаётся.

## Фактические проверки

|Проверка|Результат / граница|
|---|---|
|TypeScript / ESLint / production build|PASS, Next 16.4.5; отдельный Linux workspace/native modules, scripts disabled при acquisition; artifact verify + served verify PASS|
|Frontend E2E|9/9 PASS; после запуска нового Go API/worker ещё 3/3 order/tenant/empty/filter tests PASS. Только synthetic fixtures|
|Responsive/accessibility|94 custom checks +20 dark route checks PASS; 320/375/390/768/1024/1440, light/dark/system, CSS zoom200%, labels/focus/menus/contrast. 0 browser page/console errors. VoiceOver/axe NOT RUN|
|Go modules/unit/race/vet/build|PASS на окончательных locks; пять cmd binaries. API/worker повторная сборка дала побайтово те же outputs; это локальная воспроизводимость, не независимый rebuild|
|PostgreSQL integration race|Последний serial run PASS, 184.425s, schema16, FORCE RLS/roles/pool/auth/order/catalog/worker/channel/financial/import/crash regressions; 163 PASS entries. Сохранён отдельный concurrent run FAIL|
|Три populated tenants после restore|54 отрицательных SELECT/UPDATE/DELETE across products/orders/conversations, положительные owner reads и 9 context-free reads PASS; фактические DB fingerprints после проверок не изменились|
|Backup/restore|PASS: 2,530,571-byte dump; SHA256 `a620f694dce232b188d0dd3414f7ffb7c65d3ee2cd301fb833034499ad49fa91`; dump 1.365s, restore + data/ACL/RLS/policy verification 19.043s, 57 tables. Это локальная БД, не off-host/full service RTO|
|Control/release unit tests|35 deployment +3 rehearsal safety PASS; artifact/preflight Node tests и4 dashboard render tests PASS|
|Secret scan|Trivy secret scan selected current app/scripts/infrastructure/workflow source: PASS tool execution, 0 locations. Secret values не экспортировались; ignored operator env/local backups вне scope. Не полный аудит всех игнорируемых файлов/истории Git|
|govulncheck|BLOCKED: отсутствует. Запрошена отдельная локальная установка, не выполнена|

Окончательная rehearsal повторена с `python3 -O`: PASS; safety checks используют явный `require`, а не отключаемый assert. Отдельный optimized-code negative test подтверждает отказ при неверном marker до записи. Новая rehearsal проверяет populated tenants, чтобы отрицательные проверки не проходили на пустых чужих таблицах. Raw ACL representation (`owner-only` vs PostgreSQL default NULL) нормализуется через `aclexplode(coalesce(relacl,acldefault(...)))`; права не ослаблялись. Сохранён диагностический diff первоначального сравнения.

### Сохранённые FAIL и performance

Первый Docker network start отказал из-за исчерпанных default pools; первый Go update — из-за unwritable sumdb GOPATH; первый обновлённый offline runner — из-за неполного copied module cache. Исправлены только runner/input preparation, не проверки. Первое имя Node presentation test было неверным; корректный существующий test PASS. Первый browser invocation дал exit0 без JSON evidence и **не принят**; subsequent explicit-config run имеет9 genuine PASS. Первый secret scan потребовал `.tpl` extension; исправленный invocation PASS. Все первоначальные логи сохранены.

В `integration-final` каталог page55 вернул SERVICE_UNAVAILABLE, lock observation не уложилось в 750ms. В отдельном `integration-serial` тесты прошли, но p95 121-page sweep **827.8ms**, p95 конкурентных24 reads/4 workers **1275.9ms**. Сравнение ограничено разными условиями нагрузки; считать causal proof CPU contention нельзя. Предварительный read SLO500ms не достигнут. Нельзя повышать SQL timeouts/ослаблять lock tests или заявлять production capacity. **SQL cancellations OPEN, E01 OPEN.**

Mixed profiles 5/10/20/30 shops, p99/RPS/error/resource/queue time series, full worker load, off-host restore и новый DE-3 load test — **NOT RUN**. Вместо них не выданы фиктивные числа ёмкости.

## Supply chain и release

Обновлена DB Trivy из `ghcr.io/aquasecurity/trivy-db:2`, verified manifest/layer SHA256; UpdatedAt17Nov06:59UTC, NextUpdate18Nov06:59UTC. Trivy0.76.2/Syft1.57.1 запускались из ранее admitted tools в no-network/no-socket containers. Сохранены новые binary SBOM и свежие findings. Первый обычный `trivy fs` на директории ELF не обнаружил packages — его пустой Results **не принят**; используется scan реально наполненного Syft SBOM.

Свежая переоценка **исторических inventories**, не новый image scan:

|Старый image role|HIGH|CRITICAL|Новый digest|
|---|---:|---:|---|
|API (включает worker/migrate)|32|0|NOT BUILT|
|Web|1|2|NOT BUILT|
|PostgreSQL|32|1|NOT BUILT|
|Proxy|16|1|NOT BUILT|
|Mailpit|58|1|NOT BUILT|

`legacy-image-reassessment/subjects.json` связывает эти scans с SHA256 прежних SBOM. Их CVE coverage ограничена полнотой прежнего inventory. Новых OCI archives/пяти image subjects/целостного release manifest **нет**; старые release digests не переобозначались новыми. `binary-provenance.json` — unsigned local factual statement, не независимая attestation. Cached builder carrier provenance и полный runtime/base/CA input chain не закрыты.

`d2-current-assessment.json` рассматривает все24 прежних payload: три старых npm inputs superseded, два Go module ZIP присутствуют и проверены checksum database; Node и18 OCI blobs не приобретены в этом запуске. Старый approval document не изменён и не считается текущим согласием. Legacy `package.py BASES` и mutable Node default пока нельзя использовать как trusted recipe.

## Изменённые файлы и rollback

Ровно шесть существующих source files изменены относительно session baseline: `.github/workflows/ci.yml`, `apps/api/{Dockerfile,go.mod,go.sum}`, `apps/web/{package.json,package-lock.json}`. Новые: `scripts/readiness/{database_rehearsal.py,test_database_rehearsal.py}` и три документа этой программы. Полный список — `changed-files.json`. Backend logic/RLS/migrations/roles/provider config/UI branding не изменялись.

До rollback сначала сверить текущие файлы с `source-after.json`, чтобы не затереть параллельную работу. Затем восстановить **только эти шесть файлов** из `source-before/`, а не делать git reset. Старый host node_modules сохранён отдельно; новый npm dependency tree и прежний cache не удалены. Web workspace новый, старый artifact proof не переписан. Для возврата только synthetic preview использовать старый `.local/ux-final-polish-20261117/focus.sh`; сохранять данные и archive evidence. Новые test DB/projects не являются production; автоматической чистки/`down -v` не выполнялось.

## Оставшееся и решение владельца

1. P1: причинный SQL cancellation/lock investigation, повторный measured acceptance; не закрывать из-за одного PASS.
2. Отдельное разрешение на govulncheck и на проверенные patched Node/OCI release inputs; после проверки exact descriptors обновить producer inputs, собрать новый isolated release и выполнить все release-bound gates.
3. Новый DE-3 identity/access/recovery/read-only audit scope. Не использовать старый VPS и не запрашивать пароль в чате.
4. Затем mixed load/operational monitoring/alerts/off-host DR, новый host audit, owner-scoped private staging GO. Customer pilot только после закрытия hard NO-GO и проверки release restore.

**Demo:** YES, local synthetic only. **Trusted release:** BLOCKED. **Closed DE-3 staging:** NO-GO. **Real pilot:** NO-GO. Результат программы частичный: подтверждённые dependency defects исправлены и проверены, внешние и причинно не закрытые gates честно остаются открытыми.
