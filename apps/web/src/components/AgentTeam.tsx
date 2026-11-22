"use client";

import {useEffect, useRef, useState, type FormEvent} from "react";
import {displayDate} from "@/lib/presentation";
import {AgentApprovals} from "./AgentApprovals";
import {StatusBadge} from "./ProductUI";
import {Alert, Button, Select, Table} from "./UI";

type Agent = {
  id: string;
  name: string;
  description: string;
  permissions: string[];
  enabled: boolean;
};

type Team = {
  profile: {model: string; industry: string; revision: number; capabilities: string[]};
  agents: Agent[];
};

const industries = [
  {id: "electronics", model: "trade", label: "Электроника"},
  {id: "auto_parts", model: "trade", label: "Автозапчасти"},
  {id: "building_materials", model: "trade", label: "Строительные материалы"},
  {id: "furniture", model: "trade", label: "Мебель"},
  {id: "apparel", model: "trade", label: "Одежда"},
  {id: "vehicle_service", model: "service", label: "Автосервис"},
  {id: "personal_services", model: "service", label: "Персональные услуги"},
  {id: "real_estate", model: "agency", label: "Недвижимость"},
  {id: "mixed_nonfood", model: "hybrid", label: "Смешанный непродовольственный ассортимент"},
];

type Task = {
  id: string;
  agent_id: string;
  action: string;
  status: string;
  created_at: string;
  completed_at: string | null;
  deadline: string;
  retry_count: number;
  error_code: string | null;
  result: unknown;
};

type AuditEntry = {
  id: string;
  task_id: string;
  agent_id: string;
  action: string;
  result: string;
  error_code: string | null;
  created_at: string;
};

const bookingDeliveryResults = new Map([
  ["queued", "В очереди"],
  ["sending", "Отправляется"],
  ["pending", "Ожидается повтор отправки"],
  ["sent", "Принято провайдером"],
  ["delivered", "Доставлено"],
  ["read", "Прочитано"],
  ["failed", "Ошибка доставки"],
  ["delivery_unknown", "Доставка не подтверждена"],
  ["uncertain", "Доставка не подтверждена"],
  ["cancelled", "Отправка отменена"],
  ["superseded", "Появился более свежий статус записи"],
  ["channel_unavailable", "Канал недоступен для отправки"],
  ["instagram_send_disabled", "Отправка через Instagram отключена"],
  ["consent_required", "Требуется согласие клиента"],
  ["needs_approved_template", "Требуется согласованный шаблон WhatsApp"],
]);

const operations = [
  {id: "finance", agentId: "finance", action: "read", label: "Финансы · получить сводку"},
  {id: "analytics", agentId: "analytics", action: "read", label: "Аналитика · получить сводку"},
  {id: "business_manager", agentId: "business_manager", action: "coordinate", label: "Бизнес-менеджер · координировать команду"},
  {id: "marketing", agentId: "marketing", action: "draft", label: "Маркетинг · создать черновик"},
  {id: "coordinate_growth", agentId: "business_manager", action: "coordinate_growth", label: "Бизнес-менеджер · анализ продаж и наличия"},
] as const;

const agentNames = new Map([
  ["customer", "Клиентская поддержка"], ["sales", "Продажи"],
  ["product_inventory", "Товары и остатки"], ["finance", "Финансы"],
  ["analytics", "Аналитика"], ["marketing", "Маркетинг"],
  ["business_manager", "Бизнес-менеджер"],
]);

const taskStatuses: Record<string, string> = {
  pending: "Ожидает постановки", queued: "В очереди", running: "Выполняется",
  waiting_approval: "Требует подтверждения", completed: "Завершена",
  failed: "Ошибка", cancelled: "Отменена", expired: "Срок истёк",
};

