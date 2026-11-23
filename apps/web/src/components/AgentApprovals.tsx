import {useEffect, useRef, useState} from "react";
import {displayDate} from "@/lib/presentation";
import {StatusBadge} from "./ProductUI";

type Approval = {
  id: string;
  task_id: string;
  agent_id: string;
  action: string;
  parameters: unknown;
  parameters_hash: string;
  reason: string;
  expires_at: string;
  status: string;
};

type Decision = "approved" | "rejected";
type ApprovalList =
  | {kind: "loading"}
  | {kind: "error"; message: string}
  | {kind: "ready"; items: Approval[]; checkedAt: number};

function hasExpired(item: Approval) {
  return Date.parse(item.expires_at) <= Date.now();
}

function isApproval(value: unknown): value is Approval {
  if (typeof value !== "object" || value === null) return false;
  const item = value as Partial<Approval>;
  return typeof item.id === "string" && /^[0-9a-f-]{36}$/.test(item.id)
    && typeof item.action === "string" && item.action.trim() !== ""
    && typeof item.task_id === "string" && typeof item.agent_id === "string"
    && typeof item.reason === "string" && typeof item.status === "string"
    && typeof item.parameters_hash === "string" && item.parameters_hash.trim() !== ""
    && "parameters" in item && typeof item.expires_at === "string"
    && Number.isFinite(Date.parse(item.expires_at));
}

async function loadApprovals(endpoint: string, signal: AbortSignal): Promise<Approval[]> {
  const timedSignal = AbortSignal.any([signal, AbortSignal.timeout(10000)]);
  const response = await fetch(endpoint, {cache: "no-store", signal: timedSignal}).catch(() => null);
  if (!response) throw new Error("Не удалось загрузить запросы на подтверждение.");
  if (response.status === 401) throw new Error("Войдите снова, чтобы увидеть запросы на подтверждение.");
  if (response.status === 403) throw new Error("У вас нет доступа к запросам этого магазина.");
  if (!response.ok) throw new Error("Не удалось загрузить запросы на подтверждение.");

  const data: {items?: unknown} | null = await response.json().catch(() => null);
  const items = data?.items;
  if (!Array.isArray(items) || !items.every(isApproval) || new Set(items.map(item => item.id)).size !== items.length) {
    throw new Error("Список запросов получен в неверном формате.");
  }
  return items.filter(item => item.status === "pending");
}

