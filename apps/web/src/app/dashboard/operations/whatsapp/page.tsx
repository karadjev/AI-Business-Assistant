import {requireUser} from "@/lib/session";
import {WhatsAppOperator} from "@/components/WhatsAppOperator";
export default async function Page({searchParams}:{searchParams:Promise<{org?:string}>}){await requireUser();const {org}=await searchParams;if(!org||!/^[0-9a-f-]{36}$/.test(org))return <main id="main" tabIndex={-1}><h1>Укажите назначенный оператору магазин</h1></main>;return <main id="main" tabIndex={-1}><h1>Финансовая сверка WhatsApp</h1><WhatsAppOperator org={org}/></main>}
