import Link from "next/link";
import {organizations, request} from "@/lib/session";
import {BusinessOnboarding} from "@/components/BusinessOnboarding";
import {ServiceSetup} from "@/components/ServiceSetup";
import {StoreOnboarding} from "@/components/StoreOnboarding";

export default async function Page({searchParams}: {searchParams: Promise<{org?: string; flow?: string}>}) {
  const {org: id, flow} = await searchParams;
  const list = await organizations();
  const org = list.find(x => x.id === id) ?? (!id ? list.find(x => x.role === "owner") : undefined);
  if (!org || org.role !== "owner" || (flow === "business" && (!id || !org.is_test))) {
    return <main id="main" tabIndex={-1}><h1>Настройка организации недоступна</h1>
      <Link prefetch={false} href="/dashboard/organizations">Мои организации</Link></main>;
  }
  if (flow === "services") {
    const response = org.is_test ? await request(`organizations/${org.id}/agent-team`).catch(() => null) : null;
    const data = response?.ok ? await response.json().catch(() => null) : null;
    const model = data?.profile?.model;
    return <main id="main" tabIndex={-1}><h1>Настройка услуг · {org.name}</h1>
      {org.is_test && (model === "service" || model === "hybrid")
        ? <ServiceSetup key={org.id} org={org.id} timezone={org.timezone}/>
        : <p>{!org.is_test ? "Настройка расписания доступна только в тестовой организации." :
          !data ? "Не удалось загрузить профиль организации. Обновите страницу." : "Для настройки услуг выберите профиль с услугами."}
          {org.is_test && data && <> <Link href={`/dashboard/onboarding?org=${org.id}&flow=business`}>Выбрать отрасль</Link></>}</p>}
    </main>;
  }
  if (flow !== "business") {
    return <main id="main" tabIndex={-1}><h1>Первоначальная настройка · {org.name}</h1>
      {org.is_test && <nav className="section-tabs" aria-label="Настройка бизнеса">
        <Link href={`/dashboard/onboarding?org=${org.id}&flow=business`}>Сфера деятельности</Link>
        <Link href={`/dashboard/onboarding?org=${org.id}&flow=services`}>Услуги и расписание</Link>
        <Link href={`/dashboard/team?org=${org.id}`}>Сотрудники</Link>
      </nav>}
      <StoreOnboarding key={org.id} org={org.id}/></main>;
  }
  const response = await request(`organizations/${org.id}/agent-team`).catch(() => null);
  const data = response?.ok ? await response.json().catch(() => null) : null;
  const profile = data?.profile;
  return <main id="main" tabIndex={-1} className="onboarding-experience"><p className="intelligence-kicker">Начало работы · шаг 2 из 2</p><h1>Выберите отрасль</h1><p className="lead">{org.name}</p>
    {typeof profile?.model === "string" && typeof profile.industry === "string" && Number.isSafeInteger(profile.revision)
      ? <BusinessOnboarding key={org.id} org={org.id} profile={profile}/>
      : <><p role="alert">Не удалось загрузить профиль организации.</p>
        <Link href={`/dashboard/onboarding?org=${org.id}&flow=business`}>Повторить</Link></>}
  </main>;
}
