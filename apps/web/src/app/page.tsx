import Link from "next/link";
import {IntelligenceCore, intelligenceTeam} from "@/components/IntelligenceCore";
import {IntelligenceDemo} from "@/components/IntelligenceDemo";
import {Icon} from "@/components/ProductUI";

const situations = [
  ["Торговля", "Проверьте наличие, разберите заявки и подтвердите резерв из кабинета."],
  ["Услуги и автосервис", "Уточняйте потребность клиента, проверяйте запчасти и готовьте заявку на обслуживание."],
  ["Недвижимость", "Фиксируйте запросы клиентов и готовьте следующий шаг по обращению."],
];

export default function Home() {
  return <main id="main" tabIndex={-1} className="intelligence-landing">
    <section className="intelligence-hero">
      <div className="intelligence-wrap hero-layout">
        <div className="hero-intro">
          <p className="intelligence-kicker"><span/> The Intelligence</p>
          <h1>Управляйте бизнесом.<br/><em>Не рутиной.</em></h1>
          <p className="intelligence-lead">Семь AI-специалистов в едином рабочем пространстве. Клиенты, продажи, товары, финансы и развитие бизнеса — под вашим контролем.</p>
          <div className="intelligence-actions">
            <Link prefetch={false} className="button primary" href="/register">Создать аккаунт</Link>
            <a className="button secondary" href="#live-demo">Посмотреть демо</a>
          </div>
        </div>
        <figure className="hero-core"><IntelligenceCore/><figcaption><span>Business Manager</span><span>6 специалистов</span></figcaption></figure>
      </div>
    </section>

    <section id="live-demo" className="intelligence-wrap intelligence-section landing-demo">
      <div className="intelligence-section-intro"><p className="intelligence-kicker">Интерактивное демо</p><h2>От поручения до решения</h2><p>Выберите задачу и посмотрите, кого подключает Business Manager, какой результат получает и где нужно ваше решение.</p></div>
      <IntelligenceDemo/>
    </section>

    <section className="intelligence-wrap intelligence-section editorial-split">
      <div><h2>Что требует внимания сегодня?</h2><p className="intelligence-lead">Откройте обзор, чтобы увидеть заявки на подтверждение, состояние каталога и доступные поручения для команды.</p><Link prefetch={false} className="button secondary" href="/login">Открыть кабинет</Link></div>
      <div className="workspace-preview" aria-label="Иллюстрация рабочего кабинета">
        <div className="preview-heading"><span>Рабочее пространство</span><small>Пример интерфейса</small></div>
        <h3>Обзор организации</h3>
        <ul>{[["orders","Заявки","На подтверждении"],["catalog","Каталог","Проверка остатков"],["ai","Business Manager","Новое поручение"]].map(([icon,title,status]) => <li key={title}><Icon name={icon as "orders"|"catalog"|"ai"}/><strong>{title}</strong><span>{status}</span></li>)}</ul>
      </div>
    </section>

    <section id="intelligence-team" className="team-band">
      <div className="intelligence-wrap intelligence-section editorial-split">
        <div><p className="intelligence-kicker">AI-команда</p><h2>У каждого специалиста своя роль</h2><IntelligenceCore compact/></div>
        <dl className="landing-team">{intelligenceTeam.map(agent => <div key={agent.name}><dt>{agent.name}</dt><dd>{agent.role}</dd></div>)}</dl>
      </div>
    </section>

    <section className="intelligence-wrap intelligence-section">
      <h2>Подходит для разных видов бизнеса</h2>
      <div className="industry-list">{situations.map(([title,copy]) => <article key={title}><h3>{title}</h3><p>{copy}</p></article>)}</div>
    </section>

    <section className="intelligence-wrap intelligence-section editorial-split">
      <div><p className="intelligence-kicker">Начало работы</p><h2>От аккаунта к первому поручению</h2></div>
      <ol className="landing-steps"><li>Подтвердите email.</li><li>Создайте организацию и выберите отрасль.</li><li>Откройте рабочее пространство и выберите поручение.</li></ol>
    </section>

    <section className="intelligence-wrap intelligence-section editorial-split">
      <div><h2>Каналы связи и контроль</h2><p>В кабинете видна готовность Telegram, WhatsApp Business Cloud API и Instagram. Рабочие подключения требуют отдельной настройки и допуска.</p></div>
      <div><h3>Решения остаются за вами</h3><p>Доступ к данным ограничен организацией и ролью. Действия с последствиями проходят согласование, если оно требуется. Записи в журнале помогают проверить, что произошло.</p></div>
    </section>

    <section className="intelligence-wrap intelligence-section editorial-split">
      <h2>Частые вопросы</h2>
      <div className="landing-faq">
        <details><summary>Нужен ли аккаунт для демо?</summary><p>Демо доступно сразу и использует синтетические данные. Его действия остаются на этой странице.</p></details>
        <details><summary>Какие данные видит AI-команда?</summary><p>В рабочем пространстве доступ зависит от организации, роли и настроек. Возможности по каждой роли можно посмотреть в разделе AI-команды.</p></details>
        <details><summary>Можно ли изменить отрасль после настройки?</summary><p>Владелец может открыть профиль бизнеса в настройках организации и выбрать доступный профиль.</p></details>
      </div>
    </section>

    <section className="intelligence-wrap intelligence-section landing-final">
      <h2>Откройте рабочее пространство</h2>
      <Link prefetch={false} className="button primary" href="/register">Создать аккаунт</Link>
    </section>
    <footer className="intelligence-wrap landing-footer">
      <span>AI Business Assistant</span>
      <Link prefetch={false} href="/status">Состояние системы</Link>
    </footer>
  </main>;
}
