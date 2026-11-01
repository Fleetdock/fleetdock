"use client";

import { Download, Info, X } from "lucide-react";
import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { Suspense, useMemo, useState } from "react";

import { DataTable } from "@/components/data-table";
import {
  Badge,
  Button,
  Callout,
  EmptyState,
  IconButton,
  KeyValueList,
  Modal,
  PageHeader,
  PageSkeleton,
  Select,
  Time,
} from "@/components/ui";
import {
  ACTION_CATEGORIES,
  PERIODS,
  actionLabel,
  actorLabel,
  auditQuery,
  auditResourceHref,
  detailLine,
} from "@/lib/audit";
import { friendlyError } from "@/lib/errors";
import { downloadAuditCSV, useAuditEvents, useCan, useMe, useUsers } from "@/lib/hooks";
import type { AuditEvent } from "@/lib/types";

const OUTCOME: Record<AuditEvent["outcome"], { label: string; tone: "success" | "warning" | "danger" }> = {
  success: { label: "Done", tone: "success" },
  denied: { label: "Denied", tone: "warning" },
  failure: { label: "Failed", tone: "danger" },
};

const KIND_LABEL: Partial<Record<AuditEvent["actor_kind"], string>> = {
  api_token: "API token",
  system: "CLI",
};

export default function AuditLogPage() {
  return (
    <Suspense fallback={<PageSkeleton label="Loading audit log" />}>
      <AuditLog />
    </Suspense>
  );
}

function AuditLog() {
  const can = useCan();
  const { isLoading: meLoading } = useMe();
  if (meLoading) return <PageSkeleton label="Loading audit log" />;
  if (!can("audit:read")) {
    return (
      <EmptyState
        title="You can't see the audit log"
        hint="Reading the audit log needs the audit:read permission, which owners and admins have."
      />
    );
  }
  return <AuditTable />;
}

function AuditTable() {
  const router = useRouter();
  const pathname = usePathname();
  const sp = useSearchParams();
  const spKey = sp.toString();
  const filters = useMemo(() => {
    const q = new URLSearchParams(spKey);
    return {
      actor: q.get("actor") ?? "",
      action: q.get("action") ?? "",
      outcome: q.get("outcome") ?? "",
      period: q.get("period") ?? "7d",
      resource_id: q.get("resource_id") ?? "",
    };
  }, [spKey]);
  const resourceLabel = sp.get("resource") ?? "";
  // Computed once per filter change, so the time bound (and the query key)
  // does not move on every render.
  const query = useMemo(() => auditQuery(filters).toString(), [filters]);

  // Cursor paging: cursors[i] opens page i + 1.
  const [paging, setPaging] = useState<{ query: string; cursors: string[]; page: number }>({ query, cursors: [""], page: 1 });
  const current = paging.query === query ? paging : { query, cursors: [""], page: 1 };
  const { data, isLoading, isFetching, error, refetch } = useAuditEvents(query, current.cursors[current.page - 1] ?? "");
  const [open, setOpen] = useState<AuditEvent | null>(null);
  const [exporting, setExporting] = useState(false);
  const [exportError, setExportError] = useState<string | null>(null);

  const can = useCan();
  const users = useUsers(can("user:read"));
  const userOptions = users.data?.items ?? [];

  const setFilter = (key: string, value: string) => {
    const q = new URLSearchParams(sp.toString());
    if (value) q.set(key, value);
    else q.delete(key);
    if (key === "resource_id") q.delete("resource");
    router.replace(q.size ? `${pathname}?${q.toString()}` : pathname, { scroll: false });
  };

  const onPage = (page: number) => {
    const cursors = [...current.cursors];
    if (page > current.page && data?.next_cursor) cursors[current.page] = data.next_cursor;
    setPaging({ query, cursors, page });
  };

  const exportCSV = async () => {
    setExporting(true);
    setExportError(null);
    try {
      await downloadAuditCSV(query);
    } catch (err) {
      setExportError(friendlyError(err, "The export could not be downloaded."));
    } finally {
      setExporting(false);
    }
  };

  const filtered = Boolean(filters.actor || filters.action || filters.outcome || filters.resource_id);

  return (
    <div>
      <PageHeader
        title="Audit log"
        description="Who did what and when: sign-ins, access changes, changes to servers, databases and backups, and writes to your data. Requests refused for lack of permission are listed too."
        actions={
          <Button
            variant="secondary"
            icon={<Download size={15} aria-hidden />}
            loading={exporting}
            onClick={() => void exportCSV()}
          >
            Export CSV
          </Button>
        }
      />
      {exportError ? (
        <div className="mb-4">
          <Callout tone="danger">{exportError}</Callout>
        </div>
      ) : null}
      {filters.resource_id ? (
        <div className="mb-4">
          <Callout
            tone="info"
            actions={
              <Button size="sm" variant="ghost" icon={<X size={15} aria-hidden />} onClick={() => setFilter("resource_id", "")}>
                Show everything
              </Button>
            }
          >
            Showing events for {resourceLabel ? <strong>{resourceLabel}</strong> : "one resource"} only.
          </Callout>
        </div>
      ) : null}

      <DataTable<AuditEvent>
        columns={[
          {
            id: "time",
            header: "When",
            className: "muted whitespace-nowrap",
            render: (e) => <Time value={e.created_at} />,
          },
          {
            id: "actor",
            header: "Who",
            render: (e) => (
              <span className="inline-flex items-center gap-2 min-w-0">
                <span className="truncate max-w-56" title={actorLabel(e)}>
                  {actorLabel(e)}
                </span>
                {KIND_LABEL[e.actor_kind] ? <Badge tone="neutral">{KIND_LABEL[e.actor_kind]}</Badge> : null}
              </span>
            ),
          },
          {
            id: "action",
            header: "What",
            className: "font-medium",
            render: (e) => (
              <button type="button" className="link-plain text-left" onClick={() => setOpen(e)}>
                {actionLabel(e.action)}
              </button>
            ),
          },
          {
            id: "resource",
            header: "On",
            render: (e) => {
              const href = auditResourceHref(e);
              const name = e.resource_name || (e.resource_id ? e.resource_id.slice(0, 8) : "—");
              const detail = detailLine(e);
              return (
                <span className="block min-w-0">
                  {href ? (
                    <Link href={href} className="link-plain">
                      {name}
                    </Link>
                  ) : (
                    <span>{name}</span>
                  )}
                  {detail ? <span className="block text-xs muted truncate max-w-72">{detail}</span> : null}
                </span>
              );
            },
          },
          {
            id: "outcome",
            header: "Result",
            render: (e) => (
              <Badge tone={OUTCOME[e.outcome].tone} dot>
                {OUTCOME[e.outcome].label}
              </Badge>
            ),
          },
          { id: "ip", header: "IP address", className: "muted mono", hideOnMobile: true, render: (e) => e.ip ?? "—" },
          {
            id: "open",
            header: "",
            align: "right",
            render: (e) => (
              <IconButton label={`Details of ${actionLabel(e.action)}`} onClick={() => setOpen(e)}>
                <Info size={15} aria-hidden />
              </IconButton>
            ),
          },
        ]}
        rows={data?.items ?? []}
        rowKey={(e) => e.id}
        isLoading={isLoading || (isFetching && !data)}
        error={error ? friendlyError(error) : undefined}
        errorTitle="Could not load the audit log"
        onRetry={() => void refetch()}
        emptyTitle={filtered ? "No events match these filters" : "No events in this period"}
        emptyHint={
          filtered
            ? "Try another filter or a longer period."
            : "Sign-ins and changes made through Fleetdock appear here as they happen. Try a longer period."
        }
        toolbar={
          <>
            <Select
              size="sm"
              className="w-44"
              aria-label="Period"
              value={filters.period}
              onChange={(e) => setFilter("period", e.target.value === "7d" ? "" : e.target.value)}
            >
              {PERIODS.map((p) => (
                <option key={p.value} value={p.value}>
                  {p.label}
                </option>
              ))}
            </Select>
            <Select
              size="sm"
              className="w-52"
              aria-label="Kind of action"
              value={filters.action}
              onChange={(e) => setFilter("action", e.target.value)}
            >
              <option value="">All actions</option>
              {ACTION_CATEGORIES.map((g) => (
                <optgroup key={g.group} label={g.group}>
                  {g.options.map((o) => (
                    <option key={o.value} value={o.value}>
                      {o.label}
                    </option>
                  ))}
                </optgroup>
              ))}
            </Select>
            {userOptions.length > 0 ? (
              <Select
                size="sm"
                className="w-52"
                aria-label="User"
                value={filters.actor}
                onChange={(e) => setFilter("actor", e.target.value)}
              >
                <option value="">Everyone</option>
                {userOptions.map((u) => (
                  <option key={u.id} value={u.id}>
                    {u.email}
                  </option>
                ))}
              </Select>
            ) : null}
            <Select
              size="sm"
              className="w-36"
              aria-label="Result"
              value={filters.outcome}
              onChange={(e) => setFilter("outcome", e.target.value)}
            >
              <option value="">Any result</option>
              <option value="success">Done</option>
              <option value="denied">Denied</option>
              <option value="failure">Failed</option>
            </Select>
          </>
        }
        pagination={{
          page: current.page,
          pageCount: current.page,
          onPage,
          hasMore: Boolean(data?.next_cursor),
        }}
      />

      <EventModal event={open} onClose={() => setOpen(null)} />
    </div>
  );
}

