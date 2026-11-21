import {IdentityForm} from "@/components/IdentityForm";
export default function Page(){return <main id="main" tabIndex={-1} className="auth-page"><span className="eyebrow">AI Business Assistant</span><h1>Войти в кабинет</h1><p className="lead">Войдите с подтверждённым email.</p><IdentityForm kind="login"/></main>}
