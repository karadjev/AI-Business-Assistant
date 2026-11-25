import Link from "next/link";
import {Icon} from "@/components/ProductUI";

const steps = [
  {icon: "chat", title: "Клиент задаёт вопрос", description: "Например, уточняет наличие товара или условия услуги."},
  {icon: "ai", title: "AI-команда готовит ответ", description: "Business Manager подбирает специалистов и собирает ответ на основе данных организации."},
  {icon: "check", title: "Сотрудник проверяет результат", description: "Заявку, резерв или черновик можно проверить и согласовать в кабинете."},
] as const;

const principles = [
  {icon: "catalog", title: "Торговля и услуги", description: "Ведите каталог, остатки и заявки. Выберите профиль, чтобы получить поручения для вашей отрасли."},
  {icon: "ai", title: "Семь AI-агентов", description: "Business Manager координирует продажи, поддержку, каталог, финансы, маркетинг и аналитику."},
  {icon: "channels", title: "Каналы под вашим контролем", description: "Статусы Telegram, WhatsApp и Instagram видны в кабинете. Для работы каналов нужны настройка и допуск."},
] as const;

export default function Home() {
  return (
    <main id="main" tabIndex={-1} className="landing">
      <section className="hero">
        <p className="eyebrow">Для малого бизнеса</p>
        <h1>Ближе к клиентам.<br/><span>С помощью ИИ.</span></h1>
        <p className="hero-copy">Семь AI-агентов помогают разбирать обращения, вести каталог и анализировать бизнес. Работайте с результатами и согласовывайте решения в одном кабинете.</p>
        <div className="hero-actions">
          <Link prefetch={false} className="button primary" href="/register">Создать аккаунт <Icon name="arrow"/></Link>
          <Link prefetch={false} className="button secondary" href="/dashboard/demo">Посмотреть демо</Link>
          <Link prefetch={false} className="button secondary" href="/status">Проверить систему</Link>
        </div>
        <p className="small muted">Демо использует тестовые данные. Сообщения клиентам из демо не отправляются.</p>
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
          </article>
        ))}
      </section>
    </main>
  );
}
