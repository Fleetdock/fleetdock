"use client";

import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { Suspense, useMemo, useState } from "react";

import { Database as DatabaseIcon, Plus, Search } from "lucide-react";

import { DataTable, type DataTableColumn } from "@/components/data-table";
import { AddServerWizard } from "@/components/database/add-server-wizard";
import { BackupDatabaseModal } from "@/components/database/backup-database-modal";
import { CreateDatabaseModal } from "@/components/database/create-database-modal";
import {
  DatabaseMenu,
  InstanceGroup,
  type DatabaseActions,
} from "@/components/database/instance-group";
import { DeleteDatabaseModal } from "@/components/delete-database-modal";
import {
  EmptyState,
  PageHeader,
  StatusBadge,
  TableSkeleton,
} from "@/components/ui";
import { friendlyError } from "@/lib/errors";
import { formatBytes } from "@/lib/format";
import {
  LIST_PAGE_SIZE,
  useCanAny,
  useDatabases,
  useInstances,
  useServers,
} from "@/lib/hooks";
import type { Database } from "@/lib/types";

export default function DatabasesPage() {
  return (
    <Suspense fallback={<TableSkeleton />}>
      <Databases />
    </Suspense>
  );
}

function Databases() {
  const router = useRouter();
  const pathname = usePathname();
  const sp = useSearchParams();
  const view = sp.get("view") === "all" ? "all" : "grouped";
  const [search, setSearch] = useState("");
  const canAny = useCanAny();
  const canWrite = canAny("database:write");
  const canBackup = canAny("backup:write");
  const canAddServer = canAny("instance:write");
  // Someone granted access to single databases can't list database servers:
  // they get the flat list instead.
  const canInstances = canAny("instance:read");

  const {
    data: instances,
    isLoading: instancesLoading,
    error: instancesError,
  } = useInstances(undefined, undefined, undefined, canInstances);
  const { data: servers } = useServers(undefined, undefined, canAny("server:read"));
  const serverName = useMemo(
    () => new Map(servers?.items.map((s) => [s.id, s.name]) ?? []),
    [servers],
  );

  // ?connect=1 (from the setup checklist) opens the wizard straight away.
  const [wizardOpen, setWizardOpen] = useState(sp.get("connect") === "1");
  const [createFor, setCreateFor] = useState<string | null | undefined>(
    undefined,
  );
  const [backupTarget, setBackupTarget] = useState<Database | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Database | null>(null);

  const actions: DatabaseActions = {
    canWrite,
    canBackup,
    onBackup: setBackupTarget,
    onDelete: setDeleteTarget,
  };
  const instanceList = instances?.items ?? [];
  const writable = instanceList.filter((i) => i.has_credentials);

  function closeWizard() {
    setWizardOpen(false);
    if (sp.has("connect")) {
      const q = new URLSearchParams(sp.toString());
      q.delete("connect");
      router.replace(q.toString() ? `${pathname}?${q}` : pathname);
    }
  }

  function setView(v: "grouped" | "all") {
    const q = new URLSearchParams(sp.toString());
    if (v === "all") q.set("view", "all");
    else q.delete("view");
    router.replace(q.toString() ? `${pathname}?${q}` : pathname);
  }

  const header = (
    <PageHeader
      title="Databases"
      description="Your database servers and the databases on them. New databases are found automatically."
      actions={
        <>
          {canAddServer ? (
            <button className="btn" onClick={() => setWizardOpen(true)}>
              <Plus size={16} /> Connect database server
            </button>
          ) : null}
          {canWrite && writable.length > 0 ? (
            <button
              className="btn btn-primary"
              onClick={() => setCreateFor(null)}
            >
              <Plus size={16} /> Create database
            </button>
          ) : null}
        </>
      }
    />
  );

  const modals = (
    <>
      <AddServerWizard open={wizardOpen} onClose={closeWizard} />
      <CreateDatabaseModal
        open={createFor !== undefined}
        onClose={() => setCreateFor(undefined)}
        instances={writable}
        instanceId={createFor ?? undefined}
      />
      <BackupDatabaseModal
        database={backupTarget}
        onClose={() => setBackupTarget(null)}
      />
      <DeleteDatabaseModal
        database={deleteTarget}
        onClose={() => setDeleteTarget(null)}
      />
    </>
  );

  const searchBox = (placeholder: string) => (
    <div className="search-box">
      <Search size={16} aria-hidden />
      <input
        className="input"
        type="search"
        placeholder={placeholder}
        aria-label="Search databases"
        value={search}
        onChange={(e) => setSearch(e.target.value)}
      />
    </div>
  );

  let content: React.ReactNode;
  if (!canInstances) {
    content = (
      <>
        <div style={{ marginBottom: ".9rem" }}>{searchBox("Search databases…")}</div>
        <AllDatabases search={search} actions={actions} />
      </>
    );
  } else if (instancesLoading) {
    content = <TableSkeleton />;
  } else if (instancesError) {
    content = <EmptyState title="Could not load your database servers" hint={friendlyError(instancesError)} />;
  } else if (instanceList.length === 0) {
    const noServers = (servers?.items.length ?? 0) === 0;
    content = (
      <EmptyState
        icon={<DatabaseIcon size={22} />}
        title="No database servers yet"
        hint={
          noServers
            ? "Connect a database server you already run anywhere, or connect one of your machines first so Fleetdock can create one there for you."
            : "Connect an existing database server, or let Fleetdock create a new one on one of your servers."
        }
        action={
          canAddServer ? (
            <>
              <button className="btn btn-primary" onClick={() => setWizardOpen(true)}>
                <Plus size={16} /> Connect database server
              </button>
              {noServers ? (
                <Link href="/servers" className="btn">
                  Connect a machine
                </Link>
              ) : null}
            </>
          ) : undefined
        }
      />
    );
  } else {
    content = (
      <>
        <div className="flex items-center gap-2" style={{ marginBottom: ".9rem", flexWrap: "wrap" }}>
          {searchBox(view === "all" ? "Search databases…" : "Search servers and databases…")}
          <div className="segmented" role="group" aria-label="View">
            <button type="button" aria-pressed={view === "grouped"} onClick={() => setView("grouped")}>
              By server
            </button>
            <button type="button" aria-pressed={view === "all"} onClick={() => setView("all")}>
              All databases
            </button>
          </div>
        </div>
        {view === "grouped" ? (
          <div className="flex flex-col gap-3">
            {instanceList.map((inst) => (
              <InstanceGroup
                key={inst.id}
                instance={inst}
                serverName={inst.server_id ? serverName.get(inst.server_id) : undefined}
                search={search}
                defaultOpen={instanceList.length <= 4}
                actions={actions}
                onCreateDatabase={(id) => setCreateFor(id)}
                canManageInstance
              />
            ))}
          </div>
        ) : (
          <AllDatabases search={search} actions={actions} />
        )}
      </>
    );
  }

  // The dialogs stay at one place in the tree: switching from the empty state
  // to the list (e.g. right after the first server is connected) must not
  // remount them and lose the wizard's "done" step.
  return (
    <div>
      {header}
      {content}
      {modals}
    </div>
  );
}

