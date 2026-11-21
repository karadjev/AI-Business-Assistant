# Trusted release rebuild plan

12 ноября2026, Stage12C.1. **PLAN / NOT EXECUTED.** Старые releases не заменены, commit/push/deployment не выполнялись. P1 metadata и P2 exact scanner/DB downloads с offline execution явно разрешены и выполнены. Они не разрешают новые application inputs, source commit или server GO. Текущий результат — **C. SECURITY BLOCKED**; актуальные findings/разрешения — в [verification report](STAGE_12C1_SUPPLY_CHAIN_VERIFICATION.md) и `.local/stage12c1-20261112/owner-approvals.json`.

## Что нельзя считать trusted rebuild

Повторная сборка из тех же unverified caches не восстанавливает неизвестное прошлое происхождение. Совпадение image ID, unsigned labels, собственная подпись или созданный сегодня JSON не являются независимой аттестацией старого image. API/web source HEAD отсутствует; upstream source revisions в OCI provenance — пока только claims. Сначала принимаются trust roots/inputs и фиксируется source revision, затем происходит новая executed build и проверка.

Нынешние package.py/Compose/runtime contracts сохраняются. Для будущего release нужен отдельный reviewed provenance-capable build recipe, а не скрытая замена legacy builder или автоматический update dependencies. Actual build records показывают `DOCKER_BUILDKIT=0` в прежних API/web builds. Новые Buildx attestations могут описывать только будущую реальную сборку.

## Gate T1 — source и input admission

1. Владелец review-ит exact source set и отдельно разрешает/создаёт immutable Git commit. Current HEAD UNBORN; агент не делает git add/commit/push по этому плану. `.local/stage12c1-20261112/source-input-manifest.json` содержит content hashes/bytes build inputs и reviewable content root. Это заготовка для review, не замена требуемого Git revision.
2. Проверить отсутствие real secrets в будущем tracked tree, исключить QA `.local`, dumps/env/credentials, node_modules и build outputs. Создать read-only build context через `git archive EXACT_APPROVED_COMMIT`; не копировать `.git/config`, tokens или working-tree незакоммиченные изменения. Отдельно сохранить commit/tree IDs и hash archive. Мутация source после admission — STOP.
3. Go: go.mod/go.sum из approved commit; verify cached modules against accepted Go checksum trust. npm: package-lock exact versions/SRI; проверка tarball hashes и platform/native dependencies. `npm ls`/совпадение versions не доказывают registry tarball bytes. Network-off install/build должен отказать при недостающем cache; никаких implicit downloads.
4. Tool/base/image acquisition manifest должен содержать exact official URL/repository, immutable digest, size/hash, expected signer/issuer/trust policy и approval. `latest`, mutable tag2 и developer tags — только discovery metadata, не build inputs. Отдельно review-ить tools и builder dependencies; signed Docker Buildx binary не доказывает provenance builder image.

P2 выполнил workspace-only admission Trivy0.76.2/Syft1.57.1 и pinned schema2 DB: три payload,223947452 bytes, exact SHA/size PASS. Пять offline image scans и пять SBOM выполнены в cached nonroot runner без сети/socket; full independent Sigstore/Fulcio/Rekor trust по-прежнему не подтверждён, owner явно принял HTTPS+exact-hash bootstrap. govulncheck1.9.0 source/deps отсутствуют: `.local/stage12c1-20261112/govulncheck-acquisition-requirements.json` перечисляет exact versions и дополнительные metadata domains; его build отдельно NOT RUN. Не подменять current tool cached1.4.0 молча и не менять application go.mod ради scanner.

### Gate T1a — targeted security remediation inputs

Текущий candidate не пересобирать из прежних vulnerable inputs ради нового provenance JSON. Сначала подготовить точные acquisition proposals и разрешить missing inputs:

