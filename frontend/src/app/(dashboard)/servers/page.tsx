"use client";

import Link from "next/link";
import { useState, type FormEvent } from "react";

import { ChevronRight, Copy, Plus, Server } from "lucide-react";
import { DataTable } from "@/components/data-table";
import { ErrorText, Field, Modal, StatusBadge } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { LIST_PAGE_SIZE, useCan, useCreateAgentToken, useCreateServer, useServers } from "@/lib/hooks";
import type { Server as ServerType } from "@/lib/types";

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

      <DataTable<ServerType>
        columns={[
          { id: "name", header: "Name", className: "font-medium", render: (s) => s.name },
          { id: "hostname", header: "Hostname", className: "muted", render: (s) => s.hostname },
          { id: "status", header: "Status", render: (s) => <StatusBadge status={s.status} /> },
          { id: "agent", header: "Agent", className: "muted", render: (s) => s.agent_version ?? "—" },
          {
            id: "heartbeat",
            header: "Last heartbeat",
            className: "muted",
            render: (s) => (s.last_heartbeat_at ? new Date(s.last_heartbeat_at).toLocaleString() : "never"),
          },
          {
            id: "actions",
            header: "",
            align: "right",
            render: (s) => (
              <Link href={`/servers/${s.id}`} className="btn btn-ghost btn-sm">
                Open <ChevronRight size={15} />
              </Link>
            ),
          },
        ]}
        rows={data?.items ?? []}
        rowKey={(s) => s.id}
        isLoading={isLoading}
        error={error ? (error as ApiError).message : undefined}
        errorTitle="Could not load servers"
        emptyTitle="No servers yet"
        emptyHint='Use "Connect server" to get a one-command installer for your host.'
        emptySearchTitle="No servers match your search"
        search={{
          value: search,
          onChange: (v) => {
            setSearch(v);
            setPage(1);
          },
          placeholder: "Search name or hostname…",
        }}
        pagination={{
          page,
          pageCount: Math.max(1, Math.ceil((data?.pagination.total ?? 0) / LIST_PAGE_SIZE)),
          onPage: setPage,
        }}
      />

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

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      const res = await create.mutateAsync({ name: name || "install" });
      setCommand(res.install_command);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to generate install command");
    }
  }

  function close() {
    setName("");
    setCommand(null);
    setCopied(false);
    setError(null);
    onClose();
  }

  return (
    <Modal open={open} onClose={close} title="Connect a server">
      {command ? (
        <div>
          <p className="text-sm" style={{ marginBottom: ".6rem" }}>
            Run this on the host you want to connect (requires curl and bash):
          </p>
          <div
            className="card"
            style={{ padding: ".7rem", fontFamily: "ui-monospace, monospace", fontSize: ".75rem", wordBreak: "break-all", background: "var(--panel-2)" }}
          >
            {command}
          </div>
          <div className="flex items-center justify-end gap-2" style={{ marginTop: ".9rem" }}>
            <button
              className="btn"
              onClick={() => {
                navigator.clipboard?.writeText(command);
                setCopied(true);
              }}
            >
              <Copy size={15} /> {copied ? "Copied" : "Copy"}
            </button>
            <button className="btn btn-primary" onClick={close}>Done</button>
          </div>
        </div>
      ) : (
        <form onSubmit={onSubmit}>
          <Field label="Label (optional)">
            <input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="prod-01" />
          </Field>
          <p className="muted text-sm">Generates a single-use registration token and install script.</p>
          <ErrorText message={error ?? undefined} />
          <div className="flex items-center justify-end gap-2" style={{ marginTop: ".7rem" }}>
            <button type="button" className="btn" onClick={close}>Cancel</button>
            <button type="submit" className="btn btn-primary" disabled={create.isPending}>
              {create.isPending ? "Generating…" : "Generate command"}
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
  const [tags, setTags] = useState("");
  const [error, setError] = useState<string | null>(null);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      await create.mutateAsync({
        name,
        hostname,
        tags: tags ? tags.split(",").map((t) => t.trim()).filter(Boolean) : undefined,
      });
      setName("");
      setHostname("");
      setTags("");
      onClose();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to register server");
    }
  }

  return (
    <Modal open={open} onClose={onClose} title="Register server manually">
      <form onSubmit={onSubmit}>
        <Field label="Name">
          <input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="prod-01" required />
        </Field>
        <Field label="Hostname">
          <input className="input" value={hostname} onChange={(e) => setHostname(e.target.value)} placeholder="db1.example.com" required />
        </Field>
        <Field label="Tags (comma-separated)">
          <input className="input" value={tags} onChange={(e) => setTags(e.target.value)} placeholder="production,eu" />
        </Field>
        <ErrorText message={error ?? undefined} />
        <div className="flex items-center justify-end gap-2" style={{ marginTop: ".7rem" }}>
          <button type="button" className="btn" onClick={onClose}>Cancel</button>
          <button type="submit" className="btn btn-primary" disabled={create.isPending}>
            {create.isPending ? "Registering…" : "Register"}
          </button>
        </div>
      </form>
    </Modal>
  );
}
