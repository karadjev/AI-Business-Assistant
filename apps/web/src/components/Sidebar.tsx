"use client";
import {usePathname, useSearchParams} from "next/navigation";
import Link from "next/link";
import {useRef} from "react";
import {Icon, type IconName} from "./ProductUI";

type Item = {path: string; title: string; icon: IconName};
const main: Item[] = [{path:"",title:"Главная",icon:"home"},{path:"orders",title:"Заказы",icon:"orders"},{path:"products",title:"Товары",icon:"catalog"},{path:"conversations",title:"Диалоги",icon:"chat"}];
const management: Item[] = [{path:"ai-settings",title:"AI-ассистент",icon:"ai"},{path:"integrations",title:"Каналы связи",icon:"channels"},{path:"settings",title:"Настройки",icon:"settings"}];
const extra = [["workshop","Заявки автосервиса"],["onboarding","Начало работы"],["organizations","Мои магазины"],["catalog","Качество каталога"],["inventory","Остатки"],["categories","Категории"],["ai-playground","Тестовый чат"],["demo","Демонстрация"]];
export function Sidebar() {
  const org = useSearchParams().get("org");
  const pathname = usePathname();
  const more = useRef<HTMLDetailsElement>(null);
  const trigger = useRef<HTMLElement | null>(null);
  const part = pathname.split("/")[2] ?? "";
  const suffix = org ? `?org=${encodeURIComponent(org)}` : "";
  const selected = (path: string) => path === "products" ? ["products","catalog","inventory","categories"].includes(part) : path === "integrations" ? ["integrations","operations"].includes(part) : part === path;
  const href = (path: string) => `/dashboard${path ? "/" + path : ""}${suffix}`;
  const item = ({path,title,icon}: Item) => <Link prefetch={false} key={path} href={href(path)} aria-current={selected(path) ? "page" : undefined}><Icon name={icon}/><span>{title}</span></Link>;
  const close = () => {if (more.current) more.current.open = false;};
  return <aside className="sidebar" aria-label="Навигация магазина">
    <Link prefetch={false} className="workspace-identity" href={href("")}><span className="workspace-monogram" aria-hidden="true">A<span>✦</span></span><span><strong>AI Business</strong><small>Assistant</small></span><Icon name="chevron"/></Link>
    <span className="eyebrow sidebar-title">Обзор и продажи</span>
    <nav className="primary-nav" aria-label="Основные разделы магазина">{main.map(item)}</nav>
    <div className="desktop-management"><span className="eyebrow sidebar-title">Управление</span><nav className="management-nav" aria-label="Управление магазином">{management.map(item)}</nav></div>
    <details ref={more} className="secondary-nav" onKeyDown={event => {if (event.key === "Escape") {close(); trigger.current?.focus();}}}>
      <summary ref={node => {trigger.current = node;}}><Icon name="more"/><span>Ещё</span><Icon name="chevron"/></summary>
      <nav aria-label="Дополнительные разделы" onClick={event => {if ((event.target as HTMLElement).closest("a")) close();}}>
        {management.map(entry => <Link className="mobile-management-link" prefetch={false} key={entry.path} href={href(entry.path)} aria-current={selected(entry.path) ? "page" : undefined}><Icon name={entry.icon}/>{entry.title}</Link>)}
        {extra.map(([path,title]) => <Link prefetch={false} key={path} href={href(path)} aria-current={part === path ? "page" : undefined}>{title}</Link>)}
        <Link prefetch={false} href="/status">Состояние системы</Link>
      </nav>
    </details>
    <div className="sidebar-footer"><span className="status-badge status-neutral">{part === "workshop" ? "Рабочий контур" : "Тестовый режим"}</span><small>Решения остаются за вами</small></div>
  </aside>;
}
