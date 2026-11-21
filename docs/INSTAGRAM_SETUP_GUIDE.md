# Instagram — подготовка

Подключение к реальному аккаунту сейчас недоступно. Это руководство описывает безопасную конфигурацию текущего кода и следующие шаги.

## Текущий режим

В `.env.example` добавлены `INSTAGRAM_ENABLED=false`, `INSTAGRAM_LIVE_ENABLED=false`, `INSTAGRAM_EXTERNAL_REQUESTS_AUTHORIZED=false`, `INSTAGRAM_SENDING_KILL_SWITCH=true`, `INSTAGRAM_MONETARY_BUDGET_RUB=0`.

Зарезервированные APP_SECRET/ACCESS_TOKEN/ENCRYPTION_KEY/VERIFY_TOKEN остаются пустыми. `config.Load` отвергает непустые Instagram credentials и попытки включения. Новые зависимости и загрузки не нужны. Пример не является конфигурацией работающего OAuth.

Страница каналов показывает «Не подключён»; кнопка подключения недоступна. Account name, username, connection date, subscription и permissions ещё не получаются из Meta.

## Будущий onboarding

После реализации database/worker gates владелец магазина запускает server-side OAuth. State должен быть одноразовым, храниться как hash, жить 10 минут и быть связан с tenant/user/session hash/точным redirect URI. Callback проверяет текущую сессию и атомарно потребляет state до exchange, в том числе при отказе пользователя.

Для Instagram Login нужны `instagram_business_basic` и `instagram_business_manage_messages`. Поддерживаемая целевая аудитория — Professional Business/Creator; Personal не включать. Официальная модель использует Instagram User access tokens, `api.instagram.com` для code exchange и `graph.instagram.com` для long-lived/refresh и messaging. [Источник Meta](https://developers.facebook.com/documentation/instagram-platform/instagram-api-with-instagram-login/business-login).

Long-lived token хранить зашифрованно с tenant/integration/account AAD; expiry рассчитывать из ответа API. Refresh допустим для действующего long-lived token возрастом не менее суток. Не логировать OAuth query strings, коды, token request/response или Authorization.

Проверить identity и permissions, затем app/account subscriptions и получение подлинного события. Только совокупность подтверждений позволяет показать «Подключён». Disconnect должен отзывать доступ, отменять jobs и fences, удалять credentials; reconnect проходит полную проверку снова. Эта orchestration пока не реализована.

Реальные URL OAuth/webhook не регистрировать по предположению: backend routes ещё отсутствуют. Подготовить HTTPS домен и exact callback после фиксации API contract.
