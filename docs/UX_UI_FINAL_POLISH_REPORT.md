# UX/UI final polish — 17.11.2026

**Frontend: целевые проверки PASS. Можно проводить управляемую локальную демонстрацию на синтетическом магазине. Готовность SaaS к production, staging или работе с клиентскими данными не подтверждена.**

Это продолжение [редизайна](UX_UI_REDESIGN_REPORT.md), без нового branding, библиотек, зависимостей и backend-изменений. Evidence: `.local/ux-final-polish-20261117/`. Предыдущие файлы, сборки и FAIL сохранены.

## Подтверждённые проблемы и исправления

| Приоритет | Было | Стало |
|---|---|---|
| P1 | Приветственный блок и повторные заголовки отодвигали заказы ниже первого экрана | Одно название магазина, компактный контекст и действие; задачи перед метриками |
| P1 | Стрелка не объясняла действие; период счётчиков не был ясен | Вся карточка — ссылка с явным действием. «Все заказы», все статусы, без ограничения по дате |
| P1 | Длинное demo-имя/email с UUID в верхней строке | Понятное имя, fallback «Пользователь», «Демо-пользователь» для соответствующего fixture. Email только в раскрываемых данных аккаунта; выход там же |
| P1 | Пустой каталог и отсутствие совпадений объяснялись одинаково | Первый товар: одна основная кнопка и ссылка на Excel-мастер; фильтр без результатов: сброс поиска с сохранением магазина |
| P1 | В списке заказов не было даты; в диалогах не было последней активности | Реальные `created_at` / `updated_at`, русский формат и timezone магазина. При отсутствии даты она не выдумывается |
| P1 | Обновление ошибки диалога переводило в AI playground и теряло выбранный диалог/канал | `router.refresh()` текущего экрана сохраняет URL и контекст |
| P1 | Сетевой отказ операции мог показать техническое `Failed to fetch` | Понятное предупреждение об отсутствии ответа и необходимости проверить статус перед повторным действием; API errors/codes не менялись |
| P1 | При первой проверке новой компактной строки заказа текст сжимался на телефоне | Два уровня: номер/статус, затем сумма/переход |
| P1 | В первой версии polish при CSS zoom 200% метрика выходила за экран на 6 px | Сетка следует доступной ширине; переписка перестраивается по ширине контейнера |
| P1 | Открытые меню профиля и смены магазина могли уходить за левый край узкого экрана | Ограничена ширина профиля; мобильный список магазинов раскрывается от левого края кнопки. Проверены открытые меню на всех шести ширинах |
| P1 | Skip-link мог потерять фокус при замене временного dashboard main | Loading main помечен aria-busy; существующая логика skip-link ждёт финальное содержимое. Проверка не ослаблялась |
| P2 | Постоянная подсказка настройки в sidebar, даже для настроенного магазина | Подсказка убрана из sidebar; на главной раскрываемая настройка только при известном статусе, отличном от ACTIVE |
| P2 | NOT_STARTED / CATALOG_READY не имели понятных подписей | Добавлены подписи для фактических состояний API |
| P2 | Тема применялась после hydration | Сохранённый `aba-theme` читается до отрисовки body; начальный effect не сбрасывает выбор на system |

Новых P0-блокеров обработки заказа не подтвердилось. Смена формулировок не меняет значения backend statuses. Финансовые, staff approval, stock freshness и tenant gates сохранены.

## Главная и смысл данных

Порядок: магазин → «Требуют внимания» → показатели → быстрые действия → раскрываемая настройка → дополнительный контекст.

Ожидающие решения заказы берутся из существующего `orders?status=pending_confirmation&page=1`; дополнительно проверяется статус строки. Все заказы не становятся ожидающими. Показаны максимум первые три; число относится к загруженному списку, не к неизвестному общему итогу. Переход ведёт в настоящий отфильтрованный список.

