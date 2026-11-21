# Stage12C.1 — trusted supply chain verification

12 ноября2026. **C. SECURITY BLOCKED. PRE-VPS NOT READY. PRIVATE STAGING NO-GO. CUSTOMER PILOT NO-GO.**

P1 metadata и P2 exact tool/DB downloads с offline execution явно разрешены владельцем и выполнены. Trivy завершил scans пяти images, Syft создал пять SBOM. Обнаружены affected versions; для API подтверждён вызов уязвимого net/mail parser до проверки длины email. Provenance остаётся UNVERIFIED для5/5 images. Новый trusted release не собран; существующий release, application source/dependencies и security controls не изменены. VPS/deployment/live/paid API не использовались.

Evidence root: [INDEX](../.local/stage12c1-20261112/INDEX.md), [machine findings](../.local/stage12c1-20261112/vulnerability-report.json), [TSV](../.local/stage12c1-20261112/vulnerability-findings.tsv), [provenance matrix](../.local/stage12c1-20261112/provenance-matrix.json), [owner approvals](../.local/stage12c1-20261112/owner-approvals.json). Private evidence links требуют сохранённой `.local` директории.

## Baseline и preservation

Git HEAD **UNBORN**, весь project untracked; исходный revision exit128 сохранён. Baseline проверил292 Stage12C evidence hashes и15 прежних source-change records. Stage12B manifest SHA256 `e4229f1a2aff75e57c67cf73022edf199923e41bff4d242588f4e2c17fb5b715`; текущий Stage12C manifest `0f56784df82e41b1ef4259308db166a5240d3dca022781478bea8aa00c6fa89b`.

Fresh integrity review:2402 protected files,5 target linux/amd64 image graphs и archive245025792 bytes/SHA256 `9b11e16bd6412f427503f81bc172c3fb8ba70d747bfd504696a5468594dd9b6f` PASS. Original release/evidence не переписывались. Target-platform descriptors/config/layers и source-context anchors проверены заново; полная multiarch closure не заявляется, поскольку некоторые noncached platform descriptors отсутствуют в saved archive.

Source delta этапа — два новых review docs, без изменений существующих source files. Baseline432 files и295 build/control inputs сохранены; content root `6b7ff6df6ce31dbc6c88280b838b4f722035ccc0ea58ef44be3172f5c0dafa3e` — content manifest, не Git revision/attestation. Финальные результаты — `source-changes.json`, `final-checks.json`, `evidence-manifest-final.json`.

## Trusted tools и принятый trust

|Tool|Фактически проверено|Ограничение|
|---|---|---|
|Trivy0.76.2|P2 Linux64 binary SHA/size, ELF amd64 и actual version PASS;5 final image scans exit0|Process PASS не означает security PASS; independent publisher trust не завершён|
|Syft1.57.1|P2 Linux amd64 SHA/size/version PASS;5 final inventories, Syft JSON + CycloneDX|Inventory не заменяет vulnerability/reachability analysis|
|govulncheck|Current official source metadata1.9.0; source/deps отсутствуют; execution NOT RUN|Нет symbol/callgraph completeness; cached1.4.0 не подставлялся|
|npm|Host11.12.1/Node24.15.0; existing paths/versions/hashes recorded|Online audit POST/project upload не разрешён, NOT RUN. Offline cached-zero не vulnerability PASS; runtime npm packages проверены Trivy|
|Buildx/BuildKit|Buildx0.38.1 commit0dbe87f36ed472fe36b6796c3f70b4210a731fc5, BuildKit0.34.0; local docker driver|Build history текущего repo:0 records; legacy builds не получают прошлую attestation от наличия Buildx|
|Docker Buildx/Scout plugins|codesign --verify --strict PASS; Docker Inc9BNSXJN65R|Local signed identity не advisory/revocation/publisher-chain certification; Scout backend не запускался|

Initial state без scanners/DB сохранён в `trusted-tools-preflight.json`; актуальный P2 state — `trusted-tools-after-p2.json`, admission — `p2-input-admission.json`, runner — `scanner-runner.json`. На host глобально tools не устанавливались.

