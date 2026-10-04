"use client";

import { useState, type FormEvent } from "react";

import { DataTable, type DataTableColumn } from "@/components/data-table";

import { ErrorText, Field, Modal } from "@/components/ui";

import { ApiError } from "@/lib/api";
import {
  useDatabaseDBUsers,
  useDBPrivileges,
  useGrantOnDatabase,
  useRevokeOnDatabase,
  useSchemaGrants,
} from "@/lib/hooks";
import type { SchemaGrant } from "@/lib/types";

import { Plus } from "lucide-react";
import { useConfirm } from "@/components/confirm";
import { useToast } from "@/components/toast";
import { friendlyError } from "@/lib/errors";

// ---- Users & grants for this database ----

export function GrantsSection({
  databaseId,
  canWrite,
}: {
  databaseId: string;
  canWrite: boolean;
}) {
  const { data, isLoading, error } = useSchemaGrants(databaseId);
  const revoke = useRevokeOnDatabase();
  const [grantOpen, setGrantOpen] = useState(false);
  const confirm = useConfirm();
  const { push } = useToast();

  async function onRevoke(user: string, host: string) {
    const ok = await confirm({
      title: `Remove ${user}'s access?`,
      message: (
        <>
          <code>
            {user}@{host}
          </code>{" "}
          loses every privilege on this database. The user account itself is kept.
        </>
      ),
      confirmLabel: "Remove access",
      danger: true,
    });
    if (!ok) return;
    try {
      await revoke.mutateAsync({ databaseId, username: user, host });
      push("success", `${user} no longer has access`);
    } catch (err) {
      push("error", friendlyError(err, "Failed to remove access"));
    }
  }

  const columns: DataTableColumn<SchemaGrant>[] = [
    {
      id: "user",
      header: "User",
      className: "font-medium",
      render: (g) => g.user,
    },
    {
      id: "host",
      header: "Host",
      className: "muted",
      render: (g) => <code>{g.host}</code>,
    },
    {
      id: "privileges",
      header: "Privileges",
      render: (g) => (
        <div className="flex" style={{ flexWrap: "wrap", gap: ".25rem" }}>
          {g.privileges.map((p) => (
            <span key={p} className="badge badge-gray" style={{ fontSize: 11 }}>
              {p}
            </span>
          ))}
        </div>
      ),
    },
  ];
  if (canWrite) {
    columns.push({
      id: "actions",
      header: "Actions",
      align: "right",
      render: (g) => (
        <button
          className="btn btn-sm btn-danger"
          onClick={() => onRevoke(g.user, g.host)}
          disabled={revoke.isPending}
        >
          Revoke all
        </button>
      ),
    });
  }

  return (
    <div>
      <div
        className="flex justify-between items-center"
        style={{ marginBottom: ".6rem" }}
      >
        <p className="text-sm muted">
          Accounts with privileges on this database (live).
        </p>
        {canWrite ? (
          <button
            className="btn btn-primary btn-sm"
            onClick={() => setGrantOpen(true)}
          >
            <Plus size={15} /> Grant access
          </button>
        ) : null}
      </div>


      <DataTable<SchemaGrant>
        columns={columns}
        rows={data?.items ?? []}
        rowKey={(g) => `${g.user}@${g.host}`}
        isLoading={isLoading}
        loadingLabel="Connecting to the database server…"
        error={error ? (error as ApiError).message : undefined}
        errorTitle="Could not reach the database server"
        emptyTitle="No accounts have privileges on this database"
        emptyHint='Use "Grant access" to give a database user access.'
      />

      <GrantAccessModal
        open={grantOpen}
        onClose={() => setGrantOpen(false)}
        databaseId={databaseId}
      />
    </div>
  );
}

function GrantAccessModal({
  open,
  onClose,
  databaseId,
}: {
  open: boolean;
  onClose: () => void;
  databaseId: string;
}) {
  const grant = useGrantOnDatabase();
  const { data: users } = useDatabaseDBUsers(databaseId);
  const { data: privileges } = useDBPrivileges();
  const [account, setAccount] = useState("");
  const [selected, setSelected] = useState<Set<string>>(
    new Set(["ALL PRIVILEGES"]),
  );
  const [error, setError] = useState<string | null>(null);

  function toggle(p: string) {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(p)) {
        next.delete(p);
      } else {
        next.add(p);
      }
      return next;
    });
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    const [user, host] = account.split(" ");
    try {
      await grant.mutateAsync({
        databaseId,
        username: user,
        host,
        privileges: [...selected],
      });
      onClose();
    } catch (err) {
      setError(
        err instanceof ApiError ? err.message : "Failed to grant access",
      );
    }
  }

  return (
    <Modal open={open} onClose={onClose} title="Grant access to this database">
      <form onSubmit={onSubmit}>
        <Field label="Database user">
          <select
            className="input"
            value={account}
            onChange={(e) => setAccount(e.target.value)}
            required
          >
            <option value="" disabled>
              Select an account…
            </option>
            {(users?.items ?? []).map((u) => (
              <option key={`${u.user}@${u.host}`} value={`${u.user} ${u.host}`}>
                {u.user}@{u.host}
              </option>
            ))}
          </select>
        </Field>
        <Field label="Privileges">
          <div
            className="card"
            style={{ padding: ".75rem", maxHeight: 220, overflowY: "auto" }}
          >
            <div
              className="flex"
              style={{ flexWrap: "wrap", gap: ".3rem 1rem" }}
            >
              {(privileges?.items ?? []).map((p) => (
                <label
                  key={p}
                  className="flex items-center gap-1 text-sm"
                  style={{ cursor: "pointer" }}
                >
                  <input
                    type="checkbox"
                    checked={selected.has(p)}
                    onChange={() => toggle(p)}
                  />
                  {p}
                </label>
              ))}
            </div>
          </div>
        </Field>
        <p className="text-sm muted">
          Create new accounts on the instance detail page; here you grant
          existing accounts access.
        </p>
        <ErrorText message={error ?? undefined} />
        <div
          className="flex justify-end items-center gap-2"
          style={{ marginTop: ".5rem" }}
        >
          <button type="button" className="btn" onClick={onClose}>
            Cancel
          </button>
          <button
            type="submit"
            className="btn btn-primary"
            disabled={grant.isPending || selected.size === 0 || !account}
          >
            {grant.isPending ? "Granting…" : "Grant"}
          </button>
        </div>
      </form>
    </Modal>
  );
}
