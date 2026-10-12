"use client";

import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useEffect, useState, type FormEvent } from "react";

import { Ban, KeyRound, Pencil, Plus, Trash2 } from "lucide-react";
import { DataTable } from "@/components/data-table";
import { MetricChart, type ChartPoint } from "@/components/chart";
import { EmptyState, ErrorText, Field, Menu, Modal, PageHeader, Spinner, StatusBadge, Time } from "@/components/ui";
import { InstallCommand } from "@/components/install-command";
import { AddServerWizard } from "@/components/database/add-server-wizard";
import { engineLabel } from "@/lib/engines";
import { ApiError } from "@/lib/api";
import {
  useCan,
  useDeleteServer,
  useInstances,
  useRevokeAgentToken,
  useRotateAgentToken,
  useServer,
  useServerMetrics,
  useUpdateServer,
} from "@/lib/hooks";
import type { Server } from "@/lib/types";
import { useConfirm } from "@/components/confirm";
import { useToast } from "@/components/toast";
import { friendlyError } from "@/lib/errors";

export default function ServerDetailPage() {
  const params = useParams();
  const id = String(params.id);
  const { data: server, isLoading } = useServer(id);
  const { data: instances } = useInstances(id);
  const [open, setOpen] = useState(false);
  const [renameOpen, setRenameOpen] = useState(false);
  const can = useCan();

  if (isLoading) {
    return <div className="flex items-center gap-2 muted text-sm"><Spinner /> Loading…</div>;
  }
  if (!server) {
    return <EmptyState title="Server not found" />;
  }

  const instanceCount = instances?.items.length ?? 0;

  return (
    <div>
      <PageHeader
        breadcrumbs={[{ label: "Servers", href: "/servers" }, { label: server.name }]}
        title={server.name}
        badges={<StatusBadge status={server.status} />}
        description={server.hostname}
        actions={
          <>
            {can("server:write") ? (
              <ServerControls server={server} instanceCount={instanceCount} onRename={() => setRenameOpen(true)} />
            ) : null}
            {can("instance:write") ? (
              <button className="btn btn-primary" onClick={() => setOpen(true)}>
                <Plus size={16} /> Add database server
              </button>
            ) : null}
          </>
        }
      />

      <div className="card" style={{ padding: "1.1rem", marginBottom: "1.25rem" }}>
        <dl className="grid" style={{ gridTemplateColumns: "repeat(auto-fit, minmax(180px, 1fr))", gap: "1rem" }}>
          <Detail label="Hostname" value={server.hostname} />
          <Detail label="Address" value={server.address ?? "—"} />
          <Detail label="Agent" value={server.agent_version ?? "—"} />
          <Detail label="Tags" value={server.tags.length ? server.tags.join(", ") : "—"} />
          <Detail label="Connected" value={<Time value={server.created_at} />} />
        </dl>
      </div>

      <ServerMetrics id={id} />

      <h2 className="section-title">Database servers on this machine</h2>
      <DataTable
        columns={[
          {
            id: "name",
            header: "Name",
            className: "font-medium",
            render: (i) => (
              <Link href={`/instances/${i.id}`} className="link-plain">
                {i.name}
              </Link>
            ),
          },
          { id: "engine", header: "Engine", className: "muted", render: (i) => `${engineLabel(i.engine)} ${i.engine_version}` },
          { id: "port", header: "Port", className: "muted", hideOnMobile: true, render: (i) => i.port },
          { id: "status", header: "Status", render: (i) => <StatusBadge status={i.status} /> },
        ]}
        rows={instances?.items ?? []}
        rowKey={(i) => i.id}
        emptyTitle="No database servers on this machine yet"
        emptyHint="Let Fleetdock create one here, or tell it about one that is already running."
        emptyAction={
          can("instance:write") ? (
            <button className="btn btn-primary" onClick={() => setOpen(true)}>
              <Plus size={16} /> Add database server
            </button>
          ) : undefined
        }
      />

      <AddServerWizard open={open} onClose={() => setOpen(false)} serverId={id} />
      <RenameServerModal open={renameOpen} onClose={() => setRenameOpen(false)} server={server} />
    </div>
  );
}

