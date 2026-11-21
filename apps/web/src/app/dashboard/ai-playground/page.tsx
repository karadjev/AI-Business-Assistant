import {WorkflowPage} from "@/components/WorkflowPage";
export default async function Page({searchParams}:{searchParams:Promise<Record<string,string|undefined>>}){return <WorkflowPage view="chat" query={await searchParams}/>;}
