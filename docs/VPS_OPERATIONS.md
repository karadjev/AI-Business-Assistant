# VPS operations

Stage12C: использовать исправленные host scripts из candidate, указанного в [pre-VPS report](STAGE_12C_PRE_VPS_SECURITY_REPORT.md). GO для control.py теперь также связывает scope/database_schema и хранится private regular file; QA требует --local-qa/aba-private-qa-*. После low-level start/rollback нужен явный readiness wait и deploy.py health. Supply-chain/native-server GO ещё отсутствует.

Stage12B,11 ноября: rollback eligible только при admission v1 и сохранении hardened API/worker. Исторические API images, которые стартовали без обязательных flags, запрещены как rollback targets даже при schema16 compatibility. Проверенная QA rollback-композиция меняет только verified frontend artifact и сохраняет hardened API/worker. Current validator отклоняет legacy policy и `__pycache__`/`.pyc` внутри release scripts, даже если cache включён в hashes. First-party Python CLI запрещают bytecode writes; испорченные copies сохраняются для evidence, а не исправляются rebaseline.

10 ноября 2026. Команды будущего server use требуют отдельного GO; live/public/customer rollout запрещён. Работать из выбранного release. Все commands требуют local unix Engine; remote Docker contexts запрещены coordinator.

Health без изменения приложения:

```sh
python3 scripts/deployment/deploy.py health --release /srv/aba/current --env /srv/aba/staging/environment --project aba-private-staging --storage-root /srv/aba/staging --backup-dir /srv/aba/staging/backups --tls-ca /srv/aba/staging/proxy-ca.pem --report /srv/aba/staging/reports/health-001.json
```

Тот же вызов с `diagnose` собирает только project-scoped `docker stats` CPU/RAM/I/O/PIDs. Сырые process environments, cookies, payloads и application logs не собираются. Worker healthy — liveness, а не доказательство queue progress. Nginx health — config test; HTTPS smoke отдельно проверяет реальное serving. Снимок ресурсов не является capacity benchmark.

Update выбранной новой версии с exact-version GO и проверенным backup:

```sh
python3 scripts/deployment/deploy.py update --release /srv/aba/releases/NEW_RELEASE --env /srv/aba/staging/environment-new --project aba-private-staging --storage-root /srv/aba/staging --backup-dir /srv/aba/staging/backups --tls-ca /srv/aba/staging/proxy-ca.pem --go-file /srv/aba/staging/update-go.json --report /srv/aba/staging/reports/update-001.json
```

Указанный env должен выбирать images новой версии. Не менять running secrets одновременно с DB keys; это отдельная операция. Update требует существующий healthy project/свой DB volume, успешный dump/TOC/checksum и migration approval. Если backup/migration/readiness/smoke fails — STOP, сохранить report, containers/volumes и incomplete archive. Timeout клиента не доказывает остановку миграции в Engine; проверить только свой migration container перед повтором, не запускать конкурентный migrator. Goose session lock сохранён.

Graceful stop и rollback используют существующий control.py и отдельный GO:

```sh
python3 scripts/deployment/control.py stop --release /srv/aba/current --env /srv/aba/staging/environment --project aba-private-staging --go-file /srv/aba/staging/stop-go.json
python3 scripts/deployment/control.py rollback --release /srv/aba/releases/PREVIOUS --env /srv/aba/staging/environment-previous --project aba-private-staging --go-file /srv/aba/staging/rollback-go.json
```

Rollback требует exact previous manifest и `schema_compatible:true`; не запускает migrate down и не восстанавливает DB автоматически. Затем обязательно `deploy.py health` с previous release/env. Реальная Ubuntu rollback rehearsal остаётся NOT RUN до server approval. Нельзя `down -v`, prune, reset, автоматическое удаление evidence или изменять retention.

Процессные logs ограничены10MiB×3 на container. Это не audit/receipts retention. Следить за Docker disk, backup directory, DB growth, volume free space, restarts, health, timeout rate и backup failures; автоматической очистки нет. Для private SSH/TLS доступа следовать VPS_SECURITY.md.
