import {randomUUID} from "node:crypto";
import {existsSync, readFileSync, writeFileSync} from "node:fs";
import {expect, test} from "@playwright/test";

const fixture = process.env.ABA_SERVICE_CLOSURE_FIXTURE;
const markers = process.env.ABA_SERVICE_CLOSURE_MARKERS;
const origin = process.env.SMOKE_WEB_URL;
type Task = {id: string; parent_id?: string; status: string; action: string; result?: unknown[]};
type Order = {id: string; version: number; status: string};

test("approved offering rules and durable receipt use the committed booking after restart", async ({page}) => {
  test.skip(!fixture || !markers || !origin, "Requires isolated service closure runner");
  test.setTimeout(120_000);
  const f = JSON.parse(readFileSync(fixture!, "utf8")) as {
    id: string; owner: {email: string; password: string}; conversation: string; offering: string; resource: string;
    foreign_conversation: string; slots: {staff: {starts_at: string; ends_at: string}};
  };
  await page.goto("/login");
  await page.getByLabel("Email", {exact: true}).fill(f.owner.email);
  await page.getByLabel("Пароль", {exact: true}).fill(f.owner.password);
  await page.getByRole("button", {name: "Войти", exact: true}).click();
  await expect(page).toHaveURL(/\/dashboard$/);
  const csrf = (await page.context().cookies(origin)).find(cookie => cookie.name === "aba_csrf")!.value;
  const root = `/api/v1/organizations/${f.id}`;
  const post = async <T>(path: string, data: unknown, status = 200) => {
    const response = await page.request.post(root + path, {data, headers: {"X-CSRF-Token": csrf, Origin: origin!}});
    expect(response.status(), await response.text()).toBe(status);
    return response.json() as Promise<T>;
  };
  const tasks = async () => {
    const response = await page.request.get(root + "/agent-tasks");
    expect(response.status()).toBe(200);
    return ((await response.json()) as {items: Task[]}).items;
  };
  let [offering] = await post<Array<Record<string, unknown>>>("/operations/services/search", {query: "Synthetic26", limit: 5});
  const rule = async (part_rule: Record<string, unknown>) => {
    offering = await post<Record<string, unknown>>("/operations/services/config/offerings", {...offering, resource_ids: [f.resource], part_rule});
  };
  const submit = async (conversation = f.conversation, expected = 202) => post<{id: string}>("/agent-delegations/auto-service", {
    conversation_id: conversation, idempotency_key: randomUUID(),
    requirements: {vehicle: {make: "synthetic", model: "model25", engine: "e25", year: 2025},
      selection: {...f.slots.staff, offering_id: f.offering, offering_revision: offering.revision, timezone: "UTC", units: 1}},
  }, expected);
  const outcome = async (id: string) => {
    await expect.poll(async () => (await tasks()).find(t => t.id === id)?.status, {timeout: 30_000}).toBe("completed");
    return (await tasks()).find(t => t.id === id)!.result![0];
  };
  const receipts = async (id: string) => (await tasks()).filter(t => t.parent_id === id && t.action === "delegation.service_receipt");
  const waitReceipt = async (parent: string, status: string) => {
    const expected = expect.objectContaining({state: "backend_result", message: expect.any(String),
      receipt: expect.objectContaining({booking_status: status, service: "Synthetic26 installation"})});
    await expect.poll(async () => (await receipts(parent)).flatMap(t => t.result ?? []), {timeout: 20_000})
      .toContainEqual(expected);
  };

  await rule({state: "REQUIRED", oem: "SYN-262-PART", quantity: 1, model: "model25", year: 2025});
  await submit(f.foreign_conversation, 403);
  const first = (await submit()).id;
  expect(await outcome(first)).toMatchObject({state: "approval_required", part_requirement: "REQUIRED", proposal: {kind: "hybrid_order"}});
  const pending = await post<Order>(`/agent-delegations/auto-service/${first}/prepare`, {}, 201);
  expect(await receipts(first)).toEqual([]);
  writeFileSync(`${markers}/stop`, first);
  await expect.poll(() => existsSync(`${markers}/stopped`), {timeout: 20_000}).toBe(true);
  const path = `/orders/${pending.id}`;
  const command = {expected_version: pending.version, idempotency_key: randomUUID(), reason: "Synthetic approval"};
  const confirmed = await post<Order>(path + "/confirm", command);
  expect(confirmed.status).toBe("confirmed");
  await post<Order>(path + "/confirm", command);
  expect(await receipts(first)).toEqual([expect.objectContaining({status: "queued"})]);
  writeFileSync(`${markers}/resume`, first);
  await waitReceipt(first, "confirmed");
  const [receipt] = await receipts(first);
  expect(receipt.result![0]).toMatchObject({receipt: {version: confirmed.version, reservation_status: "active",
    parts: [expect.objectContaining({quantity: 1, unit_price: 12500})]}});
  await page.goto("/dashboard/ai-settings");
  const notification = page.getByTestId("agent-audit").getByRole("row").filter({hasText: receipt.id});
  await expect(notification.getByText("Канал недоступен для отправки", {exact: true})).toBeVisible();
  await post<Order>(path + "/cancel", {expected_version: confirmed.version, idempotency_key: randomUUID(), reason: "Synthetic cancellation"});
  await waitReceipt(first, "cancelled");

  await rule({state: "UNKNOWN"});
  const unknown = (await submit()).id;
  expect(await outcome(unknown)).toMatchObject({state: "part_requirement_unknown", part_requirement: "UNKNOWN"});
  await post(`/agent-delegations/auto-service/${unknown}/prepare`, {}, 409);

  await rule({state: "NOT_REQUIRED"});
  const second = (await submit()).id;
  expect(await outcome(second)).toMatchObject({state: "approval_required", part_requirement: "NOT_REQUIRED", proposal: {kind: "service_booking"}});
  const other = await post<Order>(`/agent-delegations/auto-service/${second}/prepare`, {}, 201);
  await post<Order>(`/orders/${other.id}/reject`, {expected_version: other.version, idempotency_key: randomUUID(), reason: "Synthetic refusal"});
  await waitReceipt(second, "rejected");
});
