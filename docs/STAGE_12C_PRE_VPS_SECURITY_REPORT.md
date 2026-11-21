# Stage 12C — pre-VPS security and release readiness

Финализация 12 ноября 2026; локальные runtime проверки выполнены 11 ноября и сохраняют свои timestamps. **PRE-VPS READINESS: B. SECURITY / SUPPLY CHAIN BLOCKED. CUSTOMER PILOT: NO-GO. Security: CONDITIONAL.**

Покупка не получила verdict READY TO PURCHASE VPS: mandatory provenance/advisory review остаётся незавершённым. Это не отказ локальной инфраструктуры: Docker и разрешённые QA работают, три подтверждённых deployment-control дефекта исправлены. VPS ещё не куплен; серверные проверки и deploy не выполнялись.

## Baseline и release identity

Проверены все9 priority files задания,65 локальных Markdown links,373 ранее sealed Stage12B evidence hashes и соответствие16 source-change records исходному состоянию. Полные результаты — `.local/stage12c-20261111/baseline.json`. Previous releases/evidence не перезаписаны. Git HEAD **UNBORN**, весь project untracked; `git-head.log` сохраняет исходный FAIL, `git-status.log` — фактические изменения. Commit/source-revision attestation отсутствует; baseline426 source files сохранён в `source-before.json`.

Stage12B input manifest SHA256 `e4229f1a2aff75e57c67cf73022edf199923e41bff4d242588f4e2c17fb5b715`,2396 files/пять OCI graphs заново проверены. Historical vps-readiness manifest `b2a0387dbdc8febbfc0f81af57b84d4dd39d682335ef3c9e5cb0a1b5b93b2af1` сохранён. Security CONDITIONAL / staging blocked / customer NO-GO не превращены в PASS по одному отчёту.

Текущий derived candidate: `.local/stage12c-20261111/verified-release/`. Меняются host scripts/tests/docs; все пять runtime IDs, Next proof, Go binaries, migrations и runtime archive неизменны. Финальный manifest SHA, file count и source delta — `INDEX.md`, `release-digest-verification.json`, `source-changes.json`. QA GO/secrets/dumps, negative fixtures и весь `.local` переносить нельзя. Совпадение hashes — content integrity, не authenticated origin.

|Role|Preserved linux/amd64 runtime ID|
|---|---|
|API/worker/migrator|`sha256:7d76a4d9bc7a51b7cc71f76bde13389fb525a6503a8881480e5d3140c24cadb2`|
|Next/BFF|`sha256:0a15ca0de37a5a0cb14225dd081226022d06cbd01721c7e1745ef43a63b43509`|
|PostgreSQL|`sha256:54451ecb8ab38c24c3ec123f2fd501303a3a1856a5c66e98cecf2460d5e1e9d7`|
|Proxy|`sha256:814a8e88df978ade80e584cc5b333144b9372a8e3c98872d07137dbf3b44d0e4`|
|Mailpit|`sha256:81370195cd4a0eab9604d17c2617a7525b0486f9365555253b6c5376c6350f1a`|

## Confirmed defects и минимальные исправления

`deployment-admission-before.log` — **FAIL27 tests /11 failed subcases**. Это первые два operator/config admission дефекта, не доказанный remote application exploit:

1. Validator принимал alternative API/worker/web entrypoint и command другого service role (6 failures). Current common.py разрешает только защищённый launcher/web guard и canonical api/worker/migrate commands; HTTPS origin path согласован с launcher.
2. Legacy control.py GO проверял action/project/hash, но игнорировал scope/schema и migration approval (5 failures). Теперь нужны private regular GO file, exact scope/database_schema, start migrations_approved:true; QA дополнительно --local-qa и aba-private-qa-* project. Rollback сохраняет schema_compatible gate. Старые неполные GO records больше не допускаются; current examples обновлены.

Дополнительная проверка обнаружила третий пробел: common CLI принимали remote Docker endpoint, хотя coordinator его запрещал. `all-cli-endpoint-before.log` сохраняет два новых FAIL. Теперь общий validator проверяет effective local Unix endpoint до Compose и закрепляет его только для текущего процесса/children; Docker settings не меняются. Actual remote-host case отказал до сетевого обращения.

После всех fixes: FRESH31 Python tests PASS на host и cached Linux/nonroot; actual QA GO без правильного scope отказ2, корректный scope разрешил start/stop. Plain Python operation не создаёт bytecode и не меняет hashed release files. Actual API/worker image отрицательная матрица30/30 PASS:11 live/budget/kill controls отдельно для каждого процесса плюс APP_ENV/provider/transport/HTTPS origin. Полные exits/logs сохранены.