function AllDatabases({
  search,
  actions,
}: {
  search: string;
  actions: DatabaseActions;
}) {
  const [page, setPage] = useState(1);
  const { data, isLoading, error } = useDatabases({ search, page });
  const columns: DataTableColumn<Database>[] = [
    {
      id: "name",
      header: "Database",
      className: "font-medium",
      render: (d) => (
        <span className="flex items-center gap-2">
          <Link href={`/databases/${d.id}`} className="link-plain">
            {d.name}
          </Link>
          {d.system ? <span className="badge badge-gray">system</span> : null}
        </span>
      ),
    },
    {
      id: "instance",
      header: "Server",
      className: "muted",
      render: (d) =>
        d.instance ? (
          <Link href={`/instances/${d.instance.id}`} className="link-plain">
            {d.instance.name}
          </Link>
        ) : (
          "—"
        ),
    },
    {
      id: "status",
      header: "Status",
      render: (d) => <StatusBadge status={d.status} />,
    },
    {
      id: "size",
      header: "Size",
      align: "right",
      className: "muted",
      hideOnMobile: true,
      render: (d) => formatBytes(d.size_bytes),
    },
    {
      id: "actions",
      header: "",
      align: "right",
      render: (d) => <DatabaseMenu database={d} actions={actions} />,
    },
  ];
  return (
    <DataTable<Database>
      columns={columns}
      rows={data?.items ?? []}
      rowKey={(d) => d.id}
      isLoading={isLoading}
      error={error ? friendlyError(error) : undefined}
      errorTitle="Could not load databases"
      emptyTitle="No databases yet"
      emptyHint="Databases on your connected servers appear here automatically."
      emptySearchTitle="No databases match your search"
      pagination={{
        page,
        pageCount: Math.max(
          1,
          Math.ceil((data?.pagination.total ?? 0) / LIST_PAGE_SIZE),
        ),
        onPage: setPage,
      }}
    />
  );
}