function ServerControls({
  server,
  instanceCount,
  onRename,
}: {
  server: Server;
  instanceCount: number;
  onRename: () => void;
}) {
  const router = useRouter();
  const del = useDeleteServer();
  const revoke = useRevokeAgentToken();
  const rotate = useRotateAgentToken();
  const confirm = useConfirm();
  const { push } = useToast();
  const [reenroll, setReenroll] = useState<string | null>(null);
  const busy = del.isPending || revoke.isPending || rotate.isPending;
  const blocked = instanceCount > 0;

  async function onRotate() {
    const ok = await confirm({
      title: `Rotate the agent token for ${server.name}?`,
      message:
        "The agent's current token stops working immediately. You get a one-time install command; run it on the machine to re-enroll this server with a fresh token. Operations wait until it reconnects.",
      confirmLabel: "Rotate token",
      danger: true,
    });
    if (!ok) return;
    try {
      const res = await rotate.mutateAsync(server.id);
      setReenroll(res.install_command);
    } catch (err) {
      push("error", friendlyError(err, "Failed to rotate the agent token"));
    }
  }

  async function onRevoke() {
    const ok = await confirm({
      title: `Revoke the agent token for ${server.name}?`,
      message:
        "The agent is cut off immediately and can no longer report in or run operations. Use this if the machine or its token may be compromised. To reconnect it later, rotate the token and run the install command.",
      confirmLabel: "Revoke token",
      danger: true,
    });
    if (!ok) return;
    try {
      await revoke.mutateAsync(server.id);
      push("success", `Agent token for ${server.name} revoked`);
    } catch (err) {
      push("error", friendlyError(err, "Failed to revoke the agent token"));
    }
  }

  async function onDelete() {
    const ok = await confirm({
      title: `Remove ${server.name}?`,
      message:
        "Fleetdock stops managing this machine and its agent can no longer connect. Nothing on the machine itself is deleted.",
      confirmLabel: "Remove server",
      danger: true,
    });
    if (!ok) return;
    try {
      await del.mutateAsync(server.id);
      push("success", `${server.name} removed`);
      router.push("/servers");
    } catch (err) {
      push("error", friendlyError(err, "Failed to remove the server"));
    }
  }

  return (
    <>
      <button className="btn btn-sm" onClick={onRename} aria-label="Rename server">
        <Pencil size={15} /> Rename
      </button>
      <button
        className="btn btn-sm btn-danger"
        disabled={busy || blocked}
        onClick={onDelete}
        title={
          blocked
            ? `Remove the ${instanceCount} database server${instanceCount === 1 ? "" : "s"} on this machine first`
            : undefined
        }
      >
        <Trash2 size={15} /> Remove
      </button>
      <Menu
        label="Agent token actions"
        items={[
          { label: "Rotate agent token", icon: <KeyRound size={15} />, onSelect: onRotate, disabled: busy },
          { label: "Revoke agent token", icon: <Ban size={15} />, onSelect: onRevoke, danger: true, disabled: busy },
        ]}
      />
      <Modal open={reenroll !== null} onClose={() => setReenroll(null)} title={`Re-enroll ${server.name}`}>
        {reenroll ? (
          <>
            <p className="callout callout-info">
              The old agent token no longer works. This command is shown once and expires in 24 hours.
            </p>
            <InstallCommand command={reenroll} />
            <div className="flex justify-end" style={{ marginTop: ".9rem" }}>
              <button className="btn btn-primary" onClick={() => setReenroll(null)}>Done</button>
            </div>
          </>
        ) : null}
      </Modal>
    </>
  );
}

