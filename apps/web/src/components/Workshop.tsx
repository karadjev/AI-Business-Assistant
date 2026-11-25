"use client";

import Link from "next/link";
import {useCallback, useEffect, useRef, useState, type FormEvent} from "react";
import type {WorkshopOverview} from "@/lib/workshop-overview";
import {csrf} from "./IdentityForm";
import {minor, money} from "@/lib/catalog";
import {WorkshopFinanceChart, type FinancePeriod} from "./WorkshopFinanceChart";
import {ManagerTaskList, WorkshopManager, type ManagerTask} from "./WorkshopManager";

type AgentResult = {id: string; agent: string; status: string; result: {message?: string} | null};
type Result = {state: string; part_name?: string; sku?: string; available?: number | null;
  message?: string; children?: AgentResult[]};
type FinancialTotals = {service_minor: number; parts_minor: number; cash_minor: number; gross_minor: number};
type FinancialReport = Result & {financial?: {finance: FinancialTotals; as_of: string;
  analytics: FinancialTotals & {completed_count: number; recorded_count: number; paid_count: number}}};
type MarketingDraft = {id: string; version: number; status: string; goal: string; content: string};
type MarketingReport = Result & {marketing_evidence?: {customer_completed: number; active_services: number;
  available_parts: number; financial: {finance: FinancialTotals; analytics: FinancialTotals}}};
type MarketingAction = {action: string; reference?: string; draft_id?: string; version?: number};
type Request = {
  marketing_consent: boolean; marketing_supported: boolean; marketing_status: string | null;
  marketing_report: MarketingReport | null; marketing_draft: MarketingDraft | null;
  financial_service_minor: number | null; financial_part_cost_minor: number | null;
  financial_paid_at: string | null; financial_status: string | null; financial_report: FinancialReport | null;
  id: string; final_task_id: string | null; final_action: string | null;
  final_outcome: Result | null; close_kind: string | null; reserved_at: string | null; customer: string; vehicle: string; service: string; quantity: number;
  status: string; proposal: Result | null; outcome: Result | null; approval_id: string | null;
  parameters_hash: string | null; expires_at: string | null; approval_status: string | null;
};
type Workspace = {
  overview: WorkshopOverview|null;
  finance_periods?: FinancePeriod[];
  tasks: ManagerTask[];
  enabled: boolean;
  customers: {id: string; name: string}[];
  parts: {id: string; name: string; sku: string}[];
  requests: Request[];
  audit: {id: string; request_id: string; action: string; result: string; created_at: string}[];
};

const specialists: Record<string, string> = {customer: "Customer", product_inventory: "Product & Inventory", sales: "Sales", finance: "Finance", analytics: "Analytics", marketing: "Marketing"};
const states: Record<string, string> = {
  queued: "В очереди", running: "Проверяется", partial_failure: "Проверки завершены частично", customer_unavailable: "Карточка клиента недоступна", approval_required: "Нужно согласование",
  approved: "Согласовано, ожидает обработки", released: "Заявка отменена, резерв освобождён",
  consumed: "Обслуживание завершено, деталь списана", reservation_conflict: "Резерв требует сверки остатков", reserved: "Деталь зарезервирована",
  part_unavailable: "Деталь недоступна", source_changed: "Карточка детали изменилась. Создайте новую заявку.", stock_stale: "Нужно сверить остаток",
  insufficient_stock: "Недостаточно доступных деталей", expired: "Срок истёк",
  rejected: "Отклонено владельцем", execution_failed: "Резерв не оформлен",
  failed: "Ошибка обработки", cancelled: "Отменено", completed: "Обработано",
};
const finalLabels: Record<string, string> = {workshop_release: "Подтвердить отмену и освобождение",
  workshop_consume: "Подтвердить завершение и списание"};
