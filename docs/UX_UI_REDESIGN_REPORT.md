# UX/UI redesign — 17 ноября 2026

Статус: **редизайн реализован, локальные frontend-проверки пройдены; полная интеграционная приёмка BLOCKED**. Это не разрешение на deployment и не security clearance релиза.

## Исследование и реальные проблемы

Изучены Next.js App Router, layouts, CSS/Tailwind, существующие компоненты, session/organization context, роли, формы и подготовленные offline runners. Применимые инструкции: корневой AGENTS.md и quality routing в AI_DEV_SYSTEM_INSTALL_PLAN.md. Новые зависимости и вторая UI-библиотека не добавлены.

Карта существующих экранов:

| Область | Реальные маршруты |
|---|---|
| Публичная | `/`, `/login`, `/register`, `/verify-email`, `/status` |
| Обзор | `/dashboard` |
| Заказы | `/dashboard/orders`, `/dashboard/orders/[id]` |
| Товары | `/dashboard/products`, `/new`, `/[id]`; `/dashboard/categories`, `/inventory`, `/catalog` |
| Общение | `/dashboard/conversations`, `/dashboard/ai-playground` |
| Помощник | `/dashboard/ai-settings` |
| Каналы | `/dashboard/integrations`, `/telegram`, `/whatsapp`; `/dashboard/operations/whatsapp` |
| Начало работы | `/dashboard/onboarding`, `/dashboard/demo` |
| Магазины | `/dashboard/organizations`, `/new`, `/dashboard/settings` |

Billing-страницы нет; новая пустая функция не создавалась. Вложенные `/new`, `/[id]`, `/telegram`, `/whatsapp` в таблице относятся к соответствующему родительскому маршруту.

- **P0:** новых подтверждённых P0 UX-дефектов не заявляем. Auth/tenant isolation, staff approval, FORCE RLS, freshness и финансовые ограничения — защищённые инварианты. Отдельный существующий blocker: установленный Next.js сообщает об известной уязвимости.
- **P1:** шесть тесных пунктов мобильной навигации; каналы спрятаны среди дополнительных переходов; таблицы заказов и товаров неудобны на телефоне; идентификаторы и технические сведения конкурируют с решением по заказу; формы и подписи местами слишком мелкие; зелёный акцент смешивается со статусом успеха.
- **P2:** дублирование общей и кабинетной навигации; разная иерархия карточек и заголовков; недостаточно заметный текущий раздел и контекст возврата.
- Найдено в браузере и исправлено: отсутствующая иконка сайта создавала 404; обратный Tab в нативном диалоге Chrome мог уводить фокус за его пределы.

## Что изменено

**Общая система.** Семантические light/dark токены: фон, поверхность, текст, рамки контролов, primary/hover, success/warning/danger/info; размерные токены, сетка 4/8 px, радиусы. Основной текст и поля 16 px, кнопки 14 px, второстепенные подписи компактнее. Видимые focus/hover/disabled состояния, reduced motion и сохранение существующего `aba-theme`. Контролы не обозначают смысл только цветом.

**Навигация.** Desktop: четыре основных раздела и три раздела управления. Мобильный: четыре подписанных раздела и «Ещё»; меню закрывается по Escape с возвратом фокуса. Все существующие дополнительные маршруты сохранены. Breadcrumbs сохраняют выбранный магазин. Убрано дублирование глобального header/footer в кабинете, оформление и выход доступны в account bar.

**Главная.** Текущий магазин и роль, реальные показатели существующих API, первые ожидающие решения заказы, задачи качества каталога и прямые следующие шаги. Новый блок заказов использует существующий GET endpoint; новых API нет. Недоступность данных не превращается в ноль или «всё хорошо». Последовательность начала работы — ссылки на реальные экраны, без выдуманных процентов выполнения.

**Заказы.** Сумма, статус, состав и решение вынесены вперёд; полные ID, версия и участники истории сохранены в раскрываемых подробностях. Даты отображаются в timezone магазина. На телефоне — карточки вместо основной таблицы. Подтверждение через доступный native dialog: безопасный начальный фокус, Tab/Shift+Tab внутри, Escape/отмена, возврат фокуса. Изменение выполняется только после явного подтверждения; повторная отправка ограничена ref/busy. Сохранены разрешённые статусы, endpoints, expected_version, reason, idempotency_key и серверное staff approval. Отмена/отклонение визуально отделены от основного действия.

**Каталог.** Фильтры и формы получили общую систему контролов, товары — мобильные карточки с ценой, наличием и статусом. Сохранены поиск, категории, сортировка, редактирование, импорт, источники и stock/freshness ограничения.

