import {randomUUID} from "node:crypto";
import {readFileSync, writeFileSync} from "node:fs";
import {expect, test, type Page} from "@playwright/test";

const origin = process.env.SMOKE_WEB_URL;
const constructionFixture = process.env.ABA_262_CONSTRUCTION_FIXTURE;
const serviceFixture = process.env.ABA_262_SERVICE_FIXTURE;
const marker = process.env.ABA_262_RESTART_MARKER;
type Shop = {id: string; conversation: string; owner: {email: string; password: string}; variant: string};
type Task = {id: string; parent_id?: string; status: string; result?: unknown[]};
type Row = {id: string; version: number; status: string; request_kind: string};

async function client(page: Page, shop: Shop) {
  await page.goto("/login");
  await page.getByLabel("Email", {exact: true}).fill(shop.owner.email);
  await page.getByLabel("Пароль", {exact: true}).fill(shop.owner.password);
  await page.getByRole("button", {name: "Войти", exact: true}).click();
  await expect(page).toHaveURL(/\/dashboard$/);
  const csrf = (await page.context().cookies(origin)).find(cookie => cookie.name === "aba_csrf")!.value;
  const root = `/api/v1/organizations/${shop.id}`;
  const post = async <T>(path: string, data: unknown, status: number) => {
    const response = await page.request.post(root + path, {data, headers: {"X-CSRF-Token": csrf, Origin: origin!}});
    expect(response.status(), await response.text()).toBe(status);
    return response.json() as Promise<T>;
  };
  const get = async <T>(path: string) => {
    const response = await page.request.get(root + path);
    expect(response.status(), await response.text()).toBe(200);
    return response.json() as Promise<T>;
  };
  const tasks = async () => (await get<{items: Task[]}>("/agent-tasks")).items;
  const submit = async (workflow: string, requirements: unknown, key = randomUUID()) =>
    post<{id: string}>(`/agent-delegations/${workflow}`, {conversation_id: shop.conversation, requirements, idempotency_key: key}, 202);
  const outcome = async (id: string) => {
    await expect.poll(async () => (await tasks()).find(task => task.id === id)?.status, {timeout: 30_000}).toBe("completed");
    return (await tasks()).find(task => task.id === id)!.result![0];
  };
  return {post, get, submit, outcome, tasks};
}

test("construction delegates a reproducible tenant quote after worker restart", async ({page}) => {
  test.skip(!origin || !constructionFixture || !marker, "Requires isolated construction runner");
  test.setTimeout(100_000);
  const {building_materials: shop, auto_parts: other} = JSON.parse(readFileSync(constructionFixture!, "utf8")) as Record<string, Shop>;
  const api = await client(page, shop);
  const dimensions = {operation: "tiles", length: {value: "9", unit: "m"}, width: {value: "5", unit: "m"},
    item_length: {value: "1", unit: "m"}, item_width: {value: "1", unit: "m"}, waste_percent: "0"};
  const requirements = {sku: "SYN25", room: "synthetic room", work: "floor tiles", calculation: dimensions};
  const key = randomUUID();
  const {id} = await api.submit("construction", requirements, key);
  expect(await api.submit("construction", requirements, key)).toEqual({id});
  const children = (await api.tasks()).filter(item => item.parent_id === id);
  expect(children.map(t => t.status).sort()).toEqual(["pending", "pending", "pending", "queued"]);
  await api.post("/agent-delegations/construction", {conversation_id: other.conversation, requirements, idempotency_key: randomUUID()}, 403);
  writeFileSync(marker!, id);
  expect(await api.outcome(id)).toMatchObject({
    state: "proposal_ready", calculation: {quantity: "45", package_count: 6, formula: expect.any(String)},
    material: {variant_id: shop.variant, manufacturer: "Synthetic25", price_amount: 12500, available: 10},
    proposal: {estimated_total: {amount: 75000, currency: "RUB"}, items: [{variant_id: shop.variant, quantity: 6}]},
    children: Array.from({length: 4}, () => expect.objectContaining({status: "completed"})),
  });
  const missing = await api.submit("construction", {sku: "SYN25", area: {value: "45", unit: "m2"}});
  expect(await api.outcome(missing.id)).toMatchObject({state: "clarification_required", required: ["dimensions"]});
  const unknown = await api.submit("construction", {...requirements, sku: "NO-SUCH-MATERIAL"});
  expect(await api.outcome(unknown.id)).toMatchObject({state: "material_unavailable"});
});

