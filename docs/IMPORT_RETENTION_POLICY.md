# Import history and retention · 10Б

Расширены существующие catalog_imports/catalog_events, не создана параллельная очередь. Статусы UPLOADED/PREVIEW/APPLIED/CANCELLED сохранены; PREVIEW уже объединяет validation и preview. Добавлены EXPIRED и FAILED. Невалидный workbook возвращает422, но в той же безопасно завершённой операции сохраняются FAILED job/hash/минимальная error summary/audit без исходного содержимого. Повтор request key восстанавливает результат; другой hash409. «VALIDATED/PREVIEWED» не создают второй несовместимый state machine.

| Данные | Политика |
|---|---|
| Raw XLSX | Не сохраняется; bounded parsing в памяти, no filesystem extraction |
| Grid/mapping preview | Default1day, настройка1..30, только после apply TTL. Истёкший job→EXPIRED; version повышается |
| Диагностические строки cancelled/expired/failed | Default7days, настройка1..365, не меньше preview_days. Если preview очищается раньше, diagnostics остаются до этого срока |
| Подробные lines APPLIED | Default90days, настройка30..3650 от applied_at; исторические legacy jobs без доказанного applied_at используют created_at только для retention |
| Минимальный job receipt/summary/hash/actor/time | Сохраняется; filename минимизируется в catalog.xlsx. Дедупликация и история доступны после очистки |
| Catalog source history / audit / financial / security | Не удаляются этим worker; отдельная утверждаемая политика обязательна до любого удаления |

Owner изменяет tenant settings с expected_version. Hold toggles имеют version check и audit. Hold не восстанавливает уже удалённые данные. Временные данные не помещаются под финансовую retention policy.

Worker раз в минуту использует HMAC-SHA256 capability с отдельным domain `aba:catalog:retention-worker:v1`, производную от отдельного optional CATALOG_RETENTION_KEY, при его отсутствии — от прежнего локального server auth key. API автоматически привязывает только hash к выбранному tenant при upload или сохранении policy, с owner authorization и audit CATALOG_CLEANUP_BOUND. Ни proof, ни hash не возвращаются UI. Это внутреннее разрешение на минимизацию import artifacts, не доступ к finance/каналам. Канал не нужен. Для старых магазинов привязка возникает при следующем разрешённом действии, а не от GET.

`catalog_cleanup_orgs(proof)` выдаёт только IDs с проверяемым hash binding (плюс совместимость с прежним integration proof). Для каждого org отдельная transaction с SET LOCAL org/proof; узкие cleanup RLS policies проверяют привязку. Proof не принимается HTTP/BFF; нет global admin scope, RLS не выключается. Ротация теперь поддерживает явную owner-перепривязку с confirmation/version/audit; см. RETENTION_CAPABILITY_ROTATION.md. Прежние upload/save сохраняются; без подходящего scope worker ничего не очищает.

Каждая транзакция блокирует до20 подходящих jobs FOR UPDATE SKIP LOCKED, пропускает held/unexpired активные previews. UPDATE/minimization/version/audit INSERT выполняются одним statement+COMMIT; crash/rollback оставляют прежние артефакты либо полный результат. Повтор ничего не удаляет второй раз; error phase может дать отдельное событие после grid phase. Нет DELETE SQL и никаких filesystem wildcard cleanup. Синтетический тест вводит ошибку между изменением и audit и проверяет rollback; это модель transaction crash, не отдельный OS-kill cleanup worker. Concurrent workers/hold/active/applied/replay/audit проверены PostgreSQL.

Quota100 теперь касается неминимизированных артефактов; минимальная история не блокирует будущие загрузки.20 активных upload/preview сохраняют fail-closed bound. API/центр возвращает receipts страницами по100 с полным tenant total и стабильным created_at/id ordering; raw файл отсутствует всегда.
