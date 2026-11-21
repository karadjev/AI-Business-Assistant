import {execFileSync, spawn, type ChildProcess} from "node:child_process";
import {once} from "node:events";
import {createServer, type IncomingMessage, type Server, type ServerResponse} from "node:http";
import {resolve} from "node:path";
import {expect, test} from "@playwright/test";

const artifact = process.env.ABA_APPROVALS_TEST_ARTIFACT;
const owner = "11111111-1111-4111-8111-111111111111";
const member = "22222222-2222-4222-8222-222222222222";
const approvalId = "33333333-3333-4333-8333-333333333333";
const secondId = "44444444-4444-4444-8444-444444444444";
const session = "aba_session=synthetic-session";
const csrf = "synthetic-csrf";
const root = (id: string) => `/api/v1/organizations/${id}/agent-approvals`;
let app: ChildProcess;
let api: Server;
let origin: string;
let listStatus: number;
let decisionStatus: number;
let items: ReturnType<typeof approval>[];
let decisions: {id: string; body: unknown; csrf: string | undefined; origin: string | undefined}[];
let releaseDecision: (() => void) | undefined;
let holdDecision: Promise<void> | undefined;

function approval(id: string, action: string, status = "pending", expiresAt = Date.now() + 600_000) {
  return {
    id, action, status, expires_at: new Date(expiresAt).toISOString(),
    task_id: "synthetic-task", agent_id: "synthetic-agent",
    parameters: {note: "<script>synthetic</script>"}, parameters_hash: `synthetic-hash-${id}`,
    reason: "Проверка синтетического действия",
  };
}

async function listen(server: Server) {
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  const address = server.address();
  if (!address || typeof address === "string") throw new Error("Missing fixture port");
  return address.port;
}

async function handle(request: IncomingMessage, response: ServerResponse) {
  const url = request.url ?? "";
  const reply = (status: number, body?: unknown) => {
    response.writeHead(status, {"Content-Type": "application/json"});
    response.end(body === undefined ? undefined : JSON.stringify(body));
  };
  if (!request.headers.cookie?.includes(session)) return reply(401);
  if (url === "/api/v1/auth/me") return reply(200, {user: {display_name: "Сотрудник", email: "synthetic@example.test"}});
  if (url === "/api/v1/organizations") return reply(200, {items: [
    {id: owner, name: "Первый магазин", is_test: true, role: "owner", timezone: "UTC"},
    {id: member, name: "Второй магазин", is_test: true, role: "member", timezone: "UTC"},
  ]});
  if (url.endsWith("/ai-settings")) return reply(200, {
    mode: "mock", model: "synthetic", settings: {version: 1}, limits: {}, platform: {limit_microrub: 0}, metrics: [],
  });
  if (url === root(member)) return reply(200, {items: [approval(secondId, "member.action")]});
  if (url === root(owner) && request.method === "GET") return reply(listStatus, {items});
  if (url.startsWith(root(owner) + "/") && url.endsWith("/decision") && request.method === "POST") {
    let raw = "";
    for await (const chunk of request) raw += chunk;
    const id = url.split("/").at(-2) ?? "";
    const csrfHeader = request.headers["x-csrf-token"];
    const body = JSON.parse(raw);
    decisions.push({id, body, csrf: typeof csrfHeader === "string" ? csrfHeader : undefined, origin: request.headers.origin});
    if (csrfHeader !== csrf || request.headers.origin !== origin || !request.headers.cookie.includes(`aba_csrf=${csrf}`)) return reply(403);
    await holdDecision;
    if (decisionStatus === 204) items = items.filter(item => item.id !== id);
    return reply(decisionStatus);
  }
  reply(405);
}

