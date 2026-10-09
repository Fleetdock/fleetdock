// Form-state helpers for an instance's data access login (see DataAccessFields).

import type { DataAccess, Instance } from "./types";

/** DataAccessDraft is the editable form state of an instance's data access. */
export interface DataAccessDraft {
  mode: DataAccess;
  username: string;
  password: string;
}

/** dataAccessDraft starts a draft from an instance (or the default for a new one). */
export function dataAccessDraft(inst?: Pick<Instance, "data_access" | "data_username"> | null): DataAccessDraft {
  return { mode: inst?.data_access ?? "admin", username: inst?.data_username ?? "", password: "" };
}

/**
 * dataAccessInput converts a draft to the API fields. The login fields are
 * only sent for mode "login"; an empty password keeps the stored one.
 */
export function dataAccessInput(d: DataAccessDraft): {
  data_access: DataAccess;
  data_username?: string;
  data_password?: string;
} {
  if (d.mode !== "login") return { data_access: d.mode };
  return { data_access: d.mode, data_username: d.username.trim(), data_password: d.password || undefined };
}

/** dataAccessChanged reports whether a draft differs from the saved setting. */
export function dataAccessChanged(d: DataAccessDraft, inst: Pick<Instance, "data_access" | "data_username">): boolean {
  const saved = dataAccessDraft(inst);
  if (d.mode !== saved.mode) return true;
  if (d.mode !== "login") return false;
  return d.username.trim() !== saved.username || !!d.password;
}

export const DATA_ACCESS_LABELS: Record<DataAccess, string> = {
  admin: "Admin login",
  login: "Dedicated login",
  managed: "Fleetdock-managed roles",
};
