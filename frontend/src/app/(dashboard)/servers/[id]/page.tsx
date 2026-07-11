"use client";

import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useEffect, useState, type FormEvent } from "react";

import { Pencil, Plus, Trash2 } from "lucide-react";
import { DataTable } from "@/components/data-table";
import { MetricChart, type ChartPoint } from "@/components/chart";
import { EmptyState, ErrorText, Field, Modal, Spinner, StatusBadge } from "@/components/ui";
import { ApiError } from "@/lib/api";
import {
  useCan,
  useCreateInstance,
  useDeleteServer,
  useInstances,
  useProvisionInstance,
  useServer,
  useServerMetrics,
  useUpdateServer,
} from "@/lib/hooks";
import type { Server } from "@/lib/types";

export default function ServerDetailPage() {
  const params = useParams();
  const id = String(params.id);
  const { data: server, isLoading } = useServer(id);
  const { data: instances } = useInstances(id);
  const [open, setOpen] = useState(false);
  const [renameOpen, setRenameOpen] = useState(false);
  const can = useCan();

  if (isLoading) {
    return <div className="flex items-center gap-2 muted text-sm"><Spinner /> Loading…</div>;
  }
  if (!server) {
    return <EmptyState title="Server not found" />;
  }

  const instanceCount = instances?.items.length ?? 0;

  return (
    <div>
      <Link href="/servers" className="muted text-sm">← Servers</Link>
      <div className="flex items-center justify-between" style={{ margin: ".6rem 0 1.1rem" }}>
        <div className="flex items-center gap-3">
          <h1 className="text-xl font-semibold">{server.name}</h1>
          <StatusBadge status={server.status} />
        </div>
        <div className="flex items-center gap-2">
          {can("server:write") ? (
            <ServerControls server={server} instanceCount={instanceCount} onRename={() => setRenameOpen(true)} />
          ) : null}
          {can("instance:write") ? (
            <button className="btn btn-primary" onClick={() => setOpen(true)}>
              <Plus size={16} /> Add instance
            </button>
          ) : null}
        </div>
      </div>

      <div className="card" style={{ padding: "1.1rem", marginBottom: "1.25rem" }}>
        <dl className="grid" style={{ gridTemplateColumns: "repeat(auto-fit, minmax(180px, 1fr))", gap: "1rem" }}>
          <Detail label="Hostname" value={server.hostname} />
          <Detail label="Address" value={server.address ?? "—"} />
          <Detail label="MariaDB" value={server.mariadb_version ?? "—"} />
          <Detail label="Agent" value={server.agent_version ?? "—"} />
          <Detail label="Tags" value={server.tags.length ? server.tags.join(", ") : "—"} />
          <Detail label="Registered" value={new Date(server.created_at).toLocaleString()} />
        </dl>
      </div>

      <ServerMetrics id={id} />

      <h2 className="font-semibold" style={{ marginBottom: ".6rem" }}>Instances</h2>
      <DataTable
        columns={[
          { id: "name", header: "Name", className: "font-medium", render: (i) => i.name },
          { id: "version", header: "Version", className: "muted", render: (i) => i.mariadb_version },
          { id: "port", header: "Port", className: "muted", render: (i) => i.port },
          { id: "status", header: "Status", render: (i) => <StatusBadge status={i.status} /> },
        ]}
        rows={instances?.items ?? []}
        rowKey={(i) => i.id}
        emptyTitle="No instances"
        emptyHint="Add a MariaDB instance running on this server."
      />

      <AddInstanceModal open={open} onClose={() => setOpen(false)} serverId={id} />
      <RenameServerModal open={renameOpen} onClose={() => setRenameOpen(false)} server={server} />
    </div>
  );
}

function ServerControls({
  server,
  instanceCount,
  onRename,
}: {
  server: Server;
  instanceCount: number;
  onRename: () => void;
}) {
  const router = useRouter();
  const del = useDeleteServer();
  const busy = del.isPending;

  async function onDelete() {
    if (instanceCount > 0) {
      alert(`Remove all ${instanceCount} instance(s) on this server before deleting it.`);
      return;
    }
    if (!confirm(`Delete server "${server.name}"? The agent on the host will no longer be managed.`)) return;
    try {
      await del.mutateAsync(server.id);
      router.push("/servers");
    } catch (err) {
      alert(err instanceof ApiError ? err.message : "Failed to delete server");
    }
  }

  return (
    <>
      <button className="btn btn-sm" onClick={onRename} aria-label="Rename server">
        <Pencil size={15} /> Rename
      </button>
      <button className="btn btn-sm btn-danger" disabled={busy} onClick={onDelete} aria-label="Delete server">
        <Trash2 size={15} /> Delete
      </button>
    </>
  );
}

