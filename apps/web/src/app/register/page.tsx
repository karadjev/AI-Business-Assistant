import {AuthExperience} from "@/components/AuthExperience";
import {IdentityForm} from "@/components/IdentityForm";
import {EmailActionsUnavailable} from "@/components/EmailActionsUnavailable";
import {connection} from "next/server";

export default async function Page() {
  await connection();
  const disabled = process.env.ABA_DISABLE_EMAIL_ACTIONS === "true";
  const localDemo = process.env.APP_ENV === "development" && process.env.ABA_LOCAL_DEMO_EMAIL_NOTICE === "1";
  return <AuthExperience title="Создать аккаунт" description={disabled ? undefined : "Укажите email и пароль. Организацию можно настроить после входа."}>
    {disabled ? <EmailActionsUnavailable/> : <>
      {localDemo && <p role="note" className="auth-local-note">Локальное демо. Используйте синтетический адрес. Письма остаются в тестовом почтовом ящике.</p>}
      <IdentityForm kind="register"/>
    </>}
  </AuthExperience>;
}
