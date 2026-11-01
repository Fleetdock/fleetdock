"use client";

import { useId, useState } from "react";

import { CircleStop, Search, XCircle } from "lucide-react";
import { TopQueriesView } from "@/components/top-queries";
import {
  Badge,
  Button,
  Callout,
  Checkbox,
  ConfirmModal,
  EmptyState,
  ErrorText,
  SearchInput,
  SectionHeader,
  Table,
  TableSkeleton,
  Tabs,
  Toolbar,
  type Tone,
} from "@/components/ui";
import { ApiError } from "@/lib/api";
import { friendlyError } from "@/lib/errors";
import { useKillProcess, useProcesses, useServerStatus, useServerVariables } from "@/lib/hooks";
import type { InstanceHealth, Process, Setting } from "@/lib/types";

type Tab = "sessions" | "queries" | "status" | "variables";

const TABS: { id: Tab; label: string }[] = [
  { id: "sessions", label: "Sessions" },
  { id: "queries", label: "Top queries" },
  { id: "status", label: "Status" },
  { id: "variables", label: "Variables" },
];

/** HealthBadge shows the latest result of the control plane's periodic probe. */
export function HealthBadge({ health }: { health?: InstanceHealth | null }) {
  if (!health) {
    return (
      <Badge tone="neutral" title="Not probed yet">
        health: —
      </Badge>
    );
  }
  const tone: Tone = health.status === "healthy" ? "success" : health.status === "unreachable" ? "danger" : "neutral";
  const when = new Date(health.checked_at).toLocaleTimeString();
  const title =
    health.status === "healthy"
      ? `${health.version ?? ""} · ${health.latency_ms} ms · checked ${when}`
      : `${health.error ?? health.status} · checked ${when}`;
  return (
    <Badge tone={tone} dot title={title}>
      {health.status === "healthy" ? `healthy · ${health.latency_ms} ms` : health.status}
    </Badge>
  );
}

/**
 * InstanceMonitoring is the live view of an instance: its sessions (with
 * cancel / terminate), most expensive statements, status counters and
 * configuration.
 */
export function InstanceMonitoring({ instanceId, canKill }: { instanceId: string; canKill: boolean }) {
  const [tab, setTab] = useState<Tab>("sessions");
  const panelId = useId();
  const current = TABS.find((t) => t.id === tab)!;
  return (
    <section className="section">
      <SectionHeader title="Monitoring" description="Live activity and configuration, read directly from the server." />
      <Tabs tabs={TABS} value={tab} onChange={setTab} label="Monitoring views" panelId={panelId} />
      <div id={panelId} role="tabpanel" aria-label={current.label} className="mt-3">
        {tab === "sessions" ? <Sessions instanceId={instanceId} canKill={canKill} /> : null}
        {tab === "queries" ? <TopQueriesView instanceId={instanceId} /> : null}
        {tab === "status" ? <SettingsTable useData={useServerStatus} instanceId={instanceId} what="status counters" /> : null}
        {tab === "variables" ? (
          <SettingsTable useData={useServerVariables} instanceId={instanceId} what="server variables" />
        ) : null}
      </div>
    </section>
  );
}

function LoadError({ error, what, onRetry }: { error: unknown; what: string; onRetry: () => void }) {
  return (
    <Callout
      tone="danger"
      title={`Couldn't load ${what}`}
      actions={
        <Button size="sm" onClick={onRetry}>
          Try again
        </Button>
      }
    >
      {friendlyError(error)}
    </Callout>
  );
}

