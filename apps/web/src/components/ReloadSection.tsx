"use client";
import {useTransition} from "react";
import {useRouter} from "next/navigation";
export function ReloadSection({label="Повторить загрузку"}:{label?:string}){const router=useRouter();const [pending,start]=useTransition();return <div className="reload-section"><button className="primary" disabled={pending} onClick={()=>start(()=>router.refresh())}>{pending?"Загружаем…":label}</button></div>}
