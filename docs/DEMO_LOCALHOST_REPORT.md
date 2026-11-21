# Ограниченный этап localhost-доступа

30 октября2026. Security CONDITIONAL, E01 OPEN. HUMAN REHEARSAL — NOT RUN.

## Причина

Воспроизведено на Docker Engine29.9.2, context desktop-linux, отдельный project `aba-localhost-diagnostic`. Приложение running/Ready, слушает0.0.0.0:3000 **внутри контейнера**, `/login` из контейнера200. Это контейнерный listener, на host wildcard bind не создавался.

Compose и `HostConfig.PortBindings` запрашивают127.0.0.1:43010→3000. После запуска `NetworkSettings.Ports` содержит `3000/tcp: []`, `docker port` пуст, а host получает Connection refused. Web подключён только к internal bridge. Доказанная непосредственная причина — Docker не создал фактическую host-привязку в этой конфигурации; приложение и его auth не являются причиной отказа соединения. Это не утверждение о доказанном upstream bug или всех версиях Docker. Более глубокое объяснение реализации engine и изменение его глобальных настроек не требуются для локального исправления.

Evidence без env/секретов: `.local/demo-localhost-diagnostic/docker-port-evidence.json`, up logs, проверка200 изнутри контейнера; исходный overlay сохранён в `compose.demo-offline.before.yml`.

## Минимальное исправление

- В `infrastructure/compose.demo-offline.yml` удалён неработающий запрос ports. Существующий PUBLIC_ORIGIN127.0.0.1 и все mock/financial/security значения сохранены.
- Добавлен `infrastructure/demo-localhost.py`: стандартный Python TCP relay, host listener строго127.0.0.1. Через `docker exec -i … node` передаёт байты в127.0.0.1:3000 существующего web. Никакого HTTP auth/proxy обхода: headers/cookies/Origin и responses идут без переписывания.
- Helper допускает только running web из `aba-demo10d-local` и проверяет internal=true всех его сетей. Не создаёт сети/маршруты, не меняет firewall/Docker daemon, не монтирует Docker socket. Нужен имеющийся доступ локального оператора к Docker CLI; предоставление новых привилегий не выполнялось.
- Ctrl+C закрывает соединения через EOF; helper не вызывает terminate/kill. Никаких зависимостей/скачиваний.

## Проверки

| Проверка | Итог |
|---|---|
| Python syntax; запуск/restart/SIGINT stop | PASS |
| Host127.0.0.1 `/login` | PASS200 |
| Anonymous demo | PASS: не содержит demo data, Next streaming NEXT_REDIRECT→login |
| Неверный пароль | PASS401 |
| Synthetic owner login | PASS200 |
| Authenticated demo, Telegram/WhatsApp URL, actual inventory | PASS200 |
| Неверный Origin / отсутствующий CSRF на mutation | PASS403/403, channel config не менялась |
| Host listener | PASS: lsof только127.0.0.1:43010; wildcard/IPv6 listener отсутствует |
| Доступ через LAN адрес host | Connection refused |
| Доступ через локальный VPN интерфейс | HTTP недоступен: RemoteDisconnected; raw TCP принимается VPN, это не ответ demo |
| Current artifact и current artifact served | PASS, строгие guards без изменений |
| Release proof и recovery.go | Byte/hash unchanged |
| Повторные Go/PG/full browser gates | NOT RUN: нет изменений application/DB; результаты10Г исторические |
| Человек/другое физическое устройство | NOT RUN |

Первый verifier ошибочно ожидал обычный redirect вместо Next streaming redirect и считал любое TCP handshake доступом к demo. FAIL сохранён в `verify-initial-fail.log`; проверка исправлена на отсутствие protected content + login redirect и реальный HTTP ответ. Итоговые результаты — `verification.json`, `verify.log`, `integrity.log`, `restart.log`. Проверки внешних адресов выполнялись только на собственных интерфейсах этой машины, не в интернете. Они вместе с точным loopback bind подтверждают отсутствие публикации demo на внешних интерфейсах; проверка с другого устройства не выполнялась.

После проверки relay остановлен, `aba-demo10d-local` остановлен с сохранением volume/credentials/history. Диагностический project убран без `-v`. Рабочие проекты не изменялись. Synthetic credentials сохранены0600 только в `.local/demo10d/manual-fixture.json`; содержимое не включено в отчёт.

## Ручной запуск

Полные копируемые команды, подготовка нового seed, выбор порта и безопасная остановка: [DEMO_WALKTHROUGH.md](DEMO_WALKTHROUGH.md).

1. Из корня определить функцию `demo` с четырьмя Compose файлами из runbook, выполнить `demo up -d api worker web`, затем integrity verify/served.
2. Использовать уже подготовленный `.local/demo10d/manual-fixture.json` либо создать новый seed по runbook.
3. Выполнить `python3 infrastructure/demo-localhost.py` в foreground.
4. Открыть http://127.0.0.1:43010/login, войти synthetic owner из приватного JSON, затем http://127.0.0.1:43010/dashboard/demo и выбрать магазин.
5. Завершить Ctrl+C, дождаться сообщения остановки, выполнить `demo stop`. Не использовать down -v для операторской истории.

HUMAN REHEARSAL — NOT RUN: техническая доступность исправлена, человек должен отдельно пройти сценарий. E01 OPEN, Security CONDITIONAL. Audit/receipts сроки не установлены. Бизнес-логика, worker code, RLS, авторизация, gates, базовая инфраструктура, CI, AGENTS.md, PATH и глобальные настройки не изменены. Внешние API/paid calls, установки, commit/push/deployment отсутствуют.
