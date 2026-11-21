# Готовность к10В

10Б развивает существующие каталог/agent/order/outbox/import/worker, без реальных интеграций. Для следующего этапа:

- Утвердить operator workflow ротации retention capability для старых tenant: сейчас новая привязка появляется при следующем owner upload/save, без silent global rebinding.
- Оценить pagination/aggregate SQL для каталогов >5000 вариантов; текущий fail-closed limit показан явно. Истории доступны recent100, экспорт/поиск за большим периодом — отдельный scope.
- Расширять source/category freshness overrides только по реальной потребности; сейчас одна объяснимая tenant policy плюс прежний environment safety cap.
- Утвердить отдельные сроки минимальных receipts/source/security/financial audits до любого их удаления. Raw XLSX уже не хранится.
- ONE_C остаётся interface-only и disabled; реальный adapter/credentials/sync требуют нового разрешения.
- E01 OPEN до доказанного writer; strict record/verify guard не ослаблять.
- Live Telegram/WhatsApp NOT READY: официальные provider pricing/templates/Graph-version и operator/privacy/HTTPS/live checks из9В не заменяются health/onboarding/source flags. Оба бюджета0 RUB. Внешние вызовы требуют отдельного явного разрешения.
