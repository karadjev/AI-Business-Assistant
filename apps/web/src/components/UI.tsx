"use client";

import {useCallback, useEffect, useId, useRef, useState, type ComponentProps, type ReactNode} from "react";

export function Button({variant = "secondary", className = "", type = "button", ...props}: ComponentProps<"button"> & {variant?: "primary" | "secondary" | "danger"}) {
  return <button type={type} className={`button ${variant} ${className}`} {...props}/>;
}
export function Input(props: ComponentProps<"input">) { return <input {...props}/>; }
export function Select(props: ComponentProps<"select">) { return <select {...props}/>; }
export function Checkbox({children, ...props}: ComponentProps<"input"> & {children: ReactNode}) {
  return <label className="consent-row"><input {...props} type="checkbox"/>{children}</label>;
}
export function Card({className = "", ...props}: ComponentProps<"section">) { return <section className={`panel ${className}`} {...props}/>; }
export function Alert({children, tone = "info"}: {children: ReactNode; tone?: "info" | "danger" | "success"}) {
  return <div className={`ui-alert ui-alert-${tone}`} role={tone === "danger" ? "alert" : "status"}>{children}</div>;
}
export function Toast({children}: {children: ReactNode}) { return <Alert tone="success">{children}</Alert>; }
export function LoadingState({label = "Загружаем данные магазина…"}: {label?: string}) {
  return <div className="dashboard-loading" aria-busy="true"><p role="status">{label}</p><div aria-hidden="true"/><div aria-hidden="true"/></div>;
}
export function Table({label, children}: {label: string; children: ReactNode}) {
  return <div className="table-scroll" tabIndex={0} role="region" aria-label={label}><table>{children}</table></div>;
}

type Confirmation = {title: string; description: string; action: string; destructive?: boolean};
/** Native dialog supplies modal semantics/inert background; Escape always cancels. */
export function useConfirmation() {
  const [pending, setPending] = useState<Confirmation | null>(null);
  const dialog = useRef<HTMLDialogElement>(null);
  const cancel = useRef<HTMLButtonElement>(null);
  const resolve = useRef<((accepted: boolean) => void) | null>(null);
  const previous = useRef<HTMLElement | null>(null);
  const id = useId();
  const request = useCallback((options: Confirmation): Promise<boolean> => {
    if (resolve.current) return Promise.resolve(false);
    previous.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    return new Promise<boolean>(done => {resolve.current = done; setPending(options);});
  }, []);
  const finish = useCallback((accepted: boolean) => {
    const done = resolve.current;
    resolve.current = null;
    dialog.current?.close();
    setPending(null);
    previous.current?.focus();
    done?.(accepted);
  }, []);
  useEffect(() => {
    if (pending && dialog.current && !dialog.current.open) {
      dialog.current.showModal();
      cancel.current?.focus();
    }
  }, [pending]);
  useEffect(() => () => {resolve.current?.(false); resolve.current = null;}, []);
  const confirmation = pending ? <dialog ref={dialog} className="confirmation-dialog" aria-labelledby={`${id}-title`} aria-describedby={`${id}-description`} onKeyDown={event => {
    if (event.key !== "Tab") return;
    const buttons = event.currentTarget.querySelectorAll<HTMLButtonElement>("button:not(:disabled)");
    const first = buttons[0], last = buttons[buttons.length - 1];
    if (event.shiftKey && document.activeElement === first) {event.preventDefault(); last?.focus();}
    else if (!event.shiftKey && document.activeElement === last) {event.preventDefault(); first?.focus();}
  }} onCancel={event => {event.preventDefault(); finish(false);}} onClose={() => {if (resolve.current) finish(false);}}>
    <span className="eyebrow">Проверьте действие</span>
    <h2 id={`${id}-title`}>{pending.title}</h2>
    <p id={`${id}-description`}>{pending.description}</p>
    <div className="dialog-actions"><Button ref={cancel} onClick={() => finish(false)}>Вернуться</Button><Button variant={pending.destructive ? "danger" : "primary"} onClick={() => finish(true)}>{pending.action}</Button></div>
  </dialog> : null;
  return {request, confirmation};
}
