"use client";

import Link from "next/link";

import { AlertOctagon, AlertTriangle, CheckCircle2, ChevronRight } from "lucide-react";

import { Time } from "@/components/ui";
import { operationLabel, resourceHref } from "@/lib/operations";
import type { AttentionItem } from "@/lib/types";

const TITLE: Record<AttentionItem["kind"], string> = {
  server_offline: "Server offline",
  instance_unreachable: "Database server unreachable",
  backup_failed: "Backup failed",
  backup_check_failed: "Backup check failed",
  no_recent_backup: "No recent backup",
  database_missing: "Database not found on server",
  operation_failed: "Task failed",
};

/** AttentionList shows the problems that need someone to look at them. */
export function AttentionList({ items }: { items: AttentionItem[] }) {
  if (items.length === 0) {
    return (
      <div className="card attention-ok" role="status">
        <CheckCircle2 size={20} aria-hidden />
        <div>
          <div className="font-medium">Everything looks good</div>
          <div className="muted text-sm">No offline servers, failed backups or other problems right now.</div>
        </div>
      </div>
    );
  }
  return (
    <ul className="card attention-list" aria-label="Problems that need attention">
      {items.map((a) => {
        const href = resourceHref(a.resource_type, a.resource_id);
        const name = a.kind === "operation_failed" ? operationLabel(a.name) : a.name;
        const body = (
          <>
            {a.severity === "critical" ? (
              <AlertOctagon size={18} className="attention-critical" aria-label="Critical" />
            ) : (
              <AlertTriangle size={18} className="attention-warning" aria-label="Warning" />
            )}
            <div style={{ flex: 1, minWidth: 0 }}>
              <div>
                <span className="font-medium">{TITLE[a.kind]}</span>
                <span className="muted"> · {name}</span>
              </div>
              <div className="muted text-sm truncate" title={a.message}>
                {a.message}
              </div>
            </div>
            <span className="muted text-sm hide-sm">
              <Time value={a.since} />
            </span>
            {href ? <ChevronRight size={16} className="muted" aria-hidden /> : null}
          </>
        );
        return (
          <li key={`${a.kind}:${a.resource_id}`}>
            {href ? (
              <Link href={href} className="attention-row">
                {body}
              </Link>
            ) : (
              <div className="attention-row">{body}</div>
            )}
          </li>
        );
      })}
    </ul>
  );
}