function Sessions({ instanceId, canKill }: { instanceId: string; canKill: boolean }) {
  const { data, isLoading, error, refetch } = useProcesses(instanceId);
  const kill = useKillProcess();
  const [hideIdle, setHideIdle] = useState(true);
  const [target, setTarget] = useState<{ p: Process; connection: boolean } | null>(null);
  const [killError, setKillError] = useState<string | null>(null);

  const rows = (data ?? []).filter((p) => !hideIdle || !/^(sleep|idle)$/i.test(p.state));

  async function confirmKill() {
    if (!target) return;
    setKillError(null);
    try {
      await kill.mutateAsync({ instanceId, pid: target.p.id, connection: target.connection });
      setTarget(null);
    } catch (err) {
      setKillError(err instanceof ApiError ? err.message : "Failed to signal the session");
    }
  }

  if (isLoading) return <TableSkeleton rows={4} columns={canKill ? 7 : 6} />;
  if (error) return <LoadError error={error} what="sessions" onRetry={() => void refetch()} />;

  const total = data?.length ?? 0;
  return (
    <div className="card">
      <Toolbar className="m-0 justify-between px-3 py-2.5">
        <span className="text-sm text-muted">
          {rows.length} of {total} sessions · refreshes every 5s
        </span>
        <Checkbox label="Hide idle" checked={hideIdle} onChange={(e) => setHideIdle(e.target.checked)} />
      </Toolbar>
      {rows.length === 0 ? (
        <EmptyState
          plain
          title="No active sessions"
          hint={
            hideIdle && total > 0
              ? `All ${total} session${total === 1 ? " is" : "s are"} idle. Clear “Hide idle” to see them.`
              : "Nothing is connected to this server right now."
          }
          action={
            hideIdle && total > 0 ? (
              <Button size="sm" onClick={() => setHideIdle(false)}>
                Show idle sessions
              </Button>
            ) : undefined
          }
        />
      ) : (
        <Table bare>
          <thead>
            <tr>
              <th>ID</th>
              <th>User</th>
              <th>Database</th>
              <th>State</th>
              <th className="num">Time</th>
              <th>Query</th>
              {canKill ? (
                <th>
                  <span className="sr-only">Actions</span>
                </th>
              ) : null}
            </tr>
          </thead>
          <tbody>
            {rows.map((p) => (
              <tr key={p.id}>
                <td className="text-muted">
                  <code>{p.id}</code>
                </td>
                <td>
                  {p.user}
                  {p.host ? <span className="text-muted"> @{p.host}</span> : null}
                </td>
                <td>{p.database ?? <span className="text-muted">—</span>}</td>
                <td>{p.state || "—"}</td>
                <td className="num">{formatSeconds(p.seconds)}</td>
                <td className="max-w-105">
                  {p.query ? (
                    <code className="text-sm whitespace-pre-wrap break-words">
                      {p.query.length > 300 ? p.query.slice(0, 300) + "…" : p.query}
                    </code>
                  ) : (
                    <span className="text-muted">—</span>
                  )}
                </td>
                {canKill ? (
                  <td className="text-right whitespace-nowrap">
                    <Button
                      variant="ghost"
                      size="sm"
                      icon={<CircleStop size={15} aria-hidden />}
                      title="Cancel the running statement"
                      onClick={() => setTarget({ p, connection: false })}
                    >
                      Cancel
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      icon={<XCircle size={15} aria-hidden />}
                      title="Terminate the session"
                      onClick={() => setTarget({ p, connection: true })}
                    >
                      Terminate
                    </Button>
                  </td>
                ) : null}
              </tr>
            ))}
          </tbody>
        </Table>
      )}
      <ConfirmModal
        open={target !== null}
        title={target?.connection ? "Terminate session?" : "Cancel statement?"}
        danger
        busy={kill.isPending}
        confirmLabel={target?.connection ? "Terminate session" : "Cancel statement"}
        message={
          <>
            <p className="mt-0">
              {target?.connection
                ? "The client connection will be closed; an open transaction is rolled back."
                : "The running statement is cancelled; the client stays connected."}
            </p>
            <p className="text-muted">
              Session {target?.p.id} · {target?.p.user}
              {target?.p.database ? ` · ${target.p.database}` : ""}
            </p>
            <ErrorText message={killError ?? undefined} />
          </>
        }
        onConfirm={confirmKill}
        onCancel={() => {
          setTarget(null);
          setKillError(null);
        }}
      />
    </div>
  );
}

function SettingsTable({
  useData,
  instanceId,
  what,
}: {
  useData: (id: string) => { data?: Setting[]; isLoading: boolean; error: unknown; refetch: () => unknown };
  instanceId: string;
  what: string;
}) {
  const { data, isLoading, error, refetch } = useData(instanceId);
  const [filter, setFilter] = useState("");
  const f = filter.trim().toLowerCase();
  const rows = (data ?? []).filter((s) => !f || s.name.toLowerCase().includes(f) || s.value.toLowerCase().includes(f));

  if (isLoading) return <TableSkeleton rows={6} columns={2} />;
  if (error) return <LoadError error={error} what={what} onRetry={() => void refetch()} />;

  return (
    <div className="card">
      <Toolbar className="m-0 px-3 py-2.5">
        <SearchInput
          icon={<Search size={15} aria-hidden />}
          className="w-full"
          placeholder="Filter by name or value…"
          aria-label={`Filter ${what}`}
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
        />
      </Toolbar>
      {rows.length === 0 ? (
        <EmptyState
          plain
          title={f ? `Nothing matches “${filter.trim()}”` : `No ${what} reported`}
          hint={f ? "Try a shorter or different filter." : "The server returned an empty list."}
          action={
            f ? (
              <Button size="sm" onClick={() => setFilter("")}>
                Clear filter
              </Button>
            ) : undefined
          }
        />
      ) : (
        <Table bare sticky wrapClassName="max-h-120 overflow-y-auto">
          <thead>
            <tr>
              <th className="w-2/5">Name</th>
              <th>Value</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((s) => (
              <tr key={s.name}>
                <td className="font-medium">
                  <code>{s.name}</code>
                </td>
                <td className="break-all">{s.value}</td>
              </tr>
            ))}
          </tbody>
        </Table>
      )}
    </div>
  );
}

function formatSeconds(s: number): string {
  if (s < 60) return `${s}s`;
  if (s < 3600) return `${Math.floor(s / 60)}m ${s % 60}s`;
  return `${Math.floor(s / 3600)}h ${Math.floor((s % 3600) / 60)}m`;
}
