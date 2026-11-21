# Release supply chain review

Финализация 12 ноября 2026, Stage 12C; локальные проверки 11 ноября. **PARTIAL / UNVERIFIED PROVENANCE. PRE-VPS: SECURITY / SUPPLY CHAIN BLOCKED. CUSTOMER PILOT NO-GO.** Проверка локальная, без downloads, установки tools или advisory DB. Наличие content digest не доказывает происхождение и отсутствие уязвимостей.

## Reviewed baseline и inventory

Input — `.local/stage12b-20261111/verified-release/manifest.json`, SHA256 `e4229f1a2aff75e57c67cf73022edf199923e41bff4d242588f4e2c17fb5b715`. Все2396 защищённых файлов, archive checksum и пять OCI descriptor/config/layer graphs заново проверены; platform linux/amd64. Archive245025792 bytes (около233.7 MiB), SHA256 `9b11e16bd6412f427503f81bc172c3fb8ba70d747bfd504696a5468594dd9b6f`. Evidence: `.local/stage12c-20261111/baseline.json`, `oci-inventory.json`, `image-runtime-inspect.json`.

Git HEAD отсутствует; все project files untracked. `git-head.log/.exit` сохраняет исходный отказ, а `git-status.log` — фактический status. Связь с immutable Git revision **NOT PROVEN**. `source-before.json` фиксирует426 текущих файлов, а source-change hashes и retained Go/Next proofs позволяют проверить локальную content identity. Это не retroactive source/build attestation.

Machine inventory: `.local/stage12c-20261111/dependency-inventory.json`; partial CycloneDX1.6 inventory — `release-sbom.cdx.json`. JSON создан из lockfile, module graph, embedded Go metadata, OCI layers/APK DB и native version probes. Отдельного schema validator/SBOM scanner нет: schema validation **NOT RUN**, universal completeness не заявляется.

|Scope|Observed versions / content anchor|Purpose / trust / verification|
|---|---|---|
|Go source module|Go1.26.2;13 requirements в go.mod; full graph88 modules включая main|API/worker/migrator/health; go.mod/go.sum hashes в inventory. FRESH `go mod verify` PASS; graph включает optional/test modules, не все linked runtime|
|Linked Go runtime|API/worker/main tools Go1.26.2; module versions/sums в `buildinfo-api-*.log`|Embedded build metadata и локальная эквивалентность bytes; upstream/toolchain origin UNVERIFIED|
|npm source lock|lockfile v3,271 packages; Next16.4.0, React/ReactDOM19.3.0|FRESH npm ls all/offline exit0; versions/tree consistent. Lockfile SHA и registry/SRI на каждой row; tarball authenticity/current advisories не проверены|
|Runtime npm|27 version-bearing package.json observations, включая root duplicates и6 Next bundled components с отдельными версиями|Обычные package versions совпадают с lock. Vendored Next не отдельные lock entries;113 bundled package.json hashes совпали с cached installed Next. Полная SRI сверка всех installed bytes с registry tarballs NOT RUN|
|Node/native runtime|Node24.21.0, bundled OpenSSL3.5.8; Sharp0.35.5 и @img libvips package1.3.4|Node удовлетворяет engines >=24.15 <25, но это не source dev pin24.15.0. Actual versions в `node-runtime-version.log`, `sharp-native-versions.log`;10 copied Node/glibc/shared-library files byte-match cached Playwright builder|
|PostgreSQL|Actual runtime version в `postgres-runtime-version.log`; input tag18.3-alpine3.23, Alpine3.23.4;54 APK packages|DB; local immutable ID verified, signed upstream/build provenance отсутствует|
|Proxy|nginx1.27.3-r1, Alpine3.20.5, libssl3 3.3.2-r1;66 APK packages|Private TLS input; old stack приоритетен для approved advisory review. Возраст версии сам по себе не CVE/доказательство exploitability|
|Mailpit|Binary v1.21.8/Go1.23.4, input tagv1.21, Alpine3.21.0;16 APK packages|Synthetic SMTP/UI, без host ports; embedded47 dependency rows перечислены точно в inventory/buildinfo. Older Go/OS требуют advisory review|
|Python|Host CLIs используют stdlib/local modules, target3.12; tests FRESH Linux3.12.3 и host3.14.5|Third-party deployment requirements не обнаружены; Python не добавлен в scratch API/web runtime|

