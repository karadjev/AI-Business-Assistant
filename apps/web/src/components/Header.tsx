import Link from "next/link";
import {ThemeSwitch} from "./ThemeSwitch";

export function Header() {
  return <header className="header intelligence-header">
    <Link prefetch={false} href="/" className="brand">
      <span className="brand-mark" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none"><path d="M12 2.5 20 8.5 18 17 12 21.5 6 17 4 8.5Z M12 2.5 9 10l3 3 3-3-3-7.5Zm-8 6 8 4.5 8-4.5m-8 4.5v8.5" stroke="currentColor" strokeWidth="1.3" strokeLinejoin="round"/></svg></span>
      <span>AI Business<span className="brand-sub">Assistant</span></span>
    </Link>
    <nav aria-label="Основная навигация">
      <Link prefetch={false} href="/#live-demo" className="nav-landing">Демо</Link>
      <Link prefetch={false} href="/#intelligence-team" className="nav-landing">AI-команда</Link>
      <Link prefetch={false} href="/login" className="nav-entry">Войти</Link>
    </nav>
    <ThemeSwitch/>
  </header>;
}
