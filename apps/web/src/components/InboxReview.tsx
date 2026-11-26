"use client";

import Link from "next/link";
import {useEffect, useRef, useState} from "react";
import {useRouter} from "next/navigation";
import {statuses} from "@/lib/workflows";
import {channelLabels} from "./ProductUI";
import {csrf} from "./IdentityForm";

export type InboxDetails = {
  customer: {id:string;channel:string};
  review_enabled:boolean;
  version:number;
  assignee_id:string|null;
  operators:{id:string;name:string}[];
  intent:string|null;
  processing_state:string|null;
  requests:{id:string;status:string}[];
  consents:{channel:string;source:string;source_message_id:string;occurred_at:string;decision:"granted"|"revoked"}[];
  delivery:{id:string;status:string;attempts:number;created_at:string}[];
  drafts:{task_id:string;body:string;status:string;expires_at:string}[];
};

const deliveries:Record<string,string> = {sent:"Тестовая доставка выполнена", pending:"В очереди", sending:"Тестовая доставка выполняется",
  failed:"Ошибка доставки. Откройте управление каналом для диагностики", cancelled:"Отменена",
  delivered:"Доставлено",read:"Прочитано",uncertain:"Доставка не подтверждена",delivery_unknown:"Доставка не подтверждена",
  needs_approved_template:"Требуется одобренный шаблон"};
const decisions:Record<string,string> = {pending:"На согласовании", approved:"Одобрен", rejected:"Отклонён", expired:"Срок истёк"};

export function InboxReview({org,conversation,owner,enabled,details}:{
  org:string;conversation?:string;owner:boolean;enabled:boolean;details?:InboxDetails;
}) {
  const router=useRouter();
  const [busy,setBusy]=useState(false);
  const submitting=useRef(false);
  const [error,setError]=useState("");
  useEffect(()=>{
    if (!conversation) return;
    const timer=window.setInterval(()=>{if (document.visibilityState==="visible") router.refresh();},8000);
    return ()=>window.clearInterval(timer);
  },[conversation,router]);

  async function submit(route:string, body:object) {
    if (submitting.current) return;
    submitting.current=true;
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
      submitting.current=false;
      setBusy(false);
    }
  }

  return <section className="panel inbox-review" aria-label="Состояние обращения" aria-busy={busy}>
    <h2>Согласование ответов AI</h2>
    {enabled?<p>Ответы управляющего в тестовом Inbox проходят согласование владельца.</p>:<p>Согласование ответов управляющего выключено.</p>}
    {owner&&!enabled&&<button type="button" disabled={busy} onClick={()=>void submit("inbox",{})}>Включить согласование в тестовом контуре</button>}
    {details&&<><p><strong>Карточка клиента</strong> · {channelLabels[details.customer.channel]??details.customer.channel} · {details.customer.id.slice(0,8)}</p>
      {details.requests.length>0&&<ul aria-label="Заявки клиента">{details.requests.map(item=><li key={item.id}>
        <Link href={`/dashboard/orders/${item.id}?org=${org}`}>Заявка №{item.id.slice(0,8)}</Link> · {statuses[item.status]??item.status}
      </li>)}</ul>}
      <details>
        <summary>Согласие на маркетинговые предложения</summary>
        <p>Тестовый контур. {details.consents[0]?.decision==="granted"?"Согласие зафиксировано для этого канала и диалога.":details.consents[0]?.decision==="revoked"?"Согласие отозвано.":"Подтверждённого согласия нет."}</p>
        {details.consents.length>0&&<ol>{details.consents.map(item=><li key={item.source_message_id}>
          {item.decision==="granted"?"Получено":"Отозвано"} · {channelLabels[item.channel]??item.channel} · <time dateTime={item.occurred_at}>{new Date(item.occurred_at).toLocaleString("ru-RU")}</time> · сообщение {item.source_message_id.slice(0,8)}
        </li>)}</ol>}
      </details>
      {details.assignee_id&&<p>Назначен сотрудник: {details.operators.find(user=>user.id===details.assignee_id)?.name??"Диалог назначен"}</p>}
      {owner&&conversation&&details.customer.channel!=="internal_chat"&&<form className="identity-form" onSubmit={event=>{
        event.preventDefault();
        const assignee=String(new FormData(event.currentTarget).get("assignee")??"");
        void submit(`conversations/${conversation}/inbox/assignee`,{assignee:assignee||null,expected_version:details.version});
      }}>
        <label>Ответственный сотрудник<select name="assignee" key={details.assignee_id??"none"} defaultValue={details.assignee_id??""}>
          <option value="">Без назначения</option>
          {details.operators.map(operator=><option key={operator.id} value={operator.id}>{operator.name}</option>)}
        </select></label>
        <button disabled={busy}>{busy?"Сохраняем…":"Сохранить ответственного"}</button>
      </form>}
      {details.processing_state&&<p>Состояние обработки · {details.processing_state}</p>}
      {conversation&&details.drafts.map(draft=><article className="inbox-draft" key={draft.task_id}>
        <p><strong>{decisions[draft.status]??draft.status}</strong> · {new Date(draft.expires_at).toLocaleString("ru-RU")}</p>
        <p>{draft.body}</p>
        {draft.status==="approved"&&details.customer.channel==="instagram"&&<p>Ответ сохранён в Inbox. Отправка Instagram выключена.</p>}
        {owner&&draft.status==="pending"&&<>
          <button type="button" disabled={busy} onClick={()=>void submit(`conversations/${conversation}/inbox/${draft.task_id}`,{decision:"approved"})}>{details.customer.channel==="instagram"?"Одобрить и сохранить в Inbox":"Одобрить тестовый ответ"}</button>
          <button type="button" disabled={busy} onClick={()=>void submit(`conversations/${conversation}/inbox/${draft.task_id}`,{decision:"rejected"})}>Отклонить</button>
        </>}
      </article>)}
      {!details.drafts.length&&<p>Черновиков для этого обращения пока нет.</p>}
      {!!details.delivery.length&&<ul aria-label="Статусы доставки">{details.delivery.map(item=><li key={item.id}>{deliveries[item.status.toLowerCase()]??item.status} · попыток {item.attempts} · <time dateTime={item.created_at}>{new Date(item.created_at).toLocaleString("ru-RU")}</time></li>)}</ul>}
    </>}
    {error&&<p role="alert">{error}</p>}
    <Link href={`/dashboard/integrations?org=${org}`}>Управление каналами</Link>
  </section>;
}