API и web FROM scratch не означают отсутствие зависимостей: API содержит Go stdlib/deps и CA bundle; web содержит Node, copied glibc/libstdc++/loader, bundled libraries и native addons. В scratch нет OS package DB, поэтому OS scanner по одной APK/dpkg DB был бы неполным. `builder-os-packages.log`, `runtime-native-provenance.json`, `sharp-native-versions.log` дополняют136 APK observations из трёх auxiliary images; package DB — inventory, не проверка подписи каждого пакета.

## Images, pinning и provenance limits

|Role|Runtime OCI ID|
|---|---|
|API/worker/migrate|`sha256:7d76a4d9bc7a51b7cc71f76bde13389fb525a6503a8881480e5d3140c24cadb2`|
|Next/BFF|`sha256:0a15ca0de37a5a0cb14225dd081226022d06cbd01721c7e1745ef43a63b43509`|
|PostgreSQL|`sha256:54451ecb8ab38c24c3ec123f2fd501303a3a1856a5c66e98cecf2460d5e1e9d7`|
|Proxy|`sha256:814a8e88df978ade80e584cc5b333144b9372a8e3c98872d07137dbf3b44d0e4`|
|Mailpit|`sha256:81370195cd4a0eab9604d17c2617a7525b0486f9365555253b6c5376c6350f1a`|

Stage12C derived candidate меняет host controls/docs, все пять runtime IDs и archive сохранены. Финальный host manifest/hash — `.local/stage12c-20261111/INDEX.md`, `release-digest-verification.json`. Source/release snapshots до documentation sealing сохранены отдельно.

Runtime Compose использует exact local IDs, pull_policy never, без build. `package.py` сначала разрешает cached tags в IDs, затем build --pull=false/--network=none и Node build arg=local ID. Mutable selectors в source **существуют и документированы**: package.py NODE=Playwright v1.64.0-noble, BASES=postgres18.3-alpine3.23/nginx1.27.3-alpine/Mailpit v1.21; developer Dockerfiles используют golang1.26.2-alpine3.23, alpine3.23.3, node24.15.0-alpine3.23; prepared smoke содержит aba-stage7-*:latest. Это development/cache selectors, не runtime release authorization; их наличие не гарантирует неизменность будущего rebuild. Полный найденный список/назначение — `mutable-source-refs.json`; cached builder IDs — `cached-builder-identities.json`.

RepoTags/RepoDigests и local OCI hashes проверяют content identity. Они не подтверждают подпись publisher, registry history, доверенный момент получения cache или build worker identity. Mailpit binary заявляет vcs.revision7703d099…, но embedded string не криптографическое подтверждение. Для всех previously cached support/build inputs: **UNVERIFIED PROVENANCE**. Никакая подпись/аттестация задним числом не создавалась.

Reproducibility **PARTIAL**: Stage12B fresh Go business binaries byte-identical deployed files; Next source/build proof/hash verified и native bytes совпадают с cached builder. Next build IDs меняются; byte-identical independent Next build/full OCI rebuild не доказан. Неисполненный future recipe не равен executed trusted build. Для closure нужен reviewable frozen source baseline/lockfiles/toolchain+base identities, approved acquisition/signature policy и новый controlled build, если историческое происхождение не восстановимо.

## Vulnerability tooling и findings

