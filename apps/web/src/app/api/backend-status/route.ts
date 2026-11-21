import { getServiceStatus } from "@/lib/api";
export const dynamic="force-dynamic";
export async function GET() {
 const result=await getServiceStatus();
 return Response.json(result,{status:result.kind==="available"?200:result.kind==="unavailable"?503:502,headers:{"Cache-Control":"no-store"}});
}
