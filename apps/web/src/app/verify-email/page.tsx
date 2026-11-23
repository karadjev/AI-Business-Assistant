import {IdentityForm} from "@/components/IdentityForm";
import {EmailActionsUnavailable} from "@/components/EmailActionsUnavailable";
import {connection} from "next/server";

export default async function Page() {
  await connection();
  return <main id="main" tabIndex={-1} className="auth-page"><span className="eyebrow">AI Business Assistant</span><h1>Подтвердить email</h1>{process.env.ABA_DISABLE_EMAIL_ACTIONS === "true" ? <EmailActionsUnavailable/> : <><p className="lead">Откройте ссылку из письма в локальной почте или вставьте код подтверждения.</p><IdentityForm kind="verify"/></>}</main>;
}
