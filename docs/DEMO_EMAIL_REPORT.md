# Offline demo: регистрация и email verification

30 октября2026. Security CONDITIONAL, E01 OPEN. Внешняя отправка не подключалась. HUMAN REHEARSAL NOT RUN.

## Проверенная причина

Регистрация сохраняет user с неподтверждённым email и запись verification token; письмо шифруется и помещается в `platform.email_outbox`. Существующий worker вызывает `auth.DeliverOne`, который отправляет его по SMTP в **локальный Mailpit, mailpit:1025**. После приёма catcher статус становится sent. Это не доставка во внешний почтовый ящик. Конфигурация auth допускает только локальные SMTP sink addresses; сети demo internal, catcher не опубликован на host.

На момент диагностики в отдельной demo БД обнаружены: **1 регистрация вне подготовленных example.test fixtures, 1 неподтверждённый user, 1 verification record, 1 outbox sent**. Mailpit содержит **1 письмо**, получатель которого соответствует этой регистрации. Адрес сопоставлен в памяти процесса и не сохранён в отчёте; тела писем, verification links/tokens, password hashes и passwords не выводились. Диагностика не потребляла verification token, не подтверждала новую регистрацию и не запускала resend.

Агрегированное evidence: `.local/demo-email/registration-summary.json`. Источники реализации: `apps/api/internal/auth/service.go`, `email.go`, `apps/api/internal/config/auth.go`, `apps/api/cmd/worker/main.go`; они не изменялись.

## Изменение UI и production

В `apps/web/src/app/register/page.tsx` добавлено уведомление: **«Локальная демонстрация: реальные письма не отправляются.»** Пояснение указывает локальный Mailpit, synthetic email и вход в подготовленный аккаунт. Credentials и ссылки подтверждения в UI не добавлены.

Уведомление включается только при `APP_ENV=development` и точном `ABA_LOCAL_DEMO_EMAIL_NOTICE=1`. Последний задан только в `infrastructure/compose.demo-offline.yml` для web. Он меняет текст, не разрешения или отправку. Страница читает runtime environment через Next connection(), чтобы demo flag не запекался в общую сборку. Техническое отличие rendering: register server-rendered on demand; production форма, сообщения/API, verification и login contracts сохранены. Без флага и в application production mode уведомления нет; production mode подавляет его даже при флаге1. Backend/auth, IdentityForm, RLS, orders, worker, security gates и retention не менялись. Новых endpoints, bypass, зависимостей или миграций нет.

Обновлён `docs/DEMO_WALKTHROUGH.md`: транспорт и безопасный вход. Этот отчёт — четвёртый изменённый/новый scoped file.

## Проверки

| Проверка | Результат |
|---|---|
| Go auth/config unit | PASS |
| Isolated PostgreSQL integration/race, TestDatabaseAndMigrations | PASS84.238s, включая существующий identity flow, verification/login/tenant/RLS и остальные проверки родительского сценария |
| Frontend lint/TypeScript/OpenAPI/native/artifact tests | PASS |
| Fresh Next16.4.0 production build | PASS |
| Current artifact + served integrity | PASS |
| Chromium: demo notice + исходная регистрационная форма | PASS |
| Chromium: production без flag, notice отсутствует | PASS |
| Chromium: production с flag1, notice отсутствует | PASS |
| Localhost: register notice, anonymous me401, fixture owner login200, me200, demo200, logout | PASS |
| Полный повтор пяти длительных demo/multichannel E2E | NOT RUN, изменение только registration notice; прежние результаты10Г исторические |
| Реальный SMTP/API/внешнее письмо | NOT RUN, запрещено scope |

Логи/команды/exit codes и checks: `.local/demo-email/`. Screenshot без PII: `.local/audit8d/browser-results/demo-email-registration.png`. Production controls — отдельные временные web containers на той же internal demo network; auth integration — отдельный QA project/БД с safety marker, не данные demo или рабочего окружения. Controls и QA очищены отдельно; synthetic demo history сохранена.

Fresh proof записан только после новой сборки: BUILD_ID `s50d06azP1leffpqWswjg`, source SHA256 `b842e76ec8e2c843acca10b505fff635a77d58f6c75886c78afb360329e37ce7`. Старый proof сохранён в evidence; guards не изменены/не ослаблены. Новая успешная сборка не закрывает E01.

## Как войти

1. Открыть `.local/demo10d/manual-fixture.json` в локальном редакторе. Файл имеет mode0600; не вставлять его в чат/логи/скриншоты.
2. Взять `owner.email` и `owner.password`, ввести на **http://127.0.0.1:43010/login**.
3. Открыть **http://127.0.0.1:43010/dashboard/demo**, выбрать synthetic магазин. Новая регистрация не требуется.

Это обычный login заранее подтверждённого synthetic fixture owner, не обход email verification. Зарегистрированная реальная учётная запись остаётся неподтверждённой. Demo и loopback-only relay оставлены запущенными; инструкция безопасной остановки сохранена в DEMO_WALKTHROUGH.md.

Секреты в отчёт не включены; реальные SMTP/API, внешние письма, платные вызовы, commit/push/deployment не выполнялись. AGENTS.md, глобальные настройки, CI и PATH не менялись. Audit/receipts deadlines не устанавливались.
