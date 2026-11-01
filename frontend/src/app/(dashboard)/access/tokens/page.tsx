"use client";

import { Key, Plus, Trash2 } from "lucide-react";
import { useState } from "react";

import { CreateTokenModal } from "@/components/access/token-modal";
import { useConfirm } from "@/components/confirm";
import { DataTable } from "@/components/data-table";
import { useErrorToast } from "@/components/toast";
import { Badge, Button, IconButton, PageHeader, SecretReveal, Time } from "@/components/ui";
import { friendlyError } from "@/lib/errors";
import { useCan, useRevokeToken, useTokens } from "@/lib/hooks";
import type { ApiToken } from "@/lib/types";
import { useDataTable } from "@/lib/use-data-table";

export default function TokensPage() {
  const { data, isLoading, error, refetch } = useTokens();
  const revoke = useRevokeToken();
  const toastError = useErrorToast();
  const confirm = useConfirm();
  const [open, setOpen] = useState(false);
  const [secret, setSecret] = useState<string | null>(null);
  const can = useCan();
  const canWrite = can("token:write");
  const table = useDataTable({ items: data?.items });

  const newButton = canWrite ? (
    <Button variant="primary" icon={<Plus size={16} aria-hidden />} onClick={() => setOpen(true)}>
      New token
    </Button>
  ) : null;

  return (
    <div>
      <PageHeader
        title="API tokens"
        description="Keys for scripts and automation. A token can do at most what you can."
        actions={newButton}
      />

      <DataTable<ApiToken>
        columns={[
          {
            id: "name",
            header: "Name",
            className: "font-medium",
            render: (t) => (
              <span className="inline-flex items-center gap-2">
                <Key size={15} aria-hidden className="text-muted" /> {t.name}
              </span>
            ),
          },
          {
            id: "prefix",
            header: "Prefix",
            className: "muted",
            render: (t) => <code className="mono">{t.prefix}…</code>,
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
            render: (t) => <Time value={t.created_at} />,
          },
          {
            id: "last_used",
            header: "Last used",
            className: "muted",
            render: (t) => (t.last_used_at ? <Time value={t.last_used_at} /> : "never"),
          },
          {
            id: "actions",
            header: "",
            align: "right",
            render: (t) =>
              t.revoked_at ? (
                <Badge tone="danger" dot>
                  revoked
                </Badge>
              ) : canWrite ? (
                <IconButton
                  label={`Revoke ${t.name}`}
                  variant="danger"
                  onClick={async () => {
                    const ok = await confirm({
                      title: `Revoke "${t.name}"?`,
                      message: "Anything using this token stops working immediately.",
                      confirmLabel: "Revoke token",
                      danger: true,
                    });
                    if (ok) revoke.mutate(t.id, { onError: toastError("Failed to revoke the token") });
                  }}
                >
                  <Trash2 size={15} aria-hidden />
                </IconButton>
              ) : null,
          },
        ]}
        rows={table.rows}
        rowKey={(t) => t.id}
        isLoading={isLoading}
        error={error ? friendlyError(error) : undefined}
        errorTitle="Could not load tokens"
        onRetry={() => void refetch()}
        emptyTitle="No tokens yet"
        emptyHint="Create a token to call the API from scripts or CI."
        emptyAction={newButton}
        pagination={{ page: table.page, pageCount: table.pageCount, onPage: table.setPage }}
      />

      <CreateTokenModal open={open} onClose={() => setOpen(false)} onCreated={setSecret} />

      <SecretReveal
        open={secret !== null}
        onClose={() => setSecret(null)}
        title="Token created"
        hint="Copy this token now — it will not be shown again."
        items={[{ label: "Token", value: secret ?? "" }]}
      />
    </div>
  );
}
