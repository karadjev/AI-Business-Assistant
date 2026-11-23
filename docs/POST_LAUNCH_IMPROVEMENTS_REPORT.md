# Post-launch promotion — AI Business Assistant

**PUBLIC SITE LIVE · DEPLOYMENT UPDATED · PUBLIC CHROMIUM E2E PASS**

На Play2Go опубликован `7a84b6a10d385edf68366feaab60f8f6a8101f5b`. Публичное приложение и прямой API используют свои действующие домены. [Сайт](https://ai-business-store.com) [API](https://api.ai-business-store.com)

| Критерий | Результат |
|---|---|
| DEPLOYED COMMIT | `7a84b6a10d385edf68366feaab60f8f6a8101f5b` |
| NEW MANIFEST SHA-256 | `949977f4997aab73e3e4b18e6a801e31510cab035d032d6613cdb9ba56a171d7` |
| DATABASE SCHEMA | 31, 62 FORCE RLS. Действующий том `aba-private-restore-cutover-20261123004447_database` сохранён. |
| DOCKER HEALTH | **PASS**, 6/6. API, web, worker, proxy, PostgreSQL и Mailpit соответствуют манифесту. Worker использует образ API. |
| PUBLIC CHROMIUM E2E | **PASS**, общая регрессия и Auto Service на синтетической организации. |
| PUBLISHED FEATURES | Профиль «Автосервис с продажей запчастей» и автоматическое обновление статуса последней отправленной задачи. |
| LOCAL-ONLY FEATURES | Нет для этого кандидата. |
| FIXED BUGS | Устранён сырой Nginx 401 для `/dashboard/demo`. Для подготовки заказа Auto Service и действий с заказом используются штатная авторизация, разрешения и CSRF. Плановый backup получает PostgreSQL через Compose с активным override. |
| REMAINING LIMITATIONS | Auto Service delegation запущен из Chromium через авторизованный BFF endpoint. Профиль, подтверждение заказа, резерв и Audit Log проверены через пользовательские страницы. Публичная регистрация, сброс пароля и реальная доставка по внешним каналам закрыты. |

## Допуск

**D2 — NOT PERFORMED.** Настоящая запись заменяет ранее переданную формулировку о положительном D2. **Owner GO и Owner Risk Acceptance** получены для указанного commit и manifest SHA-256. Независимый вердикт не заявлен. E01 остаётся OPEN.

## Проверки Play2Go

- **PASS.** SSH root через существующий ключ. Сверены исходные образы, активный override, том, схема и размер очередей. Прежние 2 432 и новые 2 437 хешей файлов совпали. Архив нового web-образа и опубликованный чанк AI Team сверены с зафиксированными SHA-256. Новый образ заменил только web. Proxy перезапущен для обновления upstream DNS. Миграции не выполнялись.
- **PASS.** Перед promotion создан свежий `promotion-7a84b6a-20261123T085114Z.dump`, SHA-256 `f1a24e4eb929dd3fd2feed6d3d0a5d82baa60aa9932170dcca32f29eabfa5b43`. Прочитано оглавление. Архив восстановлен в отдельный `aba-private-restore-promotion-7a84b6a-ansovi78`. Совпали полученные из одного snapshot отпечатки схемы, организаций, задач, Human Approvals и Audit Log. Исходный том сохранён. Предыдущий web-образ запустился и отдал страницу входа в отдельном контейнере без сети. Переключение действующего сайта обратно не потребовалось.
- **PASS.** API readiness, PostgreSQL, liveness worker и обработка новых задач после контролируемого перезапуска. Очереди пусты в итоговой проверке. Nginx `-t`, TLS при запросах Chromium и curl, здоровые службы и UFW с входящими портами 22, 80, 443. В проверенном окне стартовых логов шести контейнеров не обнаружены fatal/error. Плановые backup и мониторинг завершились с кодом 0. Внешняя копия backup не настроена.
- **PASS.** Главная и вход без Basic Auth. Анонимные Dashboard и `/dashboard/demo` в Chromium приводят на страницу входа приложения. После входа открываются Dashboard и выбор демонстрационного магазина. Прямой API остаётся защищённым. Запросы без сессии или CSRF и к чужой организации отклонены. Проверена cookie с `Secure` и `HttpOnly`. Правило rate limit входа сохранено. Нагрузочный тест лимита после обновления не запускался.
- **PASS.** Синтетический владелец видит семь агентов, меняет профиль, сохраняет модель `hybrid` и видит её после перезагрузки. Задача создана из интерфейса, прошла Human Approval и обновилась до завершённой автоматически. Результат сохранился после перезагрузки, действие присутствует в Audit Log. Очередь и завершение отдельно подтверждены событиями Audit Log `task.submit=queued` и `task.finish=completed` для новой задачи, созданной из интерфейса. Проверены logout, повторный вход и 390 px.
- **PASS.** В Auto Service worker подтвердил обязательную совместимую деталь и доступность ресурса. Подготовлен `hybrid_order`, владелец подтвердил заказ через публичный интерфейс, worker сохранил завершённую квитанцию с фактическим активным резервом и записью в Audit Log. Сценарий выполнен на 390 px с синтетическими данными. Внешних отправок не было.

Первый проверочный прогон профиля попал в окно обновления состояния формы. После ожидания PATCH и перезагрузки профиль сохранился. Второй прогон выявил неверное ожидание теста: успешный PATCH возвращает 204. Сохранены также отказы smoke во время перезагрузки edge, ошибка регистра SKU в синтетическом наборе и корректный отказ worker `source_changed` на устаревшей ревизии предложения. Финальные прогоны выполнены с исправленными предусловиями. Приложение повторно не пересобиралось.

Детали и журналы собраны в [индексе доказательств](POST_LAUNCH_RELEASE_EVIDENCE.md). Точные образы и идентификаторы зафиксированы в [записи развёртывания](POST_LAUNCH_RELEASE_MANIFEST.json). Операции и откат описаны в [VPS runbook](VPS_DEPLOYMENT_RUNBOOK.md).

## История подготовительного этапа

**PUBLIC SITE LIVE · application release unchanged.** Frontend https://ai-business-store.com opens without HTTP Basic Auth. The direct API https://api.ai-business-store.com retains its edge protection. Production runs commit `30f5343eb85e08ff7a2b464de0434bf0899a2a86`, manifest SHA-256 `4fc9424d08c84f8259a5c7e1e633e8ad6e4818c207f3b59f37f65b6d5ae3bc3d`, PostgreSQL schema 31, 62 FORCE RLS relations and six healthy containers. The previous owner risk acceptance applies only to that release.

## Installed on Play2Go

- **Volume and image guard.** `/srv/aba/staging/active_release.py check` verifies the manifest, the resolved Compose override, network exposure, images and the actual PostgreSQL mount. The recorded active volume remains `aba-private-restore-cutover-20261123004447_database`. A missing override and the old stale volume were both rejected in read-only tests. Use the guarded `restart worker`, `restart web`, `restart api` or `restart proxy` for a supported single-service operation. Manual raw Docker commands can bypass the wrapper and must use the recorded Compose args and override. The guard does not expose migration, volume replacement or deletion commands.
- **Monitoring repaired.** The timer had been failing because the old monitor expected schema 29. The existing monitor and backup jobs now derive configuration from the active record. Both systemd jobs completed with exit 0. Original scripts are retained in the private VPS operations report directory. Monitoring checks the three application services, database, queues, disk and memory. No service unit or application artifact changed.
- **Schema 31 backup verified.** The scheduled job produced archive `20261123T005659-ff410230a89b480196b02394587b67d9.dump`, SHA-256 `3e3fca3236a95ba491b9c1979479877453ca8bc7dc83f453c658909236550809`. Restored to a new isolated PostgreSQL volume. Schema 31, 62 FORCE RLS relations, zero organizations without tenant context, and current API readiness all passed. Retained both the production volume and rehearsal volume. No external backup was configured in this work. The schema-29 rollback backup and prior image set from cutover are retained. Recovery to 29 after new writes requires reconciling those writes, not replacing the live volume blindly.
- **Worker restart verified.** Restarted only the worker after checking for pending work across agent, channel, Telegram and email queues. Task, approval and audit fingerprints stayed identical across restart. A new synthetic task then completed in live Chromium. Docker, Nginx, backup and Certbot timer are enabled. All six container restart policies are `unless-stopped`. Full VPS reboot was not performed. Docker logs use 10 MiB × 3 files. Host had about 124 GiB free. TLS expires 2027-02-20 GMT and the renewal timer is enabled. No container publishes a host port.

## Live acceptance

**Public Chromium PASS after worker restart.** Landing and login without Basic Auth, Dashboard, organizations, all seven agent cards, synthetic marketing task, Human Approval, completed result, Audit Log, logout, new login and 390 px viewport. Anonymous and foreign-tenant requests were denied, CSRF protected writes and the workflow made no external send. Registration and reset remain gated pending external email.

## Local improvements and verification

In `work/post-launch-stabilization` at `7a84b6a10d385edf68366feaab60f8f6a8101f5b`, the existing business profile form now offers `vehicle_service + hybrid` as “Автосервис с продажей запчастей”. It saves the model change even when the industry stays the same. The backend permission model did not change. The last submitted task shows its state and updates it with bounded polling in the visible tab. Manual refresh remains available.

- **Isolated PostgreSQL integration PASS**, exit 0, 91.825 s. Includes required-part Auto Service, industry flows, delegations, lease expiry and retry, partial failure persistence and queue checks. This is not a public rerun of every industry, agent or fault scenario.
- **Frontend lint, typecheck and build PASS** after replacing an impure timestamp flagged by the React lint rule with a bounded polling counter.
- **Isolated Chromium PASS**, verified profile switch from service to hybrid through actual API and PostgreSQL. A synthetic task queued with the isolated worker stopped, completed after restart, and updated in the browser without a refresh at 390 px.
- **Isolated Auto Service Chromium PASS**, 28.3 s. The existing integration browser case exercised a required part, rejection of a foreign conversation, Human Approval, idempotent confirmation, recovery of the durable receipt after worker restart, cancellation and the audit result in the UI. It also covered the unknown and no-part cases. The selector fix enables the existing backend capability.

The new frontend is local only. It has no new approved manifest and was not deployed. Follow-ups are independent release review for changed application artifacts, a patched Node runtime, external encrypted backup custody if supplied, and coverage for per-agent scenarios not exercised live. SMTP, public registration and real channel sends remain disabled.

Private evidence: `.local/post-launch/` and VPS operations directory identified by `remote-ops-path.txt`. The active Compose configuration is recorded in `/srv/aba/staging/active-release.json`. The host-only operational changes leave the application manifest and Nginx policy intact.
