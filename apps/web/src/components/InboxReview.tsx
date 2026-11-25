"use client";

import Link from "next/link";
import {useEffect, useState} from "react";
import {useRouter} from "next/navigation";
import {csrf} from "./IdentityForm";

export type InboxDetails = {
  customer: {id:string;channel:string};
  review_enabled:boolean;
  version:number;
  assignee_id:string|null;
  operators:{id:string;name:string}[];
  intent:string|null;
  processing_state:string|null;
  delivery:{id:string; status:string}[];
  drafts:{task_id:string;body:string;status:string;expires_at:string}[];
};

const deliveries:Record<string,string> = {sent:"Тестовая доставка выполнена", pending:"В очереди", sending:"Тестовая доставка выполняется",
  failed:"Ошибка", cancelled:"Отменена", delivery_unknown:"Доставка не подтверждена", needs_approved_template:"Требуется одобренный шаблон"};
const decisions:Record<string,string> = {pending:"На согласовании", approved:"Одобрен", rejected:"Отклонён", expired:"Срок истёк"};

export function InboxReview({org,conversation,owner,enabled,details}:{
  org:string;conversation?:string;owner:boolean;enabled:boolean;details?:InboxDetails;
}) {
  const router=useRouter();
  const [busy,setBusy]=useState(false);
  const [error,setError]=useState("");
  useEffect(()=>{
    if (!conversation) return;
    const timer=window.setInterval(()=>{if (document.visibilityState==="visible") router.refresh();},8000);
    return ()=>window.clearInterval(timer);
  },[conversation,router]);

  async function submit(route:string, body:object) {
    setBusy(true);
    setError("");
    try {
      const response=await fetch(`/api/v1/organizations/${org}/${route}`,{
        method:"POST", headers:{"Content-Type":"application/json","X-CSRF-Token":csrf()},
        body:JSON.stringify(body), signal:AbortSignal.timeout(8000),
      });
      if (!response.ok) {
        const data=await response.json();
        throw new Error(data.error?.message??"Действие недоступно");
      }
      router.refresh();
    } catch (reason) {
      setError(reason instanceof Error?reason.message:"Не удалось выполнить действие");
    } finally {
      setBusy(false);
    }
  }

  return <section className="panel" aria-label="Состояние обращения">
    <h2>Согласование ответов AI</h2>
    {enabled?<p>Ответы управляющего в тестовом Inbox проходят согласование владельца.</p>:<p>Согласование ответов управляющего выключено.</p>}
    {owner&&!enabled&&<button type="button" disabled={busy} onClick={()=>void submit("inbox",{})}>Включить согласование в тестовом контуре</button>}
    {details&&<><p><strong>Карточка клиента</strong> · {details.customer.channel} · {details.customer.id.slice(0,8)}</p>
      {details.assignee_id&&<p>Назначен сотрудник: {details.operators.find(user=>user.id===details.assignee_id)?.name??"Диалог назначен"}</p>}
      {owner&&conversation&&details.customer.channel!=="internal_chat"&&<form onSubmit={event=>{
        event.preventDefault();
        const assignee=String(new FormData(event.currentTarget).get("assignee")??"");
        void submit(`conversations/${conversation}/inbox/assignee`,{assignee:assignee||null,expected_version:details.version});
      }}>
        <label>Ответственный сотрудник<select name="assignee" key={details.assignee_id??"none"} defaultValue={details.assignee_id??""}>
          <option value="">Без назначения</option>
          {details.operators.map(operator=><option key={operator.id} value={operator.id}>{operator.name}</option>)}
        </select></label>
        <button disabled={busy}>Передать сотруднику</button>
      </form>}
      {details.processing_state&&<p>Состояние обработки · {details.processing_state}</p>}
      {conversation&&details.drafts.map(draft=><article key={draft.task_id}>
        <p><strong>{decisions[draft.status]??draft.status}</strong> · {new Date(draft.expires_at).toLocaleString("ru-RU")}</p>
        <p>{draft.body}</p>
        {owner&&draft.status==="pending"&&<>
          <button type="button" disabled={busy} onClick={()=>void submit(`conversations/${conversation}/inbox/${draft.task_id}`,{decision:"approved"})}>Одобрить тестовый ответ</button>
          <button type="button" disabled={busy} onClick={()=>void submit(`conversations/${conversation}/inbox/${draft.task_id}`,{decision:"rejected"})}>Отклонить</button>
        </>}
      </article>)}
      {!details.drafts.length&&<p>Черновиков для этого обращения пока нет.</p>}
      {!!details.delivery.length&&<ul aria-label="Статусы доставки">{details.delivery.map(item=><li key={item.id}>{deliveries[item.status.toLowerCase()]??item.status}</li>)}</ul>}
    </>}
    {error&&<p role="alert">{error}</p>}
    <Link href={`/dashboard/integrations?org=${org}`}>Управление каналами</Link>
  </section>;
}
