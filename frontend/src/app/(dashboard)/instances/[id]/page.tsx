"use client";

import { useErrorToast, useToast } from "@/components/toast";

import Link from "next/link";
import { useParams, usePathname, useRouter, useSearchParams } from "next/navigation";
import { Suspense, useEffect, useState, type FormEvent, type ReactNode } from "react";

import { ChevronDown, ChevronRight, KeyRound, Play, Plus, RefreshCw, RotateCw, Square, Trash2 } from "lucide-react";
import { DataTable } from "@/components/data-table";
import { DeleteInstanceModal } from "@/components/delete-instance-modal";
import { DataAccessFields } from "@/components/database/data-access-fields";
import { SSHTunnelFields } from "@/components/database/ssh-tunnel-fields";
import { HealthBadge, InstanceMonitoring } from "@/components/instance-monitoring";
import {
  ConfirmModal,
  EmptyState,
  Menu,
  ErrorText,
  Field,
  Modal,
  PageHeader,
  Pagination,
  QueryTabs,
  Spinner,
  StatusBadge,
  TableSkeleton,
  Time,
} from "@/components/ui";
import { engineLabel } from "@/lib/engines";
import { fieldError, friendlyError } from "@/lib/errors";
import { ApiError } from "@/lib/api";
import { formatBytes } from "@/lib/format";
import { DATA_ACCESS_LABELS, dataAccessChanged, dataAccessDraft, dataAccessInput } from "@/lib/data-access";
import { sshChanged, sshDraft, sshInput } from "@/lib/ssh-tunnel";
import {
  LIST_PAGE_SIZE,
  useCan,
  useClientPage,
  useCreateDBUser,
  useDatabases,
  useDBPrivileges,
  useDBUsers,
  useDropDBUser,
  useGrantOnInstance,
  useInstance,
  useInstanceLifecycle,
  useProbeInstance,
  useServers,
  useSetDBUserPassword,
  useUpdateInstance,
  useUserGrants,
} from "@/lib/hooks";
import type { DBUser, Instance, TLSMode, UpdateInstanceInput } from "@/lib/types";

export default function InstanceDetailPage() {
  return (
    <Suspense fallback={<TableSkeleton />}>
      <InstanceDetail />
    </Suspense>
  );
}

/** kindLabel describes, in plain words, how Fleetdock reaches a database server. */
function kindLabel(i: Instance): string {
  if (i.provisioned) return "Created by Fleetdock";
  return i.kind === "external" ? "Connected remotely" : "On your server";
}

