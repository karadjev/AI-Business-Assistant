import Link from "next/link";
import {money} from "@/lib/catalog";

const labels: Record<string,string> = {
  open_conversations:"Открытые обращения",awaiting_staff:"Ожидают ответа сотрудника",
  requests_today:"Новые заявки",pending_confirmation:"Заявки к подтверждению",confirmed_open:"Подтверждённые в работе",
  stock_zero:"Нет доступного остатка",stock_unknown:"Остаток требует сверки",unassigned_bookings:"Записи без исполнителя сегодня",
  eligible_service_conversations:"Клиенты с согласием и оплаченной услугой",
  paid_services_today:"Услуги с подтверждённой оплатой",received_minor:"Подтверждённые поступления за услуги",
  cost_minor:"Учтённая себестоимость оплаченных услуг",ai_settled_microrub:"Учтённые AI-расходы",ai_unsettled_count:"Незавершённый учёт AI-расходов",
  bookings_today:"Записи на сегодня",assigned_today:"Назначенные записи",employees_booked_today:"Задействованные сотрудники",booked_minutes_today:"Запланировано минут",
};
const agents: Record<string,{name:string;path?:string}> = {
  customer:{name:"Клиенты",path:"conversations"},sales:{name:"Продажи",path:"orders"},
  product_inventory:{name:"Товары и ресурсы",path:"orders"},marketing:{name:"Маркетинг"},
  finance:{name:"Финансы",path:"orders"},analytics:{name:"Аналитика",path:"orders"},
};

export type DailyChild={agent_id:string;status:string;result:unknown};
function isRecord(value:unknown):value is Record<string,unknown> {
  return typeof value==="object" && value!==null && !Array.isArray(value);
}
function firstResult(value:unknown) {
  if (!Array.isArray(value) || !isRecord(value[0])) return null;
  return value[0];
}
function amount(key:string,value:number|null) {
  if (value===null) return "Нет подтверждённых данных";
  if (key.endsWith("_microrub")) return new Intl.NumberFormat("ru-RU",{style:"currency",currency:"RUB",maximumFractionDigits:6}).format(value/1_000_000);
  if (key.endsWith("_minor")) return money(value);
  return value.toLocaleString("ru-RU");
}

export function DailyChildren({items,org}:{items:DailyChild[];org:string}) {
  return <ul className="order-timeline">{items.map(item=>{
    const agent=agents[item.agent_id];
    const result=firstResult(item.result);
    const metrics=isRecord(result?.metrics)?result.metrics:null;
    const proposals=item.agent_id==="marketing" && Array.isArray(result?.proposals)
      ? result.proposals.filter((entry):entry is {conversation_id:string;offering_name:string;text:string}=>isRecord(entry)
        && typeof entry.conversation_id==="string" && /^[0-9a-f-]{36}$/i.test(entry.conversation_id)
        && typeof entry.offering_name==="string" && typeof entry.text==="string") : [];
    if (!agent) return null;
    return <li key={item.agent_id}>
      <strong>{agent.name}</strong>
      {metrics && typeof result?.date==="string" && <p>Сводка за {result.date}</p>}
      {item.status==="completed" && metrics ? <dl>{Object.entries(metrics).map(([key,value]) =>
        labels[key] && (typeof value==="number" || value===null) ? <div key={key}><dt>{labels[key]}</dt><dd>{amount(key,value)}</dd></div> : null
      )}</dl> : <p>{["queued","pending","running"].includes(item.status)?"Сводка готовится":"Результат отсутствует. Проверьте статус задания."}</p>}
      {item.agent_id==="finance" && metrics && !("ai_settled_microrub" in metrics) && <p>Учтённых AI-расходов за этот день нет.</p>}
      {proposals.length>0 && <>
        <p>Внутренние черновики на момент сводки. Перед любым использованием нужно проверить актуальность согласия. Отправка отключена.</p>
        <ul>{proposals.map(draft=><li key={draft.conversation_id}>
          <p>{draft.text}</p>
          <Link href={`/dashboard/conversations?org=${org}&chat=${draft.conversation_id}`}>Открыть исходное обращение</Link>
        </li>)}</ul>
      </>}
      {agent.path &&
        <Link href={`/dashboard/${agent.path}?org=${org}`}>Открыть раздел «{agent.name}»</Link>}
    </li>;
  })}</ul>;
}

export function DailyAgentReport({result,org}:{result:unknown;org:string}) {
  const report=firstResult(result);
  if (!report || !Array.isArray(report.children) || typeof report.date!=="string") return null;
  const children=report.children.filter((item):item is DailyChild=>isRecord(item)&&typeof item.agent_id==="string"&&typeof item.status==="string");
  const notes=Array.isArray(report.recommendations)?report.recommendations.filter((item):item is string=>typeof item==="string"):[];
  return <section aria-label="Ежедневная сводка владельца">
    <h5>Ежедневная сводка за {report.date}</h5>
    <DailyChildren items={children} org={org}/>
    {notes.length>0 && <ul>{notes.map(note=><li key={note}>{note}</li>)}</ul>}
  </section>;
}
