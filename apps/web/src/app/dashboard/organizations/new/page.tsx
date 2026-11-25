import {SandboxForm} from "@/components/Workflows";
import {requireUser} from "@/lib/session";

export default async function NewOrganization() {
  await requireUser();
  return <main id="main" tabIndex={-1} className="onboarding-experience">
    <p className="intelligence-kicker">Начало работы · шаг 1 из 2</p>
    <h1>Создать организацию</h1>
    <SandboxForm businessOnboarding/>
  </main>;
}
