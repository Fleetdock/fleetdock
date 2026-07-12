"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useEffect, useRef, type ReactNode } from "react";

import { ChevronRight } from "lucide-react";
import { EmptyState, Spinner, StatusBadge } from "@/components/ui";
import { useOperation, useOperationLogs } from "@/lib/hooks";
import type { Operation, OperationLog } from "@/lib/types";

const TERMINAL = new Set(["succeeded", "failed", "canceled"]);

const TYPE_LABEL: Record<string, string> = {
  create_database: "Create database",
  delete_database: "Delete database",
  backup: "Backup",
  restore: "Restore",
  test_connection: "Test connection",
  import_databases: "Import databases",
  provision_instance: "Provision instance",
  start_instance: "Start instance",
  stop_instance: "Stop instance",
  restart_instance: "Restart instance",
  remove_instance: "Remove instance",
};

// Operation resource_type is singular; map it to the detail route.
const RESOURCE_HREF: Record<string, (id: string) => string> = {
  database: (id) => `/databases/${id}`,
  instance: (id) => `/instances/${id}`,
  server: (id) => `/servers/${id}`,
  backup: () => `/backups`,
};

export default function OperationDetailPage() {
  const params = useParams();
  const id = String(params.id);
  const { data: op, isLoading } = useOperation(id);
  const running = op ? !TERMINAL.has(op.status) : false;
  const { data: logsData } = useOperationLogs(id, running);
  const logs = logsData?.items ?? [];

  if (isLoading) {
    return <div className="flex items-center gap-2 muted text-sm"><Spinner /> Loading…</div>;
  }
  if (!op) {
    return <EmptyState title="Operation not found" />;
  }

  return (
    <div>
      <Link href="/operations" className="muted text-sm">← Operations</Link>
      <div className="flex items-center justify-between" style={{ margin: ".6rem 0 1.1rem" }}>
        <div className="flex items-center gap-3">
          <h1 className="text-xl font-semibold">{TYPE_LABEL[op.type] ?? op.type}</h1>
          <StatusBadge status={op.status} />
          <span className="muted text-sm">{op.server_id ? "agent" : "control plane"}</span>
        </div>
      </div>

      {op.status === "running" ? <ProgressBar value={op.progress} /> : null}

      <div className="card" style={{ padding: "1.1rem", marginBottom: "1.25rem" }}>
        <dl className="grid" style={{ gridTemplateColumns: "repeat(auto-fit, minmax(180px, 1fr))", gap: "1rem" }}>
          <Detail label="Type" value={TYPE_LABEL[op.type] ?? op.type} />
          <Detail label="Status" value={op.status} />
          <Detail label="Executor" value={op.server_id ? "agent" : "control plane"} />
          <DetailNode label="Resource">{resourceLink(op)}</DetailNode>
          <Detail label="Progress" value={`${op.progress}%`} />
          <Detail label="Created" value={fmt(op.created_at)} />
          <Detail label="Started" value={op.started_at ? fmt(op.started_at) : "—"} />
          <Detail label="Finished" value={op.completed_at ? fmt(op.completed_at) : "—"} />
          <Detail label="Duration" value={duration(op)} />
        </dl>
      </div>

      {op.status === "failed" && op.error ? (
        <section style={{ marginBottom: "1.25rem" }}>
          <h2 className="font-semibold" style={{ marginBottom: ".6rem" }}>Error</h2>
          <div className="card" style={{ padding: "1rem", borderColor: "var(--danger)" }}>
            <pre style={{ margin: 0, whiteSpace: "pre-wrap", wordBreak: "break-word", color: "var(--danger)", fontSize: ".82rem" }}>
              {op.error}
            </pre>
          </div>
        </section>
      ) : null}

      <section style={{ marginBottom: "1.25rem" }}>
        <h2 className="font-semibold" style={{ marginBottom: ".6rem" }}>
          Logs {running ? <span className="muted text-sm" style={{ fontWeight: 400 }}>· live</span> : null}
        </h2>
        <LogViewer logs={logs} follow={running} />
      </section>

      <div className="grid" style={{ gridTemplateColumns: "repeat(auto-fit, minmax(280px, 1fr))", gap: "1rem" }}>
        <JsonCard title="Result" value={op.result} />
        <JsonCard title="Parameters" value={op.params} />
      </div>
    </div>
  );
}

