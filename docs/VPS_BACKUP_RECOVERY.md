# Backup and recovery

10 ноября 2026. Synthetic QA разрешён. Реальные данные требуют отдельно согласованных encryption/storage/key custody/access решений. Dump не считается доказательством восстановления. Retention audit/receipts и backup deletion не меняются.

Один проверяемый backup:

```sh
python3 /srv/aba/current/scripts/deployment/backup_job.py --release /srv/aba/current --env /srv/aba/staging/environment --project aba-private-staging --backup-dir /srv/aba/staging/backups
```

Имя UTC+UUID уникально, existing files не перезаписываются. Directory0700, archive0600. `pg_dump` custom format сохраняет platform schema/data/owners/ACL/policies без global role/password dump. После dump проверяется `pg_restore --list`, затем streaming SHA256; fail оставляет partial archive без ложного PASS. Update coordinator делает такой backup перед миграцией. TOC/checksum не заменяют restore.

Для расписания подготовлены `infrastructure/deployment/aba-backup.service.example` и `aba-backup.timer.example`, ежедневно02:30 UTC. Их установка/enable — отдельное host change approval; в этой работе units не устанавливаются. Unit требует заранее согласованного доверенного Docker operator. Никакой автоматической чистки; alert на nonzero job status, нехватку диска и отсутствие нового успешного archive. Факт наличия timer не считается успешным backup.

Восстановление только в **новый** isolated project `aba-private-restore-UNIQUE`, с собственными secrets/env/volume/internal subnet. Запустить только PostgreSQL bootstrap после разрешения; API/worker должны быть остановлены. Затем:

```sh
python3 /srv/aba/current/scripts/deployment/backup.py restore-rehearsal --release /srv/aba/current --env /srv/aba/restore/environment --project aba-private-restore-UNIQUE --archive /srv/aba/staging/backups/CHOSEN.dump
```

Helper проверяет checksum/private file, пустую platform schema и единственный running PostgreSQL; непустой target отвергается. Restore single transaction, без --clean/truncate/старого volume. Local PG admin нужен для COPY через FORCE RLS; приложение не получает admin DSN. Failure сохраняет archive/target/temp evidence.

После restore сверить schema16,45 FORCE RLS, effective owners/ACL/policies/security-definer settings, synthetic counts, foreign/anonymous0 и собственный tenant; затем runtime readiness/API/Browser gates. Ключи auth/retention должны соответствовать восстановленным данным и предоставляться отдельно. Не менять DB ради побайтового совпадения default/explicit ACL. Не переключать рабочий DSN автоматически. Disaster recovery включает согласованное решение по DNS/private access/keys и отдельный GO; данный helper не разрешает production restore.

Evidence фактической новой репетиции и её пределы перечислены в VPS_DEPLOYMENT_READINESS.md. При отсутствии restore PASS ставить NOT RUN/FAIL, а не делать вывод из одного dump.
