import Link from "next/link";
import {organizations, request} from "@/lib/session";
import type {AIState} from "@/components/AISettings";
import type {Order} from "@/lib/workflows";
import {money} from "@/lib/catalog";
import {PageIntro, MetricCard, EmptyState, Icon, StatusBadge} from "@/components/ProductUI";
import {ReloadSection} from "@/components/ReloadSection";

export const metadata = {title:"Главная"};
type Health = {health_score:number|null;metrics:{review:number;missing_price:number;incomplete:number;import_errors:number;products:number}};
type Setup = {status:string;catalog_count:number};
const setupLabels:Record<string,string> = {NOT_STARTED:"Начало настройки",DRAFT:"Начало настройки",PROFILE_READY:"Данные магазина заполнены",CATALOG_READY:"Каталог подготовлен",CHANNELS_PENDING:"Подготовьте каналы",READY_FOR_REVIEW:"Готов к проверке"};

export default async function Dashboard({searchParams}:{searchParams:Promise<{org?:string}>}) {
  const list = await organizations(), selected = (await searchParams).org;
  const active = list.find(o => o.id === selected) ?? (selected ? undefined : list[0]);
  if (!active) return <main id="main" tabIndex={-1}><PageIntro title={selected ? "Магазин недоступен" : "Начнём с вашей организации"}/><EmptyState title={selected ? "Выберите доступный магазин" : "Создайте первую организацию"} href={selected ? "/dashboard/organizations" : "/dashboard/organizations/new"} action={selected ? "Выбрать магазин" : "Создать организацию"}>{selected ? "Откройте магазин, к которому у вас есть доступ." : "Выберите отрасль и откройте кабинет с AI-командой."}</EmptyState></main>;
  const suffix = `?org=${active.id}`;
  const link = (path:string) => `/dashboard/${path}${suffix}`;
  const pendingLink = `${link("orders")}&status=pending_confirmation`;
  async function load<T>(path:string):Promise<T|null> {
    try {const r = await request(`organizations/${active!.id}/${path}`); return r.ok ? await r.json() : null;} catch {return null;}
  }
  const manages = active.is_test && active.can_manage_orders;
  const owner = active.role === "owner";
  const [state, health, pending, setup] = await Promise.all([
    manages ? load<AIState>("ai-settings") : null,
    owner ? load<Health>("catalog-control?page=1") : null,
    manages ? load<{items:Order[]}>("orders?status=pending_confirmation&page=1") : null,
    owner ? load<Setup>("onboarding") : null,
  ]);
  // Never substitute all orders or historical handoff events for current work.
  const waiting = pending?.items.filter(order => order.status === "pending_confirmation");
  const tasks = health ? [
    {n:health.metrics.import_errors,title:"Проверьте ошибки импорта",href:"onboarding",detail:"Открыть импорт"},
    {n:health.metrics.missing_price,title:"Укажите цены товаров",href:"catalog",detail:"Проверить цены"},
    {n:health.metrics.review,title:"Сверьте актуальность остатков",href:"catalog",detail:"Проверить каталог"},
    {n:health.metrics.incomplete,title:"Дополните карточки товаров",href:"catalog",detail:"Открыть каталог"},
  ].filter(task => task.n > 0) : [];
  const unavailable = (manages && (!state || !pending)) || (owner && !health);
  const hasTasks = Boolean(waiting?.length || tasks.length);
  return <main id="main" tabIndex={-1} className="overview overview-polished">
    <section className="overview-welcome shop-context" aria-label="Текущий магазин">
      <div className="shop-context-copy"><div className="shop-context-meta"><StatusBadge>{active.is_test ? "Тестовый магазин" : "Обычный магазин"}</StatusBadge><span>{owner ? "Владелец" : active.can_manage_orders ? "Сотрудник" : "Просмотр"}</span></div><h1>{active.name}</h1><p>{active.is_test ? "Тестовые данные · реальные отправки отключены" : "Товары и обращения вашего магазина"}</p></div>
      <div className="shop-context-actions">{list.length > 1 && <details className="store-picker"><summary>Сменить магазин</summary><nav aria-label="Выбор магазина">{list.map(org => <Link prefetch={false} key={org.id} href={`/dashboard?org=${org.id}`} aria-current={org.id === active.id ? "page" : undefined}>{org.name}</Link>)}</nav></details>}<Link prefetch={false} className="button primary" href={manages ? pendingLink : link("products")}>{manages ? "Проверить заказы" : "Открыть товары"}</Link></div>
    </section>

    <section className="task-panel priority-panel" aria-labelledby="attention-title">
      <div className="section-heading"><h2 id="attention-title">Требуют внимания</h2>{manages && <Link prefetch={false} href={pendingLink}>Заказы к подтверждению</Link>}</div>
      {waiting && waiting.length > 0 && <div className="pending-orders">{waiting.slice(0,3).map(order => <Link prefetch={false} className="pending-order" key={order.id} href={link(`orders/${order.id}`)}><span className="order-mini-icon"><Icon name="orders"/></span><span><strong>Заказ №{order.id.slice(0,8)}</strong><small>Ждёт вашего подтверждения</small></span><b>{order.total_amount===null ? "Сумма не согласована" : money(order.total_amount)}</b><span className="pending-order-action">Открыть заказ</span></Link>)}<p className="field-hint">В загруженном списке: {waiting.length}. {waiting.length > 3 ? "Показаны первые 3. " : ""}Товар резервируется только после подтверждения сотрудником.</p></div>}
      {tasks.length > 0 && <ul className="attention-tasks">{tasks.map(task => <li key={task.title}><span><strong>{task.title}</strong><span className="task-count">{task.n}</span></span><Link prefetch={false} href={link(task.href)}>{task.detail}</Link></li>)}</ul>}
      {unavailable && <div className="attention-error" role="status"><p>Не удалось загрузить данные. Список задач может быть неполным.</p><ReloadSection label="Повторить"/></div>}
      {!hasTasks && !unavailable && manages && <p className="calm-message"><Icon name="check"/>Сейчас всё спокойно. Новых задач нет.</p>}
      {!manages && !hasTasks && <p className="field-hint">Задачи по заказам доступны сотруднику с правом обработки заказов в тестовом магазине. Вы можете просматривать товары.</p>}
    </section>

    <section className="overview-metrics" aria-label="Состояние магазина">
      <MetricCard title="Все диалоги" value={state?.conversations ?? "Нет данных"} caption="Без ограничения по дате" action="Открыть диалоги" href={link("conversations")} icon="chat"/>
      <MetricCard title="Все заказы" value={state?.orders ?? "Нет данных"} caption="Все статусы · без ограничения по дате" action="Посмотреть заказы" href={link("orders")} icon="orders"/>
      <MetricCard title="Качество каталога" value={health?.health_score == null ? "Нет оценки" : `${health.health_score}/100`} caption="Текущие цены, наличие и карточки" action={owner ? "Проверить каталог" : "Открыть товары"} href={link(owner ? "catalog" : "products")} icon="catalog"/>
      <MetricCard title="AI-ассистент" value={!state ? "Нет данных" : state.mode === "mock" ? "Симуляция" : "Проверить режим"} caption={!state ? "Текущий режим неизвестен" : state.settings.auto_enabled ? "Автоответы разрешены" : "Автоответы на паузе"} action="Настройки помощника" href={link("ai-settings")} icon="ai"/>
    </section>

    <section className="quick-actions" aria-labelledby="quick-actions-title"><h2 id="quick-actions-title">Быстрые действия</h2><div>{owner && <><Link prefetch={false} className="button secondary" href={link("products/new")}>Добавить товар</Link><Link prefetch={false} className="button secondary" href={link("onboarding")}>Импортировать товары</Link></>}<Link prefetch={false} className="button secondary" href={link("conversations")}>Открыть диалоги</Link><Link prefetch={false} className="button secondary" href={link("integrations")}>Проверить каналы</Link></div></section>
    {owner && setup && setup.status !== "ACTIVE" && <details className="setup-panel setup-disclosure"><summary>Продолжить настройку магазина <span>{setupLabels[setup.status] ?? "Проверьте готовность"}</span></summary><p>Текущее состояние получено из настроек магазина. Можно продолжить с любого шага.</p><ol className="next-actions">{[["onboarding","Данные магазина"],["products","Товары и цены"],["ai-settings","AI-ассистент"],["integrations","Каналы связи"],["onboarding","Проверка готовности"]].map(([path,title]) => <li key={title}><Link prefetch={false} href={link(path)}>{title}</Link></li>)}</ol></details>}
    <p className="overview-footnote">{active.is_test ? "Демонстрационный контур. Реальные сообщения не отправляются." : "Режим отправки проверяется отдельно в настройках каналов."} <Link prefetch={false} href={link("integrations")}>Состояние Telegram и WhatsApp</Link></p>
  </main>;
}
