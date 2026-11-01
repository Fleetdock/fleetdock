"use client";

import { useState, type FormEvent } from "react";

import { Button, Checkbox, ErrorText, Field, Modal } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { useCreateDBUser, useDBPrivileges, useGrantOnInstance, useSetDBUserPassword } from "@/lib/hooks";
import type { DBUser } from "@/lib/types";

export function CreateDBUserModal({
  open,
  onClose,
  instanceId,
}: {
  open: boolean;
  onClose: () => void;
  instanceId: string;
}) {
  const create = useCreateDBUser();
  const [username, setUsername] = useState("");
  const [host, setHost] = useState("%");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      await create.mutateAsync({ instance_id: instanceId, username, host, password });
      setUsername("");
      setPassword("");
      onClose();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to create user");
    }
  }

  return (
    <Modal open={open} onClose={onClose} title="Create database user">
      <form onSubmit={onSubmit}>
        <div className="form-grid">
          <Field label="Username">
            <input className="input" value={username} onChange={(e) => setUsername(e.target.value)} placeholder="app_user" required />
          </Field>
          <Field
            label="Connect from"
            help="Where this user may log in from (MySQL/MariaDB). % means anywhere; localhost means only from the server itself."
          >
            <input className="input" value={host} onChange={(e) => setHost(e.target.value)} placeholder="%" />
          </Field>
        </div>
        <Field label="Password">
          <input
            className="input"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="new-password"
            required
          />
        </Field>
        <ErrorText message={error ?? undefined} />
        <div className="modal-actions">
          <Button onClick={onClose}>Cancel</Button>
          <Button type="submit" variant="primary" loading={create.isPending}>
            {create.isPending ? "Creating…" : "Create user"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

export function GrantModal({
  target,
  onClose,
  instanceId,
  databases,
}: {
  target: DBUser | null;
  onClose: () => void;
  instanceId: string;
  databases: { id: string; name: string }[];
}) {
  const grant = useGrantOnInstance();
  const { data: privileges } = useDBPrivileges();
  const [database, setDatabase] = useState("");
  const [selected, setSelected] = useState<Set<string>>(new Set(["ALL PRIVILEGES"]));
  const [error, setError] = useState<string | null>(null);

  function toggle(p: string) {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(p)) {
        next.delete(p);
      } else {
        next.add(p);
      }
      return next;
    });
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      await grant.mutateAsync({
        instanceId,
        username: target!.user,
        host: target!.host,
        database,
        privileges: [...selected],
      });
      onClose();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to grant privileges");
    }
  }

  if (!target) return null;
  return (
    <Modal open onClose={onClose} title={`Grant to '${target.user}'@'${target.host}'`}>
      <form onSubmit={onSubmit}>
        <Field label="Database">
          <select className="input" value={database} onChange={(e) => setDatabase(e.target.value)} required>
            <option value="" disabled>
              Select a database…
            </option>
            {databases.map((d) => (
              <option key={d.id} value={d.name}>
                {d.name}
              </option>
            ))}
          </select>
        </Field>
        <fieldset className="field m-0 border-0 p-0">
          <legend className="label">Privileges</legend>
          <div className="card max-h-55 overflow-y-auto p-3">
            <div className="flex flex-wrap gap-x-4 gap-y-1">
              {(privileges?.items ?? []).map((p) => (
                <Checkbox key={p} className="text-sm" checked={selected.has(p)} onChange={() => toggle(p)} label={p} />
              ))}
            </div>
          </div>
        </fieldset>
        <ErrorText message={error ?? undefined} />
        <div className="modal-actions">
          <Button onClick={onClose}>Cancel</Button>
          <Button type="submit" variant="primary" loading={grant.isPending} disabled={selected.size === 0}>
            {grant.isPending ? "Granting…" : "Grant"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

export function SetPasswordModal({ instanceId, user, onClose }: { instanceId: string; user: DBUser; onClose: () => void }) {
  const setPassword = useSetDBUserPassword();
  const [password, setPw] = useState("");
  const [error, setError] = useState<string | null>(null);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      await setPassword.mutateAsync({ instanceId, username: user.user, host: user.host, password });
      onClose();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to change the password");
    }
  }

  return (
    <Modal open onClose={onClose} title={`Set password for ${user.user}`}>
      <form onSubmit={onSubmit}>
        <Field label="New password" hint="Existing sessions stay connected; new logins need the new password.">
          <input
            className="input"
            type="password"
            value={password}
            onChange={(e) => setPw(e.target.value)}
            autoComplete="new-password"
            required
            minLength={8}
          />
        </Field>
        <ErrorText message={error ?? undefined} />
        <div className="modal-actions">
          <Button onClick={onClose}>Cancel</Button>
          <Button type="submit" variant="primary" loading={setPassword.isPending}>
            {setPassword.isPending ? "Saving…" : "Set password"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