const auditActions: Record<string, string> = {
  "task.submit": "Поручение принято", "task.retry": "Проверка поставлена на повтор", "task.delegate": "Поручение делегировано", "task.dispatch": "Задание передано специалисту",
  "delegation.workshop_customer": "Проверка карточки клиента", "delegation.workshop_stock": "Проверка запчасти",
  "delegation.workshop_sales": "Подготовка предложения", delegate_workshop: "Итог управляющего", "task.claim": "Задание получено worker",
  workshop_inspect: "Проверка остатка", workshop_reserve: "Оформление резерва",
  "workshop.financials.record": "Записаны финансовые данные", "workshop.payment.record": "Записана оплата",
  "delegation.workshop_finance": "Расчёт Finance", "delegation.workshop_analytics": "Расчёт Analytics",
  delegate_workshop_finance: "Финансовый итог управляющего",
  "workshop.consent": "Согласие на маркетинговые сообщения", "draft.persist": "Черновик сохранён",
  "draft.review": "Решение по маркетинговому предложению", delegate_workshop_marketing: "Маркетинговый итог управляющего",
  "delegation.workshop_marketing": "Анализ Marketing",
  workshop_release: "Освобождение резерва", workshop_consume: "Списание детали",
  "approval.request": "Запрошено согласование", "approval.decision": "Решение владельца",
  "approval.expire": "Согласование истекло", "task.finish": "Обработка завершена",
  "task.recover": "Восстановление задания", "task.interrupt": "Попытка прервана",
};

function requestState(item: Request) {
  if (item.close_kind === "release") return "released";
  if (item.close_kind === "consume") return "consumed";
  if (item.final_task_id) {
    return item.final_outcome?.state ?? (item.approval_status === "pending" ? "approval_required" : item.approval_status ?? "reserved");
  }
  return item.outcome?.state ?? (item.approval_status && item.approval_status !== "pending"
    ? item.approval_status : item.proposal?.state ?? item.status);
}

