import {randomUUID} from "node:crypto";
import {readFileSync, writeFileSync} from "node:fs";
import {expect, test, type Page} from "@playwright/test";

type User = {id: string; email: string; password: string};
type Slot = {starts_at: string; ends_at: string};
type Fixture = {id: string; owner: User; staff: User; member: User; outsider: User; conversation: string; offering: string;
  variants: Record<"slot_a" | "slot_b" | "stock" | "staff", string>;
  slots: Record<"slot" | "stock_a" | "stock_b" | "staff", Slot>; foreign_variant: string; foreign_conversation: string};
type Selection = Slot & {offering_id: string; offering_revision: number; timezone: string; units: number};
type Order = {id: string; version: number; status: string; request_kind: string;
  events?: {kind: string; actor_id: string}[]};
type Stock = {variant_id: string; quantity_on_hand: number; quantity_reserved: number; available: number; version: number};
type Movement = {order_id: string; variant_id: string; kind: string; delta_reserved: number; actor_id: string};
type Command = {path: string; data: unknown};
type Response = {status: number; text: string};
const fixturePath = process.env.ABA_HYBRID_E2E_FIXTURE;
const restartReceipt = process.env.ABA_HYBRID_E2E_RESTART_RECEIPT;
const web = process.env.SMOKE_WEB_URL;
const fixture = () => JSON.parse(readFileSync(fixturePath!, "utf8")) as Fixture;
const base = (f: Fixture) => `/api/v1/organizations/${f.id}`;
const operations = (f: Fixture, path: string) => `${base(f)}/operations/${path}`;

