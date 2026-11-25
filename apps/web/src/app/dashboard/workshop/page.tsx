import Link from "next/link";
import {Workshop} from "@/components/Workshop";
import {organizations} from "@/lib/session";

export default async function Page({searchParams}: {searchParams: Promise<{org?: string}>}) {
  const {org} = await searchParams;
  const owned = (await organizations()).filter(item => item.role === "owner" && !item.is_test);
  const active = org ? owned.find(item => item.id === org) : owned[0];

  return <main id="main" tabIndex={-1}>
    <h1>Заявки автосервиса</h1>
    {owned.length > 1 && <nav aria-label="Рабочая организация">
      {owned.map(item => <p key={item.id}><Link href={`?org=${item.id}`} aria-current={item.id === active?.id ? "page" : undefined}>{item.name}</Link></p>)}
    </nav>}
    {active ? <Workshop key={active.id} org={active.id} name={active.name}/> : <p>Выберите рабочую организацию, в которой у вас есть права владельца.</p>}
  </main>;
}
