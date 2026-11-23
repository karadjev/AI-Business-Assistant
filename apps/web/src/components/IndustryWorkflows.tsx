"use client";

import Link from "next/link";
import {useRef, useState, type FormEvent} from "react";
import {channelLabels} from "./ProductUI";
import {Alert, Button, Input, Select} from "./UI";
import {displayDate} from "@/lib/presentation";
import {buildRequirements, parseOfferings, readID, record, scenarios,
  type IndustryTask, type Offering, type Scenario} from "@/lib/industry-workflows";

type Conversation = {id: string; channel: string; updated_at?: string};
type Submission = {id: string; scenario: Scenario};

const summaries = new Map([
  ["approval_required", "Предложение готово. Решение принимает сотрудник."],
  ["proposal_ready", "Предложение подготовлено."],
  ["clarification_required", "Для расчёта нужно уточнить параметры."],
  ["slot_unavailable", "Выбранное время недоступно."],
  ["service_unavailable", "Услуга недоступна."],
  ["part_unavailable", "Подходящая деталь недоступна."],
  ["part_requirement_unknown", "Для услуги нужно уточнить правило по запчастям."],
  ["part_decision_required", "Нужно уточнить, требуется ли запчасть."],
  ["material_unavailable", "В каталоге нет подтверждённого материала."],
  ["calculation_invalid", "Проверьте размеры и параметры расчёта."],
  ["packaging_unverified", "Размер упаковки не подтверждён."],
  ["stock_unverified_or_insufficient", "Наличие не подтверждено или запаса недостаточно."],
  ["source_changed", "Исходные данные изменились. Найдите актуальную услугу или позицию и создайте задачу заново."],
  ["no_properties", "Подходящих актуальных объектов не найдено."],
  ["no_verified_offer", "Нет детали с подтверждённой совместимостью и достаточным остатком."],
  ["partial_failure", "Часть работы не завершена. Подробности есть в истории задачи."],
]);

function failedMessage(error: unknown) {
  if (error instanceof TypeError || error instanceof DOMException) {
    return "Сервер не ответил. Проверьте историю задач или повторите действие с теми же данными.";
  }
  return error instanceof Error ? error.message : "Операция недоступна.";
}

async function requestJSON(path: string, csrf: string, data?: unknown): Promise<unknown> {
  const response = await fetch(path, {
    method: data === undefined ? "GET" : "POST",
    headers: data === undefined ? undefined : {"Content-Type": "application/json", "X-CSRF-Token": csrf},
    body: data === undefined ? undefined : JSON.stringify(data),
    cache: "no-store",
    signal: AbortSignal.timeout(10_000),
  });
  const payload: unknown = await response.json().catch(() => null);
  if (!response.ok) {
    const message = record(record(payload)?.error)?.message;
    throw new Error(typeof message === "string" ? message : `Операция недоступна (HTTP ${response.status}).`);
  }
  return payload;
}

function parseConversations(value: unknown): Conversation[] {
  const items = record(value)?.items;
  if (!Array.isArray(items)) throw new Error("Не удалось прочитать список диалогов.");
  return items.flatMap((item: unknown) => {
    const row = record(item);
    const id = readID(item);
    return id && row?.status === "open" && typeof row.channel === "string"
      ? [{id, channel: row.channel, updated_at: typeof row.updated_at === "string" ? row.updated_at : undefined}]
      : [];
  });
}

function VehicleFields({service}: {service: boolean}) {
  return <>
    <label>Марка<Input name="make" required maxLength={80}/></label>
    <label>Модель<Input name="model" required maxLength={80}/></label>
    <label>Двигатель<Input name="engine" required maxLength={80}/></label>
    <label>Год выпуска<Input name="year" type="number" min={1886} max={2100} required/></label>
    <label>{service ? "Артикул запчасти, если уже известен" : "Артикул или OEM"}<Input name="oem" maxLength={80} required={!service}/></label>
    <label>Количество деталей<Input name="quantity" type="number" min={1} max={10} defaultValue={1} required/></label>
  </>;
}

function ConstructionFields() {
  return <>
    <label>Материал по SKU<Input name="sku" required maxLength={80}/></label>
    <label>Вид расчёта<Select name="calculation_kind"><option value="tiles">Плитка</option><option value="blocks">Блоки</option></Select></label>
    <label>Длина поверхности, м<Input name="length" inputMode="decimal" required maxLength={16}/></label>
    <label>Ширина поверхности, м<Input name="width" inputMode="decimal" required maxLength={16}/></label>
    <label>Длина одной единицы, м<Input name="item_length" inputMode="decimal" required maxLength={16}/></label>
    <label>Ширина одной единицы, м<Input name="item_width" inputMode="decimal" required maxLength={16}/></label>
    <label>Запас, %<Input name="waste_percent" inputMode="decimal" defaultValue="0" required maxLength={14}/></label>
  </>;
}

