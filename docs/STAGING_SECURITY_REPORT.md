# AI Business Assistant — безопасность staging

**Host и runtime security — PASS в границах staging.** OpenSSH принял ключ хоста из `known_hosts`. `aba-admin` создан с отдельным `authorized_keys`, home и .ssh 0700, файл 0600. Проверены независимые входы до и после reload, ограниченный sudo и отказ произвольного sudo. OpenSSH `PasswordAuthentication no`, `KbdInteractiveAuthentication no`, root разрешён только по ключу. UFW default deny incoming/routed разрешает 22, 80 и 443. Входной Nginx использует Ubuntu backports, действующий HTTPS с обоими SAN, Basic Auth и запрет публичных webhooks. Сертификат прошёл симуляцию продления.

## Установленные свойства исходников

- Private Compose изолирует все сервисы во внутренней сети и запрещает публикацию портов через `scripts/deployment/common.py`. У приложения нет привилегированных контейнеров или mount Docker socket. Входной Nginx для двух публичных имён требует отдельной проверенной конфигурации.
- Официальный образ PostgreSQL использует SCRAM и TLS. Подключения приложения и мигратора разделены. На уровне Go API проверяются роль `aba_app`, FORCE RLS и версия схемы. Deployment-скрипт сверяет исходную схему 16 с 45 FORCE RLS таблицами и схему 29 с 62. У обеих версий проверяются нулевой unforced count, ограниченная роль и отсутствие строк организаций без tenant-контекста. Изоляция подтверждена двумя разными синтетическими сессиями в рабочем staging.
- Immutable runtime admission требует mock LLM, Telegram mock, отключённые внешние вызовы и отправку, включённые kill switches и нулевые бюджеты LLM/WhatsApp. Instagram проверяет запрет live, внешних вызовов и ненулевого бюджета. Учётные данные Meta, Telegram, 1C и реального LLM не использовались.
- Go staging runtime требует `APP_ENV=staging`, HTTPS origin, `Secure` cookies и PostgreSQL `verify-full`. Mock Telegram и LLM доступны только при безопасном staging admission и независимых ключах. Production отклоняет mock LLM и Telegram. Instagram обычного staging выключен. Офлайн-тест Instagram использует отдельную сборку, тестовый marker и отдельную БД.
- Файлы секретов существующего deployment-контракта хранятся вне Git и проверяются по типу, правам и владельцам. На VPS нужно проверить native UID для файлов и контейнеров. Локальный поиск по Git history дал ноль совпадений для ограниченных шаблонов приватных ключей, GitHub/AWS и `sk-` токенов. Проприетарные форматы, пароли и новый release artifact остаются вне покрытия этого поиска. Кандидат собран из чистого Linux snapshot, а его immutable manifest и конфигурация прошли native preflight. Полный scan образов и attestation publisher остаются открытыми.

## Условия допуска на VPS

1. Проверить host fingerprint через консоль провайдера. Создать `aba-admin` с отдельным `authorized_keys`, каталогом `.ssh` 0700 и ключами 0600. Права sudo ограничить операциями сопровождения. Docker administration даёт эффективный доступ уровня root и требует отдельного контроля.
2. Подтвердить независимый SSH-вход и sudo, сохранив действующую root-сессию и рабочий консольный recovery. Проверить `sshd -t` и эффективные правила. Выполнить reload и повторно проверить независимый вход. Изменение доступных способов аутентификации допускается после этих проверок.
3. Разрешить 22/tcp до включения UFW. Внешние слушатели ограничить 22, 80 и 443. Проверить отсутствие публикации PostgreSQL, mail sink, worker и Go/Next.js upstream, затем выполнить внешний сетевой smoke.
4. Проверить DNS и TLS для обоих имён. Подтвердить обработку `Host`/`Origin`, выдачу cookies, login/logout, CSRF, доступ между организациями, сохранение audit logs и отключённые внешние провайдеры. Установить, какие API endpoints допускаются через Nginx, с учётом OpenAPI и middleware репозитория.

## E01 и supply chain

E01 остаётся **OPEN**. Историческое исследование показывает неожиданные пустые директории `.next 2` и `node_modules 2`. Они меняют standalone digest. Источник появления директорий не установлен. В текущем этапе прошли локальные регрессии artifact guard. Для VPS нужны свежая Linux-сборка и проверка исходников, manifest, digest и startup guard. Исторические артефакты сохранены. Кандидат прошёл проверки целостности на VPS. Кандидат запущен после GO владельца. Сервисы и конфигурация проверены после общего перезапуска. [Расследование E01](BUILD_INTEGRITY_INVESTIGATION.md) · [Историческая проверка выпуска](STAGE_12C_PRE_VPS_SECURITY_REPORT.md)


Действующая среда подтвердила `__Host-aba_session` с Secure, HttpOnly и SameSite=Lax, Secure/Strict CSRF-cookie, отказ cross-tenant и проверку двух реальных PostgreSQL runtime-соединений через TLS. В Go API и worker сверены 19 safe flags и отсутствие внешних provider credentials. Instagram runtime и live delivery выключены. Basic Auth и блокировка публичных webhook маршрутов сохранились после restart.
