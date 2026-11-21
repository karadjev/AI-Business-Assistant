import {test,expect,type Page,type Browser} from '@playwright/test';
import {execFileSync} from 'node:child_process';
import {createHmac} from 'node:crypto';
type User={id:string;email:string;password:string};
type Shop={id:string;owner:User;variant:string;price:number;name:string};
type Delivery={id:string;status:string;provider_id?:string;payload:{text?:string;actions?:{id:string}[];reply_markup?:{inline_keyboard:{callback_data:string}[][]}}};
type Conversation={id:string;channel:string;control_mode:string};
type Order={id:string;source:string;status:string};
async function login(browser:Browser,user:User){const context=await browser.newContext({baseURL:process.env.SMOKE_WEB_URL??"http://web:3000"});const page=await context.newPage();await page.goto('/login');await page.getByLabel('Email',{exact:true}).fill(user.email);await page.getByLabel('Пароль',{exact:true}).fill(user.password);await page.getByRole('button',{name:'Войти',exact:true}).click();await expect(page).toHaveURL(/\/dashboard$/);return page;}
async function post(page:Page,path:string,body:unknown){const csrf=(await page.context().cookies()).find(c=>c.name==='aba_csrf')?.value??'';return page.request.post(path,{headers:{Origin:new URL(page.url()).origin,'X-CSRF-Token':csrf},data:body});}
test.use({actionTimeout:15000,navigationTimeout:30000});
test('current production: three shops, Telegram and WhatsApp, consent/staff reserve, handoff/window/status/isolation',async({browser},info)=>{
 test.setTimeout(900000);const bin=process.env.ABA_BROWSER_FIXTURE_BIN;test.skip(!bin,'Requires isolated synthetic DB fixture binary and current-build guard');if(!bin)throw Error('fixture missing');
 const fixture=JSON.parse(execFileSync(bin,['seed'],{encoding:'utf8'})) as {shops:Shop[];staff:User;member:User;operator:User};
 const owners:Page[]=[];let orderA='';let convA='';const runBase=(Math.floor(Date.now()/1000)%1000000+800000)*10;
 for(let index=0;index<fixture.shops.length;index++){
  const shop=fixture.shops[index];const page=await login(browser,shop.owner);owners.push(page);const root=`/api/v1/organizations/${shop.id}`;
  await page.goto(`/dashboard/products?org=${shop.id}`);await expect(page.getByRole('link',{name:'Synthetic Galaxy P8D',exact:true})).toBeVisible();
  const products=await (await page.request.get(root+'/products')).json();expect(products.items).toHaveLength(1);
  const stock=async()=>{const x=await (await page.request.get(root+'/inventory')).json();return x.items.find((i:{variant_id:string})=>i.variant_id===shop.variant)};
  expect((await stock()).quantity_reserved).toBe(0);
  for(const channel of ['telegram','whatsapp'] as const){
   await page.goto(`/dashboard/integrations/${channel}?org=${shop.id}`);let counter=100;
   if(channel==='telegram'){
    await page.getByLabel('Тестовый credential (mock:10001)').fill('mock:'+String(runBase+index));await page.getByRole('button',{name:'Проверить и подключить',exact:true}).click();await expect(page.getByText(new RegExp('@mock_shop_'+(runBase+index)+'_bot'))).toBeVisible();
   }else{
    await expect(page.getByRole('heading',{name:'WhatsApp · MOCK',exact:true})).toBeVisible();await page.getByLabel('Phone Number ID',{exact:true}).fill(String(runBase+100+index));await page.getByRole('button',{name:'Сохранить mock-конфигурацию',exact:true}).click();await expect(page.getByText(/Phone Number ID:.*mock настроен/)).toBeVisible();await page.getByRole('button',{name:'Включить mock-отправку',exact:true}).click();await expect(page.getByRole('button',{name:'Остановить mock-отправку',exact:true})).toBeVisible();
    execFileSync(bin,['wa-safety',shop.id]);await page.reload();
    const safety=page.getByRole('region',{name:'Финансовая безопасность WhatsApp'});
    await expect(safety.getByText(/Лимит: 0 RUB/)).toBeVisible();
    await expect(safety.getByRole('heading',{name:'browser_synthetic_approved · ru',exact:true})).toBeVisible();
    await expect(safety.getByText(/UTILITY · APPROVED · SYNTHETIC/)).toBeVisible();
    await expect(safety.getByText(/UTILITY · PENDING · SYNTHETIC/)).toBeVisible();
    expect((await page.request.get(root+'/integrations/whatsapp/operator')).status()).toBe(403);
    await expect(safety.getByText(/NO_OFFICIAL_PROVIDER_INSTALLED/)).toBeVisible();
    await safety.getByRole('button',{name:'Проверить ограничения без отправки'}).click();
    await expect(safety.getByRole('alert')).toContainText('Live WhatsApp выключен');
    const blocked=await post(page,root+'/integrations/whatsapp/financial/check',{});expect(blocked.status()).toBe(403);
    expect((await post(page,root+'/integrations/whatsapp/templates',{name:'browser_synthetic_pending',status:'APPROVED'})).status()).toBe(404);const registry=await (await page.request.get(root+'/integrations/whatsapp/templates')).json();expect(registry.find((t:{name:string;status:string})=>t.name==='browser_synthetic_pending').status).toBe('PENDING');
    await page.screenshot({path:info.outputPath(`shop-${index}-wa-financial.png`),fullPage:true});

   }
   const state=async()=>{await new Promise(r=>setTimeout(r,5500));let response=await page.request.get(root+`/integrations/${channel}`);if(response.status()===429){await new Promise(r=>setTimeout(r,61000));response=await page.request.get(root+`/integrations/${channel}`)}expect(response.status()).toBe(200);return response.json()};
   const deliveries=async():Promise<Delivery[]>=>{const s=await state();return channel==='telegram'?s.integration.deliveries:s.deliveries};
   const push=async(text:string,action='')=>{await new Promise(r=>setTimeout(r,2500));counter++;if(channel==='telegram'){
    await page.getByLabel('Update ID (уникальный, по порядку)').fill(String(counter));await page.getByLabel('Сообщение',{exact:true}).fill(text);await page.getByLabel('Callback (пусто для сообщения)').fill(action);await page.getByRole('button',{name:'Сохранить update',exact:true}).click();
   }else{
    await page.getByLabel('Уникальный ID сообщения').fill('fixture-'+counter);await page.getByLabel('Текст',{exact:true}).fill(text);await page.getByLabel('Действие кнопки (необязательно)').fill(action);await page.getByRole('button',{name:'Сохранить подписанное fixture-событие',exact:true}).click();
   }};
   if(index===0){
    execFileSync(bin,['stock-state',shop.id,'STALE']);
    await push('Есть P8D?');await expect.poll(async()=>(await deliveries()).some(d=>d.payload.text?.includes('наличие требует подтверждения магазина')),{timeout:20000}).toBe(true);
    await page.screenshot({path:info.outputPath(`${channel}-stale-stock-safe.png`),fullPage:true});
    const current=await stock();const refreshed=await post(page,root+'/inventory/adjustments',{variant_id:shop.variant,quantity_on_hand:current.quantity_on_hand,expected_version:current.version,reason:'SYNTHETIC stock verification'});expect(refreshed.status()).toBe(200);
   }
   await push('Есть P8D?');await expect.poll(async()=>(await deliveries()).some(d=>d.payload.text?.includes('P8D')),{timeout:15000}).toBe(true);
   const conversations=async():Promise<Conversation[]>=>{const d=await (await page.request.get(root+'/conversations?channel='+channel)).json();return d.items};
   const conv=(await conversations())[0];expect(conv.channel).toBe(channel);const integration=(await state()).integration.id;
   const detail=await (await page.request.get(root+'/conversations/'+conv.id)).json();const cards=detail.messages.flatMap((m:{data:{cards?:{price_amount:number;variant_id:string}[]}})=>m.data.cards??[]);expect(cards.some((c:{price_amount:number;variant_id:string})=>c.price_amount===shop.price&&c.variant_id===shop.variant)).toBe(true);
   const requests=async():Promise<Order[]>=>{const d=await (await page.request.get(root+'/orders')).json();return d.items.filter((o:Order)=>o.source===channel)};
   expect(await requests()).toHaveLength(0);expect((await stock()).quantity_reserved).toBe(channel==='telegram'?0:1);
   const actions=async()=>{const d=await deliveries();return channel==='telegram'?d.flatMap(x=>x.payload.reply_markup?.inline_keyboard.flat().map(b=>b.callback_data)??[]):d.flatMap(x=>x.payload.actions?.map(a=>a.id)??[])};
   const offer=(await actions()).find(a=>a.startsWith('offer:'));expect(offer).toBeTruthy();await push('',offer);
   await expect.poll(async()=>(await actions()).some(a=>a.startsWith('confirm:')),{timeout:15000}).toBe(true);const confirm=(await actions()).find(a=>a.startsWith('confirm:'));expect(confirm).toBeTruthy();expect(await requests()).toHaveLength(0);
   await push('',confirm);await expect.poll(async()=>(await requests()).length,{timeout:15000}).toBe(1);await push('',confirm);await expect.poll(async()=>(await requests()).length).toBe(1);const order=(await requests())[0];expect(order.status).toBe('pending_confirmation');
   const reservedBefore=(await stock()).quantity_reserved;expect(reservedBefore).toBe(channel==='telegram'?0:1);
   // A separate real staff session confirms Shop A; owners of B/C are also permitted employees.
   const staff=index===0?await login(browser,fixture.staff):page;
   await staff.goto(`/dashboard/orders/${order.id}?org=${shop.id}`);await staff.getByRole('button',{name:'Подтвердить и зарезервировать',exact:true}).click();await staff.getByRole("dialog").getByRole("button",{name:"Подтвердить и зарезервировать",exact:true}).click();await expect(staff.getByText(/Подтверждена · резерв выполнен/)).toBeVisible();expect((await stock()).quantity_reserved).toBe(reservedBefore+1);
   if(index===0){orderA=order.id;convA=conv.id;await staff.context().close();}
   await expect.poll(async()=>{const s=await state();return channel==='telegram'?s.integration.pending_inbox:s.inbox_pending},{timeout:90000}).toBe(0);
   // Conversation page is actual current SSR. Handoff stops further auto replies.
   await page.goto(`/dashboard/conversations?org=${shop.id}&channel=${channel}&chat=${conv.id}`);await expect(page.getByRole('navigation',{name:'Фильтр каналов'})).toBeVisible();await expect(page.locator('.chat-history .assistant')).not.toHaveCount(0);await page.getByRole('button',{name:'Передать сотруднику',exact:true}).click();await expect(page.getByText(/Передано сотруднику — автоответы остановлены/)).toBeVisible();
   const before=await page.locator('.chat-history .assistant').count();await page.getByLabel('Сообщение',{exact:true}).fill('Synthetic staff '+channel);await page.getByRole('button',{name:'Отправить сообщение',exact:true}).click();await expect(page.getByText('Synthetic staff '+channel,{exact:true})).toBeVisible();
   await page.goto(`/dashboard/integrations/${channel}?org=${shop.id}`);await push('P8D после handoff');await expect.poll(async()=>(await deliveries()).some(d=>d.payload.text==='Synthetic staff '+channel&&d.status==='sent'),{timeout:15000}).toBe(true);
   await page.goto(`/dashboard/conversations?org=${shop.id}&channel=${channel}&chat=${conv.id}`);await expect(page.locator('.chat-history .assistant')).toHaveCount(before);
   if(channel==='whatsapp'){
    await page.goto(`/dashboard/integrations/whatsapp?org=${shop.id}`);const s=await state();expect(s.live_enabled).toBe(false);expect(s.monetary_budget).toBe(0);
    const sent=(await deliveries()).find(d=>d.payload.text==='Synthetic staff whatsapp');expect(sent?.provider_id).toBeTruthy();
    const raw=JSON.stringify({object:'whatsapp_business_account',entry:[{id:'200',changes:[{field:'messages',value:{metadata:{phone_number_id:String(runBase+100+index)},statuses:[{id:sent!.provider_id,recipient_id:'700',timestamp:String(Math.floor(Date.now()/1000)),status:'read'}]}}]}]});
    const path='http://api:8080/api/v1/webhooks/whatsapp/'+integration;expect((await page.request.post(path,{headers:{'X-Hub-Signature-256':'sha256=wrong','Content-Type':'application/json'},data:raw})).status()).toBe(403);
    const signature='sha256='+createHmac('sha256','mock:synthetic-app-secret').update(raw).digest('hex');expect((await page.request.post(path,{headers:{'X-Hub-Signature-256':signature,'Content-Type':'application/json'},data:raw})).status()).toBe(200);
    await expect.poll(async()=>(await deliveries()).find(d=>d.id===sent!.id)?.status,{timeout:15000}).toBe('read');await page.reload();if(await page.getByRole('heading',{name:'WhatsApp недоступен',exact:true}).count()){await page.waitForTimeout(61000);await page.reload()};await expect(page.getByRole('heading',{name:'Доставка: read',exact:true})).toBeVisible();
    execFileSync(bin,['expire-window',conv.id,shop.owner.id]);await expect.poll(async()=>(await deliveries()).some(d=>d.status==='needs_approved_template'),{timeout:15000}).toBe(true);await page.reload();if(await page.getByRole('heading',{name:'WhatsApp недоступен',exact:true}).count()){await page.waitForTimeout(61000);await page.reload()};await expect(page.getByRole('alert').filter({hasText:'Окно закрыто.'})).toBeVisible();
    await page.goto(`/dashboard/conversations?org=${shop.id}&channel=whatsapp&chat=${conv.id}`);await expect(page.getByText(/Вне окна сервер блокирует ответ/)).toBeVisible();await page.getByLabel('Сообщение',{exact:true}).fill('Forbidden outside window');await page.getByRole('button',{name:'Отправить сообщение',exact:true}).click();await expect(page.locator('main').getByRole('alert')).toBeVisible();
   }
   await page.goto(`/dashboard/integrations/${channel}?org=${shop.id}`);page.once('dialog',d=>d.accept());await page.getByRole('button',{name:channel==='telegram'?'Отключить':'Отключить канал',exact:true}).click();await expect.poll(async()=>(await state()).integration.active,{timeout:90000}).toBe(false);
   const denied=await post(page,root+`/integrations/${channel}/synthetic-updates`,channel==='whatsapp'?{user_id:'700',message_id:'disabled',text:'P8D'}:{update_id:99999,message:{message_id:99999,from:{id:700},chat:{id:700,type:'private'},text:'P8D'}});expect(denied.status()).toBe(404);
   if(channel==='whatsapp'){
    const fixtureResult=JSON.parse(execFileSync(bin,['wa-operator',shop.id,fixture.operator.id],{encoding:'utf8'})) as {reservation:string};
    const op=await login(browser,fixture.operator);await op.goto(`/dashboard/operations/whatsapp?org=${shop.id}`);
    const region=op.getByRole('region',{name:'Сверка WhatsApp'});await expect(region.getByText(/delivery_unknown · резерв 0 RUB/)).toBeVisible();
    for(const choice of ['INSUFFICIENT','CONFIRMED_NOT_SENT']){
     await region.getByRole('button',{name:`Сверить ${fixtureResult.reservation}`,exact:true}).click();
     await region.getByRole('combobox').selectOption(choice);
     await expect(region.getByRole('button',{name:'Сохранить решение сверки'})).toBeDisabled();
     await region.getByLabel('Ссылка на основание').fill(`synthetic:BROWSER_9C_${index}`);
     await region.getByLabel('Подтверждаю основание и финансовое решение').check();
     await region.getByRole('button',{name:'Сохранить решение сверки'}).click();
     await expect(region.getByRole('status')).toHaveText(`Результат: ${choice==='INSUFFICIENT'?'INSUFFICIENT':'COMPLETED'}`);
     await expect(region.getByText(`Доказательств: ${choice==='INSUFFICIENT'?1:2}`,{exact:true})).toBeVisible();
    }
    await expect(region.getByText('Доказательств: 2',{exact:true})).toBeVisible();
    const final=await (await op.request.get(root+'/integrations/whatsapp/operator')).json();expect(final.items.find((x:{id:string})=>x.id===fixtureResult.reservation).delivery_state).toBe('delivery_unknown');
    const unassigned=fixture.shops[(index+1)%3];expect((await op.request.get(`/api/v1/organizations/${unassigned.id}/integrations/whatsapp/operator`)).status()).toBe(index===2?200:403);
    await op.screenshot({path:info.outputPath(`shop-${index}-operator.png`),fullPage:true});await op.context().close();
   }

  }
  await page.goto(`/dashboard/ai-settings?org=${shop.id}`);await page.getByText('Подключение и расходы',{exact:true}).click();await expect(page.getByText(/Общий лимит:/)).toContainText('0 RUB');await expect(page.getByText(/Провайдер: NOT SELECTED/)).toBeVisible();
  const other=fixture.shops[(index+1)%3];expect((await page.request.get(`/api/v1/organizations/${other.id}/products`)).status()).toBe(404);expect((await page.request.get(`/api/v1/organizations/${other.id}/integrations/whatsapp`)).status()).toBe(404);expect((await page.request.get(`/api/v1/organizations/${other.id}/integrations/whatsapp/financial`)).status()).toBe(404);expect((await page.request.get(`/api/v1/organizations/${other.id}/integrations/whatsapp/templates`)).status()).toBe(404);
  await page.screenshot({path:info.outputPath(`shop-${index}-budgets.png`),fullPage:true});
 }
 const member=await login(browser,fixture.member);const org=fixture.shops[0].id;expect((await member.request.get(`/api/v1/organizations/${org}/integrations/whatsapp/financial`)).status()).toBe(403);expect((await post(member,`/api/v1/organizations/${org}/categories`,{name:'Not allowed'})).status()).toBe(403);
 expect((await post(member,`/api/v1/organizations/${org}/orders/${orderA}/cancel`,{expected_version:2,idempotency_key:crypto.randomUUID()})).status()).toBe(403);
 expect((await member.request.get(`/api/v1/organizations/${fixture.shops[1].id}/conversations/${convA}`)).status()).toBe(404);
 await member.goto(`/dashboard/orders?org=${org}`);await expect(member.getByRole('heading',{name:'Недостаточно прав',exact:true})).toBeVisible();
 await member.context().close();for(const p of owners)await p.context().close();
});
