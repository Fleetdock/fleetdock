"use client";

import { Lock, Pencil, Plus, ShieldCheck, Trash2 } from "lucide-react";
import { useState } from "react";

import { RoleModal } from "@/components/access/role-modal";
import { useConfirm } from "@/components/confirm";
import { useToast } from "@/components/toast";
import { Badge, Button, Card, EmptyState, IconButton, PageHeader, QueryError, Skeleton } from "@/components/ui";
import { friendlyError } from "@/lib/errors";
import { useDeleteRole, useMe, usePermissions, useRoles } from "@/lib/hooks";
import type { Role } from "@/lib/types";

export default function RolesPage() {
  const { data: me } = useMe();
  const { data, isLoading, error, refetch } = useRoles();
  const { data: permissions } = usePermissions();
  const [createOpen, setCreateOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<Role | null>(null);
  const confirm = useConfirm();
  const { push } = useToast();

  const del = useDeleteRole();
  const canWrite = me?.permissions.includes("user:write") ?? false;

  async function onDelete(role: Role) {
    const ok = await confirm({
      title: `Delete role "${role.name}"?`,
      message: "Users who have this role lose its permissions right away.",
      confirmLabel: "Delete role",
      danger: true,
    });
    if (!ok) return;
    try {
      await del.mutateAsync(role.id);
      push("success", `Role "${role.name}" deleted`);
    } catch (err) {
      push("error", friendlyError(err, "Failed to delete the role"));
    }
  }

  const createButton = canWrite ? (
    <Button variant="primary" icon={<Plus size={16} aria-hidden />} onClick={() => setCreateOpen(true)}>
      Create role
    </Button>
  ) : null;

  return (
    <div>
      <PageHeader
        title="Roles"
        description="Named sets of permissions you give to users. Built-in roles can't be changed; you can add your own."
        actions={createButton}
      />

      {isLoading ? (
        <div className="stack" role="status" aria-label="Loading roles">
          {[0, 1, 2].map((i) => (
            <div key={i} className="card panel stack-sm" aria-hidden>
              <Skeleton width="30%" height=".95rem" />
              <Skeleton width="75%" height=".8rem" />
            </div>
          ))}
        </div>
      ) : error ? (
        <QueryError error={error} what="list of roles" onRetry={() => void refetch()} />
      ) : !data || data.items.length === 0 ? (
        <EmptyState
          icon={<ShieldCheck size={22} />}
          title="No roles yet"
          hint="Create a role to give users a named set of permissions."
          action={createButton}
        />
      ) : (
        <ul className="stack list-none m-0 p-0">
          {data.items.map((role) => (
            <li key={role.id}>
              <Card className="panel">
                <div className="flex items-start justify-between gap-2 mb-2">
                  <div className="flex flex-wrap items-center gap-2 min-w-0">
                    <h2 className="text-sm font-semibold m-0">{role.name}</h2>
                    {role.is_system ? (
                      <Badge tone="neutral" title="System roles cannot be edited">
                        <Lock size={11} aria-hidden /> system
                      </Badge>
                    ) : (
                      <Badge tone="warning">custom</Badge>
                    )}
                    {role.description ? <span className="muted text-sm">— {role.description}</span> : null}
                  </div>
                  {canWrite && !role.is_system ? (
                    <div className="flex items-center gap-1">
                      <IconButton label={`Edit ${role.name}`} onClick={() => setEditTarget(role)}>
                        <Pencil size={15} aria-hidden />
                      </IconButton>
                      <IconButton label={`Delete ${role.name}`} variant="danger" onClick={() => onDelete(role)} disabled={del.isPending}>
                        <Trash2 size={15} aria-hidden />
                      </IconButton>
                    </div>
                  ) : null}
                </div>
                <div className="flex flex-wrap gap-1.5">
                  {role.permissions.length === 0 ? (
                    <span className="muted text-sm">No permissions</span>
                  ) : (
                    role.permissions.map((p) => (
                      <Badge key={p} tone="neutral" className="mono">
                        {p}
                      </Badge>
                    ))
                  )}
                </div>
              </Card>
            </li>
          ))}
        </ul>
      )}

      <RoleModal mode="create" open={createOpen} onClose={() => setCreateOpen(false)} allPermissions={permissions?.items ?? []} />
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