function PropertyFields() {
  return <>
    <label>Город<Input name="city" required maxLength={120}/></label>
    <label>Тип объекта<Select name="property_type">
      <option value="">Любой</option><option value="apartment">Квартира</option><option value="house">Дом</option>
      <option value="commercial">Коммерческая недвижимость</option><option value="land">Участок</option>
      <option value="new_development">Новостройка</option>
    </Select></label>
    <label>Сделка<Select name="transaction_type">
      <option value="">Любая</option><option value="sale">Покупка</option>
      <option value="long_term_rental">Долгосрочная аренда</option><option value="short_term_rental">Краткосрочная аренда</option>
    </Select></label>
    <label>Бюджет до, ₽<Input name="budget" type="number" min={0} step={1}/></label>
    <label>Пожелания<Input name="wishes" maxLength={350}/></label>
  </>;
}

function Outcome({value}: {value: Record<string, unknown>}) {
  const calculation = record(value.calculation);
  const total = record(record(value.proposal)?.estimated_total);
  const price = typeof total?.amount === "number" && Number.isSafeInteger(total.amount)
    && typeof total.currency === "string" && /^[A-Z]{3}$/.test(total.currency)
    ? new Intl.NumberFormat("ru-RU", {style: "currency", currency: total.currency}).format(total.amount / 100) : undefined;
  const partsProposal = Array.isArray(value.children)
    ? value.children.map(record).find(child => child?.action === "delegation.proposal" && child.status === "completed")
    : undefined;
  const partsResult = record(partsProposal?.result);
  const proposal = record(value.proposal);
  const proposedVariants = Array.isArray(proposal?.items)
    ? new Set(proposal.items.map(item => record(item)?.variant_id)) : new Set<unknown>();
  const selectedParts: unknown[] = partsResult?.state === "proposal_ready" && Array.isArray(partsResult.parts)
    ? partsResult.parts.filter((part: unknown) => {
      const row = record(part);
      return typeof row?.variant_id === "string" && row.compatibility === "compatible" && proposedVariants.has(row.variant_id);
    }) : [];
  const items: unknown[] = Array.isArray(value.properties) ? value.properties : selectedParts;
  const names = items.flatMap(item => {
    const row = record(item);
    const name = row?.name ?? row?.title;
    const available = row?.available;
    return typeof name === "string" ? [typeof available === "number" ? `${name} · доступно ${available}` : name] : [];
  });
  return <>
    <p>{typeof value.state === "string" ? summaries.get(value.state) ?? "Результат сохранён в истории задачи." : "Результат сохранён."}</p>
    {typeof calculation?.package_count === "number" && <p>Упаковок: {calculation.package_count}</p>}
    {price && <p>Оценочная стоимость: {price}</p>}
    {names.length > 0 && <ul>{names.map((name, index) => <li key={index}>{name}</li>)}</ul>}
  </>;
}

export function AutoServiceApproval({organizationId, taskId, getCsrfToken}: {
  organizationId: string; taskId: string; getCsrfToken: () => string;
}) {
  const [orderId, setOrderId] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const preparing = useRef(false);
  async function prepare() {
    if (preparing.current) return;
    const csrf = getCsrfToken();
    if (!csrf) {setError("Войдите снова для продолжения."); return;}
    preparing.current = true;
    setBusy(true);
    setError("");
    try {
      const path = `/api/v1/organizations/${organizationId}/agent-delegations/auto-service/${taskId}/prepare`;
      const id = readID(await requestJSON(path, csrf, {}));
      if (!id) throw new Error("Номер заявки не получен.");
      setOrderId(id);
    } catch (failure) {
      setError(failedMessage(failure));
    } finally {
      preparing.current = false;
      setBusy(false);
    }
  }
  return <div>
    {orderId
      ? <p><Link href={`/dashboard/orders/${orderId}?org=${organizationId}`}>Открыть заявку для подтверждения</Link></p>
      : <Button disabled={busy} onClick={() => void prepare()}>
        {busy ? "Готовим заявку…" : "Подготовить заявку к согласованию"}
      </Button>}
    {error && <Alert tone="danger">{error}</Alert>}
  </div>;
}

