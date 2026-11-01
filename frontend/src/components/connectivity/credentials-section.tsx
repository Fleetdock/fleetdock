"use client";

import { useState } from "react";
import { KeyRound, Plus, RotateCw, Trash2 } from "lucide-react";

import { DataTable } from "@/components/data-table";
import {
  Badge,
  Button,
  Checkbox,
  ConfirmModal,
  ErrorText,
  Field,
  Modal,
  SecretReveal,
  SectionHeader,
  StatusBadge,
  Time,
} from "@/components/ui";
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
    <section className="section">
      <SectionHeader
        title="Application credentials"
        icon={<KeyRound size={17} />}
        description="Scoped logins for apps that connect to this database. Passwords are shown once."
        actions={
          canWrite ? (
            <Button variant="primary" size="sm" icon={<Plus size={14} aria-hidden />} onClick={() => setCreating(true)}>
              Create credential
            </Button>
          ) : null
        }
      />

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
          { id: "access", header: "Access", render: (c) => <Badge>{c.access_level}</Badge> },
          {
            id: "status",
            header: "Status",
            render: (c) =>
              c.revoked_at ? (
                <span className="row">
                  <StatusBadge status="revoked" />
                  <span className="muted text-sm">
                    <Time value={c.revoked_at} />
                  </span>
                </span>
              ) : (
                <StatusBadge status="active" />
              ),
          },
          { id: "created", header: "Created", className: "muted", hideOnMobile: true, render: (c) => <Time value={c.created_at} /> },
          {
            id: "actions",
            header: "",
            align: "right",
            render: (c) =>
              canWrite && !c.revoked_at ? (
                <div className="flex justify-end gap-2">
                  <Button size="sm" icon={<RotateCw size={14} aria-hidden />} onClick={() => setPendingRotate(c)}>
                    Rotate
                  </Button>
                  <Button
                    variant="danger"
                    size="sm"
                    icon={<Trash2 size={14} aria-hidden />}
                    onClick={() => setPendingRevoke(c)}
                  >
                    Revoke
                  </Button>
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
        emptyAction={
          canWrite ? (
            <Button variant="primary" icon={<Plus size={14} aria-hidden />} onClick={() => setCreating(true)}>
              Create credential
            </Button>
          ) : undefined
        }
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

        <Checkbox
          className={publicActive ? undefined : "opacity-60"}
          checked={usePublic}
          disabled={!publicActive}
          onChange={(e) => setUsePublic(e.target.checked)}
          label="Use the public endpoint in the connection URL"
          hint={publicActive ? undefined : "Available once public access is enabled and active."}
        />

        <ErrorText message={error ?? undefined} />

        <div className="modal-actions">
          <Button onClick={onClose}>Cancel</Button>
          <Button type="submit" variant="primary" loading={create.isPending}>
            {create.isPending ? "Creating…" : "Create"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
