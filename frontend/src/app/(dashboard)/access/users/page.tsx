"use client";

import { KeyRound, Pencil, Plus, Shield, Trash2 } from "lucide-react";
import { useCallback, useMemo, useState } from "react";

import { ManageGrantsModal } from "@/components/access/grants-modal";
import { CreateUserModal, EditUserModal, ResetPasswordModal } from "@/components/access/user-modals";
import { useConfirm } from "@/components/confirm";
import { DataTable, type DataTableColumn } from "@/components/data-table";
import { useToast } from "@/components/toast";
import { Button, IconButton, PageHeader, StatusBadge, Time } from "@/components/ui";
import { friendlyError } from "@/lib/errors";
import { useDeleteUser, useMe, useRoles, useUsers } from "@/lib/hooks";
import type { User } from "@/lib/types";
import { useDataTable } from "@/lib/use-data-table";

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
  const { data, isLoading, error, refetch } = useUsers();
  const { data: roles } = useRoles();
  const [createOpen, setCreateOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<User | null>(null);
  const [resetTarget, setResetTarget] = useState<User | null>(null);
  const [grantsTarget, setGrantsTarget] = useState<User | null>(null);
  const confirm = useConfirm();
  const { push } = useToast();
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
      const ok = await confirm({
        title: `Delete ${u.email}?`,
        message: "They are signed out and can no longer log in. This can't be undone.",
        confirmLabel: "Delete user",
        danger: true,
      });
      if (!ok) return;
      try {
        await del.mutateAsync(u.id);
        push("success", `${u.email} deleted`);
      } catch (err) {
        push("error", friendlyError(err, "Failed to delete the user"));
      }
    },
    [confirm, del, push],
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
        render: (u) => <Time value={u.created_at} />,
      },
    ];
    if (canWrite) {
      cols.push({
        id: "actions",
        header: "Actions",
        align: "right",
        render: (u) => (
          <div className="flex items-center justify-end gap-1">
            <IconButton label={`Edit ${u.email}`} onClick={() => setEditTarget(u)}>
              <Pencil size={15} aria-hidden />
            </IconButton>
            <IconButton label={`Reset password for ${u.email}`} onClick={() => setResetTarget(u)}>
              <KeyRound size={15} aria-hidden />
            </IconButton>
            <IconButton label={`Manage role grants for ${u.email}`} onClick={() => setGrantsTarget(u)}>
              <Shield size={15} aria-hidden />
            </IconButton>
            {me?.id !== u.id ? (
              <IconButton label={`Delete ${u.email}`} variant="danger" onClick={() => onDelete(u)} disabled={del.isPending}>
                <Trash2 size={15} aria-hidden />
              </IconButton>
            ) : null}
          </div>
        ),
      });
    }
    return cols;
  }, [canWrite, del.isPending, me?.id, onDelete]);

  return (
    <div>
      <PageHeader
        title="Users"
        description="People who can sign in to Fleetdock, and what each of them may do."
        actions={
          canWrite ? (
            <Button variant="primary" icon={<Plus size={16} aria-hidden />} onClick={() => setCreateOpen(true)}>
              Add user
            </Button>
          ) : null
        }
      />

      <DataTable<User>
        columns={columns}
        rows={table.rows}
        rowKey={(u) => u.id}
        isLoading={isLoading}
        error={error ? friendlyError(error) : undefined}
        errorTitle="Could not load users"
        onRetry={() => void refetch()}
        emptyTitle="No users yet"
        emptyHint={canWrite ? "Add the people who should be able to sign in to Fleetdock." : undefined}
        emptyAction={
          canWrite ? (
            <Button variant="primary" icon={<Plus size={16} aria-hidden />} onClick={() => setCreateOpen(true)}>
              Add user
            </Button>
          ) : undefined
        }
        emptySearchHint="Try a different name or email."
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
