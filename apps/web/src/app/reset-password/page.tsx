import {AuthExperience} from "@/components/AuthExperience";
import {IdentityForm} from "@/components/IdentityForm";
import {EmailActionsUnavailable} from "@/components/EmailActionsUnavailable";
import {connection} from "next/server";

export default async function Page() {
  await connection();
  const disabled = process.env.ABA_DISABLE_EMAIL_ACTIONS === "true";
  return <AuthExperience title="Новый пароль" description={disabled ? undefined : "Введите новый пароль. Ссылка из письма одноразовая и действует 30 минут."}>
    {disabled ? <EmailActionsUnavailable/> : <IdentityForm kind="reset"/>}
  </AuthExperience>;
}