const uuid = /^[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$/i;
const isObject = (value: unknown): value is Record<string, unknown> =>
  typeof value === "object" && value !== null && !Array.isArray(value);
const isTextList = (value: unknown): value is string[] =>
  Array.isArray(value) && value.every(item => typeof item === "string");
const isNullableText = (value: unknown): value is string | null =>
  value === null || typeof value === "string";

function isAgent(value: unknown): value is Agent {
  return isObject(value) && typeof value.id === "string" && typeof value.name === "string"
    && typeof value.description === "string" && isTextList(value.permissions) && typeof value.enabled === "boolean";
}

function parseTeam(value: unknown): Team {
  if (isObject(value) && isObject(value.profile) && typeof value.profile.model === "string"
    && typeof value.profile.industry === "string" && isTextList(value.profile.capabilities)
    && typeof value.profile.revision === "number" && Number.isSafeInteger(value.profile.revision) && value.profile.revision > 0
    && Array.isArray(value.agents) && value.agents.every(isAgent)
    && new Set(value.agents.map(agent => agent.id)).size === value.agents.length) {
    return {
      profile: {model: value.profile.model, industry: value.profile.industry, revision: value.profile.revision,
        capabilities: value.profile.capabilities},
      agents: value.agents,
    };
  }
  throw new Error("Состав команды получен в неверном формате.");
}

function isTask(value: unknown): value is Task {
  return isObject(value) && typeof value.id === "string" && typeof value.agent_id === "string"
    && typeof value.action === "string" && typeof value.status === "string" && typeof value.created_at === "string"
    && typeof value.deadline === "string" && typeof value.retry_count === "number"
    && isNullableText(value.completed_at) && isNullableText(value.error_code) && "result" in value;
}

function isAuditEntry(value: unknown): value is AuditEntry {
  return isObject(value) && typeof value.id === "string" && typeof value.task_id === "string"
    && typeof value.agent_id === "string" && typeof value.action === "string" && typeof value.result === "string"
    && isNullableText(value.error_code) && typeof value.created_at === "string";
}

function parseItems<T>(value: unknown, check: (item: unknown) => item is T): T[] {
  if (isObject(value) && Array.isArray(value.items) && value.items.every(check)) return value.items;
  throw new Error("Список получен в неверном формате.");
}

const parseTasks = (value: unknown) => parseItems(value, isTask);
const parseAudit = (value: unknown) => parseItems(value, isAuditEntry);

type Resource<T> = {data?: T; error?: string};

function useResource<T>(endpoint: string | null, version: number, parse: (value: unknown) => T) {
  const [resource, setResource] = useState<Resource<T>>({});
  useEffect(() => {
    if (!endpoint) return;
    const controller = new AbortController();
    async function load(url: string) {
      try {
        const response = await fetch(url, {cache: "no-store", signal: controller.signal});
        if (response.status === 401) throw new Error("Войдите снова, чтобы загрузить данные.");
        if (response.status === 403) throw new Error("У вас нет доступа к этим данным магазина.");
        if (!response.ok) throw new Error(`Не удалось загрузить данные (HTTP ${response.status}).`);
        const payload: unknown = await response.json().catch(() => null);
        const data = parse(payload);
        if (!controller.signal.aborted) setResource({data});
      } catch (failure) {
        const error = failure instanceof TypeError || !(failure instanceof Error)
          ? "Сервис временно недоступен." : failure.message;
        if (!controller.signal.aborted) setResource({error});
      }
    }
    void load(endpoint);
    return () => controller.abort();
  }, [endpoint, version, parse]);
  return resource;
}

function LoadFeedback({resource, label}: {resource: Resource<unknown>; label: string}) {
  if (resource.error) return <Alert tone="danger">{label}. {resource.error}</Alert>;
  if (resource.data === undefined) return <p role="status">{label} загружается…</p>;
  return null;
}

export function AgentTeam({organizationId, timezone, owner, getCsrfToken}: {
  organizationId: string;
  timezone: string;
  owner: boolean;
  getCsrfToken: () => string;
}) {
  const root = `/api/v1/organizations/${organizationId}`;
  const tasksEndpoint = `${root}/agent-tasks`;
  const [teamVersion, setTeamVersion] = useState(0);
  const [activityVersion, setActivityVersion] = useState(0);
  const [approvalVersion, setApprovalVersion] = useState(0);
  const team = useResource(owner ? `${root}/agent-team` : null, teamVersion, parseTeam);
  const tasks = useResource(owner ? tasksEndpoint : null, activityVersion, parseTasks);
  const audit = useResource(owner ? `${root}/agent-audit` : null, activityVersion, parseAudit);
  const [selected, setSelected] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [submission, setSubmission] = useState<{id: string} | {error: string} | null>(null);
  const [savingProfile, setSavingProfile] = useState(false);
  const [profileError, setProfileError] = useState("");
  const [profileNotice, setProfileNotice] = useState("");
  const inFlight = useRef(false);
  const pendingKeys = useRef(new Map<string, string>());

  const agents = team.data?.agents ?? [];
  const availableOperations = operations.filter(operation =>
    agents.some(agent => agent.id === operation.agentId && agent.enabled)
    && (operation.action !== "coordinate_growth" || team.data?.profile.capabilities.includes("growth_review")));
  const operation = availableOperations.find(operation => operation.id === selected);
  function agentName(id: string) {
    const name = agents.find(agent => agent.id === id)?.name;
    return name && name !== id ? name : agentNames.get(id) ?? id;
  }
  const refreshActivity = () => setActivityVersion(version => version + 1);

  async function updateProfile(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const profile = team.data?.profile;
    const choice = new FormData(event.currentTarget).get("industry");
    const industry = industries.find(item => item.id === choice);
    if (!owner || !profile || !industry || savingProfile || profile.industry === industry.id) return;
    const csrf = getCsrfToken();
    setProfileNotice("");
    setProfileError("");
    if (!csrf) {
      setProfileError("Обновите страницу или войдите снова, чтобы сохранить профиль.");
      return;
    }
    setSavingProfile(true);
    try {
      const response = await fetch(`${root}/agent-business-profile`, {
        method: "PATCH", headers: {"Content-Type": "application/json", "X-CSRF-Token": csrf},
        body: JSON.stringify({model: industry.model, industry: industry.id, revision: profile.revision}),
      });
      if (response.status === 409) {
        setTeamVersion(version => version + 1);
        setProfileError("Профиль изменился. Проверьте актуальные данные и повторите выбор.");
        return;
      }
      if (!response.ok) throw new Error("Не удалось сохранить отрасль.");
      setTeamVersion(version => version + 1);
      setProfileNotice("Отрасль сохранена.");
    } catch {
      setProfileError("Не удалось сохранить отрасль. Повторите попытку.");
    } finally {
      setSavingProfile(false);
    }
  }

  async function createTask(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!owner || !operation || inFlight.current) return;
    const csrfToken = getCsrfToken();
    if (!csrfToken) {
      setSubmission({error: "Обновите страницу или войдите снова, чтобы создать задачу."});
      return;
    }

    inFlight.current = true;
    setSubmitting(true);
    setSubmission(null);
    try {
      const {agentId, action} = operation;
      const idempotencyKey = pendingKeys.current.get(operation.id) ?? crypto.randomUUID();
      pendingKeys.current.set(operation.id, idempotencyKey);
      const response = await fetch(tasksEndpoint, {
        method: "POST",
        headers: {"Content-Type": "application/json", "X-CSRF-Token": csrfToken},
        body: JSON.stringify({agent_id: agentId, action, idempotency_key: idempotencyKey}),
      });
      const data: unknown = await response.json().catch(() => null);
      if (!response.ok) {
        const message = isObject(data) && isObject(data.error) && typeof data.error.message === "string"
          ? data.error.message : `Не удалось создать задачу (HTTP ${response.status}).`;
        setSubmission({error: message});
        return;
      }
      if (!isObject(data) || typeof data.id !== "string" || !uuid.test(data.id)) {
        throw new Error("Missing task ID");
      }
      pendingKeys.current.delete(operation.id);
      setSubmission({id: data.id});
      refreshActivity();
      setApprovalVersion(version => version + 1);
    } catch {
      setSubmission({error: "Не удалось подтвердить создание задачи. Повторите отправку того же действия."});
    } finally {
      inFlight.current = false;
      setSubmitting(false);
    }
  }

  return <section className="module-card agent-team" aria-labelledby="agent-team-heading" data-testid="agent-team">
    <h2 id="agent-team-heading">AI-команда</h2>
    {owner && <><div className="page-actions">
      <Button onClick={() => {
        setTeamVersion(version => version + 1);
        refreshActivity();
        setApprovalVersion(version => version + 1);
      }} data-testid="agent-team-refresh">Обновить AI-команду</Button>
    </div>
    <LoadFeedback resource={team} label="Команда"/>
    </>}
    {owner && team.data && <>
      <dl className="agent-team-details">
        <dt>Модель бизнеса</dt><dd>{team.data.profile.model || "Не указана"}</dd>
        <dt>Отрасль</dt><dd>{team.data.profile.industry || "Не указана"}</dd>
        <dt>Возможности</dt><dd>{team.data.profile.capabilities.join(", ") || "Не указаны"}</dd>
      </dl>
      <form className="catalog-inline" onSubmit={updateProfile}>
        <label>Выбор отрасли
          <Select key={team.data.profile.revision} name="industry" defaultValue={team.data.profile.industry} disabled={savingProfile}>
            {industries.map(industry => <option key={industry.id} value={industry.id}>{industry.label}</option>)}
          </Select>
        </label>
        <Button type="submit" disabled={savingProfile}>Сохранить отрасль</Button>
      </form>
      {profileError && <Alert tone="danger">{profileError}</Alert>}
      {profileNotice && <p role="status">{profileNotice}</p>}
      {agents.length === 0 ? <p>Агенты не настроены.</p> : <div className="channel-cards">
        {agents.map(agent => <details key={agent.id} className="channel-card" data-testid={`agent-card-${agent.id}`}>
          <summary>{agentName(agent.id)} <StatusBadge tone={agent.enabled ? "positive" : "neutral"}>
            {agent.enabled ? "Включён" : "Отключён"}
          </StatusBadge></summary>
          <p>{agent.description}</p>
          <h3>Разрешения</h3>
          {agent.permissions.length > 0
            ? <ul>{agent.permissions.map(permission => <li key={permission}>{permission}</li>)}</ul>
            : <p>Разрешений нет.</p>}
        </details>)}
      </div>}
    </>}
    {owner
      ? <section aria-labelledby="agent-create-heading">
        <h3 id="agent-create-heading">Новая задача</h3>
        {team.data && availableOperations.length === 0 && <p>У включённых агентов нет доступных действий.</p>}
        {availableOperations.length > 0 && <form className="catalog-inline" onSubmit={createTask}>
          <label>Действие агента
            <Select value={operation?.id ?? ""} required disabled={submitting}
              onChange={event => {setSelected(event.target.value); setSubmission(null);}}
              data-testid="agent-task-operation">
              <option value="" disabled>Выберите действие</option>
              {availableOperations.map(item => <option key={item.id} value={item.id}>{item.label}</option>)}
            </Select>
          </label>
          <Button type="submit" variant="primary" disabled={!operation || submitting} data-testid="agent-task-create">
            Создать задачу
          </Button>
        </form>}
        <p role="status" data-testid="agent-task-submission">
          {submitting ? "Отправляем задачу…" : submission && "id" in submission ? `Задача принята · ${submission.id}` : ""}
        </p>
        {submission && "error" in submission && <Alert tone="danger">{submission.error}</Alert>}
      </section>
      : <p>Управлять AI-командой и просматривать журнал может владелец магазина.</p>}

    <AgentApprovals organizationId={organizationId} timezone={timezone} canDecide={owner} getCsrfToken={getCsrfToken}
      refreshVersion={approvalVersion} onDecision={refreshActivity}/>

    {owner && <>
      <section aria-labelledby="agent-tasks-heading" data-testid="agent-tasks">
        <h3 id="agent-tasks-heading">Задачи и результаты</h3>
        <LoadFeedback resource={tasks} label="История задач"/>
        {tasks.data?.length === 0 && <p>Задач пока нет.</p>}
        {tasks.data && tasks.data.length > 0 && <div className="agent-team-tasks">
          {tasks.data.map(task => <details className="channel-card" key={task.id} data-testid={`agent-task-${task.id}`}>
            <summary>
              {agentName(task.agent_id)} · {task.action}{" "}
              <StatusBadge tone={task.status === "completed" ? "positive" : task.status === "failed" ? "danger" :
                task.status === "waiting_approval" ? "warning" : "neutral"}>
                {Object.hasOwn(taskStatuses, task.status) ? taskStatuses[task.status] : task.status}
              </StatusBadge>
              <small>{displayDate(task.created_at, timezone)}</small>
            </summary>
            <dl className="agent-team-details">
              <dt>Задача</dt><dd>{task.id}</dd>
              <dt>Срок</dt><dd>{displayDate(task.deadline, timezone)}</dd>
              {task.completed_at && <><dt>Завершена</dt><dd>{displayDate(task.completed_at, timezone)}</dd></>}
              <dt>Повторные попытки</dt><dd>{task.retry_count}</dd>
              {task.error_code && <><dt>Код ошибки</dt><dd>{task.error_code}</dd></>}
            </dl>
            <h4>Результат</h4>
            {task.result === null ? <p>Результат отсутствует.</p>
              : <pre data-testid={`agent-task-result-${task.id}`}>{JSON.stringify(task.result, null, 2)}</pre>}
          </details>)}
        </div>}
      </section>

      <section aria-labelledby="agent-audit-heading" data-testid="agent-audit">
        <h3 id="agent-audit-heading">Журнал действий</h3>
        <LoadFeedback resource={audit} label="Журнал действий"/>
        {audit.data?.length === 0 && <p>В журнале нет событий за последние 30 дней.</p>}
        {audit.data && audit.data.length > 0 && <Table label="События AI-команды">
          <caption className="sr-only">События AI-команды</caption>
          <thead><tr><th scope="col">Время</th><th scope="col">Агент</th><th scope="col">Действие</th>
            <th scope="col">Результат</th><th scope="col">Задача</th></tr></thead>
          <tbody>{audit.data.map(entry => {
            const bookingDelivery = entry.action === "booking.delivery" || entry.action === "booking.delivery_state";
            const result = bookingDelivery ? bookingDeliveryResults.get(entry.result) ?? entry.result : entry.result;
            return <tr key={entry.id} data-testid={`agent-audit-${entry.id}`}>
              <td>{displayDate(entry.created_at, timezone)}</td><td>{agentName(entry.agent_id)}</td>
              <td>{bookingDelivery ? "Уведомление о записи" : entry.action}</td>
              <td>{result}{entry.error_code && ` · ${entry.error_code}`}</td>
              <td>{entry.task_id}</td>
            </tr>;
          })}</tbody>
        </Table>}
      </section>
    </>}
  </section>;
}
