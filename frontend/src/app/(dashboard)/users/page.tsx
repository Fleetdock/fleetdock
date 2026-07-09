"use client";

import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type FormEvent,
} from "react";

import { DataTable, type DataTableColumn } from "@/components/data-table";
import { EmptyState, ErrorText, Field, Modal, Spinner, StatusBadge } from "@/components/ui";
import { ApiError } from "@/lib/api";
import {
  useAddGrant,
  useCreateUser,
  useDatabases,
  useDeleteUser,
  useMe,
  useRemoveGrant,
  useResetUserPassword,
  useRoleGrants,
  useRoles,
  useServers,
  useUpdateUser,
  useUsers,
} from "@/lib/hooks";
import type { Role, RoleGrant, ScopeType, User } from "@/lib/types";
import { useDataTable } from "@/lib/use-data-table";
import { KeyRound, Pencil, Plus, Shield, Trash2 } from "lucide-react";

function compareUsers(a: User, b: User, key: string) {
  if (key === "created_at") {
    return new Date(a.created_at).getTime() - new Date(b.created_at).getTime();
  }
  return String(a[key as keyof User] ?? "").localeCompare(
    String(b[key as keyof User] ?? ""),
  );
}

export default function UsersPage() {
  const { data: me } = useMe();
  const { data, isLoading, error } = useUsers();
  const { data: roles } = useRoles();
  const [createOpen, setCreateOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<User | null>(null);
  const [resetTarget, setResetTarget] = useState<User | null>(null);
  const [grantsTarget, setGrantsTarget] = useState<User | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [search, setSearch] = useState("");

  const del = useDeleteUser();
  const canWrite = me?.permissions.includes("user:write") ?? false;
  const table = useDataTable({
    items: data?.items,
    search: {
      query: search,
      match: (u, q) =>
        u.name.toLowerCase().includes(q) || u.email.toLowerCase().includes(q),
    },
    sort: { key: "name", dir: "asc", compare: compareUsers },
  });

  const onDelete = useCallback(
    async (u: User) => {
      if (!confirm(`Delete user "${u.email}" permanently?`)) return;
      setNotice(null);
      try {
        await del.mutateAsync(u.id);
      } catch (err) {
        setNotice(
          err instanceof ApiError ? err.message : "Failed to delete user",
        );
      }
    },
    [del],
  );

  const columns = useMemo(() => {
    const cols: DataTableColumn<User>[] = [
      {
        id: "name",
        header: "Name",
        sortable: true,
        sortKey: "name",
        className: "font-medium",
        render: (u) => (
          <>
            {u.name}
            {me?.id === u.id ? <span className="muted"> (you)</span> : null}
          </>
        ),
      },
      {
        id: "email",
        header: "Email",
        sortable: true,
        sortKey: "email",
        className: "muted",
        render: (u) => u.email,
      },
      {
        id: "role",
        header: "Role",
        className: "muted",
        render: (u) => u.roles.join(", ") || "—",
      },
      {
        id: "status",
        header: "Status",
        render: (u) => <StatusBadge status={u.status} />,
      },
      {
        id: "created",
        header: "Created",
        sortable: true,
        sortKey: "created_at",
        className: "muted",
        render: (u) => new Date(u.created_at).toLocaleDateString(),
      },
    ];
    if (canWrite) {
      cols.push({
        id: "actions",
        header: "Actions",
        align: "right",
        render: (u) => (
          <div
            className="flex items-center gap-2"
            style={{ justifyContent: "flex-end" }}
          >
            <button
              className="btn btn-sm"
              onClick={() => setEditTarget(u)}
              aria-label={`Edit ${u.email}`}
            >
              <Pencil size={15} />
            </button>
            <button
              className="btn btn-sm"
              onClick={() => setResetTarget(u)}
              title="Reset password"
              aria-label={`Reset password for ${u.email}`}
            >
              <KeyRound size={15} />
            </button>
            <button
              className="btn btn-sm"
              onClick={() => setGrantsTarget(u)}
              title="Scoped role grants"
              aria-label={`Manage role grants for ${u.email}`}
            >
              <Shield size={15} />
            </button>
            {me?.id !== u.id ? (
              <button
                className="btn btn-sm btn-danger"
                onClick={() => onDelete(u)}
                disabled={del.isPending}
                aria-label={`Delete ${u.email}`}
              >
                <Trash2 size={15} />
              </button>
            ) : null}
          </div>
        ),
      });
    }
    return cols;
  }, [canWrite, del.isPending, me?.id, onDelete]);

  return (
    <div>
      <div
        className="flex justify-between items-center"
        style={{ marginBottom: "1.1rem" }}
      >
        <div>
          <h1 className="font-semibold text-xl">Users</h1>
          <p className="text-sm muted">Operator accounts and their roles.</p>
        </div>
        {canWrite ? (
          <button
            className="btn btn-primary"
            onClick={() => setCreateOpen(true)}
          >
            <Plus size={16} /> Add user
          </button>
        ) : null}
      </div>

      {notice ? (
        <div
          className="card"
          style={{ padding: ".7rem .9rem", marginBottom: ".9rem" }}
        >
          <span className="text-sm">{notice}</span>
        </div>
      ) : null}

      <DataTable<User>
        columns={columns}
        rows={table.rows}
        rowKey={(u) => u.id}
        isLoading={isLoading}
        error={error ? (error as ApiError).message : undefined}
        errorTitle="Could not load users"
        emptyTitle="No users"
        emptySearchTitle="No users match your search"
        search={{
          value: search,
          onChange: (v) => {
            setSearch(v);
            table.setPage(1);
          },
          placeholder: "Search name or email…",
        }}
        sort={{ key: table.sortKey, dir: table.sortDir, onSort: table.setSort }}
        pagination={{
          page: table.page,
          pageCount: table.pageCount,
          onPage: table.setPage,
        }}
      />

      <CreateUserModal
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        roles={roles?.items ?? []}
      />
      <EditUserModal
        user={editTarget}
        onClose={() => setEditTarget(null)}
        roles={roles?.items ?? []}
        isSelf={editTarget?.id === me?.id}
      />
      <ResetPasswordModal
        user={resetTarget}
        onClose={() => setResetTarget(null)}
      />
      <ManageGrantsModal
        user={grantsTarget}
        onClose={() => setGrantsTarget(null)}
        roles={roles?.items ?? []}
      />
    </div>
  );
}

function scopeLabel(
  g: RoleGrant,
  serverName: (id: string) => string,
  dbName: (id: string) => string,
): string {
  if (g.scope_type === "global") return "everything";
  if (g.scope_type === "server") return `server: ${serverName(g.scope_id ?? "")}`;
  return `database: ${dbName(g.scope_id ?? "")}`;
}

function ManageGrantsModal({
  user,
  onClose,
  roles,
}: {
  user: User | null;
  onClose: () => void;
  roles: Role[];
}) {
  const { data, isLoading, error } = useRoleGrants(user?.id ?? "");
  const { data: servers } = useServers();
  const { data: databases } = useDatabases();
  const add = useAddGrant();
  const remove = useRemoveGrant();

  const [role, setRole] = useState("viewer");
  const [scopeType, setScopeType] = useState<ScopeType>("global");
  const [scopeId, setScopeId] = useState("");
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
    if (scopeType !== "global" && !scopeId) {
      setFormError("Choose a scope target.");
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
      setFormError(err instanceof ApiError ? err.message : "Failed to add grant");
    }
  }

  async function onRemove(g: RoleGrant) {
    setFormError(null);
    try {
      await remove.mutateAsync({ userId: user!.id, grantId: g.id });
    } catch (err) {
      setFormError(err instanceof ApiError ? err.message : "Failed to remove grant");
    }
  }

  if (!user) return null;
  return (
    <Modal open onClose={onClose} title={`Role grants — ${user.email}`}>
      <p className="text-sm muted" style={{ marginBottom: ".8rem" }}>
        Grant a role globally or scoped to a single server or database. Server
        grants also cover that server&apos;s instances and databases.
      </p>

      {isLoading ? (
        <Spinner />
      ) : error ? (
        <ErrorText message={(error as ApiError).message} />
      ) : (data?.items.length ?? 0) === 0 ? (
        <EmptyState title="No grants yet" hint="Add one below." />
      ) : (
        <div className="flex flex-col gap-2" style={{ marginBottom: "1rem" }}>
          {data!.items.map((g) => (
            <div
              key={g.id}
              className="card flex items-center justify-between"
              style={{ padding: ".5rem .75rem" }}
            >
              <span className="text-sm">
                <span className="font-medium">{g.role}</span> @{" "}
                {scopeLabel(g, serverName, dbName)}
              </span>
              <button
                className="btn btn-sm btn-danger"
                onClick={() => onRemove(g)}
                disabled={remove.isPending}
                aria-label="Remove grant"
              >
                <Trash2 size={14} />
              </button>
            </div>
          ))}
        </div>
      )}

      <form onSubmit={onAdd}>
        <div className="grid" style={{ gridTemplateColumns: "1fr 1fr", gap: ".6rem" }}>
          <Field label="Role">
            <select className="input" value={role} onChange={(e) => setRole(e.target.value)}>
              {roles.map((r) => (
                <option key={r.id} value={r.name}>
                  {r.name}
                </option>
              ))}
            </select>
          </Field>
          <Field label="Scope">
            <select
              className="input"
              value={scopeType}
              onChange={(e) => {
                setScopeType(e.target.value as ScopeType);
                setScopeId("");
              }}
            >
              <option value="global">Global</option>
              <option value="server">Server</option>
              <option value="database">Database</option>
            </select>
          </Field>
        </div>
        {scopeType === "server" ? (
          <Field label="Server">
            <select className="input" value={scopeId} onChange={(e) => setScopeId(e.target.value)}>
              <option value="">Select a server…</option>
              {servers?.items.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name}
                </option>
              ))}
            </select>
          </Field>
        ) : scopeType === "database" ? (
          <Field label="Database">
            <select className="input" value={scopeId} onChange={(e) => setScopeId(e.target.value)}>
              <option value="">Select a database…</option>
              {databases?.items.map((d) => (
                <option key={d.id} value={d.id}>
                  {d.name}
                </option>
              ))}
            </select>
          </Field>
        ) : null}
        <ErrorText message={formError ?? undefined} />
        <div className="flex justify-end items-center gap-2" style={{ marginTop: ".5rem" }}>
          <button type="button" className="btn" onClick={onClose}>
            Close
          </button>
          <button type="submit" className="btn btn-primary" disabled={add.isPending}>
            {add.isPending ? "Adding…" : "Add grant"}
          </button>
        </div>
      </form>
    </Modal>
  );
}

