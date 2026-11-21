import "server-only";
import { apiBaseURL } from "./config";

export type ServiceStatus = {service:"ai-business-assistant";status:"running";api_version:"v1"};
export type StatusResult = {kind:"available";data:ServiceStatus} | {kind:"unavailable"} | {kind:"error"};

export async function getServiceStatus(): Promise<StatusResult> {
 let endpoint: string;
 try {endpoint = `${apiBaseURL()}/api/v1/status`;} catch {return {kind:"error"};}
 try {
  const response=await fetch(endpoint,{cache:"no-store",signal:AbortSignal.timeout(3000),redirect:"error"});
  if (!response.ok) return {kind:response.status>=500?"unavailable":"error"};
  const data: unknown=await response.json();
  if (typeof data!=="object" || data===null || !("service" in data) || !("status" in data) || !("api_version" in data) || data.service!=="ai-business-assistant" || data.status!=="running" || data.api_version!=="v1") return {kind:"error"};
  return {kind:"available",data:{service:data.service,status:data.status,api_version:data.api_version}};
 } catch {return {kind:"unavailable"};}
}