**AI и каналы.** Отдельные пункты «AI-ассистент» и «Каналы связи». Telegram/WhatsApp представлены симметрично; тестовая настройка не называется реальным подключением. Ошибка чтения даёт неизвестное состояние. Существующие настройки и операции сохранены, live не включался.

**Остальные экраны.** Общие tokens/контролы/карточки распространяются на публичные формы, настройки, категории, склад, onboarding, демонстрацию и операции. Их бизнес-логика не переписывалась.

Компоненты: обновлены Sidebar/Mobile navigation, ProductUI (PageIntro, MetricCard, StatusBadge, EmptyState, CatalogNavigation, icons); добавлены Breadcrumbs и UI primitives Button/Input/Select/Checkbox/Card/Alert/Toast/Table/LoadingState, а также переиспользуемый `useConfirmation`. Существующие формы и таблицы сохраняют native HTML; не все переведены на новые wrappers. Архивация и некоторые старые операции каналов пока используют существующий browser confirm.

## Файлы

19 frontend-файлов относительно сохранённого baseline:

```text
apps/web/src/app/premium.css
apps/web/src/app/icon.svg
apps/web/src/app/dashboard/layout.tsx
apps/web/src/app/dashboard/page.tsx
apps/web/src/app/dashboard/error.tsx
apps/web/src/app/dashboard/loading.tsx
apps/web/src/app/dashboard/settings/page.tsx
apps/web/src/app/dashboard/integrations/page.tsx
apps/web/src/components/AISettings.tsx
apps/web/src/components/Breadcrumbs.tsx
apps/web/src/components/CatalogWorkspace.tsx
apps/web/src/components/ProductUI.tsx
apps/web/src/components/Sidebar.tsx
apps/web/src/components/UI.tsx
apps/web/src/components/Workflows.tsx
apps/web/tests/auth.spec.ts
apps/web/tests/demo-production.spec.ts
apps/web/tests/multichannel-production.spec.ts
apps/web/tests/ux-production.spec.ts
```

Также созданы этот отчёт и приватный `.local/ux-redesign-20261117/INDEX.md`. SHA256 до/после frontend-файлов находятся в `source-before.json` и `changed-files.json`. Незавершённые серверные документы и staging Git index не перезаписывались. Go, SQL, migrations, RLS, auth/session implementation, provider configuration, production environment и dependency locks не изменялись. Commit/push/deployment не выполнялись.

## Проверки фактического исходного кода

| Проверка | Статус и доказательство |
|---|---|
| TypeScript | PASS, `npm run typecheck`, `typecheck-final-all.log`, exit 0 |
| ESLint | PASS, `npm run lint`, `lint-final-all.log`, exit 0 |
| Native build preflight | PASS, 5 Node tests, `preflight-tests.log` |
| OpenAPI | PASS, существующий schema/endpoint check, `openapi.log` |
| Artifact guard unit test | PASS, 1 тест с synthetic proof, `artifact-tests.log`; это не проверка release provenance |
| Next production compilation | PASS на обычном повторном запуске, exit 0, `build-focus-fix-retry.log` |
| Next security warning | **FAIL / существующий blocker**: первоначальный запуск завершён SecurityFatalError, `build-focus-fix.log`; повторный запуск разрешён самим CLI и сохраняет предупреждение. Код guard/dependencies не менялся; это не устранение CVE |
| Current source ↔ isolated build | PASS, `source-match.json`; source/config/tests совпадают с отдельным build workspace |
| Production UI browser | PASS, Chromium/установленный Chrome, 16 экранов × 6 ширин = 96 route/viewport checks, `browser-results.json` |
| Существующие Playwright smoke tests | PASS, 3/3 выбранных tests: публичная главная/серверный redirect неавторизованного кабинета, loading/недоступность и неверный status contract; `playwright-smoke.log`. Тест реального Go backend status не запускался |
| Публичные страницы | PASS, 5 маршрутов × 6 ширин = 30 checks; пустой login не отправляет POST, `public-results.json` |
| Мобильная навигация / клавиатура | PASS, active route, подписи, targets ≥44 px в основной навигации, «Ещё», Escape, skip-link |
| Диалог заказа | PASS, начальный фокус, цикл Tab/Shift+Tab, отмена/Escape без POST, возврат фокуса, двойное нажатие даёт один POST; version/idempotency сохранены, 409 объявляется через alert |
| Поиск / пустые состояния | PASS на synthetic API: найденный товар, отсутствие совпадений, пустой список завершённых заказов, недоступность сводки |
| Light/dark / reduced motion | PASS, persisted theme; contrast body 17.06/16.66, primary CTA 5.17/8.43; reduced motion `0s` |
| Расширенный contrast check | PASS в проверенной выборке, 1 552 видимых текстовых узла, 15 экранов × 2 темы, `contrast-results.json`; обычный текст ≥4.5, крупный ≥3 |
| Labels / overflow | PASS на 96 checks; 320, 375, 390, 768, 1024, 1440 px; обоснованные таблицы имеют отдельный scroll region |
| Внутренние ссылки | PASS, 22 различных rendered path соответствуют production route manifest; это проверка маршрутов, а не готовности данных за каждым endpoint |
| Browser console | PASS для JS/runtime: 0 исключений и 0 неожиданных console errors; 25 ожидаемых HTTP 503/409 сообщений при намеренных error-сценариях сохранены |
| Diff whitespace | PASS, `git diff --check -- apps/web` |
| Полный prepared Go/PostgreSQL browser E2E | **BLOCKED / NOT RUN**: локальный Docker socket отсутствует, `docker-prerequisites.json`; Docker не запускался |
| Полные auth/orders/RLS/stock/tenant проверки | **NOT RUN**; существующие Playwright tests обновлены под dialog, но их assertions не выдаются за выполненные |
| Полный wizard/import/demo/financial recovery с реальным backend | **NOT RUN**; страницы/ошибки проверены, бизнес-операции не исполнялись |
| Screen reader, axe, полный WCAG 2.2 AA audit | **NOT RUN**; зависимости не загружались. Автоматические DOM/contrast/keyboard checks не являются сертификацией WCAG |
| Vulnerability scan / dependency upgrade | **NOT RUN**, не входят в разрешённое frontend изменение |

