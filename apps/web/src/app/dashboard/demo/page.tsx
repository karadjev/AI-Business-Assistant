import Link from 'next/link';
import {organizations} from '@/lib/session';
import {DemoJourney} from '@/components/DemoJourney';
export default async function Page({searchParams}:{searchParams:Promise<{org?:string;channel?:string;step?:string}>}){
 const q=await searchParams;const list=await organizations();
 if(!q.org)return <main id="main" tabIndex={-1} className="cc"><span className="eyebrow">DEMO · OFFLINE</span><h1>Выберите демонстрационный магазин</h1><p>Для полного показа оператор подготавливает отдельный магазин с синтетическими данными. Вход и права обязательны; рабочие данные не используются.</p><section className="module-grid">{list.filter(o=>o.is_test&&o.can_manage_orders).map(o=><article className="module-card" key={o.id}><h2>{o.name}</h2><Link prefetch={false} href={`/dashboard/demo?org=${o.id}`}>Начать показ</Link></article>)}</section>{!list.some(o=>o.is_test&&o.can_manage_orders)&&<p>Подготовленных тестовых магазинов с доступом пока нет. Попросите оператора подготовить демонстрацию.</p>}<Link prefetch={false} href="/dashboard/organizations">Мои магазины</Link></main>;
 const org=list.find(o=>o.id===q.org);
 if(!org?.is_test||!org.can_manage_orders)return <main id="main" tabIndex={-1}><h1>Демонстрация недоступна</h1><p role="alert">Нужен отдельный тестовый магазин и право работать с заявками. Вход и права обязательны.</p><Link prefetch={false} href="/dashboard/organizations">Мои магазины</Link></main>;
 const channel=q.channel==='whatsapp'?'whatsapp':'telegram';const step=Number(q.step??0);
 return <DemoJourney key={org.id+channel} org={org} channel={channel} initialStep={Number.isInteger(step)&&step>=0&&step<9?step:0}/>;
}
