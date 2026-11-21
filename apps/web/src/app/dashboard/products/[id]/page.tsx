import {CatalogPage} from "@/components/CatalogPage";
export default async function Page({searchParams,params}:{searchParams:Promise<Record<string,string|undefined>>;params:Promise<{id:string}>}){return <CatalogPage view="detail" query={await searchParams} id={(await params).id}/>;}
