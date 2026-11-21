import Link from "next/link";
import {organizations} from "@/lib/session";
import {StoreOnboarding} from "@/components/StoreOnboarding";
export default async function Page({searchParams}:{searchParams:Promise<{org?:string}>}){const {org:id}=await searchParams;const list=await organizations();const org=list.find(x=>x.id===id)??(!id?list.find(x=>x.role==='owner'):undefined);if(!org||org.role!=='owner')return <main id="main" tabIndex={-1}><h1>Нужны права владельца магазина</h1><Link prefetch={false} href="/dashboard/organizations">Выбрать или явно создать магазин</Link></main>;return <main id="main" tabIndex={-1}><h1>Первоначальная настройка · {org.name}</h1><StoreOnboarding key={org.id} org={org.id}/></main>}
