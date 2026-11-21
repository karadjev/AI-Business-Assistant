import {randomUUID} from "node:crypto";
import {readFileSync} from "node:fs";
import {expect, test, type Page} from "@playwright/test";

type Shop = {id: string; owner: {email: string; password: string}; conversation: string;
  variant: string; offering: string; listing: string; starts_at: string; ends_at: string};
type Preview = {task_id: string; state: string; draft?: {kind: string; details: unknown; items?: unknown[]}};
const fixturePath = process.env.ABA_BUSINESS_E2E_FIXTURE;
const web = process.env.SMOKE_WEB_URL;
const fixture = () => JSON.parse(readFileSync(fixturePath!, "utf8")) as Record<string, Shop>;
const home = (shop: Shop) => `/api/v1/organizations/${shop.id}`;

async function login(page: Page, shop: Shop) {
  await page.goto("/login");
  await page.getByLabel("Email", {exact: true}).fill(shop.owner.email);
  await page.getByLabel("Пароль", {exact: true}).fill(shop.owner.password);
  await page.getByRole("button", {name: "Войти", exact: true}).click();
  await expect(page).toHaveURL(/\/dashboard$/);
}

async function post<T>(page: Page, path: string, data: unknown, status = 200, includeCSRF = true): Promise<T> {
  const csrf = (await page.context().cookies(web)).find(cookie => cookie.name === "aba_csrf");
  expect(csrf).toBeDefined();
  const response = await page.evaluate(async ({path, data, token}) => {
    const headers: Record<string, string> = {"Content-Type": "application/json"};
    if (token) headers["X-CSRF-Token"] = token;
    const response = await fetch(path, {method: "POST", headers, body: JSON.stringify(data)});
    return {status: response.status, body: await response.text()};
  }, {path, data, token: includeCSRF ? csrf!.value : ""});
  expect(response.status, `${path}: ${response.body}`).toBe(status);
  return JSON.parse(response.body) as T;
}

function preview(page: Page, shop: Shop, workflow: unknown, status = 200) {
  return post<Preview>(page, `${home(shop)}/agent-workflows/preview`, {
    conversation_id: shop.conversation, idempotency_key: randomUUID(), workflow,
  }, status);
}

function submit(page: Page, shop: Shop, draft: NonNullable<Preview["draft"]>) {
  return post<{id: string; version: number}>(page, `${home(shop)}/operations/requests`, {
    conversation_id: shop.conversation, idempotency_key: randomUUID(), kind: draft.kind, details: draft.details,
  }, 201);
}

const vehicle = {make: "synthetic", model: "model25", year: 2025, engine: "e25"};

