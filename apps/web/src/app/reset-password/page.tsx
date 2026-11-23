import {IdentityForm} from "@/components/IdentityForm";
import {EmailActionsUnavailable} from "@/components/EmailActionsUnavailable";
import {connection} from "next/server";

export default async function Page() {
  await connection();
  return <main id="main" tabIndex={-1} className="auth-page"><span className="eyebrow">AI Business Assistant</span><h1>Новый пароль</h1>{process.env.ABA_DISABLE_EMAIL_ACTIONS === "true" ? <EmailActionsUnavailable/> : <><p className="lead">Введите новый пароль и код из письма. Ссылка одноразовая и действует 30 минут.</p><IdentityForm kind="reset"/></>}</main>;
}
