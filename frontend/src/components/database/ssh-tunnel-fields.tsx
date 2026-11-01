"use client";

import { useRef, type ChangeEvent } from "react";

import { Button, Checkbox, ErrorText, Field, Input, SegmentedControl, Textarea } from "@/components/ui";
import { fieldError } from "@/lib/errors";
import type { SSHTunnelDraft } from "@/lib/ssh-tunnel";
import type { SSHTunnel } from "@/lib/types";

/**
 * SSHTunnelFields lets an external instance be reached through an SSH bastion.
 * `saved` is the instance's current tunnel when editing: its secret is then
 * optional and shown as "stored".
 */
export function SSHTunnelFields({
  value,
  onChange,
  error,
  saved,
}: {
  value: SSHTunnelDraft;
  onChange: (d: SSHTunnelDraft) => void;
  error: unknown;
  saved?: SSHTunnel | null;
}) {
  const fileRef = useRef<HTMLInputElement>(null);
  const set = (patch: Partial<SSHTunnelDraft>) => onChange({ ...value, ...patch });
  // The stored secret stays valid only for the same bastion, user and method.
  const keepsSecret =
    !!saved && saved.host === value.host.trim() && saved.username === value.username.trim() && saved.auth_method === value.auth;

  function loadKey(e: ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (!file) return;
    file.text().then((text) => set({ privateKey: text }));
    e.target.value = "";
  }

  return (
    <div className="field">
      <Checkbox
        label="Connect through an SSH tunnel"
        checked={value.enabled}
        onChange={(e) => set({ enabled: e.target.checked })}
      />
      <ErrorText message={fieldError(error, "ssh_tunnel")} />
      {value.enabled ? (
        <div className="mt-2.5">
          <p className="muted text-sm mt-0">
            Fleetdock signs in to this SSH server and connects to the database from there, so the host above is resolved on
            the SSH server — use <code>127.0.0.1</code> if the database runs on it.
          </p>
          <div className="form-grid">
            <Field label="SSH host" error={fieldError(error, "ssh_tunnel.host")}>
              <Input
                value={value.host}
                onChange={(e) => set({ host: e.target.value })}
                placeholder="bastion.example.com"
                required
              />
            </Field>
            <Field label="SSH port" error={fieldError(error, "ssh_tunnel.port")}>
              <Input
                type="number"
                min={1}
                max={65535}
                value={value.port}
                onChange={(e) => set({ port: e.target.value })}
                required
              />
            </Field>
          </div>
          <div className="form-grid">
            <Field label="SSH user" error={fieldError(error, "ssh_tunnel.username")}>
              <Input
                value={value.username}
                onChange={(e) => set({ username: e.target.value })}
                placeholder="ubuntu"
                autoComplete="off"
                required
              />
            </Field>
            <Field label="Sign in with" error={fieldError(error, "ssh_tunnel.auth_method")}>
              <SegmentedControl
                label="SSH authentication"
                value={value.auth}
                onChange={(auth) => set({ auth })}
                options={[
                  { value: "key", label: "Private key" },
                  { value: "password", label: "Password" },
                ]}
              />
            </Field>
          </div>
          {value.auth === "key" ? (
            <>
              <Field
                label="Private key"
                hint={keepsSecret ? "A key is stored. Leave empty to keep it." : "OpenSSH or PEM format. Stored encrypted."}
                error={fieldError(error, "ssh_tunnel.private_key")}
              >
                <Textarea
                  className="mono"
                  rows={4}
                  value={value.privateKey}
                  onChange={(e) => set({ privateKey: e.target.value })}
                  placeholder="-----BEGIN OPENSSH PRIVATE KEY-----"
                  spellCheck={false}
                  autoComplete="off"
                />
              </Field>
              <div className="flex items-center gap-2 mb-2.5">
                <Button size="sm" onClick={() => fileRef.current?.click()}>
                  Load from file…
                </Button>
                <input ref={fileRef} type="file" hidden onChange={loadKey} />
              </div>
              <Field
                label="Key passphrase"
                hint="Only if the key is encrypted."
                error={fieldError(error, "ssh_tunnel.passphrase")}
              >
                <Input
                  type="password"
                  value={value.passphrase}
                  onChange={(e) => set({ passphrase: e.target.value })}
                  autoComplete="new-password"
                />
              </Field>
            </>
          ) : (
            <Field
              label="SSH password"
              hint={keepsSecret ? "A password is stored. Leave empty to keep it." : "Stored encrypted."}
              error={fieldError(error, "ssh_tunnel.password")}
            >
              <Input
                type="password"
                value={value.password}
                onChange={(e) => set({ password: e.target.value })}
                autoComplete="new-password"
              />
            </Field>
          )}
        </div>
      ) : null}
    </div>
  );
}
