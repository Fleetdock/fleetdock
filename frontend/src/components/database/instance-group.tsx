"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";

import {
  Archive,
  ChevronDown,
  ChevronRight,
  Lock,
  Plus,
  Settings2,
  Trash2,
  Unlock,
} from "lucide-react";

import { HealthBadge } from "@/components/instance-monitoring";
import { useErrorToast } from "@/components/toast";
import {
  Badge,
  Button,
  ButtonLink,
  IconButton,
  Menu,
  Skeleton,
  StatusBadge,
  Table,
} from "@/components/ui";
import { engineLabel } from "@/lib/engines";
import { formatBytes } from "@/lib/format";
import { useDatabases, useLockDatabase, useUnlockDatabase } from "@/lib/hooks";
import type { Database, Instance } from "@/lib/types";

export type DatabaseActions = {
  canWrite: boolean;
  canBackup: boolean;
  onBackup: (d: Database) => void;
  onDelete: (d: Database) => void;
};

/** where describes where a database server runs, in plain words. */
export function instanceLocation(inst: Instance, serverName?: string): string {
  if (inst.kind === "external") return `${inst.host}:${inst.port}`;
  return `${serverName ?? "server"} · port ${inst.port}`;
}

/**
 * InstanceGroup is one database server on the Databases page: a summary row
 * (engine, where it runs, health, size) that expands to its databases.
 */
export function InstanceGroup({
  instance,
  serverName,
  search,
  defaultOpen,
  actions,
  onCreateDatabase,
  canManageInstance,
}: {
  instance: Instance;
  serverName?: string;
  search: string;
  defaultOpen: boolean;
  actions: DatabaseActions;
  onCreateDatabase: (instanceId: string) => void;
  canManageInstance: boolean;
}) {
  const router = useRouter();
  const [open, setOpen] = useState(defaultOpen);
  const { data, isLoading } = useDatabases({ instance_id: instance.id });
  const all = data?.items ?? [];
  const q = search.trim().toLowerCase();
  const nameMatches = !q || instance.name.toLowerCase().includes(q);
  const dbs = nameMatches
    ? all
    : all.filter((d) => d.name.toLowerCase().includes(q));
  const totalSize = all.reduce((n, d) => n + (d.size_bytes ?? 0), 0);

  // Hide groups that match neither by name nor by any database.
  if (q && !nameMatches && !isLoading && dbs.length === 0) return null;
  const expanded = open || (q !== "" && dbs.length > 0);

  return (
    <section className="instance-group card" aria-label={instance.name}>
      <div className="instance-group-head">
        <IconButton
          noTooltip
          aria-expanded={expanded}
          label={
            expanded ? `Collapse ${instance.name}` : `Expand ${instance.name}`
          }
          onClick={() => setOpen(!expanded)}
        >
          {expanded ? (
            <ChevronDown size={16} aria-hidden />
          ) : (
            <ChevronRight size={16} aria-hidden />
          )}
        </IconButton>
        <div className="instance-group-title">
          <Link href={`/instances/${instance.id}`} className="font-semibold">
            {instance.name}
          </Link>
          <span className="text-sm muted">
            {engineLabel(instance.engine)} {instance.engine_version} ·{" "}
            {instanceLocation(instance, serverName)}
          </span>
        </div>
        <div className="instance-group-meta">
          {instance.has_credentials ? (
            <HealthBadge health={instance.health} />
          ) : (
            <Badge
              tone="warning"
              title="Add an admin login so Fleetdock can manage this server"
            >
              not connected
            </Badge>
          )}
          <span className="text-sm muted hide-sm">
            {isLoading
              ? "…"
              : `${all.length} database${all.length === 1 ? "" : "s"}`}
            {totalSize ? ` · ${formatBytes(totalSize)}` : ""}
          </span>
          <Menu
            label={`Actions for ${instance.name}`}
            items={[
              {
                label: "Create database here",
                icon: <Plus size={15} aria-hidden />,
                onSelect: () => onCreateDatabase(instance.id),
                hidden: !actions.canWrite || !instance.has_credentials,
              },
              {
                label: "Open server details",
                icon: <Settings2 size={15} aria-hidden />,
                onSelect: () => router.push(`/instances/${instance.id}`),
                hidden: !canManageInstance,
              },
            ]}
          />
        </div>
      </div>

      {expanded ? (
        isLoading ? (
          <div
            role="status"
            aria-label={`Loading databases on ${instance.name}`}
            className="px-4 py-3 border-border border-t stack-sm"
          >
            {[0, 1, 2].map((i) => (
              <Skeleton key={i} width={`${70 - i * 15}%`} />
            ))}
          </div>
        ) : dbs.length === 0 ? (
          <div className="flex flex-wrap items-center gap-3 px-4 py-3 border-border border-t">
            <p className="flex-1 m-0 text-sm muted">
              {instance.has_credentials
                ? "No databases found yet. New ones appear here automatically."
                : "Fleetdock can't see this server's databases without an admin login."}
            </p>
            {instance.has_credentials ? (
              actions.canWrite ? (
                <Button
                  size="sm"
                  icon={<Plus size={15} aria-hidden />}
                  onClick={() => onCreateDatabase(instance.id)}
                >
                  Create database
                </Button>
              ) : null
            ) : canManageInstance ? (
              <ButtonLink size="sm" href={`/instances/${instance.id}`}>
                Add admin login
              </ButtonLink>
            ) : null}
          </div>
        ) : (
          <Table bare>
            <tbody>
              {dbs.map((d) => (
                <DatabaseRow key={d.id} database={d} actions={actions} />
              ))}
            </tbody>
          </Table>
        )
      ) : null}
    </section>
  );
}

