"use client";

import Link from "next/link";
import {useState, type FormEvent} from "react";
import {useRouter} from "next/navigation";
import {csrf} from "./IdentityForm";

type Team = {
  members:{id:string;email:string;display_name:string;role:string;operator:boolean}[];
  invites:{id:string;email:string;expires_at:string;operator:boolean}[];
  audit:{id:string;action:string;created_at:string}[];
};
type Usage = {
  staff:number;staff_limit:number|null;active_channels:number;tasks_30d:number;tasks_failed:number;tasks_active:number;
  incoming_failed:number;input_tokens:number;output_tokens:number;estimated_cost_microrub:number|null;reserved_cost_microrub:number;uncertain_cost_entries:number;
};
const rub=(amount:number)=>new Intl.NumberFormat("ru-RU",{style:"currency",currency:"RUB",maximumFractionDigits:6}).format(amount/1000000);
const actions:Record<string,string>={
  "invite.created":"Создано приглашение","invite.accepted":"Приглашение принято","invite.revoked":"Приглашение отозвано",
  "member.role":"Роль изменена","member.revoked":"Доступ отозван","inbox.enabled":"Согласование Inbox включено",
  "conversation.assigned":"Диалог передан сотруднику",
};

async function changeTeam(org:string,body:object) {
  const response=await fetch(`/api/v1/organizations/${org}/team`,{
    method:"POST",headers:{"Content-Type":"application/json","X-CSRF-Token":csrf()},
    body:JSON.stringify(body),signal:AbortSignal.timeout(8000),
  });
  const result=await response.json();
  if(!response.ok)throw new Error(result.error?.message??"Действие недоступно");
  return result as {token?:string};
}

export function OrganizationTeam({org,team,usage}:{org:string;team:Team;usage:Usage}) {
  const router=useRouter();
  const [code,setCode]=useState("");
  const [busy,setBusy]=useState(false);
  const [error,setError]=useState("");
  async function perform(body:object) {
    setBusy(true);
    setError("");
    try {
      const result=await changeTeam(org,body);
      if(result.token)setCode(org+"."+result.token);
      router.refresh();
    } catch(reason) {
      setError(reason instanceof Error?reason.message:"Не удалось сохранить изменения");
    } finally {setBusy(false);}
  }
  function invite(event:FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data=new FormData(event.currentTarget);
    void perform({action:"invite",email:data.get("email"),operator:data.get("role")==="operator"});
  }
  return <>
    {error&&<p role="alert">{error}</p>}
    <section className="panel team-panel"><h2>Сотрудники</h2>
      <ul className="team-members">{team.members.map(user=><li key={user.id}>
        <strong>{user.display_name}</strong> · {user.email} · {user.role==="owner"?"Владелец":user.operator?"Оператор":"Участник"}
        {user.role!=="owner"&&<>
          <button disabled={busy} onClick={()=>void perform({action:"member.role",target:user.id,operator:!user.operator})}>{user.operator?"Снять права оператора":"Назначить оператором"}</button>
          <button disabled={busy} onClick={()=>{if(confirm("Отозвать доступ сотрудника к организации?"))void perform({action:"member.revoked",target:user.id});}}>Отозвать доступ</button>
        </>}
      </li>)}</ul>
      <form className="identity-form" onSubmit={invite}>
        <label>Email сотрудника<input name="email" type="email" maxLength={254} required/></label>
        <label>Роль<select name="role"><option value="member">Участник</option><option value="operator">Оператор заявок и Inbox</option></select></label>
        <button className="primary" disabled={busy}>{busy?"Создаём…":"Создать приглашение"}</button>
      </form>
      {code&&<div role="status"><p>Передайте код сотруднику по защищённому каналу. Он действует 24 часа.</p>
        <label>Код приглашения<input value={code} readOnly onFocus={event=>event.target.select()}/></label>
        <button onClick={()=>setCode("")}>Скрыть код</button>
      </div>}
      {team.invites.length>0&&<ul aria-label="Ожидают принятия">{team.invites.map(invite=><li key={invite.id}>
        {invite.email} · {new Date(invite.expires_at).toLocaleString("ru-RU")}
        <button disabled={busy} onClick={()=>void perform({action:"invite.revoked",target:invite.id})}>Отозвать приглашение</button>
      </li>)}</ul>}
    </section>
    <section className="panel team-panel team-usage"><h2>Использование ресурсов</h2>
      <dl><dt>Сотрудники</dt><dd>{usage.staff}{usage.staff_limit===null?" · лимит не назначен":" из "+usage.staff_limit}</dd>
        <dt>Активные настройки каналов</dt><dd>{usage.active_channels}</dd>
        <dt>Задачи агентов за 30 дней</dt><dd>{usage.tasks_30d}, ошибок и таймаутов {usage.tasks_failed}</dd>
        <dt>Активные задачи</dt><dd>{usage.tasks_active}</dd>
        <dt>Ошибки входящей обработки</dt><dd>{usage.incoming_failed}</dd>
        <dt>Учтённые токены моделей</dt><dd>{usage.input_tokens} входных и {usage.output_tokens} выходных</dd>
        <dt>Оценка учтённых расходов</dt><dd>{usage.estimated_cost_microrub===null?"Нет данных":rub(usage.estimated_cost_microrub)}</dd>
        <dt>Зарезервировано на модели</dt><dd>{rub(usage.reserved_cost_microrub)}</dd>
        <dt>Записи с неопределённой стоимостью</dt><dd>{usage.uncertain_cost_entries}</dd>
      </dl>
    </section>
    <section className="panel team-panel"><h2>Последние действия</h2>{team.audit.length
      ?<ul>{team.audit.map(event=><li key={event.id}>{actions[event.action]??event.action} · {new Date(event.created_at).toLocaleString("ru-RU")}</li>)}</ul>
      :<p>Изменений пока нет.</p>}</section>
  </>;
}

export function JoinTeam() {
  const [busy,setBusy]=useState(false);
  const [error,setError]=useState("");
  const [org,setOrg]=useState("");
  async function accept(event:FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const input=String(new FormData(event.currentTarget).get("code")??"").trim();
    const match=/^([0-9a-f-]{36})\.([A-Za-z0-9_-]{43})$/.exec(input);
    if(!match){setError("Проверьте код приглашения");return;}
    setBusy(true);
    setError("");
    try {await changeTeam(match[1],{action:"accept",token:match[2]});setOrg(match[1]);}
    catch(reason){setError(reason instanceof Error?reason.message:"Не удалось принять приглашение");}
    finally{setBusy(false);}
  }
  return org?<p>Приглашение принято. <Link href={`/dashboard?org=${org}`}>Открыть организацию</Link></p>:<form className="identity-form" onSubmit={accept}>
    <label>Код приглашения<input name="code" required maxLength={80} autoComplete="off" spellCheck={false}/></label>
    <button disabled={busy}>Принять приглашение</button>
    {error&&<p role="alert">{error}</p>}
  </form>;
}
