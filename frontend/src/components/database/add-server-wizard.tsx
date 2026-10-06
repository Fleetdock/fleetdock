"use client";

import Link from "next/link";
import { useState, type FormEvent, type ReactNode } from "react";

import { Cloud, Plus, Server as ServerIcon } from "lucide-react";

import { ErrorText, Field, Modal, Spinner } from "@/components/ui";
import { fieldError, friendlyError } from "@/lib/errors";
import { ENGINE_IDS, ENGINES, type EngineId } from "@/lib/engines";
import { useCreateInstance, useDatabases, useProbeInstance, useProvisionInstance, useServers } from "@/lib/hooks";
import type { Instance, TLSMode } from "@/lib/types";

type Mode = "provision" | "register" | "external";

const MODES: { id: Mode; icon: ReactNode; title: string; text: string }[] = [
  {
    id: "provision",
    icon: <Plus size={18} />,
    title: "Create a new one",
    text: "Fleetdock starts a fresh database server (in Docker) on one of your connected servers.",
  },
  {
    id: "register",
    icon: <ServerIcon size={18} />,
    title: "Use one on my server",
    text: "A database server already runs on one of your connected servers; Fleetdock manages it through the agent.",
  },
  {
    id: "external",
    icon: <Cloud size={18} />,
    title: "Connect to one anywhere",
    text: "Any database server Fleetdock can reach over the network — hosted, cloud or on another machine.",
  },
];

/**
 * AddServerWizard is the single way to bring a database server into Fleetdock:
 * create a new one, adopt one on a connected server, or connect to one
 * anywhere. Its databases are discovered automatically afterwards.
 */
