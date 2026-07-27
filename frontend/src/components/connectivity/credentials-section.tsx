"use client";

import { useState } from "react";
import { KeyRound, Plus, RotateCw, Trash2 } from "lucide-react";

import { DataTable } from "@/components/data-table";
import { ConfirmModal, ErrorText, Field, Modal, SecretReveal } from "@/components/ui";
import { ApiError } from "@/lib/api";
import {
  useConnectivity,
  useCreateCredential,
  useDatabaseCredentials,
  useRevokeCredential,
  useRotateCredential,
} from "@/lib/hooks";
import type { CredentialCreateResult, DatabaseCredential } from "@/lib/types";

export function CredentialsSection({ databaseId, canWrite }: { databaseId: string; canWrite: boolean }) {
  const { data, isLoading, error } = useDatabaseCredentials(databaseId);
  const { data: connectivity } = useConnectivity(databaseId);
  const rotate = useRotateCredential(databaseId);
  const revoke = useRevokeCredential(databaseId);

  const [creating, setCreating] = useState(false);
  const [reveal, setReveal] = useState<CredentialCreateResult | null>(null);
  const [pendingRotate, setPendingRotate] = useState<DatabaseCredential | null>(null);
  const [pendingRevoke, setPendingRevoke] = useState<DatabaseCredential | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const publicActive = connectivity?.public?.status === "active";

  async function onRotate() {
    if (!pendingRotate) return;
    setActionError(null);
    try {
      const res = await rotate.mutateAsync(pendingRotate.id);
      setPendingRotate(null);
      setReveal(res);
    } catch (e) {
      setActionError(e instanceof ApiError ? e.message : "Failed to rotate credential");
      setPendingRotate(null);
    }
  }

  async function onRevoke() {
    if (!pendingRevoke) return;
    setActionError(null);
    try {
      await revoke.mutateAsync(pendingRevoke.id);
      setPendingRevoke(null);
    } catch (e) {
      setActionError(e instanceof ApiError ? e.message : "Failed to revoke credential");
      setPendingRevoke(null);
    }
  }

  return (
    <section style={{ marginBottom: "1.25rem" }}>
      <div className="flex items-center justify-between" style={{ marginBottom: ".75rem" }}>
        <h2 className="text-lg font-semibold flex items-center gap-2">
          <KeyRound size={18} /> Application credentials
        </h2>
        {canWrite ? (
          <button className="btn btn-sm btn-primary" onClick={() => setCreating(true)}>
            <Plus size={14} /> Create credential
          </button>
        ) : null}
      </div>

      <ErrorText message={actionError ?? undefined} />

      <DataTable<DatabaseCredential>
        columns={[
          {
            id: "name",
            header: "Name",
            render: (c) => (
              <div>
                <div className="font-medium">{c.name}</div>
                <div className="muted text-sm">{c.username}</div>
              </div>
            ),
          },
          { id: "access", header: "Access", render: (c) => c.access_level },
          {
            id: "status",
            header: "Status",
            render: (c) =>
              c.revoked_at ? (
                <span className="muted">Revoked {new Date(c.revoked_at).toLocaleDateString()}</span>
              ) : (
                <span>Active</span>
              ),
          },
          { id: "created", header: "Created", render: (c) => new Date(c.created_at).toLocaleString() },
          {
            id: "actions",
            header: "",
            align: "right",
            render: (c) =>
              canWrite && !c.revoked_at ? (
                <div className="flex gap-2 justify-end">
                  <button className="btn btn-sm" onClick={() => setPendingRotate(c)}>
                    <RotateCw size={14} /> Rotate
                  </button>
                  <button className="btn btn-sm btn-danger" onClick={() => setPendingRevoke(c)}>
                    <Trash2 size={14} /> Revoke
                  </button>
                </div>
              ) : null,
          },
        ]}
        rows={data?.items ?? []}
        rowKey={(c) => c.id}
        isLoading={isLoading}
        error={error instanceof ApiError ? error.message : error ? "Could not load credentials." : undefined}
        emptyTitle="No application credentials"
        emptyHint="Create credentials for apps connecting to this database."
      />

      {creating ? (
        <CreateCredentialModal
          databaseId={databaseId}
          publicActive={publicActive}
          onClose={() => setCreating(false)}
          onCreated={(res) => {
            setCreating(false);
            setReveal(res);
          }}
        />
      ) : null}

      <ConfirmModal
        open={Boolean(pendingRotate)}
        busy={rotate.isPending}
        title="Rotate credential"
        confirmLabel="Rotate"
        message={
          <>
            A new password is generated for <strong>{pendingRotate?.name}</strong> and the current one stops working
            immediately. The new password is shown once.
          </>
        }
        onConfirm={onRotate}
        onCancel={() => setPendingRotate(null)}
      />

      <ConfirmModal
        open={Boolean(pendingRevoke)}
        danger
        busy={revoke.isPending}
        title="Revoke credential"
        confirmLabel="Revoke"
        message={
          <>
            The database user for <strong>{pendingRevoke?.name}</strong> is dropped. Anything using it stops connecting.
          </>
        }
        onConfirm={onRevoke}
        onCancel={() => setPendingRevoke(null)}
      />

      {reveal ? (
        <SecretReveal
          open
          onClose={() => setReveal(null)}
          title="Credential ready"
          items={[
            { label: "Connection URL", value: reveal.connection_url },
            { label: "CLI command", value: reveal.cli_command ?? "" },
            { label: "Username", value: reveal.credential.username },
            { label: "Password", value: reveal.password },
            { label: "Host", value: `${reveal.fields.host}:${reveal.fields.port}` },
            { label: "Database", value: reveal.fields.database },
          ]}
        />
      ) : null}
    </section>
  );
}

