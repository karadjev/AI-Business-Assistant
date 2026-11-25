"use client";

import {useCallback, useEffect, useRef, useState} from "react";
import {csrf} from "./IdentityForm";
import {StatusBadge} from "./ProductUI";

type Draft = {id:string; version:number; content:string; goal:string; audience:string; status:string};

export function MarketingResults({organizationId, version, onDecision}: {organizationId:string; version:number; onDecision:()=>void}) {
  const endpoint = `/api/v1/organizations/${organizationId}/marketing-drafts`;
  const [drafts,setDrafts] = useState<Draft[]>([]);
  const [loaded,setLoaded] = useState(false);
  const [error,setError] = useState("");
  const [notice,setNotice] = useState("");
  const [busy,setBusy] = useState(false);
  const submitting = useRef(false);

  const load = useCallback(async (signal = AbortSignal.timeout(10000)) => {
    const response = await fetch(endpoint,{cache:"no-store",signal});
    if (!response.ok) throw new Error("Не удалось загрузить черновики.");
    return response.json() as Promise<{items:Draft[]}>;
  },[endpoint]);

  function show(data:{items:Draft[]}) {
    setDrafts(data.items);
    setLoaded(true);
    setError("");
  }

  useEffect(() => {
    const controller = new AbortController();
    void load(AbortSignal.any([controller.signal,AbortSignal.timeout(10000)])).then(data => {
      if (!controller.signal.aborted) show(data);
    }).catch(() => {
      if (!controller.signal.aborted) setError("Не удалось загрузить черновики. Обновите список.");
    });
    return () => controller.abort();
  },[load,version]);

  async function approve(draft:Draft) {
    if (submitting.current) return;
    submitting.current = true;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const response = await fetch(`${endpoint}/${draft.id}/review`,{
        method:"POST",headers:{"Content-Type":"application/json","X-CSRF-Token":csrf()},
        body:JSON.stringify({version:draft.version,action:"approved"}),
        signal:AbortSignal.timeout(10000),
      });
      if (!response.ok) throw new Error("Не удалось сохранить решение. Обновите список и проверьте версию черновика.");
      setDrafts(items => items.map(item => item.id === draft.id && item.version === draft.version ? {...item,status:"approved"} : item));
      setNotice("Черновик согласован и сохранён в организации.");
      onDecision();
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : "Не удалось сохранить решение. Обновите список.");
    } finally {
      submitting.current = false;
      setBusy(false);
    }
  }

  return <section className="panel marketing-results" aria-labelledby="marketing-results-title">
    <h3 id="marketing-results-title">Маркетинговые черновики</h3>
    {error && <p role="alert">{error} <button type="button" onClick={() => void load().then(show).catch(() => setError("Не удалось загрузить черновики. Повторите попытку."))}>Обновить</button></p>}
    {notice && <p role="status">{notice}</p>}
    {!loaded && !error && <p role="status">Загружаем черновики…</p>}
    {loaded && drafts.length === 0 && <p>Черновики появятся после выполнения поручения.</p>}
    {drafts.map(draft => <details key={`${draft.id}-${draft.version}`} data-draft-id={draft.id}>
      <summary>{draft.goal} <StatusBadge tone={draft.status === "approved" ? "positive" : "neutral"}>
        {draft.status === "approved" ? "Согласован" : draft.status === "pending" ? "К проверке" : draft.status === "rejected" ? "Отклонён" : "Предыдущая версия"}
      </StatusBadge></summary>
      <p className="field-hint">Аудитория: {draft.audience}</p>
      <p className="marketing-draft-content">{draft.content}</p>
      <a href={`#agent-task-${draft.id}`}>Задача и результат</a>
      {draft.status === "pending" && <>
        <p>Решение согласует эту версию в кабинете. Сообщение останется черновиком.</p>
        <button type="button" className="primary" disabled={busy || Boolean(error)} onClick={() => void approve(draft)}>{busy ? "Сохраняем…" : "Согласовать черновик"}</button>
      </>}
    </details>)}
  </section>;
}
