# Trusted release provenance — Stage12C.2

2026-11-16: **UNVERIFIED; new release NOT BUILT.** This is a local review statement, unsigned and not an independent attestation. No new image digest or successful build is invented.

## Existing release only

|Role|Old root identity (not new output)|New digest|
|---|---|---|
|postgres|`sha256:54451ecb8ab38c24c3ec123f2fd501303a3a1856a5c66e98cecf2460d5e1e9d7`|NOT BUILT|
|proxy|`sha256:814a8e88df978ade80e584cc5b333144b9372a8e3c98872d07137dbf3b44d0e4`|NOT BUILT|
|mailpit|`sha256:81370195cd4a0eab9604d17c2617a7525b0486f9365555253b6c5376c6350f1a`|NOT BUILT|
|api|`sha256:7d76a4d9bc7a51b7cc71f76bde13389fb525a6503a8881480e5d3140c24cadb2`|NOT BUILT|
|web|`sha256:0a15ca0de37a5a0cb14225dd081226022d06cbd01721c7e1745ef43a63b43509`|NOT BUILT|

Exact old platform/config/layer bindings and source claims: .local/stage12c1-20261112/provenance-matrix.json and oci-descriptor-review.json. Old release manifest SHA256 0f56784df82e41b1ef4259308db166a5240d3dca022781478bea8aa00c6fa89b; archive SHA256 9b11e16bd6412f427503f81bc172c3fb8ba70d747bfd504696a5468594dd9b6f. These are historical identities; no new release manifest comparison possible.

## Materials and actual evidence

- Preserved compatibility source snapshot: source-revision.json (content hash; HEAD was UNBORN). Three subsequent producer-policy changes are documented in producer-policy-change.json. Current publication revision must be captured separately before release build.
- D1 admitted Go1.26.11 linux/amd64 archive SHA256 9849f4e04ee35303714e34fbc3ac8ab44ea3e448a95aa182f91e3b9b79c710ec, 66999944 bytes. Origin accepted historically under explicit official HTTPS+exact-hash bootstrap, not independent signature trust. Actual historical compiler version and binary metadata in patched/go-version.log and packager-real-toolchain-admission.json.
- Go module locks remain go.mod/go.sum; npm lock remains apps/web/package-lock.json. Pending patches in D2 are not installed and must not be represented as current dependencies.
- API final recipe infrastructure/deployment/api.Dockerfile uses scratch and five supplied static binaries plus CA bundle. Build-context manifest/compiler/CA origins must be bound before rebuild.
- Web recipe infrastructure/deployment/web.Dockerfile copies Node/glibc from a mutable builder default. New build must bind admitted exact Node/native materials; existing cache identity alone does not prove origin.
- Existing package.py BASES are legacy versions and must not be reused as patched vendor inputs. Pending selected immutable platform descriptors and layer hashes are in d2-download-proposal.json; candidate metadata is not an executed build.
- Vendor PostgreSQL/proxy/Mailpit source/Dockerfile/attestation claims were reviewed under m3-provenance-source-review.json. No authenticated independent chain is asserted. Repackaging bytes would not prove a source rebuild.
- Historical five-image Syft inventories: .local/stage12c1-20261112/scanner-output/{api,web,postgres,proxy,mailpit}.cdx.json and syft-ROLE.json. Historical Trivy findings: same directory trivy-ROLE.json; newer Stage12C2 old-image scans in scanner-output/trivy-current-ROLE.json. These inventories are not new-release SBOMs.
- Historical patched verification binary SBOM/scan under patched/scanner-output; zero stdlib matches against Nov12 DB, remaining dependency findings. Fresh DB/current source scan NOT RUN.

No current executed build record, new OCI archive, five-image digest set, new SBOM or independent attestation exists. All five provenance chains remain release blockers. Offline regression and independent verification must bind actual new output subjects to source/materials/build commands and then compare the new release manifest. No staging/customer GO follows from owner download or Git push approval.
