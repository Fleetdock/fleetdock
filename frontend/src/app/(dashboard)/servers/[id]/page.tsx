"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useState, type FormEvent } from "react";

import { Plus } from "lucide-react";
import { EmptyState, ErrorText, Field, Modal, Spinner, StatusBadge } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { useCreateInstance, useInstances, useServer } from "@/lib/hooks";

export default function ServerDetailPage() {
  const params = useParams();
  const id = String(params.id);
  const { data: server, isLoading } = useServer(id);
  const { data: instances } = useInstances(id);
  const [open, setOpen] = useState(false);

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
        <button className="btn btn-primary" onClick={() => setOpen(true)}>
          <Plus size={16} /> Add instance
        </button>
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

      <h2 className="font-semibold" style={{ marginBottom: ".6rem" }}>Instances</h2>
      {!instances || instances.items.length === 0 ? (
        <EmptyState title="No instances" hint="Add a MariaDB instance running on this server." />
      ) : (
        <div className="card" style={{ overflow: "hidden" }}>
          <table className="table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Version</th>
                <th>Port</th>
                <th>Status</th>
              </tr>
            </thead>
            <tbody>
              {instances.items.map((i) => (
                <tr key={i.id}>
                  <td className="font-medium">{i.name}</td>
                  <td className="muted">{i.mariadb_version}</td>
                  <td className="muted">{i.port}</td>
                  <td><StatusBadge status={i.status} /></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

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

function AddInstanceModal({ open, onClose, serverId }: { open: boolean; onClose: () => void; serverId: string }) {
  const create = useCreateInstance();
  const [name, setName] = useState("");
  const [version, setVersion] = useState("11.4");
  const [port, setPort] = useState("3306");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      await create.mutateAsync({
        kind: "managed",
        server_id: serverId,
        name,
        engine: "mariadb",
        engine_version: version,
        port: Number(port),
        username: username || undefined,
        password: password || undefined,
      });
      setName("");
      setUsername("");
      setPassword("");
      onClose();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to add instance");
    }
  }

  return (
    <Modal open={open} onClose={onClose} title="Add instance">
      <form onSubmit={onSubmit}>
        <Field label="Name">
          <input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="primary" required />
        </Field>
        <Field label="MariaDB version">
          <input className="input" value={version} onChange={(e) => setVersion(e.target.value)} placeholder="11.4" required />
        </Field>
        <Field label="Port">
          <input className="input" type="number" value={port} onChange={(e) => setPort(e.target.value)} required />
        </Field>
        <div className="grid" style={{ gridTemplateColumns: "1fr 1fr", gap: ".75rem" }}>
          <Field label="Admin username (optional)">
            <input className="input" value={username} onChange={(e) => setUsername(e.target.value)} placeholder="root" autoComplete="off" />
          </Field>
          <Field label="Admin password">
            <input className="input" type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="new-password" />
          </Field>
        </div>
        <p className="muted text-sm">
          Credentials enable provisioning, imports, backups and restores via the agent.
        </p>
        <ErrorText message={error ?? undefined} />
        <div className="flex items-center justify-end gap-2" style={{ marginTop: ".5rem" }}>
          <button type="button" className="btn" onClick={onClose}>Cancel</button>
          <button type="submit" className="btn btn-primary" disabled={create.isPending}>
            {create.isPending ? "Adding…" : "Add instance"}
          </button>
        </div>
      </form>
    </Modal>
  );
}
