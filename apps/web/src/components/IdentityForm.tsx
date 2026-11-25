"use client";

import {useEffect, useRef, useState, type FormEvent} from "react";
import {useRouter} from "next/navigation";
import Link from "next/link";
import type {Organization} from "@/lib/session";

export function csrf() {
  return decodeURIComponent(document.cookie.split(";").map(c => c.trim()).find(c => c.startsWith("aba_csrf="))?.slice(9) ?? "");
}

type Kind = "register"|"login"|"verify"|"forgot"|"reset"|"create"|"settings";

const actions = {
  register:"Создать аккаунт", login:"Войти", verify:"Подтвердить email", forgot:"Отправить ссылку",
  reset:"Сохранить пароль", create:"Создать организацию", settings:"Сохранить настройки",
};

type Feedback = {message?:string; id?:string; error?:{code?:string; message?:string}};

export function IdentityForm({kind,organization}: {kind:Kind; organization?:Organization}) {
  const router = useRouter();
  const [busy,setBusy] = useState(false);
  const [error,setError] = useState("");
  const [message,setMessage] = useState("");
  const [email,setEmail] = useState("");
  const [sentTo,setSentTo] = useState("");
  const [finished,setFinished] = useState(false);
  const [showPassword,setShowPassword] = useState(false);
  const submitting = useRef(false);
  const tokenInput = useRef<HTMLInputElement>(null);
  const outcome = useRef<HTMLHeadingElement>(null);
  const emailInput = useRef<HTMLInputElement>(null);
  const focusEmail = useRef(false);
  const newPassword = kind === "register" || kind === "reset";
  const tokenFlow = kind === "verify" || kind === "reset";

  useEffect(() => {
    if (!tokenFlow) return;
    function readToken() {
      const value = new URLSearchParams(location.hash.slice(1)).get("token");
      if (!value) return;
      if (!tokenInput.current) {
        location.reload();
        return;
      }
      tokenInput.current.value = value;
      history.replaceState(null,"",location.pathname + location.search);
    }
    readToken();
    window.addEventListener("hashchange", readToken);
    return () => window.removeEventListener("hashchange", readToken);
  }, [tokenFlow]);

  useEffect(() => {if (sentTo || finished) outcome.current?.focus();}, [sentTo,finished]);
  useEffect(() => {if (!sentTo && focusEmail.current) {focusEmail.current = false; emailInput.current?.focus();}}, [sentTo]);

  async function send(endpoint:string, data:unknown, onSuccess:(payload:Feedback)=>void) {
    if (submitting.current) return;
    submitting.current = true;
    setBusy(true);
    setError("");
    setMessage("");
    try {
      const response = await fetch(`/api/v1/${endpoint}`, {
        method:kind === "settings" ? "PATCH" : "POST",
        headers:{"Content-Type":"application/json","X-CSRF-Token":csrf()},
        body:JSON.stringify(data),
        signal:AbortSignal.timeout(12000),
      });
      const result:Feedback = response.status === 204 ? {} : await response.json();
      if (!response.ok) {
        setError(result.error?.code === "INVALID_TOKEN" || (tokenFlow && response.status === 422)
          ? kind === "verify"
            ? "Ссылка истекла или уже использована. Если вы уже подтвердили адрес, войдите. Иначе запросите новое письмо."
            : "Проверьте пароль и срок действия ссылки. Если ссылка истекла или уже использована, запросите новую."
          : result.error?.message ?? "Не удалось выполнить действие. Повторите попытку.");
        return;
      }
      onSuccess(result);
    } catch {
      setError("Не удалось получить ответ сервера. Повторите попытку.");
    } finally {
      submitting.current = false;
      setBusy(false);
    }
  }

  async function submit(event:FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);
    const address = String(data.get("email") ?? "");
    const password = String(data.get("password") ?? "");
    if (newPassword && ([...password].length < 12 || [...password].length > 128)) {
      setError("Введите пароль длиной от 12 до 128 символов.");
      form.querySelector<HTMLInputElement>('[name="password"]')?.focus();
      return;
    }
    switch (kind) {
      case "register":
        await send("auth/register",{email:address,password,display_name:"Пользователь"},() => {setEmail(address);setSentTo(address);}); break;
      case "login":
        await send("auth/login",{email:address,password},() => {router.replace("/dashboard");router.refresh();}); break;
      case "verify":
        await send("auth/verify-email",{token:data.get("token")},() => setFinished(true)); break;
      case "forgot":
        await send("auth/password-reset",{email:address},result => setMessage(result.message ?? "Проверьте почту.")); break;
      case "reset":
        await send("auth/password-reset/confirm",{token:data.get("token"),password},() => setFinished(true)); break;
      case "create":
        await send("organizations",{name:data.get("name"),timezone:data.get("timezone")},result => {router.replace(`/dashboard?org=${result.id}`);router.refresh();}); break;
      case "settings":
        if (organization) await send(`organizations/${organization.id}`,{name:data.get("name"),timezone:data.get("timezone"),expected_version:organization.version},() => {setMessage("Настройки сохранены.");router.refresh();});
    }
  }

  const feedback = <>{error && <p role="alert" className="form-error">{error}</p>}{message && <p role="status" className="form-success">{message}</p>}</>;
  const resend = (address:string) => void send("auth/resend-verification",{email:address},result => setMessage(result.message ?? "Проверьте почту."));
  if (sentTo) return <section className="auth-outcome">
    <h2 tabIndex={-1} ref={outcome}>Проверьте почту</h2>
    <p>Если адрес доступен для регистрации, письмо придёт на <strong>{sentTo}</strong>. Откройте ссылку в письме, затем войдите в кабинет.</p>
    <p>Если письма нет, проверьте папку «Спам» и правильность адреса.</p>
    <div className="auth-outcome-actions">
      <button type="button" disabled={busy} onClick={() => resend(sentTo)}>{busy ? "Подождите…" : "Отправить повторно"}</button>
      <button type="button" disabled={busy} onClick={() => {focusEmail.current = true;setSentTo("");setMessage("");setError("");}}>Исправить email</button>
    </div>
    {feedback}
    <p className="auth-link"><Link prefetch={false} href="/login">Перейти ко входу</Link></p>
  </section>;

  if (finished) return <section className="auth-outcome">
    <h2 tabIndex={-1} ref={outcome}>{kind === "verify" ? "Email подтверждён" : "Пароль изменён"}</h2>
    <p>Теперь можно войти в кабинет.</p>
    <Link prefetch={false} className="button primary" href="/login">Войти</Link>
  </section>;

  return <>
    <form className="identity-form" onSubmit={submit} aria-busy={busy}>
      {(kind === "register" || kind === "login" || kind === "forgot") && <label>Email
        <input ref={emailInput} name="email" type="email" autoComplete={kind === "login" ? "username" : "email"} maxLength={254} required value={email} onChange={event => setEmail(event.target.value)} spellCheck={false}/>
      </label>}
      {tokenFlow && <label>{kind === "verify" ? "Код подтверждения" : "Код восстановления"}
        <input type="password" autoComplete="off" name="token" ref={tokenInput} required/>
      </label>}
      {(newPassword || kind === "login") && <div className="identity-password">
        <label htmlFor="identity-password">Пароль</label>
        <div className="password-control">
          <input id="identity-password" name="password" type={showPassword ? "text" : "password"} autoComplete={newPassword ? "new-password" : "current-password"} aria-describedby={newPassword ? "password-policy" : undefined} required/>
          <button type="button" aria-controls="identity-password" onClick={() => setShowPassword(value => !value)}>{showPassword ? "Скрыть" : "Показать"}</button>
        </div>
        {newPassword && <p id="password-policy" className="field-hint">От 12 до 128 символов. Можно вставить пароль из менеджера паролей.</p>}
      </div>}
      {(kind === "create" || kind === "settings") && <>
        <label>Название организации<input name="name" defaultValue={organization?.name} maxLength={120} required/></label>
        <label>Часовой пояс<input name="timezone" defaultValue={organization?.timezone ?? "Europe/Moscow"} maxLength={100} required/></label>
      </>}
      {kind !== "verify" && feedback}
      <button className="primary" disabled={busy}>{busy ? "Подождите…" : actions[kind]}</button>
    </form>
    {kind === "verify" && <>
      <details className="verification-help">
        <summary>Не пришло письмо или нужна новая ссылка?</summary>
        <p>Проверьте папку «Спам», затем запросите новую ссылку на адрес регистрации.</p>
        <form className="identity-form" aria-busy={busy} onSubmit={event => {event.preventDefault();resend(String(new FormData(event.currentTarget).get("email")));}}>
          <label>Email<input name="email" type="email" autoComplete="email" maxLength={254} required/></label>
          <button disabled={busy}>Отправить повторно</button>
        </form>
      </details>
      {feedback}
    </>}
    {kind === "reset" && <p className="auth-link"><Link prefetch={false} href="/forgot-password">Запросить новую ссылку</Link></p>}
    {["register","verify","forgot","reset"].includes(kind) && <p className="auth-link"><Link prefetch={false} href="/login">Перейти ко входу</Link></p>}
    {kind === "login" && <div className="auth-link auth-link-list">
      <Link prefetch={false} href="/register">Создать аккаунт</Link>
      <Link prefetch={false} href="/forgot-password">Забыли пароль?</Link>
      <Link prefetch={false} href="/verify-email">Подтвердить email</Link>
    </div>}
  </>;
}
export function Logout(){const router=useRouter();const [error,setError]=useState("");const [busy,setBusy]=useState(false);return <div><button disabled={busy} onClick={async()=>{setBusy(true);try{const r=await fetch("/api/v1/auth/logout",{method:"POST",headers:{"Content-Type":"application/json","X-CSRF-Token":csrf()},body:"{}"});if(!r.ok){setError("Не удалось выйти");return;}router.replace("/login");router.refresh();}catch{setError("Сервер недоступен");}finally{setBusy(false);}}}>Выйти</button>{error&&<p role="alert">{error}</p>}</div>}