export function Workshop({org, name, timezone}: {org: string; name: string; timezone:string}) {
  const root = `/api/v1/organizations/${org}/workshop`;
  const [data, setData] = useState<Workspace>();
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const submission = useRef<{fingerprint: string; key: string} | null>(null);

  const load = useCallback(async (signal?: AbortSignal) => {
    const response = await fetch(root, {cache: "no-store", signal: signal ?? AbortSignal.timeout(8000)});
    if (!response.ok) throw new Error("Не удалось прочитать данные автосервиса.");
    setData(await response.json() as Workspace);
  }, [root]);

  const pending = !data || data.tasks.some(task => ["queued","pending","running"].includes(task.status)) || data.requests.some(item =>
    ["queued", "running", "approved"].includes(requestState(item)) || financialPending(item) || marketingPending(item));
  useEffect(() => {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      try {
        await load(AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]));
        if (pending && !controller.signal.aborted) timer = setTimeout(poll, 3000);
      } catch {
        if (!controller.signal.aborted) setError("Не удалось обновить заявки. Используйте кнопку «Обновить».");
      }
    };
    void poll();
    return () => {controller.abort(); clearTimeout(timer);};
  }, [load, pending]);

  async function mutate(path: string, body: unknown, succeeded?: () => void) {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      const response = await fetch(root + path, {
        method: "POST", headers: {"Content-Type": "application/json", "X-CSRF-Token": csrf()},
        body: JSON.stringify(body), signal: AbortSignal.timeout(8000),
      });
      if (!response.ok) {
        const result = await response.json();
        throw new Error(result.error?.message ?? "Не удалось сохранить действие.");
      }
      succeeded?.();
      await load();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Проверьте состояние заявки перед повтором.");
    } finally {
      setBusy(false);
    }
  }

  function createCustomer(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    void mutate("/customers", {name: new FormData(form).get("name")}, () => form.reset());
  }

  function createRequest(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const fields = new FormData(form);
    const body = {customer_id: fields.get("customer"), variant_id: fields.get("part"),
      quantity: Number(fields.get("quantity")), vehicle: fields.get("vehicle"), service: fields.get("service")};
    const fingerprint = JSON.stringify(body);
    if (submission.current?.fingerprint !== fingerprint) {
      submission.current = {fingerprint, key: crypto.randomUUID()};
    }
    void mutate("/requests", {...body, idempotency_key: submission.current.key}, () => {
      submission.current = null;
      form.reset();
    });
  }

  return <section className="workshop-workspace">
    <h2>{name}</h2>
    <p>Рабочие данные организации</p>
    {error && <p role="alert">{error}</p>}
    <button type="button" disabled={busy} onClick={() => {
      setError("");
      void load().catch(() => setError("Не удалось обновить заявки."));
    }}>Обновить</button>
    {!data && !error && <p role="status">Загрузка заявок…</p>}
    {data && !data.enabled && <p>Для этой организации обработка рабочих заявок пока недоступна.</p>}
    {data?.enabled && <>
      <nav aria-label="Управление автосервисом">
        <a href="#workshop-manager">Управляющий</a><a href="#workshop-finance">Финансы</a>
        <a href="#workshop-alerts">Согласования</a><a href="#workshop-team">Задачи</a><a href="#workshop-requests">Заявки</a>
      </nav>
      {data.overview && <section className="panel workshop-overview" aria-label="Сводка организации">
        <h3>Сводка организации</h3>
        <dl>
          <dt>Клиентов</dt><dd>{data.overview.customers}</dd>
          <dt>Заявок</dt><dd>{data.overview.requests}</dd>
          <dt>Обслуживаний завершено</dt><dd>{data.overview.completed}</dd>
          <dt>Деталей со свежим доступным остатком</dt><dd>{data.overview.available_skus}</dd>
          <dt>Заявок с подтверждённой оплатой</dt><dd>{data.overview.paid_count}</dd>
          <dt>Подтверждённые поступления</dt><dd>{data.overview.financial_count ? money(data.overview.cash_minor) : "Финансовые данные пока не внесены"}</dd>
          <dt>Валовая прибыль по учтённым заявкам</dt><dd>{data.overview.gross_minor === null ? "Нет данных" : money(data.overview.gross_minor)}</dd>
        </dl>
      </section>}
      <WorkshopFinanceChart periods={data.finance_periods} timezone={timezone}/>
      <WorkshopManager base={root+"/manager"} tasks={data.tasks} requests={data.requests} onQueued={load}/>
      <section className="panel workshop-alerts" id="workshop-alerts" aria-label="Согласования и последние действия">
        <h3>Согласования и последние действия</h3>
        <p>Ожидают решения: {(data.overview?.approvals??0) + (data.overview?.marketing_pending??0)}.</p>
        {data.requests.filter(item => item.approval_status==="pending" || item.marketing_draft?.status==="pending").map(item =>
          <p key={item.id}><a href={`#workshop-request-${item.id}`}>{item.service}</a></p>)}
        {data.audit.length ? <ol>{data.audit.slice(0,5).map(event=><li key={event.id}>
          <a href={`#workshop-request-${event.request_id}`}>{auditActions[event.action]??event.action}</a> · {states[event.result]??event.result}
        </li>)}</ol> : <p>Последних действий пока нет.</p>}
      </section>
      <ManagerTaskList tasks={data.tasks}/>
      <section className="panel workshop-intake">
      <h3>Карточка клиента</h3>
      <form className="identity-form" onSubmit={createCustomer}>
        <label>Имя клиента<input name="name" maxLength={120} required/></label>
        <button disabled={busy}>Добавить клиента</button>
      </form>
      <h3>Новая заявка на обслуживание</h3>
      {data.parts.length === 0 && <p>Добавьте и активируйте деталь в каталоге, затем укажите её остаток.
        {" "}<Link href={`/dashboard/products?org=${org}`}>Открыть каталог</Link></p>}
      <form className="identity-form" onSubmit={createRequest}>
        <label><span id="workshop-customer-label">Клиент</span><select aria-labelledby="workshop-customer-label" name="customer" required defaultValue=""><option value="" disabled>Выберите клиента</option>
          {data.customers.map(item => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label>
        <label>Автомобиль<input name="vehicle" maxLength={120} required placeholder="Марка, модель, год"/></label>
        <label>Услуга<input name="service" maxLength={160} required/></label>
        <label><span id="workshop-part-label">Деталь</span><select aria-labelledby="workshop-part-label" name="part" required defaultValue=""><option value="" disabled>Выберите деталь</option>
          {data.parts.map(item => <option key={item.id} value={item.id}>{item.name} · {item.sku}</option>)}</select></label>
        <label>Количество<input name="quantity" type="number" min={1} max={10} defaultValue={1} required/></label>
        <p>Вы выбираете деталь. Агент проверит остаток и предложит резерв. Проверьте совместимость перед согласованием.</p>
        <button disabled={busy || !data.customers.length || !data.parts.length}>Поручить AI-управляющему</button>
      </form>
      </section>
      <h3 id="workshop-requests">Заявки и согласования</h3>
      {data.requests.length === 0 && <p>Заявок пока нет.</p>}
      {data.requests.map(item => <article className="panel workshop-request" key={item.id} id={`workshop-request-${item.id}`}>
        <h4>{item.service} · {item.vehicle}</h4>
        <p>{item.customer} · {states[requestState(item)] ?? requestState(item)}</p>
        {item.proposal?.message && <p><strong>Итог AI-управляющего.</strong> {item.proposal.message}</p>}
        {!!item.proposal?.children?.length && <details>
          <summary>Результаты специалистов</summary>
          <ul>{item.proposal.children.map(child => <li key={child.id}>
            <strong>{specialists[child.agent] ?? child.agent}</strong>{" · "}
            {child.result?.message ?? states[child.status] ?? child.status}
          </li>)}</ul>
        </details>}
        {item.proposal?.part_name && <p>{item.proposal.part_name} · {item.proposal.sku} · {item.quantity} шт.
          {item.proposal.available != null && ` · При проверке доступно: ${item.proposal.available}`}</p>}
        {item.reserved_at && !item.close_kind && !["pending", "approved"].includes(item.approval_status ?? "") && <div>
          <button type="button" disabled={busy} onClick={() => void mutate(`/requests/${item.id}/close`, {operation: "release"})}>Запросить отмену заявки</button>
          <button type="button" disabled={busy} onClick={() => void mutate(`/requests/${item.id}/close`, {operation: "consume"})}>Запросить завершение обслуживания</button>
        </div>}
        {item.approval_status === "pending" && item.approval_id && item.parameters_hash && <>
          {item.expires_at && <p>Решение до {new Date(item.expires_at).toLocaleString("ru-RU")}</p>}
          <button type="button" disabled={busy} onClick={() => void mutate(`/approvals/${item.approval_id}/decision`,
            {decision: "approved", parameters_hash: item.parameters_hash})}>{item.final_action ? finalLabels[item.final_action] : "Подтвердить резерв"}</button>
          <button type="button" disabled={busy} onClick={() => void mutate(`/approvals/${item.approval_id}/decision`,
            {decision: "rejected", parameters_hash: item.parameters_hash})}>Отклонить</button>
        </>}
        {item.close_kind === "consume" && <WorkshopFinancials item={item} busy={busy} onRecord={(service_minor,part_cost_minor,paid) =>
          void mutate(`/requests/${item.id}/financials`, {service_minor, part_cost_minor, paid})} onError={setError}/>}
        {item.close_kind === "consume" && item.marketing_supported && <WorkshopMarketing item={item} busy={busy}
          onAction={action => void mutate(`/requests/${item.id}/marketing`, action)}/>}
        {data.audit.some(entry => item.id === entry.request_id) && <details>
          <summary>Журнал действий</summary>
          <ul>{data.audit.filter(entry => item.id === entry.request_id).map(entry =>
            <li key={entry.id}><time dateTime={entry.created_at}>{new Date(entry.created_at).toLocaleString("ru-RU")}</time>
              {" · "}{auditActions[entry.action] ?? entry.action} · {states[entry.result] ?? entry.result}</li>)}</ul>
        </details>}
      </article>)}
    </>}
  </section>;
}

function financialPending(item: Request) {
  return ["pending", "queued", "running"].includes(item.financial_status ?? "");
}

function WorkshopFinancials({item,busy,onRecord,onError}: {
  item: Request; busy: boolean; onRecord: (service: number, cost: number, paid: boolean) => void;
  onError: (message: string) => void;
}) {
  const service = item.financial_service_minor;
  const cost = item.financial_part_cost_minor;
  const report = item.financial_report?.financial;
  function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const values = new FormData(event.currentTarget);
    try {
      onRecord(minor(String(values.get("service"))), minor(String(values.get("cost"))), values.get("payment") === "paid");
    } catch (error) {
      onError(error instanceof Error ? error.message : "Проверьте суммы.");
    }
  }

  return <section aria-label="Финансы заявки">
    <h5>Финансы заявки</h5>
    {service === null || cost === null ? <form onSubmit={save}>
      <label>Стоимость услуги, ₽<input name="service" inputMode="decimal" required/></label>
      <label>Себестоимость всех списанных деталей, ₽<input name="cost" inputMode="decimal" required/></label>
      <label>Оплата<select aria-label="Оплата" name="payment" required defaultValue="">
        <option value="" disabled>Выберите статус</option><option value="unpaid">Не оплачено</option><option value="paid">Оплачено полностью</option>
      </select></label>
      <button disabled={busy}>Записать и подготовить отчёт</button>
    </form> : <>
      <p>Стоимость услуги {money(service)} · Себестоимость деталей {money(cost)} · {item.financial_paid_at ? "Оплачено" : "Не оплачено"}</p>
      {!item.financial_paid_at && <button type="button" disabled={busy || financialPending(item)}
        onClick={() => onRecord(service,cost,true)}>Подтвердить получение полной оплаты</button>}
    </>}
    {financialPending(item) && <p role="status">Finance и Analytics готовят отчёт…</p>}
    {item.financial_report?.message && <p>{item.financial_report.message}</p>}
    {item.financial_status === "failed" && !report && <p role="alert">Отчёт не сформирован. Записанные данные сохранены.</p>}
    {report && <>
      <p>Получено по заявке {money(report.finance.cash_minor)} · Валовая прибыль по заявке {money(report.finance.gross_minor)}</p>
      <details><summary>Показатели организации</summary>
        <p>Снимок на {new Date(report.as_of).toLocaleString("ru-RU")}. Завершено {report.analytics.completed_count},
          финансовые данные внесены для {report.analytics.recorded_count}, оплачено {report.analytics.paid_count}.</p>
        <p>Стоимость учтённых услуг {money(report.analytics.service_minor)} · Себестоимость деталей {money(report.analytics.parts_minor)}.</p>
        <p>Получено по организации {money(report.analytics.cash_minor)} · Валовая прибыль по учтённым заявкам {money(report.analytics.gross_minor)}.</p>
      </details>
    </>}
  </section>;
}