P1: **18 HTTPS requests/176482 response bytes** при лимите20/10MiB. Только официальная release/DB/checksum/signature metadata. P2: **3 payload/223947452 bytes**, exact SHA/size PASS:

|Input|Bytes|SHA256|
|---|---:|---|
|[Trivy0.76.2 Linux64](https://github.com/aquasecurity/trivy/releases/tag/v0.76.2)|52714130|`bc0fc726e017b8927a62b2d3531d7a84ee01769daf5c99ed866dced54089fdbf`|
|[Syft1.57.1 linux amd64](https://github.com/anchore/syft/releases/tag/v1.57.1)|31432061|`bf52519eb57f675d47c07d43213cb6dff6449f3d6d1147ff6701ed91f44eef0c`|
|Trivy DB schema2 compressed layer|139801261|`74c86a033c1125f451e369060b126aa0c6b4c678ae9661915c82c345fb8e05d7`|

DB manifest `sha256:69f786ce825ea62439875b067d2ba0aa91208354189e4289e047b986275fbc7e`. Actual extracted DB SHA256 `e79569457479d18baa1ea27f21c5ccab9cf543a211c459ee7bdace2e3c52539f`,1764368384 bytes. **UpdatedAt2026-11-12T07:01:33.828310435Z, NextUpdate2026-11-13T07:01:33.828309984Z**: schema/freshness PASS во время выполнения. Original DownloadedAt year1 не переписывался. Review привязан к этому snapshot; будущая активация требует повторной freshness/advisory проверки.

Checksum lists соответствуют bundle message digests, signature math с embedded public keys PASS; selected binary bytes соответствуют release API/checksum hashes. **Full Fulcio/Rekor roots/inclusion/time verification NOT RUN.** Embedded cert не является trust anchor. Владелец явно ответил «Разрешить P2 с указанным bootstrap trust»: HTTPS+exact-hash admission принят для этих inputs, независимая authenticity не объявляется. Public registry tokens и redirect query credentials не записывались; project data не отправлялись. Ledgers и signer metadata сохранены.

## Image provenance matrix

Digest equality подтверждает content identity. Authenticated source → builder/materials → image chain **0/5**; labels и plain in-toto statements не считаются независимо проверенными подписями.

|Role / unchanged root ID|Fresh binding / source claim|Что не доказано|
|---|---|---|
|API `sha256:7d76a4d9bc7a51b7cc71f76bde13389fb525a6503a8881480e5d3140c24cadb2`|FROM scratch Dockerfile/context;5 binary SHA byte-match image, Go module locks/build command доступны; linux/amd64 CGO0 Go1.26.2|Approved source Git revision, compiler/module/CA publisher trust; signed build provenance отсутствует|
|Web `sha256:0a15ca0de37a5a0cb14225dd081226022d06cbd01721c7e1745ef43a63b43509`|FROM scratch;6 proof/artifact/guard anchors byte-match; Node/glibc materials match cached builder06a9939e…; lock binding|Source revision и authenticated builder/native/npm input chain; signed build provenance отсутствует|
|Postgres `sha256:54451ecb8ab38c24c3ec123f2fd501303a3a1856a5c66e98cecf2460d5e1e9d7`|SLSAv0.2+SPDX2.3(79 packages), subjects bind actual amd64 manifest; source claim docker-library/postgres commit3b6b5fca9ca40c84b77540fc605ea8e8353b13b2,18/alpine3.23|Publisher signature/registry/source/base trust не подтверждены; upstream Dockerfile/context не получен|
|Proxy `sha256:814a8e88df978ade80e584cc5b333144b9372a8e3c98872d07137dbf3b44d0e4`|SLSAv0.2+SPDX2.3(81 packages), bound subjects; nginxinc/docker-nginx claimd21b4f2d90a1abb712a610678872e804267f4815,mainline/alpine|Same independent publisher/build/base gaps; upstream build args/locks не установлены|
|Mailpit `sha256:81370195cd4a0eab9604d17c2617a7525b0486f9365555253b6c5376c6350f1a`|Bound SLSAv0.2, workflow run12425207023/attempt1; claimed Alpine21dc6063…/golang6c5c9590…; embedded vcs7703d099…/v1.21.8,Go1.23.4|Workflow/source/base/signer trust не подтверждены; upstream Dockerfile/context отсутствует|

Exact platform/config/layer digests, source paths, Dockerfile/context/lock hashes, build args, architecture/labels и claims — `provenance-matrix.json`, `oci-descriptor-review.json`, `attestation-predicate-review.json`. Пять statement layers обнаружены в оригинальном archive, новых attestations не создавалось. Subject binding относится к platform manifest, не к root index/config ID.

Scanner inputs отдельно экспортируют exact target manifest/config/layers и map original root → platform digest → export hash: `scan-inputs.json`, `trivy-layout-inputs.json`. Это analysis exports, не новые release images. Первый export harness ошибочно ожидал все architectures; original FAIL сохранён, исправлен только target selection. Первый Trivy tar-input отказал по формату; final OCI-directory input принят. Artifact guards/старые manifests не ослаблялись.

## Current vulnerability review

Runner — cached image `sha256:06a9939e57531807f8d5fd76ce44b53165ffb7d7501d87ab10e285c20b1e971f`, UID10001:10001, network none, no Docker socket, read-only root/tools/inputs/DB, cap-drop ALL, no-new-privileges, CPU/memory/PID limits, temporary writable `/tmp`/report output. Scans sequential, --rm, pull never; app services не запускались. Trivy DB/version/Java/VEX update paths disabled, offline-scan; Syft update checks disabled. Cache origin самого runner не повышен в VERIFIED.

**791 raw package/advisory occurrences,339 unique CVE/GHSA IDs,625 advisory/package/installed/fixed groups.** Повторения по пяти Go binaries и двум OpenSSL packages сохранены. Это version matches, а не791 доказанный exploit. Raw severity неизменна; applicability/blocker имеют отдельные поля.

|Image|CRITICAL|HIGH|MEDIUM|LOW|UNKNOWN|Syft packages|
|---|---:|---:|---:|---:|---:|---:|
|Postgres|2|71|77|66|3|59|
|Proxy|3|37|33|31|0|66|
|Mailpit|3|79|117|41|3|65|
|API/worker/helpers|0|126|82|5|9|36|
|Web|2|1|0|0|0|136|
|Total observations|10|314|309|143|15|362|

Full791-row `vulnerability-report.json` содержит ID/severity/source/package/installed/fixed, runtime reachability, prerequisites, mitigation, YES/NO blocker и raw JSON pointer. UNKNOWN не приравнивается к clean. **283 occurrences остаются staging holds** после текущих exclusions; это не283 доказанных exploitable paths. Неисследованные HIGH/CRITICAL остаются holds до patch/доказательств. Generic unknown prerequisites явно требуют дальнейшего advisory/callpath review.

### Находки, влияющие на решение

|Advisory / raw severity|Installed → fixed|Reachability / prerequisites / mitigation|Blocker|
|---|---|---|---|
|CVE-2026-39820,CVE-2026-42499 / HIGH|API stdlibGo1.26.2 →1.25.10/1.26.3|Auth Email вызывает mail.ParseAddress до len>254; register/resend/login доступны через private auth HTTP. Crafted email parser DoS prerequisite подтверждён source-call path; exploit/PoC не запускался. Patched compiler обязателен; другие DB entries требуют вплоть до1.26.11 (UNKNOWN CVE-2026-97054); official release availability не проверена, compiler version выбирается после approved metadata review|YES|
|GHSA-673q-mjm6-r5xr / CRITICAL|Next16.4.0 →16.4.1|cacheComponents/useCache=false, use-cache directives отсутствуют, текущий RCE path не доказан. Vendor рекомендует immediate update даже без похожих функций; сохранить configuration и выполнить reviewed patch|YES, vendor patch hold|
|CVE-2026-111497 / CRITICAL|Postgres brotli-libs1.2.0-r0 →1.2.1-r0|Crafted Brotli stream должен попасть в decoder; путь от активного PostgreSQL к decoder не установлен. Наличие библиотеки не доказывает remote exploit; нужен patched vendor image/callpath review|YES, conditional|
|CVE-2025-68121 / CRITICAL|gosuGo1.24.6/MailpitGo1.23.4 →1.24.13/1.25.7/1.26.0rc3|TLS resumption после мутации RootCAs/ClientCAs/config callbacks. Feature path не доказан; роль helper сама по себе не полное исключение. Patched helper/vendor build|YES, conditional|
|CVE-2024-56171 / CRITICAL|Proxy libxml2 2.12.7-r0 →2.12.7-r1|XML/XSD identity-constraint validation; reviewed nginx config без schema/XSLT activation, полный upstream callpath не проверен. Patched vendor package/image|YES, conditional|
|CVE-2026-31789 / CRITICAL|Proxy/Mailpit libcrypto3/libssl3 3.3.2-r1/r4 →3.3.7-r0|Advisory требует32-bit size_t; actual libraries ELF64/machine62/amd64. Raw severity сохранена; пересмотреть при смене platform|NO, platform mismatch|
|GHSA-2464-fhr4-w9xw / CRITICAL|Next16.4.0 →15.5.28/16.4.1|Vendor workaround Sharp>=0.35.5 **уже присутствует**; actual Sharp0.35.5 и dangerouslyAllowSVG=false. Retain workaround, patch Next|NO, documented workaround|
|CVE-2026-94482 / HIGH|Next16.4.0 →15.5.28/16.4.1|Deployed node/edge Server Action manifests:0 actions; no use-server directives. Advisory явно исключает приложения без Server Actions|NO, feature absent|
|x/crypto SSH advisory group / HIGH, etc.|Module0.50.0, fixed varies per row|No x/crypto/ssh subpackage в import graph всех5 project binaries. Module presence не означает compiled SSH code; raw matches retained|NO for these absent-package matches|
|CVE-2026-39836;CVE-2026-33811 / raw retained|stdlib versions/fixes per row|Windows-only flaw не применим к Linux; cgo LookupCNAME flaw не применим к project CGO0. Не распространять CGO0 exclusion на чужие helper builds без proof|NO for observed platform/build exclusions|

Vendor advisory bodies/references сохранены в raw scan, включая [Go net/mail GO-2026-4986](https://pkg.go.dev/vuln/GO-2026-4986), [GO-2026-4977](https://pkg.go.dev/vuln/GO-2026-4977), [Next use-cache advisory](https://github.com/vercel/next.js/security/advisories/GHSA-673q-mjm6-r5xr), [Sharp workaround](https://github.com/vercel/next.js/security/advisories/GHSA-2464-fhr4-w9xw), [Server Actions scope](https://github.com/vercel/next.js/security/advisories/GHSA-p3m5-59mp-jv5r). Эти URLs взяты из admitted DB; дополнительные сетевые запросы к ним не выполнялись.

Source exposure: `apps/api/internal/auth/security.go:30`, `service.go:60`, `http/identity.go:118`. Linux/amd64 CGO0 package graphs построены offline с GOPROXY=off/GOVCS=*:off для5 commands; это подтверждает package absence, не полную symbol reachability. x/text norm/precis присутствуют, но конкретные vulnerable function paths требуют review; они не объявлены false positive. Полные условия/доказательства: `runtime-applicability-evidence.json` и `linux-go-package-graph-*.log`.

### Coverage и честные ограничения

- Go modules/embedded stdlib всех runtime binaries и OS/npm packages проверены Trivy. govulncheck symbol scan NOT RUN:1.9.0 source и10 pinned dependencies отсутствуют; P3 metadata/download domains не разрешены. Go package graphs — дополнительное локальное доказательство, не замена govulncheck.
- Syft обнаружил runtime Node24.21.0/PostgreSQL18.3 как generic native binaries. Trivy не обеспечивает для них полного generic-CPE advisory coverage. Их отсутствие в findings **не clean result**; independent native/vendor advisory review остаётся gate.
- Supplemental Trivy scans actual Syft CDX web/postgres выполнены; web подтверждает те же3 findings, postgres third-party SBOM даёт меньший набор, не заменяет исходные219 image matches. Native query copied web libc6 2.39-0ubuntu8.9/libgcc-s1/libstdc++6 14.2.0-4ubuntu2~24.04.1 дал0 matches только для **inferred Ubuntu source-package mapping**. Это не runtime-rootfs OS scan и не полная оценка Node/glibc.
- Python third-party requirement/lock manifests в source set не найдены; release control scripts используют stdlib. Полный advisory scan host Python interpreter/macOS и всех build-environment dependencies не выполнен; отсутствие Python package findings не даёт clean host/toolchain claim. npm audit, SAST/secret scanner не запускались и не включаются в PASS.

## Security regression и сохранённые FAIL

Routing — `docs/AI_DEV_SYSTEM_INSTALL_PLAN.md`, `regression-evidence-routing.json`. Runtime/business/build-control source и images неизменны, поэтому новые DB/RLS/browser stacks не запускались ради docs/scans.

|Gate|Точный результат|
|---|---|
|Release manifest/archive/2402 protected files/5 target graphs,5 Go+6 web bindings|**FRESH PASS**|
|Current packaged control.py validate/local Unix endpoint|**FRESH PASS**; services не стартовали|
|P2 downloads exact SHA/size, DB schema/freshness, runner/tool versions|**FRESH PASS** under accepted bootstrap trust|
|5 final Trivy image scans,5 final Syft inventories; supplemental queries;5 Go package graphs|**FRESH PASS execution**, security verdict **FAIL/BLOCKED**|
|11 controls each API/worker,30 actual startup refusals; GO/entrypoint/cross-scope/remote fail-closed; integrity guards|**REUSED EVIDENCE** Stage12C:31 Python host/Linux,9 Node, actual refusals; source/evidence hashes verified|
|Go tests/race/vet/build, migrations/FORCE RLS45 tables/tenant/concurrency, frontend lint/TS/fresh build/Browser9/9/catalog2/2, backup/restore6007 variants/238 imports/ACL/digests, hardened rollback|**REUSED EVIDENCE** Stage12B/C; не новые runs|
|Strict SQL75/75 within1s|**REUSED EVIDENCE**; не закрывает historical causal gap|
|New source commit/trusted rebuild/signing, full Sigstore chain, govulncheck/npm online audit/native engine advisory completeness|**NOT RUN / BLOCKED** по missing approved inputs/trust; не PASS|
|Old E01 writer и150 SQL/two metadata cancellation attribution|**OPEN**, новых causal evidence нет|

Original FAIL evidence сохранены: tools-preflight Python NameError, missing noncached OCI platforms в первом export harness, Trivy tar format refusal, Git revision128. Исправления затронули только private analysis harness/input format; прежние controls не менялись. Exit0 scanners не скрывает findings. Scope/command/full log/exit находятся в INDEX и machine artifacts.

## Remediation и owner actions

Минимальная последовательность — [trusted rebuild plan](TRUSTED_RELEASE_REBUILD_PLAN.md): approved source revision и admitted fixed inputs, targeted Go/Next/auxiliary image remediation, controlled new build с SBOM/provenance, independent verification, affected full regressions. Для двух net/mail flaws minimum same-branch fix1.26.3;1.26.10 не закрывает все DB entries: UNKNOWN CVE-2026-97054 содержит fix claim1.26.11/1.27.4. Доступность этих Go releases отдельно NOT VERIFIED. Next16.4.1 — patch candidate из vendor advisory. Все эти версии **не скачаны/не authenticated replacements**. Не делать массовый dependency update или retroactive attestations.

`owner-approvals.json`: P1 и P2 **APPROVED AND EXECUTED**, повторное разрешение не требуется. P3 (current govulncheck source/deps/Go DB), P4 (independent upstream registry/workflow/trust), P5 (owner source revision/materials/new trusted build policy), P6 (targeted patched-input acquisition) **NOT AUTHORIZED**. Их точные unknown sizes/digests не выдуманы; сначала separate metadata proposal. Git commit/push/signing/server/live approvals не вытекают из P2.

**C** выбран из-за неустранённого affected Go parser с подтверждённым source entrypoint и других unresolved security holds; успешное выполнение scans не даёт GO. E01/SQL, native Ubuntu/SSH/firewall/TLS/backup custody, human rehearsal и formal security/privacy/provider/customer/live approvals остаются отдельными gates. Budgets0/live off сохранены; sudo, global installs/settings, app deployment, image/volume pruning не выполнялись.
