import Link from "next/link";
import type {ReactNode} from "react";
const paths = {
  home:"m3 10 9-7 9 7v10H3Z M9 20v-7h6v7",
  chat:"M4 4h16v12H9l-5 4Z M8 8h8 M8 12h5",
  orders:"M7 4h10v3H7Z M5 6H4v15h16V6h-1 M8 12h8 M8 16h5",
  catalog:"M3 3h7v7H3Z M14 3h7v7h-7Z M3 14h7v7H3Z M14 14h7v7h-7Z",
  ai:"m12 3 2.5 6.5L21 12l-6.5 2.5L12 21l-2.5-6.5L3 12l6.5-2.5Z",
  settings:"M4 7h16 M4 17h16 M8 4v6 M16 14v6",
  arrow:"M4 12h16 m-6-6 6 6-6 6",
  channels:"M5 5h5v5H5Z M14 14h5v5h-5Z M10 7h7v7 M7 10v7h7",
  more:"M5 11h.01 M12 11h.01 M19 11h.01",
  chevron:"m9 5 7 7-7 7",
  search:"M21 21l-5-5 M18 10a8 8 0 1 1-16 0 8 8 0 0 1 16 0",
  check:"m5 12 4 4 10-10",
  sales:"M4 5h2l2 10h10l2-7H7 M9 19h.01 M17 19h.01",
  finance:"M3 6h18v12H3Z M6 12h.01 M18 12h.01 M14.5 12a2.5 2.5 0 1 1-5 0 2.5 2.5 0 1 1 5 0Z",
  marketing:"m4 10 5-1 10-4v14l-10-4-5-1Z M9 15l2 5 M21 10v4",
  analytics:"M4 4v16h16 M8 16v-3 M12 16V9 M16 16v-5 M20 16V6",
  telegram:"M3 11.2 21 4l-3.8 16-5.4-4.2-3 2.7.6-5.4L17 7.9l-9.1 5.3Z",
  whatsapp:"M5 17.7 3.6 21l3.8-1a9 9 0 1 0-2.4-2.3Z M8.3 8.2c.6 3.3 2.8 5.7 6.2 6.5l1.4-1.4-2-1.5-1 1a6.5 6.5 0 0 1-2.7-2.7l.8-1.1-1.4-2Z",
  instagram:"M8 3h8a5 5 0 0 1 5 5v8a5 5 0 0 1-5 5H8a5 5 0 0 1-5-5V8a5 5 0 0 1 5-5Z M16 12a4 4 0 1 1-8 0 4 4 0 0 1 8 0Z M17.5 6.5h.01",
} as const;
export type IconName=keyof typeof paths;

const agentIcons = new Map<string, IconName>([
  ["business_manager","ai"], ["sales","sales"], ["customer","chat"],
  ["product_inventory","catalog"], ["finance","finance"],
  ["marketing","marketing"], ["analytics","analytics"],
]);
export function AgentIcon({id}:{id:string}) {return <Icon name={agentIcons.get(id) ?? "ai"}/>;}
export function Icon({name}:{name:IconName}){return <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d={paths[name]}/></svg>}
export function PageIntro({eyebrow,title,description,children}:{eyebrow?:string;title:string;description?:string;children?:ReactNode}){return <div className="page-intro"><div>{eyebrow&&<span className="eyebrow">{eyebrow}</span>}<h1>{title}</h1>{description&&<p className="lead">{description}</p>}</div>{children&&<div className="page-actions">{children}</div>}</div>}
export function MetricCard({title,value,caption,action,href,icon}:{title:string;value:ReactNode;caption:string;action?:string;href:string;icon:IconName}){return <Link prefetch={false} className="overview-metric" href={href}><span className="metric-label"><Icon name={icon}/>{title}</span><strong>{value}</strong><span className="metric-caption">{caption}</span>{action&&<span className="metric-action">{action}<Icon name="arrow"/></span>}</Link>}
export function EmptyState({title,children,href,action}:{title:string;children:ReactNode;href?:string;action?:string}){return <section className="empty-state"><span className="empty-icon"><Icon name="catalog"/></span><h2>{title}</h2><p>{children}</p>{href&&<Link prefetch={false} className="button primary" href={href}>{action??"Продолжить"}<Icon name="arrow"/></Link>}</section>}
export function StatusBadge({children,tone="neutral"}:{children:ReactNode;tone?:"neutral"|"positive"|"warning"|"danger"}){return <span className={`status-badge status-${tone}`}>{children}</span>}
export function CatalogNavigation({org,current}:{org:string;current:string}){return <nav className="section-tabs" aria-label="Разделы каталога">{[["products","Товары"],["catalog","Качество каталога"],["inventory","Остатки"],["categories","Категории"],["onboarding","Импорт и настройка"]].map(([path,title])=><Link prefetch={false} key={path} href={`/dashboard/${path}?org=${org}`} aria-current={current===path?"page":undefined}>{title}</Link>)}</nav>}
export const channelLabels:Record<string,string>={telegram:"Telegram",whatsapp:"WhatsApp",instagram:"Instagram",internal_chat:"Тестовый чат"};
export const deliveryLabels:Record<string,string>={sent:"Отправлено",delivered:"Доставлено",read:"Прочитано",pending:"Ожидает отправки",failed:"Ошибка доставки",delivery_unknown:"Доставка неизвестна — проверьте вручную",unknown:"Доставка неизвестна — проверьте вручную",needs_approved_template:"Нужен согласованный шаблон",cancelled:"Отменено",blocked:"Отправка заблокирована"};