function CreateUserModal({
  open,
  onClose,
  roles,
}: {
  open: boolean;
  onClose: () => void;
  roles: Role[];
}) {
  const create = useCreateUser();
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState("viewer");
  const [error, setError] = useState<string | null>(null);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      await create.mutateAsync({ name, email, password, role });
      setName("");
      setEmail("");
      setPassword("");
      onClose();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to create user");
    }
  }

  return (
    <Modal open={open} onClose={onClose} title="Add user">
      <form onSubmit={onSubmit}>
        <Field label="Name">
          <input
            className="input"
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
          />
        </Field>
        <Field label="Email">
          <input
            className="input"
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            required
          />
        </Field>
        <Field label="Password (min 8 characters)">
          <input
            className="input"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            minLength={8}
            autoComplete="new-password"
            required
          />
        </Field>
        <Field label="Role">
          <select
            className="input"
            value={role}
            onChange={(e) => setRole(e.target.value)}
          >
            {roles.map((r) => (
              <option key={r.id} value={r.name}>
                {r.name} — {r.description}
              </option>
            ))}
          </select>
        </Field>
        <ErrorText message={error ?? undefined} />
        <div
          className="flex justify-end items-center gap-2"
          style={{ marginTop: ".5rem" }}
        >
          <button type="button" className="btn" onClick={onClose}>
            Cancel
          </button>
          <button
            type="submit"
            className="btn btn-primary"
            disabled={create.isPending}
          >
            {create.isPending ? "Creating…" : "Create user"}
          </button>
        </div>
      </form>
    </Modal>
  );
}

