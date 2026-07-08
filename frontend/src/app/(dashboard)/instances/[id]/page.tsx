"use client";

import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { ChevronDown, ChevronRight, Play, Plus, RotateCw, Square, Trash2 } from "lucide-react";
import { DataTable } from "@/components/data-table";
import { EmptyState, ErrorText, Field, Modal, Pagination, Spinner, StatusBadge } from "@/components/ui";
import { ApiError } from "@/lib/api";
import {
  useCan,
  useClientPage,
  useCreateDBUser,
  useDatabases,
  useDBPrivileges,
  useDBUsers,
  useDeleteInstance,
  useDropDBUser,
  useGrantOnInstance,
  useInstance,
  useInstanceLifecycle,
  useUserGrants,
} from "@/lib/hooks";
import type { DBUser, Instance } from "@/lib/types";

export default function InstanceDetailPage() {
  const params = useParams();
  const id = String(params.id);
  const { data: instance, isLoading } = useInstance(id);
  const { data: databases } = useDatabases({ instance_id: id });
  const can = useCan();

  if (isLoading) {
    return <div className="flex items-center gap-2 muted text-sm"><Spinner /> Loading…</div>;
  }
  if (!instance) {
    return <EmptyState title="Instance not found" />;
  }

  return (
    <div>
      <Link href="/instances" className="muted text-sm">← Instances</Link>
      <div className="flex items-center justify-between" style={{ margin: ".6rem 0 1.1rem" }}>
        <div className="flex items-center gap-3">
          <h1 className="text-xl font-semibold">{instance.name}</h1>
          <span className={`badge ${instance.kind === "external" ? "badge-amber" : "badge-gray"}`}>
            {instance.kind}
          </span>
          {instance.provisioned ? <span className="badge badge-gray">docker</span> : null}
          <StatusBadge status={instance.status} />
        </div>
        {can("instance:write") ? <InstanceControls instance={instance} /> : null}
      </div>

      <div className="card" style={{ padding: "1.1rem", marginBottom: "1.25rem" }}>
        <dl className="grid" style={{ gridTemplateColumns: "repeat(auto-fit, minmax(160px, 1fr))", gap: "1rem" }}>
          <Detail label="Engine" value={`${instance.engine} ${instance.engine_version}`} />
          <Detail label="Location" value={instance.kind === "external" ? `${instance.host}:${instance.port}` : `port ${instance.port}`} />
          <Detail label="Admin user" value={instance.username ?? "—"} />
          <Detail label="Credentials" value={instance.has_credentials ? "configured" : "not configured"} />
          <Detail label="Registered" value={new Date(instance.created_at).toLocaleString()} />
        </dl>
      </div>

      <h2 className="font-semibold" style={{ marginBottom: ".6rem" }}>Databases</h2>
      <div style={{ marginBottom: "1.25rem" }}>
        <DataTable
          columns={[
            { id: "name", header: "Name", className: "font-medium", render: (d) => d.name },
            { id: "charset", header: "Charset", className: "muted", render: (d) => d.charset },
            { id: "status", header: "Status", render: (d) => <StatusBadge status={d.status} /> },
            {
              id: "actions",
              header: "",
              align: "right",
              render: (d) => (
                <Link href={`/databases/${d.id}`} className="btn btn-ghost btn-sm">
                  Open <ChevronRight size={15} />
                </Link>
              ),
            },
          ]}
          rows={databases?.items ?? []}
          rowKey={(d) => d.id}
          emptyTitle="No databases on this instance"
        />
      </div>

      {instance.has_credentials ? (
        <DBUsersSection instanceId={id} canWrite={can("instance:write")} databases={databases?.items ?? []} />
      ) : (
        <EmptyState
          title="Database users unavailable"
          hint="Add admin credentials to this instance to manage its database users and grants."
        />
      )}
    </div>
  );
}

function Detail({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="muted text-sm">{label}</dt>
      <dd className="font-medium" style={{ margin: 0 }}>{value}</dd>
    </div>
  );
}

function InstanceControls({ instance }: { instance: Instance }) {
  const router = useRouter();
  const lifecycle = useInstanceLifecycle();
  const del = useDeleteInstance();
  const busy = lifecycle.isPending || del.isPending;

  async function onDelete() {
    if (instance.provisioned) {
      if (!confirm(`Remove instance "${instance.name}"? Its Docker container will be stopped and removed.`)) return;
      const removeVolume = confirm("Also delete the data volume? This permanently destroys the database data. Cancel to keep it.");
      await del.mutateAsync({ id: instance.id, removeVolume });
    } else {
      if (!confirm(`Remove instance "${instance.name}" from the control plane? The database server is not touched.`)) return;
      await del.mutateAsync({ id: instance.id });
    }
    router.push("/instances");
  }

  return (
    <div className="flex items-center gap-2">
      {instance.provisioned ? (
        <>
          {instance.status === "stopped" ? (
            <button className="btn btn-sm" disabled={busy} onClick={() => lifecycle.mutate({ id: instance.id, action: "start" })}>
              <Play size={15} /> Start
            </button>
          ) : (
            <button className="btn btn-sm" disabled={busy} onClick={() => lifecycle.mutate({ id: instance.id, action: "stop" })}>
              <Square size={15} /> Stop
            </button>
          )}
          <button className="btn btn-sm" disabled={busy} onClick={() => lifecycle.mutate({ id: instance.id, action: "restart" })}>
            <RotateCw size={15} /> Restart
          </button>
        </>
      ) : null}
      <button className="btn btn-sm btn-danger" disabled={busy} onClick={onDelete} aria-label="Delete instance">
        <Trash2 size={15} /> Delete
      </button>
    </div>
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
  const [notice, setNotice] = useState<string | null>(null);

  async function onDrop(u: DBUser) {
    if (!confirm(`Drop database user '${u.user}'@'${u.host}'?`)) return;
    setNotice(null);
    try {
      await drop.mutateAsync({ instanceId, username: u.user, host: u.host });
    } catch (err) {
      setNotice(err instanceof ApiError ? err.message : "Failed to drop user");
    }
  }

  return (
    <div>
      <div className="flex items-center justify-between" style={{ marginBottom: ".6rem" }}>
        <div>
          <h2 className="font-semibold">Database users</h2>
          <p className="muted text-sm">MariaDB accounts on this instance (live).</p>
        </div>
        {canWrite ? (
          <button className="btn btn-primary" onClick={() => setCreateOpen(true)}>
            <Plus size={16} /> Create DB user
          </button>
        ) : null}
      </div>

      {notice ? (
        <div className="card" style={{ padding: ".7rem .9rem", marginBottom: ".9rem" }}>
          <span className="text-sm">{notice}</span>
        </div>
      ) : null}

      {isLoading ? (
        <div className="flex items-center gap-2 muted text-sm"><Spinner /> Connecting to instance…</div>
      ) : error ? (
        <EmptyState title="Could not reach the instance" hint={(error as ApiError).message} />
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
                    onDrop={() => onDrop(u)}
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
}: {
  instanceId: string;
  user: DBUser;
  open: boolean;
  onToggle: () => void;
  canWrite: boolean;
  onGrant: () => void;
  onDrop: () => void;
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
          <Field label="Host">
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
