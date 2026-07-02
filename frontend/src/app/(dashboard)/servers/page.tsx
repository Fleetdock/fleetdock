"use client";

import Link from "next/link";
import { useState, type FormEvent } from "react";

import { ChevronRight, Copy, Plus, Search, Server } from "lucide-react";
import { EmptyState, ErrorText, Field, Modal, Pagination, Spinner, StatusBadge } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { LIST_PAGE_SIZE, useCan, useCreateAgentToken, useCreateServer, useServers } from "@/lib/hooks";

export default function ServersPage() {
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(1);
  const { data, isLoading, error } = useServers(search, page);
  const [registerOpen, setRegisterOpen] = useState(false);
  const [connectOpen, setConnectOpen] = useState(false);
  const can = useCan();

  return (
    <div>
      <div className="flex items-center justify-between" style={{ marginBottom: "1.1rem" }}>
        <div>
          <h1 className="text-xl font-semibold">Servers</h1>
          <p className="muted text-sm">Hosts running the db-manager agent.</p>
        </div>
        {can("server:write") ? (
          <div className="flex items-center gap-2">
            <button className="btn" onClick={() => setRegisterOpen(true)}>
              <Plus size={16} /> Register manually
            </button>
            <button className="btn btn-primary" onClick={() => setConnectOpen(true)}>
              <Server size={16} /> Connect server
            </button>
          </div>
        ) : null}
      </div>

      <div className="flex items-center gap-2" style={{ marginBottom: ".9rem", maxWidth: "22rem" }}>
        <div style={{ position: "relative", width: "100%" }}>
          <span style={{ position: "absolute", left: 10, top: 9, color: "var(--muted)" }}>
            <Search size={16} />
          </span>
          <input
            className="input"
            style={{ paddingLeft: "2rem" }}
            placeholder="Search name or hostname…"
            value={search}
            onChange={(e) => {
              setSearch(e.target.value);
              setPage(1);
            }}
          />
        </div>
      </div>

      {isLoading ? (
        <div className="flex items-center gap-2 muted text-sm"><Spinner /> Loading…</div>
      ) : error ? (
        <EmptyState title="Could not load servers" hint={(error as ApiError).message} />
      ) : !data || data.items.length === 0 ? (
        <EmptyState
          title="No servers yet"
          hint="Use “Connect server” to get a one-command installer for your host."
        />
      ) : (
        <div className="card" style={{ overflow: "hidden" }}>
          <table className="table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Hostname</th>
                <th>Status</th>
                <th>Agent</th>
                <th>Last heartbeat</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {data.items.map((s) => (
                <tr key={s.id}>
                  <td className="font-medium">{s.name}</td>
                  <td className="muted">{s.hostname}</td>
                  <td><StatusBadge status={s.status} /></td>
                  <td className="muted">{s.agent_version ?? "—"}</td>
                  <td className="muted">
                    {s.last_heartbeat_at ? new Date(s.last_heartbeat_at).toLocaleString() : "never"}
                  </td>
                  <td style={{ textAlign: "right" }}>
                    <Link href={`/servers/${s.id}`} className="btn btn-ghost btn-sm">
                      Open <ChevronRight size={15} />
                    </Link>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {data ? (
        <div className="flex items-center justify-end" style={{ marginTop: ".6rem" }}>
          <Pagination
            page={page}
            pageCount={Math.max(1, Math.ceil(data.pagination.total / LIST_PAGE_SIZE))}
            onPage={setPage}
          />
        </div>
      ) : null}

      <RegisterServerModal open={registerOpen} onClose={() => setRegisterOpen(false)} />
      <ConnectServerModal open={connectOpen} onClose={() => setConnectOpen(false)} />
    </div>
  );
}

function ConnectServerModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const create = useCreateAgentToken();
  const [name, setName] = useState("");
  const [command, setCommand] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function generate(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      const res = await create.mutateAsync({ name: name || undefined, ttl_hours: 24 });
      setCommand(res.install_command);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to create registration token");
    }
  }

  function close() {
    setCommand(null);
    setName("");
    setCopied(false);
    onClose();
  }

  return (
    <Modal open={open} onClose={close} title="Connect a server">
      {command ? (
        <div>
          <p className="text-sm" style={{ marginBottom: ".6rem" }}>
            Run this on your server as root. The agent installs itself, enrolls with the control
            plane and appears here within ~30 seconds. The token is single-use and valid for 24h.
          </p>
          <pre
            className="card"
            style={{ padding: ".8rem", fontSize: 12, overflowX: "auto", whiteSpace: "pre-wrap", wordBreak: "break-all" }}
          >
            {command}
          </pre>
          <div className="flex items-center justify-end gap-2" style={{ marginTop: ".6rem" }}>
            <button
              className="btn"
              onClick={() => {
                navigator.clipboard.writeText(command);
                setCopied(true);
              }}
            >
              <Copy size={15} /> {copied ? "Copied!" : "Copy command"}
            </button>
            <button className="btn btn-primary" onClick={close}>Done</button>
          </div>
        </div>
      ) : (
        <form onSubmit={generate}>
          <p className="muted text-sm" style={{ marginBottom: ".8rem" }}>
            Generates a single-use registration token and the one-command installer
            (like Dokploy). No manual server registration needed.
          </p>
          <Field label="Label (optional)">
            <input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="db-eu-1" />
          </Field>
          <ErrorText message={error ?? undefined} />
          <div className="flex items-center justify-end gap-2" style={{ marginTop: ".5rem" }}>
            <button type="button" className="btn" onClick={close}>Cancel</button>
            <button type="submit" className="btn btn-primary" disabled={create.isPending}>
              {create.isPending ? "Generating…" : "Generate install command"}
            </button>
          </div>
        </form>
      )}
    </Modal>
  );
}

function RegisterServerModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const create = useCreateServer();
  const [name, setName] = useState("");
  const [hostname, setHostname] = useState("");
  const [address, setAddress] = useState("");
  const [tags, setTags] = useState("");
  const [error, setError] = useState<string | null>(null);

  function reset() {
    setName("");
    setHostname("");
    setAddress("");
    setTags("");
    setError(null);
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      await create.mutateAsync({
        name,
        hostname,
        address: address || undefined,
        tags: tags ? tags.split(",").map((t) => t.trim()).filter(Boolean) : undefined,
      });
      reset();
      onClose();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to register server");
    }
  }

  return (
    <Modal open={open} onClose={onClose} title="Register server manually">
      <form onSubmit={onSubmit}>
        <p className="muted text-sm" style={{ marginBottom: ".8rem" }}>
          Registers a server record without installing an agent. Prefer “Connect server”
          for full functionality (heartbeats, backups, database provisioning).
        </p>
        <Field label="Name">
          <input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="db-eu-1" required />
        </Field>
        <Field label="Hostname">
          <input className="input" value={hostname} onChange={(e) => setHostname(e.target.value)} placeholder="db1.internal" required />
        </Field>
        <Field label="Address (optional)">
          <input className="input" value={address} onChange={(e) => setAddress(e.target.value)} placeholder="10.0.0.10" />
        </Field>
        <Field label="Tags (comma separated)">
          <input className="input" value={tags} onChange={(e) => setTags(e.target.value)} placeholder="prod, eu" />
        </Field>
        <ErrorText message={error ?? undefined} />
        <div className="flex items-center justify-end gap-2" style={{ marginTop: ".5rem" }}>
          <button type="button" className="btn" onClick={onClose}>Cancel</button>
          <button type="submit" className="btn btn-primary" disabled={create.isPending}>
            {create.isPending ? "Registering…" : "Register"}
          </button>
        </div>
      </form>
    </Modal>
  );
}
