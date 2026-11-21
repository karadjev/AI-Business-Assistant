import Link from "next/link";
import {redirect} from "next/navigation";
import {organizations,request} from "@/lib/session";
import {CatalogWorkspace} from "./CatalogWorkspace";
type Query=Record<string,string|string[]|undefined>;
export async function CatalogPage({view,query,id}:{view:string;query:Query;id?:string}){
 const list=await organizations();const org=list.find(o=>o.id===query.org)??(query.org?undefined:list[0]);
 if(!org)return <main id="main" tabIndex={-1}><h1>Выберите магазин</h1><p>Организация недоступна или ещё не создана.</p><Link prefetch={false} href="/dashboard/organizations">Организации</Link></main>;
 const root=`organizations/${org.id}`;
 async function load(path:string){const r=await request(path);if(r.status===401)redirect("/login");if(!r.ok){const body=await r.json();return {data:null,error:body.error?.message??`Ошибка ${r.status}`};}return {data:await r.json(),error:""};}
 const categoryResult=await load(root+"/categories");const params=new URLSearchParams();for(const key of ["q","category_id","status","stock","sort","page","page_size"]){if(typeof query[key]==="string"&&query[key])params.set(key,query[key]);}
 const dataResult=view==="new"?{data:{},error:""}:view==="detail"?await load(root+`/products/${id}`):await load(root+`/${view}?${params}`);
 const movementsResult=view==="inventory"?await load(root+`/inventory/movements?${params}`):null;
 const categories=categoryResult.data;const data=dataResult.data;const movements=movementsResult?.data;const error=dataResult.error||categoryResult.error||movementsResult?.error||"";
 if(error||view==="detail"&&!data)return <main id="main" tabIndex={-1}><h1>{view==="detail"?"Товар недоступен":"Данные недоступны"}</h1><p role="alert">{error}</p><Link prefetch={false} href={"/dashboard/products?org="+org.id}>Вернуться к каталогу</Link></main>;
 return <CatalogWorkspace key={org.id+":"+JSON.stringify(data)} org={org} list={list} view={view} data={{...data,page:Number(query.page??1)}} categories={categories?.items??[]} product={view==="detail"?data:undefined} movements={movements?.items??[]} initialError={error}/>;
}
