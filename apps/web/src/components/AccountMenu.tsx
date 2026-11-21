"use client";
import {useRef} from "react";
import {Logout} from "./IdentityForm";
import {Icon} from "./ProductUI";

import {friendlyName} from "@/lib/presentation";

export function AccountMenu({name,email}:{name:string;email:string}) {
  const details = useRef<HTMLDetailsElement>(null);
  const summary = useRef<HTMLElement | null>(null);
  const label = friendlyName(name);
  return <details className="profile-menu" ref={details} onKeyDown={event => {
    if (event.key === "Escape" && details.current?.open) {details.current.open = false; summary.current?.focus();}
  }}><summary ref={node => {summary.current = node;}} aria-label={`Профиль: ${label}`}><span className="profile-avatar" aria-hidden="true">{label[0].toUpperCase()}</span><span className="profile-name">{label}</span><Icon name="chevron"/></summary><div className="profile-popover"><strong>Данные аккаунта</strong><p>{email}</p><Logout/></div></details>;
}
