import {randomUUID} from "node:crypto";
import {readFileSync, writeFileSync} from "node:fs";
import {expect, test, type APIResponse, type Page} from "@playwright/test";

type User = {email: string; password: string};
type Fixture = {shops: {id: string; owner: User}[]; member: User; staff: User};
type Approval = {id: string; task_id: string; parameters_hash: string; status: string};
type Task = {id: string; status: string; result: unknown};
type Audit = {task_id: string; approval_id: string; action: string; result: string};
const fixtureFile = process.env.ABA_AGENT_E2E_FIXTURE;
const web = process.env.SMOKE_WEB_URL;
const root = (id: string) => `/api/v1/organizations/${id}`;

function fixture(): Fixture {
  return JSON.parse(readFileSync(fixtureFile!, "utf8")) as Fixture;
}

async function login(page: Page, user: User) {
  await page.goto("/login");
  await page.getByLabel("Email", {exact: true}).fill(user.email);
  await page.getByLabel("Пароль", {exact: true}).fill(user.password);
  await page.getByRole("button", {name: "Войти", exact: true}).click();
  await expect(page).toHaveURL(/\/dashboard$/);
}

async function expectStatus(response: APIResponse, status: number) {
  expect(response.status(), `${response.url()} returned ${await response.text()}`).toBe(status);
}

async function body<T>(response: APIResponse, status = 200): Promise<T> {
  await expectStatus(response, status);
  return response.json() as Promise<T>;
}

async function csrf(page: Page) {
  const cookie = (await page.context().cookies(web)).find(item => item.name === "aba_csrf");
  expect(cookie, "Go login should set a CSRF cookie").toBeDefined();
  return {"X-CSRF-Token": cookie!.value, Origin: web!};
}

async function submitDraft(page: Page, organization: string) {
  const response = await page.request.post(`${root(organization)}/agent-tasks`, {
    headers: await csrf(page),
    data: {agent_id: "marketing", action: "draft", idempotency_key: randomUUID()},
  });
  return (await body<{id: string}>(response, 202)).id;
}

async function approvals(page: Page, organization: string) {
  return (await body<{items: Approval[]}>(await page.request.get(`${root(organization)}/agent-approvals`))).items;
}

async function tasks(page: Page, organization: string) {
  return (await body<{items: Task[]}>(await page.request.get(`${root(organization)}/agent-tasks`))).items;
}

