import {test,expect,type Browser} from "@playwright/test";
import {execFileSync} from "node:child_process";
import {randomUUID} from "node:crypto";
import {friendlyName,displayDate} from "../src/lib/presentation";
type User={email:string;password:string};
type Fixture={org:string;staff:User;owner:User;member:User};
async function login(browser:Browser,user:User) {
 const ctx=await browser.newContext();const p=await ctx.newPage();await p.goto('/login');
 await p.getByLabel('Email',{exact:true}).fill(user.email);await p.getByLabel('Пароль',{exact:true}).fill(user.password);await p.getByRole('button',{name:'Войти',exact:true}).click();await expect(p).toHaveURL(/\/dashboard/);return p;
}
test('presentation: missing/technical names and invalid date data stay readable',()=>{
 for(const name of [undefined,null,'','  ','demo@example.test','11111111-1111-4111-8111-111111111111','demo 11111111-1111-4111-8111-111111111111'])expect(friendlyName(name)).toBe('Пользователь');
 expect(friendlyName('SYNTHETIC demo owner')).toBe('Демо-пользователь');expect(friendlyName('  Амина  ')).toBe('Амина');
 expect(displayDate(undefined)).toBe('Дата неизвестна');expect(displayDate('invalid')).toBe('Дата неизвестна');expect(displayDate('2026-11-17T09:00:00Z','Europe/Moscow')).toContain('12:00');expect(displayDate('2026-11-17T09:00:00Z','invalid-zone')).toContain('UTC');
});
test('polish real backend: staff approval, cancel, network uncertainty, double click, stock and member boundary',async({browser})=>{
 test.setTimeout(180000);const bin=process.env.ABA_BROWSER_FIXTURE_BIN;test.skip(!bin,'Requires isolated DB marker and prepared fixture CLI');if(!bin)throw Error('Fixture missing');
 const f=JSON.parse(execFileSync(bin,['demo',randomUUID()],{encoding:'utf8'})) as Fixture;
 const p=await login(browser,f.staff);const root='/api/v1/organizations/'+f.org;
 const orders=await (await p.request.get(root+'/orders?status=pending_confirmation')).json();expect(orders.items).toHaveLength(1);const order=orders.items[0];expect(order.status).toBe('pending_confirmation');
 const stock=async()=>{const state=await (await p.request.get(root+'/inventory')).json();return state.items.find((item:{variant_id:string})=>item.variant_id===order.items[0].variant_id)};
 // List responses may omit items: read the detail through the actual BFF first.
 const detail=await (await p.request.get(root+'/orders/'+order.id)).json();order.items=detail.items;
 const before=await stock();await p.goto('/dashboard/orders/'+order.id+'?org='+f.org);await expect(p.getByRole('heading',{name:'Заказ',exact:true})).toBeVisible();
 const button=p.getByRole('button',{name:'Подтвердить и зарезервировать',exact:true});const dialog=p.getByRole('dialog');
 await button.click();await expect(dialog.getByRole('button',{name:'Вернуться',exact:true})).toBeFocused();await p.keyboard.press('Shift+Tab');await expect(dialog.getByRole('button',{name:'Подтвердить и зарезервировать',exact:true})).toBeFocused();await p.keyboard.press('Tab');await expect(dialog.getByRole('button',{name:'Вернуться',exact:true})).toBeFocused();await p.keyboard.press('Escape');await expect(button).toBeFocused();expect((await stock()).quantity_reserved).toBe(before.quantity_reserved);
 const confirmPath='**/api/v1/organizations/'+f.org+'/orders/'+order.id+'/confirm';let attempts=0;
 await p.route(confirmPath,async route=>{attempts++;await route.abort('failed')});await button.click();await dialog.getByRole('button',{name:'Подтвердить и зарезервировать',exact:true}).click();await expect(p.getByRole('main').getByRole('alert')).toContainText('проверьте статус');expect(attempts).toBe(1);expect((await stock()).quantity_reserved).toBe(before.quantity_reserved);expect((await (await p.request.get(root+'/orders/'+order.id)).json()).status).toBe('pending_confirmation');await p.unroute(confirmPath);
 // A 409 remains an error, never a successful or automatic retry.
 await p.route(confirmPath,async route=>{attempts++;await route.fulfill({status:409,json:{error:{code:'VERSION_CONFLICT',message:'Синтетический конфликт версии'}}})});await button.click();await dialog.getByRole('button',{name:'Подтвердить и зарезервировать',exact:true}).click();await expect(p.getByRole('main').getByRole('alert')).toContainText('Синтетический конфликт версии');expect(attempts).toBe(2);await p.unroute(confirmPath);
 let sent=0;p.on('request',r=>{if(r.method()==='POST'&&r.url().endsWith('/orders/'+order.id+'/confirm')){sent++;const body=r.postDataJSON();expect(body.expected_version).toBe(order.version);expect(body.idempotency_key).toMatch(/^[a-f0-9-]{36}$/)}});
 await p.route(confirmPath,async route=>{await new Promise(resolve=>setTimeout(resolve,350));await route.continue()});await button.click();await dialog.getByRole('button',{name:'Подтвердить и зарезервировать',exact:true}).dblclick();await expect(p.getByText('Подтверждена · резерв выполнен',{exact:true})).toBeVisible();expect(sent).toBe(1);expect((await stock()).quantity_reserved).toBe(before.quantity_reserved+detail.items[0].quantity);await p.reload();await expect(p.getByText('Подтверждена · резерв выполнен',{exact:true})).toBeVisible();
 await p.getByRole('button',{name:'Отменить и освободить резерв',exact:true}).click();await dialog.getByRole('button',{name:'Отменить и освободить резерв',exact:true}).click();await expect(p.getByText('Отменена',{exact:true})).toBeVisible();expect((await stock()).quantity_reserved).toBe(before.quantity_reserved);
 const member=await login(browser,f.member);await member.goto('/dashboard/orders/'+order.id+'?org='+f.org);await expect(member.getByRole('heading',{name:'Недостаточно прав'})).toBeVisible();await expect(member.getByRole('button',{name:'Подтвердить и зарезервировать'})).toHaveCount(0);expect((await member.request.get(root+'/orders/'+order.id)).status()).toBe(403);
 // Tenant selection is never proof of access: a foreign UUID must not reveal the order.
 await member.goto('/dashboard/orders/'+order.id+'?org='+randomUUID());await expect(member.getByRole('heading',{name:'Выберите магазин',exact:true})).toBeVisible();await expect(member.getByRole('main')).not.toContainText(order.id);
 await p.context().close();await member.context().close();
});
test('polish real backend: owned store switch, empty catalog, filtered results and tenant links',async({browser})=>{
 test.setTimeout(120000);const bin=process.env.ABA_BROWSER_FIXTURE_BIN;test.skip(!bin,'Requires isolated fixture');if(!bin)throw Error('Fixture missing');
 const f=JSON.parse(execFileSync(bin,['demo',randomUUID()],{encoding:'utf8'})) as Fixture;const p=await login(browser,f.owner);
 await p.goto('/dashboard/organizations/new');await p.getByLabel('Название магазина',{exact:true}).fill('SYNTHETIC — новый магазин');await p.getByRole('button',{name:'Создать магазин',exact:true}).click();await expect(p).toHaveURL(/\/dashboard\?org=/);
 const second=new URL(p.url()).searchParams.get('org')!;expect(second).not.toBe(f.org);await expect(p.locator('main h1')).toHaveText('SYNTHETIC — новый магазин');await expect(p.getByText('Обычный магазин',{exact:true})).toBeVisible();await expect(p.getByText('Сейчас всё спокойно. Новых задач нет.',{exact:true})).toHaveCount(0);
 await p.getByRole('link',{name:'Открыть товары',exact:true}).first().click();await expect(p).toHaveURL(new RegExp('/products\\?org='+second));await expect(p.getByRole('heading',{name:'Добавьте первый товар',exact:true})).toBeVisible();await expect(p.locator('main .button.primary')).toHaveCount(1);await expect(p.getByRole('link',{name:'Импортировать из Excel',exact:true})).toHaveAttribute('href',new RegExp('org='+second));
 const nav=p.getByRole('navigation',{name:'Основные разделы'});for(const link of await nav.getByRole('link').all())await expect(link).toHaveAttribute('href',new RegExp('org='+second));
 await nav.getByRole('link',{name:'Обзор',exact:true}).click();await p.locator('.store-picker summary').click();const shops=await (await p.request.get('/api/v1/organizations')).json();const name=shops.items.find((shop:{id:string;name:string})=>shop.id===f.org).name;await p.getByRole('navigation',{name:'Выбор магазина'}).getByRole('link',{name,exact:true}).click();await expect(p).toHaveURL(new RegExp('org='+f.org));await expect(p.locator('main h1')).toHaveText(name);
 await nav.getByRole('link',{name:'Товары',exact:true}).click();await p.getByLabel('Поиск',{exact:true}).fill('SYNTHETIC_NO_MATCH_2719');await p.getByRole('button',{name:'Найти',exact:true}).click();await expect(p.getByRole('heading',{name:'Ничего не найдено',exact:true})).toBeVisible();await p.getByRole('link',{name:'Сбросить поиск и фильтры',exact:true}).click();await expect(p).toHaveURL(new RegExp('/products\\?org='+f.org+'$'));const available=await (await p.request.get('/api/v1/organizations/'+f.org+'/products')).json();expect(available.items.length).toBeGreaterThan(0);await expect(p.getByRole('link',{name:available.items[0].name,exact:true})).toBeVisible();
 await p.context().close();
});
