import Link from "next/link";
import {redirect} from "next/navigation";
import {organizations,request} from "@/lib/session";
import {ReloadSection} from "./ReloadSection";
import {Workflows,SandboxForm} from "./Workflows";
export async function WorkflowPage({view,query,id}:{view:"chat"|"orders"|"order";query:Record<string,string|undefined>;id?:string}){
 const list=await organizations();const org=list.find(o=>o.id===query.org)??(query.org?undefined:list[0]);
 if(!org)return <main id="main" tabIndex={-1}><h1>Выберите магазин</h1><Link prefetch={false} href="/dashboard/organizations">Организации</Link><SandboxForm/></main>;
 if(!org.is_test)return <main id="main" tabIndex={-1}><h1>Тестовый контур</h1><p className="lead">{org.name} — обычный магазин. Тестовый чат и заявки не используют его склад.</p><SandboxForm/><p>Уже есть тестовый магазин? {list.filter(o=>o.is_test).map(o=><Link prefetch={false} className="button secondary" key={o.id} href={`/dashboard/ai-playground?org=${o.id}`}>{o.name}</Link>)}</p></main>;
 if(!org.can_manage_orders)return <main id="main" tabIndex={-1}><h1>Недостаточно прав</h1><p role="alert">Владелец должен выдать право staff на заявки и тестовый чат.</p></main>;
 const root=`organizations/${org.id}`;
 async function load(path:string){const r=await request(path);if(r.status===401)redirect("/login");if(!r.ok){const result=await r.json();return {data:null,error:result.error?.message??`Ошибка ${r.status}`};}return {data:await r.json(),error:""};}
 if(view==="chat"){
  const result=await load(root+"/conversations"+(query.channel?"?channel="+encodeURIComponent(query.channel):""));const agent=await load(root+"/agent/status");const chosen=query.chat??result.data?.items?.[0]?.id;const detail=chosen?await load(root+"/conversations/"+chosen):{data:undefined,error:""};const error=result.error||agent.error||detail.error;
  if(error)return <main id="main" tabIndex={-1}><h1>Диалог недоступен</h1><p role="alert">{error}</p><ReloadSection/></main>;
  return <><nav className="section-tabs channel-tabs" aria-label="Фильтр каналов">{[["","Все каналы"],["telegram","Telegram"],["whatsapp","WhatsApp"],["instagram","Instagram"]].map(([channel,label])=><Link prefetch={false} aria-current={(query.channel??"")===channel?"page":undefined} key={channel} href={`/dashboard/conversations?org=${org.id}&channel=${channel}`}>{label}</Link>)}</nav><Workflows key={org.id+":"+JSON.stringify(detail.data)} org={org} organizations={list} view={view} conversations={result.data.items} conversation={detail.data} mode={agent.data.mode}/></>;
 }
 const params=new URLSearchParams();if(query.status)params.set("status",query.status);if(query.page)params.set("page",query.page);
 const result=await load(root+(view==="order"?"/orders/"+id:"/orders?"+params));if(result.error)return <main id="main" tabIndex={-1}><h1>Заявки недоступны</h1><p role="alert">{result.error}</p><Link prefetch={false} href={`/dashboard/orders?org=${org.id}`}>Обновить список</Link></main>;
 return <Workflows key={org.id+":"+JSON.stringify(result.data)} org={org} organizations={list} view={view} order={view==="order"?result.data:undefined} orders={view==="orders"?result.data.items:undefined}/>;
}