export function IndustryWorkflows({organizationId, timezone, getCsrfToken, onSubmitTask, task}: {
  organizationId: string; timezone: string; getCsrfToken: () => string;
  onSubmitTask: (id: string) => void; task?: IndustryTask;
}) {
  const root = `/api/v1/organizations/${organizationId}`;
  const [scenario, setScenario] = useState<Scenario>("auto-service");
  const [dialogues, setDialogues] = useState<Conversation[]>();
  const [offerings, setOfferings] = useState<Offering[]>();
  const [offeringId, setOfferingId] = useState("");
  const [submission, setSubmission] = useState<Submission>();
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const locked = useRef(false);
  const pending = useRef(new Map<string, string>());
  const offering = offerings?.find(value => value.id === offeringId);
  const result = task?.id === submission?.id && task?.status === "completed" && Array.isArray(task.result)
    ? record(task.result[0]) : undefined;

  async function perform(label: string, action: () => Promise<void>) {
    if (locked.current) return;
    locked.current = true;
    setBusy(label);
    setError("");
    try { await action(); }
    catch (failure) { setError(failedMessage(failure)); }
    finally { locked.current = false; setBusy(""); }
  }

  function post(path: string, input: unknown) {
    const csrf = getCsrfToken();
    if (!csrf) throw new Error("Войдите снова для продолжения.");
    return requestJSON(root + path, csrf, input);
  }

  function search(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const query = new FormData(event.currentTarget).get("query");
    void perform("Ищем услуги…", async () => {
      setOfferings(undefined);
      setOfferingId("");
      const matches = parseOfferings(await post("/operations/services/search", {query, limit: 20}));
      setOfferings(matches);
      if (matches.length === 1) setOfferingId(matches[0].id);
    });
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    void perform("Создаём задачу…", async () => {
      const conversation = form.get("conversation");
      if (typeof conversation !== "string" || !dialogues?.some(row => row.id === conversation)) {
        throw new Error("Выберите открытый диалог.");
      }
      const input = {conversation_id: conversation, requirements: buildRequirements(scenario, form, offering)};
      const fingerprint = JSON.stringify([scenario, input]);
      const key = pending.current.get(fingerprint) ?? crypto.randomUUID();
      pending.current.set(fingerprint, key);
      const id = readID(await post(`/agent-delegations/${scenario}`, {...input, idempotency_key: key}));
      if (!id) throw new Error("Номер задачи не получен. Проверьте историю перед повторной отправкой.");
      pending.current.delete(fingerprint);
      setSubmission({id, scenario});
      onSubmitTask(id);
    });
  }

  return <section className="module-card" aria-labelledby="industry-workflows-title">
    <h2 id="industry-workflows-title">Отраслевые задачи</h2>
    <label>Сценарий<Select value={scenario} disabled={!!busy} onChange={event => {
      const next = scenarios.find(item => item.id === event.target.value);
      if (next) {setScenario(next.id); setError("");}
    }}>{scenarios.map(item => <option key={item.id} value={item.id}>{item.label}</option>)}</Select></label>
    {dialogues === undefined
      ? <p><Button disabled={!!busy} onClick={() => void perform("Загружаем диалоги…", async () => {
        setDialogues(parseConversations(await requestJSON(root + "/conversations", "")));
      })}>Загрузить открытые диалоги</Button></p>
      : dialogues.length === 0 && <p>Открытых диалогов нет. <Link href={`/dashboard/conversations?org=${organizationId}`}>Перейти в диалоги</Link></p>}
    {dialogues && dialogues.length > 0 && <>
      {scenario === "auto-service" && <form className="catalog-inline" onSubmit={search}>
        <label>Название услуги<Input name="query" required maxLength={120}/></label>
        <Button type="submit" disabled={!!busy}>Найти услугу</Button>
        {offerings?.length === 0 && <p>Подходящих услуг нет.</p>}
      </form>}
      <form key={scenario} className="identity-form" onSubmit={submit}>
        <label>Диалог<Select name="conversation" required defaultValue="">
          <option value="" disabled>Выберите диалог</option>
          {dialogues.map(row => <option key={row.id} value={row.id}>
            {channelLabels[row.channel] ?? row.channel} · {row.updated_at ? displayDate(row.updated_at, timezone) + " · " : ""}{row.id.slice(0, 8)}
          </option>)}
        </Select></label>
        {scenario === "construction" && <ConstructionFields/>}
        {scenario === "real-estate" && <PropertyFields/>}
        {(scenario === "auto-service" || scenario === "auto-parts") && <VehicleFields service={scenario === "auto-service"}/>}
        {scenario === "auto-service" && <>
          <label>Услуга<Select name="offering" required value={offeringId} onChange={e => setOfferingId(e.target.value)}>
            <option value="" disabled>Выберите услугу из поиска</option>
            {offerings?.map(row => <option key={row.id} value={row.id}>{row.name}</option>)}
          </Select></label>
          {offering && <>
            <label>Начало в местном времени устройства<Input name="slot" type="datetime-local" required/></label>
            <p className="field-hint">Расписание услуги ведётся в часовом поясе {offering.timezone}.</p>
            <label>Длительность, мин<Input key={offering.id} name="duration" type="number" min={offering.min_minutes}
              max={offering.max_minutes} defaultValue={offering.min_minutes} required/></label>
          </>}
          <label>Комментарий<Input name="notes" maxLength={250}/></label>
        </>}
        <Button type="submit" variant="primary" disabled={!!busy || scenario === "auto-service" && !offering}>Создать отраслевую задачу</Button>
      </form>
    </>}
    {busy && <p role="status">{busy}</p>}
    {error && <Alert tone="danger">{error}</Alert>}
    {submission && <div data-testid="industry-task">
      <p role="status">Задача принята. <a href={`#agent-task-${submission.id}`} onClick={() => {
        const details = document.getElementById(`agent-task-${submission.id}`);
        if (details instanceof HTMLDetailsElement) details.open = true;
      }}>Открыть задачу в истории</a></p>
      {result && <Outcome value={result}/>}
      {result?.state === "approval_required" && submission.scenario === "auto-service" &&
        <AutoServiceApproval key={submission.id} organizationId={organizationId} taskId={submission.id} getCsrfToken={getCsrfToken}/>}
    </div>}
  </section>;
}
