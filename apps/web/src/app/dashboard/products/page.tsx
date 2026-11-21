import {CatalogPage} from "@/components/CatalogPage";
export default async function Page({searchParams}:{searchParams:Promise<Record<string,string|undefined>>}){return <CatalogPage view="products" query={await searchParams}/>;}
