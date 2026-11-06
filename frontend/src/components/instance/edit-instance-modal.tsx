"use client";

import { useEffect, useState, type FormEvent } from "react";

import { DataAccessFields } from "@/components/database/data-access-fields";
import { SSHTunnelFields } from "@/components/database/ssh-tunnel-fields";
import { Button, Checkbox, ErrorText, Field, Modal } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { dataAccessChanged, dataAccessDraft, dataAccessInput } from "@/lib/data-access";
import { fieldError } from "@/lib/errors";
import { useUpdateInstance } from "@/lib/hooks";
import { sshChanged, sshDraft, sshInput } from "@/lib/ssh-tunnel";
import type { Instance, TLSMode, UpdateInstanceInput } from "@/lib/types";
import { ENGINES, type EngineId } from "@/lib/engines";

/** EditInstanceModal edits a database server's connection and logins. Pass null to close it. */
export function EditInstanceModal({
  instance,
  onClose,
}: {
  instance: Instance | null;
  onClose: () => void;
}) {
  const update = useUpdateInstance();
  const [name, setName] = useState("");
  const [host, setHost] = useState("");
  const [port, setPort] = useState("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [clearPassword, setClearPassword] = useState(false);
  const [tlsMode, setTlsMode] = useState<TLSMode>("prefer");
  const [ssh, setSsh] = useState(sshDraft());
  const [dataAccess, setDataAccess] = useState(dataAccessDraft());
  const [error, setError] = useState<unknown>(null);

  // Re-seed the form whenever a different instance is opened.
  useEffect(() => {
    if (!instance) return;
    setName(instance.name);
    setHost(instance.host ?? "");
    setPort(String(instance.port));
    setUsername(instance.username ?? "");
    setPassword("");
    setClearPassword(false);
    setTlsMode(instance.tls_mode ?? "prefer");
    setSsh(sshDraft(instance.ssh_tunnel));
    setDataAccess(dataAccessDraft(instance));
    setError(null);
  }, [instance]);

  if (!instance) return null;

  const portLocked = instance.provisioned;
  // The API refuses to send the stored password to a new host; the user has to
  // re-enter it (or drop it).
  const hostChanged = instance.kind === "external" && host.trim() !== (instance.host ?? "");
  // Adding, moving or removing the SSH tunnel re-routes the connection too.
  const saved = instance.ssh_tunnel;
  const tunnelRerouted =
    ssh.enabled !== !!saved ||
    (ssh.enabled && !!saved && (ssh.host.trim() !== saved.host || (Number(ssh.port) || 22) !== saved.port));
  const needsPassword = (hostChanged || tunnelRerouted) && instance.has_credentials && !clearPassword && username !== "";
  // The stored data login password stays usable only for the same username and route.
  const keepsDataPassword =
    instance.data_access === "login" &&
    dataAccess.username.trim() === (instance.data_username ?? "") &&
    !hostChanged &&
    !tunnelRerouted;

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (!instance) return;
    setError(null);

    // Send only what actually changed, so an untouched password field never
    // rotates the stored secret.
    const input: UpdateInstanceInput & { id: string } = { id: instance.id };
    if (name !== instance.name) input.name = name;
    if (instance.kind === "external" && host !== (instance.host ?? "")) input.host = host;
    if (!portLocked && Number(port) !== instance.port) input.port = Number(port);
    if (username !== (instance.username ?? "")) input.username = username;
    if (tlsMode !== (instance.tls_mode ?? "prefer")) input.tls_mode = tlsMode;
    if (clearPassword) {
      input.password = "";
    } else if (password) {
      input.password = password;
    }
    if (instance.kind === "external" && sshChanged(ssh, instance.ssh_tunnel)) {
      if (ssh.enabled) input.ssh_tunnel = sshInput(ssh);
      else input.remove_ssh_tunnel = true;
    }
    if (dataAccessChanged(dataAccess, instance) || (!keepsDataPassword && dataAccess.mode === "login")) {
      Object.assign(input, dataAccessInput(dataAccess));
    }

    try {
      await update.mutateAsync(input);
      onClose();
    } catch (err) {
      setError(err);
    }
  }

  return (
    <Modal open onClose={onClose} title={`Edit "${instance.name}"`}>
      <form onSubmit={onSubmit}>
        <div className="form-grid">
          <Field label="Name">
            <input className="input" value={name} onChange={(e) => setName(e.target.value)} required />
          </Field>
          <Field label="Port" hint={portLocked ? "Fixed: Fleetdock created this server in a container on this port." : undefined}>
            <input
              className="input"
              type="number"
              value={port}
              onChange={(e) => setPort(e.target.value)}
              disabled={portLocked}
              required
            />
          </Field>
        </div>
        <Field label="Encryption" help="How Fleetdock protects its connection to this server. Use “verify” across networks you don’t trust.">
          <select className="input" value={tlsMode} onChange={(e) => setTlsMode(e.target.value as TLSMode)}>
            <option value="prefer">Use encryption when available (default)</option>
            <option value="require">Always encrypt</option>
            <option value="verify-full">Always encrypt and verify the certificate</option>
            <option value="disable">Never encrypt</option>
          </select>
        </Field>
        {instance.kind === "external" ? (
          <>
            <Field
              label="Host"
              hint={ssh.enabled ? "Hostname or IP address as seen from the SSH server." : "Hostname or IP address Fleetdock can reach."}
              help={
                <>
                  On the same computer as Fleetdock? Use <code>host.docker.internal</code> — <code>localhost</code> would mean
                  Fleetdock&apos;s own container.
                </>
              }
              error={fieldError(error, "host")}
            >
              <input
                className="input"
                value={host}
                onChange={(e) => setHost(e.target.value)}
                placeholder={ssh.enabled ? "127.0.0.1" : "db.example.com or 10.0.0.5"}
                required
              />
            </Field>
            <SSHTunnelFields value={ssh} onChange={setSsh} error={error} saved={instance.ssh_tunnel} />
          </>
        ) : null}

        <h3 className="mt-4 mb-1 text-sm font-semibold">Admin credentials</h3>
        <p className="mt-0 mb-3 text-sm text-muted">
          Used for provisioning, discovery, backups and live administration. Stored encrypted; the current password is
          never displayed.
        </p>
        <div className="form-grid">
          <Field label="Admin username">
            <input
              className="input"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              placeholder={ENGINES[instance.engine as EngineId]?.adminUser ?? "root"}
              autoComplete="off"
            />
          </Field>
          <Field
            label={
              needsPassword
                ? `Password (required for the new ${hostChanged ? "host" : "route"})`
                : instance.has_credentials
                  ? "New password"
                  : "Password"
            }
            error={fieldError(error, "password")}
          >
            <input
              className="input"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder={needsPassword ? "re-enter the admin password" : instance.has_credentials ? "leave blank to keep current" : ""}
              disabled={clearPassword}
              required={needsPassword}
              autoComplete="new-password"
            />
          </Field>
        </div>
        {instance.has_credentials ? (
          <Checkbox
            checked={clearPassword}
            onChange={(e) => setClearPassword(e.target.checked)}
            label="Remove the stored password (makes this instance metadata-only)"
          />
        ) : null}

        <div className="mt-4">
          <DataAccessFields value={dataAccess} onChange={setDataAccess} error={error} hasStoredLogin={keepsDataPassword} />
        </div>

        <ErrorText message={error ? (error instanceof ApiError ? error.message : "Failed to save the changes") : undefined} />
        <div className="modal-actions">
          <Button onClick={onClose}>Cancel</Button>
          <Button type="submit" variant="primary" loading={update.isPending}>
            {update.isPending ? "Saving…" : "Save changes"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
