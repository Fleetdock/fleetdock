"use client";

import { useState, type FormEvent } from "react";

import { Key, Plus, Trash2 } from "lucide-react";
import { EmptyState, ErrorText, Field, Modal, Pagination, Spinner } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { useCan, useClientPage, useCreateToken, useRevokeToken, useTokens } from "@/lib/hooks";

export default function TokensPage() {
  const { data, isLoading, error } = useTokens();
  const revoke = useRevokeToken();
  const [open, setOpen] = useState(false);
  const [secret, setSecret] = useState<string | null>(null);
  const can = useCan();
  const canWrite = can("token:write");
  const paged = useClientPage(data?.items);

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

      {isLoading ? (
        <div className="flex items-center gap-2 muted text-sm"><Spinner /> Loading…</div>
      ) : error ? (
        <EmptyState title="Could not load tokens" hint={(error as ApiError).message} />
      ) : !data || data.items.length === 0 ? (
        <EmptyState title="No tokens" hint="Create a token to call the API from scripts or CI." />
      ) : (
        <div className="card" style={{ overflow: "hidden" }}>
          <table className="table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Prefix</th>
                <th>Created</th>
                <th>Last used</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {paged.items.map((t) => (
                <tr key={t.id}>
                  <td className="font-medium flex items-center gap-2">
                    <Key size={15} /> {t.name}
                  </td>
                  <td className="muted"><code>{t.prefix}…</code></td>
                  <td className="muted">{new Date(t.created_at).toLocaleDateString()}</td>
                  <td className="muted">{t.last_used_at ? new Date(t.last_used_at).toLocaleString() : "never"}</td>
                  <td style={{ textAlign: "right" }}>
                    {t.revoked_at ? (
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
                    ) : null}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <div className="flex items-center justify-end" style={{ marginTop: ".6rem" }}>
        <Pagination page={paged.page} pageCount={paged.pageCount} onPage={paged.setPage} />
      </div>

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
  const [name, setName] = useState("");
  const [error, setError] = useState<string | null>(null);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      const res = await create.mutateAsync({ name });
      setName("");
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
        <p className="muted text-sm">The token inherits your account permissions.</p>
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