Ошибки импорта, отсутствующие цены, актуальность остатков и неполные карточки берутся из текущего catalog-control. Это предложения проверить каталог, без утверждения, что все продажи остановлены. Исторический `handoff` из AI settings не используется как число новых обращений: backend считает события передачи за всё время. Нет достоверного unread/action-required показателя — нет выдуманной метрики.

При успешных чтениях и отсутствии задач выводится «Сейчас всё спокойно. Новых задач нет». При недоступности обязательных данных выводится неполнота списка и «Повторить». Состояние неизвестно не считается нулём. Для обычного магазина/недостаточных прав не обещается обработка заказов тестового контура. Existing loading skeleton сохранён.

| Карточка | Источник / период | Действие |
|---|---|---|
| Все диалоги | `ai-settings.conversations`, все сохранённые записи, без фильтра по дате | Диалоги |
| Все заказы | `ai-settings.orders`, все статусы, без фильтра по дате | Список заказов |
| Качество каталога | `catalog-control.health_score`, текущий снимок | Каталог, либо товары для роли без сводки |
| AI-ассистент | Текущий `mode` и `settings.auto_enabled` | Настройки помощника |

Метрики «сегодня», выручка и прогнозы не добавлялись. Режим магазина обозначается как тестовый/обычный по `is_test`; обычный магазин не называется подтверждённым production. Реальное подключение каналов по-прежнему не подменяется mock-конфигурацией.

## Измерение до/после

Один и тот же существующий synthetic demo tenant, одинаковые viewport, scale factor1 и темы; API, PostgreSQL и обычная авторизация настоящие. Для сравнения не использован прежний HTTP fixture.

Desktop **1440×900**:

- Приветственный блок: **239.58 → 145.89 px**, уменьшение примерно **39%** без фиксированной высоты.
- Начало задач: **747.23 → 253.89 px**, примерно на **493 px выше**.
- Задачи и четыре показателя помещаются в первый экран в проверенном состоянии.
- До/после совпали исходные `ai-settings` и список pending orders. Значения: 4 диалога, 4 заказа; catalog score66/100. Это данные fixture, не реальные показатели бизнеса.

Мобильное сравнение: **390×844**. Заказ читается без горизонтального переполнения; номер, сумма и переход не конкурируют за одну узкую строку.

Скриншоты:

| Экран | До | После |
|---|---|---|
| Главная desktop | [light](../.local/ux-final-polish-20261117/before-1440-light-dashboard.png) | [light](../.local/ux-final-polish-20261117/after-1440-light-dashboard.png) |
| Главная mobile | [light](../.local/ux-final-polish-20261117/before-390-light-dashboard.png) | [light](../.local/ux-final-polish-20261117/after-390-light-dashboard.png) |
| Заказы desktop | [light](../.local/ux-final-polish-20261117/before-1440-light-dashboard-orders.png) | [light](../.local/ux-final-polish-20261117/after-1440-light-dashboard-orders.png) |
| Заказы mobile | [light](../.local/ux-final-polish-20261117/before-390-light-dashboard-orders.png) | [light](../.local/ux-final-polish-20261117/after-390-light-dashboard-orders.png) |
| Главная desktop dark | [dark](../.local/ux-final-polish-20261117/before-1440-dark-dashboard.png) | [dark](../.local/ux-final-polish-20261117/after-1440-dark-dashboard.png) |
| Главная mobile dark | [dark](../.local/ux-final-polish-20261117/before-390-dark-dashboard.png) | [dark](../.local/ux-final-polish-20261117/after-390-dark-dashboard.png) |

Dark-скриншоты заказов находятся рядом с теми же именами и заменой `light` на `dark`. `initial-after/` сохраняет первую версию polish до исправления мобильной строки; `zoom-before-fix.png` и FAIL log сохраняют переполнение. Они не являются финальным результатом.

## Файлы этого этапа

22 frontend-файла относительно baseline **после предыдущего редизайна**; staging Git index не менялся.

