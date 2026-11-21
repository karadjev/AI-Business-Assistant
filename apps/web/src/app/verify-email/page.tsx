import {IdentityForm} from "@/components/IdentityForm";
export default function Page(){return <main id="main" tabIndex={-1} className="auth-page"><span className="eyebrow">AI Business Assistant</span><h1>Подтвердить email</h1><p className="lead">Откройте ссылку из письма в локальной почте или вставьте код подтверждения.</p><IdentityForm kind="verify"/></main>}
