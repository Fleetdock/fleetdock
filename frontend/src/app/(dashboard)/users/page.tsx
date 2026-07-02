"use client";

import { useEffect, useState, type FormEvent } from "react";

import { KeyRound, Pencil, Plus, Trash2 } from "lucide-react";
import { EmptyState, ErrorText, Field, Modal, Pagination, Spinner, StatusBadge } from "@/components/ui";
import { ApiError } from "@/lib/api";
import {
  useClientPage,
  useCreateUser,
  useDeleteUser,
  useMe,
  useResetUserPassword,
  useRoles,
  useUpdateUser,
  useUsers,
} from "@/lib/hooks";
import type { Role, User } from "@/lib/types";

export default function UsersPage() {
  const { data: me } = useMe();
  const { data, isLoading, error } = useUsers();
  const { data: roles } = useRoles();
  const [createOpen, setCreateOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<User | null>(null);
  const [resetTarget, setResetTarget] = useState<User | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const del = useDeleteUser();
  const canWrite = me?.permissions.includes("user:write") ?? false;
  const paged = useClientPage(data?.items);

  async function onDelete(u: User) {
    if (!confirm(`Delete user "${u.email}" permanently?`)) return;
    setNotice(null);
    try {
      await del.mutateAsync(u.id);
    } catch (err) {
      setNotice(err instanceof ApiError ? err.message : "Failed to delete user");
    }
  }

  return (
    <div>
      <div className="flex items-center justify-between" style={{ marginBottom: "1.1rem" }}>
        <div>
          <h1 className="text-xl font-semibold">Users</h1>
          <p className="muted text-sm">Operator accounts and their roles.</p>
        </div>
        {canWrite ? (
          <button className="btn btn-primary" onClick={() => setCreateOpen(true)}>
            <Plus size={16} /> Add user
          </button>
        ) : null}
      </div>

      {notice ? (
        <div className="card" style={{ padding: ".7rem .9rem", marginBottom: ".9rem" }}>
          <span className="text-sm">{notice}</span>
        </div>
      ) : null}

      {isLoading ? (
        <div className="flex items-center gap-2 muted text-sm"><Spinner /> Loading…</div>
      ) : error ? (
        <EmptyState title="Could not load users" hint={(error as ApiError).message} />
      ) : !data || data.items.length === 0 ? (
        <EmptyState title="No users" />
      ) : (
        <div className="card" style={{ overflow: "hidden" }}>
          <table className="table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Email</th>
                <th>Role</th>
                <th>Status</th>
                <th>Created</th>
                {canWrite ? <th style={{ textAlign: "right" }}>Actions</th> : null}
              </tr>
            </thead>
            <tbody>
              {paged.items.map((u) => (
                <tr key={u.id}>
                  <td className="font-medium">
                    {u.name}
                    {me?.id === u.id ? <span className="muted"> (you)</span> : null}
                  </td>
                  <td className="muted">{u.email}</td>
                  <td className="muted">{u.roles.join(", ") || "—"}</td>
                  <td><StatusBadge status={u.status} /></td>
                  <td className="muted">{new Date(u.created_at).toLocaleDateString()}</td>
                  {canWrite ? (
                    <td style={{ textAlign: "right" }}>
                      <div className="flex items-center gap-2" style={{ justifyContent: "flex-end" }}>
                        <button className="btn btn-sm" onClick={() => setEditTarget(u)} aria-label={`Edit ${u.email}`}>
                          <Pencil size={15} />
                        </button>
                        <button className="btn btn-sm" onClick={() => setResetTarget(u)} title="Reset password" aria-label={`Reset password for ${u.email}`}>
                          <KeyRound size={15} />
                        </button>
                        {me?.id !== u.id ? (
                          <button className="btn btn-sm btn-danger" onClick={() => onDelete(u)} disabled={del.isPending} aria-label={`Delete ${u.email}`}>
                            <Trash2 size={15} />
                          </button>
                        ) : null}
                      </div>
                    </td>
                  ) : null}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <div className="flex items-center justify-end" style={{ marginTop: ".6rem" }}>
        <Pagination page={paged.page} pageCount={paged.pageCount} onPage={paged.setPage} />
      </div>

      <CreateUserModal open={createOpen} onClose={() => setCreateOpen(false)} roles={roles?.items ?? []} />
      <EditUserModal user={editTarget} onClose={() => setEditTarget(null)} roles={roles?.items ?? []} isSelf={editTarget?.id === me?.id} />
      <ResetPasswordModal user={resetTarget} onClose={() => setResetTarget(null)} />
    </div>
  );
}

function CreateUserModal({ open, onClose, roles }: { open: boolean; onClose: () => void; roles: Role[] }) {
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
          <input className="input" value={name} onChange={(e) => setName(e.target.value)} required />
        </Field>
        <Field label="Email">
          <input className="input" type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
        </Field>
        <Field label="Password (min 8 characters)">
          <input className="input" type="password" value={password} onChange={(e) => setPassword(e.target.value)} minLength={8} autoComplete="new-password" required />
        </Field>
        <Field label="Role">
          <select className="input" value={role} onChange={(e) => setRole(e.target.value)}>
            {roles.map((r) => (
              <option key={r.id} value={r.name}>{r.name} — {r.description}</option>
            ))}
          </select>
        </Field>
        <ErrorText message={error ?? undefined} />
        <div className="flex items-center justify-end gap-2" style={{ marginTop: ".5rem" }}>
          <button type="button" className="btn" onClick={onClose}>Cancel</button>
          <button type="submit" className="btn btn-primary" disabled={create.isPending}>
            {create.isPending ? "Creating…" : "Create user"}
          </button>
        </div>
      </form>
    </Modal>
  );
}

function EditUserModal({ user, onClose, roles, isSelf }: { user: User | null; onClose: () => void; roles: Role[]; isSelf: boolean }) {
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
          <input className="input" value={name} onChange={(e) => setName(e.target.value)} required />
        </Field>
        <Field label="Email">
          <input className="input" type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
        </Field>
        <Field label="Role">
          <select className="input" value={role} onChange={(e) => setRole(e.target.value)}>
            {roles.map((r) => (
              <option key={r.id} value={r.name}>{r.name} — {r.description}</option>
            ))}
          </select>
        </Field>
        <Field label="Status">
          <select className="input" value={status} onChange={(e) => setStatus(e.target.value)} disabled={isSelf}>
            <option value="active">active</option>
            <option value="suspended">suspended</option>
          </select>
        </Field>
        {isSelf ? <p className="muted text-sm">You cannot suspend your own account.</p> : null}
        <ErrorText message={error ?? undefined} />
        <div className="flex items-center justify-end gap-2" style={{ marginTop: ".5rem" }}>
          <button type="button" className="btn" onClick={onClose}>Cancel</button>
          <button type="submit" className="btn btn-primary" disabled={update.isPending}>
            {update.isPending ? "Saving…" : "Save changes"}
          </button>
        </div>
      </form>
    </Modal>
  );
}

function ResetPasswordModal({ user, onClose }: { user: User | null; onClose: () => void }) {
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
      setError(err instanceof ApiError ? err.message : "Failed to reset password");
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
          <p className="text-sm">Password updated. Share it with the user securely.</p>
          <div className="flex items-center justify-end" style={{ marginTop: ".8rem" }}>
            <button className="btn btn-primary" onClick={close}>Done</button>
          </div>
        </div>
      ) : (
        <form onSubmit={onSubmit}>
          <Field label="New password (min 8 characters)">
            <input className="input" type="password" value={password} onChange={(e) => setPassword(e.target.value)} minLength={8} autoComplete="new-password" required />
          </Field>
          <ErrorText message={error ?? undefined} />
          <div className="flex items-center justify-end gap-2" style={{ marginTop: ".5rem" }}>
            <button type="button" className="btn" onClick={close}>Cancel</button>
            <button type="submit" className="btn btn-primary" disabled={reset.isPending}>
              {reset.isPending ? "Saving…" : "Set password"}
            </button>
          </div>
        </form>
      )}
    </Modal>
  );
}
