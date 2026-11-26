"use client";

import {useRef, useState, type FormEvent} from "react";
import Link from "next/link";
import {useRouter} from "next/navigation";
import {csrf} from "./IdentityForm";
import {money} from "@/lib/catalog";
import {displayDate} from "@/lib/presentation";
import {statuses} from "@/lib/workflows";
import {Button, useConfirmation} from "./UI";

export type ServiceWorkState = {
  assignment: {user_id: string; name: string; mine: boolean; assigned_at: string} | null;
  receipt: {received_minor: number; cost_minor: number; recorded_at: string} | null;
  resources: {id: string; name: string; user_id: string | null}[];
};
export type ServiceMember = {id: string; display_name: string; email: string; operator: boolean};
export type ScheduleItem = {order_id: string; status: string; starts_at: string; ends_at: string; timezone: string; resource_name: string};
export type ServiceTotals = {completed_count: number; paid_count: number; received_minor: number | null; cost_minor: number | null; difference_minor: number | null};

export function ServiceSchedule({org, items, totals}: {org: string; items: ScheduleItem[]; totals?: ServiceTotals}) {
  return <>
    <section className="panel" aria-label="Моё расписание">
      <h2>Моё расписание</h2>
      {items.length === 0 ? <p>У вас пока нет назначенных услуг.</p> :
        <ul className="order-timeline">{items.map(item => <li key={item.order_id}>
          <Link href={`/dashboard/orders/${item.order_id}?org=${org}`}>{item.resource_name} · заявка №{item.order_id.slice(0, 8)}</Link>
          <p>{displayDate(item.starts_at, item.timezone)} — {displayDate(item.ends_at, item.timezone)} · {item.timezone}</p>
          <p>{item.status === "confirmed" ? "Запланирована" : statuses[item.status] ?? item.status}</p>
        </li>)}</ul>}
    </section>
    {totals && <section className="panel" aria-label="Финансовый итог услуг">
      <h2>Финансовый итог услуг</h2>
      <p>За всё время · завершено {totals.completed_count} · с подтверждённой оплатой {totals.paid_count}</p>
      {totals.received_minor === null ? <p>Подтверждённых поступлений пока нет.</p> :
        <dl><dt>Получено</dt><dd>{money(totals.received_minor)}</dd>
          <dt>Учтённая себестоимость</dt><dd>{money(totals.cost_minor!)}</dd>
          <dt>Поступления за вычетом себестоимости оплаченных услуг</dt><dd>{money(totals.difference_minor!)}</dd></dl>}
    </section>}
  </>;
}

export function ServiceWork({org, order, status, owner, state, members}: {org: string; order: string; status: string; owner: boolean; state: ServiceWorkState; members: ServiceMember[]}) {
  const router = useRouter();
  const [resource, setResource] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const running = useRef(false);
  const {request: confirm, confirmation} = useConfirmation();
  const assigned = state.assignment;
  const receipt = state.receipt;
  const selected = state.resources.find(item => item.id === resource);
  const availableMembers = members.filter(item => item.operator && (!selected?.user_id || selected.user_id === item.id));

  async function submit(event: FormEvent<HTMLFormElement>, command: "assign" | "receipt") {
    event.preventDefault();
    if (running.current) return;
    running.current = true;
    setError("");
    const form = new FormData(event.currentTarget);
    try {
      const input = command === "assign"
        ? {resource_id: resource, user_id: String(form.get("staff"))}
        : {received_minor: Math.round(Number(form.get("received")) * 100), cost_minor: Math.round(Number(form.get("cost")) * 100)};
      if (command === "receipt" && !await confirm({title: "Подтвердить оплату", action: "Учесть оплату", description: "В отчёт попадут фактически полученная сумма и себестоимость. Запись можно учесть только один раз."})) return;
      setBusy(true);
      const response = await fetch(`/api/v1/organizations/${org}/operations/services/work/${command}`, {
        method: "POST", headers: {"Content-Type": "application/json", "X-CSRF-Token": csrf()},
        body: JSON.stringify({order_id: order, ...input}),
      });
      if (response.status === 401) {router.replace("/login"); return;}
      const result = await response.json();
      if (!response.ok) throw new Error(result.error?.message ?? "Не удалось сохранить данные");
      router.refresh();
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : "Не удалось сохранить данные");
    } finally {
      setBusy(false);
      running.current = false;
    }
  }

  return <section className="panel" aria-label="Исполнитель и оплата" aria-busy={busy}>
    <h2>Исполнитель и оплата</h2>
    {error && <p role="alert">{error}</p>}
    {assigned ? <p role="status">Исполнитель: {members.find(member => member.id === assigned.user_id)?.display_name || assigned.name}{assigned.mine ? " (вы)" : ""}</p> :
      <p>{status === "pending_confirmation" ? "После подтверждения записи назначьте исполнителя." : "Исполнитель не назначен."}</p>}
    {owner && status === "confirmed" && !assigned && (state.resources.length === 0 ? <p>Для назначения требуется запись с выделенным ресурсом сотрудника вместимостью 1.</p> :
      <form className="identity-form" onSubmit={event => void submit(event, "assign")}>
        <label htmlFor="service-resource">Ресурс записи</label><select id="service-resource" required value={resource} onChange={event => setResource(event.target.value)}>
          <option value="">Выберите ресурс</option>
          {state.resources.map(item => <option value={item.id} key={item.id}>{item.name}</option>)}
        </select>
        <label htmlFor="service-executor">Исполнитель</label><select id="service-executor" name="staff" key={resource} required disabled={!resource}>
          <option value="">Выберите сотрудника</option>
          {availableMembers.map(member => <option value={member.id} key={member.id}>{member.display_name} · {member.email}</option>)}
        </select>
        <p className="field-hint">При первом назначении ресурс закрепляется за сотрудником. Сервер учитывает утверждённое расписание ресурса.</p>
        <Button type="submit" disabled={busy || !resource}>{busy ? "Сохраняем…" : "Назначить исполнителя"}</Button>
      </form>)}
    {owner && assigned && (receipt ? <p role="status">Оплата учтена. Получено {money(receipt.received_minor)}, себестоимость {money(receipt.cost_minor)}.</p> :
      status === "completed" ? <form className="identity-form" onSubmit={event => void submit(event, "receipt")}>
        <p>Услуга завершена. Подтвердите оплату после фактического поступления денег.</p>
        <label>Получено, ₽<input name="received" type="number" min="0.01" max="10000000000" step="0.01" required/></label>
        <label>Себестоимость, ₽<input name="cost" type="number" min="0" max="10000000000" step="0.01" required/></label>
        <Button type="submit" disabled={busy}>{busy ? "Сохраняем…" : "Подтвердить оплату"}</Button>
      </form> : status === "confirmed" && <p>Оплату можно учесть после завершения услуги.</p>)}
    {confirmation}
  </section>;
}
