# Аутентификация — ТЗ № 3

## Реализованный поток

POST /api/v1/auth/register принимает email, password, display_name. Email: trim + lowercase, ASCII адрес до 254 байт, без display-name/CRLF; UNIQUE в PostgreSQL. Пароль 12–128 Unicode символов, максимум 512 UTF-8 байт. Имя 1–100 символов. Новый и уже зарегистрированный адрес получают одинаковый 202 и текст; существующий пароль не меняется. Ошибки формата —422, неизвестные JSON-поля/некорректный JSON —400. Наличие аккаунта не раскрывается регистрацией/resend/login.

Argon2id: 64 MiB, 2 итерации, parallelism=1, соль 16 байт, результат 32 байта; encoded hash с параметрами. Параметры выше минимального набора [OWASP Password Storage](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html). На процесс API максимум два одновременных password hashes; ожидание ограничено request context. Для неизвестного email выполняется сравнение с dummy hash с теми же параметрами. Это не гарантия математически одинакового timing всего HTTP-пути; production anti-abuse требует отдельной оценки.

Регистрация атомарно записывает user, SHA-256 verification token и зашифрованную email job. Token —32 случайных байта (base64url, 43 символа), по умолчанию 24 часа. Письмо содержит URL `/verify-email#token=...`: fragment не передаётся HTTP server. Frontend читает его в поле, удаляет fragment из адресной строки и выполняет POST только по явному нажатию, не при загрузке/сканировании ссылки. Токен не возвращается в API, не логируется и не хранится plaintext в БД.

POST verify-email атомарно потребляет ещё не использованный/не истёкший token и отмечает email_verified_at в транзакции. Повтор, неизвестный и истёкший token —одинаковый 422 INVALID_TOKEN. Resend для непроверенного адреса выдаёт новый token, аннулируя прежние; для неизвестного или уже проверенного адреса тот же 202, без письма. Открывать последнюю ссылку. Существующий verified аккаунт не требует повторной проверки. Password reset, MFA, приглашения и управление всеми сессиями не входят в этот этап.

## Mailpit и worker

Только локальный SMTP sink `mailpit:1025` / `127.0.0.1:1025` / `localhost:1025`, без relay/реальной рассылки. Mailpit UI: http://127.0.0.1:8025. В Compose SMTP port не опубликован; native override публикует только 127.0.0.1:1025. UI также только loopback. Не публикуйте Mailpit через внешний tunnel: письма содержат одноразовые секреты.

AUTH_EMAIL_KEY —32 байта hex, отдельный локальный ключ AES-GCM; в .env.example явный dummy, в ignored .env сгенерирован отдельный случайный. Секретный payload To/link шифруется с уникальным nonce и AAD. После отправки payload очищается. Worker: SKIP LOCKED claim, lease 30 s, SMTP timeout 5 s, job deadline 8 s, retry через 15 s, максимум 5 попыток →failed. At-least-once доставка может повторить письмо после сбоя между SMTP ACK и DB update; одноразовость токена остаётся в БД. Нет фиктивного успешного delivery: sent только после SMTP ACK. Повтор failed письма пользователем —resend, новая ссылка. Старые pending письма могут содержать уже аннулированную ссылку.

Ключ нужно сохранять между перезапусками до доставки queued mail; его ротация требует отдельной процедуры. Auth rate/session/token/outbox retention cleanup ещё не автоматизирован; это эксплуатационное ограничение dev-пилота, до увеличения трафика нужен отдельный maintenance job и утверждённые сроки. Worker теперь обслуживает только dev-email; каталога/заказов/LLM/Telegram jobs нет.

## Сессии и CSRF

POST login разрешён только с verified email. Неверный пароль, неизвестный или unverified email —401 INVALID_CREDENTIALS с одним текстом. Каждая успешная авторизация создаёт новый случайный 256-bit session ID и CSRF token; предыдущая сессия этого браузера отзывается, предотвращая fixation. Другие устройства не отзываются автоматически. API никогда не возвращает session hash/raw session ID в JSON.