function InstanceDetail() {
  const params = useParams();
  const id = String(params.id);
  const router = useRouter();
  const pathname = usePathname();
  const sp = useSearchParams();
  const tab = sp.get("tab") ?? "databases";
  const { data: instance, isLoading, error } = useInstance(id);
  const { data: servers } = useServers();
  // Paginated: an instance with more databases than one page used to render a
  // silently truncated list with no indication there was more.
  const [dbPage, setDbPage] = useState(1);
  const { data: databases } = useDatabases({ instance_id: id, page: dbPage });
  const can = useCan();

  if (isLoading) return <TableSkeleton />;
  if (!instance) {
    return (
      <EmptyState
        title="Database server not found"
        hint={error ? friendlyError(error) : "It may have been removed."}
        action={
          <Link href="/databases" className="btn">
            Back to databases
          </Link>
        }
      />
    );
  }

  const serverName = servers?.items.find((s) => s.id === instance.server_id)?.name;
  const where =
    instance.kind === "external"
      ? `${instance.host}:${instance.port}${instance.ssh_tunnel ? ` via ${instance.ssh_tunnel.host}` : ""}`
      : `${serverName ?? "server"}, port ${instance.port}`;

  function selectTab(t: string) {
    const q = new URLSearchParams(sp.toString());
    if (t === "databases") q.delete("tab");
    else q.set("tab", t);
    router.replace(q.toString() ? `${pathname}?${q}` : pathname);
  }

  return (
    <div>
      <PageHeader
        breadcrumbs={[{ label: "Databases", href: "/databases" }, { label: instance.name }]}
        title={instance.name}
        badges={
          <>
            <StatusBadge status={instance.status} />
            {instance.has_credentials ? <HealthBadge health={instance.health} /> : null}
          </>
        }
        description={`${engineLabel(instance.engine)} ${instance.engine_version} · ${kindLabel(instance)} · ${where}`}
        actions={can("instance:write") ? <InstanceControls instance={instance} /> : null}
      />

      {!instance.has_credentials ? (
        <div className="callout callout-warning" role="status">
          <strong>Fleetdock can&apos;t log in to this server yet.</strong> Add an admin user and password (Edit) so it
          can find databases, manage users, take backups and show monitoring.
        </div>
      ) : null}

      <div className="card" style={{ padding: "1.1rem", marginBottom: "1.25rem" }}>
        <dl className="grid" style={{ gridTemplateColumns: "repeat(auto-fit, minmax(160px, 1fr))", gap: "1rem" }}>
          <Detail label="Engine" value={`${engineLabel(instance.engine)} ${instance.engine_version}`} />
          <Detail label="Address" value={where} />
          <Detail label="Admin user" value={instance.username ?? "—"} />
          <Detail label="Encryption" value={TLS_LABEL[instance.tls_mode ?? "prefer"]} />
          <Detail
            label="Data browsing as"
            value={
              instance.data_access === "login" && instance.data_username
                ? instance.data_username
                : DATA_ACCESS_LABELS[instance.data_access ?? "admin"]
            }
          />
          {instance.ssh_tunnel ? (
            <Detail label="SSH tunnel" value={<SSHTunnelDetail instance={instance} canWrite={can("instance:write")} />} />
          ) : null}
          <Detail label="Added" value={<Time value={instance.created_at} />} />
          {instance.has_credentials ? <Detail label="Last checked" value={<LastChecked instance={instance} />} /> : null}
        </dl>
        {instance.health?.error && instance.health.status !== "healthy" ? (
          <p className="field-error" style={{ margin: ".8rem 0 0" }}>
            Last check failed: {instance.health.error}
          </p>
        ) : null}
      </div>

      <QueryTabs
        label="Database server sections"
        onSelect={selectTab}
        tabs={[
          { id: "databases", label: `Databases${databases ? ` (${databases.pagination.total})` : ""}` },
          { id: "monitoring", label: "Monitoring", hidden: !instance.has_credentials },
          { id: "users", label: "Database users", hidden: !instance.has_credentials },
        ]}
      />

      {tab === "monitoring" && instance.has_credentials ? (
        <InstanceMonitoring instanceId={id} canKill={can("instance:write")} />
      ) : tab === "users" && instance.has_credentials ? (
        <DBUsersSection instanceId={id} canWrite={can("instance:write")} databases={databases?.items ?? []} />
      ) : (
        <DataTable
          columns={[
            {
              id: "name",
              header: "Name",
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
            { id: "status", header: "Status", render: (d) => <StatusBadge status={d.status} /> },
            { id: "size", header: "Size", align: "right", className: "muted", render: (d) => formatBytes(d.size_bytes) },
            { id: "conns", header: "Connections", align: "right", className: "muted", hideOnMobile: true, render: (d) => d.active_connections ?? 0 },
            { id: "charset", header: "Charset", className: "muted", hideOnMobile: true, render: (d) => d.charset },
          ]}
          rows={databases?.items ?? []}
          rowKey={(d) => d.id}
          emptyTitle="No databases found yet"
          emptyHint={
            instance.has_credentials
              ? "Databases on this server appear here automatically within a minute."
              : "Add an admin login so Fleetdock can see this server's databases."
          }
          pagination={{
            page: dbPage,
            pageCount: Math.max(1, Math.ceil((databases?.pagination.total ?? 0) / LIST_PAGE_SIZE)),
            onPage: setDbPage,
          }}
        />
      )}
    </div>
  );
}

const TLS_LABEL: Record<string, string> = {
  disable: "Off",
  prefer: "When available",
  require: "Always",
  "verify-full": "Always, verified",
};

function EditInstanceModal({
  instance,
  onClose,
}: {
  instance: Instance | null;
  onClose: () => void;
}) {
  const update = useUpdateInstance();
  const [name, setName] = useState("");
  const [host, setHost] = useState("");
  const [port, setPort] = useState("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [clearPassword, setClearPassword] = useState(false);
  const [tlsMode, setTlsMode] = useState<TLSMode>("prefer");
  const [ssh, setSsh] = useState(sshDraft());
  const [dataAccess, setDataAccess] = useState(dataAccessDraft());
  const [error, setError] = useState<unknown>(null);

  // Re-seed the form whenever a different instance is opened.
  useEffect(() => {
    if (!instance) return;
    setName(instance.name);
    setHost(instance.host ?? "");
    setPort(String(instance.port));
    setUsername(instance.username ?? "");
    setPassword("");
    setClearPassword(false);
    setTlsMode(instance.tls_mode ?? "prefer");
    setSsh(sshDraft(instance.ssh_tunnel));
    setDataAccess(dataAccessDraft(instance));
    setError(null);
  }, [instance]);

  if (!instance) return null;

  const portLocked = instance.provisioned;
  // The API refuses to send the stored password to a new host; the user has to
  // re-enter it (or drop it).
  const hostChanged = instance.kind === "external" && host.trim() !== (instance.host ?? "");
  // Adding, moving or removing the SSH tunnel re-routes the connection too.
  const saved = instance.ssh_tunnel;
  const tunnelRerouted =
    ssh.enabled !== !!saved ||
    (ssh.enabled && !!saved && (ssh.host.trim() !== saved.host || (Number(ssh.port) || 22) !== saved.port));
  const needsPassword = (hostChanged || tunnelRerouted) && instance.has_credentials && !clearPassword && username !== "";
  // The stored data login password stays usable only for the same username and route.
  const keepsDataPassword =
    instance.data_access === "login" &&
    dataAccess.username.trim() === (instance.data_username ?? "") &&
    !hostChanged &&
    !tunnelRerouted;

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (!instance) return;
    setError(null);

    // Send only what actually changed, so an untouched password field never
    // rotates the stored secret.
    const input: UpdateInstanceInput & { id: string } = { id: instance.id };
    if (name !== instance.name) input.name = name;
    if (instance.kind === "external" && host !== (instance.host ?? "")) input.host = host;
    if (!portLocked && Number(port) !== instance.port) input.port = Number(port);
    if (username !== (instance.username ?? "")) input.username = username;
    if (tlsMode !== (instance.tls_mode ?? "prefer")) input.tls_mode = tlsMode;
    if (clearPassword) {
      input.password = "";
    } else if (password) {
      input.password = password;
    }
    if (instance.kind === "external" && sshChanged(ssh, instance.ssh_tunnel)) {
      if (ssh.enabled) input.ssh_tunnel = sshInput(ssh);
      else input.remove_ssh_tunnel = true;
    }
    if (dataAccessChanged(dataAccess, instance) || (!keepsDataPassword && dataAccess.mode === "login")) {
      Object.assign(input, dataAccessInput(dataAccess));
    }

    try {
      await update.mutateAsync(input);
      onClose();
    } catch (err) {
      setError(err);
    }
  }

  return (
    <Modal open onClose={onClose} title={`Edit "${instance.name}"`}>
      <form onSubmit={onSubmit}>
        <div className="grid" style={{ gridTemplateColumns: "1fr 1fr", gap: ".75rem" }}>
          <Field label="Name">
            <input className="input" value={name} onChange={(e) => setName(e.target.value)} required />
          </Field>
          <Field label="Port" hint={portLocked ? "Fixed: Fleetdock created this server in a container on this port." : undefined}>
            <input
              className="input"
              type="number"
              value={port}
              onChange={(e) => setPort(e.target.value)}
              disabled={portLocked}
              required
            />
          </Field>
        </div>
        <Field label="Encryption" help="How Fleetdock protects its connection to this server. Use “verify” across networks you don’t trust.">
          <select className="input" value={tlsMode} onChange={(e) => setTlsMode(e.target.value as TLSMode)}>
            <option value="prefer">Use encryption when available (default)</option>
            <option value="require">Always encrypt</option>
            <option value="verify-full">Always encrypt and verify the certificate</option>
            <option value="disable">Never encrypt</option>
          </select>
        </Field>
        {instance.kind === "external" ? (
          <>
            <Field label="Host" hint={ssh.enabled ? "Hostname or IP address as seen from the SSH server." : "Hostname or IP address Fleetdock can reach."}
                help={<>On the same computer as Fleetdock? Use <code>host.docker.internal</code> — <code>localhost</code> would mean Fleetdock&apos;s own container.</>}
                error={fieldError(error, "host")}>
              <input
                className="input"
                value={host}
                onChange={(e) => setHost(e.target.value)}
                placeholder={ssh.enabled ? "127.0.0.1" : "db.example.com or 10.0.0.5"}
                required
              />
            </Field>
            <SSHTunnelFields value={ssh} onChange={setSsh} error={error} saved={instance.ssh_tunnel} />
          </>
        ) : null}

        <h3 className="font-semibold text-sm" style={{ margin: "1rem 0 .3rem" }}>
          Admin credentials
        </h3>
        <p className="muted text-sm" style={{ marginTop: 0 }}>
          Used for provisioning, discovery, backups and live administration. Stored
          encrypted; the current password is never displayed.
        </p>
        <div className="grid" style={{ gridTemplateColumns: "1fr 1fr", gap: ".75rem" }}>
          <Field label="Admin username">
            <input
              className="input"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              placeholder={instance.engine === "postgres" ? "postgres" : "root"}
              autoComplete="off"
            />
          </Field>
          <Field
            label={needsPassword ? `Password (required for the new ${hostChanged ? "host" : "route"})` : instance.has_credentials ? "New password" : "Password"}
            error={fieldError(error, "password")}
          >
            <input
              className="input"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder={needsPassword ? "re-enter the admin password" : instance.has_credentials ? "leave blank to keep current" : ""}
              disabled={clearPassword}
              required={needsPassword}
              autoComplete="new-password"
            />
          </Field>
        </div>
        {instance.has_credentials ? (
          <label className="flex items-center gap-2 text-sm" style={{ cursor: "pointer" }}>
            <input
              type="checkbox"
              checked={clearPassword}
              onChange={(e) => setClearPassword(e.target.checked)}
            />
            Remove the stored password (makes this instance metadata-only)
          </label>
        ) : null}

        <div style={{ marginTop: "1rem" }} />
        <DataAccessFields value={dataAccess} onChange={setDataAccess} error={error} hasStoredLogin={keepsDataPassword} />

        <ErrorText message={error ? (error instanceof ApiError ? error.message : "Failed to save the changes") : undefined} />
        <div className="flex items-center justify-end gap-2" style={{ marginTop: ".8rem" }}>
          <button type="button" className="btn" onClick={onClose}>
            Cancel
          </button>
          <button type="submit" className="btn btn-primary" disabled={update.isPending}>
            {update.isPending ? "Saving…" : "Save changes"}
          </button>
        </div>
      </form>
    </Modal>
  );
}

/**
 * SSHTunnelDetail shows the bastion and its pinned host key. Resetting the pin
 * is how an expected host key change (a rebuilt bastion) is accepted.
 */
function SSHTunnelDetail({ instance, canWrite }: { instance: Instance; canWrite: boolean }) {
  const t = instance.ssh_tunnel!;
  const update = useUpdateInstance();
  const toastError = useErrorToast();
  const [confirm, setConfirm] = useState(false);

  async function reset() {
    try {
      await update.mutateAsync({ id: instance.id, reset_ssh_host_key: true });
      setConfirm(false);
    } catch (err) {
      toastError("Couldn't reset the host key")(err);
    }
  }

  return (
    <>
      <span>
        {t.username}@{t.host}
        {t.port !== 22 ? `:${t.port}` : ""}
      </span>
      <span className="muted text-sm" style={{ display: "block", wordBreak: "break-all" }}>
        {t.host_key_fingerprint ? (
          <>
            Host key <code>{t.host_key_fingerprint}</code>
            {canWrite ? (
              <button type="button" className="btn btn-ghost btn-sm" style={{ marginLeft: ".25rem" }} onClick={() => setConfirm(true)}>
                Reset
              </button>
            ) : null}
          </>
        ) : (
          "Host key is pinned on the first connection."
        )}
      </span>
      <ConfirmModal
        open={confirm}
        title="Reset the SSH host key?"
        confirmLabel="Reset host key"
        busy={update.isPending}
        message="Fleetdock forgets the pinned key and trusts whatever key the SSH server presents next time. Only do this if you know the server's key changed (for example, it was rebuilt)."
        onConfirm={reset}
        onCancel={() => setConfirm(false)}
      />
    </>
  );
}

function Detail({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div>
      <dt className="muted text-sm">{label}</dt>
      <dd className="font-medium" style={{ margin: 0 }}>{value}</dd>
    </div>
  );
}

/**
 * LastChecked shows when Fleetdock last reached the server (it checks every
 * minute on its own) with a button to check — and pick up new databases — now.
 */
function LastChecked({ instance }: { instance: Instance }) {
  const probe = useProbeInstance();
  const toastError = useErrorToast();
  return (
    <span className="flex items-center gap-2">
      {instance.health ? <Time value={instance.health.checked_at} /> : "not yet"}
      <button
        type="button"
        className="btn btn-ghost btn-sm"
        disabled={probe.isPending}
        onClick={() => probe.mutate(instance.id, { onError: toastError("Could not check the server") })}
        title="Check the connection and look for new databases now"
      >
        {probe.isPending ? <Spinner /> : <RefreshCw size={14} />} Check now
      </button>
    </span>
  );
}

function InstanceControls({ instance }: { instance: Instance }) {
  const router = useRouter();
  const lifecycle = useInstanceLifecycle();
  const toastError = useErrorToast();
  const [editOpen, setEditOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [confirm, setConfirm] = useState<"stop" | "restart" | null>(null);
  const busy = lifecycle.isPending;

  const run = (action: "start" | "stop" | "restart") =>
    lifecycle.mutate(
      { id: instance.id, action },
      { onError: toastError(`Failed to ${action} ${instance.name}`), onSettled: () => setConfirm(null) },
    );

  return (
    <>
      {instance.provisioned && instance.status === "stopped" ? (
        <button className="btn" disabled={busy} onClick={() => run("start")}>
          <Play size={15} /> Start
        </button>
      ) : null}
      <button className="btn" disabled={busy} onClick={() => setEditOpen(true)}>
        <KeyRound size={15} /> {instance.has_credentials ? "Edit" : "Add login"}
      </button>
      <Menu
        label={`More actions for ${instance.name}`}
        items={[
          {
            label: "Restart",
            icon: <RotateCw size={15} />,
            onSelect: () => setConfirm("restart"),
            hidden: !instance.provisioned || instance.status === "stopped",
          },
          {
            label: "Stop",
            icon: <Square size={15} />,
            onSelect: () => setConfirm("stop"),
            hidden: !instance.provisioned || instance.status === "stopped",
          },
          { label: "Remove…", icon: <Trash2 size={15} />, danger: true, onSelect: () => setDeleteOpen(true) },
        ]}
      />
      <ConfirmModal
        open={confirm !== null}
        danger={confirm === "stop"}
        title={confirm === "stop" ? `Stop ${instance.name}?` : `Restart ${instance.name}?`}
        confirmLabel={confirm === "stop" ? "Stop server" : "Restart server"}
        busy={busy}
        message={
          confirm === "stop"
            ? "Every application connected to its databases loses its connection until you start it again. No data is deleted."
            : "Connected applications are disconnected for a few seconds while it restarts."
        }
        onConfirm={() => confirm && run(confirm)}
        onCancel={() => setConfirm(null)}
      />
      <EditInstanceModal instance={editOpen ? instance : null} onClose={() => setEditOpen(false)} />
      <DeleteInstanceModal
        instance={deleteOpen ? instance : null}
        onClose={() => setDeleteOpen(false)}
        onDeleted={() => router.push("/databases")}
      />
    </>
  );
}

function DBUsersSection({
  instanceId,
  canWrite,
  databases,
}: {
  instanceId: string;
  canWrite: boolean;
  databases: { id: string; name: string }[];
}) {
  const { data, isLoading, error } = useDBUsers(instanceId);
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

  return (
    <div>
      <div className="flex items-center justify-between" style={{ marginBottom: ".6rem" }}>
        <div>
          <h2 className="font-semibold">Database users</h2>
          <p className="muted text-sm">Accounts on this instance (live).</p>
        </div>
        {canWrite ? (
          <button className="btn btn-primary" onClick={() => setCreateOpen(true)}>
            <Plus size={16} /> Create DB user
          </button>
        ) : null}
      </div>


      {isLoading ? (
        <div className="flex items-center gap-2 muted text-sm"><Spinner /> Connecting to instance…</div>
      ) : error ? (
        <EmptyState title="Could not reach the database server" hint={(error as ApiError).message} />
      ) : !data || data.items.length === 0 ? (
        <EmptyState title="No database users" />
      ) : (
        <div className="card" style={{ overflow: "hidden" }}>
          <table className="table">
            <thead>
              <tr>
                <th style={{ width: 30 }} />
                <th>User</th>
                <th>Host</th>
                {canWrite ? <th style={{ textAlign: "right" }}>Actions</th> : null}
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
          </table>
        </div>
      )}

      <div className="flex items-center justify-end" style={{ marginTop: ".6rem" }}>
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
          <p style={{ marginTop: 0 }}>
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
      <GrantModal
        target={grantTarget}
        onClose={() => setGrantTarget(null)}
        instanceId={instanceId}
        databases={databases}
      />
    </div>
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
  return (
    <>
      <tr style={{ cursor: "pointer" }} onClick={onToggle}>
        <td>{open ? <ChevronDown size={15} /> : <ChevronRight size={15} />}</td>
        <td className="font-medium">{user.user}</td>
        <td className="muted"><code>{user.host}</code></td>
        {canWrite ? (
          <td style={{ textAlign: "right" }} onClick={(e) => e.stopPropagation()}>
            <div className="flex items-center gap-2" style={{ justifyContent: "flex-end" }}>
              <button className="btn btn-sm" onClick={onGrant}>Grant…</button>
              <button className="btn btn-sm" onClick={onPassword} aria-label="Set password">
                <KeyRound size={15} />
              </button>
              <button className="btn btn-sm btn-danger" onClick={onDrop} aria-label="Drop user">
                <Trash2 size={15} />
              </button>
            </div>
          </td>
        ) : null}
      </tr>
      {open ? (
        <tr>
          <td colSpan={canWrite ? 4 : 3} style={{ background: "var(--panel-2, var(--panel))" }}>
            <GrantsList instanceId={instanceId} user={user} />
          </td>
        </tr>
      ) : null}
    </>
  );
}

function GrantsList({ instanceId, user }: { instanceId: string; user: DBUser }) {
  const { data, isLoading, error } = useUserGrants(instanceId, user.user, user.host);
  if (isLoading) return <span className="muted text-sm">Loading grants…</span>;
  if (error) return <span className="muted text-sm">{(error as ApiError).message}</span>;
  return (
    <div className="flex flex-col gap-1" style={{ padding: ".25rem 0" }}>
      {(data?.items ?? []).map((g, i) => (
        <code key={i} className="text-sm" style={{ fontSize: 12 }}>{g}</code>
      ))}
    </div>
  );
}

function CreateDBUserModal({
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
        <div className="grid" style={{ gridTemplateColumns: "1fr 1fr", gap: ".75rem" }}>
          <Field label="Username">
            <input className="input" value={username} onChange={(e) => setUsername(e.target.value)} placeholder="app_user" required />
          </Field>
          <Field label="Connect from" help="Where this user may log in from (MySQL/MariaDB). % means anywhere; localhost means only from the server itself.">
            <input className="input" value={host} onChange={(e) => setHost(e.target.value)} placeholder="%" />
          </Field>
        </div>
        <Field label="Password">
          <input className="input" type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="new-password" required />
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

function GrantModal({
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
            <option value="" disabled>Select a database…</option>
            {databases.map((d) => (
              <option key={d.id} value={d.name}>{d.name}</option>
            ))}
          </select>
        </Field>
        <Field label="Privileges">
          <div className="card" style={{ padding: ".75rem", maxHeight: 220, overflowY: "auto" }}>
            <div className="flex" style={{ flexWrap: "wrap", gap: ".3rem 1rem" }}>
              {(privileges?.items ?? []).map((p) => (
                <label key={p} className="flex items-center gap-1 text-sm" style={{ cursor: "pointer" }}>
                  <input type="checkbox" checked={selected.has(p)} onChange={() => toggle(p)} />
                  {p}
                </label>
              ))}
            </div>
          </div>
        </Field>
        <ErrorText message={error ?? undefined} />
        <div className="flex items-center justify-end gap-2" style={{ marginTop: ".5rem" }}>
          <button type="button" className="btn" onClick={onClose}>Cancel</button>
          <button type="submit" className="btn btn-primary" disabled={grant.isPending || selected.size === 0}>
            {grant.isPending ? "Granting…" : "Grant"}
          </button>
        </div>
      </form>
    </Modal>
  );
}

function SetPasswordModal({ instanceId, user, onClose }: { instanceId: string; user: DBUser; onClose: () => void }) {
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
        <Field label="New password">
          <input className="input" type="password" value={password} onChange={(e) => setPw(e.target.value)} autoComplete="new-password" required minLength={8} />
        </Field>
        <p className="text-sm muted">Existing sessions stay connected; new logins need the new password.</p>
        <ErrorText message={error ?? undefined} />
        <div className="flex gap-2" style={{ marginTop: ".8rem" }}>
          <button className="btn btn-primary" type="submit" disabled={setPassword.isPending}>
            {setPassword.isPending ? "Saving…" : "Set password"}
          </button>
          <button className="btn" type="button" onClick={onClose}>
            Cancel
          </button>
        </div>
      </form>
    </Modal>
  );
}