/** DatabaseRow is one database inside a server group. */
export function DatabaseRow({
  database: d,
  actions,
}: {
  database: Database;
  actions: DatabaseActions;
}) {
  return (
    <tr>
      <td className="pl-[2.6rem]">
        <span className="flex items-center gap-2">
          <Link href={`/databases/${d.id}`} className="font-medium link-plain">
            {d.name}
          </Link>
          {d.system ? (
            <Badge title="Belongs to the database engine itself. You can browse and back it up, but not remove it.">
              system
            </Badge>
          ) : null}
        </span>
      </td>
      <td>
        <StatusBadge status={d.status} />
      </td>
      <td className="text-right muted hide-sm num">
        {formatBytes(d.size_bytes)}
      </td>
      <td className="text-right muted hide-sm num" title="Open connections">
        {d.active_connections ? `${d.active_connections} conn.` : ""}
      </td>
      <td className="w-px text-right">
        <DatabaseMenu database={d} actions={actions} />
      </td>
    </tr>
  );
}

/** DatabaseMenu holds a database's row actions: back up, lock/unlock, remove. */
export function DatabaseMenu({
  database: d,
  actions,
}: {
  database: Database;
  actions: DatabaseActions;
}) {
  const lock = useLockDatabase();
  const unlock = useUnlockDatabase();
  const toastError = useErrorToast();
  return (
    <Menu
      label={`Actions for ${d.name}`}
      items={[
        {
          label: "Back up now",
          icon: <Archive size={15} aria-hidden />,
          onSelect: () => actions.onBackup(d),
          hidden: !actions.canBackup,
        },
        {
          label: "Unlock",
          icon: <Unlock size={15} aria-hidden />,
          onSelect: () =>
            unlock.mutate(d.id, {
              onError: toastError("Failed to unlock the database"),
            }),
          hidden: !actions.canWrite || d.status !== "locked",
        },
        {
          label: "Lock (block writes)",
          icon: <Lock size={15} aria-hidden />,
          onSelect: () =>
            lock.mutate(d.id, {
              onError: toastError("Failed to lock the database"),
            }),
          hidden: !actions.canWrite || d.status !== "active",
        },
        {
          label: "Remove…",
          icon: <Trash2 size={15} aria-hidden />,
          danger: true,
          onSelect: () => actions.onDelete(d),
          hidden: !actions.canWrite || d.system,
        },
      ]}
    />
  );
}
