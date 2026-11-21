"use client";
import {useEffect,useSyncExternalStore} from "react";
const modes=["system","light","dark"] as const;
const names:Record<string,string>={system:"как в системе",light:"светлая",dark:"тёмная"};
function read(){try{const mode=localStorage.getItem("aba-theme");return modes.includes(mode as typeof modes[number])?mode!:"system"}catch{return "system"}}
function subscribe(fn:()=>void){window.addEventListener("storage",fn);window.addEventListener("aba-theme",fn);return()=>{window.removeEventListener("storage",fn);window.removeEventListener("aba-theme",fn)}}
export function ThemeSwitch(){const mode=useSyncExternalStore(subscribe,read,()=>"system");useEffect(()=>{const selected=read();if(selected==="system")delete document.documentElement.dataset.theme;else document.documentElement.dataset.theme=selected},[mode]);return <button className="theme-switch" title="Переключить оформление" aria-label={`Тема: ${names[mode]}. Переключить`} onClick={()=>{try{localStorage.setItem("aba-theme",modes[(modes.indexOf(mode as typeof modes[number])+1)%modes.length]);window.dispatchEvent(new Event("aba-theme"))}catch{/* Browser storage may be unavailable. */}}}><span aria-hidden="true">◐</span><span className="theme-label">Оформление</span></button>}
