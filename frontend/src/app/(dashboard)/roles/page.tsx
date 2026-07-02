"use client";

import { useEffect, useMemo, useState, type FormEvent } from "react";

import { Lock, Pencil, Plus, Trash2 } from "lucide-react";
import { EmptyState, ErrorText, Field, Modal, Spinner } from "@/components/ui";
import { ApiError } from "@/lib/api";
import {
  useCreateRole,
  useDeleteRole,
  useMe,
  usePermissions,
  useRoles,
  useUpdateRole,
} from "@/lib/hooks";
import type { Role } from "@/lib/types";

export default function RolesPage() {
  const { data: me } = useMe();
  const { data, isLoading, error } = useRoles();
  const { data: permissions } = usePermissions();
  const [createOpen, setCreateOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<Role | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const del = useDeleteRole();
  const canWrite = me?.permissions.includes("user:write") ?? false;

  async function onDelete(role: Role) {
    if (!confirm(`Delete role "${role.name}"?`)) return;
    setNotice(null);
    try {
      await del.mutateAsync(role.id);
    } catch (err) {
      setNotice(err instanceof ApiError ? err.message : "Failed to delete role");
    }
  }

  return (
    <div>
      <div className="flex items-center justify-between" style={{ marginBottom: "1.1rem" }}>
        <div>
          <h1 className="text-xl font-semibold">Roles</h1>
          <p className="muted text-sm">
            Permission sets you can assign to users. System roles are fixed; custom roles are yours.
          </p>
        </div>
        {canWrite ? (
          <button className="btn btn-primary" onClick={() => setCreateOpen(true)}>
            <Plus size={16} /> Create role
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
        <EmptyState title="Could not load roles" hint={(error as ApiError).message} />
      ) : !data || data.items.length === 0 ? (
        <EmptyState title="No roles" />
      ) : (
        <div className="flex flex-col gap-3">
          {data.items.map((role) => (
            <div key={role.id} className="card" style={{ padding: "1rem 1.1rem" }}>
              <div className="flex items-center justify-between" style={{ marginBottom: ".55rem" }}>
                <div className="flex items-center gap-2">
                  <span className="font-semibold">{role.name}</span>
                  {role.is_system ? (
                    <span className="badge badge-gray" title="System roles cannot be edited">
                      <Lock size={11} /> system
                    </span>
                  ) : (
                    <span className="badge badge-amber">custom</span>
                  )}
                  {role.description ? <span className="muted text-sm">— {role.description}</span> : null}
                </div>
                {canWrite && !role.is_system ? (
                  <div className="flex items-center gap-2">
                    <button className="btn btn-sm" onClick={() => setEditTarget(role)} aria-label={`Edit ${role.name}`}>
                      <Pencil size={15} />
                    </button>
                    <button className="btn btn-sm btn-danger" onClick={() => onDelete(role)} disabled={del.isPending} aria-label={`Delete ${role.name}`}>
                      <Trash2 size={15} />
                    </button>
                  </div>
                ) : null}
              </div>
              <div className="flex" style={{ flexWrap: "wrap", gap: ".35rem" }}>
                {role.permissions.length === 0 ? (
                  <span className="muted text-sm">No permissions</span>
                ) : (
                  role.permissions.map((p) => (
                    <span key={p} className="badge badge-gray" style={{ fontFamily: "monospace", fontSize: 11 }}>
                      {p}
                    </span>
                  ))
                )}
              </div>
            </div>
          ))}
        </div>
      )}

      <RoleModal
        mode="create"
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        allPermissions={permissions?.items ?? []}
      />
      <RoleModal
        mode="edit"
        role={editTarget}
        open={editTarget !== null}
        onClose={() => setEditTarget(null)}
        allPermissions={permissions?.items ?? []}
      />
    </div>
  );
}

function groupPermissions(perms: string[]) {
  const groups = new Map<string, string[]>();
  for (const p of perms) {
    const [resource] = p.split(":");
    groups.set(resource, [...(groups.get(resource) ?? []), p]);
  }
  return [...groups.entries()];
}

type RoleModalProps = {
  mode: "create" | "edit";
  role?: Role | null;
  open: boolean;
  onClose: () => void;
  allPermissions: string[];
};

function RoleModal({ mode, role, open, onClose, allPermissions }: RoleModalProps) {
  const create = useCreateRole();
  const update = useUpdateRole();
  const isEdit = mode === "edit";

  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    if (isEdit && role) {
      setName(role.name);
      setDescription(role.description);
      setSelected(new Set(role.permissions));
    } else {
      setName("");
      setDescription("");
      setSelected(new Set());
    }
    setError(null);
  }, [open, isEdit, role]);

  const groups = useMemo(() => groupPermissions(allPermissions), [allPermissions]);
  const pending = create.isPending || update.isPending;

  function toggle(perm: string) {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(perm)) {
        next.delete(perm);
      } else {
        next.add(perm);
      }
      return next;
    });
  }

  function toggleGroup(perms: string[]) {
    setSelected((prev) => {
      const next = new Set(prev);
      const allOn = perms.every((p) => next.has(p));
      perms.forEach((p) => (allOn ? next.delete(p) : next.add(p)));
      return next;
    });
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    const payload = { name, description, permissions: [...selected] };
    try {
      if (isEdit && role) {
        await update.mutateAsync({ id: role.id, ...payload });
      } else {
        await create.mutateAsync(payload);
      }
      onClose();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : `Failed to ${isEdit ? "update" : "create"} role`);
    }
  }

  return (
    <Modal open={open} onClose={onClose} title={isEdit ? `Edit role "${role?.name}"` : "Create role"}>
      <form onSubmit={onSubmit}>
        <div className="grid" style={{ gridTemplateColumns: "1fr 1fr", gap: ".75rem" }}>
          <Field label="Name (lowercase, digits, - _)">
            <input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="backup-operator" required />
          </Field>
          <Field label="Description (optional)">
            <input className="input" value={description} onChange={(e) => setDescription(e.target.value)} placeholder="Can run and restore backups" />
          </Field>
        </div>
        <Field label="Permissions">
          <div className="card" style={{ padding: ".75rem", maxHeight: 280, overflowY: "auto" }}>
            {groups.map(([resource, perms]) => (
              <div key={resource} style={{ marginBottom: ".6rem" }}>
                <label className="flex items-center gap-2 font-medium text-sm" style={{ cursor: "pointer", marginBottom: ".25rem" }}>
                  <input
                    type="checkbox"
                    checked={perms.every((p) => selected.has(p))}
                    onChange={() => toggleGroup(perms)}
                  />
                  {resource}
                </label>
                <div className="flex" style={{ flexWrap: "wrap", gap: ".25rem .9rem", paddingLeft: "1.4rem" }}>
                  {perms.map((p) => (
                    <label key={p} className="flex items-center gap-1 text-sm muted" style={{ cursor: "pointer" }}>
                      <input type="checkbox" checked={selected.has(p)} onChange={() => toggle(p)} />
                      {p.split(":")[1]}
                    </label>
                  ))}
                </div>
              </div>
            ))}
          </div>
        </Field>
        <p className="muted text-sm">{selected.size} permission{selected.size === 1 ? "" : "s"} selected</p>
        <ErrorText message={error ?? undefined} />
        <div className="flex items-center justify-end gap-2" style={{ marginTop: ".5rem" }}>
          <button type="button" className="btn" onClick={onClose}>Cancel</button>
          <button type="submit" className="btn btn-primary" disabled={pending}>
            {pending ? "Saving…" : isEdit ? "Save changes" : "Create role"}
          </button>
        </div>
      </form>
    </Modal>
  );
}
