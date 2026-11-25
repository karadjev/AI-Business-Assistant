"use client";

import type {Organization} from "@/lib/session";
import {AgentTeam} from "./AgentTeam";
import {csrf} from "./IdentityForm";

export function CommandTasks({org}: {org:Organization}) {
  return <AgentTeam organizationId={org.id} timezone={org.timezone} owner={org.role === "owner"} getCsrfToken={csrf} commandCenter/>;
}
