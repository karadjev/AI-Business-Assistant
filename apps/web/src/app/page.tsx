import Link from "next/link";
import {intelligenceTeam} from "@/components/IntelligenceCore";
import {AgentIcon, Icon} from "@/components/ProductUI";

const steps = [
  {icon: "chat", title: "Клиент задаёт вопрос", description: "Например, уточняет наличие товара или условия услуги."},
  {icon: "ai", title: "AI-команда готовит ответ", description: "Business Manager подбирает специалистов и собирает ответ на основе данных организации."},
  {icon: "check", title: "Сотрудник проверяет результат", description: "Заявку, резерв или черновик можно проверить и согласовать в кабинете."},
] as const;

const channels = [["telegram","Telegram"],["whatsapp","WhatsApp"],["instagram","Instagram"]] as const;

const principles = [
  {icon: "catalog", title: "Торговля и услуги", description: "Ведите каталог, остатки и заявки. Выберите профиль, чтобы получить поручения для вашей отрасли."},
  {icon: "ai", title: "Семь AI-агентов", description: "Business Manager распределяет поручения между специалистами и собирает результаты."},
  {icon: "channels", title: "Каналы под вашим контролем", description: "Подключения настраиваются отдельно для каждой организации."},
] as const;

export default function Home() {
  return (
    <main id="main" tabIndex={-1} className="landing">
      <section className="hero">
        <p className="eyebrow">ИНТЕЛЛЕКТ ДЛЯ БИЗНЕСА</p>
        <h1>Управляйте бизнесом.<br/><span>С помощью ИИ.</span></h1>
        <p className="hero-copy">Семь AI-специалистов в единой платформе.</p>
        <div className="hero-actions">
          <Link prefetch={false} className="button primary" href="/register">Начать работу <Icon name="arrow"/></Link>
        </div>
      </section>
      <section className="flow-card" aria-labelledby="flow-title">
        <div className="flow-heading">
          <h2 id="flow-title" className="eyebrow">Как проходит демонстрация</h2>
          <span className="stage-tag">Тестовые данные</span>
        </div>
        <ol className="flow-steps">
          {steps.map(({icon, title, description}) => (
            <li className="flow-step" key={icon}>
              <span className="step-number"><Icon name={icon}/></span>
              <div><h3>{title}</h3><p>{description}</p></div>
            </li>
          ))}
        </ol>
        <p className="flow-bottom">В кабинете видны поручение, результат и статус согласования.</p>
      </section>
      <section className="principles" aria-label="Возможности AI Business Assistant">
        {principles.map(({icon, title, description}) => (
          <article key={icon}>
            <span className="principle-icon"><Icon name={icon}/></span>
            <h2>{title}</h2>
            <p>{description}</p>
            {icon === "ai" && <ul className="public-agent-list" aria-label="Роли AI-команды">
              {intelligenceTeam.map(({id, name}) => (
                <li key={id}><AgentIcon id={id}/>{name}</li>
              ))}
            </ul>}
            {icon === "channels" && <>
              <ul className="public-channel-list" aria-label="Поддерживаемые каналы">
                {channels.map(([id, title]) => <li key={id}><Icon name={id}/>{title}</li>)}
              </ul>
              <p className="public-channel-note">Статус подключения — в кабинете.</p>
            </>}
          </article>
        ))}
      </section>
    </main>
  );
}
