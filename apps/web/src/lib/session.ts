import "server-only";
import {cookies} from "next/headers";
import {redirect} from "next/navigation";
import {apiBaseURL} from "./config";
export type User={id:string;email:string;display_name:string};
export type Organization={id:string;name:string;business_type:string;timezone:string;version:number;role:"owner"|"member";is_test:boolean;can_manage_orders:boolean};
export async function request(path:string){
 const jar=await cookies();const header=["aba_session","__Host-aba_session","aba_csrf"].map(name=>jar.get(name)).filter(Boolean).map(c=>`${c!.name}=${c!.value}`).join("; ");
 try{return await fetch(`${apiBaseURL()}/api/v1/${path}`,{headers:{Cookie:header},cache:"no-store",redirect:"error",signal:AbortSignal.timeout(4000)});}catch{throw new Error("Backend temporarily unavailable");}
}
export async function requireUser():Promise<User>{const r=await request("auth/me");if(r.status===401)redirect("/login");if(!r.ok)throw new Error("Session check unavailable");return (await r.json()).user;}
export async function organizations():Promise<Organization[]>{const r=await request("organizations");if(r.status===401)redirect("/login");if(!r.ok)throw new Error("Organizations unavailable");return (await r.json()).items;}
