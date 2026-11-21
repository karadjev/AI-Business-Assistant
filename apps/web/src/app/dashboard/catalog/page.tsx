import Link from 'next/link';
import {ReloadSection} from '@/components/ReloadSection';
import {organizations,request} from '@/lib/session';
import {CatalogControlCenter} from '@/components/CatalogControlCenter';
export default async function Page({searchParams}:{searchParams:Promise<{org?:string}>}){
 const {org:id}=await searchParams;const list=await organizations();const org=list.find(x=>x.id===id)??(!id?list.find(x=>x.role==='owner'):undefined);
 if(!org||org.role!=='owner')return <main id="main" tabIndex={-1}><h1>Центр управления каталогом</h1><p>Выберите магазин, которым вы управляете как владелец.</p><Link prefetch={false} href="/dashboard/organizations">Мои магазины</Link></main>;
 const r=await request(`organizations/${org.id}/catalog-control?page=1`);if(!r.ok)return <main id="main" tabIndex={-1}><h1>Каталог недоступен</h1><p role="alert">Не удалось получить проверенные показатели. Это не означает отсутствие товаров или ошибок. Повторите загрузку, когда сервис будет доступен.</p><ReloadSection/></main>;
 return <CatalogControlCenter key={org.id} org={org.id} initial={await r.json()} shops={list.filter(x=>x.role==='owner').map(x=>({id:x.id,name:x.name}))}/>;
}
