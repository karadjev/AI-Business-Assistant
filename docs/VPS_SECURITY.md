# VPS security boundaries

10 ноября 2026. **Security CONDITIONAL, CUSTOMER PILOT NO-GO.** Smoke PASS не закрывает E01/SQL/privacy/advisory blockers. Используются прежние auth/tenant/financial gates; новый coordinator не меняет их.

- Staging network internal, published ports0, PostgreSQL5432/SMTP/Mailpit/admin/Docker API не публикуются. Docker socket не монтируется в приложение. Оператор Engine — доверенная административная граница.
- API/worker UID10001, web10001, proxy101; application roots read-only, cap_drop ALL/no-new-privileges. PG bootstrap использует штатный entrypoint/UID70, отдельный volume. Runtime aba_app — без superuser/BYPASSRLS/schema CREATE/членства migrator; миграции отдельной ролью.
- Env0600 — только IDs/paths/origin. Secret files вне image/context/release, private keys0400 с требуемым UID. Coordinator проверяет формат/роли/раздельность ключей и DB passwords, verify-full/CA; значения не печатает. На local QA host UIDs не считаются native Ubuntu evidence.
- Telegram/WhatsApp/1C live OFF, LLM mock, внешние authorization flags false, kill switches true, budgets0. Channel encryption keys в production не добавлять до отдельного privacy approval; mock integration suite выполняется отдельным prepared runner.
- HTTPS TLS1.2/1.3; hostname/SAN и CA trust обязательны. Новый smoke ходит только к private `api`/`proxy`, не отключает TLS verification, не пишет cookies/body/URL. CSRF probe проверяет отказ запроса без Origin; полный authenticated CSRF/consent/staff сценарий — отдельные integration/Browser gates.
- Next guard обязателен перед export/упаковкой и каждым startup. Исходный polluted artifact/evidence сохранены. Никаких E01 exclusions/rebaseline или automatic retry unknown delivery.

Перед server GO оператор отдельно настраивает default-deny inbound, SSH keys, no root/password login, allowlisted IP/VPN, NTP и disk monitoring. Проверить вторую SSH-сессию до применения firewall; учитывать Docker chains/DOCKER-USER. Scripts не выполняют sudo и не меняют sshd/UFW/global Docker settings.

Проверки после разрешённого start (только свой project):

```sh
docker compose --env-file /srv/aba/staging/environment -f /srv/aba/current/infrastructure/deployment/compose.yml -p aba-private-staging ps
python3 /srv/aba/current/scripts/deployment/control.py validate --release /srv/aba/current --env /srv/aba/staging/environment --project aba-private-staging
```

Не печатать full `docker inspect`, config environment, secret contents или raw auth logs в shared output. Coordinator сохраняет только allowlisted status/metrics и фиксированное имя failed stage; subprocess errors подавляются, чтобы DSN/SQL не попали в report.

Private access после GO: SSH tunnel bind127.0.0.1 к8443 IP своего proxy на Ubuntu, как в существующем VPS_DEPLOYMENT_RUNBOOK.md. Не добавлять public80/443/public production overlay до отдельного review. Access logging proxy выключен; nginx error context может содержать URI — logging/privacy review до реальных данных остаётся обязательным. Advisory/provenance/secret scanner не скачиваются автоматически.