test.describe("AI settings approvals on the offline production build", () => {
  test.skip(!artifact, "Set ABA_APPROVALS_TEST_ARTIFACT to the verified standalone server.js");

  test.beforeAll(async () => {
    api = createServer((request, response) => {
      void handle(request, response).catch(() => {response.writeHead(500).end();});
    });
    const apiPort = await listen(api);
    const portProbe = createServer();
    const webPort = await listen(portProbe);
    await new Promise<void>(resolve => portProbe.close(() => resolve()));
    origin = `http://127.0.0.1:${webPort}`;
    app = spawn(process.execPath, [artifact!], {
      env: {PATH: process.env.PATH, NODE_ENV: "production", HOSTNAME: "127.0.0.1", PORT: String(webPort),
        API_BASE_URL: `http://127.0.0.1:${apiPort}`, NEXT_TELEMETRY_DISABLED: "1"},
      stdio: ["ignore", "pipe", "pipe"],
    });
    app.stdout?.pipe(process.stdout);
    app.stderr?.pipe(process.stderr);
    await expect.poll(async () => (await fetch(origin + root(owner)).catch(() => null))?.status).toBe(401);
    execFileSync(process.execPath, [resolve(__dirname, "../../../scripts/build-env/artifacts.cjs"), "served"], {
      env: {...process.env, ABA_WEB_ROOT: resolve(artifact!, "../../.."),
        ABA_WEB_SOURCE: resolve(__dirname, ".."), SMOKE_WEB_URL: origin},
      stdio: "inherit",
    });
  });

  test.beforeEach(async ({context}) => {
    listStatus = 200;
    decisionStatus = 204;
    items = [approval(approvalId, "catalog.update"), approval(secondId, "catalog.review")];
    decisions = [];
    releaseDecision = undefined;
    holdDecision = undefined;
    await context.addCookies([
      {name: "aba_session", value: "synthetic-session", url: origin, httpOnly: true},
      {name: "aba_csrf", value: csrf, url: origin},
    ]);
  });

  test.afterEach(() => releaseDecision?.());
  test.afterAll(async () => {
    if (app && app.exitCode === null) {
      app.kill();
      await once(app, "exit");
    }
    if (api) {
      api.closeAllConnections();
      await new Promise<void>(resolve => api.close(() => resolve()));
    }
  });

  test("reviews parameters by keyboard and posts each decision once through BFF", async ({page}) => {
    items.push(approval("55555555-5555-4555-8555-555555555555", "closed.action", "approved"));
    await page.goto(`${origin}/dashboard/ai-settings?org=${owner}`);
    const section = page.getByRole("region", {name: "Требуют подтверждения"});
    const first = section.getByRole("listitem").filter({hasText: "catalog.update"});
    const details = first.getByText("Детали запроса", {exact: true});
    await details.focus();
    await page.keyboard.press("Enter");
    await expect(first.getByText("<script>synthetic</script>", {exact: false})).toBeVisible();
    await expect(first.locator("script")).toHaveCount(0);
    await expect(section.getByText("closed.action")).toHaveCount(0);

    holdDecision = new Promise(resolve => {releaseDecision = resolve;});
    const confirm = first.getByRole("button", {name: "Подтвердить", exact: true});
    await page.keyboard.press("Tab");
    await expect(confirm).toBeFocused();
    await page.keyboard.press("Enter");
    await page.keyboard.press("Enter");
    await expect(section.getByRole("status")).toHaveText("Отправка решения…");
    await expect(confirm).toBeDisabled();
    await expect.poll(() => decisions.length).toBe(1);
    releaseDecision?.();
    await expect(section.getByRole("status")).toHaveText("Действие подтверждено.");
    await expect(section.getByRole("button", {name: "Обновить запросы"})).toBeFocused();

    const second = section.getByRole("listitem").filter({hasText: "catalog.review"});
    await second.getByText("Детали запроса", {exact: true}).click();
    await second.getByRole("button", {name: "Отклонить", exact: true}).click();
    await expect(section.getByRole("status")).toHaveText("Действие отклонено.");
    await expect(section.getByText("Нет запросов, ожидающих подтверждения.")).toBeVisible();
    expect(decisions).toEqual([
      {id: approvalId, body: {decision: "approved", parameters_hash: `synthetic-hash-${approvalId}`}, csrf, origin},
      {id: secondId, body: {decision: "rejected", parameters_hash: `synthetic-hash-${secondId}`}, csrf, origin},
    ]);
    expect((await page.request.post(origin + root(owner))).status()).toBe(404);
    expect((await page.request.get(`${origin}${root(owner)}/${approvalId}/decision`)).status()).toBe(404);
  });

  test("recovers list errors and conflicts, disables expired requests and resets on store switch", async ({page}) => {
    listStatus = 503;
    decisionStatus = 409;
    items[1] = approval(secondId, "catalog.review", "pending", Date.now() - 1000);
    await page.goto(`${origin}/dashboard/ai-settings?org=${owner}`);
    const section = page.getByRole("region", {name: "Требуют подтверждения"});
    const refresh = section.getByRole("button", {name: "Обновить запросы"});
    await expect(section.getByRole("alert")).toContainText("Не удалось загрузить");
    listStatus = 200;
    await refresh.click();
    const expired = section.getByRole("listitem").filter({hasText: "catalog.review"});
    await expired.getByText("Детали запроса", {exact: true}).click();
    await expect(expired.getByText("Срок истёк")).toBeVisible();
    await expect(expired.getByRole("button", {name: "Подтвердить", exact: true})).toBeDisabled();

    const first = section.getByRole("listitem").filter({hasText: "catalog.update"});
    await first.getByText("Детали запроса", {exact: true}).click();
    await first.getByRole("button", {name: "Отклонить", exact: true}).click();
    await expect(section.getByRole("alert")).toContainText("Обновите список");
    await expect(first.getByRole("button", {name: "Отклонить", exact: true})).toBeDisabled();
    expect(decisions).toHaveLength(1);
    await refresh.click();
    await first.getByText("Детали запроса", {exact: true}).click();
    await expect(first.getByRole("button", {name: "Отклонить", exact: true})).toBeEnabled();

    await page.getByLabel("Активный магазин").selectOption(member);
    await expect(section.getByText("member.action")).toBeVisible();
    await expect(section.getByText("catalog.update")).toHaveCount(0);
    await section.getByText("Детали запроса", {exact: true}).click();
    await expect(section.getByRole("button", {name: "Подтвердить", exact: true})).toHaveCount(0);
  });
});
