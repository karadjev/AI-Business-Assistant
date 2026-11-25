"use client";

import {usePathname, useSearchParams} from "next/navigation";
import Link from "next/link";
import {useEffect, useRef, useState} from "react";
import {Icon, type IconName} from "./ProductUI";
import type {Organization} from "@/lib/session";

type Item = {path:string; title:string; icon:IconName};
const home:Item = {path:"",title:"Обзор",icon:"home"};
const command:Item = {path:"command",title:"Business Manager",icon:"ai"};
const orgs:Item = {path:"organizations",title:"Организации",icon:"home"};
const addOrg:Item = {path:"organizations/new",title:"Создать организацию",icon:"home"};
const products:Item = {path:"products",title:"Товары",icon:"catalog"};
const orders:Item = {path:"orders",title:"Заявки",icon:"orders"};
const inbox:Item = {path:"conversations",title:"Inbox",icon:"chat"};
const integrations:Item = {path:"integrations",title:"Каналы связи",icon:"channels"};
const settings:Item = {path:"settings",title:"Настройки",icon:"settings"};
const team:Item = {path:"team",title:"Сотрудники и подписка",icon:"settings"};
const stock:Item = {path:"inventory",title:"Остатки",icon:"catalog"};
const catalog:Item = {path:"catalog",title:"Качество каталога",icon:"catalog"};
const onboarding:Item = {path:"onboarding",title:"Настройка магазина",icon:"settings"};
const aiSettings:Item = {path:"ai-settings",title:"Настройки AI",icon:"settings"};
const demo:Item = {path:"demo",title:"Демонстрация",icon:"ai"};
const withoutOrg = new Set(["organizations","organizations/new"]);

export function Sidebar({organizations}: {organizations:Organization[]}) {
  const selectedId = useSearchParams().get("org");
  const pathname = usePathname();
  const part = pathname.replace(/^\/dashboard\/?/, "");
  const active = selectedId ? organizations.find(org => org.id === selectedId) : organizations[0];
  const suffix = active ? `?org=${encodeURIComponent(active.id)}` : "";
  const owner = active?.role === "owner";
  const main = active ? [home,command,products,...(active.is_test && active.can_manage_orders ? [orders,inbox] : [])] : [home,orgs];
  const extra = [orgs, ...(active ? [integrations,stock] : [addOrg]),
    ...(active?.is_test ? [aiSettings,demo] : []), ...(owner ? [settings,team,catalog,onboarding] : [])];
  const [collapsed,setCollapsed] = useState(false);
  const [query,setQuery] = useState("");
  const more = useRef<HTMLDetailsElement>(null);
  const orgPicker = useRef<HTMLDetailsElement>(null);
  const dialog = useRef<HTMLDialogElement>(null);

  useEffect(() => {
    function keydown(event:KeyboardEvent) {
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        if (dialog.current?.open) dialog.current.close();
        else {setQuery("");dialog.current?.showModal();}
      }
    }
    window.addEventListener("keydown",keydown);
    return () => window.removeEventListener("keydown",keydown);
  }, []);

  const href = (path:string) => `/dashboard${path ? "/" + path : ""}${withoutOrg.has(path) ? "" : suffix}`;
  const selected = (path:string) => path === "" ? part === "" : part === path || (path !== "organizations" && part.startsWith(path+"/"));
  const item = ({path,title,icon}:Item) => <Link prefetch={false} key={path} href={href(path)} title={title} aria-current={selected(path) ? "page" : undefined} onClick={() => {if (more.current) more.current.open = false;}}><Icon name={icon}/><span>{title}</span></Link>;
  const matches = [...new Map([...main,...extra].map(entry => [entry.path,entry])).values()].filter(entry => entry.title.toLocaleLowerCase("ru").includes(query.trim().toLocaleLowerCase("ru")));

  return <aside className="sidebar" data-collapsed={collapsed} aria-label="Навигация рабочего пространства">
    <div className="sidebar-top">
      <Link prefetch={false} className="workspace-identity" href={href("")} title="AI Business Assistant"><span className="workspace-monogram"><Icon name="ai"/></span><span><strong>AI Business</strong><small>Assistant</small></span></Link>
      <button className="sidebar-collapse" type="button" aria-label={collapsed ? "Развернуть навигацию" : "Свернуть навигацию"} aria-expanded={!collapsed} aria-controls="sidebar-primary sidebar-extra" onClick={() => setCollapsed(value => !value)}><Icon name="chevron"/></button>
    </div>
    <details className="workspace-org-picker" ref={orgPicker}>
      <summary title={active?.name ?? "Организации"}><Icon name="home"/><span>{active?.name ?? "Организации"}</span><Icon name="chevron"/></summary>
      <nav aria-label="Переключить организацию">
        {organizations.map(org => <Link prefetch={false} key={org.id} href={`/dashboard?org=${org.id}`} aria-current={org.id === active?.id ? "true" : undefined} onClick={() => {if (orgPicker.current) orgPicker.current.open = false;}}>{org.name}</Link>)}
        <Link prefetch={false} href="/dashboard/organizations" onClick={() => {if (orgPicker.current) orgPicker.current.open = false;}}>Все организации</Link>
      </nav>
    </details>
    <button type="button" className="sidebar-palette" aria-label="Быстрый переход" onClick={() => {setQuery("");dialog.current?.showModal();}}><Icon name="search"/><span>Быстрый переход</span><kbd>⌘ K</kbd></button>
    <nav id="sidebar-primary" className="primary-nav" aria-label="Основные разделы" style={{"--mobile-nav-count":main.length} as React.CSSProperties}>{main.map(item)}</nav>
    <details id="sidebar-extra" ref={more} className="secondary-nav" onKeyDown={event => {
      if (event.key === "Escape" && more.current?.open) {more.current.open = false;more.current.querySelector("summary")?.focus();}
    }}>
      <summary><Icon name="more"/><span>Ещё</span><Icon name="chevron"/></summary>
      <nav aria-label="Дополнительные разделы">{extra.map(item)}</nav>
    </details>
    <dialog className="command-palette" ref={dialog} aria-labelledby="navigation-dialog-title">
      <div className="palette-heading"><h2 id="navigation-dialog-title">Быстрый переход</h2><button type="button" onClick={() => dialog.current?.close()}>Закрыть</button></div>
      <label>Название раздела<input autoFocus value={query} onChange={event => setQuery(event.target.value)} placeholder="Например, остатки"/></label>
      <ul>{matches.map(entry => <li key={entry.path}><Link prefetch={false} href={href(entry.path)} onClick={() => dialog.current?.close()}><Icon name={entry.icon}/>{entry.title}</Link></li>)}</ul>
      {matches.length === 0 && <p role="status">Разделы не найдены.</p>}
    </dialog>
  </aside>;
}