- Go compiler: current1.26.2 содержит net/mail CVE-2026-39820/CVE-2026-42499; source вызывает ParseAddress до ограничения длины. Их fixes начинаются с1.26.3, но другие admitted DB entries содержат fixes вплоть до1.26.11 (UNKNOWN CVE-2026-97054).1.26.10 не закрывает весь список. Доступность patched releases и выбранная версия должны пройти отдельную official release metadata/checksum/signature проверку до admission всех пяти новых Go binaries. Pre-parse length admission — дополнительная защита, не замена patched compiler.
- Next16.4.0 → vendor-fixed16.4.1 как минимальный patch candidate, с lock/native/fresh-build/Browser review. Sharp0.35.5 workaround уже присутствует; отсутствие use-cache функций не отменяет vendor update recommendation. Не выполнять массовое обновление npm.
- PostgreSQL/proxy/Mailpit: выбрать exact vendor-fixed image/material versions по current OS/helper findings и доказать publisher/build chain. Fixed package numbers в vulnerability-report.json — не существующий authenticated replacement image digest. Native PostgreSQL/Node engine coverage должна быть отдельно дополнена до принятия candidate.
- x/crypto SSH module matches исключены только для текущих binaries по отсутствию SSH packages в compiled import graph. Не обновлять module только ради этих false positives; отдельно решить достижимые/неисследованные x/text и stdlib findings. Все raw severity остаются в отчёте.

Новые version/digest/size/hash пока не получены и не разрешены. P6 в owner-approvals.json фиксирует scope следующего remediation review; это не разрешение на download, rebuild или deployment. Для source-only defense-in-depth изменения отдельно применить auth regression route; новая серия binaries всё равно требует полного применимого Gate T5.

## Gate T2 — пять image roots и материалы

|Role|Проверенные content anchors|Что требуется до нового trusted candidate|
|---|---|---|
|API/worker/migrator|Final FROM scratch;5 binaries byte-match saved Go context. Compiler material cached Go1.26.2 ID в matrix; CA bundle — отдельный material|Approved source commit/Go compiler acquisition/module inputs/CA source. Static CGO_ENABLED=0,trimpath и прежние role/entrypoint contracts; реальный build record каждого binary|
|Next/BFF|Final FROM scratch; Node/glibc copied from cached Playwright image;6 source/artifact anchors match image,113 bundled package metadata bindings reused|Authenticate approved Node/glibc builder digest, exact Node version/native ABI/lockfile inputs; exclusive fresh Next build, native Sharp probe и unchanged artifact/served proof guards|
|PostgreSQL|OCI SLSA claim docker-library/postgres commit3b6b5fca9ca40c84b77540fc605ea8e8353b13b2,18/alpine3.23; subject bound; SPDX79 packages|Independently authenticate official publisher/registry/referrers and claimed source/build/base chain, or build a reviewed replacement from approved exact upstream source/materials|
|nginx proxy|OCI SLSA claim nginxinc/docker-nginx commitd21b4f2d90a1abb712a610678872e804267f4815,mainline/alpine; subject bound; SPDX81 packages|Same upstream authentication/advisory gate; retain private TLS/nonroot/RO configuration. Never treat old version age alone as CVE proof or private access as severity downgrade|
|Mailpit|OCI SLSA claim GitHub Actions run12425207023/attempt1; claimed Alpine/golang base digests; embedded vcs7703d099…/v1.21.8|Verify workflow/release identity/source/base digest against approved independent publisher evidence; otherwise admit a new reviewed exact version/rebuild. Synthetic-only local SMTP retained|

Полная machine matrix с IDs/config digests/labels/build args/material claims/context hashes — `.local/stage12c1-20261112/provenance-matrix.json`. Claimed materials не повышаются в VERIFIED без signature/identity policy и независимой проверки. Для официальных auxiliary images допустима approved upstream acquisition вместо самостоятельной пересборки всех OS packages, если chain достаточна и reviewed; это отдельное explicit acceptance, не автоматическое trust по tag.

## Gate T3 — controlled execution и SBOM

Будущий build использует только утверждённые cached inputs, linux/amd64, local Unix Engine и зафиксированные Buildx/BuildKit versions. Current local Buildx0.38.1/BuildKit0.34.0 доступны, но это не completed build. Не создавать новый builder, не bootstrap/pull его image автоматически.

Recipe должен явно pin FROM/builder digests, context hashes, network none, pull false, approved build args, compiler/node/npm versions и locale/time/source-date settings. Никаких credentials в ARG/ENV/logs/provenance. Не передавать весь host environment; allowlist-only metadata. Runtime budgets/live flags остаются0/off и не нужны для compiler access.

