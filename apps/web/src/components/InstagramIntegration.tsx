"use client";

import {useState} from "react";
import {StatusBadge} from "./ProductUI";

import {instagramStatuses,type InstagramState} from "@/lib/instagram";

export function InstagramIntegration({org,initial}:{org:string;initial:InstagramState}) {
 const [state,setState]=useState(initial),[busy,setBusy]=useState(false),[error,setError]=useState("");
 const root=`/api/v1/organizations/${org}/integrations/instagram`;
 async function call(path:string,method="GET") {
  const csrf=document.cookie.split("; ").find(x=>x.startsWith("aba_csrf="))?.slice(9)??"";
  const response=await fetch(path,{method,headers:method==="POST"?{"X-CSRF-Token":csrf,"Content-Type":"application/json"}:{},body:method==="POST"?"{}":undefined,cache:"no-store",redirect:"error"});
  const result=await response.json();
  if(!response.ok)throw new Error(result.error?.message??"Не удалось выполнить действие");
  return result;
 }
 async function action(kind:"connect"|"disconnect"|"refresh") {
  setBusy(true);setError("");
  try {
   if(kind==="connect") {
    const result=await call(root+"/oauth/start","POST");
    // Backend returns only a local synthetic callback. Never follow arbitrary URLs.
    const callback=new URL(result.callback_url,window.location.origin);
    if(callback.origin!==window.location.origin||callback.pathname!==root+"/oauth/callback")throw new Error("Некорректный адрес подключения");
    setState(await call(callback.pathname+callback.search));
   } else setState(await call(kind==="disconnect"?root+"/disconnect":root,kind==="disconnect"?"POST":"GET"));
  } catch(e) {setError(e instanceof Error?e.message:"Сервис временно недоступен");}
  finally {setBusy(false);}
 }
 const connection=state.connection,status=connection?.status??"not_connected";
 return <section className="channel-card" aria-busy={busy}>
  <div className="channel-card-top"><h2>Instagram</h2><StatusBadge tone={status==="connected"?"positive":["webhook_error","permission_error","reauthorization_required"].includes(status)?"warning":"neutral"}>{busy?"Подключение":instagramStatuses[status]??"Состояние неизвестно"}</StatusBadge></div>
  <p className="notice">Только синтетический режим MOCK. Реального подключения к Meta нет; реальные сообщения не принимаются и не отправляются.</p>
  <dl><dt>Username</dt><dd>{connection?.username??"Не подключён"}</dd><dt>Дата создания настройки</dt><dd>{connection?new Date(connection.created_at).toLocaleString("ru-RU"):"—"}</dd><dt>Webhook subscription</dt><dd>{connection?.webhook_subscribed?"Подтверждена в MOCK":"Не подтверждена"}</dd><dt>Получение события</dt><dd>{connection?.webhook_seen_at?"Подтверждено подписанным тестовым событием":"Не подтверждено"}</dd><dt>Доступ к сообщениям</dt><dd>{connection?.permissions.includes("instagram_business_manage_messages")?"Предоставлен в MOCK":"Не предоставлен"}</dd></dl>
  {status==="connecting"&&<p role="status">Ожидается подписанное тестовое событие webhook. Подключение ещё не подтверждено.</p>}
  <div className="catalog-inline"><button type="button" disabled={busy||!state.runtime_enabled} onClick={()=>action("connect")}>{connection&&status!=="not_connected"?"Переподключить MOCK Instagram":"Подключить MOCK Instagram"}</button><button type="button" className="secondary" disabled={busy} onClick={()=>action("refresh")}>Обновить статус</button>{connection&&status!=="not_connected"&&<button type="button" className="secondary" disabled={busy} onClick={()=>action("disconnect")}>Отключить</button>}</div>
  {!state.runtime_enabled&&<p className="field-hint">Тестовый backend Instagram выключен. Реальная авторизация недоступна.</p>}
  {error&&<p role="alert">{error}</p>}
 </section>;
}
