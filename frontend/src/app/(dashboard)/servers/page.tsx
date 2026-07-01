"use client";

import Link from "next/link";
import { useState, type FormEvent } from "react";

import { ChevronRightIcon, PlusIcon, SearchIcon } from "@/components/icons";
import { EmptyState, ErrorText, Field, Modal, Spinner, StatusBadge } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { useCreateServer, useServers } from "@/lib/hooks";

export default function ServersPage() {
  const [search, setSearch] = useState("");
  const { data, isLoading, error } = useServers(search);
  const [open, setOpen] = useState(false);

  return (
    <div>
      <div className="flex items-center justify-between" style={{ marginBottom: "1.1rem" }}>
        <div>
          <h1 className="text-xl font-semibold">Servers</h1>
          <p className="muted text-sm">Hosts running the MariaDB agent.</p>
        </div>
        <button className="btn btn-primary" onClick={() => setOpen(true)}>
          <PlusIcon size={16} /> Register server
        </button>
      </div>

      <div className="flex items-center gap-2" style={{ marginBottom: ".9rem", maxWidth: "22rem" }}>
        <div style={{ position: "relative", width: "100%" }}>
          <span style={{ position: "absolute", left: 10, top: 9, color: "var(--muted)" }}>
            <SearchIcon size={16} />
          </span>
          <input
            className="input"
            style={{ paddingLeft: "2rem" }}
            placeholder="Search name or hostname…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </div>
      </div>

      {isLoading ? (
        <div className="flex items-center gap-2 muted text-sm"><Spinner /> Loading…</div>
      ) : error ? (
        <EmptyState title="Could not load servers" hint={(error as ApiError).message} />
      ) : !data || data.items.length === 0 ? (
        <EmptyState title="No servers yet" hint="Register your first server to get started." />
      ) : (
        <div className="card" style={{ overflow: "hidden" }}>
          <table className="table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Hostname</th>
                <th>Status</th>
                <th>Tags</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {data.items.map((s) => (
                <tr key={s.id}>
                  <td className="font-medium">{s.name}</td>
                  <td className="muted">{s.hostname}</td>
                  <td><StatusBadge status={s.status} /></td>
                  <td className="muted">{s.tags.length ? s.tags.join(", ") : "—"}</td>
                  <td style={{ textAlign: "right" }}>
                    <Link href={`/servers/${s.id}`} className="btn btn-ghost btn-sm">
                      Open <ChevronRightIcon size={15} />
                    </Link>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <RegisterServerModal open={open} onClose={() => setOpen(false)} />
    </div>
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
    <Modal open={open} onClose={onClose} title="Register server">
      <form onSubmit={onSubmit}>
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
