# Trusted template status — 9В

Реальный Meta verifier отсутствует. Mock approval всегда SYNTHETIC и никогда не является подтверждением Meta.

Используется прежняя wa_templates: tenant/integration/WABA/name/language/category/status/components/parameter schema/provider identity/verification/source/policy version. wa_provider_records добавляет versioned provenance, SHA256, expiry и revocation. Provider template identifier опционален и не является секретом доступа.

`MockTemplateSource.TemplateStatus` возвращает только synthetic attestation. `Service.VerifyTemplate` совместим с прежним mock verifier, но направляет результат в общий provisioning, вместо отдельной доверительной логики. SQL проверяет backend proof и связь tenant/integration/WABA. Name/language/WABA resource identity не меняются silently при обновлении. Body/text parameter constraints9Б сохранены.

PENDING→APPROVED, APPROVED→PAUSED/REJECTED/DISABLED/UNKNOWN допустимы только через backend provisioning с актуальной revision. Эти состояния не отменяют provenance checks: APPROVED с отозванным/истёкшим/изменённым ресурсом блокируется. Одинаковый импорт идемпотентен; два различных статуса с одной expected_version не могут оба примениться. Неподходящий WABA/tenant и неправильное evidence отвергаются.

Существующий template outbox и `wa_template_check` повторно проверяют актуальный status, components/parameters, source, digest, verification expiry и revocation непосредственно перед dispatch. Queue-time/UI validation недостаточны. Финансовое admission остаётся обязательным. Generic text fallback отсутствует. Отдельная очередь не создавалась.

Owner UI показывает readonly Meta-unverified/synthetic registry, source type, verification date, provider readiness и причину блокировки. Нет UI изменения APPROVED или кнопки live activation. POST произвольного approval через BFF не поддерживается. Operator может отозвать record через tenant-scoped endpoint; создать реальное Meta approval такой отзыв/назначение не позволяет.