```text
apps/web/playwright.config.ts
apps/web/src/app/layout.tsx
apps/web/src/app/dashboard/layout.tsx
apps/web/src/app/dashboard/loading.tsx
apps/web/src/app/dashboard/page.tsx
apps/web/src/app/premium.css
apps/web/src/components/AccountMenu.tsx
apps/web/src/components/CatalogWorkspace.tsx
apps/web/src/components/ConversationList.tsx
apps/web/src/components/ProductUI.tsx
apps/web/src/components/ReloadSection.tsx
apps/web/src/components/Sidebar.tsx
apps/web/src/components/StoreOnboarding.tsx
apps/web/src/components/ThemeSwitch.tsx
apps/web/src/components/WorkflowPage.tsx
apps/web/src/components/Workflows.tsx
apps/web/src/lib/presentation.ts
apps/web/src/lib/workflows.ts
apps/web/tests/auth.spec.ts
apps/web/tests/ux-production.spec.ts
apps/web/tests/polish-production.spec.ts
apps/web/tests/dashboard-render.test.mjs
```

Также созданы этот отчёт и private evidence/index. `Conversation.updated_at` только описывает уже существующее поле API. AccountMenu и presentation helpers не меняют identity или права. Подавление hydration warning ограничено атрибутом темы корневого html, не всем интерфейсом. Новые API/маршруты не добавлены.

## PASS

| Проверка | Фактический результат / evidence |
|---|---|
| TypeScript | exit0, `typecheck-acceptance.log` |
| ESLint | exit0, `lint-acceptance.log`, без исключений для новых тестов |
| Production build | exit0, `build-focus.log`, отдельный Linux workspace, штатный offline production.sh |
| Native preflight | 5 tests PASS, `preflight.log` |
| OpenAPI | PASS, `openapi.log` |
| Artifact guard unit test | 1 PASS, `artifact-tests.log`; не независимая аттестация |
| Current artifact / served identity | PASS до и после проверок, `integrity-focus.log`, `integrity-focus-post.log`; guard не изменялся |
| Server presentation branches | 4 tests PASS: empty/unknown, pending-only, ACTIVE, обычный магазин; `dashboard-render-final.log` |
| Playwright polish | 3 tests PASS, `polish-final.log` |
| Existing UX production suite | 6 tests PASS, все шесть viewport, реальные данные/формы, theme/onboarding, настоящий429 и recovery, skip-link; `existing-ux-final.log` |
| Existing smoke subset | 3 tests PASS, `smoke-final.log`: анонимный redirect, loading/outage, неверный status contract |
| Responsive | 10 экранов × 6 ширин =60 проверок light; 320/375/390/768/1024/1440; `usability-results.json` |
| Dark routes | 10 экранов × 2 ширины =20 проверок, `dark-results.json` |
| Theme switching | 24 дополнительных проверки light/dark/system, persistence/reload; нет hydration errors |
| Zoom/reflow | 4 экрана при CSS zoom200%: главная, заказы, товары, диалоги; без page overflow; `zoom-200-*.png` |
| Custom accessibility automation | 5 197 видимых текстовых узла, контраст обычного текста≥4.5/крупного≥3; labels, button/summary targets≥24px, single main h1, AX main landmark, no page overflow |
| Focus / non-text sample | Input border contrast3.35 light /6.22 dark; focus ring3px, contrast4.94 /10.13; input text17.85 /14.87; `controls-results.json` |
| Keyboard / expanded state | Skip-link, profile, More, Escape/focus return; native AX expanded false→true→false, обе темы |
| Открытые меню | 18 проверок горизонтальных границ профиля, выбора магазина и «Ещё» на всех шести ширинах; `menus-final.json` |
| Console | 0 JS exceptions и 0 console errors в read-only usability проходе; намеренные network abort/409 относятся к отдельному error E2E |
| Diff whitespace | `git diff --check -- apps/web`, exit0 |

Playwright polish использует свежие синтетические магазины через существующий fixture CLI, проверяющий `aba_test` safety marker. Проверено через реальный BFF → Go → PostgreSQL:

