# Следующий этап после10Г — offline operator rehearsal

Предложение для отдельного согласования; не является разрешением реализации, live-пилота или рабочего rollout. Основание: STAGE_10D_REPORT.md и DEMO_WALKTHROUGH.md. Security CONDITIONAL, E01 OPEN.

## Цель и объём

Проверить демонстрацию с реальным оператором и наблюдателем на подготовленном isolated synthetic tenant. Автоматизированные паузы рассказчика подтверждают воспроизводимый сценарий, но не заменяют human usability evidence. Не добавлять новые продажи, интеграции или параллельные catalog/identity системы.

1. Воспроизвести runbook из cached artifacts в отдельном loopback project; при отсутствии cache остановиться без downloads. Проверить сохранение истории при stop/start и повторном CLI UUID; новый показ — новый run.
2. Провести отдельно Telegram и WhatsApp mock rehearsal: каталог/health, buyer, grounded reply, consent, заявка без резерва, отдельный staff, reserve и handoff. Измерить5–10 минут без искусственного wait; записать реальные затруднения и необходимость помощи оператора.
3. Проверить desktop/mobile390×844 и keyboard в живом браузере: понятность consent, разделение sample/actual results, stale/error/retry, staff handoff, пустые состояния. Дополнительная accessibility проверка со screen reader — отдельный явно зафиксированный результат, не предположение.
4. Внести только подтверждённые минимальные UX исправления после согласования scope; затем повторить затронутые offline gates и fresh integrity/served checks. Не менять working components ради косметики без необходимости.
5. Подготовить решение о готовности к следующему ограниченному пилоту с перечнем blockers. Никакой автоматической активации live.

## Критерии приёмки

Оба канала пройдены реальным оператором за5–10 минут с честно зафиксированными seed/login boundaries; согласие явно; до сотрудника резерв0; повтор не создаёт duplicate order; handoff прекращает автоответ. Нет fake success/ROI, page overflow или скрытого удаления истории. Права owner/staff/read-only, foreign tenant, RLS и существующие gates сохранены. Все результаты имеют PASS/FAIL/BLOCKED/NOT RUN и provenance, credentials/PII в отчёт не попадают.

## Блокировки и отдельные решения

- Технический host-loopback blocker устранён отдельным авторизованным этапом: loopback-only Docker-stdio relay; login/demo/auth/Origin/CSRF/integrity/served PASS. См. DEMO_LOCALHOST_REPORT.md и обновлённый DEMO_WALKTHROUGH.md. Human rehearsal всё ещё NOT RUN; запускать только отдельный internal demo project.

- Нужны согласование такого этапа, оператор/наблюдатель и локальная browser session. Автоматический показ10Г не считается human study.
- E01 OPEN: отсутствие повторения не устанавливает первопричину и не разрешает ослабление artifact guards. При дрейфе сохранить failure evidence; не удалять подозрительные пути и не переиздавать proof для обхода проверки.
- Security CONDITIONAL и live readiness blockers прежних этапов сохраняются. Реальные Telegram/WhatsApp/1С/LLM, деньги, provider credentials, public exposure, downloads и production deployment требуют самостоятельного разрешения и проверенных prerequisites.
- Audit/receipts retention proposal10В не утверждён. Сроки и удаления не устанавливать в rehearsal.
- Demo fixture предназначен только для маркированной aba_test; не переносить privileged seed в HTTP или рабочую БД. Пределы demo summary20 orders/inventory100 учитывать при длинной истории; для чистого показа новый synthetic run, не reset старого.
- Наличие подготовленных cached images/modules/native bindings и совпадающего fresh proof обязательно. Занятые порты/Audio процессы не завершать принудительно; выбрать отдельный loopback port.

AGENTS.md, глобальные настройки, PATH и CI не изменять. Никаких commit/push/deploy без отдельного разрешения.

## Возврат после согласованного10F

Пользователь явно разрешил отдельный Premium UX/UI Transformation10F **перед** этой репетицией. Это меняет последовательность, а не критерии10E. Отчёт: STAGE_10F_UX_REPORT.md; новая навигация: UX_NAVIGATION.md. HUMAN REHEARSAL остаётся **NOT RUN**,10E не завершён автоматически.

После завершения автоматических проверок10F оператор и наблюдатель должны:

1. Убедиться в актуальном integrity/served proof и доступе127.0.0.1 по DEMO_WALKTHROUGH.md; использовать synthetic owner/staff и отдельный demo tenant.
2. Пройти Telegram и WhatsApp: найти нужный раздел с новой навигацией, проверить каталог/AI, получить фактический ответ, выразить consent, подтвердить заявку отдельным staff и проверить reserve/handoff.
3. Измерить5–10 минут без автоматических narrator waits; отдельно отметить подготовку/login, подсказки оператора и затруднения. Автоматические screenshots/contrast/focus assertions10F не являются human study.
4. Проверить390×844 и375×812, keyboard, читаемость обеих тем, мобильную панель, error/empty/retry и понятность неизвестной доставки. Screen-reader/device результаты фиксировать только после фактического запуска.
5. Сохранить проблемы и наблюдения для отдельного согласования минимальных исправлений. Не считать настройку магазина или synthetic counters доказательством live readiness.

E01 OPEN, Security CONDITIONAL. Network isolation, роли, financial/consent/RLS gates и нерешённый audit/receipts retention proposal сохраняются.

Финальный10F: девять production browser cases прошли на новом изолированном артефакте; источник результатов и ограничения — STAGE_10F_UX_REPORT.md. После повторения E01 обязателен local overlay `.local/ux10f/compose.isolated-build.yml` из обновлённого DEMO_WALKTHROUGH.md и успешный verify/served перед ручной сессией. Исходный загрязнённый build workspace не использовать. Свежий synthetic run для owner/staff подготовлен в `.local/demo10d/10f-rehearsal-fixture.json`, без изменения прежнего магазина. HUMAN REHEARSAL NOT RUN; E01 OPEN; Security CONDITIONAL.


## Решение владельца перед11А — 31 октября2026

**USER-REPORTED PASS**: владелец сообщил, что ручная проверка интерфейса проведена и основные функции работают. Это пользовательское свидетельство, не независимо проверенный полный checklist10E. Тайминги обоих каналов, staff/consent/handoff evidence, mobile/keyboard и полный протокол не представлены. **Формальная HUMAN REHEARSAL — NOT RUN / не подтверждена полным evidence;10E не закрыт.**

Отдельным решением разрешён11А Security & Reliability Hardening. Это совместимо с readiness: расследование E01 и SQL в isolated offline QA, без live rollout. E01 OPEN, Security CONDITIONAL; сроки audit/receipts не утверждены.
