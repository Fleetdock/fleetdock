"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useEffect, useRef, type ReactNode } from "react";

import { ChevronRight } from "lucide-react";
import { EmptyState, PageHeader, Spinner, StatusBadge } from "@/components/ui";
import { useOperation, useOperationLogs } from "@/lib/hooks";
import { operationLabel, operationTitle, resourceHref } from "@/lib/operations";
import type { Operation, OperationLog } from "@/lib/types";

const TERMINAL = new Set(["succeeded", "failed", "canceled"]);

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
      <PageHeader
        breadcrumbs={[{ label: "Activity", href: "/activity" }, { label: operationTitle(op) }]}
        title={operationTitle(op)}
        badges={<StatusBadge status={op.status} />}
      />

      {op.status === "running" ? <ProgressBar value={op.progress} /> : null}

      <div className="card" style={{ padding: "1.1rem", marginBottom: "1.25rem" }}>
        <dl className="grid" style={{ gridTemplateColumns: "repeat(auto-fit, minmax(180px, 1fr))", gap: "1rem" }}>
          <Detail label="Task" value={operationLabel(op.type)} />
          <DetailNode label="On">{resourceLink(op)}</DetailNode>
          <Detail label="Run by" value={op.server_id ? "The agent on the server" : "Fleetdock"} />
          <Detail label="Progress" value={`${op.progress}%`} />
          <Detail label="Created" value={fmt(op.created_at)} />
          <Detail label="Started" value={op.started_at ? fmt(op.started_at) : "—"} />
          <Detail label="Finished" value={op.completed_at ? fmt(op.completed_at) : "—"} />
          <Detail label="Duration" value={duration(op)} />
        </dl>
      </div>

      {op.status === "failed" && op.error ? (
        <section style={{ marginBottom: "1.25rem" }}>
          <h2 className="font-semibold" style={{ marginBottom: ".6rem" }}>What went wrong</h2>
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
  const label = op.resource_name || `${op.resource_type} ${op.resource_id.slice(0, 8)}`;
  const href = resourceHref(op.resource_type, op.resource_id);
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
