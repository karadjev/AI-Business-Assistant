import {WorkflowPage} from "@/components/WorkflowPage";
export default async function Page({searchParams,params}:{searchParams:Promise<Record<string,string|undefined>>;params:Promise<{id:string}>}){return <WorkflowPage view="order" query={await searchParams} id={(await params).id}/>;}
