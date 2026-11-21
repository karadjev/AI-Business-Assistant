import type {Metadata} from "next";
import {SkipLink} from "@/components/SkipLink";
import {Header} from "@/components/Header";
import "./globals.css";
import "./premium.css";
export const metadata:Metadata={title:{default:"AI Business Assistant",template:"%s · AI Business Assistant"},description:"ИИ-продавец с каталогом и заявками для малого бизнеса. Магазин, диалоги и заявки под вашим контролем."};
export default function RootLayout({children}:{children:React.ReactNode}){return <html lang="ru" suppressHydrationWarning><head><script dangerouslySetInnerHTML={{__html:'try{var t=localStorage.getItem("aba-theme");if(t==="light"||t==="dark")document.documentElement.dataset.theme=t}catch{}'}}/></head><body><SkipLink/><Header/>{children}<footer><span>AI Business Assistant</span><span>Каталог. Диалоги. Решения под вашим контролем.</span></footer></body></html>}
