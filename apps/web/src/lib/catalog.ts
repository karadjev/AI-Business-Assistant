export type Category={id:string;name:string;version:number};
export type Inventory={catalog_source?:"MANUAL"|"EXCEL"|"ONE_C";variant_id:string;sku:string;variant_name:string;product_name:string;quantity_on_hand:number;quantity_reserved:number;available:number;version:number;status:string;manual_updated_at:string|null};
export type Variant={catalog_source?:"MANUAL"|"EXCEL"|"ONE_C";id:string;sku:string;variant_name:string;attributes:Record<string,string>;price_amount:number;currency_code:string;version:number;inventory:Inventory};
export type Product={catalog_source?:"MANUAL"|"EXCEL"|"ONE_C";id:string;name:string;description:string;brand:string;category_id:string|null;status:"draft"|"active"|"archived";version:number;price_amount:number;available:number;variants:Variant[]};
export type Movement={id:string;sku:string;product_name:string;actor_id:string;reason:string;quantity_before:number;quantity_after:number;delta:number;created_at:string;kind:"adjust"|"reserve"|"release"|"consume";reserved_before:number;reserved_after:number};
export function money(amount:number){return new Intl.NumberFormat("ru-RU",{style:"currency",currency:"RUB"}).format(amount/100);}
// Parse decimal input as digits, never multiply a floating point currency amount.
export function minor(value:string){if(!/^\d{1,11}([.,]\d{1,2})?$/.test(value))throw new Error("Цена: неотрицательное число, до двух знаков после запятой");const [whole,fraction=""]=value.replace(",",".").split(".");const amount=Number(whole)*100+Number(fraction.padEnd(2,"0"));if(amount>9000000000000)throw new Error("Слишком большая цена");return amount;}
export function decimal(amount:number){return `${Math.floor(amount/100)}.${String(amount%100).padStart(2,"0")}`;}
