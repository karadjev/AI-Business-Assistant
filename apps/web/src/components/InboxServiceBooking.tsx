"use client";

import Link from "next/link";
import {useRef, useState, type FormEvent} from "react";
import {csrf} from "./IdentityForm";
import {buildServiceSelection, parseOfferings, readID, type Offering} from "@/lib/industry-workflows";
import {idempotencyKey} from "@/lib/workflows";

export function InboxServiceBooking({org, conversation}: {org: string; conversation: string}) {
  const [offerings, setOfferings] = useState<Offering[]>();
  const [selected, setSelected] = useState("");
  const [order, setOrder] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const active = useRef(false);
  const attempt = useRef<{fingerprint: string; key: string} | undefined>(undefined);
  const offering = offerings?.find(item => item.id === selected);
  const root = `/api/v1/organizations/${org}/operations/`;

  async function post(path: string, body: object): Promise<unknown> {
    const response = await fetch(root + path, {method:"POST",
      headers:{"Content-Type":"application/json","X-CSRF-Token":csrf()},
      body:JSON.stringify(body), signal:AbortSignal.timeout(8000)});
    const result = await response.json();
    if (!response.ok) throw new Error(result.error?.message ?? "Сервер отклонил действие.");
    return result;
  }

  async function perform(event: FormEvent<HTMLFormElement>, action: (input: FormData) => Promise<void>) {
    event.preventDefault();
    if (active.current || order) return;
    const input = new FormData(event.currentTarget);
    active.current = true;
    setBusy(true);
    setError("");
    try {
      await action(input);
    } catch (reason) {
      setError(reason instanceof TypeError || reason instanceof DOMException
        ? "Сервер не подтвердил действие. Проверьте заявки перед повтором."
        : reason instanceof Error ? reason.message : "Не удалось выполнить действие.");
    } finally {
      active.current = false;
      setBusy(false);
    }
  }

  if (order) return <p role="status">Заявка создана. <Link href={`/dashboard/orders/${order}?org=${org}`}>Открыть заявку для подтверждения записи</Link></p>;
  return <details className="panel">
    <summary>Подготовить запись на услугу</summary>
    <p>Выберите услугу и время в часовом поясе устройства. Запись закрепит сотрудник при подтверждении заявки.</p>
    <form className="identity-form" onSubmit={event => void perform(event, async input => {
      const matches = parseOfferings(await post("services/search",{query:String(input.get("query") ?? "").trim(),limit:20}));
      setOfferings(matches);
      setSelected(matches.length === 1 ? matches[0].id : "");
    })}>
      <label>Поиск услуги<input name="query" maxLength={65}/></label>
      <button disabled={busy}>Найти услуги</button>
    </form>
    {offerings?.length === 0 && <p role="status">Подходящих услуг нет.</p>}
    {!!offerings?.length && <form className="identity-form" onSubmit={event => void perform(event, async input => {
      if (!offering) throw new Error("Выберите услугу.");
      const selection = buildServiceSelection(input, offering);
      const details = {selection, summary:String(input.get("summary") ?? "").trim()};
      const fingerprint = JSON.stringify(details);
      if (attempt.current?.fingerprint !== fingerprint) attempt.current = {fingerprint,key:idempotencyKey()};
      const id = readID(await post("requests",{conversation_id:conversation,kind:"service_booking",details,idempotency_key:attempt.current.key}));
      if (!id) throw new Error("Номер заявки не получен. Проверьте список заявок.");
      setOrder(id);
    })}>
      <label>Услуга<select required value={selected} onChange={event=>setSelected(event.target.value)} disabled={busy}>
        <option value="">Выберите услугу</option>
        {offerings.map(item=><option key={item.id} value={item.id}>{item.name}</option>)}
      </select></label>
      {offering && <div key={offering.id}>
        <label>Начало записи<input type="datetime-local" name="slot" required/></label>
        <label>Длительность записи, минут<input type="number" name="duration" min={offering.min_minutes}
          max={offering.max_minutes} defaultValue={offering.min_minutes} required/></label>
        <label>Комментарий к заявке<input name="summary" maxLength={500}/></label>
      </div>}
      <button disabled={busy || !offering}>Создать заявку на запись</button>
    </form>}
    {error && <p role="alert">{error}</p>}
  </details>;
}
