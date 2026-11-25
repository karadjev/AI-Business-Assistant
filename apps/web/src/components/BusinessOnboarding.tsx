"use client";

import Link from "next/link";
import {useRouter} from "next/navigation";
import {useRef, useState, type FormEvent} from "react";
import {csrf} from "./IdentityForm";

const profiles = [
  {id: "vehicle_service_hybrid", model: "hybrid", industry: "vehicle_service", label: "Автосервис с продажей запчастей"},
  {id: "vehicle_service", model: "service", industry: "vehicle_service", label: "Автосервис без продажи запчастей"},
  {id: "electronics", model: "trade", industry: "electronics", label: "Электроника"},
  {id: "auto_parts", model: "trade", industry: "auto_parts", label: "Автозапчасти"},
  {id: "building_materials", model: "trade", industry: "building_materials", label: "Строительные материалы"},
  {id: "furniture", model: "trade", industry: "furniture", label: "Мебель"},
  {id: "apparel", model: "trade", industry: "apparel", label: "Одежда"},
  {id: "personal_services", model: "service", industry: "personal_services", label: "Персональные услуги"},
  {id: "real_estate", model: "agency", industry: "real_estate", label: "Недвижимость"},
];

type Profile = {model: string; industry: string; revision: number};

export function BusinessOnboarding({org, profile}: {org: string; profile: Profile}) {
  const router = useRouter();
  const submitting = useRef(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const current = profiles.find(item => item.model === profile.model && item.industry === profile.industry);
  const target = `/dashboard/command?org=${org}`;

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const selected = profiles.find(item => item.id === new FormData(event.currentTarget).get("profile"));
    if (!selected || submitting.current) return;
    submitting.current = true;
    setBusy(true);
    setError("");
    try {
      if (current?.id !== selected.id) {
        const response = await fetch(`/api/v1/organizations/${org}/agent-business-profile`, {
          method: "PATCH",
          headers: {"Content-Type": "application/json", "X-CSRF-Token": csrf()},
          body: JSON.stringify({model: selected.model, industry: selected.industry, revision: profile.revision}),
          signal: AbortSignal.timeout(10000),
        });
        if (response.status === 401) {router.replace("/login"); return;}
        if (!response.ok) {
          const payload = await response.json().catch(() => null);
          throw new Error(payload?.error?.message ?? "Не удалось сохранить профиль. Обновите страницу и проверьте выбранную отрасль.");
        }
      }
      router.push(target);
      router.refresh();
    } catch (failure) {
      setError(failure instanceof Error && failure.name === "Error"
        ? failure.message : "Не удалось получить ответ. Обновите страницу и проверьте выбранную отрасль.");
      submitting.current = false;
      setBusy(false);
    }
  }

  return <section className="module-card onboarding-card">
    <p>Выберите профиль, чтобы AI-команда показала доступные для вашей отрасли поручения.</p>
    <form className="identity-form" onSubmit={save}>
      <label>Отрасль и профиль<select name="profile" defaultValue={current?.id ?? profiles[0].id} disabled={busy}>
        {profiles.map(item => <option key={item.id} value={item.id}>{item.label}</option>)}
      </select></label>
      <button className="primary" disabled={busy}>{busy ? "Сохраняем…" : "Сохранить профиль и открыть AI-команду"}</button>
      {error && <p className="form-error" role="alert">{error}</p>}
    </form>
    <nav aria-label="Дальнейшие действия"><Link prefetch={false} href="/dashboard/organizations">К списку организаций</Link><Link prefetch={false} href={`/dashboard?org=${org}`}>Настроить позже</Link></nav>
  </section>;
}
