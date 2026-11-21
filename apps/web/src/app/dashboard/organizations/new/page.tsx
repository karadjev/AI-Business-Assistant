import {requireUser} from "@/lib/session";
import {IdentityForm} from "@/components/IdentityForm";
export default async function NewOrganization(){await requireUser();return <main id="main" tabIndex={-1}><h1>Создать магазин</h1><p className="lead">Новый магазин электроники будет принадлежать вашему аккаунту.</p><IdentityForm kind="create"/></main>}
