import {AuthExperience} from "@/components/AuthExperience";
import {IdentityForm} from "@/components/IdentityForm";
import {EmailActionsUnavailable} from "@/components/EmailActionsUnavailable";
import {connection} from "next/server";

export default async function Page() {
  await connection();
  const disabled = process.env.ABA_DISABLE_EMAIL_ACTIONS === "true";
  return <AuthExperience title="Подтвердить email" description={disabled ? undefined : "Откройте ссылку из письма и нажмите кнопку подтверждения."}>
    {disabled ? <EmailActionsUnavailable/> : <IdentityForm kind="verify"/>}
  </AuthExperience>;
}
