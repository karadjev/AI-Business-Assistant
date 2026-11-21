import {randomUUID} from "node:crypto";
import {readFileSync} from "node:fs";
import {expect, test, type Page} from "@playwright/test";

const fixturePath = process.env.ABA_BUSINESS_E2E_FIXTURE;
const web = process.env.SMOKE_WEB_URL;
type Task = {id: string; agent_id: string; status: string; result?: unknown};
type Approval = {id: string; task_id: string; parameters_hash: string; status: string};
type Draft = {id: string; version: number; status: string; content: string; reason?: string};

async function request<T>(page: Page, path: string, expected: number, data?: unknown): Promise<T> {
  const csrf = (await page.context().cookies(web)).find(cookie => cookie.name === "aba_csrf");
  expect(csrf).toBeDefined();
  const response = await page.evaluate(async ({path, data, token}) => {
    const response = await fetch(path, data === undefined ? undefined : {
      method: "POST",
      headers: {"Content-Type": "application/json", "X-CSRF-Token": token},
      body: JSON.stringify(data),
    });
    return {status: response.status, text: await response.text()};
  }, {path, data, token: csrf!.value});
  expect(response.status, path).toBe(expected);
  return response.text ? JSON.parse(response.text) as T : undefined as T;
}

test.describe("Stage 2.6 marketing decisions through the real BFF", () => {
  test.skip(!fixturePath || !web, "Requires the isolated Stage 2.6 runner");
  for (const decision of ["approved", "rejected"] as const) {
    test(`persists the ${decision} marketing outcome and its audit history`, async ({page}) => {
      test.setTimeout(60_000);
      if (decision === "rejected") await page.setViewportSize({width: 390, height: 844});
      const {auto_parts: shop} = JSON.parse(readFileSync(fixturePath!, "utf8")) as {
        auto_parts: {id: string; owner: {email: string; password: string}};
      };
      await page.goto("/login");
      await page.getByLabel("Email", {exact: true}).fill(shop.owner.email);
      await page.getByLabel("Пароль", {exact: true}).fill(shop.owner.password);
      await page.getByRole("button", {name: "Войти", exact: true}).click();
      await expect(page).toHaveURL(/\/dashboard$/);

      const path = `/api/v1/organizations/${shop.id}`;
      const tasks = async () => (await request<{items: Task[]}>(page, `${path}/agent-tasks`, 200)).items;
      const existing = new Set((await tasks()).map(task => task.id));
      const parent = await request<{id: string}>(page, `${path}/agent-tasks`, 202, {
        agent_id: "business_manager", action: "coordinate_growth", idempotency_key: randomUUID(),
      });
      const marketing = (await tasks()).find(task => task.agent_id === "marketing" && !existing.has(task.id));
      expect(marketing).toBeDefined();
      const approval = (await request<{items: Approval[]}>(page, `${path}/agent-approvals`, 200))
        .items.find(item => item.task_id === marketing!.id);
      expect(approval).toMatchObject({status: "pending", parameters_hash: expect.stringMatching(/^[a-f0-9]{64}$/)});
      await request(page, `${path}/agent-approvals/${approval!.id}/decision`, 204, {
        decision, parameters_hash: approval!.parameters_hash,
      });
      for (const action of ["publish", "send"]) {
        await request(page, `${path}/agent-tasks`, 422, {
          agent_id: "marketing", action, idempotency_key: randomUUID(),
        });
      }
      await expect.poll(async () => (await tasks()).find(task => task.id === parent.id)?.status, {timeout: 25_000})
        .toBe("completed");
      const outcome = (await tasks()).find(task => task.id === marketing!.id);
      expect(outcome?.status).toBe(decision === "approved" ? "completed" : "cancelled");
      const drafts = async () => (await request<{items: Draft[]}>(page, `${path}/marketing-drafts`, 200))
        .items.filter(draft => draft.id === marketing!.id);
      if (decision === "approved") {
        expect(outcome?.result).toEqual([expect.objectContaining({
          generation: "synthetic_contextual", draft: true, external_communication: false,
          evidence: expect.objectContaining({industry: "auto_parts"}),
          goal: expect.any(String), audience: expect.any(String), channel: expect.any(String),
          idea: expect.any(String), rationale: expect.any(String), assumptions: expect.any(String),
          required_approvals: expect.any(String), text: expect.any(String),
        })]);
        const [original] = await drafts();
        expect(original).toMatchObject({version: 1, status: "pending", content: expect.any(String)});
        const review = `${path}/marketing-drafts/${marketing!.id}/review`;
        const reject = {version: 1, action: "rejected", reason: "Synthetic launch declined"};
        await request(page, review, 204, reject);
        await request(page, review, 409, reject);
        const content = "Synthetic revised campaign";
        await request(page, review, 204, {version: 1, action: "revise", content});
        await request(page, review, 409, {version: 1, action: "approved"});
        await request(page, review, 204, {version: 2, action: "approved"});
        expect(await drafts()).toEqual([
          expect.objectContaining({version: 2, status: "approved", content}),
          expect.objectContaining({version: 1, status: "rejected", reason: reject.reason, content: original.content}),
        ]);
      } else {
        expect(outcome?.result == null).toBe(true);
        expect(await drafts()).toEqual([]);
      }
      const audit = await request<{items: unknown[]}>(page, `${path}/agent-audit?task_id=${marketing!.id}`, 200);
      expect(audit.items).toContainEqual(expect.objectContaining({action: "approval.decision", result: decision}));
    });
  }
});
