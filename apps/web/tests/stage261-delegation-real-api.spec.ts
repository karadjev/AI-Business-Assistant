import {randomUUID} from "node:crypto";
import {readFileSync, writeFileSync} from "node:fs";
import {expect, test} from "@playwright/test";

const fixture = process.env.ABA_DELEGATION_FIXTURE;
const marker = process.env.ABA_DELEGATION_RESTART_MARKER;
const origin = process.env.SMOKE_WEB_URL;
type Shop = {id: string; conversation: string; variant: string; owner: {email: string; password: string}};
type Task = {id: string; parent_id?: string; correlation_id: string; status: string; action: string; result?: unknown};

test("Auto Parts delegation survives worker restart and returns a verified proposal", async ({page}) => {
  test.skip(!fixture || !marker || !origin, "Requires the isolated delegation runner");
  test.setTimeout(90_000);
  const {auto_parts: shop, building_materials: other} = JSON.parse(readFileSync(fixture!, "utf8")) as Record<string, Shop>;
  await page.goto("/login");
  await page.getByLabel("Email", {exact: true}).fill(shop.owner.email);
  await page.getByLabel("Пароль", {exact: true}).fill(shop.owner.password);
  await page.getByRole("button", {name: "Войти", exact: true}).click();
  await expect(page).toHaveURL(/\/dashboard$/);
  const csrf = (await page.context().cookies(origin)).find(cookie => cookie.name === "aba_csrf")!.value;
  const root = `/api/v1/organizations/${shop.id}`;
  const get = async <T>(path: string) => {
    const response = await page.request.get(path);
    expect(response.status()).toBe(200);
    return response.json() as Promise<T>;
  };
  const path = `${root}/agent-delegations/auto-parts`;
  const requirements = {oem: "SYN-25-OEM", vehicle: {make: "synthetic", model: "model25", engine: "e25", year: 2025}, quantity: 2};
  const data = {conversation_id: shop.conversation, requirements, idempotency_key: randomUUID()};
  const headers = {"X-CSRF-Token": csrf, Origin: origin!};
  expect((await page.request.post(path, {headers: {Origin: origin!}, data})).status()).toBe(403);
  const submit = await page.request.post(path, {headers, data});
  expect(submit.status(), await submit.text()).toBe(202);
  const {id} = await submit.json() as {id: string};
  const tasks = async () => (await get<{items: Task[]}>(`${root}/agent-tasks`)).items;
  const initial = (await tasks()).filter(task => task.parent_id === id);
  expect(initial.map(task => task.status).sort()).toEqual(["pending", "pending", "pending", "pending", "queued"]);
  expect(initial.every(task => task.correlation_id === id)).toBe(true);
  expect((await tasks()).find(task => task.id === id)?.status).toBe("queued");
  const replay = await page.request.post(path, {headers, data});
  expect(replay.status()).toBe(202);
  expect(await replay.json()).toEqual({id});
  expect((await page.request.post(path, {headers, data: {...data, requirements: {...requirements, quantity: 3}}})).status()).toBe(409);
  expect((await page.request.post(path, {headers, data: {...data, idempotency_key: randomUUID(), conversation_id: other.conversation}})).status()).toBe(403);
  expect((await page.request.post(path, {headers, data: {...data, agent_id: "marketing"}})).status()).toBe(400);

  writeFileSync(marker!, id + "\n", {flag: "wx", mode: 0o600});
  await expect.poll(async () => (await tasks()).find(task => task.id === id)?.status, {timeout: 45_000}).toBe("completed");
  const result = (await tasks()).find(task => task.id === id)?.result;
  expect(result).toEqual([expect.objectContaining({
    state: "proposal_ready",
    proposal: expect.objectContaining({kind: "product_quote", estimated_total: {amount: 25000, currency: "RUB"},
      items: [{variant_id: shop.variant, quantity: 2}]}),
    children: [
      expect.objectContaining({agent: "customer", status: "completed"}),
      expect.objectContaining({agent: "sales", status: "completed"}),
      expect.objectContaining({action: "delegation.find_parts", status: "completed"}),
      expect.objectContaining({action: "delegation.verify_fitment", status: "completed",
        result: expect.objectContaining({parts: [expect.objectContaining({variant_id: shop.variant, available: 10, compatibility: "compatible", price_amount: 12500})]})}),
      expect.objectContaining({agent: "sales", status: "completed"}),
    ],
  })]);
  const audit = await get<{items: {action: string; result: string}[]}>(`${root}/agent-audit?task_id=${initial[0].id}`);
  expect(audit.items).toContainEqual(expect.objectContaining({action: "task.finish", result: "completed"}));
  expect(audit.items).toContainEqual(expect.objectContaining({action: "task.delegate"}));
  const orders = await get<{items: unknown[]}>(`${root}/orders`);
  expect(orders.items).toEqual([]);
  const quote = await page.request.post(`${root}/conversations/${shop.conversation}/quotes`, {
    headers, data: {items: [{variant_id: shop.variant, quantity: 2}]},
  });
  expect(quote.status(), await quote.text()).toBe(201);
  expect(await quote.json()).toMatchObject({reserved: false, total_amount: 25000});
  expect((await get<{items: unknown[]}>(`${root}/orders`)).items).toEqual([]);

  for (const [vehicle, state] of [
    [{...requirements.vehicle, engine: ""}, "clarification_required"],
    [{...requirements.vehicle, engine: "unverified"}, "no_verified_offer"],
  ] as const) {
    const alternate = await page.request.post(path, {
      headers, data: {...data, idempotency_key: randomUUID(), requirements: {...requirements, vehicle}},
    });
    expect(alternate.status(), await alternate.text()).toBe(202);
    const {id: alternateID} = await alternate.json() as {id: string};
    await expect.poll(async () => (await tasks()).find(task => task.id === alternateID)?.status, {timeout: 30_000})
      .toBe("completed");
    const outcome = (await tasks()).find(task => task.id === alternateID)?.result as {state: string; proposal?: unknown; required?: string[]}[];
    expect(outcome[0].state).toBe(state);
    expect(outcome[0].proposal).toBeUndefined();
    if (state === "clarification_required") expect(outcome[0].required).toContain("engine");
  }
});
