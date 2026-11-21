"use client";
import {useEffect,useState} from "react";
import type {StatusResult} from "@/lib/api";

type State=StatusResult | {kind:"loading"};
export function StatusIndicator(){
 const [state,setState]=useState<State>({kind:"loading"});
 const [attempt,setAttempt]=useState(0);
 useEffect(()=>{
  const controller=new AbortController();let active=true;
  const timer=setTimeout(()=>controller.abort(),5000);
  async function load(){
   try {
    const response=await fetch("/api/backend-status",{cache:"no-store",signal:controller.signal});
    const value:unknown=await response.json();
    if(!active)return;
    if(typeof value!=="object" || value===null || !("kind" in value)){setState({kind:"error"});return;}
    if(response.ok && value.kind==="available" && "data" in value && typeof value.data==="object" && value.data!==null && "service" in value.data && value.data.service==="ai-business-assistant" && "status" in value.data && value.data.status==="running" && "api_version" in value.data && value.data.api_version==="v1") {setState({kind:"available",data:{service:"ai-business-assistant",status:"running",api_version:"v1"}});}
    else {setState({kind:response.status===503 && value.kind==="unavailable"?"unavailable":"error"});}
   }catch {if(active)setState({kind:"unavailable"});}finally{clearTimeout(timer);}
  }
  void load();return ()=>{active=false;clearTimeout(timer);controller.abort();};
 },[attempt]);
 const label=state.kind==="loading"?"Проверяем соединение…":state.kind==="available"?"Backend доступен":state.kind==="unavailable"?"Backend недоступен":"Ошибка ответа backend";
 return <section className="status-panel" aria-live="polite" aria-busy={state.kind==="loading"}><div className={`status-dot ${state.kind}`} aria-hidden="true"/><div><h2>{label}</h2><p>{state.kind==="available"?`${state.data.service} · API ${state.data.api_version}. Проверка процесса, не готовности базы данных.`:state.kind==="loading"?"Запрашиваем фактическое состояние Go API.":"Не удалось подтвердить состояние сервиса. Проверьте запуск API и повторите запрос."}</p></div><button disabled={state.kind==="loading"} onClick={()=>{setState({kind:"loading"});setAttempt(n=>n+1)}}>Проверить снова</button></section>
}