function marketingPending(item: Request) {
  return ["pending", "queued", "running"].includes(item.marketing_status ?? "");
}

function WorkshopMarketing({item,busy,onAction}: {item: Request; busy: boolean; onAction: (action: MarketingAction) => void}) {
  const draft = item.marketing_draft;
  const ready = item.marketing_report?.state === "marketing_review" && !!draft;
  const evidence = item.marketing_report?.marketing_evidence;
  return <section aria-label="Маркетинг заявки">
    <h5>Маркетинг заявки</h5>
    {item.marketing_consent ? <>
      <p>Согласие клиента записано.</p>
      <button type="button" disabled={busy} onClick={() => onAction({action:"revoke"})}>Отозвать согласие</button>
      {!draft && item.financial_report?.state === "financial_report_ready" && <button type="button" disabled={busy || marketingPending(item)}
        onClick={() => onAction({action:"prepare"})}>{item.marketing_status === "failed" ? "Повторить маркетинговый анализ" : "Подготовить маркетинговое предложение"}</button>}
    </> : <form onSubmit={event => {
      event.preventDefault();
      const reference = String(new FormData(event.currentTarget).get("reference") ?? "");
      onAction({action:"consent", reference});
    }}>
      <label>Внутренняя ссылка на согласие клиента<input name="reference" maxLength={120} required/></label>
      <label><input name="confirmed" type="checkbox" required/>Клиент дал согласие на маркетинговые сообщения</label>
      <button disabled={busy}>Записать согласие</button>
    </form>}
    {marketingPending(item) && <p role="status">Marketing готовит предложение для проверки владельцем…</p>}
    {item.marketing_consent && item.marketing_report?.state === "marketing_unavailable" && <p>{item.marketing_report.message}</p>}
    {item.marketing_consent && ready && draft && <div>
      <strong>{draft.goal}</strong>
      <p>{item.marketing_report?.message}</p>
      {evidence && <p>Завершённых обслуживаний клиента {evidence.customer_completed} · Активных услуг {evidence.active_services} ·
        Деталей со свежим остатком {evidence.available_parts} · Валовая прибыль по заявке {money(evidence.financial.finance.gross_minor)} ·
        Получено по учтённым заявкам {money(evidence.financial.analytics.cash_minor)}</p>}
      <p><strong>Черновик для внутреннего согласования</strong></p>
      <p>{draft.content}</p>
      {draft.status === "pending" ? <>
        <button type="button" disabled={busy} onClick={() => onAction({action:"approved", draft_id:draft.id, version:draft.version})}>Согласовать предложение</button>
        <button type="button" disabled={busy} onClick={() => onAction({action:"rejected", draft_id:draft.id, version:draft.version})}>Отклонить предложение</button>
      </> : <p role="status">{draft.status === "approved" ? "Предложение согласовано" : "Предложение отклонено"}</p>}
    </div>}
  </section>;
}
