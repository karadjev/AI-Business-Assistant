import {AuthExperience} from "@/components/AuthExperience";
import {IdentityForm} from "@/components/IdentityForm";
import {EmailActionsUnavailable} from "@/components/EmailActionsUnavailable";
import {connection} from "next/server";

export default async function Page() {
  await connection();
  const disabled = process.env.ABA_DISABLE_EMAIL_ACTIONS === "true";
  return <AuthExperience title="Восстановить пароль" description={disabled ? undefined : "Укажите email. Если адрес подтверждён, мы отправим ссылку для смены пароля."}>
    {disabled ? <EmailActionsUnavailable/> : <IdentityForm kind="forgot"/>}
  </AuthExperience>;
}