Для каждого будущего image отдельно сохранять:

- Actual source commit/tree/context digest и all materials (включая compiler, base, native libs, lockfiles).
- Exact executed command/approved args, tool identities, UTC start/end/status и output platform manifest/config/root index digests.
- SBOM, generated approved offline tool from final runtime filesystem plus Go/native metadata. Scope/limitations явны: scratch не значит dependency-free; vendored Next и copied libs должны попасть в inventory.
- Actual build provenance (например Buildx `--provenance=mode=max` в поддержанном reviewed exporter) с digest subjects, materials и builder identity; подпись только после реальной сборки разрешённым signer.

**Не включать Buildx --sbom автоматически:** default SBOM generator может pull новый scanner image. Нужен заранее approved immutable generator либо отдельный admitted Syft offline export. Нельзя включать push/registry export ради сохранения attestations без authorization. OCI archive должен сохранять attestations и signatures в поддержанном формате; classic image-store/load behavior не считать достаточным без проверки.

Reproducibility означает frozen inputs/recipe и независимое повторение с измерением совпадения. Bit-identical Next/OCI bytes пока NOT PROVEN: Next BUILD_ID и build timestamps могут различаться. Если требуется bit reproducibility, отдельно review-ить deterministic build-ID/timestamp policy, выполнить два isolated builds и сравнить допустимые outputs; не переписывать proof или добавлять guard exclusions. Даже идентичные bytes без accepted origin не дают trusted status.

## Gate T4 — independent verification

Второй reviewer/independently admitted verifier получает source/material approvals, build record, SBOM, signature bundles и OCI archive. Он заново проверяет:

1. Content hashes/size/platform/descriptor graph, image subject ↔ config ↔ layers ↔ saved archive ↔ release manifest.
2. Signer identity/issuer, trust roots, transparency/integrated-time evidence и policy. Embedded certificate/public key не является собственным trust anchor; собственная подпись не называется независимой attestation.
3. Provenance materials/source revision соответствуют реально выполненной recipe; no unapproved fallback/remote builder/undeclared download.
4. Dependency SBOM охватывает OS/Go/npm/native/Python-host scope; current admitted advisory snapshot/schema/freshness зафиксированы. Отсутствие CVE при stale/missing DB не PASS.

Сохранённые multiarch root indexes могут ссылаться на noncached architectures, отсутствующие в локальном archive. Проверка текущего target linux/amd64 и bound attestations — отдельный scope. Для scanners prepared single-platform archives в `scan-inputs.json` сохраняют target platform manifest/config/layers и map original root ID; это analysis exports, не новые release IDs. Для нового release graph/export policy должны быть проверены отдельно, без изменения старого manifest ради PASS.

## Gate T5 — regression и activation decision

Выполнять affected-component tests, различая FRESH PASS / REUSED EVIDENCE / NOT RUN. При новых binaries/dependencies обязательно Go tests/race/vet/build, PG/RLS/tenant/concurrency, frontend lint/typecheck/fresh build/Browser/catalog, strict SQL deadline unchanged,30 actual API/worker startup refusals (11 controls ×2 плюс4 required parameters ×2), malicious config/entrypoint/remote endpoint/GO negatives, artifact/served integrity, startup/restart/persistence, отдельный backup/restore/effective ACL и compatible hardened rollback.

Если меняются только review docs/metadata, неизменённые Stage12C/12B gates переиспользуются с проверенными hashes. E01 и150 SQL/two metadata cancellation attribution остаются OPEN без нового causal evidence; trusted rebuild сам по себе их не закрывает. No RLS/authorization/freshness/financial gate weakening, no unknown cost as free, no automatic DELIVERY_UNKNOWN resend.

Только после reviewed results новый candidate получает новый integrity manifest и independently verified digest set. Старые releases/evidence сохраняются; никакого overwrite текущего candidate или public/server deployment. Любой confirmed CRITICAL/HIGH блокирует staging. Successful supply-chain scan не выдаёт Security/server/customer GO: native Ubuntu/SSH/firewall/TLS/backup custody, human rehearsal и exact-manifest owner approvals остаются отдельными gates.
