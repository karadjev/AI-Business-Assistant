# Stage 2.6.2 — Instagram Offline Delivery Completion

## Границы тестового режима

Предыдущий [Safety Gate](STAGE_2_6_2_INSTAGRAM_GATE_REPORT.md) подтвердил защитный запрет и выявил отсутствие исходящего пути через worker. Сохранена обычная сборка worker с прежним Instagram `SendOne`. Для offline-прогона нужно одновременно собрать тот же worker с тегом `instagramoffline`, запустить его с `APP_ENV=test` и явным режимом mock-провайдера, а также установить тестовую SQL-фикстуру в отдельной БД `aba_test` с маркером `aba-synthetic-isolated`. Тестовый runner дополнительно требует пустую схему до миграций и отсутствие маршрута по умолчанию внутри контейнера.

SQL-фикстура хранится в `tests/integration/testdata` и устанавливается после обычных миграций. Она использует существующие `channel_outbox`, `channel_events` и `agent_audit`, сохраняет `sending_enabled=false` и ограничивает очередь квитанциями Auto Service для синтетических tenants. Фикстура проверяет связь подключения, получателя, диалога и заказа, активность подключения, permissions, webhook, срок токена, окно переписки и согласие. Задание создаётся из результата Business Manager с ID его задачи. Worker повторяет проверки перед отправкой, сверяет версию и статус заказа и состояние резерва.

Tagged worker исполняет настоящий цикл очереди, расшифровывает синтетический токен для связанного аккаунта и вызывает существующий `OfflineTransport`. Провайдер обрабатывает HTTP-представление сообщения в памяти. Обработка и сохранение результата mock-вызова происходят в одной транзакции PostgreSQL. Подтверждённый временный отказ сохраняет срок следующей попытки. Постоянный отказ и неопределённый результат имеют разные состояния. Новый worker читает задания из той же БД.

## Изменённые файлы

- `apps/api/cmd/worker/main.go`, `instagram_default.go`, `instagram_offline.go` — выбор очереди в обычной сборке и сборке с тестовым тегом.
- `apps/api/internal/instagram/offline_queue.go` — обработчик offline-очереди и mock-провайдер без сетевого клиента.
- `apps/api/tests/integration/testdata/instagram_offline.sql` — проходящий через существующую очередь тестовый SQL-механизм и его проверки доступа.
- `apps/api/tests/integration/instagram_offline_test.go` — синтетические сценарии с реальными Go API handlers, отдельными процессами worker и PostgreSQL.
- Настоящий отчёт и локальный [индекс доказательств](../.local/stage-2-6-2-instagram-offline/INDEX.md).

## Команды

```sh
python3 .local/stage-2-6-2-instagram-offline/run_go.py linux-integration-compile linux-worker-build linux-offline-worker-build
python3 .local/stage-2-6-2-instagram-offline/run_go.py unit race vet offline-unit offline-vet
sh .local/stage-2-6-2-instagram-offline/run_integration.sh
ABA_IG_TEST_PATTERN=^TestDatabaseAndMigrations sh .local/stage-2-6-2-instagram-offline/run_integration.sh
git status --short --branch
git diff --check
```

Go API проверен через штатные HTTP-обработчики и реальные сессии в `httptest`. Обычный и тестовый `cmd/worker` запускались как отдельные процессы. Синтетическое подключение и согласие подтверждены через OAuth-фикстуру и подписанный webhook.

## Результаты

| Критерий | Статус | Результат |
|---|---|---|
| Offline Delivery | PASS | Подтверждение сотрудником через API, сохранение результата Business Manager, PostgreSQL outbox, отдельный worker, существующий Instagram adapter, mock provider, состояние `sent` и аудит для того же задания. |
| Safety Block | PASS | Обычная сборка оставила задание в `pending` без попытки. `sending_enabled=false` сохранён. Tagged worker отклонил запуск без SQL-разрешения и за пределами `APP_ENV=test`. |
| Согласие | PASS | При отсутствии opt-in записан `consent_required`, задания нет. После подписанного отзыва согласия ожидающее задание отменено до первой попытки. |
| Получатель и tenant isolation | PASS | Провайдер принял адрес привязанного получателя. PostgreSQL отклонила неправильного получателя. Чужая сессия видела ноль строк из outbox. |
| Отмена и актуальность | PASS | Отмена до отправки аннулировала ожидающее подтверждение с нулём попыток. Провайдер принял только квитанцию об отмене. |
| Повтор, retry и restart | PASS | Повтор команды подтверждения сохранил одно задание. Один процесс зафиксировал `429` и отложенный повтор. Следующий процесс обработал это же задание, аудит содержит одну успешную отправку. |
| Постоянная ошибка и недоступность mock | PASS | Сохранены `failed` и `delivery_unknown` соответственно. Повторный запуск сохранил оба терминальных исхода без дополнительных попыток. |
| Backend-регрессия | PASS | Полная PostgreSQL suite прошла на отдельной чистой БД со штатной схемой 00027. FORCE RLS подтверждён на 61 таблице. Хеши обоих worker и тестового бинарника совпали до и после прогонов. |
| Go проверки | PASS | Unit, race, vet, проверки с offline-тегом и сборки обеих версий worker завершились с exit 0. |
| Chromium | NOT RUN | Изменения ограничены изолированным тестовым режимом backend. |
| Live Delivery | BLOCKED | Live-отправка и OAuth с реальным Meta — NOT RUN. Offline-провайдер обрабатывает только имитацию запроса в памяти. Deployment — NOT RUN. |

## Git

`origin` настроен. Текущий `HEAD` указывает на `main` без коммитов. Других локальных или remote-tracking веток нет. В индексе уже есть большое количество ранее подготовленных файлов. Commit и push — **BLOCKED**, безопасной рабочей ветки в доступном checkout нет. SHA отсутствует. Stage, commit, push, merge и создание ветки в этом прогоне **NOT RUN**. Существующий индекс сохранён.