function EventModal({ event, onClose }: { event: AuditEvent | null; onClose: () => void }) {
  if (!event) return null;
  const { sql, ...rest } = event.metadata ?? {};
  const href = auditResourceHref(event);
  const name = event.resource_name || event.resource_id || "—";
  return (
    <Modal open onClose={onClose} title={actionLabel(event.action)} wide>
      <div className="stack">
        <KeyValueList
          items={[
            { label: "When", value: <Time value={event.created_at} absolute /> },
            {
              label: "Who",
              value: (
                <>
                  {actorLabel(event)}
                  {KIND_LABEL[event.actor_kind] ? <span className="muted"> · {KIND_LABEL[event.actor_kind]}</span> : null}
                </>
              ),
            },
            { label: "Result", value: OUTCOME[event.outcome].label },
            {
              label: "On",
              value: href ? (
                <Link href={href} className="link">
                  {name}
                </Link>
              ) : (
                name
              ),
            },
            { label: "Kind", value: event.resource_type.replaceAll("_", " ") },
            { label: "Action code", value: event.action, mono: true },
            { label: "IP address", value: event.ip ?? "—", mono: true },
            { label: "Browser or client", value: event.user_agent ?? "—", hidden: !event.user_agent },
            { label: "API token", value: event.token_id ?? "", mono: true, hidden: !event.token_id },
          ]}
        />
        {typeof sql === "string" ? (
          <div>
            <h3 className="panel-title">SQL</h3>
            <pre className="code-block">{sql}</pre>
          </div>
        ) : null}
        {Object.keys(rest).length > 0 ? (
          <div>
            <h3 className="panel-title">Details</h3>
            <pre className="code-block">{JSON.stringify(rest, null, 2)}</pre>
          </div>
        ) : null}
      </div>
      <div className="modal-actions">
        <Button variant="secondary" onClick={onClose}>
          Close
        </Button>
      </div>
    </Modal>
  );
}