export function AddServerWizard({
  open,
  onClose,
  serverId,
}: {
  open: boolean;
  onClose: () => void;
  /** Preselect a connected server (when opened from a server's page). */
  serverId?: string;
}) {
  const { data: servers } = useServers();
  const create = useCreateInstance();
  const provision = useProvisionInstance();
  const [mode, setMode] = useState<Mode | null>(serverId ? "provision" : null);
  const [server, setServer] = useState(serverId ?? "");
  const [engine, setEngine] = useState<EngineId>("postgres");
  const [version, setVersion] = useState(ENGINES.postgres.version);
  const [port, setPort] = useState(String(ENGINES.postgres.port));
  const [name, setName] = useState("");
  const [host, setHost] = useState("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [tls, setTls] = useState<TLSMode>("prefer");
  const [error, setError] = useState<unknown>(null);
  const [done, setDone] = useState<{ instance: Instance; operationId?: string } | null>(null);

  const probe = useProbeInstance();
  const serverList = servers?.items ?? [];
  const busy = create.isPending || provision.isPending;

  function close() {
    setMode(serverId ? "provision" : null);
    setName("");
    setHost("");
    setUsername("");
    setPassword("");
    setError(null);
    setDone(null);
    onClose();
  }

  function pickEngine(e: EngineId) {
    setEngine(e);
    setVersion(ENGINES[e].version);
    setPort(String(ENGINES[e].port));
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      if (mode === "provision") {
        const res = await provision.mutateAsync({ server_id: server, name, engine, engine_version: version, port: Number(port) });
        setDone({ instance: res.instance, operationId: res.operation_id });
      } else {
        const inst = await create.mutateAsync({
          kind: mode === "external" ? "external" : "managed",
          server_id: mode === "register" ? server : undefined,
          host: mode === "external" ? host.trim() : undefined,
          name,
          engine,
          engine_version: version,
          port: Number(port),
          username: username || undefined,
          password: password || undefined,
          tls_mode: tls,
        });
        setDone({ instance: inst });
        // Check it right away instead of waiting for the next minute's probe,
        // so its databases show up while the user is still looking.
        if (inst.has_credentials) probe.mutate(inst.id);
      }
    } catch (err) {
      setError(err);
    }
  }

  const title = done ? "Database server added" : mode ? MODES.find((m) => m.id === mode)!.title : "Connect a database server";

  return (
    <Modal open={open} onClose={close} title={title} wide>
      {done ? (
        <div>
          <p style={{ marginTop: 0 }}>
            <strong>{done.instance.name}</strong> was added.{" "}
            {done.operationId
              ? "Fleetdock is starting it now — this usually takes under a minute."
              : done.instance.has_credentials
                ? "Fleetdock is looking for its databases — they appear automatically."
                : "Add an admin login later so Fleetdock can find its databases."}
          </p>
          {done.instance.has_credentials || done.operationId ? <Discovery instanceId={done.instance.id} /> : null}
          <div className="flex justify-end gap-2" style={{ flexWrap: "wrap" }}>
            {done.operationId ? (
              <Link href={`/activity/${done.operationId}`} className="btn">
                Follow progress
              </Link>
            ) : null}
            <Link href={`/instances/${done.instance.id}`} className="btn btn-primary" onClick={close}>
              Open {done.instance.name}
            </Link>
          </div>
        </div>
      ) : !mode ? (
        <div className="grid gap-2">
          {MODES.map((m) => (
            <button key={m.id} type="button" className="choice-card" onClick={() => setMode(m.id)}>
              <span className="choice-card-icon">{m.icon}</span>
              <span>
                <span className="font-medium">{m.title}</span>
                <span className="muted text-sm" style={{ display: "block", marginTop: ".15rem" }}>
                  {m.text}
                </span>
              </span>
            </button>
          ))}
        </div>
      ) : (
        <form onSubmit={onSubmit}>
          {mode !== "external" ? (
            serverList.length === 0 ? (
              <p className="text-sm" style={{ marginTop: 0 }}>
                You haven&apos;t connected a server yet. <Link href="/servers" className="link">Connect a server</Link> first,
                or connect to a database server anywhere instead.
              </p>
            ) : (
              <Field label="On which server?" error={fieldError(error, "server_id")}>
                <select className="input" value={server} onChange={(e) => setServer(e.target.value)} required>
                  <option value="" disabled>
                    Choose a server…
                  </option>
                  {serverList.map((s) => (
                    <option key={s.id} value={s.id}>
                      {s.name}
                    </option>
                  ))}
                </select>
              </Field>
            )
          ) : null}

          <div className="form-grid">
            <Field label="Engine">
              <select className="input" value={engine} onChange={(e) => pickEngine(e.target.value as EngineId)}>
                {ENGINE_IDS.map((id) => (
                  <option key={id} value={id}>
                    {ENGINES[id].label}
                  </option>
                ))}
              </select>
            </Field>
            <Field
              label="Version"
              error={fieldError(error, "engine_version")}
              hint={mode === "provision" ? undefined : "The version running there — used for compatibility."}
            >
              <input className="input" list="engine-versions" value={version} onChange={(e) => setVersion(e.target.value)} required />
            </Field>
            <datalist id="engine-versions">
              {ENGINES[engine].versions.map((v) => (
                <option key={v} value={v} />
              ))}
            </datalist>
          </div>

          <div className="form-grid">
            <Field label="Name" hint="How it appears in Fleetdock, e.g. “production” or “analytics”." error={fieldError(error, "name")}>
              <input className="input" value={name} onChange={(e) => setName(e.target.value)} required autoFocus />
            </Field>
            <Field
              label="Port"
              error={fieldError(error, "port")}
              help={mode === "provision" ? "The port the new server listens on, on its host. Pick a free one." : undefined}
            >
              <input className="input" type="number" min={1} max={65535} value={port} onChange={(e) => setPort(e.target.value)} required />
            </Field>
          </div>

          {mode === "external" ? (
            <Field label="Host" hint="Hostname or IP address Fleetdock can reach."
              help={<>On the same computer as Fleetdock? Use <code>host.docker.internal</code> — <code>localhost</code> would mean Fleetdock&apos;s own container.</>} error={fieldError(error, "host")}>
              <input className="input" value={host} onChange={(e) => setHost(e.target.value)} placeholder="db.example.com" required />
            </Field>
          ) : null}

          {mode !== "provision" ? (
            <>
              <div className="form-grid">
                <Field
                  label="Admin user"
                  help="An account allowed to create databases and users. Fleetdock needs it to find databases, take backups and show monitoring — you can add it later."
                  error={fieldError(error, "username")}
                >
                  <input className="input" value={username} onChange={(e) => setUsername(e.target.value)} placeholder={ENGINES[engine].adminUser} autoComplete="off" />
                </Field>
                <Field label="Password" error={fieldError(error, "password")}>
                  <input className="input" type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="new-password" />
                </Field>
              </div>
              <Field
                label="Encryption"
                help="How Fleetdock protects its connection to this server. Use “Verify” across networks you don't trust."
              >
                <select className="input" value={tls} onChange={(e) => setTls(e.target.value as TLSMode)}>
                  <option value="prefer">Use encryption when available (default)</option>
                  <option value="require">Always encrypt</option>
                  <option value="verify-full">Always encrypt and verify the certificate</option>
                  <option value="disable">Never encrypt</option>
                </select>
              </Field>
            </>
          ) : (
            <p className="muted text-sm">Fleetdock generates a strong admin password and keeps it encrypted.</p>
          )}

          <ErrorText message={error ? friendlyError(error) : undefined} />
          <div className="flex justify-between gap-2" style={{ marginTop: ".9rem", flexWrap: "wrap" }}>
            {serverId ? <span /> : (
              <button type="button" className="btn btn-ghost" onClick={() => setMode(null)} disabled={busy}>
                ← Back
              </button>
            )}
            <button className="btn btn-primary" type="submit" disabled={busy || (mode !== "external" && serverList.length === 0)}>
              {busy ? "Working…" : mode === "provision" ? "Create database server" : "Connect"}
            </button>
          </div>
        </form>
      )}
    </Modal>
  );
}

/** Discovery shows the databases found on a newly added server as they arrive. */
function Discovery({ instanceId }: { instanceId: string }) {
  const { data } = useDatabases({ instance_id: instanceId });
  const found = (data?.items ?? []).filter((d) => !d.system);
  return (
    <p className="muted text-sm flex items-center gap-2" role="status" aria-live="polite">
      {found.length === 0 ? (
        <>
          <Spinner /> Waiting for databases…
        </>
      ) : (
        <>
          Found {found.length} database{found.length === 1 ? "" : "s"}:{" "}
          {found
            .slice(0, 5)
            .map((d) => d.name)
            .join(", ")}
          {found.length > 5 ? ` and ${found.length - 5} more` : ""}
        </>
      )}
    </p>
  );
}
