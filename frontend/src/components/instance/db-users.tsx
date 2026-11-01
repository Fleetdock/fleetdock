"use client";

import { useState } from "react";

import { ChevronDown, ChevronRight, KeyRound, Plus, Trash2, Users } from "lucide-react";
import { useToast } from "@/components/toast";
import {
  Button,
  Callout,
  ConfirmModal,
  EmptyState,
  IconButton,
  Pagination,
  SectionHeader,
  Skeleton,
  Table,
  TableSkeleton,
} from "@/components/ui";
import { friendlyError } from "@/lib/errors";
import { useClientPage, useDBUsers, useDropDBUser, useUserGrants } from "@/lib/hooks";
import type { DBUser } from "@/lib/types";

import { CreateDBUserModal, GrantModal, SetPasswordModal } from "./db-user-modals";

/** DBUsersSection lists the login accounts on a database server, live, with their grants. */
export function DBUsersSection({
  instanceId,
  canWrite,
  databases,
}: {
  instanceId: string;
  canWrite: boolean;
  databases: { id: string; name: string }[];
}) {
  const { data, isLoading, error, refetch } = useDBUsers(instanceId);
  const paged = useClientPage(data?.items);
  const drop = useDropDBUser();
  const [createOpen, setCreateOpen] = useState(false);
  const [grantTarget, setGrantTarget] = useState<DBUser | null>(null);
  const [expanded, setExpanded] = useState<string | null>(null);
  const { push } = useToast();
  const [dropTarget, setDropTarget] = useState<DBUser | null>(null);
  const [passwordTarget, setPasswordTarget] = useState<DBUser | null>(null);

  async function confirmDrop() {
    if (!dropTarget) return;
    const target = dropTarget;
    setDropTarget(null);
    try {
      await drop.mutateAsync({ instanceId, username: target.user, host: target.host });
      push("success", `Database user ${target.user} removed`);
    } catch (err) {
      push("error", friendlyError(err, "Failed to remove the database user"));
    }
  }

  const createButton = canWrite ? (
    <Button variant="primary" icon={<Plus size={16} aria-hidden />} onClick={() => setCreateOpen(true)}>
      Create DB user
    </Button>
  ) : null;

  return (
    <section className="section">
      <SectionHeader
        title="Database users"
        icon={<Users size={17} aria-hidden />}
        description="Accounts on this instance (live)."
        actions={createButton}
      />

      {isLoading ? (
        <TableSkeleton rows={4} columns={canWrite ? 4 : 3} />
      ) : error ? (
        <Callout
          tone="danger"
          title="Could not reach the database server"
          actions={
            <Button size="sm" onClick={() => void refetch()}>
              Try again
            </Button>
          }
        >
          {friendlyError(error)}
        </Callout>
      ) : !data || data.items.length === 0 ? (
        <EmptyState
          icon={<Users size={22} />}
          title="No database users"
          hint={canWrite ? "Create a login for an application or a person." : "This server has no login accounts Fleetdock can see."}
          action={createButton}
        />
      ) : (
        <Table>
          <thead>
            <tr>
              <th className="w-10">
                <span className="sr-only">Show grants</span>
              </th>
              <th>User</th>
              <th>Host</th>
              {canWrite ? <th className="text-right">Actions</th> : null}
            </tr>
          </thead>
          <tbody>
            {paged.items.map((u) => {
              const key = `${u.user}@${u.host}`;
              const isOpen = expanded === key;
              return (
                <UserRow
                  key={key}
                  instanceId={instanceId}
                  user={u}
                  open={isOpen}
                  onToggle={() => setExpanded(isOpen ? null : key)}
                  canWrite={canWrite}
                  onGrant={() => setGrantTarget(u)}
                  onDrop={() => setDropTarget(u)}
                  onPassword={() => setPasswordTarget(u)}
                />
              );
            })}
          </tbody>
        </Table>
      )}

      <div className="mt-3 flex items-center justify-end">
        <Pagination page={paged.page} pageCount={paged.pageCount} onPage={paged.setPage} />
      </div>

      <CreateDBUserModal open={createOpen} onClose={() => setCreateOpen(false)} instanceId={instanceId} />
      <ConfirmModal
        open={dropTarget !== null}
        danger
        title="Drop database user?"
        confirmLabel="Drop user"
        confirmText={dropTarget?.user}
        busy={drop.isPending}
        message={
          <p className="mt-0">
            <code>
              {dropTarget?.user}@{dropTarget?.host}
            </code>{" "}
            loses access immediately; applications using it will fail to connect.
          </p>
        }
        onConfirm={() => void confirmDrop()}
        onCancel={() => setDropTarget(null)}
      />
      {passwordTarget ? (
        <SetPasswordModal instanceId={instanceId} user={passwordTarget} onClose={() => setPasswordTarget(null)} />
      ) : null}
      <GrantModal target={grantTarget} onClose={() => setGrantTarget(null)} instanceId={instanceId} databases={databases} />
    </section>
  );
}

