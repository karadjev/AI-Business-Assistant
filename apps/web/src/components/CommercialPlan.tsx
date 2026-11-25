"use client";

import {useRouter} from "next/navigation";
import {useState, type FormEvent} from "react";
import {csrf} from "./IdentityForm";

const caps = [
  {key:"staff_limit", usage:"staff", label:"Сотрудники", max:1000},
  {key:"agent_limit", usage:"agents", label:"Агенты за месяц", max:7},
  {key:"channel_limit", usage:"channels", label:"Активные каналы", max:3},
  {key:"task_limit", usage:"tasks", label:"Задачи за месяц", max:1000000},
  {key:"ai_monthly_cap", usage:"ai_exposure", label:"Расходы и резервы AI за месяц, мк₽", max:1000000000000},
] as const;
export type PlanState = {
  subscription:{plan_code:string;status:"active"|"paused";version:number} & Partial<Record<(typeof caps)[number]["key"],number|null>>;
  can_edit:boolean;
  cycle_start:string;
  plans:{code:string;name:string}[];
  usage:Record<(typeof caps)[number]["usage"],number>;
};

const blockReasons:Record<(typeof caps)[number]["usage"],string> = {
  staff:"Приглашения останутся открыты, пока не появится свободное место.",
  agents:"Новые типы агентов недоступны до следующего месяца. Ранее использованные агенты учитываются в лимите задач.",
  channels:"Дополнительное подключение недоступно, пока не освободится место для канала.",
  tasks:"Новые задачи недоступны до начала следующего месяца UTC или изменения лимита.",
  ai_exposure:"Новые расходы ограничены. В расчёт входят резервы и затраты с неподтверждённой суммой.",
};

export function CommercialPlan({org,state,legacyStaffLimit}:{org:string;state:PlanState;legacyStaffLimit?:number|null}) {
  const router=useRouter();
  const [message,setMessage]=useState("");
  const [error,setError]=useState("");
  const [busy,setBusy]=useState(false);
  const {subscription,usage}=state;
  const active=subscription.status==="active";
  const usageStart=new Intl.DateTimeFormat("ru-RU",{dateStyle:"medium",timeZone:"UTC"}).format(new Date(state.cycle_start));
  const effectiveLimit=(cap:(typeof caps)[number])=>{
    const configured=subscription[cap.key];
    if(cap.usage!=="staff"||legacyStaffLimit==null)return configured;
    return configured==null?legacyStaffLimit:Math.min(configured,legacyStaffLimit);
  };

  async function save(event:FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError("");
    setMessage("");
    const form=new FormData(event.currentTarget);
    const limits=Object.fromEntries(caps.map(cap=>{
      const value=String(form.get(cap.key)??"").trim();
      return [cap.key,value===""?null:Number(value)];
    }));
    try {
      const response=await fetch(`/api/v1/organizations/${org}/commercial`,{
        method:"PUT",headers:{"Content-Type":"application/json","X-CSRF-Token":csrf()},
        body:JSON.stringify({plan_code:form.get("plan_code"),status:form.get("status"),
          expected_version:subscription.version,...limits}),signal:AbortSignal.timeout(8000),
      });
      if (!response.ok) {
        const issue=await response.json();
        throw new Error(issue.error?.message??"Подписку не удалось сохранить");
      }
      setMessage("Изменения сохранены");
      router.refresh();
    } catch (reason) {
      setError(reason instanceof Error?reason.message:"Подписку не удалось сохранить");
    } finally {setBusy(false);}
  }

  return <section className="panel" aria-label="Тариф и лимиты">
    <h2>Тариф и лимиты</h2>
    <p>{state.plans.find(plan=>plan.code===subscription.plan_code)?.name??subscription.plan_code} · {active?"Активен":"Приостановлен"}</p>
    {active
      ?<p>Возможности тарифа включают управление сотрудниками, задачи агентов и настройки допущенных каналов. Подключение рабочих каналов согласуется отдельно.</p>
      :<p role="status">Подписка приостановлена. История доступна. Новые задачи, места сотрудников, подключения и расходы AI заблокированы.</p>}
    <p className="field-hint">Месячный учёт с {usageStart} UTC. Сотрудники и каналы учитываются по текущему состоянию.</p>
    <dl>{caps.map(cap=>{
      const limit=effectiveLimit(cap);
      const used=usage[cap.usage];
      return <div key={cap.key}>
        <dt>{cap.label}</dt>
        <dd>{used.toLocaleString("ru-RU")} / {limit?.toLocaleString("ru-RU")??"без лимита"}
          {active&&limit!=null&&used>=limit&&<p>{blockReasons[cap.usage]}</p>}
          {cap.usage==="staff"&&legacyStaffLimit!=null&&(subscription.staff_limit==null||legacyStaffLimit<subscription.staff_limit)&&
            <p>Действует ранее установленный лимит мест.</p>}
        </dd>
      </div>;
    })}</dl>
    {state.can_edit&&<form key={subscription.version} onSubmit={save}>
      <label>Тариф<select name="plan_code" defaultValue={subscription.plan_code}>{state.plans.map(plan=><option value={plan.code} key={plan.code}>{plan.name}</option>)}</select></label>
      <label>Статус подписки<select name="status" defaultValue={subscription.status}><option value="active">Активна</option><option value="paused">Приостановлена</option></select></label>
      {caps.map(cap=><label key={cap.key}>{cap.label}<input name={cap.key} type="number" min={0} max={cap.max} step={1} defaultValue={subscription[cap.key]??""}/></label>)}
      <p className="field-hint">Пустое поле снимает лимит подписки. Действующие ограничения безопасности сохраняются.</p>
      <button disabled={busy}>Сохранить тариф и лимиты</button>
    </form>}
    {message&&<p role="status">{message}</p>}
    {error&&<p role="alert">{error}</p>}
  </section>;
}
