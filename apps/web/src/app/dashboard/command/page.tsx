import Link from "next/link";
import {IntelligenceCore, intelligenceTeam} from "@/components/IntelligenceCore";
import {CommandTasks} from "@/components/CommandTasks";
import {Workshop} from "@/components/Workshop";
import {workshopOverview} from "@/lib/workshop-overview";
import {organizations} from "@/lib/session";

export const metadata = {title:"Business Manager"};

export default async function CommandPage({searchParams}: {searchParams:Promise<{org?:string}>}) {
  const list = await organizations();
  const id = (await searchParams).org;
  const org = id ? list.find(item => item.id === id) : list[0];
  const workspace = org?.role === "owner" && !org.is_test ? await workshopOverview(org.id) : null;
  return <main id="main" tabIndex={-1} className="command-center">
    <section className="command-feature">
      <div>
        <p className="intelligence-kicker"><span/>{org?.is_test ? "Тестовое пространство" : "AI-команда"}</p>
        <h1>Business Manager</h1>
        <p>Выберите поручение, проследите за работой специалистов и проверьте результат перед согласованием.</p>
        {workspace && <a className="button primary" href="#workshop-manager">Открыть рабочие поручения</a>}
        {org?.is_test && org.role === "owner" && <a className="button primary" href="#agent-create-heading">Выбрать поручение</a>}
        {!org && <Link className="button primary" href={id ? "/dashboard/organizations" : "/dashboard/organizations/new"}>{id ? "Выбрать организацию" : "Создать организацию"}</Link>}
      </div>
      <IntelligenceCore compact/>
    </section>
    {org?.is_test && <CommandTasks key={org.id} org={org}/>}
    {org && workspace && <Workshop key={org.id} org={org.id} name={org.name} timezone={org.timezone}/>}
    {org && !org.is_test && org.role === "owner" && !workspace && <p className="panel">Рабочие поручения сейчас недоступны. Проверьте допуск организации или откройте <Link href="/#live-demo">публичное демо</Link>.</p>}
    {org && !org.is_test && org.role !== "owner" && <p className="panel">Рабочие поручения доступны владельцу организации.</p>}
    {!org?.is_test && <details className="panel command-roles">
      <summary>Роли AI-команды</summary>
      <dl>{intelligenceTeam.map(agent => <div key={agent.name}><dt>{agent.name}</dt><dd>{agent.role}</dd></div>)}</dl>
    </details>}
  </main>;
}
