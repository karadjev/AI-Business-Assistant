import type {Metadata} from "next";
import {SkipLink} from "@/components/SkipLink";
import {Header} from "@/components/Header";
import "./globals.css";
import "./premium.css";
import "./intelligence.css";
export const metadata:Metadata={title:{default:"AI Business Assistant",template:"%s · AI Business Assistant"},description:"Семь AI-агентов для торговли и услуг. Обращения, каталог, финансы и согласования в одном кабинете."};
export default function RootLayout({children}:{children:React.ReactNode}){return <html lang="ru" suppressHydrationWarning><head><script dangerouslySetInnerHTML={{__html:'try{var t=localStorage.getItem("aba-theme");if(t==="light"||t==="dark")document.documentElement.dataset.theme=t}catch{}'}}/></head><body><SkipLink/><Header/>{children}<footer><span>AI Business Assistant</span><span>Обращения, каталог и поручения.</span></footer></body></html>}
