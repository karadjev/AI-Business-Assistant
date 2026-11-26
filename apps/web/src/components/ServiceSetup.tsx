"use client";

import Link from "next/link";
import {useRef, useState, type FormEvent} from "react";
import {minor} from "@/lib/catalog";
import {formatServiceInstant} from "@/lib/industry-workflows";
import {csrf} from "./IdentityForm";

const layers = [
  ["business", "Часы работы компании"],
  ["offering", "Время оказания услуги"],
  ["shift", "Смена исполнителя"],
] as const;

type Saved = {id: string; name: string; revision: number};
type Layer = typeof layers[number][0];

export function ServiceSetup({org, timezone}: {org: string; timezone: string}) {
  const pending = useRef(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [resource, setResource] = useState<Saved>();
  const [offering, setOffering] = useState<Saved>();
  const [windows, setWindows] = useState<Partial<Record<Layer, string>>>({});

  async function submit(event: FormEvent<HTMLFormElement>, action: (form: FormData) => Promise<void>) {
    event.preventDefault();
    if (pending.current) return;
    pending.current = true;
    const form = new FormData(event.currentTarget);
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await action(form);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Действие не удалось выполнить.");
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }

  async function send<T>(path: string, body: object): Promise<T> {
    const result = await fetch(`/api/v1/organizations/${org}/operations/services/${path}`, {
      method: "POST",
      headers: {"Content-Type": "application/json", "X-CSRF-Token": csrf()},
      body: JSON.stringify(body), signal: AbortSignal.timeout(8000),
    }).catch(() => {throw new Error("Сервер не подтвердил действие. Сверьте сохранённые данные перед повтором.");});
    const data = await result.json();
    if (!result.ok) throw new Error(data.error?.message ?? "Действие отклонено сервером.");
    return data as T;
  }

  return <section className="panel">
    <h2>Услуги, цены и расписание</h2>
    <p>Настройте первую услугу в тестовой организации. Время полей соответствует часовому поясу устройства. Сервер сохранит интервалы в часовом поясе организации ({timezone}).</p>
    <form className="identity-form" onSubmit={event => void submit(event, async form => {
      const name = String(form.get("name") ?? "").trim();
      if (!name) throw new Error("Укажите имя исполнителя.");
      setResource(await send<Saved>("config/resources", {name, kind:"staff", capacity:1, active:true, revision:0}));
      setNotice("Ресурс расписания сохранён.");
    })}>
      <label>Исполнитель для расписания<input name="name" required maxLength={64} disabled={busy || !!resource}/></label>
      <button disabled={busy || !!resource}>Сохранить исполнителя</button>
    </form>
    <p><Link href={`/dashboard/team?org=${org}`}>Пригласить сотрудника в кабинет</Link>. Ресурс расписания сам по себе не выдаёт доступ к кабинету.</p>
    {resource && <form className="identity-form" onSubmit={event => void submit(event, async form => {
      const name = String(form.get("name") ?? "").trim();
      if (!name) throw new Error("Укажите название услуги.");
      const duration = Number(form.get("duration"));
      const amount = minor(String(form.get("price") ?? ""));
      setOffering(await send<Saved>("config/offerings", {
        name, status:"active", revision:0, price_model:"fixed", price_state:"known", amount_minor:amount,
        currency:"RUB", min_minutes:duration, max_minutes:duration, buffer_before:0, buffer_after:0,
        timezone, resource_ids:[resource.id],
      }));
      setNotice("Услуга и цена сохранены.");
    })}>
      <label>Название услуги<input name="name" required maxLength={100} disabled={busy || !!offering}/></label>
      <label>Цена, ₽<input name="price" inputMode="decimal" required disabled={busy || !!offering}/></label>
      <label>Длительность, минут<input name="duration" type="number" required min={1} max={1440} defaultValue={60} disabled={busy || !!offering}/></label>
      <button disabled={busy || !!offering}>Сохранить услугу и цену</button>
    </form>}
    {resource && offering && <section aria-label="Расписание услуги">
      <h3>Расписание · {offering.name}</h3>
      <p>Для записи интервал должен входить во все три окна. Добавляйте даты для одной услуги, пока она открыта на этом экране.</p>
      <form className="identity-form" onSubmit={event => void submit(event, async form => {
        const layer = layers.find(item => item[0] === form.get("layer"))?.[0];
        const start = new Date(String(form.get("start")));
        const end = new Date(String(form.get("end")));
        if (!layer || !Number.isFinite(start.getTime()) || !Number.isFinite(end.getTime()) || end <= start) {
          throw new Error("Укажите вид окна и время окончания позже начала.");
        }
        const startsAt = formatServiceInstant(start, timezone);
        const endsAt = formatServiceInstant(end, timezone);
        const interval = `${startsAt} → ${endsAt}`;
        if (windows[layer] === interval) throw new Error("Это окно уже сохранено на этом экране.");
        await send("config/windows", {offering_id:offering.id, resource_id:resource.id, layer,
          starts_at:startsAt, ends_at:endsAt, timezone});
        setWindows(previous => ({...previous, [layer]:interval}));
        setNotice("Окно расписания сохранено.");
      })}>
        <label>Вид окна<select name="layer">{layers.map(([id,label]) => <option key={id} value={id}>{label}</option>)}</select></label>
        <label>Начало<input type="datetime-local" name="start" required/></label>
        <label>Окончание<input type="datetime-local" name="end" required/></label>
        <button disabled={busy}>Добавить окно расписания</button>
      </form>
      <ul>{layers.filter(([id]) => windows[id]).map(([id,label]) => <li key={id}>{label} · {windows[id]}</li>)}</ul>
    </section>}
    {error && <p role="alert">{error}</p>}
    {notice && <p role="status">{notice}</p>}
    <Link href={`/dashboard/command?org=${org}`}>Открыть Business Manager</Link>
  </section>;
}
