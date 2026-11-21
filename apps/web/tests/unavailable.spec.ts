import {test,expect} from "@playwright/test";
test("реальный остановленный backend отображается как недоступный",async({page,request})=>{
 test.skip(process.env.SMOKE_BACKEND_DOWN!=="1","Runs only after controlled backend shutdown");
 const response=await request.get("/api/backend-status");expect(response.status()).toBe(503);expect(await response.json()).toEqual({kind:"unavailable"});
 await page.goto("/status");await expect(page.getByRole("heading",{name:"Backend недоступен"})).toBeVisible();await expect(page.getByRole("heading",{name:"Backend доступен",exact:true})).toHaveCount(0);
});
