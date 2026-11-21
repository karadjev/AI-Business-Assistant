import {test,expect} from "@playwright/test";

test("главная страница и серверная защита кабинета",async({page},testInfo)=>{
 await page.goto("/");await expect(page.getByRole("heading",{level:1})).toContainText("Ближе к покупателю");
 await page.screenshot({path:testInfo.outputPath("home-desktop.png"),fullPage:true});
 for(const path of ["/dashboard/products","/dashboard/categories","/dashboard/inventory","/dashboard/products/new","/dashboard/orders","/dashboard/ai-playground"]){await page.goto(path);await expect(page).toHaveURL(/\/login$/);}
 await page.goto("/dashboard");await expect(page).toHaveURL(/\/login$/);await expect(page.getByRole("heading",{name:"Войти в кабинет"})).toBeVisible();
 await page.setViewportSize({width:390,height:844});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBeTruthy();
});
test("реальный backend status без подмены",async({page,request})=>{
 const api=process.env.SMOKE_API_URL??"http://127.0.0.1:8080";
 for(const path of ["/health/live","/api/v1/status"]){const response=await request.get(api+path);expect(response.status()).toBe(200);}
 const status=await request.get(api+"/api/v1/status");expect(await status.json()).toEqual({service:"ai-business-assistant",status:"running",api_version:"v1"});
 if(process.env.SMOKE_REQUIRE_READY==="1"){expect((await request.get(api+"/health/ready")).status()).toBe(200);}
 await page.goto("/status");await expect(page.getByRole("heading",{name:"Backend доступен"})).toBeVisible();
});
test("loading и недоступный backend не показывают успех",async({page})=>{
 await page.route("**/api/backend-status",async route=>{await new Promise(resolve=>setTimeout(resolve,400));await route.fulfill({status:503,contentType:"application/json",body:JSON.stringify({kind:"unavailable"})});});
 await page.goto("/status");await expect(page.getByRole("heading",{name:"Проверяем соединение…"})).toBeVisible();await expect(page.getByRole("heading",{name:"Backend недоступен"})).toBeVisible();await expect(page.getByRole("heading",{name:"Backend доступен",exact:true})).toHaveCount(0);
 await page.getByRole("button",{name:"Проверить снова"}).click();await expect(page.getByRole("heading",{name:"Backend недоступен"})).toBeVisible();
});
test("ошибка контракта не выглядит рабочим ответом",async({page})=>{
 await page.route("**/api/backend-status",route=>route.fulfill({status:200,contentType:"application/json",body:JSON.stringify({kind:"available",data:{service:"fake"}})}));await page.goto("/status");await expect(page.getByRole("heading",{name:"Ошибка ответа backend"})).toBeVisible();
});
