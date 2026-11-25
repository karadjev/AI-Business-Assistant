"use client";

import {useCallback, useEffect, useRef, useState, type FormEvent} from "react";
import {csrf} from "./IdentityForm";

export type ManagerTask = {id:string;request_id:string;agent_id:string;action:string;status:string;error_code:string|null};
type Turn = {id:string;body:string;reply:string;task_id:string|null;request_id:string|null;status:string|null;error_code:string|null};
type Request = {id:string;service:string;close_kind:string|null;status:string};

const taskNames: Record<string,string> = {
  delegate_workshop:"Подготовка заявки", "delegation.workshop_customer":"Проверка клиента",
  "delegation.workshop_stock":"Проверка остатка", "delegation.workshop_sales":"Предложение Sales",
  workshop_reserve:"Резерв", workshop_release:"Освобождение резерва", workshop_consume:"Списание",
  delegate_workshop_finance:"Финансовый отчёт", "delegation.workshop_finance":"Расчёт Finance",
  "delegation.workshop_analytics":"Расчёт Analytics", delegate_workshop_marketing:"Маркетинговое предложение",
  "delegation.workshop_marketing":"Подготовка черновика",
};
const statuses: Record<string,string> = {queued:"В очереди",pending:"Ожидает предыдущий этап",running:"Выполняется",
  waiting_approval:"Нужно согласование",completed:"Завершено",failed:"Ошибка",cancelled:"Отменено",expired:"Срок истёк"};
export const agentNames: Record<string,string> = {business_manager:"Business Manager",customer:"Поддержка клиентов",product_inventory:"Каталог и остатки",
  sales:"Продажи",finance:"Финансы",analytics:"Аналитика",marketing:"Маркетинг"};

export function ManagerTaskList({tasks}: {tasks:ManagerTask[]}) {
  return <section className="panel" id="workshop-team">
    <h2>Задачи команды</h2>
    {tasks.length ? <ol>{tasks.map(task=><li key={task.id}>
      <a href={`#workshop-request-${task.request_id}`}>{taskNames[task.action] ?? task.action}</a>
      {" · "}{agentNames[task.agent_id] ?? task.agent_id}{" · "}{statuses[task.status] ?? task.status}
      {task.error_code && ` · ${task.error_code}`}
    </li>)}</ol> : <p>Задач пока нет.</p>}
  </section>;
}

export function WorkshopManager({base,tasks,requests,onQueued}: {base:string;tasks:ManagerTask[];requests:Request[];onQueued:()=>Promise<void>}) {
  const [turns,setTurns] = useState<Turn[]>([]);
  const [busy,setBusy] = useState(false);
  const [loaded,setLoaded] = useState(false);
  const [error,setError] = useState("");
  const submitting = useRef(false);
  const composer = useRef<HTMLTextAreaElement>(null);
  const retry = useRef<{fingerprint:string;id:string}|null>(null);
  const readHistory = useCallback(async (signal=AbortSignal.timeout(8000)) => {
    const response=await fetch(base,{cache:"no-store",signal});
    if (!response.ok) throw Error("Не удалось прочитать историю поручений.");
    return response.json() as Promise<Turn[]>;
  },[base]);

  function showHistory(history:Turn[]) {
    setTurns(history);
    setLoaded(true);
  }

  useEffect(()=>{
    const abort=new AbortController();
    void readHistory(AbortSignal.any([abort.signal,AbortSignal.timeout(8000)])).then(showHistory).catch(()=>{
      if (!abort.signal.aborted) setError("Не удалось прочитать историю поручений.");
    });
    return ()=>abort.abort();
  },[readHistory]);

  const load=()=>readHistory().then(showHistory);

  async function submit(event:FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (submitting.current) return;
    const form=event.currentTarget;
    const values=new FormData(form);
    const body={text:String(values.get("message")??"").trim(),request_id:String(values.get("request")??"")};
    if (!body.text) return;
    const fingerprint=JSON.stringify(body);
    if (retry.current?.fingerprint!==fingerprint) retry.current={fingerprint,id:crypto.randomUUID()};
    submitting.current = true;
    setBusy(true);
    setError("");
    try {
      const response=await fetch(base,{method:"POST",headers:{"Content-Type":"application/json","X-CSRF-Token":csrf()},
        body:JSON.stringify({...body,id:retry.current.id}),signal:AbortSignal.timeout(8000)});
      if (!response.ok) {
        const payload=await response.json();
        throw Error(payload.error?.message??"Не удалось отправить поручение.");
      }
      retry.current=null;
      form.reset();
      await Promise.all([load(),onQueued()]);
    } catch(reason) {
      setError(reason instanceof Error?reason.message:"Не удалось отправить поручение.");
    } finally {submitting.current = false;setBusy(false);}
  }

  return <section className="panel workshop-chat" id="workshop-manager" aria-labelledby="workshop-manager-title">
    <h2 id="workshop-manager-title">AI-управляющий</h2>
    <p className="field-hint">Рабочие поручения по данным вашей организации.</p>
    <ol className="manager-history" aria-live="polite" aria-label="История поручений">
      {turns.map(turn => {
        const current = tasks.find(task => task.id === turn.task_id);
        const status = current?.status ?? turn.status;
        return <li key={turn.id}>
          <div className="manager-message manager-user"><strong>Вы</strong><p>{turn.body}</p></div>
          <div className="manager-message manager-reply"><strong>Business Manager</strong><p>{turn.reply}</p>
            {status && <p className="field-hint">{statuses[status] ?? status}
              {turn.request_id && <> · <a href={`#workshop-request-${turn.request_id}`}>Результат и действия</a></>}
            </p>}
          </div>
        </li>;
      })}
    </ol>
    {!turns.length && <p className="field-hint">{loaded ? "Выберите пример или напишите поручение." : "Загружаем историю поручений…"}</p>}
    <div className="manager-examples" aria-label="Примеры поручений">
      {["Сводка по организации","Что ждёт согласования?","Покажи финансовые показатели"].map(example => <button key={example} type="button" disabled={busy} onClick={() => {
        if (composer.current) {composer.current.value = example;composer.current.focus();}
      }}>{example}</button>)}
    </div>
    <form className="identity-form" onSubmit={submit} aria-busy={busy}>
      <label>Поручение управляющему<textarea ref={composer} name="message" maxLength={2000} rows={3} required
        placeholder="Например, оформи заявку на замену фильтра"/></label>
      {requests.some(request => request.close_kind === "consume" || request.status === "failed") && <label>Привязать заявку
        <select name="request" aria-label="Привязать заявку" defaultValue="">
          <option value="">Без привязки</option>
          {requests.filter(item => item.close_kind === "consume" || item.status === "failed").map(item => <option key={item.id} value={item.id}>{item.service}</option>)}
        </select>
      </label>}
      <button className="primary" disabled={busy}>{busy ? "Отправляем поручение…" : "Отправить поручение"}</button>
    </form>
    {error && <p role="alert">{error} <button type="button" onClick={() => void load().then(() => setError("")).catch(() => setError("История пока недоступна."))}>Обновить историю</button></p>}
  </section>;
}
