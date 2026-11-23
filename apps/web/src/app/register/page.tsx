import {IdentityForm} from "@/components/IdentityForm";
import {EmailActionsUnavailable} from "@/components/EmailActionsUnavailable";
import {connection} from "next/server";
import Link from "next/link";
export default async function Page(){
 await connection();
 if(process.env.ABA_DISABLE_EMAIL_ACTIONS==="true") {
  return <main id="main" tabIndex={-1} className="auth-page"><h1>Создать аккаунт</h1><EmailActionsUnavailable/></main>;
 }
 const localDemo=process.env.APP_ENV==="development"&&process.env.ABA_LOCAL_DEMO_EMAIL_NOTICE==="1";
 return <main id="main" tabIndex={-1} className="auth-page"><span className="eyebrow">AI Business Assistant</span><h1>Создать аккаунт</h1><p className="lead">Подтвердите email, чтобы создать свой магазин.</p>
 {localDemo&&<aside role="note" className="form-success"><strong>Локальная демонстрация: реальные письма не отправляются.</strong><p>Письма подтверждения остаются в локальном почтовом перехватчике Mailpit. Используйте синтетический email. Для просмотра demo можно <Link prefetch={false} href="/login">войти в подготовленный аккаунт</Link>; данные для входа находятся у локального оператора.</p></aside>}
 <IdentityForm kind="register"/></main>;
}
