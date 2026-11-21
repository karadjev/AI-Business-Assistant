/* Synthetic server presentation branches, not backend integration. */
import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import {fileURLToPath} from "node:url";
import ts from "typescript";
import React from "react";
import {renderToStaticMarkup} from "react-dom/server";
import * as runtime from "react/jsx-runtime";
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),"../src");
function load(file,imports){const m={exports:{}};const code=ts.transpileModule(fs.readFileSync(path.join(root,file),'utf8'),{compilerOptions:{module:ts.ModuleKind.CommonJS,jsx:ts.JsxEmit.ReactJSX}}).outputText;new Function('require','module','exports',code)(name=>{if(name in imports)return imports[name];if(name==='react/jsx-runtime')return runtime;throw Error('Unapproved test import '+name)},m,m.exports);return m.exports;}
const Link=({href,children,...props})=>{delete props.prefetch;return React.createElement('a',{href,...props},children)};
const ui=load('components/ProductUI.tsx',{'next/link':{default:Link}});
const org={id:'11111111-1111-4111-8111-111111111111',name:'Synthetic test store',role:'owner',is_test:true,can_manage_orders:true};
async function render({pending=[],fail=false,setup='ACTIVE',role='owner',isTest=true}={}){
 const source=load('app/dashboard/page.tsx',{
 'next/link':{default:Link},'@/lib/session':{organizations:async()=>[{...org,role,is_test:isTest}],request:async p=>{if(fail)throw Error('Synthetic read failure');return {ok:true,json:async()=>p.includes('ai-settings')?{conversations:4,orders:4,mode:'mock',settings:{auto_enabled:false}}:p.includes('catalog-control')?{health_score:100,metrics:{review:0,missing_price:0,incomplete:0,import_errors:0,products:7}}:p.includes('onboarding')?{status:setup,catalog_count:7}:{items:pending}};}},
 '@/lib/catalog':{money:amount=>amount/100+' ₽'},'@/components/ProductUI':ui,'@/components/ReloadSection':{ReloadSection:({label})=>React.createElement('button',null,label)},
 });return renderToStaticMarkup(await source.default({searchParams:Promise.resolve({org:org.id})}));
}
test('healthy empty attention is calm; all-time orders never become pending tasks',async()=>{const html=await render({pending:[{id:'processed',status:'confirmed'}]});assert.match(html,/Сейчас всё спокойно/);assert.match(html,/Все заказы/);assert.match(html,/Все статусы · без ограничения по дате/);assert.doesNotMatch(html,/Заказ №processed/);assert.doesNotMatch(html,/Продолжить настройку/)});
test('failed reads remain unknown, offer retry and never a false calm/zero',async()=>{const html=await render({fail:true});assert.match(html,/Не удалось загрузить данные/);assert.match(html,/Повторить/);assert.match(html,/Нет данных/);assert.doesNotMatch(html,/Сейчас всё спокойно/)});
test('only pending rows appear first, filtered navigation and tenant remain intact',async()=>{const html=await render({pending:[{id:'waiting',status:'pending_confirmation',total_amount:100000},{id:'processed',status:'completed'}],setup:'NOT_STARTED'});assert.match(html,/Заказ №waiting/);assert.doesNotMatch(html,/Заказ №processed/);assert.match(html,/status=pending_confirmation/);assert.ok(html.indexOf('Требуют внимания')<html.indexOf('Все диалоги'));assert.match(html,/Продолжить настройку/);assert.match(html,/Начало настройки/);assert.match(html,/org=11111111-1111-4111-8111-111111111111/)});
test('ordinary store and read-only member are not promised order processing or fake completion',async()=>{for(const options of [{isTest:false},{role:'member',isTest:false}]){const html=await render(options);assert.doesNotMatch(html,/Сейчас всё спокойно/);assert.doesNotMatch(html,/Тестовый магазин/);assert.match(html,/Обычный магазин/);assert.match(html,/Задачи по заказам доступны/);}});
