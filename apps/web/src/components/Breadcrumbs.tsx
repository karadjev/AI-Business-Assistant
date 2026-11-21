"use client";
import Link from "next/link";
import {usePathname, useSearchParams} from "next/navigation";
const names: Record<string,string> = {orders:"Заказы",products:"Товары",catalog:"Качество каталога",inventory:"Остатки",categories:"Категории",conversations:"Диалоги","ai-settings":"AI-ассистент",integrations:"Каналы связи",settings:"Настройки",onboarding:"Начало работы",organizations:"Мои магазины",demo:"Демонстрация","ai-playground":"Тестовый чат",operations:"Обработка сообщений"};
export function Breadcrumbs() {
  const pathname = usePathname();
  const org = useSearchParams().get("org");
  const [, , part, detail] = pathname.split("/");
  const suffix = org ? `?org=${encodeURIComponent(org)}` : "";
  return <nav className="breadcrumbs" aria-label="Хлебные крошки"><Link prefetch={false} href={`/dashboard${suffix}`}>Рабочее пространство</Link><span aria-hidden="true">/</span><span aria-current={!detail ? "page" : undefined}>{names[part] ?? "Главная"}</span>{detail && <><span aria-hidden="true">/</span><span aria-current="page">{detail === "new" ? "Создание" : detail === "telegram" ? "Telegram" : detail === "whatsapp" ? "WhatsApp" : "Подробности"}</span></>}</nav>;
}
