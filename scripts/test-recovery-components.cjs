/* Offline-only browser checks of current source components and actual BFF code.
 * Uses installed TypeScript/Next-bundled webpack/Playwright; no Next build or download.
 * Does not substitute for a production Next build or PostgreSQL integration tests.
 */
const fs=require('node:fs/promises');
const http=require('node:http');
const path=require('node:path');
const {createRequire}=require('node:module');
const assert=require('node:assert/strict');
const requireWeb=createRequire('/apps/web/package.json');
const ts=requireWeb('typescript');
const {webpack}=requireWeb('next/dist/compiled/webpack/webpack.js');
const {chromium,expect}=requireWeb('@playwright/test');
(async()=>{
 const dir=await fs.mkdtemp('/tmp/aba-recovery-');
 const loader=path.join(dir,'loader.cjs');
 await fs.writeFile(loader,`module.exports=function(source){return require(${JSON.stringify(requireWeb.resolve('typescript'))}).transpileModule(source,{compilerOptions:{jsx:4,module:99,target:7}}).outputText}`);
 await fs.writeFile(path.join(dir,'entry.tsx'),`import React from 'react';import {createRoot} from 'react-dom/client';import {DeliveryRecovery} from '/apps/web/src/components/DeliveryRecovery';import {BudgetRecovery} from '/apps/web/src/components/BudgetRecovery';const p=new URLSearchParams(location.search);const id=p.get('org')??'11111111-1111-4111-8111-111111111111';createRoot(document.getElementById('root')!).render(<><DeliveryRecovery organizationId={id}/><BudgetRecovery organizationId={id} owner={p.get('owner')!=='false'}/></>);`);
 await new Promise((resolve,reject)=>{const compiler=webpack({mode:'development',entry:path.join(dir,'entry.tsx'),output:{path:dir,filename:'bundle.js'},resolve:{extensions:['.tsx','.ts','.js'],modules:['/apps/web/node_modules']},module:{rules:[{test:/\.tsx?$/,use:loader}]}});compiler.run((e,stats)=>{compiler.close(()=>{});if(e||stats.hasErrors())reject(e??Error(stats.toString()));else resolve()})});
 const bundle=await fs.readFile(path.join(dir,'bundle.js'));
 const server=http.createServer((req,res)=>{if(req.url==='/bundle.js'){res.setHeader('Content-Type','application/javascript; charset=utf-8');res.end(bundle)}else{res.setHeader('Content-Type','text/html; charset=utf-8');res.end('<div id="root"></div><script src="/bundle.js"></script>')}});
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
 const origin=`http://127.0.0.1:${server.address().port}`;
 const browser=await chromium.launch({headless:true});
 try{
  const context=await browser.newContext();await context.addCookies([{name:'aba_csrf',value:'synthetic-csrf',url:origin}]);
  const page=await context.newPage();page.on('requestfailed',r=>console.error('Harness resource:',new URL(r.url()).pathname,r.failure()?.errorText));page.on('pageerror',e=>console.error('Component runtime:',e.message));page.on('dialog',d=>d.accept());let deliveryError=true,conflict=true,resolved=false,budgetKnown=false;const posts=[];const reads=[];
  await page.route('**/*',async route=>{
   const r=route.request(),u=new URL(r.url());if(u.origin!==origin)return route.abort();
   if(!u.pathname.startsWith('/api/'))return route.continue();
   if(r.method()==='POST'){
    const body=r.postDataJSON();posts.push({path:u.pathname,body});assert.equal(r.headers()['x-csrf-token'],'synthetic-csrf');
    if(u.pathname.endsWith('/resolve')){assert.equal(body.confirm_resolution,true);assert.equal(body.decision,'abandon');assert.equal(body.expected_version,7);assert.match(body.idempotency_key,/^[a-f0-9-]{36}$/);if(conflict)return route.fulfill({status:409,json:{error:{message:'Доставка изменилась'}}});resolved=true;return route.fulfill({json:{id:'22222222-2222-4222-8222-222222222222',status:'cancelled',version:8}})}
    assert.equal(body.confirm_conservative_charge,true);budgetKnown=true;return route.fulfill({json:{reconciled:true}});
   }
   reads.push(u.pathname);
   if(u.pathname.endsWith('/deliveries')){
    if(deliveryError)return route.fulfill({status:403,json:{error:{message:'Операция запрещена'}}});
    return route.fulfill({json:{items:resolved?[]:[{id:'22222222-2222-4222-8222-222222222222',status:'uncertain',version:7,attempts:1,events:[{id:1,new_status:'uncertain',attempt:1,decision:null}]}]}});
   }
   return route.fulfill({json:{items:[{id:'33333333-3333-4333-8333-333333333333',status:budgetKnown?'uncertain':'pending',amount:20,charged:budgetKnown?20:null,events:[{id:1,status:'pending'}]}]}});
  });
  await page.goto(origin);await page.getByRole('button',{name:'Проверить неопределённые доставки'}).click();await expect(page.getByRole('alert')).toHaveText('Операция запрещена');
  deliveryError=false;await page.getByRole('button',{name:'Проверить неопределённые доставки'}).click();await expect(page.getByRole('heading',{name:'DELIVERY UNKNOWN',exact:true})).toBeVisible();
  await page.getByRole('button',{name:'Отказаться от дальнейшей доставки'}).click();await expect(page.getByRole('alert')).toHaveText('Доставка изменилась');
  conflict=false;await page.getByRole('button',{name:'Отказаться от дальнейшей доставки'}).click();await expect(page.getByRole('status')).toContainText('Повторной отправки не выполнялось');await expect(page.getByText('Неопределённых или ошибочных доставок нет.')).toBeVisible();
  await page.getByRole('button',{name:'Проверить бюджетные операции'}).click();await page.getByRole('button',{name:'Учесть старый резерв консервативно'}).click();await expect(page.getByText(/33333333.*UNKNOWN/)).toBeVisible();
  const second='44444444-4444-4444-8444-444444444444';await page.goto(origin+'/?org='+second+'&owner=false');await page.getByRole('button',{name:'Проверить бюджетные операции'}).click();await expect(page.getByRole('button',{name:'Учесть старый резерв консервативно'})).toHaveCount(0);assert.ok(reads.at(-1).includes(second));assert.equal(posts.length,3);assert.equal(posts[0].body.idempotency_key,posts[1].body.idempotency_key);
  console.log('PASS: current recovery components in real Chromium: 403/409, explicit consent, version/idempotency/CSRF, UNKNOWN, owner UI and tenant switch');
 }finally{await browser.close();await new Promise(resolve=>server.close(resolve))}
 // Execute the actual BFF route after TS transpilation, intercept fetch locally.
 const source=await fs.readFile('/apps/web/src/app/api/[...path]/route.ts','utf8');const module={exports:{}};
 new Function('require','module','exports',ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.CommonJS}}).outputText)(name=>{assert.equal(name,'@/lib/config');return {apiBaseURL:()=> 'http://api:8080'}},module,module.exports);
 const prior=global.fetch;let forwarded=0;
 global.fetch=async(url,options)=>{forwarded++;assert.ok(url.startsWith('http://api:8080/api/v1/organizations/'));assert.equal(options.headers.get('X-CSRF-Token'),'synthetic');assert.equal(options.headers.get('Cookie'),'aba_session=synthetic');return new Response('{}',{status:200})};
 try{
  const org='11111111-1111-4111-8111-111111111111';const root='v1/organizations/'+org;
  for(const suffix of ['/integrations/telegram/deliveries/22222222-2222-4222-8222-222222222222/resolve','/llm-reservations/33333333-3333-4333-8333-333333333333/recover','/integrations/whatsapp/control','/integrations/whatsapp/synthetic-updates']){
   const r=new Request('http://web/api/'+root+suffix,{method:'POST',headers:{Cookie:'aba_session=synthetic; other_secret=forbidden','X-CSRF-Token':'synthetic'},body:'{}'});
   const response=await module.exports.POST(r,{params:Promise.resolve({path:(root+suffix).split('/')})});assert.equal(response.status,200);
  }
  const denied=await module.exports.POST(new Request('http://web/api/x',{method:'POST'}),{params:Promise.resolve({path:[root,'arbitrary']})});assert.equal(denied.status,404);assert.equal(forwarded,4);
  console.log('PASS: current BFF path allowlist and restricted cookie/CSRF forwarding; no external fetch');
 }finally{global.fetch=prior;await fs.rm(dir,{recursive:true,force:true})}
})().catch(e=>{console.error(e);process.exit(1)});
