import {randomUUID} from "node:crypto";
import {expect, test, type APIRequestContext, type Page} from "@playwright/test";

const origin = process.env.SMOKE_WEB_URL;
const mailpit = process.env.TEST_MAILPIT_URL;
const enabled = process.env.ABA_FLAGSHIP_E2E === "1"
  && Boolean(origin?.startsWith("http://127.0.0.1:"))
  && Boolean(mailpit?.startsWith("http://127.0.0.1:"));

async function mailLink(request:APIRequestContext, email:string, route:string) {
  let link = "";
  await expect.poll(async () => {
    const response = await request.get(`${mailpit}/api/v1/messages`);
    const {messages} = await response.json() as {messages:{ID:string;To:{Address:string}[]}[]};
    const message = messages.find(item => item.To.some(recipient => recipient.Address === email));
    if (!message) return false;
    const detail = await (await request.get(`${mailpit}/api/v1/message/${message.ID}`)).json() as {Text:string; HTML:string};
    const localLink = detail.Text.match(/http:\/\/127\.0\.0\.1:\d+\/[a-z-]+#token=[A-Za-z0-9_-]{43}/)?.[0];
    if (!localLink || !localLink.startsWith(`${origin}/${route}#token=`)) return false;
    if (!detail.HTML?.includes(localLink)) throw new Error("HTML mail does not contain the direct identity link");
    link = localLink;
    return true;
  }, {timeout:20_000}).toBe(true);
  return link;
}

async function login(page:Page, email:string, password:string) {
  await page.goto("/login");
  await page.getByLabel("Email",{exact:true}).fill(email);
  await page.getByLabel("Пароль",{exact:true}).fill(password);
  await page.getByRole("button",{name:"Войти",exact:true}).click();
  await expect(page).toHaveURL(/\/dashboard$/);
}

test("flagship onboarding, approval, saved result and password recovery", async ({page,request},testInfo) => {
  test.skip(!enabled,"Requires the isolated Flagship web, API, worker, PostgreSQL and Mailpit preview");
  test.setTimeout(120_000);
  const email = `flagship-${randomUUID()}@example.test`;
  const password = `Synthetic-start-${randomUUID()}`;
  const replacement = `Synthetic-reset-${randomUUID()}`;

  await page.goto("/register");
  await page.getByLabel("Email",{exact:true}).fill(email);
  await page.getByLabel("Пароль",{exact:true}).fill(password);
  await page.getByRole("button",{name:"Показать",exact:true}).click();
  await expect(page.getByLabel("Пароль",{exact:true})).toHaveAttribute("type","text");
  await page.getByRole("button",{name:"Скрыть",exact:true}).click();
  await page.getByRole("button",{name:"Создать аккаунт",exact:true}).click();
  await expect(page.getByRole("heading",{name:"Проверьте почту"})).toBeFocused();
  await expect(page.getByText(email,{exact:true})).toBeVisible();
  const verification = await mailLink(request,email,"verify-email");
  await page.goto(verification);
  await expect(page).toHaveURL(/\/verify-email$/);
  await page.getByRole("button",{name:"Подтвердить email",exact:true}).click();
  await expect(page.getByRole("heading",{name:"Email подтверждён"})).toBeFocused();
  await page.goto(verification);
  await page.getByRole("button",{name:"Подтвердить email",exact:true}).click();
  await expect(page.locator("main").getByRole("alert")).toContainText("истекла или уже использована");

  await login(page,email,password);
  const session = (await page.context().cookies()).find(cookie => cookie.name === "aba_session");
  expect(session?.httpOnly).toBe(true);
  expect(session?.sameSite).toBe("Lax");
  await page.locator("main").getByRole("link",{name:"Создать организацию",exact:true}).click();
  const organization = "Север · тестовая организация";
  await page.getByLabel("Название организации").fill(organization);
  await page.getByRole("button",{name:"Создать и выбрать отрасль"}).click();
  await expect(page.getByRole("heading",{name:"Выберите отрасль"})).toBeVisible();
  await page.getByLabel("Отрасль и профиль").selectOption("auto_parts");
  await page.getByRole("button",{name:"Сохранить профиль и открыть AI-команду"}).click();
  await expect(page.getByRole("heading",{name:"Business Manager",level:1})).toBeVisible();
  const id = new URL(page.url()).searchParams.get("org");
  expect(id).toBeTruthy();
  const root = `/api/v1/organizations/${id}`;
  const list = await (await page.request.get("/api/v1/organizations")).json();
  expect(list.items).toHaveLength(1);
  expect(list.items[0]).toMatchObject({id,name:organization,is_test:true});

  const denied = await page.request.post(`${root}/agent-tasks`,{
    headers:{Origin:origin!},
    data:{agent_id:"business_manager",action:"coordinate",idempotency_key:randomUUID()},
  });
  expect(denied.status()).toBe(403);
  const foreign = await page.request.get(`/api/v1/organizations/${randomUUID()}/agent-team`);
  expect([403,404]).toContain(foreign.status());

  await page.getByTestId("agent-task-operation").selectOption("coordinate_growth");
  await page.getByTestId("agent-task-create").click();
  await expect(page.getByTestId("agent-task-submission")).toContainText("Задача принята");
  const approvals = page.getByTestId("agent-approvals");
  await expect(approvals.getByText("После подтверждения агент подготовит черновик.",{exact:false})).toBeVisible();
  await approvals.getByText("Детали запроса",{exact:true}).click();
  await approvals.getByRole("button",{name:"Подтвердить",exact:true}).click();
  await expect(approvals.getByRole("status")).toHaveText("Действие подтверждено.");
  const result = page.getByRole("region",{name:"Маркетинговые черновики"});
  await expect(result.locator("details").first()).toBeVisible({timeout:30_000});
  await result.locator("summary").first().click();
  await expect(result.locator(".marketing-draft-content")).not.toBeEmpty();
  await result.getByRole("button",{name:"Согласовать черновик",exact:true}).click();
  await expect(result.getByRole("status")).toHaveText("Черновик согласован и сохранён в организации.");
  await page.reload();
  await expect(result.getByText("Согласован",{exact:true})).toBeVisible();
  await result.locator("summary").first().click();
  await result.screenshot({path:testInfo.outputPath("first-value.png")});
  const stored = await (await page.request.get(`${root}/marketing-drafts`)).json();
  expect(stored.items).toEqual(expect.arrayContaining([expect.objectContaining({status:"approved"})]));

  await page.setViewportSize({width:390,height:844});
  const nav = page.getByRole("navigation",{name:"Основные разделы"});
  await nav.getByRole("link",{name:"Обзор",exact:true}).click();
  await expect(page.getByRole("heading",{name:organization,exact:true})).toBeVisible();
  await page.keyboard.press("Control+k");
  const palette = page.getByRole("dialog",{name:"Быстрый переход"});
  await expect(palette.getByLabel("Название раздела")).toBeFocused();
  await palette.getByLabel("Название раздела").fill("Business");
  await palette.getByRole("link",{name:"Business Manager"}).click();
  await expect(page).toHaveURL(/\/dashboard\/command/);
  await expect(palette).not.toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);

  await page.locator(".profile-menu summary").click();
  await page.getByRole("button",{name:"Выйти",exact:true}).click();
  await page.goto("/forgot-password");
  await page.getByLabel("Email",{exact:true}).fill(email);
  await page.getByRole("button",{name:"Отправить ссылку",exact:true}).click();
  await expect(page.getByRole("status")).toContainText("письмо для восстановления");
  const reset = await mailLink(request,email,"reset-password");
  await page.goto(reset);
  await expect(page).toHaveURL(/\/reset-password$/);
  await page.getByLabel("Пароль",{exact:true}).fill(replacement);
  await page.getByRole("button",{name:"Сохранить пароль",exact:true}).click();
  await expect(page.getByRole("heading",{name:"Пароль изменён"})).toBeFocused();
  await login(page,email,replacement);
});