test.describe("Agent Team with the real Go API and PostgreSQL", () => {
  test.skip(!fixtureFile || !web, "Run the isolated Stage 2.4 real API runner");

  test("enforces approval CSRF and tenant roles through the BFF, then executes an approved draft", async ({page, browser}) => {
    test.setTimeout(45_000);
    const {shops: [shop, other], member} = fixture();
    const path = root(shop.id);
    await login(page, shop.owner);

    const task = await submitDraft(page, shop.id);
    const approval = (await approvals(page, shop.id)).find(item => item.task_id === task);
    expect(approval).toMatchObject({status: "pending", parameters_hash: expect.stringMatching(/^[a-f0-9]{64}$/)});
    expect((await tasks(page, shop.id)).find(item => item.id === task)?.status).toBe("waiting_approval");

    const decisionPath = `${path}/agent-approvals/${approval!.id}/decision`;
    const data = {decision: "approved", parameters_hash: approval!.parameters_hash};
    await expectStatus(await page.request.post(decisionPath, {headers: {Origin: web!}, data}), 403);
    const headers = await csrf(page);
    await expectStatus(await page.request.post(decisionPath, {
      headers, data: {...data, parameters_hash: "0".repeat(64)},
    }), 409);

    for (const {user, status} of [{user: member, status: 403}, {user: other.owner, status: 404}]) {
      const context = await browser.newContext({baseURL: web});
      try {
        const unauthorized = await context.newPage();
        await login(unauthorized, user);
        await expectStatus(await unauthorized.request.get(`${path}/agent-approvals`), status);
        await expectStatus(await unauthorized.request.post(decisionPath, {headers: await csrf(unauthorized), data}), status);
      } finally {
        await context.close();
      }
    }
    expect((await approvals(page, shop.id)).find(item => item.id === approval!.id)?.status).toBe("pending");
    await expectStatus(await page.request.post(decisionPath, {headers, data}), 204);
    await expectStatus(await page.request.post(decisionPath, {headers, data}), 409);

    await expect.poll(async () => (await tasks(page, shop.id)).find(item => item.id === task)?.status, {timeout: 12_000})
      .toBe("completed");
    expect((await tasks(page, shop.id)).find(item => item.id === task)?.result).toEqual([
      expect.objectContaining({draft: true, external_communication: false}),
    ]);

    const audit = await body<{items: Audit[]}>(await page.request.get(`${path}/agent-audit?task_id=${task}`));
    expect(audit.items.filter(item => item.action === "approval.decision")).toEqual([
      expect.objectContaining({task_id: task, approval_id: approval!.id, result: "approved"}),
    ]);
    expect(audit.items).toContainEqual(expect.objectContaining({task_id: task, action: "draft", result: "succeeded"}));
  });

  test("renders the team and carries a delegated approval through tasks and audit", async ({page, browser}) => {
    test.setTimeout(45_000);
    await page.setViewportSize({width: 390, height: 844});
    const {shops: [shop], staff} = fixture();
    const path = root(shop.id);
    const settings = `/dashboard/ai-settings?org=${shop.id}`;
    await login(page, shop.owner);
    const teamResponse = page.waitForResponse(response =>
      new URL(response.url()).pathname === `${path}/agent-team` && response.request().method() === "GET");
    await page.goto(settings);
    const response = await teamResponse;
    expect(response.status(), await response.text()).toBe(200);
    const team = page.getByRole("region", {name: "AI-команда"});
    await expect(team.getByText("trade", {exact: true})).toBeVisible();
    await expect(team.getByText("electronics", {exact: true})).toBeVisible();
    const cards = team.getByTestId(/^agent-card-/);
    await expect(cards).toHaveCount(7);
    const marketing = team.getByTestId("agent-card-marketing");
    await marketing.locator("summary").click();
    await expect(marketing.getByText("marketing.draft", {exact: true})).toBeVisible();

    await team.getByLabel("Действие агента").selectOption("business_manager");
    const [submitted] = await Promise.all([
      page.waitForResponse(candidate => new URL(candidate.url()).pathname === `${path}/agent-tasks`
        && candidate.request().method() === "POST"),
      team.getByRole("button", {name: "Создать задачу", exact: true}).click(),
    ]);
    expect(submitted.status(), await submitted.text()).toBe(202);
    const {id: manager} = await submitted.json() as {id: string};
    await expect(team.getByTestId("agent-task-submission")).toContainText(manager);

    const approvalRegion = team.getByRole("region", {name: "Требуют подтверждения"});
    const pending = approvalRegion.getByRole("listitem").filter({hasText: "draft"});
    await pending.getByText("Детали запроса", {exact: true}).click();
    await expect(pending.locator("pre")).toContainText('"action": "draft"');
    const [rejected] = await Promise.all([
      page.waitForResponse(candidate => new URL(candidate.url()).pathname.startsWith(`${path}/agent-approvals/`)
        && candidate.request().method() === "POST"),
      pending.getByRole("button", {name: "Отклонить", exact: true}).click(),
    ]);
    expect(rejected.status()).toBe(204);
    await expect(approvalRegion.getByRole("status")).toHaveText("Действие отклонено.");

    await expect.poll(async () => (await tasks(page, shop.id)).find(item => item.id === manager)?.status, {timeout: 12_000})
      .toBe("completed");
    await team.getByRole("button", {name: "Обновить AI-команду"}).click();
    const managerCard = team.getByTestId(`agent-task-${manager}`);
    await expect(managerCard.locator("summary")).toContainText("Завершена");
    await managerCard.locator("summary").click();
    const result = team.getByTestId(`agent-task-result-${manager}`);
    await expect(result).toBeVisible();
    expect(JSON.parse((await result.textContent())!)).toMatchObject({Children: [
      expect.objectContaining({agent_id: "analytics", status: "completed"}),
      expect.objectContaining({agent_id: "marketing", status: "cancelled"}),
    ]});
    const audit = team.getByRole("table", {name: "События AI-команды"});
    await expect(audit.getByRole("row").filter({hasText: "approval.decision"}).filter({hasText: "rejected"})).toHaveCount(1);

    const context = await browser.newContext({baseURL: web});
    try {
      const readOnly = await context.newPage();
      await login(readOnly, staff);
      await readOnly.goto(settings);
      const memberTeam = readOnly.getByRole("region", {name: "AI-команда"});
      await expect(memberTeam.getByText(/может владелец магазина/)).toBeVisible();
      await expect(memberTeam.getByRole("button", {name: "Создать задачу"})).toHaveCount(0);
      await expect(memberTeam.getByRole("region", {name: "Журнал действий"})).toHaveCount(0);
      await expectStatus(await readOnly.request.get(`${path}/agent-audit`), 403);
    } finally {
      await context.close();
    }
  });

  test("completes a browser task queued across a worker restart", async ({page}) => {
    test.setTimeout(60_000);
    const marker = process.env.ABA_AGENT_E2E_RESTART_MARKER;
    expect(marker, "The runner must stop the worker and configure its restart marker").toBeTruthy();
    const {shops: [, , shop]} = fixture();
    const path = root(shop.id);
    await login(page, shop.owner);
    await page.goto(`/dashboard/ai-settings?org=${shop.id}`);
    const team = page.getByRole("region", {name: "AI-команда"});
    await team.getByLabel("Действие агента").selectOption("analytics");
    const [submitted] = await Promise.all([
      page.waitForResponse(response =>
        new URL(response.url()).pathname === `${path}/agent-tasks` && response.request().method() === "POST"),
      team.getByRole("button", {name: "Создать задачу", exact: true}).click(),
    ]);
    expect(submitted.status(), await submitted.text()).toBe(202);
    const {id} = await submitted.json() as {id: string};
    const card = team.getByTestId(`agent-task-${id}`);
    await expect(card.locator("summary")).toContainText("В очереди");
    expect((await tasks(page, shop.id)).find(item => item.id === id)?.status).toBe("queued");
    writeFileSync(marker!, id + "\n", {flag: "wx", mode: 0o600});

    await expect.poll(async () => (await tasks(page, shop.id)).find(item => item.id === id)?.status, {timeout: 25_000})
      .toBe("completed");
    await team.getByRole("button", {name: "Обновить AI-команду"}).click();
    await expect(card.locator("summary")).toContainText("Завершена");
    const audit = team.getByRole("table", {name: "События AI-команды"});
    await expect(audit.getByRole("row").filter({hasText: id}).filter({hasText: "succeeded"})).toHaveCount(1);
  });
});