function Detail({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="muted text-sm">{label}</dt>
      <dd className="font-medium" style={{ margin: 0 }}>{value}</dd>
    </div>
  );
}

function DetailNode({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div>
      <dt className="muted text-sm">{label}</dt>
      <dd className="font-medium" style={{ margin: 0 }}>{children}</dd>
    </div>
  );
}

function ProgressBar({ value }: { value: number }) {
  const pct = Math.max(0, Math.min(100, value));
  return (
    <div style={{ height: 6, background: "var(--panel-2)", borderRadius: 999, overflow: "hidden", marginBottom: "1.25rem" }}>
      <div style={{ width: `${pct}%`, height: "100%", background: "var(--accent)", transition: "width .3s" }} />
    </div>
  );
}

function LogViewer({ logs, follow }: { logs: OperationLog[]; follow: boolean }) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (follow && ref.current) {
      ref.current.scrollTop = ref.current.scrollHeight;
    }
  }, [logs.length, follow]);

  if (logs.length === 0) {
    return (
      <div className="card" style={{ padding: "1rem" }}>
        <span className="muted text-sm">No logs recorded.</span>
      </div>
    );
  }
  return (
    <div
      ref={ref}
      className="card"
      style={{
        padding: ".75rem 1rem",
        maxHeight: 360,
        overflowY: "auto",
        fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace",
        fontSize: ".78rem",
        lineHeight: 1.6,
      }}
    >
      {logs.map((l) => (
        <div key={l.seq} style={{ display: "flex", gap: ".7rem", whiteSpace: "pre-wrap", wordBreak: "break-word" }}>
          <span className="muted" style={{ flexShrink: 0 }}>{new Date(l.created_at).toLocaleTimeString()}</span>
          <span style={{ flexShrink: 0, width: "3.4rem", color: levelColor(l.level), textTransform: "uppercase", fontSize: ".68rem", paddingTop: ".08rem" }}>
            {l.level}
          </span>
          <span style={{ color: l.level === "error" ? "var(--danger)" : "inherit" }}>{l.message}</span>
        </div>
      ))}
    </div>
  );
}

function levelColor(level: string): string {
  if (level === "error") return "var(--danger)";
  if (level === "warn") return "var(--warning)";
  return "var(--muted)";
}

function JsonCard({ title, value }: { title: string; value?: Record<string, unknown> | null }) {
  const has = value && Object.keys(value).length > 0;
  return (
    <section>
      <h2 className="font-semibold" style={{ marginBottom: ".6rem" }}>{title}</h2>
      <div className="card" style={{ padding: ".75rem 1rem", overflowX: "auto" }}>
        {has ? (
          <pre style={{ margin: 0, fontSize: ".78rem", lineHeight: 1.5 }}>{JSON.stringify(value, null, 2)}</pre>
        ) : (
          <span className="muted text-sm">—</span>
        )}
      </div>
    </section>
  );
}

function resourceLink(op: Operation): ReactNode {
  if (!op.resource_id) return <span className="muted">—</span>;
  const label = `${op.resource_type} · ${op.resource_id.slice(0, 8)}`;
  const href = RESOURCE_HREF[op.resource_type]?.(op.resource_id);
  if (!href) return <span title={op.resource_id}>{label}</span>;
  return (
    <Link href={href} title={op.resource_id} style={{ color: "var(--accent)", display: "inline-flex", alignItems: "center", gap: ".15rem" }}>
      {label} <ChevronRight size={13} />
    </Link>
  );
}

function fmt(s: string): string {
  return new Date(s).toLocaleString();
}

function duration(op: Operation): string {
  const start = op.started_at ?? op.created_at;
  if (!start) return "—";
  const from = new Date(start).getTime();
  const to = op.completed_at ? new Date(op.completed_at).getTime() : Date.now();
  const ms = to - from;
  if (ms < 0) return "—";
  const secs = Math.round(ms / 1000);
  if (secs < 60) return `${secs}s`;
  const m = Math.floor(secs / 60);
  return `${m}m ${secs % 60}s`;
}
