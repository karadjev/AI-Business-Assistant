# Безопасность и персональные данные

> ТЗ № 3: реализованный auth/CSRF/RLS описан в AUTHENTICATION.md и TENANT_ISOLATION.md. Ниже также приведены будущие требования production, не свидетельство полного аудита. Фактический запуск и проверки: [LOCAL_DEVELOPMENT](LOCAL_DEVELOPMENT.md).
Проектные меры, не свидетельство выполненного аудита. До реального пилота нужна проверка реализации и актуальных правовых условий.

## Аутентификация и API

Пароли Argon2id с солью и параметрами, выбранными по измерению времени/памяти сервера; минимум 12 символов, поддержка длинных паролей, без принудительных бессмысленных периодических смен. Email verification, generic ответы на login/reset, rate limit по IP и account; успешная регистрация не раскрывает существующий email. Reset token одноразовый, hash в БД, срок предложен 30 минут; изменение пароля отзывает все сессии. Verification token предложен 24 часа.

Случайные opaque session tokens ≥256 бит, hash в БД, cookie Secure/HttpOnly/SameSite=Lax. Rotate при входе/изменении привилегий. Предлагаемые idle timeout 24 ч, absolute 7 суток; чувствительные операции требуют повторного ввода пароля. TLS, CSRF token и Origin check для unsafe methods; CORS allowlist, CSP, запрет embedding кабинета, no-store для чувствительных ответов. XSS: экранирование текста покупателя, без произвольного HTML/Markdown, инструкции агента отображаются как текст. JWT не нужен для браузерных сессий MVP. MFA owner — рекомендованный следующий этап до расширенного коммерческого запуска.

JSON строго валидируется, лимиты body/text/page, SQL только параметризованный, allowlist сортировки/фильтров, timeout на DB/HTTP, pagination. Базовые rate limits предложены: login 5/min на account+IP с backoff, register/reset 3/hour на IP, кабинет 120/min на user+org, сообщения 30/min на chat. Подбирать по пилоту, не считать универсально безопасными. Redis fail → fail-closed для auth/public abuse-sensitive endpoints; кабинет допускает ограниченный локальный limiter, worker сохранённые задачи не теряет.

## RBAC и tenant isolation

Membership проверяется при каждом HTTP запросе и фоновой команде; сохранённая cookie не фиксирует вечные права. Права tools получены из policy AND platform allowlist. RLS, составные FK и org-scoped repositories обязательны. Чужие ID в path/body/tools/jobs — отдельные негативные тесты. Cache keys, search results, cursor, logs, rate limit и AI history включают tenant. Запрет кросс-tenant customer merge.

Права подключения integration и расшифровки credentials отделены. Внутренний административный доступ к журналам/перепискам только по необходимости, ограниченный по времени, с основанием и аудитом; никаких публичных superadmin endpoints в MVP. Роль мигратора не используется runtime. Export/backup также обходят RLS только отдельной процедурой и защищены независимо.

## Секреты и supply chain

Токены интеграций encrypted at rest (authenticated encryption, например AES-GCM, уникальный nonce, AAD=org+integration+field). Ключ шифрования вне БД и Git; версия ключа хранится с ciphertext. Локально dummy secrets в ignored .env, будущий сервер — secret store/защищённый runtime environment. Ключ и backup БД не лежат вместе без контроля доступа. Нужны процедура ротации и восстановление ключа. API никогда не возвращает plaintext сохранённых токенов.

Не логировать authorization headers, cookies, webhook secrets, provider URL с bot token, prompt/response целиком. Redaction до logger и error tracker. Зависимости закреплены lockfiles, CI проверяет уязвимости и утечки секретов, least-privilege GitHub token. Актуальные версии выбираются на этапе skeleton, сейчас dependencies не устанавливаются.

## Карта данных и внешние LLM

Переписки содержат телефоны, имена, адреса, намерения покупки и иногда непредусмотренные чувствительные сведения. Поток: канал → Go inbox → PostgreSQL → минимизированный prompt → согласованный LLM → проверенный ответ → канал. Данные email владельца, credentials, контакты покупателя и внутренние заметки не нужны LLM. Заменять идентификаторы псевдонимами, редактировать телефоны/email/адреса в prompt; эвристики не гарантируют полного обезличивания свободного текста.

