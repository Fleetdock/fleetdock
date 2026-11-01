"use client";

import { Shield, Trash2 } from "lucide-react";
import { useCallback, useState, type FormEvent } from "react";

import { Badge, Button, Callout, EmptyState, Field, IconButton, Modal, Select, Skeleton } from "@/components/ui";
import { friendlyError } from "@/lib/errors";
import { useAddGrant, useDatabases, useRemoveGrant, useRoleGrants, useServers } from "@/lib/hooks";
import type { Role, RoleGrant, ScopeType, User } from "@/lib/types";

function scopeLabel(g: RoleGrant, serverName: (id: string) => string, dbName: (id: string) => string): string {
  if (g.scope_type === "global") return "everything";
  if (g.scope_type === "server") return `server: ${serverName(g.scope_id ?? "")}`;
  return `database: ${dbName(g.scope_id ?? "")}`;
}

/** ManageGrantsModal lists a user's scoped role grants and adds or removes them. */
export function ManageGrantsModal({ user, onClose, roles }: { user: User | null; onClose: () => void; roles: Role[] }) {
  const { data, isLoading, error, refetch } = useRoleGrants(user?.id ?? "");
  const { data: servers } = useServers();
  const { data: databases } = useDatabases();
  const add = useAddGrant();
  const remove = useRemoveGrant();

  const [role, setRole] = useState("viewer");
  const [scopeType, setScopeType] = useState<ScopeType>("global");
  const [scopeId, setScopeId] = useState("");
  const [scopeError, setScopeError] = useState<string | null>(null);
  const [formError, setFormError] = useState<string | null>(null);

  const serverName = useCallback(
    (id: string) => servers?.items.find((s) => s.id === id)?.name ?? id.slice(0, 8),
    [servers],
  );
  const dbName = useCallback(
    (id: string) => databases?.items.find((d) => d.id === id)?.name ?? id.slice(0, 8),
    [databases],
  );

  async function onAdd(e: FormEvent) {
    e.preventDefault();
    setFormError(null);
    setScopeError(null);
    if (scopeType !== "global" && !scopeId) {
      setScopeError(scopeType === "server" ? "Choose a server." : "Choose a database.");
      return;
    }
    try {
      await add.mutateAsync({
        userId: user!.id,
        role,
        scope_type: scopeType,
        scope_id: scopeType === "global" ? undefined : scopeId,
      });
      setScopeId("");
    } catch (err) {
      setFormError(friendlyError(err, "Failed to add grant"));
    }
  }

  async function onRemove(g: RoleGrant) {
    setFormError(null);
    try {
      await remove.mutateAsync({ userId: user!.id, grantId: g.id });
    } catch (err) {
      setFormError(friendlyError(err, "Failed to remove grant"));
    }
  }

  if (!user) return null;
  const grants = data?.items ?? [];
  return (
    <Modal open onClose={onClose} title={`Role grants — ${user.email}`}>
      <p className="text-sm muted mb-3">
        Grant a role globally or scoped to a single server or database. Server grants also cover that server&apos;s
        instances and databases.
      </p>

      {isLoading ? (
        <div className="stack-sm mb-4" role="status" aria-label="Loading grants">
          <Skeleton height="2.4rem" radius="var(--radius-md)" />
          <Skeleton height="2.4rem" radius="var(--radius-md)" />
        </div>
      ) : error ? (
        <Callout
          tone="danger"
          className="mb-4"
          title="Couldn't load grants"
          actions={
            <Button size="sm" onClick={() => void refetch()}>
              Try again
            </Button>
          }
        >
          {friendlyError(error)}
        </Callout>
      ) : grants.length === 0 ? (
        <div className="mb-4">
          <EmptyState icon={<Shield size={20} />} title="No grants yet" hint="Pick a role and a scope below, then add it." />
        </div>
      ) : (
        <ul className="stack-sm mb-4 list-none p-0">
          {grants.map((g) => {
            const scope = scopeLabel(g, serverName, dbName);
            return (
              <li key={g.id} className="card flex items-center justify-between gap-2 px-3 py-2">
                <span className="text-sm flex items-center gap-2 min-w-0">
                  <Badge tone="accent">{g.role}</Badge>
                  <span className="truncate">{scope}</span>
                </span>
                <IconButton
                  label={`Remove ${g.role} on ${scope}`}
                  variant="danger"
                  onClick={() => onRemove(g)}
                  disabled={remove.isPending}
                >
                  <Trash2 size={14} aria-hidden />
                </IconButton>
              </li>
            );
          })}
        </ul>
      )}

      <form onSubmit={onAdd}>
        <div className="form-grid">
          <Field label="Role">
            <Select value={role} onChange={(e) => setRole(e.target.value)}>
              {roles.map((r) => (
                <option key={r.id} value={r.name}>
                  {r.name}
                </option>
              ))}
            </Select>
          </Field>
          <Field label="Scope">
            <Select
              value={scopeType}
              onChange={(e) => {
                setScopeType(e.target.value as ScopeType);
                setScopeId("");
                setScopeError(null);
              }}
            >
              <option value="global">Global</option>
              <option value="server">Server</option>
              <option value="database">Database</option>
            </Select>
          </Field>
        </div>
        {scopeType === "server" ? (
          <Field label="Server" error={scopeError ?? undefined}>
            <Select value={scopeId} onChange={(e) => setScopeId(e.target.value)}>
              <option value="">Select a server…</option>
              {servers?.items.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name}
                </option>
              ))}
            </Select>
          </Field>
        ) : scopeType === "database" ? (
          <Field label="Database" error={scopeError ?? undefined}>
            <Select value={scopeId} onChange={(e) => setScopeId(e.target.value)}>
              <option value="">Select a database…</option>
              {databases?.items.map((d) => (
                <option key={d.id} value={d.id}>
                  {d.name}
                </option>
              ))}
            </Select>
          </Field>
        ) : null}
        {formError ? (
          <Callout tone="danger" className="mb-3">
            {formError}
          </Callout>
        ) : null}
        <div className="modal-actions">
          <Button onClick={onClose}>Close</Button>
          <Button type="submit" variant="primary" loading={add.isPending}>
            {add.isPending ? "Adding…" : "Add grant"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
