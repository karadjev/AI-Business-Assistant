import Link from "next/link";
import {organizations,request} from "@/lib/session";
import {PageIntro} from "@/components/ProductUI";
import {CommercialPlan} from "@/components/CommercialPlan";
import {OrganizationTeam} from "@/components/OrganizationTeam";

export default async function Page({searchParams}:{searchParams:Promise<{org?:string}>}) {
 const id=(await searchParams).org;
 const list=await organizations();
 const org=id?list.find(item=>item.id===id):list[0];
 if(!org||org.role!=="owner")return <main id="main" tabIndex={-1} className="team-workspace"><PageIntro title="Сотрудники и ресурсы"/><p>Выберите организацию, где у вас есть права владельца.</p><Link href="/dashboard/organizations">Организации</Link></main>;
 const path=`organizations/${org.id}`;
 const [team,usage,commercial]=await Promise.all([request(path+"/team"),request(path+"/workspace-usage"),request(path+"/commercial")]);
 const usageData=usage.ok?await usage.json():null;
 return <main id="main" tabIndex={-1} className="team-workspace"><PageIntro title="Сотрудники и ресурсы"/>
  {team.ok&&usageData?<OrganizationTeam key={org.id} org={org.id} team={await team.json()} usage={usageData}/>:<p role="alert">Не удалось загрузить данные организации.</p>}
  {commercial.ok?<CommercialPlan key={org.id} org={org.id} state={await commercial.json()} legacyStaffLimit={usageData?.staff_limit}/>:<p role="alert">Тариф и лимиты временно недоступны.</p>}
 </main>;
}
