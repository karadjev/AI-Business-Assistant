import {test,expect} from "@playwright/test";
import {execFileSync} from "node:child_process";

// Must run through the existing current-build guard and isolated synthetic DB.
test("Instagram unavailable state and mobile layout",async({page},info)=>{
 const bin=process.env.ABA_BROWSER_FIXTURE_BIN;
 test.skip(!bin,"Requires isolated fixture binary and current production build guard");
 if(!bin)throw new Error("Fixture binary required");
 const fixture=JSON.parse(execFileSync(bin,["seed"],{encoding:"utf8"})) as {shops:{id:string;owner:{email:string;password:string}}[]};
 const shop=fixture.shops[0];
 await page.goto("/login");
 await page.getByLabel("Email",{exact:true}).fill(shop.owner.email);
 await page.getByLabel("Пароль",{exact:true}).fill(shop.owner.password);
 await page.getByRole("button",{name:"Войти",exact:true}).click();
 await expect(page).toHaveURL(/\/dashboard$/);
 await page.goto(`/dashboard/integrations?org=${shop.id}`);
 await expect(page.getByRole("heading",{name:"Instagram",exact:true})).toBeVisible();
 await page.getByRole("link",{name:/Настройки Instagram/}).click();
 await expect(page.getByText("Не подключён",{exact:true}).first()).toBeVisible();
 await expect(page.getByRole("button",{name:"Подключить MOCK Instagram",exact:true})).toBeDisabled();
 await expect(page.getByText("Не подтверждена",{exact:true})).toBeVisible();
 await expect(page.getByText("Не предоставлен",{exact:true})).toBeVisible();
 await page.setViewportSize({width:390,height:844});
 await expect(page.getByRole("heading",{name:"Instagram",exact:true})).toBeVisible();
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBe(true);
 await page.screenshot({path:info.outputPath("instagram-mobile.png"),fullPage:true});
});