function RenameServerModal({
  open,
  onClose,
  server,
}: {
  open: boolean;
  onClose: () => void;
  server: Server;
}) {
  const update = useUpdateServer();
  const [name, setName] = useState(server.name);
  const [tags, setTags] = useState(server.tags.join(", "));
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    setName(server.name);
    setTags(server.tags.join(", "));
    setError(null);
  }, [open, server]);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    const parsedTags = tags
      .split(",")
      .map((t) => t.trim())
      .filter(Boolean);
    try {
      await update.mutateAsync({
        id: server.id,
        name: name.trim(),
        tags: parsedTags,
      });
      onClose();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to update server");
    }
  }

  return (
    <Modal open={open} onClose={onClose} title="Rename server">
      <form onSubmit={onSubmit}>
        <Field label="Name">
          <input
            className="input"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="my-server"
            required
            pattern="[a-z0-9_-]{2,63}"
            title="Lowercase letters, digits, hyphens and underscores (2–63 characters)"
          />
        </Field>
        <Field label="Tags (comma-separated)">
          <input className="input" value={tags} onChange={(e) => setTags(e.target.value)} placeholder="prod, eu-west" />
        </Field>
        <ErrorText message={error ?? undefined} />
        <div className="flex items-center justify-end gap-2" style={{ marginTop: ".5rem" }}>
          <button type="button" className="btn" onClick={onClose}>Cancel</button>
          <button type="submit" className="btn btn-primary" disabled={update.isPending}>
            {update.isPending ? "Saving…" : "Save"}
          </button>
        </div>
      </form>
    </Modal>
  );
}

function Detail({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div>
      <dt className="muted text-sm">{label}</dt>
      <dd className="font-medium" style={{ margin: 0 }}>{value}</dd>
    </div>
  );
}

const RANGES = [
  { label: "1h", hours: 1 },
  { label: "6h", hours: 6 },
  { label: "24h", hours: 24 },
  { label: "7d", hours: 168 },
];

function ServerMetrics({ id }: { id: string }) {
  const [hours, setHours] = useState(6);
  const { data, isLoading } = useServerMetrics(id, hours);
  const samples = data?.items ?? [];

  const cpu: ChartPoint[] = samples.map((s) => ({ t: s.collected_at, v: s.cpu_pct ?? null }));
  const mem: ChartPoint[] = samples.map((s) => ({ t: s.collected_at, v: pct(s.mem_used_bytes, s.mem_total_bytes) }));
  const disk: ChartPoint[] = samples.map((s) => ({ t: s.collected_at, v: pct(s.disk_used_bytes, s.disk_total_bytes) }));
  const conns: ChartPoint[] = samples.map((s) => ({ t: s.collected_at, v: s.active_connections ?? null }));

  return (
    <section style={{ marginBottom: "1.5rem" }}>
      <div className="flex items-center justify-between" style={{ marginBottom: ".6rem" }}>
        <h2 className="font-semibold">Metrics</h2>
        <div className="flex items-center gap-1">
          {RANGES.map((r) => (
            <button
              key={r.hours}
              className={`btn btn-sm${hours === r.hours ? " btn-primary" : ""}`}
              onClick={() => setHours(r.hours)}
            >
              {r.label}
            </button>
          ))}
        </div>
      </div>
      {isLoading && samples.length === 0 ? (
        <div className="flex items-center gap-2 muted text-sm"><Spinner /> Loading metrics…</div>
      ) : (
        <div className="grid" style={{ gridTemplateColumns: "repeat(auto-fit, minmax(240px, 1fr))", gap: ".9rem" }}>
          <MetricChart title="CPU" points={cpu} unit="%" max={100} color="var(--accent)" />
          <MetricChart title="Memory used" points={mem} unit="%" max={100} color="#22c55e" />
          <MetricChart title="Disk used" points={disk} unit="%" max={100} color="#f59e0b" />
          <MetricChart title="Connections" points={conns} color="#8b5cf6" />
        </div>
      )}
    </section>
  );
}

function pct(used?: number | null, total?: number | null): number | null {
  if (used == null || total == null || total === 0) return null;
  return (used / total) * 100;
}