type Hybrid = Shop & {offering: string; foreign_conversation: string; variants: {staff: string};
  slots: {staff: {starts_at: string; ends_at: string}}};

test("auto service delegates slot and fitment then confirms one atomic booking", async ({page}) => {
  test.skip(!origin || !serviceFixture || !marker, "Requires isolated service runner");
  test.setTimeout(100_000);
  const shop = JSON.parse(readFileSync(serviceFixture!, "utf8")) as Hybrid;
  const api = await client(page, shop);
  const offerings = await api.post<{id: string; revision: number}[]>("/operations/services/search", {query: "Synthetic26", limit: 5}, 200);
  const offering = offerings.find(row => row.id === shop.offering)!;
  const selection = {...shop.slots.staff, offering_id: offering.id, offering_revision: offering.revision, timezone: "UTC", units: 1};
  const requirements = {vehicle: {make: "synthetic", model: "model25", year: 2025, engine: "e25"}, selection,
    part: {oem: "SYN-262-PART", quantity: 1}};
  const {id} = await api.submit("auto-service", requirements);
  expect((await api.tasks()).filter(item => item.parent_id === id).map(t => t.status).sort()).toEqual(["pending", "pending", "pending", "pending", "queued"]);
  await api.post("/agent-delegations/auto-service", {conversation_id: shop.foreign_conversation, requirements, idempotency_key: randomUUID()}, 403);
  writeFileSync(marker!, id);
  expect(await api.outcome(id)).toMatchObject({
    state: "approval_required",
    proposal: {kind: "hybrid_order", items: [{variant_id: shop.variants.staff, quantity: 1}], details: {selection, vehicle: requirements.vehicle}},
  });
  const orders = () => api.get<{items: Row[]}>("/orders");
  expect((await orders()).items).toHaveLength(0);
  const prepare = `/agent-delegations/auto-service/${id}/prepare`;
  const pending = await api.post<Row>(prepare, {}, 201);
  expect(pending.status).toBe("pending_confirmation");
  expect(await api.post<Row>(prepare, {}, 201)).toMatchObject({id: pending.id, version: pending.version});
  await api.post("/operations/services/availability", selection, 200);
  const order = `/orders/${pending.id}`;
  const command = {expected_version: pending.version, idempotency_key: randomUUID(), reason: "Synthetic operator approval"};
  const confirmed = await api.post<Row>(order + "/confirm", command, 200);
  expect(confirmed.status).toBe("confirmed");
  expect(await api.post<Row>(order + "/confirm", command, 200)).toMatchObject({id: confirmed.id, version: confirmed.version});
  const stock = async () => (await api.get<{items: {variant_id: string; quantity_reserved: number}[]}>("/inventory"))
    .items.find(row => row.variant_id === shop.variants.staff)?.quantity_reserved;
  expect(await stock()).toBe(1);
  const occupied = await api.submit("auto-service", requirements);
  expect(await api.outcome(occupied.id)).toMatchObject({state: "slot_unavailable"});
  const cancelled = await api.post<Row>(order + "/cancel", {expected_version: confirmed.version, idempotency_key: randomUUID(), reason: "Synthetic cleanup"}, 200);
  expect(cancelled.status).toBe("cancelled");
  expect(await stock()).toBe(0);
  await api.post("/operations/services/availability", selection, 200);
  const missing = await api.submit("auto-service", {...requirements, part: {oem: "SYN-UNAVAILABLE", quantity: 1}});
  expect(await api.outcome(missing.id)).toMatchObject({state: "part_unavailable"});
  await api.post(`/agent-delegations/auto-service/${missing.id}/prepare`, {}, 409);
  expect((await orders()).items).toHaveLength(1);
  expect(await stock()).toBe(0);
});