Session cookie `aba_session`: HttpOnly, Path=/, SameSite=Lax, Max-Age по absolute TTL; Secure при APP_ENV=production и имя `__Host-aba_session`, без Domain. В production PUBLIC_ORIGIN должен быть HTTPS, DATABASE_URL sslmode=verify-full. В локальном development cookie без Secure для HTTP; NODE_ENV Next.js не определяет безопасность Go cookie. Браузерные authentication tokens не помещаются в localStorage.

По умолчанию absolute TTL 168 h, idle TTL 24 h; idle продлевается после проверки me/защищённого запроса, но не дальше absolute expiry. БД хранит только SHA-256 session и CSRF hashes. Sessions нельзя напрямую SELECT runtime-ролью: lookup/touch/revoke только через узкие функции. GET me проверяет revoked/absolute/idle и verified email в БД. Logout требует действующую сессию, отзывает её и удаляет оба cookies.

Для всех POST/PATCH требуется точный Origin=PUBLIC_ORIGIN. Для cookie-authenticated мутаций дополнительно X-CSRF-Token, сравнение hash constant-time с hash текущей session. `aba_csrf` cookie читается JS, SameSite=Strict; отдельно он не является авторизацией. Это session-bound synchronizer token, не простое сравнение двух клиентских значений. [OWASP CSRF guidance](https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html). Public login/register/verify/resend защищены Origin, JSON-only и rate limiting; login CSRF не разрешён. Нет wildcard CORS.

## Rate limits и ошибки

PostgreSQL атомарный UPSERT счётчика по SHA-256(scope+key), minute/hour buckets. На каждый public auth action: 40/min по API peer IP; дополнительно register/resend 3/min на нормализованный email, login 5/min на email, verification 30/min. Создание организации 10/hour на user. 429 с Retry-After=60. Это fixed-window limit: на границе окна допускается burst. За BFF peer IP общий, account limit остаётся отдельным; X-Forwarded-For не доверяется. Для большего трафика определить доверенную proxy topology/client IP и более точный retry/backoff, не ослаблять лимит обходом клиентского заголовка.

401 unauthenticated, 403 CSRF/owner-only, 404 чужая либо отсутствующая организация, 409 optimistic version, 422 validation/token, 503 DB/API недоступность. API ошибки содержат code/message/request_id, без SQL, password/token/hash/stack trace. Логи: request method/status/generated request ID, delivery failure без email/link/credentials. Body/request cookies не логируются.

## Frontend и BFF

Browser обращается `/api/v1/...` к Next.js. Allowlist proxy пересылает только известные auth/org paths, нужные HTTP methods, Origin/Content-Type/Cookie/X-CSRF-Token; body ≤16 KiB, фиксированный API_BASE_URL server-only, redirects запрещены. Set-Cookie передаётся без выдачи tokens в JSON. Бизнес-routes не открывают произвольный proxy/URL. SSR `requireUser` вызывает реальный Go me; 401 →/login, 503 →страница ошибки. Org страницы также читают данные через backend без cache. SSR layout + отдельная проверка create page защищают прямые загрузки всех `/dashboard/*`.

Активная организация выбирается query parameter `org`; сервер сверяет его со списком memberships. ID не считается полномочием и не сохраняет backend tenant context. HTML в пользовательских полях выводится как React text, не dangerouslySetInnerHTML; проверено браузерным XSS-сценарием.

## Конфигурация и ручной сценарий

```sh
docker compose up --build -d --wait
docker compose ps
```

Для существующего .env добавить AUTH_EMAIL_KEY (случайные 64 hex символа), PUBLIC_ORIGIN=http://127.0.0.1:3000, SMTP_ADDR=127.0.0.1:1025. Текущий .env уже подготовлен, не перезаписывать. При изменении WEB_PORT менять PUBLIC_ORIGIN, иначе CSRF должен отклонять запросы. Контейнерный SMTP_ADDR задан mailpit:1025. Optional SESSION_TTL/SESSION_IDLE_TTL/VERIFICATION_TTL, interval 1s..168h; idle ≤24h и ≤absolute TTL.

Открыть /register, указать синтетический email `owner@example.test`, пароль ≥12 символов, имя. В Mailpit найти последнее письмо, открыть его ссылку, нажать подтверждение. Войти /login, создать магазин, изменить название, обновить страницу, выйти. Не использовать реальные клиентские данные.
