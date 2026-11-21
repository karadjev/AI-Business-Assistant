"use client";
import {Alert} from "@/components/UI";
import {useEffect,useRef} from "react";
import Link from "next/link";
export default function DashboardError({reset}:{error:Error&{digest?:string};reset:()=>void}){const title=useRef<HTMLHeadingElement>(null);useEffect(()=>title.current?.focus(),[]);return <main id="main" tabIndex={-1} className="dashboard-error"><h1 tabIndex={-1} ref={title}>Не удалось загрузить раздел</h1><Alert tone="danger">Данные сейчас недоступны. Это не означает, что заявок, товаров или ошибок нет. Проверьте локальное подключение и повторите загрузку.</Alert><div className="page-actions"><button className="primary" onClick={reset}>Попробовать снова</button><Link prefetch={false} className="button secondary" href="/dashboard">На главную</Link></div></main>}
