"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useState, type FormEvent } from "react";

import { Plus } from "lucide-react";
import { DataTable } from "@/components/data-table";
import { MetricChart, type ChartPoint } from "@/components/chart";
import { EmptyState, ErrorText, Field, Modal, Spinner, StatusBadge } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { useCan, useCreateInstance, useInstances, useProvisionInstance, useServer, useServerMetrics } from "@/lib/hooks";

export default function ServerDetailPage() {
  const params = useParams();
  const id = String(params.id);
  const { data: server, isLoading } = useServer(id);
  const { data: instances } = useInstances(id);
  const [open, setOpen] = useState(false);
  const can = useCan();

  if (isLoading) {
    return <div className="flex items-center gap-2 muted text-sm"><Spinner /> Loading…</div>;
  }
  if (!server) {
    return <EmptyState title="Server not found" />;
  }

  return (
    <div>
      <Link href="/servers" className="muted text-sm">← Servers</Link>
      <div className="flex items-center justify-between" style={{ margin: ".6rem 0 1.1rem" }}>
        <div className="flex items-center gap-3">
          <h1 className="text-xl font-semibold">{server.name}</h1>
          <StatusBadge status={server.status} />
        </div>
        {can("instance:write") ? (
          <button className="btn btn-primary" onClick={() => setOpen(true)}>
            <Plus size={16} /> Add instance
          </button>
        ) : null}
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
