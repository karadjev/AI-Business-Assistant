# Demo Experience · offline10Г

Только отдельный test tenant, mock AI/Telegram/WhatsApp, budget0 RUB. Вход обязателен; роли owner/staff/read-only сохраняют реальные права. Это не разрешение рабочего rollout, live, paid API или удаления audit/receipts. E01 OPEN, Security CONDITIONAL.

## Проверенный localhost-доступ

Дополнительная диагностика исправила доступ локальным relay. На Docker29.9.2 / desktop-linux запрошенный bind Docker присутствует в HostConfig, но фактический NetworkSettings.Ports пуст для этой internal-only конфигурации. Внутри web `/login` отвечает200. Demo overlay больше не запрашивает неработающую публикацию порта. Вместо неё `infrastructure/demo-localhost.py` слушает **только127.0.0.1**, передавая TCP через Docker stdio (`docker exec`) в127.0.0.1:3000 существующего web. Новая сеть, Docker socket mount, зависимости и внешние запросы не нужны. Все сети web проверяются как internal до запуска. Human rehearsal **NOT RUN**.

## Изолированная сборка после повторения E01

В исходном `.local/build-env/web/.next/standalone` повторно появились `.next 2` и `node_modules 2`; guard заблокировал запуск. Это дерево и его proof сохранены без очистки. Текущий demo использует отдельный локальный cache-workspace через `.local/ux10f/compose.isolated-build.yml`. Путь записан в `.local/ux10f/isolated-workspace.json`; это cache приложения, не глобальная настройка Codex. Override меняет только источники mounts `/apps/web`; readonly runtime, сети и env остаются прежними. E01 OPEN.

Для запуска обязателен этот подготовленный overlay и успешный proof именно новой сборки. При отсутствии cache/overlay остановиться; не подменять старым загрязнённым workspace, не скачивать и не переписывать proof.

## Если demo уже работает

Не запускайте второй relay поверх занятого43010. Проверьте `http://127.0.0.1:43010/login` и используйте существующий процесс. После обновления сборки обновите вкладку браузера, чтобы загрузить новые frontend chunks.

Для запущенного агентом relay PID записан в `.local/demo10d/relay.pid`. Перед остановкой сверьте процесс:

```sh
ps -p "$(cat .local/demo10d/relay.pid)" -o pid=,comm=,args=
```

Только если это `python3 infrastructure/demo-localhost.py`, отправьте обычный interrupt (не kill -9):

```sh
kill -INT "$(cat .local/demo10d/relay.pid)"
```

Затем `demo stop` с функцией ниже. История/volume сохраняются. Если PID не совпадает с helper, чужой процесс не останавливать.

## Точный запуск

Из корня проекта, используя уже подготовленные cached images/modules/binaries и текущий прошедший integrity guard production artifact. При недостающем cache остановиться, не устанавливать и не скачивать. Нужны существующие Docker CLI и Python3 со standard library.

В одном терминале определить команду для отдельного project:

```sh
demo() {
  docker compose --env-file .local/audit8d/compose.env \
    -f infrastructure/compose.environment.yml \
    -f infrastructure/compose.environment-smoke.yml \
    -f infrastructure/compose.prepared-build.yml \
    -f infrastructure/compose.demo-offline.yml \
    -f .local/ux10f/compose.isolated-build.yml \
    -p aba-demo10d-local "$@"
}
demo up -d api worker web
demo run --rm --no-deps --entrypoint sh browser -ec 'node /scripts/build-env/artifacts.cjs verify && node /scripts/build-env/artifacts.cjs served'
```

Если integrity FAIL — остановиться; proof не переписывать. Рабочие контейнеры и DSN не использовать. Для нового показа создать synthetic tenant (для уже подготовленного показа можно использовать существующий `.local/demo10d/manual-fixture.json`):

```sh
umask 077
mkdir -p .local/demo10d
DEMO_RUN=$(python3 -c 'import uuid; print(uuid.uuid4())')
demo run --rm --no-deps --entrypoint /artifacts/browserfixture browser demo "$DEMO_RUN" > ".local/demo10d/$DEMO_RUN.json"
python3 infrastructure/demo-localhost.py
```

Последняя команда остаётся в foreground. Открыть **http://127.0.0.1:43010/login**. Войти owner из приватного seed JSON; затем открыть **http://127.0.0.1:43010/dashboard/demo** и выбрать synthetic магазин. Прямой адрес: `/dashboard/demo?org=ORG_ID`, где ORG_ID — поле `org` того же JSON. Staff проверять отдельной session. Credentials не публиковать и не добавлять в отчёты.

Если43010 занят, не завершать чужой процесс. До `demo up` задать в этом терминале `export ABA_DEMO_PORT=43011`, запустить relay с `--port 43011` и открыть127.0.0.1:43011. Это локальная переменная процесса, не изменение PATH/global settings. PUBLIC_ORIGIN и relay port должны совпадать. Bind host изменить нельзя: helper всегда127.0.0.1. Нельзя использовать shared/working project или добавлять external network.

## Безопасная остановка и повтор

Нажать **Ctrl+C** в терминале relay, дождаться `Demo relay stopped`; затем в том же терминале:

```sh
demo stop
```