Docker root/operator может напрямую заменить entrypoint или GO и обойти весь pipeline; это доверенная административная граница, а не исправленная гарантия против root. У приложения нет socket/privileges для такой операции. Существующий protected Compose не изменялся ради запуска, unknown cost/DELIVERY_UNKNOWN/financial/ownership/template gates не ослаблены. Severity трёх подтверждённых operator defects MEDIUM; старые host controls BLOCK PRIVATE STAGING независимо от CVE severity. См. `vulnerability-findings.json`.

## Supply chain/security verdict

Полный [supply chain report](RELEASE_SUPPLY_CHAIN_REPORT.md):88 Go modules full graph,271 npm lock entries,136 APK observations, embedded runtime modules, Node/native libraries,27 unique image layers и partial SBOM582 components. Go cache checksums PASS; installed npm tree consistent; native Node/glibc bytes совпали с cached builder;113 bundled Next package.json hashes совпали с installed Next. Это ограниченная local binding, не проверка upstream signatures или всех npm tarball bytes.

**UNVERIFIED PROVENANCE:** authenticated origin/build identity исторических cached base/support images и source revision не доказаны. Frozen hashes/RepoDigests не создают аттестацию задним числом. Полная independent reproducibility Next/OCI не доказана; Go byte equivalence подтверждена прежним executed build.

Current vulnerability review **PARTIAL / Go/npm/OS/native advisory scans NOT RUN**: локальных scanner executables/usable advisory DB нет, дата обновления DB UNKNOWN. Cached govulncheck source не равна installed scanner/DB. `npm audit --offline` exit0/zero demonstrably пропускает advisory query в npm11.19.0; это не PASS. Confirmed CRITICAL/HIGH CVE list **UNASSESSED**, не «уязвимостей нет». Любая найденная confirmed CRITICAL/HIGH блокирует private staging; severity не понижалась. Внешних downloads/tools installs/upgrades не было.

Bounded secret review PARTIAL: image layers/metadata,9 actual synthetic QA values и curated signatures —0 matches; selected166 build/test logs —0 matches. Scope/хеши в `offline-image-secret-scan.json`, `secret-log-review.json`. Универсальный repo/history/entropy scanner отсутствует. Настоящие credentials не создавались, не публиковались и не добавлялись в release.

## Runtime security и automation

FRESH `runtime-hardening-inspect.json`, `verified-actual-process-*.log`, `reports/current-health-final.json`: шесть healthy services,0 host port bindings, internal private network, no socket mounts/privileged containers. API/worker/web/Mailpit UID10001, proxy UID101; RO rootfs/cap_drop ALL/NNP/tmpfs. PostgreSQL writable bootstrap сохраняет default capabilities, затем main processes фактически UID70; это явное исключение, не whole-stack rootless claim. Его5432 публично не опубликован; runtime aba_app restricted, schema16,45 FORCE RLS и anonymous0 проверены health gate.

`deploy.py` существующая последовательность сохранена: local Unix Engine → resources/storage → integrity/images/config → secret/TLS files → owner-scoped install/update → pre-update backup → PG healthy → migration success → runtime readiness → DB/RLS/role/HTTPS smoke → private JSON report. Unit tests проверяют order и fail-before-mutation paths. Actual preflight/start/migrate/health/backup/restore проверены в Stage12C; full coordinator install/update/rollback lifecycle и ACL/data equality Stage12B переиспользованы для неизменённых механизмов. Low-level control.py не заменяет host provisioning/native preflight.

Первый immediate health после manual start дал **BLOCKED42 runtime_health**: legacy start не ждёт final readiness. Исходные report/log и harness assertion сохранены. После explicit --wait native контейнеров FRESH health PASS; `initial-readiness-snapshot.log` показывает состояние перед wait. Этот результат не переобъявлен application defect и не маскируется status rewrite. State stop всегда graceful, DB volumes/evidence сохранены.

## Verification ledger

Пути evidence в таблице относительны `.local/stage12c-20261111/`, если не указан Stage12B. REUSED EVIDENCE означает проверенный сохранённый PASS11 ноября, а не новый прогон12 ноября. Go/SQL/frontend/domain source не менялись; повтор дорогих unchanged suites не добавлял доказательств.