test.describe("Stage 2.6 staff workflow previews through real Chromium", () => {
  test.skip(!fixturePath || !web, "Requires isolated Stage 2.6 fixture");

  test("B prepares a quote for verified fitment and refuses to confirm unknown fitment", async ({page}) => {
    const {auto_parts: shop, building_materials: other} = fixture();
    await login(page, shop);
    const input = {search: {limit: 5}, vehicle, variant_id: shop.variant, quantity: 1};
    const result = await preview(page, shop, {scenario: "B", auto_parts: input});
    expect(result).toMatchObject({task_id: expect.any(String), fitment: {status: "compatible"},
      draft: {kind: "product_quote", items: [{variant_id: shop.variant, quantity: 1}]}});
    const auditStatus = await page.evaluate(async path => {
      const response = await fetch(path);
      return {status: response.status, audit: await response.json()};
    }, `${home(shop)}/agent-audit?task_id=${result.task_id}`);
    expect(auditStatus.status).toBe(200);
    expect(auditStatus.audit).toMatchObject({items: [expect.objectContaining({
      task_id: result.task_id, action: "workflow_preview_B", result: "completed",
    })]});
    const conversation = `${home(shop)}/conversations/${shop.conversation}`;
    const quote = await post<{id: string}>(page, `${conversation}/quotes`, {items: result.draft!.items}, 201);
    expect(quote).toMatchObject({total_amount: 12500, currency_code: "RUB", reserved: false});
    const consent = {quote_id: quote.id, accepted: true, idempotency_key: randomUUID()};
    const created = await post<{id: string; version: number}>(page, `${conversation}/confirm-request`, consent);
    expect(created).toMatchObject({status: "pending_confirmation", total_amount: 12500});
    expect(await post(page, `${conversation}/confirm-request`, consent)).toMatchObject({id: created.id});
    const confirmed = await post<{version: number}>(page, `${home(shop)}/orders/${created.id}/confirm`, {
      expected_version: created.version, idempotency_key: randomUUID(), reason: "Synthetic staff approval",
    });
    expect(confirmed).toMatchObject({status: "confirmed"});
    await post(page, `${home(shop)}/orders/${created.id}/cancel`, {
      expected_version: confirmed.version, idempotency_key: randomUUID(), reason: "Synthetic fixture cleanup",
    });
    const unknown = await preview(page, shop, {scenario: "B", auto_parts: {...input, vehicle: {...vehicle, engine: "unverified"}}});
    expect(unknown).toMatchObject({state: "fitment_unknown", fitment: {status: "unknown"}});
    expect(unknown.draft).toBeUndefined();
    await preview(page, other, {scenario: "B", auto_parts: input}, 404);
    const path = `${home(shop)}/agent-workflows/preview`;
    const workflow = {scenario: "B", auto_parts: input};
    await post(page, path, {conversation_id: shop.conversation, idempotency_key: randomUUID(), workflow}, 403, false);
    await post(page, path, {conversation_id: shop.conversation, idempotency_key: randomUUID(),
      workflow: {...workflow, sql: "synthetic unsupported field"}}, 400);
  });

  test("C calculates quantities and saves a separate commercial request", async ({page}) => {
    const {building_materials: shop} = fixture();
    await login(page, shop);
    const dimensions = {length: {value: "300", unit: "cm"}, width: {value: "2", unit: "m"}, waste_percent: "10"};
    const selection = {variant_id: shop.variant, search: {limit: 5}};
    const result = await preview(page, shop, {scenario: "C", construction: {
      ...selection, calculation: {...dimensions, operation: "area"},
    }});
    expect(result).toMatchObject({calculation: {quantity: "6.6", unit: "m2", formula: expect.any(String),
      intermediate: {base: "6", waste_amount: "0.6", with_waste: "6.6"}}});
    const packaged = await preview(page, shop, {scenario: "C", construction: {
      ...selection, calculation: {...dimensions, operation: "tiles", units_per_package: 8,
        item_length: {value: "1", unit: "m"}, item_width: {value: "1", unit: "m"}},
    }});
    expect(packaged).toMatchObject({calculation: {quantity: "7", unit: "piece", package_count: 1},
      draft: {kind: "quotation_request", items: [{variant_id: shop.variant, quantity: 1}]}});
    expect(await submit(page, shop, packaged.draft!)).toMatchObject({
      request_kind: "quotation_request", status: "pending_confirmation",
    });
  });

  test("D prepares a service request from a current slot and flags changed revisions", async ({page}) => {
    const {vehicle_service: shop} = fixture();
    await login(page, shop);
    const query = "Synthetic25";
    const offerings = await post<{id: string; revision: number}[]>(page, `${home(shop)}/operations/services/search`, {query, limit: 5});
    const {id, revision} = offerings[0];
    const selection = {offering_id: id, offering_revision: revision, timezone: "UTC", units: 1,
      starts_at: shop.ends_at, ends_at: new Date(Date.parse(shop.ends_at) + 3_600_000).toISOString()};
    const result = await preview(page, shop, {scenario: "D", service_booking: {query, selection}});
    expect(result).toMatchObject({draft: {kind: "service_booking"}, availability: {available: true}});
    const request = await submit(page, shop, result.draft!);
    expect(request).toMatchObject({request_kind: "service_booking", status: "pending_confirmation"});
    const command = {expected_version: request.version, idempotency_key: randomUUID(), reason: "Synthetic service approval"};
    const confirm = `${home(shop)}/orders/${request.id}/confirm`;
    const confirmed = await post(page, confirm, command);
    expect(confirmed).toMatchObject({id: request.id, status: "confirmed"});
    expect(await post(page, confirm, command)).toEqual(confirmed);
    const unavailable = await preview(page, shop, {scenario: "D", service_booking: {query, selection}});
    expect(unavailable).toMatchObject({state: "slot_unavailable"});
    expect(unavailable.draft).toBeUndefined();
    const changed = await preview(page, shop, {scenario: "D", service_booking: {query,
      selection: {...selection, offering_revision: revision + 1}}});
    expect(changed).toMatchObject({state: "source_changed"});
    expect(changed.draft).toBeUndefined();
  });

  test("E selects a current listing and saves a viewing request on mobile", async ({page}) => {
    await page.setViewportSize({width: 390, height: 844});
    const {real_estate: shop} = fixture();
    await login(page, shop);
    const result = await preview(page, shop, {scenario: "E", property_viewing: {
      search: {rooms: 2, min_area_m2: 60}, listing_id: shop.listing,
    }});
    expect(result).toMatchObject({properties: [expect.objectContaining({id: shop.listing, status: "available"})],
      draft: {kind: "property_viewing_request", details: {listing_id: shop.listing}}});
    expect(await submit(page, shop, result.draft!)).toMatchObject({
      request_kind: "property_viewing_request", status: "pending_confirmation", listing_id: shop.listing,
    });
  });
});
