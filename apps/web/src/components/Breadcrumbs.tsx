"use client";
import Link from "next/link";
import {usePathname, useSearchParams} from "next/navigation";
const names: Record<string,string> = {command:"Business Manager",team:"Сотрудники и подписка",workshop:"Заявки автосервиса",orders:"Заявки",products:"Товары",catalog:"Качество каталога",inventory:"Остатки",categories:"Категории",conversations:"Inbox","ai-settings":"AI-ассистент",integrations:"Каналы связи",settings:"Настройки",onboarding:"Начало работы",organizations:"Организации",demo:"Демонстрация","ai-playground":"Тестовый чат",operations:"Обработка сообщений"};
export function Breadcrumbs() {
  const pathname = usePathname();
  const org = useSearchParams().get("org");
  const [, , part, detail] = pathname.split("/");
  const suffix = org ? `?org=${encodeURIComponent(org)}` : "";
  return <nav className="breadcrumbs" aria-label="Хлебные крошки"><Link prefetch={false} href={`/dashboard${suffix}`}>Рабочее пространство</Link><span aria-hidden="true">/</span><span aria-current={!detail ? "page" : undefined}>{names[part] ?? "Обзор"}</span>{detail && <><span aria-hidden="true">/</span><span aria-current="page">{detail === "new" ? "Создание" : detail === "telegram" ? "Telegram" : detail === "whatsapp" ? "WhatsApp" : detail === "instagram" ? "Instagram" : "Подробности"}</span></>}</nav>;
}