function EditUserModal({
  user,
  onClose,
  roles,
  isSelf,
}: {
  user: User | null;
  onClose: () => void;
  roles: Role[];
  isSelf: boolean;
}) {
  const update = useUpdateUser();
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [role, setRole] = useState("");
  const [status, setStatus] = useState("active");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (user) {
      setName(user.name);
      setEmail(user.email);
      setRole(user.roles[0] ?? "viewer");
      setStatus(user.status);
      setError(null);
    }
  }, [user]);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      await update.mutateAsync({
        id: user!.id,
        name,
        email,
        role,
        status,
      });
      onClose();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to update user");
    }
  }

  if (!user) return null;
  return (
    <Modal open onClose={onClose} title={`Edit ${user.email}`}>
      <form onSubmit={onSubmit}>
        <Field label="Name">
          <input
            className="input"
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
          />
        </Field>
        <Field label="Email">
          <input
            className="input"
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            required
          />
        </Field>
        <Field label="Role">
          <select
            className="input"
            value={role}
            onChange={(e) => setRole(e.target.value)}
          >
            {roles.map((r) => (
              <option key={r.id} value={r.name}>
                {r.name} — {r.description}
              </option>
            ))}
          </select>
        </Field>
        <Field label="Status">
          <select
            className="input"
            value={status}
            onChange={(e) => setStatus(e.target.value)}
            disabled={isSelf}
          >
            <option value="active">active</option>
            <option value="suspended">suspended</option>
          </select>
        </Field>
        {isSelf ? (
          <p className="text-sm muted">You cannot suspend your own account.</p>
        ) : null}
        <ErrorText message={error ?? undefined} />
        <div
          className="flex justify-end items-center gap-2"
          style={{ marginTop: ".5rem" }}
        >
          <button type="button" className="btn" onClick={onClose}>
            Cancel
          </button>
          <button
            type="submit"
            className="btn btn-primary"
            disabled={update.isPending}
          >
            {update.isPending ? "Saving…" : "Save changes"}
          </button>
        </div>
      </form>
    </Modal>
  );
}

