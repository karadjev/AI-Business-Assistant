import {randomUUID} from "node:crypto";
import {readFileSync} from "node:fs";
import {expect, test, type Page} from "@playwright/test";

type User = {id: string; email: string; password: string};
type Business = {id: string; owner: User; variant: string; conversation: string; offering: string;
  listing: string; starts_at: string; ends_at: string};
type Fixture = Record<"auto_parts" | "building_materials" | "vehicle_service" | "real_estate", Business>;
type Task = {id: string; agent_id: string; status: string; result: unknown};
type Approval = {id: string; task_id: string; status: string; parameters_hash: string};
type Audit = {task_id: string; action: string; result: string};
const fixturePath = process.env.ABA_BUSINESS_E2E_FIXTURE;
const web = process.env.SMOKE_WEB_URL;
const base = (id: string) => `/api/v1/organizations/${id}`;
const fixture = () => JSON.parse(readFileSync(fixturePath!, "utf8")) as Fixture;

async function login(page: Page, user: User) {
  await page.goto("/login");
  await page.getByLabel("Email", {exact: true}).fill(user.email);
  await page.getByLabel("Пароль", {exact: true}).fill(user.password);
  await page.getByRole("button", {name: "Войти", exact: true}).click();
  await expect(page).toHaveURL(/\/dashboard$/);
}

// These are Chromium fetches from the authenticated page origin, not mocks or
// direct API calls: session/Origin/CSRF all pass through the production Next BFF.
async function post<T = Record<string, unknown>>(page: Page, path: string, data: unknown, status = 200, withCSRF = true): Promise<T> {
  const csrf = (await page.context().cookies(web)).find(cookie => cookie.name === "aba_csrf");
  expect(csrf).toBeDefined();
  const response = await page.evaluate(async ({path, data, token}) => {
    const headers: Record<string, string> = {"Content-Type": "application/json"};
    if (token) headers["X-CSRF-Token"] = token;
    const response = await fetch(path, {method: "POST", headers, credentials: "same-origin", body: JSON.stringify(data)});
    return {status: response.status, text: await response.text()};
  }, {path, data, token: withCSRF ? csrf!.value : ""});
  expect(response.status, `${path}: ${response.text}`).toBe(status);
  return (response.text ? JSON.parse(response.text) : undefined) as T;
}
async function get<T>(page: Page, path: string): Promise<T> {
  const response = await page.evaluate(async path => {
    const response = await fetch(path, {credentials: "same-origin"});
    return {status: response.status, text: await response.text()};
  }, path);
  expect(response.status, `${path}: ${response.text}`).toBe(200);
  return JSON.parse(response.text) as T;
}
const operation = (business: Business, action: string) => `${base(business.id)}/operations/${action}`;

