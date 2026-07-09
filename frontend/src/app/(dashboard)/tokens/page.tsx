"use client";

import { useMemo, useState, type FormEvent } from "react";

import { Key, Plus, Trash2 } from "lucide-react";
import { DataTable } from "@/components/data-table";
import { ErrorText, Field, Modal } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { useCan, useCreateToken, useMe, useRevokeToken, useTokens } from "@/lib/hooks";
import { useDataTable } from "@/lib/use-data-table";
import type { ApiToken } from "@/lib/types";

export default function TokensPage() {
  const { data, isLoading, error } = useTokens();
  const revoke = useRevokeToken();
  const [open, setOpen] = useState(false);
  const [secret, setSecret] = useState<string | null>(null);
  const can = useCan();
  const canWrite = can("token:write");
  const table = useDataTable({ items: data?.items });

  return (
    <div>
      <div className="flex items-center justify-between" style={{ marginBottom: "1.1rem" }}>
        <div>
          <h1 className="text-xl font-semibold">API Tokens</h1>
          <p className="muted text-sm">Programmatic access scoped to your permissions.</p>
        </div>
        {canWrite ? (
          <button className="btn btn-primary" onClick={() => setOpen(true)}>
            <Plus size={16} /> New token
          </button>
        ) : null}
      </div>

      <DataTable<ApiToken>
        columns={[
          {
            id: "name",
            header: "Name",
            className: "font-medium flex items-center gap-2",
            render: (t) => (
              <>
                <Key size={15} /> {t.name}
              </>
            ),
          },
          {
            id: "prefix",
            header: "Prefix",
            className: "muted",
            render: (t) => <code>{t.prefix}…</code>,
          },
          {
            id: "scopes",
            header: "Scopes",
            className: "muted text-sm",
            render: (t) =>
              t.scopes.length === 0 ? (
                <span className="muted">all (account)</span>
              ) : (
                t.scopes.join(", ")
              ),
          },
          {
            id: "expires",
            header: "Expires",
            className: "muted",
            render: (t) => (t.expires_at ? new Date(t.expires_at).toLocaleDateString() : "never"),
          },
          {
            id: "created",
            header: "Created",
            className: "muted",
            render: (t) => new Date(t.created_at).toLocaleDateString(),
          },
          {
            id: "last_used",
            header: "Last used",
            className: "muted",
            render: (t) => (t.last_used_at ? new Date(t.last_used_at).toLocaleString() : "never"),
          },
          {
            id: "actions",
            header: "",
            align: "right",
            render: (t) =>
              t.revoked_at ? (
                <span className="badge badge-red"><span className="dot" /> revoked</span>
              ) : canWrite ? (
                <button
                  className="btn btn-sm btn-danger"
                  onClick={() => {
                    if (confirm(`Revoke token "${t.name}"?`)) revoke.mutate(t.id);
                  }}
                  aria-label="Revoke"
                >
                  <Trash2 size={15} />
                </button>
              ) : null,
          },
        ]}
        rows={table.rows}
        rowKey={(t) => t.id}
        isLoading={isLoading}
        error={error ? (error as ApiError).message : undefined}
        emptyTitle="No tokens"
        emptyHint="Create a token to call the API from scripts or CI."
        pagination={{ page: table.page, pageCount: table.pageCount, onPage: table.setPage }}
      />

      <CreateTokenModal open={open} onClose={() => setOpen(false)} onCreated={setSecret} />

      <Modal open={secret !== null} onClose={() => setSecret(null)} title="Token created">
        <p className="text-sm" style={{ marginBottom: ".6rem" }}>
          Copy this token now — it will not be shown again.
        </p>
        <div
          className="card"
          style={{ padding: ".7rem", fontFamily: "ui-monospace, monospace", fontSize: ".8rem", wordBreak: "break-all", background: "var(--panel-2)" }}
        >
          {secret}
        </div>
        <div className="flex items-center justify-end gap-2" style={{ marginTop: ".9rem" }}>
          <button
            className="btn"
            onClick={() => {
              if (secret) navigator.clipboard?.writeText(secret);
            }}
          >
            Copy
          </button>
          <button className="btn btn-primary" onClick={() => setSecret(null)}>Done</button>
        </div>
      </Modal>
    </div>
  );
}

function CreateTokenModal({
  open,
  onClose,
  onCreated,
}: {
  open: boolean;
  onClose: () => void;
  onCreated: (secret: string) => void;
}) {
  const create = useCreateToken();
  const { data: me } = useMe();
  const [name, setName] = useState("");
  const [scopes, setScopes] = useState<string[]>([]);
  const [ttlHours, setTtlHours] = useState("");
  const [error, setError] = useState<string | null>(null);

  // A token can only carry scopes the user actually holds (at any scope).
  const available = useMemo(() => {
    const held = new Set((me?.grants ?? []).map((g) => g.permission));
    return [...held].sort();
  }, [me]);

  function toggle(perm: string) {
    setScopes((s) => (s.includes(perm) ? s.filter((p) => p !== perm) : [...s, perm]));
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    const ttl = ttlHours.trim() === "" ? undefined : Number(ttlHours);
    if (ttl !== undefined && (!Number.isFinite(ttl) || ttl <= 0)) {
      setError("Expiry must be a positive number of hours.");
      return;
    }
    try {
      const res = await create.mutateAsync({ name, scopes, ttl_hours: ttl });
      setName("");
      setScopes([]);
      setTtlHours("");
      onClose();
      onCreated(res.token);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to create token");
    }
  }

  return (
    <Modal open={open} onClose={onClose} title="New API token">
      <form onSubmit={onSubmit}>
        <Field label="Name">
          <input className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="ci-pipeline" required />
        </Field>
        <Field label="Scopes (none selected = inherit all your permissions)">
          <div
            className="card"
            style={{ padding: ".5rem .6rem", maxHeight: "11rem", overflowY: "auto", display: "grid", gap: ".3rem" }}
          >
            {available.length === 0 ? (
              <span className="muted text-sm">No permissions available.</span>
            ) : (
              available.map((perm) => (
                <label key={perm} className="flex items-center gap-2 text-sm" style={{ cursor: "pointer" }}>
                  <input type="checkbox" checked={scopes.includes(perm)} onChange={() => toggle(perm)} />
                  <code>{perm}</code>
                </label>
              ))
            )}
          </div>
        </Field>
        <Field label="Expires in (hours, optional)">
          <input
            className="input"
            type="number"
            min={1}
            value={ttlHours}
            onChange={(e) => setTtlHours(e.target.value)}
            placeholder="never"
          />
        </Field>
        <ErrorText message={error ?? undefined} />
        <div className="flex items-center justify-end gap-2" style={{ marginTop: ".7rem" }}>
          <button type="button" className="btn" onClick={onClose}>Cancel</button>
          <button type="submit" className="btn btn-primary" disabled={create.isPending}>
            {create.isPending ? "Creating…" : "Create token"}
          </button>
        </div>
      </form>
    </Modal>
  );
}
