function catalogMethods(path:string):string[]|undefined{
 const root="v1/organizations/[0-9a-f-]{36}";
 if(new RegExp(`^${root}/integrations/instagram$`).test(path))return ["GET"];
 if(new RegExp(`^${root}/integrations/instagram/(oauth/start|disconnect)$`).test(path))return ["POST"];
 if(new RegExp(`^${root}/integrations/instagram/(oauth/callback|webhook-status)$`).test(path))return ["GET"];
 if(new RegExp(`^${root}/catalog-control$`).test(path))return ["GET"];
 if(new RegExp(`^${root}/catalog-control/policy$`).test(path))return ["PUT"];
 if(new RegExp(`^${root}/catalog-control/retention-binding$`).test(path))return ["POST"];
 if(new RegExp(`^${root}/catalog-control/source$`).test(path))return ["POST"];
 if(new RegExp(`^${root}/catalog-imports/[0-9a-f-]{36}/hold$`).test(path))return ["POST"];
 if(new RegExp(`^${root}/onboarding$`).test(path))return ["GET","POST"];
 if(new RegExp(`^${root}/store-profile$`).test(path))return ["PUT"];
 if(new RegExp(`^${root}/catalog-imports$`).test(path))return ["POST"];
 if(new RegExp(`^${root}/catalog-imports/[0-9a-f-]{36}$`).test(path))return ["GET"];
 if(new RegExp(`^${root}/catalog-imports/[0-9a-f-]{36}/(mapping|confirm|cancel)$`).test(path))return ["POST"];
 if(new RegExp(`^${root}/integrations/whatsapp/(financial|templates|provider|operator)$`).test(path))return ["GET"];
 if(new RegExp(`^${root}/integrations/whatsapp/provider/[0-9a-f-]{36}/revoke$`).test(path))return ["POST"];
 if(new RegExp(`^${root}/integrations/whatsapp/financial/(check|[0-9a-f-]{36}/reconcile)$`).test(path))return ["POST"];
 if(new RegExp(`^${root}/integrations/whatsapp$`).test(path))return ["GET"];
 if(new RegExp(`^${root}/integrations/whatsapp/(connect|control|synthetic-updates)$`).test(path))return ["POST"];
 if(new RegExp(`^${root}/integrations/whatsapp/deliveries/[0-9a-f-]{36}/resolve$`).test(path))return ["POST"];
 if(new RegExp(`^${root}/integrations/telegram/deliveries$`).test(path))return ["GET"];
 if(new RegExp(`^${root}/integrations/telegram/deliveries/[0-9a-f-]{36}/resolve$`).test(path))return ["POST"];
 if(new RegExp(`^${root}/llm-reservations$`).test(path))return ["GET"];
 if(new RegExp(`^${root}/llm-reservations/[0-9a-f-]{36}/recover$`).test(path))return ["POST"];
 if(new RegExp(`^${root}/ai-settings$`).test(path))return ["GET","PATCH"];
 if(new RegExp(`^${root}/agent-(team|audit)$`).test(path))return ["GET"];
 if(new RegExp(`^${root}/agent-business-profile$`).test(path))return ["PATCH"];
 if(new RegExp(`^${root}/agent-tasks$`).test(path))return ["GET","POST"];
 if(new RegExp(`^${root}/marketing-drafts$`).test(path))return ["GET"];
 if(new RegExp(`^${root}/marketing-drafts/[0-9a-f-]{36}/review$`).test(path))return ["POST"];
 if(new RegExp(`^${root}/agent-delegations/(auto-parts|construction|auto-service|real-estate|property-viewing|(auto-service|property-viewing)/[0-9a-f-]{36}/prepare)$`).test(path))return ["POST"];
 if(new RegExp(`^${root}/agent-workflows/preview$`).test(path))return ["POST"];
 if(new RegExp(`^${root}/agent-approvals$`).test(path))return ["GET"];
 if(new RegExp(`^${root}/agent-approvals/[0-9a-f-]{36}/decision$`).test(path))return ["POST"];
 if(new RegExp(`^${root}/agent/metrics$`).test(path))return ["GET"];
 if(new RegExp(`^${root}/integrations/telegram/live-operations$`).test(path))return ["POST"];
 if(new RegExp(`^${root}/conversations/[0-9a-f-]{36}/purge-test-data$`).test(path))return ["POST"];
 if(new RegExp(`^${root}/integrations/telegram$`).test(path))return ["GET"];
 if(new RegExp(`^${root}/integrations/telegram/(connect|check|disconnect|synthetic-updates)$`).test(path))return ["POST"];
 if(new RegExp(`^${root}/operations/property-viewings/[0-9a-f-]{36}$`).test(path))return ["GET"];
 if(new RegExp(`^${root}/operations/(catalog/(search|fitment)|construction/calculate|services/(search|availability|config/(resources|offerings|windows))|properties/(search|discover)|requests)$`).test(path))return ["POST"];
 if(new RegExp(`^${root}/(orders|conversations)$`).test(path))return path.endsWith("conversations")?["GET","POST"]:["GET"];
 if(new RegExp(`^${root}/(orders|conversations)/[0-9a-f-]{36}$`).test(path))return ["GET"];
 if(new RegExp(`^${root}/orders/[0-9a-f-]{36}/(confirm|reject|cancel|complete)$`).test(path))return ["POST"];
 if(new RegExp(`^${root}/conversations/[0-9a-f-]{36}/(messages|handoff|resume|delete|quotes|confirm-request)$`).test(path))return ["POST"];
 if(new RegExp(`^${root}/agent/status$`).test(path))return ["GET"];
 if(new RegExp(`^${root}/members/[0-9a-f-]{36}/order-access$`).test(path))return ["PATCH"];
 if(new RegExp(`^${root}/(categories|products)$`).test(path))return ["GET","POST"];
 if(new RegExp(`^${root}/(categories|variants)/[0-9a-f-]{36}$`).test(path))return ["PATCH"];
 if(new RegExp(`^${root}/products/[0-9a-f-]{36}$`).test(path))return ["GET","PATCH"];
 if(new RegExp(`^${root}/products/[0-9a-f-]{36}/variants$`).test(path))return ["GET","POST"];
 if(new RegExp(`^${root}/products/[0-9a-f-]{36}/(archive|publish)$`).test(path))return ["POST"];
 if(new RegExp(`^${root}/inventory(/movements)?$`).test(path))return ["GET"];
 if(new RegExp(`^${root}/inventory/adjustments$`).test(path))return ["POST"];
}
import {apiBaseURL} from "@/lib/config";
const allowed=new Map([
 ["v1/auth/register",["POST"]],["v1/auth/login",["POST"]],["v1/auth/logout",["POST"]],
 ["v1/auth/verify-email",["POST"]],["v1/auth/resend-verification",["POST"]],["v1/auth/me",["GET"]],
 ["v1/organizations",["GET","POST"]],["v1/organizations/test",["POST"]],
]);
export const dynamic="force-dynamic";
async function proxy(request:Request,{params}:{params:Promise<{path:string[]}>}){
 const path=(await params).path.join("/");const methods=catalogMethods(path)??allowed.get(path)??(/^v1\/organizations\/[0-9a-f-]{36}$/.test(path)?["GET","PATCH"]:[]);
 if(!methods.includes(request.method))return Response.json({error:{code:"NOT_FOUND",message:"Ресурс недоступен"}},{status:404});
 const upload=/^v1\/organizations\/[0-9a-f-]{36}\/catalog-imports$/.test(path)&&request.method==="POST";const bodyLimit=upload?1048576:65536;
 const headers=new Headers();for(const key of ["Content-Type","Origin","X-CSRF-Token","X-Idempotency-Key","X-Catalog-Filename"]){const v=request.headers.get(key);if(v)headers.set(key,v);}
 const cookie=(request.headers.get("cookie")??"").split(";").filter(v=>/^(aba_session|__Host-aba_session|aba_csrf)=/.test(v.trim())).join(";");headers.set("Cookie",cookie);
 let body:Uint8Array|undefined;
 if(request.method!=="GET"){
  if(Number(request.headers.get("content-length")??0)>bodyLimit)return Response.json({error:{code:"BODY_TOO_LARGE",message:"Слишком большой запрос"}},{status:413});
  const reader=request.body?.getReader();const chunks:Uint8Array[]=[];let size=0;
  if(reader){for(;;){const next=await reader.read();if(next.done)break;size+=next.value.length;if(size>bodyLimit){await reader.cancel();return Response.json({error:{code:"BODY_TOO_LARGE",message:"Слишком большой запрос"}},{status:413});}chunks.push(next.value);}}
  body=new Uint8Array(size);let offset=0;for(const chunk of chunks){body.set(chunk,offset);offset+=chunk.length;}
 }
 try{
  const response=await fetch(`${apiBaseURL()}/api/${path}${new URL(request.url).search}`,{method:request.method,headers,body:body?Buffer.from(body):undefined,cache:"no-store",redirect:"error",signal:AbortSignal.timeout(5000)});
  const out=new Headers({"Cache-Control":"no-store","Content-Type":"application/json; charset=utf-8"});
  for(const cookie of response.headers.getSetCookie())out.append("Set-Cookie",cookie);
  for(const name of ["X-Request-ID","Retry-After"]){const value=response.headers.get(name);if(value)out.set(name,value);}
  return new Response(response.status===204?null:await response.text(),{status:response.status,headers:out});
 }catch{return Response.json({error:{code:"SERVICE_UNAVAILABLE",message:"Backend временно недоступен"}},{status:503,headers:{"Cache-Control":"no-store"}});}
}
export {proxy as GET,proxy as POST,proxy as PATCH,proxy as PUT};