Browser harness использует отдельную loopback synthetic API на `127.0.0.1:19080` и production Next на `127.0.0.1:19082`. Все товары, пользователь и заказы вымышлены; fixture существует только в игнорируемом `.local`, не импортируется продуктом. Проверка POST подтверждения намеренно получает 409; реальный резерв/успех не симулируется как доказательство backend. Метрики приложения берутся из API, mock values в production source не добавлялись.

Сборка выполнялась в `.local/ux-redesign-20261117/after/apps/web`, с установленными зависимостями, без замены рабочей `.next` и без rerecord/rebaseline E01 proof. E01 остаётся OPEN. Прямой UI harness не заменяет guarded release runner.

Первичные FAIL сохранены: неверный cwd build runner; Turbopack/symlink проблема baseline (перешли на штатный webpack); security предупреждения Next; первая ошибочная формула прозрачного фона в test harness; отсутствующий favicon; клавиатурный цикл диалога. Логи не удалялись и не выдаются за PASS.

## Скриншоты

Одинаковый synthetic магазин; baseline снят в dev webpack, after — production build. Первичные снимки loading не используются для сравнения.

- [До, desktop](../.local/ux-redesign-20261117/before-ready-1440.png)
- [После, desktop](../.local/ux-redesign-20261117/after-1440-dashboard.png)
- [До, mobile](../.local/ux-redesign-20261117/before-ready-390.png)
- [После, mobile](../.local/ux-redesign-20261117/after-390-dashboard.png)
- [Мобильный заказ](../.local/ux-redesign-20261117/after-390-orders-22222222-2222-4222-8222-222222222222.png)
- [Товары](../.local/ux-redesign-20261117/after-390-products.png)
- [AI-ассистент](../.local/ux-redesign-20261117/after-1440-ai-settings.png)
- [Каналы](../.local/ux-redesign-20261117/after-1440-integrations.png)
- [Тёмная тема](../.local/ux-redesign-20261117/after-390-dark.png)
- [Подтверждение заказа](../.local/ux-redesign-20261117/after-confirmation-mobile.png)

## Что осталось для полной приёмки

1. В доступном изолированном Docker/Go/PostgreSQL окружении выполнить существующие реальные auth/order/catalog/onboarding/demo/multichannel suites. Проверить staff confirm → реальный резерв → отмена/завершение, tenant/member ограничения и импорт. Не направлять тесты на рабочую БД.
2. Отдельным согласованным security изменением разобраться с предупреждением Next.js и пройти актуальные advisory/release gates. UI-сборка не означает trusted release.
3. Провести screen-reader и ручной end-to-end аудит с владельцем магазина; проверить успешные populated wizard/import/financial states, которые synthetic error harness не покрывает.
4. При дальнейшей унификации перевести оставшиеся browser confirms на общий dialog без изменения бизнес-правил. Это не блокирует существующие действия, но визуальная система диалогов ещё не охватывает все старые операции.

Нагрузка, коммерческая capacity, readiness каналов, безопасность backend и разрешение на staging этим этапом не утверждаются.
