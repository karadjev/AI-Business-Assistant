import Link from "next/link";
import {organizations, request} from "@/lib/session";
import {BusinessOnboarding} from "@/components/BusinessOnboarding";
import {StoreOnboarding} from "@/components/StoreOnboarding";

export default async function Page({searchParams}: {searchParams: Promise<{org?: string; flow?: string}>}) {
  const {org: id, flow} = await searchParams;
  const list = await organizations();
  const org = list.find(x => x.id === id) ?? (!id ? list.find(x => x.role === "owner") : undefined);
  if (!org || org.role !== "owner" || (flow === "business" && (!id || !org.is_test))) {
    return <main id="main" tabIndex={-1}><h1>Настройка организации недоступна</h1>
      <Link prefetch={false} href="/dashboard/organizations">Мои организации</Link></main>;
  }
  if (flow !== "business") {
    return <main id="main" tabIndex={-1}><h1>Первоначальная настройка · {org.name}</h1>
      <StoreOnboarding key={org.id} org={org.id}/></main>;
  }
  const response = await request(`organizations/${org.id}/agent-team`).catch(() => null);
  const data = response?.ok ? await response.json().catch(() => null) : null;
  const profile = data?.profile;
  return <main id="main" tabIndex={-1}><h1>Настройка бизнеса · {org.name}</h1>
    {typeof profile?.model === "string" && typeof profile.industry === "string" && Number.isSafeInteger(profile.revision)
      ? <BusinessOnboarding key={org.id} org={org.id} profile={profile}/>
      : <><p role="alert">Не удалось загрузить профиль организации.</p>
        <Link href={`/dashboard/onboarding?org=${org.id}&flow=business`}>Повторить</Link></>}
  </main>;
}
