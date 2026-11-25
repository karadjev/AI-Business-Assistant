import "server-only";
import {request} from "./session";

export type WorkshopOverview = {
  customers:number; requests:number; completed:number; financial_count:number; paid_count:number;
  cash_minor:number; gross_minor:number|null; available_skus:number; approvals:number; marketing_pending:number;
};

export async function workshopOverview(id:string):Promise<WorkshopOverview|null> {
  const response = await request(`organizations/${id}/workshop`).catch(() => null);
  if (!response?.ok) return null;
  const data = await response.json().catch(() => null) as {enabled?:boolean;overview?:WorkshopOverview} | null;
  return data?.enabled ? data.overview ?? null : null;
}
