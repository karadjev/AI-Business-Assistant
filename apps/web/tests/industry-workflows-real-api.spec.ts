import {readFileSync} from "node:fs";
import {expect, test, type Page} from "@playwright/test";

const origin = process.env.SMOKE_WEB_URL;
const serviceFixture = process.env.ABA_INDUSTRY_UI_SERVICE_FIXTURE;
const industryFixture = process.env.ABA_INDUSTRY_UI_FIXTURE;
type Shop = {id: string; conversation: string; owner: {email: string; password: string}};
type ServiceShop = Shop & {offering: string; resource: string; slots: {staff: {starts_at: string}}};

async function openWorkflow(page: Page, shop: Shop) {
  await page.goto("/login");
  await page.getByLabel("Email", {exact: true}).fill(shop.owner.email);
  await page.getByLabel("Пароль", {exact: true}).fill(shop.owner.password);
  await page.getByRole("button", {name: "Войти", exact: true}).click();
  await expect(page).toHaveURL(/\/dashboard$/);
  await page.goto(`/dashboard/ai-settings?org=${shop.id}`);
  const panel = page.getByRole("region", {name: "Отраслевые задачи"});
  await panel.getByRole("button", {name: "Загрузить открытые диалоги"}).click();
  return panel;
}

test("Auto Service creates an owner-reviewed reservation from the industry form", async ({page}) => {
  test.skip(!origin || !serviceFixture, "Requires the isolated Auto Service fixture");
  test.setTimeout(90_000);
  await page.setViewportSize({width: 390, height: 844});
  const shop = JSON.parse(readFileSync(serviceFixture!, "utf8")) as ServiceShop;
  const panel = await openWorkflow(page, shop);
  const csrf = (await page.context().cookies()).find(item => item.name === "aba_csrf")!.value;
  const path = `/api/v1/organizations/${shop.id}/operations/services`;
  const headers = {"X-CSRF-Token": csrf, Origin: origin!};
  const search = await page.request.post(path + "/search", {headers, data: {query: "Synthetic26", limit: 5}});
  expect(search.status()).toBe(200);
  const offering = (await search.json() as Array<{id: string}>).find(item => item.id === shop.offering);
  expect(offering).toBeDefined();
  const configured = await page.request.post(path + "/config/offerings", {headers, data: {
    ...offering, resource_ids: [shop.resource],
    part_rule: {state: "REQUIRED", oem: "SYN-262-PART", quantity: 1, model: "model25", year: 2025},
  }});
  expect(configured.status()).toBe(200);

  await panel.getByLabel("Название услуги").fill("Synthetic26");
  await panel.getByRole("button", {name: "Найти услугу"}).click();
  await expect(panel.getByRole("combobox", {name: "Услуга", exact: true})).toHaveValue(shop.offering);
  await panel.getByLabel("Диалог").selectOption(shop.conversation);
  await panel.getByLabel("Марка").fill("synthetic");
  await panel.getByLabel("Модель", {exact: true}).fill("model25");
  await panel.getByLabel("Двигатель").fill("e25");
  await panel.getByLabel("Год выпуска").fill("2025");
  const localSlot = await page.evaluate(value => {
    const at = new Date(value);
    const two = (n: number) => String(n).padStart(2, "0");
    return `${at.getFullYear()}-${two(at.getMonth() + 1)}-${two(at.getDate())}T${two(at.getHours())}:${two(at.getMinutes())}`;
  }, shop.slots.staff.starts_at);
  await panel.getByLabel("Начало в местном времени устройства").fill(localSlot);
  await panel.getByRole("button", {name: "Создать отраслевую задачу"}).click();
  await expect(panel.getByText("Предложение готово. Решение принимает сотрудник.")).toBeVisible({timeout: 35_000});
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  const taskLink = await panel.getByRole("link", {name: "Открыть задачу в истории"}).getAttribute("href");
  expect(taskLink).toMatch(/^#agent-task-/);

  await page.reload();
  const task = page.locator(taskLink!);
  await task.locator("summary").click();
  await task.getByRole("button", {name: "Подготовить заявку к согласованию"}).click();
  await task.getByRole("link", {name: "Открыть заявку для подтверждения"}).click();
  await expect(page).toHaveURL(/\/dashboard\/orders\//);
  await page.getByLabel("Причина действия").fill("Synthetic stage 3 approval");
  const confirm = "Подтвердить и зарезервировать";
  await page.getByRole("button", {name: confirm, exact: true}).click();
  await page.getByRole("dialog").getByRole("button", {name: confirm, exact: true}).click();
  await expect(page.getByText("Подтверждён сотрудником")).toBeVisible({timeout: 20_000});
  const root = `/api/v1/organizations/${shop.id}`;
  const taskID = taskLink!.slice("#agent-task-".length);
  await expect.poll(async () => {
    const response = await page.request.get(root + "/agent-tasks");
    const {items} = await response.json() as {items: Array<{parent_id?: string; action: string; result: unknown}>};
    const receipt = items.find(row => row.parent_id === taskID && row.action === "delegation.service_receipt");
    return JSON.stringify(receipt?.result ?? null);
  }, {timeout: 30_000}).toContain('"reservation_status":"active"');
});

for (const [scenario, kind, result] of [
  ["construction", "building_materials", "Упаковок: 6"],
  ["auto-parts", "auto_parts", "Synthetic25 auto_parts"],
  ["real-estate", "real_estate", "Synthetic25 current listing"],
] as const) {
  test(`${scenario} industry form renders a persisted worker result`, async ({page}) => {
    test.skip(!origin || !industryFixture, "Requires isolated industry fixtures");
    test.setTimeout(65_000);
    const shops = JSON.parse(readFileSync(industryFixture!, "utf8")) as Record<string, Shop>;
    const shop = shops[kind];
    const panel = await openWorkflow(page, shop);
    await panel.getByLabel("Сценарий").selectOption(scenario);
    await panel.getByLabel("Диалог").selectOption(shop.conversation);
    if (scenario === "construction") {
      await panel.getByLabel("Материал по SKU").fill("SYN25");
      await panel.getByLabel("Длина поверхности, м").fill("9");
      await panel.getByLabel("Ширина поверхности, м").fill("5");
      await panel.getByLabel("Длина одной единицы, м").fill("1");
      await panel.getByLabel("Ширина одной единицы, м").fill("1");
    } else if (scenario === "auto-parts") {
      await panel.getByLabel("Марка").fill("synthetic");
      await panel.getByLabel("Модель").fill("model25");
      await panel.getByLabel("Двигатель").fill("e25");
      await panel.getByLabel("Год выпуска").fill("2025");
      await panel.getByLabel("Артикул или OEM").fill("SYN-25-OEM");
    } else {
      await panel.getByLabel("Город").fill("Synthetic city");
      await panel.getByLabel("Тип объекта").selectOption("apartment");
      await panel.getByLabel("Сделка").selectOption("sale");
      await panel.getByLabel("Бюджет до, ₽").fill("100000");
    }
    await panel.getByRole("button", {name: "Создать отраслевую задачу"}).click();
    await expect(panel.getByText(result, {exact: scenario !== "auto-parts"})).toBeVisible({timeout: 35_000});
  });
}