Evidence: `local-security-tools.json`, `advisory-database-availability.json`, `vulnerability-findings.json`. govulncheck, Trivy, Grype, Syft, Gitleaks, TruffleHog, OSV scanner и cosign executables не найдены; cached govulncheck source не является установленным scanner/актуальной DB. Usable local advisory DB не обнаружена. **Дата последнего обновления DB UNKNOWN**, а cache/file mtime не заменяет upstream update date. Новых scanners/DB не скачивали и не устанавливали.

`npm audit --offline` с пустым cache вернул exit0 и нули. Это **не vulnerability PASS**: cached npm11.19.0 Arborist явно делает `if (options.offline === true) return null` до advisory request. Сохранены stdout, verbose output и implementation: `npm-audit-offline.log`, `npm-audit-empty-cache-verbose.log`, `npm-audit-offline-implementation.log:320`. Advisory review npm, Go, OS и copied native libraries — **NOT RUN**, общий review PARTIAL. No CRITICAL/HIGH conclusion **UNASSESSED**, не «0 vulnerabilities».

|Finding|Severity / reachability|Status / action|
|---|---|---|
|12C-F01: alternative entrypoint/command accepted by validator|MEDIUM release-control defect; privileged config/operator access needed, no unprivileged app exploit shown|FIXED in current common.py; six original negative failures preserved. Old host controls BLOCK PRIVATE STAGING|
|12C-F02: manual control GO ignored scope/schema/migration approval|MEDIUM operator admission defect; privileged CLI and mismatched GO record required|FIXED in current control.py; five original failures, actual cross-scope refusal2 и valid scoped QA start/stop. Old host controls BLOCK PRIVATE STAGING|
|12C-F03: common CLIs did not reject remote Docker endpoint|MEDIUM operator boundary defect; explicit host/context misconfiguration required; no remote connection performed|FIXED shared local Unix guard/pinning before Compose;2 original offline FAIL,31 host/Linux tests and actual no-network refusal. Old host tools BLOCK PRIVATE STAGING|
|12C-G01: source/build/base origin not authenticated|UNKNOWN assurance gap, affects all deployed/compiled components|OPEN, BLOCKS PRIVATE STAGING; obtain genuine provenance or rebuild from approved inputs|
|12C-G02: current advisory review unavailable|UNKNOWN assurance gap; proxy TLS, authenticated API/Next, MIME/native parsing и DB need review even on private route|OPEN, BLOCKS PRIVATE STAGING; approve scanner/database/tool provenance and reachability review|

Для подтверждённой CRITICAL/HIGH CVE ещё нет достоверного current advisory evidence, поэтому vulnerable/fixed versions и exploitability не выдуманы. Это не утверждение отсутствия таких CVE. При approved review каждая CRITICAL/HIGH должна иметь component/version/runtime, advisory ID/date, reachable scenario, fixed version и действие; любая confirmed CRITICAL/HIGH блокирует staging. Private access не основание снижать severity или объявлять неиспользуемость без call-path evidence. Массовых upgrades не было.

## Secret review

FRESH bounded scan27 unique layers/607238130 regular-file bytes, nine existing synthetic QA values и curated AWS/GitHub/private-PEM signatures:0 matches; image metadata/history/env scan также без выявленных совпадений. Ни env/run-secrets payload files не обнаружено. Build scripts используют только ID build arg, mount secret pattern отдельно; selected build/test logs проверены без публикации совпадающих values (`secret-log-review.json`). Это **PARTIAL**, не универсальная secret-scanner certification/entropy proof и не анализ всех когда-либо существовавших logs.

Native isolation/kill/budget/tenant evidence уменьшает текущую exposure, но не закрывает supply-chain gates. Отдельные integrity PASS, SBOM и local QA GO не разрешают VPS/customer/live. Следующий разрешаемый этап: owner-approved scanner+current DB acquisition/review и provenance policy/frozen baseline; затем native first boot/rehearsal на купленном сервере после отдельного authorization. См. [Stage12C report](STAGE_12C_PRE_VPS_SECURITY_REPORT.md).