function UserRow({
  instanceId,
  user,
  open,
  onToggle,
  canWrite,
  onGrant,
  onDrop,
  onPassword,
}: {
  instanceId: string;
  user: DBUser;
  open: boolean;
  onToggle: () => void;
  canWrite: boolean;
  onGrant: () => void;
  onDrop: () => void;
  onPassword: () => void;
}) {
  const grantsId = `grants-${user.user}-${user.host}`.replace(/[^a-zA-Z0-9_-]/g, "_");
  return (
    <>
      <tr className="cursor-pointer" onClick={onToggle}>
        <td onClick={(e) => e.stopPropagation()}>
          {/* The whole row toggles on click; this button is the keyboard and screen-reader way in. */}
          <IconButton
            label={open ? `Hide grants for ${user.user}` : `Show grants for ${user.user}`}
            aria-expanded={open}
            aria-controls={open ? grantsId : undefined}
            onClick={onToggle}
            noTooltip
          >
            {open ? <ChevronDown size={15} aria-hidden /> : <ChevronRight size={15} aria-hidden />}
          </IconButton>
        </td>
        <td className="font-medium">{user.user}</td>
        <td className="text-muted">
          <code>{user.host}</code>
        </td>
        {canWrite ? (
          <td className="text-right" onClick={(e) => e.stopPropagation()}>
            <div className="flex items-center justify-end gap-2">
              <Button size="sm" onClick={onGrant}>
                Grant…
              </Button>
              <IconButton label="Set password" variant="secondary" onClick={onPassword}>
                <KeyRound size={15} aria-hidden />
              </IconButton>
              <IconButton label="Drop user" variant="danger" onClick={onDrop}>
                <Trash2 size={15} aria-hidden />
              </IconButton>
            </div>
          </td>
        ) : null}
      </tr>
      {open ? (
        <tr id={grantsId}>
          <td colSpan={canWrite ? 4 : 3} className="bg-panel-2">
            <GrantsList instanceId={instanceId} user={user} />
          </td>
        </tr>
      ) : null}
    </>
  );
}

function GrantsList({ instanceId, user }: { instanceId: string; user: DBUser }) {
  const { data, isLoading, error } = useUserGrants(instanceId, user.user, user.host);
  if (isLoading) {
    return (
      <div className="stack-sm py-1" role="status" aria-label="Loading grants">
        <Skeleton width="60%" height=".8rem" />
        <Skeleton width="45%" height=".8rem" />
      </div>
    );
  }
  if (error) return <span className="text-sm text-danger">{friendlyError(error)}</span>;
  const grants = data?.items ?? [];
  if (grants.length === 0) return <span className="text-sm text-muted">No grants yet.</span>;
  return (
    <div className="flex flex-col gap-1 py-1">
      {grants.map((g, i) => (
        <code key={i} className="text-xs">
          {g}
        </code>
      ))}
    </div>
  );
}
