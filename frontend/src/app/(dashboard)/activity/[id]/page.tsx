"use client";

import { ChevronRight, ScrollText } from "lucide-react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useEffect, useRef, type ReactNode } from "react";

import {
  Badge,
  Callout,
  Card,
  CardBody,
  CardHeader,
  DetailSkeleton,
  EmptyState,
  KeyValueList,
  PageHeader,
  QueryError,
  SectionHeader,
  Skeleton,
  StatusBadge,
  Time,
} from "@/components/ui";
import { useOperation, useOperationLogs } from "@/lib/hooks";
import { operationLabel, operationTitle, resourceHref } from "@/lib/operations";
import type { Operation, OperationLog } from "@/lib/types";

const TERMINAL = new Set(["succeeded", "failed", "canceled"]);

export default function OperationDetailPage() {
  const params = useParams();
  const id = String(params.id);
  const { data: op, isLoading, error, refetch } = useOperation(id);
  const running = op ? !TERMINAL.has(op.status) : false;
  const { data: logsData, isLoading: logsLoading } = useOperationLogs(id, running);
  const logs = logsData?.items ?? [];

  if (isLoading) return <DetailSkeleton label="Loading task" />;
  if (!op) {
    return (
      <div>
        <PageHeader breadcrumbs={[{ label: "Activity", href: "/activity" }, { label: "Task" }]} title="Task" />
        <QueryError error={error} what="task" onRetry={() => void refetch()} backHref="/activity" backLabel="Back to Activity" />
      </div>
    );
  }

  return (
    <div>
      <PageHeader
        breadcrumbs={[{ label: "Activity", href: "/activity" }, { label: operationTitle(op) }]}
        title={operationTitle(op)}
        badges={<StatusBadge status={op.status} />}
      />

      {op.status === "running" ? <ProgressBar value={op.progress} /> : null}

      <Card className="mb-5">
        <CardBody>
          <KeyValueList
            items={[
              { label: "Task", value: operationLabel(op.type) },
              { label: "On", value: resourceLink(op) },
              { label: "Run by", value: op.server_id ? "The agent on the server" : "Fleetdock" },
              { label: "Progress", value: <span className="num">{op.progress}%</span> },
              { label: "Created", value: <Time value={op.created_at} absolute /> },
              { label: "Started", value: <Time value={op.started_at} absolute /> },
              { label: "Finished", value: <Time value={op.completed_at} absolute /> },
              { label: "Duration", value: <span className="num">{duration(op)}</span> },
            ]}
          />
        </CardBody>
      </Card>

      {op.status === "failed" && op.error ? (
        <Callout tone="danger" title="What went wrong" className="mb-5">
          <pre className="m-0 whitespace-pre-wrap break-words font-mono text-xs">{op.error}</pre>
        </Callout>
      ) : null}

      <section className="section" aria-labelledby="op-logs-title">
        <SectionHeader
          title={<span id="op-logs-title">Logs</span>}
          actions={
            running ? (
              <Badge tone="accent" dot pulse>
                Live
              </Badge>
            ) : null
          }
        />
        {logsLoading ? (
          <div className="log-view stack-sm" role="status" aria-label="Loading logs">
            {[0, 1, 2, 3].map((i) => (
              <Skeleton key={i} height=".8rem" width={`${85 - i * 13}%`} />
            ))}
          </div>
        ) : (
          <LogViewer logs={logs} follow={running} />
        )}
      </section>

      <div className="grid gap-4 md:grid-cols-2">
        <JsonCard title="Result" value={op.result} />
        <JsonCard title="Parameters" value={op.params} />
      </div>
    </div>
  );
}

function ProgressBar({ value }: { value: number }) {
  const pct = Math.max(0, Math.min(100, value));
  return (
    <div className="progress mb-5" role="progressbar" aria-label="Progress" aria-valuenow={pct} aria-valuemin={0} aria-valuemax={100}>
      {/* Width is the live progress percentage. */}
      <span style={{ width: `${pct}%` }} />
    </div>
  );
}

const LEVEL_CLASS: Record<string, string> = {
  error: "text-danger",
  warn: "text-warning",
};

function LogViewer({ logs, follow }: { logs: OperationLog[]; follow: boolean }) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (follow && ref.current) {
      ref.current.scrollTop = ref.current.scrollHeight;
    }
  }, [logs.length, follow]);

  if (logs.length === 0) {
    return (
      <EmptyState
        icon={<ScrollText size={22} />}
        title={follow ? "Waiting for log lines" : "No logs recorded"}
        hint={follow ? "Lines appear here as soon as the task writes them." : "This task finished without writing to its log."}
      />
    );
  }
  return (
    <div ref={ref} className="log-view" role="log" aria-live={follow ? "polite" : "off"} aria-label="Task log" tabIndex={0}>
      {logs.map((l) => (
        <div key={l.seq} className="flex gap-3">
          <span className="muted shrink-0">{new Date(l.created_at).toLocaleTimeString()}</span>
          <span className={`shrink-0 w-14 uppercase text-(length:--fs-2xs) pt-px ${LEVEL_CLASS[l.level] ?? "text-muted"}`}>{l.level}</span>
          <span className={l.level === "error" ? "text-danger" : undefined}>{l.message}</span>
        </div>
      ))}
    </div>
  );
}

function JsonCard({ title, value }: { title: string; value?: Record<string, unknown> | null }) {
  const has = value && Object.keys(value).length > 0;
  return (
    <Card>
      <CardHeader title={title} />
      <CardBody>
        {has ? (
          <pre className="code-block overflow-x-auto">{JSON.stringify(value, null, 2)}</pre>
        ) : (
          <p className="muted text-sm m-0">Nothing recorded.</p>
        )}
      </CardBody>
    </Card>
  );
}

function resourceLink(op: Operation): ReactNode {
  if (!op.resource_id) return <span className="muted">—</span>;
  const label = op.resource_name || `${op.resource_type} ${op.resource_id.slice(0, 8)}`;
  const href = resourceHref(op.resource_type, op.resource_id);
  if (!href) return <span title={op.resource_id}>{label}</span>;
  return (
    <Link href={href} title={op.resource_id} className="link inline-flex items-center gap-0.5">
      {label} <ChevronRight size={13} aria-hidden />
    </Link>
  );
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
