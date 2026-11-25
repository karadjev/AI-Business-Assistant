import {SandboxForm} from "@/components/Workflows";
import {requireUser} from "@/lib/session";

export default async function NewOrganization() {
  await requireUser();
  return <main id="main" tabIndex={-1}>
    <h1>Создать организацию</h1>
    <SandboxForm businessOnboarding/>
  </main>;
}
