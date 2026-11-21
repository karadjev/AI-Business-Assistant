import {randomUUID} from "node:crypto";
import {existsSync, readFileSync, writeFileSync} from "node:fs";
import {expect, test} from "@playwright/test";

const fixture = process.env.ABA_REAL_ESTATE_FIXTURE;
const markers = process.env.ABA_REAL_ESTATE_MARKERS;
const origin = process.env.SMOKE_WEB_URL;
type Task = {id: string; parent_id?: string; correlation_id: string; status: string; action: string; result?: unknown};

test("Real Estate collects requirements and restores the viewing decision after worker restart", async ({page}) => {
  test.skip(!fixture || !markers || !origin, "Requires the isolated Real Estate runner");
  test.setTimeout(90_000);
  const shop = JSON.parse(readFileSync(fixture!, "utf8")).real_estate as {
    id: string; conversation: string; listing: string; offering: string; starts_at: string; ends_at: string;
    owner: {email: string; password: string};
  };
  await page.goto("/login");
  await page.getByLabel("Email", {exact: true}).fill(shop.owner.email);
  await page.getByLabel("Пароль", {exact: true}).fill(shop.owner.password);
  await page.getByRole("button", {name: "Войти", exact: true}).click();
  await expect(page).toHaveURL(/\/dashboard$/);
  const csrf = (await page.context().cookies(origin)).find(cookie => cookie.name === "aba_csrf")!.value;
  const root = `/api/v1/organizations/${shop.id}`;
  const post = async <T>(path: string, data: unknown, status: number) => {
    const response = await page.request.post(root + path, {headers: {Origin: origin!, "X-CSRF-Token": csrf}, data});
    expect(response.status(), await response.text()).toBe(status);
    return response.json() as Promise<T>;
  };
  const get = async <T>(path: string) => {
    const response = await page.request.get(root + path);
    expect(response.status(), await response.text()).toBe(200);
    return response.json() as Promise<T>;
  };
  const tasks = async () => (await get<{items: Task[]}>("/agent-tasks")).items;
  const outcome = async (id: string) => {
    await expect.poll(async () => (await tasks()).find(task => task.id === id)?.status, {timeout: 30_000}).toBe("completed");
    return (await tasks()).find(task => task.id === id)?.result;
  };
  const conversation_id = shop.conversation;
  const initial = await post<{id: string}>("/agent-delegations/real-estate", {
    conversation_id, idempotency_key: randomUUID(),
    requirements: {city: "Synthetic city", district: "Center", property_type: "apartment", rooms: 2},
  }, 202);
  expect(await outcome(initial.id)).toMatchObject([{state: "clarification_required", required: ["transaction_type", "currency_code", "max_price_amount"]}]);
  const search = await post<{id: string}>("/agent-delegations/real-estate", {
    conversation_id, idempotency_key: randomUUID(), previous_task_id: initial.id,
    requirements: {transaction_type: "sale", currency_code: "RUB", max_price_amount: 10000000},
  }, 202);
  expect(await outcome(search.id)).toMatchObject([{state: "proposal_ready", properties: [
    {id: shop.listing, city: "Synthetic city", rooms: 2, price_amount: 9500000, status: "available"},
  ]}]);
  const offerings = await post<{id: string; revision: number}[]>("/operations/services/search", {query: "Synthetic25 inspection", limit: 10}, 200);
  const offering = offerings.find(candidate => candidate.id === shop.offering);
  expect(offering).toBeDefined();
  const input = {
    conversation_id, idempotency_key: randomUUID(), offer_task_id: search.id, property_id: shop.listing,
    selection: {offering_id: shop.offering, offering_revision: offering!.revision, starts_at: shop.starts_at, ends_at: shop.ends_at,
      timezone: "UTC", units: 1}, comment: "Synthetic browser viewing",
  };
  const viewing = await post<{id: string}>("/agent-delegations/property-viewing", input, 202);
  expect(await post("/agent-delegations/property-viewing", input, 202)).toEqual(viewing);
  expect(await outcome(viewing.id)).toMatchObject([{state: "approval_required"}]);
  const request = await post<{id: string; version: number}>(`/agent-delegations/property-viewing/${viewing.id}/prepare`, null, 201);
  const requestPath = `/operations/property-viewings/${request.id}`;
  expect(await get(requestPath)).toMatchObject({status: "pending", approval_status: "pending", property_id: shop.listing});
  writeFileSync(`${markers}/stop`, viewing.id);
  await expect.poll(() => existsSync(`${markers}/stopped`), {timeout: 20_000}).toBe(true);
  const orderPath = `/orders/${request.id}`;
  await post(orderPath + "/confirm", {idempotency_key: randomUUID(), expected_version: request.version}, 200);
  expect(await get(requestPath)).toMatchObject({status: "approved", approval_status: "approved",
    assigned_employees: [{name: "Synthetic25 resource"}]});
  expect((await tasks()).filter(task => task.parent_id === viewing.id && task.action === "delegation.property_receipt"))
    .toContainEqual(expect.objectContaining({status: "queued", correlation_id: viewing.id}));
  writeFileSync(`${markers}/resume`, viewing.id);
  await expect.poll(async () => (await tasks()).filter(task => task.parent_id === viewing.id &&
    task.action === "delegation.property_receipt" && task.status === "completed").length, {timeout: 30_000}).toBe(2);
  expect((await tasks()).filter(task => task.parent_id === viewing.id && task.action === "delegation.property_receipt")
    .some(task => JSON.stringify(task.result).includes("Просмотр подтверждён"))).toBe(true);
  await post(orderPath + "/cancel", {idempotency_key: randomUUID(), expected_version: request.version + 1}, 200);
  expect(await get(requestPath)).toMatchObject({status: "cancelled"});
});