function CreateCredentialModal({
  databaseId,
  publicActive,
  onClose,
  onCreated,
}: {
  databaseId: string;
  publicActive: boolean;
  onClose: () => void;
  onCreated: (result: CredentialCreateResult) => void;
}) {
  const create = useCreateCredential(databaseId);
  const [name, setName] = useState("");
  const [username, setUsername] = useState("");
  const [accessLevel, setAccessLevel] = useState("readwrite");
  const [usePublic, setUsePublic] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      const res = await create.mutateAsync({
        name,
        access_level: accessLevel,
        use_public: usePublic,
        // Omitted entirely when blank so the server generates one.
        ...(username.trim() ? { username: username.trim() } : {}),
      });
      onCreated(res);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Failed to create credential");
    }
  }

  return (
    <Modal open onClose={onClose} title="Create application credential">
      <form onSubmit={onSubmit}>
        <Field label="Name">
          <input className="input" required value={name} onChange={(e) => setName(e.target.value)} />
        </Field>
        <Field label="Database username (optional)">
          <input
            className="input"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            placeholder="generated from the name"
          />
        </Field>
        <Field label="Permission profile">
          <select className="input" value={accessLevel} onChange={(e) => setAccessLevel(e.target.value)}>
            <option value="readonly">Read-only</option>
            <option value="readwrite">Read / Write</option>
            <option value="admin">Admin</option>
          </select>
        </Field>

        <label
          className="flex items-center gap-2 text-sm"
          style={{ margin: ".5rem 0", opacity: publicActive ? 1 : 0.6 }}
        >
          <input
            type="checkbox"
            checked={usePublic}
            disabled={!publicActive}
            onChange={(e) => setUsePublic(e.target.checked)}
          />
          Use the public endpoint in the connection URL
        </label>
        {!publicActive ? (
          <p className="muted text-sm" style={{ marginTop: "-.25rem", marginBottom: ".5rem" }}>
            Available once public access is enabled and active.
          </p>
        ) : null}

        <ErrorText message={error ?? undefined} />

        <button className="btn btn-primary" style={{ marginTop: ".5rem" }} disabled={create.isPending}>
          {create.isPending ? "Creating…" : "Create"}
        </button>
      </form>
    </Modal>
  );
}