Риски: хранение/обучение у провайдера, зарубежные subprocessors, изменение условий, восстановление личности по переписке, prompt injection, утечка через telemetry, недоступность/санкционные и договорные ограничения, неконтролируемые расходы. Выбор провайдера: договор на обработку, разрешённые регионы, срок хранения, запрет обучения на данных, режимы удаления/zero retention (не предполагать доступность), список subprocessors, доступность API для РФ, инструменты и фактический pricing. Переключение адаптера не означает автоматическое разрешение новой передачи данных.

До согласования обработчика использовать mock и синтетические данные. Если передачу PII законно/договорно обеспечить нельзя — выбрать подходящий локальный/региональный сервис либо оставить обращения сотруднику. Согласие не считается универсальной заменой всем требованиям закона.

## Правовой контур РФ

Нужно определить роли: магазин обычно определяет цели клиентского общения, платформа может обрабатывать по поручению; для собственных аккаунтов/биллинга у платформы отдельные обязанности. Это гипотеза, уточняется договорами и юридическим анализом. Требуются цели, состав данных, основания, политика конфиденциальности, порядок запросов субъектов, договор с магазином, оценка уведомлений Роскомнадзору и трансграничных передач. Учитывать сам мессенджер, а не только LLM.

Предлагается хранить primary DB, replicas и backups с ПД в РФ. Проверить действующую ст.18(5) и ст.12 152-ФЗ перед запуском: локальное хранение не снимает автоматически ограничения зарубежной обработки. [Официальный текст 152-ФЗ на сайте Минтруда](https://mintrud.gov.ru/docs/laws/130); для текущих формулировок проверены [ст.18](https://www.consultant.ru/document/cons_doc_LAW_61801/cbf4e15b7c330f9372e876cdf2bc928bad7950ef/) и [ст.12](https://www.consultant.ru/document/cons_doc_LAW_61801/e4ebbe1780de623c7cf32a59ca82a7bb523a25dd/). Форма уведомления: [портал Роскомнадзора](https://pd.rkn.gov.ru/cross-border-transmission/form2/). Эти ссылки — материал для согласования, не заключение о соответствии конкретного проекта.

## Хранение, удаление и восстановление

Предложенные сроки для пилота, требуют утверждения: raw webhook 7 дней, переписка 90 дней, очищенный operational log 30 дней, audit 180 дней, backups 30 дней. Заявки/контакты и бухгалтерские данные имеют разные цели и законные сроки; окончательный retention установить после выбора модели продаж, не объявлять всё бессрочным.

Удаление охватывает customers, messages, quote/snapshots, inbound_events, AI summaries, кэши и допустимые копии у провайдера. PII не должна оставаться в audit details. В backups удаление происходит по expiry; после restore повторно применять deletion ledger, чтобы не вернуть удалённые сведения. Сам ledger минимальный, с контролем доступа; его схема и retention уточняются до production.

Предварительная backup-политика: ежедневный encrypted full backup, WAL/PITR для цели RPO ≤15 мин; RTO ≤4 ч — цель, проверяемая restore drill, не обещание. Отдельное хранилище, ограниченные роли, контроль успешности, размер/целостность, сохранение ключей. Проверка восстановления перед пилотом и ежемесячно далее. Пока инфраструктура не создана и восстановление не выполнялось.

## Инциденты и обязательные проверки

Сценарии: компрометация bot token → disable integration, rotate, audit; утечка tenant → остановить affected flow, сохранить очищенные следы, оценить уведомления по актуальным требованиям; перерасход → блокировать новые AI runs; сбой LLM → handoff. Контакт ответственного и юридический порядок уведомлений назначаются до пилота.

Будущие проверки: два tenant и все CRUD/FK; отсутствие app context и повторное использование pool connection; worker с чужим org; просроченная/чужая session/confirmation; CSRF, XSS, webhook spoof, replay; DB race последней единицы; prompt injection; redaction logs; восстановление и повторное удаление. Это план, а не результаты тестов.

## Реализация ТЗ № 5

Stage5 orders/chat только в отдельно созданной test organization; is_test нельзя менять runtime UPDATE organizations. Новые FORCE RLS tables требуют selected org + verified session + staff + test. Member can_manage_orders не разрешает менять role/каталог/настройки. Узкие create_test_organization/list_organization_access/set_order_access/lock_order_products definer functions имеют фиксированный search_path, PUBLIC EXECUTE revoked, явные membership/scope checks и не исполняют dynamic SQL. Stock staff policy только test org; служит server transactional order flow.

Consent не берётся из LLM/user text: explicit server quote/chat/customer binding, accepted checkbox/button, five-minute expiry, fresh price/stock recheck, immutable order snapshot. Order actions expected_version + UUID idempotency JSON, snapshot/audit append-only. Price conflict требует нового клиентского согласия, не staff override.

Все model tools strict allowlist/typed validation, без org selection/SQL/HTTP/files. Free LLM content не отображается как товарные факты; backend cards/template, XSS escaping. Model timeout2s/iterations2/tools4/input-output/day quotas и conservative reservations for unknown usage. Handoff/revision fences late replies; live disabled/defaultmock and no paid calls. Token limit не валютный budget; pricing/privacy/live evals нужны перед paid provider.

Synthetic chat expiry7d, explicit erase payload/hash and close; lazy tenant purge on list/get, а не гарантированный cron. Полный production deletion/backup retention/audit и юридическая политика до настоящих данных не утверждены. Не вводить реальные ПД. Детали и пределы —AI_AGENT_IMPLEMENTATION, CONVERSATION_LIFECYCLE, STAGE_5_REPORT.

## Этап6 Telegram

См. [TELEGRAM_SECURITY](TELEGRAM_SECURITY.md). Runtime только mock/test shops; production enabled fail-closed. AES256GCM separate server key + org AAD, secrets/hash/capabilities не API/logs. Integration UUID/header constant-time hash проверка + повторный DB binding; body16KiB, strict relevant private fields, quota, durable ACK/dedup. Channel customer lease не owner session, active read/single chat/pending request only; employee confirms reserve. Same-origin BFF sessions/CSRF/SSR сохранены. Background synthetic payload purge, metadata сохраняются до отдельной production privacy/retention policy. Real Telegram/LLM calls не выполнены.

## Этап 7: внешние gates и денежные операции

Согласовано 0 RUB, provider NOT SELECTED, Telegram external NOT AUTHORIZED; реальные пользователи и production запрещены. Не хранить токены в frontend/localStorage/OpenAPI. LLM HTTPS base URL exact allowlist + DNS/IP validation + no proxy/redirects; инструмент не выбирает URL/tenant. Only Metered может выдать budget permit. Telegram user/chat allowlist проверяется до создания диалога и отправки, sending зависит от env kill switch и org flag. Внешние ручные операции требуют HTTPS origin, owner/session/CSRF и подтверждение; открытие dashboard не выполняет getMe. Auto off аннулирует conversation in-flight fencing. SQL денежные функции узкие SECURITY DEFINER, transaction-local tenant и FORCE RLS; global budget не предоставлен runtime роли. Остатки/quote consent сохраняют прежние правила. Новые PostgreSQL проверки FORCE RLS, бюджета и удаления PASS; полный draft retention и future approvals — DATA_RETENTION_POLICY_DRAFT/PILOT_READINESS.

## Уточнение этапа 8А (16.10.2026)

Актуальные независимые доказательства и ограничения: [SECURITY_AUDIT_REPORT.md](SECURITY_AUDIT_REPORT.md). Миграция8 закрепляет неизменяемость автора messages, erasure-only channel update, send fence против disconnect/handoff и контроль FORCE RLS в readiness. Runtime SQL credentials и SECURITY DEFINER функции остаются доверенной границей; RLS не защищает от полного захвата процесса. Telegram/LLM live не разрешены, бюджет0; source scan и offline tests не заменяют live/TLS/legal/CVE review. Открытые A09/A10 блокируют соответствующий переход к live.

## 8Б.2 — второй канал

WhatsApp mock использует отдельный AES-GCM key и server worker proof, raw-body HMAC, WABA/Phone routing и allowlist до principal. Tenant scope `app.channel_cap` не принимается от HTTP/LLM и отличается от Telegram scope. Миграция10 добавляет5 FORCE RLS таблиц и не меняет миграции1–9. Shared orders/quote/consent/reserve остаются едиными. Outbound admission повторяет window/active/control/lease; UNKNOWN без auto-resend. Live WhatsApp transport отсутствует, CHECK monetary_budget=0, положительный env/live flags отклоняются. Детали и ограничения: WHATSAPP_SECURITY и STAGE_8B2_REPORT. Реальные Meta/Telegram/LLM вызовы не разрешены.