- staff key actions, Escape и отмена диалога без изменения резерва;
- сетевой отказ без автоматического повтора; 409 сохраняет сообщение ошибки;
- медленный ответ и double click: один POST, исходный expected_version и idempotency_key;
- подтверждение увеличивает реальный резерв, reload сохраняет статус; отмена освобождает резерв;
- read-only member не видит действие, реальный GET заказа возвращает403;
- недоступный tenant не раскрывает заказ; это репрезентативная проверка, не полный RLS proof;
- создание второго своего synthetic магазина, переключение через UI, tenant в ссылках;
- пустой каталог с одним primary CTA; поиск/сброс на фактическом каталоге.

Сетевой отказ и409 в этих E2E намеренно создаются browser interception. Успешные подтверждение/отмена, чтение резерва, создание магазина и member403 выполняются через настоящий backend.

Полная регистрация/Mailpit и остальные старые suites, кроме выполненного ux-production, не выдаются за выполненные. В старых auth/UX tests обновлены затронутые подписи/профиль; их полный прогон остаётся отдельным.

## FAIL, устранённые в ходе работы

- Первый server-render unit harness неправильно экспортировал stub Next Link; исправлен test harness, `dashboard-render-initial-fail.log` сохранён.
- ESLint отклонил CommonJS require в новом тесте; тест переведён в mjs без ослабления правил. `lint-node-test-fail.log` сохранён.
- Первый usability runner не задал baseURL; ошибка URL сохранена в `usability-1.log`, приложение не менялось для её обхода.
- `usability-2.log` и `zoom-diagnostic.log` подтвердили реальные 6px overflow при200%; исправлено адаптивной сеткой. Финальный `usability-3.log` PASS.
- Первая мобильная строка была слишком тесной — скриншоты сохранены в `initial-after/`, финальная версия проверена отдельно.
- `menus-before-fix.log` подтвердил уход профиля на34px при320 и выбора магазина за левый край при390/768. Исходные geometry и снимок сохранены; после исправления проверены все18 открытых состояний.

- Расширенный существующий UX-набор сначала дал3/6 FAIL на skip-link (`existing-ux-focus-fail.log` и traces). Причина — dashboard loading main без aria-busy, при том что SkipLink уже ожидает этот признак. Добавлен атрибут; тот же набор без ослабления ожиданий прошёл6/6.
- Вспомогательный controls check один раз не удержал ранний программный focus (`controls-early-focus-fail.log`). Перед измерением теперь проверяется реальное клиентское действие «Добавить характеристику», затем сохраняются строгие Tab/Shift+Tab/focus/contrast/expanded assertions. Код приложения для этого runner FAIL не менялся.

## Browser sandbox

