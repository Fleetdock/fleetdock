"use client";

import { useMemo, useState, type FormEvent } from "react";

import { Download, Plug, Plus, Trash2 } from "lucide-react";
import { EmptyState, ErrorText, Field, Modal, Spinner, StatusBadge } from "@/components/ui";
import { ApiError } from "@/lib/api";
import {
  useCreateInstance,
  useDeleteInstance,
  useImportDatabases,
  useInstances,
  useServers,
  useTestConnection,
} from "@/lib/hooks";
import type { Instance } from "@/lib/types";

export default function InstancesPage() {
  const { data, isLoading, error } = useInstances();
  const { data: servers } = useServers();
  const [open, setOpen] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);

  const del = useDeleteInstance();
  const test = useTestConnection();
  const importDbs = useImportDatabases();

  const serverName = useMemo(() => {
    const m = new Map<string, string>();
    servers?.items.forEach((s) => m.set(s.id, s.name));
    return m;
  }, [servers]);

  async function onTest(i: Instance) {
    setNotice(null);
    try {
      const res = await test.mutateAsync(i.id);
      if (res.mode === "async") {
        setNotice(`Connection test queued for the agent on "${serverName.get(i.server_id ?? "") ?? "server"}" — see Operations.`);
      } else if (res.ok) {
        setNotice(`"${i.name}": connected (${res.version}).`);
      } else {
        setNotice(`"${i.name}": connection failed — ${res.error}`);
      }
    } catch (err) {
      setNotice(err instanceof ApiError ? err.message : "Connection test failed");
    }
  }

  async function onImport(i: Instance) {
    setNotice(null);
    try {
      const res = await importDbs.mutateAsync(i.id);
      if (res.mode === "async") {
        setNotice("Import queued for the agent — discovered databases will appear shortly.");
      } else {
        setNotice(`Imported ${res.imported} database${res.imported === 1 ? "" : "s"} from "${i.name}".`);
      }
    } catch (err) {
      setNotice(err instanceof ApiError ? err.message : "Import failed");
    }
  }

  return (
    <div>
      <div className="flex items-center justify-between" style={{ marginBottom: "1.1rem" }}>
        <div>
          <h1 className="text-xl font-semibold">Instances</h1>
          <p className="muted text-sm">
            Database instances — managed on your servers, or external (e.g. running under Dokploy).
          </p>
        </div>
        <button className="btn btn-primary" onClick={() => setOpen(true)}>
          <Plus size={16} /> Add instance
        </button>
      </div>

      {notice ? (
        <div className="card" style={{ padding: ".7rem .9rem", marginBottom: ".9rem" }}>
          <span className="text-sm">{notice}</span>
        </div>
      ) : null}

      {isLoading ? (
        <div className="flex items-center gap-2 muted text-sm"><Spinner /> Loading…</div>
      ) : error ? (
        <EmptyState title="Could not load instances" hint={(error as ApiError).message} />
      ) : !data || data.items.length === 0 ? (
        <EmptyState
          title="No instances yet"
          hint="Add an instance on a connected server, or register an external database."
        />
      ) : (
        <div className="card" style={{ overflow: "hidden" }}>
          <table className="table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Kind</th>
                <th>Engine</th>
                <th>Location</th>
                <th>Credentials</th>
                <th>Status</th>
                <th style={{ textAlign: "right" }}>Actions</th>
              </tr>
            </thead>
            <tbody>
              {data.items.map((i) => (
                <tr key={i.id}>
                  <td className="font-medium">{i.name}</td>
                  <td>
                    <span className={`badge ${i.kind === "external" ? "badge-amber" : "badge-gray"}`}>
                      {i.kind}
                    </span>
                  </td>
                  <td className="muted">{i.engine} {i.engine_version}</td>
                  <td className="muted">
                    {i.kind === "external"
                      ? `${i.host}:${i.port}`
                      : `${serverName.get(i.server_id ?? "") ?? "server"}:${i.port}`}
                  </td>
                  <td className="muted">{i.has_credentials ? "configured" : "—"}</td>
                  <td><StatusBadge status={i.status} /></td>
                  <td style={{ textAlign: "right" }}>
                    <div className="flex items-center gap-2" style={{ justifyContent: "flex-end" }}>
                      {i.has_credentials ? (
                        <>
                          <button className="btn btn-sm" onClick={() => onTest(i)} disabled={test.isPending} title="Test connection">
                            <Plug size={15} /> Test
                          </button>
                          <button className="btn btn-sm" onClick={() => onImport(i)} disabled={importDbs.isPending} title="Import existing databases">
                            <Download size={15} /> Import DBs
                          </button>
                        </>
                      ) : null}
                      <button
                        className="btn btn-sm btn-danger"
                        onClick={() => {
                          if (confirm(`Remove instance "${i.name}" from the control plane? The actual database server is not touched.`)) {
                            del.mutate(i.id);
                          }
                        }}
                        disabled={del.isPending}
                        aria-label="Delete"
                      >
                        <Trash2 size={15} />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <AddInstanceModal open={open} onClose={() => setOpen(false)} servers={servers?.items ?? []} />
    </div>
  );
}

function AddInstanceModal({
  open,
  onClose,
  servers,
}: {
  open: boolean;
  onClose: () => void;
  servers: { id: string; name: string }[];
}) {
  const create = useCreateInstance();
  const [kind, setKind] = useState<"managed" | "external">("managed");
  const [serverId, setServerId] = useState("");
  const [host, setHost] = useState("");
  const [name, setName] = useState("");
  const [version, setVersion] = useState("11.4");
  const [port, setPort] = useState("3306");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);

  function reset() {
    setName("");
    setHost("");
    setUsername("");
    setPassword("");
    setError(null);
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      await create.mutateAsync({
        kind,
        server_id: kind === "managed" ? serverId : undefined,
        host: kind === "external" ? host : undefined,
        name,
        engine: "mariadb",
        engine_version: version,
        port: Number(port),
        username: username || undefined,
        password: password || undefined,
      });
      reset();
      onClose();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to add instance");
    }
  }

  return (
    <Modal open={open} onClose={onClose} title="Add instance">
      <form onSubmit={onSubmit}>
        <Field label="Kind">
          <select className="input" value={kind} onChange={(e) => setKind(e.target.value as "managed" | "external")}>
            <option value="managed">Managed — runs on a connected server</option>
            <option value="external">External — reachable over the network (e.g. Dokploy)</option>
          </select>
        </Field>
        {kind === "managed" ? (
          <Field label="Server">
            <select className="input" value={serverId} onChange={(e) => setServerId(e.target.value)} required>
              <option value="" disabled>Select a server…</option>
              {servers.map((s) => (
                <option key={s.id} value={s.id}>{s.name}</option>
              ))}
            </select>
          </Field>
        ) : (
          <Field label="Host">
            <input className="input" value={host} onChange={(e) => setHost(e.target.value)} placeholder="db.example.com or 10.0.0.5" required />
          </Field>
        )}
        <Field label="Name">
          <input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="primary" required />
        </Field>
        <div className="grid" style={{ gridTemplateColumns: "1fr 1fr", gap: ".75rem" }}>
          <Field label="Engine version">
            <input className="input" value={version} onChange={(e) => setVersion(e.target.value)} placeholder="11.4" required />
          </Field>
          <Field label="Port">
            <input className="input" type="number" value={port} onChange={(e) => setPort(e.target.value)} required />
          </Field>
        </div>
        <div className="grid" style={{ gridTemplateColumns: "1fr 1fr", gap: ".75rem" }}>
          <Field label="Admin username (optional)">
            <input className="input" value={username} onChange={(e) => setUsername(e.target.value)} placeholder="root" autoComplete="off" />
          </Field>
          <Field label="Admin password">
            <input className="input" type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="new-password" />
          </Field>
        </div>
        <p className="muted text-sm" style={{ marginTop: ".2rem" }}>
          Credentials are encrypted at rest and enable database provisioning, imports,
          backups and restores. Without them the instance is metadata-only.
        </p>
        <ErrorText message={error ?? undefined} />
        <div className="flex items-center justify-end gap-2" style={{ marginTop: ".5rem" }}>
          <button type="button" className="btn" onClick={onClose}>Cancel</button>
          <button type="submit" className="btn btn-primary" disabled={create.isPending || (kind === "managed" && servers.length === 0)}>
            {create.isPending ? "Adding…" : "Add instance"}
          </button>
        </div>
        {kind === "managed" && servers.length === 0 ? (
          <p className="muted text-sm">Connect a server first, or add an external instance.</p>
        ) : null}
      </form>
    </Modal>
  );
}
