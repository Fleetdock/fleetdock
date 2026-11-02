"use client";

import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useEffect, useState, type FormEvent } from "react";

import { Activity, Ban, Database, HeartPulse, KeyRound, Pencil, Plus, Trash2 } from "lucide-react";
import { DataTable } from "@/components/data-table";
import { MetricChart, type ChartPoint } from "@/components/chart";
import {
  Button,
  Callout,
  DetailSkeleton,
  ErrorText,
  Field,
  KeyValueList,
  Menu,
  Modal,
  PageHeader,
  QueryError,
  SectionHeader,
  SegmentedControl,
  Skeleton,
  StatTile,
  StatusBadge,
  Time,
} from "@/components/ui";
import { AgentUpdateNotice } from "@/components/agent-update";
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
  const { data: server, isLoading, error, refetch } = useServer(id);
  const { data: instances, isLoading: instancesLoading } = useInstances(id);
  const [open, setOpen] = useState(false);
  const [renameOpen, setRenameOpen] = useState(false);
  const can = useCan();

  if (isLoading) {
    return <DetailSkeleton label="Loading server" />;
  }
  if (!server) {
    return (
      <QueryError
        error={error}
        what="server"
        onRetry={() => void refetch()}
        backHref="/servers"
        backLabel="Back to servers"
      />
    );
  }

  const instanceCount = instances?.items.length ?? 0;
  const addButton = can("instance:write") ? (
    <Button variant="primary" icon={<Plus size={16} aria-hidden />} onClick={() => setOpen(true)}>
      Add database server
    </Button>
  ) : undefined;

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
            {addButton}
          </>
        }
      />

      <AgentUpdateNotice server={server} />

      <div className="stat-strip">
        <StatTile
          label="Database servers"
          icon={<Database size={13} aria-hidden />}
          value={instancesLoading ? "—" : instanceCount}
          hint="On this machine"
        />
        <StatTile
          label="Last heartbeat"
          icon={<HeartPulse size={13} aria-hidden />}
          value={server.last_heartbeat_at ? <Time value={server.last_heartbeat_at} /> : "Never"}
          hintTone={server.status === "offline" ? "danger" : undefined}
          hint={server.status === "offline" ? "No heartbeat for over two minutes" : undefined}
        />
        <StatTile
          label="Agent"
          icon={<Activity size={13} aria-hidden />}
          value={server.agent_version ?? "—"}
          hint={
            !server.agent_version
              ? "Not connected yet"
              : server.agent_status === "outdated"
                ? "Update available"
                : server.agent_status === "unsupported"
                  ? "Too old: update it"
                  : "Installed version"
          }
          hintTone={server.agent_status === "unsupported" ? "danger" : undefined}
        />
      </div>

      <div className="detail-grid">
        <div className="detail-main">
          <ServerMetrics id={id} />

          <section className="section">
            <SectionHeader
              title="Database servers on this machine"
              icon={<Database size={17} aria-hidden />}
            />
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
                { id: "port", header: "Port", align: "right", className: "muted num", hideOnMobile: true, render: (i) => i.port },
                { id: "status", header: "Status", render: (i) => <StatusBadge status={i.status} /> },
              ]}
              rows={instances?.items ?? []}
              rowKey={(i) => i.id}
              isLoading={instancesLoading}
              emptyTitle="No database servers on this machine yet"
              emptyHint="Let Fleetdock create one here, or tell it about one that is already running."
              emptyAction={addButton}
            />
          </section>
        </div>

        <aside className="detail-aside" aria-label="Server details">
          <div className="card panel aside-card">
            <h2 className="panel-title">Details</h2>
            <KeyValueList
              items={[
                { label: "Hostname", value: server.hostname, mono: true, copy: server.hostname },
                { label: "Address", value: server.address ?? "—", mono: !!server.address, copy: server.address ?? undefined },
                { label: "Tags", value: server.tags.length ? server.tags.join(", ") : "—" },
                { label: "Connected", value: <Time value={server.created_at} /> },
              ]}
            />
          </div>
        </aside>
      </div>

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
      <Button icon={<Pencil size={15} aria-hidden />} onClick={onRename} aria-label="Rename server">
        Rename
      </Button>
      <Button
        variant="danger"
        icon={<Trash2 size={15} aria-hidden />}
        disabled={busy || blocked}
        onClick={onDelete}
        title={
          blocked
            ? `Remove the ${instanceCount} database server${instanceCount === 1 ? "" : "s"} on this machine first`
            : undefined
        }
      >
        Remove
      </Button>
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
            <Callout tone="info">
              The old agent token no longer works. This command is shown once and expires in 24 hours.
            </Callout>
            <InstallCommand command={reenroll} />
            <div className="modal-actions">
              <Button variant="primary" onClick={() => setReenroll(null)}>
                Done
              </Button>
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
        <Field label="Name" hint="Lowercase letters, digits, hyphens and underscores (2–63 characters).">
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
        <div className="modal-actions">
          <Button onClick={onClose}>Cancel</Button>
          <Button type="submit" variant="primary" loading={update.isPending}>
            {update.isPending ? "Saving…" : "Save"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

type Range = "1" | "6" | "24" | "168";

const RANGES: { value: Range; label: string }[] = [
  { value: "1", label: "1h" },
  { value: "6", label: "6h" },
  { value: "24", label: "24h" },
  { value: "168", label: "7d" },
];

function ServerMetrics({ id }: { id: string }) {
  const [range, setRange] = useState<Range>("6");
  const { data, isLoading } = useServerMetrics(id, Number(range));
  const samples = data?.items ?? [];

  const cpu: ChartPoint[] = samples.map((s) => ({ t: s.collected_at, v: s.cpu_pct ?? null }));
  const mem: ChartPoint[] = samples.map((s) => ({ t: s.collected_at, v: pct(s.mem_used_bytes, s.mem_total_bytes) }));
  const disk: ChartPoint[] = samples.map((s) => ({ t: s.collected_at, v: pct(s.disk_used_bytes, s.disk_total_bytes) }));
  const conns: ChartPoint[] = samples.map((s) => ({ t: s.collected_at, v: s.active_connections ?? null }));

  return (
    <section className="section">
      <SectionHeader
        title="Metrics"
        icon={<Activity size={17} aria-hidden />}
        actions={<SegmentedControl options={RANGES} value={range} onChange={setRange} label="Time range" size="sm" />}
      />
      {isLoading && samples.length === 0 ? (
        <div className="chart-grid-layout" role="status" aria-label="Loading metrics">
          {[0, 1, 2, 3].map((i) => (
            <Skeleton key={i} height="10.5rem" radius="var(--radius)" />
          ))}
        </div>
      ) : (
        <div className="chart-grid-layout">
          <MetricChart title="CPU" points={cpu} unit="%" max={100} color="var(--chart-1)" />
          <MetricChart title="Memory used" points={mem} unit="%" max={100} color="var(--chart-2)" />
          <MetricChart title="Disk used" points={disk} unit="%" max={100} color="var(--chart-3)" />
          <MetricChart title="Connections" points={conns} color="var(--chart-4)" />
        </div>
      )}
    </section>
  );
}

function pct(used?: number | null, total?: number | null): number | null {
  if (used == null || total == null || total === 0) return null;
  return (used / total) * 100;
}
