// Form-state helpers for an instance's SSH tunnel (see SSHTunnelFields).

import type { SSHAuthMethod, SSHTunnel, SSHTunnelInput } from "./types";

/** SSHTunnelDraft is the editable form state of an SSH tunnel. */
export interface SSHTunnelDraft {
  enabled: boolean;
  host: string;
  port: string;
  username: string;
  auth: SSHAuthMethod;
  password: string;
  privateKey: string;
  passphrase: string;
}

/** sshDraft starts a draft from an instance's current tunnel (or none). */
export function sshDraft(t?: SSHTunnel | null): SSHTunnelDraft {
  return {
    enabled: !!t,
    host: t?.host ?? "",
    port: String(t?.port ?? 22),
    username: t?.username ?? "",
    auth: t?.auth_method ?? "key",
    password: "",
    privateKey: "",
    passphrase: "",
  };
}

/**
 * sshInput converts a draft to the API shape. Only the secret for the chosen
 * auth method is sent; empty secrets keep the stored ones (on update).
 */
export function sshInput(d: SSHTunnelDraft): SSHTunnelInput {
  const base = { host: d.host.trim(), port: Number(d.port) || 22, username: d.username.trim(), auth_method: d.auth };
  return d.auth === "key"
    ? { ...base, private_key: d.privateKey || undefined, passphrase: d.passphrase || undefined }
    : { ...base, password: d.password || undefined };
}

/** sshChanged reports whether a draft differs from the saved tunnel. */
export function sshChanged(d: SSHTunnelDraft, t?: SSHTunnel | null): boolean {
  if (!d.enabled) return !!t;
  if (!t) return true;
  return (
    d.host.trim() !== t.host ||
    (Number(d.port) || 22) !== t.port ||
    d.username.trim() !== t.username ||
    d.auth !== t.auth_method ||
    !!d.password ||
    !!d.privateKey
  );
}