|Gate|Status / freshness|Evidence|
|---|---|---|
|Baseline9 files/links/previous hashes; OCI archive/files/platform|FRESH PASS|baseline.json, release-digest-verification.json|
|Go module integrity/full graph|FRESH PASS|go-mod-verify.log, go-module-tree.log|
|npm installed tree/native binding|FRESH PASS within stated scope|npm-installed-tree.log, runtime-native-provenance.json|
|Go tests/race/vet/build|REUSED EVIDENCE PASS|../stage12b-20261111/go.*, race.*, binary-equivalence.json|
|Full PostgreSQL/RLS/channel/finance/catalog concurrency integration|REUSED EVIDENCE PASS73.000s;3 subprocess-only helper SKIP, parent cases executed|../stage12b-20261111/integration.*|
|Frontend lint/typecheck/OpenAPI/fresh Next build|REUSED EVIDENCE PASS|../stage12b-20261111/web-check.*, web-build.*|
|Browser9/9; accumulated Catalog2/2|REUSED EVIDENCE PASS|../stage12b-20261111/browser-full.*, catalog-accum-1/2.*|
|Strict SQL75/75,4workers/pool5, unchanged1s SQL deadline|REUSED EVIDENCE PASS; no new stress|../stage12b-20261111/sql-regression-final.*|
|Deployment Python/Node tests|FRESH PASS31 Linux +31 host /9 Node|all-cli-endpoint-after-linux.*, all-cli-endpoint-after-host.*, deployment-node.*|
|Runtime flags actual omissions|FRESH PASS30 refused starts|runtime-negative-matrix.json, runtime-checks.log|
|Protected-file mutation/E01 pollution|FRESH expected refusal1/42; clean image Sharp PASS|protected-file-tamper-refusal.*, fresh-original-tainted-image-refusal.*, fresh-native-sharp.*|
|Plain Python immutable operations/scoped GO/local endpoint|FRESH PASS; cross-scope expected refusal2, remote-host refusal1|verified-actual-control-immutability.json, verified-actual-*.log, all-cli-current-local-validator.*, actual-remote-host-refusal.*|
|Fresh migrations/import/onboarding/backup/separate restore/HTTPS|FRESH PASS;6007 variants/238 imports|capacity-*.log, capacity-outcome.json|
|Full restored data digests/effective ACL/policies/definers; safe rollback/roll-forward|REUSED EVIDENCE PASS|../stage12b-20261111/restore-persistence.json, restore-final-security-equivalence.json, rollback-immutable-summary.json|
|Resource experiment|FRESH PARTIAL; workload PASS, true peak not measured|resource-measurement-report.json, resource-samples.jsonl|
|Advisories/verified upstream provenance/current CRITICAL-HIGH absence|NOT RUN / OPEN; NOT a security PASS|advisory-database-availability.json, vulnerability-findings.json|
|Real Ubuntu/SSH/firewall/UID/TLS/access/encrypted off-host restore|NOT RUN|VPS not purchased / external authorization absent|

## Capacity и Ubuntu compatibility

Target сохраняется **Ubuntu24.04 LTS amd64,4 vCPU,8 GB RAM,80 GB SSD/NVMe**, local Docker Engine/Compose/Python3.12. Docker Desktop29.9.2/Compose5.6.1, VM12CPU/8.32e9 RAM и containerd store проверены, native Ubuntu NOT RUN. Current archive IDs требуют совместимого image store после load; не заменять manifest IDs ради PASS. Filesystem/volume ownership/TLS/SSH/firewall/bootstrap prerequisites подготовлены, не выполнены на сервере.

Stage12C per-container runtime CPU ceilings суммарно4 cores; VM/global config не менялась. Синтетические6007/238 прошли startup/migration/import/backup/restore. Sampled steady max386.2 MiB и0.188 CPU core; onboarding sampled max397.8 MiB/1.21 cores; PG volume74544 KiB, dump1127760 bytes, image archive245025792 bytes. Physical DB sizes до/после restore могут различаться; counts совпали. Это не full-content equality test текущего run — такой evidence переиспользован из Stage12B.

Один Docker stats вызов дал EOF/CalledProcessError, сохранён в capacity.log; sample labels могут пересекать phase boundaries, короткий migration/backup/restore peak мог быть пропущен. Настоящий peak/OOM envelope, disk pressure stress, dedicated4CPU contention, SLA и native capacity не измерены. Другие baseline проекты работали; negative probes также локальные. Поэтому resource review PARTIAL и требования4/8/80 не снижены. OOM/forced137 проверены отдельно при финализации, не выводятся из sample average.

## Open risks: facts, hypotheses и closure

