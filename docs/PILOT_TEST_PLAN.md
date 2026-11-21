# Актуализация 8Г — 21.10.2026

**CONDITIONAL / NOT READY.** Независимо повторены Go tests/vet/build, PostgreSQL integration (**24.514s**),32-table FORCE RLS, tenant/channel isolation, order/reserve/recovery/budget regression. Реальных API не было, оба бюджета0 RUB, live off.

Go race **BLOCKED**: в isolated runner нет C compiler; host clang17 есть, но дополнительный host network sandbox не разрешён (`sandbox_apply: Operation not permitted`). Никакого обхода/повышения прав. Next production build **BLOCKED**: недоступны Linux GNU SWC16.4.0/Oxide4.3.3/lightningcss1.32.0. Теперь prebuild завершает процесс42 при missing/mismatch/unloadable native modules; тесты gate PASS5. Это не успешная Next build.

Текущие production browser E2E **Telegram BLOCKED / WhatsApp BLOCKED**. Cached stage7 browser и current TSX/BFF PASS имеют ограниченную область и их не заменяют. Для WA после fresh build нужен отдельный полный browser spec. Подробные актуальные доказательства и команды: [STAGE_8G_REPORT](STAGE_8G_REPORT.md), [GO_RACE_READINESS](GO_RACE_READINESS.md), [NEXTJS_BUILD_RECOVERY](NEXTJS_BUILD_RECOVERY.md), [FULL_BROWSER_E2E_REPORT](FULL_BROWSER_E2E_REPORT.md).

Ниже сохранена история предыдущего этапа; её результаты не выдаются за повторные результаты8Г.

---

# План будущего контролируемого пилотного теста

20.10.2026. **ПЛАН, НЕ РАЗРЕШЕНИЕ. Сейчас внешние API запрещены, Telegram/WhatsApp sending off, LLM mock/0 RUB, WhatsApp0 RUB. Реальные покупатели запрещены.**

## До любого live-действия

Ответственный разработчик закрывает Go race и свежую production frontend build, запускает полные текущие SSR/browser E2E обоих каналов, повторяет security/RLS/queue regression. Владелец утверждает synthetic test organization, участников/allowlist, обработку/срок хранения данных (предложение7days), временной интервал и контакты остановки. Для WA требуется подготовить guarded live transport, затем сверить Graph version, window/templates и тарификацию по актуальным официальным источникам в отдельно разрешённой среде. Не включать платные операции при budget0 или неизвестной стоимости.

| Шаг | Требует отдельного разрешения | Проверка успеха | Остановка/откат |
|---|---|---|---|
| Вручную создать отличимый тестовый Telegram bot через BotFather | Да, владелец | Отдельный test bot, нет рабочих пользователей | Отозвать токен через BotFather |
| Локально передать токен через ignored server env/secret store | Да для настоящего токена | Не в чате/Git/frontend/logs; encryption key отдельно от DB | Удалить secret и ротировать |
| Однократно getMe | Да, точный бот/операция | Bot ID соответствует ожидаемому, no cross-org ownership | Sending off; не продолжать при mismatch |
| Утвердить HTTPS endpoint и certificate | Да | Secret integration route, valid TLS, size/rate controls | Снять публичный ingress; туннели не запускать автоматически |
| Установить/проверить webhook | Да отдельно для setWebhook/getWebhookInfo | integration-bound secret, минимальные update types, inbox ACK | После отдельного разрешения deleteWebhook, отправку выключить локально сразу |
| Создать Meta App/WABA/тестовый номер и credentials | Да для каждого внешнего шага | Официальный test contour, isolated phone, согласованные IDs | Deactivate channel/revoke credential |
| Проверить WA verification/HMAC webhook | Да | Challenge только GET, POST HMAC по raw body, correct phone/org | Отключить подписку в разрешённой Meta операции |
| Разрешить отправку конкретным test accounts | Да отдельно по каждому каналу/интервалу | Allowlist до conversation/send, mock/live ясно виден | Kill switch обоих каналов; pending jobs не отправлять |
| Synthetic спросить товар/цену/нет товара | Да на реальные сообщения | Ответ совпадает с own catalog, stale stock не обещает наличие | Handoff, captured correlation IDs без полных переписок |
| Явное quote-bound подтверждение | Да | Одна pending заявка; reserve0 до staff, confirm reserve, cancel/reject invariant | Cancel synthetic reservation один раз |
| Проверить WA window/template ограничения | Да, без платного send | Вне окна отказ free text, NEEDS_APPROVED_TEMPLATE | Никакие templates автоматически не создавать/не отправлять |
| Ошибки/retry/UNKNOWN/аудит | Да на контролируемые live действия | Неопределённый send не повторяется, manual resolution с audit | Не имитировать ошибку на настоящих клиентах |
| Отключить канал | Локальный kill разрешён оператору; provider операции отдельно | Новые/queued send blocked, inflight outcome inspected | Не resend UNKNOWN |
| Завершить тест/удалить synthetic data | Только утверждённая test organization | Снять резервы, purge/retention, сохранить минимальный audit | Не удалять чужие volumes/данные |

## Контроль результата

Ответственный QA сохраняет даты, версии, integration ID/correlation ID без токенов, exact operation approval, число сообщений, коды ответов/доставки и стоимость либо её неопределённость. HTTP200 не равен корректному ответу/доставке. Telegram и WhatsApp получают отдельные результаты; разрешение одного не включает другой. LLM остаётся mock — этот план не требует положительного LLM бюджета.

Критерии остановки: signature/routing mismatch, foreign data, duplicate order/reserve, утечка секрета, неожиданный расход, отправка неallowlisted пользователю, runaway retries. Сразу локальные kill switches; не пытаться «исправить» неопределённую доставку повторной отправкой. Секрет ротировать через отдельно разрешённую provider процедуру, оценить инцидент и только затем планировать повтор.

Резервная копия и rollback проверяются на synthetic contour; старые migrations1–10 не редактировать. При необходимости отката нового приложения сначала disable ingress/sending, остановить worker, сохранить очередь/UNKNOWN, восстановить совместимые API/worker вместе; destructive DB downgrade не выполнять автоматически. Перед реальными покупателями нужен отдельный правовой/операционный этап, этот тест его не заменяет.
