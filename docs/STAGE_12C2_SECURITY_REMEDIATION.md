# Stage 12C.2 — Security remediation

Reviewed 2026-11-16. **C. SECURITY BLOCKED. PRE-VPS NOT READY; PRIVATE STAGING NO-GO; CUSTOMER PILOT NO-GO.**

No trusted new release exists. Source remediation and historical patched binary verification do not clear unchanged deployed artifacts. This review found pre-existing unfinished Stage12C.2 work and preserved it. No application-code edits were needed during this resumption. No deployment, downloads, live API, budget increase or production volume operations were performed.

## Confirmed vulnerability and source fix

CVE-2026-39820 / GO-2026-4986: quadratic concatenation in net/mail consumeComment; CVE-2026-42499 / GO-2026-4977: quadratic concatenation in consumePhrase, allowing CPU/memory denial of service. Saved official advisory responses identify fixed versions 1.25.10 and 1.26.3. Selected admitted compiler is Go1.26.11, accounting for later stdlib findings. These are historical official metadata observations, not fresh internet verification.

Path: POST /api/v1/auth/{register,resend-verification,login} → identityRoutes decode/publicLimit → auth.Service.Register/Resend/Login → auth.Email → mail.ParseAddress. Rate limits run before service validation. Shared middleware checks Content-Length and uses MaxBytesReader for streamed bodies; default MAX_BODY_BYTES=1048576, allowed configuration range 1..10485760. No new body-limit contract introduced.

Current apps/api/internal/auth/security.go trims outer spaces, rejects empty and >254-byte normalized strings, rejects non-ASCII, then lowercases and invokes ParseAddress. This closes the previously unbounded parser input; TrimSpace still processes the body-bounded original input. Short ASCII still reaches net/mail; length admission alone is not a compiler CVE fix. The only direct project net/mail import/call is this helper. Worker shares the auth package; no additional direct parser was found. No attacker input added to logs.

Pre-existing remediation files: apps/api/internal/auth/security.go; apps/api/internal/auth/email_security_test.go; apps/api/internal/http/email_security_test.go; apps/api/go.mod; scripts/build-env/check-go.sh; scripts/deployment/package.py. The latter three differ from the preserved compatibility snapshot: they now require Go1.26.11 and reject old/mixed compiler metadata for all five Go binaries. go.sum is unchanged. Existing generic Docker builder recipes are not approved release inputs merely because go.mod changed.

Tests cover normal/normalized/empty/malformed emails, ASCII/Unicode/invalid UTF8, 253/254/255-byte boundaries, long punctuation, repeated concurrent rejection, panic detection, stable errors, service rejection before DB, known/streamed HTTP body refusal and input non-disclosure. Existing coverage evidence shows parser execution count zero for admission failures and positive for accepted parser inputs.

## Verification and provenance limits

Full commands/logs/exits: ../.local/stage12c2-20261112/. Historical patched runs refer to source_revision sha256:9d1ac2c43d3ec0e79fc1742c5f3731f79fcb2ccf00fb649a28186ef501431a7e, not current post-policy files or a Git commit.

|Gate|Result|Scope|
|---|---|---|
|Go unit, race, vet, build, targeted coverage|Historical PASS|patched/*.exit and logs; Go1.26.11 snapshot|
|Isolated migration/RLS/tenant/Excel/concurrency|Historical PASS|patched/go-integration.log; helper-only SKIP retained|
|30 startup refusals, flags, 2 positive controls|Historical PASS|patched/runtime-negative-matrix.json; bind-mounted binaries, not final images|
|Source and five old image Trivy scans|Historical process PASS / security FAIL|sca-summary.json; old image findings unchanged|
|Patched binary rootfs/SBOM scan|Historical process PASS|patched/remediation-sca.json: zero stdlib/netmail matches, 42 other dependency observations|
|Syft|Historical PASS|patched/scanner-output and Stage12C1 five inventories|
|Current deployment Python suite|Fresh PASS|35 tests, review-20261116/deployment.log|
|Current Go unit/targeted/race/vet/build|Fresh BLOCKED|Docker socket missing; every exit1/log preserved in review-20261116|
|govulncheck|NOT RUN|Executable unavailable; no tool download authorized|
|New five-image build/SBOM/scan/integrity|NOT RUN|Missing D2 inputs; local Docker unavailable|
|Post-rebuild GO/remote endpoint/migrations/FORCE RLS/Excel|NOT RUN|No new release; old Python control tests do not certify new images|
|Post-rebuild Browser 9/9, Catalog 2/2, SQL|NOT RUN|No new release|
|Post-rebuild backup/restore, rollback/roll-forward, HTTPS smoke|NOT RUN|No new release|

Trivy DB UpdatedAt 2026-11-12, NextUpdate 2026-11-13 is STALE at this review. Zero matches against it cannot establish current clearance. Initial zero-file fs scan was invalid coverage and remains preserved; supplemental rootfs/SBOM evidence is the relevant historical result. Unit success is not absence-of-CVE proof. Previous failures, including runtime SBOM and DB wrapper failures, remain intact.

Fresh preservation check rehashed all 299 Stage12C1 sealed files: PASS. See review-20261116/summary.json. Existing OCI archives were not overwritten. E01 and SQL cancellation attribution remain OPEN. Other dependency HIGH/CRITICAL and origin gaps remain blocking.

## Required owner actions

D1, M1–M3 approvals exist as local historical records; they are not D2 approval. Exact pending request: approve D2_RELEASE_INPUTS_APPROVAL.md and d2-download-proposal.json (254808036 compressed payload bytes; ceiling 280MiB, 140 requests, specified official hosts, exact hashes/SRI/H1, offline remediation/build only). This includes Next16.4.1, Node24.22.3, x/text0.41.0, x/sync0.22.0 and three pinned vendor images. It does not authorize deployment or establish independent publisher authentication.

Separately authorize fresh official advisory/Trivy DB descriptor metadata discovery (up to20 HTTPS GET/HEAD requests, 2MiB, ghcr.io and aquasecurity/trivy-db official GitHub namespace), to prepare a new exact digest/size payload request; do not reuse old DB freshness. DB payload download remains pending a separate exact approval. Restore the intended local Docker daemon for isolated runners; no remote Docker/VPS fallback.

Git publication is separately authorized by the later owner instruction. Publication cannot change this security verdict. Source snapshot is content identity, not independent attestation. New release provenance must bind the final published source revision and all actually admitted materials.

## Publication preparation (2026-11-16)

Owner explicitly requested commit/push of the project. Configured destination is the single monorepo https://github.com/karadjev/AI-Business-Assistant.git; ls-remote --heads succeeded with no branch refs. No additional repository inferred or created. Initial local HEAD remains unborn until an actual commit is made.

Fresh heuristic credential-pattern scan covered 460 candidate files before exclusion of .srl, found zero high-confidence token/private-key patterns and zero files >10MiB; this is not comprehensive secret-scanner clearance. .local, env secrets, dependencies/builds, private keys and certificate serial files are excluded by .gitignore. Private evidence therefore is not part of the GitHub artifact and must be retained locally.

Publication changes: .gitignore excludes *.srl; .github/workflows/ci.yml uses workflow_dispatch only to prevent unapproved networked CI on push/PR, and requests Go1.26.11 consistently with go.mod. CI NOT RUN. These changes do not grant build provenance or release clearance. Commit author name/email are missing from local Git configuration and requested from owner; global Git settings untouched.
