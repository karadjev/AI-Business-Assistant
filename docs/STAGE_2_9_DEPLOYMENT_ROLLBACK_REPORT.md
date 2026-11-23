# Stage 2.9 — deployment и rollback

- PASS: PostgreSQL 18.7, schema 29, 62 FORCE RLS, действующий volume `aba-private-staging_database`.
- PASS: `aba-backup.timer` включён, расписание 02:30 UTC, последний `ExecMainStatus=0`.
- PASS: `stage272-20261122T175412Z.dump` совпал с sidecar. SHA-256 `8713742c1b71434f24ab4619ec7096b38d8ff288519aa990e31b1079dbe55ec9`. Команда `docker exec -i aba-private-staging-postgres-1 pg_restore --list < archive` прочитала оглавление, 1375 строк. Предыдущие вызовы, передававшие имя файла вместо чтения stdin, завершились ошибкой.
- Исторический PASS: restore этого архива в отдельный volume и сравнение отпечатков описаны в `STAGE_2_7_2_BACKUP_RESTORE_REPORT.md`.
- BLOCKED: рабочий стек не соответствует одному manifest. Новый rollback point, резервная копия непосредственно перед обновлением и rehearsal с миграциями 00030/00031 ещё требуются. Down 00031 отказывается работать при активных reset токенах. План отката должен учитывать совместимость приложения со схемой и одобренное восстановление при необходимости, без потери более новых записей.
- NOT RUN: deployment, миграции и rollback в Stage 2.9.

## Current rollback point · 23 November 2026

The prior public image set from `30f5343` is intact and compatible with schema 31. All prior image IDs were verified on Play2Go and the prior web image served login in a separate networkless container. The current fresh archive was restored to an isolated volume with matching fingerprints. The live volume remained mounted and no live rollback was needed. Use the saved prior record and the recorded active-volume override for image-only rollback.

[Current runbook](VPS_DEPLOYMENT_RUNBOOK.md) [Current manifest and recovery identifiers](POST_LAUNCH_RELEASE_MANIFEST.json)