Точный прежний запуск: `.local/ux-preview-20261117-5db7faec/show-preview.cjs`, `launchPersistentContext()` без `chromiumSandbox`. Установленный Playwright1.64.0 добавляет `--no-sandbox`, если опция не равна true (`playwright-core/lib/coreBundle.js`, chromium defaultArgs). Это соответствует [официальной реализации Playwright](https://github.com/microsoft/playwright/blob/main/packages/playwright-core/src/server/chromium/chromium.ts).

Прежний принадлежащий этому показу браузер закрыт. В project Playwright config и новых capture/usability/preview launchers явно `chromiumSandbox:true`. На macOS запуск успешен. Через CDP получен PID браузера и проверен его фактический argv: `--no-sandbox` отсутствует; `before-sandbox.json`, `after-sandbox.json`, `visible-preview.json`. Это подтверждение параметров запуска, не независимый аудит реализации OS sandbox. Реальные аккаунты не использовались.

Старые launchers в `.local/ux-redesign-*` и `.local/ux-preview-*` сохранены как историческое evidence; не использовать их как текущий launcher. Новый видимый показ: `.local/ux-final-polish-20261117/show.cjs`. Никакие OS security settings, sudo, seccomp или Docker daemon options не менялись.

**BLOCKED/NOT RUN:** Chromium sandbox внутри прежнего root Docker browser runner не проверялся и не отключается для прохождения тестов. Текущие E2E используют sandbox-enabled Chrome на host и реальные изолированные Docker API/DB. Для контейнерного browser runner нужен отдельный проверенный non-root/sandbox-compatible запуск.

## Next.js — отдельный security follow-up

Установлено и закреплено **Next.js16.4.0**. Источник наблюдавшегося предупреждения — `next/dist/lib/upgrade/nudge.js:262`; исходный SecurityFatalError сохранён в `.local/ux-redesign-20261117/build-focus-fix.log`. Offline build в этом этапе не считается новым vulnerability scan.

Vendor advisory **GHSA-673q-mjm6-r5xr** включает16.4.0 в затронутый диапазон и указывает16.4.1 как исправленную версию. Vendor рекомендует обновление даже при отсутствии аналогичных use-cache функций. Это проверено по [официальному advisory](https://github.com/vercel/next.js/security/advisories/GHSA-673q-mjm6-r5xr); новый CVE-номер не придумывается.

Готовый план уже есть в [TRUSTED_RELEASE_REBUILD_PLAN.md](TRUSTED_RELEASE_REBUILD_PLAN.md) и [D2 execution plan](STAGE_12C3_D2_EXECUTION_PLAN.md): Next/env/SWC16.4.1 и источники/SRI metadata. **Это подготовленный кандидат, не установленный fix.** Package lock и зависимости в этом этапе не менялись.

Конкретный следующий security scope: проверить approval/admission payloads, совместимость Next/env/SWC/ESLint/native для Linux и macOS; обновить только согласованный patch и lock; выполнить native probes, fresh build, artifact/served guards, auth/order/catalog/channel regressions, актуальные scan/SBOM/provenance. Не выполнять массовый npm update, не снимать Next warning искусственно. Этот blocker остаётся OPEN.

## NOT RUN и оставшиеся ограничения

- VoiceOver / ручной screen-reader audit: **NOT RUN**. Приложение VoiceOver на macOS доступно, но проверенного ручного сеанса/прослушивания в этой работе не было; OS accessibility/security настройки не менялись.
- axe: **NOT RUN**, пакет отсутствует, не устанавливался. Выполнены собственные DOM/contrast/keyboard/AX checks. Они не являются полным WCAG2.2 AA audit. Критерии non-text оценивались по необходимым индикаторам/фокусу, не по декоративным границам карточек; см. [W3C SC1.4.11](https://www.w3.org/WAI/WCAG22/Understanding/non-text-contrast.html).
- Реальный browser zoom через GUI, другие браузеры и физический телефон: **NOT RUN**. Выполнены перечисленные viewport/CSS zoom проверки в установленном Chrome.
- Полные Go unit/race/vet/build, SQL/RLS matrix, все auth/Mailpit/import/multichannel/financial suites: **NOT RUN**. Backend не менялся. Целевые реальные order/stock/role E2E выполнены и не подменяются stub-сервером.
- Обновление Next/полный свежий vulnerability scan/OCI provenance: **NOT RUN/OPEN**; отдельный scope. E01 остаётся OPEN; proof не перезаписывался для принятия drift.
- Поведение остальных существующих native browser confirms сохранено. Полная унификация всех старых подтверждений не выполнялась.

Новых регрессий в выполненных сценариях не обнаружено; отсутствие регрессий вне этого покрытия не утверждается. Ни Go/SQL/RLS/migrations, ни auth/session contracts, ни provider secrets/configuration, ни VPS/production environment не изменялись. Live, paid calls, deployment, commit и push не выполнялись. Все изменения данных — только выделенные синтетические fixtures.

## Можно ли показывать магазину

**Да — как управляемую локальную демонстрацию с синтетическими данными**, без реальных сообщений, клиентских аккаунтов и денег. Открыт sandbox-enabled Chrome: http://127.0.0.1:43010/dashboard.

**Реальный customer pilot / production / security или deployment GO — не выданы.** Нужны security remediation/trusted release и отдельная проверка полного рабочего сценария с представителем магазина, включая ручную доступность.
