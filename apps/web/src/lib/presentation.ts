/** Presentation only: never infer permissions or business state. */
export function friendlyName(value: string | null | undefined) {
 const name=value?.trim();
 if(!name||name.includes("@")||/[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9-]{20,}/i.test(name))return "Пользователь";
 if(/^synthetic\s+demo\s+(owner|staff|member)$/i.test(name))return "Демо-пользователь";
 return name;
}
export function displayDate(value:string|undefined,timezone="UTC") {
 if(!value||!Number.isFinite(new Date(value).getTime()))return "Дата неизвестна";
 try{return new Intl.DateTimeFormat("ru-RU",{dateStyle:"short",timeStyle:"short",timeZone:timezone}).format(new Date(value));}
 catch{return new Intl.DateTimeFormat("ru-RU",{dateStyle:"short",timeStyle:"short",timeZone:"UTC"}).format(new Date(value))+" UTC";}
}