function RenameServerModal({
  open,
  onClose,
  server,
}: {
  open: boolean;
  onClose: () => void;
  server: Server;
}) {
  const update = useUpdateServer();
  const [name, setName] = useState(server.name);
  const [tags, setTags] = useState(server.tags.join(", "));
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    setName(server.name);
    setTags(server.tags.join(", "));
    setError(null);
  }, [open, server]);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    const parsedTags = tags
      .split(",")
      .map((t) => t.trim())
      .filter(Boolean);
    try {
      await update.mutateAsync({
        id: server.id,
        name: name.trim(),
        tags: parsedTags,
      });
      onClose();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to update server");
    }
  }

  return (
    <Modal open={open} onClose={onClose} title="Rename server">
      <form onSubmit={onSubmit}>
        <Field label="Name">
          <input
            className="input"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="my-server"
            required
            pattern="[a-z0-9_-]{2,63}"
            title="Lowercase letters, digits, hyphens and underscores (2–63 characters)"
          />
        </Field>
        <Field label="Tags (comma-separated)">
          <input className="input" value={tags} onChange={(e) => setTags(e.target.value)} placeholder="prod, eu-west" />
        </Field>
        <ErrorText message={error ?? undefined} />
        <div className="flex items-center justify-end gap-2" style={{ marginTop: ".5rem" }}>
          <button type="button" className="btn" onClick={onClose}>Cancel</button>
          <button type="submit" className="btn btn-primary" disabled={update.isPending}>
            {update.isPending ? "Saving…" : "Save"}
          </button>
        </div>
      </form>
    </Modal>
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

const RANGES = [
  { label: "1h", hours: 1 },
  { label: "6h", hours: 6 },
  { label: "24h", hours: 24 },
  { label: "7d", hours: 168 },
];

function ServerMetrics({ id }: { id: string }) {
  const [hours, setHours] = useState(6);
  const { data, isLoading } = useServerMetrics(id, hours);
  const samples = data?.items ?? [];

  const cpu: ChartPoint[] = samples.map((s) => ({ t: s.collected_at, v: s.cpu_pct ?? null }));
  const mem: ChartPoint[] = samples.map((s) => ({ t: s.collected_at, v: pct(s.mem_used_bytes, s.mem_total_bytes) }));
  const disk: ChartPoint[] = samples.map((s) => ({ t: s.collected_at, v: pct(s.disk_used_bytes, s.disk_total_bytes) }));
  const conns: ChartPoint[] = samples.map((s) => ({ t: s.collected_at, v: s.active_connections ?? null }));

  return (
    <section style={{ marginBottom: "1.5rem" }}>
      <div className="flex items-center justify-between" style={{ marginBottom: ".6rem" }}>
        <h2 className="font-semibold">Metrics</h2>
        <div className="flex items-center gap-1">
          {RANGES.map((r) => (
            <button
              key={r.hours}
              className={`btn btn-sm${hours === r.hours ? " btn-primary" : ""}`}
              onClick={() => setHours(r.hours)}
            >
              {r.label}
            </button>
          ))}
        </div>
      </div>
      {isLoading && samples.length === 0 ? (
        <div className="flex items-center gap-2 muted text-sm"><Spinner /> Loading metrics…</div>
      ) : (
        <div className="grid" style={{ gridTemplateColumns: "repeat(auto-fit, minmax(240px, 1fr))", gap: ".9rem" }}>
          <MetricChart title="CPU" points={cpu} unit="%" max={100} color="var(--accent)" />
          <MetricChart title="Memory used" points={mem} unit="%" max={100} color="#22c55e" />
          <MetricChart title="Disk used" points={disk} unit="%" max={100} color="#f59e0b" />
          <MetricChart title="Connections" points={conns} color="#8b5cf6" />
        </div>
      )}
    </section>
  );
}

function pct(used?: number | null, total?: number | null): number | null {
  if (used == null || total == null || total === 0) return null;
  return (used / total) * 100;
}

const ENGINE_DEFAULTS: Record<string, { version: string; port: string }> = {
  mariadb: { version: "11.4", port: "3306" },
  mysql: { version: "8.4", port: "3306" },
  postgres: { version: "16", port: "5432" },
};

function AddInstanceModal({ open, onClose, serverId }: { open: boolean; onClose: () => void; serverId: string }) {
  const create = useCreateInstance();
  const provision = useProvisionInstance();
  const [mode, setMode] = useState<"provision" | "register">("provision");
  const [engine, setEngine] = useState("mariadb");
  const [name, setName] = useState("");
  const [version, setVersion] = useState("11.4");
  const [port, setPort] = useState("3306");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);

  function reset() {
    setName("");
    setUsername("");
    setPassword("");
    setError(null);
  }

  function onEngineChange(next: string) {
    setEngine(next);
    const d = ENGINE_DEFAULTS[next];
    if (d) {
      setVersion(d.version);
      setPort(d.port);
    }
  }

  const pending = create.isPending || provision.isPending;

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      if (mode === "provision") {
        await provision.mutateAsync({
          server_id: serverId,
          name,
          engine,
          engine_version: version,
          port: Number(port),
        });
      } else {
        await create.mutateAsync({
          kind: "managed",
          server_id: serverId,
          name,
          engine,
          engine_version: version,
          port: Number(port),
          username: username || undefined,
          password: password || undefined,
        });
      }
      reset();
      onClose();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to add instance");
    }
  }

  return (
    <Modal open={open} onClose={onClose} title="Add instance">
      <div className="flex gap-1" style={{ marginBottom: "1rem" }}>
        <button type="button" className={`btn btn-sm${mode === "provision" ? " btn-primary" : ""}`} onClick={() => setMode("provision")}>
          Provision new
        </button>
        <button type="button" className={`btn btn-sm${mode === "register" ? " btn-primary" : ""}`} onClick={() => setMode("register")}>
          Register existing
        </button>
      </div>
      <form onSubmit={onSubmit}>
        <div className="grid" style={{ gridTemplateColumns: "1fr 1fr", gap: ".75rem" }}>
          <Field label="Name">
            <input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="primary" required />
          </Field>
          <Field label="Engine">
            <select className="input" value={engine} onChange={(e) => onEngineChange(e.target.value)}>
              <option value="mariadb">MariaDB</option>
              <option value="mysql">MySQL</option>
              <option value="postgres">PostgreSQL</option>
            </select>
          </Field>
        </div>
        <div className="grid" style={{ gridTemplateColumns: "1fr 1fr", gap: ".75rem" }}>
          <Field label="Version">
            <input className="input" value={version} onChange={(e) => setVersion(e.target.value)} placeholder="11.4" required />
          </Field>
          <Field label="Port">
            <input className="input" type="number" value={port} onChange={(e) => setPort(e.target.value)} required />
          </Field>
        </div>
        {mode === "register" ? (
          <div className="grid" style={{ gridTemplateColumns: "1fr 1fr", gap: ".75rem" }}>
            <Field label="Admin username (optional)">
              <input className="input" value={username} onChange={(e) => setUsername(e.target.value)} placeholder="root" autoComplete="off" />
            </Field>
            <Field label="Admin password">
              <input className="input" type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="new-password" />
            </Field>
          </div>
        ) : null}
        <p className="muted text-sm">
          {mode === "provision"
            ? "The agent launches a new database container via Docker with a generated admin password (stored encrypted). Requires Docker on the server."
            : "Register a database already running on this server. Credentials enable imports, backups and restores via the agent."}
        </p>
        <ErrorText message={error ?? undefined} />
        <div className="flex items-center justify-end gap-2" style={{ marginTop: ".5rem" }}>
          <button type="button" className="btn" onClick={onClose}>Cancel</button>
          <button type="submit" className="btn btn-primary" disabled={pending}>
            {pending ? "Working…" : mode === "provision" ? "Provision" : "Register"}
          </button>
        </div>
      </form>
    </Modal>
  );
}
