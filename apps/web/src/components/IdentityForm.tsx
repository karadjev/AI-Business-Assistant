"use client";
import {useEffect,useState,useRef} from "react";
import {useRouter} from "next/navigation";
import Link from "next/link";
import type {Organization} from "@/lib/session";
export function csrf(){return decodeURIComponent(document.cookie.split(";").map(c=>c.trim()).find(c=>c.startsWith("aba_csrf="))?.slice(9)??"");}
export function IdentityForm({kind,organization}:{kind:"register"|"login"|"verify"|"create"|"settings";organization?:Organization}){
 const router=useRouter();const [busy,setBusy]=useState(false);const [error,setError]=useState("");const [message,setMessage]=useState("");const tokenInput=useRef<HTMLInputElement>(null);
 useEffect(()=>{if(kind==="verify"){const value=new URLSearchParams(location.hash.slice(1)).get("token");if(value){if(tokenInput.current)tokenInput.current.value=value;history.replaceState(null,"",location.pathname);}}},[kind]);
 async function submit(event:React.FormEvent<HTMLFormElement>){event.preventDefault();setBusy(true);setError("");setMessage("");const data=new FormData(event.currentTarget);
 const endpoint=kind==="register"?"auth/register":kind==="login"?"auth/login":kind==="verify"?"auth/verify-email":kind==="settings"?`organizations/${organization!.id}`:"organizations";
 const body=kind==="register"?{email:data.get("email"),password:data.get("password"),display_name:data.get("display_name")}:kind==="login"?{email:data.get("email"),password:data.get("password")}:kind==="verify"?{token:data.get("token")}:{name:data.get("name"),timezone:data.get("timezone"),...(kind==="settings"?{expected_version:organization!.version}:{})};
 try{const r=await fetch(`/api/v1/${endpoint}`,{method:kind==="settings"?"PATCH":"POST",headers:{"Content-Type":"application/json","X-CSRF-Token":csrf()},body:JSON.stringify(body)});const result=r.status===204?{}:await r.json();if(!r.ok){setError(result.error?.message??"Не удалось выполнить действие");return;}
 if(kind==="login"){router.push("/dashboard");router.refresh();}else if(kind==="create"){router.push(`/dashboard?org=${result.id}`);router.refresh();}else if(kind==="settings"){setMessage("Настройки сохранены");router.refresh();}else if(kind==="verify"){if(tokenInput.current)tokenInput.current.value="";setMessage("Email подтверждён. Теперь можно войти.");}else{setMessage(result.message);}
 }catch{setError("Не удалось связаться с сервером. Повторите попытку.");}finally{setBusy(false);}}
 async function resend(event:React.FormEvent<HTMLFormElement>){event.preventDefault();setBusy(true);setError("");try{const r=await fetch("/api/v1/auth/resend-verification",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({email:new FormData(event.currentTarget).get("email")})});const data=await r.json();if(r.ok)setMessage(data.message);else setError(data.error?.message??"Ошибка запроса");}catch{setError("Сервер недоступен");}finally{setBusy(false);}}
 return <><form className="identity-form" onSubmit={submit}>
 {(kind==="register"||kind==="login")&&<><label>Email<input name="email" type="email" autoComplete="email" maxLength={254} required/></label><label>Пароль<input name="password" type="password" autoComplete={kind==="register"?"new-password":"current-password"} minLength={12} maxLength={128} required/></label></>}
 {kind==="register"&&<><label>Ваше имя<input name="display_name" autoComplete="name" maxLength={100} required/></label><p className="small">Пароль от 12 до 128 символов. Для локальной проверки используйте синтетический email.</p></>}
 {kind==="verify"&&<label>Код подтверждения<input aria-label="Код подтверждения" type="password" autoComplete="off" name="token" ref={tokenInput} required/></label>}
 {(kind==="create"||kind==="settings")&&<><label>Название магазина<input name="name" defaultValue={organization?.name} maxLength={120} required/></label><label>Часовой пояс<input name="timezone" defaultValue={organization?.timezone??"Europe/Moscow"} maxLength={100} required/></label></>}
 {error&&<p role="alert" className="form-error">{error}</p>}{message&&<p role="status" className="form-success">{message}</p>}
 <button className="primary" disabled={busy}>{busy?"Выполняем…":kind==="register"?"Зарегистрироваться":kind==="login"?"Войти":kind==="verify"?"Подтвердить email":kind==="create"?"Создать магазин":"Сохранить настройки"}</button>
 </form>
 {kind==="verify"&&<form onSubmit={resend} className="identity-form"><h2>Нужна новая ссылка?</h2><label>Email<input name="email" type="email" required/></label><button disabled={busy}>Отправить повторно</button></form>}
 {(kind==="register"||kind==="verify")&&<p className="auth-link"><Link prefetch={false} href="/login">Перейти ко входу →</Link></p>}{kind==="login"&&<p className="auth-link"><Link prefetch={false} href="/register">Создать аккаунт</Link> · <Link prefetch={false} href="/verify-email">Подтвердить email</Link></p>}
 </>;
}
export function Logout(){const router=useRouter();const [error,setError]=useState("");const [busy,setBusy]=useState(false);return <div><button disabled={busy} onClick={async()=>{setBusy(true);try{const r=await fetch("/api/v1/auth/logout",{method:"POST",headers:{"Content-Type":"application/json","X-CSRF-Token":csrf()},body:"{}"});if(!r.ok){setError("Не удалось выйти");return;}router.replace("/login");router.refresh();}catch{setError("Сервер недоступен");}finally{setBusy(false);}}}>Выйти</button>{error&&<p role="alert">{error}</p>}</div>}