Relay закрывает подключения через EOF, без kill процессов. Stop сохраняет synthetic volume, заявки и audit history. Не выполнять `down -v` для операторского показа. Для продолжения: `demo up -d api worker web`, integrity verify/served, `python3 infrastructure/demo-localhost.py`; использовать прежний JSON. Новый UUID означает новый test tenant, а не reset старых данных.

Fixture принимает только postgres/aba_test + safety marker; приложение использует aba_app. Privileged seed остаётся CLI, не HTTP. UUID валидируется; IDs детерминированы, инициализация сериализуется advisory lock и completion audit receipt. Повтор не сбрасывает склад/каналы/заявки. Учебные orders создаются действующими services, contacts synthetic@example.test. Сроки audit/receipts не устанавливаются.

## Регистрация и письма

В этом offline demo регистрация сохраняет неподтверждённую учётную запись и verification token, а зашифрованное письмо — в email outbox. Worker передаёт его по SMTP в локальный Mailpit (`mailpit:1025`). Статус `sent` означает приём Mailpit, не доставку на реальную почту. Внешняя SMTP/API отправка отсутствует; Mailpit не опубликован на host. Использовать только synthetic email.

На странице регистрации показывается уведомление, включённое только `ABA_LOCAL_DEMO_EMAIL_NOTICE=1` в отдельном demo overlay при `APP_ENV=development`. В application production mode и без явного demo flag уведомление отсутствует. Флаг меняет только текст; verification и login остаются прежними. Публичной ссылки/обхода подтверждения нет.

Для входа без регистрации открыть `.local/demo10d/manual-fixture.json` **в локальном редакторе**, взять `owner.email` и `owner.password`, ввести на `/login`, затем открыть `/dashboard/demo`. Не вставлять содержимое JSON в чат, терминальные отчёты, screenshots или issue. Подготовленный synthetic owner уже подтверждён существующим CLI fixture; реальная новая регистрация этим не подтверждается. При необходимости работы с письмом использовать отдельный операторский локальный процесс; не публиковать catcher/verification tokens.

## Сценарий6:15 с объяснением

| Шаг | Рассказ и действие | План |
|---|---|---|
| Знакомство | Для владельца магазина электроники: каталог, мессенджеры, контроль сотрудника. Без обещаний ROI |45s|
| Настройка | Выбран test tenant; подключить только выбранный mock канал. Мастер профиля/импорта доступен по ссылке |45s|
| Каталог |7 товаров: Galaxy S25, iPhone16, чехол, charger30W, наушники, планшет, неполный кабель. Показать health и разные состояния |60s|
| Покупатель | Synthetic user700, сообщение через прежний ingress; это не настоящий клиент |30s|
| Ответ | Проверить устаревший/отсутствующий товар, затем свежий DEMO-S25. Цена69990 RUB — fixture, не рыночная котировка. Injection-текст не меняет каталог |45s|
| Согласие | Получить предложение из фактической доставки. Отдельно отметить согласие и отправить callback. Без согласия заявки нет |45s|
| Заявка |pending_confirmation, резерв0. Повторить тот же event; заявка одна. Refresh сохраняет backend state и URL шага |30s|
| Сотрудник |Отдельная staff session или owner с правами: подтвердить, проверить резерв, выполнить handoff. Автоответы после handoff остановлены |45s|
| Результат |Фактические сообщения/ответы/заявки текущего диалога, учебные примеры отдельно. Обсудить prerequisites будущего пилота |30s|

Учебные заявки относятся к DEMO-CHARGER и уже могут иметь резерв/списание. Основной показ и проверки отсутствия резерва до сотрудника относятся к DEMO-S25; итог текущего channel conversation не включает эти начальные примеры.

Snapshot polling соблюдает Retry-After при429 и не показывает отсутствие диалога как установленный факт при ошибке загрузки. Повторяются только чтения; POST-события автоматически не пересылаются.

Telegram и WhatsApp имеют один маршрут и одинаковые основные действия. WhatsApp-specific window/template/provider/financial/signature/unknown gates не выключаются. Доставка mock отображается только в статусе sent; неопределённая доставка не запускает автоматический повтор.

Таймер маршрута показывает browser elapsed, не экономию времени. Презентационный план6:15 включает чтение/объяснение; быстрый автоматический click-path может быть короче. E2E в режиме ABA_DEMO_PRESENTATION=1 отдельно фиксирует narration и interaction duration; это автоматизированный показ, не human usability study. Не выдавать его за измерение времени реального клиента.

Для чистого повторного показа создать новый run/test tenant. Возврат к старому run продолжает реальные записи, не очищает историю. Не использовать delete conversation/reset inventory как скрытый reset. Между повторными показами использовать stop собственного demo project, не down -v: история остаётся в его volume. Одноразовые QA projects отделены от этого операторского контура. Завершать только собственный demo project; seed/run UUID и evidence сохранять по существующим ограничениям, не назначая audit retention.

## Безопасные границы

Шаг/канал в URL — навигация, не разрешение. Каждый API проверяет права и tenant. Отсутствие mock mode блокирует действия; UI не включает live. Initial fixture orders internal_chat помечены как учебные, не входят в outcome текущего channel conversation. Полная сводка заявок не выдаётся при достижении ограничения20 записей первой страницы. Данные старого snapshot при ошибке явно обозначаются; fake success нет. Простые states/счётчики не заменяют security/live readiness.
