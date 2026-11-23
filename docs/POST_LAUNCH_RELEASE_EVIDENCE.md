# Post-launch release evidence

The approved manifest bytes are retained in the owner-approved package and on Play2Go. The checked deployment fields are recorded in [the deployment receipt](POST_LAUNCH_RELEASE_MANIFEST.json). Independent D2 was not performed. The owner explicitly accepted this risk for the exact candidate.

| Evidence | Location |
|---|---|
| Exact build, original manifest, new web OCI archive, SBOM and hash checks | Directory identified by `.local/post-launch/release-7a84b6a-verified-path.txt`. On Play2Go, the expanded manifest is `/srv/aba/promotion-7a84b6a.hoyob3xn/package/manifest.json`. |
| Promotion, monitor, startup and daily backup results | Directory identified by `.local/post-launch/promotion-path.txt`. Files `preflight.log`, `promote.log`, `final-vps.log`, `startup-timers.log`, `backup-timer.log` and `worker-restart.log`. |
| Backup TOC, snapshot fingerprints, old record, edge and override | Private VPS directory `/srv/aba/staging/reports/promotion-7a84b6a.ansovi78`. |
| Public Chromium regression and required-part workflow | `public-promotion.log`, `public-auto.log` and `public-queue.log` in the local promotion directory. Earlier failed attempts are retained there. |
| Prior web image startup and tightened edge checks | `rollback-web.log` and `approval-edge.log` in the local promotion directory. |
