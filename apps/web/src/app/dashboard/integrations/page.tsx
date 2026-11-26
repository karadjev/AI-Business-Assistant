import Link from "next/link";
import {organizations, request} from "@/lib/session";
import {instagramStatuses} from "@/lib/instagram";
import {Icon, PageIntro, StatusBadge} from "@/components/ProductUI";
export const metadata = {title:"Каналы связи"};
export default async function Page({searchParams}:{searchParams:Promise<{org?:string}>}) {
 const q=await searchParams;const list=await organizations();const org=list.find(o=>o.id===q.org)??(q.org?undefined:list[0]);
 return <main id="main" tabIndex={-1}><PageIntro eyebrow="Общение с покупателями" title="Каналы связи" description="Один помощник, привычные мессенджеры. Настройка и состояние каждого канала — здесь."/>
 <div className="notice"><strong>Сейчас — только тестовый режим</strong><p>Реальные сообщения не отправляются. Тестовая настройка не означает подключение к Telegram, WhatsApp или Instagram. Бюджет — 0 ₽.</p></div>
 {!org?<Link prefetch={false} className="button secondary" href="/dashboard/organizations">Выберите магазин</Link>:!org.is_test?<section className="empty-state"><h2>Проверьте каналы в тестовом магазине</h2><p>Подключения обычного магазина не изменяются. Для безопасной проверки выберите отдельный тестовый контур.</p><Link prefetch={false} className="button secondary" href="/dashboard/organizations">Мои магазины</Link></section>:<div className="channel-cards">{await Promise.all((["telegram","whatsapp"] as const).map(async channel=>{
  let label="Состояние неизвестно";let error="";
  try{const r=await request(`organizations/${org.id}/integrations/${channel}`);const result=await r.json();if(r.ok){
   if(channel==="telegram"&&result.enabled===false)label="Канал выключен сервером";
   else if(channel==="whatsapp"&&result.configured===false)label="Тестовый канал недоступен";
   else if(!result.integration)label="Не настроен";
   else if(!result.integration.active)label="Подключение отключено";
   else if(channel==="telegram"&&result.integration.last_error)label="Требуется проверить webhook";
   else if(channel==="whatsapp"&&result.integration.sending_enabled===false)label="Приём настроен · отправка выключена";
   else label="Тестовое подключение активно";
  }else error=result.error?.message??`Ошибка ${r.status}`;}catch{error="Не удалось проверить состояние. Повторите загрузку раздела.";}
  const title=channel==="telegram"?"Telegram":"WhatsApp";
  return <section className="channel-card" key={channel}><div className="channel-card-top"><span className="channel-card-symbol" aria-hidden="true"><Icon name={channel}/></span><h2>{title}</h2><StatusBadge tone={error?"warning":"neutral"}>{error?"Состояние неизвестно":label}</StatusBadge></div><p>{channel==="telegram"?"Проверяйте обращения и ответы помощника в тестовом Telegram-канале.":"Проверяйте обращения, доставку и ограничения тестового WhatsApp-канала."}</p>{error&&<p role="alert">{error}</p>}<p className="channel-note">Реальное подключение не подтверждено. Реальные отправки отключены.</p><Link prefetch={false} className="button secondary" href={`/dashboard/integrations/${channel}?org=${org.id}`}>Настройки {title} <span aria-hidden="true">→</span></Link></section>;
 }))}{await (async()=>{
 let label="Состояние неизвестно";
 if(org.role!=="owner")label="Доступно владельцу";
 else try{const response=await request(`organizations/${org.id}/integrations/instagram`);if(response.ok){const result=await response.json();label=instagramStatuses[result.connection?.status??"not_connected"]??"Состояние неизвестно";}}catch{label="Состояние неизвестно";}
 return <section className="channel-card"><div className="channel-card-top"><span className="channel-card-symbol" aria-hidden="true"><Icon name="instagram"/></span><h2>Instagram</h2><StatusBadge tone="neutral">{label}</StatusBadge></div><p>Instagram Direct для профессиональных аккаунтов Business и Creator.</p><p className="channel-note">Только MOCK. Реальная авторизация и отправка выключены.</p><Link prefetch={false} className="button secondary" href={`/dashboard/integrations/instagram?org=${encodeURIComponent(org.id)}`}>Настройки Instagram <span aria-hidden="true">→</span></Link></section>;
 })()}</div>}
 {org?.role==="owner"&&<section className="panel" aria-label="Подготовка рабочих каналов">
  <h2>Подготовка рабочих каналов</h2>
  <p>Проверка реальных подключений требует отдельного допуска. Настройки тестового контура показаны выше.</p>
  <ul>
    <li>Telegram · выделенный бот, проверка токена через getMe и отдельный секрет HTTPS webhook. <a href="https://core.telegram.org/bots/api#setwebhook">Инструкция Telegram</a></li>
    <li>WhatsApp · аккаунт WABA, Phone Number ID, системный токен, App Secret и verify token. <a href="https://developers.facebook.com/documentation/business-messaging/whatsapp/get-started">Инструкция Meta</a></li>
    <li>Instagram · профессиональный аккаунт, OAuth приложения Meta и разрешение на сообщения. <a href="https://developers.facebook.com/documentation/instagram-platform/instagram-api-with-instagram-login">Инструкция Meta</a></li>
  </ul>
  <p>Передавайте секреты через защищённую серверную конфигурацию.</p>
 </section>}
 </main>;
}