function ResetPasswordModal({
  user,
  onClose,
}: {
  user: User | null;
  onClose: () => void;
}) {
  const reset = useResetUserPassword();
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [done, setDone] = useState(false);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      await reset.mutateAsync({ id: user!.id, password });
      setDone(true);
    } catch (err) {
      setError(
        err instanceof ApiError ? err.message : "Failed to reset password",
      );
    }
  }

  function close() {
    setPassword("");
    setDone(false);
    setError(null);
    onClose();
  }

  if (!user) return null;
  return (
    <Modal open onClose={close} title={`Reset password for ${user.email}`}>
      {done ? (
        <div>
          <p className="text-sm">
            Password updated. Share it with the user securely.
          </p>
          <div
            className="flex justify-end items-center"
            style={{ marginTop: ".8rem" }}
          >
            <button className="btn btn-primary" onClick={close}>
              Done
            </button>
          </div>
        </div>
      ) : (
        <form onSubmit={onSubmit}>
          <Field label="New password (min 8 characters)">
            <input
              className="input"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              minLength={8}
              autoComplete="new-password"
              required
            />
          </Field>
          <ErrorText message={error ?? undefined} />
          <div
            className="flex justify-end items-center gap-2"
            style={{ marginTop: ".5rem" }}
          >
            <button type="button" className="btn" onClick={close}>
              Cancel
            </button>
            <button
              type="submit"
              className="btn btn-primary"
              disabled={reset.isPending}
            >
              {reset.isPending ? "Saving…" : "Set password"}
            </button>
          </div>
        </form>
      )}
    </Modal>
  );
}
