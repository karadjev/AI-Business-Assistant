import type {Metadata} from "next";
import {SkipLink} from "@/components/SkipLink";
import {Header} from "@/components/Header";
import "./globals.css";
import "./premium.css";
import "./intelligence.css";
export const metadata:Metadata={title:{default:"AI Business Assistant",template:"%s · AI Business Assistant"},description:"Семь AI-специалистов в едином рабочем пространстве для бизнеса."};
export default function RootLayout({children}:{children:React.ReactNode}){return <html lang="ru" suppressHydrationWarning><head><script dangerouslySetInnerHTML={{__html:'try{var t=localStorage.getItem("aba-theme");if(t==="light"||t==="dark")document.documentElement.dataset.theme=t}catch{}'}}/></head><body><SkipLink/><Header/>{children}</body></html>}
