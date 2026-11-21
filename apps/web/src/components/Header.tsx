import Link from "next/link";
import {ThemeSwitch} from "./ThemeSwitch";
export function Header(){return <header className="header"><Link prefetch={false} href="/" className="brand"><span className="brand-mark" aria-hidden="true">A</span><span>AI Business<span className="brand-sub">Assistant</span></span></Link><nav aria-label="Основная навигация"><Link prefetch={false} href="/dashboard">Личный кабинет</Link><Link prefetch={false} href="/status" className="nav-status">Состояние системы</Link></nav><span className="stage-tag">Telegram · WhatsApp</span><ThemeSwitch/></header>}