async function login(page: Page, user: User) {
  await page.goto("/login");
  await page.getByLabel("Email", {exact: true}).fill(user.email);
  await page.getByLabel("Пароль", {exact: true}).fill(user.password);
  await page.getByRole("button", {name: "Войти", exact: true}).click();
  await expect(page).toHaveURL(/\/dashboard$/);
}
async function commands(page: Page, attempts: Command[], withCSRF = true): Promise<Response[]> {
  const csrf = (await page.context().cookies(web)).find(cookie => cookie.name === "aba_csrf");
  expect(csrf).toBeDefined();
  // Actual concurrent Chromium requests from the authenticated BFF origin.
  return page.evaluate(async ({attempts, token}) => Promise.all(attempts.map(async attempt => {
    const headers: Record<string, string> = {"Content-Type": "application/json"};
    if (token) headers["X-CSRF-Token"] = token;
    const response = await fetch(attempt.path, {method: "POST", credentials: "same-origin", headers, body: JSON.stringify(attempt.data)});
    return {status: response.status, text: await response.text()};
  })), {attempts, token: withCSRF ? csrf!.value : ""});
}
async function post<T = Order>(page: Page, command: Command, status = 200, withCSRF = true): Promise<T> {
  const [response] = await commands(page, [command], withCSRF);
  expect(response.status, `${command.path}: ${response.text}`).toBe(status);
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
async function selection(page: Page, f: Fixture, slot: keyof Fixture["slots"]): Promise<Selection> {
  const offerings = await post<{id: string; revision: number}[]>(page, {path: operations(f, "services/search"), data: {query: "Synthetic26", limit: 5}});
  expect(offerings).toHaveLength(1);
  expect(offerings[0].id).toBe(f.offering);
  expect(Number.isSafeInteger(offerings[0].revision) && offerings[0].revision > 0).toBe(true);
  return {...f.slots[slot], offering_id: f.offering, offering_revision: offerings[0].revision, timezone: "UTC", units: 1};
}
function intake(f: Fixture, selected: Selection, variant: string): Command {
  return {path: operations(f, "requests"), data: {conversation_id: f.conversation, idempotency_key: randomUUID(), kind: "hybrid_order",
    details: {selection: selected, items: [{variant_id: variant, quantity: 1}]}}};
}
function transition(f: Fixture, order: Order, action: "confirm" | "cancel"): Command {
  return {path: `${base(f)}/orders/${order.id}/${action}`, data: {expected_version: order.version, idempotency_key: randomUUID(), reason: "Synthetic26 hybrid browser verification"}};
}
async function inventory(page: Page, f: Fixture, variant: string): Promise<Stock> {
  const all = await get<{items: Stock[]}>(page, `${base(f)}/inventory`);
  const row = all.items.find(item => item.variant_id === variant);
  expect(row, `Expected synthetic stock ${variant}`).toBeDefined();
  return row!;
}
async function movements(page: Page, f: Fixture, ids: string[]): Promise<Movement[]> {
  return (await get<{items: Movement[]}>(page, `${base(f)}/inventory/movements?page_size=100`)).items.filter(item => ids.includes(item.order_id));
}
async function available(page: Page, f: Fixture, selected: Selection, expected = true) {
  const command = {path: operations(f, "services/availability"), data: selected};
  if (expected) expect(await post(page, command)).toMatchObject({available: true});
  else await post(page, command, 409);
}
async function pending(page: Page, command: Command): Promise<Order> {
  const order = await post(page, command, 201);
  expect(order).toMatchObject({request_kind: "hybrid_order", status: "pending_confirmation", total_amount: null});
  return order;
}
async function race(page: Page, attempts: Command[]) {
  const results = await commands(page, attempts);
  expect(results.map(response => response.status).sort(), JSON.stringify(results)).toEqual([200, 409]);
  const winnerIndex = results.findIndex(response => response.status === 200);
  const winner = JSON.parse(results[winnerIndex].text) as Order;
  expect(winner.status).toBe("confirmed");
  return {winnerIndex, loserIndex: 1 - winnerIndex, winner};
}

test.describe("Stage 2.6 hybrid orders with real Chromium BFF and PostgreSQL", () => {
  test.setTimeout(90_000);
  test.skip(!fixturePath || !web, "Requires isolated Stage 2.6 runner and migrated synthetic fixtures");

  test("slot conflict rolls back the losing item's stock; success and cancellation retries never double reserve or release", async ({page}) => {
    const f = fixture();
    await login(page, f.owner);
    const selected = await selection(page, f, "slot");
    const variants = [f.variants.slot_a, f.variants.slot_b];
    const before = await Promise.all(variants.map(variant => inventory(page, f, variant)));
    await available(page, f, selected);
    const create = variants.map(variant => intake(f, selected, variant));
    const orders = [await pending(page, create[0]), await pending(page, create[1])];
    expect(await post(page, create[0], 201)).toMatchObject({id: orders[0].id, version: orders[0].version});
    const original = create[0].data as {details: {items: unknown[]}};
    await post(page, {...create[0], data: {...original, details: {...original.details, summary: "Changed canonical request"}}}, 409);
    expect(await Promise.all(variants.map(variant => inventory(page, f, variant)))).toEqual(before);
    await available(page, f, selected);
    const confirms = orders.map(order => transition(f, order, "confirm"));
    const {winnerIndex, loserIndex, winner} = await race(page, confirms);
    expect(winner.id).toBe(orders[winnerIndex].id);
    const after = await Promise.all(variants.map(variant => inventory(page, f, variant)));
    expect(after[winnerIndex]).toMatchObject({quantity_on_hand: 2, quantity_reserved: 1, available: 1});
    expect(after[loserIndex]).toEqual(before[loserIndex]);
    expect(await get<Order>(page, `${base(f)}/orders/${orders[loserIndex].id}`)).toMatchObject({status: "pending_confirmation", version: orders[loserIndex].version});
    expect(await movements(page, f, orders.map(order => order.id))).toEqual([
      expect.objectContaining({order_id: winner.id, variant_id: variants[winnerIndex], kind: "reserve", delta_reserved: 1}),
    ]);
    await available(page, f, selected, false);
    expect(await post(page, confirms[winnerIndex])).toMatchObject({id: winner.id, status: "confirmed", version: winner.version});
    expect(await Promise.all(variants.map(variant => inventory(page, f, variant)))).toEqual(after);
    const cancel = transition(f, winner, "cancel");
    const cancelled = await post(page, cancel);
    expect(cancelled.status).toBe("cancelled");
    const released = await Promise.all(variants.map(variant => inventory(page, f, variant)));
    for (const row of released) expect(row).toMatchObject({quantity_on_hand: 2, quantity_reserved: 0, available: 2});
    await available(page, f, selected);
    expect(await post(page, cancel)).toMatchObject({id: winner.id, status: "cancelled", version: cancelled.version});
    // A late retry returns the old result without resurrecting cancelled effects.
    expect(await post(page, confirms[winnerIndex])).toMatchObject({status: "confirmed", version: winner.version});
    expect(await get<Order>(page, `${base(f)}/orders/${winner.id}`)).toMatchObject({status: "cancelled", version: cancelled.version});
    expect(await Promise.all(variants.map(variant => inventory(page, f, variant)))).toEqual(released);
    const history = await movements(page, f, [winner.id]);
    expect(history.map(item => item.kind).sort()).toEqual(["release", "reserve"]);
    // The failed command had no durable effect; retry after release can succeed.
    const retriedLoser = await post(page, confirms[loserIndex]);
    expect(retriedLoser).toMatchObject({id: orders[loserIndex].id, status: "confirmed"});
    await available(page, f, selected, false);
    await post(page, transition(f, retriedLoser, "cancel"));
    await available(page, f, selected);
  });

  test("inventory conflict rolls back the losing slot and cancellation frees both resources for a failed-command retry", async ({page}) => {
    const f = fixture();
    await login(page, f.owner);
    const selected = [await selection(page, f, "stock_a"), await selection(page, f, "stock_b")];
    const before = await inventory(page, f, f.variants.stock);
    expect(before).toMatchObject({quantity_on_hand: 1, quantity_reserved: 0, available: 1});
    for (const slot of selected) await available(page, f, slot);
    const orders = [await pending(page, intake(f, selected[0], f.variants.stock)), await pending(page, intake(f, selected[1], f.variants.stock))];
    expect(await inventory(page, f, f.variants.stock)).toEqual(before);
    const confirms = orders.map(order => transition(f, order, "confirm"));
    const {winnerIndex, loserIndex, winner} = await race(page, confirms);
    expect(await inventory(page, f, f.variants.stock)).toMatchObject({quantity_on_hand: 1, quantity_reserved: 1, available: 0});
    await available(page, f, selected[winnerIndex], false);
    await available(page, f, selected[loserIndex]);
    expect(await movements(page, f, orders.map(order => order.id))).toEqual([
      expect.objectContaining({order_id: winner.id, kind: "reserve", delta_reserved: 1}),
    ]);
    await post(page, transition(f, winner, "cancel"));
    expect(await inventory(page, f, f.variants.stock)).toMatchObject({quantity_on_hand: 1, quantity_reserved: 0, available: 1});
    for (const slot of selected) await available(page, f, slot);
    const retried = await post(page, confirms[loserIndex]);
    expect(retried).toMatchObject({id: orders[loserIndex].id, status: "confirmed"});
    await available(page, f, selected[loserIndex], false);
    await available(page, f, selected[winnerIndex]);
    await post(page, transition(f, retried, "cancel"));
    expect(await inventory(page, f, f.variants.stock)).toMatchObject({quantity_on_hand: 1, quantity_reserved: 0, available: 1});
  });

  test("authorized staff can reserve and cancel hybrid orders; CSRF, read-only staff and foreign tenants cannot", async ({page, browser}) => {
    const f = fixture();
    await login(page, f.owner);
    const selected = await selection(page, f, "staff");
    const before = await inventory(page, f, f.variants.staff);
    await post(page, intake(f, selected, f.foreign_variant), 404);
    const foreignConversation = intake(f, selected, f.variants.staff);
    await post(page, {...foreignConversation, data: {...foreignConversation.data as object, conversation_id: f.foreign_conversation}}, 404);
    const staffContext = await browser.newContext({baseURL: web});
    try {
      const staffPage = await staffContext.newPage();
      await login(staffPage, f.staff);
      const request = await pending(staffPage, intake(f, selected, f.variants.staff));
      const confirm = transition(f, request, "confirm");
      await post(page, confirm, 403, false);
      for (const {user, status} of [{user: f.member, status: 403}, {user: f.outsider, status: 404}]) {
        const context = await browser.newContext({baseURL: web});
        try {
          const denied = await context.newPage();
          await login(denied, user);
          await post(denied, intake(f, selected, f.variants.staff), status);
          await post(denied, confirm, status);
          await post(denied, transition(f, request, "cancel"), status);
        } finally {
          await context.close();
        }
      }
      expect(await inventory(page, f, f.variants.staff)).toEqual(before);
      await available(page, f, selected);
      const confirmed = await post(staffPage, confirm);
      expect(confirmed.status).toBe("confirmed");
      expect(await inventory(page, f, f.variants.staff)).toMatchObject({quantity_on_hand: 2, quantity_reserved: 1, available: 1});
      await available(page, f, selected, false);
      const cancelled = await post(staffPage, transition(f, confirmed, "cancel"));
      expect(cancelled.status).toBe("cancelled");
      expect(await inventory(page, f, f.variants.staff)).toMatchObject({quantity_on_hand: 2, quantity_reserved: 0, available: 2});
      await available(page, f, selected);
      const history = await movements(page, f, [request.id]);
      expect(history.map(item => item.kind).sort()).toEqual(["release", "reserve"]);
      for (const item of history) expect(item.actor_id).toBe(f.staff.id);
      const saved = await get<Order>(page, `${base(f)}/orders/${request.id}`);
      expect(saved.events).toContainEqual(expect.objectContaining({kind: "confirmed", actor_id: f.staff.id}));
      expect(saved.events).toContainEqual(expect.objectContaining({kind: "cancelled", actor_id: f.staff.id}));
    } finally {
      await staffContext.close();
    }
  });

  test("persists a confirmed reservation before the worker stops", async ({page}) => {
    test.skip(!restartReceipt, "Requires the isolated worker restart runner");
    const f = fixture();
    await login(page, f.owner);
    const slot = await selection(page, f, "staff");
    const request = await pending(page, intake(f, slot, f.variants.staff));
    const confirmed = await post(page, transition(f, request, "confirm"));
    expect(confirmed.status).toBe("confirmed");
    await available(page, f, slot, false);
    writeFileSync(restartReceipt!, confirmed.id + "\n", {flag: "wx", mode: 0o600});
  });

  test("hybrid reservation survives a worker restart", async ({page}) => {
    test.skip(!restartReceipt, "Requires the isolated worker restart runner");
    const f = fixture();
    await login(page, f.owner);
    const id = readFileSync(restartReceipt!, "utf8").trim();
    const order = await get<Order>(page, `${base(f)}/orders/${id}`);
    expect(order).toMatchObject({status: "confirmed", request_kind: "hybrid_order"});
    expect(await inventory(page, f, f.variants.staff)).toMatchObject({quantity_on_hand: 2, quantity_reserved: 1});
    const slot = await selection(page, f, "staff");
    await available(page, f, slot, false);
    expect((await post(page, transition(f, order, "cancel"))).status).toBe("cancelled");
    await available(page, f, slot);
    expect(await inventory(page, f, f.variants.staff)).toMatchObject({quantity_on_hand: 2, quantity_reserved: 0});
  });
});