test.describe("Stage 2.5 universal operations through real Chromium BFF", () => {
  test.setTimeout(45_000);
  test.skip(!fixturePath || !web, "Requires isolated Stage 2.5 runner and synthetic fixture");

  test("searches scoped auto parts and verifies fitment evidence without inventing a match", async ({page}) => {
    const {auto_parts: shop, building_materials: other} = fixture();
    await login(page, shop.owner);
    const vehicle = {make: "synthetic", model: "model25", year: 2025, engine: "e25"};
    const query = {industry: "auto_parts", text: [{field: "oem", match: "exact", value: "SYN-25-OEM"}],
      vehicle, availability: "in_stock", limit: 5};
    await post(page, operation(shop, "catalog/search"), query, 403, false);
    const result = await post<{items: {variant_id: string; available: number; freshness: string}[]}>(page, operation(shop, "catalog/search"), query);
    expect(result.items).toEqual([expect.objectContaining({variant_id: shop.variant, available: 10, freshness: "FRESH"})]);
    expect(await post(page, operation(shop, "catalog/search"), {...query, vehicle: {...vehicle, engine: "unverified"}}))
      .toMatchObject({items: []});
    await post(page, operation(other, "catalog/search"), {industry: "building_materials", limit: 5,
      text: [{field: "sku", match: "exact", value: "SYN25"}]}, 404);
    await post(page, operation(shop, "catalog/search"), {...query, text: [{field: "sql", match: "exact", value: "x"}]}, 422);
    const fitment = {variant_id: shop.variant, ...vehicle};
    const verified = await post<{status: string; evidence: {source_ref: string}[]}>(page, operation(shop, "catalog/fitment"), fitment);
    expect(verified).toMatchObject({status: "compatible", reason: "verified_evidence", variant_id: shop.variant});
    expect(verified.evidence).toEqual([expect.objectContaining({source_ref: "synthetic:stage25"})]);
    expect(await post(page, operation(shop, "catalog/fitment"), {...fitment, engine: "unverified"}))
      .toMatchObject({status: "unknown", reason: "no_verified_evidence", evidence: []});
    await post(page, operation(shop, "catalog/fitment"), {...fitment, variant_id: other.variant}, 404);
  });

  test("calculates construction with explicit units and creates an idempotent nonfinancial request", async ({page}) => {
    const {building_materials: shop, auto_parts: other} = fixture();
    await login(page, shop.owner);
    const calculation = {operation: "area", length: {value: "300", unit: "cm"}, width: {value: "2", unit: "m"}, waste_percent: "10"};
    expect(await post(page, operation(shop, "construction/calculate"), calculation))
      .toMatchObject({quantity: "6.6", unit: "m2", intermediate: {base: "6", waste_amount: "0.6", with_waste: "6.6"}});
    await post(page, operation(shop, "construction/calculate"), {...calculation, width: {value: "2", unit: "kg"}}, 422);
    const request = {conversation_id: shop.conversation, idempotency_key: randomUUID(), kind: "quotation_request",
      details: {summary: "Synthetic25 construction estimate; no payment or reservation"}};
    const created = await post<{id: string}>(page, operation(shop, "requests"), request, 201);
    expect(created).toMatchObject({request_kind: "quotation_request", status: "pending_confirmation", total_amount: null, quote_id: null});
    expect(await post(page, operation(shop, "requests"), request, 201)).toMatchObject({id: created.id});
    await post(page, operation(shop, "requests"), {...request, details: {summary: "Changed canonical request"}}, 409);
    await post(page, operation(shop, "requests"), {...request, idempotency_key: randomUUID(), conversation_id: other.conversation}, 404);
  });

  test("discovers services and admits exactly one of two concurrent booking confirmations", async ({page}) => {
    const {vehicle_service: shop, auto_parts: other} = fixture();
    await login(page, shop.owner);
    const offerings = await post<{id: string; revision: number; amount_minor: number}[]>(page, operation(shop, "services/search"), {query: "Synthetic25", limit: 5});
    expect(offerings).toEqual([expect.objectContaining({id: shop.offering, amount_minor: 50000})]);
    const revision = offerings[0].revision;
    expect(Number.isSafeInteger(revision) && revision > 0).toBe(true);
    const selection = {offering_id: shop.offering, offering_revision: revision, starts_at: shop.starts_at, ends_at: shop.ends_at, timezone: "UTC", units: 1};
    expect(await post(page, operation(shop, "services/availability"), selection)).toMatchObject({available: true});
    await post(page, operation(shop, "services/availability"), {...selection, offering_revision: revision + 1}, 409);
    await post(page, operation(other, "services/search"), {query: "Synthetic25", limit: 5}, 404);
    const requests: {id: string; version: number}[] = [];
    for (let index = 0; index < 2; index++) {
      const request = await post<{id: string; version: number}>(page, operation(shop, "requests"), {
        conversation_id: shop.conversation, idempotency_key: randomUUID(), kind: "service_booking", details: {selection},
      }, 201);
      expect(request).toMatchObject({request_kind: "service_booking", status: "pending_confirmation", total_amount: null});
      requests.push(request);
    }
    expect(new Set(requests.map(request => request.id)).size).toBe(2);
    // Preparing both requests must not reserve capacity before staff confirmation.
    expect(await post(page, operation(shop, "services/availability"), selection)).toMatchObject({available: true});
    const token = (await page.context().cookies(web)).find(cookie => cookie.name === "aba_csrf")!.value;
    const attempts = requests.map(request => ({path: `${base(shop.id)}/orders/${request.id}/confirm`,
      data: {expected_version: request.version, idempotency_key: randomUUID(), reason: "Synthetic25 concurrent capacity admission"}}));
    const confirmations = await page.evaluate(async ({attempts, token}) => Promise.all(attempts.map(async attempt => {
      const response = await fetch(attempt.path, {method: "POST", credentials: "same-origin",
        headers: {"Content-Type": "application/json", "X-CSRF-Token": token}, body: JSON.stringify(attempt.data)});
      return {status: response.status, text: await response.text()};
    })), {attempts, token});
    expect(confirmations.map(response => response.status).sort(), JSON.stringify(confirmations)).toEqual([200, 409]);
    expect(JSON.parse(confirmations.find(response => response.status === 200)!.text)).toMatchObject({status: "confirmed"});
    const saved = await Promise.all(requests.map(request => get<{status: string}>(page, `${base(shop.id)}/orders/${request.id}`)));
    expect(saved.map(order => order.status).sort()).toEqual(["confirmed", "pending_confirmation"]);
    await post(page, operation(shop, "services/availability"), selection, 409);
  });

  test("filters current properties and creates a viewing request with stale listing downgrade", async ({page}) => {
    const {real_estate: shop} = fixture();
    await login(page, shop.owner);
    const result = await post<{items: {id: string; title: string; status: string}[]}>(page, operation(shop, "properties/search"), {rooms: 2, min_area_m2: 60});
    expect(result.items).toEqual([expect.objectContaining({id: shop.listing, status: "available"})]);
    const all = await post<{items: {id: string; status: string}[]}>(page, operation(shop, "properties/search"), {});
    const expired = all.items.find(item => item.id !== shop.listing);
    expect(expired).toMatchObject({status: "unknown"});
    const request = {conversation_id: shop.conversation, idempotency_key: randomUUID(), kind: "property_viewing_request", details: {listing_id: shop.listing}};
    expect(await post(page, operation(shop, "requests"), request, 201))
      .toMatchObject({request_kind: "property_viewing_request", status: "pending_confirmation", listing_id: shop.listing, total_amount: null});
    await post(page, operation(shop, "requests"), {...request, idempotency_key: randomUUID(), details: {listing_id: expired!.id}}, 409);
  });

  test("coordinates growth with separate analytics, sales, inventory and approved marketing results and audit", async ({page}) => {
    test.setTimeout(60_000);
    const {auto_parts: shop} = fixture();
    await login(page, shop.owner);
    const path = base(shop.id);
    const tasks = async () => (await get<{items: Task[]}>(page, `${path}/agent-tasks`)).items;
    const previous = new Set((await tasks()).map(task => task.id));
    const parent = await post<{id: string}>(page, `${path}/agent-tasks`, {agent_id: "business_manager", action: "coordinate_growth", idempotency_key: randomUUID()}, 202);
    // The public task projection deliberately omits parent_id. This synthetic
    // owner has no concurrent submitter; the four new task IDs are its children.
    const children = (await tasks()).filter(task => task.id !== parent.id && !previous.has(task.id));
    expect(children.map(task => task.agent_id).sort()).toEqual(["analytics", "marketing", "product_inventory", "sales"]);
    const marketing = children.find(task => task.agent_id === "marketing")!;
    const pending = (await get<{items: Approval[]}>(page, `${path}/agent-approvals`)).items.find(approval => approval.task_id === marketing.id)!;
    expect(pending).toMatchObject({status: "pending", parameters_hash: expect.stringMatching(/^[a-f0-9]{64}$/)});
    await post(page, `${path}/agent-approvals/${pending.id}/decision`, {decision: "approved", parameters_hash: pending.parameters_hash}, 204);
    await expect.poll(async () => (await tasks()).find(task => task.id === parent.id)?.status, {timeout: 25_000}).toBe("completed");
    const result = (await tasks()).find(task => task.id === parent.id)!.result as {Children: {agent_id: string; status: string; result: unknown}[]; recommendations: string[]};
    expect(result.Children).toHaveLength(4);
    for (const id of ["analytics", "sales", "product_inventory", "marketing"]) {
      expect(result.Children).toContainEqual(expect.objectContaining({agent_id: id, status: "completed", result: expect.any(Array)}));
    }
    const metrics = (id: string) => (result.Children.find(child => child.agent_id === id)!.result as {metrics: {name: string; value: number}[]}[])[0].metrics;
    expect(metrics("sales").map(metric => metric.name).sort()).toEqual(["categories", "units", "variants"]);
    expect(metrics("product_inventory")).toContainEqual(expect.objectContaining({name: "in_stock", value: 1}));
    expect(result.recommendations).toContain("При подготовке предложений используйте только позиции с подтверждённым наличием.");
    for (const child of children) {
      const audit = await get<{items: Audit[]}>(page, `${path}/agent-audit?task_id=${child.id}`);
      expect(audit.items).toContainEqual(expect.objectContaining({task_id: child.id, action: "task.delegate"}));
      expect(audit.items).toContainEqual(expect.objectContaining({task_id: child.id, result: "succeeded"}));
    }
    const marketingAudit = await get<{items: Audit[]}>(page, `${path}/agent-audit?task_id=${marketing.id}`);
    expect(marketingAudit.items).toContainEqual(expect.objectContaining({action: "approval.decision", result: "approved"}));
    await page.goto(`/dashboard/ai-settings?org=${shop.id}`);
    const team = page.getByRole("region", {name: "AI-команда"});
    await expect(team.getByTestId(`agent-task-${parent.id}`)).toBeVisible();
  });
});
