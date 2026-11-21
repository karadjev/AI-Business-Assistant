import Link from "next/link";
import {organizations,request} from "@/lib/session";
import {PageIntro} from "@/components/ProductUI";
import {InstagramIntegration} from "@/components/InstagramIntegration";
import type {InstagramState} from "@/lib/instagram";

export const metadata={title:"Instagram"};
export default async function Page({searchParams}:{searchParams:Promise<{org?:string}>}) {
 const q=await searchParams,list=await organizations();
 const org=list.find(o=>o.id===q.org)??(q.org?undefined:list[0]);
 let state:InstagramState|undefined,error="";
 if(org?.is_test&&org.role==="owner") {
  try {const response=await request(`organizations/${org.id}/integrations/instagram`);const result=await response.json();if(response.ok)state=result;else error=result.error?.message??"Не удалось получить состояние Instagram";}
  catch {error="Backend временно недоступен. Статус подключения неизвестен.";}
 }
 return <main id="main" tabIndex={-1}>
  <PageIntro eyebrow="Каналы связи" title="Instagram" description="Instagram Direct · проверка интеграции в тестовом магазине."/>
  {!org?<Link prefetch={false} href="/dashboard/organizations">Выберите магазин</Link>:!org.is_test||org.role!=="owner"?<p>Настройка доступна только владельцу тестового магазина.</p>:state?<InstagramIntegration key={org.id} org={org.id} initial={state}/>:<p role="alert">{error}</p>}
  <Link prefetch={false} href={`/dashboard/integrations${org?`?org=${encodeURIComponent(org.id)}`:""}`}>Все каналы связи</Link>
 </main>;
}