export function AgentApprovals({organizationId, timezone, canDecide, getCsrfToken, refreshVersion = 0, onDecision, taskIds}: {
  organizationId: string;
  timezone: string;
  canDecide: boolean;
  getCsrfToken: () => string;
  refreshVersion?: number;
  onDecision?: () => void;
  taskIds?: ReadonlySet<string>;
}) {
  const endpoint = `/api/v1/organizations/${organizationId}/agent-approvals`;
  const [list, setList] = useState<ApprovalList>({kind: "loading"});
  const [reload, setReload] = useState(0);
  const [savingId, setSavingId] = useState<string | null>(null);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const saving = useRef(false);
  const loading = useRef<AbortController | null>(null);
  const refreshButton = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    const controller = new AbortController();
    loading.current = controller;
    loadApprovals(endpoint, controller.signal).then(
      items => {
        if (!controller.signal.aborted) setList({kind: "ready", items, checkedAt: Date.now()});
      },
      failure => {
        if (!controller.signal.aborted) {
          const message = failure instanceof Error ? failure.message : "Не удалось загрузить запросы на подтверждение.";
          setList({kind: "error", message});
        }
      },
    ).finally(() => {
      if (loading.current === controller) loading.current = null;
    });
    return () => controller.abort();
  }, [endpoint, reload, refreshVersion]);

  useEffect(() => {
    if (list.kind !== "ready") return;
    const nextExpiry = list.items.reduce((next, item) => {
      const expiry = Date.parse(item.expires_at);
      return expiry > list.checkedAt ? Math.min(next, expiry) : next;
    }, Infinity);
    if (!Number.isFinite(nextExpiry)) return;
    const timer = window.setTimeout(() => {
      setList(current => current === list ? {...list, checkedAt: Date.now()} : current);
    }, Math.min(2_147_483_647, Math.max(0, nextExpiry - Date.now())));
    return () => window.clearTimeout(timer);
  }, [list]);

  function refresh() {
    if (saving.current || list.kind === "loading") return;
    setError("");
    setNotice("");
    setList({kind: "loading"});
    setReload(current => current + 1);
  }

  async function decide(item: Approval, decision: Decision) {
    if (!canDecide || saving.current || list.kind !== "ready" || error) return;
    if (hasExpired(item)) {
      setError("Срок действия запроса истёк. Обновите список.");
      return;
    }
    const csrfToken = getCsrfToken();
    if (!csrfToken) {
      setError("Обновите страницу или войдите снова, чтобы отправить решение.");
      return;
    }

    saving.current = true;
    setSavingId(item.id);
    setNotice("");
    try {
      const response = await fetch(`${endpoint}/${item.id}/decision`, {
        method: "POST",
        signal: AbortSignal.timeout(10000),
        headers: {"Content-Type": "application/json", "X-CSRF-Token": csrfToken},
        body: JSON.stringify({decision, parameters_hash: item.parameters_hash}),
      });
      if (response.status === 204) {
        if (loading.current) {
          loading.current.abort();
          setReload(current => current + 1);
        }
        if (document.activeElement?.closest("[data-approval-id]")?.getAttribute("data-approval-id") === item.id) {
          refreshButton.current?.focus();
        }
        setList(current => current.kind === "ready"
          ? {...current, items: current.items.filter(approval => approval.id !== item.id)}
          : current);
        setNotice(decision === "approved" ? "Действие подтверждено." : "Действие отклонено.");
        onDecision?.();
      } else if (response.status === 401 || response.status === 403) {
        setError("Нет доступа к отправке решения. Проверьте права доступа или войдите снова.");
      } else {
        setError("Не удалось сохранить решение. Обновите список и проверьте статус запроса.");
      }
    } catch {
      setError("Не удалось получить ответ сервера. Обновите список и проверьте статус запроса.");
    } finally {
      saving.current = false;
      setSavingId(null);
    }
  }

  const busy = list.kind === "loading" || savingId !== null;
  const alert = list.kind === "error" ? list.message : error;
  const empty = list.kind === "ready" && list.items.length === 0;
  const status = list.kind === "loading" ? "Загрузка запросов…"
    : savingId ? "Отправка решения…" : notice || (empty ? "Нет запросов, ожидающих подтверждения." : "");
  return (
    <section className="module-card agent-approvals" aria-labelledby="agent-approvals-heading" data-testid="agent-approvals">
      <h2 id="agent-approvals-heading">Требуют подтверждения</h2>
      <button ref={refreshButton} type="button" className="button secondary" aria-disabled={busy} onClick={refresh}>
        Обновить запросы
      </button>
      <p role="status">{status}</p>
      {alert && <p role="alert">{alert}</p>}
      {empty && notice && <p>Нет запросов, ожидающих подтверждения.</p>}
      {list.kind === "ready" && !empty && <>
          {!canDecide && <p>Решение принимает владелец магазина.</p>}
          <ul>
            {list.items.map(item => {
              const expired = Date.parse(item.expires_at) <= list.checkedAt;
              const decisionDisabled = busy || expired || Boolean(error);
              return (
                <li key={item.id} data-approval-id={item.id}>
                  <h3>{item.action}</h3>
                  <StatusBadge tone={expired ? "neutral" : "warning"}>
                    {expired ? "Срок истёк" : "Ожидает решения"}
                  </StatusBadge>
                  <p>{item.reason}</p>
                  {item.agent_id === "marketing" && item.action === "draft" && <p>
                    После подтверждения агент подготовит черновик. Публикация и отправка требуют отдельных разрешений.
                  </p>}
                  <p>Срок действия: <time dateTime={item.expires_at}>{displayDate(item.expires_at, timezone)}</time></p>
                  <details className="technical-details">
                    <summary>Детали запроса</summary>
                    <dl>
                      <dt>Запрос</dt><dd>{item.id}</dd>
                      <dt>Задача</dt><dd>{canDecide && taskIds?.has(item.task_id)
                        ? <a href={`#agent-task-${item.task_id}`}>{item.task_id}</a> : item.task_id}</dd>
                      <dt>Агент</dt><dd>{item.agent_id}</dd>
                      <dt>Параметры действия</dt><dd><pre>{JSON.stringify(item.parameters, null, 2)}</pre></dd>
                    </dl>
                    {canDecide && <div className="page-actions">
                      <button type="button" className="button primary" aria-disabled={decisionDisabled}
                        onClick={() => {if (!decisionDisabled) void decide(item, "approved");}}>
                        Подтвердить
                      </button>
                      <button type="button" className="button secondary" aria-disabled={decisionDisabled}
                        onClick={() => {if (!decisionDisabled) void decide(item, "rejected");}}>
                        Отклонить
                      </button>
                    </div>}
                  </details>
                </li>
              );
            })}
          </ul>
        </>}
    </section>
  );
}
