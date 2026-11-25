import {AuthExperience} from "@/components/AuthExperience";
import {IdentityForm} from "@/components/IdentityForm";

export default function Page() {
  return <AuthExperience title="Войти в кабинет" description="Используйте email, который подтвердили при регистрации.">
    <IdentityForm kind="login"/>
  </AuthExperience>;
}
