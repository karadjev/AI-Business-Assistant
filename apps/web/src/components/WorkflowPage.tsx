import Link from "next/link";
import {redirect} from "next/navigation";
import {organizations,request} from "@/lib/session";
import {InboxServiceBooking} from "./InboxServiceBooking";
import {InboxReview,type InboxDetails} from "./InboxReview";
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
  const page=/^[1-9][0-9]{0,3}$/.test(query.page??"")?Number(query.page):1;
  const filter=new URLSearchParams({page:String(page)});
  if(query.channel)filter.set("channel",query.channel);
  const [result,agent,inbox]=await Promise.all([load(root+"/conversations?"+filter),load(root+"/agent/status"),load(root+"/inbox")]);
  const chosen=query.chat??result.data?.items?.[0]?.id;
  const [detail,review]=chosen?await Promise.all([load(root+"/conversations/"+chosen),load(root+"/conversations/"+chosen+"/inbox")]):
   [{data:undefined,error:""},{data:undefined,error:""}];
  const error=result.error||agent.error||detail.error||inbox.error||review.error;
  if(error)return <main id="main" tabIndex={-1}><h1>Диалог недоступен</h1><p role="alert">{error}</p><ReloadSection/></main>;
  const inboxURL=(p:number)=>`/dashboard/conversations?org=${org.id}&page=${p}&channel=${encodeURIComponent(query.channel??"")}`;
  return <>
   <nav className="section-tabs channel-tabs" aria-label="Фильтр каналов">{[["","Все каналы"],["telegram","Telegram"],["whatsapp","WhatsApp"],["instagram","Instagram"]].map(([channel,label])=><Link prefetch={false} aria-current={(query.channel??"")===channel?"page":undefined} key={channel} href={`/dashboard/conversations?org=${org.id}&channel=${channel}`}>{label}</Link>)}</nav>
   <Workflows key={org.id+":"+JSON.stringify(detail.data)} org={org} organizations={list} view={view} conversations={result.data.items} conversation={detail.data} mode={agent.data.mode}/>
   <nav className="section-tabs" aria-label="Страницы обращений">{page>1&&<Link href={inboxURL(page-1)}>Предыдущая</Link>}{result.data.items.length===20&&page<9999&&<Link href={inboxURL(page+1)}>Следующая</Link>}</nav>
   {chosen && detail.data?.status === "open" && detail.data.channel === "internal_chat" && <InboxServiceBooking key={chosen} org={org.id} conversation={chosen}/>}
   <InboxReview key={chosen??org.id} org={org.id} conversation={chosen} owner={org.role==="owner"} enabled={inbox.data.review_enabled} details={review.data as InboxDetails|undefined}/>
  </>;

 }
 const params=new URLSearchParams();if(query.status)params.set("status",query.status);if(query.page)params.set("page",query.page);
 const result=await load(root+(view==="order"?"/orders/"+id:"/orders?"+params));if(result.error)return <main id="main" tabIndex={-1}><h1>Заявки недоступны</h1><p role="alert">{result.error}</p><Link prefetch={false} href={`/dashboard/orders?org=${org.id}`}>Обновить список</Link></main>;
 return <Workflows key={org.id+":"+JSON.stringify(result.data)} org={org} organizations={list} view={view} order={view==="order"?result.data:undefined} orders={view==="orders"?result.data.items:undefined}/>;
}