|Risk|Факты / недоказанное|Current controls / reopening / closure|Impact|
|---|---|---|---|
|E01 writer OPEN|Original standalone `.next 2`/`node_modules 2` и proof/inode/timestamps сохранены. Stage12C refusal42 и hash/stat comparison PASS. Unchanged build/packaging не воспроизвели suffixed paths в сохранённых fresh runs; writer/PID/операция не установлены|Exclusive build dirs, RO source/runtime, package/start/served guards. При новом drift STOP/capture original bytes+UTC, не rebaseline. Closure: causal writer/operation capture, минимальный fix и reproduction/refusal regression; host privileged audit требует отдельного scope|MONITORING REQUIRED approved synthetic private; BLOCKS CUSTOMER PILOT до causal resolution/отдельного risk decision|
|First150 SQL attribution OPEN|Reviewed matrix A/B/A50+50+50 aggregate57014 и server corroboration. Actual failing statements не имеют доказанной полной causal plan attribution|47 historical index hashes verified; known scoped-JIT fix/RLS/tenant/lock guards сохраняются. При57014/503 на bounded workload capture PID/query family/plan/waits/CPU, без bind secrets. Closure: causal incident-equivalent plan/waits, fix/oracle и unchanged deadline regression|MONITORING REQUIRED bounded private; BLOCKS CUSTOMER PILOT до causal/SLA/risk review|
|Two metadata cancellations OPEN|Oracle73/75 имеет2 metadata timeouts; later75 metadata captures без cancel не объясняют прошлые2|Ноябрь3 JIT95 aggregate cancels47/0/48 — отдельное proven reproduction, не attribution metadata/150. Stage12B75/75 не causal closure. Повторное расследование только при новом cancellation/новой проверяемой гипотезе, no endless stress|MONITORING REQUIRED private; BLOCKS CUSTOMER PILOT|
|Unverified provenance / current advisories|No immutable Git revision/authenticated upstream/build origin; no usable current advisory DB|Frozen hashes/inventory/SBOM не заменяют trust review. Closure: approved inputs/acquisition/provenance либо controlled rebuild; актуальный advisory+reachability review, remediation/retest всех blockers|BLOCKS PRIVATE STAGING; PRE-VPS B|
|Native Ubuntu and owner server GO|VPS absent, local-qa не проверяет native UID/filesystem/SSH/firewall/private TLS/image-store behavior|Read-only first boot → approved bootstrap → exact-manifest owner GO → native rehearsal/restore/rollback. --local-qa нельзя переносить на сервер|BLOCKS PRIVATE STAGING|
|Encryption/custody/scheduler/retention/capacity|Plaintext synthetic backups0600/0700; real encrypted off-host restore/scheduler/true peaks NOT RUN|Конкретный proposed plan ниже; owner decisions, tested encryption/recovery/operational alerts и capacity review нужны до real data. Worker liveness не queue progress|MONITORING REQUIRED synthetic private; BLOCKS CUSTOMER PILOT|
|Formal10E/providers/privacy/live controls|Human rehearsal/Security/provider/templates/ownership/pricing/live/budget approvals отсутствуют|Оба канала mock regression preserved; live OFF, budgets0; unknown cost не free, DELIVERY_UNKNOWN не resend|BLOCKS CUSTOMER PILOT|

Новые summary/hash references — `open-risk-evidence-review.json`, `sql-causal-evidence-review.json`. JIT локальный фикс остаётся transaction-scoped, SQL deadline не повышался, PostgreSQL settings/RLS для маскировки не менялись. Подтверждённый CRITICAL/HIGH всегда блокирует staging; Security CONDITIONAL не означает их отсутствие.

## Owner decisions и следующий этап

Подготовлены [provider checklist](VPS_PROVIDER_ACCEPTANCE_CHECKLIST.md), [private security plan](VPS_PRIVATE_STAGING_SECURITY_PLAN.md), [read-only first boot](VPS_FIRST_BOOT_VERIFICATION.md), [supply chain report](RELEASE_SUPPLY_CHAIN_REPORT.md). Все реальные значения в примерах — placeholders, не credentials.

1. До verdict READY TO PURCHASE разрешить конкретный scoped supply-chain review: доверенный scanner/binary/advisory DB и допустимый network/download scope либо предоставить проверяемый offline bundle с датой/подписью; согласовать source/build/base provenance policy. Такое разрешение ещё не дано и downloads не выполнялись.
2. После closure выбрать provider по checklist: Ubuntu24.04 amd64/4vCPU/8GB/80GB, понятные CPU/IO/network/cost/recovery ограничения. Показанные характеристики не обещают production performance.
3. После покупки предоставить IP/domain, operator/key-only SSH и recovery/allowlist plan, отдельный read-only server authorization. Secret values не передавать в чат. Owner выбирает private TLS CA/lifecycle, backup encryption/tool/custody/retention/RPO/RTO, off-host storage и incident operator.
4. Следующий этап Codex: approved supply-chain closure; затем read-only first boot, reviewed offline bootstrap/provisioning и native private rehearsal по отдельному exact-manifest GO. Покупка сама по себе не разрешает deploy или pilot.

В текущем scope выполнено всё доступное без новых инструментов/DB/серверного доступа. Никто не подключался к VPS; sudo, commit/push, public deployment, external downloads/package installs, live/paid API, global Docker edits и удаление working data/evidence не выполнялись. Финальная сохранность/остановка только своих QA и external baseline — `cleanup-final.json`; все source changes — `source-changes.json`.
