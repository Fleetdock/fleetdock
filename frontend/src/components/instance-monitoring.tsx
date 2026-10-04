"use client";

import { useMemo, useState } from "react";

import { CircleStop, XCircle } from "lucide-react";
import { ConfirmModal, EmptyState, ErrorText, Spinner } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { useKillProcess, useProcesses, useServerStatus, useServerVariables } from "@/lib/hooks";
import type { InstanceHealth, Process, Setting } from "@/lib/types";

type Tab = "sessions" | "status" | "variables";

/** HealthBadge shows the latest result of the control plane's periodic probe. */
export function HealthBadge({ health }: { health?: InstanceHealth | null }) {
  if (!health) return <span className="badge badge-gray" title="Not probed yet">health: —</span>;
  const cls =
    health.status === "healthy" ? "badge-green" : health.status === "unreachable" ? "badge-red" : "badge-gray";
  const when = new Date(health.checked_at).toLocaleTimeString();
  const title =
    health.status === "healthy"
      ? `${health.version ?? ""} · ${health.latency_ms} ms · checked ${when}`
      : `${health.error ?? health.status} · checked ${when}`;
  return (
    <span className={`badge ${cls}`} title={title}>
      {health.status === "healthy" ? `healthy · ${health.latency_ms} ms` : health.status}
    </span>
  );
}

/**
 * InstanceMonitoring is the live view of an instance: its sessions (with
 * cancel / terminate), status counters and configuration.
 */
export function InstanceMonitoring({ instanceId, canKill }: { instanceId: string; canKill: boolean }) {
  const [tab, setTab] = useState<Tab>("sessions");
  return (
    <div style={{ marginBottom: "1.25rem" }}>
      <div className="flex items-center justify-between" style={{ marginBottom: ".6rem" }}>
        <h2 className="font-semibold">Monitoring</h2>
        <div className="flex gap-2" role="tablist">
          {(["sessions", "status", "variables"] as Tab[]).map((t) => (
            <button
              key={t}
              role="tab"
              aria-selected={tab === t}
              className={`btn btn-sm ${tab === t ? "btn-primary" : "btn-ghost"}`}
              onClick={() => setTab(t)}
            >
              {t[0].toUpperCase() + t.slice(1)}
            </button>
          ))}
        </div>
      </div>
      {tab === "sessions" ? <Sessions instanceId={instanceId} canKill={canKill} /> : null}
      {tab === "status" ? <SettingsTable useData={useServerStatus} instanceId={instanceId} /> : null}
      {tab === "variables" ? <SettingsTable useData={useServerVariables} instanceId={instanceId} /> : null}
    </div>
  );
}

function Sessions({ instanceId, canKill }: { instanceId: string; canKill: boolean }) {
  const { data, isLoading, error } = useProcesses(instanceId);
  const kill = useKillProcess();
  const [hideIdle, setHideIdle] = useState(true);
  const [target, setTarget] = useState<{ p: Process; connection: boolean } | null>(null);
  const [killError, setKillError] = useState<string | null>(null);

  const rows = useMemo(
    () => (data ?? []).filter((p) => !hideIdle || !/^(sleep|idle)$/i.test(p.state)),
    [data, hideIdle],
  );

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

  if (isLoading) return <div className="flex items-center gap-2 muted text-sm"><Spinner /> Loading sessions…</div>;
  if (error) return <ErrorText message={error instanceof ApiError ? error.message : "Failed to load sessions"} />;

  return (
    <div className="card" style={{ overflowX: "auto" }}>
      <div className="flex items-center justify-between" style={{ padding: ".6rem .8rem" }}>
        <span className="muted text-sm">{rows.length} of {data?.length ?? 0} sessions · refreshes every 5s</span>
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" checked={hideIdle} onChange={(e) => setHideIdle(e.target.checked)} /> Hide idle
        </label>
      </div>
      {rows.length === 0 ? (
        <EmptyState title="No active sessions" />
      ) : (
        <table className="table">
          <thead>
            <tr>
              <th>ID</th>
              <th>User</th>
              <th>Database</th>
              <th>State</th>
              <th style={{ textAlign: "right" }}>Time</th>
              <th>Query</th>
              {canKill ? <th /> : null}
            </tr>
          </thead>
          <tbody>
            {rows.map((p) => (
              <tr key={p.id}>
                <td className="muted"><code>{p.id}</code></td>
                <td>{p.user}{p.host ? <span className="muted"> @{p.host}</span> : null}</td>
                <td>{p.database ?? <span className="muted">—</span>}</td>
                <td>{p.state || "—"}</td>
                <td style={{ textAlign: "right" }}>{formatSeconds(p.seconds)}</td>
                <td style={{ maxWidth: 420 }}>
                  {p.query ? (
                    <code className="text-sm" style={{ whiteSpace: "pre-wrap", wordBreak: "break-word" }}>
                      {p.query.length > 300 ? p.query.slice(0, 300) + "…" : p.query}
                    </code>
                  ) : (
                    <span className="muted">—</span>
                  )}
                </td>
                {canKill ? (
                  <td style={{ whiteSpace: "nowrap", textAlign: "right" }}>
                    <button className="btn btn-ghost btn-sm" title="Cancel the running statement"
                      onClick={() => setTarget({ p, connection: false })}>
                      <CircleStop size={15} /> Cancel
                    </button>
                    <button className="btn btn-ghost btn-sm" title="Terminate the session"
                      onClick={() => setTarget({ p, connection: true })}>
                      <XCircle size={15} /> Terminate
                    </button>
                  </td>
                ) : null}
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <ConfirmModal
        open={target !== null}
        title={target?.connection ? "Terminate session?" : "Cancel statement?"}
        danger
        busy={kill.isPending}
        confirmLabel={target?.connection ? "Terminate session" : "Cancel statement"}
        message={
          <>
            <p style={{ marginTop: 0 }}>
              {target?.connection
                ? "The client connection will be closed; an open transaction is rolled back."
                : "The running statement is cancelled; the client stays connected."}
            </p>
            <p className="muted">Session {target?.p.id} · {target?.p.user}{target?.p.database ? ` · ${target.p.database}` : ""}</p>
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
}: {
  useData: (id: string) => { data?: Setting[]; isLoading: boolean; error: unknown };
  instanceId: string;
}) {
  const { data, isLoading, error } = useData(instanceId);
  const [filter, setFilter] = useState("");
  const rows = useMemo(() => {
    const f = filter.trim().toLowerCase();
    return (data ?? []).filter((s) => !f || s.name.toLowerCase().includes(f) || s.value.toLowerCase().includes(f));
  }, [data, filter]);

  if (isLoading) return <div className="flex items-center gap-2 muted text-sm"><Spinner /> Loading…</div>;
  if (error) return <ErrorText message={error instanceof ApiError ? error.message : "Failed to load"} />;

  return (
    <div className="card">
      <div style={{ padding: ".6rem .8rem" }}>
        <input className="input" placeholder="Filter by name or value…" value={filter} onChange={(e) => setFilter(e.target.value)} />
      </div>
      <div style={{ maxHeight: 480, overflowY: "auto" }}>
        <table className="table">
          <tbody>
            {rows.map((s) => (
              <tr key={s.name}>
                <td className="font-medium" style={{ width: "40%" }}><code>{s.name}</code></td>
                <td style={{ wordBreak: "break-all" }}>{s.value}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function formatSeconds(s: number): string {
  if (s < 60) return `${s}s`;
  if (s < 3600) return `${Math.floor(s / 60)}m ${s % 60}s`;
  return `${Math.floor(s / 3600)}h ${Math.floor((s % 3600) / 60)}m`;
}
